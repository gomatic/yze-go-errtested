// Package a pins the errtested contract: a sentinel referenced from inside a
// function body in this non-test file is EMITTED, and must be asserted by a
// test in this package. A sentinel referenced only from a declaration is a
// re-export, not an emission, and is never reported.
package a

import errs "github.com/gomatic/go-error"

// The sentinels this fixture exercises, one per contract branch.
const (
	ErrDirect     errs.Const = "asserted directly with errors.Is"
	ErrTestify    errs.Const = "asserted with testify ErrorIs"
	ErrTabled     errs.Const = "asserted through a wantErr case field"
	ErrClosure    errs.Const = "emitted from inside a closure"
	ErrPositional errs.Const = "asserted through a positionally-bound case field"
	ErrMentioned  errs.Const = "named in a table that matches nothing generically"
	ErrInline     errs.Const = "asserted from an inline anonymous case table"
	ErrUntested   errs.Const = "emitted and never asserted"
	ErrDeclOnly   errs.Const = "declared and re-exported, never emitted"
	ErrInVarDecl  errs.Const = "referenced from a var declaration only"
)

// Alias re-exports a sentinel from a declaration. A declaration reference is
// not an emission, so ErrDeclOnly is not reported.
const Alias = ErrDeclOnly

// fallback holds a sentinel in a package-level var declaration — again a
// declaration, not an emission, so ErrInVarDecl is not reported.
var fallback = ErrInVarDecl

// Direct emits a sentinel the tests assert with errors.Is.
func Direct() error { return ErrDirect }

// Testify emits a sentinel the tests assert with testify's ErrorIs.
func Testify() error { return ErrTestify }

// Tabled emits a sentinel the tests assert through a table case's wantErr.
func Tabled() error { return ErrTabled }

// Positional emits a sentinel the tests bind into a case POSITIONALLY, in an
// unkeyed literal, and match through that case's expectation field — asserted
// just as surely as a keyed binding.
func Positional() error { return ErrPositional }

// Inline emits a sentinel the tests bind positionally into an INLINE
// anonymous case struct — the ordinary table-driven idiom.
func Inline() error { return ErrInline }

// Mentioned emits a sentinel a test merely names in an unkeyed literal that
// feeds no generic matcher, so nothing asserts it.
func Mentioned() error { return ErrMentioned } // want "sentinel ErrMentioned is emitted by this package"

// Closure emits a sentinel from inside a function literal, which is still an
// emission because the reference sits in a function body.
func Closure() func() error {
	return func() error { return ErrClosure }
}

// Untested emits a sentinel no test asserts.
func Untested() error { return ErrUntested } // want "sentinel ErrUntested is emitted by this package"

// Fallback keeps the package-level var referenced so the fixture compiles.
func Fallback() error { return fallback }

// Wrapped emits a sentinel through a call in a non-test file, so the analyzer's
// "skip non-test call sites" path is exercised by the fixture rather than left
// to a unit test.
func Wrapped() error { return ErrDirect.With(nil) }
