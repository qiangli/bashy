package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

type agentsQueue struct {
	Root    string             `json:"root"`
	Items   []agentsQueueItem  `json:"items"`
	Stories []agentsQueueStory `json:"stories"`
}

type agentsQueueItem struct {
	ID          int64             `json:"id"`
	Title       string            `json:"title"`
	State       string            `json:"state"`
	Tool        string            `json:"tool"`
	Owner       string            `json:"owner"`
	Points      int               `json:"points,omitempty"`
	Created     time.Time         `json:"created"`
	StartedAt   time.Time         `json:"started_at"`
	FinishedAt  time.Time         `json:"finished_at,omitempty"`
	WrapperPID  int               `json:"wrapper_pid,omitempty"`
	Stale       bool              `json:"stale,omitempty"`
	Blocked     bool              `json:"blocked,omitempty"`
	LaunchPhase string            `json:"launch_phase,omitempty"`
	LaunchSpec  *agentsLaunchSpec `json:"launch_spec,omitempty"`
	Comments    []agentsComment   `json:"comments,omitempty"`
}

type agentsLaunchSpec struct {
	Agent      string        `json:"agent,omitempty"`
	Tool       string        `json:"tool,omitempty"`
	MaxRuntime time.Duration `json:"max_runtime,omitempty"`
}

type agentsComment struct {
	At   time.Time `json:"at"`
	Kind string    `json:"kind,omitempty"`
}

type agentsQueueStory struct {
	ID      int64             `json:"id"`
	Column  string            `json:"column"`
	Updated time.Time         `json:"updated_at,omitempty"`
	Runs    []agentsSprintRun `json:"runs"`
}

type agentsSprintLease struct {
	Holder string    `json:"holder"`
	At     time.Time `json:"at"`
	// AttachedPID mirrors weaveStoryLease.AttachedPID: the process holding the
	// seat open in the foreground, or zero when no process claims to be.
	AttachedPID int `json:"attached_pid,omitempty"`
}

type agentsSprintRun struct {
	Repo  string    `json:"repo"`
	ID    int64     `json:"id"`
	Queue string    `json:"queue,omitempty"`
	Born  time.Time `json:"born,omitempty"`
	Story string    `json:"story,omitempty"`
}

func defaultAgentWeaveRoots(home string) []string {
	return []string{
		filepath.Join(home, ".bashy", "weave"),
		filepath.Join(home, ".agents", "weave"),
		filepath.Join(home, ".agents", "ycode", "weave"),
	}
}

func readAgentJSON(path string, dst any) (bool, error) {
	for attempt := 0; attempt < 3; attempt++ {
		b, err := os.ReadFile(path)
		if os.IsNotExist(err) {
			return false, nil
		}
		if err != nil {
			return true, err
		}
		if err := json.Unmarshal(b, dst); err == nil {
			return true, nil
		} else if attempt == 2 {
			return true, err
		}
		time.Sleep(time.Millisecond)
	}
	return false, nil
}

func itemLastProgress(item agentsQueueItem) time.Time {
	for i := len(item.Comments) - 1; i >= 0; i-- {
		if relevantWorkerComment(item.Comments[i].Kind) && !item.Comments[i].At.IsZero() {
			return item.Comments[i].At
		}
	}
	if !item.StartedAt.IsZero() {
		return item.StartedAt
	}
	return item.Created
}

func relevantWorkerComment(kind string) bool {
	switch kind {
	case "blocker", "progress", "decision", "review":
		return true
	default:
		return false
	}
}
