package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/qiangli/yoke/pkg/role"
)

type weaveCommentView struct {
	Kind        string    `json:"kind"`
	Body        string    `json:"body"`
	At          time.Time `json:"at"`
	Author      string    `json:"author,omitempty"`
	StoryID     string    `json:"story_id,omitempty"`
	EvidenceRef string    `json:"evidence_ref,omitempty"`
}

// sprintWaitQueue is the minimal shape of queue.json needed for wait.
// It mirrors weave.weaveQueue's Sprints field without importing unexported type.
type sprintWaitQueue struct {
	Stories []*sprintWaitStory `json:"stories"`
}

type sprintWaitStory struct {
	ID      int64              `json:"id"`
	Column  string             `json:"column"`
	Thread  []weaveCommentView `json:"thread,omitempty"`
	Created time.Time          `json:"created"`
	Owner   string             `json:"owner,omitempty"`
	Lease   *agentsSprintLease `json:"lease,omitempty"`
	Contact *role.Contact      `json:"contact,omitempty"`
	Runs    []agentsSprintRun  `json:"runs,omitempty"`
}

func loadSprintWaitQueue(dir string) (*sprintWaitQueue, error) {
	b, err := os.ReadFile(filepath.Join(dir, "queue.json"))
	if err != nil {
		if os.IsNotExist(err) {
			return &sprintWaitQueue{}, nil
		}
		return nil, err
	}
	var q sprintWaitQueue
	if err := json.Unmarshal(b, &q); err != nil {
		return nil, fmt.Errorf("queue parse: %w", err)
	}
	return &q, nil
}
