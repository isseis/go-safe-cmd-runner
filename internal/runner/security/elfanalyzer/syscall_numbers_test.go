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
