package agentos

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	yokemcp "github.com/qiangli/yoke/mcp"
	"github.com/qiangli/yoke/pkg/coopauth"
	"github.com/qiangli/yoke/pkg/fleet"
	"github.com/qiangli/yoke/pkg/webconsole"
)

// siteMCPTools are the tool names of the `site` profile, in the order the
// site template's /.well-known/mcp/server-card.json lists them.
var siteMCPTools = []string{"list_pages", "read_page", "cite",
	"shape_wish", "shape_verify", "shape_preview", "shape_approve", "shape_undo"}

// Caller tiers, the same names the outpost relationship stamp carries.
const (
	siteTierOwner     = "owner"
	siteTierMember    = "member"
	siteTierAccount   = "account"
	siteTierAnonymous = "anonymous"
)

// siteMCP is the `bashy mcp serve --tools site` profile: the site's pages
// read through the site's own renderer (its /llms.txt index and JSON/Markdown
// twins, so revisions and section hashes are the ones the pages carry), and
// the shape verbs through the existing shape service, each gated by the
// caller tier the pages' front stamped on the request.
type siteMCP struct {
	app     string // shaped app and outpost app name (--site-app)
	dir     string // site module directory (--site-dir)
	outpost bool   // behind the outpost app proxy: callers come from its stamp

	ctx    context.Context
	client *http.Client

	once    sync.Once
	base    string
	baseErr error
}

type siteCaller struct{ Principal, Tier string }

func newSiteMCP(ctx context.Context, app, dir string, outpost bool) *siteMCP {
	return &siteMCP{app: app, dir: dir, outpost: outpost, ctx: ctx, client: &http.Client{Timeout: 30 * time.Second}}
}

// caller resolves the principal from the request the front proxied. Without
// --outpost the server is loopback-only and its caller is the local owner.
// Behind --outpost only a signature from this app's key counts: an unsigned
// or foreign-signed request is anonymous, a signed caller without a verified
// relationship stamp is an account holder.
func (s *siteMCP) caller(header http.Header) siteCaller {
	if !s.outpost {
		principal := strings.TrimSpace(os.Getenv("BASHY_PRINCIPAL"))
		if principal == "" {
			principal = coopauth.SystemUser()
		}
		if principal == "" {
			principal = "local owner"
		}
		return siteCaller{Principal: principal, Tier: siteTierOwner}
	}
	anonymous := siteCaller{Principal: "anonymous", Tier: siteTierAnonymous}
	if header == nil || s.app == "" {
		return anonymous
	}
	prefix := strings.TrimRight(header.Get(coopauth.HdrForwardedPrefix), "/")
	if !strings.HasSuffix(prefix, "/app/"+s.app) {
		return anonymous
	}
	config, ok := readMCPOutpostConfig()
	if !ok {
		return anonymous
	}
	r := &http.Request{Header: header}
	user := strings.TrimSpace(header.Get(coopauth.HdrRemoteUser))
	for _, key := range config.signingKeys(s.app) {
		if !coopauth.VerifyOutpost(r, []byte(key)) {
			continue
		}
		if rel, ok := coopauth.VerifyRelationship(r, []byte(key)); ok {
			principal := user
			if principal == "" && rel.Visitor != "" {
				principal = "visitor:" + rel.Visitor
			}
			if principal == "" {
				principal = "anonymous"
			}
			tier := rel.Kind
			if tier == "signed-in" {
				tier = siteTierAccount
			}
			return siteCaller{Principal: principal, Tier: tier}
		}
		if user != "" {
			return siteCaller{Principal: user, Tier: siteTierAccount}
		}
		return anonymous
	}
	return anonymous
}

// requireLogin is the site's read tier: the outpost app's require_login.
// A site served without outpost is read by its local owner only.
func (s *siteMCP) requireLogin() bool {
	if !s.outpost {
		return false
	}
	config, ok := readMCPOutpostConfig()
	if !ok {
		return true
	}
	entry, ok := config.app(s.app)
	return !ok || entry.RequireLogin
}

func siteRefusal(c siteCaller, tool, rule string) error {
	return fmt.Errorf("refused: caller %s (tier %s) is not authorized for %s; %s", c.Principal, c.Tier, tool, rule)
}

func (s *siteMCP) allowRead(c siteCaller, tool string) error {
	if !s.requireLogin() {
		return nil
	}
	if c.Tier == siteTierOwner || c.Tier == siteTierMember {
		return nil
	}
	return siteRefusal(c, tool, "member or owner tier required (the site requires login)")
}

