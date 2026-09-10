package forge

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	// ProtocolVersion is the v1 forge IPC version.
	ProtocolVersion = 1
	// DefaultTimeout bounds one forge child invocation.
	DefaultTimeout = 60 * time.Second
	maxStdout      = 64 * 1024
	maxStderr      = 8 * 1024
)

const (
	OpCapabilities = "capabilities"
	OpUpsert       = "upsert"
	OpGet          = "get"
)

const (
	StateOpen   = "open"
	StateMerged = "merged"
	StateClosed = "closed"
)

// Request is stdin JSON for one forge op.
type Request struct {
	ProtocolVersion int    `json:"protocolVersion"`
	Op              string `json:"op"`
	Head            string `json:"head,omitempty"`
	Base            string `json:"base,omitempty"`
	Title           string `json:"title,omitempty"`
	Body            string `json:"body,omitempty"`
	Draft           bool   `json:"draft,omitempty"`
	TraceID         string `json:"traceId,omitempty"`
	Origin          string `json:"origin,omitempty"`
}

// Response is the success stdout document for upsert/get.
type Response struct {
	ProtocolVersion int    `json:"protocolVersion"`
	Found           bool   `json:"found"`
	Number          int    `json:"number,omitempty"`
	URL             string `json:"url,omitempty"`
	Head            string `json:"head,omitempty"`
	Base            string `json:"base,omitempty"`
	State           string `json:"state,omitempty"`
	Draft           bool   `json:"draft,omitempty"`
}

// Capabilities is stdout for the capabilities op.
type Capabilities struct {
	ProtocolVersion int      `json:"protocolVersion"`
	Ops             []string `json:"ops"`
}

// InvokeOpts configures one forge child.
type InvokeOpts struct {
	Command    []string
	ColonyRoot string
	TraceID    string
	Timeout    time.Duration
}

// CheckCommand reports whether forge.command can be executed (absolute, colony-relative, or PATH).
func CheckCommand(command []string, colonyRoot string) error {
	_, err := resolveArgv(command, colonyRoot)
	return err
}

// Invoke runs <command...> <op> with JSON stdin and a single JSON object on stdout.
func Invoke(ctx context.Context, opts InvokeOpts, req Request) (Response, error) {
	if len(opts.Command) == 0 {
		return Response{}, fmt.Errorf("forge: command is not configured")
	}
	op := strings.TrimSpace(req.Op)
	if op == "" {
		return Response{}, fmt.Errorf("forge: op is required")
	}
	req.ProtocolVersion = ProtocolVersion
	req.Op = op

	argv, err := resolveArgv(opts.Command, opts.ColonyRoot)
	if err != nil {
		return Response{}, err
	}
	full := append(append([]string{}, argv...), op)

	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	payload, err := json.Marshal(req)
	if err != nil {
		return Response{}, fmt.Errorf("forge: encode request: %w", err)
	}

	cmd := exec.CommandContext(runCtx, full[0], full[1:]...)
	if opts.ColonyRoot != "" {
		cmd.Dir = opts.ColonyRoot
	}
	cmd.Stdin = bytes.NewReader(payload)
	cmd.Env = append(os.Environ(),
		"PASEKA_FORGE_OP="+op,
		"PASEKA_FORGE_COLONY_ROOT="+opts.ColonyRoot,
		"PASEKA_FORGE_TRACE_ID="+strings.TrimSpace(opts.TraceID),
		"PASEKA_FORGE_HEAD="+strings.TrimSpace(req.Head),
		"PASEKA_FORGE_BASE="+strings.TrimSpace(req.Base),
	)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	runErr := cmd.Run()
	stderrText := capText(stderr.Bytes(), maxStderr)
	if runCtx.Err() == context.DeadlineExceeded {
		return Response{}, fmt.Errorf("forge: %s timed out: %s", op, stderrText)
	}
	if runErr != nil {
		return Response{}, fmt.Errorf("forge: %s failed: %w: %s", op, runErr, stderrText)
	}

	if op == OpCapabilities {
		var caps Capabilities
		if err := decodeOneJSON(stdout.Bytes(), &caps); err != nil {
			return Response{}, fmt.Errorf("forge: capabilities stdout: %w", err)
		}
		if caps.ProtocolVersion != ProtocolVersion {
			return Response{}, fmt.Errorf("forge: unsupported protocolVersion %d", caps.ProtocolVersion)
		}
		return Response{ProtocolVersion: ProtocolVersion, Found: true}, nil
	}

	var resp Response
	if err := decodeOneJSON(stdout.Bytes(), &resp); err != nil {
		return Response{}, fmt.Errorf("forge: %s stdout: %w", op, err)
	}
	if err := validateResponse(op, resp); err != nil {
		return Response{}, err
	}
	return resp, nil
}

func validateResponse(op string, resp Response) error {
	if resp.ProtocolVersion != ProtocolVersion {
		return fmt.Errorf("forge: unsupported protocolVersion %d", resp.ProtocolVersion)
	}
	if op == OpGet && !resp.Found {
		return nil
	}
	if op == OpUpsert && !resp.Found {
		return fmt.Errorf("forge: upsert returned found: false")
	}
	if !resp.Found {
		return nil
	}
	if strings.TrimSpace(resp.URL) == "" {
		return fmt.Errorf("forge: url is required when found is true")
	}
	if strings.TrimSpace(resp.Head) == "" {
		return fmt.Errorf("forge: head is required when found is true")
	}
	switch strings.TrimSpace(resp.State) {
	case StateOpen, StateMerged, StateClosed:
	default:
		return fmt.Errorf("forge: state must be open, merged, or closed")
	}
	return nil
}

func decodeOneJSON(raw []byte, dest any) error {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return fmt.Errorf("empty stdout")
	}
	if len(trimmed) > maxStdout {
		trimmed = trimmed[:maxStdout]
	}
	dec := json.NewDecoder(bytes.NewReader(trimmed))
	if err := dec.Decode(dest); err != nil {
		return fmt.Errorf("must be a single JSON object: %w", err)
	}
	if dec.More() {
		return fmt.Errorf("stdout must be a single JSON object")
	}
	return nil
}

func resolveArgv(command []string, colonyRoot string) ([]string, error) {
	if len(command) == 0 || strings.TrimSpace(command[0]) == "" {
		return nil, fmt.Errorf("forge: command is not configured")
	}
	out := append([]string{}, command...)
	bin := strings.TrimSpace(out[0])
	if filepath.IsAbs(bin) {
		if err := requireExecutable(bin); err != nil {
			return nil, err
		}
		out[0] = bin
		return out, nil
	}
	if colonyRoot != "" {
		candidate := filepath.Join(colonyRoot, bin)
		if err := requireExecutable(candidate); err == nil {
			out[0] = candidate
			return out, nil
		}
	}
	if looked, err := exec.LookPath(bin); err == nil {
		out[0] = looked
		return out, nil
	}
	return nil, fmt.Errorf("forge: command %q is not executable", bin)
}

func requireExecutable(path string) error {
	st, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("forge: command %q is not executable", path)
	}
	if st.IsDir() {
		return fmt.Errorf("forge: command %q is a directory", path)
	}
	if st.Mode()&0o111 == 0 {
		return fmt.Errorf("forge: command %q is not executable", path)
	}
	return nil
}

func capText(b []byte, max int) string {
	s := strings.TrimSpace(string(b))
	if max > 0 && len(s) > max {
		return s[:max] + "\n...[truncated]"
	}
	return s
}
