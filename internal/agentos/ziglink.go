package agentos

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/qiangli/yoke/external/zigcc"
)

// windowsGnuLinkDrop lists what rustc's x86_64-pc-windows-gnu link line
// names and zig's own mingw-w64 libc already provides (or ignores): passing
// them makes zig search for a dynamic 'msvcrt' it will never find. The list
// is cargo-zigbuild's.
var windowsGnuLinkDrop = map[string]bool{
	"-lgcc_eh": true, "-lgcc_s": true, "-lgcc": true,
	"-lmsvcrt": true, "-lmingwex": true, "-lmingw32": true,
	"-l:libpthread.a": true, "-l:libunwind.a": true,
	"-fno-use-linker-plugin": true,
	"-Wl,--dynamicbase":      true, "-Wl,--disable-auto-image-base": true,
}

// provisionedLinker is the "cc-linker" row: one program rustc's -C linker=
// can name. Off Windows it is yoke's shell wrapper over `zig cc`; on
// Windows a .cmd that re-enters bashy's `zig-link` verb, which filters the
// gnu link line before handing it to zig cc.
func provisionedLinker(ctx context.Context) ([]string, string, error) {
	if runtime.GOOS != "windows" {
		wrapper, err := zigcc.Linker(ctx)
		return single(wrapper, "selected provisioned zig cc as the linker", err)
	}
	zig, err := zigcc.Ensure(ctx)
	if err != nil {
		return nil, "", err
	}
	wrapper := filepath.Join(filepath.Dir(zig), "zig-link.cmd")
	body := "@\"" + bashySelfPath() + "\" zig-link \"" + zig + "\" %*\r\n"
	if data, err := os.ReadFile(wrapper); err != nil || string(data) != body {
		if err := os.WriteFile(wrapper, []byte(body), 0o755); err != nil {
			return nil, "", fmt.Errorf("zig-link: write wrapper: %w", err)
		}
	}
	return []string{wrapper}, "selected provisioned zig cc as the linker (windows-gnu)", nil
}

// dispatchZigLink is `bashy zig-link ZIG ARGS...`: zig cc with the mingw
// libraries zig supplies itself removed from the line. Exit status is zig's.
func dispatchZigLink(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "bashy zig-link: usage: zig-link ZIG LINKER-ARGS...")
		return 2
	}
	zig := args[0]
	linkArgs, cleanup, err := normalizeWindowsGnuResponseArgs(args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, "bashy zig-link:", err)
		return 1
	}
	defer cleanup()
	argv := []string{"cc"}
	unwind := false
	for _, a := range normalizeWindowsGnuDefArgs(linkArgs) {
		a = strings.TrimSpace(a)
		if windowsGnuLinkDrop[a] {
			// The panic unwinder's _Unwind_* symbols come from libgcc_eh on
			// mingw; zig bundles libunwind instead.
			unwind = unwind || strings.Contains(a, "gcc_eh") || strings.Contains(a, "unwind")
			continue
		}
		argv = append(argv, a)
	}
	if unwind {
		argv = append(argv, "-lunwind")
	}
	cmd := exec.Command(zig, argv...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "bashy zig-link: raw argv=%q normalized argv=%q\n", args[1:], argv[1:])
		for _, a := range argv[1:] {
			if path, ok := rustcExportListPath(a); ok {
				fmt.Fprintf(os.Stderr, "bashy zig-link: retained rustc export list as positional input %q\n", path)
				break
			}
		}
		if ee, ok := err.(*exec.ExitError); ok {
			return ee.ExitCode()
		}
		fmt.Fprintln(os.Stderr, "bashy zig-link:", err)
		return 1
	}
	return 0
}

