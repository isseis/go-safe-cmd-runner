package redaction

import (
	"errors"
	"testing"
)

func TestErrRedactionDepthExceeded_Error(t *testing.T) {
	err := &ErrRedactionDepthExceeded{
		Key:   "test_key",
		Depth: 10,
	}

	expected := `redaction depth limit (10) exceeded for attribute "test_key"`
	if got := err.Error(); got != expected {
		t.Errorf("Error() = %q, want %q", got, expected)
	}
}

func TestErrLogValuePanic_Error(t *testing.T) {
	err := &ErrLogValuePanic{
		Key:        "test_key",
		PanicValue: "test panic",
		StackTrace: "stack trace here",
	}

	expected := `LogValue() panicked for attribute "test_key": test panic`
	if got := err.Error(); got != expected {
		t.Errorf("Error() = %q, want %q", got, expected)
	}
}

func TestErrRegexCompilationFailed_Error(t *testing.T) {
	innerErr := errors.New("invalid regex")
	err := &ErrRegexCompilationFailed{
		Pattern: "[invalid",
		Err:     innerErr,
	}

	expected := `failed to compile regex pattern "[invalid": invalid regex`
	if got := err.Error(); got != expected {
		t.Errorf("Error() = %q, want %q", got, expected)
	}
}
