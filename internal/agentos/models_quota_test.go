package agentos

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/qiangli/yoke/pkg/fleet"
	"github.com/qiangli/yoke/pkg/llmbudget"
	"github.com/qiangli/yoke/pkg/quotaobs"
	"github.com/qiangli/yoke/pkg/snippet"
	"github.com/qiangli/yoke/pkg/weave"
	"github.com/spf13/cobra"
)

func quotaNow() time.Time    { return time.Date(2026, 10, 19, 13, 24, 0, 0, time.UTC) }
func quotaPtr[T any](v T) *T { return &v }
func quotaFixture() []llmbudget.QuotaRow {
	return []llmbudget.QuotaRow{
		{Account: "claude-max", Label: "claude", Configured: true, Provider: "anthropic", Plan: "max", Window: "5h", UsedPercent: quotaPtr(62.0), ResetAt: quotaPtr(quotaNow().Add(100 * time.Minute)), Tokens: quotaPtr(1200.0), Source: "fixture", ObservedAt: quotaPtr(quotaNow().Add(-90 * time.Second)), Status: "ok", Headroom: quotaPtr(.38), SuggestedWorkers: quotaPtr(2), Classification: "measured", Predict: "warn in 45m", Immune: "warn"},
		{Account: "muse-pro", Label: "muse", Provider: "openai-compat", Configured: true, Status: "rate-limited", Source: "vendor-error (matcher) 2m", ObservedAt: quotaPtr(quotaNow().Add(-2 * time.Minute)), Classification: "observed"},
		{Account: "chatgpt-pro", Label: "codex", Provider: "openai", Configured: true, Window: "week", UsedPercent: quotaPtr(12.0), Status: "ok"},
		{Account: "google-pro", Label: "agy", Provider: "google", Configured: true, Status: "unmeasured"},
		{Account: "glm-pro", Label: "glm", Provider: "openai-compat", Configured: true, Window: "5h", Limit: "12000 credits/5h", Status: "unmeasured"},
		{Account: "deepseek-key", Label: "deepseek", Provider: "openai-compat", Configured: true, Balance: quotaPtr(4.5), BalanceUnit: "USD", Status: "ok"},
		{Account: "kimi-key", Label: "kimi", Provider: "openai-compat", Status: "not-configured"},
		{Account: "local", Label: "local", Provider: "ollama", Configured: true, Status: "unmeasured"},
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
	isolateQuotaCatalog(t)
	for _, refresh := range []bool{false, true} {
		t.Run(map[bool]string{false: "cached", true: "refresh"}[refresh], func(t *testing.T) {
			collect := func(ctx context.Context, opt llmbudget.QuotaOptions) ([]llmbudget.QuotaRow, error) {
				if !opt.All || opt.Provider != "claude" || opt.Refresh != refresh || !opt.Now.Equal(quotaNow()) {
					t.Fatalf("options %+v", opt)
				}
				deadline, ok := ctx.Deadline()
				limit := 5 * time.Second
				if refresh {
					limit = 30 * time.Second
				}
				if !ok || time.Until(deadline) > limit || time.Until(deadline) < limit-time.Second {
					t.Fatal("deadline", deadline)
				}
				return quotaFixture(), nil
			}
			ingested := 0
			cmd := modelQuotaCommandWithMetadataAndIngest(collect, func([]llmbudget.QuotaRow, time.Time) error { return nil }, quotaNow, func() ([]quotaProbeMetadata, error) { return []quotaProbeMetadata{{timeout: 25 * time.Second}}, nil }, func(context.Context, time.Time) error { ingested++; return nil })
			args := []string{"--all", "--json", "--provider", "claude"}
			if refresh {
				args = append(args, "--refresh")
			}
			out, _, err := executeQuota(cmd, args...)
			if err != nil {
				t.Fatal(err)
			}
			if ingested != map[bool]int{false: 0, true: 1}[refresh] {
				t.Fatalf("ingest calls: %d", ingested)
			}
			var envelope struct {
				Schema     string `json:"schema_version"`
				Kind, View string
				Items      []map[string]any
			}
			if err = json.Unmarshal([]byte(out), &envelope); err != nil {
				t.Fatal(err)
			}
			if envelope.Schema != fleet.ListSchemaVersion || envelope.Kind != "quota" || envelope.View != "all" || len(envelope.Items) != 8 {
				t.Fatal(out)
			}
			for _, r := range envelope.Items {
				for _, key := range []string{"headroom", "stale", "suggested_workers", "classification", "status", "account", "label", "configured", "provider"} {
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
	rows := []llmbudget.QuotaRow{{Account: "plan-a", Provider: "vendor", Status: "unmeasured"}, {Account: "plan-a", Provider: "vendor", ResetAt: quotaPtr(quotaNow().Add(2 * time.Hour)), Status: "ok"}, {Account: "plan-b", Provider: "vendor", Status: "ok"}}
	agents := []fleet.Agent{{Name: "agent", Tool: "fixture", Model: "model"}}
	model := func(string) (fleet.Model, bool) { return fleet.Model{Provider: "vendor", Plan: "plan-a"}, true }
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
	for _, verb := range []string{"enable", "disable"} {
		out, _, err := executeQuota(modelQuotaConsentCommand(verb, resolve, llmbudget.EnableProbe), "analyzer")
		if err != nil || !strings.Contains(out, verb+" analyzer") {
			t.Fatalf("%q %v", out, err)
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var policy llmbudget.Policy
		if err := json.Unmarshal(raw, &policy); err != nil {
			t.Fatal(err)
		}
		if policy.Analyzer.Enabled != (verb == "enable") {
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
	t.Setenv("BASHY_HOME", filepath.Join(home, ".bashy"))
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
func (a quotaCountingAdapter) Collect(_ context.Context, source llmbudget.SourceConfig, _ time.Time) (llmbudget.SourceResult, error) {
	if source.ID != "probe:model:quota-consent-fixture/q" {
		return llmbudget.SourceResult{}, nil
	}
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

func TestQuotaSortWindows(t *testing.T) {
	rows := []llmbudget.QuotaRow{}
	for _, w := range []string{"-", "concurrent", "month", "week", "day", "5h"} {
		rows = append(rows, llmbudget.QuotaRow{Label: "same", Account: "a", Window: w})
	}
	rows = append(rows, llmbudget.QuotaRow{Label: "aaa", Window: "month"}, llmbudget.QuotaRow{Label: "zzz", Window: "5h"}, llmbudget.QuotaRow{Label: "same", Account: "z", Window: "5h"})
	sortQuotaRows(rows)
	var got []string
	for _, r := range rows {
		got = append(got, r.Label+":"+r.Window)
	}
	want := "aaa:month same:5h same:5h same:day same:week same:month same:concurrent same:- zzz:5h"
	if strings.Join(got, " ") != want {
		t.Fatal(got)
	}
}

func TestQuotaHints(t *testing.T) {
	missing := llmbudget.QuotaRow{Account: "claude-max", Status: "not-configured", Limitations: []string{"no snapshot at /fixture/.bashy/quota/claude.json"}}
	rows := []llmbudget.QuotaRow{missing, missing, {Account: "deepseek-key", Status: "unmeasured"}}
	probes := []quotaProbeMetadata{
		{account: "deepseek-key", selector: "deepseek/balance", net: true},
		{account: "deepseek-key", selector: "deepseek/balance", net: true},
		{account: "deepseek-key", selector: "deepseek/read"},
		{account: "absent", selector: "absent/balance", net: true},
	}
	cmd := modelQuotaCommandWithMetadata(func(context.Context, llmbudget.QuotaOptions) ([]llmbudget.QuotaRow, error) { return rows, nil }, func([]llmbudget.QuotaRow, time.Time) error { return nil }, quotaNow, func() ([]quotaProbeMetadata, error) { return probes, nil })
	out, stderr, err := executeQuota(cmd)
	want := "hint: bashy model quota enable deepseek/balance\nhint: register bashy model quota capture --to ~/.bashy/quota/claude.json as the Claude statusLine command\n"
	if err != nil || stderr != "" || !strings.HasSuffix(out, want) || strings.Count(out, "hint:") != 2 {
		t.Fatalf("%q %q %v", out, stderr, err)
	}
	rows[0].Status, rows[1].Status = "ok", "not-configured"
	rows[1].Limitations = []string{"missing credential"}
	var buf bytes.Buffer
	if err := renderQuotaHints(&buf, rows, nil); err != nil || buf.Len() != 0 {
		t.Fatalf("%q %v", buf.String(), err)
	}
}

func TestQuotaHintDedupesFreshAccount(t *testing.T) {
	rows := []llmbudget.QuotaRow{{Account: "a", Source: "probe:plan:a/fast", ObservedAt: quotaPtr(quotaNow().Add(-time.Hour)), Stale: true, Status: "ok"}, {Account: "b", Status: "unmeasured"}}
	probes := []quotaProbeMetadata{{id: "probe:plan:a/fast", account: "a", selector: "a/fast", net: true, enabled: true}, {account: "a", selector: "a/slow", net: true}, {account: "b", selector: "b/z", net: true}, {account: "b", selector: "b/a", net: true}}
	var out bytes.Buffer
	if err := renderQuotaHints(&out, rows, probes); err != nil {
		t.Fatal(err)
	}
	if got, want := out.String(), "hint: bashy model quota enable b/a\n"; got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestQuotaHintRespectsSiblingModelConsent(t *testing.T) {
	isolateQuotaCatalog(t)
	policyPath := filepath.Join(t.TempDir(), "policy.json")
	t.Setenv("BASHY_LLM_BUDGET_POLICY", policyPath)
	if err := os.WriteFile(policyPath, []byte(`{"version":1,"probe_consent":["probe:model:kimi-k3/balance"]}`), 0600); err != nil {
		t.Fatal(err)
	}
	probes, err := loadQuotaProbeMetadata()
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	rows := []llmbudget.QuotaRow{{Account: "moonshot", Status: "unmeasured", Stale: true}}
	if err := renderQuotaHints(&out, rows, probes); err != nil || strings.Contains(out.String(), "kimi-k2.6/balance") || strings.Contains(out.String(), "quota enable") {
		t.Fatalf("unexpected sibling hint: %q %v", out.String(), err)
	}
}

func TestQuotaIngestBackfillsWeaveLogs(t *testing.T) {
	isolateQuotaCatalog(t)
	t.Setenv("BASHY_MODELS_DIR", t.TempDir())
	cat := fleet.New()
	for _, model := range []fleet.Model{
		{Name: "backfill-codex", Provider: "openai", Kind: fleet.ModelKindSubscription, Plan: "account-a"},
		{Name: "backfill-zai", Provider: "zai", Kind: fleet.ModelKindSubscription, Plan: "account-a"},
		{Name: "backfill-gemini", Provider: "google", Kind: fleet.ModelKindSubscription, Plan: "account-b"},
	} {
		if err := cat.SaveModel(model); err != nil {
			t.Fatal(err)
		}
	}
	if err := cat.SaveAgent(fleet.Agent{Name: "backfill-agent", Tool: "codex", Model: "backfill-codex"}); err != nil {
		t.Fatal(err)
	}
	queueDir := filepath.Join(os.Getenv("HOME"), ".bashy", "weave", "quota-fixture")
	logsDir := filepath.Join(queueDir, "logs")
	if err := os.MkdirAll(logsDir, 0700); err != nil {
		t.Fatal(err)
	}
	queue := `{"items":[{"id":1,"tool":"codex","owner":"codex-a","launch_spec":{"agent":"backfill-agent","model":"provider-side-codex"}},{"id":2,"tool":"opencode","owner":"glm-b","launch_spec":{"model":"backfill-zai"}},{"id":3,"tool":"agy","owner":"agy-c","launch_spec":{"model":"backfill-gemini"}},{"id":4,"tool":"codex","launch_spec":{"model":"backfill-codex"}}]}`
	if err := os.WriteFile(filepath.Join(queueDir, "queue.json"), []byte(queue), 0600); err != nil {
		t.Fatal(err)
	}
	fixtures := []string{
		strings.Repeat("ordinary output\n", 2000) + "You've hit your usage limit. Try again at 11:41 AM.\n",
		`{"error":{"code":"1308","message":"Usage limit reached for 5 hour. Your limit will reset at 2026-10-19 17:34:44"}}`,
		`{"error":{"code":429,"message":"Quota exceeded","status":"RESOURCE_EXHAUSTED"}}`,
		"old usage limit reached",
	}
	for i, fixture := range fixtures {
		path := filepath.Join(logsDir, fmt.Sprintf("issue-%d.log", i+1))
		if err := os.WriteFile(path, []byte(fixture), 0600); err != nil {
			t.Fatal(err)
		}
		mtime := quotaNow().Add(-time.Hour)
		if i == 3 {
			mtime = quotaNow().Add(-31 * 24 * time.Hour)
		}
		if err := os.Chtimes(path, mtime, mtime); err != nil {
			t.Fatal(err)
		}
	}
	if dirs := quotaQueueDirs(); len(dirs) != 1 || dirs[0] != queueDir {
		t.Fatalf("queue discovery: %v", dirs)
	}
	for pass := 0; pass < 2; pass++ {
		out, _, err := executeQuota(modelQuotaIngestCommand(func(context.Context, time.Time) error { return nil }, quotaNow), "--logs-since", "30d")
		if err != nil || !strings.Contains(out, "account-a weave: 2") || !strings.Contains(out, "account-b weave: 1") {
			t.Fatalf("pass %d: %q %v", pass, out, err)
		}
		observations, err := quotaobs.Read(time.Time{}, nil)
		if err != nil || len(observations) != 3 {
			t.Fatalf("pass %d: %d observations, %v", pass, len(observations), err)
		}
		for _, o := range observations {
			if o.Source != "weave" || o.Model == "" || o.Account == "" || o.Agent == "" {
				t.Fatalf("incomplete observation: %+v", o)
			}
		}
	}
	out, _, err := executeQuota(modelQuotaIngestCommand(func(context.Context, time.Time) error { return nil }, quotaNow), "--logs-since", "bad")
	if err == nil || out != "" {
		t.Fatalf("invalid duration accepted: %q %v", out, err)
	}
}

func TestQuotaLogFilterAndJSON(t *testing.T) {
	var got llmbudget.LogFilter
	entry := llmbudget.LogEntry{At: quotaNow(), Source: "vendor-error", Account: "a", Tool: "claude", Excerpt: "redacted fixture", Parsed: &quotaobs.ParsedSignal{Kind: "exhausted", Window: "5h", ResetAt: quotaPtr(quotaNow().Add(time.Hour))}, Classification: "observed", Analysis: "limit reached"}
	log := func(_ context.Context, f llmbudget.LogFilter) []llmbudget.LogEntry {
		got = f
		return []llmbudget.LogEntry{entry}
	}
	out, _, err := executeQuota(modelQuotaLogCommand(log, quotaNow), "--account", "a", "--since", "2h", "--limit", "7")
	if err != nil || got.Account != "a" || !got.Since.Equal(quotaNow().Add(-2*time.Hour)) || got.Limit != 7 || !strings.Contains(out, "exhausted") || strings.Contains(out, entry.Excerpt) {
		t.Fatalf("%+v %q %v", got, out, err)
	}
	out, _, err = executeQuota(modelQuotaLogCommand(log, quotaNow), "--json")
	if err != nil || !strings.Contains(out, `"excerpt":"redacted fixture"`) {
		t.Fatalf("%q %v", out, err)
	}
}

func TestQuotaAnalyzeConsentHint(t *testing.T) {
	called := 0
	analyze := func(_ context.Context, _ llmbudget.AnalyzeOptions) (llmbudget.AnalyzeResult, error) {
		called++
		return llmbudget.AnalyzeResult{Hint: "enable the analyzer with quota enable analyzer"}, nil
	}
	log := func(context.Context, llmbudget.LogFilter) []llmbudget.LogEntry {
		return []llmbudget.LogEntry{{Classification: "observed"}}
	}
	out, _, err := executeQuota(modelQuotaAnalyzeCommand(analyze, log, quotaNow))
	if err != nil || called != 1 || !strings.Contains(out, "quota enable analyzer") {
		t.Fatalf("%q %v", out, err)
	}
	out, _, err = executeQuota(modelQuotaAnalyzeCommand(analyze, log, quotaNow), "--dry-run")
	if err != nil || called != 1 || !strings.Contains(out, "1 observations") {
		t.Fatalf("%q %v", out, err)
	}
}

func TestQuotaForecastRender(t *testing.T) {
	f := llmbudget.AccountForecast{Account: "a", Window: "5h", UsedPercent: 62, TokensPerPercent: 123, CoefficientOfVariation: .2, BurnRate: 14, P50: quotaPtr(quotaNow().Add(time.Hour)), P90: quotaPtr(quotaNow().Add(2 * time.Hour)), ResetAt: quotaNow().Add(3 * time.Hour), Verdict: "warning", Confidence: .8, Regime: "stable"}
	forecast := func(context.Context, time.Time) []llmbudget.AccountForecast {
		return []llmbudget.AccountForecast{f, {Account: "b"}}
	}
	out, _, err := executeQuota(modelQuotaForecastCommand(forecast, quotaNow), "--account", "a")
	if err != nil || !strings.Contains(out, "TOKENS/%") || !strings.Contains(out, "123.0") || !strings.Contains(out, "warning") || strings.Contains(out, " b ") {
		t.Fatalf("%q %v", out, err)
	}
	out, _, err = executeQuota(modelQuotaForecastCommand(forecast, quotaNow), "--account", "a", "--json")
	if err != nil || !strings.Contains(out, `"account":"a"`) || strings.Contains(out, `"account":"b"`) {
		t.Fatalf("%q %v", out, err)
	}
}

func TestQuotaRefreshTimeout(t *testing.T) {
	for _, tc := range []struct{ declared, want time.Duration }{
		{0, 15 * time.Second}, {2 * time.Second, 7 * time.Second}, {25 * time.Second, 30 * time.Second}, {30 * time.Second, 35 * time.Second}, {time.Minute, 40 * time.Second},
	} {
		if got := quotaRefreshTimeout([]quotaProbeMetadata{{timeout: tc.declared}}); got != tc.want {
			t.Fatalf("%s: %s != %s", tc.declared, got, tc.want)
		}
	}
}

func TestQuotaMetadata(t *testing.T) {
	isolateQuotaCatalog(t)
	cat := fleet.New()
	m := fleet.Model{Name: "quota-metadata-fixture", Provider: "fixture", APIKeyRef: "fixture-account", Usage: []fleet.UsageProbe{{ID: "balance", Run: snippet.Snippet{Body: "echo fixture", Effects: []string{"read", "net"}, Timeout: 25 * time.Second}}}}
	if err := cat.SaveModel(m); err != nil {
		t.Fatal(err)
	}
	policy := filepath.Join(t.TempDir(), "policy.json")
	t.Setenv("BASHY_LLM_BUDGET_POLICY", policy)
	for _, enabled := range []bool{false, true} {
		p := llmbudget.Policy{Version: 1}
		if enabled {
			p.ProbeConsent = []string{"probe:model:quota-metadata-fixture/balance"}
		}
		raw, _ := json.Marshal(p)
		if err := os.WriteFile(policy, raw, 0600); err != nil {
			t.Fatal(err)
		}
		metadata, err := loadQuotaProbeMetadata()
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, p := range metadata {
			if p.account == "fixture-account" {
				found = true
				if p.enabled != enabled || !p.net || p.timeout != 25*time.Second || p.selector != "quota-metadata-fixture/balance" {
					t.Fatalf("%+v", p)
				}
			}
		}
		if !found {
			t.Fatal("missing fixture probe")
		}
	}
}

func TestQuotaCooldownAccountTools(t *testing.T) {
	models := map[string]fleet.Model{
		"subscription": {Provider: "shared", Plan: "plan", APIKeyRef: "ignored"},
		"api":          {Provider: "shared", APIKeyRef: "key"},
		"probe":        {Name: "probe", Provider: "shared", Usage: []fleet.UsageProbe{{ID: "q"}}},
		"local-model":  {Provider: "shared", Kind: fleet.ModelKindLocal, Plan: "ignored"},
	}
	agents := []fleet.Agent{
		{Name: "subscription-agent", Model: "subscription", Tool: "claude"},
		{Name: "api-agent", Model: "api", Tool: "opencode"},
		{Name: "probe-agent", Model: "probe", Tool: "probe-tool"},
		{Name: "local-agent", Model: "local-model", Tool: "local-tool"},
	}
	rows := []llmbudget.QuotaRow{}
	for _, account := range []string{"plan", "key", "probe", "local", "unrelated"} {
		rows = append(rows, llmbudget.QuotaRow{Account: account, Label: account, Provider: "shared", Status: "unmeasured"})
	}
	calls := map[string]int{}
	status := func(_ []string, tool, model, agent string, _ time.Time) (bool, string) {
		calls[tool]++
		if tool == "opencode" {
			return false, ""
		}
		return true, "cooling-down until " + quotaNow().Add(time.Hour).Format(time.RFC3339)
	}
	err := overlayQuotaSeats(rows, agents, func(name string) (fleet.Model, bool) { m, ok := models[name]; return m, ok }, nil, status, quotaNow())
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rows {
		want := "cooling-down"
		if r.Account == "key" || r.Account == "unrelated" {
			want = "unmeasured"
		}
		if r.Status != want || r.Label != r.Account {
			t.Fatalf("%+v", r)
		}
	}
	for _, tool := range []string{"claude", "opencode", "probe-tool", "local-tool"} {
		if calls[tool] != 1 {
			t.Fatal(calls)
		}
	}
}
