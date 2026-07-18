package main

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/paseka/paseka/internal/bus"
	"github.com/paseka/paseka/internal/colony"
	"github.com/paseka/paseka/internal/invites"
	"github.com/paseka/paseka/internal/protocol"
	"github.com/paseka/paseka/internal/sessions"
	"github.com/spf13/cobra"
)

func newInviteCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "invite",
		Short: "Manage Human Gateway session invites",
	}
	cmd.AddCommand(newInviteListCmd())
	cmd.AddCommand(newInviteAcceptCmd())
	cmd.AddCommand(newInviteRejectCmd())
	cmd.AddCommand(newInviteRecordCmd())
	return cmd
}

func newInviteListCmd() *cobra.Command {
	var (
		startDir string
		traceID  string
		status   string
	)
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List session invites",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctxColony, err := colony.ResolveContext(startDir)
			if err != nil {
				return err
			}
			if status == "" {
				status = colony.InviteStatusPending
			}
			entries, err := colony.ListInvites(ctxColony.Slug, status, traceID)
			if err != nil {
				return err
			}
			if len(entries) == 0 {
				fmt.Println("No invites.")
				return nil
			}
			fmt.Printf("%-14s %-14s %-8s %-10s %s\n", "INVITE", "TRACE", "BEE", "STATUS", "TASK")
			for _, e := range entries {
				task := e.Task
				if len(task) > 48 {
					task = task[:45] + "..."
				}
				fmt.Printf("%-14s %-14s %-8s %-10s %s\n", e.InviteID, e.TraceID, e.Bee, e.Status, task)
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&startDir, "path", "C", "", "directory inside the git repository")
	cmd.Flags().StringVar(&traceID, "trace", "", "filter by trace id")
	cmd.Flags().StringVar(&status, "status", "", "filter by status (default: pending)")
	return cmd
}

func newInviteAcceptCmd() *cobra.Command {
	var (
		startDir string
		attach   bool
	)
	cmd := &cobra.Command{
		Use:   "accept <inviteId>",
		Short: "Accept a pending invite and start an interactive session",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctxColony, err := colony.ResolveContext(startDir)
			if err != nil {
				return err
			}
			client, err := bus.ConnectColony(ctxColony, false)
			if err != nil {
				return err
			}
			if client == nil {
				return fmt.Errorf("nats url not configured")
			}
			defer client.Close()

			svc := &invites.Service{
				Colony:   ctxColony,
				Bus:      client,
				Sessions: sessions.DefaultManager,
			}
			res, err := svc.Accept(cmd.Context(), args[0], attach)
			if err != nil {
				return err
			}
			fmt.Printf("Accepted invite %s\n", res.Invite.InviteID)
			fmt.Printf("  trace:   %s\n", res.TraceID)
			fmt.Printf("  session: %s\n", res.SessionID)
			if !attach {
				fmt.Printf("  attach:  paseka session attach %s\n", res.SessionID)
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&startDir, "path", "C", "", "directory inside the git repository")
	cmd.Flags().BoolVar(&attach, "attach", false, "attach terminal to the session (default: detached)")
	return cmd
}

func newInviteRejectCmd() *cobra.Command {
	var (
		startDir string
		deferIt  bool
	)
	cmd := &cobra.Command{
		Use:   "reject <inviteId>",
		Short: "Reject or defer a pending invite",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctxColony, err := colony.ResolveContext(startDir)
			if err != nil {
				return err
			}
			client, err := bus.ConnectColony(ctxColony, false)
			if err != nil {
				return err
			}
			if client == nil {
				return fmt.Errorf("nats url not configured")
			}
			defer client.Close()

			svc := &invites.Service{Colony: ctxColony, Bus: client}
			invite, err := svc.Reject(cmd.Context(), args[0], deferIt)
			if err != nil {
				return err
			}
			fmt.Printf("Invite %s marked %s\n", invite.InviteID, invite.Status)
			return nil
		},
	}
	cmd.Flags().StringVarP(&startDir, "path", "C", "", "directory inside the git repository")
	cmd.Flags().BoolVar(&deferIt, "defer", false, "defer instead of cancel")
	return cmd
}

func newInviteRecordCmd() *cobra.Command {
	var (
		startDir    string
		traceID     string
		fromStdin   bool
		inviteID    string
		bee         string
		intent      string
		body        string
		artifactRef string
	)
	cmd := &cobra.Command{
		Use:   "record",
		Short: "Upsert a pending invite in local state (offline seed)",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctxColony, err := colony.ResolveContext(startDir)
			if err != nil {
				return err
			}
			payload, trace, err := resolveInviteRecordInput(fromStdin, traceID, inviteID, bee, intent, body, artifactRef, cmd.InOrStdin())
			if err != nil {
				return err
			}
			svc := &invites.Service{Colony: ctxColony}
			entry, err := svc.Record(invites.RecordInput{TraceID: trace, Payload: payload})
			if err != nil {
				return err
			}
			fmt.Printf("Recorded invite %s on trace %s\n", entry.InviteID, entry.TraceID)
			return nil
		},
	}
	cmd.Flags().StringVarP(&startDir, "path", "C", "", "directory inside the git repository")
	cmd.Flags().StringVar(&traceID, "trace", "", "flight trail id")
	cmd.Flags().BoolVar(&fromStdin, "stdin", false, "read session.invite JSON from stdin")
	cmd.Flags().StringVar(&inviteID, "invite-id", "", "invite id (generated when omitted)")
	cmd.Flags().StringVar(&bee, "bee", "", "bee role")
	cmd.Flags().StringVar(&intent, "intent", "", "session intent")
	cmd.Flags().StringVar(&body, "body", "", "initial task body text")
	cmd.Flags().StringVar(&artifactRef, "artifact-ref", "", "repo-relative artifact path for handoff invites")
	return cmd
}

func resolveInviteRecordInput(fromStdin bool, traceID, inviteID, bee, intent, body, artifactRef string, stdin io.Reader) (protocol.SessionInvitePayload, string, error) {
	if fromStdin {
		data, err := io.ReadAll(stdin)
		if err != nil {
			return protocol.SessionInvitePayload{}, "", err
		}
		var in protocol.EventInput
		if err := json.Unmarshal(data, &in); err == nil && len(in.Payload) > 0 {
			var payload protocol.SessionInvitePayload
			if err := json.Unmarshal(in.Payload, &payload); err != nil {
				return protocol.SessionInvitePayload{}, "", err
			}
			trace := strings.TrimSpace(in.TraceID)
			if trace == "" {
				trace = strings.TrimSpace(traceID)
			}
			return payload, trace, nil
		}
		var payload protocol.SessionInvitePayload
		if err := json.Unmarshal(data, &payload); err != nil {
			return protocol.SessionInvitePayload{}, "", fmt.Errorf("invite record: invalid stdin JSON")
		}
		return payload, strings.TrimSpace(traceID), nil
	}
	if strings.TrimSpace(bee) == "" || strings.TrimSpace(body) == "" {
		return protocol.SessionInvitePayload{}, "", fmt.Errorf("invite record: --bee and --body are required (or use --stdin)")
	}
	return protocol.SessionInvitePayload{
		Kind:        protocol.SignalSessionInvite,
		InviteID:    strings.TrimSpace(inviteID),
		Bee:         strings.TrimSpace(bee),
		Intent:      strings.TrimSpace(intent),
		Task:        strings.TrimSpace(body),
		Status:      protocol.InviteStatusPending,
		ArtifactRef: strings.TrimSpace(artifactRef),
	}, strings.TrimSpace(traceID), nil
}
