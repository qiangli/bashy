// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

//go:build !windows

package cast

import (
	"errors"
	"os"
	"os/exec"

	"github.com/creack/pty/v2"
)

// session is the Unix pseudo-terminal: creack/pty, as in pkg/webterm.
type session struct {
	pty *os.File
	cmd *exec.Cmd
}

func start(argv []string, dir string, env []string, cols, rows uint16) (*session, error) {
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Dir = dir
	cmd.Env = env
	if env == nil {
		cmd.Env = append(os.Environ(), "TERM="+term)
	}
	f, err := pty.StartWithSize(cmd, &pty.Winsize{Cols: cols, Rows: rows})
	if err != nil {
		return nil, err
	}
	return &session{pty: f, cmd: cmd}, nil
}

func (s *session) Read(b []byte) (int, error) { return s.pty.Read(b) }

// finish reaps the child after the master read hangup and returns its exit
// status (128+N for a signal death, as a shell reports it).
func (s *session) finish() (int, error) {
	err := s.cmd.Wait()
	_ = s.pty.Close()
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		if code := ee.ExitCode(); code >= 0 {
			return code, nil
		}
		return 128 + signalNumber(ee), nil
	}
	return 0, err
}

func (s *session) Close() error {
	if s.cmd.Process != nil {
		_ = s.cmd.Process.Kill()
	}
	_ = s.cmd.Wait()
	return s.pty.Close()
}
