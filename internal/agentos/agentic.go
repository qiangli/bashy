package agentos

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"slices"

	"mvdan.cc/sh/v3/interp"

	"github.com/qiangli/coreutils/pkg/bscript"
	"github.com/qiangli/coreutils/pkg/recall"
	"github.com/qiangli/coreutils/pkg/weavecli"
	"github.com/qiangli/coreutils/tool"
)

// dispatchAgentic runs exactly one action. Native tools and scripts re-enter
// bashy so they retain the existing userland/middleware chain; external
// programs are direct children with inherited streams and no post-processing.
func dispatchAgentic(args []string) int {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		fmt.Fprintln(os.Stdout, "usage: bashy agentic [--] ACTION [ARG...]")
		fmt.Fprintln(os.Stdout, "Run one action with BASHY_AGENTIC=1. If a native command or script cannot proceed, return one input_required skill proposal; never retry or persist it.")
		if len(args) == 0 {
			return 2
		}
		return 0
	}
	if args[0] == "--" {
		args = args[1:]
	}
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "bashy agentic: ACTION is required")
		return 2
	}

	cmd, kind, returnYield := agenticCommand(args)
	cmd.Env = runCommandEnv(os.Environ())
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Start(); err != nil {
		return emitAgenticYield(kind, args)
	}
	_ = cmd.Wait()
	status, _ := procStatus(cmd.ProcessState)
	if status != 0 && returnYield {
		return emitAgenticYield(kind, args)
	}
	return status
}

// agenticCommand returns the execution command, yield kind, and whether a
// non-zero result belongs to Bashy's native/script path. Externals deliberately
// return their exact status and never pass through output middleware.
func agenticCommand(args []string) (*exec.Cmd, bscript.Kind, bool) {
	name := args[0]
	if isScriptPath(name) {
		return exec.Command(bashySelfPath(), args...), bscript.Script, true
	}
	if tool.Lookup(name) != nil || interp.IsBuiltin(name) {
		cmdArgs := append([]string{"-c", `command "$@"`, "bashy agentic"}, args...)
		return exec.Command(bashySelfPath(), cmdArgs...), bscript.Command, true
	}
	if name != "agentic" && (slices.Contains(alwaysShimVerbs, name) || slices.Contains(directFrontDoorVerbs, name) || slices.Contains(hiddenFrontDoorVerbs, name)) {
		return exec.Command(bashySelfPath(), args...), bscript.Command, true
	}
	return exec.Command(name, args[1:]...), bscript.Command, false
}

func isScriptPath(path string) bool {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return false
	}
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	var head [2]byte
	n, _ := f.Read(head[:])
	return n == len(head) && bytes.Equal(head[:], []byte("#!"))
}

func emitAgenticYield(kind bscript.Kind, args []string) int {
	req := bscript.Request{Kind: kind, Readers: recall.ContextReaders()}
	if kind == bscript.Script {
		req.Args = append([]string(nil), args...)
	} else {
		req.Args = append([]string(nil), args...)
	}
	y, err := bscript.Lower(req)
	if err != nil {
		fmt.Fprintln(os.Stderr, "bashy agentic:", err)
		return 1
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(y); err != nil {
		fmt.Fprintln(os.Stderr, "bashy agentic:", err)
		return 1
	}
	return weavecli.ExitInputRequired
}
