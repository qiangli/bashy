package agentos

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/qiangli/yoke/pkg/fleet"
	"github.com/qiangli/yoke/pkg/providerselector"
)

func runProviders(args []string, stdout, stderr io.Writer) int {
	selectMode := len(args) > 0 && args[0] == "select"
	if selectMode {
		args = args[1:]
	}
	if len(args) > 0 && (args[0] == "--help" || args[0] == "-h") {
		fmt.Fprintln(stdout, "Usage: bashy providers [ROLE] [--json] | providers select ROLE [--provider NAME] [--route oss|vendor|browser|api] [--yes] [--json] [REQUEST.json]")
		return 0
	}
	role := ""
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		role = args[0]
		args = args[1:]
	}
	if selectMode && role == "" {
		fmt.Fprintln(stderr, "providers select: ROLE required")
		return 2
	}
	fs := flag.NewFlagSet("providers", flag.ContinueOnError)
	fs.SetOutput(stderr)
	provider := fs.String("provider", "", "explicit provider name")
	route := fs.String("route", "", "oss, vendor, browser, or api")
	asJSON := fs.Bool("json", false, "emit JSON")
	yes := fs.Bool("yes", false, "allow spend/send effects")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() > 1 {
		fmt.Fprintln(stderr, "providers: expected at most one REQUEST.json")
		return 2
	}
	if *route != "" && *route != "oss" && *route != "vendor" && *route != "browser" && *route != "api" {
		fmt.Fprintln(stderr, "providers: invalid route")
		return 2
	}
	var request any = map[string]any{}
	if fs.NArg() == 1 {
		data, err := os.ReadFile(fs.Arg(0))
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 2
		}
		if err = json.Unmarshal(data, &request); err != nil {
			fmt.Fprintln(stderr, err)
			return 2
		}
	}
	// A broken tool record is reported, never fatal: the selector rejects
	// an invalid command on its own and every other provider stays usable.
	tools, toolErrs := fleet.New().Tools(false)
	for _, err := range toolErrs {
		fmt.Fprintln(stderr, "providers: warning:", err)
	}
	if !selectMode && role == "" {
		// List every declared role using the same projection path.
		roles := map[string]bool{}
		for _, rec := range registeredCommands() {
			for _, r := range rec.Provides {
				roles[r] = true
			}
		}
		for _, tool := range tools {
			for _, c := range tool.Commands {
				roles[c.Name] = true
			}
		}
		var all []providerselector.Candidate
		orderedRoles := make([]string, 0, len(roles))
		for r := range roles {
			orderedRoles = append(orderedRoles, r)
		}
		sort.Strings(orderedRoles)
		for _, r := range orderedRoles {
			all = append(all, providerselector.Project(registeredCommands(), tools, providerselector.Options{Role: r, Request: request, Yes: *yes})...)
		}
		if *asJSON {
			json.NewEncoder(stdout).Encode(all)
		} else {
			for _, c := range all {
				fmt.Fprintf(stdout, "%s\t%s\t%s\t%s\n", c.Role, c.Name, c.Route, c.Rejected)
			}
		}
		return 0
	}
	opt := providerselector.Options{Role: role, Provider: *provider, Route: *route, Request: request, Yes: *yes}
	candidates := providerselector.Project(registeredCommands(), tools, opt)
	if !selectMode {
		if *asJSON {
			json.NewEncoder(stdout).Encode(candidates)
		} else {
			for _, c := range candidates {
				fmt.Fprintf(stdout, "%s\t%s\t%s\n", c.Name, c.Route, c.Rejected)
			}
		}
		return 0
	}
	selected, err := providerselector.Select(candidates, opt)
	if err != nil {
		fmt.Fprintln(stderr, "providers select:", err)
		return 126
	}
	if *asJSON {
		json.NewEncoder(stdout).Encode(selected)
	} else {
		fmt.Fprintln(stdout, selected.Name)
	}
	return 0
}
