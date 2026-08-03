package errtested

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"testing"

	errs "github.com/gomatic/go-error"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// errRead is the failure an injected reader returns to exercise the fail-open
// paths.
const errRead errs.Const = "cannot read"

// TestAssertedSentinelsReadsBothTestPackages is the point of the directory scan.
// Go splits a package's tests into the internal `package p` files and the
// external `package p_test` ones, which go/analysis presents as separate passes;
// a pass over p can never see p_test. The directory holds both, so a sentinel
// asserted only from the external package still counts.
func TestAssertedSentinelsReadsBothTestPackages(t *testing.T) {
	t.Parallel()

	files := map[string]string{
		"a.go":           "package a\nfunc F() error { return ErrSource }\n",
		"a_test.go":      "package a\nfunc TestInternal(t *testing.T) { assert.ErrorIs(t, F(), ErrInternal) }\n",
		"a_ext_test.go":  "package a_test\nfunc TestExternal(t *testing.T) { assert.ErrorIs(t, a.F(), a.ErrExternal) }\n",
		"table_test.go":  "package a\nvar cases = []c{{wantErr: ErrTabled}}\n",
		"notes.txt":      "not Go at all",
		"broken_test.go": "package a\nthis is not Go\n",
	}
	dir := func(dirPath) ([]string, error) {
		names := make([]string, 0, len(files))
		for name := range files {
			names = append(names, name)
		}
		return names, nil
	}
	read := func(path string) ([]byte, error) {
		src, ok := files[filepath.Base(path)]
		if !ok {
			return nil, errRead
		}
		return []byte(src), nil
	}

	got := assertedSentinels(dir, read, "/pkg")

	assert.True(t, got["ErrInternal"], "asserted from the internal test package")
	assert.True(t, got["ErrExternal"], "asserted from the EXTERNAL test package")
	assert.True(t, got["ErrTabled"], "asserted through a table case's wantErr")
	assert.False(t, got["ErrSource"], "a sentinel merely emitted is not asserted")
}

// TestAssertedSentinelsFailsOpen pins that a filesystem failure contributes
// nothing rather than turning into findings. Reporting a sentinel as unasserted
// because a file could not be opened would be a finding about the filesystem.
func TestAssertedSentinelsFailsOpen(t *testing.T) {
	t.Parallel()
	want := assert.New(t)

	unreadableDir := func(dirPath) ([]string, error) { return nil, errRead }
	want.Empty(assertedSentinels(unreadableDir, os.ReadFile, "/pkg"))

	oneTest := func(dirPath) ([]string, error) { return []string{"a_test.go"}, nil }
	unreadableFile := func(string) ([]byte, error) { return nil, errRead }
	want.Empty(assertedSentinels(oneTest, unreadableFile, "/pkg"))

	unparseable := func(string) ([]byte, error) { return []byte("package a\n???"), nil }
	want.Empty(assertedSentinels(oneTest, unparseable, "/pkg"))
}

// TestOsReadDirNamesListsEntriesOrFails pins the real reader both ways.
func TestOsReadDirNamesListsEntriesOrFails(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a_test.go"), []byte("package a"), 0o600))

	names, err := osReadDirNames(dirPath(dir))
	require.NoError(t, err)
	assert.Equal(t, []string{"a_test.go"}, names)

	_, err = osReadDirNames(dirPath(filepath.Join(dir, "absent")))
	assert.Error(t, err, "an unreadable directory is an error, not an empty listing")
}

// TestFromFieldIgnoresANonIdentifierKey pins that a case field keyed by
// something other than a plain name marks nothing.
func TestFromFieldIgnoresANonIdentifierKey(t *testing.T) {
	t.Parallel()

	marked := assertions{}
	fromField(&ast.KeyValueExpr{
		Key:   &ast.BasicLit{Kind: token.STRING, Value: `"wantErr"`},
		Value: &ast.Ident{Name: "ErrThing"},
	}, marked)

	assert.Empty(t, marked)
}

// TestErrorMatcherNames pins the accepted matcher set, including the formatted
// variants, and that an unrelated call is not mistaken for an assertion.
func TestErrorMatcherNames(t *testing.T) {
	t.Parallel()
	want := assert.New(t)

	for _, name := range []memberName{"Is", "As", "ErrorIs", "ErrorAs", "NotErrorIs", "ErrorIsf"} {
		want.True(isErrorMatcher(name), name)
	}
	for _, name := range []memberName{"Equal", "NoError", "Error", ""} {
		want.False(isErrorMatcher(name), name)
	}
}

// TestExpectationFieldNames pins which case-field names carry an expectation.
func TestExpectationFieldNames(t *testing.T) {
	t.Parallel()
	want := assert.New(t)

	for _, name := range []memberName{"wantErr", "want", "Want", "expectErr", "EXPECTED"} {
		want.True(isExpectation(name), name)
	}
	for _, name := range []memberName{"name", "input", "err", ""} {
		want.False(isExpectation(name), name)
	}
}

