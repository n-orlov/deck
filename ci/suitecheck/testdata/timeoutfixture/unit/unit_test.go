// Package unit is the timeout fixture's ordinary Go package: one passing
// test, so ci/suite.sh's step-1 pass produces a real, all-passing
// junit-go.xml alongside the aborted features/ pass.
package unit

import "testing"

func TestFixtureUnitPasses(t *testing.T) {}
