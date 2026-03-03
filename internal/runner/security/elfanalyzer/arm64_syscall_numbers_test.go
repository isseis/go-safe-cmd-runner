//go:build test

package elfanalyzer

import (
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestARM64LinuxSyscallTable_GetSyscallName(t *testing.T) {
	table := NewARM64LinuxSyscallTable()

	tests := []struct {
		number int
		want   string
	}{
		{198, "socket"},
		{199, "socketpair"},
		{200, "bind"},
		{201, "listen"},
		{202, "accept"},
		{203, "connect"},
		{206, "sendto"},
		{207, "recvfrom"},
		{211, "sendmsg"},
		{212, "recvmsg"},
		{242, "accept4"},
		{243, "recvmmsg"},
		{269, "sendmmsg"},
		// Non-network syscalls
		{63, "read"},
		{64, "write"},
		// Unknown syscall
		{9999, ""},
		{-1, ""},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			assert.Equal(t, tt.want, table.GetSyscallName(tt.number))
		})
	}
}

func TestARM64LinuxSyscallTable_IsNetworkSyscall(t *testing.T) {
	table := NewARM64LinuxSyscallTable()

	tests := []struct {
		number int
		want   bool
	}{
		{198, true},   // socket
		{199, true},   // socketpair
		{200, true},   // bind
		{201, true},   // listen
		{202, true},   // accept
		{203, true},   // connect
		{206, true},   // sendto
		{207, true},   // recvfrom
		{211, true},   // sendmsg
		{212, true},   // recvmsg
		{242, true},   // accept4
		{243, true},   // recvmmsg
		{269, true},   // sendmmsg
		{63, false},   // read (non-network)
		{64, false},   // write (non-network)
		{9999, false}, // unknown
	}

	for _, tt := range tests {
		name := table.GetSyscallName(tt.number)
		if name == "" {
			name = "unknown"
		}
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tt.want, table.IsNetworkSyscall(tt.number))
		})
	}
}

func TestARM64LinuxSyscallTable_GetNetworkSyscalls(t *testing.T) {
	table := NewARM64LinuxSyscallTable()

	expected := []int{198, 199, 200, 201, 202, 203, 206, 207, 211, 212, 242, 243, 269}
	got := table.GetNetworkSyscalls()

	require.Len(t, got, len(expected), "should have exactly %d network syscalls", len(expected))

	sort.Ints(got)
	sort.Ints(expected)
	assert.Equal(t, expected, got)
}

func TestARM64LinuxSyscallTable_ImplementsInterface(_ *testing.T) {
	// Compile-time interface check
	var _ SyscallNumberTable = (*ARM64LinuxSyscallTable)(nil)
}

func TestARM64LinuxSyscallTable_GetNetworkSyscallsReturnsCopy(t *testing.T) {
	table := NewARM64LinuxSyscallTable()

	// Modify the returned slice and verify the internal state is unchanged
	got1 := table.GetNetworkSyscalls()
	require.NotEmpty(t, got1)
	got1[0] = -9999

	got2 := table.GetNetworkSyscalls()
	for _, n := range got2 {
		assert.NotEqual(t, -9999, n, "GetNetworkSyscalls should return a copy, not the internal slice")
	}
}
