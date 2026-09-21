// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

package agentos

// The rest of the predefined decorator set (Sprint 231): timeout, memo and
// auth. Each is a thin connection to a rod bashy already has — the context
// deadline, a process-wide map, the login/status verb — and, like every
// native, a script function of the same name shadows it.

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"mvdan.cc/sh/v3/interp"
)

// Exit statuses the three decorators seal a call with: timeout(1)'s 124 and
// sysexits' EX_NOPERM.
const (
	exitTimeout = 124
	exitNoPerm  = 77
)

// timeoutDecorator cancels the rest of the chain at the deadline: Next runs
// under context.WithTimeout, and when the deadline fired the call is sealed
// with 124 and one stderr line. Under @retry each attempt re-arms (retry is
// the outer rung and calls Next afresh). The one argument is the duration,
// positional or d:.
func timeoutDecorator(stderr io.Writer) nativeDecoratorFunc {
	return func(ctx context.Context, c *nativeDecoratorCall, args []interp.DecoratorArg) error {
		d, err := oneDuration("timeout", "d", args)
		if err != nil {
			return err
		}
		tctx, cancel := context.WithTimeout(ctx, d)
		defer cancel()
		c.Next(tctx)
		if errors.Is(tctx.Err(), context.DeadlineExceeded) && ctx.Err() == nil {
			c.Status = exitTimeout
			fmt.Fprintf(stderr, "%s: timeout after %s\n", c.Name, d)
		}
		return nil
	}
}

// memoDecorator memoizes a call per process: the same function with the same
// arguments returns the cached Results and Status without running the body.
// Only a zero status is cached; an optional ttl (positional or ttl:) expires
// an entry. What is memoized is what the call RETURNS — a body's printed
// output is not replayed, so memo is for typed (value-returning) functions.
// Never advisable: it replaces execution.
func memoDecorator(ctx context.Context, c *nativeDecoratorCall, args []interp.DecoratorArg) error {
	if c.Advised != "" {
		return errors.New("memo is source-only and never applied by advice")
	}
	ttl := time.Duration(0)
	if len(args) > 0 {
		d, err := oneDuration("memo", "ttl", args)
		if err != nil {
			return err
		}
		ttl = d
	}
	key := c.Name + "\x00" + fmt.Sprint(c.Args...)
	if v, ok := memoStore.Load(key); ok {
		e := v.(*memoEntry)
		if ttl == 0 || time.Since(e.at) < ttl {
			c.Results, c.Status = append([]any(nil), e.results...), e.status
			return nil
		}
		memoStore.Delete(key)
	}
	c.Next(ctx)
	if c.Status == 0 {
		memoStore.Store(key, &memoEntry{results: append([]any(nil), c.Results...), status: 0, at: time.Now()})
	}
	return nil
}

type memoEntry struct {
	results []any
	status  int
	at      time.Time
}

type memoStoreType = sync.Map

var memoStore memoStoreType

// authDecorator authenticates ONCE per process and binds the principal. via
// (default `bashy tessaro status`, the pairing this host holds) is a shell
// command run in the call's frame: exit 0 means authenticated and its first
// stdout line is the principal, exported to the body as BASHY_PRINCIPAL. as:
// names the principal the call requires. A failure seals the call with 77
// (EX_NOPERM) and one stderr line; the via verdict is remembered per via
// string, so the same principal is not re-authenticated on every call.
func authDecorator(stderr io.Writer) nativeDecoratorFunc {
	return func(ctx context.Context, c *nativeDecoratorCall, args []interp.DecoratorArg) error {
		via, as, err := authPolicy(args)
		if err != nil {
			return err
		}
		principal, ok := authOnce(ctx, c, via)
		if !ok {
			c.Status = exitNoPerm
			fmt.Fprintf(stderr, "%s: not authenticated (via: %s)\n", c.Name, via)
			return nil
		}
		if as != "" && principal != as {
			c.Status = exitNoPerm
			fmt.Fprintf(stderr, "%s: principal %q, want %q\n", c.Name, principal, as)
			return nil
		}
		c.Run(ctx, `declare -gx BASHY_PRINCIPAL="$AUTH_PRINCIPAL"`, map[string]string{"AUTH_PRINCIPAL": principal})
		c.Next(ctx)
		return nil
	}
}

const authDefaultVia = "bashy tessaro status"

type authVerdict struct {
	principal string
	ok        bool
}

type authStoreType = sync.Map

var authStore authStoreType

// authOnce runs via in the call's frame the first time this process sees it
// and remembers the verdict. The principal is via's first stdout line,
// captured by command substitution so nothing reaches the caller's stdout.
func authOnce(ctx context.Context, c *nativeDecoratorCall, via string) (string, bool) {
	if v, ok := authStore.Load(via); ok {
		e := v.(authVerdict)
		return e.principal, e.ok
	}
	// The principal travels back through a file the check writes: c.Run
	// returns only a status.
	tmp, err := authScratch()
	if err != nil {
		return "", false
	}
	status := c.Run(ctx, `__p="$( { `+via+`; } 2>/dev/null )" || return 1; printf '%s\n' "${__p%%$'\n'*}" > "$AUTH_OUT"`, map[string]string{"AUTH_OUT": tmp.name})
	principal := strings.TrimSpace(tmp.read())
	tmp.remove()
	verdict := authVerdict{principal: principal, ok: status == 0}
	authStore.Store(via, verdict)
	return verdict.principal, verdict.ok
}

// authPolicy resolves @auth's arguments: via (the check command, positional
// or via:) and as (the required principal).
func authPolicy(args []interp.DecoratorArg) (via, as string, err error) {
	via = authDefaultVia
	positional := 0
	for _, a := range args {
		name := a.Name
		if name == "" {
			if positional == 0 {
				name = "via"
			} else {
				name = "as"
			}
			positional++
		}
		switch name {
		case "via":
			if strings.TrimSpace(a.Value) == "" {
				return "", "", errors.New("auth: empty via command")
			}
			via = a.Value
		case "as":
			as = strings.TrimSpace(a.Value)
		default:
			return "", "", fmt.Errorf("auth takes via and as, not %q", name)
		}
	}
	return via, as, nil
}

// oneDuration resolves a decorator's single duration argument, positional or
// named `key`.
func oneDuration(deco, key string, args []interp.DecoratorArg) (time.Duration, error) {
	if len(args) != 1 || (args[0].Name != "" && args[0].Name != key) {
		return 0, fmt.Errorf("%s takes exactly one argument, %s: %q", deco, key, "10s")
	}
	d, err := time.ParseDuration(strings.TrimSpace(args[0].Value))
	if err != nil || d <= 0 {
		return 0, fmt.Errorf("%s %s must be a positive duration (e.g. %q), got %q", deco, key, "10s", args[0].Value)
	}
	return d, nil
}

// authScratch is the one-line file the via check writes the principal to.
type authScratch_t struct{ name string }

func authScratch() (*authScratch_t, error) {
	f, err := os.CreateTemp("", "bashy-auth-*")
	if err != nil {
		return nil, err
	}
	f.Close()
	return &authScratch_t{name: f.Name()}, nil
}

func (s *authScratch_t) read() string {
	b, _ := os.ReadFile(s.name)
	return string(b)
}

func (s *authScratch_t) remove() { os.Remove(s.name) }
