package agentos

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/spf13/cobra"
)

type socksDialer func(context.Context, string, string) (net.Conn, error)

func newProxyCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "proxy", Short: "Run a local network proxy", Args: cobra.NoArgs}
	cmd.AddCommand(newSOCKS5Cmd())
	return cmd
}

func newSOCKS5Cmd() *cobra.Command {
	var listen, username, password string
	cmd := &cobra.Command{
		Use: "socks", Short: "Run a SOCKS5 CONNECT proxy",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if (username == "") != (password == "") {
				return errors.New("--username and --password must be supplied together")
			}
			if len(username) > 255 || len(password) > 255 {
				return errors.New("username and password must be at most 255 bytes")
			}
			listener, err := net.Listen("tcp", listen)
			if err != nil {
				return err
			}
			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			logger := slog.New(slog.NewJSONHandler(cmd.ErrOrStderr(), nil))
			logger.Info("socks5 listening", "listen", listener.Addr().String(), "auth", username != "")
			return serveSOCKS5(ctx, listener, username, password, (&net.Dialer{}).DialContext, logger)
		},
	}
	cmd.Flags().StringVar(&listen, "listen", "127.0.0.1:1080", "TCP listen address")
	cmd.Flags().StringVar(&username, "username", "", "SOCKS5 username (requires --password)")
	cmd.Flags().StringVar(&password, "password", "", "SOCKS5 password (requires --username)")
	return cmd
}

func serveSOCKS5(ctx context.Context, listener net.Listener, username, password string, dial socksDialer, logger *slog.Logger) error {
	defer listener.Close()
	go func() { <-ctx.Done(); listener.Close() }()
	for {
		client, err := listener.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		go func() {
			defer client.Close()
			if logger != nil {
				logger.Info("socks5 client connected", "client", client.RemoteAddr().String())
			}
			closeOnCancel := make(chan struct{})
			go func() {
				select {
				case <-ctx.Done():
					client.Close()
				case <-closeOnCancel:
				}
			}()
			defer close(closeOnCancel)
			if err := handleSOCKS5(ctx, client, username, password, dial); err != nil && logger != nil {
				logger.Warn("socks5 connection", "client", client.RemoteAddr().String(), "error", err)
			}
		}()
	}
}

func handleSOCKS5(ctx context.Context, client net.Conn, username, password string, dial socksDialer) error {
	// Bound the greeting, authentication and request so idle clients cannot
	// hold connections indefinitely. Data transfer has no deadline.
	if err := client.SetDeadline(time.Now().Add(10 * time.Second)); err != nil {
		return err
	}
	var header [4]byte
	if _, err := io.ReadFull(client, header[:2]); err != nil {
		return err
	}
	if header[0] != 5 {
		return errors.New("unsupported SOCKS version")
	}
	methods := make([]byte, int(header[1]))
	if _, err := io.ReadFull(client, methods); err != nil {
		return err
	}
	method := byte(0)
	if username != "" {
		method = 2
	}
	chosen := byte(255)
	for _, m := range methods {
		if m == method {
			chosen = method
			break
		}
	}
	if _, err := client.Write([]byte{5, chosen}); err != nil {
		return err
	}
	if chosen == 255 {
		return errors.New("no supported authentication method")
	}
	if chosen == 2 {
		if _, err := io.ReadFull(client, header[:2]); err != nil {
			return err
		}
		if header[0] != 1 {
			return errors.New("unsupported authentication version")
		}
		user := make([]byte, int(header[1]))
		if _, err := io.ReadFull(client, user); err != nil {
			return err
		}
		if _, err := io.ReadFull(client, header[:1]); err != nil {
			return err
		}
		pass := make([]byte, int(header[0]))
		if _, err := io.ReadFull(client, pass); err != nil {
			return err
		}
		ok := subtle.ConstantTimeCompare(user, []byte(username)) == 1 && subtle.ConstantTimeCompare(pass, []byte(password)) == 1
		status := byte(1)
		if ok {
			status = 0
		}
		if _, err := client.Write([]byte{1, status}); err != nil {
			return err
		}
		if !ok {
			return errors.New("authentication failed")
		}
	}
	if _, err := io.ReadFull(client, header[:4]); err != nil {
		return err
	}
	if header[0] != 5 {
		return errors.New("unsupported request version")
	}
	if header[1] != 1 {
		writeSOCKS5Reply(client, 7, nil)
		return errors.New("only CONNECT is supported")
	}
	var host string
	switch header[3] {
	case 1:
		b := make([]byte, 4)
		if _, err := io.ReadFull(client, b); err != nil {
			return err
		}
		host = net.IP(b).String()
	case 4:
		b := make([]byte, 16)
		if _, err := io.ReadFull(client, b); err != nil {
			return err
		}
		host = net.IP(b).String()
	case 3:
		if _, err := io.ReadFull(client, header[:1]); err != nil {
			return err
		}
		if header[0] == 0 {
			writeSOCKS5Reply(client, 8, nil)
			return errors.New("empty domain")
		}
		b := make([]byte, int(header[0]))
		if _, err := io.ReadFull(client, b); err != nil {
			return err
		}
		host = string(b) // Domain name reaches the operator's dialer for remote DNS.
	default:
		writeSOCKS5Reply(client, 8, nil)
		return errors.New("unsupported address type")
	}
	if _, err := io.ReadFull(client, header[:2]); err != nil {
		return err
	}
	port := int(header[0])<<8 | int(header[1])
	if err := client.SetDeadline(time.Time{}); err != nil {
		return err
	}
	upstream, err := dial(ctx, "tcp", net.JoinHostPort(host, strconv.Itoa(port)))
	if err != nil {
		writeSOCKS5Reply(client, 5, nil)
		return fmt.Errorf("connect %s: %w", host, err)
	}
	defer upstream.Close()
	if err := writeSOCKS5Reply(client, 0, upstream.LocalAddr()); err != nil {
		return err
	}
	finished := make(chan struct{}, 2)
	copyConn := func(dst, src net.Conn) {
		io.Copy(dst, src)
		if c, ok := dst.(interface{ CloseWrite() error }); ok {
			c.CloseWrite()
		}
		finished <- struct{}{}
	}
	go copyConn(upstream, client)
	go copyConn(client, upstream)
	<-finished
	return nil
}

func writeSOCKS5Reply(w io.Writer, status byte, addr net.Addr) error {
	response := []byte{5, status, 0, 1, 0, 0, 0, 0, 0, 0}
	if tcp, ok := addr.(*net.TCPAddr); ok {
		ip := tcp.IP.To4()
		if ip == nil {
			ip = tcp.IP.To16()
			response[3] = 4
		}
		if ip != nil {
			response = append(response[:4], ip...)
			response = append(response, byte(tcp.Port>>8), byte(tcp.Port))
		}
	}
	_, err := w.Write(response)
	return err
}
