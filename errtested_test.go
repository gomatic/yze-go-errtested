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

// TestRegistrationIsWellFormed pins the yze wiring: the rule id the gate reports
// under, and that the registration carries this package's analyzer.
func TestRegistrationIsWellFormed(t *testing.T) {
	t.Parallel()

	assert.NoError(t, errtested.Registration.Validate())
	assert.Equal(t, "yze/errtested", errtested.Registration.RuleID())
	assert.Same(t, errtested.Analyzer, errtested.Registration.Analyzer)
}
