//go:build test

package security

import "os"

// getwdHook is the seam used to reach the branch of ResolvePathForCheck that
// handles a failure to make a relative path absolute. That branch is otherwise
// unreachable, since os.Getwd only fails in situations a test cannot create (for
// example the working directory having been removed).
//
// It lives here, not next to the production definition, so that the production
// build keeps no swappable function value; see getwd.go.
var getwdHook = os.Getwd

// getwd returns the process working directory, via the test seam above.
func getwd() (string, error) {
	return getwdHook()
}
