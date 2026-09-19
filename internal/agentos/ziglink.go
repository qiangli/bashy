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
	argv := []string{"cc"}
	unwind := false
	for _, a := range args[1:] {
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
		if ee, ok := err.(*exec.ExitError); ok {
			return ee.ExitCode()
		}
		fmt.Fprintln(os.Stderr, "bashy zig-link:", err)
		return 1
	}
	return 0
}
