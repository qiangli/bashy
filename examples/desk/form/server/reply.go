package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"strings"

	kit "desk-kit"
)

const modeReply = "replyform"

// ReplyRow retains only field identity, safe proposal and guard reason.
// Browser indexes, raw attributes, content and errors never enter the store.
type ReplyRow struct {
	StableID string `json:"stable_id"`
	Selector string `json:"selector"`
	Label    string `json:"label"`
	Value    string `json:"value,omitempty"`
	Status   string `json:"status"`
	Reason   string `json:"reason,omitempty"`
	Done     bool   `json:"done"`
}

func buildReply(extract, values, refusals json.RawMessage) ([]ReplyRow, error) {
	var envelope struct {
		Data json.RawMessage `json:"data"`
	}
	if json.Unmarshal(extract, &envelope) != nil || len(bytes.TrimSpace(envelope.Data)) == 0 || bytes.Equal(bytes.TrimSpace(envelope.Data), []byte("null")) {
		return nil, fmt.Errorf("extract must contain data")
	}
	raw := bytes.TrimSpace(envelope.Data)
	var dataBytes []byte
	switch {
	case len(raw) > 0 && raw[0] == '{':
		dataBytes = raw
	case len(raw) > 0 && raw[0] == '"':
		var s string
		if json.Unmarshal(raw, &s) != nil {
			return nil, fmt.Errorf("extract data must contain elements")
		}
		dataBytes = []byte(s)
	default:
		return nil, fmt.Errorf("extract data must contain elements")
	}
	// stable_id uses the frozen wire spelling.
	var wire struct {
		Elements []struct {
			Tag      string            `json:"tag"`
			StableID string            `json:"stable_id"`
			Label    string            `json:"label"`
			Selector string            `json:"selector"`
			Name     string            `json:"name"`
			Attrs    map[string]string `json:"attrs"`
		} `json:"elements"`
	}
	if json.Unmarshal(dataBytes, &wire) != nil || wire.Elements == nil {
		return nil, fmt.Errorf("extract data must contain elements")
	}
	var proposed map[string]json.RawMessage
	var results map[string]struct {
		Outcome string `json:"outcome_class"`
		Reason  string `json:"reason"`
	}
	if len(values) > 0 && json.Unmarshal(values, &proposed) != nil {
		return nil, fmt.Errorf("values must be keyed by stable_id (or selector)")
	}
	if len(refusals) > 0 && json.Unmarshal(refusals, &results) != nil {
		return nil, fmt.Errorf("refusals must be keyed by stable_id (or selector)")
	}
	rows := []ReplyRow{}
	seen := map[string]bool{}
	for _, e := range wire.Elements {
		tag := strings.ToLower(e.Tag)
		if tag != "input" && tag != "select" && tag != "textarea" {
			continue
		}
		if tag == "input" && contains([]string{"button", "submit", "reset", "image", "hidden"}, strings.ToLower(e.Attrs["type"])) {
			continue
		}
		id := e.StableID
		if id == "" {
			id = e.Selector
		}
		if id == "" || seen[id] {
			return nil, fmt.Errorf("extract fields require unique stable_id or selector")
		}
		seen[id] = true
		label := e.Label
		if label == "" {
			label = e.Name
		}
		if label == "" {
			label = e.Attrs["placeholder"]
		}
		if label == "" {
			label = id
		}
		row := ReplyRow{StableID: id, Selector: e.Selector, Label: label, Status: kit.StatusPending}
		result, blocked := results[id]
		if blocked {
			row.Status = kit.StatusFailed
			if result.Outcome == "refused" || strings.Contains(result.Outcome, "refus") || result.Outcome == "guard" || result.Outcome == "denied" || result.Outcome == "blocked" {
				row.Status = kit.StatusNeeds
			}
			row.Reason = result.Reason
		} else if strings.EqualFold(e.Attrs["type"], "password") || kit.SecretRefused([]kit.Field{{Name: e.Name}, {Name: id}}) {
			row.Status = kit.StatusNeeds
			row.Reason = "field:secret"
		} else if raw, ok := proposed[id]; ok {
			var proposal struct {
				Value  string `json:"value"`
				Status string `json:"status"`
			}
			if json.Unmarshal(raw, &row.Value) != nil {
				if json.Unmarshal(raw, &proposal) != nil {
					return nil, fmt.Errorf("proposal must be a string or value/status object")
				}
				if proposal.Status != "" && proposal.Status != kit.StatusPending && proposal.Status != kit.StatusFilled {
					return nil, fmt.Errorf("failed or refused proposals belong in refusals")
				}
				row.Value = proposal.Value
				if proposal.Status != "" {
					row.Status = proposal.Status
				}
			}
		}
		rows = append(rows, row)
	}
	for id := range results {
		if !seen[id] {
			return nil, fmt.Errorf("refusal does not match an extracted field")
		}
	}
	for id := range proposed {
		if !seen[id] {
			return nil, fmt.Errorf("proposal does not match an extracted field")
		}
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("extract has no form fields")
	}
	return rows, nil
}

func (a *app) reply(w http.ResponseWriter, req request, me string) {
	rows, err := buildReply(req.Extract, req.Values, req.Refusals)
	if err != nil {
		kit.Fail(w, 400, err.Error())
		return
	}
	// Reuse F1 creation and persistence, with reply rows replacing ordinary inputs.
	req.Definition = nil
	req.Fields = nil
	req.Body = ""
	req.ReplyRows = rows
	a.createItem(w, req, me, modeReply)
}

func replyAnswers(it *item) map[string]ReplyRow {
	out := make(map[string]ReplyRow, len(it.ReplyRows))
	for _, row := range it.ReplyRows {
		row.Done, _ = it.Answers[row.StableID].(bool)
		out[row.StableID] = row
	}
	return out
}

func normalizeReply(rows []ReplyRow, answers Answers, complete bool) (Answers, error) {
	known := map[string]bool{}
	for _, row := range rows {
		known[row.StableID] = true
	}
	out := Answers{}
	for id, v := range answers {
		done, ok := v.(bool)
		if !known[id] || !ok {
			return nil, fmt.Errorf("reply answers must be boolean completion marks for known fields")
		}
		out[id] = done
	}
	if complete {
		for _, row := range rows {
			if out[row.StableID] != true && row.Status != kit.StatusFilled {
				return nil, fmt.Errorf("mark every remaining field done before submitting")
			}
		}
	}
	return out, nil
}

func replyHTML(row ReplyRow) string {
	checked := ""
	if row.Done {
		checked = " checked"
	}
	value := ""
	if row.Status != kit.StatusNeeds && row.Status != kit.StatusFailed {
		value = "<pre>" + html.EscapeString(row.Value) + "</pre>" + kit.CopyButton("Copy value", row.Value)
	}
	return `<div class="desk-field" data-field="` + html.EscapeString(row.StableID) + `" data-kind="reply"><label>` + html.EscapeString(row.Label) + `</label>` + kit.Badge(row.Status) +
		`<p class="desk-muted desk-content" data-desk-render="inline">` + html.EscapeString(row.Reason) + `</p>` + value +
		`<label><input type="checkbox"` + checked + `> Mark done</label></div>`
}
