// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package agentos

import (
	"go/ast"
	"go/parser"
	"go/token"
)

// checkBashPPNullSafety parses a Go-shaped Bash++ source file without changing
// the shell grammar. Bash# null safety is deliberately a checker rule: the
// package clause exists only in this private parse buffer and is never lowered
// or executed. false means the input is not a complete Go-shaped source unit,
// so the ordinary shell checker remains responsible for it.
func checkBashPPNullSafety(filename string, src []byte) (bool, []checkDiagnostic) {
	wrapped := append([]byte("package bashpp\n"), src...)
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filename, wrapped, parser.SkipObjectResolution)
	if err != nil {
		return false, nil
	}

	var diagnostics []checkDiagnostic
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		a := nullFuncAnalyzer{
			filename: filename,
			fset:     fset,
			kinds:    make(map[string]nullKind),
			seen:     make(map[string]bool),
		}
		a.collectParams(fn.Type.Params)
		a.analyzeBlock(fn.Body, newNullState())
		diagnostics = append(diagnostics, a.diagnostics...)
	}
	return true, diagnostics
}

type nullKind uint8

const (
	nullPointer nullKind = iota + 1
	nullIndexable
	nullCallable
)

type nullState struct {
	nonNil     map[string]bool
	reassigned map[string]bool
}

func newNullState() nullState {
	return nullState{nonNil: make(map[string]bool), reassigned: make(map[string]bool)}
}

func (s nullState) clone() nullState {
	out := newNullState()
	for name, value := range s.nonNil {
		out.nonNil[name] = value
	}
	for name, value := range s.reassigned {
		out.reassigned[name] = value
	}
	return out
}

type nullFuncAnalyzer struct {
	filename    string
	fset        *token.FileSet
	kinds       map[string]nullKind
	seen        map[string]bool
	diagnostics []checkDiagnostic
}

func (a *nullFuncAnalyzer) collectParams(fields *ast.FieldList) {
	if fields == nil {
		return
	}
	for _, field := range fields.List {
		kind := nullableKind(field.Type)
		if kind == 0 {
			continue
		}
		for _, name := range field.Names {
			a.kinds[name.Name] = kind
		}
	}
}

func nullableKind(expr ast.Expr) nullKind {
	switch x := expr.(type) {
	case *ast.ParenExpr:
		return nullableKind(x.X)
	case *ast.StarExpr:
		return nullPointer
	case *ast.ArrayType:
		if x.Len == nil {
			return nullIndexable
		}
	case *ast.MapType:
		return nullIndexable
	case *ast.FuncType:
		return nullCallable
	}
	return 0
}

// analyzeBlock returns the state on paths that leave the block normally and
// whether every path terminates. That distinction is what turns
// `if p == nil { return }` into a durable narrowing after the guard.
func (a *nullFuncAnalyzer) analyzeBlock(block *ast.BlockStmt, state nullState) (nullState, bool) {
	if block == nil {
		return state, false
	}
	current := state.clone()
	for _, stmt := range block.List {
		var terminates bool
		current, terminates = a.analyzeStmt(stmt, current)
		if terminates {
			return current, true
		}
	}
	return current, false
}

func (a *nullFuncAnalyzer) analyzeStmt(stmt ast.Stmt, state nullState) (nullState, bool) {
	switch s := stmt.(type) {
	case *ast.ReturnStmt:
		for _, result := range s.Results {
			a.checkExpr(result, state)
		}
		return state, true
	case *ast.IfStmt:
		current := state.clone()
		if s.Init != nil {
			current, _ = a.analyzeStmt(s.Init, current)
		}
		a.checkExpr(s.Cond, current)
		trueState := a.refine(current, s.Cond, true)
		falseState := a.refine(current, s.Cond, false)
		thenState, thenTerminates := a.analyzeBlock(s.Body, trueState)
		elseState, elseTerminates := falseState, false
		if s.Else != nil {
			elseState, elseTerminates = a.analyzeStmt(s.Else, falseState)
		}
		switch {
		case thenTerminates && elseTerminates:
			return mergeNullStates(thenState, elseState), true
		case thenTerminates:
			return elseState, false
		case elseTerminates:
			return thenState, false
		default:
			return mergeNullStates(thenState, elseState), false
		}
	case *ast.BlockStmt:
		return a.analyzeBlock(s, state)
	case *ast.AssignStmt:
		current := state.clone()
		for _, rhs := range s.Rhs {
			a.checkExpr(rhs, current)
		}
		for _, lhs := range s.Lhs {
			a.checkExpr(lhs, current)
			if id, ok := lhs.(*ast.Ident); ok && a.kinds[id.Name] != 0 {
				current.nonNil[id.Name] = definitelyNonNilAssignment(s.Rhs)
				current.reassigned[id.Name] = true
			}
		}
		return current, false
	case *ast.ExprStmt:
		a.checkExpr(s.X, state)
	case *ast.IncDecStmt:
		a.checkExpr(s.X, state)
	case *ast.DeclStmt:
		current := state.clone()
		if decl, ok := s.Decl.(*ast.GenDecl); ok {
			for _, spec := range decl.Specs {
				value, ok := spec.(*ast.ValueSpec)
				if !ok {
					continue
				}
				for _, init := range value.Values {
					a.checkExpr(init, current)
				}
				kind := nullableKind(value.Type)
				for _, name := range value.Names {
					if kind != 0 {
						a.kinds[name.Name] = kind
						current.nonNil[name.Name] = definitelyNonNilAssignment(value.Values)
					}
				}
			}
		}
		return current, false
	}
	return state, false
}

