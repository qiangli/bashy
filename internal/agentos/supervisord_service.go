// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package agentos

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/qiangli/yoke/pkg/fleet"
	"github.com/qiangli/yoke/pkg/room"
	"github.com/qiangli/yoke/pkg/secrets"
	"github.com/qiangli/yoke/pkg/service"
)

const supervisordMonitorUsage = `usage: bashy supervisord [--pid1] [--until-done] [--socket PATH] [--poll DUR]

The service monitor: keep every enabled unit in ` + "`bashy service list`" + ` in its
desired state, in the foreground. Each process unit runs as a foreground child
in its own process group, output to BASHY_HOME/service/NAME/output.log
(rotated); each internal unit is applied by the process that hosts its
capability. One policy for every unit, read from its declaration: requires
ordering, the http|tcp|exec health probe, restart (always|on-failure|no) with
min/max backoff and healthy_after, and start_limit — more than burst starts in
interval leaves the unit failed. A port held by a foreign listener fails the
unit with that reason. Unit add/set/enable/disable apply live without
restarting other units. Status is served on a local socket (` + "`bashy service\nstatus|start|stop|restart`" + `); every transition is a service.* event on the
room timeline and failed also reaches the notify sinks. TERM/INT stop every
unit, dependents first, and exit 0.

A dag root is a unit whose exec is ` + "`bashy dag DAG.md TARGET`" + `; the old
` + "`bashy supervisord DAG.md TARGET`" + ` form is refused (exit 2) with that unit spelled out.

flags:
  --pid1          container entrypoint: reap orphaned descendants (PID 1 or a
                  Linux child subreaper; refused where neither is possible)
  --until-done    exit once no enabled unit wants to run any more (every one
                  completed or failed): 0 if none failed, else 1
  --socket PATH   control socket (default BASHY_HOME/service/supervisord.sock)
  --poll DUR      re-read the registry this often as well (default %v)

`

const supervisordPoll = 5 * time.Second

