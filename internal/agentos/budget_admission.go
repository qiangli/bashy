package agentos

import (
	"context"
	"github.com/qiangli/coreutils/pkg/llmbudget"
)

// Candidate selection is an observation. The eventual chat/harness launch must
// obtain its own reservation; ranking never consumes a rate or work slot.
func previewStewardBudget(model, agent string) llmbudget.Decision {
	a, e := llmbudget.Preview(context.Background(), llmbudget.Request{Model: model, Agent: agent, UnknownTokens: true, Concurrency: 1})
	if e != nil {
		return llmbudget.Decision{Action: llmbudget.Block, Model: model, Reason: e.Error()}
	}
	if a.Decision.Action != llmbudget.Allow {
		return llmbudget.Decision{Action: llmbudget.Block, Model: model, Reason: a.Decision.Reason}
	}
	return a.Decision
}
