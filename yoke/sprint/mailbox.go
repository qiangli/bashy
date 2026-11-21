package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/qiangli/coreutils/pkg/lockfile"
)

const mailboxSchema = "bashy-mailbox-v1"

type mailboxSpec struct{ Key, Address, Kind string }

type mailboxMark struct {
	ReadAt    string `json:"read_at,omitempty"`
	AckedAt   string `json:"acked_at,omitempty"`
	Preserved bool   `json:"preserved,omitempty"`
	Project   string `json:"project,omitempty"`
	Status    string `json:"status,omitempty"`
}

type mailboxState struct {
	Schema        string                 `json:"schema"`
	Marks         map[string]mailboxMark `json:"marks"`
	Participation map[string]string      `json:"participation,omitempty"`
}

func mailboxDir() (string, error) {
	if d := strings.TrimSpace(os.Getenv("BASHY_MAILBOX_DIR")); d != "" {
		if err := os.MkdirAll(d, 0o700); err != nil {
			return "", err
		}
		_ = os.Chmod(d, 0o700)
		return d, nil
	}
	// Same ladder as the other stores (audit, activity, skills): BASHY_HOME,
	// then $HOME — never the passwd home, which ignores an isolated HOME.
	var d string
	if home := strings.TrimSpace(os.Getenv("BASHY_HOME")); home != "" {
		d = filepath.Join(home, "inbox")
	} else {
		h, err := os.UserHomeDir()
		if err != nil || strings.TrimSpace(h) == "" {
			return "", fmt.Errorf("inbox: no home directory (set BASHY_MAILBOX_DIR, BASHY_HOME or HOME)")
		}
		d = filepath.Join(h, ".bashy", "inbox")
	}
	if err := os.MkdirAll(d, 0o700); err != nil {
		return "", err
	}
	_ = os.Chmod(d, 0o700)
	return d, nil
}

func mailboxStatePath(spec mailboxSpec) (string, error) {
	d, err := mailboxDir()
	if err != nil {
		return "", err
	}
	name := strings.NewReplacer("/", "_", "\\", "_", ":", "_").Replace(spec.Key)
	return filepath.Join(d, name+".json"), nil
}

func loadMailboxState(spec mailboxSpec) (mailboxState, error) {
	s := mailboxState{Schema: mailboxSchema, Marks: map[string]mailboxMark{}}
	p, err := mailboxStatePath(spec)
	if err != nil {
		return s, err
	}
	b, err := os.ReadFile(p)
	if os.IsNotExist(err) {
		return s, nil
	}
	if err != nil {
		return s, err
	}
	if err = json.Unmarshal(b, &s); err != nil {
		return s, fmt.Errorf("inbox: read mailbox state: %w", err)
	}
	if s.Marks == nil {
		s.Marks = map[string]mailboxMark{}
	}
	return s, nil
}

func saveMailboxState(spec mailboxSpec, s mailboxState) error {
	p, err := mailboxStatePath(spec)
	if err != nil {
		return err
	}
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	t := fmt.Sprintf("%s.tmp-%d", p, os.Getpid())
	if err = os.WriteFile(t, b, 0o600); err != nil {
		return err
	}
	_ = os.Chmod(t, 0o600)
	if err = os.Rename(t, p); err != nil {
		_ = os.Remove(t)
		return err
	}
	return os.Chmod(p, 0o600)
}

func updateMailboxState(spec mailboxSpec, change func(*mailboxState) error) error {
	p, err := mailboxStatePath(spec)
	if err != nil {
		return err
	}
	host, _ := os.Hostname()
	l, err := lockfile.Acquire(p+".lock", lockfile.Holder{Name: host, PID: os.Getpid(), Intent: "update mailbox state", Since: time.Now()})
	if err != nil {
		return err
	}
	defer l.Release()
	s, err := loadMailboxState(spec)
	if err != nil {
		return err
	}
	if err := change(&s); err != nil {
		return err
	}
	return saveMailboxState(spec, s)
}