// dispatchSupervisordMonitor is `bashy supervisord` with no DAG operands.
func dispatchSupervisordMonitor(args []string) int {
	fs := flag.NewFlagSet("supervisord", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	pid1 := fs.Bool("pid1", false, "")
	untilDone := fs.Bool("until-done", false, "")
	sock := fs.String("socket", supervisordSocket(), "")
	poll := fs.Duration("poll", supervisordPoll, "")
	if err := fs.Parse(args); err != nil || fs.NArg() != 0 {
		if errors.Is(err, flag.ErrHelp) {
			supervisordHelp(os.Stdout)
			return 0
		}
		if err == nil {
			err = fmt.Errorf("unexpected operand %q", fs.Arg(0))
		}
		fmt.Fprintln(os.Stderr, "bashy supervisord:", err)
		supervisordHelp(os.Stderr)
		return 2
	}
	sigs := make(chan os.Signal, 2)
	signal.Notify(sigs, syscall.SIGTERM, os.Interrupt)
	defer signal.Stop(sigs)
	r := startReaper()
	if r != nil {
		defer r.stop()
	} else if *pid1 {
		fmt.Fprintln(os.Stderr, "bashy supervisord: --pid1: cannot reap orphans here (not PID 1 and no child subreaper on this OS)")
		return 2
	}
	m := newServiceMonitor(r, serviceHome(), os.Stderr)
	m.pollEvery = *poll
	m.untilDone = *untilDone
	stop, err := m.serveControl(*sock)
	if err != nil {
		fmt.Fprintln(os.Stderr, "bashy supervisord:", err)
		return 1
	}
	defer stop()
	m.logf("monitoring service units (socket %s, pid %d)", *sock, os.Getpid())
	return m.run(sigs)
}

func supervisordHelp(w io.Writer) {
	fmt.Fprintf(w, supervisordMonitorUsage, supervisordPoll)
}

// newServiceMonitor wires the production monitor: the registry as desired
// state, the process provider (logs under logRoot) and the delegated internal
// provider, runtime resolution, the timeline and the notify sink.
func newServiceMonitor(r *reaper, logRoot string, log io.Writer) *monitor {
	m := newMonitor()
	m.log = log
	m.load = func() ([]fleet.Service, []error) { return fleet.New().Services() }
	m.providers = service.Providers{
		fleet.ServiceProviderProcess:  newProcessProvider(bashySelfPath(), logRoot, r),
		fleet.ServiceProviderInternal: newDelegatedInternal(),
	}
	m.resolve = resolveUnitRuntime
	m.probe = func(ctx context.Context, u fleet.Service, p unitPolicy) (error, bool) { return runProbe(ctx, u, p, r) }
	m.events = recordServiceEvent
	m.notify = notifyServiceFailed
	return m
}

// resolveUnitRuntime is what a provider gets besides the declaration: env
// with every vault reference resolved, and the pinned download provisioned
// through the same binmgr path a registered command uses.
func resolveUnitRuntime(ctx context.Context, u fleet.Service) (service.Runtime, error) {
	rt := service.Runtime{Env: map[string]string{}}
	for k, v := range u.Env {
		rt.Env[k] = v
	}
	for _, s := range u.Secrets {
		// A service declares the exact environment name its process consumes.
		// A host may bind that name directly; the vault ref remains the fallback
		// for hosts that use secrets.map. Model credential suffix conventions do
		// not cover arbitrary service environment names.
		if value, ok := os.LookupEnv(s.Env); ok && strings.TrimSpace(value) != "" {
			rt.Env[s.Env] = value
			continue
		}
		kv, ok := secrets.ResolveAgentKey(os.Environ(), s.Ref)
		if !ok {
			return rt, fmt.Errorf("secret %s (ref %q) is not bound on this host", s.Env, s.Ref)
		}
		_, v, _ := strings.Cut(kv, "=")
		rt.Env[s.Env] = v
	}
	if u.Download != nil {
		p, err := fleet.Command{Name: u.Name, Download: u.Download}.Ensure(ctx)
		if err != nil {
			return rt, fmt.Errorf("download: %w", err)
		}
		rt.Download = p
	}
	return rt, nil
}

const serviceActor = "bashy-supervisord"

// recordServiceEvent puts a transition on the room timeline (S3) as kind
// service.<state>, the object in Target.
func recordServiceEvent(ev service.Event) {
	room.Record(ev.Kind(), serviceActor, "service:"+ev.Unit, string(ev.From), ev.Reason)
}

// notifyServiceFailed is the failed-unit notification: a principal-stamped
// notify event on the timeline, which the notify sinks (S7) deliver.
func notifyServiceFailed(ev service.Event) {
	_ = room.Notify(room.Event{Principal: serviceActor, Actor: serviceActor, Topic: "service",
		Target: "service:" + ev.Unit, Body: room.Brief(fmt.Sprintf("service %s failed: %s", ev.Unit, ev.Reason), 280)})
}

// ─── bashy service ────────────────────────────────────────────────────────

// dispatchService mounts the unit-definition tree (fleet.NewServiceCmd: list
// show schema add set rm edit enable disable retire unretire) and adds the
// runtime verbs over the same records. A declaration write is followed by a
// reload request, so a running monitor applies it now instead of at its next
// poll; with no monitor the write simply stands.
func dispatchService(args []string) int {
	cmd := newServiceCmd()
	cmd.SetArgs(args)
	if err := cmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "bashy service:", err)
		var ec interface{ ExitCode() int }
		if errors.As(err, &ec) {
			return ec.ExitCode()
		}
		return fleet.ExitCode(err)
	}
	return 0
}

// serviceWriteVerbs change a declaration or its desired state.
var serviceWriteVerbs = map[string]bool{"add": true, "set": true, "rm": true, "edit": true,
	"enable": true, "disable": true, "retire": true, "unretire": true}

