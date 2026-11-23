package agentos

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/qiangli/yoke/pkg/kb"
)

const (
	fixtureAgentBlock = "## For agents {#agents}\n\nRead these site instructions.\n"
	fixtureSiteSkill  = "---\nname: fixture-site\ndescription: Read the fixture site\nmetadata:\n  requires: \"has=go\"\n---\n\nRead the public pages.\n"
	fixtureSection    = "The citation text."
)

func siteCLI(t *testing.T, args ...string) (string, error) {
	t.Helper()
	root := newSiteCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(io.Discard)
	root.SetArgs(args)
	err := root.Execute()
	return out.String(), err
}

func TestSiteVerbsFixture(t *testing.T) {
	var mu sync.Mutex
	var requests []string
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requests = append(requests, r.URL.Path)
		mu.Unlock()
		switch r.URL.Path {
		case "/agents.txt":
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			fmt.Fprintf(w, "# contract comment\nsite: /\ntitle: Fixture\nrevision: rev-1\nindex: /llms.txt\nmarkdown: /guide.md\njson: /guide.json\nsource: https://example.org/demo.git\nmcp: /mcp\nskill: /skill.md\nignored: value\n\n%s", fixtureAgentBlock)
		case "/guide":
			w.Header().Set("Bashy-Shaped", `module="fixture", revision="header revision", agents.txt="/agents.txt", skill="/skill.md"`)
			w.Header().Add("Link", `</guide.md>; rel="alternate"; type="text/markdown", </guide.json>; rel="alternate"; type="application/json"`)
			w.Header().Add("Link", `<https://example.org/edit>; rel="edit", </AGENTS.md>; rel="service-doc"`)
			switch r.Header.Get("Accept") {
			case "text/html":
				fmt.Fprint(w, `<script id="agent-recipe" type="text/bashsharp">echo ready
bashy fetch '/payload'
</script>`)
			default:
				w.Header().Set("Content-Type", "text/markdown")
				fmt.Fprintf(w, "---\nagent:\n  instructions: Markdown instructions\n  agents_txt: /agents.txt\n  skills: %s\n---\n%s", siteSkillIndex, fixtureAgentBlock)
			}
		case "/guide.json":
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w, `{"revision":"rev-1","agents":{"instructions":"Read these site instructions."},"sections":[{"id":"citation","sha256":"%x","markdown":%q}]}`, sha256.Sum256([]byte(fixtureSection)), fixtureSection)
		case siteSkillIndex:
			fmt.Fprintf(w, `{"version":"0.2.0","skills":[{"name":"fixture-site","type":"skill-md","url":"/skill.md","sha256":"%x"}]}`, sha256.Sum256([]byte(fixtureSiteSkill)))
		case "/skill.md":
			fmt.Fprint(w, fixtureSiteSkill)
		case "/payload":
			fmt.Fprint(w, "payload")
		default:
			http.NotFound(w, r)
		}
	})
	srv := httptest.NewServer(h)
	defer srv.Close()
	page := srv.URL + "/guide"

	out, err := siteCLI(t, "explore", "--json", page)
	var result siteDiscovery
	if err != nil || json.Unmarshal([]byte(out), &result) != nil {
		t.Fatalf("explore: %s %v", out, err)
	}
	if result.AgentBlock != fixtureAgentBlock || result.AgentsTxt["revision"] != "rev-1" || result.AgentsTxt["ignored"] != "" ||
		result.Agent["instructions"] != "Markdown instructions" || result.BashyShaped["revision"] != "header revision" || len(result.Links) != 4 {
		t.Fatalf("explore: %+v", result)
	}
	mu.Lock()
	first := requests[0]
	mu.Unlock()
	if first != "/agents.txt" {
		t.Fatalf("first discovery request was %s", first)
	}
	out, err = siteCLI(t, "explore", "--dry-run", page)
	if err != nil || !strings.HasPrefix(out, fixtureAgentBlock) || !strings.Contains(out, "mcp: "+srv.URL+"/mcp") {
		t.Fatalf("explore text: %q %v", out, err)
	}

	out, err = siteCLI(t, "cite", "--json", page+"#citation")
	var citation siteCitation
	if err != nil || json.Unmarshal([]byte(out), &citation) != nil || citation.SectionID != "citation" ||
		citation.Quote != fixtureSection || citation.SectionSHA256 != fmt.Sprintf("%x", sha256.Sum256([]byte(fixtureSection))) {
		t.Fatalf("cite: %s %v", out, err)
	}
	out, err = siteCLI(t, "shape", "--dry-run", page)
	if err != nil || !strings.Contains(out, "https://example.org/demo.git") || !strings.Contains(out, "bashy shape undo") {
		t.Fatalf("shape preview: %s %v", out, err)
	}

	skillStore := t.TempDir()
	t.Setenv("BASHY_SKILLS_DIR", skillStore)
	t.Setenv("BASHY_KB_DIR", t.TempDir())
	out, err = siteCLI(t, "attach", "--dry-run", page)
	if err != nil || !strings.Contains(out, siteSkillIndex) {
		t.Fatalf("attach preview: %s %v", out, err)
	}
	if entries, err := os.ReadDir(skillStore); err != nil || len(entries) != 0 {
		t.Fatalf("attach preview modified skill store: %v %v", entries, err)
	}
	out, err = siteCLI(t, "attach", "--json", page)
	if err != nil || !strings.Contains(out, "fixture-site") {
		t.Fatalf("attach: %s %v", out, err)
	}
	if out, err := siteCLI(t, "attach", "--json", page); err != nil || !strings.Contains(out, "fixture-site") {
		t.Fatalf("repeat attach: %s %v", out, err)
	}
	if data, err := os.ReadFile(filepath.Join(skillStore, "fixture-site", "SKILL.md")); err != nil || string(data) != fixtureSiteSkill {
		t.Fatalf("installed skill: %q %v", data, err)
	}
	pages, err := kb.Open(kb.DefaultDir()).List()
	if err != nil || len(pages) != 1 || pages[0].Resource != page {
		t.Fatalf("KB: %v %v", pages, err)
	}

	for _, flags := range [][]string{nil, {"--dry-run", "--run"}, {"--run"}} {
		out, err = siteCLI(t, append([]string{"recipe", page}, flags...)...)
		if err != nil || !strings.HasPrefix(out, "echo ready\nbashy fetch") {
			t.Fatalf("recipe %v: %q %v", flags, out, err)
		}
		if len(flags) == 1 && !strings.HasSuffix(out, "ready\npayload") {
			t.Fatalf("recipe did not execute: %q", out)
		}
	}
	mu.Lock()
	defer mu.Unlock()
	hits := 0
	for _, request := range requests {
		if request == "/payload" {
			hits++
		}
	}
	if hits != 1 {
		t.Fatalf("expected exactly one explicit recipe run, saw %d payload requests", hits)
	}
}

