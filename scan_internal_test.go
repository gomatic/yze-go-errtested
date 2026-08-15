package errtested

import (
	"fmt"
	"go/ast"
	"go/build"
	"go/token"
	"io"
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
		"table_test.go":  "package a\nvar cases = []c{{wantErr: ErrTabled}}\nfunc TestTabled(t *testing.T) { for _, tc := range cases { errors.Is(nil, tc.wantErr) } }\n",
		"notes.txt":      "not Go at all",
		"broken_test.go": "package a\nthis is not Go\n",
	}
	got := assertedSentinels(namesOf(files), sourceOf(files), "/pkg")

	assert.True(t, got["ErrInternal"], "asserted from the internal test package")
	assert.True(t, got["ErrExternal"], "asserted from the EXTERNAL test package")
	assert.True(t, got["ErrTabled"], "asserted through a table case's wantErr, read by the test that names the table")
	assert.False(t, got["ErrSource"], "a sentinel merely emitted is not asserted")
}

// TestAssertedSentinelsCountsOnlyWhatTheTestBuildCompiles pins the first half
// of the evidence rule: a file the go tool never compiles into the package's
// test binary proves nothing about that binary, so it credits nothing. Each
// name below is a file `go test` reports `[no test files]` for — dot-prefixed
// and underscore-prefixed names the go tool skips outright, and a build
// constraint no ordinary build satisfies.
func TestAssertedSentinelsCountsOnlyWhatTheTestBuildCompiles(t *testing.T) {
	t.Parallel()
	want := assert.New(t)

	const assertion = "func TestSilence(t *testing.T) { errors.Is(nil, %s) }\n"
	files := map[string]string{
		"a_test.go":        "package a\n" + fmt.Sprintf(assertion, "ErrCompiled"),
		".silence_test.go": "package a\n" + fmt.Sprintf(assertion, "ErrDot"),
		"_silence_test.go": "package a\n" + fmt.Sprintf(assertion, "ErrUnderscore"),
		"zz_test.go":       "//go:build never_ever_built\n\npackage a\n" + fmt.Sprintf(assertion, "ErrNeverBuilt"),
	}

	got := assertedSentinels(namesOf(files), sourceOf(files), "/pkg")

	want.True(got["ErrCompiled"], "the control: an ordinary test file the build compiles counts")
	want.False(got["ErrDot"], "a dot-prefixed file is never read by the go tool, so it credits nothing")
	want.False(got["ErrUnderscore"], "an underscore-prefixed file is never read by the go tool either")
	want.False(got["ErrNeverBuilt"], "a file behind an unsatisfied build constraint is in no test binary")
}

// TestAssertedSentinelsCountsOnlyWhatATestRunReaches pins the second half: a
// file the build compiles still proves nothing unless a test run reaches the
// code naming the sentinel. A package-level declaration bound to the blank
// identifier runs at init and asserts nothing, and nothing can ever name it to
// make it run as a test.
func TestAssertedSentinelsCountsOnlyWhatATestRunReaches(t *testing.T) {
	t.Parallel()
	want := assert.New(t)

	files := map[string]string{
		"forge_test.go": "package a\n" +
			"var _ = struct{ wantErr error }{wantErr: ErrForged}\n" +
			"var _ = errors.Is(nil, ErrVacuous)\n" +
			"var orphan = struct{ wantErr error }{wantErr: ErrOrphan}\n" +
			"func helper() { errors.Is(nil, ErrUncalled) }\n",
		"real_test.go": "package a\n" +
			"var cases = []struct{ name string; wantErr error }{{\"tabled\", ErrTabled}}\n" +
			"func check(t *testing.T) { errors.Is(nil, ErrHelped) }\n" +
			"func TestReal(t *testing.T) { check(t); _ = cases; errors.Is(nil, ErrDirect) }\n",
	}

	got := assertedSentinels(namesOf(files), sourceOf(files), "/pkg")

	want.True(got["ErrDirect"], "the control: a matcher inside a test function counts")
	want.True(got["ErrHelped"], "a helper the test calls is reached by the run")
	want.True(got["ErrTabled"], "a package-level table the test names is reached through it")
	want.False(got["ErrForged"], "a blank-identifier case literal is named by nothing and asserts nothing")
	want.False(got["ErrVacuous"], "a blank-identifier matcher runs at init, and init is not a test")
	want.False(got["ErrOrphan"], "a named declaration no reachable code names is never reached")
	want.False(got["ErrUncalled"], "a helper no test calls is compiled but never run")
}

