package errtested

import (
	"go/ast"
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
