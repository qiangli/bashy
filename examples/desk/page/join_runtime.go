package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"time"

	"desk-kit"
)

func runOutpost(args ...string) []byte {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	raw, err := exec.CommandContext(ctx, envOr("OUTPOST_BIN", "outpost"), args...).Output()
	if err != nil {
		return nil
	}
	return raw
}

func pageOutpostSecret() string {
	if secret := strings.TrimSpace(os.Getenv("PAGE_SSO_SECRET")); secret != "" {
		return secret
	}
	return strings.TrimSpace(string(runOutpost("apps", "secret", "page")))
}

func pageSatelliteBase() string {
	if base := strings.TrimSpace(os.Getenv("PAGE_SATELLITE_URL")); base != "" {
		return base
	}
	var result struct {
		Status struct {
			Cloudbox string `json:"cloudbox_url"`
			Agent    string `json:"agent_name"`
		} `json:"status"`
	}
	if json.Unmarshal(runOutpost("status", "--json"), &result) != nil || result.Status.Agent == "" {
		return ""
	}
	base, err := url.Parse(result.Status.Cloudbox)
	if err != nil || base.Host == "" || (base.Scheme != "http" && base.Scheme != "https") {
		return ""
	}
	return strings.TrimRight(base.String(), "/") + "/matrix/h/" + url.PathEscape(result.Status.Agent) + "/app/page"
}

func explicitAddr(args []string) bool {
	for _, arg := range args {
		name, _, _ := strings.Cut(strings.TrimLeft(arg, "-"), "=")
		if strings.HasPrefix(arg, "-") && name == "addr" {
			return true
		}
	}
	return false
}

func moderationArgs(args []string, open bool) (kind, source, target string, err error) {
	if open && len(args) == 1 {
		return "open", "", "", nil
	}
	if len(args) == 2 && args[1] == "list" {
		return "list", "", "", nil
	}
	if len(args) == 3 && args[1] == "kick" {
		return "kick", args[2], "", nil
	}
	if len(args) == 4 && (args[1] == "rename" || args[1] == "merge") {
		return args[1], args[2], args[3], nil
	}
	return "", "", "", fmt.Errorf("use page participant ID --open|list|kick HANDLE|rename HANDLE NAME|merge HANDLE INTO")
}

func publishPage(w io.Writer, o *cliOptions, dry bool) int {
	if len(o.args) > 1 || len(o.args) == 1 && o.args[0] != "page" || o.public && o.private {
		return kit.UsageError(w, tool, o.asJSON, "use publish-outpost [page] [--public|--private] [--icon URL]")
	}
	var argv []string
	result := map[string]any{"app": "page", "verb": strings.TrimSuffix(o.verb, "-outpost"), "dryRun": dry}
	if o.verb == "unpublish-outpost" {
		if o.public || o.private || o.icon != "" {
			return kit.UsageError(w, tool, o.asJSON, "publish options are available on publish-outpost")
		}
		argv = []string{"apps", "rm", "page"}
	} else {
		_, port, err := net.SplitHostPort(o.addr)
		if err != nil || !fixedPort(port) {
			return kit.UsageError(w, tool, o.asJSON, "publish-outpost requires a fixed PAGE_ADDR port")
		}
		target := "http://" + net.JoinHostPort("127.0.0.1", port) + strings.TrimRight(o.prefix, "/") + "/"
		argv = []string{"apps", "add", "page", "--url", target, "--trust-cloud-identity"}
		if !o.public {
			argv = append(argv, "--require-login")
		}
		if o.icon != "" {
			argv = append(argv, "--icon", o.icon)
		}
		result["target"], result["public"] = target, o.public
		result["requireLogin"], result["icon"] = !o.public, o.icon
	}
	binary := envOr("OUTPOST_BIN", "outpost")
	result["commands"] = [][]string{append([]string{binary}, argv...)}
	if !dry {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := exec.CommandContext(ctx, binary, argv...).Run(); err != nil {
			fmt.Fprintf(w, "page: outpost registration failed: %v\n", err)
			return 1
		}
		if o.verb == "publish-outpost" {
			result["url"] = pageSatelliteBase()
		}
	}
	if o.asJSON {
		fmt.Fprintln(w, kit.CanonicalJSON(kit.Envelope(tool, result)))
	} else if dry {
		fmt.Fprintf(w, "page: would run %s %s\n", binary, strings.Join(argv, " "))
	} else if o.verb == "publish-outpost" {
		fmt.Fprintf(w, "page: published %s\n", result["url"])
	} else {
		fmt.Fprintln(w, "page: removed from outpost")
	}
	return 0
}
