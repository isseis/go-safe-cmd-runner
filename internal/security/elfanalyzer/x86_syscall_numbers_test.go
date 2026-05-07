package elfanalyzer

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestX86_64SyscallTable_NetworkSyscalls verifies that all network-related syscalls
// specified in the requirements are registered in the x86_64 syscall table.
func TestX86_64SyscallTable_NetworkSyscalls(t *testing.T) {
	table := NewX86_64SyscallTable()

	// All network syscalls required by the specification (FR-3.1.x)
	requiredNetworkSyscalls := []struct {
		number int
		name   string
	}{
		{41, "socket"},
		{42, "connect"},
		{43, "accept"},
		{44, "sendto"},
		{45, "recvfrom"},
		{46, "sendmsg"},
		{47, "recvmsg"},
		{49, "bind"},
		{50, "listen"},
		{53, "socketpair"},
		{288, "accept4"},
		{299, "recvmmsg"},
		{307, "sendmmsg"},
	}

	for _, tc := range requiredNetworkSyscalls {
		t.Run(tc.name, func(t *testing.T) {
			assert.True(t, table.IsNetworkSyscall(tc.number),
				"syscall %d (%s) should be a network syscall", tc.number, tc.name)
			assert.Equal(t, tc.name, table.GetSyscallName(tc.number),
				"syscall %d should have name %q", tc.number, tc.name)
		})
	}

	// Verify all required syscalls appear in GetNetworkSyscalls()
	networkNums := table.GetNetworkSyscalls()
	networkNumSet := make(map[int]bool, len(networkNums))
	for _, n := range networkNums {
		networkNumSet[n] = true
	}
	for _, tc := range requiredNetworkSyscalls {
		assert.True(t, networkNumSet[tc.number],
			"syscall %d (%s) should appear in GetNetworkSyscalls()", tc.number, tc.name)
	}
	assert.Equal(t, len(requiredNetworkSyscalls), len(networkNums),
		"GetNetworkSyscalls() should return exactly the required network syscalls")
}

func TestX86_64SyscallTable_NonNetworkSyscall(t *testing.T) {
	table := NewX86_64SyscallTable()

	assert.False(t, table.IsNetworkSyscall(1), "write (1) should not be a network syscall")
	assert.Equal(t, "write", table.GetSyscallName(1))
}

func TestX86_64SyscallTable_UnknownSyscall(t *testing.T) {
	table := NewX86_64SyscallTable()

	assert.False(t, table.IsNetworkSyscall(9999))
	assert.Equal(t, "", table.GetSyscallName(9999))
}

func TestX86_64SyscallTable_GetNetworkSyscalls_ReturnsCopy(t *testing.T) {
	table := NewX86_64SyscallTable()

	nums1 := table.GetNetworkSyscalls()
	nums2 := table.GetNetworkSyscalls()
	assert.Equal(t, nums1, nums2)

	// Mutating the returned slice should not affect subsequent calls
	if len(nums1) > 0 {
		nums1[0] = -1
		nums3 := table.GetNetworkSyscalls()
		assert.Equal(t, nums2, nums3)
	}
}

func TestX86_64SyscallTable_IsExecSyscall(t *testing.T) {
	table := NewX86_64SyscallTable()

	tests := []struct {
		name   string
		number int
		want   bool
	}{
		{name: "execve", number: 59, want: true},
		{name: "execveat", number: 322, want: true},
		{name: "socket", number: 41, want: false},
		{name: "read", number: 0, want: false},
		{name: "unknown-negative", number: -1, want: false},
		{name: "unknown-large", number: 9999, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, table.IsExecSyscall(tt.number))
		})
	}
}

func TestX86_64SyscallTable_GetExecSyscalls(t *testing.T) {
	table := NewX86_64SyscallTable()

	execNums := table.GetExecSyscalls()
	execNumSet := make(map[int]bool, len(execNums))
	for _, n := range execNums {
		execNumSet[n] = true
	}

	assert.True(t, execNumSet[59], "GetExecSyscalls() should include execve(59)")
	assert.True(t, execNumSet[322], "GetExecSyscalls() should include execveat(322)")
	assert.Len(t, execNums, 2, "GetExecSyscalls() should include only exec syscalls")

	// Mutating the returned slice must not affect subsequent calls.
	if len(execNums) > 0 {
		execNums[0] = -1
		execNums2 := table.GetExecSyscalls()
		assert.True(t, contains(execNums2, 59))
		assert.True(t, contains(execNums2, 322))
	}
}

func contains(nums []int, target int) bool {
	for _, n := range nums {
		if n == target {
			return true
		}
	}
	return false
}
