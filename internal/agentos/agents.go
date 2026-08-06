// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package agentos

// The bare `agents` view deliberately reads weave's durable queue.  A process
// list cannot say whether a worker is assigned work (and a worker need not have
// a stable process at all); the queue is the execution source of truth.

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"text/tabwriter"
	"time"

	"github.com/qiangli/coreutils/pkg/fleet"
	"github.com/spf13/cobra"
)

const agentsRosterSchema = "bashy-agents-v1"

type agentsQueue struct {
	Root    string             `json:"root"`
	Items   []agentsQueueItem  `json:"items"`
	Stories []agentsQueueStory `json:"stories"`
}

type agentsQueueItem struct {
	ID        int64     `json:"id"`
	Title     string    `json:"title"`
	State     string    `json:"state"`
	Tool      string    `json:"tool"`
	Owner     string    `json:"owner"`
	Created   time.Time `json:"created"`
	StartedAt time.Time `json:"started_at"`
}

type agentsQueueStory struct {
	ID   int64 `json:"id"`
	Runs []struct {
		Repo string `json:"repo"`
		ID   int64  `json:"id"`
	} `json:"runs"`
}

type agentAssignment struct {
	Agent  string `json:"agent"`
	Owner  string `json:"owner,omitempty"`
	Repo   string `json:"repo"`
	Run    int64  `json:"run"`
	Sprint int64  `json:"sprint,omitempty"`
	State  string `json:"state"`
	Age    string `json:"age,omitempty"`
	Title  string `json:"title"`
}

type agentsRoster struct {
	SchemaVersion string            `json:"schema_version"`
	Assignments   []agentAssignment `json:"assignments"`
}

var agentsHomeDir = os.UserHomeDir

func newAgentsRosterCmd(opts ...fleet.Option) *cobra.Command {
	cmd := fleet.NewAgentsCmd(opts...)
	cmd.Short = "Show active agent assignments (use `agents list` for the catalog)"
	cmd.Long = "Show currently active agent assignments from weave's durable queue.\n\n" +
		"Use `bashy agents list` to list all registered agent bindings."
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		for _, name := range []string{"all", "band", "min-band"} {
			if f := cmd.Flags().Lookup(name); f != nil && f.Changed {
				return fmt.Errorf("--%s applies to `bashy agents list`, not the active-assignment view", name)
			}
		}
		return renderAgentRoster(cmd.OutOrStdout(), cmd.Flags().Lookup("json").Changed)
	}
	return cmd
}

func renderAgentRoster(w io.Writer, asJSON bool) error {
	assignments, err := activeAgentAssignments()
	if err != nil {
		return err
	}
	if asJSON {
		return json.NewEncoder(w).Encode(agentsRoster{SchemaVersion: agentsRosterSchema, Assignments: assignments})
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "AGENT\tOWNER\tREPO\tRUN\tSPRINT\tSTATE\tAGE\tWORK")
	for _, a := range assignments {
		sprint := "-"
		if a.Sprint != 0 {
			sprint = fmt.Sprintf("#%d", a.Sprint)
		}
		owner := a.Owner
		if owner == "" {
			owner = "-"
		}
		age := a.Age
		if age == "" {
			age = "-"
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t#%d\t%s\t%s\t%s\t%s\n", a.Agent, owner, a.Repo, a.Run, sprint, a.State, age, a.Title)
	}
	if err := tw.Flush(); err != nil {
		return err
	}
	_, err = fmt.Fprintln(w, "To list all registered agents: bashy agents list")
	return err
}

func activeAgentAssignments() ([]agentAssignment, error) {
	home, err := agentsHomeDir()
	if err != nil {
		return nil, err
	}
	var out []agentAssignment
	seen := map[string]bool{}
	for _, base := range []string{filepath.Join(home, ".bashy", "weave"), filepath.Join(home, ".agents", "weave"), filepath.Join(home, ".agents", "ycode", "weave")} {
		entries, err := os.ReadDir(base)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, err
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			path := filepath.Join(base, entry.Name(), "queue.json")
			data, err := os.ReadFile(path)
			if os.IsNotExist(err) {
				continue
			}
			if err != nil {
				return nil, err
			}
			var q agentsQueue
			if err := json.Unmarshal(data, &q); err != nil {
				return nil, fmt.Errorf("read weave queue %s: %w", path, err)
			}
			repo := filepath.Base(q.Root)
			if repo == "." || repo == "" {
				repo = entry.Name()
			}
			sprints := queueSprints(q, repo)
			for _, item := range q.Items {
				if item.State != "working" {
					continue
				}
				key := fmt.Sprintf("%s/%d", q.Root, item.ID)
				if seen[key] {
					continue
				}
				seen[key] = true
				agent := item.Owner
				if agent == "" {
					agent = item.Tool
				}
				if agent == "" {
					agent = "?"
				}
				out = append(out, agentAssignment{Agent: agent, Owner: item.Owner, Repo: repo, Run: item.ID, Sprint: sprints[item.ID], State: item.State, Age: assignmentAge(item, time.Now()), Title: item.Title})
			}
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Repo != out[j].Repo {
			return out[i].Repo < out[j].Repo
		}
		return out[i].Run < out[j].Run
	})
	return out, nil
}

func queueSprints(q agentsQueue, repo string) map[int64]int64 {
	out := make(map[int64]int64)
	for _, story := range q.Stories {
		for _, run := range story.Runs {
			if run.Repo == repo {
				out[run.ID] = story.ID
			}
		}
	}
	return out
}

func assignmentAge(item agentsQueueItem, now time.Time) string {
	then := item.StartedAt
	if then.IsZero() {
		then = item.Created
	}
	if then.IsZero() || then.After(now) {
		return ""
	}
	d := now.Sub(then).Round(time.Second)
	if d >= time.Hour {
		return fmt.Sprintf("%dh%02dm", int(d.Hours()), int(d.Minutes())%60)
	}
	if d >= time.Minute {
		return fmt.Sprintf("%dm%02ds", int(d.Minutes()), int(d.Seconds())%60)
	}
	return fmt.Sprintf("%ds", int(d.Seconds()))
}
