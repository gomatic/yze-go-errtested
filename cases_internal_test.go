package errtested

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

// TestElementExprNamesSliceArrayAndMapValues pins where an elided case
// literal's type comes from: the element type of a slice or array, or a map's
// value type. A literal that is not a collection yields nothing.
func TestElementExprNamesSliceArrayAndMapValues(t *testing.T) {
	t.Parallel()
	want := assert.New(t)

	element := func(src string) (ast.Expr, bool) {
		expr := parseSource(t, "package p\n\nvar v = "+src+"\n").
			Decls[0].(*ast.GenDecl).Specs[0].(*ast.ValueSpec).Values[0].(*ast.CompositeLit)
		return elementExpr(expr.Type)
	}

	got, ok := element("[]tabled{}")
	want.True(ok)
	want.Equal("tabled", got.(*ast.Ident).Name)

	got, ok = element("[2]tabled{}")
	want.True(ok)
	want.Equal("tabled", got.(*ast.Ident).Name)

	got, ok = element("map[string]tabled{}")
	want.True(ok)
	want.Equal("tabled", got.(*ast.Ident).Name)

	_, ok = element("struct{}{}")
	want.False(ok, "a non-collection literal has no element type")
}

// TestExpectationAtReadsInlineAndNamedCaseTypes pins how a case type resolves
// to its expectation position: an inline struct is measured directly (the
// table-driven idiom), a named struct is looked up among the file's
// declarations, a type carrying no expectation field resolves to nothing, and
// anything that is not a struct type resolves to nothing.
func TestExpectationAtReadsInlineAndNamedCaseTypes(t *testing.T) {
	t.Parallel()
	want := assert.New(t)

	file := parseSource(t, `package p

type tabled struct {
	name    string
	wantErr error
}

var (
	inline = []struct {
		name    string
		detail  string
		wantErr error
	}{}
	plainInline = []struct{ name string }{}
	pointers    = []*tabled{}
)
`)
	fields := expectationPositions(file)
	typeOf := func(index int) ast.Expr {
		lit := file.Decls[1].(*ast.GenDecl).Specs[index].(*ast.ValueSpec).Values[0].(*ast.CompositeLit)
		element, ok := elementExpr(lit.Type)
		require.True(t, ok)
		return element
	}

	at, found := expectationAt(typeOf(0), fields)
	want.True(found, "an inline case struct is measured directly")
	want.Equal(fieldIndex(2), at)

	at, found = expectationAt(&ast.Ident{Name: "tabled"}, fields)
	want.True(found, "a named case struct is looked up")
	want.Equal(fieldIndex(1), at)

	_, found = expectationAt(typeOf(1), fields)
	want.False(found, "an inline struct with no expectation field is not read positionally")

	_, found = expectationAt(typeOf(2), fields)
	want.False(found, "a type that is not a struct resolves to no position")

	_, found = expectationAt(&ast.StructType{}, fields)
	want.False(found, "a struct with no field list resolves to no position")
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
