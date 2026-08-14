//go:build !test

package security

import "os"

// getwd returns the process working directory.
//
// This file and test_helpers_getwd.go define getwd exclusively by build tag. The
// production build deliberately has no function value to swap: path resolution is
// a precondition of the directory permission check, so nothing outside a test
// build may redirect it.
func getwd() (string, error) {
	return os.Getwd()
}