func (s *siteMCP) allowShape(c siteCaller, tool string) error {
	if c.Tier != siteTierOwner {
		return siteRefusal(c, tool, "owner tier required")
	}
	if s.app == "" {
		return fmt.Errorf("%s: no shaped app; serve with --site-app APP", tool)
	}
	return nil
}

func requestHeader(req *mcpsdk.CallToolRequest) http.Header {
	if req == nil || req.Extra == nil {
		return nil
	}
	return req.Extra.Header
}

// siteBase is the loopback base URL of the site's renderer: the upstream the
// outpost app proxies the pages to, or the site module's own server started
// from its directory (the --site-dir, or the shaped app's source).
func (s *siteMCP) siteBase() (string, error) {
	s.once.Do(func() {
		if s.app != "" && s.dir == "" {
			if config, ok := readMCPOutpostConfig(); ok {
				if entry, ok := config.app(s.app); ok && entry.Port > 0 {
					scheme, host := entry.Scheme, entry.Host
					if scheme == "" {
						scheme = "http"
					}
					if host == "" {
						host = "127.0.0.1"
					}
					s.base = scheme + "://" + net.JoinHostPort(host, fmt.Sprint(entry.Port)) + strings.TrimRight(entry.BasePath, "/")
					return
				}
			}
			a, ok := fleet.New().App(s.app)
			if !ok || a.Source == "" {
				s.baseErr = fmt.Errorf("site: app %q has neither an outpost upstream nor a source directory", s.app)
				return
			}
			s.dir = a.Source
		}
		if s.dir == "" {
			s.baseErr = errors.New("site: serve with --site-dir DIR or --site-app APP")
			return
		}
		s.base, s.baseErr = startSiteServer(s.ctx, s.dir)
	})
	return s.base, s.baseErr
}

// startSiteServer builds the site module's cmd/server and runs it on a free
// loopback port for the life of ctx, reading the live directory (-dir).
func startSiteServer(ctx context.Context, dir string) (string, error) {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(filepath.Join(dir, "site.json")); err != nil {
		return "", fmt.Errorf("site: %s is not a site module (no site.json)", dir)
	}
	bin, err := os.MkdirTemp("", "bashy-mcp-site-")
	if err != nil {
		return "", err
	}
	go func() { <-ctx.Done(); _ = os.RemoveAll(bin) }()
	exe := filepath.Join(bin, "site-server")
	build := exec.CommandContext(ctx, "go", "build", "-o", exe, "./cmd/server")
	build.Dir = dir
	build.Env = append(os.Environ(), "GOWORK=off")
	if out, err := build.CombinedOutput(); err != nil {
		return "", fmt.Errorf("site: building %s/cmd/server: %v\n%s", dir, err, out)
	}
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", err
	}
	addr := l.Addr().String()
	_ = l.Close()
	server := exec.CommandContext(ctx, exe, "-addr", addr, "-dir", dir)
	server.Dir = dir
	server.Stderr = io.Discard
	if err := server.Start(); err != nil {
		return "", err
	}
	go func() { _ = server.Wait() }()
	base := "http://" + addr
	client := &http.Client{Timeout: 2 * time.Second}
	for deadline := time.Now().Add(30 * time.Second); time.Now().Before(deadline); time.Sleep(100 * time.Millisecond) {
		if resp, err := client.Get(base + "/llms.txt"); err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return base, nil
			}
		}
	}
	return "", fmt.Errorf("site: server for %s did not answer on %s", dir, addr)
}

func (s *siteMCP) get(path, accept string) ([]byte, string, error) {
	base, err := s.siteBase()
	if err != nil {
		return nil, "", err
	}
	req, err := http.NewRequestWithContext(s.ctx, http.MethodGet, base+path, nil)
	if err != nil {
		return nil, "", err
	}
	if accept != "" {
		req.Header.Set("Accept", accept)
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, "", err
	}
	if resp.StatusCode == http.StatusNotFound {
		return nil, "", fmt.Errorf("site: no page at %s", path)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("site: GET %s: %s", path, resp.Status)
	}
	return body, resp.Header.Get("Content-Type"), nil
}

// sitePageJSON is the page's JSON twin, as the site template renders it.
type sitePageJSON struct {
	Path     string `json:"path"`
	Title    string `json:"title"`
	Summary  string `json:"summary,omitempty"`
	Revision string `json:"revision"`
	Sections []struct {
		ID       string `json:"id"`
		Heading  string `json:"heading"`
		Markdown string `json:"markdown"`
		SHA256   string `json:"sha256"`
	} `json:"sections"`
}

