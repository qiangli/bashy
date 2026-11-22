// Test-only Outpost command fixture. Browser tests use separate HTTP sockets
// and the production proxy on both sides. A real mesh rehearsal is independent.
package main

import (
	"encoding/json"
	"fmt"
	"os"
)

const peer = "TestDemoQueuePeer000000000000000000000000000000"

func main() {
	if len(os.Args) < 3 || os.Args[1] != "mesh" {
		os.Exit(2)
	}
	state := os.Getenv("DEMO_QUEUE_TEST_MESH")
	var err error
	switch os.Args[2] {
	case "status":
		err = json.NewEncoder(os.Stdout).Encode(map[string]any{"status": map[string]any{
			"peer_id": peer, "peers": []map[string]any{{"id": peer, "direct": true}},
		}})
	case "expose":
		err = os.WriteFile(state, []byte(os.Args[4]), 0600)
	case "listen":
		var b []byte
		b, err = os.ReadFile(state)
		if err == nil {
			_, err = fmt.Fprintln(os.Stdout, string(b))
		}
	case "unexpose":
		err = os.Remove(state)
	case "unlisten":
	default:
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
