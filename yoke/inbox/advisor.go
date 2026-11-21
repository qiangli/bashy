package main

// hint is one piece of advice for a failed command.
type hint struct {
	dimension string // network | disk | cwd | compute
	retryable bool   // false ⇒ re-running as-is is doomed; change approach
	text      string // the human-readable explanation
	suggest   string // the concrete next action
}
