package sessions

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"

	"github.com/creack/pty"
	"github.com/paseka/paseka/internal/adapters"
)

// ptyProcess wraps a PTY-backed child process.
type ptyProcess struct {
	cmd    *exec.Cmd
	pty    *os.File
	mu     sync.Mutex
	closed bool
}

func startPTY(cmd adapters.SessionCommand) (*ptyProcess, error) {
	c := exec.Command(cmd.Binary, cmd.Args...)
	c.Dir = cmd.Dir
	c.Env = cmd.Env

	ptmx, err := pty.Start(c)
	if err != nil {
		return nil, fmt.Errorf("sessions: start pty: %w", err)
	}

	return &ptyProcess{cmd: c, pty: ptmx}, nil
}

func (p *ptyProcess) ReadWriteCloser() io.ReadWriteCloser {
	return p.pty
}

func (p *ptyProcess) PID() int {
	if p.cmd.Process == nil {
		return 0
	}
	return p.cmd.Process.Pid
}

func (p *ptyProcess) Wait() error {
	return p.cmd.Wait()
}

func (p *ptyProcess) Kill() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return nil
	}
	p.closed = true
	if p.cmd.Process != nil {
		_ = p.cmd.Process.Kill()
	}
	return p.pty.Close()
}
