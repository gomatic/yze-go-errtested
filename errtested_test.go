package errtested_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"golang.org/x/tools/go/analysis/analysistest"

	errtested "github.com/gomatic/yze-go-errtested"
)

// TestEmittedSentinelsMustBeAsserted pins the whole contract against the
// fixture: an emitted-and-unasserted sentinel is reported, while sentinels
// asserted directly, via testify, via a table's wantErr field, or emitted only
// from a declaration are not.
func TestEmittedSentinelsMustBeAsserted(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), errtested.Analyzer, "a")
}

// TestLineDirectiveDoesNotMoveTheFile pins that neither answer this analyzer
// takes from a file's path — is it a test file, which directory holds the
// assertion corpus — can be changed by the file being judged.
//
// A `//line` directive is ordinary compiled source that rewrites what
// fset.Position reports, so a matcher reading the adjusted path is switched off
// by one comment line, leaving no entry in any configuration file and no marker
// to grep for. Both fixtures carry a directive and both verdicts are the ones
// the real names produce: linename holds a source file claiming to be a test
// (still judged) beside a test file claiming to be source (still exempt), and
// linedir aims its corpus at a sibling directory holding forged assertions
// (still judged).
func TestLineDirectiveDoesNotMoveTheFile(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), errtested.Analyzer, "linename", "linedir")
}

// TestRegistrationIsWellFormed pins the yze wiring: the rule id the gate reports
// under, and that the registration carries this package's analyzer.
func TestRegistrationIsWellFormed(t *testing.T) {
	t.Parallel()

	assert.NoError(t, errtested.Registration.Validate())
	assert.Equal(t, "yze/errtested", errtested.Registration.RuleID())
	assert.Same(t, errtested.Analyzer, errtested.Registration.Analyzer)
}
