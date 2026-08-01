package groupmembership

import (
	"context"
	"log/slog"
	"os"
	"os/user"
	"slices"
	"strconv"
	"sync"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Common test helper functions

// logCaptureRecord holds one captured slog record: its level, its message,
// and a map of attribute name to value.
type logCaptureRecord struct {
	level      slog.Level
	message    string
	attributes map[string]any
}

// logCaptureHandler is a slog.Handler that captures records for tests that
// need to assert on log output. It records the level, message, and
// attribute name-to-value map of each record, and is safe for concurrent
// use. When failErr is non-nil, Handle returns it for every record instead
// of capturing, allowing tests to exercise handler failure paths.
type logCaptureHandler struct {
	mu      sync.Mutex
	records []logCaptureRecord
	failErr error
}

// newLogCaptureHandler returns a handler that captures records. A non-nil
// failErr makes Handle return it for every record.
func newLogCaptureHandler(failErr error) *logCaptureHandler {
	return &logCaptureHandler{failErr: failErr}
}

// Enabled implements slog.Handler.
func (h *logCaptureHandler) Enabled(context.Context, slog.Level) bool { return true }

// Handle implements slog.Handler.
func (h *logCaptureHandler) Handle(_ context.Context, r slog.Record) error {
	if h.failErr != nil {
		return h.failErr
	}
	attrs := make(map[string]any)
	r.Attrs(func(a slog.Attr) bool {
		attrs[a.Key] = a.Value.Any()
		return true
	})
	record := logCaptureRecord{level: r.Level, message: r.Message, attributes: attrs}

	h.mu.Lock()
	h.records = append(h.records, record)
	h.mu.Unlock()
	return nil
}

// WithAttrs implements slog.Handler. Handle captures only the attributes
// carried by the record itself, so attributes attached here would be dropped
// silently and an assertion on them would pass vacuously. Panic instead, so
// that a test reaching for slog.Logger.With fails loudly and the capture
// support is extended deliberately.
func (h *logCaptureHandler) WithAttrs([]slog.Attr) slog.Handler {
	panic("logCaptureHandler does not capture WithAttrs attributes")
}

// WithGroup implements slog.Handler. Groups are not captured, for the same
// reason as WithAttrs.
func (h *logCaptureHandler) WithGroup(string) slog.Handler {
	panic("logCaptureHandler does not capture WithGroup groups")
}

// Records returns a copy of the records captured so far.
func (h *logCaptureHandler) Records() []logCaptureRecord {
	h.mu.Lock()
	defer h.mu.Unlock()
	return slices.Clone(h.records)
}

// getCurrentUserGID returns the current user's primary group ID
func getCurrentUserGID(t *testing.T) uint32 {
	t.Helper()
	currentUser, err := user.Current()
	require.NoError(t, err, "Failed to get current user")

	currentGID, err := strconv.ParseUint(currentUser.Gid, 10, 32)
	require.NoError(t, err, "Failed to parse current user GID")

	return uint32(currentGID)
}

// createTempFileWithStat creates a temporary file and returns its UID/GID
func createTempFileWithStat(t *testing.T) (uint32, uint32, func()) {
	t.Helper()
	tempFile, err := os.CreateTemp("", "grouptest")
	require.NoError(t, err, "Failed to create temp file")

	cleanup := func() {
		tempFile.Close()
		os.Remove(tempFile.Name())
	}

	// Get file stat info
	fileInfo, err := tempFile.Stat()
	require.NoError(t, err, "Failed to stat temp file")

	stat, ok := fileInfo.Sys().(*syscall.Stat_t)
	require.True(t, ok, "Failed to get syscall.Stat_t")

	return stat.Uid, stat.Gid, cleanup
}

// Common test implementations

func TestGetGroupMembers_Common(t *testing.T) {
	currentGID := getCurrentUserGID(t)

	// Test getting members of current user's primary group
	members, err := getGroupMembers(currentGID)
	assert.NoError(t, err, "getGroupMembers should not return an error")
	assert.NotNil(t, members, "getGroupMembers should return a slice")

	// The result might be empty if the group has no explicit members
	// (only primary group assignment), which is valid
	t.Logf("Group %d has %d explicit members: %v", currentGID, len(members), members)
}

func TestGetGroupMembers_InvalidGID_Common(t *testing.T) {
	// Use a GID that's very unlikely to exist
	const invalidGID = 99999

	members, err := getGroupMembers(invalidGID)
	assert.NoError(t, err, "getGroupMembers should not return an error for non-existent group")
	assert.Empty(t, members, "getGroupMembers should return empty slice for non-existent group")
}
