package common

const (
	// SecurePathEnv is the fixed PATH used for command resolution.
	// This hardcoded value prevents PATH manipulation attacks by eliminating
	// environment variable PATH inheritance.
	// Note: /sbin is included for compatibility with system commands that may
	// only exist there on some distributions.
	SecurePathEnv = "/sbin:/usr/sbin:/bin:/usr/bin"
)
