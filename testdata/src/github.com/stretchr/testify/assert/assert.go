// Package assert is a testdata stub of testify's assert package, exposing just
// the call shapes the analyzer discriminates between.
package assert

// TestingT mirrors testify's minimal testing hook.
type TestingT interface {
	Errorf(format string, args ...any)
}

// EqualError mirrors testify's message-equality assertion (banned).
func EqualError(t TestingT, err error, msg string, args ...any) bool { return true }

// ErrorContains mirrors testify's message-substring assertion (banned).
func ErrorContains(t TestingT, err error, contains string, args ...any) bool { return true }

// ErrorIs mirrors testify's errors.Is assertion (sanctioned).
func ErrorIs(t TestingT, err, target error, args ...any) bool { return true }

// Assertions mirrors testify's assert.New receiver style.
type Assertions struct{ t TestingT }

// New mirrors testify's constructor.
func New(t TestingT) *Assertions { return &Assertions{t: t} }

// EqualError mirrors the method form of the banned assertion.
func (a *Assertions) EqualError(err error, msg string, args ...any) bool { return true }

// ErrorContains mirrors the method form of the banned assertion.
func (a *Assertions) ErrorContains(err error, contains string, args ...any) bool { return true }

// ErrorIs mirrors the method form of the sanctioned assertion.
func (a *Assertions) ErrorIs(err, target error, args ...any) bool { return true }