func TestSiteVerbsFallbacks(t *testing.T) {
	for _, channel := range []string{"header", "link", "markdown", "/AGENTS.md", "/llms.txt", "plain"} {
		t.Run(channel, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/":
					switch channel {
					case "header":
						w.Header().Set("Bashy-Shaped", "module=fixture")
					case "link":
						w.Header().Set("Link", `</card.json>; rel="service-desc"`)
					case "markdown":
						fmt.Fprint(w, "---\nagent:\n  instructions: fixture\n---\n"+fixtureAgentBlock)
						return
					}
					fmt.Fprint(w, "<html><p>A plain page</p></html>")
				case channel:
					w.Header().Set("Content-Type", "text/plain")
					fmt.Fprint(w, "Site instructions")
				default:
					http.NotFound(w, r)
				}
			}))
			defer srv.Close()
			_, err := siteCLI(t, "explore", srv.URL)
			if channel == "plain" {
				if err == nil || !strings.Contains(err.Error(), "not an agent-ready site") {
					t.Fatalf("plain site: %v", err)
				}
			} else if err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestSiteVerbsPlainCatchAll(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "Ordinary text on every path")
	}))
	defer srv.Close()
	_, err := siteCLI(t, "explore", srv.URL)
	if err == nil || !strings.Contains(err.Error(), "not an agent-ready site") {
		t.Fatalf("plain catch-all site: %v", err)
	}
}

func TestSiteVerbsRecipeRefusals(t *testing.T) {
	foreign := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("foreign origin was contacted")
	}))
	defer foreign.Close()
	for _, body := range []string{
		"echo would-run\nmkdir /tmp/write",
		"echo would-run > /tmp/write",
		"echo \"$(echo hidden)\"",
		"fetch " + foreign.URL,
		"fetch /redirect-foreign",
		"fetch /redirect-unnamed",
	} {
		t.Run(body, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/":
					fmt.Fprintf(w, `<script id="agent-recipe" type="text/bashsharp">%s</script>`, body)
				case "/redirect-foreign":
					http.Redirect(w, r, foreign.URL, http.StatusFound)
				case "/redirect-unnamed":
					http.Redirect(w, r, "/unnamed", http.StatusFound)
				default:
					t.Error("unexpected recipe target", r.URL.Path)
					http.NotFound(w, r)
				}
			}))
			defer srv.Close()
			out, err := siteCLI(t, "recipe", "--run", srv.URL)
			if err == nil || out != body+"\n" {
				t.Fatalf("expected the recipe to be printed and refused, got %q %v", out, err)
			}
		})
	}
}
