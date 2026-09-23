// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

package agentos

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
)

const (
	barMaxArchiveBytes = 512 << 20
	barMaxExpanded     = 2 << 30
	barMaxFiles        = 20000
)

func isBarBundle(name string) bool {
	if info, err := os.Stat(name); err != nil || !info.Mode().IsRegular() {
		return false
	}
	lower := strings.ToLower(name)
	for _, ext := range []string{".bar", ".zip", ".tar", ".tar.gz", ".tgz", ".gz"} {
		if strings.HasSuffix(lower, ext) {
			return true
		}
	}
	f, err := os.Open(name)
	if err != nil {
		return false
	}
	defer f.Close()
	var head [512]byte
	n, err := io.ReadFull(f, head[:])
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		return false
	}
	if n >= 4 && bytes.Equal(head[:4], []byte{'P', 'K', 3, 4}) || n >= 2 && head[0] == 0x1f && head[1] == 0x8b {
		return true
	}
	if n == len(head) && string(head[257:262]) == "ustar" {
		return true
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return false
	}
	_, err = tar.NewReader(f).Next()
	return err == nil
}

func isDagEntry(name string) bool {
	info, err := os.Stat(name)
	if err != nil {
		return false
	}
	if info.IsDir() {
		entry, err := os.Stat(filepath.Join(name, "dag.md"))
		return err == nil && entry.Mode().IsRegular()
	}
	return info.Mode().IsRegular() && filepath.Base(name) == "dag.md"
}

func dispatchDagEntry(source, target string, args []string, capture bool) int {
	isArchive := isBarBundle(source)
	var root, dagFile string
	var err error
	if isArchive {
		root, err = materializeBar(source)
		dagFile = "dag.md"
		if target == "" {
			target = "main"
		}
	} else {
		absolute, absErr := filepath.Abs(source)
		if absErr != nil {
			fmt.Fprintf(os.Stderr, "bashy run: resolve DAG path: %v\n", absErr)
			return 2
		}
		info, statErr := os.Stat(absolute)
		if statErr != nil {
			fmt.Fprintf(os.Stderr, "bashy run: inspect DAG path: %v\n", statErr)
			return 2
		}
		if info.IsDir() {
			root, dagFile = absolute, "dag.md"
		} else {
			root, dagFile = filepath.Dir(absolute), filepath.Base(absolute)
		}
		if target == "" && hasDAGTarget(root, dagFile, "main") {
			target = "main"
		}
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "bashy run: %v\n", err)
		return 2
	}
	// The DAG receives user arguments as JSON so every original argument
	// remains distinct and can be decoded by a Bashy Python fence if needed.
	encodedArgs, err := json.Marshal(args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "bashy run: encode bundle arguments: %v\n", err)
		return 2
	}
	self, err := os.Executable()
	if err != nil {
		fmt.Fprintf(os.Stderr, "bashy run: locate executable: %v\n", err)
		return 2
	}
	argv := []string{self, "dag", "--quiet", "--file", dagFile}
	if target != "" {
		argv = append(argv, target)
	}
	caller, _ := os.Getwd()
	extraEnv := []string{"BASHY_DAG_ARGS_JSON=" + string(encodedArgs), "BASHY_DAG_CALLER_PWD=" + caller}
	if isArchive {
		archivePath, _ := filepath.Abs(source)
		extraEnv = append(extraEnv, "BASHY_BAR_ARGS_JSON="+string(encodedArgs), "BASHY_BAR_PATH="+archivePath)
	}
	env, status := runCommandAtEnv(argv, capture, false, root, extraEnv, os.Stdout, os.Stderr)
	b, _ := json.Marshal(env)
	if capture {
		fmt.Fprintln(os.Stdout, string(b))
	} else {
		fmt.Fprintln(os.Stderr, string(b))
	}
	return status
}

