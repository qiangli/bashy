//go:build darwin

package agentos

// Sprint: #421; Story: #1906; Story-ID: e853e9f1e2d9

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"testing"
)

func TestSeatbeltProfileAllowsOnlyListedLoopbackPorts(t *testing.T) {
	if got := seatbeltProfile(nil); got != seatbeltDenyNetwork {
		t.Fatalf("no ports must leave the deny-all profile unchanged:\n%s", got)
	}
	got := seatbeltProfile([]int{24556, 51234})
	for _, want := range []string{
		"(deny network*)",
		`(allow network-outbound (remote ip "localhost:24556"))`,
		`(allow network-outbound (remote ip "localhost:51234"))`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("profile missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, `"*:`) || strings.Contains(got, "remote tcp") || strings.Contains(got, "api.z.ai") {
		t.Fatalf("profile opens more than loopback ports:\n%s", got)
	}
}

func TestContainNativeReachesOnlyAllowedModelEndpoint(t *testing.T) {
	if nativeContainSupported() != nil {
		t.Skip("no Seatbelt on this host")
	}
	if _, err := exec.LookPath("curl"); err != nil {
		t.Skip("no curl(1)")
	}
	hit := func(name string) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, name) }))
	}
	model, stranger := hit("model"), hit("stranger")
	defer model.Close()
	defer stranger.Close()

	old := loadContainEgress
	defer func() { loadContainEgress = old }()
	loadContainEgress = func() containEgress {
		return containEgress{Remote: []string{strings.TrimPrefix(model.URL, "http://")}}
	}

	curl := func(args string) int {
		return runNativeContained([]string{"sh", "-c", "curl -sf -m 5 -o /dev/null " + args})
	}
	viaProxy := `--noproxy '' -x "$http_proxy" `
	if code := curl(viaProxy + model.URL); code != 0 {
		t.Fatalf("allowed model endpoint through the proxy: exit %d, want 0", code)
	}
	if code := curl("--noproxy '*' " + model.URL); code == 0 {
		t.Fatalf("direct connection from the contained tree succeeded")
	}
	if code := curl(viaProxy + stranger.URL); code == 0 {
		t.Fatalf("an endpoint outside the allowlist was reachable through the proxy")
	}
}
