package console

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/russ-p/paseka/internal/hiveview"
	"github.com/russ-p/paseka/internal/protocol"
)

const (
	chromeSchemaVersion = 1
	chromeTick          = 3 * time.Second
	chromeGitRefresh    = 9 * time.Second
	chromeHeartbeat     = 15 * time.Second
)

// ChromeAttentionView is pending review and invite counts for tab badges.
type ChromeAttentionView struct {
	Reviews  int `json:"reviews"`
	Sessions int `json:"sessions"`
}

// ChromeView is one SSE chrome frame for always-visible Queen Console chrome.
type ChromeView struct {
	SchemaVersion  int                  `json:"schemaVersion"`
	Runtime        hiveview.RuntimeView `json:"runtime"`
	RuntimeError   string               `json:"runtimeError,omitempty"`
	Agents         hiveview.AgentsView  `json:"agents"`
	AgentsError    string               `json:"agentsError,omitempty"`
	Host           SystemView           `json:"host"`
	HostError      string               `json:"hostError,omitempty"`
	Git            *GitPlaqueView       `json:"git,omitempty"`
	GitError       string               `json:"gitError,omitempty"`
	Attention      ChromeAttentionView  `json:"attention"`
	AttentionError string               `json:"attentionError,omitempty"`
}

type chromeHub struct {
	a          *api
	mu         sync.Mutex
	subs       map[chan []byte]struct{}
	running    bool
	lastJSON   []byte
	lastGit    GitPlaqueView
	lastGitErr string
	lastGitAt  time.Time
}

func newChromeHub(a *api) *chromeHub {
	return &chromeHub{
		a:    a,
		subs: make(map[chan []byte]struct{}),
	}
}

func (h *chromeHub) subscribe() (<-chan []byte, func()) {
	ch := make(chan []byte, 4)
	frame := h.ensureFrame()
	h.mu.Lock()
	h.subs[ch] = struct{}{}
	if !h.running {
		h.running = true
		go h.loop()
	}
	h.mu.Unlock()
	if frame != nil {
		select {
		case ch <- frame:
		default:
		}
	}
	return ch, func() {
		h.mu.Lock()
		delete(h.subs, ch)
		h.mu.Unlock()
	}
}

func (h *chromeHub) ensureFrame() []byte {
	h.mu.Lock()
	if h.lastJSON != nil {
		out := h.lastJSON
		h.mu.Unlock()
		return out
	}
	h.mu.Unlock()
	h.publish()
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.lastJSON
}

func (h *chromeHub) loop() {
	ticker := time.NewTicker(chromeTick)
	defer ticker.Stop()
	for range ticker.C {
		h.mu.Lock()
		n := len(h.subs)
		if n == 0 {
			h.running = false
			h.mu.Unlock()
			return
		}
		h.mu.Unlock()
		h.publish()
	}
}

func (h *chromeHub) publish() {
	frame := h.buildJSON()
	h.mu.Lock()
	defer h.mu.Unlock()
	if bytes.Equal(frame, h.lastJSON) {
		return
	}
	h.lastJSON = frame
	for ch := range h.subs {
		select {
		case ch <- frame:
		default:
		}
	}
}

func (h *chromeHub) buildJSON() []byte {
	view := h.buildView()
	data, err := json.Marshal(view)
	if err != nil {
		data = []byte(`{"schemaVersion":1}`)
	}
	return data
}

func (h *chromeHub) buildView() ChromeView {
	a := h.a
	view := ChromeView{SchemaVersion: chromeSchemaVersion}

	if rt, err := hiveview.GetRuntime(a.ctx, a.runtime); err != nil {
		view.RuntimeError = err.Error()
	} else {
		view.Runtime = rt
	}

	if agents, err := hiveview.GetAgents(a.ctx, a.sessions); err != nil {
		view.AgentsError = err.Error()
	} else {
		view.Agents = agents
	}

	sampler := a.sampler
	if sampler == nil {
		sampler = newCPUSampler()
		a.sampler = sampler
	}
	host := snapshotHostPlaque(sampler, a.ctx.ColonyRoot)
	view.Host = host
	if host.Error != "" {
		view.HostError = host.Error
	}

	git, gitErr := h.gitPlaque()
	view.Git = git
	view.GitError = gitErr

	var attErrs []string
	if queue, err := ListReviewQueue(a.ctx); err != nil {
		attErrs = append(attErrs, err.Error())
	} else {
		view.Attention.Reviews = queue.Count
	}
	if invites, err := hiveview.ListInvites(a.ctx, protocol.InviteStatusPending); err != nil {
		attErrs = append(attErrs, err.Error())
	} else {
		view.Attention.Sessions = len(invites)
	}
	if len(attErrs) > 0 {
		view.AttentionError = strings.Join(attErrs, "; ")
	}
	return view
}

func (h *chromeHub) gitPlaque() (*GitPlaqueView, string) {
	now := time.Now()
	h.mu.Lock()
	if !h.lastGitAt.IsZero() && now.Sub(h.lastGitAt) < chromeGitRefresh {
		g, e := h.lastGit, h.lastGitErr
		h.mu.Unlock()
		out := g
		return &out, e
	}
	h.mu.Unlock()

	full, err := GetGit(h.a.ctx.ColonyRoot, h.a.ctx.Slug)
	if err != nil {
		h.mu.Lock()
		defer h.mu.Unlock()
		if !h.lastGitAt.IsZero() {
			g := h.lastGit
			return &g, err.Error()
		}
		return nil, err.Error()
	}
	plaque := GitPlaqueFrom(full)
	h.mu.Lock()
	h.lastGit = plaque
	h.lastGitErr = ""
	h.lastGitAt = now
	h.mu.Unlock()
	out := plaque
	return &out, ""
}

func (a *api) handleChromeStream(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	ch, unsub := a.chrome.subscribe()
	defer unsub()

	ping := time.NewTicker(chromeHeartbeat)
	defer ping.Stop()

	writeChrome := func(payload []byte) error {
		if _, err := fmt.Fprintf(w, "event: chrome\ndata: %s\n\n", payload); err != nil {
			return err
		}
		flusher.Flush()
		return nil
	}

	for {
		select {
		case <-r.Context().Done():
			return
		case payload, ok := <-ch:
			if !ok {
				return
			}
			if err := writeChrome(payload); err != nil {
				return
			}
		case <-ping.C:
			if _, err := fmt.Fprint(w, ": ping\n\n"); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}
