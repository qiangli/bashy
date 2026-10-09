// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package corpuslocales

// Corpus locale provisioning for the Windows Bash 5.3 gate (Sprint 379).
//
// The two locale-sensitive fixtures (glob-test, intl) gate on `locale -a`.
// On a provider-less Windows host coreutils' locale applet can only advertise
// its carried names (C, POSIX, de_DE.*), so the fixtures report their
// truthful missing-locale diagnostics and the gate measures 84/86. The
// certified coreutils locale applet is NOT patched for this (conductor
// decision); instead bashy provisions real locale data and compiles it with
// coreutils' own localedef(1) into the Go locale store (LOCPATH), which
// withCompiledLocales advertises and compiledData serves. Zero applet changes.
//
// Data provenance (runtime download, gitignored cache, never vendored — the
// same embedded-URL pattern as EnsureBash53Fixtures):
//
//   - glibc 2.44 tarball, pinned URL + SHA-256 below (hash corroborated from
//     two independent mirrors before pinning). Only localedata/locales/* and
//     localedata/charmaps/* are extracted.
//   - Licence: every locale source used carries the FSF dedication "The Free
//     Software Foundation does not claim any copyright interest in the
//     locale data contained in this file." The BIG5 charmap states
//     "Distribution and use is free, even for comercial purpose." The other
//     charmaps carry no licence header (Unicode mapping facts). The glibc
//     library as a whole is LGPL, but no library code is fetched, vendored,
//     linked or shipped here — only the dedicated locale data, compiled at
//     run time on the operator's own host.
//
// Staging (byte-preserving; applied in memory at provision time, never
// committed): the certified localedef accepts POSIX locale sources, not
// every glibc extension. Each rule below names what is rewritten and why;
// everything else compiles byte-identical to glibc 2.44:
//
//  1. LC_COLLATE sections are dropped from every staged source. The corpus
//     collation is the 84k-row multi-script ISO 14651 table (one order_start
//     per script plus HAN/pinyin tailorings); the certified compiler accepts
//     a single order_start/order_end pair, so the table cannot be carried.
//     A locale without LC_COLLATE keeps the store's designed semantics: the
//     name is advertised and selectable, LC_COLLATE is refused by name, and
//     every other category is served from real data. No manufactured
//     weights; `sort` keeps its existing no-data fallback.
//  2. LC_PAPER/LC_MEASUREMENT sections that are pure `copy` directives are
//     dropped (real ones, e.g. en_US, are kept and skip silently). The store
//     format carries no extension categories, so a copy of one can never
//     resolve; keeping the directive would fail the whole locale closed.
//  3. Custom LC_CTYPE `class` blocks (combining, combining_level3, hanzi,
//     jspace/jdigit/jhira/jkata/jkanji) and `map` blocks (totitle) are
//     dropped, as are their `charclass`/`charconv` declarations (ja_JP).
//     The store has no titlecase or implementation-defined class consumers;
//     classes are invisible via locale(1). The POSIX classes, toupper and
//     tolower compile in full.
//  4. `<UHHHH>..<UHHHH>` ranges in LC_CTYPE classes are expanded to
//     `;`-separated members (the compiler's ellipsis form covers single-byte
//     ASCII only). Expansion is exact: same code points, same order.
//  5. A `/`-continuation whose next physical line starts with the comment
//     character is pre-joined (zh_TW d_t_fmt/date_fmt: the continuation line
//     starts with the strftime %M). glibc joins physical lines before
//     comment stripping; the certified reader drops such lines.
//  6. The UTF-8 charmap's `<U..>..<U..>` ranges are expanded with the TRUE
//     UTF-8 encoding of each code point. glibc's generator emits ranges that
//     cross continuation-byte boundaries, which a byte-carry expansion
//     corrupts (~13k symbols); re-encoding from the code point is what the
//     range denotes. Other charmaps are used as shipped.
//
// Helpers (i18n_ctype, i18n, zh_CN) compile first so `copy` directives in the
// corpus files resolve against the store; the three gated corpus files
// themselves are never textually altered except by rules 2, 4 and 5 above. LC_CTYPE
// classes are therefore shared from one UTF-8-compiled i18n: correct for
// locale(1) purposes (charmap + mb_cur report per locale) and invisible
// everywhere else — coreutils has no compiled-class consumer. This
// charmap-sharing limitation, the dropped collation, and rules 3/5 are the
// known deltas versus a glibc-compiled set; all are unobservable to the
// fixtures, which query `locale -a` plus engine-side conversions only.

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	corelocale "github.com/qiangli/coreutils/pkg/locale"
	"github.com/qiangli/coreutils/pkg/localedef"
)

