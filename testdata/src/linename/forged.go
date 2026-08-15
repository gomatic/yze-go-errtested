//line zz_test.go:1
// Package linename pins that a file's identity is read from the name the file
// cannot rewrite. The `//line` directive on the first line of this file is a
// compiler feature for generated code: it makes fset.Position report every
// position below it as though it came from zz_test.go, and it changes nothing
// else — the go tool still compiles this as ordinary source (GoFiles holds it,
// TestGoFiles is empty) and an importer still links it.
//
// An analyzer that decides "is this a test file?" from the ADJUSTED position is
// therefore switched off by one comment line. Both sentinels below must be
// reported: this file is production source and its emissions are emissions.
package linename

import errs "github.com/gomatic/go-error"

// The sentinels this fixture emits, neither of them asserted anywhere.
const (
	ErrForgedName errs.Const = "emitted from a file claiming to be a test"
	ErrAlsoForged errs.Const = "emitted from the same file, through a closure"
)

// ForgedName emits a sentinel from a file whose adjusted position names a
// _test.go file it is not.
func ForgedName() error { return ErrForgedName } // want "sentinel ErrForgedName is emitted by this package"

// AlsoForged emits from inside a closure in the same rewritten file.
func AlsoForged() func() error {
	return func() error { return ErrAlsoForged } // want "sentinel ErrAlsoForged is emitted by this package"
}
