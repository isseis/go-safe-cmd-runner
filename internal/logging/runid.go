package logging

import (
	"crypto/rand"
	"errors"
	"fmt"
	"time"

	"github.com/oklog/ulid/v2"
)

// MaxRunIDLength is the maximum number of characters a run ID may contain.
const MaxRunIDLength = 64

// RunIDFormatDescription describes the accepted run ID format. It is safe to
// print: it is derived from MaxRunIDLength and never contains any part of a
// rejected value.
var RunIDFormatDescription = fmt.Sprintf("1-%d characters, each of A-Z a-z 0-9 '_' '-'", MaxRunIDLength)

// ErrInvalidRunID is returned when a run ID does not match the accepted format.
var ErrInvalidRunID = errors.New("invalid run ID")

// GenerateRunID generates a new ULID for run identification.
// Its output always satisfies ValidateRunID.
func GenerateRunID() string {
	entropy := ulid.Monotonic(rand.Reader, 0)
	return ulid.MustNew(ulid.Timestamp(time.Now()), entropy).String()
}

// ValidateRunID reports whether runID matches the accepted format, returning an
// error wrapping ErrInvalidRunID when it does not.
//
// The returned error never contains the rejected value verbatim. When the value
// contains a disallowed byte, the error identifies that byte's index and its
// Go-quoted form (%q), which escapes newline, NUL, ESC and quote characters.
func ValidateRunID(runID string) error {
	if len(runID) == 0 {
		return fmt.Errorf("%w: run ID is empty", ErrInvalidRunID)
	}
	if len(runID) > MaxRunIDLength {
		return fmt.Errorf("%w: length %d exceeds maximum %d", ErrInvalidRunID, len(runID), MaxRunIDLength)
	}
	for i := 0; i < len(runID); i++ {
		if !isAllowedRunIDByte(runID[i]) {
			return fmt.Errorf("%w: byte at index %d has value %q", ErrInvalidRunID, i, runID[i])
		}
	}
	return nil
}

// isAllowedRunIDByte reports whether b is a byte the accepted run ID format
// permits: A-Z a-z 0-9 '_' '-'.
func isAllowedRunIDByte(b byte) bool {
	switch {
	case b >= 'A' && b <= 'Z':
		return true
	case b >= 'a' && b <= 'z':
		return true
	case b >= '0' && b <= '9':
		return true
	case b == '_' || b == '-':
		return true
	default:
		return false
	}
}
