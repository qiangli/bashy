package agentos

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

func modelQuotaCaptureCommand(clock func() time.Time) *cobra.Command {
	var path, account string
	cmd := &cobra.Command{Use: "capture --to FILE", Short: "Capture JSON for a quota probe or Claude statusline", SilenceUsage: true, Args: cobra.NoArgs, Long: `Read up to 1 MiB of JSON from stdin and atomically save a private quota capture.
Write errors are logged to stderr and do not fail the statusline. Claude statusline payloads echo the model display name and 5h used percentage when present.

Register in Claude settings.json:
  "statusLine": {"type":"command","command":"bashy model quota capture --to ~/.bashy/quota/claude.json"}

Fields: schema_version (bashy-claude-statusline-v1), observed_at (RFC3339), account, payload (input JSON).`}
	cmd.Flags().StringVar(&path, "to", "", "destination file")
	cmd.Flags().StringVar(&account, "account", "default", "account name")
	_ = cmd.MarkFlagRequired("to")
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		raw, err := io.ReadAll(io.LimitReader(cmd.InOrStdin(), (1<<20)+1))
		if err != nil || len(raw) > 1<<20 || !json.Valid(raw) {
			fmt.Fprintln(cmd.ErrOrStderr(), "quota capture: expected valid JSON no larger than 1 MiB")
			return nil
		}
		if line := quotaStatusline(raw); line != "" {
			fmt.Fprintln(cmd.OutOrStdout(), line)
		}
		if account == "" {
			account = "default"
		}
		if err = writeQuotaCapture(path, account, raw, clock()); err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "quota capture: %v\n", err)
		}
		return nil
	}
	return cmd
}
func writeQuotaCapture(path, account string, raw json.RawMessage, now time.Time) error {
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		path = filepath.Join(home, path[2:])
	}
	data, err := json.Marshal(struct {
		Schema   string          `json:"schema_version"`
		Observed string          `json:"observed_at"`
		Account  string          `json:"account"`
		Payload  json.RawMessage `json:"payload"`
	}{"bashy-claude-statusline-v1", now.Format(time.RFC3339), account, raw})
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err = os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".quota-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	// CreateTemp opens with mode 0600; rename replaces any prior inode.
	if _, err = f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}
func quotaStatusline(raw []byte) string {
	var payload struct {
		Model struct {
			DisplayName string `json:"display_name"`
		} `json:"model"`
		RateLimits struct {
			FiveHour struct {
				UsedPercent *float64 `json:"used_percentage"`
			} `json:"five_hour"`
		} `json:"rate_limits"`
	}
	if json.Unmarshal(raw, &payload) != nil || payload.Model.DisplayName == "" {
		return ""
	}
	name := strings.Join(strings.Fields(payload.Model.DisplayName), " ")
	if p := payload.RateLimits.FiveHour.UsedPercent; p != nil {
		name += fmt.Sprintf(" | 5h %.0f%%", *p)
	}
	return name
}
