package agentos

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/qiangli/bashy/internal/cli"
	"github.com/qiangli/yoke/pkg/shape"
	"github.com/qiangli/yoke/pkg/src"
	"github.com/spf13/cobra"
)

const shapePromotionSchema = "bashy-shape-promotion-v1"

var errShapePending = errors.New("shape: changes pending")

type shapePromotionChange struct {
	shape.OutgoingChange
	Diff      string `json:"diff,omitempty"`
	DiffError string `json:"diff_error,omitempty"`
}

type shapePromotionEnvelope struct {
	SchemaVersion string                 `json:"schema_version"`
	Verb          string                 `json:"verb"`
	App           string                 `json:"app"`
	Tier          string                 `json:"tier,omitempty"`
	Remotes       map[string]string      `json:"remotes,omitempty"`
	Changes       []shapePromotionChange `json:"changes,omitempty"`
	Pushed        []shapePushed          `json:"pushed,omitempty"`
	Incoming      []shape.IncomingChange `json:"incoming,omitempty"`
	Pulled        *shape.PullResult      `json:"pulled,omitempty"`
	NextSteps     []string               `json:"next_steps,omitempty"`
	Pending       int                    `json:"pending"`
}

type shapePushed struct {
	ID string `json:"id"`
	shape.PushResult
}

func shapeTierCheck(tier string) error {
	if tier != "org" && tier != "upstream" {
		return fmt.Errorf("shape: tier must be org or upstream, got %q", tier)
	}
	return nil
}

func shapePromotionService(app string) (shape.Service, error) {
	a, err := shapeAppLookup(app)
	if err != nil {
		return shape.Service{}, err
	}
	opts := shapeServiceOptions(src.Home())
	if v := cli.BashyVersion(); v != "" && opts.InstalledVersion == "" {
		opts.InstalledVersion = "v" + strings.TrimPrefix(v, "v")
	}
	return shape.Service{App: a, Options: opts}, nil
}

func shapeWriteEnvelope(w io.Writer, env shapePromotionEnvelope, asJSON bool) error {
	env.SchemaVersion = shapePromotionSchema
	if asJSON {
		return writeShapeResult(w, env, true)
	}
	var b strings.Builder
	switch env.Verb {
	case "remote":
		for _, t := range []string{"org", "upstream"} {
			u := env.Remotes[t]
			if u == "" {
				u = "unconfigured"
			}
			fmt.Fprintf(&b, "%s: %s\n", t, u)
		}
	case "outgoing":
		if len(env.Changes) == 0 {
			fmt.Fprintf(&b, "nothing outgoing to %s\n", env.Tier)
		}
		for _, c := range env.Changes {
			fmt.Fprintf(&b, "%s\t%s\t%s\n", c.ID, c.Status[env.Tier], c.Title)
			if c.Warning != "" {
				fmt.Fprintf(&b, "  warning: %s\n", c.Warning)
			}
			if c.Diff != "" {
				b.WriteString(c.Diff)
				if !strings.HasSuffix(c.Diff, "\n") {
					b.WriteString("\n")
				}
			}
		}
	case "incoming":
		if len(env.Incoming) == 0 {
			fmt.Fprintf(&b, "nothing incoming from %s\n", env.Tier)
		}
		for _, c := range env.Incoming {
			fmt.Fprintf(&b, "%s\t%s\t%s\t%s\n", c.Kind, c.ID, c.Status, c.Title)
			if c.Warning != "" {
				fmt.Fprintf(&b, "  warning: %s\n", c.Warning)
			}
		}
	case "pull":
		if env.Pulled != nil && env.Pulled.Kind == "release" {
			fmt.Fprintf(&b, "release %s\n", env.Pulled.Tag)
		} else if env.Pulled != nil {
			fmt.Fprintf(&b, "pulled change into %s\n", env.Pulled.Path)
		}
		for _, n := range env.NextSteps {
			fmt.Fprintf(&b, "next: %s\n", n)
		}
	case "push":
		if len(env.Pushed) == 0 {
			fmt.Fprintf(&b, "nothing to push to %s\n", env.Tier)
		}
		for _, p := range env.Pushed {
			fmt.Fprintf(&b, "%s\tbranch %s", p.ID, p.Branch)
			if p.PR != "" {
				fmt.Fprintf(&b, "\tpr %s", p.PR)
			} else if p.CompareURL != "" {
				fmt.Fprintf(&b, "\tcompare %s", p.CompareURL)
			}
			b.WriteString("\n")
		}
	}
	_, err := io.WriteString(w, b.String())
	return err
}

var shapeOutgoingHook func(app, tier string, diff bool) ([]shapePromotionChange, error)

