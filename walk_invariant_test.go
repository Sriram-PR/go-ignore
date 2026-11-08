package ignore_test

import "testing"

// TestWalkAgreesWithMatch runs checkWalkAgreesWithMatch on the parity
// scenarios. Needs no git.
//
// Case-insensitive scenarios are left out: there, sibling directories that
// differ only in case get each other's rules in load order, a state that
// cannot exist on a case-insensitive filesystem. On Windows, paths with a
// backslash in a name are left out too: there it is a path separator.
func TestWalkAgreesWithMatch(t *testing.T) {
	for _, s := range walkInvariantScenarios() {
		checkWalkAgreesWithMatch(t, s)
	}
}
