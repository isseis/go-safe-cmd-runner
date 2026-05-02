//go:build darwin

package security

const darwinAdminGID uint32 = 80

// isTrustedGroup checks only the default trusted GID set on macOS.
// On macOS, Config.TrustedGIDs is intentionally ignored.
// GID 80 is the macOS admin group. GID 0 is the root group.
func (v *Validator) isTrustedGroup(gid uint32) bool {
	switch gid {
	case 0, darwinAdminGID:
		return true
	default:
		return false
	}
}
