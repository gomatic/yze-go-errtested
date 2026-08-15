//line notatest.go:1
// This is the other direction, and a corpus that only proves one of them cannot
// tell a fixed matcher from one that stopped exempting anything at all.
//
// This file IS a test file — the go tool compiles it into the test binary and
// nowhere else — and it claims through a `//line` directive to be production
// source. Its emissions must still be exempt: a reference from a test file is
// not an emission, so ErrOnlyFromTest is never reported, no matter what the
// adjusted position calls this file.
package linename

import errs "github.com/gomatic/go-error"

// ErrOnlyFromTest is declared and referenced only from this test file, and no
// test asserts it. If the emission matcher believed the adjusted name, this
// reference would count as an emission and be reported.
const ErrOnlyFromTest errs.Const = "referenced only from a test file"

// fromTest references the sentinel from inside a function body in a test file.
// Bodily reference plus test file equals no emission.
func fromTest() error { return ErrOnlyFromTest }

var _ = fromTest
