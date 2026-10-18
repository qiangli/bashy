// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package agentos

import (
	"github.com/qiangli/yoke/pkg/kbmemory"
	"github.com/spf13/cobra"
)

// newKBMemoryCmd is the one `kb memory` tree: the runtime dispatch and the
// schema projection both build it here, so capture and retrieval cannot drift
// apart. Capture owns the parent command (init/select/add/inspect/update/
// supersede/forget); Wave2 lexical retrieval hangs off it as two subcommands
// owned by the retrieval story:
//
//	kb memory retrieve TARGET --index INDEX   scoped lexical recall + freshness
//	kb memory index-sync TARGET --index INDEX explicit index build/reconcile
//	kb memory maintain VERB TARGET [ID]       explicit candidates/supersede/expire/delete/export/import/backup/restore
//
// Retrieval commands run the same parsers and envelopes as their MCP tools
// (kbmemory.MCPRetrieveCommand/MCPSyncIndexCommand), so the CLI and MCP share
// one contract. Failure codes stay on kbExitCode, the same ExitError mapping
// capture already uses.
func newKBMemoryCmd() *cobra.Command {
	cmd := kbmemory.NewMemoryCmd()
	cmd.AddCommand(kbmemory.NewRetrieveCmd(), kbmemory.NewSyncIndexCmd(), kbmemory.NewMaintainCmd())
	return cmd
}
