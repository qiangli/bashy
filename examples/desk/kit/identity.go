package kit

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"log"
	"net"
	"net/http"
	"net/netip"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// Participant contains the private fields used to bind an identity.
// Apps should expose only the public fields in their views and exports.
type Participant struct {
	Handle      string `json:"handle"`
	Principal   string `json:"principal,omitempty"`
	Address     string `json:"address,omitempty"`
	Fingerprint string `json:"fingerprint,omitempty"`
	Cookie      string `json:"cookie,omitempty"`
	Auth        string `json:"auth"` // member | account | anonymous
	JoinedAt    string `json:"joined_at,omitempty"`
	LastSeen    string `json:"last_seen,omitempty"`
	Kicked      bool   `json:"kicked,omitempty"`
}

var (
	ErrHandleInUse   = errors.New("that handle is already in use")
	ErrKicked        = errors.New("the organizer removed this participant")
	ErrJoinNeeded    = errors.New("join this page first")
	ErrTunnelAddress = errors.New("the satellite did not supply a client address; join over LAN or sign in")
)

type ParticipantCaller struct {
	Principal, Handle, Auth, Address, Fingerprint, Cookie string
	Tunnel                                                bool
	Relationship                                          string
}

// The console injects its principal after admission on its private loopback
// hop. Outpost supplies a separate HMAC contract. Its legacy signature also
// vouches for anonymous tunnel ingress when the relationship tuple is absent.
func IdentifyParticipant(r *http.Request, cookieName, consolePrincipal, secret string, now time.Time) ParticipantCaller {
	host, _, _ := net.SplitHostPort(r.RemoteAddr)
	peer, _ := netip.ParseAddr(host)
	c := ParticipantCaller{Auth: "anonymous", Address: peer.Unmap().String()}
	if !peer.IsValid() {
		c.Address = ""
	}
	fingerprint := sha256.Sum256([]byte(r.UserAgent() + "\x00" + r.Header.Get("Accept-Language")))
	c.Fingerprint = hex.EncodeToString(fingerprint[:])
	if cookie, err := r.Cookie(cookieName); err == nil && ValidToken(cookie.Value) {
		c.Cookie = cookie.Value
	}
	if !peer.IsLoopback() {
		return c
	}
	h := r.Header
	if consolePrincipal != "" {
		c.Principal, c.Handle, c.Auth = consolePrincipal, consolePrincipal, "member"
		if _, name, ok := strings.Cut(consolePrincipal, ":"); ok && name != "" {
			c.Handle = name
		}
	}
	kind, user, visitor, tunnel := outpostParticipant(h, secret, now)
	if !tunnel {
		return c
	}
	c.Tunnel, c.Relationship = true, kind
	c.Address = ""
	// The trusted tunnel supplies the observed client at the right of the
	// chain. A TCP ingress additionally appends its local tunnel peer.
	chain := strings.Split(strings.Join(h.Values("X-Forwarded-For"), ","), ",")
	ip, err := netip.ParseAddr(strings.TrimSpace(chain[len(chain)-1]))
	if err == nil && ip.IsLoopback() && len(chain) > 1 {
		ip, err = netip.ParseAddr(strings.TrimSpace(chain[len(chain)-2]))
	}
	if err == nil && ip.Zone() == "" {
		c.Address = ip.Unmap().String()
	}
	switch kind {
	case "owner", "member":
		if user != "" {
			c.Principal, c.Handle, c.Auth = "cloud:"+strings.ToLower(user), user, "member"
		}
	case "account", "signed-in":
		if kind == "signed-in" {
			log.Printf("desk/kit: deprecated X-Outpost-Relationship %q accepted as %q alias", kind, "account")
			c.Relationship = "account"
		}
		if raw, err := hex.DecodeString(visitor); err == nil && len(raw) == 16 {
			c.Principal, c.Handle, c.Auth = "visitor:"+visitor, "guest-"+visitor[:8], "account"
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

func CleanHandle(raw string) (string, error) {
	handle := strings.TrimSpace(raw)
	if !utf8.ValidString(handle) || utf8.RuneCountInString(handle) > 80 || handle == "" ||
		strings.ContainsFunc(handle, unicode.IsControl) {
		return "", errors.New("enter a name or email of up to 80 characters")
	}
	return handle, nil
}

func AnonymousKey(handle, address string) string {
	return strings.ToLower(handle) + "@" + address
}

func ParticipantCookieKey(participants map[string]*Participant, c ParticipantCaller) string {
	if c.Cookie == "" || c.Address == "" {
		return ""
	}
	for key, p := range participants {
		if p != nil && p.Principal == "" && p.Address == c.Address && p.Cookie == c.Cookie && p.Fingerprint == c.Fingerprint {
			return key
		}
	}
	return ""
}

func HandleTaken(participants map[string]*Participant, handle, except string) bool {
	for key, p := range participants {
		if p != nil && key != except && strings.EqualFold(p.Handle, handle) {
			return true
		}
	}
	return false
}

// ResolveParticipant binds an identity inside the caller's store transaction.
// A nil handle means an action or refresh. A non-nil handle is a deliberate
// join submission. Binding and the caller's item writes share a store lock.
func ResolveParticipant(participants *map[string]*Participant, openHandles bool, c ParticipantCaller, handle *string, suffix bool, now time.Time) (string, error) {
	key, display := c.Principal, c.Handle
	if key == "" {
		switch {
		case c.Address == "" && c.Tunnel:
			return "", ErrTunnelAddress
		case c.Address == "":
			return "", ErrJoinNeeded
		case handle == nil:
			key = ParticipantCookieKey(*participants, c)
			if key == "" {
				return "", ErrJoinNeeded
			}
		default:
			var err error
			display, err = CleanHandle(*handle)
			if err != nil {
				return "", err
			}
			key = AnonymousKey(display, c.Address)
			if prior := ParticipantCookieKey(*participants, c); prior != "" {
				key = prior
			}
			if (*participants)[key] == nil && !openHandles && HandleTaken(*participants, display, "") {
				if !suffix {
					return "", ErrHandleInUse
				}
				base := display
				for i := 2; ; i++ {
					tail := "-" + strconv.Itoa(i)
					runes := []rune(base)
					display = string(runes[:min(len(runes), 80-len(tail))]) + tail
					key = AnonymousKey(display, c.Address)
					if (*participants)[key] != nil || !HandleTaken(*participants, display, "") {
						break
					}
				}
			}
		}
	}
	stamp := now.UTC().Format(time.RFC3339Nano)
	p := (*participants)[key]
	if p == nil {
		if (*participants) == nil {
			(*participants) = make(map[string]*Participant)
		}
		p = &Participant{Handle: display, Principal: c.Principal, Auth: c.Auth, JoinedAt: stamp}
		(*participants)[key] = p
	}
	if p.Kicked {
		return "", ErrKicked
	}
	p.Address, p.Fingerprint, p.Cookie, p.Auth, p.LastSeen = c.Address, c.Fingerprint, c.Cookie, c.Auth, stamp
	return key, nil
}

// MintParticipantCookie uses the desk kit's cookie format and policy.
func MintParticipantCookie(w http.ResponseWriter, r *http.Request, cookieName string, c *ParticipantCaller) {
	if c.Cookie == "" {
		c.Cookie = ParticipantCookie(w, r, cookieName)
	}
}
