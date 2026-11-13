// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

// Focused tests for film.bsh (Sprint 627 M7): storyboard lock, oss draft,
// --final on a commercial route, agy-style QA through the video.understand
// door.
//
// VENUE: fixture rods only. Every role film calls is served by
// testdata/fixture-rod.bsh, which emits small synthetic ffmpeg artifacts —
// no model, no download, no network, no spend. The film runs under a freshly
// built bashy, the real service door and the real `bashy providers`
// selector, in a scratch command ring (BASHY_COMMANDS_DIR/BASHY_HOME in temp
// dirs). No test passes --yes: the full final path uses commercial-route
// fixture rods that declare no spend; spend-declaring rods are used only to
// prove that nothing runs without --yes. Assembly needs the host's ffmpeg,
// ffprobe and agg (as examples/media/test.sh does); without them the tests
// skip with the reason.
package film

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

var (
	buildOnce sync.Once
	buildBin  string
	buildErr  error
	buildDir  string
)

func TestMain(m *testing.M) {
	var err error
	buildDir, err = os.MkdirTemp("", "film-*")
	if err != nil {
		panic(err)
	}
	code := m.Run()
	os.RemoveAll(buildDir)
	os.Exit(code)
}

// moduleRoot is the bashy repository root (the directory holding cmd/bashy).
func moduleRoot(t *testing.T) string {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	dir := filepath.Dir(file)
	for {
		if _, err := os.Stat(filepath.Join(dir, "cmd", "bashy")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("bashy repository root (cmd/bashy) not found above test file")
		}
		dir = parent
	}
}

func bashyBin(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fixture rods are POSIX paths")
	}
	buildOnce.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		bin := filepath.Join(buildDir, "bashy")
		cmd := exec.CommandContext(ctx, "go", "build", "-o", bin, "./cmd/bashy")
		cmd.Dir = moduleRoot(t)
		if workspace := filepath.Join(filepath.Dir(cmd.Dir), "go.work"); fileExists(workspace) {
			cmd.Env = append(os.Environ(), "GOWORK="+workspace)
		}
		if out, err := cmd.CombinedOutput(); err != nil {
			buildErr = &buildError{err, string(out)}
			return
		}
		buildBin = bin
	})
	if buildErr != nil {
		t.Fatalf("build ./cmd/bashy: %v", buildErr)
	}
	return buildBin
}

type buildError struct {
	err error
	out string
}

func (e *buildError) Error() string { return e.err.Error() + "\n" + e.out }

func fileExists(path string) bool { _, err := os.Stat(path); return err == nil }

type fixture struct {
	t                     *testing.T
	bin, film, root, proj string
	env                   []string
}