// siteTwinPaths mirrors the template's TwinPaths: / is /index.{md,json}.
func siteTwinPaths(path string) (string, string) {
	stem := strings.TrimRight(path, "/")
	if stem == "" {
		stem = "/index"
	}
	return stem + ".md", stem + ".json"
}

// sitePagePath reduces a page path, a twin path or a page URL to the page's
// canonical path.
func sitePagePath(ref string) (string, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "", errors.New("site: empty page reference")
	}
	if strings.Contains(ref, "://") {
		u, err := url.Parse(ref)
		if err != nil {
			return "", fmt.Errorf("site: invalid page URL %q", ref)
		}
		ref = u.Path
	}
	if !strings.HasPrefix(ref, "/") {
		ref = "/" + ref
	}
	for _, ext := range []string{".md", ".json"} {
		if strings.HasSuffix(ref, ext) {
			ref = strings.TrimSuffix(ref, ext)
			if ref == "/index" {
				ref = "/"
			}
			break
		}
	}
	return ref, nil
}

func (s *siteMCP) page(path string) (sitePageJSON, error) {
	var page sitePageJSON
	_, jsonTwin := siteTwinPaths(path)
	body, _, err := s.get(jsonTwin, "application/json")
	if err != nil {
		return page, err
	}
	if err := json.Unmarshal(body, &page); err != nil {
		return page, fmt.Errorf("site: %s: %v", jsonTwin, err)
	}
	return page, nil
}

// llms.txt lines: - [Title](ORIGIN/path): markdown ORIGIN/x.md · json ORIGIN/x.json · revision R
var siteLLMsLine = regexp.MustCompile(`^- \[[^\]]*\]\(([^)]*)\): markdown (\S+) · json (\S+) · revision (\S+)`)

type siteIndexEntry struct{ URL, Markdown, JSON string }

func (s *siteMCP) index() ([]siteIndexEntry, error) {
	body, _, err := s.get("/llms.txt", "text/plain")
	if err != nil {
		return nil, err
	}
	var out []siteIndexEntry
	for _, line := range strings.Split(string(body), "\n") {
		if m := siteLLMsLine.FindStringSubmatch(line); m != nil {
			out = append(out, siteIndexEntry{URL: m[1], Markdown: m[2], JSON: m[3]})
		}
	}
	if len(out) == 0 {
		return nil, errors.New("site: /llms.txt lists no pages")
	}
	return out, nil
}

// pageURL is the canonical URL the site's own index gives the page.
func (s *siteMCP) pageURL(path string) string {
	entries, err := s.index()
	if err == nil {
		for _, e := range entries {
			if p, err := sitePagePath(e.URL); err == nil && p == path {
				return e.URL
			}
		}
	}
	return path
}

type siteTwins struct {
	Markdown string `json:"markdown"`
	JSON     string `json:"json"`
}
type sitePageEntry struct {
	Path     string    `json:"path"`
	Title    string    `json:"title"`
	Summary  string    `json:"summary"`
	Revision string    `json:"revision"`
	Twins    siteTwins `json:"twins"`
}
type siteListPagesInput struct{}
type siteListPagesOutput struct {
	Pages []sitePageEntry `json:"pages"`
}

func (s *siteMCP) listPages(_ context.Context, req *mcpsdk.CallToolRequest, _ siteListPagesInput) (*mcpsdk.CallToolResult, siteListPagesOutput, error) {
	var out siteListPagesOutput
	if err := s.allowRead(s.caller(requestHeader(req)), "list_pages"); err != nil {
		return nil, out, err
	}
	entries, err := s.index()
	if err != nil {
		return nil, out, err
	}
	out.Pages = []sitePageEntry{}
	for _, e := range entries {
		path, err := sitePagePath(e.URL)
		if err != nil {
			return nil, out, err
		}
		page, err := s.page(path)
		if err != nil {
			return nil, out, err
		}
		out.Pages = append(out.Pages, sitePageEntry{Path: page.Path, Title: page.Title, Summary: page.Summary,
			Revision: page.Revision, Twins: siteTwins{Markdown: e.Markdown, JSON: e.JSON}})
	}
	return nil, out, nil
}

