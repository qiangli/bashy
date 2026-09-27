package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
)

// resolveToken finds the provider token: the environment variable named by
// the policy, then the vault through `bashy secret get`. The value is read
// from a pipe and never printed.
var resolveToken = func(name string) (string, error) {
	if v := strings.TrimSpace(os.Getenv(name)); v != "" {
		return v, nil
	}
	var out, errOut bytes.Buffer
	c := exec.Command("bashy", "secret", "get", name)
	c.Stdout, c.Stderr = &out, &errOut
	if err := c.Run(); err != nil {
		return "", fmt.Errorf("bashy secret get %s: %v: %s", name, err, strings.TrimSpace(errOut.String()))
	}
	return strings.TrimSpace(out.String()), nil
}

// newProvider is the seam tests replace with a fake.
var newProvider = func(p Policy) (Provider, error) {
	if p.Provider != "digitalocean" {
		return nil, fmt.Errorf("unknown provider %q", p.Provider)
	}
	tok, err := resolveToken(p.TokenSecret)
	if err != nil || tok == "" {
		return nil, fmt.Errorf("no %s token (%v)", p.TokenSecret, err)
	}
	return NewDigitalOcean(tok), nil
}

func loadManager(needProvider bool) (*Manager, error) {
	dir, err := StateDir()
	if err != nil {
		return nil, err
	}
	p, err := LoadPolicy(dir)
	if err != nil {
		return nil, err
	}
	m := &Manager{Policy: p, Ledger: Ledger{Dir: dir}, Seat: seatName()}
	prov, err := newProvider(p)
	if err != nil {
		if needProvider {
			return nil, fmt.Errorf("%w — store it with `bashy ask --name %s --stdout | bashy secret set %s`",
				err, p.TokenSecret, p.TokenSecret)
		}
		return m, nil
	}
	m.Provider = prov
	return m, nil
}

// NewCmd returns the ephemeral-host command tree.
func NewCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "ephemeral-host",
		Short: "rent short-lived cloud hosts with a deadline and a budget, and give them back",
		Long: `ephemeral-host rents a cloud machine for a run and makes sure it is given back.

Every host is created with a deadline (--ttl) and a budget (--cap, USD); neither
has a default. The host is recorded in a ledger on this machine and tagged with
its deadline at the provider, and only hosts in the ledger can be destroyed.

The provider token must belong to an account (a DigitalOcean team) that holds
nothing but ephemeral hosts: a token that can delete one droplet can delete them
all, so the account is the boundary and the ledger is the courtesy. The token is
read from the vault (policy token_secret, default DO_EPHEMERAL_TOKEN) and never
printed.

Policy (max TTL, max cap, daily cap, GPU, defaults) lives in
$BASHY_HOME/ephemeral-host/policy.json; see ` + "`ephemeral-host policy`" + `.`,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	cmd.CompletionOptions.DisableDefaultCmd = true
	cmd.AddCommand(newCreateCmd(), newListCmd(), newPolicyCmd())
	return cmd
}

func newCreateCmd() *cobra.Command {
	var (
		req         CreateRequest
		asJSON      bool
		wait        bool
		waitTimeout time.Duration
	)
	cmd := &cobra.Command{
		Use:   "create NAME --ttl DURATION --cap USD",
		Short: "rent a host with a deadline and a budget cap",
		Example: `  ephemeral-host create s311-bench --ttl 3h --cap 1 --sprint 311
  ephemeral-host create big --ttl 2h --cap 2 --size s-8vcpu-16gb --dry-run`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			req.Name = args[0]
			m, err := loadManager(true)
			if err != nil {
				return err
			}
			ctx := cmd.Context()
			if ctx == nil {
				ctx = context.Background()
			}
			res, err := m.Create(ctx, req)
			if err != nil {
				return err
			}
			if !res.DryRun && wait {
				h, werr := m.Wait(ctx, res.Lease.ID, waitTimeout, 5*time.Second)
				res.Host = h
				if werr != nil {
					printCreate(cmd.OutOrStdout(), res, asJSON)
					return fmt.Errorf("created and recorded, but not ready: %w", werr)
				}
			}
			return printCreate(cmd.OutOrStdout(), res, asJSON)
		},
	}
	f := cmd.Flags()
	f.DurationVar(&req.TTL, "ttl", 0, "deadline from now (required), e.g. 3h")
	f.Float64Var(&req.CapUSD, "cap", 0, "budget in USD (required); refused if price × ttl exceeds it")
	f.StringVar(&req.Size, "size", "", "provider size slug (default: policy default_size)")
	f.StringVar(&req.Region, "region", "", "provider region (default: policy default_region)")
	f.StringVar(&req.Image, "image", "", "image slug (default: policy default_image)")
	f.StringVar(&req.Sprint, "sprint", "", "sprint the host is rented for (recorded and tagged)")
	f.BoolVar(&req.DryRun, "dry-run", false, "check the policy and price, rent nothing")
	f.BoolVar(&wait, "wait", true, "wait until the host is active with a public address")
	f.DurationVar(&waitTimeout, "wait-timeout", 5*time.Minute, "how long --wait waits")
	f.BoolVar(&asJSON, "json", false, "machine-readable output")
	return cmd
}

