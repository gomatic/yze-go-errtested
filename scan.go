// This file holds the assertion scan: reading a package's directory to learn
// which sentinels its tests match against.
//
// It is separate from the analyzer because it is a separate concern — the
// analyzer reasons about types in one compilation unit, this reads files off
// disk — and because keeping each file to one thing is what lets the 1:1
// test-layout rule give each of them its own test file.
//
// The corpus is the package's TEST BUILD, not the files whose names end in
// _test.go. Which files a build reads is the go tool's decision, so it is taken
// from go/build rather than restated here: a name the go tool skips (any
// leading `.` or `_`), or a build constraint the context does not satisfy,
// excludes the file from the test binary, and a file no test binary compiles is
// evidence about no test run. Each file is read ONCE and both answers about it
// — is it in the build, what does it assert — come off that one read, so the
// two can never be taken from different bytes.
//
// What the build compiles is only half of it; reach.go carries the other half,
// which is what a test RUN reaches.

package errtested

import (
	"bytes"
	"go/ast"
	"go/build"
	"go/parser"
	"go/token"
	"io"
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

// assertedSentinels is every sentinel name a run of the directory's tests
// matches against. A directory or file that cannot be read contributes nothing:
// the analyzer fails OPEN, since reporting a sentinel as unasserted because a
// file could not be opened would be a finding about the filesystem.
func assertedSentinels(dir dirReader, file fileReader, at dirPath) assertions {
	names, err := dir(at)
	if err != nil {
		return assertions{}
	}
	found := newCorpus()
	for _, name := range names {
		collectEntry(file, at, entryName(name), found)
	}
	return found.asserted()
}

// entryName is one name a directory listing yields.
type entryName string

// collectEntry reads one directory entry, once, and adds it to the corpus when
// the test build compiles it. Reading the file before judging it is what keeps
// the two answers about it — is it in the build, what does it assert — derived
// from one and the same bytes.
func collectEntry(file fileReader, at dirPath, name entryName, into corpus) {
	if !isTest(fileName(name)) {
		return
	}
	path := filePath(filepath.Join(string(at), string(name)))
	src, err := file(string(path))
	if err != nil {
		return
	}
	if !inTestBuild(src, at, name) {
		return
	}
	collectSource(path, src, into)
}

// buildContext is a go/build context serving one file's already-read bytes, so
// the go tool's own selection rules are applied to exactly the source the scan
// goes on to parse.
func buildContext(src []byte) build.Context {
	context := build.Default
	context.OpenFile = func(string) (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(src)), nil
	}
	return context
}

// inTestBuild reports whether the named source is a file the go tool would
// compile into this package's test binary. A file it would not — because the
// name is one the tool skips, or because a build constraint excludes it — is in
// no test binary, so a run of the tests never executes a line of it.
func inTestBuild(src []byte, at dirPath, name entryName) bool {
	context := buildContext(src)
	matched, err := context.MatchFile(string(at), string(name))
	return err == nil && matched
}

// filePath is the location of one test file being scanned.
type filePath string

// collectSource adds one test file's top-level declarations to the corpus.
//
// The file is parsed for syntax only — no type information crosses the pass
// boundary — so sentinels are matched by NAME. Within one package's own tests a
// name identifies a single sentinel, and matching by name is what lets the
// external test package count at all.
func collectSource(path filePath, src []byte, into corpus) {
	parsed, err := parser.ParseFile(token.NewFileSet(), string(path), src, 0)
	if err != nil {
		return
	}
	fields := expectationPositions(parsed)
	for _, decl := range parsed.Decls {
		collectDecl(decl, fields, into)
	}
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
