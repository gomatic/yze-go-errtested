// Package errtested provides a go/analysis analyzer enforcing that every error
// sentinel a package can emit is asserted by that package's own tests.
//
// The gomatic testing standard holds that every failure path is part of the
// contract and gets a test asserting the SPECIFIC failure, not merely that an
// error occurred. Statement coverage cannot see this: a sentinel's return
// statement is covered the moment any test walks that line, whether or not the
// test checks which error came back.
//
// Scope is one package. A sentinel is "emitted" when a non-test file of the
// package references it from inside a function body — a reference in a const or
// var declaration is a re-export, not an emission, and is deliberately ignored.
// A sentinel is "asserted" when a test file of the same package passes it to an
// errors.Is-style matcher, or binds it to a want/expect-prefixed field of a
// table case (the shape yze/errtest mandates, where the loop matches
// tt.wantErr generically and the sentinel appears only in the case literal).
//
// Keying on the emission site rather than the declaration site is what keeps
// this analyzer quiet across module boundaries: a library that declares
// sentinels it never returns is not asked to test them, and the consumer that
// actually returns one is.
package errtested

import (
	"go/ast"
	"go/token"
	"go/types"
	"slices"
	"sort"
	"strings"

	goyze "github.com/gomatic/go-yze"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
)

// message is the diagnostic emitted for an unasserted sentinel. It is reported
// against the package's tests rather than the emission site, because the defect
// is a missing test and that is where the fix belongs — and because a diagnostic
// anchored in a non-test file would be unreachable for the plain (test-free)
// pass of the same package, which by design stays silent.
const message = "sentinel %s is emitted by this package but no test asserts it with errors.Is"

// Analyzer reports sentinels a package emits without any test asserting them.
var Analyzer = &analysis.Analyzer{
	Name:     "errtested",
	Doc:      "reports error sentinels a package emits that no test in the package asserts with errors.Is",
	Requires: []*analysis.Analyzer{inspect.Analyzer},
	Run:      run,
}

// Registration declares this analyzer to the yze framework.
var Registration = goyze.Registration{
	Name:       "errtested",
	Categories: []goyze.Category{"errors", "tests"},
	URL:        "https://docs.gomatic.dev/yze/errtested",
	Analyzer:   Analyzer,
}

// fileName is a source file's path as recorded in the pass's file set.
type fileName string

// memberName is an identifier being classified — a callee or a struct field.
type memberName string

// sites records, per sentinel, the position it was first seen at.
type sites map[*types.Const]token.Pos

// run reports every sentinel emitted by the package's non-test code that none
// of its test files assert.
func run(pass *analysis.Pass) (any, error) {
	if !isCorrelatable(pass) {
		return nil, nil
	}
	ins := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)
	emitted, asserted := sites{}, sites{}
	collectEmitted(pass, ins, emitted)
	collectAsserted(pass, ins, asserted)
	report(pass, emitted, asserted)
	return nil, nil
}

// isCorrelatable reports whether this pass can see both an emission and its
// assertion — that is, whether it holds test files AND non-test files at once.
//
// go/analysis presents a package as several passes: the plain package (no test
// files) and its test variant (non-test files plus any `package a` test files).
// Only the latter can correlate the two, so the plain pass must stay silent or
// every sentinel would be reported as unasserted. This same guard means a
// package whose tests live entirely in an EXTERNAL `package a_test` is not
// analyzed: its assertions sit in a different package from the emissions, and
// no single pass holds both. Silence there is deliberate — a false positive on
// correct code would disqualify this as a gate.
func isCorrelatable(pass *analysis.Pass) bool {
	var tests, sources int
	for _, file := range pass.Files {
		if isTest(fileOf(pass, file)) {
			tests++
			continue
		}
		sources++
	}
	return tests > 0 && sources > 0
}

// collectEmitted records every sentinel referenced from inside a function body
// in a non-test file.
func collectEmitted(pass *analysis.Pass, ins *inspector.Inspector, into sites) {
	ins.WithStack([]ast.Node{(*ast.Ident)(nil)}, func(n ast.Node, isPush bool, stack []ast.Node) bool {
		if !isPush || isTest(fileOf(pass, n)) || !inFuncBody(stack) {
			return true
		}
		into.add(sentinelOf(pass, n), n.Pos())
		return true
	})
}

// collectAsserted records every sentinel a test file matches against.
func collectAsserted(pass *analysis.Pass, ins *inspector.Inspector, into sites) {
	nodes := []ast.Node{(*ast.CallExpr)(nil), (*ast.KeyValueExpr)(nil)}
	ins.Preorder(nodes, func(n ast.Node) {
		if !isTest(fileOf(pass, n)) {
			return
		}
		switch node := n.(type) {
		case *ast.CallExpr:
			fromCall(pass, node, into)
		case *ast.KeyValueExpr:
			fromField(pass, node, into)
		}
	})
}

// fromCall records the sentinels passed to an errors.Is-style matcher.
func fromCall(pass *analysis.Pass, call *ast.CallExpr, into sites) {
	if !isErrorMatcher(calleeOf(call)) {
		return
	}
	for _, arg := range call.Args {
		into.add(sentinelOf(pass, arg), arg.Pos())
	}
}

