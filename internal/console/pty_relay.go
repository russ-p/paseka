package console

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"

	"github.com/coder/websocket"
)

type ptyClientMessage struct {
	Type string `json:"type"`
	Cols uint16 `json:"cols"`
	Rows uint16 `json:"rows"`
}

type ptyStatusMessage struct {
	Type   string `json:"type"`
	State  string `json:"state"`
	Reason string `json:"reason,omitempty"`
}

func (a *api) handleSessionPTY(w http.ResponseWriter, r *http.Request, sessionID string) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	conn, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close(websocket.StatusNormalClosure, "")

	stream, err := a.sessions.AttachPTY(sessionID)
	if err != nil {
		status, _ := json.Marshal(ptyStatusMessage{
			Type:   "status",
			State:  "exited",
			Reason: err.Error(),
		})
		_ = conn.Write(r.Context(), websocket.MessageText, status)
		conn.Close(websocket.StatusPolicyViolation, err.Error())
		return
	}
	defer stream.Close()

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	// Send scrollback catch-up.
	if scroll := stream.Scrollback(); len(scroll) > 0 {
		if err := conn.Write(ctx, websocket.MessageBinary, scroll); err != nil {
			return
		}
	}

	var writeMu sync.Mutex
	writeMsg := func(msgType websocket.MessageType, data []byte) error {
		writeMu.Lock()
		defer writeMu.Unlock()
		return conn.Write(ctx, msgType, data)
	}

	// PTY output → WebSocket.
	go func() {
		output := stream.Output()
		for {
			select {
			case <-ctx.Done():
				return
			case chunk, ok := <-output:
				if !ok {
					// PTY EOF can precede session finalization. Keep the relay
					// alive until Done closes so the browser receives the
					// terminal status message instead of reconnecting.
					output = nil
					continue
				}
				if err := writeMsg(websocket.MessageBinary, chunk); err != nil {
					cancel()
					return
				}
			case <-stream.Done():
				status, _ := json.Marshal(ptyStatusMessage{
					Type:  "status",
					State: "exited",
				})
				_ = writeMsg(websocket.MessageText, status)
				cancel()
				return
			}
		}
	}()

	writeInput := func(data []byte) bool {
		if len(data) == 0 {
			return true
		}
		if err := stream.Write(data); err != nil {
			status, _ := json.Marshal(ptyStatusMessage{
				Type:   "status",
				State:  "exited",
				Reason: err.Error(),
			})
			_ = writeMsg(websocket.MessageText, status)
			return false
		}
		return true
	}

	// WebSocket → PTY input / resize.
	for {
		msgType, data, err := conn.Read(ctx)
		if err != nil {
			return
		}
		switch msgType {
		case websocket.MessageBinary:
			if !writeInput(data) {
				return
			}
		case websocket.MessageText:
			// Control messages are JSON (e.g. resize). Raw text is keyboard
			// input from clients that send strings instead of binary frames.
			var msg ptyClientMessage
			if err := json.Unmarshal(data, &msg); err == nil && msg.Type != "" {
				if msg.Type == "resize" && msg.Cols > 0 && msg.Rows > 0 {
					_ = stream.Resize(msg.Cols, msg.Rows)
				}
				continue
			}
			if !writeInput(data) {
				return
			}
		default:
			continue
		}
	}
}

func parseSessionPTYPath(path string) (sessionID string, ok bool) {
	path = strings.Trim(path, "/")
	if !strings.HasSuffix(path, "/pty") {
		return "", false
	}
	sessionID = strings.TrimSuffix(path, "/pty")
	sessionID = strings.Trim(sessionID, "/")
	if sessionID == "" {
		return "", false
	}
	return sessionID, true
}
