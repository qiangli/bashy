package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// AIRoute is any OpenAI-compatible chat endpoint: a provider's free tier
// called with the owner's key, or `bashy llm` on the owner's host. The key
// itself never lives in site.json, only the name of the variable holding it.
type AIRoute struct {
	BaseURL   string `json:"base_url"`
	Model     string `json:"model"`
	KeyEnv    string `json:"key_env"`
	DailyCap  int    `json:"daily_cap"`  // views per caller per day
	DailyMax  int    `json:"daily_max"`  // views per day across all callers (owner's budget)
	MaxTokens int    `json:"max_tokens"` // optional
}

// View is a generated presentation of the site for one audience.
type View struct {
	Name     string   `json:"name"`
	Title    string   `json:"title"`
	Prompt   string   `json:"prompt"`
	Audience Audience `json:"audience"`
}

// allowance counts views per caller and in total, per UTC day.
type allowance struct {
	mu    sync.Mutex
	day   string
	per   map[string]int
	total int
}

func (a *allowance) take(id string, perCap, totalCap int, now time.Time) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if d := now.UTC().Format("2006-01-02"); d != a.day {
		a.day, a.per, a.total = d, map[string]int{}, 0
	}
	if totalCap > 0 && a.total >= totalCap {
		return errors.New("the site's views for today are used up; try again tomorrow")
	}
	if perCap > 0 && a.per[id] >= perCap {
		return fmt.Errorf("you have used today's %d views; try again tomorrow", perCap)
	}
	a.per[id]++
	a.total++
	return nil
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// viewPrompt builds the messages for a view. Only the data the caller may
// see goes in: the public page text and the fields their audience admits.
func viewPrompt(v View, title, page string, fields []Field) []chatMessage {
	var b strings.Builder
	fmt.Fprintf(&b, "Site: %s\n\nPublic page text:\n%s\n", title, page)
	if len(fields) > 0 {
		b.WriteString("\nAdditional facts shared with this reader:\n")
		for _, f := range fields {
			fmt.Fprintf(&b, "- %s: %s\n", f.Label, f.Body)
		}
	}
	return []chatMessage{
		{Role: "system", Content: "You present a personal website for one reader. Use only the facts given; never invent contact details, dates, employers or credentials. Answer in concise Markdown.\n\n" + v.Prompt},
		{Role: "user", Content: b.String()},
	}
}

// complete calls the route once. The caller's identity is never sent.
func complete(ctx context.Context, client *http.Client, route AIRoute, key string, msgs []chatMessage) (string, error) {
	body := map[string]any{"model": route.Model, "messages": msgs}
	if route.MaxTokens > 0 {
		body["max_tokens"] = route.MaxTokens
	}
	raw, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(route.BaseURL, "/")+"/chat/completions", bytes.NewReader(raw))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("model route answered %d", resp.StatusCode)
	}
	var out struct {
		Choices []struct {
			Message chatMessage `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(data, &out); err != nil || len(out.Choices) == 0 {
		return "", errors.New("model route returned no answer")
	}
	return out.Choices[0].Message.Content, nil
}
