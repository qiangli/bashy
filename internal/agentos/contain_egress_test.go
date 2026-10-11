package agentos

// Sprint: #421; Story: #1906; Story-ID: e853e9f1e2d9

import (
	"bufio"
	"io"
	"net"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestContainEgressSplitsProvidersFromLoopback(t *testing.T) {
	got := buildContainEgress([]string{
		"https://api.z.ai/api/coding/paas/v4",
		"API.Z.AI",
		"http://models.example:8080/v1",
		"http://127.0.0.1:24556/v1",
		"localhost:9999",
		"not a url ::",
		"",
	}, 24556)
	want := containEgress{
		Remote:        []string{"api.z.ai:443", "models.example:8080"},
		LoopbackPorts: []int{9999, 24556},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("egress = %+v, want %+v", got, want)
	}
	if !buildContainEgress(nil, 0).empty() {
		t.Fatalf("no endpoints and no door must be an empty allowlist")
	}
}

func TestContainAllowHostEnvAddsEndpoints(t *testing.T) {
	t.Setenv(containAllowHostEnv, "llm.corp.example:8443, https://other.example")
	got := loadContainEgress()
	for _, want := range []string{"llm.corp.example:8443", "other.example:443"} {
		found := false
		for _, r := range got.Remote {
			found = found || r == want
		}
		if !found {
			t.Fatalf("%s missing from %v", want, got.Remote)
		}
	}
}

func TestContainProxyServesOnlyTheAllowlist(t *testing.T) {
	upstream, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer upstream.Close()
	go func() {
		for {
			c, err := upstream.Accept()
			if err != nil {
				return
			}
			go func() { defer c.Close(); io.Copy(c, c) }()
		}
	}()
	other, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()

	proxy, err := startContainProxy([]string{upstream.Addr().String()})
	if err != nil {
		t.Fatal(err)
	}
	defer proxy.close()

	connect := func(target string) (*bufio.Reader, net.Conn, string) {
		c, err := net.DialTimeout("tcp", proxy.addr(), 3*time.Second)
		if err != nil {
			t.Fatal(err)
		}
		c.SetDeadline(time.Now().Add(5 * time.Second))
		io.WriteString(c, "CONNECT "+target+" HTTP/1.1\r\nHost: "+target+"\r\n\r\n")
		r := bufio.NewReader(c)
		status, err := r.ReadString('\n')
		if err != nil {
			t.Fatalf("CONNECT %s: %v", target, err)
		}
		return r, c, status
	}

	r, c, status := connect(upstream.Addr().String())
	defer c.Close()
	if !strings.Contains(status, " 200 ") {
		t.Fatalf("allowed endpoint refused: %q", status)
	}
	for {
		line, _ := r.ReadString('\n')
		if line == "\r\n" || line == "" {
			break
		}
	}
	io.WriteString(c, "ping\n")
	if echo, _ := r.ReadString('\n'); echo != "ping\n" {
		t.Fatalf("tunnel did not carry data: %q", echo)
	}

	_, denied, status := connect(other.Addr().String())
	defer denied.Close()
	if !strings.Contains(status, " 403 ") {
		t.Fatalf("endpoint outside the allowlist was not refused: %q", status)
	}
}

func TestContainProxyEnvKeepsLoopbackDirect(t *testing.T) {
	env := strings.Join(containProxyEnv("127.0.0.1:4242"), "\n")
	for _, want := range []string{"HTTPS_PROXY=http://127.0.0.1:4242", "https_proxy=http://127.0.0.1:4242", "NO_PROXY=localhost,127.0.0.1,::1"} {
		if !strings.Contains(env, want) {
			t.Fatalf("proxy env missing %q:\n%s", want, env)
		}
	}
}
