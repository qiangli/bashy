// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

//go:build e2e

package agentos

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestE2ERunBarArchivesAndCacheVersions(t *testing.T) {
	bin := bashyBinary(t)
	home := t.TempDir()
	formats := []string{"bar", "zip", "tar", "tar.gz", "tgz", "gz"}
	for _, format := range formats {
		t.Run(format, func(t *testing.T) {
			archive := filepath.Join(t.TempDir(), "sample."+format)
			writeBarFixture(t, archive, format, barDAG("one"), nil)
			stdout, stderr, code := runBashyStdEnv(bin,
				[]string{"BASHY_HOME=" + home}, "run", "--capture", archive, "first", "two words")
			if code != 0 {
				t.Fatalf("bashy run exited %d\nstdout=%s\nstderr=%s", code, stdout, stderr)
			}
			first := decodeRunEnvelope(t, stdout)
			if !strings.Contains(first.Cwd, filepath.Join("cache", "bars")) {
				t.Fatalf("archive ran outside bundle cache: cwd=%q", first.Cwd)
			}
			if first.Exit != 0 {
				t.Fatalf("bundle target exit = %d", first.Exit)
			}
			wantArgs, _ := json.Marshal([]string{"first", "two words"})
			if got, err := os.ReadFile(filepath.Join(first.Cwd, "args.txt")); err != nil || strings.TrimSpace(string(got)) != string(wantArgs) {
				t.Fatalf("forwarded args = %q, err=%v; want %s", got, err, wantArgs)
			}
			if got, err := os.ReadFile(filepath.Join(first.Cwd, "marker.txt")); err != nil || string(got) != "one" {
				t.Fatalf("main target marker = %q, err=%v", got, err)
			}

			// Identical bytes reuse the exact cache directory. A changed archive
			// publishes a new content version without replacing the active one.
			stdout, stderr, code = runBashyStdEnv(bin, []string{"BASHY_HOME=" + home}, "run", "--capture", archive)
			if code != 0 {
				t.Fatalf("repeat run exited %d\nstdout=%s\nstderr=%s", code, stdout, stderr)
			}
			repeat := decodeRunEnvelope(t, stdout)
			if repeat.Cwd != first.Cwd {
				t.Fatalf("unchanged bundle cache changed: first=%q second=%q", first.Cwd, repeat.Cwd)
			}

			writeBarFixture(t, archive, format, barDAG("two"), nil)
			stdout, stderr, code = runBashyStdEnv(bin, []string{"BASHY_HOME=" + home}, "run", "--capture", archive)
			if code != 0 {
				t.Fatalf("updated bundle run exited %d\nstdout=%s\nstderr=%s", code, stdout, stderr)
			}
			updated := decodeRunEnvelope(t, stdout)
			if updated.Cwd == first.Cwd {
				t.Fatal("changed bundle reused the old content cache directory")
			}
			if got, err := os.ReadFile(filepath.Join(first.Cwd, "marker.txt")); err != nil || string(got) != "one" {
				t.Fatalf("old content version was overwritten: marker=%q err=%v", got, err)
			}
			if got, err := os.ReadFile(filepath.Join(updated.Cwd, "marker.txt")); err != nil || string(got) != "two" {
				t.Fatalf("new content version marker=%q err=%v", got, err)
			}
		})
	}
}

func TestE2ERunDagFileFolderAndTarget(t *testing.T) {
	bin := bashyBinary(t)
	dir := t.TempDir()
	dag := "## Tasks\n\n### main\nEffects: write\n\n```bash\nprintf main > selected.txt\n```\n\n### custom\nEffects: write\n\n```bash\nprintf custom > selected.txt\n```\n"
	if err := os.WriteFile(filepath.Join(dir, "dag.md"), []byte(dag), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"run", dir}, {"run", filepath.Join(dir, "dag.md"), "--target", "custom"}} {
		cliArgs := append([]string{"run", "--capture"}, args[1:]...)
		stdout, stderr, code := runBashyStdEnv(bin, nil, cliArgs...)
		if code != 0 {
			t.Fatalf("bashy %v exited %d\nstdout=%s\nstderr=%s", cliArgs, code, stdout, stderr)
		}
		result := decodeRunEnvelope(t, stdout)
		if result.Cwd != dir {
			t.Fatalf("DAG ran from %q, want its own directory %q", result.Cwd, dir)
		}
		got, err := os.ReadFile(filepath.Join(dir, "selected.txt"))
		if err != nil {
			t.Fatal(err)
		}
		want := "main"
		if strings.Contains(strings.Join(cliArgs, " "), "custom") {
			want = "custom"
		}
		if string(got) != want {
			t.Fatalf("target output = %q, want %q", got, want)
		}
	}
}

