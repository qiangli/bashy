package agentos

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/qiangli/yoke/pkg/fleet"
	"github.com/qiangli/yoke/pkg/llmbudget"
	"github.com/qiangli/yoke/pkg/snippet"
	"github.com/qiangli/yoke/pkg/weave"
	"github.com/spf13/cobra"
)

func quotaNow() time.Time    { return time.Date(2026, 10, 19, 13, 24, 0, 0, time.UTC) }
func quotaPtr[T any](v T) *T { return &v }
func quotaFixture() []llmbudget.QuotaRow {
	return []llmbudget.QuotaRow{
		{Provider: "claude", Plan: "max", Window: "5h", UsedPercent: quotaPtr(62.0), ResetAt: quotaPtr(quotaNow().Add(100 * time.Minute)), Tokens: quotaPtr(1200.0), Source: "fixture", ObservedAt: quotaPtr(quotaNow().Add(-90 * time.Second)), Status: "ok", Headroom: quotaPtr(.38), SuggestedWorkers: quotaPtr(2), Classification: "measured"},
		{Provider: "unknown", Status: "unmeasured"},
	}
}
func executeQuota(cmd *cobra.Command, args ...string) (string, string, error) {
	var out, stderr bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&stderr)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), stderr.String(), err
}
func TestQuotaRenderGolden(t *testing.T) {
	t.Setenv("TZ", "UTC")
	var out bytes.Buffer
	if err := renderModelQuota(&out, quotaFixture(), quotaNow(), time.UTC); err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile("testdata/model_quota.golden")
	if err != nil {
		t.Fatal(err)
	}
	if out.String() != string(want) {
		t.Fatalf("render mismatch\ngot:\n%s\nwant:\n%s", out.String(), want)
	}
	var local bytes.Buffer
	renderModelQuota(&local, quotaFixture(), quotaNow(), time.FixedZone("west", -7*3600))
	if !strings.Contains(local.String(), "Mon 08:04 (in 1h40m)") {
		t.Fatal(local.String())
	}
}
func TestQuotaJSONAndDeadline(t *testing.T) {
	for _, refresh := range []bool{false, true} {
		t.Run(map[bool]string{false: "cached", true: "refresh"}[refresh], func(t *testing.T) {
			collect := func(ctx context.Context, opt llmbudget.QuotaOptions) ([]llmbudget.QuotaRow, error) {
				if !opt.All || opt.Provider != "claude" || opt.Refresh != refresh || !opt.Now.Equal(quotaNow()) {
					t.Fatalf("options %+v", opt)
				}
				deadline, ok := ctx.Deadline()
				limit := 5 * time.Second
				if refresh {
					limit = 20 * time.Second
				}
				if !ok || time.Until(deadline) > limit || time.Until(deadline) < limit-time.Second {
					t.Fatal("deadline", deadline)
				}
				return quotaFixture(), nil
			}
			cmd := modelQuotaCommand(collect, func([]llmbudget.QuotaRow, time.Time) error { return nil }, quotaNow)
			args := []string{"--all", "--json", "--provider", "claude"}
			if refresh {
				args = append(args, "--refresh")
			}
			out, _, err := executeQuota(cmd, args...)
			if err != nil {
				t.Fatal(err)
			}
			var envelope struct {
				Schema     string `json:"schema_version"`
				Kind, View string
				Items      []map[string]any
			}
			if err = json.Unmarshal([]byte(out), &envelope); err != nil {
				t.Fatal(err)
			}
			if envelope.Schema != fleet.ListSchemaVersion || envelope.Kind != "quota" || envelope.View != "all" || len(envelope.Items) != 2 {
				t.Fatal(out)
			}
			for _, r := range envelope.Items {
				for _, key := range []string{"headroom", "stale", "suggested_workers", "classification", "status"} {
					if _, ok := r[key]; !ok {
						t.Fatalf("missing %s: %s", key, out)
					}
				}
			}
		})
	}
	cmd := modelQuotaCommand(func(context.Context, llmbudget.QuotaOptions) ([]llmbudget.QuotaRow, error) { return nil, nil }, func([]llmbudget.QuotaRow, time.Time) error { return nil }, quotaNow)
	out, _, err := executeQuota(cmd, "--json")
	if err != nil || !strings.Contains(out, `"items": []`) {
		t.Fatalf("%s %v", out, err)
	}
}
func TestQuotaCooldownOverlay(t *testing.T) {
	dir := t.TempDir()
	reset := quotaNow().Add(time.Hour)
	raw := `{"tools":{"fixture":"` + reset.Format(time.RFC3339) + `"},"causes":{"fixture":"quota-exhausted"}}`
	if err := os.WriteFile(filepath.Join(dir, "tool_cooldowns.json"), []byte(raw), 0600); err != nil {
		t.Fatal(err)
	}
	rows := []llmbudget.QuotaRow{{Provider: "vendor", Status: "unmeasured"}, {Provider: "vendor", ResetAt: quotaPtr(quotaNow().Add(2 * time.Hour)), Status: "ok"}, {Provider: "other", Status: "ok"}}
	agents := []fleet.Agent{{Name: "agent", Tool: "fixture", Model: "model"}}
	model := func(string) (fleet.Model, bool) { return fleet.Model{Provider: "vendor"}, true }
	if err := overlayQuotaSeats(rows, agents, model, []string{dir}, weave.SeatQuotaStatus, quotaNow()); err != nil {
		t.Fatal(err)
	}
	if rows[0].Status != "quota-exhausted" || rows[0].ResetAt == nil || !rows[0].ResetAt.Equal(reset) || !rows[1].ResetAt.Equal(quotaNow().Add(2*time.Hour)) || rows[2].Status != "ok" {
		t.Fatalf("%+v", rows)
	}
}
func TestQuotaCapture(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "capture.json")
	raw := `{"model":{"display_name":"Claude Opus"},"rate_limits":{"five_hour":{"used_percentage":62}}}`
	for i := 0; i < 2; i++ {
		cmd := modelQuotaCaptureCommand(quotaNow)
		cmd.SetIn(strings.NewReader(raw))
		start := time.Now()
		out, stderr, err := executeQuota(cmd, "--to", path, "--account", "work")
		if err != nil || stderr != "" || out != "Claude Opus | 5h 62%\n" {
			t.Fatalf("%q %q %v", out, stderr, err)
		}
		if elapsed := time.Since(start); elapsed >= 100*time.Millisecond {
			t.Fatalf("capture took %s", elapsed)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var env struct {
			Schema   string `json:"schema_version"`
			Observed string `json:"observed_at"`
			Account  string
			Payload  json.RawMessage
		}
		if err = json.Unmarshal(data, &env); err != nil {
			t.Fatal(err)
		}
		if env.Schema != "bashy-claude-statusline-v1" || env.Observed != quotaNow().Format(time.RFC3339) || env.Account != "work" || string(env.Payload) != raw {
			t.Fatal(string(data))
		}
		stat, _ := os.Stat(path)
		if runtime.GOOS != "windows" && stat.Mode().Perm() != 0600 {
			t.Fatal(stat.Mode())
		}
		entries, _ := os.ReadDir(filepath.Dir(path))
		if len(entries) != 1 {
			t.Fatal("temporary file leaked", entries)
		}
	}
	// A failed rename preserves the statusline and returns success.
	cmd := modelQuotaCaptureCommand(quotaNow)
	cmd.SetIn(strings.NewReader(raw))
	out, stderr, err := executeQuota(cmd, "--to", filepath.Dir(path))
	if err != nil || out != "Claude Opus | 5h 62%\n" || !strings.Contains(stderr, "quota capture:") {
		t.Fatalf("%q %q %v", out, stderr, err)
	}
	for _, input := range []string{`{"anything":true}`, `[]`, `null`, `{"broken"`, strings.Repeat(" ", 1<<20) + "{}"} {
		cmd := modelQuotaCaptureCommand(quotaNow)
		cmd.SetIn(strings.NewReader(input))
		out, _, err := executeQuota(cmd, "--to", path)
		if err != nil || out != "" {
			t.Fatalf("unexpected stdout/error %q %v", out, err)
		}
	}
}
func fixtureQuotaProbe() quotaProbe {
	return quotaProbe{ID: "probe:plan:fixture/q", Plan: "fixture", Probe: fleet.UsageProbe{ID: "q", Run: snippet.Snippet{Body: "echo fixture", Effects: []string{"net"}}, Metrics: []fleet.UsageMetricMap{{Name: "used_percent", Value: ".pct", Unit: "percent", Window: "5h", Reset: ".reset"}}}}
}
func TestQuotaProbeTest(t *testing.T) {
	p := fixtureQuotaProbe()
	resolve := func(string) ([]quotaProbe, error) { return []quotaProbe{p}, nil }
	for _, tc := range []struct {
		name   string
		result snippet.Result
		err    error
		status string
	}{
		{"ok", snippet.Result{Stdout: []byte(`{"pct":62,"reset":"2026-10-19T15:04:00Z"}`)}, nil, "ok"},
		{"jq", snippet.Result{Stdout: []byte(`{"pct":{}}`)}, nil, "unavailable"},
		{"refused", snippet.Result{Exit: 126, Refused: []string{"curl"}}, nil, "unavailable"},
		{"input", snippet.Result{Exit: 6}, snippet.ErrNeedsInput, "needs-input"},
		{"missing", snippet.Result{}, snippet.ErrNotConfigured, "not-configured"},
		{"runner", snippet.Result{}, errors.New("runner failed"), "unavailable"},
		{"long", snippet.Result{Stdout: []byte(strings.Repeat("x", 5000))}, nil, "unavailable"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			calls := 0
			run := func(ctx context.Context, q quotaProbe) (snippet.Result, error) {
				calls++
				if !strings.Contains(out.String(), "effects: net\n") {
					t.Fatal("effects not printed first")
				}
				return tc.result, tc.err
			}
			cmd := modelQuotaTestCommand(resolve, run, quotaNow)
			cmd.SetOut(&out)
			cmd.SetErr(&bytes.Buffer{})
			cmd.SetArgs([]string{"fixture/q"})
			err := cmd.Execute()
			if (err == nil) != (tc.status == "ok") || calls != 1 || !strings.Contains(out.String(), "status: "+tc.status) {
				t.Fatalf("%s %v calls=%d", out.String(), err, calls)
			}
			if tc.name == "ok" && (!strings.Contains(out.String(), `"name":"quota.used_percent"`) || !strings.Contains(out.String(), `"value":62`)) {
				t.Fatal(out.String())
			}
			if tc.name == "long" && (!strings.Contains(out.String(), "[truncated at 4 KiB]") || strings.Contains(out.String(), strings.Repeat("x", 4097))) {
				t.Fatal("truncation")
			}
			if tc.name == "refused" && !strings.Contains(out.String(), "refused commands: curl") {
				t.Fatal(out.String())
			}
		})
	}
}
func TestQuotaProbeMapping(t *testing.T) {
	p := fixtureQuotaProbe()
	p.Probe.Metrics[0].Value = ".pct | error(\"bad jq\")"
	_, err := mapQuotaProbe(p, snippet.Result{Stdout: []byte(`{"pct":62}`)}, quotaNow())
	if err == nil || !strings.Contains(err.Error(), "bad jq") {
		t.Fatal(err)
	}
	p.Probe.Metrics = nil
	rows, err := mapQuotaProbe(p, snippet.Result{Classification: "inferred", Stdout: []byte(`{"schema_version":"bashy-usage-v1","metrics":[{"name":"balance","value":12,"unit":"credits","reset_at":1792418400000}]}`)}, quotaNow())
	if err != nil || len(rows) != 1 || rows[0].Classification != "inferred" || rows[0].ResetAt == nil || *rows[0].Value != 12 {
		t.Fatalf("%+v %v", rows, err)
	}
}
func TestQuotaConsentRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "policy.json")
	t.Setenv("BASHY_LLM_BUDGET_POLICY", path)
	if err := os.WriteFile(path, []byte(`{"version":1}`), 0600); err != nil {
		t.Fatal(err)
	}
	restore := llmbudget.SetDefault(llmbudget.New(llmbudget.Config{}))
	defer restore()
	p := fixtureQuotaProbe()
	resolve := func(string) ([]quotaProbe, error) { return []quotaProbe{p}, nil }
	for _, verb := range []string{"enable", "disable"} {
		change := llmbudget.EnableProbe
		if verb == "disable" {
			change = llmbudget.DisableProbe
		}
		out, _, err := executeQuota(modelQuotaConsentCommand(verb, resolve, change), "fixture/q")
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out, p.ID) || !strings.Contains(out, "effects: net") {
			t.Fatal(out)
		}
		raw, _ := os.ReadFile(path)
		var policy llmbudget.Policy
		if err = json.Unmarshal(raw, &policy); err != nil {
			t.Fatal(err)
		}
		if verb == "enable" && (len(policy.ProbeConsent) != 1 || policy.ProbeConsent[0] != p.ID) {
			t.Fatal(string(raw))
		}
		if verb == "disable" && len(policy.ProbeConsent) != 0 {
			t.Fatal(string(raw))
		}
	}
}

