package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Tiers a caller resolves to. The tier for a signed-in cloudbox account that
// is not the owner's member is "signed-in", never "guest": guest is a member
// circle.
const (
	TierAnonymous = "anonymous"
	TierSignedIn  = "signed-in"
	TierMember    = "member"
	TierOwner     = "owner"
)

// Caller is who is asking, as outpost vouched for them. Email is set only for
// the owner and members; a signed-in stranger is known by Visitor alone.
type Caller struct {
	Tier    string   `json:"tier"`
	Circle  string   `json:"-"` // the owner's private label: used, never shown
	Groups  []string `json:"-"`
	Email   string   `json:"-"`
	Visitor string   `json:"-"`
}

// ID is the key allowances are counted under ("" for anonymous).
func (c Caller) ID() string {
	if c.Email != "" {
		return "e:" + strings.ToLower(c.Email)
	}
	if c.Visitor != "" {
		return "v:" + c.Visitor
	}
	return ""
}

const skew = 60 * time.Second

// resolveCaller verifies outpost's version-2 relationship stamp. Anything
// unsigned, stale or forged is anonymous: the headers are only trusted when
// their signature verifies under this app's SSO secret.
func resolveCaller(r *http.Request, secret string, now time.Time) Caller {
	anon := Caller{Tier: TierAnonymous}
	if secret == "" {
		return anon
	}
	h := r.Header
	prefix, ts, sig := h.Get("X-Forwarded-Prefix"), h.Get("X-Outpost-Identity-Ts"), h.Get("X-Outpost-Relationship-Sig")
	kind := h.Get("X-Outpost-Relationship")
	if prefix == "" || ts == "" || sig == "" || kind == "" {
		return anon
	}
	t, err := strconv.ParseInt(ts, 10, 64)
	if err != nil {
		return anon
	}
	if d := now.Sub(time.Unix(t, 0)); d > skew || d < -skew {
		return anon
	}
	circle, groups, visitor := h.Get("X-Outpost-Circle"), h.Get("X-Outpost-Groups"), h.Get("X-Outpost-Visitor")
	user := h.Get("Remote-User")
	payload := "v2\n" + kind + "\n" + circle + "\n" + groups + "\n" + visitor + "\n" + user + "\n" + prefix + "\n" + ts
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(payload))
	got, err := hex.DecodeString(sig)
	if err != nil || !hmac.Equal(got, mac.Sum(nil)) {
		return anon
	}
	c := Caller{Visitor: visitor}
	switch kind {
	case "owner":
		c.Tier, c.Email = TierOwner, user
	case "member":
		c.Tier, c.Email, c.Circle = TierMember, user, circle
		if groups != "" {
			c.Groups = strings.Split(groups, ",")
		}
	case "signed-in":
		if visitor == "" {
			return anon
		}
		c.Tier = TierSignedIn
	default:
		return anon
	}
	if c.Tier != TierSignedIn && c.Email == "" {
		return anon
	}
	return c
}
