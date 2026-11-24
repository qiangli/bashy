// Copyright (c) 2026 qiangli
// See LICENSE for licensing information.

// Fake gws / gh CLI binary for hermetic H5 hosted-runner load benchmarks.
// Dispatches by argv[0] basename ("gws" or "gh") or --as <tool>.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func emitJSON(v any) {
	enc := json.NewEncoder(os.Stdout)
	_ = enc.Encode(v)
}

func runGWS(args []string) int {
	if len(args) > 0 && (args[0] == "--version" || args[0] == "version") {
		fmt.Println("gws 0.1.0-fake")
		return 0
	}
	if os.Getenv("GWS_ACCOUNT") == "" {
		fmt.Fprintln(os.Stderr, "fake gws: GWS_ACCOUNT is required")
		return 2
	}
	credPath := os.Getenv("GOOGLE_WORKSPACE_CLI_CREDENTIALS_FILE")
	if credPath == "" {
		fmt.Fprintln(os.Stderr, "fake gws: GOOGLE_WORKSPACE_CLI_CREDENTIALS_FILE is required")
		return 2
	}
	st, err := os.Stat(credPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "fake gws: stat credentials: %v\n", err)
		return 2
	}
	if st.Mode().Perm()&0o077 != 0 {
		fmt.Fprintf(os.Stderr, "fake gws: credentials mode %o must be 0600\n", st.Mode().Perm())
		return 2
	}
	raw, err := os.ReadFile(credPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "fake gws: read credentials: %v\n", err)
		return 2
	}
	var cred map[string]any
	if err := json.Unmarshal(raw, &cred); err != nil || cred["refresh_token"] == "" {
		fmt.Fprintln(os.Stderr, "fake gws: invalid credentials JSON")
		return 2
	}

	if len(args) >= 4 && args[0] == "gmail" && args[1] == "users" && args[2] == "messages" && args[3] == "list" {
		emitJSON(map[string]any{
			"messages": []map[string]string{
				{"id": "msg-1", "threadId": "thr-1"},
				{"id": "msg-2", "threadId": "thr-2"},
			},
			"resultSizeEstimate": 2,
		})
		return 0
	}

	if len(args) >= 3 && args[0] == "drive" && args[1] == "files" && args[2] == "list" {
		q := ""
		for i := 3; i < len(args)-1; i++ {
			if args[i] == "--params" {
				var p struct {
					Q string `json:"q"`
				}
				if err := json.Unmarshal([]byte(args[i+1]), &p); err == nil {
					q = p.Q
				}
			}
		}
		folderType := "application/vnd.google-apps.folder"
		var files []map[string]any
		switch {
		case strings.Contains(q, "'root' in parents"):
			files = []map[string]any{
				{"id": "project", "name": "Project", "mimeType": folderType},
			}
		case strings.Contains(q, "'project' in parents"):
			files = []map[string]any{
				{"id": "note", "name": "O'Brien.txt", "mimeType": "text/plain", "size": "12", "modifiedTime": "2026-11-24T00:00:00Z"},
				{"id": "sub", "name": "sub", "mimeType": folderType},
			}
		default:
			files = []map[string]any{}
		}
		emitJSON(map[string]any{"files": files})
		return 0
	}

	fmt.Fprintf(os.Stderr, "fake gws: unsupported args %v\n", args)
	return 2
}

func runGH(args []string) int {
	if len(args) > 0 && (args[0] == "--version" || args[0] == "version") {
		fmt.Println("gh version 2.102.0 (fake)")
		return 0
	}
	if len(args) >= 2 && args[0] == "pr" && args[1] == "list" {
		emitJSON([]map[string]any{
			{"number": 42, "title": "H5 hosted runner load test", "state": "OPEN"},
		})
		return 0
	}
	if len(args) >= 1 && args[0] == "api" {
		endpoint := args[len(args)-1]
		switch {
		case endpoint == "repos/demo/repo":
			emitJSON(map[string]any{"default_branch": "main", "has_issues": true})
			return 0
		case strings.HasPrefix(endpoint, "repos/demo/repo/git/trees/"):
			emitJSON(map[string]any{
				"truncated": false,
				"tree": []map[string]any{
					{"path": "README.md", "type": "blob", "sha": strings.Repeat("a", 40), "mode": "100644", "size": 11},
				},
			})
			return 0
		}
	}
	fmt.Fprintf(os.Stderr, "fake gh: unsupported args %v\n", args)
	return 2
}

func main() {
	tool := filepath.Base(os.Args[0])
	args := os.Args[1:]
	if len(args) >= 2 && args[0] == "--as" {
		tool = args[1]
		args = args[2:]
	}
	switch tool {
	case "gws":
		os.Exit(runGWS(args))
	case "gh":
		os.Exit(runGH(args))
	default:
		fmt.Fprintf(os.Stderr, "fakecli: unknown tool %q\n", tool)
		os.Exit(2)
	}
}
