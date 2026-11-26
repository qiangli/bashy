package agentos

import (
	"encoding/json"
	"errors"
	"fmt"

	shapestate "github.com/qiangli/yoke/pkg/shape/state"
	"github.com/spf13/cobra"
)

// addShapeAutoCmd mounts `bashy shape auto`: the host switch, the one-run
// override and the decision log for unattended shaping (incident repair and
// every other source that shapes without a person asking). The state lives
// in yoke/pkg/shape/state; this is only the command line.
func addShapeAutoCmd(parent *cobra.Command, o *shapeCLIOptions) {
	auto := &cobra.Command{Use: "auto", Short: "Switch unattended shaping off or on for this host, and read its decision log"}
	for _, spec := range []struct{ verb, short string }{
		{"off", "Stop unattended shaping: incidents are still recorded, no repair starts, a running repair stops before its next attempt"},
		{"on", "Allow unattended shaping again"},
		{"status", "Show whether unattended shaping is on"},
		{"log", "Print the decision log"},
	} {
		spec := spec
		var override bool
		var reason string
		sub := &cobra.Command{Use: spec.verb, Short: spec.short, Args: cobra.NoArgs}
		if spec.verb == "on" {
			sub.Flags().BoolVar(&override, "override", false, "let the next unattended run exceed the host bound (requires --reason)")
			sub.Flags().StringVar(&reason, "reason", "", "why the override is needed; logged")
		}
		sub.RunE = func(c *cobra.Command, _ []string) error {
			if o.Remote != "" {
				return errors.New("shape auto: a host control; run it on the host, without --remote")
			}
			home, err := shapestate.Home()
			if err != nil {
				return err
			}
			out := c.OutOrStdout()
			switch spec.verb {
			case "log":
				log, err := shapestate.ReadAutoLog(home)
				if err != nil {
					return err
				}
				enc := json.NewEncoder(out)
				for _, d := range log {
					if o.JSON {
						if err := enc.Encode(d); err != nil {
							return err
						}
						continue
					}
					fmt.Fprintf(out, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n", d.Time.Format("2006-01-02T15:04:05Z07:00"), d.Event, d.Result, d.App, d.Source, d.Ref, d.Detail)
				}
				return nil
			case "off":
				if err := shapestate.SetAuto(home, false); err != nil {
					return err
				}
			case "on":
				// Refuse before changing anything: an override with no
				// reason must not switch the host on as a side effect.
				if reason != "" && !override {
					return errors.New("shape auto: --reason needs --override")
				}
				if override && reason == "" {
					return errors.New("shape auto: --override requires --reason")
				}
				if err := shapestate.SetAuto(home, true); err != nil {
					return err
				}
				if override {
					if err := shapestate.ArmAutoOverride(home, reason); err != nil {
						return err
					}
				}
			}
			state := "off"
			if shapestate.AutoEnabled(home) {
				state = "on"
			}
			pending, armed := shapestate.AutoOverridePending(home)
			if o.JSON {
				result := map[string]any{"auto": state}
				if armed {
					result["override"] = pending
				}
				return json.NewEncoder(out).Encode(result)
			}
			fmt.Fprintln(out, "unattended shaping:", state)
			if armed {
				fmt.Fprintln(out, "override armed for the next run:", pending)
			}
			return nil
		}
		auto.AddCommand(sub)
	}
	parent.AddCommand(auto)
}
