package main

import (
	"errors"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/qiangli/bashy/examples/desk/kit"
)

const pageParticipantCookie = "page-participant"

func identifyParticipant(r *http.Request, consolePrincipal, secret string, now time.Time) kit.ParticipantCaller {
	return kit.IdentifyParticipant(r, pageParticipantCookie, consolePrincipal, secret, now)
}

func resolveParticipant(page *PageItem, c kit.ParticipantCaller, handle *string, suffix bool, now time.Time) (string, error) {
	return kit.ResolveParticipant((*map[string]*Participant)(&page.Participants), page.OpenHandles, c, handle, suffix, now)
}

func mintParticipantCookie(w http.ResponseWriter, r *http.Request, c *kit.ParticipantCaller) {
	kit.MintParticipantCookie(w, r, pageParticipantCookie, c)
}

func randomHandle() string {
	const alphabet = "23456789abcdefghjkmnpqrstuvwxyz"
	var suffix [6]byte
	for i := range suffix {
		suffix[i] = alphabet[randomIndex(len(alphabet))]
	}
	return "guest-" + string(suffix[:])
}

// moderateParticipant mutates the identity and its existing vote together.
// A merge preserves the target's vote if both records have voted.
func moderateParticipant(page *PageItem, action, source, target string) error {
	if action == "open" {
		page.OpenHandles = true
		return nil
	}
	p := page.Participants[source]
	if p == nil || p.Kicked {
		return errors.New("participant not found")
	}
	switch action {
	case "kick":
		p.Kicked = true
		// The removed identity no longer speaks for anyone on a board.
		delete(page.BoardVoters, source)
		page.remapHand(source, "")
	case "rename":
		handle, err := kit.CleanHandle(target)
		if err != nil {
			return err
		}
		if kit.HandleTaken(page.Participants, handle, source) {
			return kit.ErrHandleInUse
		}
		if p.Principal == "" {
			key := kit.AnonymousKey(handle, p.Address)
			if key != source {
				// Reserve the previous handle at this address so a lost
				// cookie cannot turn a rename into a second respondent.
				old := *p
				old.Kicked, old.Cookie = true, ""
				page.Participants[source] = &old
				page.Participants[key] = p
				page.remapHand(source, key)
				if vote, ok := page.Votes[source]; ok {
					page.Votes[key] = vote
					delete(page.Votes, source)
				}
				if token, ok := page.Respondents[source]; ok {
					page.Respondents[key] = token
					delete(page.Respondents, source)
				}
				if token, ok := page.BoardVoters[source]; ok {
					page.BoardVoters[key] = token
					delete(page.BoardVoters, source)
				}
			}
		}
		p.Handle = handle
	case "merge":
		to := page.Participants[target]
		if source == target || to == nil || to.Kicked {
			return errors.New("merge needs a different active participant")
		}
		page.remapHand(source, target)
		if vote, ok := page.Votes[source]; ok {
			if _, exists := page.Votes[target]; !exists {
				page.Votes[target] = vote
			}
			delete(page.Votes, source)
		}
		// Native block responses follow the same rule: the target keeps
		// its own respondent, or inherits the source's.
		if token, ok := page.Respondents[source]; ok {
			if _, exists := page.Respondents[target]; !exists {
				page.Respondents[target] = token
			}
			delete(page.Respondents, source)
		}
		if token, ok := page.BoardVoters[source]; ok {
			if _, exists := page.BoardVoters[target]; !exists {
				page.BoardVoters[target] = token
			}
			delete(page.BoardVoters, source)
		}
		p.Kicked = true
	default:
		return errors.New("unknown participant action")
	}
	return nil
}

// participantExport is the allowlist used by record rendering.
type participantExport struct {
	Handle string `json:"handle"`
	Auth   string `json:"auth"`
}

func exportedParticipants(page *PageItem) []participantExport {
	rows := make([]participantExport, 0, len(page.Participants))
	for _, p := range page.Participants {
		if p != nil && !p.Kicked {
			rows = append(rows, participantExport{Handle: p.Handle, Auth: p.Auth})
		}
	}
	slices.SortFunc(rows, func(a, b participantExport) int {
		if order := strings.Compare(a.Handle, b.Handle); order != 0 {
			return order
		}
		return strings.Compare(a.Auth, b.Auth)
	})
	return rows
}