// corpusLocalesGlibcURL is the embedded runtime-download URL for the glibc
// sources; only the localedata subset named below is used, fetched into a
// gitignored cache. bashy never vendors it.
const corpusLocalesGlibcURL = "https://ftp.gnu.org/gnu/glibc/glibc-2.44.tar.gz"

// corpusLocalesGlibcSHA256 pins the tarball above (corroborated from
// ftp.gnu.org and mirrors.edge.kernel.org, 2026-10-09).
const corpusLocalesGlibcSHA256 = "1217fc41ac7fb1f310c8c32b9c6c009cc769398d4b367b6aa57be0bb3ea8c1ef"

// corpusLocalesVersion selects the cache directory. corpusLocalesProvisioner
// versions the staging rules: bump it whenever the rules above change so a
// stale store is rebuilt rather than trusted.
const corpusLocalesVersion = "glibc-2.44"

const corpusLocalesProvisioner = "v2"

// corpusLocaleSources are the tarball members fetched, relative to the
// tarball root: the advertisement-gated corpus locales plus the copy-chain
// helpers. en_US, de_DE, zh_HK and ru_RU are deliberately NOT compiled (see
// the compiled-set note below).
var corpusLocaleSources = []string{
	"localedata/locales/fr_FR",
	"localedata/locales/zh_TW",
	"localedata/locales/ja_JP",
	"localedata/locales/zh_CN",
	"localedata/locales/i18n",
	"localedata/locales/i18n_ctype",
}

// corpusLocaleCharmaps are the tarball charmap members fetched.
var corpusLocaleCharmaps = []string{
	"localedata/charmaps/UTF-8",
	"localedata/charmaps/ISO-8859-1",
	"localedata/charmaps/BIG5",
	"localedata/charmaps/SHIFT_JIS",
}

// corpusLocaleSpec is one localedef invocation: compile Source with Charmap
// under StoreName. Build order is dependency order: helpers first so every
// `copy` resolves against the store being built.
type corpusLocaleSpec struct {
	StoreName string
	Source    string
	Charmap   string
}

// corpusLocaleBuild is the full provisioned set in compile order.
//
// Compiled-set note: a compiled locale is STRICT about the categories it
// does not carry — collate.OpenEnv errors on a compiled locale without
// LC_COLLATE ("has no LC_COLLATE"), while an unknown locale name falls
// back silently. The certified compiler cannot carry LC_COLLATE (rule 1),
// so compiling a locale the fixtures place in LC_COLLATE/LC_ALL position
// in front of a collation-consuming applet (sed, grep, sort, ...) turns a
// working fallback into a hard error — measured: compiling en_US.UTF-8
// breaks intl, whose intl.tests exports LC_ALL=en_US.UTF-8 globally.
// The compiled set is therefore exactly the advertisement-gated names
// whose fixture exposure is LC_CTYPE-only (plus the copy-chain helpers,
// which no fixture names):
//
//   - zh_TW.big5: gated by glob2.sub:17 and unicode1.sub:121; glob-test
//     exposure past the gate is LC_ALL=zh_TW.big5 with od (LC_ALL=C
//     prefixed), recho (byte-level harness helper) and the engine only.
//   - fr_FR.ISO8859-1: gated by unicode1.sub:101; LC_CTYPE-prefix only.
//   - ja_JP.SJIS: gated by unicode1.sub:321; LC_CTYPE-prefix only.
//
// Deliberately NOT compiled: en_US.UTF-8 (no fixture gates on it —
// unicode1.sub:602 TestCodePage runs unconditionally engine-side — and
// dozens of fixtures export LC_ALL/LANG=en_US.UTF-8 in front of sed and
// friends), de_DE.UTF-8 (already carried by the applet; same strictness
// risk under LANG=de_DE), zh_HK.big5hkscs (its only corpus mention is a
// commented-out LANG in read1.sub), ru_RU.CP1251 (unicode2.sub:34 uses it
// LC_CTYPE-prefixed engine-side, which passes unadvertised). When the
// certified toolchain learns LC_COLLATE, this set extends to all seven.
var corpusLocaleBuild = []corpusLocaleSpec{
	{StoreName: "i18n_ctype", Source: "i18n_ctype", Charmap: "UTF-8"},
	{StoreName: "i18n", Source: "i18n", Charmap: "UTF-8"},
	{StoreName: "zh_CN", Source: "zh_CN", Charmap: "UTF-8"},
	{StoreName: "zh_TW.big5", Source: "zh_TW", Charmap: "BIG5"},
	{StoreName: "fr_FR.ISO8859-1", Source: "fr_FR", Charmap: "ISO-8859-1"},
	{StoreName: "ja_JP.SJIS", Source: "ja_JP", Charmap: "SHIFT_JIS"},
}

