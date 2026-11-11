// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package cast

import (
	"os"

	xpty "github.com/aymanbagabas/go-pty"
	"golang.org/x/sys/windows"
)

// session is the Windows pseudo-console: ConPTY through aymanbagabas/go-pty,
// the same backend pkg/webterm and pkg/agentpty use.
type session struct {
	pty  xpty.Pty
	cmd  *xpty.Cmd
	done chan struct{}
	code int
	err  error
}

func start(argv []string, dir string, env []string, cols, rows uint16) (*session, error) {
	p, err := xpty.New()
	if err != nil {
		return nil, err
	}
	if err := p.Resize(int(cols), int(rows)); err != nil {
		_ = p.Close()
		return nil, err
	}
	c := p.Command(argv[0], argv[1:]...)
	c.Dir = dir
	c.Env = env
	if env == nil {
		c.Env = append(os.Environ(), "TERM="+term)
	}
	if err := c.Start(); err != nil {
		_ = p.Close()
		return nil, err
	}
	s := &session{pty: p, cmd: c, done: make(chan struct{})}
	// ConPTY's output pipe does not hang up when the client exits: it stays
	// open until the pseudo-console is closed. So the waiter releases the
	// console itself once the child is gone — conhost then flushes what is
	// left and closes its end, and Read drains to EOF. go-pty's Close would
	// also close the pipe we are reading, dropping that tail, hence the raw
	// ClosePseudoConsole here and p.Close only after the drain.
	go func() {
		s.err = c.Wait()
		if c.ProcessState != nil {
			s.code = c.ProcessState.ExitCode()
		}
		windows.ClosePseudoConsole(windows.Handle(p.Fd()))
		close(s.done)
	}()
	return s, nil
}

func (s *session) Read(b []byte) (int, error) { return s.pty.Read(b) }

func (s *session) finish() (int, error) {
	<-s.done
	_ = s.pty.Close()
	if s.code >= 0 {
		return s.code, nil
	}
	return 1, s.err
}

func (s *session) Close() error {
	if s.cmd.Process != nil {
		_ = s.cmd.Process.Kill()
	}
	<-s.done
	return s.pty.Close()
}
