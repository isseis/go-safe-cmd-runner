//go:build linux

package security

// defaultTrustedGIDs is the default trusted group GID set for Linux.
// Only GID 0 is trusted by default.
var defaultTrustedGIDs = map[uint32]struct{}{
	0: {},
}

// isTrustedGroup checks both Linux default trusted GIDs and Config.TrustedGIDs.
func (v *Validator) isTrustedGroup(gid uint32) bool {
	if _, ok := defaultTrustedGIDs[gid]; ok {
		return true
	}

	for _, trusted := range v.config.TrustedGIDs {
		if trusted == gid {
			return true
		}
	}

	return false
}
