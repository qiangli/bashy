package agentos

import (
	"context"
	"errors"
	"os/exec"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func TestParseConnectArgs(t *testing.T) {
	for _, tt := range []struct {
		name string
		args []string
		want connectOptions
	}{
		{"shell", []string{"elephant"}, connectOptions{host: "elephant"}},
		{"user and port", []string{"dev@elephant:2222"}, connectOptions{host: "dev@elephant:2222"}},
		{"native before host", []string{"--native", "elephant"}, connectOptions{host: "elephant", native: true}},
		{"native after host", []string{"elephant", "--native"}, connectOptions{host: "elephant", native: true}},
		{"command", []string{"elephant", "--", "codex"}, connectOptions{host: "elephant", command: []string{"codex"}}},
		{"opaque arguments", []string{"elephant", "--", "printf", "", "a b", "'quoted'", "$(echo no)", "--help", "--native"}, connectOptions{host: "elephant", command: []string{"printf", "", "a b", "'quoted'", "$(echo no)", "--help", "--native"}}},
		{"help", []string{"--help"}, connectOptions{help: true}},
		{"short help", []string{"-h"}, connectOptions{help: true}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseConnectArgs(tt.args)
			if err != nil || !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("parseConnectArgs(%q) = %#v, %v; want %#v", tt.args, got, err, tt.want)
			}
		})
	}
}

func TestConnectTransportInvocation(t *testing.T) {
	for _, host := range []string{"elephant", "dev@elephant:2222", "list"} {
		t.Run(host, func(t *testing.T) {
			calls := 0
			ctx := context.WithValue(context.Background(), struct{}{}, host)
			code := runConnect(ctx, []string{host}, func(gotCtx context.Context, args ...string) error {
				calls++
				if gotCtx != ctx || !slices.Equal(args, []string{"ssh", "--", host}) {
					t.Fatalf("transport received context %v, argv %q", gotCtx, args)
				}
				return nil
			})
			if code != 0 || calls != 1 {
				t.Fatalf("exit = %d, calls = %d", code, calls)
			}
		})
	}
}

func TestConnectForwardsRemoteCommand(t *testing.T) {
	for _, tt := range []struct {
		name string
		host string
		cmd  []string
	}{
		{"host", "elephant", []string{"uptime"}},
		{"user at host", "dev@elephant", []string{"df", "-h", "/tmp"}},
		{"user at host with port", "dev@elephant:2222", []string{"printf", "a b", "codex"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			calls := 0
			args := append([]string{tt.host, "--"}, tt.cmd...)
			want := append([]string{"ssh", "--", tt.host}, tt.cmd...)
			code := runConnect(context.Background(), args, func(_ context.Context, got ...string) error {
				calls++
				if !slices.Equal(got, want) {
					t.Fatalf("transport argv = %q, want %q", got, want)
				}
				return nil
			})
			if code != 0 || calls != 1 {
				t.Fatalf("exit = %d, calls = %d", code, calls)
			}
		})
	}
}

func TestConnectRemoteFlagsStayRemote(t *testing.T) {
	for _, remote := range [][]string{{"--help"}, {"--native"}, {"-x"}, {"--help", "--native", "-x"}} {
		t.Run(strings.Join(remote, ","), func(t *testing.T) {
			calls := 0
			args := append([]string{"elephant", "--"}, remote...)
			want := append([]string{"ssh", "--", "elephant"}, remote...)
			code := runConnect(context.Background(), args, func(_ context.Context, got ...string) error {
				calls++
				if !slices.Equal(got, want) {
					t.Fatalf("transport argv = %q, want %q", got, want)
				}
				return nil
			})
			if code != 0 || calls != 1 {
				t.Fatalf("exit = %d, calls = %d", code, calls)
			}
		})
	}
}

func TestConnectPropagatesRemoteExitStatus(t *testing.T) {
	probe := exec.Command("sh", "-c", "exit 42")
	remoteErr := probe.Run()
	var exitErr *exec.ExitError
	if !errors.As(remoteErr, &exitErr) {
		t.Skipf("could not produce a real *exec.ExitError: %v", remoteErr)
	}
	calls := 0
	code := runConnect(context.Background(), []string{"elephant", "--", "uptime"}, func(_ context.Context, got ...string) error {
		calls++
		if !slices.Equal(got, []string{"ssh", "--", "elephant", "uptime"}) {
			t.Fatalf("transport argv = %q", got)
		}
		return remoteErr
	})
	if code != 42 || calls != 1 {
		t.Fatalf("exit = %d, calls = %d; want exit 42", code, calls)
	}
}

func TestConnectNativeStillRefused(t *testing.T) {
	for _, args := range [][]string{{"elephant", "--native"}, {"--native", "elephant"}, {"elephant", "--native", "--", "uptime"}, {"dev@elephant:2222", "--native"}} {
		stderr := captureStderr(t, func() {
			code := runConnect(context.Background(), args, func(context.Context, ...string) error {
				t.Fatal("invalid or unsupported request reached transport")
				return nil
			})
			if code != 2 {
				t.Errorf("exit = %d, want 2", code)
			}
		})
		if !strings.Contains(stderr, "bashy connect:") || !strings.Contains(stderr, "--native") {
			t.Errorf("missing --native diagnostic: %q", stderr)
		}
	}
}

func TestConnectTransportFailure(t *testing.T) {
	stderr := captureStderr(t, func() {
		code := runConnect(context.Background(), []string{"elephant"}, func(context.Context, ...string) error {
			return errors.New("fake transport unavailable")
		})
		if code != 1 {
			t.Errorf("exit = %d, want 1", code)
		}
	})
	if !strings.Contains(stderr, "fake transport unavailable") {
		t.Fatalf("missing transport error: %q", stderr)
	}
}

func TestConnectCatalogAndHelp(t *testing.T) {
	_, _, verbs := commandsCatalog()
	if !slices.Contains(verbs, "connect") || !isFrontDoorInvocation("connect") {
		t.Fatal("connect must be catalogued and dispatched")
	}
	if _, ok := lookupEntry("connect"); !ok {
		t.Fatal("connect must have an atlas entry")
	}
	if strings.Count(connectHelp, "\n  bashy connect ") != 3 {
		t.Fatal("help must include exactly three examples")
	}
	for _, flag := range []string{"--help", "-h"} {
		out := captureStdout(t, func() {
			code := runConnect(context.Background(), []string{flag}, func(context.Context, ...string) error {
				t.Fatal("help reached transport")
				return nil
			})
			if code != 0 {
				t.Errorf("help exit = %d", code)
			}
		})
		if out != connectHelp {
			t.Errorf("help output = %q", out)
		}
	}
}

func TestParseConnectArgsRejectsInvalid(t *testing.T) {
	for _, args := range [][]string{
		nil, {""}, {" "}, {"bad\nhost"}, {"--native"}, {"--wat"},
		{"--", "codex"}, {"elephant", "--"}, {"elephant", "--", ""},
		{"elephant", "codex"}, {"elephant", "--tty"},
	} {
		if _, err := parseConnectArgs(args); err == nil {
			t.Errorf("parseConnectArgs(%q) succeeded", args)
		}
	}
}
