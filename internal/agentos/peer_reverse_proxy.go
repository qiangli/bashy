// Copyright (c) 2026 qiangli
// See LICENSE for licensing information.

package agentos

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"

	"github.com/qiangli/yoke/pkg/sshclient"
)

// PeerReverseProxy owns a remote loopback listener backed by an operator proxy.
// It is scoped to ctx and must be closed when its commands have finished.
type PeerReverseProxy struct {
	channel  *PeerChannel
	ctx      context.Context
	cancel   context.CancelFunc
	proxyURL string
	noProxy  string
}

// ReverseProxy forwards a remote loopback listener to an already running local
// bashy proxy. Proxy credentials, if present, are retained in the remote URL.
// Empty remoteAddr chooses an ephemeral IPv4 loopback port. NO_PROXY is explicit
// so the remote cannot silently inherit a bypass such as '*'.
func (c *PeerChannel) ReverseProxy(ctx context.Context, operatorURL, remoteAddr, noProxy string) (*PeerReverseProxy, error) {
	if ctx == nil {
		return nil, errors.New("reverse proxy: nil context")
	}
	u, err := parseOperatorProxy(operatorURL)
	if err != nil {
		return nil, err
	}
	if strings.ContainsRune(noProxy, 0) {
		return nil, errors.New("reverse proxy: NUL in NO_PROXY")
	}
	if remoteAddr == "" {
		remoteAddr = "127.0.0.1:0"
	}
	forwardCtx, cancel := context.WithCancel(ctx)
	addr, err := c.RemoteForward(forwardCtx, remoteAddr, u.Host)
	if err != nil {
		cancel()
		return nil, err
	}
	u.Host = addr.String()
	return &PeerReverseProxy{channel: c, ctx: forwardCtx, cancel: cancel, proxyURL: u.String(), noProxy: noProxy}, nil
}

func parseOperatorProxy(raw string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, errors.New("reverse proxy: invalid proxy URL")
	}
	if u.Scheme != "http" && u.Scheme != "socks5" && u.Scheme != "socks5h" {
		return nil, errors.New("reverse proxy: proxy scheme must be http, socks5 or socks5h")
	}
	host := u.Hostname()
	ip := net.ParseIP(host)
	if host != "localhost" && (ip == nil || !ip.IsLoopback()) {
		return nil, errors.New("reverse proxy: operator proxy must be on loopback")
	}
	port, err := strconv.Atoi(u.Port())
	if err != nil || port < 1 || port > 65535 || u.Path != "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.Opaque != "" {
		return nil, errors.New("reverse proxy: proxy URL requires a port and no path, query or fragment")
	}
	// Remote name resolution is essential on an air-gapped host.
	if u.Scheme == "socks5" {
		u.Scheme = "socks5h"
	}
	return u, nil
}

func (p *PeerReverseProxy) URL() string { return p.proxyURL }
func (p *PeerReverseProxy) Close() error {
	if p != nil && p.cancel != nil {
		p.cancel()
	}
	return nil
}

// Exec exports both conventional cases before the literal command. SSH env
// requests depend on server AcceptEnv policy, so use quoted shell exports.
func (p *PeerReverseProxy) Exec(ctx context.Context, opts sshclient.ExecOptions) (*sshclient.ExecResult, error) {
	if p == nil || p.channel == nil {
		return nil, errors.New("reverse proxy: nil proxy")
	}
	if ctx == nil {
		return nil, errors.New("reverse proxy: nil context")
	}
	if err := p.ctx.Err(); err != nil {
		return nil, err
	}
	if opts.Command == "" {
		return nil, errors.New("reverse proxy: empty command")
	}
	var exports strings.Builder
	for _, key := range []string{"ALL_PROXY", "HTTPS_PROXY", "HTTP_PROXY", "NO_PROXY", "all_proxy", "https_proxy", "http_proxy", "no_proxy"} {
		value := p.proxyURL
		if strings.EqualFold(key, "NO_PROXY") {
			value = p.noProxy
		}
		fmt.Fprintf(&exports, "export %s=%s; ", key, shellQuote(value))
	}
	opts.Command = exports.String() + opts.Command
	execCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	stop := context.AfterFunc(p.ctx, cancel)
	defer stop()
	return p.channel.Exec(execCtx, opts)
}
