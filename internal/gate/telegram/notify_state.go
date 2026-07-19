package telegram

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/paseka/paseka/internal/colony"
)

// NotifyState tracks which outbound notifications were already delivered (machine-local dedup).
type NotifyState struct {
	mu       sync.Mutex
	path     string
	Notified map[string]string `json:"notified"`
}

// NotifyStatePath returns ~/.config/paseka/<slug>/telegram-notify-state.json.
func NotifyStatePath(slug string) (string, error) {
	homeDir, err := colony.HomeDir(slug)
	if err != nil {
		return "", err
	}
	return filepath.Join(homeDir, "telegram-notify-state.json"), nil
}

// LoadNotifyState reads notify dedup state for a colony slug.
func LoadNotifyState(slug string) (*NotifyState, error) {
	path, err := NotifyStatePath(slug)
	if err != nil {
		return nil, err
	}
	st := &NotifyState{path: path, Notified: make(map[string]string)}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return st, nil
		}
		return nil, fmt.Errorf("telegram gate: read notify state: %w", err)
	}
	if err := json.Unmarshal(data, st); err != nil {
		return nil, fmt.Errorf("telegram gate: parse notify state: %w", err)
	}
	if st.Notified == nil {
		st.Notified = make(map[string]string)
	}
	return st, nil
}

// ShouldNotify reports whether key has not been delivered yet.
func (s *NotifyState) ShouldNotify(key string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.Notified[key]
	return !ok
}

// MarkNotified records that key was delivered and persists state.
func (s *NotifyState) MarkNotified(key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.Notified == nil {
		s.Notified = make(map[string]string)
	}
	s.Notified[key] = "sent"
	return s.saveLocked()
}

func (s *NotifyState) saveLocked() error {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

func inviteNotifyKey(inviteID, status string) string {
	return fmt.Sprintf("invite:%s:%s", inviteID, status)
}

func taskNotifyKey(traceID, taskID, status string) string {
	return fmt.Sprintf("task:%s:%s:%s", traceID, taskID, status)
}

func taskCompletedNotifyKey(traceID, taskID string) string {
	return fmt.Sprintf("task:%s:%s:completed", traceID, taskID)
}