func shapeOutgoing(ctx context.Context, app, tier string, diff bool) ([]shapePromotionChange, error) {
	if shapeOutgoingHook != nil {
		return shapeOutgoingHook(app, tier, diff)
	}
	svc, err := shapePromotionService(app)
	if err != nil {
		return nil, err
	}
	list, err := svc.Outgoing(ctx, tier)
	if err != nil {
		return nil, err
	}
	out := make([]shapePromotionChange, 0, len(list))
	for _, c := range list {
		pc := shapePromotionChange{OutgoingChange: c}
		if diff && c.Warning == "" {
			caller := shape.Caller{Principal: shape.HostPrincipal, Actor: shape.HostPrincipal, RepresentedUser: shape.HostPrincipal}
			if d, e := svc.Diff(ctx, caller, c.ID); e != nil {
				pc.DiffError = e.Error()
			} else {
				pc.Diff = fmt.Sprint(d)
			}
		}
		out = append(out, pc)
	}
	return out, nil
}

// shapePromotionStatusLine summarizes pending promotion without ever failing status.
func shapePromotionStatusLine(ctx context.Context, app string) string {
	count := func(tier string, incoming bool) string {
		svc, err := shapePromotionService(app)
		if err != nil {
			return "unconfigured"
		}
		n := 0
		if incoming {
			list, err := svc.Incoming(ctx, tier)
			if err != nil {
				return promotionTierErr(err)
			}
			for _, c := range list {
				if c.Warning == "" {
					n++
				}
			}
			return fmt.Sprintf("%d incoming", n)
		}
		list, err := svc.Outgoing(ctx, tier)
		if err != nil {
			return promotionTierErr(err)
		}
		for _, c := range list {
			if c.Warning == "" {
				n++
			}
		}
		return fmt.Sprintf("%d outgoing", n)
	}
	return fmt.Sprintf("promotion: %s (org) / %s (upstream)\nincoming: %s (org) / %s (upstream)",
		count("org", false), count("upstream", false), count("org", true), count("upstream", true))
}

func promotionTierErr(err error) string {
	if strings.Contains(err.Error(), "not configured") {
		return "unconfigured"
	}
	return "unavailable"
}

// shapeExpandIncomingRef turns a bare change id into its full ref.
func shapeExpandIncomingRef(ref string) string {
	if strings.HasPrefix(ref, "refs/") {
		return ref
	}
	return "refs/heads/shape-submit/" + ref
}

