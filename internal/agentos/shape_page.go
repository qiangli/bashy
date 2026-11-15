package agentos

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// Live page actions are transient browser state, distinct from source revisions.
// Submission is never retried: inspect result ID after an ambiguous disconnect.
func addShapePageCmds(parent *cobra.Command, o *shapeCLIOptions) {
	page := &cobra.Command{Use: "page", Short: "Control a connected operator page through named actions (not source revisions)", Long: `Control a connected operator page. Results are JSON. list exposes typed action descriptions.
invoke returns a command ID, not proof that the browser completed the action; inspect result.
A successful shape.submit means the source candidate was accepted, never approved.
Examples:
  bashy shape page list console
  bashy shape page invoke console PAGE shape.draft '{"request":"Make the header compact"}'
  bashy shape page invoke console PAGE shape.submit
  bashy shape page result console COMMAND
  bashy shape page invoke console PAGE presentation.shape-only '{"enabled":true}'
  bashy shape page invoke console PAGE presentation.shape-only '{"enabled":false}'`}
	for _, spec := range []struct {
		verb, use string
		min, max  int
	}{{"pages", "list APP", 1, 1}, {"invoke", "invoke APP PAGE ACTION [JSON]", 3, 4}, {"result", "result APP COMMAND", 2, 2}} {
		spec := spec
		page.AddCommand(&cobra.Command{Use: spec.use, Args: cobra.RangeArgs(spec.min, spec.max), RunE: func(c *cobra.Command, args []string) error {
			if o.User != "" || o.Actor != "" {
				return fmt.Errorf("page: principal comes from the authenticated console session")
			}
			target := o.Console
			if o.Remote != "" {
				target = o.Remote
			}
			u, err := shapeConsoleURL(target)
			if err != nil {
				return err
			}
			ip := net.ParseIP(u.Hostname())
			local := u.Hostname() == "localhost" || (ip != nil && ip.IsLoopback())
			if !local && (o.Remote == "" || u.Scheme != "https") {
				return fmt.Errorf("page: non-loopback requires --remote HTTPS and --cookie-file")
			}
			if o.Remote != "" && o.CookieFile == "" {
				return fmt.Errorf("page: --remote requires --cookie-file")
			}
			if args[0] == "" || strings.ContainsAny(args[0], "/\\?#") {
				return fmt.Errorf("page: invalid app name")
			}
			u.Path = strings.TrimRight(u.Path, "/") + "/api/page/" + args[0] + "/" + spec.verb
			var body io.Reader
			method := http.MethodGet
			if spec.verb == "invoke" {
				input := json.RawMessage(`{}`)
				if len(args) == 4 {
					input = json.RawMessage(args[3])
				}
				if !json.Valid(input) {
					return fmt.Errorf("page: input must be JSON")
				}
				payload, _ := json.Marshal(map[string]any{"page_id": args[1], "action": args[2], "input": input})
				body = bytes.NewReader(payload)
				method = http.MethodPost
			} else if spec.verb == "result" {
				q := u.Query()
				q.Set("id", args[1])
				u.RawQuery = q.Encode()
			}
			req, err := http.NewRequestWithContext(c.Context(), method, u.String(), body)
			if err != nil {
				return err
			}
			req.Header.Set("Accept", "application/json")
			if body != nil {
				req.Header.Set("Content-Type", "application/json")
			}
			if o.CookieFile != "" {
				b, err := os.ReadFile(o.CookieFile)
				if err != nil {
					return err
				}
				value := strings.TrimSpace(string(b))
				cookie := &http.Cookie{Name: "bashy_console", Value: value}
				if value == "" || strings.ContainsAny(value, "\r\n;") || cookie.Valid() != nil {
					return fmt.Errorf("page: invalid session cookie")
				}
				req.AddCookie(cookie)
			}
			client := &http.Client{Timeout: 35 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
			resp, err := client.Do(req)
			if err != nil {
				return fmt.Errorf("page: request failed; do not automatically retry invoke: %w", err)
			}
			defer resp.Body.Close()
			if resp.StatusCode < 200 || resp.StatusCode >= 300 {
				return fmt.Errorf("page: operation refused (HTTP %d)", resp.StatusCode)
			}
			var result any
			if err := json.NewDecoder(io.LimitReader(resp.Body, 2<<20)).Decode(&result); err != nil {
				return fmt.Errorf("page: invalid result: %w", err)
			}
			return json.NewEncoder(c.OutOrStdout()).Encode(result)
		}})
	}
	parent.AddCommand(page)
}
