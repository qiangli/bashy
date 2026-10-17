package agentos

// Sprint: #455; Story: #2308; Story-ID: 140dc1006a0e
//
// The genie recipe pulls the model it picked when the host does not have it
// (lib/model-server.bsh: `bashy ollama pull`). Before this file that pull
// had no bound: a non-interactive `bashy ycode prompt` on a fresh host
// auto-picked the largest model that fit the memory and downloaded 17 GB
// into a disk that could not hold it (E1 F8). The front door now decides a
// pull policy and exports it to the recipe; bashy's own `ollama pull`
// dispatch reads it back and refuses a pull that is not allowed or would
// not fit on the disk. The recipe itself is unchanged: the guard sits in
// the one binary every genie-initiated pull goes through.

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	geniesrc "github.com/qiangli/ycode/examples/genie"
)

const (
	// geniePullPolicyEnv carries the front door's decision to the recipe and
	// on to `bashy ollama pull`.
	geniePullPolicyEnv = "GENIE_PULL_POLICY"
	// geniePullAllow: the model may be downloaded if it fits on the disk.
	geniePullAllow = "allow"
	// geniePullDeny: an auto-picked model in a non-interactive session with
	// no --pull consent is never downloaded.
	geniePullDeny = "deny"
	// geniePullDiskMarginGB is what a pull must leave free on the model
	// store's disk beyond the model's own weights: the blobs are written
	// through a partial file and the host needs working room.
	geniePullDiskMarginGB = 5.0
)

// geniePullPolicy is the front door's decision. An explicitly named model
// (-m, GENIE_MODEL_ID) is the user's own choice; --pull is explicit consent
// for whatever the pick is; a terminal session shows the download and can
// stop it. Only the remaining case — an auto-picked model nobody is
// watching — is denied.
func geniePullPolicy(explicitModel, consent, interactive bool) string {
	if explicitModel || consent || interactive {
		return geniePullAllow
	}
	return geniePullDeny
}

// genieExportPullPolicy sets the policy for the recipe this invocation runs.
func genieExportPullPolicy(explicitModel, consent bool) {
	os.Setenv(geniePullPolicyEnv, geniePullPolicy(explicitModel, consent, isTerminal(os.Stdin)))
}

// geniePullFreeBytes measures the free space backing dir (its nearest
// existing ancestor when the store is not there yet); ok is false where the
// platform cannot say. A seam so tests fake the disk.
var geniePullFreeBytes = func(dir string) (uint64, bool) {
	for {
		if _, err := os.Stat(dir); err == nil {
			free, _, ok := probeDisk(dir)
			return free, ok
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return 0, false
		}
		dir = parent
	}
}

// genieModelSizeGB is the download size of a model in genie's candidate table
// (models.json, weights_gb); ok is false for a model the table does not
// know. A seam so tests fake the table.
var genieModelSizeGB = func(model string) (float64, bool) {
	data, err := geniesrc.Source.ReadFile("models.json")
	if err != nil {
		return 0, false
	}
	var table struct {
		Models []struct {
			ID        string  `json:"id"`
			WeightsGB float64 `json:"weights_gb"`
		} `json:"models"`
	}
	if json.Unmarshal(data, &table) != nil {
		return 0, false
	}
	for _, m := range table.Models {
		if m.ID == model && m.WeightsGB > 0 {
			return m.WeightsGB, true
		}
	}
	return 0, false
}

// genieModelStoreDir is bashy's own ollama model store, the same resolution
// as the managed engine's (external/ollama ManagedModelsDir): $OLLAMA_MODELS,
// else $BASHY_OLLAMA_DIR/models, else ~/.agents/bashy/ollama/models. Mirrored
// here because that package is linked only into the engine build.
func genieModelStoreDir() string {
	if d := strings.TrimSpace(os.Getenv("OLLAMA_MODELS")); d != "" {
		return d
	}
	if d := strings.TrimSpace(os.Getenv("BASHY_OLLAMA_DIR")); d != "" {
		return filepath.Join(d, "models")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.TempDir(), "bashy-ollama", "models")
	}
	return filepath.Join(home, ".agents", "bashy", "ollama", "models")
}

// genieGuardOllamaPull runs before `bashy ollama ARGS` is dispatched. Outside
// a genie session (no policy in the environment) or for any verb but pull it
// does nothing. Returns the exit status to stop with, 0 to proceed.
func genieGuardOllamaPull(args []string, stderr io.Writer) int {
	policy := strings.TrimSpace(os.Getenv(geniePullPolicyEnv))
	if policy == "" || len(args) == 0 || args[0] != "pull" {
		return 0
	}
	model := ""
	for _, arg := range args[1:] {
		if !strings.HasPrefix(arg, "-") {
			model = arg
			break
		}
	}
	if policy != geniePullAllow {
		fmt.Fprintf(stderr, "genie: model %s is not on this host and nobody is at the terminal to watch a download: run again with --pull to download it, or with -m NAME to use a model already pulled (bashy ollama list)\n", model)
		return 2
	}
	size, sized := genieModelSizeGB(model)
	if !sized {
		return 0
	}
	free, ok := geniePullFreeBytes(genieModelStoreDir())
	if !ok {
		return 0
	}
	freeGB := float64(free) / 1e9
	if need := size + geniePullDiskMarginGB; freeGB < need {
		fmt.Fprintf(stderr, "genie: not pulling %s: it needs about %.0f GB plus %.0f GB of working room, and the model store's disk has %.0f GB free; free some space, or run with -m NAME to use a model already pulled (bashy ollama list)\n", model, size, geniePullDiskMarginGB, freeGB)
		return 2
	}
	return 0
}
