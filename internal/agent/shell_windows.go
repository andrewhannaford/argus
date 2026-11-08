//go:build windows

package agent

import (
	"io"
	"os"
	"os/exec"
	"sync"

	"github.com/UserExistsError/conpty"
)

type Shell struct {
	onOutput func(string)
	mu       sync.Mutex
	cpty     *conpty.ConPty
	cmd      *exec.Cmd
	stdin    io.WriteCloser
}

func NewShell(onOutput func(string)) *Shell {
	return &Shell{onOutput: onOutput}
}

func (s *Shell) Start() error {
	c, err := conpty.Start("powershell.exe -NoLogo", conpty.ConPtyEnv(os.Environ()))
	if err != nil {
		return s.startPipe()
	}
	s.cpty = c
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := c.Read(buf)
			if n > 0 {
				s.onOutput(string(buf[:n]))
			}
			if err != nil {
				return
			}
		}
	}()
	return nil
}

func (s *Shell) startPipe() error {
	cmd := newCommand("powershell.exe", "-NoLogo", "-NoExit")
	cmd.Env = os.Environ()
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	s.cmd = cmd
	s.stdin = stdin

	readPipe := func(r io.Reader) {
		buf := make([]byte, 4096)
		for {
			n, err := r.Read(buf)
			if n > 0 {
				s.onOutput(string(buf[:n]))
			}
			if err != nil {
				return
			}
		}
	}
	go readPipe(stdout)
	go readPipe(stderr)
	return nil
}

func (s *Shell) Write(data string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cpty != nil {
		s.cpty.Write([]byte(data))
	} else if s.stdin != nil {
		s.stdin.Write([]byte(data))
	}
}

func (s *Shell) Resize(cols, rows uint16) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cpty != nil {
		s.cpty.Resize(int(cols), int(rows))
	}
}

func (s *Shell) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cpty != nil {
		s.cpty.Close()
		s.cpty = nil
	}
	if s.stdin != nil {
		s.stdin.Close()
	}
	if s.cmd != nil && s.cmd.Process != nil {
		s.cmd.Process.Kill()
	}
}
