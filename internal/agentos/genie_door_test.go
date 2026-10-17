// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package agentos

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/qiangli/yoke/pkg/broker/door"
)

func pointDoorAt(t *testing.T, port int) {
	t.Setenv("BASHY_HOME", t.TempDir())
	t.Setenv(door.PortEnv, strconv.Itoa(port))
}

func TestGenieDoorStatusUp(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"pid":4242,"version":"v9.9","local_engine":true,"engine_up":true}`))
	})
	mux.HandleFunc("/v1/models", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":[{"x_kind":"local"},{"x_kind":"local"},{"x_kind":"agent"}]}`))
	})
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewUnstartedServer(mux)
	srv.Listener.Close()
	srv.Listener = ln
	srv.Start()
	t.Cleanup(srv.Close)
	pointDoorAt(t, ln.Addr().(*net.TCPAddr).Port)

	line, st := genieDoorStatus(context.Background())
	if !st.Up || st.LocalModels != 2 || st.AgentEntries != 1 {
		t.Fatalf("status: %+v", st)
	}
	for _, want := range []string{"up at 127.0.0.1:", "pid 4242", "version v9.9", "ollama-local present=true healthy=true", "2 local models", "1 agent entries"} {
		if !strings.Contains(line, want) {
			t.Errorf("door line missing %q: %s", want, line)
		}
	}
}

func TestGenieDoorStatusDown(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	pointDoorAt(t, port)

	start := time.Now()
	line, st := genieDoorStatus(context.Background())
	if time.Since(start) > 2*time.Second {
		t.Fatalf("down probe took %s", time.Since(start))
	}
	if st.Up || st.Error == "" {
		t.Fatalf("status: %+v", st)
	}
	if !strings.HasPrefix(line, "down: "+st.Error) || !strings.Contains(line, "bashy llm up") || !strings.Contains(line, "bashy llm up --service") {
		t.Fatalf("down line: %s", line)
	}
}
