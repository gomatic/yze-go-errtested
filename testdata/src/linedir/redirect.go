//line ../elsewhere/redirect.go:1
// Package linedir pins the directory half of the same rule, which is a
// different defect from the filename half and has a wider blast radius.
//
// This analyzer reads its assertion corpus from the package's DIRECTORY, and
// that directory was derived from an adjusted position. A relative path in a
// `//line` directive resolves against the directory of the file carrying it, so
// the one line above points the corpus at a sibling directory this package
// neither compiles nor tests. Whatever "tests" sit there would then credit
// every sentinel this package emits — the whole corpus aimed elsewhere by one
// comment, rather than one file exempted.
//
// ../elsewhere/forged_test.go asserts ErrRedirected with errors.Is and is no
// test of this package: `go test ./linedir` compiles none of it. The sentinel
// must be reported.
package linedir

import errs "github.com/gomatic/go-error"

// ErrRedirected is emitted here and asserted only by the sibling directory the
// directive points at.
const ErrRedirected errs.Const = "asserted only by a directory this package does not test"

// Redirected emits the sentinel.
func Redirected() error { return ErrRedirected } // want "sentinel ErrRedirected is emitted by this package"
