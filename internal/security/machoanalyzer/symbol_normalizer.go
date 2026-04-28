package machoanalyzer

import "strings"

// NormalizeSymbolName strips the leading underscore and version suffix
// from a macOS imported symbol name.
//
// Examples:
//
//	NormalizeSymbolName("_socket")          → "socket"
//	NormalizeSymbolName("_socket$UNIX2003") → "socket"
//	NormalizeSymbolName("socket")           → "socket"
func NormalizeSymbolName(name string) string {
	// Strip leading underscore (macOS C symbol convention)
	name = strings.TrimPrefix(name, "_")
	// Strip version suffix (e.g., "$UNIX2003", "$INODE64")
	if idx := strings.IndexByte(name, '$'); idx >= 0 {
		name = name[:idx]
	}
	return name
}
