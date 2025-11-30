package common

import "os"

// GetHostname returns the hostname of the current machine.
// If os.Hostname() fails, it returns "unknown-host" as a fallback.
func GetHostname() string {
	hostname, err := os.Hostname()
	if err != nil {
		return "unknown-host"
	}
	return hostname
}
