package kit

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// AppRuntime is the console's address, private data directory, and browser
// URL for one locally managed app instance.
type AppRuntime struct {
	Addr string `json:"addr"`
	Data string `json:"data"`
	URL  string `json:"url"`
}

// DiscoverAppRuntime asks the local console for the operator's app instance.
func DiscoverAppRuntime(app string) (info AppRuntime, retErr error) {
	env := strings.ToUpper(app)
	defer func() {
		if retErr != nil && !strings.Contains(retErr.Error(), env+"_ADDR") {
			retErr = fmt.Errorf("%w; open the app in the console or pass %s_ADDR and %s_DATA", retErr, env, env)
		}
	}()
	bin := os.Getenv("BASHY_BIN")
	if bin == "" {
		bin = "bashy"
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, "app", "show", app, "--runtime", "--json")
	out, err := cmd.Output()
	if ctx.Err() != nil {
		return AppRuntime{}, ctx.Err()
	}
	if err != nil {
		if failure, ok := err.(*exec.ExitError); ok && len(failure.Stderr) > 0 {
			return AppRuntime{}, fmt.Errorf("%s", strings.Join(strings.Fields(string(failure.Stderr)), " "))
		}
		return AppRuntime{}, err
	}

	if err := json.Unmarshal(out, &info); err != nil {
		return AppRuntime{}, fmt.Errorf("decode app runtime: %w", err)
	}
	if err := CheckLoopback(info.Addr); err != nil {
		return AppRuntime{}, err
	}
	if !filepath.IsAbs(info.Data) || info.URL == "" {
		return AppRuntime{}, fmt.Errorf("incomplete app runtime handoff")
	}
	if err := SetPublicMount(&http.Request{Header: make(http.Header)}, info.URL); err != nil {
		return AppRuntime{}, err
	}
	return info, nil
}