// normalizeWindowsGnuResponseArgs adapts rustc's overflow @file without
// expanding or re-escaping its other arguments. Rust 1.98 writes one
// POSIX-escaped argument per UTF-8 line for windows-gnu; removing only the
// -Wl, wrapper leaves the same escaped path as a positional .def input.
func normalizeWindowsGnuResponseArgs(args []string) ([]string, func(), error) {
	out := append([]string(nil), args...)
	var temps []string
	cleanup := func() {
		for _, name := range temps {
			_ = os.Remove(name)
		}
	}
	for i, arg := range out {
		candidate := strings.Trim(strings.TrimSpace(arg), `"`)
		if !strings.HasPrefix(candidate, "@") {
			continue
		}
		source := strings.Trim(strings.TrimSpace(candidate[1:]), `"`)
		data, err := os.ReadFile(source)
		if err != nil {
			cleanup()
			return nil, func() {}, fmt.Errorf("read response file: %w", err)
		}
		normalized, changed := normalizeWindowsGnuResponse(data)
		if !changed {
			continue
		}
		file, err := os.CreateTemp(filepath.Dir(source), "bashy-linker-*.rsp")
		if err != nil {
			cleanup()
			return nil, func() {}, fmt.Errorf("create response file: %w", err)
		}
		name := file.Name()
		temps = append(temps, name)
		if _, err := file.Write(normalized); err != nil {
			_ = file.Close()
			cleanup()
			return nil, func() {}, fmt.Errorf("write response file: %w", err)
		}
		if err := file.Close(); err != nil {
			cleanup()
			return nil, func() {}, fmt.Errorf("close response file: %w", err)
		}
		out[i] = "@" + name
	}
	return out, cleanup, nil
}

func normalizeWindowsGnuResponse(data []byte) ([]byte, bool) {
	lines := strings.Split(string(data), "\n")
	out := make([]string, 0, len(lines))
	changed := false
	unwind := false
	for i := 0; i < len(lines); i++ {
		raw := lines[i]
		body := strings.TrimSuffix(raw, "\r")
		cr := raw[len(body):]
		candidate := strings.Trim(strings.TrimSpace(body), `"`)
		if windowsGnuLinkDrop[candidate] {
			unwind = unwind || strings.Contains(candidate, "gcc_eh") || strings.Contains(candidate, "unwind")
			changed = true
			continue
		}
		if strings.HasPrefix(candidate, "-Wl,") {
			path := candidate[len("-Wl,"):]
			if _, ok := rustcExportListPath(path); ok {
				out = append(out, path+cr)
				changed = true
				continue
			}
		}
		if candidate == "-Xlinker" && i+1 < len(lines) {
			next := lines[i+1]
			nextBody := strings.TrimSuffix(next, "\r")
			if _, ok := rustcExportListPath(nextBody); ok {
				out = append(out, next)
				i++
				changed = true
				continue
			}
		}
		out = append(out, raw)
	}
	if unwind {
		if len(out) > 0 && out[len(out)-1] == "" {
			out = append(out[:len(out)-1], "-lunwind", "")
		} else {
			out = append(out, "-lunwind")
		}
	}
	return []byte(strings.Join(out, "\n")), changed
}

// normalizeWindowsGnuDefArgs presents rustc's temporary export list as an
// input file. Zig accepts .def inputs but rejects rustc's GNU -Wl spelling.
// Everything else is kept byte-for-byte so this adapter cannot silently
// reinterpret unrelated linker flags.
func normalizeWindowsGnuDefArgs(args []string) []string {
	out := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		a := args[i]
		candidate := strings.Trim(strings.TrimSpace(a), `"`)
		if strings.HasPrefix(candidate, "-Wl,") {
			if path, ok := rustcExportListPath(candidate[len("-Wl,"):]); ok {
				out = append(out, path)
				continue
			}
		}
		if candidate == "-Xlinker" && i+1 < len(args) {
			if path, ok := rustcExportListPath(args[i+1]); ok {
				out = append(out, path)
				i++
				continue
			}
		}
		out = append(out, a)
	}
	return out
}

func rustcExportListPath(arg string) (string, bool) {
	path := strings.Trim(strings.TrimSpace(arg), `"`)
	// The .cmd re-entry boundary can preserve Windows separators. Zig accepts
	// native paths, but its cc input classifier is deterministic with slashes.
	path = strings.ReplaceAll(path, `\`, "/")
	base := path
	if i := strings.LastIndexByte(base, '/'); i >= 0 {
		base = base[i+1:]
	}
	return path, strings.EqualFold(base, "list.def")
}
