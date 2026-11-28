package agentos

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/qiangli/yoke/pkg/bus"
	"github.com/qiangli/yoke/pkg/shape"
	"github.com/qiangli/yoke/pkg/webconsole"
	"github.com/spf13/cobra"
)

type shapeCLIRequest struct {
	BaseRevision, Patch string
	PatchMetadata       shape.PatchMetadata
	// Caller and Owner are set only by authenticated in-process surfaces.
	Caller *shape.Caller
	// Source is the wish's trigger (shape wish --source KIND:REF, local only).
	Source                                                                       shape.Source
	Owner                                                                        string
	PreviewArgs                                                                  []string
	IncidentKind, IncidentPath                                                   string
	Verb, App, ID, Request, From, User, Actor, Agent, Story, Todo, Page, Element string
	Remote, CookieFile, Console                                                  string
	Reproduction, Expected, Evidence                                             string
	To                                                                           []string
	All, Preview, Repair                                                         bool
	Contract                                                                     []string
	// MaxBytes bounds a ledger read (shape.Bound).
	MaxBytes int
}

type shapeCLIOptions struct {
	User, Actor, Agent, Remote, CookieFile, Console string
	JSON                                            bool
}

var shapeAuthoredActor = bus.ResolveAuthoredActor
var shapeCLIExecute = executeShapeCLI

