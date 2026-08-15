package a

// The forgery: a dot-prefixed file. The go tool never reads it, `go test`
// reports it in no package, and no test binary holds a line of it — so the
// assertion below is an assertion in nothing, and ErrHidden is reported.

import "errors"

func TestHidden(t *testing.T) {
	if !errors.Is(Hidden(), ErrHidden) {
		t.Fatal("this test does not exist")
	}
}
