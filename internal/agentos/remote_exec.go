// Copyright (c) 2026 qiangli
// See LICENSE for licensing information.

package agentos

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/qiangli/outpost/pkg/sshclient"
	"github.com/spf13/cobra"
)

func remoteExecCmd() *cobra.Command {
	var proxyURL, listen, noProxy, user string
	cmd := &cobra.Command{
		Use:   "exec <host> <command>",
		Short: "Run a command over the installed peer channel with optional reverse proxy egress",
		Example: `  bashy proxy http --listen 127.0.0.1:8080
  bashy remote exec --proxy http://127.0.0.1:8080 host 'curl https://example.com'`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if proxyURL != "" {
				if _, err := parseOperatorProxy(proxyURL); err != nil {
					return err
				}
			}
			ctx, cancel := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			defer cancel()
			ch, err := DialInstalledPeerChannel(ctx, args[0], user)
			if err != nil {
				return err
			}
			defer ch.Close()
			exec := ch.Exec
			if proxyURL != "" {
				p, err := ch.ReverseProxy(ctx, proxyURL, listen, noProxy)
				if err != nil {
					return err
				}
				defer p.Close()
				exec = p.Exec
			}
			result, err := exec(ctx, sshclient.ExecOptions{Command: args[1], Stdin: cmd.InOrStdin()})
			if err != nil {
				return err
			}
			if _, err := cmd.OutOrStdout().Write(result.Stdout); err != nil {
				return err
			}
			if _, err := cmd.ErrOrStderr().Write(result.Stderr); err != nil {
				return err
			}
			if result.StdoutTruncated || result.StderrTruncated {
				return fmt.Errorf("remote command output exceeded capture limit")
			}
			if result.ExitCode != 0 {
				return fmt.Errorf("remote command exited with status %d", result.ExitCode)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&proxyURL, "proxy", "", "Operator loopback proxy URL (http or socks5h)")
	cmd.Flags().StringVar(&listen, "proxy-listen", "127.0.0.1:0", "Remote loopback address for reverse egress")
	cmd.Flags().StringVar(&noProxy, "no-proxy", "localhost,127.0.0.1,::1", "Remote NO_PROXY bypass list")
	cmd.Flags().StringVar(&user, "user", "", "Remote peer user (defaults to host user or local USER)")
	return cmd
}
