package errtested

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// scanned is the corpus one directory of test sources reads as, run through the
// real selection and the real parser.
func scanned(files map[string]string) corpus {
	found := newCorpus()
	for name := range files {
		collectEntry(sourceOf(files), "/pkg", entryName(name), found)
	}
	return found
}

// TestIsTestEntryNamesWhatAGoTestRunEnters pins the entry points a run starts
// from, and that an ordinary helper is not one.
func TestIsTestEntryNamesWhatAGoTestRunEnters(t *testing.T) {
	t.Parallel()
	want := assert.New(t)

	for _, name := range []memberName{"TestThing", "TestMain", "BenchmarkThing", "FuzzThing", "ExampleThing"} {
		want.True(isTestEntry(name), name)
	}
	for _, name := range []memberName{"helper", "check", "newThing", ""} {
		want.False(isTestEntry(name), name)
	}
}

// TestBlankDeclarationsAreRecordedAndFollowedByNothing pins the identifier the
// whole rule turns on. `var _ = ...` is the cheapest forgery there is, and it
// is unreachable for a reason no implementation choice can soften: nothing can
// name the blank identifier. Mentioning it — `_ = cases`, `for _, tc := range`
// — is not naming it either, so a blank in a reachable body never drags a blank
// declaration into the walk.
func TestBlankDeclarationsAreRecordedAndFollowedByNothing(t *testing.T) {
	t.Parallel()
	want := assert.New(t)

	found := scanned(map[string]string{
		"a_test.go": "package a\n" +
			"var _ = errors.Is(nil, ErrBlank)\n" +
			"func TestMentionsBlank(t *testing.T) { _ = 1; errors.Is(nil, ErrReal) }\n",
	})

	want.NotContains(found.scopes, blank, "a blank declaration is recorded under no name")
	want.NotContains(found.scopes["TestMentionsBlank"].references, blank, "mentioning blank references nothing")

	got := found.asserted()
	want.True(got["ErrReal"])
	want.False(got["ErrBlank"], "a blank declaration is reached by nothing, however it is written")
}

// TestWalkNeverRevisitsADeclaration pins the walk's bound. Test helpers call one
// another, and two that call each other are an ordinary shape — without the
// bound the walk would not return, so this case is the guard's test and its
// termination is the assertion.
func TestWalkNeverRevisitsADeclaration(t *testing.T) {
	t.Parallel()
	want := assert.New(t)

	found := scanned(map[string]string{
		"a_test.go": "package a\n" +
			"func first() { second(); errors.Is(nil, ErrFirst) }\n" +
			"func second() { first(); errors.Is(nil, ErrSecond) }\n" +
			"func TestMutual(t *testing.T) { first() }\n",
	})

	got := found.asserted()

	want.True(got["ErrFirst"], "the helper the test calls is reached")
	want.True(got["ErrSecond"], "and the helper that one calls, once")
}

// TestScopeForSharesOneScopeAcrossFiles pins that one name is one scope. The
// internal and external test packages may each declare a helper of the same
// name, and crediting both wherever either is reached is the accepting
// direction this scan is built to err in.
func TestScopeForSharesOneScopeAcrossFiles(t *testing.T) {
	t.Parallel()
	want := assert.New(t)

	found := scanned(map[string]string{
		"in_test.go": "package a\nfunc helper() { errors.Is(nil, ErrInternal) }\n",
		"ext_test.go": "package a_test\nfunc helper() { errors.Is(nil, ErrExternal) }\n" +
			"func TestExternal(t *testing.T) { helper() }\n",
	})

	got := found.asserted()

	want.True(got["ErrExternal"])
	want.True(got["ErrInternal"], "one name is one scope, so either declaration is credited")
}

// TestCollectDeclReadsOnlyWhatCanCarryAnAssertion pins the declaration forms the
// scan files: an import names nothing a run enters, a type declaration holds no
// value, a function with no body is somebody else's code, and the names of a
// multi-valued declaration past the last initializer carry nothing.
func TestCollectDeclReadsOnlyWhatCanCarryAnAssertion(t *testing.T) {
	t.Parallel()
	want := assert.New(t)

	found := scanned(map[string]string{
		"a_test.go": "package a\n" +
			"import \"testing\"\n" +
			"type c struct{ wantErr error }\n" +
			"func stub()\n" +
			"var one, two = c{wantErr: ErrOne}\n" +
			"func TestAll(t *testing.T) { stub(); _ = one; _ = two }\n",
	})

	want.NotContains(found.scopes, reachName("testing"), "an import declares no reachable body")
	want.NotContains(found.scopes, reachName("c"), "a type declaration holds no value to read")
	want.NotContains(found.scopes, reachName("stub"), "a declaration with no body holds nothing")
	want.NotContains(found.scopes, reachName("two"), "a name past the last initializer carries nothing")
	want.True(found.asserted()["ErrOne"], "the name that does carry the initializer is read")
}

// TestAssertedFollowsNothingWithoutAnEntryPoint pins that a corpus of test files
// carrying no entry point credits nothing: there is no run to reach anything
// from, which is the whole content of `[no tests to run]`.
func TestAssertedFollowsNothingWithoutAnEntryPoint(t *testing.T) {
	t.Parallel()

	found := scanned(map[string]string{
		"a_test.go": "package a\nfunc helper() { errors.Is(nil, ErrHelped) }\n",
	})

	assert.Empty(t, found.asserted())
}
