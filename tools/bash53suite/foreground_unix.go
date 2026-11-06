//go:build unix

package main

import (
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"golang.org/x/sys/unix"
)

// fixtureTerminal holds the controlling terminal, never fixture stdin. Stdin
// remains the idle pipe used by read -t 0; only /dev/tty changes owner.
type fixtureTerminal struct {
	file  *os.File
	owner int
	child int
}

func openFixtureTerminal() (*fixtureTerminal, error) {
	f, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if errors.Is(err, syscall.ENXIO) || errors.Is(err, syscall.ENODEV) || errors.Is(err, os.ErrNotExist) {
		return nil, nil // headless invocation
	}
	if err != nil {
		return nil, fmt.Errorf("open controlling terminal: %w", err)
	}
	owner, err := unix.IoctlGetInt(int(f.Fd()), unix.TIOCGPGRP)
	if err != nil {
		f.Close()
		return nil, fmt.Errorf("get terminal foreground group: %w", err)
	}
	if owner != syscall.Getpgrp() {
		f.Close()
		return nil, fmt.Errorf("controlling terminal belongs to process group %d, runner group is %d", owner, syscall.Getpgrp())
	}
	return &fixtureTerminal{file: f, owner: owner}, nil
}

func (t *fixtureTerminal) handoff(pid int) error {
	if t == nil {
		return nil
	}
	owner, err := unix.IoctlGetInt(int(t.file.Fd()), unix.TIOCGPGRP)
	if err != nil {
		return fmt.Errorf("check terminal foreground group before handoff: %w", err)
	}
	if owner != t.owner {
		return fmt.Errorf("terminal foreground group changed from %d to %d before handoff", t.owner, owner)
	}
	group, err := syscall.Getpgid(pid)
	if err != nil {
		return fmt.Errorf("get fixture process group: %w", err)
	}
	if group != pid {
		return fmt.Errorf("fixture guard %d belongs to process group %d", pid, group)
	}
	if err := unix.IoctlSetPointerInt(int(t.file.Fd()), unix.TIOCSPGRP, pid); err != nil {
		return fmt.Errorf("give fixture process group terminal foreground: %w", err)
	}
	t.child = pid
	owner, err = unix.IoctlGetInt(int(t.file.Fd()), unix.TIOCGPGRP)
	if err != nil {
		return fmt.Errorf("verify fixture terminal foreground group: %w", err)
	}
	if owner != pid {
		return fmt.Errorf("fixture terminal foreground group is %d, want %d", owner, pid)
	}
	return nil
}

func (t *fixtureTerminal) restore() error {
	if t == nil {
		return nil
	}
	defer t.file.Close()
	if t.child == 0 {
		return nil
	}
	owner, err := unix.IoctlGetInt(int(t.file.Fd()), unix.TIOCGPGRP)
	if err != nil {
		return fmt.Errorf("check terminal foreground group before restoration: %w", err)
	}
	if owner == t.owner {
		return nil
	}
	if owner != t.child {
		return fmt.Errorf("terminal foreground group changed from fixture group %d to %d; refusing to replace unrelated owner", t.child, owner)
	}
	// The runner is now in the background. POSIX permits tcsetpgrp while
	// SIGTTOU is ignored; preserve an inherited ignore disposition.
	ignored := signal.Ignored(syscall.SIGTTOU)
	signal.Ignore(syscall.SIGTTOU)
	err = unix.IoctlSetPointerInt(int(t.file.Fd()), unix.TIOCSPGRP, t.owner)
	if !ignored {
		// Notify is the documented way to undo Ignore. Reset can leave the
		// kernel disposition at SIG_IGN for Go's normally handled SIGTTOU,
		// which would leak an ignored trap into every later fixture exec.
		// Stop removes our subscription while retaining the runtime handler;
		// os/exec resets that handler to the child's default disposition.
		ch := make(chan os.Signal, 1)
		signal.Notify(ch, syscall.SIGTTOU)
		signal.Stop(ch)
	}
	if err != nil {
		return fmt.Errorf("restore terminal foreground group %d: %w", t.owner, err)
	}
	t.child = 0
	return nil
}