func newServiceCmd() *cobra.Command {
	root := fleet.NewServiceCmd()
	root.Use = "service"
	for _, c := range root.Commands() {
		if !serviceWriteVerbs[c.Name()] || c.RunE == nil {
			continue
		}
		run := c.RunE
		c.RunE = func(cmd *cobra.Command, args []string) error {
			if err := run(cmd, args); err != nil {
				return err
			}
			_, _ = ctlCall(supervisordSocket(), ctlRequest{Op: "reload"}, 2*time.Second)
			return nil
		}
	}
	root.AddCommand(newServiceStatusCmd(), newServiceCtlCmd("start"), newServiceCtlCmd("stop"),
		newServiceCtlCmd("restart"), newServiceLogsCmd(), newServiceRunCmd())
	return root
}

// serviceStatusDoc is `bashy service status --json`.
type serviceStatusDoc struct {
	Schema  string           `json:"schema_version"`
	Monitor monitorInfo      `json:"monitor"`
	Units   []service.Status `json:"units"`
}

type monitorInfo struct {
	Running bool   `json:"running"`
	PID     int    `json:"pid,omitempty"`
	Socket  string `json:"socket"`
}

// serviceStatus asks the monitor; without one it reports each unit's desired
// state as inactive and does not guess (contract rule 7).
func serviceStatus(sock string, names []string, load func() ([]fleet.Service, []error)) (serviceStatusDoc, error) {
	doc := serviceStatusDoc{Schema: ServiceStatusSchema, Monitor: monitorInfo{Socket: sock}, Units: []service.Status{}}
	want := map[string]bool{}
	for _, n := range names {
		want[n] = true
	}
	if resp, err := ctlCall(sock, ctlRequest{Op: "status"}, 3*time.Second); err == nil {
		doc.Monitor.Running, doc.Monitor.PID = true, resp.PID
		for _, st := range resp.Units {
			if len(want) == 0 || want[st.Unit] {
				doc.Units = append(doc.Units, st)
				delete(want, st.Unit)
			}
		}
	}
	units, _ := load()
	known := map[string]bool{}
	for _, st := range doc.Units {
		known[st.Unit] = true
	}
	for _, u := range units {
		if known[u.Name] || u.IsRetired() || (len(names) > 0 && !want[u.Name]) {
			continue
		}
		delete(want, u.Name)
		reason := "supervisord is not running"
		if doc.Monitor.Running {
			reason = "not monitored"
		}
		doc.Units = append(doc.Units, service.Status{Unit: u.Name, Provider: u.ProviderName(), Enabled: u.Enabled,
			State: service.Inactive, Reason: reason, Ports: unitPorts(u)})
	}
	for n := range want {
		return doc, fmt.Errorf("no service %q", n)
	}
	return doc, nil
}

func newServiceStatusCmd() *cobra.Command {
	var asJSON bool
	c := &cobra.Command{
		Use:           "status [NAME...]",
		Short:         "Show runtime state of service units (from the running bashy supervisord)",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			doc, err := serviceStatus(supervisordSocket(), args, func() ([]fleet.Service, []error) { return fleet.New().Services() })
			if err != nil {
				return err
			}
			if asJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(doc)
			}
			writeServiceStatus(cmd.OutOrStdout(), doc)
			return nil
		},
	}
	c.Flags().BoolVar(&asJSON, "json", false, "emit "+ServiceStatusSchema+" JSON")
	return c
}

func writeServiceStatus(w io.Writer, doc serviceStatusDoc) {
	if !doc.Monitor.Running {
		fmt.Fprintf(w, "supervisord: not running (%s)\n", doc.Monitor.Socket)
	}
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "NAME\tPROVIDER\tENABLED\tSTATE\tPID\tRESTARTS\tHEALTH\tREASON")
	for _, st := range doc.Units {
		pid := "-"
		if st.PID > 0 {
			pid = fmt.Sprint(st.PID)
		}
		fmt.Fprintf(tw, "%s\t%s\t%v\t%s\t%s\t%d\t%s\t%s\n", st.Unit, st.Provider, st.Enabled, st.State, pid,
			st.Restarts, dashIfBlank(st.Health), dashIfBlank(st.Reason))
	}
	tw.Flush()
}

