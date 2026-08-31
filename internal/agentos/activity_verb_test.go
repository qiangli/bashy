// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

package agentos

import (
	"bytes"
	"strings"
	"testing"

	"github.com/qiangli/bashy/internal/agentos/activity"
)

// isolateActivity points the verb at a temp outbox AND stubs the bus
// transport. Both are required: BASHY_ACTIVITY_DIR moves this package's state,
// but it does not move room.Dir(), so an unstubbed emit would reach the
// operator's live timeline (see activity.TestTransportMustBeStubbedNotRedirected).
func isolateActivity(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("BASHY_ACTIVITY_DIR", dir)
	t.Setenv("BASHY_HOME", dir)

	oldEnsure, oldPublish, oldWake := activity.EnsureInbox, activity.PublishDurable, activity.WakeLive
	t.Cleanup(func() {
		activity.EnsureInbox, activity.PublishDurable, activity.WakeLive = oldEnsure, oldPublish, oldWake
	})
	activity.EnsureInbox = func(string) error { return nil }
	activity.PublishDurable = func(_, _, _, _, _ string) error { return nil }
	activity.WakeLive = func(string, string) (bool, string) { return false, "test" }
}

func TestActivityVerbUnknownSubcommandIsRefused(t *testing.T) {
	isolateActivity(t)
	if code := dispatchActivity([]string{"frobnicate"}); code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
}

func TestActivitySubscribeRoundTrip(t *testing.T) {
	isolateActivity(t)
	if code := activitySubscribe([]string{"atlas", "--source", "weave,sprint", "--repo", "bashy", "--max-wake-per-min", "2"}); code != 0 {
		t.Fatalf("subscribe exit = %d", code)
	}
	var out bytes.Buffer
	if code := activityInterests(&out, nil); code != 0 {
		t.Fatalf("interests exit = %d", code)
	}
	got := out.String()
	for _, want := range []string{"atlas", "source=weave,sprint", "repo=bashy", "max-wake-per-min=2"} {
		if !strings.Contains(got, want) {
			t.Fatalf("interests output %q missing %q", got, want)
		}
	}
	// --no-wake and --mute are visible in the compact rendering, because an
	// operator debugging "why was I not told" needs to see them at a glance.
	if code := activitySubscribe([]string{"quiet", "--repo", "bashy", "--no-wake"}); code != 0 {
		t.Fatalf("subscribe exit = %d", code)
	}
	if code := activitySubscribe([]string{"gone", "--mute"}); code != 0 {
		t.Fatalf("subscribe exit = %d", code)
	}
	out.Reset()
	activityInterests(&out, nil)
	if !strings.Contains(out.String(), "muted") || !strings.Contains(out.String(), "no") {
		t.Fatalf("wake policy is not visible: %q", out.String())
	}

	if code := activityUnsubscribe([]string{"atlas"}); code != 0 {
		t.Fatalf("unsubscribe exit = %d", code)
	}
	if code := activityUnsubscribe([]string{"atlas"}); code == 0 {
		t.Fatalf("unsubscribing an absent interest reported success")
	}
}

func TestActivityTailShowRoutesRenderCompactly(t *testing.T) {
	isolateActivity(t)
	if code := activitySubscribe([]string{"watcher", "--object", "weave:run/42"}); code != 0 {
		t.Fatalf("subscribe exit = %d", code)
	}
	a, err := activity.For(activity.SourceWeave)
	if err != nil {
		t.Fatal(err)
	}
	res, err := a.As("conductor").In(activity.Scope{Repo: "bashy", Sprint: "88"}).
		Lifecycle(activity.ActionFail, "run", "weave:run/42", activity.StatusFailed, "failed",
			activity.Interested{Owner: "steward", Members: []string{"atlas"}})
	if err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	if code := activityTail(&out, nil); code != 0 {
		t.Fatalf("tail exit = %d", code)
	}
	line := strings.TrimSpace(out.String())
	if strings.Count(line, "\n") != 0 {
		t.Fatalf("one event rendered as %d lines: %q", strings.Count(line, "\n")+1, line)
	}
	// Compact means the rendered subject a recipient actually saw.
	if line != res.Event.Subject()+" -> 3" {
		t.Fatalf("tail line = %q, want the subject plus a recipient count", line)
	}

	// routes explains WHY each recipient matched — the whole point of the
	// reason field.
	out.Reset()
	if code := activityRoutes(&out, []string{res.Event.ID}); code != 0 {
		t.Fatalf("routes exit = %d", code)
	}
	got := out.String()
	for _, want := range []string{"steward", "ownership", "owns weave:run/42", "atlas", "membership", "watcher", "subscription"} {
		if !strings.Contains(got, want) {
			t.Fatalf("routes output missing %q:\n%s", want, got)
		}
	}

	// show turns the trailing id= back into the full envelope.
	out.Reset()
	if code := activityShow(&out, []string{res.Event.ID}); code != 0 {
		t.Fatalf("show exit = %d", code)
	}
	if !strings.Contains(out.String(), `"object": "weave:run/42"`) {
		t.Fatalf("show did not return the envelope:\n%s", out.String())
	}
	if code := activityShow(&out, []string{"nosuchid"}); code == 0 {
		t.Fatalf("show reported success for an unknown id")
	}
}

func TestActivityStatusReportsOwedDeliveries(t *testing.T) {
	isolateActivity(t)
	activity.PublishDurable = func(_, to, _, _, _ string) error {
		if to == "steward" {
			return errTestTransport
		}
		return nil
	}
	a, _ := activity.For(activity.SourceWeave)
	if _, err := a.As("c").Lifecycle(activity.ActionFail, "run", "weave:run/1", activity.StatusFailed, "f",
		activity.Interested{Owner: "steward"}); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	if code := activityStatus(&out, nil); code != 0 {
		t.Fatalf("status exit = %d", code)
	}
	got := out.String()
	if !strings.Contains(got, "owed:      1") {
		t.Fatalf("status did not report the owed delivery:\n%s", got)
	}
	// An owed delivery is not a lost one, so status must say what closes it.
	if !strings.Contains(got, "bashy activity recover") {
		t.Fatalf("status did not name the remedy:\n%s", got)
	}

	activity.PublishDurable = func(_, _, _, _, _ string) error { return nil }
	out.Reset()
	if code := activityRecover(&out, nil); code != 0 {
		t.Fatalf("recover exit = %d", code)
	}
	out.Reset()
	activityStatus(&out, nil)
	if !strings.Contains(out.String(), "owed:      0") {
		t.Fatalf("recover did not clear the owed delivery:\n%s", out.String())
	}
}

func TestActivityStatusJSONIsMachineReadable(t *testing.T) {
	isolateActivity(t)
	var out bytes.Buffer
	if code := activityStatus(&out, []string{"--json"}); code != 0 {
		t.Fatalf("status --json exit = %d", code)
	}
	if !strings.Contains(out.String(), `"schema": "`+activity.SchemaVersion+`"`) {
		t.Fatalf("status --json is not schema-tagged:\n%s", out.String())
	}
}

var errTestTransport = errTransport("transport down")

type errTransport string

func (e errTransport) Error() string { return string(e) }
