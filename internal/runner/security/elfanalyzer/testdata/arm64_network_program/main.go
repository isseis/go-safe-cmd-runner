// Compile with:
//
//	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build //	  -o testdata/arm64_network_program/binary //	  ./testdata/arm64_network_program/
package main

import (
	"net"
	"os"
)

// main runs network operations to ensure the binary contains network-related
// syscalls (socket, connect, etc.) that can be detected by the ELF analyzer.
func main() {
	// Attempt a TCP connection to embed network syscalls in the binary.
	// The connection is expected to fail; we only need the syscalls to exist.
	conn, err := net.Dial("tcp", "127.0.0.1:0")
	if err != nil {
		os.Exit(0)
	}
	conn.Close()
}
