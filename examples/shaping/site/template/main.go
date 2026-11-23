// site: deterministic server and renderer for an <owner>/site content module.
//
// Each page under content/*.md carries front matter (title, path) and sections
// with explicit {#id} identifiers (never derived from headings). One renderer
// produces an HTML fragment (and full HTML document), Markdown with front
// matter, and JSON under one content SHA-256 revision.
//
// It is also the server of a shaped app (shaped.json): the owner shapes copy
// with `bashy shape`, and frozen checks under checks/ verify render
// determinism, unique ids, internal link resolution, and stale-base wishes.
package main

import (
	"embed"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
)

//go:embed site.json content
var assets embed.FS

func main() {
	addr := flag.String("addr", "127.0.0.1:8090", "listen address")
	_ = flag.String("data", "", "shaped-app data directory (unused; content is embedded)")
	origin := flag.String("origin", "", "override origin from site.json")
	dir := flag.String("dir", "", "load site from directory instead of embedded assets")
	renderPath := flag.String("render", "", "render a single page path (e.g. / or /about) and exit")
	format := flag.String("format", "html", "output format for -render: html, html-doc, md, json")
	checkName := flag.String("check", "", "run verification check (all, render-determinism, unique-ids, links-resolve, stale-base) and exit")
	baseDir := flag.String("base-dir", "", "base directory for stale-base check (defaults to -dir)")
	pageHint := flag.String("page", "", "wish page context for stale-base check")
	element := flag.String("element", "", "wish element context (e.g. 'section=team revision=...') for stale-base check")
	wishFile := flag.String("wish-file", "", "optional .wish or wish.json file for stale-base check")
	commitMsgFile := flag.String("commit-msg-file", "", "optional git commit message file for stale-base check")
	asJSON := flag.Bool("json", false, "emit JSON output for -render or -check")
	flag.Parse()

	if *checkName != "" {
		targetDir := *dir
		if targetDir == "" {
			targetDir = "."
		}
		bDir := *baseDir
		if bDir == "" {
			bDir = targetDir
		}
		pHint, elem, err := resolveWishInput(*pageHint, *element, *wishFile, *commitMsgFile)
		if err != nil {
			writeCheckError(*asJSON, targetDir, *checkName, err)
			os.Exit(1)
		}
		outcomes, err := RunChecks(targetDir, bDir, *checkName, pHint, elem)
		if *asJSON {
			env := map[string]any{
				"ok":     err == nil,
				"dir":    targetDir,
				"checks": outcomes,
			}
			if err != nil {
				env["error"] = err.Error()
			}
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			_ = enc.Encode(env)
		} else {
			for _, o := range outcomes {
				if o.Detail != "" {
					fmt.Fprintf(os.Stdout, "%s: %s (%s)\n", o.Name, o.Status, o.Detail)
				} else {
					fmt.Fprintf(os.Stdout, "%s: %s\n", o.Name, o.Status)
				}
			}
			if err != nil {
				fmt.Fprintln(os.Stderr, err.Error())
			}
		}
		if err != nil {
			os.Exit(1)
		}
		return
	}

	if *renderPath != "" {
		targetDir := *dir
		if targetDir == "" {
			targetDir = "."
		}
		site, err := LoadSiteDir(targetDir)
		if err != nil {
			log.Fatal(err)
		}
		if *origin != "" {
			site.SetOrigin(*origin)
		}
		if err := ValidateSite(site); err != nil {
			log.Fatal(err)
		}
		p, ok := site.ByPath[*renderPath]
		if !ok {
			log.Fatalf("site: unknown page path %q", *renderPath)
		}
		fmtName := *format
		if *asJSON && fmtName == "html" && !flagWasPassed("format") {
			fmtName = "json"
		}
		switch fmtName {
		case "json":
			b, err := p.RenderJSON()
			if err != nil {
				log.Fatal(err)
			}
			_, _ = os.Stdout.Write(b)
		case "md", "markdown":
			out := p.RenderMarkdown()
			if *asJSON {
				writeRenderEnvelope(p, "md", out)
				return
			}
			_, _ = os.Stdout.WriteString(out)
		case "html":
			out := p.RenderHTMLFragment()
			if *asJSON {
				writeRenderEnvelope(p, "html", out)
				return
			}
			_, _ = os.Stdout.WriteString(out)
		case "html-doc":
			out := site.RenderHTMLDocument(p)
			if *asJSON {
				writeRenderEnvelope(p, "html-doc", out)
				return
			}
			_, _ = os.Stdout.WriteString(out)
		default:
			log.Fatalf("site: unsupported format %q (want html, md, or json)", fmtName)
		}
		return
	}

	var (
		s   *server
		err error
	)
	if *dir != "" {
		s, err = newServerFrom(os.DirFS(*dir), os.Getenv)
	} else {
		s, err = newServer(os.Getenv)
	}
	if err != nil {
		log.Fatal(err)
	}
	if *origin != "" {
		s.site.SetOrigin(*origin)
	}
	log.Printf("site: serving %s (%d pages) on %s", s.site.Config.Title, len(s.site.Pages), *addr)
	log.Fatal(http.ListenAndServe(*addr, s))
}

func flagWasPassed(name string) bool {
	found := false
	flag.Visit(func(f *flag.Flag) {
		if f.Name == name {
			found = true
		}
	})
	return found
}

func writeRenderEnvelope(p *Page, format, content string) {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(map[string]any{
		"path":     p.Path,
		"title":    p.Title,
		"revision": p.Revision,
		"format":   format,
		"content":  content,
	})
}

func writeCheckError(asJSON bool, dir, checkName string, err error) {
	if asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(map[string]any{
			"ok":     false,
			"dir":    dir,
			"checks": []CheckOutcome{{Name: checkName, Status: "fail", Detail: err.Error()}},
			"error":  err.Error(),
		})
		return
	}
	fmt.Fprintln(os.Stderr, err.Error())
}
