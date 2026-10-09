// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package corpuslocales

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	corelocale "github.com/qiangli/coreutils/pkg/locale"
)

// Synthetic fixtures only: no test here touches the network beyond a
// loopback httptest server, and none needs the glibc tarball.

func TestStageDropCopyOnlySection(t *testing.T) {
	in := []string{
		"LC_PAPER",
		`copy "i18n"`,
		"END LC_PAPER",
		"LC_MEASUREMENT",
		"measurement 2",
		"END LC_MEASUREMENT",
		"LC_NUMERIC",
		`decimal_point ","`,
		"END LC_NUMERIC",
	}
	got := dropCopyOnlyStagedSection(in, "LC_PAPER")
	for _, l := range got {
		if l == "LC_PAPER" || l == `copy "i18n"` {
			t.Fatalf("copy-only section survived: %q", got)
		}
	}
	got = dropCopyOnlyStagedSection(in, "LC_MEASUREMENT")
	found := false
	for _, l := range got {
		if l == "measurement 2" {
			found = true
		}
	}
	if !found {
		t.Fatalf("real section was dropped: %q", got)
	}
	got = dropCopyOnlyStagedSection(in, "LC_MISSING")
	if len(got) != len(in) {
		t.Fatalf("absent section changed the input: %q", got)
	}
}

func TestStageDropCollate(t *testing.T) {
	in := []string{
		"LC_CTYPE",
		`copy "i18n"`,
		"END LC_CTYPE",
		"LC_COLLATE",
		`copy "iso14651_t1"`,
		"END LC_COLLATE",
		"LC_NUMERIC",
		"END LC_NUMERIC",
	}
	got := dropStagedSection(in, "LC_COLLATE")
	for _, l := range got {
		if l == "LC_COLLATE" || strings.Contains(l, "iso14651") {
			t.Fatalf("collation survived: %q", got)
		}
	}
	if len(got) != 5 {
		t.Fatalf("dropped wrong line count: %q", got)
	}
}

func TestStageDropBlock(t *testing.T) {
	in := []string{
		"upper /",
		"   <U0041>;/",
		"   <U0042>",
		`map "totitle"; /`,
		"   (<U0061>,<U0041>);/",
		"   (<U0062>,<U0042>)",
		`class "combining"; /`,
		"   <U0300>..<U036F>;/",
		"   <U0483>..<U0489>",
		"lower /",
		"   <U0061>",
	}
	got := dropStagedBlock(dropStagedBlock(in, `map "totitle"`), `class "combining"`)
	want := []string{
		"upper /",
		"   <U0041>;/",
		"   <U0042>",
		"lower /",
		"   <U0061>",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("got %q want %q", got, want)
	}
	// A prefix that matches nothing leaves the input alone.
	if got := dropStagedBlock(in, "tojhira"); len(got) != len(in) {
		t.Fatalf("non-matching prefix changed the input")
	}
}

func TestStageExpandURanges(t *testing.T) {
	in := []string{`upper <U0041>..<U0043>;<U00E0>..<U00E2>;<U004A>`}
	got := expandStagedURanges(in)
	want := `upper <U0041>;<U0042>;<U0043>;<U00E0>;<U00E1>;<U00E2>;<U004A>`
	if len(got) != 1 || got[0] != want {
		t.Fatalf("got %q want %q", got, want)
	}
	// Reversed ranges are kept verbatim (compiler reports them).
	in = []string{`upper <U0043>..<U0041>`}
	if got := expandStagedURanges(in); got[0] != in[0] {
		t.Fatalf("reversed range rewritten: %q", got)
	}
}

