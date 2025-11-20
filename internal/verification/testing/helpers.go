//go:build test

package verificationtesting

import (
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/stretchr/testify/mock"
)

// MatchRuntimeGroupWithName creates a mock.ArgumentMatcher that validates RuntimeGroup with expected group name.
// This is a helper function to avoid code duplication in mock expectations.
//
// Usage:
//
//	mockVerificationManager.On("VerifyGroupFiles", verificationtesting.MatchRuntimeGroupWithName("test-group")).Return(...)
func MatchRuntimeGroupWithName(expectedName string) interface{} {
	return mock.MatchedBy(func(rg *runnertypes.RuntimeGroup) bool {
		return rg != nil && rg.Spec != nil && rg.Spec.Name == expectedName
	})
}
