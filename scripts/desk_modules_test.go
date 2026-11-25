package scripts_test

import (
	"bufio"
	"bytes"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"golang.org/x/mod/modfile"
	"golang.org/x/mod/module"
	"golang.org/x/mod/sumdb/dirhash"
	modzip "golang.org/x/mod/zip"
)

// The shared desk modules (kit, chart) are published only to the local Go
// module proxy, by examples/desk/register-modules.sh, at the versions it
// names. Every app requires them by version (no sibling replace), so a fresh
// host builds an app only when:
//   - the app requires exactly the versions register-modules.sh publishes
//     (an older version exists on no proxy), and
//   - the app's go.sum hash for that version is the hash of the tree that
//     register-modules.sh publishes today (a module edit without a version
//     bump is a checksum mismatch, refused by go as a SECURITY ERROR).
//
// Regression for the rc.41 finding: five apps pinned kit v0.3.0 and the
// others recorded a kit v0.4.0 hash from older content.
func TestDeskModuleConsumersMatchPublishedVersions(t *testing.T) {
	root := repoRoot(t)
	script, err := os.ReadFile(filepath.Join(root, "examples/desk/register-modules.sh"))
	if err != nil {
		t.Fatal(err)
	}
	published := map[string]string{} // module path -> version
	dirs := map[string]string{}      // module path -> source dir
	for name, dir := range map[string]string{"kit": "examples/desk/kit", "chart": "examples/desk/chart"} {
		m := regexp.MustCompile(`(?m)^desk_` + name + `_version=(v\S+)$`).FindSubmatch(script)
		if m == nil {
			t.Fatalf("register-modules.sh names no desk_%s_version", name)
		}
		path := modulePath(t, filepath.Join(root, dir))
		published[path] = string(m[1])
		dirs[path] = dir
	}

	hashes := map[string]string{}
	for path, version := range published {
		hashes[path] = trackedTreeHash(t, root, dirs[path], module.Version{Path: path, Version: version})
	}

	consumers := 0
	err = filepath.WalkDir(filepath.Join(root, "examples"), func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || d.Name() != "go.mod" {
			return err
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		f, err := modfile.Parse(p, data, nil)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, filepath.Dir(p))
		sums := goSum(t, filepath.Join(filepath.Dir(p), "go.sum"))
		uses := false
		for _, r := range f.Require {
			want, ok := published[r.Mod.Path]
			if !ok {
				continue
			}
			uses = true
			if r.Mod.Version != want {
				t.Errorf("%s requires %s %s; register-modules.sh publishes %s (no proxy serves any other version)", rel, r.Mod.Path, r.Mod.Version, want)
				continue
			}
			if got := sums[r.Mod.Path+" "+want]; got != hashes[r.Mod.Path] {
				t.Errorf("%s go.sum records %s %s as %q; the published tree hashes to %q (bump the version in register-modules.sh and move the consumers)", rel, r.Mod.Path, want, got, hashes[r.Mod.Path])
			}
		}
		for _, r := range f.Replace {
			if _, ok := published[r.Old.Path]; ok {
				t.Errorf("%s replaces %s; desk apps require the published version instead", rel, r.Old.Path)
			}
		}
		if uses {
			consumers++
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if consumers == 0 {
		t.Fatal("no example module requires the desk modules; the walk is broken")
	}
}

func modulePath(t *testing.T, dir string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	return modfile.ModulePath(data)
}

// trackedTreeHash zips the git-tracked files of dir the way `bashy mod proxy
// publish --version` does and returns the module's h1 hash.
func trackedTreeHash(t *testing.T, root, dir string, m module.Version) string {
	t.Helper()
	cmd := exec.Command("git", "ls-files", "-z", "--", dir)
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		t.Skipf("git ls-files unavailable (%v); the hash check needs a git checkout", err)
	}
	var files []modzip.File
	for _, name := range strings.Split(strings.TrimRight(string(out), "\x00"), "\x00") {
		if name == "" {
			continue
		}
		full := filepath.Join(root, filepath.FromSlash(name))
		if _, err := os.Lstat(full); err != nil {
			continue
		}
		files = append(files, zipFile{path: strings.TrimPrefix(name, dir+"/"), full: full})
	}
	var buf bytes.Buffer
	if err := modzip.Create(&buf, m, files); err != nil {
		t.Fatalf("zip %s: %v", m, err)
	}
	tmp := filepath.Join(t.TempDir(), "m.zip")
	if err := os.WriteFile(tmp, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	h, err := dirhash.HashZip(tmp, dirhash.DefaultHash)
	if err != nil {
		t.Fatal(err)
	}
	return h
}

type zipFile struct{ path, full string }

func (f zipFile) Path() string                 { return f.path }
func (f zipFile) Lstat() (os.FileInfo, error)  { return os.Lstat(f.full) }
func (f zipFile) Open() (io.ReadCloser, error) { return os.Open(f.full) }

func goSum(t *testing.T, path string) map[string]string {
	t.Helper()
	sums := map[string]string{}
	f, err := os.Open(path)
	if err != nil {
		return sums
	}
	defer f.Close()
	s := bufio.NewScanner(f)
	for s.Scan() {
		fields := strings.Fields(s.Text())
		if len(fields) == 3 && !strings.HasSuffix(fields[1], "/go.mod") {
			sums[fields[0]+" "+fields[1]] = fields[2]
		}
	}
	return sums
}
