//go:build darwin

package security

// defaultTrustedGIDs is the default trusted group GID set for macOS.
// GID 80 is the macOS admin group. GID 0 is the root group.
var defaultTrustedGIDs = map[uint32]struct{}{
	0:  {},
	80: {},
}

// isTrustedGroup checks only the default trusted GID set on macOS.
// On macOS, Config.TrustedGIDs is intentionally ignored.
func (v *Validator) isTrustedGroup(gid uint32) bool {
	_, ok := defaultTrustedGIDs[gid]
	return ok
}