// TestAssertionsMarkIgnoresTheEmptyName pins that an expression naming nothing
// marks nothing.
func TestAssertionsMarkIgnoresTheEmptyName(t *testing.T) {
	t.Parallel()

	marked := assertions{}
	marked.mark("")

	assert.Empty(t, marked)
}

// parseSource parses one source string into a file for the positional-case
// helpers to read.
func parseSource(t *testing.T, src string) *ast.File {
	t.Helper()
	parsed, err := parser.ParseFile(token.NewFileSet(), "case_test.go", src, 0)
	require.NoError(t, err)
	return parsed
}

// TestExpectationPositionsIndexesOnlyStructsCarryingOne pins which case types
// can be read positionally: one whose declaration carries a want/expect field
// is indexed at that field's position — counting each name of a grouped field
// and each embedded field separately — and a struct without one is absent, so
// nothing about it is ever read by position.
func TestExpectationPositionsIndexesOnlyStructsCarryingOne(t *testing.T) {
	t.Parallel()
	want := assert.New(t)

	positions := expectationPositions(parseSource(t, `package p

type tabled struct {
	name    string
	wantErr error
}

type grouped struct {
	name, detail string
	expectErr    error
}

type embedded struct {
	base
	wantErr error
}

type plain struct {
	name string
	err  error
}

type notAStruct = string

const alsoNotAType = 1
`))

	want.Equal(fieldIndex(1), positions["tabled"])
	want.Equal(fieldIndex(2), positions["grouped"], "each name of a grouped field fills its own position")
	want.Equal(fieldIndex(1), positions["embedded"], "an embedded field fills one position")
	want.NotContains(positions, caseType("plain"), "a struct with no expectation field is never read positionally")
}

// TestPositionalNamesFlattensInFillOrder pins the flattening an unkeyed
// literal follows: one entry per name, and one unnamed entry per embedded
// field.
func TestPositionalNamesFlattensInFillOrder(t *testing.T) {
	t.Parallel()

	structure := parseSource(t, `package p

type c struct {
	base
	name, detail string
	wantErr      error
}
`).Decls[0].(*ast.GenDecl).Specs[0].(*ast.TypeSpec).Type.(*ast.StructType)

	assert.Equal(t, []string{"", "name", "detail", "wantErr"}, positionalNames(structure.Fields.List))
}

// TestElementTypeNamesSliceArrayAndMapValues pins where a case literal's type
// comes from when the literal itself is elided: the element type of a slice or
// array, or a map's value type. Anything else — an unnamed element type, a
// literal with no type at all — yields nothing rather than a guess.
func TestElementTypeNamesSliceArrayAndMapValues(t *testing.T) {
	t.Parallel()
	want := assert.New(t)

	named := func(src string) (caseType, bool) {
		expr := parseSource(t, "package p\n\nvar v = "+src+"\n").
			Decls[0].(*ast.GenDecl).Specs[0].(*ast.ValueSpec).Values[0].(*ast.CompositeLit)
		return elementType(expr.Type)
	}

	got, ok := named("[]tabled{}")
	want.True(ok)
	want.Equal(caseType("tabled"), got)

	got, ok = named("[2]tabled{}")
	want.True(ok)
	want.Equal(caseType("tabled"), got)

	got, ok = named("map[string]tabled{}")
	want.True(ok)
	want.Equal(caseType("tabled"), got)

	_, ok = named("[]*tabled{}")
	want.False(ok, "an unnamed element type is not read positionally")

	_, ok = named("struct{}{}")
	want.False(ok)
}

// TestFromUnkeyedCasesReadsOnlyTheExpectationPosition pins the positional
// read: the sentinel at the expectation position is recorded, a keyed element
// there is left to the keyed path, a case shorter than that position is
// ignored, and a literal of an unindexed type records nothing.
func TestFromUnkeyedCasesReadsOnlyTheExpectationPosition(t *testing.T) {
	t.Parallel()
	want := assert.New(t)

	file := parseSource(t, `package p

type tabled struct {
	name    string
	wantErr error
}

type plain struct {
	name string
	err  error
}

var (
	slice  = []tabled{{"a", ErrSlice}}
	decoy  = []tabled{{ErrDecoy, ErrRealOne}}
	single = tabled{"b", ErrSingle}
	keyed  = []tabled{{name: "c", wantErr: ErrKeyed}}
	short  = []tabled{{"d"}}
	other  = []plain{{"e", ErrPlain}}
)
`)
	positions := expectationPositions(file)
	got := assertions{}
	ast.Inspect(file, func(n ast.Node) bool {
		if lit, ok := n.(*ast.CompositeLit); ok {
			fromUnkeyedCases(lit, positions, got)
		}
		return true
	})

	want.True(got["ErrSlice"], "an element of a slice of cases is read")
	want.True(got["ErrRealOne"], "the expectation position is read wherever the case sits")
	want.False(
		got["ErrDecoy"],
		"ONLY the expectation position is read — a sentinel at another position is not asserted by being mentioned there",
	)
	want.True(got["ErrSingle"], "a bare case literal is read")
	want.False(got["ErrKeyed"], "a keyed element belongs to the keyed path")
	want.False(got["ErrPlain"], "a case type carrying no expectation field is never read positionally")
}
