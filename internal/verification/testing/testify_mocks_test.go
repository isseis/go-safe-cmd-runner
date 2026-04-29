package verificationtesting_test

import (
	"errors"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/verification"
	verificationtesting "github.com/isseis/go-safe-cmd-runner/internal/verification/testing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMockManager_ResolvePath(t *testing.T) {
	mockManager := new(verificationtesting.MockManager)
	expectedPath := "/usr/bin/test"
	expectedErr := errors.New("resolution error")

	mockManager.On("ResolvePath", "test").Return(expectedPath, expectedErr)

	path, err := mockManager.ResolvePath("test")

	assert.Equal(t, expectedPath, path)
	assert.Equal(t, expectedErr, err)
	mockManager.AssertExpectations(t)
}

func TestMockManager_VerifyGroupFiles(t *testing.T) {
	mockManager := new(verificationtesting.MockManager)
	input := &verification.GroupVerificationInput{Name: "test-group"}
	expectedResult := &verification.Result{TotalFiles: 1, VerifiedFiles: 1}
	expectedErr := errors.New("verification error")

	mockManager.On("VerifyGroupFiles", input).Return(expectedResult, expectedErr)

	result, err := mockManager.VerifyGroupFiles(input)

	assert.Equal(t, expectedResult, result)
	assert.Equal(t, expectedErr, err)
	mockManager.AssertExpectations(t)
}

func TestMockManager_SuccessScenario(t *testing.T) {
	mockManager := new(verificationtesting.MockManager)
	input := &verification.GroupVerificationInput{Name: "test-group"}
	expectedResult := &verification.Result{TotalFiles: 1, VerifiedFiles: 1}

	mockManager.On("ResolvePath", "test").Return("/usr/bin/test", nil)
	mockManager.On("VerifyGroupFiles", verificationtesting.MatchRuntimeGroupWithName("test-group")).Return(expectedResult, nil)

	path, err1 := mockManager.ResolvePath("test")
	result, err2 := mockManager.VerifyGroupFiles(input)

	assert.NoError(t, err1)
	assert.Equal(t, "/usr/bin/test", path)
	assert.NoError(t, err2)
	require.NotNil(t, result)
	assert.Equal(t, 1, result.TotalFiles)
	assert.Equal(t, 1, result.VerifiedFiles)
	mockManager.AssertExpectations(t)
}

func TestMockManager_VerifyGroupFiles_NilResult(t *testing.T) {
	mockManager := new(verificationtesting.MockManager)
	input := &verification.GroupVerificationInput{Name: "test-group"}
	expectedErr := errors.New("verification failed")

	mockManager.On("VerifyGroupFiles", input).Return(nil, expectedErr)

	result, err := mockManager.VerifyGroupFiles(input)

	assert.Nil(t, result)
	assert.Equal(t, expectedErr, err)
	mockManager.AssertExpectations(t)
}

func TestMockManager_VerifyCommandDynLibDeps(t *testing.T) {
	mockManager := new(verificationtesting.MockManager)
	expectedErr := errors.New("dynlib verification error")

	mockManager.On("VerifyCommandDynLibDeps", "/usr/bin/test").Return(expectedErr)

	err := mockManager.VerifyCommandDynLibDeps("/usr/bin/test")

	assert.Equal(t, expectedErr, err)
	mockManager.AssertExpectations(t)
}