func materializeBar(source string) (string, error) {
	archivePath, err := filepath.Abs(source)
	if err != nil {
		return "", fmt.Errorf("resolve bundle path: %w", err)
	}
	data, err := os.ReadFile(archivePath)
	if err != nil {
		return "", fmt.Errorf("read bundle: %w", err)
	}
	if len(data) == 0 || len(data) > barMaxArchiveBytes {
		return "", fmt.Errorf("bundle size must be between 1 byte and %d bytes", barMaxArchiveBytes)
	}
	sum := sha256.Sum256(data)
	name := safeBarName(filepath.Base(archivePath))
	home := strings.TrimSpace(os.Getenv("BASHY_HOME"))
	if home == "" {
		home, err = os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve home directory: %w", err)
		}
		home = filepath.Join(home, ".bashy")
	}
	cache := filepath.Join(home, "cache", "bars", name, hex.EncodeToString(sum[:]))
	if info, statErr := os.Stat(filepath.Join(cache, "dag.md")); statErr == nil && info.Mode().IsRegular() {
		if err := validateBarMain(cache); err == nil {
			return cache, nil
		}
		_ = os.RemoveAll(cache)
	}
	parent := filepath.Dir(cache)
	if err := os.MkdirAll(parent, 0o700); err != nil {
		return "", fmt.Errorf("create bundle cache: %w", err)
	}
	stage, err := os.MkdirTemp(parent, ".extract-*")
	if err != nil {
		return "", fmt.Errorf("create extraction directory: %w", err)
	}
	defer os.RemoveAll(stage)
	if err := extractBar(data, stage); err != nil {
		return "", err
	}
	if info, err := os.Stat(filepath.Join(stage, "dag.md")); err != nil || !info.Mode().IsRegular() {
		return "", errors.New("bundle must contain a top-level regular dag.md")
	}
	if err := validateBarMain(stage); err != nil {
		return "", err
	}
	if err := os.Rename(stage, cache); err != nil {
		// Another invocation may have published this content hash first.
		if info, statErr := os.Stat(filepath.Join(cache, "dag.md")); statErr == nil && info.Mode().IsRegular() {
			return cache, nil
		}
		return "", fmt.Errorf("publish bundle to cache: %w", err)
	}
	return cache, nil
}

func extractBar(data []byte, dest string) error {
	if len(data) >= 4 && bytes.Equal(data[:4], []byte{'P', 'K', 3, 4}) {
		return extractBarZip(data, dest)
	}
	var reader io.Reader = bytes.NewReader(data)
	if len(data) >= 2 && data[0] == 0x1f && data[1] == 0x8b {
		gz, err := gzip.NewReader(reader)
		if err != nil {
			return fmt.Errorf("open gzip bundle: %w", err)
		}
		defer gz.Close()
		reader = gz
	}
	tr := tar.NewReader(reader)
	seen := map[string]bool{}
	var expanded int64
	files := 0
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("read tar bundle: %w", err)
		}
		if h.Name == "./" || h.Name == "." {
			continue
		}
		name, err := safeArchivePath(h.Name)
		if err != nil {
			return err
		}
		if h.Typeflag == tar.TypeDir {
			if err := os.MkdirAll(filepath.Join(dest, filepath.FromSlash(name)), 0o755); err != nil {
				return fmt.Errorf("create bundle directory: %w", err)
			}
			continue
		}
		if h.Typeflag != tar.TypeReg && h.Typeflag != tar.TypeRegA {
			return fmt.Errorf("bundle entry %q has unsupported tar type %d", h.Name, h.Typeflag)
		}
		files++
		if files > barMaxFiles || h.Size < 0 || h.Size > barMaxExpanded || expanded > barMaxExpanded-h.Size {
			return errors.New("bundle exceeds extraction limits")
		}
		expanded += h.Size
		if seen[name] {
			return fmt.Errorf("bundle contains duplicate path %q", name)
		}
		seen[name] = true
		mode := os.FileMode(h.Mode) & 0o777
		if mode == 0 {
			mode = 0o644
		}
		if err := writeBarFile(dest, name, mode, io.LimitReader(tr, h.Size), h.Size); err != nil {
			return err
		}
	}
	return nil
}

func validateBarMain(dir string) error {
	if hasDAGTarget(dir, "dag.md", "main") {
		return nil
	}
	return errors.New("bundle dag.md must declare a top-level main target")
}

