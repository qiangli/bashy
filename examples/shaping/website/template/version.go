package main

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
)

const (
	defaultFollowInterval = 2 * time.Second
	failedRevRetry        = 30 * time.Second
)

// computeAssetsVersion returns a deterministic short content digest of the
// embedded site assets so the server always has a stable version id even when
// SITE_VERSION is unset (e.g. before the first approval or inside unit tests).
func computeAssetsVersion(fsys fs.FS) string {
	var paths []string
	_ = fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		paths = append(paths, p)
		return nil
	})
	sort.Strings(paths)
	h := sha256.New()
	for _, p := range paths {
		data, err := fs.ReadFile(fsys, p)
		if err != nil {
			continue
		}
		_, _ = io.WriteString(h, p)
		_, _ = h.Write([]byte{0})
		_, _ = h.Write(data)
		_, _ = h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))[:12]
}

// shortRev abbreviates a commit SHA for logs and temporary filenames.
func shortRev(rev string) string {
	if len(rev) > 12 {
		return rev[:12]
	}
	return rev
}

// encodeStateComponent matches yoke/pkg/shape/state's directory encoding:
// ASCII alphanumerics plus '-', '_', '.' stay literal; other bytes become %XX.
func encodeStateComponent(value string) string {
	value = strings.TrimSpace(value)
	var b strings.Builder
	const hexDigits = "0123456789ABCDEF"
	for _, r := range []byte(value) {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.' {
			b.WriteByte(r)
			continue
		}
		b.WriteByte('%')
		b.WriteByte(hexDigits[r>>4])
		b.WriteByte(hexDigits[r&15])
	}
	return b.String()
}

type shapeStateRecord struct {
	ActiveRevision string `json:"active_revision"`
}

// shapeFollower watches the host's shaping state for the owner's active
// revision, rebuilds the site-shaping binary from that revision in the managed
// src checkout, and hands off serving to the rebuilt binary.
type shapeFollower struct {
	Home           string
	Apps           []string
	Principal      string // explicit owner principal; empty scans app state dirs
	FallbackUser   string // BASHY_PRINCIPAL fallback checked before scanning
	GoBin          string
	GitBin         string
	BinPath        string
	PollInterval   time.Duration
	CurrentVersion string
	OnSwap         func(bin, rev string, env []string) error

	failedRev string
	failedAt  time.Time
}

func newShapeFollower(getenv func(string) string, currentVersion string) *shapeFollower {
	rawApps := strings.TrimSpace(getenv("SITE_SHAPE_APP"))
	if rawApps == "" {
		return nil
	}
	var apps []string
	for _, part := range strings.Split(rawApps, ",") {
		if a := strings.TrimSpace(part); a != "" {
			apps = append(apps, a)
		}
	}
	if len(apps) == 0 {
		return nil
	}
	home := strings.TrimSpace(getenv("SITE_SHAPE_HOME"))
	if home == "" {
		home = strings.TrimSpace(getenv("BASHY_HOME"))
	}
	if home == "" {
		if h, err := os.UserHomeDir(); err == nil && h != "" {
			home = filepath.Join(h, ".bashy")
		}
	}
	if home == "" {
		return nil
	}
	goBin := strings.TrimSpace(getenv("SITE_GO_BIN"))
	if goBin == "" {
		goBin = strings.TrimSpace(getenv("WEBSITE_GO_BIN"))
	}
	if goBin == "" {
		goBin = "go"
	}
	gitBin := strings.TrimSpace(getenv("SITE_GIT_BIN"))
	if gitBin == "" {
		gitBin = "git"
	}
	binPath := strings.TrimSpace(getenv("SITE_BUILD_BIN"))
	if binPath == "" {
		if exe, err := os.Executable(); err == nil {
			if resolved, rerr := filepath.EvalSymlinks(exe); rerr == nil {
				binPath = resolved
			} else {
				binPath = exe
			}
		}
	}
	interval := defaultFollowInterval
	if raw := strings.TrimSpace(getenv("SITE_FOLLOW_INTERVAL")); raw != "" {
		if d, err := time.ParseDuration(raw); err == nil && d > 0 {
			interval = d
		}
	}
	return &shapeFollower{
		Home:           home,
		Apps:           apps,
		Principal:      strings.TrimSpace(getenv("SITE_SHAPE_PRINCIPAL")),
		FallbackUser:   strings.TrimSpace(getenv("BASHY_PRINCIPAL")),
		GoBin:          goBin,
		GitBin:         gitBin,
		BinPath:        binPath,
		PollInterval:   interval,
		CurrentVersion: currentVersion,
	}
}

func readActiveRevisionFile(path string) (string, time.Time, bool) {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return "", time.Time{}, false
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", time.Time{}, false
	}
	var st shapeStateRecord
	if err := json.Unmarshal(raw, &st); err != nil {
		return "", time.Time{}, false
	}
	rev := strings.TrimSpace(st.ActiveRevision)
	if rev == "" {
		return "", time.Time{}, false
	}
	return rev, info.ModTime(), true
}

// activeRevision looks up the owner's active revision for the first configured
// shaped app whose managed src checkout and state record exist.
func (f *shapeFollower) activeRevision() (app, srcDir, rev string, ok bool) {
	for _, candidate := range f.Apps {
		sdir := filepath.Join(f.Home, "src", candidate)
		st, err := os.Stat(sdir)
		if err != nil || !st.IsDir() {
			continue
		}
		appStateDir := filepath.Join(f.Home, "shape", encodeStateComponent(candidate))
		if f.Principal != "" {
			p := filepath.Join(appStateDir, encodeStateComponent(f.Principal), "state.json")
			if r, _, found := readActiveRevisionFile(p); found {
				return candidate, sdir, r, true
			}
			continue
		}
		if f.FallbackUser != "" {
			p := filepath.Join(appStateDir, encodeStateComponent(f.FallbackUser), "state.json")
			if r, _, found := readActiveRevisionFile(p); found {
				return candidate, sdir, r, true
			}
		}
		// Only the owner's line is public. Per-person versions of members or
		// other principals never become the served site (no newest-of-any scan).
	}
	return "", "", "", false
}

