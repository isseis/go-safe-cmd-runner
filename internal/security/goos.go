package security

// GosDarwin is the GOOS value for macOS.
const GosDarwin = "darwin"

// RequireGOOS returns goos unchanged, panicking if it is empty.
func RequireGOOS(goos string) string {
	if goos == "" {
		panic("goos must not be empty")
	}
	return goos
}
