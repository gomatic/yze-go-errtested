package errtested

import (
	"go/ast"
	"go/token"
	"go/types"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// sentinel builds a distinct error-typed constant for the set tests.
func sentinel(name string) *types.Const {
	named := types.NewNamed(types.NewTypeName(0, nil, name+"Type", nil), types.Typ[types.String], nil)
	named.AddMethod(types.NewFunc(0, nil, "Error", types.NewSignatureType(
		types.NewVar(0, nil, "e", named), nil, nil, nil,
		types.NewTuple(types.NewVar(0, nil, "", types.Typ[types.String])), false,
	)))
	return types.NewConst(0, nil, name, named, nil)
}

// TestIdentOfReducesEveryReferenceForm pins the shapes a sentinel reference can
// take: a bare name, a qualified selector, a parenthesized reference, and a
// node that names nothing.
func TestIdentOfReducesEveryReferenceForm(t *testing.T) {
	t.Parallel()
	want := assert.New(t)

	bare := &ast.Ident{Name: "ErrThing"}
	want.Same(bare, identOf(bare))
	want.Same(bare, identOf(&ast.SelectorExpr{X: &ast.Ident{Name: "pkg"}, Sel: bare}))
	want.Same(bare, identOf(&ast.ParenExpr{X: bare}))
	want.Same(bare, identOf(&ast.ParenExpr{X: &ast.ParenExpr{X: bare}}))
	want.Nil(identOf(&ast.BasicLit{Kind: token.STRING, Value: `"x"`}))
}

// TestImplementsErrorRejectsNonErrors pins the type classifier: a missing type
// and a plain string are not sentinels; an error-implementing named type is.
func TestImplementsErrorRejectsNonErrors(t *testing.T) {
	t.Parallel()
	want := assert.New(t)

	want.False(implementsError(nil))
	want.False(implementsError(types.Typ[types.String]))
	want.True(implementsError(sentinel("ErrX").Type()))
}

// TestSitesAddKeepsTheEarliestPosition pins that a sentinel emitted more than
// once is reported at its FIRST emission, and that a non-sentinel is ignored.
func TestSitesAddKeepsTheEarliestPosition(t *testing.T) {
	t.Parallel()
	want := assert.New(t)

	first, later := token.Pos(10), token.Pos(20)
	one := sentinel("ErrOne")
	recorded := sites{}

	recorded.add(nil, first)
	want.Empty(recorded, "a non-sentinel expression records nothing")

	recorded.add(one, later)
	recorded.add(one, first)
	want.Equal(first, recorded[one], "the earliest emission wins")

	recorded.add(one, later)
	want.Equal(first, recorded[one], "a later emission never displaces the first")
}

// TestSortedUnassertedOrdersByEmission pins that unasserted sentinels come back
// in emission order and that asserted ones are excluded.
func TestSortedUnassertedOrdersByEmission(t *testing.T) {
	t.Parallel()

	early, middle, late := sentinel("ErrEarly"), sentinel("ErrMiddle"), sentinel("ErrLate")
	emitted := sites{late: token.Pos(30), early: token.Pos(10), middle: token.Pos(20)}
	asserted := sites{middle: token.Pos(99)}

	got := emitted.sortedUnasserted(asserted)

	require.Len(t, got, 2)
	assert.Equal(t, []*types.Const{early, late}, got)
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

// TestCalleeOfNamesTheInvokedFunction pins callee extraction for the plain,
// qualified, and non-identifier call shapes.
func TestCalleeOfNamesTheInvokedFunction(t *testing.T) {
	t.Parallel()
	want := assert.New(t)

	want.Equal(memberName("Is"), calleeOf(&ast.CallExpr{Fun: &ast.Ident{Name: "Is"}}))
	qualified := &ast.SelectorExpr{X: &ast.Ident{Name: "errors"}, Sel: &ast.Ident{Name: "Is"}}
	want.Equal(memberName("Is"), calleeOf(&ast.CallExpr{Fun: qualified}))
	want.Equal(memberName(""), calleeOf(&ast.CallExpr{Fun: &ast.FuncLit{}}))
}

// TestIsTestRecognisesTestFiles pins the file classifier both ways.
func TestIsTestRecognisesTestFiles(t *testing.T) {
	t.Parallel()
	want := assert.New(t)

	want.True(isTest("/src/a/a_test.go"))
	want.False(isTest("/src/a/a.go"))
	want.False(isTest("/src/a/testdata.go"))
}

// TestInFuncBodyDistinguishesDeclarationFromEmission pins the discriminator
// that separates a re-export from an emission.
func TestInFuncBodyDistinguishesDeclarationFromEmission(t *testing.T) {
	t.Parallel()
	want := assert.New(t)

	want.False(inFuncBody([]ast.Node{&ast.File{}, &ast.GenDecl{}}))
	want.True(inFuncBody([]ast.Node{&ast.File{}, &ast.FuncDecl{}, &ast.ReturnStmt{}}))
	want.True(inFuncBody([]ast.Node{&ast.File{}, &ast.GenDecl{}, &ast.FuncLit{}}))
}
