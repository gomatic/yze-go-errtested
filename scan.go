// This file holds the assertion scan: reading a package's directory to learn
// which sentinels its tests match against.
//
// It is separate from the analyzer because it is a separate concern — the
// analyzer reasons about types in one compilation unit, this reads files off
// disk — and because keeping each file to one thing is what lets the 1:1
// test-layout rule give each of them its own test file.

package errtested

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
)

// mark records a sentinel name as asserted, ignoring an empty one.
func (a assertions) mark(name sentinelName) {
	if name != "" {
		a[name] = true
	}
}

// assertions is the set of sentinel names a package's tests match against.
type assertions map[sentinelName]bool

var (
	readDir  dirReader  = osReadDirNames
	readFile fileReader = os.ReadFile
)

// Injected collaborators, so the directory scan is testable without a real tree.
type (
	dirReader  func(dir dirPath) ([]string, error)
	fileReader func(path string) ([]byte, error)
)

// assertedSentinels is every sentinel name the directory's test files match
// against. A directory or file that cannot be read contributes nothing: the
// analyzer fails OPEN, since reporting a sentinel as unasserted because a file
// could not be opened would be a finding about the filesystem.
func assertedSentinels(dir dirReader, file fileReader, at dirPath) assertions {
	found := assertions{}
	names, err := dir(at)
	if err != nil {
		return found
	}
	for _, name := range names {
		if !isTest(fileName(name)) {
			continue
		}
		collectFile(file, filePath(filepath.Join(string(at), name)), found)
	}
	return found
}

// filePath is the location of one test file being scanned.
type filePath string

// collectFile adds every sentinel the one test file asserts.
//
// The file is parsed for syntax only — no type information crosses the pass
// boundary — so sentinels are matched by NAME. Within one package's own tests a
// name identifies a single sentinel, and matching by name is what lets the
// external test package count at all.
func collectFile(read fileReader, path filePath, into assertions) {
	src, err := read(string(path))
	if err != nil {
		return
	}
	parsed, err := parser.ParseFile(token.NewFileSet(), string(path), src, 0)
	if err != nil {
		return
	}
	fields := expectationPositions(parsed)
	ast.Inspect(parsed, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.CallExpr:
			fromCall(node, into)
		case *ast.KeyValueExpr:
			fromField(node, into)
		case *ast.CompositeLit:
			fromUnkeyedCases(node, fields, into)
		}
		return true
	})
}

// caseType is a table case's struct type name.
type caseType string

// fieldIndex is a field's position in its struct's declaration order.
type fieldIndex int

// expectationPositions indexes, for each struct type the file declares, the
// position its want/expect-prefixed field occupies — what an unkeyed case
// literal fills at that position. A struct with no such field is absent, so
// nothing about it is ever read positionally.
func expectationPositions(file *ast.File) map[caseType]fieldIndex {
	positions := map[caseType]fieldIndex{}
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok {
			continue
		}
		for _, spec := range gen.Specs {
			recordExpectationPosition(spec, positions)
		}
	}
	return positions
}

// recordExpectationPosition records spec's expectation-field position when it
// declares a struct carrying one.
func recordExpectationPosition(spec ast.Spec, into map[caseType]fieldIndex) {
	typed, ok := spec.(*ast.TypeSpec)
	if !ok {
		return
	}
	structure, ok := typed.Type.(*ast.StructType)
	if !ok || structure.Fields == nil {
		return
	}
	if at, found := expectationIndex(structure.Fields.List); found {
		into[caseType(typed.Name.Name)] = at
	}
}

// expectationIndex is the position of the first want/expect-prefixed field,
// counting each name of a grouped field separately — the order an unkeyed
// literal fills.
func expectationIndex(list []*ast.Field) (fieldIndex, bool) {
	for at, name := range positionalNames(list) {
		if isExpectation(memberName(name)) {
			return fieldIndex(at), true
		}
	}
	return 0, false
}

// positionalNames flattens a struct's fields into the order an unkeyed literal
// fills them, one entry per name; an embedded field contributes one unnamed
// position.
func positionalNames(list []*ast.Field) []string {
	var names []string
	for _, field := range list {
		if len(field.Names) == 0 {
			names = append(names, "")
			continue
		}
		for _, name := range field.Names {
			names = append(names, name.Name)
		}
	}
	return names
}

// fromUnkeyedCases records the sentinel each unkeyed case literal binds at its
// type's expectation position. A positional binding is the same assertion a
// keyed one makes — `{"empty", ErrEmpty}` and `{name: "empty", wantErr:
// ErrEmpty}` say the same thing — so the two forms are read alike. The scan is
// syntax-only, so only a case whose struct the same file declares can be read
// this way; anything else is left to the keyed path.
func fromUnkeyedCases(lit *ast.CompositeLit, fields map[caseType]fieldIndex, into assertions) {
	if named, ok := lit.Type.(*ast.Ident); ok {
		markCase(lit, fields, caseType(named.Name), into)
		return
	}
	element, ok := elementType(lit.Type)
	if !ok {
		return
	}
	for _, entry := range lit.Elts {
		if nested, ok := entry.(*ast.CompositeLit); ok {
			markCase(nested, fields, element, into)
		}
	}
}

// elementType is the named element type of a slice, array, or map literal.
func elementType(node ast.Expr) (caseType, bool) {
	var value ast.Expr
	switch typed := node.(type) {
	case *ast.ArrayType:
		value = typed.Elt
	case *ast.MapType:
		value = typed.Value
	default:
		return "", false
	}
	named, ok := value.(*ast.Ident)
	if !ok {
		return "", false
	}
	return caseType(named.Name), true
}

// markCase records the sentinel lit binds at its type's expectation position.
func markCase(lit *ast.CompositeLit, fields map[caseType]fieldIndex, of caseType, into assertions) {
	at, found := fields[of]
	if !found || int(at) >= len(lit.Elts) {
		return
	}
	if _, keyed := lit.Elts[at].(*ast.KeyValueExpr); keyed {
		return
	}
	into.mark(nameOf(lit.Elts[at]))
}

// fromCall records the sentinels passed to an errors.Is-style matcher.
func fromCall(call *ast.CallExpr, into assertions) {
	if !isErrorMatcher(calleeOf(call)) {
		return
	}
	for _, arg := range call.Args {
		into.mark(nameOf(arg))
	}
}

// fromField records the sentinel bound to a want/expect-prefixed case field.
func fromField(kv *ast.KeyValueExpr, into assertions) {
	key, ok := kv.Key.(*ast.Ident)
	if !ok || !isExpectation(memberName(key.Name)) {
		return
	}
	into.mark(nameOf(kv.Value))
}

// osReadDirNames lists the entry names of a directory.
func osReadDirNames(dir dirPath) ([]string, error) {
	entries, err := os.ReadDir(string(dir))
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	return names, nil
}