func addShapePromotionCmds(cmd *cobra.Command, o *shapeCLIOptions) {
	var app string
	cmd.PersistentFlags().StringVar(&app, "app", "bashy", "app whose promotion tiers are used (remote/outgoing/push)")
	local := func(c *cobra.Command) error {
		if o.Remote != "" {
			return errors.New("shape: promotion verbs run locally; --remote is not supported")
		}
		return nil
	}

	showRemotes := func(c *cobra.Command, _ []string) error {
		if err := local(c); err != nil {
			return err
		}
		env := shapePromotionEnvelope{Verb: "remote", App: app, Remotes: map[string]string{}}
		for _, t := range []string{"org", "upstream"} {
			u, err := src.ConfiguredTier(src.Home(), app, t)
			if err != nil && t == "org" {
				return err
			}
			env.Remotes[t] = u
		}
		return shapeWriteEnvelope(c.OutOrStdout(), env, o.JSON)
	}
	remote := &cobra.Command{Use: "remote", Short: "Show or set the org and upstream promotion remotes", Args: cobra.NoArgs, RunE: showRemotes}
	remote.AddCommand(&cobra.Command{Use: "show", Short: "Show the org and upstream remotes", Args: cobra.NoArgs, RunE: showRemotes})
	remote.AddCommand(&cobra.Command{Use: "set TIER URL", Short: "Set the org or upstream remote", Args: cobra.ExactArgs(2), RunE: func(c *cobra.Command, args []string) error {
		if err := local(c); err != nil {
			return err
		}
		if err := shapeTierCheck(args[0]); err != nil {
			return err
		}
		if err := src.SetTier(src.Home(), app, args[0], args[1]); err != nil {
			return err
		}
		return showRemotes(c, nil)
	}})

	var outTo string
	var outDiff bool
	outgoing := &cobra.Command{Use: "outgoing", Short: "List approved shapes not yet in org/upstream (exit 3 when pending)", Args: cobra.NoArgs, RunE: func(c *cobra.Command, _ []string) error {
		if err := local(c); err != nil {
			return err
		}
		if err := shapeTierCheck(outTo); err != nil {
			return err
		}
		changes, err := shapeOutgoing(c.Context(), app, outTo, outDiff)
		if err != nil {
			return err
		}
		env := shapePromotionEnvelope{Verb: "outgoing", App: app, Tier: outTo, Changes: changes}
		for _, ch := range changes {
			if ch.Warning == "" {
				env.Pending++
			}
		}
		if err := shapeWriteEnvelope(c.OutOrStdout(), env, o.JSON); err != nil {
			return err
		}
		if env.Pending > 0 {
			return errShapePending
		}
		return nil
	}}
	outgoing.Flags().StringVar(&outTo, "to", "org", "destination tier: org or upstream")
	outgoing.Flags().BoolVar(&outDiff, "diff", false, "include each change's patch")

	var pushTo string
	var pushPR bool
	push := &cobra.Command{Use: "push [ID...]", Short: "Push approved shapes to org or upstream (local -> org -> upstream)", RunE: func(c *cobra.Command, ids []string) error {
		if err := local(c); err != nil {
			return err
		}
		if err := shapeTierCheck(pushTo); err != nil {
			return err
		}
		svc, err := shapePromotionService(app)
		if err != nil {
			return err
		}
		if len(ids) == 0 {
			list, err := svc.Outgoing(c.Context(), pushTo)
			if err != nil {
				return err
			}
			for _, ch := range list {
				if ch.Warning == "" {
					ids = append(ids, ch.ID)
				}
			}
		}
		env := shapePromotionEnvelope{Verb: "push", App: app, Tier: pushTo}
		var errs []error
		for _, id := range ids {
			res, err := svc.Push(c.Context(), pushTo, id, pushPR)
			if err != nil {
				errs = append(errs, fmt.Errorf("%s: %w", id, err))
				continue
			}
			env.Pushed = append(env.Pushed, shapePushed{ID: id, PushResult: res})
		}
		if err := shapeWriteEnvelope(c.OutOrStdout(), env, o.JSON); err != nil {
			return err
		}
		return errors.Join(errs...)
	}}
	push.Flags().StringVar(&pushTo, "to", "", "destination tier: org or upstream (required)")
	push.Flags().BoolVar(&pushPR, "pr", false, "open a pull request (upstream; prints compare URL without gh)")
	_ = push.MarkFlagRequired("to")

	var inFrom string
	incoming := &cobra.Command{Use: "incoming", Short: "List changes and releases waiting in org/upstream (exit 3 when pending)", Args: cobra.NoArgs, RunE: func(c *cobra.Command, _ []string) error {
		if err := local(c); err != nil {
			return err
		}
		if err := shapeTierCheck(inFrom); err != nil {
			return err
		}
		svc, err := shapePromotionService(app)
		if err != nil {
			return err
		}
		list, err := svc.Incoming(c.Context(), inFrom)
		if err != nil {
			return err
		}
		env := shapePromotionEnvelope{Verb: "incoming", App: app, Tier: inFrom, Incoming: list}
		for _, ch := range list {
			if ch.Warning == "" {
				env.Pending++
			}
		}
		if err := shapeWriteEnvelope(c.OutOrStdout(), env, o.JSON); err != nil {
			return err
		}
		if env.Pending > 0 {
			return errShapePending
		}
		return nil
	}}
	incoming.Flags().StringVar(&inFrom, "from", "org", "source tier: org or upstream")

	var pullFrom string
	pull := &cobra.Command{Use: "pull REF|ID", Short: "Fetch an incoming change into a gated wish, or name a release (never installs)", Args: cobra.ExactArgs(1), RunE: func(c *cobra.Command, args []string) error {
		if err := local(c); err != nil {
			return err
		}
		if err := shapeTierCheck(pullFrom); err != nil {
			return err
		}
		svc, err := shapePromotionService(app)
		if err != nil {
			return err
		}
		res, err := svc.Pull(c.Context(), pullFrom, shapeExpandIncomingRef(args[0]))
		if err != nil {
			return err
		}
		env := shapePromotionEnvelope{Verb: "pull", App: app, Tier: pullFrom, Pulled: &res}
		if res.Kind == "release" {
			env.NextSteps = []string{"bashy upgrade (release " + res.Tag + " is available; nothing was installed)"}
			return shapeWriteEnvelope(c.OutOrStdout(), env, o.JSON)
		}
		r := shapeCLIRequest{Verb: "wish", App: app, From: res.Path, Agent: o.Agent, Remote: o.Remote, CookieFile: o.CookieFile, Console: o.Console}
		r.User = strings.TrimSpace(o.User)
		if r.User == "" {
			r.User = strings.TrimSpace(os.Getenv("BASHY_PRINCIPAL"))
		}
		if r.User == "" {
			return errors.New("shape: needs --user or BASHY_PRINCIPAL")
		}
		if r.Actor, err = shapeAuthoredActor(o.Actor); err != nil {
			return err
		}
		result, err := shapeCLIExecute(c.Context(), r)
		if err != nil {
			return err
		}
		if !o.JSON && result != nil {
			if err := writeShapeResult(c.OutOrStdout(), result, false); err != nil {
				return err
			}
		}
		for _, v := range []string{"verify", "preview", "approve", "undo"} {
			env.NextSteps = append(env.NextSteps, "bashy shape "+v+" "+app)
		}
		return shapeWriteEnvelope(c.OutOrStdout(), env, o.JSON)
	}}
	pull.Flags().StringVar(&pullFrom, "from", "", "source tier: org or upstream (required)")
	_ = pull.MarkFlagRequired("from")

	cmd.AddCommand(remote, outgoing, push, incoming, pull)
}
