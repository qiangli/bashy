package agentos

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/qiangli/bash/expand"
	"github.com/qiangli/bash/interp"
	"github.com/qiangli/bash/syntax"
	"github.com/spf13/cobra"

	"github.com/qiangli/yoke/cmds/fetch/agentwire"
	"github.com/qiangli/yoke/pkg/dag"
	"github.com/qiangli/yoke/pkg/policy/advice"
)

func siteRecipe(cmd *cobra.Command, client *http.Client, page *url.URL, asJSON, run bool) error {
	if run {
		client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 || !sameSiteOrigin(page, req.URL) {
				return fmt.Errorf("site recipe: refused redirect outside the page origin")
			}
			return nil
		}
	}
	response, err := siteGet(cmd.Context(), client, page, "text/html")
	if err != nil {
		return err
	}
	_, recipe := agentwire.HTMLAgent(response.body)
	if strings.TrimSpace(recipe) == "" {
		return fmt.Errorf("site recipe: no text/bashsharp recipe found")
	}
	if !asJSON {
		fmt.Fprintln(cmd.OutOrStdout(), strings.TrimRight(recipe, "\n"))
		if run {
			_, err := runSiteRecipe(cmd.Context(), client, response.url, recipe, cmd.OutOrStdout(), cmd.ErrOrStderr())
			return err
		}
		return nil
	}
	var output bytes.Buffer
	result := map[string]any{"url": page.String(), "recipe": recipe, "ran": false}
	if run {
		var ran bool
		ran, err = runSiteRecipe(cmd.Context(), client, response.url, recipe, &output, cmd.ErrOrStderr())
		result["ran"] = ran
		if err != nil {
			result["error"] = err.Error()
		}
		result["output"] = output.String()
	}
	if encodeErr := siteJSON(cmd, result); encodeErr != nil {
		return encodeErr
	}
	return err
}

func sameSiteOrigin(a, b *url.URL) bool {
	port := func(u *url.URL) string {
		if p := u.Port(); p != "" {
			return p
		}
		if u.Scheme == "https" {
			return "443"
		}
		return "80"
	}
	return b.User == nil && a.Scheme == b.Scheme && strings.EqualFold(a.Hostname(), b.Hostname()) && port(a) == port(b)
}

func recipeLiteral(word *syntax.Word) (string, bool) {
	// Keep the parser and interpreter in agreement by accepting a single
	// literal or single-quoted word, with no shell expansion or globbing.
	if len(word.Parts) != 1 {
		return "", false
	}
	switch part := word.Parts[0].(type) {
	case *syntax.Lit:
		return part.Value, !strings.ContainsAny(part.Value, "\\*?[~")
	case *syntax.SglQuoted:
		return part.Value, !part.Dollar
	case *syntax.DblQuoted:
		if part.Dollar || len(part.Parts) != 1 {
			return "", false
		}
		lit, ok := part.Parts[0].(*syntax.Lit)
		if ok && !strings.ContainsRune(lit.Value, '\\') {
			return lit.Value, true
		}
	}
	return "", false
}

// A cap on dispatched commands alone cannot guard shell builtins or
// expansions. Validate the entire program before starting it, and keep the
// only external command behind an in-process GET-only handler.
func validateSiteRecipe(prog *syntax.File, page *url.URL) (map[string]*url.URL, error) {
	allowed := map[string]*url.URL{}
	if len(prog.Stmts) > 16 {
		return nil, fmt.Errorf("site recipe: too many statements")
	}
	for _, stmt := range prog.Stmts {
		call, ok := stmt.Cmd.(*syntax.CallExpr)
		if !ok || len(call.Assigns) != 0 || len(call.Args) == 0 || len(stmt.Redirs) != 0 || stmt.Background || stmt.Coprocess || stmt.Disown || stmt.Negated {
			return nil, fmt.Errorf("site recipe: only foreground literal read/network commands are permitted")
		}
		argv := make([]string, 0, len(call.Args))
		for _, word := range call.Args {
			value, ok := recipeLiteral(word)
			if !ok {
				return nil, fmt.Errorf("site recipe: shell expansion is outside the recipe cap")
			}
			argv = append(argv, value)
		}
		switch {
		case argv[0] == "echo":
			continue
		case argv[0] == "printf" && len(argv) >= 2 && slices.Contains([]string{"%s", "%s\\n"}, argv[1]):
			continue
		case argv[0] == "fetch":
			argv = argv[1:]
		case len(argv) >= 2 && argv[0] == "bashy" && argv[1] == "fetch":
			argv = argv[2:]
		default:
			return nil, fmt.Errorf("site recipe: command %q is outside the recipe cap", argv[0])
		}
		if len(argv) != 1 || strings.HasPrefix(argv[0], "-") {
			return nil, fmt.Errorf("site recipe: fetch requires exactly one literal URL")
		}
		target, err := page.Parse(argv[0])
		if err != nil || !sameSiteOrigin(page, target) {
			return nil, fmt.Errorf("site recipe: fetch target %q is outside the page origin", argv[0])
		}
		target.Fragment = ""
		allowed[argv[0]] = target
	}
	return allowed, nil
}

func runSiteRecipe(ctx context.Context, client *http.Client, page *url.URL, recipe string, out, stderr io.Writer) (bool, error) {
	if len(recipe) > 32<<10 {
		return false, fmt.Errorf("site recipe: recipe exceeds the size limit")
	}
	prog, err := syntax.NewParser(syntax.Variant(syntax.LangBashPP)).Parse(strings.NewReader(recipe), "site recipe")
	if err != nil {
		return false, err
	}
	allowed, err := validateSiteRecipe(prog, page)
	if err != nil {
		return false, err
	}
	network := *client
	network.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) < 10 && sameSiteOrigin(page, req.URL) {
			for _, target := range allowed {
				if target.String() == req.URL.String() {
					return nil
				}
			}
		}
		return fmt.Errorf("site recipe: redirect target is outside the recipe's literal URLs")
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	cap, err := advice.ParseCap("read,net")
	if err != nil {
		return false, err
	}
	handler := func(ctx context.Context, args []string) error {
		if len(args) == 3 && args[0] == "bashy" && args[1] == "fetch" {
			args = args[1:]
		}
		if len(args) != 2 || args[0] != "fetch" || allowed[args[1]] == nil {
			return fmt.Errorf("site recipe: refused command outside the read/network sandbox")
		}
		response, err := siteGet(ctx, &network, allowed[args[1]], "*/*")
		if err != nil {
			return err
		}
		_, err = io.WriteString(interp.HandlerCtx(ctx).Stdout, response.body)
		return err
	}
	runner, err := interp.New(
		interp.Lang(syntax.LangBashPP),
		interp.Env(expand.ListEnviron("PATH=")),
		interp.StdIO(nil, out, stderr),
		interp.ExecHandlers(dag.CapExecHandler(), func(interp.ExecHandlerFunc) interp.ExecHandlerFunc { return handler }),
	)
	if err != nil {
		return false, err
	}
	return true, runner.Run(advice.WithCap(ctx, cap), prog)
}
