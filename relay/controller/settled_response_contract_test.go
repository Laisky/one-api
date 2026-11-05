package controller

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestSettledResponseErrorContract guards the relay-helper settlement contract
// structurally. A helper that schedules post-billing for a response that may
// carry an error (a local respErr) must call markResponseSettlement, and must
// not report success with a bare "return nil" once settlement is scheduled.
// RelayResponseAPIHelper and RelayVoiceCloneHelper once did, so failed native
// Responses attempts were recorded as successes and an undecodable DeepSeek
// reply was answered with an empty HTTP 200.
func TestSettledResponseErrorContract(t *testing.T) {
	files, err := filepath.Glob("*.go")
	require.NoError(t, err)
	fset := token.NewFileSet()
	checked := map[string]bool{}
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, name, nil, parser.SkipObjectResolution)
		require.NoError(t, err)
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			billing, usesRespErr, marks := settledResponseFacts(fn.Body)
			if !billing.IsValid() || !usesRespErr {
				continue
			}
			checked[fn.Name.Name] = true
			require.True(t, marks, "%s schedules post-billing for respErr but never calls markResponseSettlement", fn.Name.Name)
			for _, ret := range topLevelReturns(fn.Body) {
				if ret.Pos() < billing || len(ret.Results) != 1 {
					continue
				}
				ident, ok := ret.Results[0].(*ast.Ident)
				require.False(t, ok && ident.Name == "nil",
					"%s returns nil after scheduling settlement at %s; report respErr instead", fn.Name.Name, fset.Position(ret.Pos()))
			}
		}
	}
	// The guard must keep covering the helpers that settle errored responses.
	for _, helper := range []string{"RelayTextHelper", "RelayResponseAPIHelper", "RelayClaudeMessagesHelper", "RelayRerankHelper", "RelayOCRHelper", "RelayVoiceCloneHelper"} {
		require.True(t, checked[helper], "contract guard no longer recognizes %s", helper)
	}
}

// settledResponseFacts reports the first runPostBillingWithTimeout call, whether
// the function handles a respErr value and whether it marks the settlement.
func settledResponseFacts(body *ast.BlockStmt) (billing token.Pos, usesRespErr, marks bool) {
	ast.Inspect(body, func(node ast.Node) bool {
		switch n := node.(type) {
		case *ast.Ident:
			if n.Name == "respErr" {
				usesRespErr = true
			}
		case *ast.CallExpr:
			if callee, ok := n.Fun.(*ast.Ident); ok {
				switch callee.Name {
				case "runPostBillingWithTimeout":
					if !billing.IsValid() || n.Pos() < billing {
						billing = n.Pos()
					}
				case "markResponseSettlement":
					marks = true
				}
			}
		}
		return true
	})
	return billing, usesRespErr, marks
}

// topLevelReturns lists the function's own return statements, excluding those
// of nested function literals such as the post-billing callback.
func topLevelReturns(body *ast.BlockStmt) []*ast.ReturnStmt {
	var returns []*ast.ReturnStmt
	ast.Inspect(body, func(node ast.Node) bool {
		switch n := node.(type) {
		case *ast.FuncLit:
			return false
		case *ast.ReturnStmt:
			returns = append(returns, n)
		}
		return true
	})
	return returns
}
