package kit

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"log"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

func signedIdentityRequest(secret, kind, user, visitor string, ts time.Time) *http.Request {
	const prefix = "/matrix/h/host/app/page"
	tsStr := strconv.FormatInt(ts.Unix(), 10)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "127.0.0.1:4321"
	req.Header.Set("X-Forwarded-Prefix", prefix)
	req.Header.Set("X-Forwarded-For", "192.0.2.10")
	req.Header.Set("X-Outpost-Identity-Ts", tsStr)
	req.Header.Set("X-Outpost-Relationship", kind)
	if user != "" {
		req.Header.Set("Remote-User", user)
	}
	if visitor != "" {
		req.Header.Set("X-Outpost-Visitor", visitor)
	}
	fields := []string{"v2", kind, "", "", visitor, user, prefix, tsStr}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(strings.Join(fields, "\n")))
	req.Header.Set("X-Outpost-Relationship-Sig", hex.EncodeToString(mac.Sum(nil)))
	return req
}

func TestIdentifyParticipantRelationshipsAndSignedInAlias(t *testing.T) {
	const (
		secret  = "kit-test-secret"
		visitor = "0123456789abcdef0123456789abcdef"
	)
	now := time.Now()

	for _, tc := range []struct {
		name      string
		kind      string
		user      string
		visitor   string
		wantAuth  string
		wantRel   string
		wantPrinc string
		wantHdl   string
		wantLog   bool
	}{
		{"owner", "owner", "owner@example.test", "", "member", "owner", "cloud:owner@example.test", "owner@example.test", false},
		{"member", "member", "member@example.test", "", "member", "member", "cloud:member@example.test", "member@example.test", false},
		{"account", "account", "", visitor, "account", "account", "visitor:" + visitor, "guest-01234567", false},
		{"signed-in alias", "signed-in", "", visitor, "account", "account", "visitor:" + visitor, "guest-01234567", true},
		{"unknown kind drops to anonymous", "user", "", visitor, "anonymous", "user", "", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			oldOut := log.Writer()
			log.SetOutput(&buf)
			t.Cleanup(func() { log.SetOutput(oldOut) })

			req := signedIdentityRequest(secret, tc.kind, tc.user, tc.visitor, now)
			got := IdentifyParticipant(req, "desk-participant", "", secret, now)
			if !got.Tunnel || got.Auth != tc.wantAuth || got.Relationship != tc.wantRel || got.Principal != tc.wantPrinc || got.Handle != tc.wantHdl {
				t.Fatalf("IdentifyParticipant = %+v, want auth=%q rel=%q principal=%q handle=%q", got, tc.wantAuth, tc.wantRel, tc.wantPrinc, tc.wantHdl)
			}
			logged := strings.Contains(buf.String(), "signed-in")
			if logged != tc.wantLog {
				t.Fatalf("alias log = %v (%q), want %v", logged, buf.String(), tc.wantLog)
			}
		})
	}

	t.Run("forged signed-in alias is anonymous and does not log", func(t *testing.T) {
		var buf bytes.Buffer
		oldOut := log.Writer()
		log.SetOutput(&buf)
		t.Cleanup(func() { log.SetOutput(oldOut) })

		req := signedIdentityRequest(secret, "signed-in", "", visitor, now)
		req.Header.Set("X-Outpost-Relationship-Sig", strings.Repeat("0", sha256.Size*2))
		got := IdentifyParticipant(req, "desk-participant", "", secret, now)
		if got.Tunnel || got.Auth != "anonymous" || got.Principal != "" {
			t.Fatalf("forged request = %+v, want anonymous without tunnel", got)
		}
		if buf.Len() != 0 {
			t.Fatalf("forged alias must not log, got %q", buf.String())
		}
	})

	t.Run("stale account stamp is anonymous", func(t *testing.T) {
		req := signedIdentityRequest(secret, "account", "", visitor, now.Add(-5*time.Minute))
		got := IdentifyParticipant(req, "desk-participant", "", secret, now)
		if got.Tunnel || got.Auth != "anonymous" || got.Principal != "" {
			t.Fatalf("stale request = %+v, want anonymous without tunnel", got)
		}
	})
}
