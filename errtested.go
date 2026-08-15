// Package errtested provides a go/analysis analyzer enforcing that every error
// sentinel a package can emit is asserted by that package's own tests.
//
// The gomatic testing standard holds that every failure path is part of the
// contract and gets a test asserting the SPECIFIC failure, not merely that an
// error occurred. Statement coverage cannot see this: a sentinel's return
// statement is covered the moment any test walks that line, whether or not the
// test checks which error came back.
//
// A sentinel is "emitted" when a non-test file of the package references it from
// inside a function body — a reference in a const or var declaration is a
// re-export, not an emission, and is deliberately ignored. It is "asserted" when
// a test file IN THE PACKAGE'S DIRECTORY passes it to an errors.Is-style
// matcher, or binds it to a want/expect-prefixed field of a table case (the
// shape yze/errtest mandates, where the loop matches tt.wantErr generically and
// the sentinel appears only in the case literal).
//
// The assertion corpus is read from the DIRECTORY rather than from the analysis
// pass, and that is the whole point. Go splits a package's tests across two
// compilation units — the internal `package p` test files and the external
// `package p_test` ones — which go/analysis presents as separate passes. A pass
// over p can never see p_test's files, so reading assertions from the pass
// reported every sentinel asserted only from the external test package, which
// is the dominant style. The directory holds both. yze/testfile reads the
// directory for the same reason.
//
// Reading the directory is not reading its filenames. The evidence this rule
// accepts is a test RUN, so a file credits a sentinel only when both of these
// hold, and forging either one means writing a test that runs:
//
//   - THE TEST BUILD COMPILES THE FILE. Which files a build reads is the go
//     tool's decision and is taken from go/build, not restated here: a name the
//     tool skips (any leading `.` or `_`) and a build constraint the context
//     does not satisfy both leave the file out of the test binary, and a file no
//     test binary compiles is evidence about no test run. A consequence worth
//     stating: build constraints are evaluated for the platform the analyzer
//     runs on, the same platform the emission side is read for, so a test file
//     constrained to another GOOS credits nothing here — just as it asserts
//     nothing there.
//
//   - A TEST RUN REACHES THE CODE. Compiling a line is not running it as a test.
//     `var _ = errors.Is(nil, ErrThing)` does execute, at init, and asserts
//     nothing about any test: nothing can name the blank identifier, so no test
//     can be the thing that runs it. An assertion counts inside a test entry
//     point (a Test, Benchmark, Fuzz or Example function, including a suite
//     method named that way), or inside any declaration a reachable one names —
//     which is how a package-level table counts, through the test that reads it.
//     Reachability is by name and errs toward accepting, exactly as the matcher
//     set does: every identifier a body mentions is an edge, and two files
//     declaring one name share its scope.
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
	"path/filepath"
	"slices"
	"sort"
	"strings"

	goyze "github.com/gomatic/go-yze"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
)

// message is the diagnostic emitted for an unasserted sentinel.
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

// dirPath is the filesystem path of the analyzed package's directory.
type dirPath string

// memberName is an identifier being classified — a callee or a struct field.
type memberName string

// sentinelName is an error sentinel's identifier.
type sentinelName string

// emissions records, per sentinel, the position it is first emitted from.
type emissions map[sentinelName]token.Pos

// run reports every sentinel emitted by the package's non-test code that none
// of its tests assert.
func run(pass *analysis.Pass) (any, error) {
	ins := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)
	emitted := emittedSentinels(pass, ins)
	if len(emitted) == 0 {
		return nil, nil
	}
	report(pass, emitted, assertedSentinels(readDir, readFile, packageDir(pass.Fset, pass.Files)))
	return nil, nil
}

// emittedSentinels records every sentinel referenced from inside a function body
// in a non-test file, at its earliest such position.
func emittedSentinels(pass *analysis.Pass, ins *inspector.Inspector) emissions {
	found := emissions{}
	ins.WithStack([]ast.Node{(*ast.Ident)(nil)}, func(n ast.Node, isPush bool, stack []ast.Node) bool {
		if !isPush || isTest(fileOf(pass, n)) || !inFuncBody(stack) {
			return true
		}
		found.add(sentinelOf(pass.TypesInfo, n), n.Pos())
		return true
	})
	return found
}

// add records pos as the sentinel's emission site, keeping the earliest seen. An
// empty name (the expression was not a sentinel) is ignored.
func (e emissions) add(name sentinelName, pos token.Pos) {
	if name == "" {
		return
	}
	if prior, seen := e[name]; seen && prior <= pos {
		return
	}
	e[name] = pos
}

// sortedUnasserted returns the emitted sentinels absent from asserted, ordered
// by emission position so output is deterministic.
func (e emissions) sortedUnasserted(asserted assertions) []sentinelName {
	out := make([]sentinelName, 0, len(e))
	for name := range e {
		if !asserted[name] {
			out = append(out, name)
		}
	}
	sort.Slice(out, func(i, j int) bool { return e[out[i]] < e[out[j]] })
	return out
}

// report emits a diagnostic at each unasserted sentinel's emission site.
func report(pass *analysis.Pass, emitted emissions, asserted assertions) {
	for _, name := range emitted.sortedUnasserted(asserted) {
		pass.Reportf(emitted[name], message, string(name))
	}
}

// packageDir is the directory holding the package under analysis, or empty when
// the pass carries no files.
func packageDir(fset *token.FileSet, files []*ast.File) dirPath {
	for _, file := range files {
		return dirPath(filepath.Dir(fset.Position(file.Pos()).Filename))
	}
	return ""
}

// sentinelOf returns the name of the error-sentinel constant node denotes, or
// empty when node is not a reference to one.
func sentinelOf(info *types.Info, node ast.Node) sentinelName {
	ident := identOf(node)
	if ident == nil {
		return ""
	}
	constant, ok := info.ObjectOf(ident).(*types.Const)
	if !ok || !implementsError(constant.Type()) {
		return ""
	}
	return sentinelName(constant.Name())
}

// nameOf is the identifier an expression names, ignoring any qualifier.
func nameOf(node ast.Node) sentinelName {
	if ident := identOf(node); ident != nil {
		return sentinelName(ident.Name)
	}
	return ""
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

// errorInterface is the builtin error interface, resolved once. Universe binds
// error to an interface, so resolving it here keeps implementsError free of a
// branch no input could take.
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
