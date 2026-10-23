package console

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/russ-p/paseka/internal/trailview"
)

func (a *api) handleTrails(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	collection, err := ListTrails(r.Context(), a.ctx)
	if err != nil {
		writeTrailError(w, err)
		return
	}
	writeJSON(w, collection)
}

func (a *api) handleTrailByID(w http.ResponseWriter, r *http.Request) {
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/trails/"), "/")
	if path == "" {
		http.NotFound(w, r)
		return
	}
	parts := strings.Split(path, "/")
	traceID := parts[0]
	switch {
	case len(parts) == 1:
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		trail, ok, err := trailview.LoadTrail(a.ctx, traceID)
		if err != nil {
			writeTrailError(w, err)
			return
		}
		if !ok {
			http.NotFound(w, r)
			return
		}
		writeJSON(w, trail)
	case len(parts) == 2 && parts[1] == "config":
		a.handleTrailConfig(w, r, traceID, "read")
	case len(parts) == 3 && parts[1] == "config" && parts[2] == "apply":
		a.handleTrailConfig(w, r, traceID, "apply")
	case len(parts) == 3 && parts[1] == "config" && parts[2] == "cancel":
		a.handleTrailConfig(w, r, traceID, "cancel")
	case len(parts) == 2 && parts[1] == "forks":
		a.handleTrailForks(w, r, traceID)
	case len(parts) == 2 && parts[1] == "fork-requests":
		a.handleTrailForkRequests(w, r, traceID)
	default:
		http.NotFound(w, r)
	}
}

func (a *api) handleTrailConfig(w http.ResponseWriter, r *http.Request, traceID, action string) {
	switch action {
	case "read":
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		manifest, err := GetTrailConfig(r.Context(), a.ctx, traceID)
		if err != nil {
			writeTrailError(w, err)
			return
		}
		writeJSON(w, manifest)
	case "apply":
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req TrailConfigApplyRequest
		if err := decodeOptionalJSON(r, &req); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		res, err := ApplyTrailConfig(r.Context(), a.ctx, traceID, req)
		if err != nil {
			writeTrailError(w, err)
			return
		}
		writeJSON(w, res)
	case "cancel":
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req TrailConfigCancelRequest
		if err := decodeOptionalJSON(r, &req); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		manifest, err := CancelTrailConfig(r.Context(), a.ctx, traceID, req)
		if err != nil {
			writeTrailError(w, err)
			return
		}
		writeJSON(w, manifest)
	}
}

func (a *api) handleTrailForks(w http.ResponseWriter, r *http.Request, traceID string) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	forks, err := ListForksOf(r.Context(), a.ctx, traceID)
	if err != nil {
		writeTrailError(w, err)
		return
	}
	writeJSON(w, forks)
}

func (a *api) handleTrailForkRequests(w http.ResponseWriter, r *http.Request, traceID string) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	requests, err := ListForkRequests(r.Context(), a.ctx, traceID, false)
	if err != nil {
		writeTrailError(w, err)
		return
	}
	writeJSON(w, requests)
}

func (a *api) handleForkRequests(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	includeDecided := false
	if raw := strings.TrimSpace(r.URL.Query().Get("all")); raw != "" {
		if parsed, err := strconv.ParseBool(raw); err == nil {
			includeDecided = parsed
		}
	}
	requests, err := ListForkRequests(r.Context(), a.ctx, r.URL.Query().Get("source"), includeDecided)
	if err != nil {
		writeTrailError(w, err)
		return
	}
	writeJSON(w, requests)
}

func (a *api) handleForkRequestByID(w http.ResponseWriter, r *http.Request) {
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/fork-requests/"), "/")
	if path == "" {
		http.NotFound(w, r)
		return
	}
	parts := strings.Split(path, "/")
	requestID := parts[0]
	action := ""
	if len(parts) > 1 {
		action = parts[1]
	}
	if len(parts) > 2 {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Action string `json:"action"`
		Reason string `json:"reason"`
	}
	if err := decodeOptionalJSON(r, &req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	if action == "" {
		action = strings.TrimSpace(req.Action)
	}
	var (
		decision ForkDecision
		err      error
	)
	switch action {
	case "approve":
		decision, err = ApproveForkRequest(r.Context(), a.ctx, requestID, req.Reason)
	case "reject":
		decision, err = RejectForkRequest(r.Context(), a.ctx, requestID, req.Reason)
	case "cancel":
		decision, err = CancelForkRequest(r.Context(), a.ctx, requestID, req.Reason)
	default:
		http.Error(w, "invalid fork request action", http.StatusBadRequest)
		return
	}
	if err != nil {
		writeTrailError(w, err)
		return
	}
	writeJSON(w, decision)
}

// decodeOptionalJSON accepts an absent or empty body as the zero request, so a
// decision that needs no operator reason can be a bare POST.
func decodeOptionalJSON(r *http.Request, dst any) error {
	if r.Body == nil {
		return nil
	}
	err := json.NewDecoder(r.Body).Decode(dst)
	if errors.Is(err, io.EOF) {
		return nil
	}
	return err
}

func writeTrailError(w http.ResponseWriter, err error) {
	msg := err.Error()
	status := http.StatusInternalServerError
	lower := strings.ToLower(msg)
	switch {
	case strings.Contains(lower, "not found"):
		status = http.StatusNotFound
	case strings.Contains(lower, "nats url not configured"):
		status = http.StatusServiceUnavailable
	case strings.Contains(lower, "pinned"),
		strings.Contains(lower, "expected revision"),
		strings.Contains(lower, "another configuration change is pending"),
		strings.Contains(lower, "no pending configuration change"),
		strings.Contains(lower, "cannot be"),
		strings.Contains(lower, "expired"),
		strings.Contains(lower, "already used"):
		status = http.StatusConflict
	case strings.Contains(lower, "required"),
		strings.Contains(lower, "invalid"),
		strings.Contains(lower, "choose exactly one"):
		status = http.StatusBadRequest
	}
	http.Error(w, msg, status)
}
