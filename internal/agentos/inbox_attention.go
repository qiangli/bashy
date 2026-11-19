package agentos

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/qiangli/yoke/pkg/ask"
	"github.com/spf13/cobra"
)

// attentionNotice is optional structured content inside an existing mailbox
// message, not a second request store. Producers must name the actual decision
// and immutable artifact/revision; none of these claims authorize an operation.
type attentionNotice struct {
	Schema       string   `json:"schema"`
	Initiative   string   `json:"initiative"`
	Kind         string   `json:"kind"`
	Urgency      string   `json:"urgency"`
	Summary      string   `json:"summary"`
	Artifacts    []string `json:"artifacts,omitempty"`
	Consequences string   `json:"consequences,omitempty"`
	Decision     string   `json:"decision,omitempty"`
	Revision     string   `json:"revision,omitempty"`
}
type attentionItem struct {
	ID string `json:"id"`
	attentionNotice
	Ready   bool     `json:"review_ready"`
	Missing []string `json:"missing,omitempty"`
	Next    []string `json:"next,omitempty"`
}
type attentionGroup struct {
	Initiative string          `json:"initiative"`
	Urgency    string          `json:"urgency"`
	Preference string          `json:"preference"`
	Items      []attentionItem `json:"items"`
}

func attentionRank(s string) int {
	switch s {
	case "urgent":
		return 0
	case "pending":
		return 1
	default:
		return 2
	}
}
func validParticipation(s string) bool {
	switch s {
	case "observe", "answer-questions", "review-milestones", "direct-work":
		return true
	}
	return false
}

// composeAttention never truncates pending requests or equates repeated prose
// with identity. snapshotMailbox has already deduplicated proven MB/Meet copies.
func composeAttention(mail []mailboxItem, requests []ask.Request, prefs map[string]string) []attentionGroup {
	groups := map[string]*attentionGroup{}
	seen := map[string]bool{}
	add := func(i attentionItem) {
		if seen[i.ID] {
			return
		}
		seen[i.ID] = true
		if i.Initiative == "" {
			i.Initiative = "unassigned"
		}
		if i.Urgency != "urgent" && i.Urgency != "pending" {
			i.Urgency = "routine"
		}
		refs := make([]string, 0, len(i.Artifacts))
		for _, ref := range i.Artifacts {
			if strings.TrimSpace(ref) != "" {
				refs = append(refs, ref)
			}
		}
		i.Artifacts = refs
		i.Ready = true
		if i.Kind == "review" {
			if len(i.Artifacts) == 0 {
				i.Missing = append(i.Missing, "artifact or diff")
			}
			if strings.TrimSpace(i.Consequences) == "" {
				i.Missing = append(i.Missing, "consequences")
			}
			if strings.TrimSpace(i.Decision) == "" {
				i.Missing = append(i.Missing, "exact decision")
			}
			if strings.TrimSpace(i.Revision) == "" {
				i.Missing = append(i.Missing, "candidate revision")
			}
			i.Ready = len(i.Missing) == 0
		}
		key := i.Initiative + "\x00" + i.Urgency
		g := groups[key]
		if g == nil {
			preference := prefs[i.Initiative]
			if !validParticipation(preference) {
				preference = "observe"
			}
			g = &attentionGroup{Initiative: i.Initiative, Urgency: i.Urgency, Preference: preference}
			groups[key] = g
		}
		g.Items = append(g.Items, i)
	}
	for _, m := range mail {
		if m.Acknowledged {
			continue
		}
		n := attentionNotice{Initiative: m.Project, Kind: "notification", Urgency: "routine", Summary: m.Body}
		if n.Initiative == "" {
			n.Initiative = m.Room
		}
		if n.Initiative == "" {
			n.Initiative = m.Topic
		}
		switch strings.ToLower(m.Status) {
		case "blocked", "conflict":
			n.Kind = "conflict"
			n.Urgency = "urgent"
		case "question":
			n.Kind = "question"
			n.Urgency = "pending"
		case "review", "complete", "completed", "done":
			n.Kind = "review"
			n.Urgency = "pending"
		case "approval":
			n.Kind = "approval"
			n.Urgency = "pending"
		}
		var explicit attentionNotice
		if json.Unmarshal([]byte(m.Body), &explicit) == nil && explicit.Schema == "bashy-attention-notice-v1" {
			if explicit.Initiative == "" {
				explicit.Initiative = n.Initiative
			}
			n = explicit
		}
		switch n.Kind {
		case "conflict", "question", "approval", "review":
		default:
			n.Kind = "notification"
		}
		// Important requests cannot be demoted by producer-supplied urgency.
		if n.Kind == "conflict" {
			n.Urgency = "urgent"
		} else if n.Kind == "question" || n.Kind == "approval" || n.Kind == "review" {
			if n.Urgency != "urgent" {
				n.Urgency = "pending"
			}
		}
		add(attentionItem{ID: m.ID, attentionNotice: n})
	}
	for _, r := range requests {
		n := attentionNotice{Initiative: "unassigned", Kind: "question", Urgency: "pending", Summary: r.Prompt, Decision: "Answer the question through ask answer", Consequences: r.Sink.Describe()}
		next := []string{"bashy", "ask", "answer", r.ID}
		if r.IsApproval() && r.Approval != nil {
			a := r.Approval
			n.Kind = "approval"
			n.Initiative = a.Sprint
			n.Revision = a.Token
			n.Artifacts = append([]string(nil), a.Argv...)
			effects, _ := json.Marshal(a.Effects)
			argv, _ := json.Marshal(a.Argv)
			options, _ := json.Marshal(a.Options)
			n.Consequences = "Effects: " + string(effects)
			n.Decision = "Allow or deny exact argv " + string(argv) + "; options: " + string(options)
			next = []string{"bashy", "approve", "show", r.ID}
		}
		add(attentionItem{ID: "ask:" + r.ID, attentionNotice: n, Next: next})
	}
	out := make([]attentionGroup, 0, len(groups))
	for _, g := range groups {
		sort.Slice(g.Items, func(i, j int) bool { return g.Items[i].ID < g.Items[j].ID })
		out = append(out, *g)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Urgency != out[j].Urgency {
			return attentionRank(out[i].Urgency) < attentionRank(out[j].Urgency)
		}
		return out[i].Initiative < out[j].Initiative
	})
	return out
}