// extractRevision unpacks git commit rev from srcDir into dstDir without
// modifying srcDir's working tree.
func (f *shapeFollower) extractRevision(ctx context.Context, srcDir, rev, dstDir string) error {
	gitBin := f.GitBin
	if gitBin == "" {
		gitBin = "git"
	}
	cmd := exec.CommandContext(ctx, gitBin, "-c", "safe.directory=*", "-C", srcDir, "archive", "--format=tar", rev)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("git archive %s: %w", shortRev(rev), err)
	}
	untarErr := untarInto(stdout, dstDir)
	waitErr := cmd.Wait()
	if untarErr != nil {
		return untarErr
	}
	if waitErr != nil {
		return fmt.Errorf("git archive %s: %v (%s)", shortRev(rev), waitErr, strings.TrimSpace(stderr.String()))
	}
	return nil
}

func untarInto(r io.Reader, dstDir string) error {
	tr := tar.NewReader(r)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		if hdr.Typeflag == tar.TypeXHeader || hdr.Typeflag == tar.TypeXGlobalHeader {
			continue
		}
		clean := filepath.Clean(filepath.FromSlash(hdr.Name))
		if clean == "" || clean == "." || clean == ".." || filepath.IsAbs(clean) || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
			continue
		}
		target := filepath.Join(dstDir, clean)
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg, tar.TypeRegA:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			mode := os.FileMode(hdr.Mode) & 0o777
			if mode == 0 {
				mode = 0o644
			}
			out, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
			if err != nil {
				return err
			}
			_, copyErr := io.Copy(out, tr)
			closeErr := out.Close()
			if copyErr != nil {
				return copyErr
			}
			if closeErr != nil {
				return closeErr
			}
		}
	}
}

// buildRevision extracts rev from srcDir and compiles a new site-shaping binary.
func (f *shapeFollower) buildRevision(ctx context.Context, srcDir, rev string) (string, error) {
	if f.BinPath == "" {
		return "", errors.New("cannot determine executable path for rebuild")
	}
	tmp, err := os.MkdirTemp("", "site-shaping-rev-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tmp)

	src := filepath.Join(tmp, "src")
	if err := os.MkdirAll(src, 0o755); err != nil {
		return "", err
	}
	if err := f.extractRevision(ctx, srcDir, rev, src); err != nil {
		return "", err
	}

	binDir := filepath.Dir(f.BinPath)
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		return "", err
	}
	goBin := f.GoBin
	if goBin == "" {
		goBin = "go"
	}

	var outPath string
	if runtime.GOOS == "windows" {
		outPath = filepath.Join(binDir, "site-shaping-"+shortRev(rev)+".exe")
	} else {
		outPath = filepath.Join(binDir, ".site-shaping-"+shortRev(rev)+".tmp")
	}
	cmd := exec.CommandContext(ctx, goBin, "build", "-trimpath", "-o", outPath, ".")
	cmd.Dir = src
	cmd.Env = withEnv(os.Environ(), "GOWORK", "off")
	cmd.Env = withEnv(cmd.Env, "CGO_ENABLED", "0")
	if out, err := cmd.CombinedOutput(); err != nil {
		_ = os.Remove(outPath)
		return "", fmt.Errorf("go build %s: %v\n%s", shortRev(rev), err, strings.TrimSpace(string(out)))
	}
	if runtime.GOOS == "windows" {
		return outPath, nil
	}
	if err := os.Rename(outPath, f.BinPath); err != nil {
		_ = os.Remove(outPath)
		return "", err
	}
	return f.BinPath, nil
}

func withEnv(environ []string, key, value string) []string {
	prefix := key + "="
	out := make([]string, 0, len(environ)+1)
	replaced := false
	for _, kv := range environ {
		if strings.HasPrefix(kv, prefix) {
			if !replaced {
				out = append(out, prefix+value)
				replaced = true
			}
			continue
		}
		out = append(out, kv)
	}
	if !replaced {
		out = append(out, prefix+value)
	}
	return out
}

// step checks once whether the owner's active revision has changed, rebuilds
// from that revision when it has, and invokes OnSwap.
func (f *shapeFollower) step(ctx context.Context) (bool, error) {
	_, srcDir, rev, ok := f.activeRevision()
	if !ok || rev == f.CurrentVersion {
		return false, nil
	}
	if rev == f.failedRev && time.Since(f.failedAt) < failedRevRetry {
		return false, nil
	}
	buildCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	bin, err := f.buildRevision(buildCtx, srcDir, rev)
	if err != nil {
		f.failedRev, f.failedAt = rev, time.Now()
		return false, err
	}
	f.failedRev = ""
	env := withEnv(os.Environ(), "SITE_VERSION", rev)
	if f.OnSwap != nil {
		if err := f.OnSwap(bin, rev, env); err != nil {
			f.failedRev, f.failedAt = rev, time.Now()
			return false, err
		}
	}
	f.CurrentVersion = rev
	return true, nil
}

func (f *shapeFollower) run(ctx context.Context) {
	interval := f.PollInterval
	if interval <= 0 {
		interval = defaultFollowInterval
	}
	check := func() bool {
		swapped, err := f.step(ctx)
		if err != nil {
			log.Printf("site-shaping: follow approval failed: %v", err)
			return false
		}
		return swapped
	}
	if check() {
		return
	}
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if check() {
				return
			}
		}
	}
}