func TestE2ERunArchiveDetectionUsesContainerAndDagMarker(t *testing.T) {
	bin := bashyBinary(t)
	for _, tc := range []struct {
		name      string
		filename  string
		container string
	}{
		{name: "zip payload with official extension", filename: "app.bar", container: "zip"},
		{name: "tar payload without extension", filename: "app", container: "tar"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			archive := filepath.Join(t.TempDir(), tc.filename)
			writeBarFixture(t, archive, tc.container, barDAG("marker"), nil)
			stdout, stderr, code := runBashyStdEnv(bin, []string{"BASHY_HOME=" + t.TempDir()}, "run", "--capture", archive)
			if code != 0 {
				t.Fatalf("archive run exited %d\nstdout=%s\nstderr=%s", code, stdout, stderr)
			}
			result := decodeRunEnvelope(t, stdout)
			if got, err := os.ReadFile(filepath.Join(result.Cwd, "marker.txt")); err != nil || string(got) != "marker" {
				t.Fatalf("archive main target marker=%q err=%v", got, err)
			}
		})
	}
	stdout, stderr, code := runBashyStd(bin, "run", "--help")
	if code != 0 || !strings.Contains(stdout, "dag.md") || !strings.Contains(stdout, "--target") {
		t.Fatalf("run help code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
}

func TestE2ERunBarRequiresTopLevelDagAndMain(t *testing.T) {
	bin := bashyBinary(t)
	for _, tc := range []struct {
		name    string
		entries map[string]string
		want    string
	}{
		{name: "missing dag", entries: map[string]string{"nested/dag.md": barDAG("x")}, want: "top-level regular dag.md"},
		{name: "missing main", entries: map[string]string{"dag.md": "## Tasks\n\n### other\n```bash\necho other\n```\n"}, want: "main target"},
		{name: "path traversal", entries: map[string]string{"dag.md": barDAG("x"), "../outside": "bad"}, want: "unsafe path"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			archive := filepath.Join(t.TempDir(), "invalid.bar")
			writeBarFixture(t, archive, "bar", tc.entries["dag.md"], tc.entries)
			stdout, stderr, code := runBashyStdEnv(bin, []string{"BASHY_HOME=" + t.TempDir()}, "run", archive)
			if code == 0 || !strings.Contains(stdout+stderr, tc.want) {
				t.Fatalf("invalid bundle result code=%d, wanted error %q\nstdout=%s\nstderr=%s", code, tc.want, stdout, stderr)
			}
		})
	}
}

type runOutput struct {
	Cwd  string `json:"cwd"`
	Exit int    `json:"exit"`
}

func decodeRunEnvelope(t *testing.T, raw string) runOutput {
	t.Helper()
	var out runOutput
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		t.Fatalf("parse bashy run envelope: %v\n%s", err, raw)
	}
	return out
}

func barDAG(value string) string {
	return fmt.Sprintf("## Tasks\n\n### main\nEffects: write\n\n```bash\nprintf %%s %q > marker.txt\nprintf '%%s' \"$BASHY_BAR_ARGS_JSON\" > args.txt\n```\n", value)
}

func writeBarFixture(t *testing.T, file, format, dag string, extra map[string]string) {
	t.Helper()
	f, err := os.Create(file)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var tarOut *tar.Writer
	var zipOut *zip.Writer
	var gzipOut *gzip.Writer
	var out bytes.Buffer
	compress := format == "bar" || format == "tar.gz" || format == "tgz" || format == "gz"
	if compress {
		gzipOut = gzip.NewWriter(&out)
		tarOut = tar.NewWriter(gzipOut)
	} else if format == "zip" {
		zipOut = zip.NewWriter(&out)
	} else {
		tarOut = tar.NewWriter(&out)
	}
	entries := map[string]string{}
	if dag != "" {
		entries["dag.md"] = dag
	}
	for name, value := range extra {
		entries[name] = value
	}
	for name, value := range entries {
		if tarOut != nil {
			if err := tarOut.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(value)), Typeflag: tar.TypeReg}); err != nil {
				t.Fatal(err)
			}
			if _, err := tarOut.Write([]byte(value)); err != nil {
				t.Fatal(err)
			}
		} else {
			w, err := zipOut.Create(name)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := w.Write([]byte(value)); err != nil {
				t.Fatal(err)
			}
		}
	}
	if tarOut != nil {
		if err := tarOut.Close(); err != nil {
			t.Fatal(err)
		}
	}
	if gzipOut != nil {
		if err := gzipOut.Close(); err != nil {
			t.Fatal(err)
		}
	}
	if zipOut != nil {
		if err := zipOut.Close(); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := f.Write(out.Bytes()); err != nil {
		t.Fatal(err)
	}
}
