// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package agentos

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/qiangli/yoke/pkg/bus"
)

// nativeSession is a vendor-CLI agent session that declared nothing. sessionID
// is the vendor's session id; empty means the tool exposes none, and (under
// `go test`) no harness process is found above the command either.
func nativeSession(t *testing.T, tool, sessionID string) *bytes.Buffer {
	t.Helper()
	isolateUnifiedInbox(t)
	for _, env := range []string{"BASHY_PRINCIPAL", "BASHY_INSTANCE", "BASHY_INSTANCE_UUID",
		"BASHY_INSTANCE_FAMILY", "BASHY_INSTANCE_LABEL", "BASHY_AGENT", "WEAVE_AGENT",
		"BASHY_AGENT_ID", "WEAVE_CONDUCTOR", "BASHY_SESSION_CLAIM", bus.AdHocIdentityEnv} {
		t.Setenv(env, "")
	}
	priorDetect, priorSession, priorNotice := bus.DetectHarness, bus.CurrentSessionClaim, bus.AdHocNotice
	bus.DetectHarness = func() (string, bool) { return tool, tool != "" }
	bus.CurrentSessionClaim = func(string) string { return sessionID }
	named := &bytes.Buffer{}
	bus.AdHocNotice = named
	t.Cleanup(func() {
		bus.DetectHarness, bus.CurrentSessionClaim, bus.AdHocNotice = priorDetect, priorSession, priorNotice
	})
	return named
}

// A native session bashy can identify is corrected, not refused: any
// front-door verb stamps it with its ad hoc name, which is then what a verb
// that records an actor sees.
func TestAgentIdentityGateNamesANativeSession(t *testing.T) {
	named := nativeSession(t, "claude", "native-session-1")
	var stderr bytes.Buffer
	if code, refused := agentIdentityGate([]string{"claim", "list"}, &stderr); refused {
		t.Fatalf("an identifiable session was refused (%d): %s", code, stderr.String())
	}
	label := os.Getenv("BASHY_AGENT")
	if !strings.HasPrefix(label, "claude-") || len(label) != len("claude-")+8 {
		t.Fatalf("BASHY_AGENT = %q, want claude-<8 hex>", label)
	}
	if os.Getenv("BASHY_INSTANCE") == "" || os.Getenv("BASHY_INSTANCE_LABEL") != label {
		t.Fatalf("instance stamp incomplete: BASHY_INSTANCE=%q BASHY_INSTANCE_LABEL=%q", os.Getenv("BASHY_INSTANCE"), os.Getenv("BASHY_INSTANCE_LABEL"))
	}
	if !strings.Contains(named.String(), label) {
		t.Fatalf("the session was not told its name: %q", named.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("the gate itself wrote: %q", stderr.String())
	}
}

// A native session bashy cannot identify is refused on a verb whose record
// names an actor, and the refusal says how to name the session. A verb that
// records nobody, a request for help, and the off switch all pass.
func TestAgentIdentityGateRefusesAnUnnamedSessionOnCoordinationVerbs(t *testing.T) {
	nativeSession(t, "gemini", "")
	var stderr bytes.Buffer
	code, refused := agentIdentityGate([]string{"claim", "host:x"}, &stderr)
	if !refused || code == 0 {
		t.Fatalf("an unnamed session ran a coordination verb (refused=%v code=%d)", refused, code)
	}
	for _, want := range []string{"bashy claim:", "agent id is required", "gemini", "BASHY_AGENT=NAME", "bashy agent add NAME"} {
		if !strings.Contains(stderr.String(), want) {
			t.Errorf("refusal lacks %q:\n%s", want, stderr.String())
		}
	}
	if os.Getenv("BASHY_AGENT") != "" {
		t.Fatalf("a refused session was stamped: BASHY_AGENT=%q", os.Getenv("BASHY_AGENT"))
	}

	for name, argv := range map[string][]string{
		"records nobody": {"commands"},
		"how to fix it":  {"agent", "add", "x"},
		"help":           {"claim", "--help"},
		"not a verb":     {"-c", "echo hi"},
		"a script path":  {"./script.sh"},
	} {
		stderr.Reset()
		if _, refused := agentIdentityGate(argv, &stderr); refused || stderr.Len() != 0 {
			t.Errorf("%s (%v): refused=%v stderr=%q; want it to pass silently", name, argv, refused, stderr.String())
		}
	}

	t.Setenv(bus.AdHocIdentityEnv, "off")
	if _, refused := agentIdentityGate([]string{"claim", "host:x"}, &stderr); refused {
		t.Fatal("refused with the check switched off")
	}
}

// Whoever already has a name, and whoever is not an agent, is untouched.
func TestAgentIdentityGateLeavesNamedAndHumanCallersAlone(t *testing.T) {
	nativeSession(t, "gemini", "")
	t.Setenv("BASHY_AGENT", "a-name-i-chose")
	var stderr bytes.Buffer
	if _, refused := agentIdentityGate([]string{"claim", "host:x"}, &stderr); refused || os.Getenv("BASHY_AGENT") != "a-name-i-chose" {
		t.Fatalf("a self-named agent: refused=%v BASHY_AGENT=%q", refused, os.Getenv("BASHY_AGENT"))
	}

	nativeSession(t, "", "")
	if _, refused := agentIdentityGate([]string{"claim", "host:x"}, &stderr); refused || os.Getenv("BASHY_AGENT") != "" || os.Getenv("BASHY_INSTANCE") != "" {
		t.Fatalf("a human: refused=%v BASHY_AGENT=%q", refused, os.Getenv("BASHY_AGENT"))
	}
}
