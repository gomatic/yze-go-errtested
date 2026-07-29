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
	ast.Inspect(parsed, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.CallExpr:
			fromCall(node, into)
		case *ast.KeyValueExpr:
			fromField(node, into)
		}
		return true
	})
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
