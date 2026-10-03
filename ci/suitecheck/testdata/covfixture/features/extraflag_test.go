//go:build suiteextraflag

// Compiled only under -tags=suiteextraflag (see unit/extraflag_test.go):
// sets the marker TestFeatures writes, proving the features/TestFeatures
// pass's `go test` also received ci/suite.sh's DECK_CI_GO_EXTRA_FLAGS.
package features

func init() { extraFlagMarker = "tagged" }
