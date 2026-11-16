package main

// The "Browser hub" item (Sprint 529, S529.10). The browser hub runs as
// outpost's browser-hub service; on/off is the hub's LOGICAL gate, so the tray
// still never starts, stops or restarts a process. It reads GET /gate and
// flips it with POST /gate, authorised by the control token the hub keeps in
// $BASHY_HOME/browser/hub-control.token (the same file `bashy browser hub
// pause|resume` uses).

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// hubPort is the Chrome hub's loopback port ($BASHY_BROWSER_HUB_PORT, default
// 58082, the browser-hub service); edgeHubPort is Edge's own hub
// ($BASHY_BROWSER_EDGE_HUB_PORT, default 58083, browser-hub-edge).
func hubPort() string {
	if p := strings.TrimSpace(os.Getenv("BASHY_BROWSER_HUB_PORT")); p != "" {
		return p
	}
	return "58082"
}

func edgeHubPort() string {
	if p := strings.TrimSpace(os.Getenv("BASHY_BROWSER_EDGE_HUB_PORT")); p != "" {
		return p
	}
	return "58083"
}

func hubGateToken() (string, error) {
	home := strings.TrimSpace(os.Getenv("BASHY_HOME"))
	if home == "" {
		h, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		home = filepath.Join(h, ".bashy")
	}
	b, err := os.ReadFile(filepath.Join(home, "browser", "hub-control.token"))
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(b)), nil
}

// hubState is what the item shows.
type hubState int

const (
	hubDown   hubState = iota // nothing answers on the port
	hubOn                     // gate open
	hubOff                    // gate paused
	hubNoGate                 // an older hub without the gate
)

var hubClient = &http.Client{Timeout: 2 * time.Second}

func readHubGate() hubState { return readHubGateAt(hubPort()) }

func readHubGateAt(port string) hubState {
	resp, err := hubClient.Get("http://127.0.0.1:" + port + "/gate")
	if err != nil {
		return hubDown
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return hubNoGate
	}
	var g struct {
		Paused bool `json:"paused"`
	}
	if json.NewDecoder(resp.Body).Decode(&g) != nil {
		return hubNoGate
	}
	if g.Paused {
		return hubOff
	}
	return hubOn
}

func setHubGate(paused bool) error { return setHubGateAt(hubPort(), paused) }

func setHubGateAt(port string, paused bool) error {
	tok, err := hubGateToken()
	if err != nil {
		return fmt.Errorf("no hub control token (start the browser hub once): %w", err)
	}
	body, _ := json.Marshal(map[string]any{"paused": paused, "by": "tray"})
	req, err := http.NewRequest(http.MethodPost, "http://127.0.0.1:"+port+"/gate", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("X-Bashy-Gate-Token", tok)
	req.Header.Set("Content-Type", "application/json")
	resp, err := hubClient.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return errors.New("hub refused: " + resp.Status)
	}
	return nil
}

func (s hubState) title() string { return s.titleFor("Browser hub") }

// titleFor names the hub ("Browser hub", "Edge hub") in the item text.
func (s hubState) titleFor(name string) string {
	switch s {
	case hubOn:
		return name + ": on"
	case hubOff:
		return name + ": off"
	case hubNoGate:
		return name + ": on (update bashy for on/off)"
	default:
		return name + " (not running)"
	}
}