func hasDAGTarget(dir, dagFile, target string) bool {
	self, err := os.Executable()
	if err != nil {
		return false
	}
	cmd := exec.Command(self, "dag", "--list", "--json", "--file", dagFile)
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if cmd.Run() != nil {
		return false
	}
	var envelope struct {
		Status string `json:"status"`
		Result struct {
			Tasks []struct {
				Name string `json:"name"`
			} `json:"tasks"`
		} `json:"result"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		return false
	}
	if envelope.Status != "ok" {
		return false
	}
	for _, task := range envelope.Result.Tasks {
		if task.Name == target {
			return true
		}
	}
	return false
}

func extractBarZip(data []byte, dest string) error {
	r, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return fmt.Errorf("open zip bundle: %w", err)
	}
	if len(r.File) > barMaxFiles {
		return errors.New("bundle exceeds extraction limits")
	}
	seen := map[string]bool{}
	var expanded uint64
	files := 0
	for _, f := range r.File {
		name, err := safeArchivePath(f.Name)
		if err != nil {
			return err
		}
		mode := f.Mode()
		if mode.IsDir() {
			if err := os.MkdirAll(filepath.Join(dest, filepath.FromSlash(name)), 0o755); err != nil {
				return fmt.Errorf("create bundle directory: %w", err)
			}
			continue
		}
		if !mode.IsRegular() {
			return fmt.Errorf("bundle entry %q is not a regular file", f.Name)
		}
		files++
		if files > barMaxFiles || f.UncompressedSize64 > uint64(barMaxExpanded) || expanded > uint64(barMaxExpanded)-f.UncompressedSize64 {
			return errors.New("bundle exceeds extraction limits")
		}
		expanded += f.UncompressedSize64
		if seen[name] {
			return fmt.Errorf("bundle contains duplicate path %q", name)
		}
		seen[name] = true
		rc, err := f.Open()
		if err != nil {
			return fmt.Errorf("open bundle entry %q: %w", name, err)
		}
		perm := mode.Perm() & 0o777
		if perm == 0 {
			perm = 0o644
		}
		err = writeBarFile(dest, name, perm, rc, int64(f.UncompressedSize64))
		closeErr := rc.Close()
		if err != nil {
			return err
		}
		if closeErr != nil {
			return fmt.Errorf("close bundle entry %q: %w", name, closeErr)
		}
	}
	return nil
}

func safeArchivePath(name string) (string, error) {
	if strings.ContainsRune(name, '\\') || strings.HasPrefix(name, "/") || filepath.IsAbs(name) {
		return "", fmt.Errorf("bundle contains unsafe path %q", name)
	}
	clean := path.Clean(name)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", fmt.Errorf("bundle contains unsafe path %q", name)
	}
	return clean, nil
}

func writeBarFile(root, name string, mode os.FileMode, src io.Reader, size int64) error {
	if size < 0 || size > barMaxExpanded {
		return errors.New("bundle exceeds extraction limits")
	}
	dest := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return fmt.Errorf("create parent for %q: %w", name, err)
	}
	f, err := os.OpenFile(dest, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
	if err != nil {
		return fmt.Errorf("create bundle entry %q: %w", name, err)
	}
	written, copyErr := io.CopyN(f, src, size)
	closeErr := f.Close()
	if copyErr != nil {
		return fmt.Errorf("write bundle entry %q: %w", name, copyErr)
	}
	if written != size {
		return fmt.Errorf("bundle entry %q has a truncated payload", name)
	}
	if closeErr != nil {
		return fmt.Errorf("close bundle entry %q: %w", name, closeErr)
	}
	return nil
}

func safeBarName(name string) string {
	lower := strings.ToLower(name)
	for _, ext := range []string{".tar.gz", ".tgz", ".bar", ".zip", ".tar", ".gz"} {
		if strings.HasSuffix(lower, ext) {
			name = name[:len(name)-len(ext)]
			break
		}
	}
	var b strings.Builder
	for _, r := range name {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	if b.Len() == 0 {
		return "bundle"
	}
	return b.String()
}
