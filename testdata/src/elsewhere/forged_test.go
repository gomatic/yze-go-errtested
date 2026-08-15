// Package elsewhere is not a package under analysis and is not any package's
// test. It exists to hold the forged evidence ../linedir/redirect.go points its
// `//line` directive at: a file that looks exactly like an assertion and tests
// nothing, because `go test ./linedir` never compiles it.
//
// A rule that reads its corpus from an adjusted directory would find this file
// and credit ErrRedirected. Reading the real directory does not.
package elsewhere

import (
	"errors"
	"testing"
)

// ErrRedirected shadows the name linedir emits, which is the whole trick: the
// corpus scan matches on the identifier, so a same-named sentinel in a
// directory nothing compiles is indistinguishable from the real one.
var ErrRedirected error

// TestRedirected is a real test entry point asserting the forged sentinel,
// which makes it exactly the evidence the corpus scan accepts.
func TestRedirected(t *testing.T) {
	if errors.Is(nil, ErrRedirected) {
		t.Fatal("unreachable")
	}
}