// namesOf lists a fixture's file names as a directory read would.
func namesOf(files map[string]string) dirReader {
	return func(dirPath) ([]string, error) {
		names := make([]string, 0, len(files))
		for name := range files {
			names = append(names, name)
		}
		return names, nil
	}
}

// sourceOf reads a fixture's files by base name, so a path a scan joins still
// resolves.
func sourceOf(files map[string]string) fileReader {
	return func(path string) ([]byte, error) {
		src, ok := files[filepath.Base(path)]
		if !ok {
			return nil, errRead
		}
		return []byte(src), nil
	}
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

	unparseable := func(string) ([]byte, error) { return []byte("package a\nthis is not Go\n"), nil }
	want.Empty(assertedSentinels(oneTest, unparseable, "/pkg"))
}

// TestInTestBuildTakesTheGoToolsAnswer pins that membership of the test build is
// decided by go/build rather than by the filename suffix. The suffix is
// necessary and collectEntry checks it; everything else — the names the tool
// skips, the extensions it recognises, the constraints it evaluates — is the go
// tool's answer, taken rather than restated.
func TestInTestBuildTakesTheGoToolsAnswer(t *testing.T) {
	t.Parallel()
	want := assert.New(t)

	files := map[string]string{
		"a_test.go":        "package a\n",
		"notes.txt":        "not Go at all",
		".silence_test.go": "package a\n",
		"_silence_test.go": "package a\n",
		"zz_test.go":       "//go:build never_ever_built\n\npackage a\n",
	}
	in := func(name entryName) bool { return inTestBuild([]byte(files[string(name)]), "/pkg", name) }

	want.True(in("a_test.go"))
	want.False(in("notes.txt"), "a file that is not Go is not in any build")
	want.False(in(".silence_test.go"), "the go tool skips a dot-prefixed name")
	want.False(in("_silence_test.go"), "the go tool skips an underscore-prefixed name")
	want.False(in("zz_test.go"), "an unsatisfied build constraint keeps the file out of the binary")
}

// TestInTestBuildEvaluatesConstraintsForThisPlatform pins the platform
// consequence of taking the go tool's answer, in both directions, so the
// verdict's dependence on GOOS is asserted rather than discovered: a test file
// constrained to the platform the analyzer runs on is in the build, and one
// constrained to any other platform is not.
func TestInTestBuildEvaluatesConstraintsForThisPlatform(t *testing.T) {
	t.Parallel()
	want := assert.New(t)

	other := "linux"
	if build.Default.GOOS == other {
		other = "darwin"
	}
	files := map[string]string{
		"here_test.go":  "//go:build " + build.Default.GOOS + "\n\npackage a\n",
		"there_test.go": "//go:build " + other + "\n\npackage a\n",
	}
	in := func(name entryName) bool { return inTestBuild([]byte(files[string(name)]), "/pkg", name) }

	want.True(in("here_test.go"), "this platform's tests are in this platform's build")
	want.False(in("there_test.go"), "another platform's tests are in no build here")
}

// TestBuildContextServesExactlyTheSourceItWasGiven pins the seam that makes the
// selection and the parse agree: the context answers every path with the bytes
// already read for the entry, so go/build judges exactly what the parser reads
// and neither can be handed a different file.
func TestBuildContextServesExactlyTheSourceItWasGiven(t *testing.T) {
	t.Parallel()

	context := buildContext([]byte("package a\n"))

	opened, err := context.OpenFile("/anywhere/else.go")
	require.NoError(t, err)
	src, err := io.ReadAll(opened)
	require.NoError(t, err)
	require.NoError(t, opened.Close())

	assert.Equal(t, "package a\n", string(src))
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