type siteReadPageInput struct {
	PathOrURL string `json:"path_or_url" jsonschema:"page path (/about), twin path (/about.md) or page URL"`
	Format    string `json:"format,omitempty" jsonschema:"md (default), html or json"`
}
type siteReadPageOutput struct {
	Path        string `json:"path"`
	Title       string `json:"title"`
	Revision    string `json:"revision"`
	ContentType string `json:"content_type"`
	Content     string `json:"content"`
}

func (s *siteMCP) readPage(_ context.Context, req *mcpsdk.CallToolRequest, in siteReadPageInput) (*mcpsdk.CallToolResult, siteReadPageOutput, error) {
	var out siteReadPageOutput
	if err := s.allowRead(s.caller(requestHeader(req)), "read_page"); err != nil {
		return nil, out, err
	}
	path, err := sitePagePath(in.PathOrURL)
	if err != nil {
		return nil, out, err
	}
	page, err := s.page(path)
	if err != nil {
		return nil, out, err
	}
	mdTwin, jsonTwin := siteTwinPaths(path)
	var target, accept string
	switch in.Format {
	case "", "md", "markdown":
		target, accept = mdTwin, "text/markdown"
	case "json":
		target, accept = jsonTwin, "application/json"
	case "html":
		target, accept = path, "text/html"
	default:
		return nil, out, fmt.Errorf("read_page: format %q: want md, html or json", in.Format)
	}
	body, contentType, err := s.get(target, accept)
	if err != nil {
		return nil, out, err
	}
	return nil, siteReadPageOutput{Path: page.Path, Title: page.Title, Revision: page.Revision,
		ContentType: contentType, Content: string(body)}, nil
}

type siteCiteInput struct {
	Ref string `json:"ref" jsonschema:"<page url or path>#<section id>"`
}
type siteCiteOutput struct {
	URL           string `json:"url"`
	SectionID     string `json:"section_id"`
	Revision      string `json:"revision"`
	SectionSHA256 string `json:"section_sha256"`
	Quote         string `json:"quote"`
}

// siteSectionSHA256 is the template's ComputeSectionHash.
func siteSectionSHA256(id, heading, markdown string) string {
	sum := sha256.Sum256([]byte("site-section-v1\nid:" + id + "\nheading:" + heading + "\n" + markdown))
	return hex.EncodeToString(sum[:])
}

func (s *siteMCP) cite(_ context.Context, req *mcpsdk.CallToolRequest, in siteCiteInput) (*mcpsdk.CallToolResult, siteCiteOutput, error) {
	var out siteCiteOutput
	if err := s.allowRead(s.caller(requestHeader(req)), "cite"); err != nil {
		return nil, out, err
	}
	ref, id, ok := strings.Cut(in.Ref, "#")
	if !ok || id == "" {
		return nil, out, fmt.Errorf("cite: ref %q needs <url_or_path>#<section_id>", in.Ref)
	}
	if ref == "" {
		ref = "/"
	}
	path, err := sitePagePath(ref)
	if err != nil {
		return nil, out, err
	}
	page, err := s.page(path)
	if err != nil {
		return nil, out, err
	}
	for _, section := range page.Sections {
		if section.ID != id {
			continue
		}
		sum := siteSectionSHA256(section.ID, section.Heading, section.Markdown)
		if section.SHA256 != "" && section.SHA256 != sum {
			return nil, out, fmt.Errorf("cite: section %s of %s: served sha256 %s does not match its content (%s)", id, path, section.SHA256, sum)
		}
		return nil, siteCiteOutput{URL: s.pageURL(path) + "#" + id, SectionID: id, Revision: page.Revision,
			SectionSHA256: sum, Quote: section.Markdown}, nil
	}
	return nil, out, fmt.Errorf("cite: %s has no section %q at revision %s", path, id, page.Revision)
}

type siteShapeWishInput struct {
	Request string `json:"request" jsonschema:"what to change, in words"`
	Page    string `json:"page,omitempty" jsonschema:"page path the wish is about"`
	Element string `json:"element,omitempty" jsonschema:"element context, e.g. 'section=ID revision=REV' (compare-and-set)"`
	Agent   string `json:"agent,omitempty" jsonschema:"shaping agent as tool:model (bashy shape wish --agent)"`
}
type siteShapeIDInput struct {
	ID string `json:"id" jsonschema:"candidate id"`
}
type siteShapeUndoInput struct{}
type siteShapeOutput struct {
	App       string `json:"app"`
	Verb      string `json:"verb"`
	Principal string `json:"principal"`
	Result    any    `json:"result"`
}

