package a

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

// tabledCase is a named table case type; the sentinel reaches the matcher only
// through wantErr, which is the shape yze/errtest mandates and the shape this
// analyzer must recognize as an assertion.
type tabledCase struct {
	name    string
	wantErr error
}

// The forgery: a case literal carrying a sentinel in its expectation field,
// bound to the blank identifier. It is in the test binary and it even runs, at
// init — and it asserts nothing, because no test can reach it. ErrForgedCase is
// reported all the same.
var _ = tabledCase{"forged", ErrForgedCase}

// TestDirect asserts a sentinel with the stdlib matcher.
func TestDirect(t *testing.T) {
	if !errors.Is(Direct(), ErrDirect) {
		t.Fatal("Direct must report ErrDirect")
	}
}

// TestTestify asserts a sentinel with testify's matcher.
func TestTestify(t *testing.T) {
	assert.ErrorIs(t, Testify(), ErrTestify)
}

// TestTabled asserts a sentinel carried by a table case's wantErr field.
func TestTabled(t *testing.T) {
	cases := []tabledCase{{name: "tabled", wantErr: ErrTabled}}
	for _, tc := range cases {
		if !errors.Is(Tabled(), tc.wantErr) {
			t.Fatalf("%s: Tabled must report its sentinel", tc.name)
		}
	}
}

// TestClosure asserts the sentinel emitted from inside a function literal.
func TestClosure(t *testing.T) {
	assert.ErrorIs(t, Closure()(), ErrClosure)
}

// TestFallback asserts the sentinel held by the package-level var.
func TestFallback(t *testing.T) {
	assert.ErrorIs(t, Fallback(), ErrInVarDecl)
}

// TestPositional asserts a sentinel bound POSITIONALLY into an unkeyed case
// literal and matched through the case's expectation field.
func TestPositional(t *testing.T) {
	cases := []tabledCase{{"positional", ErrPositional}}
	for _, tc := range cases {
		if !errors.Is(Positional(), tc.wantErr) {
			t.Fatal(tc.name)
		}
	}
}

// TestMentioned names a sentinel in an unkeyed literal but matches nothing
// generically, so the sentinel stays unasserted.
func TestMentioned(t *testing.T) {
	noted := []noteCase{{"mentioned", ErrMentioned}}
	if len(noted) != 1 {
		t.Fatal("unreachable")
	}
}

// noteCase carries a sentinel no matcher ever receives.
type noteCase struct {
	name string
	err  error
}

// TestInline asserts a sentinel bound positionally into an INLINE anonymous
// case struct, the shape most table-driven tests are written in.
func TestInline(t *testing.T) {
	cases := []struct {
		name    string
		wantErr error
	}{
		{"inline", ErrInline},
	}
	for _, tc := range cases {
		if !errors.Is(Inline(), tc.wantErr) {
			t.Fatal(tc.name)
		}
	}
}