func isolateQuotaCatalog(t *testing.T) {
	t.Helper()
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		if strings.HasPrefix(key, "BASHY_") {
			t.Setenv(key, "")
		}
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
}
func TestQuotaResolveProbes(t *testing.T) {
	isolateQuotaCatalog(t)
	dir := t.TempDir()
	t.Setenv("BASHY_PLANS_DIR", dir)
	if err := os.WriteFile(filepath.Join(dir, "quota-fixture.yaml"), []byte("name: quota-fixture\nvendor: Fixture\nusage:\n- id: q\n  run:\n    body: 'echo fixture'\n    effects: [net]\n- id: read\n  run: 'echo fixture'\n"), 0600); err != nil {
		t.Fatal(err)
	}
	p, err := resolveQuotaProbes("quota-fixture")
	if err != nil || len(p) != 2 || p[0].ID != "probe:plan:quota-fixture/q" {
		t.Fatalf("%+v %v", p, err)
	}
	p, err = resolveQuotaProbes("quota-fixture/read")
	if err != nil || len(p) != 1 || p[0].Plan != "quota-fixture" {
		t.Fatalf("%+v %v", p, err)
	}
	for _, s := range []string{"quota-fixture/absent", "quota-fixture/", "/q", "not-a-real-quota-record"} {
		if _, err := resolveQuotaProbes(s); err == nil {
			t.Fatalf("accepted %q", s)
		}
	}
	cat := fleet.New()
	if err := cat.SaveModel(fleet.Model{Name: "quota-fixture-model", Provider: "fixture", Kind: fleet.ModelKindSubscription, Plan: "quota-fixture", Usage: []fleet.UsageProbe{{ID: "m", Run: snippet.Snippet{Body: "echo fixture"}}}}); err != nil {
		t.Fatal(err)
	}
	p, err = resolveQuotaProbes("quota-fixture-model/m")
	if err != nil || len(p) != 1 || p[0].ID != "probe:model:quota-fixture-model/m" || p[0].Model != "quota-fixture-model" {
		t.Fatalf("%+v %v", p, err)
	}
}
func TestQuotaViewDoesNotRunDisabledProbe(t *testing.T) {
	isolateQuotaCatalog(t)
	cat := fleet.New()
	if err := cat.SaveModel(fleet.Model{Name: "quota-consent-fixture", Provider: "fixture", Kind: fleet.ModelKindSubscription, Usage: []fleet.UsageProbe{{ID: "q", Run: snippet.Snippet{Body: "echo should-not-run", Effects: []string{"net"}}}}}); err != nil {
		t.Fatal(err)
	}
	calls := 0
	gate := llmbudget.New(llmbudget.Config{Policy: &llmbudget.Policy{Version: 1}, StatePath: filepath.Join(t.TempDir(), "state.json"), Adapters: []llmbudget.Adapter{quotaCountingAdapter{&calls}}})
	cmd := modelQuotaCommand(gate.Quota, func([]llmbudget.QuotaRow, time.Time) error { return nil }, quotaNow)
	_, _, err := executeQuota(cmd, "--provider", "fixture", "--refresh", "--json")
	if err != nil || calls != 0 {
		t.Fatalf("disabled probe called %d times: %v", calls, err)
	}
}

type quotaCountingAdapter struct{ calls *int }

func (quotaCountingAdapter) Kind() string { return "probe" }
func (a quotaCountingAdapter) Collect(context.Context, llmbudget.SourceConfig, time.Time) (llmbudget.SourceResult, error) {
	*a.calls++
	return llmbudget.SourceResult{}, errors.New("disabled probe executed")
}

func TestQuotaRealSnippetRedaction(t *testing.T) {
	// Pure local fixture proves the production snippet wrapper redacts declared
	// credential values. No credential resolver or account is contacted.
	t.Setenv("QUOTA_TEST_SECRET", "quota-fixture-secret")
	p := quotaProbe{ID: "probe:model:fixture/q", Probe: fleet.UsageProbe{Run: snippet.Snippet{Body: "echo ok; echo $TOKEN >&2", Effects: []string{"pure"}, Env: map[string]string{"TOKEN": "env:QUOTA_TEST_SECRET"}}}}
	r, err := runQuotaProbe(context.Background(), p)
	if err != nil || r.Exit != 0 || strings.Contains(string(r.Stderr), "quota-fixture-secret") || !strings.Contains(string(r.Stderr), "***") {
		t.Fatalf("redaction failed: exit=%d err=%v", r.Exit, err)
	}
}