// newFixture: a scratch ring with ffmpeg/ffprobe/agg registered (what
// `media doctor` does for a member) and a project whose storyboard is small
// enough to render in seconds.
func newFixture(t *testing.T) *fixture {
	t.Helper()
	tools := map[string]string{}
	for _, name := range []string{"ffmpeg", "ffprobe", "agg", "jq"} {
		p, err := exec.LookPath(name)
		if err != nil {
			t.Skipf("film assembly needs %s on PATH (examples/media prerequisites)", name)
		}
		tools[name] = p
	}
	root := t.TempDir()
	bin := bashyBin(t)
	f := &fixture{
		t:    t,
		bin:  bin,
		film: filepath.Join(moduleRoot(t), "examples", "film", "film.bsh"),
		root: root,
		proj: filepath.Join(root, "proj"),
	}
	for _, d := range []string{"ring", "home", "fleet/tools", "tmp"} {
		if err := os.MkdirAll(filepath.Join(root, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	f.env = append(os.Environ(),
		"BASHY_COMMANDS_DIR="+filepath.Join(root, "ring"), "BASHY_HOME="+filepath.Join(root, "home"),
		"BASHY_FLEET_DIR="+filepath.Join(root, "fleet"), "BASHY_TOOLS_DIR="+filepath.Join(root, "fleet", "tools"), "BASHY_TOOLS_PATH=", "BASHY_FLEET_SEEDS=off",
		"BASHY_HINTS=off", "BASHY_TELEMETRY_QUIET=1", "BASHY_SVC_BASHY_BIN="+bin,
		"FILM_ROD_LOG="+f.rodLog(), "TMPDIR="+filepath.Join(root, "tmp"),
		"PATH="+filepath.Dir(bin)+string(os.PathListSeparator)+os.Getenv("PATH"))
	for _, name := range []string{"ffmpeg", "ffprobe", "agg"} {
		f.bashy("commands", "add", name, "--set", "exec.0="+tools[name], "--set", "effects.0=read", "--set", "effects.1=write", "--set", "effects.2=exec")
	}
	f.addRod("qa-fixture", "provides.0=video.understand", "route=vendor", "effects.0=read", "effects.1=exec")
	if _, stderr, code := f.run("init", f.proj, "--title", "Fixture Song"); code != 0 {
		t.Fatalf("film init: exit %d\n%s", code, stderr)
	}
	f.editStoryboard(func(sb map[string]any) {
		sb["size"] = "320x180"
		sb["song"].(map[string]any)["duration_s"] = 6
		sb["cards"].(map[string]any)["secs"] = 1
	})
	return f
}

func (f *fixture) rodLog() string { return filepath.Join(f.root, "rod.log") }

func (f *fixture) bashy(args ...string) {
	f.t.Helper()
	cmd := exec.Command(f.bin, args...)
	cmd.Env = f.env
	if out, err := cmd.CombinedOutput(); err != nil {
		f.t.Fatalf("bashy %v: %v\n%s", args, err, out)
	}
}

// addRod registers testdata/fixture-rod.bsh under name with the given fields.
func (f *fixture) addRod(name string, sets ...string) {
	f.t.Helper()
	rod := filepath.Join(moduleRoot(f.t), "examples", "film", "testdata", "fixture-rod.bsh")
	args := []string{"commands", "add", name, "--set", "exec.0=" + f.bin, "--set", "exec.1=--bashsharp", "--set", "exec.2=" + rod}
	for _, s := range sets {
		args = append(args, "--set", s)
	}
	f.bashy(args...)
}

// ossRods: the draft route for song, image, animate (optional) and transcribe.
func (f *fixture) ossRods(animate bool) {
	f.t.Helper()
	for _, role := range []string{"song", "image.generate", "animate", "transcribe"} {
		if role == "animate" && !animate {
			continue
		}
		f.addRod("oss-"+strings.Split(role, ".")[0], "provides.0="+role, "route=oss", "effects.0=read", "effects.1=write", "effects.2=exec")
	}
}

// commercialRods: route browser rods for the generative roles. spend adds
// the spend effect, as a real browser or api rod declares.
func (f *fixture) commercialRods(spend bool) {
	f.t.Helper()
	for _, role := range []string{"song", "image.generate", "animate"} {
		sets := []string{"provides.0=" + role, "route=browser", "effects.0=read", "effects.1=write", "effects.2=exec", "effects.3=net"}
		if spend {
			sets = append(sets, "effects.4=spend")
		}
		f.addRod("web-"+strings.Split(role, ".")[0], sets...)
	}
}

func (f *fixture) editStoryboard(edit func(map[string]any)) {
	f.t.Helper()
	path := filepath.Join(f.proj, "storyboard.json")
	b, err := os.ReadFile(path)
	if err != nil {
		f.t.Fatal(err)
	}
	var sb map[string]any
	if err := json.Unmarshal(b, &sb); err != nil {
		f.t.Fatal(err)
	}
	edit(sb)
	if b, err = json.MarshalIndent(sb, "", "  "); err != nil {
		f.t.Fatal(err)
	}
	if err := os.WriteFile(path, b, 0o644); err != nil {
		f.t.Fatal(err)
	}
}

func (f *fixture) run(args ...string) (string, string, int) {
	f.t.Helper()
	return f.runEnv(nil, args...)
}

func (f *fixture) runEnv(extra []string, args ...string) (string, string, int) {
	f.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, f.bin, append([]string{"--bashsharp", f.film}, args...)...)
	cmd.Env = append(append([]string{}, f.env...), extra...)
	cmd.Dir = f.root
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	code := 0
	if ee, ok := err.(*exec.ExitError); ok {
		code = ee.ExitCode()
	} else if err != nil {
		f.t.Fatalf("film %v: %v", args, err)
	}
	return strings.TrimSpace(string(out)), stderr.String(), code
}

type stage struct {
	Stage     string              `json:"stage"`
	Role      string              `json:"role"`
	Provider  string              `json:"provider"`
	Route     string              `json:"route"`
	Outcome   string              `json:"outcome"`
	CostClass string              `json:"cost_class"`
	Outputs   []map[string]string `json:"outputs"`
}

type manifest struct {
	Schema      string              `json:"schema_version"`
	Kind        string              `json:"kind"`
	ID          string              `json:"id"`
	Route       string              `json:"route"`
	Outcome     string              `json:"outcome"`
	Output      string              `json:"output"`
	Storyboard  string              `json:"storyboard_sha256"`
	Lock        string              `json:"lock_sha256"`
	Stages      []stage             `json:"stages"`
	CostClasses []string            `json:"cost_classes"`
	Degraded    []map[string]string `json:"degraded"`
	QA          *struct {
		Verdict  string   `json:"verdict"`
		Provider string   `json:"provider"`
		Issues   []string `json:"issues"`
	} `json:"qa"`
}

func (f *fixture) manifest(path string) manifest {
	f.t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		f.t.Fatalf("manifest: %v", err)
	}
	var m manifest
	if err := json.Unmarshal(b, &m); err != nil {
		f.t.Fatalf("manifest %s: %v\n%s", path, err, b)
	}
	if m.Schema != "bashy-film-render-v1" {
		f.t.Fatalf("manifest schema_version = %q", m.Schema)
	}
	return m
}

// rods returns the handoffs the fixture rod logged, one per run.
func (f *fixture) rods() []map[string]string {
	f.t.Helper()
	file, err := os.Open(f.rodLog())
	if os.IsNotExist(err) {
		return nil
	} else if err != nil {
		f.t.Fatal(err)
	}
	defer file.Close()
	var calls []map[string]string
	sc := bufio.NewScanner(file)
	for sc.Scan() {
		var c map[string]string
		if err := json.Unmarshal(sc.Bytes(), &c); err != nil {
			f.t.Fatalf("rod log line %q: %v", sc.Text(), err)
		}
		calls = append(calls, c)
	}
	return calls
}

func (f *fixture) resetRodLog() { os.Remove(f.rodLog()) }

// probe returns the container duration and the codec types of a media file.
func probe(t *testing.T, path string) (float64, []string) {
	t.Helper()
	out, err := exec.Command("ffprobe", "-v", "error", "-show_entries", "format=duration:stream=codec_type", "-of", "csv=p=0", path).Output()
	if err != nil {
		t.Fatalf("ffprobe %s: %v", path, err)
	}
	var d float64
	var types []string
	for l := range strings.FieldsSeq(string(out)) {
		if v, err := strconv.ParseFloat(l, 64); err == nil {
			d = v
		} else {
			types = append(types, l)
		}
	}
	return d, types
}

func (f *fixture) draft(extra ...string) manifest {
	f.t.Helper()
	out, stderr, code := f.run(append([]string{"draft", f.proj, "--qa-provider", "qa-fixture"}, extra...)...)
	if code != 0 {
		f.t.Fatalf("film draft: exit %d\nstdout: %s\nstderr: %s", code, out, stderr)
	}
	return f.manifest(out)
}

func TestDraftLockFinal(t *testing.T) {
	f := newFixture(t)
	f.ossRods(true)
	f.commercialRods(false)

	d := f.draft()
	if d.Kind != "draft" || d.ID != "01" || d.Route != "oss" || d.Outcome != "success" || d.QA == nil || d.QA.Verdict != "pass" {
		t.Fatalf("draft manifest: %+v", d)
	}
	var stages []string
	for _, s := range d.Stages {
		stages = append(stages, s.Stage+"="+s.Provider+"/"+s.Route)
	}
	want := "song=oss-song/oss portrait:lead=oss-image/oss transcribe=oss-transcribe/oss animate:1=oss-animate/oss animate:2=oss-animate/oss qa=qa-fixture/vendor"
	if got := strings.Join(stages, " "); got != want {
		t.Fatalf("draft stages:\n got %s\nwant %s", got, want)
	}
	if strings.Join(d.CostClasses, ",") != "oss,vendor" || len(d.Degraded) != 0 {
		t.Fatalf("draft cost classes %v, degraded %v", d.CostClasses, d.Degraded)
	}
	// 6 s song between a 1 s title card and a 1 s end card, subtitles muxed.
	dur, types := probe(t, d.Output)
	if dur < 7.5 || dur > 8.6 || strings.Join(types, ",") != "video,audio,subtitle" {
		t.Fatalf("film.mp4: duration %.3f, streams %v", dur, types)
	}
	// Shot 2 starts at lyric line 2's cue (the fixture transcript puts it at 3.2 s).
	shots, _ := os.ReadFile(filepath.Join(f.proj, "drafts", "01", "shots.tsv"))
	if !strings.Contains(string(shots), "1\t3.200\t6.000") {
		t.Fatalf("shots.tsv:\n%s", shots)
	}
	ass, _ := os.ReadFile(filepath.Join(f.proj, "drafts", "01", "subs.ass"))
	if !strings.Contains(string(ass), `sung line 2{\rSecondary}\NSegunda línea de la canción`) {
		t.Fatalf("subs.ass is not bilingual:\n%s", ass)
	}
	for _, c := range f.rods() {
		if c["yes"] != "0" {
			t.Fatalf("a draft rod ran with consent: %v", c)
		}
	}

	out, stderr, code := f.run("lock", f.proj)
	if code != 0 || out != filepath.Join(f.proj, "lock", "lock.json") {
		t.Fatalf("film lock: exit %d, %q\n%s", code, out, stderr)
	}
	var lock struct {
		Schema string            `json:"schema_version"`
		Draft  string            `json:"draft"`
		Files  map[string]string `json:"files"`
	}
	b, _ := os.ReadFile(out)
	if err := json.Unmarshal(b, &lock); err != nil || lock.Draft != "01" {
		t.Fatalf("lock.json: %v\n%s", err, b)
	}
	for _, name := range []string{"storyboard.json", "timing.srt", "shots.tsv", "duration", "portraits/lead.png"} {
		if len(lock.Files[name]) != 64 {
			t.Fatalf("lock.json has no sha256 for %s: %v", name, lock.Files)
		}
	}

	f.resetRodLog()
	out, stderr, code = f.run("final", f.proj, "--route", "browser", "--qa-provider", "qa-fixture")
	if code != 0 {
		t.Fatalf("film final: exit %d\n%s\n%s", code, out, stderr)
	}
	fm := f.manifest(out)
	if fm.Kind != "final" || fm.Route != "browser" || fm.Outcome != "success" || len(fm.Lock) != 64 || fm.Storyboard != lock.Files["storyboard.json"] {
		t.Fatalf("final manifest: %+v", fm)
	}
	// Only generative stages re-render: no transcribe, timing reused from the lock.
	var roles []string
	for _, c := range f.rods() {
		roles = append(roles, c["role"]+"/"+c["route"])
		if c["role"] == "image.generate" && c["reference"] != filepath.Join(f.proj, "lock", "portraits", "lead.png") {
			t.Fatalf("final portrait did not reference the locked portrait: %v", c)
		}
	}
	if got := strings.Join(roles, " "); got != "song/browser image.generate/browser animate/browser animate/browser video.understand/vendor" {
		t.Fatalf("final rod calls: %s", got)
	}
	locked, _ := os.ReadFile(filepath.Join(f.proj, "lock", "timing.srt"))
	used, _ := os.ReadFile(filepath.Join(f.proj, "finals", "01", "timing.srt"))
	if string(locked) != string(used) {
		t.Fatal("final did not reuse the locked timing")
	}
	if dur, _ := probe(t, fm.Output); dur < 7.5 || dur > 8.6 {
		t.Fatalf("final film.mp4 duration %.3f", dur)
	}

	status, _, code := f.run("status", f.proj)
	if code != 0 || !strings.Contains(status, "draft\t01\toss\tsuccess") || !strings.Contains(status, "final\t01\tbrowser\tsuccess") || !strings.Contains(status, "lock\tdraft 01") {
		t.Fatalf("film status:\n%s", status)
	}
}

func TestFinalSpendNeedsYes(t *testing.T) {
	f := newFixture(t)
	f.ossRods(true)
	f.draft("--no-qa")
	if _, stderr, code := f.run("lock", f.proj); code != 0 {
		t.Fatalf("lock: %s", stderr)
	}
	f.commercialRods(true)
	f.resetRodLog()

	// --dry-run names the rods each generative stage would use; nothing runs.
	out, stderr, code := f.run("final", f.proj, "--route", "browser", "--dry-run")
	if code != 0 {
		t.Fatalf("final --dry-run: exit %d\n%s", code, stderr)
	}
	m := f.manifest(out)
	var got []string
	for _, s := range m.Stages {
		got = append(got, s.Stage+"="+s.Provider+"/"+s.Outcome)
	}
	if m.ID != "dry-run" || m.Outcome != "dry-run" || strings.Join(got, " ") != "song=web-song/dry-run portrait:lead=web-image/dry-run animate=web-animate/dry-run" {
		t.Fatalf("dry run: id %s outcome %s stages %v", m.ID, m.Outcome, got)
	}

	// Without --yes the door refuses the spend rods: exit 126 and no rod runs.
	out, stderr, code = f.run("final", f.proj, "--route", "browser")
	if code != 126 || !strings.Contains(stderr, "rerun with --yes") {
		t.Fatalf("final without --yes: exit %d\n%s", code, stderr)
	}
	if m := f.manifest(out); m.Outcome != "unavailable" {
		t.Fatalf("final without --yes: outcome %s", m.Outcome)
	}
	if calls := f.rods(); len(calls) != 0 {
		t.Fatalf("rods ran without --yes: %v", calls)
	}
}

func TestFinalRefusals(t *testing.T) {
	f := newFixture(t)
	f.ossRods(true)
	f.commercialRods(false)

	if _, stderr, code := f.run("final", f.proj, "--route", "browser"); code != 1 || !strings.Contains(stderr, "no lock") {
		t.Fatalf("final without a lock: exit %d\n%s", code, stderr)
	}
	if _, stderr, code := f.run("lock", f.proj); code != 1 || !strings.Contains(stderr, "no successful draft") {
		t.Fatalf("lock without a draft: exit %d\n%s", code, stderr)
	}
	f.draft("--no-qa")
	if _, stderr, code := f.run("lock", f.proj); code != 0 {
		t.Fatalf("lock: %s", stderr)
	}
	if _, stderr, code := f.run("final", f.proj); code != 1 || !strings.Contains(stderr, "--route browser|api is required") {
		t.Fatalf("final without --route: exit %d\n%s", code, stderr)
	}
	if _, stderr, code := f.run("final", f.proj, "--route", "oss"); code != 1 || !strings.Contains(stderr, "route oss is for drafts") {
		t.Fatalf("final --route oss: exit %d\n%s", code, stderr)
	}
	// A changed locked file is refused before any stage runs.
	f.resetRodLog()
	timing := filepath.Join(f.proj, "lock", "timing.srt")
	b, _ := os.ReadFile(timing)
	if err := os.WriteFile(timing, append(b, "\n3\n00:00:09,000 --> 00:00:10,000\nextra\n"...), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, stderr, code := f.run("final", f.proj, "--route", "browser"); code != 1 || !strings.Contains(stderr, "locked file changed") {
		t.Fatalf("final after tampering: exit %d\n%s", code, stderr)
	}
	if calls := f.rods(); len(calls) != 0 {
		t.Fatalf("rods ran with a broken lock: %v", calls)
	}
	// Editing the storyboard after a draft blocks a lock until the next draft.
	f.editStoryboard(func(sb map[string]any) { sb["title"] = "Changed" })
	if _, stderr, code := f.run("lock", f.proj); code != 1 || !strings.Contains(stderr, "changed since draft") {
		t.Fatalf("lock after a storyboard edit: exit %d\n%s", code, stderr)
	}
}

func TestDraftStillPanWithoutAnimateRod(t *testing.T) {
	f := newFixture(t)
	f.ossRods(false)
	d := f.draft()
	if d.Outcome != "success" || len(d.Degraded) != 2 {
		t.Fatalf("draft without animate: outcome %s degraded %v", d.Outcome, d.Degraded)
	}
	for _, g := range d.Degraded {
		if g["note"] != "still image with a slow pan" {
			t.Fatalf("degraded note: %v", g)
		}
	}
	if _, types := probe(t, d.Output); strings.Join(types, ",") != "video,audio,subtitle" {
		t.Fatalf("film.mp4 streams %v", types)
	}

	// A final never degrades: the same gap fails with the door's hint.
	if _, stderr, code := f.run("lock", f.proj); code != 0 {
		t.Fatalf("lock: %s", stderr)
	}
	f.addRod("web-song", "provides.0=song", "route=browser", "effects.0=read", "effects.1=write", "effects.2=exec")
	f.addRod("web-image", "provides.0=image.generate", "route=browser", "effects.0=read", "effects.1=write", "effects.2=exec")
	if _, stderr, code := f.run("final", f.proj, "--route", "browser", "--no-qa"); code != 126 || !strings.Contains(stderr, "animate:1: no usable provider") {
		t.Fatalf("final without an animate rod: exit %d\n%s", code, stderr)
	}
}

func TestQAVerdicts(t *testing.T) {
	f := newFixture(t)
	f.ossRods(true)
	cases := []struct{ reply, verdict string }{
		{`Lip sync drifts. {"pass": false, "issues": ["shot 2 lip sync late"]}`, "fail"},
		{"looks fine to me", "unparsed"},
	}
	for _, c := range cases {
		out, stderr, code := f.runEnv([]string{"FILM_FIXTURE_QA=" + c.reply}, "draft", f.proj, "--qa-provider", "qa-fixture")
		if code != 0 {
			t.Fatalf("draft: exit %d\n%s", code, stderr)
		}
		m := f.manifest(out)
		if m.QA == nil || m.QA.Verdict != c.verdict {
			t.Fatalf("reply %q: qa %+v", c.reply, m.QA)
		}
		if c.verdict == "fail" && (len(m.QA.Issues) != 1 || !strings.Contains(stderr, "qa issue: shot 2 lip sync late")) {
			t.Fatalf("fail verdict issues %v\n%s", m.QA.Issues, stderr)
		}
	}
}

func TestStoryboardValidation(t *testing.T) {
	f := newFixture(t)
	f.editStoryboard(func(sb map[string]any) {
		sb["shots"] = []any{map[string]any{"character": "nobody", "prompt": "x"}}
	})
	if _, stderr, code := f.run("draft", f.proj); code != 1 || !strings.Contains(stderr, "every shot.character must name a character id") {
		t.Fatalf("bad shot: exit %d\n%s", code, stderr)
	}
	f.editStoryboard(func(sb map[string]any) {
		sb["shots"] = []any{map[string]any{"character": "lead", "prompt": "x"}}
		sb["translation"].(map[string]any)["lines"] = []any{"solo una"}
	})
	if _, stderr, code := f.run("draft", f.proj); code != 1 || !strings.Contains(stderr, "translation.lines must match lyrics") {
		t.Fatalf("short translation: exit %d\n%s", code, stderr)
	}
	if calls := f.rods(); len(calls) != 0 {
		t.Fatalf("rods ran for an invalid storyboard: %v", calls)
	}
}