var siteShapeExecute = func(ctx context.Context, r shapeCLIRequest) (any, error) { return shapeCLIExecute(ctx, r) }

func (s *siteMCP) shape(ctx context.Context, req *mcpsdk.CallToolRequest, tool string, r shapeCLIRequest) (*mcpsdk.CallToolResult, siteShapeOutput, error) {
	var out siteShapeOutput
	c := s.caller(requestHeader(req))
	if err := s.allowShape(c, tool); err != nil {
		return nil, out, err
	}
	actor, err := shapeAuthoredActor("")
	if err != nil {
		return nil, out, err
	}
	r.App, r.User, r.Actor = s.app, c.Principal, actor
	// The local console the CLI's --console defaults to: preview names the
	// candidate's isolated preview there.
	r.Console = fmt.Sprintf("http://127.0.0.1:%d", webconsole.DefaultPort)
	result, err := siteShapeExecute(ctx, r)
	if err != nil {
		return nil, out, err
	}
	data, err := json.Marshal(result)
	if err != nil {
		return nil, out, err
	}
	var view any
	if err := json.Unmarshal(data, &view); err != nil {
		return nil, out, err
	}
	return nil, siteShapeOutput{App: s.app, Verb: r.Verb, Principal: c.Principal, Result: view}, nil
}

// newSiteMCPServer builds the site profile's server: exactly the eight site
// tools, no shell sessions, no registry commands, every call audited.
func newSiteMCPServer(s *siteMCP, version string, audit func(yokemcp.Record)) *mcpsdk.Server {
	srv := mcpsdk.NewServer(&mcpsdk.Implementation{Name: "bashy-site", Version: version}, nil)
	readOnly := &mcpsdk.ToolAnnotations{ReadOnlyHint: true}
	mcpsdk.AddTool(srv, &mcpsdk.Tool{Name: "list_pages", Annotations: readOnly,
		Description: "List the site's pages: path, title, summary, revision and the Markdown/JSON twins."}, s.listPages)
	mcpsdk.AddTool(srv, &mcpsdk.Tool{Name: "read_page", Annotations: readOnly,
		Description: "Read one page as Markdown (default), HTML or JSON at its current revision."}, s.readPage)
	mcpsdk.AddTool(srv, &mcpsdk.Tool{Name: "cite", Annotations: readOnly,
		Description: "Return the citation record {url, section_id, revision, section_sha256, quote} for <url_or_path>#<section_id>."}, s.cite)
	mcpsdk.AddTool(srv, &mcpsdk.Tool{Name: "shape_wish",
		Description: "Owner only: ask the shape service for a candidate change to the site."},
		func(ctx context.Context, req *mcpsdk.CallToolRequest, in siteShapeWishInput) (*mcpsdk.CallToolResult, siteShapeOutput, error) {
			if strings.TrimSpace(in.Request) == "" {
				return nil, siteShapeOutput{}, errors.New("shape_wish: request is required")
			}
			return s.shape(ctx, req, "shape_wish", shapeCLIRequest{Verb: "wish", Request: in.Request, Page: in.Page, Element: in.Element, Agent: in.Agent})
		})
	for _, verb := range []struct{ tool, verb, doc string }{
		{"shape_verify", "verify", "Owner only: run the trusted verifier on a candidate."},
		{"shape_preview", "preview", "Owner only: return the isolated preview URL of a verified candidate."},
		{"shape_approve", "approve", "Owner only: verify again and activate a candidate."},
	} {
		verb := verb
		mcpsdk.AddTool(srv, &mcpsdk.Tool{Name: verb.tool, Description: verb.doc},
			func(ctx context.Context, req *mcpsdk.CallToolRequest, in siteShapeIDInput) (*mcpsdk.CallToolResult, siteShapeOutput, error) {
				if strings.TrimSpace(in.ID) == "" {
					return nil, siteShapeOutput{}, fmt.Errorf("%s: id is required", verb.tool)
				}
				return s.shape(ctx, req, verb.tool, shapeCLIRequest{Verb: verb.verb, ID: in.ID})
			})
	}
	mcpsdk.AddTool(srv, &mcpsdk.Tool{Name: "shape_undo",
		Description: "Owner only: restore the previous version through the trusted gate."},
		func(ctx context.Context, req *mcpsdk.CallToolRequest, _ siteShapeUndoInput) (*mcpsdk.CallToolResult, siteShapeOutput, error) {
			return s.shape(ctx, req, "shape_undo", shapeCLIRequest{Verb: "undo"})
		})
	if audit != nil {
		srv.AddReceivingMiddleware(siteAuditMiddleware(audit))
	}
	return srv
}

