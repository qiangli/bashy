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

// Each browser has its own hub port, 58081 + (first letter - 'b'): brave
// 58081, chrome 58082 (browser-hub; $BASHY_BROWSER_HUB_PORT), edge 58084
// (browser-hub-edge; $BASHY_BROWSER_EDGE_HUB_PORT).
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
	return "58084"
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

// hubPorts are every browser's hub slot (Brave 58081, Chrome, Edge); only
// the ones that answer count.
func hubPorts() []string {
	brave := strings.TrimSpace(os.Getenv("BASHY_BROWSER_BRAVE_HUB_PORT"))
	if brave == "" {
		brave = "58081"
	}
	return []string{brave, hubPort(), edgeHubPort()}
}

// readAllHubs folds every running hub into one state for the single tray
// item: on when every running hub is open, off when any is paused,
// not running when none answers.
func readAllHubs() hubState {
	running, paused, noGate := 0, 0, 0
	for _, p := range hubPorts() {
		switch readHubGateAt(p) {
		case hubOn:
			running++
		case hubOff:
			running++
			paused++
		case hubNoGate:
			running++
			noGate++
		}
	}
	switch {
	case running == 0:
		return hubDown
	case paused > 0:
		return hubOff
	case noGate == running:
		return hubNoGate
	default:
		return hubOn
	}
}

// setAllHubs pauses or resumes every running hub; per-browser control is
// `bashy browser --browser edge hub pause|resume` (or outpost).
func setAllHubs(paused bool) error {
	var errs []error
	for _, p := range hubPorts() {
		if s := readHubGateAt(p); s == hubOn || s == hubOff {
			if err := setHubGateAt(p, paused); err != nil {
				errs = append(errs, fmt.Errorf("port %s: %w", p, err))
			}
		}
	}
	return errors.Join(errs...)
}
