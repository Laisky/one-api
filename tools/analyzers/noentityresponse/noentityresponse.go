// Command/analyzer noentityresponse forbids serializing raw management-API
// entities (model.User, model.Token, model.Channel, model.Redemption,
// model.Log) at the HTTP boundary.
//
// It is the compile-time gate (E3) described in
// docs/proposals/20260714_boundary-response-dtos.md. The external S2 strict-out
// contract (external UUID identifiers only; no internal integer ids; no
// secrets) used to be enforced by a value-receiver MarshalJSON on each entity —
// an ambient guarantee that also governed cache/log serialization and produced
// production incident #353. That guarantee now lives in explicit dto.*Response
// shapes built at the boundary; this analyzer replaces the type-level guarantee
// with a mechanical one, catching any handler (including brand-new ones the
// runtime contract gate does not yet know about) that hands a raw entity to a
// gin JSON responder or to json.Marshal.
//
// It is type-aware on purpose: gin.H{"data": users} needs type resolution to
// see that users is []*model.User, which a purely syntactic linter (ast-grep)
// cannot do.
package noentityresponse

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"os"
	"strings"

	"golang.org/x/tools/go/analysis"
)

// Analyzer is the go/analysis entry point.
var Analyzer = &analysis.Analyzer{
	Name: "noentityresponse",
	Doc: "reports raw management-API entities (User/Token/Channel/Redemption/Log) " +
		"passed to a gin JSON responder or json.Marshal; return a dto.*Response " +
		"mapper at the boundary instead of serializing the entity",
	Run: run,
}

// Configurable package paths (overridable so the analyzer's own tests can point
// at lightweight stub packages instead of the whole repo).
var (
	modelPkg = "github.com/Laisky/one-api/model"
	ginPkg   = "github.com/gin-gonic/gin"
	jsonPkg  = "encoding/json"
	strict   = true
)

func init() {
	Analyzer.Flags.StringVar(&modelPkg, "modelpkg", modelPkg, "import path of the package that defines the forbidden entities")
	Analyzer.Flags.StringVar(&ginPkg, "ginpkg", ginPkg, "import path of the gin package")
	Analyzer.Flags.BoolVar(&strict, "strict", strict, "report diagnostics (fail); when false, print warnings to stderr instead")
}

// forbiddenTypes are the entity type names in modelPkg whose raw serialization
// at the boundary would leak internal integer ids or secrets.
var forbiddenTypes = map[string]bool{
	"User":       true,
	"Token":      true,
	"Channel":    true,
	"Redemption": true,
	"Log":        true,
}

// ginJSONMethods are the *gin.Context response emitters that serialize their
// body argument to JSON.
var ginJSONMethods = map[string]bool{
	"JSON":                true,
	"IndentedJSON":        true,
	"PureJSON":            true,
	"AsciiJSON":           true,
	"SecureJSON":          true,
	"JSONP":               true,
	"AbortWithStatusJSON": true,
}

// allowlistSuffixes are file-path suffixes exempt from the rule. model/cache.go
// legitimately marshals a raw entity for the internal Redis object cache — with
// the entity marshalers retired, that serialization is honest (id-bearing) and
// desired; it is the one intentional internal whole-struct round-trip.
var allowlistSuffixes = []string{
	"model/cache.go",
}

func run(pass *analysis.Pass) (interface{}, error) {
	for _, file := range pass.Files {
		filename := pass.Fset.Position(file.Pos()).Filename
		if strings.HasSuffix(filename, "_test.go") || isAllowlisted(filename) {
			continue
		}
		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			body := jsonBodyArg(pass, call)
			if body == nil {
				return true
			}
			if name := findForbidden(pass, body, map[ast.Expr]bool{}); name != "" {
				report(pass, body.Pos(), name)
			}
			return true
		})
	}
	return nil, nil
}

func report(pass *analysis.Pass, pos token.Pos, entity string) {
	msg := fmt.Sprintf("raw model.%s reaches a JSON boundary; return dto.%sResponse "+
		"(or model.%ssToResponses for slices) instead of serializing the entity",
		entity, entity, entity)
	if strict {
		pass.Reportf(pos, "%s", msg)
		return
	}
	fmt.Fprintf(os.Stderr, "%s: warning: %s\n", pass.Fset.Position(pos), msg)
}