// siteAuditMiddleware keeps every site call in the same hash-chained audit as
// the other MCP profiles; a tier refusal is recorded as a denial.
func siteAuditMiddleware(audit func(yokemcp.Record)) mcpsdk.Middleware {
	return func(next mcpsdk.MethodHandler) mcpsdk.MethodHandler {
		return func(ctx context.Context, method string, req mcpsdk.Request) (mcpsdk.Result, error) {
			if method != "tools/call" {
				return next(ctx, method, req)
			}
			start := time.Now()
			call := req.(*mcpsdk.CallToolRequest)
			effects := []string{"pure"}
			if strings.HasPrefix(call.Params.Name, "shape_") {
				effects = []string{"write"}
			}
			record := yokemcp.Record{Time: start.UTC().Format(time.RFC3339Nano), Tool: call.Params.Name,
				Command: call.Params.Name, Effects: effects, Allowed: true}
			result, err := next(ctx, method, req)
			record.Duration = time.Since(start)
			if res, ok := result.(*mcpsdk.CallToolResult); ok && res != nil && res.IsError {
				record.ExitCode = 1
				for _, content := range res.Content {
					if text, ok := content.(*mcpsdk.TextContent); ok && strings.HasPrefix(text.Text, "refused: ") {
						record.Allowed, record.Denial, record.ExitCode = false, text.Text, 126
					}
				}
			}
			if err != nil {
				record.ExitCode = 1
			}
			audit(record)
			return result, err
		}
	}
}

// siteToolDefinitions lists the site profile's tools without serving it.
func siteToolDefinitions() ([]*mcpsdk.Tool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	srv := newSiteMCPServer(newSiteMCP(ctx, "", "", false), "help", nil)
	st, ct := mcpsdk.NewInMemoryTransports()
	ss, err := srv.Connect(ctx, st, nil)
	if err != nil {
		return nil, err
	}
	defer ss.Close()
	cs, err := mcpsdk.NewClient(&mcpsdk.Implementation{Name: "bashy-help", Version: "1"}, nil).Connect(ctx, ct, nil)
	if err != nil {
		return nil, err
	}
	defer cs.Close()
	page, err := cs.ListTools(ctx, &mcpsdk.ListToolsParams{})
	if err != nil {
		return nil, err
	}
	return page.Tools, nil
}

// runSiteMCP serves the site profile over stdio, or over the same loopback
// Streamable HTTP transport as the other profiles. Behind --outpost the
// transport admits every loopback request: the tier of each call is decided
// per tool from the stamp, so an anonymous caller still reaches the read
// tools of a public site and is refused the rest by name.
func runSiteMCP(ctx context.Context, transport, listen, app, dir, version string, outpost bool) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	srv := newSiteMCPServer(newSiteMCP(ctx, app, dir, outpost), version, mcpAudit())
	if transport != "http" {
		err := srv.Run(ctx, &mcpsdk.StdioTransport{})
		if errors.Is(err, io.EOF) || errors.Is(err, context.Canceled) || (err != nil && strings.Contains(err.Error(), "server is closing")) {
			return nil
		}
		return err
	}
	addr, shutdown, err := yokemcp.ServeHTTPServerWithAuth(ctx, srv, listen, func(r *http.Request) bool {
		return coopauth.IsLoopbackAddr(r.RemoteAddr)
	})
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "listening on http://%s/mcp\n", addr)
	<-ctx.Done()
	return shutdown()
}

// dispatchSiteMCPTools answers `bashy mcp tools --tools site [--json]`.
func dispatchSiteMCPTools(asJSON bool) int {
	env := mcpToolsEnvelope{SchemaVersion: mcpToolsSchemaVersion, Profile: "site", Transports: []string{"stdio", "http"},
		Tools: append([]string{}, siteMCPTools...), Registered: []string{}}
	if !asJSON {
		for _, name := range env.Tools {
			fmt.Println(name)
		}
		return 0
	}
	var err error
	if env.Definitions, err = siteToolDefinitions(); err != nil {
		fmt.Fprintln(os.Stderr, "bashy mcp:", err)
		return 1
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(env); err != nil {
		fmt.Fprintln(os.Stderr, "bashy mcp:", err)
		return 1
	}
	return 0
}
