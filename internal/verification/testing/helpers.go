//go:build test

package verificationtesting

import (
	"github.com/isseis/go-safe-cmd-runner/internal/verification"
	"github.com/stretchr/testify/mock"
)

// MatchRuntimeGroupWithName creates a matcher that validates GroupVerificationInput with the expected group name.
// This is a helper function to avoid code duplication in mock expectations.
//
// Usage:
//
//	mockVerificationManager.On("VerifyGroupFiles", verificationtesting.MatchRuntimeGroupWithName("test-group")).Return(...)
func MatchRuntimeGroupWithName(expectedName string) any {
	return mock.MatchedBy(func(input *verification.GroupVerificationInput) bool {
		return input != nil && input.Name == expectedName
	})
}
