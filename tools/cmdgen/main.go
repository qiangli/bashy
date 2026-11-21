// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

// Command cmdgen emits one tiny standalone main package per bashy command
// catalog entry. Parallel P4 workers invoke it over disjoint verb lists:
//
//	go run ./tools/cmdgen -catalog internal/agentos/cmdcatalog/entries \
//	    -verbs-file batch-p4a.txt -gomod go.mod -out /tmp/generated-cmds
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/qiangli/bashy/internal/cmdgen"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "cmdgen:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	fs := flag.NewFlagSet("cmdgen", flag.ContinueOnError)
	catalogPath := fs.String("catalog", "", "path to catalog YAML file or directory of per-verb YAML files (required)")
	verbsFlag := fs.String("verbs", "", "comma-separated subset of verb names to generate")
	verbsFile := fs.String("verbs-file", "", "file containing verb names (one per line, # comments allowed)")
	gomodPath := fs.String("gomod", "", "optional go.mod path to resolve pinned module versions and replaces")
	outDir := fs.String("out", "", "output directory (creates <out>/<verb>/{main.go,go.mod})")
	stdout := fs.Bool("stdout", false, "print generated main.go to stdout (requires a single selected verb)")
	build := fs.Bool("build", false, "also build selected verbs into the tree-hash cache and activate their shape slots")
	gowork := fs.String("gowork", "", "optional go.work path used when -build is set")
	home := fs.String("home", "", "optional BASHY_HOME override used when -build is set")
	asJSON := fs.Bool("json", false, "emit machine-readable JSON summary")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*catalogPath) == "" {
		return fmt.Errorf("-catalog is required")
	}

	entries, err := cmdgen.LoadCatalog(*catalogPath)
	if err != nil {
		return err
	}

	var verbs []string
	if *verbsFlag != "" {
		verbs = append(verbs, cmdgen.ParseVerbList(*verbsFlag)...)
	}
	if *verbsFile != "" {
		fromFile, err := cmdgen.ReadVerbListFile(*verbsFile)
		if err != nil {
			return err
		}
		verbs = append(verbs, fromFile...)
	}
	selected, err := cmdgen.SelectVerbs(entries, verbs)
	if err != nil {
		return err
	}
	if len(selected) == 0 {
		return fmt.Errorf("no catalog entries selected")
	}

	var modInfo cmdgen.GoModInfo
	if *gomodPath != "" {
		modInfo, err = cmdgen.ParseGoModFile(*gomodPath)
		if err != nil {
			return err
		}
	}

	if *stdout {
		if len(selected) != 1 {
			return fmt.Errorf("-stdout requires exactly 1 selected verb (got %d)", len(selected))
		}
		src, err := cmdgen.GenerateMain(selected[0])
		if err != nil {
			return err
		}
		_, err = os.Stdout.Write(src)
		return err
	}

	type row struct {
		Name      string `json:"name"`
		Package   string `json:"package"`
		Module    string `json:"module"`
		Version   string `json:"version,omitempty"`
		Dir       string `json:"dir,omitempty"`
		SpecHash  string `json:"spec_hash,omitempty"`
		TreeHash  string `json:"tree_hash,omitempty"`
		SlotPath  string `json:"slot_path,omitempty"`
		CacheHit  bool   `json:"cache_hit,omitempty"`
		BuildMS   int64  `json:"build_ms,omitempty"`
	}

	var rows []row
	if *outDir != "" {
		gen, err := cmdgen.GenerateAll(selected, *outDir, cmdgen.GenerateOptions{GoMod: modInfo})
		if err != nil {
			return err
		}
		for _, g := range gen {
			rows = append(rows, row{
				Name:     g.Entry.Name,
				Package:  g.Entry.Package,
				Module:   g.Entry.Module,
				Version:  g.Entry.Version,
				Dir:      g.Dir,
				SpecHash: g.SpecHash,
			})
		}
	}

	if *build {
		bopts := cmdgen.BuildOptions{
			Home:   *home,
			GoWork: *gowork,
			GoMod:  modInfo,
		}
		for i, e := range selected {
			res, err := cmdgen.BuildAndActivateSlot(context.Background(), e, bopts)
			if err != nil {
				return err
			}
			if i < len(rows) {
				rows[i].TreeHash = res.TreeHash
				rows[i].SlotPath = res.Version.Path
				rows[i].CacheHit = res.CacheHit
				rows[i].BuildMS = res.Duration.Milliseconds()
			} else {
				resolved := e.ResolveVersion(modInfo)
				rows = append(rows, row{
					Name:     resolved.Name,
					Package:  resolved.Package,
					Module:   resolved.Module,
					Version:  resolved.Version,
					TreeHash: res.TreeHash,
					SlotPath: res.Version.Path,
					CacheHit: res.CacheHit,
					BuildMS:  res.Duration.Milliseconds(),
				})
			}
		}
	}

	if *outDir == "" && !*build {
		return fmt.Errorf("specify -out, -stdout, or -build")
	}

	if *asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(map[string]any{
			"schema_version": "bashy-cmdgen-v1",
			"count":          len(rows),
			"commands":       rows,
		})
	}
	for _, r := range rows {
		if r.SlotPath != "" {
			fmt.Printf("%s\t%s\tcache_hit=%t\t%dms\t%s\n", r.Name, r.TreeHash[:12], r.CacheHit, r.BuildMS, r.SlotPath)
		} else {
			fmt.Printf("%s\t%s\t%s\n", r.Name, r.SpecHash[:12], r.Dir)
		}
	}
	return nil
}
