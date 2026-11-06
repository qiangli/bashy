// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package agentos

// P0 control for the typed service-status producer.
//
// `bashy service status --json` is the incident's actual repeated poll: every
// invocation used to land one generic exec span, healthy or not, forever.
// The command now reports every unit through telemetry.ServiceStatus, which
// collapses identical polls (first observation + transitions only), while the
// exec middleware steps aside for exactly this command shape. This test pins
// the producer half: first observations and meaningful transitions (with
// prior state and evidence) are reported, identical repeats are not, and a
// failed query is recorded separately from reported state.
//
// The command's JSON/stdout/exit behavior is unchanged — that contract stays
// with the schema golden test (TestServiceStatusJSONGolden).
//
// NOTE: this test needs a yoke pin that includes telemetry.ServiceStatus
// (the P0 spool repair). Until the pin is bumped it does not compile; the
// manager bumps the pin before running gates.

import (
	"context"
	"errors"
	"mvdan.cc/sh/v3/interp"
	"testing"

	"github.com/qiangli/yoke/pkg/service"
	"github.com/qiangli/yoke/pkg/telemetry"
)

func TestServiceStatusCommandReportsTypedObservations(t *testing.T) {
	t.Setenv("OTEL_TRACES_EXPORTER", "none")
	sr := recordSpans(t)
	ctx := context.Background()
	sock := "/tmp/test-bashy-supervisord.sock"
	doc := serviceStatusDoc{Schema: ServiceStatusSchema, Units: []service.Status{
		{Unit: "loom", Enabled: true, State: service.Active},
		{Unit: "zot", Enabled: true, State: service.Failed, Reason: "boom", Health: "failing"},
	}}

	reportServiceStatus(ctx, sock, doc, nil)
	reportServiceStatus(ctx, sock, doc, nil) // identical repeat: collapsed

	doc.Units[0] = service.Status{Unit: "loom", Enabled: true, State: service.Failed, Reason: "down"}
	reportServiceStatus(ctx, sock, doc, nil) // healthy → failed: transition

	byService := map[string][]map[string]string{}
	for _, s := range sr.Ended() {
		if s.Name() != "service.status" {
			continue
		}
		m := map[string]string{}
		for _, kv := range s.Attributes() {
			m[string(kv.Key)] = kv.Value.Emit()
		}
		byService[m["status.service"]] = append(byService[m["status.service"]], m)
	}

	if len(byService["loom"]) != 2 {
		t.Fatalf("loom produced %d records, want 2 (first + transition)", len(byService["loom"]))
	}
	if byService["loom"][0]["status.state"] != telemetry.StatusHealthy {
		t.Errorf("loom first record wrong: %+v", byService["loom"][0])
	}
	if byService["loom"][1]["status.prior_state"] != telemetry.StatusHealthy ||
		byService["loom"][1]["status.state"] != telemetry.StatusError {
		t.Errorf("loom transition wrong: %+v", byService["loom"][1])
	}
	if byService["loom"][1]["status.detail"] != "down" {
		t.Errorf("loom transition detail missing: %+v", byService["loom"][1])
	}
	if len(byService["zot"]) != 1 {
		t.Fatalf("zot produced %d records, want 1", len(byService["zot"]))
	}
	if byService["zot"][0]["status.detail"] != "health=failing; boom" {
		t.Errorf("zot evidence missing: %+v", byService["zot"][0])
	}
	if got := len(sr.Ended()); got != 3 {
		t.Errorf("total %d typed records, want 3 — repeats must collapse", got)
	}
}

// A failed query (here: an unknown unit name) is a query-failure record, not
// a reported state: no status.state attribute, query error preserved.
func TestServiceStatusQueryFailureReportedSeparately(t *testing.T) {
	t.Setenv("OTEL_TRACES_EXPORTER", "none")
	sr := recordSpans(t)
	ctx := context.Background()

	reportServiceStatus(ctx, "/tmp/test-bashy-supervisord.sock", serviceStatusDoc{},
		errors.New(`no service "ghost"`))

	spans := sr.Ended()
	if len(spans) != 1 || spans[0].Name() != "service.status_query_failed" {
		t.Fatalf("want 1 service.status_query_failed record, got %v", spans)
	}
	attrs := map[string]string{}
	for _, kv := range spans[0].Attributes() {
		attrs[string(kv.Key)] = kv.Value.Emit()
	}
	if _, hasState := attrs["status.state"]; hasState {
		t.Errorf("query failure carries a reported state: %+v", attrs)
	}
	if attrs["status.query_error"] != `no service "ghost"` {
		t.Errorf("query error missing: %+v", attrs)
	}
}

// The mapping itself: reported states to seam vocabulary, details preserved.
// Inactive units and monitor-down placeholders are unknown with their reason,
// never an inferred healthy or error.
func TestServiceStatusStateMapping(t *testing.T) {
	if got := serviceStatusState(service.Active); got != telemetry.StatusHealthy {
		t.Errorf("active -> %q", got)
	}
	if got := serviceStatusState(service.Degraded); got != telemetry.StatusDegraded {
		t.Errorf("degraded -> %q", got)
	}
	if got := serviceStatusState(service.Failed); got != telemetry.StatusError {
		t.Errorf("failed -> %q", got)
	}
	for _, st := range []service.State{service.Inactive, service.Starting, service.Stopping, "bogus"} {
		if got := serviceStatusState(st); got != telemetry.StatusUnknown {
			t.Errorf("%q -> %q, want unknown", st, got)
		}
	}
	if got := serviceStatusDetail(service.Status{Reason: "not monitored"}); got != "not monitored" {
		t.Errorf("reason-only detail = %q", got)
	}
	if got := serviceStatusDetail(service.Status{Health: "failing", Reason: "boom"}); got != "health=failing; boom" {
		t.Errorf("health+reason detail = %q", got)
	}
}

// Exercise the actual incident argv through middleware and the real typed
// producer. A command-shaped lookalike alone cannot suppress generic evidence.
func TestServiceStatusDispatchHandshake(t *testing.T) {
	t.Setenv("OTEL_TRACES_EXPORTER", "none")
	sr := recordSpans(t)
	doc := serviceStatusDoc{Schema: ServiceStatusSchema, Units: []service.Status{{Unit: "dispatch-handshake-review", State: service.Active}}}
	handler := telemetry.ExecMiddleware(func(ctx context.Context, _ []string) error {
		reportServiceStatus(ctx, "/virtual/dispatch-handshake.sock", doc, nil)
		return nil
	})
	for i := 0; i < 2; i++ {
		if err := handler(context.Background(), []string{"service", "status", "--json"}); err != nil {
			t.Fatal(err)
		}
	}
	if got := sr.Ended(); len(got) != 1 || got[0].Name() != "service.status" {
		t.Fatalf("actual producer failed to suppress repeated generic spans: %v", got)
	}
	// Retain the command's original failure behavior even after acknowledging.
	failed := telemetry.ExecMiddleware(func(ctx context.Context, _ []string) error {
		reportServiceStatus(ctx, "/virtual/dispatch-handshake.sock", serviceStatusDoc{}, errors.New("query failure"))
		return interp.ExitStatus(4)
	})
	if err := failed(context.Background(), []string{"service", "status", "--json"}); !errors.Is(err, interp.ExitStatus(4)) {
		t.Fatalf("changed query exit: %v", err)
	}
	got := sr.Ended()
	if len(got) != 2 || got[1].Name() != "service.status_query_failed" {
		t.Fatalf("query evidence lost: %v", got)
	}
}
