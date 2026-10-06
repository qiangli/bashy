package agentos

import (
	"context"
	"io"
	"net"
	"strconv"
	"testing"
	"time"

	"golang.org/x/net/proxy"
)

func TestSOCKS5Interop(t *testing.T) {
	for _, tc := range []struct {
		name, user, pass, target string
	}{
		{"no auth", "", "", "127.0.0.1"},
		{"username password", "alice", "secret", "127.0.0.1"},
		{"remote DNS", "", "", "localhost"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			echo, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			defer echo.Close()
			go func() {
				c, err := echo.Accept()
				if err == nil {
					defer c.Close()
					io.Copy(c, c)
				}
			}()
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			seen := make(chan string, 1)
			dial := func(ctx context.Context, network, address string) (net.Conn, error) {
				seen <- address
				return (&net.Dialer{}).DialContext(ctx, network, address)
			}
			done := make(chan error, 1)
			go func() { done <- serveSOCKS5(ctx, listener, tc.user, tc.pass, dial, nil) }()
			var auth *proxy.Auth
			if tc.user != "" {
				auth = &proxy.Auth{User: tc.user, Password: tc.pass}
			}
			client, err := proxy.SOCKS5("tcp", listener.Addr().String(), auth, proxy.Direct)
			if err != nil {
				t.Fatal(err)
			}
			port := strconv.Itoa(echo.Addr().(*net.TCPAddr).Port)
			c, err := client.Dial("tcp", net.JoinHostPort(tc.target, port))
			if err != nil {
				t.Fatal(err)
			}
			defer c.Close()
			c.SetDeadline(time.Now().Add(2 * time.Second))
			if _, err := c.Write([]byte("ping")); err != nil {
				t.Fatal(err)
			}
			buf := make([]byte, 4)
			if _, err := io.ReadFull(c, buf); err != nil {
				t.Fatal(err)
			}
			if string(buf) != "ping" {
				t.Fatalf("echo = %q", buf)
			}
			if got := <-seen; got != net.JoinHostPort(tc.target, port) {
				t.Fatalf("dialed %q", got)
			}
			cancel()
			select {
			case err := <-done:
				if err != nil {
					t.Fatal(err)
				}
			case <-time.After(2 * time.Second):
				t.Fatal("server did not stop")
			}
		})
	}
}

func TestSOCKS5RejectsUnsupportedAndBadCredentials(t *testing.T) {
	for _, tc := range []struct {
		name, user, pass   string
		method, wantMethod byte
		command, wantReply byte
	}{
		{"requires password auth", "alice", "secret", 0, 255, 0, 0},
		{"rejects BIND", "", "", 0, 0, 2, 7},
		{"rejects UDP", "", "", 0, 0, 3, 7},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server, client := net.Pipe()
			defer client.Close()
			go func() { defer server.Close(); handleSOCKS5(context.Background(), server, tc.user, tc.pass, nil) }()
			client.SetDeadline(time.Now().Add(time.Second))
			if _, err := client.Write([]byte{5, 1, tc.method}); err != nil {
				t.Fatal(err)
			}
			method := make([]byte, 2)
			if _, err := io.ReadFull(client, method); err != nil {
				t.Fatal(err)
			}
			if method[0] != 5 || method[1] != tc.wantMethod {
				t.Fatalf("method reply %v", method)
			}
			if tc.wantMethod == 255 {
				return
			}
			if _, err := client.Write([]byte{5, tc.command, 0, 1}); err != nil {
				t.Fatal(err)
			}
			reply := make([]byte, 10)
			if _, err := io.ReadFull(client, reply); err != nil {
				t.Fatal(err)
			}
			if reply[1] != tc.wantReply {
				t.Fatalf("reply %v", reply)
			}
		})
	}
}

func TestSOCKS5WrongPassword(t *testing.T) {
	server, client := net.Pipe()
	defer client.Close()
	go func() { defer server.Close(); handleSOCKS5(context.Background(), server, "alice", "secret", nil) }()
	client.SetDeadline(time.Now().Add(time.Second))
	client.Write([]byte{5, 1, 2})
	method := make([]byte, 2)
	if _, err := io.ReadFull(client, method); err != nil {
		t.Fatal(err)
	}
	client.Write([]byte{1, 5, 'a', 'l', 'i', 'c', 'e', 3, 'b', 'a', 'd'})
	status := make([]byte, 2)
	if _, err := io.ReadFull(client, status); err != nil {
		t.Fatal(err)
	}
	if status[0] != 1 || status[1] != 1 {
		t.Fatalf("auth status %v", status)
	}
}