func printCreate(w io.Writer, res CreateResult, asJSON bool) error {
	if asJSON {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(res)
	}
	le := res.Lease
	verb := "created"
	if res.DryRun {
		verb = "dry run: would create"
	}
	fmt.Fprintf(w, "%s %s", verb, le.Name)
	if le.ID != "" {
		fmt.Fprintf(w, " (id %s)", le.ID)
	}
	fmt.Fprintf(w, ": %s in %s, $%.4f/h, ceiling $%.2f of cap $%.2f, deadline %s\n",
		le.Size, le.Region, le.PriceHourly, res.CeilingUSD, le.CapUSD, le.Deadline.Local().Format("2006-01-02 15:04 MST"))
	if res.Host.IPv4 != "" {
		fmt.Fprintf(w, "ssh root@%s\n", res.Host.IPv4)
	}
	return nil
}

func newListCmd() *cobra.Command {
	var all, asJSON bool
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "hosts in the ledger joined with what the provider can see",
		Long: `list shows every open lease with its cost so far and time left, and every host
the token can see that the ledger does not know ("untracked"). In an ephemeral
account an untracked host is somebody's forgotten box. Without a token the
ledger alone is shown (state "unknown").`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			m, err := loadManager(false)
			if err != nil {
				return err
			}
			ctx := cmd.Context()
			if ctx == nil {
				ctx = context.Background()
			}
			rows, err := m.List(ctx, all)
			if err != nil {
				return err
			}
			if asJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(rows)
			}
			return printRows(cmd.OutOrStdout(), rows, now())
		},
	}
	cmd.Flags().BoolVar(&all, "all", false, "include closed leases")
	cmd.Flags().BoolVar(&asJSON, "json", false, "machine-readable output")
	return cmd
}

func printRows(w io.Writer, rows []Row, t time.Time) error {
	if len(rows) == 0 {
		fmt.Fprintln(w, "no ephemeral hosts")
		return nil
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "NAME\tID\tSTATE\tSEAT\tSPRINT\tSIZE\t$/H\tSPENT\tCAP\tLEFT\tIP")
	for _, r := range rows {
		name, id, seat, sprint, size, price, spent, cp, left, ip := "-", "-", "-", "-", "-", "-", "-", "-", "-", "-"
		if h := r.Host; h != nil {
			name, id, size, ip = h.Name, h.ID, h.Size, orDash(h.IPv4)
			price = fmt.Sprintf("%.4f", h.PriceHourly)
			if d, ok := DeadlineFromTags(h.Tags); ok {
				left = leftString(d, t)
			}
		}
		if le := r.Lease; le != nil {
			name, id, seat, sprint, size = le.Name, le.ID, le.Seat, orDash(le.Sprint), le.Size
			price = fmt.Sprintf("%.4f", le.PriceHourly)
			spent = fmt.Sprintf("%.2f", le.SpentUSD(t))
			cp = fmt.Sprintf("%.2f", le.CapUSD)
			if !le.Closed {
				left = leftString(le.Deadline, t)
			}
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			name, id, r.State, seat, sprint, size, price, spent, cp, left, ip)
	}
	return tw.Flush()
}

func leftString(deadline, t time.Time) string {
	d := deadline.Sub(t).Round(time.Minute)
	if d < 0 {
		return "OVERDUE " + (-d).String()
	}
	return d.String()
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func newPolicyCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "policy",
		Short: "print the policy in force (defaults merged with policy.json)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			dir, err := StateDir()
			if err != nil {
				return err
			}
			p, err := LoadPolicy(dir)
			if err != nil {
				return err
			}
			enc := json.NewEncoder(cmd.OutOrStdout())
			enc.SetIndent("", "  ")
			return enc.Encode(p)
		},
	}
}
