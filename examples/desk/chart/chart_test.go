package main

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	kit "desk-kit"
	"desk-kit/contracttest"
)

func startServer(t *testing.T) (*app, *httptest.Server) {
	t.Helper()
	a := &app{dir: t.TempDir(), items: map[string]*item{}}
	s := httptest.NewServer(a)
	a.baseURL = s.URL
	t.Cleanup(s.Close)
	return a, s
}
func TestDeskContract(t *testing.T) {
	_, s := startServer(t)
	contracttest.Run(t, contracttest.Config{BaseURL: s.URL, Tool: "chart"})
}
func TestSVGGoldens(t *testing.T) {
	for _, kind := range widgetKinds {
		t.Run(kind, func(t *testing.T) {
			s := series{Name: "Requests <safe>", Unit: "ms", Kind: kind, Points: []point{{X: 0, Y: 2, Label: "0"}, {X: 2, Y: 6, Label: "2"}, {X: 5, Y: 9, Label: "5"}}, Buckets: []bucket{{0, 10, 2}, {10, 20, 6}, {20, 40, 2}}}
			got, e := renderSVG(s, kind)
			if e != nil {
				t.Fatal(e)
			}
			d := xml.NewDecoder(strings.NewReader(got))
			for {
				_, e = d.Token()
				if e == io.EOF {
					break
				}
				if e != nil {
					t.Fatal(e)
				}
			}
			path := filepath.Join("testdata", kind+".svg")
			if os.Getenv("UPDATE_GOLDEN") == "1" {
				if e = os.WriteFile(path, []byte(got+"\n"), 0600); e != nil {
					t.Fatal(e)
				}
			}
			want, e := os.ReadFile(path)
			if e != nil {
				t.Fatal(e)
			}
			if got+"\n" != string(want) {
				t.Fatalf("SVG differs from %s", path)
			}
		})
	}
}
func TestOTLPFixture(t *testing.T) {
	raw, e := os.ReadFile("testdata/otlp.json")
	if e != nil {
		t.Fatal(e)
	}
	ss, e := parseData(raw, "otlp")
	if e != nil {
		t.Fatal(e)
	}
	if len(ss) != 5 {
		t.Fatalf("series: %d", len(ss))
	}
	want := []string{"counter", "gauge", "updowncounter", "histogram", "exponentialhistogram"}
	for i, s := range ss {
		if s.Kind != want[i] {
			t.Fatalf("kind %d: %s", i, s.Kind)
		}
		got, e := renderSVG(s, "auto")
		if e != nil {
			t.Fatal(e)
		}
		p := filepath.Join("testdata", "otlp-"+s.Kind+".svg")
		if os.Getenv("UPDATE_GOLDEN") == "1" {
			if e = os.WriteFile(p, []byte(got+"\n"), 0600); e != nil {
				t.Fatal(e)
			}
		}
		b, e := os.ReadFile(p)
		if e != nil {
			t.Fatal(e)
		}
		if got+"\n" != string(b) {
			t.Fatalf("fixture SVG differs: %s", p)
		}
	}
	rates := ratePoints(ss[0])
	if len(rates) != 2 || rates[0].Y != 3 || rates[1].Y != 2 {
		t.Fatalf("rates: %+v", rates)
	}
	if q := quantile(ss[3].Buckets, .5); q != "≈ 15" {
		t.Fatal(q)
	}
	if q := quantile(ss[4].Buckets, .95); q != "≈ 3.75" {
		t.Fatal(q)
	}
}
func TestInputFormats(t *testing.T) {
	for _, raw := range []string{`[1,2,3]`, `[{"time":0,"value":1},{"time":1,"value":2},{"time":2,"value":3}]`, "time,value\n0,1\n1,2\n2,3\n", "{\"time\":0,\"value\":1}\n{\"time\":1,\"value\":2}\n{\"time\":2,\"value\":3}"} {
		ss, e := parseData([]byte(raw), "auto")
		if e != nil {
			t.Fatal(e)
		}
		if len(ss) != 1 || len(ss[0].Points) != 3 || ss[0].Points[2].Y != 3 {
			t.Fatalf("bad parse: %+v", ss)
		}
	}
	for _, raw := range []string{"", `["NaN"]`, `["Inf"]`, `{"type":"secret","value":2}`, `{"password":2}`, "password,value\nx,1", `{"fields":[{"name":"token","type":"secret"}]}`, `{"x":1} {"x":2}`, `[]`, "value\nNaN"} {
		if _, e := parseData([]byte(raw), "auto"); e == nil {
			t.Errorf("accepted invalid/secret: %q", raw)
		}
	}
	if _, e := parseData(bytes.Repeat([]byte{'1'}, maxInput+1), "json"); e == nil {
		t.Fatal("unbounded input")
	}
}
func TestCounterSemantics(t *testing.T) {
	tests := []struct {
		name string
		s    series
		want []float64
	}{
		{"cumulative", series{Points: []point{{X: 1, Y: 5}, {X: 3, Y: 11}, {X: 4, Y: 13}}}, []float64{3, 2}},
		{"delta", series{Delta: true, Points: []point{{Nano: 3e9, Start: 1e9, Y: 6}, {Nano: 5e9, Start: 3e9, Y: 8}}}, []float64{3, 4}},
		{"reset", series{Points: []point{{Nano: 3e9, Start: 1e9, Y: 8}, {Nano: 5e9, Start: 4e9, Y: 3}}}, []float64{3}},
		{"unknown reset", series{Points: []point{{X: 1, Y: 10}, {X: 2, Y: 2}, {X: 4, Y: 8}}}, []float64{3}},
		{"duplicate time", series{Points: []point{{X: 1, Y: 2}, {X: 1, Y: 4}}}, nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ratePoints(tc.s)
			if len(got) != len(tc.want) {
				t.Fatalf("%+v", got)
			}
			for i, v := range tc.want {
				if got[i].Y != v {
					t.Fatalf("%+v", got)
				}
			}
		})
	}
}
func TestHistogramValidation(t *testing.T) {
	for _, p := range []otlpPoint{{Count: 2, Bounds: []num{1}, Counts: []num{2}}, {Count: 3, Bounds: []num{2, 1}, Counts: []num{1, 1, 1}}, {Count: 1, Counts: []num{-1}}, {Count: 3, Counts: []num{2}}} {
		if _, e := histBuckets(p, "histogram"); e == nil {
			t.Fatal("invalid histogram accepted")
		}
	}
	bs := []bucket{{math.Inf(-1), 10, 1}, {10, 20, 8}, {20, math.Inf(1), 1}}
	if quantile(bs, .01) != "≤ 10" || quantile(bs, .99) != "≥ 20" || quantile(bs, .5) != "≈ 15" {
		t.Fatal("quantile semantics")
	}
	if quantile(nil, .5) != "—" {
		t.Fatal("empty quantile")
	}
}
func post(t *testing.T, client *http.Client, base, key string, p any) (int, map[string]any) {
	t.Helper()
	raw, _ := json.Marshal(p)
	r, _ := http.NewRequest("POST", base+"/api/action", bytes.NewReader(raw))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("X-Facilitator-Key", key)
	res, e := client.Do(r)
	if e != nil {
		t.Fatal(e)
	}
	defer res.Body.Close()
	var v map[string]any
	if e = json.NewDecoder(res.Body).Decode(&v); e != nil {
		t.Fatal(e)
	}
	return res.StatusCode, v
}
func TestCLIAndHTTPDuality(t *testing.T) {
	_, s := startServer(t)
	addr := strings.TrimPrefix(s.URL, "http://")
	var out bytes.Buffer
	run := func(args ...string) map[string]any {
		t.Helper()
		out.Reset()
		args = append(args, "--addr", addr, "--json")
		if c := cli(args, strings.NewReader("[1,2,3]"), &out); c != 0 {
			t.Fatalf("exit %d: %s", c, out.String())
		}
		var m map[string]any
		if e := json.Unmarshal(out.Bytes(), &m); e != nil {
			t.Fatal(e)
		}
		return m
	}
	created := run("new", "--id", "probe", "--input", "-", "--widget", "gauge")
	key := created["ownerKey"].(string)
	opened := run("open", "probe")
	if len(opened) != 3 {
		t.Fatalf("open leaked content: %v", opened)
	}
	got := run("get", "probe", "--key", key)
	if !strings.Contains(got["html"].(string), "<svg") {
		t.Fatal("no SVG")
	}
	code, httpGot := post(t, s.Client(), s.URL, key, map[string]any{"action": "get", "id": "probe"})
	if code != 200 || kit.CanonicalJSON(got) != kit.CanonicalJSON(httpGot) {
		t.Fatal("doors differ")
	}
	run("put", "probe", "--key", key, "--input", "-", "--widget", "bar")
	shared := run("share", "probe", "--key", key)
	jar, _ := cookiejar.New(nil)
	guest := &http.Client{Jar: jar}
	code, _ = post(t, guest, s.URL, "", map[string]any{"action": "get", "id": "probe"})
	if code != 403 {
		t.Fatal("anonymous read")
	}
	code, _ = post(t, guest, s.URL, "", map[string]any{"action": "join", "id": "probe", "invite": shared["invite"]})
	if code != 200 {
		t.Fatal("join")
	}
	code, v := post(t, guest, s.URL, "", map[string]any{"action": "get", "id": "probe"})
	if code != 200 || !strings.Contains(v["html"].(string), "bar") {
		t.Fatal("guest view")
	}
	code, _ = post(t, s.Client(), s.URL, "", map[string]any{"action": "join", "id": "probe", "invite": shared["invite"]})
	if code != 403 {
		t.Fatal("invite reused")
	}
	run("revoke", "probe", "--key", key)
	code, _ = post(t, guest, s.URL, "", map[string]any{"action": "get", "id": "probe"})
	if code != 403 {
		t.Fatal("revoke failed")
	}
	run("close", "probe", "--key", key)
	code, _ = post(t, s.Client(), s.URL, key, map[string]any{"action": "put", "id": "probe", "chart": chartConfig{Input: "[99]"}})
	if code != 409 {
		t.Fatal("closed mutation")
	}
}
func TestRefreshAndSnapshot(t *testing.T) {
	a, s := startServer(t)
	path := filepath.Join(t.TempDir(), "data.json")
	if e := os.WriteFile(path, []byte("[1]"), 0600); e != nil {
		t.Fatal(e)
	}
	code, v := post(t, s.Client(), s.URL, "", map[string]any{"action": "new", "id": "live", "chart": chartConfig{Source: path, Refresh: "1s"}})
	if code != 200 {
		t.Fatal(v)
	}
	key := v["ownerKey"].(string)
	if e := os.WriteFile(path, []byte("[9]"), 0600); e != nil {
		t.Fatal(e)
	}
	a.mu.Lock()
	a.items["live"].Chart.Updated = time.Now().Add(-2 * time.Second)
	a.mu.Unlock()
	_, v = post(t, s.Client(), s.URL, key, map[string]any{"action": "get", "id": "live"})
	if v["chart"].(map[string]any)["input"] != "[9]" {
		t.Fatal(v)
	}
	if e := os.WriteFile(path, []byte("invalid"), 0600); e != nil {
		t.Fatal(e)
	}
	a.mu.Lock()
	a.items["live"].Chart.Updated = time.Now().Add(-2 * time.Second)
	a.mu.Unlock()
	_, v = post(t, s.Client(), s.URL, key, map[string]any{"action": "get", "id": "live"})
	if v["refreshError"] == "" || v["chart"].(map[string]any)["input"] != "[9]" {
		t.Fatal("failed refresh lost last good data")
	}
	c := chartConfig{Input: "[2]"}
	if e := prepareConfig(&c); e != nil {
		t.Fatal(e)
	}
	if c.Source != "" || c.Refresh != "" {
		t.Fatal("not snapshot")
	}
	for _, src := range []string{"https://127.0.0.1/x", "http://example.com/x", "http://localhost/x", "http://user:pass@127.0.0.1/x", "relative.json"} {
		if checkSource(src) == nil {
			t.Fatalf("unsafe source: %s", src)
		}
	}
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, "http://example.com/", 302) }))
	defer redirect.Close()
	if _, e := readSource(redirect.URL); e == nil {
		t.Fatal("followed redirect")
	}
	local := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "[42]") }))
	defer local.Close()
	c = chartConfig{Source: local.URL, Refresh: "1s"}
	if e := prepareConfig(&c); e != nil || c.Input != "[42]" {
		t.Fatalf("loopback refresh %v", e)
	}
}
func TestShapedTemplateAndEscaping(t *testing.T) {
	c := chartConfig{Input: `[{"name":"<script>bad()</script>","value":3}]`, Title: "<unsafe>", Template: `<h2>{{.Title}}</h2>{{range .Widgets}}{{.}}{{end}}`}
	html, e := renderCharts(c)
	if e != nil {
		t.Fatal(e)
	}
	if strings.Contains(html, "<unsafe>") || strings.Contains(html, "<script>bad") {
		t.Fatal("unescaped input")
	}
	if !strings.Contains(html, "<h2>&lt;unsafe&gt;</h2>") {
		t.Fatal(html)
	}
	c.Template = "{{"
	if _, e = renderCharts(c); e == nil {
		t.Fatal("invalid template")
	}
}
func TestPersistenceAndFailedSave(t *testing.T) {
	a, s := startServer(t)
	_, v := post(t, s.Client(), s.URL, "", map[string]any{"action": "new", "id": "persist", "chart": chartConfig{Input: "[7]"}})
	key := v["ownerKey"].(string)
	a.mu.Lock()
	a.items = map[string]*item{}
	a.mu.Unlock()
	code, v := post(t, s.Client(), s.URL, key, map[string]any{"action": "get", "id": "persist"})
	if code != 200 || v["chart"].(map[string]any)["input"] != "[7]" {
		t.Fatal(v)
	}
	a.mu.Lock()
	bad := filepath.Join(t.TempDir(), "file")
	if e := os.WriteFile(bad, []byte("x"), 0600); e != nil {
		t.Fatal(e)
	}
	a.dir = bad
	a.mu.Unlock()
	code, _ = post(t, s.Client(), s.URL, key, map[string]any{"action": "share", "id": "persist"})
	if code != 500 {
		t.Fatal("save should fail")
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if len(a.items["persist"].Invites) != 0 {
		t.Fatal("failed save mutated invites")
	}
}

func TestStrictRequestAndTable(t *testing.T) {
	_, s := startServer(t)
	res, e := http.Post(s.URL+"/api/action", "application/json", strings.NewReader(`{"action":"list"} {}`))
	if e != nil {
		t.Fatal(e)
	}
	defer res.Body.Close()
	if res.StatusCode != 400 {
		t.Fatal("second JSON value accepted")
	}
	c := chartConfig{Input: "team,state,count\nalpha,ready,3\nbeta,waiting,2", Widget: "table"}
	got, e := renderCharts(c)
	if e != nil {
		t.Fatal(e)
	}
	for _, s := range []string{"alpha", "waiting", "count"} {
		if !strings.Contains(got, s) {
			t.Fatal(got)
		}
	}
	c.Widget = "line"
	if _, e = renderCharts(c); e == nil {
		t.Fatal("non-numeric line accepted")
	}
}

func TestFiniteSVGAndEmptyHistogram(t *testing.T) {
	for _, ps := range [][]point{{{X: 0, Y: math.MaxFloat64}}, {{X: 0, Y: -math.MaxFloat64}, {X: 1, Y: math.MaxFloat64}}} {
		got, e := renderSVG(series{Name: "large", Kind: "line", Points: ps}, "line")
		if e != nil {
			t.Fatal(e)
		}
		if strings.Contains(got, "NaN") || strings.Contains(got, "Inf") {
			t.Fatal("non-finite SVG geometry")
		}
	}
	got, e := renderSVG(series{Name: "empty", Kind: "exponentialhistogram"}, "auto")
	if e != nil || !strings.Contains(got, "No observations") {
		t.Fatalf("empty distribution: %v %s", e, got)
	}
}
func TestOTLPStreamSeparationAndLatest(t *testing.T) {
	raw := `{"resourceMetrics":[{"scopeMetrics":[{"metrics":[{"name":"requests","sum":{"isMonotonic":true,"dataPoints":[{"attributes":[{"key":"host","value":{"stringValue":"a"}}],"timeUnixNano":"1000000000","asInt":"3"},{"attributes":[{"key":"host","value":{"stringValue":"b"}}],"timeUnixNano":"1000000000","asInt":"8"}]}}]}]}]}`
	ss, e := parseData([]byte(raw), "otlp")
	if e != nil || len(ss) != 2 {
		t.Fatalf("mixed streams: %v %+v", e, ss)
	}
	export := func(at, count int) string {
		return fmt.Sprintf(`{"resourceMetrics":[{"scopeMetrics":[{"metrics":[{"name":"h","histogram":{"dataPoints":[{"timeUnixNano":"%d","count":%d,"explicitBounds":[10],"bucketCounts":[%d,0]}]}}]}]}]}`, at, count, count)
	}
	ss, e = parseData([]byte(export(2, 7)+"\n"+export(1, 3)), "ndjson")
	if e != nil || len(ss) != 1 || ss[0].Buckets[0].Count != 7 {
		t.Fatalf("latest distribution: %v %+v", e, ss)
	}
}

// Widgets carry no colour or font of their own: the kit tokens and the kit
// data palette only (kit conformance, the chart-series allowlist).
func TestSVGKitConformance(t *testing.T) {
	files, _ := filepath.Glob(filepath.Join("testdata", "*.svg"))
	if len(files) == 0 {
		t.Fatal("no golden SVGs")
	}
	for _, f := range files {
		raw, e := os.ReadFile(f)
		if e != nil {
			t.Fatal(e)
		}
		for _, v := range contracttest.FragmentViolations(string(raw), nil) {
			t.Errorf("%s: %s", f, v)
		}
	}
}
