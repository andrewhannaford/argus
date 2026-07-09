//go:build !windows

package agent

import (
	"io"
	"os"
	"os/exec"

	"github.com/creack/pty"
)

type Shell struct {
	onOutput func(string)
	ptmx     *os.File
}

func NewShell(onOutput func(string)) *Shell {
	return &Shell{onOutput: onOutput}
}

func (s *Shell) Start() error {
	shell := "/bin/bash"
	if _, err := os.Stat(shell); err != nil {
		shell = "/bin/sh"
	}
	cmd := exec.Command(shell)
	cmd.Env = append(os.Environ(), "TERM=xterm-256color")

	ptmx, err := pty.Start(cmd)
	if err != nil {
		return err
	}
	s.ptmx = ptmx

	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := ptmx.Read(buf)
			if n > 0 {
				s.onOutput(string(buf[:n]))
			}
			if err != nil {
				if err != io.EOF {
					return
				}
				return
			}
		}
	}()

	return nil
}

func (s *Shell) Write(data string) {
	if s.ptmx != nil {
		s.ptmx.Write([]byte(data))
	}
}

func (s *Shell) Resize(cols, rows uint16) {
	if s.ptmx != nil {
		pty.Setsize(s.ptmx, &pty.Winsize{Cols: cols, Rows: rows})
	}
}

func (s *Shell) Close() {
	if s.ptmx != nil {
		s.ptmx.Close()
	}
}