// corpusAdvertisedNames are the store names the Bash 5.3 corpus gates on.
// Helpers (i18n_ctype, i18n, zh_CN) are also advertised as a side effect;
// the fixtures grep anchored names, so extras are harmless.
var corpusAdvertisedNames = []string{
	"zh_TW.big5",
	"fr_FR.ISO8859-1",
	"ja_JP.SJIS",
}

// EnsureCorpusLocales makes the compiled corpus locale store present,
// fetching the pinned glibc localedata at runtime into a gitignored
// user-cache and compiling it with coreutils' own localedef. Idempotent; a
// no-op when the provisioned store is current. It returns the store
// directory: set LOCPATH to it so locale(1) advertises the corpus set.
func EnsureCorpusLocales() (string, error) {
	cacheBase, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	dst := filepath.Join(cacheBase, "bashy", "locales", corpusLocalesVersion)
	store := filepath.Join(dst, "store")
	if validCorpusStore(dst, store) {
		return store, nil
	}
	fmt.Fprintf(os.Stderr, "corpus locales: provisioning glibc localedata (gitignored cache, not vendored)\n  %s\n", corpusLocalesGlibcURL)
	parent := filepath.Dir(dst)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return "", err
	}
	tmp, err := os.MkdirTemp(parent, "locales.fetch-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tmp)
	srcDir := filepath.Join(tmp, "src")
	if err := fetchGlibcLocaledata(corpusLocalesGlibcURL, srcDir, corpusLocalesGlibcSHA256); err != nil {
		return "", fmt.Errorf("fetch glibc localedata: %w", err)
	}
	storeTmp := filepath.Join(tmp, "store")
	if err := compileCorpusLocales(srcDir, storeTmp); err != nil {
		return "", err
	}
	marker := corpusStoreMarker()
	if err := os.WriteFile(filepath.Join(storeTmp, ".bashy-locales-provisioned"), []byte(marker), 0o644); err != nil {
		return "", err
	}
	if err := os.RemoveAll(dst); err != nil {
		return "", err
	}
	if err := os.Rename(tmp, dst); err != nil {
		return "", fmt.Errorf("publish corpus locale cache: %w", err)
	}
	return store, nil
}

// corpusStoreMarker identifies a current store: glibc version, tarball
// digest and staging-rules version, one per line.
func corpusStoreMarker() string {
	return corpusLocalesVersion + "\n" + corpusLocalesGlibcSHA256 + "\n" + corpusLocalesProvisioner + "\n"
}

// validCorpusStore reports whether dst already holds a current provisioned
// store: marker match plus every expected store file present.
func validCorpusStore(dst, store string) bool {
	b, err := os.ReadFile(filepath.Join(store, ".bashy-locales-provisioned"))
	if err != nil || string(b) != corpusStoreMarker() {
		return false
	}
	for _, spec := range corpusLocaleBuild {
		path, err := corelocale.StorePath(store, spec.StoreName)
		if err != nil {
			return false
		}
		if fi, err := os.Stat(path); err != nil || fi.IsDir() {
			return false
		}
	}
	return true
}