func dashIfBlank(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func newServiceCtlCmd(op string) *cobra.Command {
	short := map[string]string{
		"start":   "Start a unit now (runtime only; a disabled unit runs until stopped) and clear its failed state",
		"stop":    "Stop a unit and keep it down until start (runtime only; enable state unchanged)",
		"restart": "Restart a unit and clear its failed state and start limit",
	}[op]
	return &cobra.Command{
		Use:           op + " NAME",
		Short:         short,
		Args:          cobra.ExactArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			resp, err := ctlCall(supervisordSocket(), ctlRequest{Op: op, Unit: args[0]}, 5*time.Second)
			if err != nil {
				if errors.Is(err, errNoMonitor) {
					return fmt.Errorf("%w; start it with `bashy supervisord` (or run one unit in the foreground: bashy service run %s)", err, args[0])
				}
				return err
			}
			for _, st := range resp.Units {
				fmt.Fprintf(cmd.OutOrStdout(), "%s: %s requested (state %s)\n", st.Unit, op, st.State)
			}
			return nil
		},
	}
}

func newServiceLogsCmd() *cobra.Command {
	var lines int
	var follow bool
	c := &cobra.Command{
		Use:           "logs NAME",
		Short:         "Print a unit's output log (BASHY_HOME/service/NAME/output.log)",
		Args:          cobra.ExactArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			path := filepath.Join(unitLogDir(serviceHome(), args[0]), unitLogName)
			return tailFile(cmd.Context(), cmd.OutOrStdout(), path, lines, follow)
		},
	}
	c.Flags().IntVarP(&lines, "lines", "n", 50, "last N lines (0 = all)")
	c.Flags().BoolVarP(&follow, "follow", "f", false, "keep printing as the log grows")
	return c
}

// tailFile prints the last n lines of path, then (follow) what is appended.
func tailFile(ctx context.Context, w io.Writer, path string, n int, follow bool) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	text := string(b)
	if n > 0 {
		ls := strings.SplitAfter(text, "\n")
		if ls[len(ls)-1] == "" {
			ls = ls[:len(ls)-1]
		}
		if len(ls) > n {
			ls = ls[len(ls)-n:]
		}
		text = strings.Join(ls, "")
	}
	io.WriteString(w, text)
	if !follow {
		return nil
	}
	off := int64(len(b))
	if ctx == nil {
		ctx = context.Background()
	}
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(500 * time.Millisecond):
		}
		f, err := os.Open(path)
		if err != nil {
			continue
		}
		st, _ := f.Stat()
		if st != nil && st.Size() < off {
			off = 0 // rotated
		}
		f.Seek(off, io.SeekStart)
		k, _ := io.Copy(w, f)
		off += k
		f.Close()
	}
}

func newServiceRunCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "run NAME",
		Short: "Run one unit in the foreground under the monitor's policy, output to this terminal",
		Long: "Run one unit in the foreground with the same restart, health and start-limit\n" +
			"policy the monitor applies, enabled or not, output to this terminal and no\n" +
			"control socket. Units it requires are not started. Exits 0 when the unit\n" +
			"completes or on TERM/INT, 1 when it ends failed.",
		Args:          cobra.ExactArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if _, ok := fleet.New().Service(args[0]); !ok {
				return fmt.Errorf("no service %q", args[0])
			}
			sigs := make(chan os.Signal, 2)
			signal.Notify(sigs, syscall.SIGTERM, os.Interrupt)
			defer signal.Stop(sigs)
			r := startReaper()
			if r != nil {
				defer r.stop()
			}
			m := newServiceMonitor(r, "", os.Stderr)
			m.only, m.force = map[string]bool{args[0]: true}, true
			if code := m.run(sigs); code != 0 {
				return exitCodeError(code)
			}
			return nil
		},
	}
}

type exitCodeError int

func (e exitCodeError) Error() string { return fmt.Sprintf("exit %d", int(e)) }
func (e exitCodeError) ExitCode() int { return int(e) }
