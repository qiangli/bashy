// Command elfaudit verifies the structural contract of Bashy's Cloudbox ELF.
package main

import (
	"debug/buildinfo"
	"debug/elf"
	"fmt"
	"os"
	"strings"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: elfaudit PATH")
		os.Exit(2)
	}
	if err := audit(os.Args[1]); err != nil {
		fmt.Fprintf(os.Stderr, "elfaudit: %v\n", err)
		os.Exit(1)
	}
}

func audit(path string) error {
	build, err := buildinfo.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read Go build information from %s: %w", path, err)
	}
	settings := make(map[string]string, len(build.Settings))
	for _, setting := range build.Settings {
		settings[setting.Key] = setting.Value
	}
	for key, want := range map[string]string{
		"GOOS":        "linux",
		"GOARCH":      "amd64",
		"CGO_ENABLED": "0",
	} {
		if got := settings[key]; got != want {
			return fmt.Errorf("%s build setting is %q, want %q", key, got, want)
		}
	}
	tagged := false
	for _, tag := range strings.Split(settings["-tags"], ",") {
		if tag == "bashy_scratch" {
			tagged = true
			break
		}
	}
	if !tagged {
		return fmt.Errorf("build tags %q do not contain bashy_scratch", settings["-tags"])
	}

	f, err := elf.Open(path)
	if err != nil {
		return fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()

	if f.Class != elf.ELFCLASS64 || f.Data != elf.ELFDATA2LSB || f.Machine != elf.EM_X86_64 {
		return fmt.Errorf("%s is %s/%s machine %s, want 64-bit little-endian amd64 ELF", path, f.Class, f.Data, f.Machine)
	}
	if f.Type != elf.ET_EXEC {
		return fmt.Errorf("%s has ELF type %s, want executable", path, f.Type)
	}

	for _, prog := range f.Progs {
		if prog.Type == elf.PT_INTERP {
			return fmt.Errorf("%s has an ELF INTERP program header", path)
		}
	}

	if f.SectionByType(elf.SHT_DYNAMIC) != nil {
		needed, err := f.DynString(elf.DT_NEEDED)
		if err != nil {
			return fmt.Errorf("read dynamic NEEDED entries from %s: %w", path, err)
		}
		if len(needed) != 0 {
			return fmt.Errorf("%s has ELF NEEDED entries: %v", path, needed)
		}
	}

	fmt.Printf("elfaudit: PASS %s is static linux/amd64 ELF (no INTERP or NEEDED)\n", path)
	return nil
}
