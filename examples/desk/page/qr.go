package main

import (
	"fmt"
	"io"
	"net"
	"net/url"
	"strconv"
	"strings"

	qrcode "github.com/skip2/go-qrcode"
)

func pageCodeURL(base, code string) string {
	return strings.TrimRight(base, "/") + "/" + code
}

func printJoinLinks(w io.Writer, code, local, satellite string) error {
	if local != "" {
		local = pageCodeURL(local, code)
	}
	if satellite != "" {
		satellite = pageCodeURL(satellite, code)
	}
	return printJoinURLs(w, code, local, satellite)
}

func printJoinURLs(w io.Writer, code, local, satellite string) error {
	for _, door := range []struct{ label, base string }{
		{"Local", local},
		{"Satellite", satellite},
	} {
		if door.base == "" {
			continue
		}
		link := door.base
		qr, err := qrcode.New(link, qrcode.Medium)
		if err != nil {
			return fmt.Errorf("encode %s link: %w", door.label, err)
		}
		if _, err := fmt.Fprintf(w, "%s %s\n%s\n", door.label, link, qr.ToSmallString(false)); err != nil {
			return err
		}
	}
	_, err := fmt.Fprintln(w, code)
	return err
}

// lanJoinBase substitutes the host's mDNS name while keeping the configured
// fixed port and the service's own path prefix.
func lanJoinBase(base, hostname string) string {
	u, err := url.Parse(base)
	if err != nil || hostname == "" || !fixedPort(u.Port()) {
		return base
	}
	host := strings.TrimSuffix(hostname, ".")
	if !strings.HasSuffix(strings.ToLower(host), ".local") {
		host = strings.SplitN(host, ".", 2)[0] + ".local"
	}
	u.Host = net.JoinHostPort(host, u.Port())
	return u.String()
}

func fixedPort(port string) bool {
	n, err := strconv.ParseUint(port, 10, 16)
	return err == nil && n > 0
}
