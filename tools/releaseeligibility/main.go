// Command releaseeligibility rejects diagnostic Bashy build profiles before
// an artifact is promoted to a shipped path. Signal structure is audited
// separately by tools/elfaudit.
package main

import (
	"debug/buildinfo"
	"fmt"
	"os"
	"strings"
	"unicode"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: releaseeligibility PATH")
		os.Exit(2)
	}
	if err := audit(os.Args[1]); err != nil {
		fmt.Fprintf(os.Stderr, "releaseeligibility: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("releaseeligibility: PASS %s\n", os.Args[1])
}

func audit(path string) error {
	build, err := buildinfo.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read Go build information from %s: %w", path, err)
	}
	if build.Path != "github.com/qiangli/bashy/cmd/bashy" {
		return fmt.Errorf("%s contains %q, want cmd/bashy", path, build.Path)
	}
	for _, setting := range build.Settings {
		if setting.Key != "-tags" {
			continue
		}
		for _, tag := range strings.FieldsFunc(setting.Value, func(r rune) bool { return r == ',' || unicode.IsSpace(r) }) {
			if tag == "bashy_core" {
				return fmt.Errorf("%s uses diagnostic bashy_core tag; do not ship until optional command parity and route gates pass", path)
			}
		}
	}
	return nil
}