func newAttentionCmd(human bool) *cobra.Command {
	var as, initiative, preference string
	var jsonOut bool
	c := &cobra.Command{Use: "attention", Short: "group pending questions, approvals, conflicts and reviews by initiative", Long: `Attention composes the durable mailbox without acknowledging it. The human lane
also reads the existing local ask/approval queue. No answer, permission, or
approval is inferred from a participation preference. All pending items remain
visible in every mode; use inbox ack explicitly when a mailbox request is settled.

  bashy inbox human attention --json
  bashy inbox human attention --initiative sprint:666 --participation review-milestones

Review producers may send JSON in an existing mailbox body with schema
bashy-attention-notice-v1, initiative, kind (question/approval/conflict/review),
summary, artifacts (stable references), consequences, decision and revision.
Incomplete reviews remain visible with missing fields. Approval and question
responses use the existing approve and ask answer commands; inspect their full
request before deciding. A preference never grants authority or settles a request.`, Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if preference != "" && (!validParticipation(preference) || strings.TrimSpace(initiative) == "") {
				return fmt.Errorf("attention: --participation needs --initiative and observe, answer-questions, review-milestones or direct-work")
			}
			spec, err := mailboxSpecFor(human, as)
			if err != nil {
				return err
			}
			if preference != "" {
				if err = updateMailboxState(spec, func(s *mailboxState) error {
					if s.Participation == nil {
						s.Participation = map[string]string{}
					}
					s.Participation[initiative] = preference
					return nil
				}); err != nil {
					return err
				}
			}
			mail, state, err := snapshotMailbox(spec)
			if err != nil {
				return err
			}
			var requests []ask.Request
			if human {
				requests, err = ask.InspectPending()
				if err != nil {
					return err
				}
			}
			groups := composeAttention(mail, requests, state.Participation)
			if initiative != "" {
				filtered := make([]attentionGroup, 0)
				for _, g := range groups {
					if g.Initiative == initiative {
						filtered = append(filtered, g)
					}
				}
				groups = filtered
			}
			if jsonOut {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(struct {
					Schema string           `json:"schema"`
					Groups []attentionGroup `json:"groups"`
				}{"bashy-attention-v1", groups})
			}
			fmt.Fprintln(cmd.OutOrStdout(), "Participation preferences do not grant permissions. Pending requests remain until explicitly settled.")
			for _, g := range groups {
				fmt.Fprintf(cmd.OutOrStdout(), "\n%s / %s (%s)\n", g.Initiative, g.Urgency, g.Preference)
				for _, i := range g.Items {
					fmt.Fprintf(cmd.OutOrStdout(), "  %s [%s] %s\n", i.ID, i.Kind, i.Summary)
					if i.Decision != "" {
						fmt.Fprintln(cmd.OutOrStdout(), "    Decision:", i.Decision)
					}
					if i.Consequences != "" {
						fmt.Fprintln(cmd.OutOrStdout(), "    Consequences:", i.Consequences)
					}
					if len(i.Artifacts) > 0 {
						b, _ := json.Marshal(i.Artifacts)
						fmt.Fprintln(cmd.OutOrStdout(), "    Artifacts/operation:", string(b), "revision:", i.Revision)
					}
					if !i.Ready {
						fmt.Fprintln(cmd.OutOrStdout(), "    Needs:", strings.Join(i.Missing, ", "))
					}
					if len(i.Next) > 0 {
						b, _ := json.Marshal(i.Next)
						fmt.Fprintln(cmd.OutOrStdout(), "    Next command argv:", string(b))
					}
				}
			}
			return nil
		}}
	c.Flags().StringVar(&as, "as", "", "read as your existing inbox identity")
	c.Flags().StringVar(&initiative, "initiative", "", "select one initiative (omitted: all pending initiatives)")
	c.Flags().StringVar(&preference, "participation", "", "save presentation preference only")
	c.Flags().BoolVar(&jsonOut, "json", false, "emit grouped attention JSON")
	return c
}
