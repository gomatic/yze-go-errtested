package errtested

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"testing"

	"github.com/stretchr/testify/assert"
	"golang.org/x/tools/go/analysis"
)

// TestFileOfNeverReadsTheAdjustedPath names fileOf's claim: it yields the path
// the go tool selected the file under, never the path a `//line` directive in
// that file rewrites positions to. The second assertion is what makes the first
// mean anything — it proves the directive was in force, so a fileOf reading
// fset.Position would have answered zz_test.go and dropped the emission.
func TestFileOfNeverReadsTheAdjustedPath(t *testing.T) {
	t.Parallel()
	want := assert.New(t)

	const src = "//line zz_test.go:1\npackage p\n\nvar Sentinel = 1\n"

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "real.go", src, parser.ParseComments)
	want.NoError(err)

	want.Equal(fileName("real.go"), fileOf(&analysis.Pass{Fset: fset}, file.Decls[0]))
	want.Equal("zz_test.go", fset.Position(file.Decls[0].Pos()).Filename,
		"the directive is in force, so reading the adjusted path answers differently")
}

// TestEmissionsKeepTheEarliestSite pins that a sentinel emitted more than once
// is reported at its FIRST emission, and that a non-sentinel records nothing.
func TestEmissionsKeepTheEarliestSite(t *testing.T) {
	t.Parallel()
	want := assert.New(t)

	first, later := token.Pos(10), token.Pos(20)
	found := emissions{}

	found.add("", first)
	want.Empty(found, "an expression that is not a sentinel records nothing")

	found.add("ErrOne", later)
	found.add("ErrOne", first)
	want.Equal(first, found["ErrOne"], "the earliest emission wins")

	found.add("ErrOne", later)
	want.Equal(first, found["ErrOne"], "a later emission never displaces the first")
}

// TestSortedUnassertedOrdersByEmission pins that unasserted sentinels come back
// in emission order and asserted ones are excluded.
func TestSortedUnassertedOrdersByEmission(t *testing.T) {
	t.Parallel()

	emitted := emissions{"ErrLate": token.Pos(30), "ErrEarly": token.Pos(10), "ErrMiddle": token.Pos(20)}

	got := emitted.sortedUnasserted(assertions{"ErrMiddle": true})

	assert.Equal(t, []sentinelName{"ErrEarly", "ErrLate"}, got)
}

// TestIdentOfReducesEveryReferenceForm pins the shapes a sentinel reference can
// take: a bare name, a qualified selector, a parenthesized one, and a node that
// names nothing.
func TestIdentOfReducesEveryReferenceForm(t *testing.T) {
	t.Parallel()
	want := assert.New(t)

	bare := &ast.Ident{Name: "ErrThing"}
	want.Same(bare, identOf(bare))
	want.Same(bare, identOf(&ast.SelectorExpr{X: &ast.Ident{Name: "pkg"}, Sel: bare}))
	want.Same(bare, identOf(&ast.ParenExpr{X: bare}))
	want.Nil(identOf(&ast.BasicLit{Kind: token.STRING, Value: `"x"`}))

	want.Equal(sentinelName("ErrThing"), nameOf(bare))
	want.Empty(nameOf(&ast.BasicLit{Kind: token.STRING, Value: `"x"`}))
}

// TestImplementsErrorRejectsNonErrors pins the type classifier.
func TestImplementsErrorRejectsNonErrors(t *testing.T) {
	t.Parallel()
	want := assert.New(t)

	want.False(implementsError(nil))
	want.False(implementsError(types.Typ[types.String]))

	named := types.NewNamed(types.NewTypeName(0, nil, "Const", nil), types.Typ[types.String], nil)
	sig := types.NewSignatureType(types.NewVar(0, nil, "e", named), nil, nil, nil,
		types.NewTuple(types.NewVar(0, nil, "", types.Typ[types.String])), false)
	named.AddMethod(types.NewFunc(0, nil, "Error", sig))
	want.True(implementsError(named))
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
// separating a re-export from an emission.
func TestInFuncBodyDistinguishesDeclarationFromEmission(t *testing.T) {
	t.Parallel()
	want := assert.New(t)

	want.False(inFuncBody([]ast.Node{&ast.File{}, &ast.GenDecl{}}))
	want.True(inFuncBody([]ast.Node{&ast.File{}, &ast.FuncDecl{}, &ast.ReturnStmt{}}))
	want.True(inFuncBody([]ast.Node{&ast.File{}, &ast.GenDecl{}, &ast.FuncLit{}}))
}

// TestPackageDirIsTheDirectoryOfTheFirstFile pins the directory lookup, and that
// a pass carrying no files yields no directory rather than a bad one.
func TestPackageDirIsTheDirectoryOfTheFirstFile(t *testing.T) {
	t.Parallel()

	fset := token.NewFileSet()
	added := fset.AddFile("/src/pkg/a.go", -1, 20)
	file := &ast.File{Package: added.Pos(0)}

	assert.Equal(t, dirPath("/src/pkg"), packageDir(fset, []*ast.File{file}))
	assert.Empty(t, packageDir(fset, nil), "no files means no directory")
}

// TestSentinelOfIgnoresWhatIsNotASentinel pins the classifier's guards: a node
// naming no identifier, and an identifier that resolves to no constant.
func TestSentinelOfIgnoresWhatIsNotASentinel(t *testing.T) {
	t.Parallel()
	want := assert.New(t)

	info := &types.Info{Uses: map[*ast.Ident]types.Object{}}
	want.Empty(sentinelOf(info, &ast.BasicLit{Kind: token.STRING, Value: `"x"`}), "a literal names nothing")

	unknown := &ast.Ident{Name: "ErrThing"}
	want.Empty(sentinelOf(info, unknown), "an unresolved identifier is no sentinel")

	plain := &ast.Ident{Name: "Count"}
	info.Uses[plain] = types.NewConst(0, nil, "Count", types.Typ[types.Int], nil)
	want.Empty(sentinelOf(info, plain), "a constant that is not an error is no sentinel")
}
