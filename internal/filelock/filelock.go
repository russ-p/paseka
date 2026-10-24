// Package filelock provides an ownership-checked lock file.
//
// A lock records its owning pid and a random nonce. The holder refreshes the
// file's modification time while it is held, so a slow operation is not mistaken
// for an abandoned lock. A waiter may remove a lock only when its owner is no
// longer alive, and a release removes only the lock it still owns.
package filelock

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

// Options configures one lock acquisition.
type Options struct {
	// Budget bounds how long to wait for an existing lock.
	Budget time.Duration
	// StaleAfter is the age at which an abandoned lock may be reclaimed. A lock
	// whose owner is still alive is never reclaimed.
	StaleAfter time.Duration
	// Heartbeat refreshes the lock while it is held. It defaults to StaleAfter/3.
	Heartbeat time.Duration
}

// Lock is one acquired lock.
type Lock struct {
	path  string
	pid   int
	nonce string

	stop chan struct{}
	done chan struct{}
	once sync.Once
}

type owner struct {
	pid   int
	nonce string
}

// Acquire tries to create the lock file at path within opts.Budget.
func Acquire(path string, opts Options) (*Lock, error) {
	if opts.Budget <= 0 {
		opts.Budget = 5 * time.Second
	}
	if opts.StaleAfter <= 0 {
		opts.StaleAfter = 30 * time.Second
	}
	if opts.Heartbeat <= 0 {
		opts.Heartbeat = opts.StaleAfter / 3
	}
	if opts.Heartbeat <= 0 {
		opts.Heartbeat = time.Second
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("filelock: mkdir lock dir: %w", err)
	}

	nonce, err := newNonce()
	if err != nil {
		return nil, err
	}
	pid := os.Getpid()
	deadline := time.Now().Add(opts.Budget)
	for {
		file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err == nil {
			_, writeErr := fmt.Fprintf(file, "%d %s\n", pid, nonce)
			closeErr := file.Close()
			if writeErr != nil || closeErr != nil {
				_ = removeIfOwned(path, owner{pid: pid, nonce: nonce})
				return nil, fmt.Errorf("filelock: write lock: %w", errors.Join(writeErr, closeErr))
			}
			lock := &Lock{
				path:  path,
				pid:   pid,
				nonce: nonce,
				stop:  make(chan struct{}),
				done:  make(chan struct{}),
			}
			go lock.heartbeat(opts.Heartbeat)
			return lock, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return nil, fmt.Errorf("filelock: create lock: %w", err)
		}
		if stale, statErr := staleLock(path, opts.StaleAfter); statErr != nil {
			return nil, statErr
		} else if stale {
			if removeErr := removeStaleLock(path, opts.StaleAfter); removeErr == nil {
				continue
			}
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("filelock: timed out waiting for %s", path)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// Release removes the lock if it is still owned by this acquisition.
func (l *Lock) Release() {
	if l == nil {
		return
	}
	l.once.Do(func() {
		close(l.stop)
		<-l.done
		_ = removeIfOwned(l.path, owner{pid: l.pid, nonce: l.nonce})
	})
}

func (l *Lock) heartbeat(interval time.Duration) {
	defer close(l.done)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-l.stop:
			return
		case now := <-ticker.C:
			if !ownedBy(l.path, owner{pid: l.pid, nonce: l.nonce}) {
				return
			}
			_ = os.Chtimes(l.path, now, now)
		}
	}
}

func staleLock(path string, staleAfter time.Duration) (bool, error) {
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, fmt.Errorf("filelock: stat lock: %w", err)
	}
	if time.Since(info.ModTime()) <= staleAfter {
		return false, nil
	}
	current, err := readOwner(path)
	if err != nil {
		// A lock old enough to be stale whose owner record cannot be read is
		// abandoned rather than held by a live process.
		return true, nil
	}
	if current.pid <= 0 {
		return true, nil
	}
	return !processAlive(current.pid), nil
}

func removeStaleLock(path string, staleAfter time.Duration) error {
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	if time.Since(info.ModTime()) <= staleAfter {
		return fmt.Errorf("filelock: lock is no longer stale")
	}
	current, err := readOwner(path)
	if err != nil {
		return os.Remove(path)
	}
	if current.pid > 0 && processAlive(current.pid) {
		return fmt.Errorf("filelock: lock owner is still alive")
	}
	return removeIfOwned(path, current)
}

func removeIfOwned(path string, expected owner) error {
	current, err := readOwner(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	if current.pid != expected.pid || current.nonce != expected.nonce {
		return fmt.Errorf("filelock: lock ownership changed")
	}
	return os.Remove(path)
}

func ownedBy(path string, expected owner) bool {
	current, err := readOwner(path)
	if err != nil {
		return false
	}
	return current.pid == expected.pid && current.nonce == expected.nonce
}

func readOwner(path string) (owner, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return owner{}, err
	}
	fields := strings.Fields(string(data))
	if len(fields) == 0 {
		return owner{}, fmt.Errorf("filelock: empty lock file")
	}
	pid, err := strconv.Atoi(fields[0])
	if err != nil {
		return owner{}, fmt.Errorf("filelock: invalid lock pid: %w", err)
	}
	nonce := ""
	if len(fields) > 1 {
		nonce = fields[1]
	}
	return owner{pid: pid, nonce: nonce}, nil
}

func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	if err == nil {
		return true
	}
	return errors.Is(err, syscall.EPERM)
}

func newNonce() (string, error) {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", fmt.Errorf("filelock: read nonce: %w", err)
	}
	return hex.EncodeToString(buf[:]), nil
}