// isAllowlisted reports whether a file path is exempt from the rule.
func isAllowlisted(filename string) bool {
	norm := strings.ReplaceAll(filename, "\\", "/")
	for _, suffix := range allowlistSuffixes {
		if strings.HasSuffix(norm, suffix) {
			return true
		}
	}
	return false
}

// jsonBodyArg returns the argument expression that a JSON-emitting call
// serializes, or nil when the call is not a gin JSON responder / json.Marshal.
func jsonBodyArg(pass *analysis.Pass, call *ast.CallExpr) ast.Expr {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return nil
	}
	name := sel.Sel.Name

	// encoding/json.Marshal / MarshalIndent — body is the first argument.
	if name == "Marshal" || name == "MarshalIndent" {
		if isPkgLevelFunc(pass, sel, jsonPkg) && len(call.Args) >= 1 {
			return call.Args[0]
		}
		return nil
	}

	// (*gin.Context).JSON and friends — body is the last argument.
	if ginJSONMethods[name] {
		if isGinContextRecv(pass, sel.X) && len(call.Args) >= 1 {
			return call.Args[len(call.Args)-1]
		}
	}
	return nil
}

// isPkgLevelFunc reports whether sel resolves to a package-level function in the
// given import path (e.g. encoding/json.Marshal).
func isPkgLevelFunc(pass *analysis.Pass, sel *ast.SelectorExpr, pkgPath string) bool {
	obj := pass.TypesInfo.Uses[sel.Sel]
	fn, ok := obj.(*types.Func)
	if !ok {
		return false
	}
	// A package-level function has a nil receiver signature.
	if sig, ok := fn.Type().(*types.Signature); ok && sig.Recv() != nil {
		return false
	}
	return fn.Pkg() != nil && fn.Pkg().Path() == pkgPath
}

// isGinContextRecv reports whether the receiver expression has type
// *gin.Context (or gin.Context).
func isGinContextRecv(pass *analysis.Pass, recv ast.Expr) bool {
	t := pass.TypesInfo.TypeOf(recv)
	if t == nil {
		return false
	}
	if ptr, ok := t.(*types.Pointer); ok {
		t = ptr.Elem()
	}
	named, ok := t.(*types.Named)
	if !ok {
		return false
	}
	obj := named.Obj()
	return obj != nil && obj.Pkg() != nil && obj.Pkg().Path() == ginPkg && obj.Name() == "Context"
}

// findForbidden walks an expression the way encoding/json would descend into it
// — through gin.H / slice / array composite literals and address-of — and
// returns the first forbidden entity name it can prove statically, or "".
func findForbidden(pass *analysis.Pass, expr ast.Expr, seen map[ast.Expr]bool) string {
	expr = unparen(expr)
	if expr == nil || seen[expr] {
		return ""
	}
	seen[expr] = true

	switch e := expr.(type) {
	case *ast.CompositeLit:
		if name := forbiddenNamed(pass.TypesInfo.TypeOf(e)); name != "" {
			return name
		}
		for _, elt := range e.Elts {
			value := elt
			if kv, ok := elt.(*ast.KeyValueExpr); ok {
				value = kv.Value
			}
			if name := findForbidden(pass, value, seen); name != "" {
				return name
			}
		}
		return ""
	case *ast.UnaryExpr:
		// &entity, and the like.
		if name := forbiddenNamed(pass.TypesInfo.TypeOf(e)); name != "" {
			return name
		}
		return findForbidden(pass, e.X, seen)
	default:
		return forbiddenNamed(pass.TypesInfo.TypeOf(expr))
	}
}

// forbiddenNamed unwraps pointer/slice/array/map layers and returns the name of
// a forbidden entity type from modelPkg, or "".
func forbiddenNamed(t types.Type) string {
	switch u := t.(type) {
	case nil:
		return ""
	case *types.Pointer:
		return forbiddenNamed(u.Elem())
	case *types.Slice:
		return forbiddenNamed(u.Elem())
	case *types.Array:
		return forbiddenNamed(u.Elem())
	case *types.Map:
		return forbiddenNamed(u.Elem())
	case *types.Named:
		obj := u.Obj()
		if obj != nil && obj.Pkg() != nil && obj.Pkg().Path() == modelPkg && forbiddenTypes[obj.Name()] {
			return obj.Name()
		}
		return ""
	default:
		return ""
	}
}

// unparen strips enclosing parentheses.
func unparen(e ast.Expr) ast.Expr {
	for {
		p, ok := e.(*ast.ParenExpr)
		if !ok {
			return e
		}
		e = p.X
	}
}
