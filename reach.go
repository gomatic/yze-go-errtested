package errtested

// Reading the corpus as what a TEST RUN reaches, rather than as a bag of text.
//
// Compiling a file is not running it. `var _ = errors.Is(nil, ErrThing)` sits
// in the test binary and even executes, at init, and asserts nothing about any
// test — nothing can name the blank identifier, so no test can ever be the
// thing that runs it. The same is true of a helper no test calls and of a table
// no test reads. So an assertion is credited only where a run enters it: inside
// a test entry point, inside something a reachable declaration names, or inside
// a package-level table a reachable declaration names.
//
// Reachability is by NAME and it errs toward accepting, exactly as the matcher
// set does: a missed diagnostic costs a finding nobody sees, while a false
// reject fails a package whose sentinel is genuinely asserted and leaves its
// author no fix. So every identifier a body mentions is an edge, two files
// declaring the same name share one scope, and a method named Test is an entry
// point because a suite runner enters it.

import (
	"go/ast"
	"strings"
)

// reachName is the name a test run follows to reach a declaration: a function's
// own name, or a package-level variable's.
type reachName string

// reachSet is a set of such names.
type reachSet map[reachName]bool

// blank is the identifier nothing can name. A declaration bound to it is
// unreachable by construction — which is exactly what makes `var _ = ...` a
// forgery rather than evidence — and a mention of it in an assignment reaches
// nothing. So it is neither recorded nor followed.
const blank reachName = "_"

// scope is one top-level declaration, read as what it asserts and what it
// names.
type scope struct {
	asserts    assertions
	references reachSet
}

// scopes indexes each declaration by the name that reaches it.
type scopes map[reachName]scope

// corpus is a package's test build: every declaration its test files carry, and
// which of them a test run enters on its own.
type corpus struct {
	scopes scopes
	roots  reachSet
}

// newCorpus is an empty corpus ready to collect into.
func newCorpus() corpus {
	return corpus{scopes: scopes{}, roots: reachSet{}}
}

// entryPrefixes are the names `go test` enters a function by.
var entryPrefixes = []string{"Test", "Benchmark", "Fuzz", "Example"}

// isTestEntry reports whether a test run enters the named function on its own.
// A method carrying one of these names counts too: a testify suite runner
// enters it, and reading it as an entry point errs toward accepting.
func isTestEntry(name memberName) bool {
	for _, prefix := range entryPrefixes {
		if strings.HasPrefix(string(name), prefix) {
			return true
		}
	}
	return false
}

// scopeFor is the declaration's scope, created empty on first sight. Two
// declarations of one name — a helper in the internal test package and another
// in the external one — share it, which credits both wherever either is
// reached.
func (c corpus) scopeFor(name reachName) scope {
	if found, ok := c.scopes[name]; ok {
		return found
	}
	created := scope{asserts: assertions{}, references: reachSet{}}
	c.scopes[name] = created
	return created
}

// refer records a name the body mentions, which is what a run follows out of
// it. The blank identifier names nothing, so mentioning it reaches nothing.
func (s scope) refer(name reachName) {
	if name != blank {
		s.references[name] = true
	}
}

// record reads one declaration's body into its scope: the sentinels it asserts,
// and every name it mentions. A declaration bound to the blank identifier is
// read by nothing, because nothing can name it.
func (c corpus) record(name reachName, body ast.Node, fields expectationFields) {
	if name == blank {
		return
	}
	found := c.scopeFor(name)
	ast.Inspect(body, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.CallExpr:
			fromCall(node, found.asserts)
		case *ast.KeyValueExpr:
			fromField(node, found.asserts)
		case *ast.CompositeLit:
			fromUnkeyedCases(node, fields, found.asserts)
		case *ast.Ident:
			found.refer(reachName(node.Name))
		}
		return true
	})
}

// asserted is every sentinel a run of these tests matches against: the
// assertions of each entry point, and of everything reachable from one.
func (c corpus) asserted() assertions {
	found := assertions{}
	walked := reachSet{}
	for name := range c.roots {
		c.walk(name, walked, found)
	}
	return found
}

// walk folds one declaration's assertions in and follows the names it mentions.
// walked bounds the walk: test helpers call one another, and two that call each
// other would otherwise never return.
func (c corpus) walk(name reachName, walked reachSet, into assertions) {
	if walked[name] {
		return
	}
	walked[name] = true
	found, ok := c.scopes[name]
	if !ok {
		return
	}
	for sentinel := range found.asserts {
		into.mark(sentinel)
	}
	for next := range found.references {
		c.walk(next, walked, into)
	}
}

// collectDecl files one top-level declaration under the name that reaches it.
// An import declaration names nothing a test run can enter and is skipped.
func collectDecl(decl ast.Decl, fields expectationFields, into corpus) {
	switch typed := decl.(type) {
	case *ast.FuncDecl:
		collectFunc(typed, fields, into)
	case *ast.GenDecl:
		for _, spec := range typed.Specs {
			collectValue(spec, fields, into)
		}
	}
}

// collectFunc files a function's body under its own name, and marks it a root
// when a test run enters it directly. A declaration with no body is somebody
// else's code — an assembly stub — and holds nothing to read.
func collectFunc(decl *ast.FuncDecl, fields expectationFields, into corpus) {
	name := reachName(decl.Name.Name)
	if isTestEntry(memberName(decl.Name.Name)) {
		into.roots[name] = true
	}
	if decl.Body == nil {
		return
	}
	into.record(name, decl.Body, fields)
}

// collectValue files a package-level variable's initializer under the
// variable's own name, so it is read only where reachable code names it. A
// name with no initializer of its own — the extra names of a multi-valued
// declaration — carries nothing to read.
func collectValue(spec ast.Spec, fields expectationFields, into corpus) {
	valued, ok := spec.(*ast.ValueSpec)
	if !ok {
		return
	}
	for at, name := range valued.Names {
		if at >= len(valued.Values) {
			return
		}
		into.record(reachName(name.Name), valued.Values[at], fields)
	}
}
