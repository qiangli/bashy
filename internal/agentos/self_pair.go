package agentos

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/qiangli/yoke/pkg/binmgr"
)

func productMemberNames() []string {
	return []string{binmgr.BinaryName("bashy"), binmgr.BinaryName("bash"), binmgr.BinaryName("sh"), binmgr.BinaryName("outpost")}
}
func adjacentProduct(exe string) (map[string]string, error) {
	if real, err := filepath.EvalSymlinks(exe); err == nil {
		exe = real
	}
	paths := map[string]string{}
	for _, name := range productMemberNames() {
		path := filepath.Join(filepath.Dir(exe), name)
		fi, err := os.Stat(path)
		if err != nil || !fi.Mode().IsRegular() {
			return nil, fmt.Errorf("adjacent release is incomplete: %s missing; extract the complete release archive or pass --version", name)
		}
		paths[name] = path
	}
	return paths, nil
}

// installProduct probes all inputs before touching the installed generation.
// A running daemon owns its upgrade/rollback ledger, so hand it the whole
// staged archive using the candidate outpost CLI (which understands this format).
func installProduct(ctx context.Context, members map[string]string, target string, stdout, stderr io.Writer) error {
	marker := filepath.Join(filepath.Dir(target), ".outpost-installed-via")
	if data, err := os.ReadFile(marker); err == nil {
		via := strings.TrimSpace(string(data))
		if via != "" && via != "installer" && via != "manual" {
			return fmt.Errorf("installed via %s; use that package manager to upgrade", via)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	outpost := members[binmgr.BinaryName("outpost")]
	_, err := probeProduct(ctx, members)
	if err != nil {
		return err
	}
	expected := filepath.Join(filepath.Dir(target), binmgr.BinaryName("outpost"))
	running := false
	addr := ""
	if _, err := os.Stat(expected); err == nil {
		checkctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		data, err := exec.CommandContext(checkctx, outpost, "status", "--local-presence").Output()
		cancel()
		if err != nil {
			return fmt.Errorf("cannot establish whether installed outpost is running: %w", err)
		}
		var presence struct {
			Running bool   `json:"running"`
			Addr    string `json:"addr"`
		}
		if err := json.Unmarshal(data, &presence); err != nil || presence.Addr == "" {
			return errors.New("invalid outpost presence response")
		}
		running, addr = presence.Running, presence.Addr
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if running {
		checkctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		data, err := exec.CommandContext(checkctx, outpost, "--host", addr, "status", "--json").Output()
		cancel()
		if err != nil {
			return fmt.Errorf("running outpost is inaccessible; refusing to overwrite it: %w", err)
		}

		var status struct {
			Status struct {
				BinaryPath string `json:"binary_path"`
			} `json:"status"`
		}
		if err := json.Unmarshal(data, &status); err != nil {
			return fmt.Errorf("read running outpost status: %w", err)
		}
		if filepath.Clean(status.Status.BinaryPath) != filepath.Clean(expected) {
			return fmt.Errorf("running outpost is at %s; install into its directory to upgrade the pair", status.Status.BinaryPath)
		}
		tmp, err := os.MkdirTemp("", "bashy-pair-install-*")
		if err != nil {
			return err
		}
		defer os.RemoveAll(tmp)
		archive := filepath.Join(tmp, "product.tar.gz")
		sum, err := packProduct(archive, members)
		if err != nil {
			return err
		}
		child := exec.CommandContext(ctx, outpost, "--host", addr, "upgrade", "--local", archive, "--sha256", sum, "--force")
		child.Stdout, child.Stderr = stdout, stderr
		if err := child.Run(); err != nil {
			return err
		}
		return os.WriteFile(marker, []byte("installer\n"), 0644)
	}
	if err := installProductFiles(members, target); err != nil {
		return err
	}
	return os.WriteFile(marker, []byte("installer\n"), 0644)
}

func probeProduct(ctx context.Context, members map[string]string) (string, error) {
	probeCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	data, err := exec.CommandContext(probeCtx, members[binmgr.BinaryName("outpost")], "version", "--json").Output()
	if err != nil {
		return "", fmt.Errorf("probe outpost: %w", err)
	}
	var build struct{ Version, OS, Arch string }
	if err := json.Unmarshal(data, &build); err != nil {
		return "", fmt.Errorf("probe outpost version: %w", err)
	}
	if build.OS != runtime.GOOS || build.Arch != runtime.GOARCH {
		return "", fmt.Errorf("outpost platform %s/%s does not match %s/%s", build.OS, build.Arch, runtime.GOOS, runtime.GOARCH)
	}
	if build.Version == "" {
		return "", errors.New("paired install requires a stamped outpost release")
	}
	for _, name := range productMemberNames() {
		if name == binmgr.BinaryName("outpost") {
			continue
		}
		data, err := exec.CommandContext(probeCtx, members[name], "--version").Output()
		if err != nil {
			return "", fmt.Errorf("probe %s: %w", name, err)
		}
		if !matchingProductBanner(string(data), build.Version) {
			return "", fmt.Errorf("%s does not match outpost %s", name, build.Version)
		}
	}
	return build.Version, nil
}

// installProductFiles stages every member and old file before the first swap.
// It rolls earlier swaps back on an ordinary I/O failure; existing daemons use
// outpost's persistent upgrade transaction instead.
func installProductFiles(members map[string]string, target string) error {
	dir := filepath.Dir(target)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	stage, err := os.MkdirTemp(dir, ".bashy-install-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	type item struct {
		dst, src, old string
		existed       bool
	}
	items := []item{}
	for _, name := range productMemberNames() {
		dst := filepath.Join(dir, name)
		if name == releaseBinaryName() {
			dst = target
		}
		it := item{dst: dst, src: filepath.Join(stage, name), old: filepath.Join(stage, name+".previous")}
		if err := installExecutable(members[name], it.src); err != nil {
			return err
		}
		if _, err := os.Stat(dst); err == nil {
			it.existed = true
			if err := installExecutable(dst, it.old); err != nil {
				return err
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		items = append(items, it)
	}
	for i, it := range items {
		if err := installExecutable(it.src, it.dst); err != nil {
			all := []error{err}
			for j := i - 1; j >= 0; j-- {
				prev := items[j]
				if prev.existed {
					all = append(all, installExecutable(prev.old, prev.dst))
				} else {
					all = append(all, os.Remove(prev.dst))
				}
			}
			return errors.Join(all...)
		}
	}
	return nil
}

func packProduct(path string, members map[string]string) (string, error) {
	f, err := os.Create(path)
	if err != nil {
		return "", err
	}
	sum := sha256.New()
	gz := gzip.NewWriter(io.MultiWriter(f, sum))
	tw := tar.NewWriter(gz)
	for _, name := range productMemberNames() {
		src, err := os.Open(members[name])
		if err != nil {
			tw.Close()
			gz.Close()
			f.Close()
			return "", err
		}
		fi, err := src.Stat()
		if err == nil {
			err = tw.WriteHeader(&tar.Header{Name: name, Size: fi.Size(), Mode: 0755})
		}
		if err == nil {
			_, err = io.Copy(tw, src)
		}
		src.Close()
		if err != nil {
			tw.Close()
			gz.Close()
			f.Close()
			return "", err
		}
	}
	if err := tw.Close(); err != nil {
		gz.Close()
		f.Close()
		return "", err
	}
	if err := gz.Close(); err != nil {
		f.Close()
		return "", err
	}
	if err := f.Close(); err != nil {
		return "", err
	}
	return hex.EncodeToString(sum.Sum(nil)), nil
}

func matchingProductBanner(banner, version string) bool {
	_, stamp, ok := strings.Cut(banner, "-bashy-")
	if !ok {
		return false
	}
	if end := strings.IndexAny(stamp, " \n\r\t()"); end >= 0 {
		stamp = stamp[:end]
	}
	normalize := func(s string) string {
		return strings.TrimSuffix(strings.TrimPrefix(strings.TrimSpace(s), "v"), "-dev")
	}
	return normalize(stamp) != "" && normalize(stamp) == normalize(version)
}
