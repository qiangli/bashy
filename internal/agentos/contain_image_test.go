package agentos

// Sprint: #290; Story: #1042; Story-ID: 530c3e2e68ac

import (
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

const testDigestRef = "docker.io/swebench/sweb.eval.x86_64.django_1776_django-11740@sha256:cd95eb83c40d5738a534d6210a485345fffc9cf653c2c5995b4395d202ff0eb4"

func TestContainImageSpecValidate(t *testing.T) {
	ok := func(s containImageSpec) *containImageSpec {
		if s.Image == "" {
			s.Image = testDigestRef
		}
		if s.Argv == nil {
			s.Argv = []string{"true"}
		}
		return &s
	}
	for _, tc := range []struct {
		name string
		spec *containImageSpec
		want string // "" = valid
	}{
		{"digest", ok(containImageSpec{}), ""},
		{"tag refused", ok(containImageSpec{Image: "docker.io/swebench/x:latest"}), "not pinned by digest"},
		{"bare name refused", ok(containImageSpec{Image: "alpine"}), "not pinned by digest"},
		{"short digest refused", ok(containImageSpec{Image: "alpine@sha256:abc"}), "not pinned by digest"},
		{"door needs sticky", ok(containImageSpec{Net: "door"}), "needs --sticky"},
		{"door with sticky", ok(containImageSpec{Net: "door", Sticky: "arm-genie"}), ""},
		{"sticky needs door", ok(containImageSpec{Sticky: "k"}), "needs --net door"},
		{"allow refused", ok(containImageSpec{Net: "allow"}), "deny or door"},
		{"sticky path chars", ok(containImageSpec{Net: "door", Sticky: "a/b"}), "invalid sticky"},
		{"relative workdir", ok(containImageSpec{Workdir: "testbed"}), "absolute"},
		{"ro ok", ok(containImageSpec{RO: []string{"/opt/py:/opt/py"}}), ""},
		{"ro relative host", ok(containImageSpec{RO: []string{"py:/opt/py"}}), "HOST:CTR"},
		{"ro with mode", ok(containImageSpec{RO: []string{"/a:/b:rw"}}), "HOST:CTR"},
		{"ro over bashy", ok(containImageSpec{RO: []string{"/tmp/x:/.bashy/bashy"}}), "reserved"},
		{"ro over out", ok(containImageSpec{RO: []string{"/tmp/x:/out"}}), "reserved"},
		{"env value refused", ok(containImageSpec{Env: []string{"A=1"}}), "NAME"},
		{"no command", &containImageSpec{Image: testDigestRef}, "no command"},
	} {
		err := tc.spec.validate()
		switch {
		case tc.want == "" && err != nil:
			t.Errorf("%s: unexpected error %v", tc.name, err)
		case tc.want != "" && (err == nil || !strings.Contains(err.Error(), tc.want)):
			t.Errorf("%s: error %v, want it to mention %q", tc.name, err, tc.want)
		}
	}
	s := ok(containImageSpec{})
	_ = s.validate()
	if s.Net != "deny" {
		t.Errorf("default net = %q, want deny", s.Net)
	}
}

func TestDispatchContainImageFlags(t *testing.T) {
	for _, args := range [][]string{
		{"--workdir", "/testbed", "--net", "deny", "--", "true"}, // image-only flag without --image
		{"--image", "alpine:latest", "--", "true"},               // a tag never reaches podman
		{"--image", testDigestRef, "--provider", "native", "--", "true"},
		{"--image", testDigestRef, "--net", "door", "--", "true"}, // door without a sticky key
	} {
		if code := dispatchContain(args); code != 2 {
			t.Errorf("dispatchContain(%q) = %d, want 2", args, code)
		}
	}
	if code := dispatchContain([]string{"pin"}); code != 2 {
		t.Errorf("contain pin without an image = %d, want 2", code)
	}
}

func TestContainRunArgs(t *testing.T) {
	t.Setenv("CONTAIN_TEST_PASS", "yes")
	s := &containImageSpec{Image: testDigestRef, Workdir: "/testbed", Out: "/tmp/out", Net: "door", Sticky: "arm-x",
		Env: []string{"CONTAIN_TEST_PASS", "CONTAIN_TEST_UNSET"}, RO: []string{"/opt/tc:/opt/tc"}, Argv: []string{"bashy", "-c", "echo hi"}}
	args := containRunArgs(s, "bashy-contain-t", "/usr/local/bin/bashy", "/tmp/run")
	line := strings.Join(args, " ")
	for _, want := range []string{
		"podman run --rm -i --name bashy-contain-t --network=none",
		"--entrypoint /.bashy/bashy",
		"-v /usr/local/bin/bashy:/.bashy/bashy:ro",
		"-v /tmp/run:/.bashy/run",
		"-v /tmp/out:/out",
		"-v /opt/tc:/opt/tc:ro",
		"-w /testbed",
		"-e CONTAIN_TEST_PASS=yes",
		"-e OPENAI_BASE_URL=http://127.0.0.1:24556/sticky/arm-x/v1",
		"-e OPENAI_API_KEY=contained",
		testDigestRef + " contain --init --door-sock /.bashy/run/door.sock -- bashy -c echo hi",
	} {
		if !strings.Contains(line, want) {
			t.Errorf("run args lack %q:\n%s", want, line)
		}
	}
	if strings.Contains(line, "CONTAIN_TEST_UNSET") {
		t.Error("an unset --env name must not be passed")
	}
	deny := containRunArgs(&containImageSpec{Image: testDigestRef, Net: "deny", Argv: []string{"true"}}, "n", "/b", "")
	if l := strings.Join(deny, " "); strings.Contains(l, "OPENAI") || strings.Contains(l, "door-sock") || strings.Contains(l, "/.bashy/run") {
		t.Errorf("net deny must carry no door: %s", l)
	}
}

func TestDoorAllowed(t *testing.T) {
	for _, tc := range []struct {
		method, path string
		ok           bool
	}{
		{"POST", "/sticky/k1/v1/chat/completions", true},
		{"POST", "/sticky/k1/openai/v1/chat/completions", true},
		{"POST", "/sticky/k1/anthropic/v1/messages", true},
		{"POST", "/sticky/k1/api/chat", true},
		{"GET", "/sticky/k1/v1/models", true},
		{"GET", "/sticky/k1/health", true},
		{"POST", "/sticky/k2/v1/chat/completions", false}, // another binding
		{"POST", "/v1/chat/completions", false},           // unbound
		{"POST", "/sticky/k1/v1/sticky", false},           // the binding API
		{"GET", "/sticky/k1/v1/sticky/k1", false},
		{"POST", "/sticky/k1/k/tok/v1/chat/completions", false}, // a token of its own
		{"POST", "/sticky/k1/cligw/v1/chat/completions", false},
		{"POST", "/sticky/k1/api/pull", false},
		{"DELETE", "/sticky/k1/v1/models", false},
	} {
		if _, ok := doorAllowed(tc.method, tc.path, "k1"); ok != tc.ok {
			t.Errorf("doorAllowed(%s %s) = %v, want %v", tc.method, tc.path, ok, tc.ok)
		}
	}
}

func TestDoorProxyInjectsTokenAndFilters(t *testing.T) {
	var gotPath, gotAuth, gotKey string
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotAuth, gotKey = r.URL.Path, r.Header.Get("Authorization"), r.Header.Get("x-api-key")
		w.Header().Set("X-Bashy-Identity", "abc")
		_, _ = io.WriteString(w, "ok")
	}))
	defer up.Close()
	u, _ := url.Parse(up.URL)
	proxy := httptest.NewServer(newDoorProxy(u, "OWNER", "k1"))
	defer proxy.Close()

	req, _ := http.NewRequest("POST", proxy.URL+"/sticky/k1/v1/chat/completions", strings.NewReader("{}"))
	req.Header.Set("Authorization", "Bearer contained")
	req.Header.Set("x-api-key", "contained")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 || string(body) != "ok" || resp.Header.Get("X-Bashy-Identity") != "abc" {
		t.Fatalf("allowed request: %s %q identity %q", resp.Status, body, resp.Header.Get("X-Bashy-Identity"))
	}
	if gotPath != "/sticky/k1/v1/chat/completions" || gotAuth != "Bearer OWNER" || gotKey != "" {
		t.Errorf("upstream saw path %q auth %q x-api-key %q", gotPath, gotAuth, gotKey)
	}

	gotPath = ""
	resp, err = http.Post(proxy.URL+"/v1/sticky", "application/json", strings.NewReader("{}"))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden || gotPath != "" {
		t.Errorf("binding API: %s, upstream path %q (must be refused before the door)", resp.Status, gotPath)
	}

	// A /k/<token> upstream carries its own credential: no bearer is added.
	u2, _ := url.Parse(up.URL + "/k/TOK")
	proxy2 := httptest.NewServer(newDoorProxy(u2, "", "k1"))
	defer proxy2.Close()
	resp, err = http.Get(proxy2.URL + "/sticky/k1/v1/models")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if gotPath != "/k/TOK/sticky/k1/v1/models" || gotAuth != "" {
		t.Errorf("token-path upstream saw path %q auth %q", gotPath, gotAuth)
	}
}

