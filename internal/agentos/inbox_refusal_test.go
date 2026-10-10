// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

package agentos

import (
	"strings"
	"testing"
)

// An unattributed session that names an identity owning no mailbox is refused —
// and that refusal must name `bashy inbox` and say how to establish an identity,
// so the dead end becomes an actionable next step.
func TestInboxRefusalNamesInboxAndIdentity(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("BASHY_ROOT", t.TempDir())
	t.Setenv("BASHY_PRINCIPAL", "") // not an authenticated agent
	t.Setenv("BASHY_INSTANCE", "")

	_, err := resolveInboxReader("no-such-identity-xyz-404")
	if err == nil {
		t.Fatal("expected a refusal for an unregistered --as name")
	}
	msg := err.Error()
	if !strings.Contains(msg, "bashy inbox") {
		t.Errorf("refusal must name bashy inbox: %q", msg)
	}
	if !strings.Contains(msg, "BASHY_AGENT") {
		t.Errorf("refusal must say how to establish an identity: %q", msg)
	}
}
