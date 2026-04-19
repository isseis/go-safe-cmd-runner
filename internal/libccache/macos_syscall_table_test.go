//go:build test

package libccache

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestMacOSSyscallTable_NetworkEntries verifies that all expected network syscall entries exist.
func TestMacOSSyscallTable_NetworkEntries(t *testing.T) {
	table := MacOSSyscallTable{}

	networkSyscalls := []struct {
		number int
		name   string
	}{
		{27, "recvmsg"},
		{28, "sendmsg"},
		{29, "recvfrom"},
		{30, "accept"},
		{31, "getpeername"},
		{32, "getsockname"},
		{97, "socket"},
		{98, "connect"},
		{104, "bind"},
		{105, "setsockopt"},
		{106, "listen"},
		{118, "getsockopt"},
		{133, "sendto"},
		{134, "shutdown"},
		{135, "socketpair"},
	}

	for _, tc := range networkSyscalls {
		t.Run(tc.name, func(t *testing.T) {
			assert.True(t, table.IsNetworkSyscall(tc.number),
				"expected %s (%d) to be a network syscall", tc.name, tc.number)
			assert.Equal(t, tc.name, table.GetSyscallName(tc.number))
		})
	}
}

// TestMacOSSyscallTable_SocketNumber verifies that socket=97 and connect=98.
func TestMacOSSyscallTable_SocketNumber(t *testing.T) {
	table := MacOSSyscallTable{}

	assert.Equal(t, "socket", table.GetSyscallName(97))
	assert.True(t, table.IsNetworkSyscall(97))
	assert.Equal(t, "connect", table.GetSyscallName(98))
	assert.True(t, table.IsNetworkSyscall(98))
}

// TestMacOSSyscallTable_NonNetworkEntries verifies non-network syscalls are correctly classified.
func TestMacOSSyscallTable_NonNetworkEntries(t *testing.T) {
	table := MacOSSyscallTable{}

	nonNetworkSyscalls := []struct {
		number int
		name   string
	}{
		{3, "read"},
		{4, "write"},
		{5, "open"},
		{6, "close"},
		{74, "mprotect"},
	}

	for _, tc := range nonNetworkSyscalls {
		t.Run(tc.name, func(t *testing.T) {
			assert.False(t, table.IsNetworkSyscall(tc.number),
				"expected %s (%d) to not be a network syscall", tc.name, tc.number)
			assert.Equal(t, tc.name, table.GetSyscallName(tc.number))
		})
	}
}

// TestMacOSSyscallTable_UnknownSyscall verifies unknown syscall numbers return empty/false.
func TestMacOSSyscallTable_UnknownSyscall(t *testing.T) {
	table := MacOSSyscallTable{}

	assert.Equal(t, "", table.GetSyscallName(9999))
	assert.False(t, table.IsNetworkSyscall(9999))
}

// TestNetworkSyscallWrapperNames verifies the fallback list contains expected names.
func TestNetworkSyscallWrapperNames(t *testing.T) {
	expected := []string{
		"socket", "connect", "bind", "listen", "accept",
		"sendto", "recvfrom", "sendmsg", "recvmsg",
		"socketpair", "shutdown", "setsockopt", "getsockopt",
		"getpeername", "getsockname",
	}
	for _, name := range expected {
		found := false
		for _, n := range networkSyscallWrapperNames {
			if n == name {
				found = true
				break
			}
		}
		assert.True(t, found, "expected %s in networkSyscallWrapperNames", name)
	}

	// sendmmsg and recvmmsg must not appear (Linux-specific).
	for _, name := range networkSyscallWrapperNames {
		assert.NotEqual(t, "sendmmsg", name, "sendmmsg is Linux-specific and should not be in the list")
		assert.NotEqual(t, "recvmmsg", name, "recvmmsg is Linux-specific and should not be in the list")
	}
}
