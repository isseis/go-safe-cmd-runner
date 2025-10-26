package securitytesting_test

import (
	"errors"
	"testing"

	securitytesting "github.com/isseis/go-safe-cmd-runner/internal/runner/security/testing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestMockValidator_ValidateAllEnvironmentVars(t *testing.T) {
	// Arrange
	mockValidator := new(securitytesting.MockValidator)
	envVars := map[string]string{"TEST": "value"}
	expectedErr := errors.New("validation error")

	mockValidator.On("ValidateAllEnvironmentVars", envVars).Return(expectedErr)

	// Act
	err := mockValidator.ValidateAllEnvironmentVars(envVars)

	// Assert
	assert.Equal(t, expectedErr, err)
	mockValidator.AssertExpectations(t)
}

func TestMockValidator_ValidateEnvironmentValue(t *testing.T) {
	// Arrange
	mockValidator := new(securitytesting.MockValidator)
	expectedErr := errors.New("validation error")

	mockValidator.On("ValidateEnvironmentValue", "KEY", "value").Return(expectedErr)

	// Act
	err := mockValidator.ValidateEnvironmentValue("KEY", "value")

	// Assert
	assert.Equal(t, expectedErr, err)
	mockValidator.AssertExpectations(t)
}

func TestMockValidator_ValidateCommand(t *testing.T) {
	// Arrange
	mockValidator := new(securitytesting.MockValidator)
	expectedErr := errors.New("validation error")

	mockValidator.On("ValidateCommand", "/bin/test").Return(expectedErr)

	// Act
	err := mockValidator.ValidateCommand("/bin/test")

	// Assert
	assert.Equal(t, expectedErr, err)
	mockValidator.AssertExpectations(t)
}

func TestMockValidator_SuccessScenario(t *testing.T) {
	// Arrange
	mockValidator := new(securitytesting.MockValidator)

	mockValidator.On("ValidateAllEnvironmentVars", mock.Anything).Return(nil)
	mockValidator.On("ValidateEnvironmentValue", mock.Anything, mock.Anything).Return(nil)
	mockValidator.On("ValidateCommand", mock.Anything).Return(nil)

	// Act
	err1 := mockValidator.ValidateAllEnvironmentVars(map[string]string{"TEST": "value"})
	err2 := mockValidator.ValidateEnvironmentValue("KEY", "value")
	err3 := mockValidator.ValidateCommand("/bin/test")

	// Assert
	assert.NoError(t, err1)
	assert.NoError(t, err2)
	assert.NoError(t, err3)
	mockValidator.AssertExpectations(t)
}
