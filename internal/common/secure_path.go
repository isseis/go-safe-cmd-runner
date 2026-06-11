package common

const (
	// CoreutilsDir is the directory where Ubuntu 26.04+ places the Rust coreutils
	// single binary and its hardlinks.
	CoreutilsDir = "/usr/lib/cargo/bin/coreutils"

	// SecurePathEnv is the fixed PATH used for command resolution.
	// This hardcoded value prevents PATH manipulation attacks by eliminating
	// environment variable PATH inheritance.
	// Note: /sbin is included for compatibility with system commands that may
	// only exist there on some distributions.
	SecurePathEnv = "/sbin:/usr/sbin:/bin:/usr/bin:" + CoreutilsDir
)
