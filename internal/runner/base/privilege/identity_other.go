//go:build !linux && !windows

package privilege

// readSavedIDs returns (0, 0, nil) on non-Linux platforms.
// The saved-set-user-id and saved-set-group-id concepts are Linux-specific
// (getresuid/getresgid are not available on darwin). On these platforms the
// caller must skip the saved-set verification, which is consistent with the
// return value of (0, 0) making any equality check against a captured (0, 0)
// pass.
func readSavedIDs() (suid, sgid int, err error) {
	return 0, 0, nil
}
