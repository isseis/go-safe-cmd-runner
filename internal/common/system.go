//nolint:revive // common is an appropriate name for shared utilities package
package common

import "os"

const (
	// UnknownHostFallback is the fallback value returned when hostname cannot be determined
	UnknownHostFallback = "unknown-host"
)

// osHostname is a package-level variable that points to os.Hostname.
// This allows tests to mock the hostname function for testing error paths.
var osHostname = os.Hostname

// GetHostname returns the hostname of the current machine.
// If os.Hostname() fails, it returns UnknownHostFallback as a fallback.
func GetHostname() string {
	hostname, err := osHostname()
	if err != nil {
		return UnknownHostFallback
	}
	return hostname
}
