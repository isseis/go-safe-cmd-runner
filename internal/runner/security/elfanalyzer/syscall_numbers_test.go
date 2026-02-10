//go:build test

package elfanalyzer

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestX86_64SyscallTable_GetSyscallName(t *testing.T) {
	table := NewX86_64SyscallTable()

	tests := []struct {
		name     string
		number   int
		wantName string
	}{
		{"socket", 41, "socket"},
		{"connect", 42, "connect"},
		{"accept", 43, "accept"},
		{"accept4", 288, "accept4"},
		{"sendto", 44, "sendto"},
		{"recvfrom", 45, "recvfrom"},
		{"sendmsg", 46, "sendmsg"},
		{"recvmsg", 47, "recvmsg"},
		{"bind", 49, "bind"},
		{"listen", 50, "listen"},
		{"read", 0, "read"},
		{"write", 1, "write"},
		{"exit", 60, "exit"},
		{"unknown syscall", 999, ""},
		{"negative number", -1, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := table.GetSyscallName(tt.number)
			assert.Equal(t, tt.wantName, got)
		})
	}
}

func TestX86_64SyscallTable_IsNetworkSyscall(t *testing.T) {
	table := NewX86_64SyscallTable()

	tests := []struct {
		name   string
		number int
		want   bool
	}{
		{"socket is network", 41, true},
		{"connect is network", 42, true},
		{"accept is network", 43, true},
		{"accept4 is network", 288, true},
		{"sendto is network", 44, true},
		{"recvfrom is network", 45, true},
		{"sendmsg is network", 46, true},
		{"recvmsg is network", 47, true},
		{"bind is network", 49, true},
		{"listen is network", 50, true},
		{"read is not network", 0, false},
		{"write is not network", 1, false},
		{"exit is not network", 60, false},
		{"unknown syscall", 999, false},
		{"negative number", -1, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := table.IsNetworkSyscall(tt.number)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestX86_64SyscallTable_GetNetworkSyscalls(t *testing.T) {
	table := NewX86_64SyscallTable()
	networkSyscalls := table.GetNetworkSyscalls()

	// Should have exactly 10 network syscalls
	assert.Len(t, networkSyscalls, 10)

	// All returned numbers should be network syscalls
	for _, num := range networkSyscalls {
		assert.True(t, table.IsNetworkSyscall(num),
			"syscall %d (%s) should be a network syscall",
			num, table.GetSyscallName(num))
	}

	// Verify specific expected network syscall numbers are present
	expectedNumbers := map[int]bool{
		41: true, 42: true, 43: true, 44: true, 45: true,
		46: true, 47: true, 49: true, 50: true, 288: true,
	}
	for _, num := range networkSyscalls {
		assert.True(t, expectedNumbers[num],
			"unexpected network syscall number: %d", num)
	}
}
