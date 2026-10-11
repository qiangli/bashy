package agentos

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/qiangli/yoke/pkg/bus"
)

func TestSprintScopedInboxAndBoardPreserveCursor(t *testing.T) {
	isolateUnifiedInbox(t)
	seedSprintBoard(t, []map[string]any{seededStory(418, "", time.Now().UTC())})
	t.Setenv("BASHY_PRINCIPAL", "dhnt:agent/"+inboxTestReader)
	for _, body := range []string{"Sprint 418 review", "Sprint 419 unrelated"} {
		if err := bus.PostMessage(bus.Post{From: "peer", To: inboxTestReader, Body: body}); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"inbox", "mb"} {
		cmd, _, ok := newBusFrontDoorCmd(name)
		if !ok {
			t.Fatalf("%s not mounted", name)
		}
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetArgs([]string{"--as", inboxTestReader, "--sprint", "418"})
		if err := cmd.Execute(); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if !strings.Contains(out.String(), "Sprint 418 review") || strings.Contains(out.String(), "Sprint 419 unrelated") {
			t.Fatalf("%s scope result: %s", name, out.String())
		}
		if got := bus.SeenSeq(inboxTestReader); got != 0 {
			t.Fatalf("%s advanced board cursor to %d", name, got)
		}
	}
}

func TestSprintScopedInboxRejectsWatch(t *testing.T) {
	cmd := newUnifiedInboxCmd()
	cmd.SetArgs([]string{"--sprint", "418", "--watch"})
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "--watch cannot be combined with --sprint") {
		t.Fatalf("watch rejection: %v", err)
	}
}
