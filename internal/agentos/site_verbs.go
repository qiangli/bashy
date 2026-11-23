package agentos

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/qiangli/yoke/cmds/fetch/agentwire"
)

const siteSkillIndex = "/.well-known/agent-skills/index.json"

var errSiteMissing = errors.New("not an agent-ready site")

type siteResponse struct {
	body   string
	header http.Header
	url    *url.URL
}

func siteURL(raw string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" || u.User != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return nil, errors.New("expected an HTTP(S) URL without credentials")
	}
	return u, nil
}

func siteGet(ctx context.Context, client *http.Client, target *url.URL, accept string) (siteResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
	if err != nil {
		return siteResponse{}, err
	}
	req.Header.Set("User-Agent", "bashy-site")
	req.Header.Set("Accept", accept)
	resp, err := client.Do(req)
	if err != nil {
		return siteResponse{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return siteResponse{header: resp.Header, url: resp.Request.URL}, fmt.Errorf("%s: HTTP %d", target, resp.StatusCode)
	}
	const limit = 4 << 20
	body, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err == nil && len(body) > limit {
		err = fmt.Errorf("%s: response exceeds the size limit", target)
	}
	return siteResponse{string(body), resp.Header, resp.Request.URL}, err
}

type siteDiscovery struct {
	URL         string                `json:"url"`
	AgentsTxt   map[string]string     `json:"agents_txt"`
	Agent       map[string]string     `json:"agent"`
	BashyShaped map[string]string     `json:"bashy_shaped"`
	Links       []agentwire.AgentLink `json:"links"`
	AgentBlock  string                `json:"agent_block"`
	fields      map[string]string
}

func parseAgentsTxt(body string) (map[string]string, string) {
	fields := map[string]string{}
	head, block, ok := strings.Cut(body, "\n\n")
	if crlfHead, crlfBlock, crlf := strings.Cut(body, "\r\n\r\n"); crlf && (!ok || len(crlfHead) < len(head)) {
		head, block = crlfHead, crlfBlock
	}
	for _, line := range strings.Split(head, "\n") {
		key, value, found := strings.Cut(strings.TrimSpace(line), ":")
		if !found {
			continue
		}
		switch key {
		case "site", "title", "module", "revision", "index", "agents_md", "markdown", "json", "skill", "skills_index", "mcp", "mcp_card", "source", "edit", "shape", "cite", "bashy":
			fields[key] = strings.TrimSpace(value)
		}
	}
	if !strings.HasPrefix(block, "## For agents {#agents}") {
		block = ""
	}
	return fields, block
}

func (s *siteDiscovery) fill(base *url.URL, key, value string) {
	if value == "" || s.fields[key] != "" {
		return
	}
	switch key {
	case "title", "module", "revision", "cite", "bashy":
		s.fields[key] = value
	default:
		if u, err := base.Parse(value); err == nil {
			if _, err := siteURL(u.String()); err == nil {
				s.fields[key] = u.String()
			}
		}
	}
}

// Page-local references override the home-page defaults in /agents.txt.
func (s *siteDiscovery) pageField(base *url.URL, key, value string) {
	previous := s.fields[key]
	delete(s.fields, key)
	s.fill(base, key, value)
	if s.fields[key] == "" && previous != "" {
		s.fields[key] = previous
	}
}

func (s *siteDiscovery) pageHeaders(page siteResponse) bool {
	shaped := strings.Join(page.header.Values("Bashy-Shaped"), ", ")
	s.BashyShaped = agentwire.ShapedFields(shaped)
	for _, key := range []string{"module", "skill", "mcp", "source", "shape"} {
		s.fill(page.url, key, s.BashyShaped[key])
	}
	s.fill(page.url, "agents_txt", s.BashyShaped["agents"])
	s.fill(page.url, "agents_txt", s.BashyShaped["agents.txt"])
	s.pageField(page.url, "revision", s.BashyShaped["revision"])
	announced := shaped != ""
	for _, link := range agentwire.AgentLinks(page.header) {
		s.Links = append(s.Links, link)
		announced = announced || link.AnnouncesAgent()
		for _, rel := range link.Rel {
			switch rel {
			case "alternate":
				switch link.Type {
				case "text/markdown":
					s.pageField(page.url, "markdown", link.URL)
				case "application/json":
					s.pageField(page.url, "json", link.URL)
				}
			case "service-doc":
				s.fill(page.url, "agents_md", link.URL)
			case "service-desc":
				s.fill(page.url, "mcp_card", link.URL)
			case "describedby":
				s.fill(page.url, "index", link.URL)
			case "via":
				s.fill(page.url, "source", link.URL)
			case "edit":
				s.pageField(page.url, "edit", link.URL)
			}
		}
	}
	return announced
}

func discoverSite(ctx context.Context, client *http.Client, pageURL *url.URL) (siteDiscovery, error) {
	s := siteDiscovery{
		URL: pageURL.String(), fields: map[string]string{}, AgentsTxt: map[string]string{},
		Agent: map[string]string{}, BashyShaped: map[string]string{}, Links: []agentwire.AgentLink{},
	}
	agentsURL := pageURL.ResolveReference(&url.URL{Path: "/agents.txt"})
	readAgents := func(target *url.URL) bool {
		response, err := siteGet(ctx, client, target, "text/plain")
		if err != nil {
			return false
		}
		s.AgentsTxt, s.AgentBlock = parseAgentsTxt(response.body)
		for key, value := range s.AgentsTxt {
			s.fill(response.url, key, value)
		}
		return s.AgentBlock != "" || len(s.AgentsTxt) > 0
	}
	foundAgents := readAgents(agentsURL)
	found := foundAgents
	page, err := siteGet(ctx, client, pageURL, "text/markdown, text/html;q=0.9, application/json;q=0.8")
	if page.url != nil {
		found = s.pageHeaders(page) || found
	}
	if alternate := s.fields["agents_txt"]; !foundAgents && alternate != "" && alternate != agentsURL.String() {
		target, _ := siteURL(alternate)
		found = readAgents(target) || found
	}
	if err == nil {
		block, agent := agentwire.MarkdownAgent(page.body)
		if strings.Contains(page.header.Get("Content-Type"), "json") {
			block, agent = agentwire.JSONAgent(page.body)
		}
		agentBase := page.url
		if len(agent) == 0 && s.fields["markdown"] != "" && s.fields["markdown"] != page.url.String() {
			twinURL, _ := siteURL(s.fields["markdown"])
			if twin, err := siteGet(ctx, client, twinURL, "text/markdown"); err == nil {
				block, agent = agentwire.MarkdownAgent(twin.body)
				agentBase = twin.url
			}
		}
		if block == "" {
			block = agentwire.ResponseAgent(page.body, page.header.Get("Content-Type"))
		}
		if len(agent) > 0 {
			s.Agent = agent
			for key, value := range agent {
				if key == "skills" {
					key = "skills_index"
				}
				if key == "edit" || key == "cite" {
					s.pageField(agentBase, key, value)
				} else {
					s.fill(agentBase, key, value)
				}
			}
		}
		if s.AgentBlock == "" {
			s.AgentBlock = block
		}
		found = found || block != "" || len(agent) > 0
	}
	if s.AgentBlock == "" {
		for _, channel := range []struct{ key, path string }{{"agents_md", "/AGENTS.md"}, {"index", "/llms.txt"}} {
			target := pageURL.ResolveReference(&url.URL{Path: channel.path})
			if advertised := s.fields[channel.key]; advertised != "" {
				target, _ = siteURL(advertised)
			}
			if fallback, err := siteGet(ctx, client, target, "text/markdown, text/plain"); err == nil && strings.TrimSpace(fallback.body) != strings.TrimSpace(page.body) && usableSiteFallback(fallback) {
				s.AgentBlock = fallback.body
				s.fill(pageURL, channel.key, fallback.url.String())
				found = true
				break
			}
		}
	}
	if !found {
		if ctx.Err() != nil {
			return s, ctx.Err()
		}
		return s, errSiteMissing
	}
	return s, nil
}

func usableSiteFallback(response siteResponse) bool {
	body := strings.TrimSpace(response.body)
	media := response.header.Get("Content-Type")
	return body != "" && !strings.HasPrefix(body, "<") &&
		(strings.HasPrefix(media, "text/plain") || strings.HasPrefix(media, "text/markdown"))
}

func newSiteCmd() *cobra.Command {
	root := &cobra.Command{
		Use: "site", Short: "Discover and use agent-ready websites",
		SilenceUsage: true, SilenceErrors: true,
	}
	for _, verb := range []struct{ name, summary string }{
		{"explore", "Discover a site's agent channels"},
		{"attach", "Admit a site skill and record the site in the knowledge base"},
		{"cite", "Read a section citation from the site's JSON twin"},
		{"shape", "Register the source repository as a shaped app"},
		{"recipe", "Print a page's Bash# recipe or explicitly run it under a read/network cap"},
	} {
		var asJSON, dryRun, run bool
		sub := &cobra.Command{
			Use: verb.name + " URL", Short: verb.summary, Args: cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				page, err := siteURL(args[0])
				if err != nil {
					return err
				}
				client := &http.Client{Timeout: 15 * time.Second}
				switch verb.name {
				case "explore":
					s, err := discoverSite(cmd.Context(), client, page)
					if err != nil {
						return err
					}
					if asJSON {
						return siteJSON(cmd, s)
					}
					if s.AgentBlock != "" {
						fmt.Fprint(cmd.OutOrStdout(), s.AgentBlock)
						if !strings.HasSuffix(s.AgentBlock, "\n") {
							fmt.Fprintln(cmd.OutOrStdout())
						}
					}
					for _, key := range []string{"index", "markdown", "json", "source", "edit", "mcp", "skill", "skills_index", "revision"} {
						if value := s.fields[key]; value != "" {
							fmt.Fprintf(cmd.OutOrStdout(), "%s: %s\n", key, value)
						}
					}
					return nil
				case "attach":
					return siteAttach(cmd, page, asJSON, dryRun)
				case "cite":
					return siteCite(cmd, client, page, asJSON)
				case "shape":
					return siteShape(cmd, client, page, asJSON, dryRun)
				default:
					return siteRecipe(cmd, client, page, asJSON, run && !dryRun)
				}
			},
		}
		sub.Flags().BoolVar(&asJSON, "json", false, "emit machine-readable output")
		sub.Flags().BoolVar(&dryRun, "dry-run", false, "inspect and preview without making changes or running a recipe")
		if verb.name == "recipe" {
			sub.Flags().BoolVar(&run, "run", false, "run the recipe after validating its effects and URLs")
		}
		root.AddCommand(sub)
	}
	return root
}

func siteJSON(cmd *cobra.Command, value any) error {
	return json.NewEncoder(cmd.OutOrStdout()).Encode(value)
}
