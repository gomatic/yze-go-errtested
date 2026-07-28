// Package errs is a testdata stub of github.com/gomatic/go-error, giving the
// fixtures a real sentinel-const type at the mechanism's import path.
package errs

// Const mirrors the mechanism's string-backed sentinel type.
type Const string

// Error implements the error interface.
func (e Const) Error() string { return string(e) }

// With mirrors the mechanism's wrapping signature; fixtures only need the shape.
func (e Const) With(err error, args ...any) error { return e }