func TestStageJoinCommentContinuations(t *testing.T) {
	in := []string{
		`d_t_fmt "A%H時/`,
		`%M分%S秒"`,
		`mon "x";/`,
		`   "y"`,
	}
	got := joinStagedCommentContinuations(in)
	want := []string{
		`d_t_fmt "A%H時%M分%S秒"`,
		`mon "x";/`,
		`   "y"`,
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestStageLocaleSourceRules(t *testing.T) {
	src := strings.Join([]string{
		"comment_char %",
		"escape_char /",
		"LC_CTYPE",
		`copy "i18n"`,
		"END LC_CTYPE",
		"LC_COLLATE",
		`copy "iso14651_t1"`,
		"END LC_COLLATE",
		"LC_PAPER",
		`copy "i18n"`,
		"END LC_PAPER",
		"LC_NUMERIC",
		`decimal_point ","`,
		"END LC_NUMERIC",
	}, "\n")
	got := string(stageLocaleSource("fr_FR", []byte(src)))
	if strings.Contains(got, "LC_COLLATE") || strings.Contains(got, "LC_PAPER") {
		t.Fatalf("dropped sections survive:\n%s", got)
	}
	if !strings.Contains(got, `decimal_point ","`) || !strings.Contains(got, `copy "i18n"`) {
		t.Fatalf("kept sections damaged:\n%s", got)
	}
	// en_US keeps its real extension sections.
	srcUS := strings.Join([]string{
		"LC_PAPER",
		"height   279",
		"END LC_PAPER",
	}, "\n")
	if got := string(stageLocaleSource("en_US", []byte(srcUS))); !strings.Contains(got, "height   279") {
		t.Fatalf("real en_US section dropped:\n%s", got)
	}
}

// TestStageCharmapRangesTrueBytes is the regression test for the glibc
// UTF-8 generator's carry-unsafe ranges: a range crossing a
// continuation-byte boundary must expand to each code point's true encoding,
// not to byte-carry artifacts. Under byte-carry, <U00032B25> would resolve
// to f0 b2 ab e5 (invalid); the true encoding is f0 b2 ab a5.
func TestStageCharmapRangesTrueBytes(t *testing.T) {
	in := "<code_set_name> UTF-8\nCHARMAP\n<U00032AF0>..<U00032B2F> /xf0/xb2/xab/xb0 <CJK>\n<U0041> /x41 A\nEND CHARMAP\n"
	got := string(expandStagedCharmapRanges([]byte(in)))
	lines := strings.Split(got, "\n")
	byName := map[string]string{}
	for _, l := range lines {
		var name, esc string
		if _, err := fmt.Sscanf(l, "<%s", &name); err != nil {
			continue
		}
		name = strings.TrimSuffix(name, ">")
		fields := strings.Fields(l)
		if len(fields) < 2 {
			continue
		}
		esc = fields[1]
		byName[name] = esc
	}
	for _, want := range []string{"U00032AF0", "U00032B25", "U00032B2F", "U0041"} {
		esc, ok := byName[want]
		if !ok {
			t.Fatalf("expanded charmap lacks <%s>:\n%s", want, got)
		}
		var cp uint64
		if _, err := fmt.Sscanf(want, "U%X", &cp); err != nil {
			t.Fatal(err)
		}
		var enc strings.Builder
		for _, b := range []byte(string(rune(cp))) {
			fmt.Fprintf(&enc, "/x%02x", b)
		}
		if esc != enc.String() {
			t.Fatalf("<%s> = %s, want true UTF-8 %s", want, esc, enc.String())
		}
	}
	// Non-UTF-8 charmaps pass through untouched.
	raw := "<code_set_name> BIG5\nCHARMAP\n<U0000> /x00\nEND CHARMAP\n"
	if got := string(stageCharmap("BIG5", []byte(raw))); got != raw {
		t.Fatalf("non-UTF-8 charmap rewritten:\n%s", got)
	}
}

func TestCorpusBuildTable(t *testing.T) {
	if len(corpusLocaleBuild) != 10 {
		t.Fatalf("build has %d specs, want 10 (3 helpers + 7 corpus)", len(corpusLocaleBuild))
	}
	seen := map[string]bool{}
	helperIdx := map[string]int{}
	for i, spec := range corpusLocaleBuild {
		if seen[spec.StoreName] {
			t.Fatalf("duplicate store name %q", spec.StoreName)
		}
		seen[spec.StoreName] = true
		helperIdx[spec.StoreName] = i
		if !corelocale.ValidStoreName(spec.StoreName) {
			t.Fatalf("invalid store name %q", spec.StoreName)
		}
	}
	// Helpers must precede every corpus locale that copies them: i18n_ctype
	// and i18n before everything, zh_CN before the zh_TW/zh_HK copies.
	if helperIdx["i18n_ctype"] != 0 || helperIdx["i18n"] != 1 || helperIdx["zh_CN"] != 2 {
		t.Fatalf("helpers not first in build order: %v", helperIdx)
	}
	for _, name := range []string{"zh_TW.big5", "zh_HK.big5hkscs"} {
		if helperIdx["zh_CN"] > helperIdx[name] {
			t.Fatalf("zh_CN compiled after %q", name)
		}
	}
	// Every advertised corpus name is built, with the charmap the corpus gates on.
	charmaps := map[string]string{}
	for _, spec := range corpusLocaleBuild {
		charmaps[spec.StoreName] = spec.Charmap
	}
	want := map[string]string{
		"en_US.UTF-8":   "UTF-8",
		"zh_TW.big5":    "BIG5",
		"ja_JP.SJIS":    "SHIFT_JIS",
		"fr_FR.ISO8859-1": "ISO-8859-1",
		"de_DE.UTF-8":   "UTF-8",
		"zh_HK.big5hkscs": "BIG5-HKSCS",
		"ru_RU.CP1251":  "CP1251",
	}
	for _, name := range corpusAdvertisedNames {
		got, ok := charmaps[name]
		if !ok {
			t.Fatalf("advertised name %q not built", name)
		}
		if got != want[name] {
			t.Fatalf("%s charmap = %s, want %s", name, got, want[name])
		}
	}
}

func TestValidCorpusStore(t *testing.T) {
	dir := t.TempDir()
	store := filepath.Join(dir, "store")
	if validCorpusStore(dir, store) {
		t.Fatal("empty dir validates")
	}
	if err := os.MkdirAll(store, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(store, ".bashy-locales-provisioned"), []byte("stale\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if validCorpusStore(dir, store) {
		t.Fatal("stale marker validates")
	}
	if err := os.WriteFile(filepath.Join(store, ".bashy-locales-provisioned"), []byte(corpusStoreMarker()), 0o644); err != nil {
		t.Fatal(err)
	}
	if validCorpusStore(dir, store) {
		t.Fatal("marker without store files validates")
	}
	for _, spec := range corpusLocaleBuild {
		path, err := corelocale.StorePath(store, spec.StoreName)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("{}"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if !validCorpusStore(dir, store) {
		t.Fatal("complete store does not validate")
	}
}

// TestCompileSyntheticLocale exercises the in-process localedef path
// (stage, copy resolution against the store being built, save) with
// fixture data only.
func TestCompileSyntheticLocale(t *testing.T) {
	store := t.TempDir()
	base := "LC_CTYPE\nupper <A>\ntolower (<A>,<a>)\nEND LC_CTYPE\nLC_NUMERIC\ndecimal_point \".\"\nEND LC_NUMERIC\n"
	if err := compileOneCorpusLocale(store,
		corpusLocaleSpec{StoreName: "base", Source: "base", Charmap: "UTF-8"},
		[]byte("<code_set_name> UTF-8\n<comment_char> %\n<escape_char> /\n<mb_cur_min> 1\n<mb_cur_max> 4\nCHARMAP\n<A> /x41\n<a> /x61\nEND CHARMAP\n"),
		[]byte(base)); err != nil {
		t.Fatalf("base: %v", err)
	}
	child := "LC_CTYPE\ncopy \"base\"\nEND LC_CTYPE\nLC_NUMERIC\ndecimal_point \",\"\nEND LC_NUMERIC\n"
	if err := compileOneCorpusLocale(store,
		corpusLocaleSpec{StoreName: "xx_YY", Source: "child", Charmap: "UTF-8"},
		[]byte("<code_set_name> UTF-8\n<comment_char> %\n<escape_char> /\n<mb_cur_min> 1\n<mb_cur_max> 4\nCHARMAP\nEND CHARMAP\n"),
		[]byte(child)); err != nil {
		t.Fatalf("child: %v", err)
	}
	names := corelocale.CompiledNames([]string{"LOCPATH=" + store})
	found := false
	for _, n := range names {
		if n == "xx_YY" {
			found = true
		}
	}
	if !found {
		t.Fatalf("xx_YY not advertised: %v", names)
	}
	c, ok := corelocale.LookupCompiled([]string{"LOCPATH=" + store, "LC_ALL=xx_YY"}, "xx_YY")
	if !ok || c.Charmap != "UTF-8" {
		t.Fatalf("lookup = %v %v, want UTF-8", c, ok)
	}
	if s, ok := c.KeywordString("LC_NUMERIC", "decimal_point"); !ok || s != "," {
		t.Fatalf("decimal_point = %q %v", s, ok)
	}
	// An undefined symbol outside LC_CTYPE fails closed (no store output).
	bad := "LC_NUMERIC\ndecimal_point \"<absent>\"\nEND LC_NUMERIC\n"
	err := compileOneCorpusLocale(store,
		corpusLocaleSpec{StoreName: "bad", Source: "bad", Charmap: "UTF-8"},
		[]byte("<code_set_name> UTF-8\n<comment_char> %\n<escape_char> /\nCHARMAP\n<A> /x41\nEND CHARMAP\n"),
		[]byte(bad))
	if err == nil {
		t.Fatal("undefined symbol compiled without error")
	}
	if _, ok := corelocale.LookupCompiled([]string{"LOCPATH=" + store}, "bad"); ok {
		t.Fatal("failed compile left store output")
	}
}

// corpusTestTarball builds a minimal tarball with every wanted member so the
// fetch path (stream, subset extract, digest verify) runs against loopback.
func corpusTestTarball(t *testing.T) ([]byte, string) {
	t.Helper()
	var buf bytes.Buffer
	h := sha256.New()
	gz := gzip.NewWriter(io.MultiWriter(&buf, h))
	tw := tar.NewWriter(gz)
	for _, f := range corpusLocaleSources {
		body := "LC_NUMERIC\ndecimal_point \".\"\nEND LC_NUMERIC\n"
		name := "glibc-2.44/" + f
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(body)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	for _, f := range corpusLocaleCharmaps {
		body := "<code_set_name> TEST\nCHARMAP\n<period> /x2e\nEND CHARMAP\n"
		name := "glibc-2.44/" + f
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(body)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes(), hex.EncodeToString(h.Sum(nil))
}

func TestFetchGlibcLocaledataLoopback(t *testing.T) {
	body, sha := corpusTestTarball(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(body)
	}))
	defer srv.Close()
	dst := t.TempDir()
	if err := fetchGlibcLocaledata(srv.URL, filepath.Join(dst, "src"), sha); err != nil {
		t.Fatalf("fetch: %v", err)
	}
	for _, f := range append(append([]string{}, corpusLocaleSources...), corpusLocaleCharmaps...) {
		if _, err := os.Stat(filepath.Join(dst, "src", f)); err != nil {
			t.Fatalf("missing extracted member %s: %v", f, err)
		}
	}
	// Wrong digest fails closed.
	if err := fetchGlibcLocaledata(srv.URL, filepath.Join(dst, "bad"), strings.Repeat("0", 64)); err == nil ||
		!strings.Contains(err.Error(), "sha256 mismatch") {
		t.Fatalf("wrong digest: err = %v", err)
	}
}
