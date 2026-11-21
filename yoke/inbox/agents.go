package main

import (
	"time"
)

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
