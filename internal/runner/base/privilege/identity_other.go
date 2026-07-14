//go:build !linux && !windows

package privilege

// readSavedIDs returns ErrSavedSetNotSupported on non-Linux platforms.
// The saved-set-user-id and saved-set-group-id concepts are Linux-specific
// (getresuid/getresgid are not available on darwin). On these platforms the
// caller must skip the saved-set verification. The explicit error return
// ensures that the skip is structural (gated on the error type, not on
// implicit equality of constant zero values).
func readSavedIDs() (suid, sgid int, err error) {
	return 0, 0, ErrSavedSetNotSupported
}