func bashyShapeCmd() *cobra.Command {
	var o shapeCLIOptions
	cmd := &cobra.Command{Use: "shape", Short: shapeHelpShort, SilenceUsage: true, SilenceErrors: true}
	cmd.PersistentFlags().StringVar(&o.User, "user", "", "principal whose app version is selected (default BASHY_PRINCIPAL)")
	cmd.PersistentFlags().StringVar(&o.User, "principal", "", "alias for --user")
	cmd.PersistentFlags().StringVar(&o.Actor, "as", "", "accountable caller, checked against the current session")
	cmd.PersistentFlags().StringVar(&o.Agent, "agent", "", "shaping agent as tool:model")
	cmd.PersistentFlags().StringVar(&o.Remote, "remote", "", "authenticated console URL; local mode calls the shaping service directly")
	cmd.PersistentFlags().StringVar(&o.CookieFile, "cookie-file", "", "file containing the paired bashy_console session cookie value (remote only)")
	cmd.PersistentFlags().StringVar(&o.Console, "console", fmt.Sprintf("http://127.0.0.1:%d", webconsole.DefaultPort), "console URL for opening a local web preview")
	cmd.PersistentFlags().BoolVar(&o.JSON, "json", false, "emit JSON results")
	for _, spec := range []struct {
		verb, use, short string
		min, max         int
	}{
		{"status", "status APP", "Inspect the active version and candidates", 1, 1},
		{"wish", "wish APP [REQUEST]", "Generate a candidate or import with --from", 1, 2},
		{"show", "show APP ID", "Inspect a candidate", 2, 2},
		{"diff", "diff APP ID", "Show the candidate patch", 2, 2},
		{"verify", "verify APP ID", "Run the trusted verifier", 2, 2},
		{"preview", "preview APP ID [-- ARGS...]", "Preview an isolated candidate", 2, -1},
		{"spec-review", "spec-review APP ID", "Owner: mark a candidate's spec change reviewed so it is graded against its own spec", 2, 2},
		{"approve", "approve APP ID", "Verify again and activate explicitly", 2, 2},
		{"reject", "reject APP ID", "Discard a candidate without activation", 2, 2},
		{"suggest", "suggest APP", "Suggest requests for the current app", 1, 1},
		{"incident", "incident APP [MESSAGE]", "List incidents or report a problem", 1, 2},
		{"history", "history APP", "List the selected principal's versions", 1, 1},
		{"ledger", "ledger APP", "Show why the app was shaped: attempts grouped by problem", 1, 1},
		{"undo", "undo APP", "Restore the previous version through the trusted gate", 1, 1},
		{"restore", "restore APP REV", "Restore a version through the trusted gate", 2, 2},
		{"release", "release APP REV", "Name a release through the existing source rules", 2, 2},
		{"offer", "offer APP REV", "Offer a released version to other principals", 2, 2},
		{"adopt", "adopt APP OFFER", "Preview or adopt an offering through the trusted gate", 2, 2},
		{"submit", "submit APP REV", "Submit an approved version through existing review gates", 2, 2},
	} {
		spec := spec
		var from, story, todo, page, element, source, incidentKind, incidentPath, incidentID string
		var reproduction, expected, evidence string
		var to, contract []string
		var all, preview, repair bool
		var maxBytes int
		var base string
		checkArgs := cobra.RangeArgs(spec.min, spec.max)
		if spec.max < 0 {
			checkArgs = cobra.MinimumNArgs(spec.min)
		}
		sub := &cobra.Command{Use: spec.use, Short: spec.short, Args: checkArgs}
		if spec.verb == "wish" {
			sub.Flags().StringVar(&from, "from", "", "import a hand-made workspace into an isolated candidate")
			sub.Flags().StringVar(&story, "story", "", "link an existing story")
			sub.Flags().StringVar(&todo, "todo", "", "link an existing todo")
			sub.Flags().StringVar(&source, "source", "", "what triggered the wish, as KIND:REF (human, incident, feed, schedule); feed and schedule obey `shape auto` (local only)")
			sub.Flags().StringVar(&page, "page", "", "request's app page context")
			sub.Flags().StringVar(&element, "element", "", "request's element context")
			sub.Flags().StringVar(&reproduction, "reproduction", "", "defect report: how to reproduce")
			sub.Flags().StringVar(&expected, "expected", "", "defect report: expected behavior")
			sub.Flags().StringVar(&evidence, "evidence", "", "defect report: observed evidence")
		}
		if spec.verb == "ledger" {
			sub.Flags().StringVar(&base, "base", "", "view the ledger as of this revision; later attempts are marked, not hidden")
			sub.Flags().IntVar(&maxBytes, "max-bytes", 0, "bound the output for a small context (0: 8 KiB, negative: everything)")
		}
		if spec.verb == "submit" {
			sub.Flags().StringVar(&story, "story", "", "link submission to an existing story")
		}
		if spec.verb == "incident" {
			sub.Flags().StringVar(&incidentKind, "kind", "script", "reported failure kind: script or asset")
			sub.Flags().StringVar(&incidentPath, "path", "/", "app path where the failure occurred")
			sub.Flags().StringVar(&incidentID, "id", "", "inspect one incident instead of listing")
		}
		if spec.verb == "suggest" {
			sub.Flags().StringVar(&page, "page", "/", "app page context")
		}
		if spec.verb == "offer" {
			sub.Flags().StringSliceVar(&to, "to", nil, "eligible principals")
			sub.Flags().BoolVar(&all, "all", false, "offer to all principals")
		}
		if spec.verb == "adopt" {
			sub.Flags().BoolVar(&preview, "preview", false, "preview without activating")
		}
		if spec.verb == "release" {
			sub.Flags().BoolVar(&repair, "repair", false, "declare a repair")
			sub.Flags().StringSliceVar(&contract, "contract", nil, "explicitly approved contract removals")
		}
		sub.RunE = func(c *cobra.Command, args []string) error {
			r := shapeCLIRequest{Verb: spec.verb, App: args[0], From: from, Agent: o.Agent, Story: story, Todo: todo, Page: page, Element: element, Reproduction: reproduction, Expected: expected, Evidence: evidence, Remote: o.Remote, CookieFile: o.CookieFile, Console: o.Console, To: to, All: all, Preview: preview, Repair: repair, Contract: contract}
			r.IncidentKind, r.IncidentPath = incidentKind, incidentPath
			if spec.verb == "preview" && len(args) > 2 {
				r.PreviewArgs = append([]string(nil), args[2:]...)
			}
			if spec.verb == "incident" {
				r.ID = incidentID
			}
			if spec.verb == "ledger" {
				r.ID, r.MaxBytes = base, maxBytes
			}
			if r.App == "." || r.App == ".." || strings.ContainsAny(r.App, "/\\") {
				return errors.New("shape: app must be one path segment")
			}
			if len(args) > 1 {
				if r.Verb == "wish" || r.Verb == "incident" {
					r.Request = args[1]
				} else {
					r.ID = args[1]
				}
			}
			if spec.verb == "incident" && r.ID != "" && r.Request != "" {
				return errors.New("shape: incident --id cannot be combined with a new report")
			}
			if story != "" && todo != "" {
				return errors.New("shape: --story and --todo are mutually exclusive")
			}
			if r.Verb == "wish" && r.From == "" && strings.TrimSpace(r.Request) == "" {
				return errors.New("shape: wish needs REQUEST or --from WORKSPACE")
			}
			if source != "" {
				if o.Remote != "" {
					return errors.New("shape: --source is local only; a remote wish takes its source from the console")
				}
				kind, ref, _ := strings.Cut(source, ":")
				r.Source = shape.Source{Kind: strings.TrimSpace(kind), Ref: strings.TrimSpace(ref)}
				if r.Source.Kind == "" {
					return errors.New("shape: --source wants KIND:REF")
				}
				if err := r.Source.Validate(); err != nil {
					return err
				}
			}
			if o.Remote != "" {
				if o.User != "" || o.Actor != "" {
					return errors.New("shape: remote identity comes from the authenticated console session; --user/--principal/--as cannot override it")
				}
			} else {
				r.User = strings.TrimSpace(o.User)
				if r.User == "" {
					r.User = strings.TrimSpace(os.Getenv("BASHY_PRINCIPAL"))
				}
				if r.User == "" {
					return errors.New("shape: needs --user or BASHY_PRINCIPAL")
				}
				var err error
				r.Actor, err = shapeAuthoredActor(o.Actor)
				if err != nil {
					return err
				}
			}
			result, err := shapeCLIExecute(c.Context(), r)
			if result != nil {
				if outErr := writeShapeResult(c.OutOrStdout(), result, o.JSON); outErr != nil {
					return outErr
				}
			}
			if err == nil && spec.verb == "status" && !o.JSON && o.Remote == "" {
				fmt.Fprintln(c.OutOrStdout(), shapePromotionStatusLine(c.Context(), r.App))
			}
			return err
		}
		cmd.AddCommand(sub)
	}
	addShapePromotionCmds(cmd, &o)
	addShapeScopeCmd(cmd, &o)
	addShapeAutoCmd(cmd, &o)
	addShapePageCmds(cmd, &o)
	return cmd
}

func writeShapeResult(w io.Writer, result any, asJSON bool) error {
	if l, ok := result.(shape.Ledger); ok && !asJSON {
		result = l.Text()
	}
	if text, ok := result.(string); ok && !asJSON {
		_, err := fmt.Fprintln(w, text)
		return err
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(result)
}

func runShape(args []string) int {
	cmd := bashyShapeCmd()
	setCobraArgs(cmd, args)
	if err := cmd.Execute(); err != nil {
		if errors.Is(err, errShapePending) {
			return 3
		}
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}
