//go:build !darwin

package security

// isTrustedGroup checks default trusted GIDs and configured trusted GIDs
// for non-macOS platforms.
func (v *Validator) isTrustedGroup(gid uint32) bool {
	if gid == 0 {
		return true
	}

	_, ok := v.trustedGIDs[gid]
	return ok
}
