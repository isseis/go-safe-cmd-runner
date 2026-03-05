package machoanalyzer

import "strings"

// normalizeSymbolName strips the leading underscore and version suffix
// from a macOS imported symbol name.
//
// Examples:
//
//	normalizeSymbolName("_socket")          → "socket"
//	normalizeSymbolName("_socket$UNIX2003") → "socket"
//	normalizeSymbolName("socket")           → "socket"
func normalizeSymbolName(name string) string {
	// Strip leading underscore (macOS C symbol convention)
	name = strings.TrimPrefix(name, "_")
	// Strip version suffix (e.g., "$UNIX2003", "$INODE64")
	if idx := strings.IndexByte(name, '$'); idx >= 0 {
		name = name[:idx]
	}
	return name
}
