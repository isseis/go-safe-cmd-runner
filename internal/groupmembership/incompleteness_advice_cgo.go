//go:build cgo

package groupmembership

// adviseIncompleteness returns the fact and the remediation this build can
// offer for the given cause. This build already resolves group members
// through libc, so building with cgo is not a remediation it can offer: what
// is left is to stop relying on the group for the path, or to configure the
// user database with sources whose enumeration is exhaustive.
func adviseIncompleteness(cause incompletenessCause) incompletenessAdvice {
	switch cause {
	case causeUnsupportedPlatform:
		return incompletenessAdvice{
			fact:        "this platform offers no way to determine how its user database is configured, so a group's member list cannot be confirmed to cover every member",
			remediation: "clear the group-writable bit on the path (chmod g-w)",
		}
	case causeNSSSources:
		// The remediation says "only" because every source named on the line
		// has to be one whose enumeration is exhaustive: adding files beside
		// sss leaves the line as unusable as it was.
		return incompletenessAdvice{
			fact:        "/etc/nsswitch.conf does not establish that every member of a group is enumerated: a source it names gives no guarantee of exhaustive enumeration (SSSD returns no directory users under enumerate = False, and no explicit members under ignore_group_members = True), a line it needs is missing or could not be read as written, or the file could not be read; the detail says which",
			remediation: "clear the group-writable bit on the path (chmod g-w), or configure the passwd and group lines with only sources whose enumeration is exhaustive (files, systemd)",
		}
	case causeMalformedLine:
		// Only a build that scans the user database files itself can attach
		// this cause. Keeping the branch keeps the switch exhaustive, and
		// reaching it means the implementation is wrong, not the host.
		return implementationDefectAdvice("a cause only a build that scans the user database files directly can produce was reported")
	case causeUnspecified:
		return implementationDefectAdvice("the enumeration was judged incomplete but recorded no cause")
	default:
		return implementationDefectAdvice("the enumeration was judged incomplete for a cause this build does not recognize")
	}
}
