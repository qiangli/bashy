package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// prepareUserland lays out the POSIX-shaped root a Windows fixture run sees
// (Sprint 245): the fixtures hard-code /bin/sh, /bin/echo, /usr/bin/printf,
// /etc/passwd and friends, and the shell under test resolves those through
// its BASHY_ROOT mounts. Unix has that root in the OS; Windows gets one
// materialized here, populated from bashy's OWN userland — the pure-Go
// multicall binary (yoke: coreutils + yoke applets) that `bashy` ships —
// so the measurement isolates the shell against tools whose behaviour is
// ours to fix, not a foreign MSYS runtime's.
//
//	<treeRoot>/root/usr/bin/<name>.exe   one hard link per applet the binary lists
//	<treeRoot>/root/usr/bin/<name>       its extensionless twin (Cygwin's view)
//	<treeRoot>/root/usr/bin/bash.exe     the testee, copied once
//	<treeRoot>/root/usr/bin/{sh.exe,sh,bash}  hard links to that copy
//	<treeRoot>/root/etc/passwd           a root line (redir.tests, coproc.tests)
//
// The multicall binary is copied into usr/bin first so every link is on the
// same volume as its target (a hosted runner checks out on D: and keeps
// TEMP on C:). It returns the root and the bin directory; the caller exports
// BASHY_ROOT and puts the bin dir on the fixture PATH.
func prepareUserland(treeRoot, userlandBin, bashPath string) (rootDir, binDir string, names int, err error) {
	rootDir = filepath.Join(treeRoot, "root")
	binDir = filepath.Join(rootDir, "usr", "bin")
	etcDir := filepath.Join(rootDir, "etc")
	for _, d := range []string{binDir, etcDir, filepath.Join(rootDir, "usr", "share")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return "", "", 0, err
		}
	}
	src, err := filepath.Abs(userlandBin)
	if err != nil {
		return "", "", 0, err
	}
	if _, err := os.Stat(src); err != nil {
		return "", "", 0, fmt.Errorf("userland binary: %v", err)
	}
	multicall := filepath.Join(binDir, filepath.Base(src))
	if err := copyFile(src, multicall, 0o755); err != nil {
		return "", "", 0, fmt.Errorf("copy userland binary: %v", err)
	}
	list, err := exec.Command(multicall, "--list").Output()
	if err != nil {
		return "", "", 0, fmt.Errorf("%s --list: %v", multicall, err)
	}
	for name := range strings.FieldsSeq(string(list)) {
		if !validAppletName(name) {
			continue
		}
		dst := filepath.Join(binDir, exeName(name))
		if err := linkOrCopy(multicall, dst); err != nil {
			return "", "", 0, fmt.Errorf("applet %s: %v", name, err)
		}
		// The extensionless twin is what a fixture sees when it treats
		// the root as a Unix tree — `cp /bin/sh .`, `ls /bin/echo` — the
		// way Cygwin presents its .exe files. CreateProcess runs a PE
		// image by full path whatever its name, so the twin also execs.
		if exeName(name) != name {
			if err := linkOrCopy(multicall, filepath.Join(binDir, name)); err != nil {
				return "", "", 0, fmt.Errorf("applet %s (extensionless): %v", name, err)
			}
		}
		names++
	}
	// The shell itself: fixtures exec /bin/sh (posix2.tests's `#! /bin/sh`,
	// jobs3.sub, rsh1.sub's `cp /bin/sh .`) and `type -t /bin/sh` must say
	// `file`. Copy the testee once, then link sh to it.
	bashCopy := filepath.Join(binDir, exeName("bash"))
	if err := copyFile(bashPath, bashCopy, 0o755); err != nil {
		return "", "", 0, fmt.Errorf("copy testee into root: %v", err)
	}
	for _, twin := range []string{exeName("sh"), "sh", "bash"} {
		if twin == exeName("bash") {
			continue
		}
		if err := linkOrCopy(bashCopy, filepath.Join(binDir, twin)); err != nil {
			return "", "", 0, fmt.Errorf("%s: %v", twin, err)
		}
	}
	passwd := "root:x:0:0:root:/root:/bin/sh\n" +
		"nobody:x:65534:65534:nobody:/:/bin/sh\n"
	if err := os.WriteFile(filepath.Join(etcDir, "passwd"), []byte(passwd), 0o644); err != nil {
		return "", "", 0, err
	}
	return rootDir, binDir, names, nil
}

// validAppletName keeps names that can be a file on NTFS and that a fixture
// could plausibly look up on PATH; the multicall binary lists a few
// bracket/punctuation aliases ("[" is fine, anything with a path separator
// or a reserved character is not).
func validAppletName(name string) bool {
	if name == "" || name == "." || name == ".." {
		return false
	}
	return !strings.ContainsAny(name, `/\:*?"<>|`)
}

// linkOrCopy hard-links dst to src, replacing dst, and copies when linking
// is impossible (another volume, a filesystem without links).
func linkOrCopy(src, dst string) error {
	_ = os.Remove(dst)
	if err := os.Link(src, dst); err == nil {
		return nil
	}
	return copyFile(src, dst, 0o755)
}

// posixSpelling returns a native Windows path in the shell's portable
// spelling (`C:\a\b` → `/c/a/b`); other hosts return the path unchanged.
// The harness hands the fixtures THIS_SH, BUILD_DIR and TMPDIR in this form
// because the corpus post-processes them as POSIX text — arith-for.tests
// strips `$0` with `sed 's|^.*/||'`, which a backslash path defeats — and
// because the shell under test prints $PWD in the same form.
func posixSpelling(path string, windows bool) string {
	if !windows || path == "" {
		return path
	}
	p := strings.ReplaceAll(path, `\`, "/")
	if len(p) >= 2 && p[1] == ':' && isASCIILetter(p[0]) {
		rest := strings.TrimPrefix(p[2:], "/")
		drive := strings.ToLower(p[:1])
		if rest == "" {
			return "/" + drive
		}
		return "/" + drive + "/" + rest
	}
	return p
}

func isASCIILetter(c byte) bool {
	return ('a' <= c && c <= 'z') || ('A' <= c && c <= 'Z')
}

// userlandHeader is the line printed under the run header so the count can
// be interpreted: which root, which binary, how many applets.
func userlandHeader(rootDir, userlandBin string, names int) string {
	var b bytes.Buffer
	fmt.Fprintf(&b, "Fixture root: %s (BASHY_ROOT; userland %s, %d applets under usr/bin, sh = the testee)", rootDir, filepath.Base(userlandBin), names)
	return b.String()
}

// warnUserlandMissing explains an unset -userland on Windows: the run still
// measures, but against whatever BASH53_TOOLS_PATH names.
func warnUserlandMissing(w io.Writer, tools string) {
	if strings.TrimSpace(tools) == "" {
		fmt.Fprintln(w, "bash53-suite: note: no -userland and no BASH53_TOOLS_PATH; fixtures resolve cat/sed/awk/diff from the harness's own PATH")
		return
	}
	fmt.Fprintf(w, "bash53-suite: note: no -userland; fixtures resolve cat/sed/awk/diff from BASH53_TOOLS_PATH=%s and no /bin, /usr or /etc root exists for them\n", tools)
}
