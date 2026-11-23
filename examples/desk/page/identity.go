package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net"
	"net/http"
	"net/netip"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/qiangli/bashy/examples/desk/kit"
)

const pageParticipantCookie = "page-participant"

var (
	errHandleInUse   = errors.New("that handle is already in use")
	errKicked        = errors.New("the organizer removed this participant")
	errJoinNeeded    = errors.New("join this page first")
	errTunnelAddress = errors.New("the satellite did not supply a client address; join over LAN or sign in")
)

type participantCaller struct {
	principal, handle, auth, address, fingerprint, cookie string
	tunnel                                                bool
}

// The console injects its principal after admission on its private loopback
// hop. Outpost supplies a separate HMAC contract. Its legacy signature also
// vouches for anonymous tunnel ingress when the relationship tuple is absent.
func identifyParticipant(r *http.Request, consolePrincipal, secret string, now time.Time) participantCaller {
	host, _, _ := net.SplitHostPort(r.RemoteAddr)
	peer, _ := netip.ParseAddr(host)
	c := participantCaller{auth: "anonymous", address: peer.Unmap().String()}
	if !peer.IsValid() {
		c.address = ""
	}
	fingerprint := sha256.Sum256([]byte(r.UserAgent() + "\x00" + r.Header.Get("Accept-Language")))
	c.fingerprint = hex.EncodeToString(fingerprint[:])
	if cookie, err := r.Cookie(pageParticipantCookie); err == nil && kit.ValidToken(cookie.Value) {
		c.cookie = cookie.Value
	}
	if !peer.IsLoopback() {
		return c
	}
	h := r.Header
	if consolePrincipal != "" {
		c.principal, c.handle, c.auth = consolePrincipal, consolePrincipal, "member"
		if _, name, ok := strings.Cut(consolePrincipal, ":"); ok && name != "" {
			c.handle = name
		}
	}
	kind, user, visitor, tunnel := outpostParticipant(h, secret, now)
	if !tunnel {
		return c
	}
	c.tunnel = true
	c.address = ""
	// The trusted tunnel supplies the observed client at the right of the
	// chain. A TCP ingress additionally appends its local tunnel peer.
	chain := strings.Split(strings.Join(h.Values("X-Forwarded-For"), ","), ",")
	ip, err := netip.ParseAddr(strings.TrimSpace(chain[len(chain)-1]))
	if err == nil && ip.IsLoopback() && len(chain) > 1 {
		ip, err = netip.ParseAddr(strings.TrimSpace(chain[len(chain)-2]))
	}
	if err == nil && ip.Zone() == "" {
		c.address = ip.Unmap().String()
	}
	switch kind {
	case "owner", "member":
		if user != "" {
			c.principal, c.handle, c.auth = "cloud:"+strings.ToLower(user), user, "member"
		}
	case "signed-in":
		if raw, err := hex.DecodeString(visitor); err == nil && len(raw) == 16 {
			c.principal, c.handle, c.auth = "visitor:"+visitor, "guest-"+visitor[:8], "signed-in"
		}
	}
	return c
}

func outpostParticipant(h http.Header, secret string, now time.Time) (kind, user, visitor string, valid bool) {
	if secret == "" {
		return "", "", "", false
	}
	prefix, ts := h.Get("X-Forwarded-Prefix"), h.Get("X-Outpost-Identity-Ts")
	unix, err := strconv.ParseInt(ts, 10, 64)
	if err != nil || prefix == "" {
		return "", "", "", false
	}
	if d := now.Sub(time.Unix(unix, 0)); d < -time.Minute || d > time.Minute {
		return "", "", "", false
	}
	kind, user, visitor = h.Get("X-Outpost-Relationship"), h.Get("Remote-User"), h.Get("X-Outpost-Visitor")
	if kind != "" {
		fields := []string{"v2", kind, h.Get("X-Outpost-Circle"), h.Get("X-Outpost-Groups"), visitor, user, prefix, ts}
		if verifyOutpostMAC(secret, fields, h.Get("X-Outpost-Relationship-Sig")) {
			return kind, user, visitor, true
		}
		return "", "", "", false
	}
	return "", "", "", verifyOutpostMAC(secret, []string{user, h.Get("Remote-Groups"), prefix, ts}, h.Get("X-Outpost-Identity-Sig"))
}