// fromField records the sentinel bound to a want/expect-prefixed case field.
func fromField(pass *analysis.Pass, kv *ast.KeyValueExpr, into sites) {
	key, ok := kv.Key.(*ast.Ident)
	if !ok || !isExpectation(memberName(key.Name)) {
		return
	}
	into.add(sentinelOf(pass, kv.Value), kv.Value.Pos())
}

// report emits a diagnostic for each emitted sentinel that is never asserted,
// in emission order so the output is deterministic, anchored at the package's
// tests where the missing assertion belongs.
func report(pass *analysis.Pass, emitted, asserted sites) {
	at := testAnchor(pass)
	for _, sentinel := range emitted.sortedUnasserted(asserted) {
		pass.Reportf(at, message, sentinel.Name())
	}
}

// testAnchor is the position every diagnostic is reported at: the package
// clause of the pass's first test file by name, chosen so the anchor is stable
// no matter what order the loader presents files in.
func testAnchor(pass *analysis.Pass) token.Pos {
	first := ""
	at := token.NoPos
	for _, file := range pass.Files {
		name := string(fileOf(pass, file))
		if !isTest(fileName(name)) || (first != "" && name >= first) {
			continue
		}
		first, at = name, file.Package
	}
	return at
}

// add records pos as sentinel's site, keeping the earliest position seen. A nil
// sentinel (the expression was not one) is ignored.
func (s sites) add(sentinel *types.Const, pos token.Pos) {
	if sentinel == nil {
		return
	}
	if prior, seen := s[sentinel]; seen && prior <= pos {
		return
	}
	s[sentinel] = pos
}

// sortedUnasserted returns the sentinels in s that are absent from asserted,
// ordered by their recorded position.
func (s sites) sortedUnasserted(asserted sites) []*types.Const {
	out := make([]*types.Const, 0, len(s))
	for sentinel := range s {
		if _, ok := asserted[sentinel]; !ok {
			out = append(out, sentinel)
		}
	}
	sort.Slice(out, func(i, j int) bool { return s[out[i]] < s[out[j]] })
	return out
}

// sentinelOf returns the error-sentinel constant node denotes, or nil when node
// is not a reference to one.
func sentinelOf(pass *analysis.Pass, node ast.Node) *types.Const {
	ident := identOf(node)
	if ident == nil {
		return nil
	}
	constant, ok := pass.TypesInfo.ObjectOf(ident).(*types.Const)
	if !ok || !implementsError(constant.Type()) {
		return nil
	}
	return constant
}

// identOf reduces a node to the identifier naming it, unwrapping parentheses
// and a qualified selector (pkg.ErrThing) to its selected name.
func identOf(node ast.Node) *ast.Ident {
	switch n := node.(type) {
	case *ast.Ident:
		return n
	case *ast.SelectorExpr:
		return n.Sel
	case *ast.ParenExpr:
		return identOf(n.X)
	}
	return nil
}

// errorInterface is the builtin error interface, resolved once. Universe's
// error is always an interface, so binding it here keeps implementsError free
// of a branch that no input could ever take.
var errorInterface = types.Universe.Lookup("error").Type().Underlying().(*types.Interface)

// implementsError reports whether t satisfies the builtin error interface.
func implementsError(t types.Type) bool {
	return t != nil && types.Implements(types.Unalias(t), errorInterface)
}

// inFuncBody reports whether the inspected node sits inside a function or
// closure body, which is what distinguishes emitting a sentinel from
// re-declaring one.
func inFuncBody(stack []ast.Node) bool {
	return slices.ContainsFunc(stack, isFunc)
}

// isFunc reports whether node introduces a function body.
func isFunc(node ast.Node) bool {
	switch node.(type) {
	case *ast.FuncDecl, *ast.FuncLit:
		return true
	}
	return false
}

// fileOf is the path of the file containing n.
func fileOf(pass *analysis.Pass, n ast.Node) fileName {
	return fileName(pass.Fset.Position(n.Pos()).Filename)
}

// isTest reports whether name is a Go test file.
func isTest(name fileName) bool {
	return strings.HasSuffix(string(name), "_test.go")
}

// calleeOf is the name of the function or method a call invokes.
func calleeOf(call *ast.CallExpr) memberName {
	ident := identOf(call.Fun)
	if ident == nil {
		return ""
	}
	return memberName(ident.Name)
}

// isErrorMatcher reports whether name is an errors.Is-style matcher — the
// stdlib functions and testify's assertion forms alike. The set errs toward
// accepting: a false accept only costs a missed diagnostic, while a false
// reject would fail a package whose sentinel is genuinely asserted.
func isErrorMatcher(name memberName) bool {
	switch strings.TrimSuffix(string(name), "f") {
	case "Is", "As", "ErrorIs", "ErrorAs", "NotErrorIs":
		return true
	}
	return false
}

// isExpectation reports whether name is a table case's expectation field, whose
// value carries the sentinel the loop matches generically.
func isExpectation(name memberName) bool {
	lower := strings.ToLower(string(name))
	return strings.HasPrefix(lower, "want") || strings.HasPrefix(lower, "expect")
}
