package main

import (
	"testing"
)

func isolateMailbox(t *testing.T) mailboxSpec {
	t.Helper()
	isolateUnifiedInbox(t)
	t.Setenv("BASHY_MAILBOX_DIR", t.TempDir())
	spec, err := currentHumanMailbox()
	if err != nil {
		t.Fatal(err)
	}
	return spec
}
