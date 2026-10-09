// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package agentos

import (
	"bytes"
	"encoding/json"
	"io"

	"github.com/spf13/cobra"
)

// fleetListSchemaVersion is the versioned envelope `bashy tool|model|agent
// list --json` (and the hidden plurals) emit from the front door. The fleet
// registry (yoke/pkg/fleet) still writes a bare JSON array; the release bar
// (docs/release-bar.md) requires a top-level schema_version on every probed
// --json surface, and the five-point rule does not accept an array. The
// envelope is additive: `items` is the array exactly as the registry wrote
// it, so a consumer that selected `.[]` now selects `.items[]`.
const fleetListSchemaVersion = "bashy-fleet-list-v1"

type fleetListEnvelope struct {
	SchemaVersion string          `json:"schema_version"`
	Kind          string          `json:"kind"`
	Items         json.RawMessage `json:"items"`
}

// wrapFleetListEnvelope rewrites the registry's `list --json` array into the
// bashy-fleet-list-v1 envelope on the way out. The bare noun shares the
// list verb's RunE (fleet.newRoot), so the root is wrapped too unless the
// front door already gave it another body (`bashy agent` is the roster).
// Anything that is not a JSON array — the text table, or a registry that
// has grown its own envelope — passes through byte-for-byte.
func wrapFleetListEnvelope(cmd *cobra.Command, kind string, root bool) {
	wrap := func(c *cobra.Command) {
		if c == nil || c.RunE == nil || c.Flags().Lookup("json") == nil {
			return
		}
		inner := c.RunE
		c.RunE = func(c *cobra.Command, args []string) error {
			if f := c.Flags().Lookup("json"); f == nil || !f.Changed {
				return inner(c, args)
			}
			real := c.OutOrStdout()
			var buf bytes.Buffer
			c.SetOut(&buf)
			runErr := inner(c, args)
			c.SetOut(real)
			if err := writeFleetListEnvelope(real, kind, buf.Bytes()); err != nil {
				return err
			}
			return runErr
		}
	}
	if root {
		wrap(cmd)
	}
	for _, sub := range cmd.Commands() {
		if sub.Name() == "list" {
			wrap(sub)
		}
	}
}

// writeFleetListEnvelope writes raw to w, wrapped in the versioned envelope
// when raw is a JSON array and verbatim otherwise.
func writeFleetListEnvelope(w io.Writer, kind string, raw []byte) error {
	trimmed := bytes.TrimSpace(raw)
	if !bytes.HasPrefix(trimmed, []byte("[")) || !json.Valid(trimmed) {
		_, err := w.Write(raw)
		return err
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(fleetListEnvelope{SchemaVersion: fleetListSchemaVersion, Kind: kind, Items: json.RawMessage(trimmed)})
}