func verifyOutpostMAC(secret string, fields []string, signature string) bool {
	for _, field := range fields {
		if strings.ContainsAny(field, "\r\n") {
			return false
		}
	}
	got, err := hex.DecodeString(signature)
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(strings.Join(fields, "\n")))
	return hmac.Equal(got, mac.Sum(nil))
}

func cleanHandle(raw string) (string, error) {
	handle := strings.TrimSpace(raw)
	if !utf8.ValidString(handle) || utf8.RuneCountInString(handle) > 80 || handle == "" ||
		strings.ContainsFunc(handle, unicode.IsControl) {
		return "", errors.New("enter a name or email of up to 80 characters")
	}
	return handle, nil
}

func anonymousKey(handle, address string) string {
	return strings.ToLower(handle) + "@" + address
}

func participantCookieKey(page *PageItem, c participantCaller) string {
	if c.cookie == "" || c.address == "" {
		return ""
	}
	for key, p := range page.Participants {
		if p != nil && p.Principal == "" && p.Address == c.address && p.Cookie == c.cookie && p.Fingerprint == c.fingerprint {
			return key
		}
	}
	return ""
}

func handleTaken(page *PageItem, handle, except string) bool {
	for key, p := range page.Participants {
		if p != nil && key != except && strings.EqualFold(p.Handle, handle) {
			return true
		}
	}
	return false
}

// resolveParticipant is called with a mutable page inside its store transaction.
// A nil handle means a page action or refresh. A non-nil handle is a deliberate
// join submission. Binding and tally reads/writes must share the same store lock.
func resolveParticipant(page *PageItem, c participantCaller, handle *string, suffix bool, now time.Time) (string, error) {
	key, display := c.principal, c.handle
	if key == "" {
		switch {
		case c.address == "" && c.tunnel:
			return "", errTunnelAddress
		case c.address == "":
			return "", errJoinNeeded
		case handle == nil:
			key = participantCookieKey(page, c)
			if key == "" {
				return "", errJoinNeeded
			}
		default:
			var err error
			display, err = cleanHandle(*handle)
			if err != nil {
				return "", err
			}
			key = anonymousKey(display, c.address)
			if prior := participantCookieKey(page, c); prior != "" {
				key = prior
			}
			if page.Participants[key] == nil && !page.OpenHandles && handleTaken(page, display, "") {
				if !suffix {
					return "", errHandleInUse
				}
				base := display
				for i := 2; ; i++ {
					tail := "-" + strconv.Itoa(i)
					runes := []rune(base)
					display = string(runes[:min(len(runes), 80-len(tail))]) + tail
					key = anonymousKey(display, c.address)
					if page.Participants[key] != nil || !handleTaken(page, display, "") {
						break
					}
				}
			}
		}
	}
	stamp := now.UTC().Format(time.RFC3339Nano)
	p := page.Participants[key]
	if p == nil {
		if page.Participants == nil {
			page.Participants = make(map[string]*Participant)
		}
		p = &Participant{Handle: display, Principal: c.principal, Auth: c.auth, JoinedAt: stamp}
		page.Participants[key] = p
	}
	if p.Kicked {
		return "", errKicked
	}
	p.Address, p.Fingerprint, p.Cookie, p.Auth, p.LastSeen = c.address, c.fingerprint, c.cookie, c.auth, stamp
	return key, nil
}

// mintParticipantCookie uses the desk kit's cookie format and policy.
func mintParticipantCookie(w http.ResponseWriter, r *http.Request, c *participantCaller) {
	if c.cookie == "" {
		c.cookie = kit.ParticipantCookie(w, r, pageParticipantCookie)
	}
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
		handle, err := cleanHandle(target)
		if err != nil {
			return err
		}
		if handleTaken(page, handle, source) {
			return errHandleInUse
		}
		if p.Principal == "" {
			key := anonymousKey(handle, p.Address)
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