func TestRelayToUnix(t *testing.T) {
	dir, err := os.MkdirTemp("", "bcr")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	sock := filepath.Join(dir, "s")
	uln, err := net.Listen("unix", sock)
	if err != nil {
		t.Skipf("unix sockets unavailable: %v", err)
	}
	defer uln.Close()
	go func() {
		_ = http.Serve(uln, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, "via "+r.URL.Path) }))
	}()
	tln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer tln.Close()
	go relayToUnix(tln, sock)
	resp, err := http.Get("http://" + tln.Addr().String() + "/sticky/k/v1/models")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if string(body) != "via /sticky/k/v1/models" {
		t.Errorf("relay answered %q", body)
	}
}

func TestContainScriptFunctions(t *testing.T) {
	dump := strings.Join([]string{
		"agent () ",
		"{ ",
		"    command '/Users/x/.local/bin/bashy.real' agent \"$@\"",
		"}",
		"helper () ",
		"{ ",
		"    echo \"helper $1\"",
		"}",
		"@effects(\"read,write,exec\")",
		"@contain(image: \"x@sha256:00\", net: \"door\", sticky: \"k\")",
		"arm () ",
		"{ ",
		"    helper \"$1\";",
		"    python3 -c 'print(1)'",
		"}",
		"@contain(net: \"deny\")",
		"tests () ",
		"{ ",
		"    python -m pytest",
		"}",
		"",
	}, "\n")
	got := containScriptFunctions(dump, "arm")
	for _, want := range []string{"helper () ", "echo \"helper $1\"", "arm () ", "python3 -c 'print(1)'", "@contain(net: \"deny\")\ntests () "} {
		if !strings.Contains(got, want) {
			t.Errorf("script lacks %q:\n%s", want, got)
		}
	}
	for _, bad := range []string{"bashy.real", "agent () ", "@contain(image:", "@effects"} {
		if strings.Contains(got, bad) {
			t.Errorf("script keeps %q:\n%s", bad, got)
		}
	}
}

func TestContainInjectedBashy(t *testing.T) {
	notELF := filepath.Join(t.TempDir(), "bashy")
	if err := os.WriteFile(notELF, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("BASHY_CONTAIN_BASHY", notELF)
	if _, err := containInjectedBashy("amd64"); err == nil {
		t.Error("a non-ELF injected bashy must be refused")
	}
	if runtime.GOOS != "linux" {
		t.Setenv("BASHY_CONTAIN_BASHY", "")
		if _, err := containInjectedBashy("amd64"); err == nil || !strings.Contains(err.Error(), "BASHY_CONTAIN_BASHY") {
			t.Errorf("a non-linux host without BASHY_CONTAIN_BASHY: %v", err)
		}
	}
}
