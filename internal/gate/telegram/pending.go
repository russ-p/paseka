package telegram

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
)

// PendingTask is a /task preview awaiting Confirm.
type PendingTask struct {
	Text    string
	Bee     string
	Intent  string
	Review  string
	Autorun bool
}

// PendingTasks stores in-flight /task previews keyed by short callback ids.
type PendingTasks struct {
	mu    sync.Mutex
	items map[string]PendingTask
}

// NewPendingTasks creates an empty pending-task store.
func NewPendingTasks() *PendingTasks {
	return &PendingTasks{items: make(map[string]PendingTask)}
}

// Put stores a pending task and returns a short id for callback data.
func (p *PendingTasks) Put(task PendingTask) (string, error) {
	id, err := randomPendingID()
	if err != nil {
		return "", err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.items == nil {
		p.items = make(map[string]PendingTask)
	}
	p.items[id] = task
	return id, nil
}

// Take removes and returns a pending task by id.
func (p *PendingTasks) Take(id string) (PendingTask, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	task, ok := p.items[id]
	if ok {
		delete(p.items, id)
	}
	return task, ok
}

func randomPendingID() (string, error) {
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}