// fetchGlibcLocaledata streams the tarball and extracts only the locale
// sources and charmaps the corpus build needs (never the whole glibc tree).
func fetchGlibcLocaledata(url, dst, wantSHA256 string) error {
	client := &http.Client{Timeout: 300 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("http %d", resp.StatusCode)
	}
	h := sha256.New()
	gz, err := gzip.NewReader(io.TeeReader(resp.Body, h))
	if err != nil {
		return err
	}
	defer gz.Close()
	want := map[string]bool{}
	for _, f := range corpusLocaleSources {
		want["glibc-2.44/"+f] = true
	}
	for _, f := range corpusLocaleCharmaps {
		want["glibc-2.44/"+f] = true
	}
	tr := tar.NewReader(gz)
	found := map[string]bool{}
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		if !want[hdr.Name] {
			continue
		}
		rel := strings.TrimPrefix(hdr.Name, "glibc-2.44/")
		if strings.Contains(rel, "..") {
			continue // path-traversal guard
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		target := filepath.Join(dst, rel)
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		f, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
		if err != nil {
			return err
		}
		if _, err := io.Copy(f, tr); err != nil {
			f.Close()
			return err
		}
		f.Close()
		found[hdr.Name] = true
	}
	gotSHA256 := hex.EncodeToString(h.Sum(nil))
	if gotSHA256 != strings.ToLower(strings.TrimSpace(wantSHA256)) {
		return fmt.Errorf("sha256 mismatch: got %s, want %s", gotSHA256, wantSHA256)
	}
	var missing []string
	for name := range want {
		if !found[name] {
			missing = append(missing, strings.TrimPrefix(name, "glibc-2.44/"))
		}
	}
	if len(missing) != 0 {
		return fmt.Errorf("tarball did not contain %d needed localedata member(s): %s", len(missing), strings.Join(missing, ", "))
	}
	return nil
}

// compileCorpusLocales stages and compiles every spec in build order into
// storeDir, resolving `copy` directives against the store being built — the
// in-process equivalent of running localedef(1) with -c per locale.
func compileCorpusLocales(srcDir, storeDir string) error {
	if err := os.MkdirAll(storeDir, 0o755); err != nil {
		return err
	}
	charmaps := map[string][]byte{}
	for _, spec := range corpusLocaleBuild {
		if _, ok := charmaps[spec.Charmap]; ok {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(srcDir, "localedata", "charmaps", spec.Charmap))
		if err != nil {
			return fmt.Errorf("read charmap %s: %w", spec.Charmap, err)
		}
		charmaps[spec.Charmap] = stageCharmap(spec.Charmap, raw)
	}
	for _, spec := range corpusLocaleBuild {
		raw, err := os.ReadFile(filepath.Join(srcDir, "localedata", "locales", spec.Source))
		if err != nil {
			return fmt.Errorf("read locale source %s: %w", spec.Source, err)
		}
		staged := stageLocaleSource(spec.Source, raw)
		if err := compileOneCorpusLocale(storeDir, spec, charmaps[spec.Charmap], staged); err != nil {
			return err
		}
	}
	return nil
}

// compileOneCorpusLocale compiles one staged source under its store name.
// Diagnostics that localedef(1) reports as warnings (undefined symbols in
// LC_CTYPE/LC_COLLATE) are tolerated like -c; anything else fails closed:
// errors never create store output.
func compileOneCorpusLocale(storeDir string, spec corpusLocaleSpec, charmap, source []byte) error {
	cm, err := localedef.ParseCharmap(bytes.NewReader(charmap))
	if err != nil {
		return fmt.Errorf("localedef %s: charmap %s: %w", spec.StoreName, spec.Charmap, err)
	}
	src, err := localedef.ParseSource(bytes.NewReader(source))
	if err != nil {
		return fmt.Errorf("localedef %s: source %s: %w", spec.StoreName, spec.Source, err)
	}
	for _, d := range localedef.Validate(src, cm) {
		if !d.Warning {
			return fmt.Errorf("localedef %s: %v", spec.StoreName, d)
		}
		fmt.Fprintf(os.Stderr, "corpus locales: %s: warning: %v\n", spec.StoreName, d)
	}
	env := []string{"LOCPATH=" + storeDir}
	resolve := func(name string) (*corelocale.Compiled, bool) {
		return corelocale.LookupCompiled(env, name)
	}
	compiled, err := localedef.CompileWithCopy(spec.StoreName, src, cm, resolve)
	if err != nil {
		return fmt.Errorf("localedef %s: %w", spec.StoreName, err)
	}
	if err := corelocale.Save(storeDir, compiled); err != nil {
		return fmt.Errorf("localedef %s: save: %w", spec.StoreName, err)
	}
	return nil
}
