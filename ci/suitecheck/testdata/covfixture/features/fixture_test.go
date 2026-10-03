// Package features is the covfixture's stand-in for the real Godog entry
// point (task 002, R187): a no-op TestFeatures, fast enough that
// ci/suite.sh's second pass contributes nothing to this fixture's
// coverage data either -- which is the point, see unit/unit_test.go's own
// header.
package features

import "testing"

func TestFeatures(t *testing.T) {}
