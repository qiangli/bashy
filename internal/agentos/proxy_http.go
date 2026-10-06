package agentos

import (
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"

	"github.com/qiangli/bashy/internal/httpproxy"
	"github.com/spf13/cobra"
)

func newHTTPProxyCmd() *cobra.Command {
	var listen, auth string
	cmd := &cobra.Command{
		Use: "http", Short: "Run an HTTP forward proxy with HTTPS CONNECT tunnelling",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			logger := slog.New(slog.NewJSONHandler(cmd.ErrOrStderr(), nil))
			cfg := httpproxy.Config{Auth: auth, Logger: logger}
			// Validate configuration before binding the listener.
			h, err := httpproxy.New(cfg)
			if err != nil {
				return err
			}
			h.Close()
			listener, err := net.Listen("tcp", listen)
			if err != nil {
				return err
			}
			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			logger.Info("http proxy listening", "listen", listener.Addr().String(), "auth", auth != "")
			return httpproxy.Serve(ctx, listener, cfg)
		},
	}
	cmd.Flags().StringVar(&listen, "listen", "127.0.0.1:8080", "TCP listen address")
	cmd.Flags().StringVar(&auth, "auth", "", "optional proxy credentials (user:pass)")
	return cmd
}
