// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package agentos

import (
	"errors"

	"mvdan.cc/sh/v3/lower"
)

// checkBashSharpDiagnostics projects the engine's one static-analysis result
// into the front door's report shape. Parsing happens exactly once through the
// Bash# grammar, so shebangs, comments, and ordinary shell statements cannot
// disable checks and the CLI cannot drift from lowering semantics.
func checkBashSharpDiagnostics(filename string, err error) []checkDiagnostic {
	var list lower.ErrorList
	if !errors.As(err, &list) {
		return nil
	}
	diagnostics := make([]checkDiagnostic, 0, len(list))
	for _, d := range list {
		diagnostics = append(diagnostics, checkDiagnostic{
			Code: d.Code, Level: "error", File: filename,
			Line: d.Pos.Line(), Column: d.Pos.Col(), Message: d.Msg,
		})
	}
	return diagnostics
}
