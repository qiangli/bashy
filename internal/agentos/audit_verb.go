// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

package agentos

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/qiangli/coreutils/pkg/policy/audit"
)

// dispatchAudit implements `bashy audit {status,tail,verify,export,path}` — the
// read side of the compliance audit trail. It never writes the log (that is the
// ExecHandler middleware); it only reports it.
func dispatchAudit(args []string) int {
	sub := "status"
	if len(args) > 0 {
		sub = args[0]
		args = args[1:]
	}
	path := auditPath()
	switch sub {
	case "path":
		fmt.Println(path)
		return 0
	case "status":
		return auditStatus(path)
	case "tail":
		return auditTail(path, args)
	case "verify":
		return auditVerify(path)
	case "export":
		return auditExport(path)
	case "-h", "--help", "help":
		fmt.Println("usage: bashy audit {status|tail [N]|verify|export|path}")
		fmt.Println("  status   whether auditing is on, the log path, record count, chain state")
		fmt.Println("  tail [N] the last N records (default 20), one JSON object per line")
		fmt.Println("  verify   walk the hash chain and report the first break, if any")
		fmt.Println("  export   the full chain plus a verification summary (evidence bundle)")
		fmt.Println("  path     print the configured log path")
		fmt.Println("Enable auditing with BASHY_AUDIT=1 (default path) or BASHY_AUDIT=<file>.")
		return 0
	default:
		fmt.Fprintf(os.Stderr, "bashy audit: unknown subcommand %q (try: status tail verify export path)\n", sub)
		return 2
	}
}

func openAuditLog(path string) (*os.File, bool) {
	f, err := os.Open(path)
	if err != nil {
		return nil, false
	}
	return f, true
}

func auditStatus(path string) int {
	on := auditEnabled()
	fmt.Printf("auditing: %s\n", onOff(on))
	fmt.Printf("log:      %s\n", path)
	f, ok := openAuditLog(path)
	if !ok {
		fmt.Println("records:  0 (no log yet)")
		if !on {
			fmt.Println("\nenable with BASHY_AUDIT=1 (default path) or BASHY_AUDIT=<file>")
		}
		return 0
	}
	defer f.Close()
	res := audit.Verify(f)
	fmt.Printf("records:  %d\n", res.Records)
	if res.OK {
		fmt.Println("chain:    intact ✓")
		return 0
	}
	fmt.Printf("chain:    BROKEN at seq %d — %s\n", res.BadSeq, res.Reason)
	return 1
}

func auditTail(path string, args []string) int {
	n := 20
	if len(args) > 0 {
		if v, err := strconv.Atoi(args[0]); err == nil && v > 0 {
			n = v
		}
	}
	f, ok := openAuditLog(path)
	if !ok {
		fmt.Fprintf(os.Stderr, "bashy audit: no log at %s\n", path)
		return 1
	}
	defer f.Close()
	lines, err := lastLines(f, n)
	if err != nil {
		fmt.Fprintf(os.Stderr, "bashy audit: read error: %v\n", err)
		return 1
	}
	for _, ln := range lines {
		fmt.Println(ln)
	}
	return 0
}

func auditVerify(path string) int {
	f, ok := openAuditLog(path)
	if !ok {
		fmt.Fprintf(os.Stderr, "bashy audit: no log at %s\n", path)
		return 1
	}
	defer f.Close()
	res := audit.Verify(f)
	if res.OK {
		fmt.Printf("chain intact: %d records verified ✓\n", res.Records)
		return 0
	}
	fmt.Printf("CHAIN BROKEN at seq %d after %d records: %s\n", res.BadSeq, res.Records, res.Reason)
	return 1
}

// auditExport prints the full chain plus a machine-readable verification
// summary — the evidence bundle a reviewer or a SIEM ingests. Kept simple for
// this slice (JSONL body + a summary object); the control-mapped pack is later.
func auditExport(path string) int {
	f, ok := openAuditLog(path)
	if !ok {
		fmt.Fprintf(os.Stderr, "bashy audit: no log at %s\n", path)
		return 1
	}
	defer f.Close()
	// Verify first (separate read), then stream the body.
	res := audit.Verify(f)
	summary := map[string]any{
		"schema":   audit.SchemaVersion,
		"log":      path,
		"records":  res.Records,
		"chain_ok": res.OK,
		"host":     auditHost(),
	}
	if !res.OK {
		summary["broken_at_seq"] = res.BadSeq
		summary["reason"] = res.Reason
	}
	b, _ := json.Marshal(map[string]any{"audit_export": summary})
	fmt.Println(string(b))
	if _, err := f.Seek(0, io.SeekStart); err == nil {
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 0, 1<<20), 8<<20)
		for sc.Scan() {
			if strings.TrimSpace(sc.Text()) != "" {
				fmt.Println(sc.Text())
			}
		}
		if err := sc.Err(); err != nil {
			fmt.Fprintf(os.Stderr, "bashy audit: read error: %v\n", err)
			return 1
		}
	}
	if res.OK {
		return 0
	}
	return 1
}

// lastLines returns the last n non-empty lines of f in order.
func lastLines(f *os.File, n int) ([]string, error) {
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 1<<20), 8<<20)
	var all []string
	for sc.Scan() {
		if strings.TrimSpace(sc.Text()) != "" {
			all = append(all, sc.Text())
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if len(all) > n {
		all = all[len(all)-n:]
	}
	return all, nil
}

func onOff(b bool) string {
	if b {
		return "on"
	}
	return "off"
}