func definitelyNonNilAssignment(values []ast.Expr) bool {
	if len(values) != 1 {
		return false
	}
	switch value := values[0].(type) {
	case *ast.UnaryExpr:
		return value.Op == token.AND
	case *ast.FuncLit, *ast.CompositeLit:
		return true
	}
	return false
}

func mergeNullStates(left, right nullState) nullState {
	out := newNullState()
	for name, value := range left.nonNil {
		out.nonNil[name] = value && right.nonNil[name]
	}
	for name, value := range right.nonNil {
		if _, ok := out.nonNil[name]; !ok {
			out.nonNil[name] = value && left.nonNil[name]
		}
	}
	for name, value := range left.reassigned {
		out.reassigned[name] = value
	}
	for name, value := range right.reassigned {
		out.reassigned[name] = out.reassigned[name] || value
	}
	return out
}

func (a *nullFuncAnalyzer) refine(state nullState, expr ast.Expr, truth bool) nullState {
	out := state.clone()
	switch e := expr.(type) {
	case *ast.ParenExpr:
		return a.refine(out, e.X, truth)
	case *ast.UnaryExpr:
		if e.Op == token.NOT {
			return a.refine(out, e.X, !truth)
		}
	case *ast.BinaryExpr:
		if name, equal, ok := nilComparison(e); ok && a.kinds[name] != 0 {
			nilOnPath := truth == equal
			out.nonNil[name] = !nilOnPath
			return out
		}
		if e.Op == token.LAND && truth {
			return a.refine(a.refine(out, e.X, true), e.Y, true)
		}
		if e.Op == token.LOR && !truth {
			return a.refine(a.refine(out, e.X, false), e.Y, false)
		}
	}
	return out
}

func nilComparison(expr *ast.BinaryExpr) (name string, equal bool, ok bool) {
	if expr.Op != token.EQL && expr.Op != token.NEQ {
		return "", false, false
	}
	if id, yes := expr.X.(*ast.Ident); yes && isNilIdent(expr.Y) {
		return id.Name, expr.Op == token.EQL, true
	}
	if id, yes := expr.Y.(*ast.Ident); yes && isNilIdent(expr.X) {
		return id.Name, expr.Op == token.EQL, true
	}
	return "", false, false
}

func isNilIdent(expr ast.Expr) bool {
	id, ok := expr.(*ast.Ident)
	return ok && id.Name == "nil"
}

func (a *nullFuncAnalyzer) checkExpr(expr ast.Expr, state nullState) {
	if expr == nil {
		return
	}
	if binary, ok := expr.(*ast.BinaryExpr); ok && (binary.Op == token.LAND || binary.Op == token.LOR) {
		a.checkExpr(binary.X, state)
		rhsTruth := binary.Op == token.LAND
		a.checkExpr(binary.Y, a.refine(state, binary.X, rhsTruth))
		return
	}
	ast.Inspect(expr, func(node ast.Node) bool {
		if nested, ok := node.(*ast.BinaryExpr); ok && nested != expr && (nested.Op == token.LAND || nested.Op == token.LOR) {
			a.checkExpr(nested, state)
			return false
		}
		switch n := node.(type) {
		case *ast.StarExpr:
			if id, ok := n.X.(*ast.Ident); ok && a.kinds[id.Name] == nullPointer && !state.nonNil[id.Name] {
				message := id.Name + " may be nil when dereferenced"
				if state.reassigned[id.Name] {
					message = id.Name + " may be nil after reassignment"
				}
				a.add("BASHPP-ENULL-DEREF", message, n.Pos())
			}
		case *ast.IndexExpr:
			if id, ok := n.X.(*ast.Ident); ok && a.kinds[id.Name] == nullIndexable && !state.nonNil[id.Name] {
				a.add("BASHPP-ENULL-INDEX", id.Name+" may be nil when indexed", n.Pos())
			}
		case *ast.CallExpr:
			if id, ok := n.Fun.(*ast.Ident); ok && a.kinds[id.Name] == nullCallable && !state.nonNil[id.Name] {
				a.add("BASHPP-ENULL-CALL", id.Name+" may be nil when called", n.Pos())
			}
		}
		return true
	})
}

func (a *nullFuncAnalyzer) add(code, message string, pos token.Pos) {
	position := a.fset.Position(pos)
	key := code + "\x00" + message + "\x00" + position.String()
	if a.seen[key] {
		return
	}
	a.seen[key] = true
	line := position.Line - 1 // compensate for the checker-private package line
	if line < 1 {
		line = 1
	}
	a.diagnostics = append(a.diagnostics, checkDiagnostic{
		Code: code, Level: "error", File: a.filename,
		Line: uint(line), Column: uint(position.Column), Message: message,
	})
}
