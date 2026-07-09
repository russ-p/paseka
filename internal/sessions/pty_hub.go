package sessions

import (
	"errors"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/creack/pty"
	"github.com/paseka/paseka/internal/runs"
)

const defaultScrollbackSize = 512 * 1024

// PTYStream is a subscription to live PTY output for one browser or relay client.
type PTYStream struct {
	hub         *ptyHub
	sessionDone <-chan struct{}
	outputCh    chan []byte
	closed      chan struct{}
	closeOnce   sync.Once
	subscriber  *ptySubscriber
}

// Output returns a channel of raw PTY output bytes (ANSI preserved).
func (s *PTYStream) Output() <-chan []byte {
	return s.outputCh
}

// Write sends raw input bytes to the PTY master.
func (s *PTYStream) Write(data []byte) error {
	if s.hub == nil {
		return fmt.Errorf("sessions: pty stream closed")
	}
	return s.hub.write(data)
}

// Resize sets the PTY window size.
func (s *PTYStream) Resize(cols, rows uint16) error {
	if s.hub == nil {
		return fmt.Errorf("sessions: pty stream closed")
	}
	return s.hub.resize(cols, rows)
}

// Scrollback returns a copy of the recent raw PTY output ring buffer.
func (s *PTYStream) Scrollback() []byte {
	if s.hub == nil {
		return nil
	}
	return s.hub.scrollbackCopy()
}

// Close unsubscribes from PTY output without stopping the session.
func (s *PTYStream) Close() error {
	s.closeOnce.Do(func() {
		if s.hub != nil && s.subscriber != nil {
			s.hub.unsubscribe(s.subscriber)
		}
		close(s.closed)
	})
	return nil
}

// Done is closed when session artifacts are fully written and the session is finished.
func (s *PTYStream) Done() <-chan struct{} {
	if s.sessionDone != nil {
		return s.sessionDone
	}
	ch := make(chan struct{})
	close(ch)
	return ch
}

type ptySubscriber struct {
	id int
	ch chan []byte
}

type ptyHub struct {
	process    *ptyProcess
	runDir     runs.Dir
	scrollback *byteRing
	closeOnce  sync.Once

	mu          sync.Mutex
	subscribers map[int]*ptySubscriber
	nextSubID   int
	closed      bool
}

func newPTYHub(proc *ptyProcess, runDir runs.Dir) *ptyHub {
	return &ptyHub{
		process:     proc,
		runDir:      runDir,
		scrollback:  newByteRing(defaultScrollbackSize),
		subscribers: make(map[int]*ptySubscriber),
	}
}

func (h *ptyHub) start() {
	go h.readLoop()
}

func (h *ptyHub) readLoop() {
	defer h.close()

	buf := make([]byte, 4096)
	for {
		n, err := h.process.ReadWriteCloser().Read(buf)
		if n > 0 {
			chunk := append([]byte(nil), buf[:n]...)
			h.broadcast(chunk)
			h.appendTranscript(chunk)
		}
		if err != nil {
			if err != io.EOF && !errors.Is(err, syscall.EIO) {
				_ = h.runDir.AppendTranscript(runs.TranscriptEntry{
					At:      time.Now().UTC(),
					Role:    "system",
					Content: fmt.Sprintf("pty read error: %v", err),
				})
			}
			return
		}
	}
}

func (h *ptyHub) broadcast(chunk []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return
	}
	h.scrollback.write(chunk)
	for _, sub := range h.subscribers {
		select {
		case sub.ch <- chunk:
		default:
			// Drop if client is slow; scrollback catches up on reconnect.
		}
	}
}

func (h *ptyHub) appendTranscript(chunk []byte) {
	text := runs.NormalizePTYOutput(chunk)
	if text == "" {
		return
	}
	_ = h.runDir.AppendTranscript(runs.TranscriptEntry{
		At:      time.Now().UTC(),
		Role:    "agent",
		Content: text,
	})
}

func (h *ptyHub) subscribe(sessionDone <-chan struct{}) *PTYStream {
	h.mu.Lock()
	defer h.mu.Unlock()

	id := h.nextSubID
	h.nextSubID++
	sub := &ptySubscriber{
		id: id,
		ch: make(chan []byte, 64),
	}
	h.subscribers[id] = sub

	stream := &PTYStream{
		hub:         h,
		sessionDone: sessionDone,
		outputCh:    sub.ch,
		closed:      make(chan struct{}),
		subscriber:  sub,
	}
	return stream
}

func (h *ptyHub) unsubscribe(sub *ptySubscriber) {
	if sub == nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, ok := h.subscribers[sub.id]; !ok {
		return
	}
	delete(h.subscribers, sub.id)
	close(sub.ch)
}

func (h *ptyHub) scrollbackCopy() []byte {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.scrollback.bytes()
}

func (h *ptyHub) write(data []byte) error {
	h.mu.Lock()
	closed := h.closed
	h.mu.Unlock()
	if closed {
		return fmt.Errorf("sessions: pty hub closed")
	}
	_, err := h.process.ReadWriteCloser().Write(data)
	return err
}

func (h *ptyHub) resize(cols, rows uint16) error {
	h.mu.Lock()
	closed := h.closed
	h.mu.Unlock()
	if closed {
		return fmt.Errorf("sessions: pty hub closed")
	}
	return pty.Setsize(h.process.pty, &pty.Winsize{Cols: cols, Rows: rows})
}

func (h *ptyHub) close() {
	h.closeOnce.Do(func() {
		h.mu.Lock()
		h.closed = true
		for id, sub := range h.subscribers {
			close(sub.ch)
			delete(h.subscribers, id)
		}
		h.mu.Unlock()
	})
}

type byteRing struct {
	buf  []byte
	size int
	head atomic.Uint64
	len  atomic.Uint64
}

func newByteRing(size int) *byteRing {
	return &byteRing{
		buf:  make([]byte, size),
		size: size,
	}
}

func (r *byteRing) write(p []byte) {
	for _, b := range p {
		pos := int(r.head.Load() % uint64(r.size))
		r.buf[pos] = b
		r.head.Add(1)
		cur := r.len.Load()
		if cur < uint64(r.size) {
			r.len.Add(1)
		}
	}
}

func (r *byteRing) bytes() []byte {
	n := int(r.len.Load())
	if n == 0 {
		return nil
	}
	out := make([]byte, n)
	head := r.head.Load()
	start := head - uint64(n)
	for i := 0; i < n; i++ {
		pos := int((start + uint64(i)) % uint64(r.size))
		out[i] = r.buf[pos]
	}
	return out
}
