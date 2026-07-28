package a // want "sentinel ErrUntested is emitted by this package but no test asserts it with errors.Is"

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
