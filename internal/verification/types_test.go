package verification

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestFailureReason_IsTamperingSignal verifies that only hash_mismatch is
// classified as a tampering signal. All other failure reasons are environment
// causes and must return false.
func TestFailureReason_IsTamperingSignal(t *testing.T) {
	testCases := []struct {
		name   string
		reason FailureReason
		want   bool
	}{
		{"hash_mismatch is tampering signal", ReasonHashMismatch, true},
		{"hash_directory_not_found is environment cause", ReasonHashDirNotFound, false},
		{"hash_file_not_found is environment cause", ReasonHashFileNotFound, false},
		{"file_read_error is environment cause", ReasonFileReadError, false},
		{"permission_denied is environment cause", ReasonPermissionDenied, false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, tc.reason.IsTamperingSignal())
		})
	}
}

// TestUnverifiedFileUsage_IsTamperingSignal verifies that an unverified file
// usage carries a tampering signal only when its Failure field points to
// ReasonHashMismatch. skipped_no_validator (Failure == nil) and environment
// causes are never tampering signals.
func TestUnverifiedFileUsage_IsTamperingSignal(t *testing.T) {
	mismatch := ReasonHashMismatch
	notFound := ReasonHashFileNotFound

	testCases := []struct {
		name  string
		usage UnverifiedFileUsage
		want  bool
	}{
		{
			name: "skipped_no_validator is not tampering signal",
			usage: UnverifiedFileUsage{
				Path:    "/etc/app/cfg.toml",
				Reason:  string(UnverifiedReasonNoValidator),
				Context: "config",
				Failure: nil,
			},
			want: false,
		},
		{
			name: "hash_mismatch is tampering signal",
			usage: UnverifiedFileUsage{
				Path:    "/etc/app/cfg.toml",
				Reason:  string(UnverifiedReasonFromFailure(ReasonHashMismatch)),
				Context: "config",
				Failure: &mismatch,
			},
			want: true,
		},
		{
			name: "hash_file_not_found is environment cause",
			usage: UnverifiedFileUsage{
				Path:    "/etc/app/cfg.toml",
				Reason:  string(UnverifiedReasonFromFailure(ReasonHashFileNotFound)),
				Context: "config",
				Failure: &notFound,
			},
			want: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, tc.usage.IsTamperingSignal())
		})
	}
}
