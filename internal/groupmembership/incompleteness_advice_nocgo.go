//go:build !cgo

package groupmembership

// adviseIncompleteness returns the fact and the remediation this build can
// offer for the given cause. This build scans the user database files itself,
// so every cause it can reach is also resolved by building with cgo, where
// libc consults whatever the host has configured.
//
// The cause alone selects the advice: the detail text is never inspected.
func adviseIncompleteness(cause incompletenessCause) incompletenessAdvice {
	switch cause {
	case causeUnsupportedPlatform:
		// The platforms this reaches are the ones with no /etc/nsswitch.conf,
		// so the remediation names the platform's own user database rather
		// than NSS.
		return incompletenessAdvice{
			fact:        "this build cannot enumerate all members of a group on this platform",
			remediation: "rebuild with CGO_ENABLED=1 so that group members are resolved through the platform's own user database via libc",
		}
	case causeNSSSources:
		return incompletenessAdvice{
			fact:        "/etc/nsswitch.conf names a user database source this build cannot consult, or could not be read",
			remediation: "check the passwd and group lines of /etc/nsswitch.conf, then rebuild with CGO_ENABLED=1 so that the configured sources are consulted",
		}
	case causeMalformedLine:
		return incompletenessAdvice{
			fact:        "a line of the user database files could not be parsed and was skipped, so the members listed there are unknown",
			remediation: "check the reported line: correct it if its format is wrong, or, if it is a NIS compatibility entry (a line starting with + or -), rebuild with CGO_ENABLED=1",
		}
	case causeUnspecified:
		return implementationDefectAdvice("the enumeration was judged incomplete but recorded no cause")
	default:
		return implementationDefectAdvice("the enumeration was judged incomplete for a cause this build does not recognize")
	}
}
