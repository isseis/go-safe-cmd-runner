package groupmembership

import (
	"context"
	"log/slog"
	"os"
	"os/user"
	"strconv"
	"sync"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Common test helper functions

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

// capturedRecord describes a single log record captured by captureHandler.
type capturedRecord struct {
	level   slog.Level
	message string
	attrs   map[string]any
}

// captureHandler is a slog.Handler used by tests in this package to observe
// emitted log records without depending on a global default. It is safe for
// concurrent use: Handle may be called from many goroutines (for example by
// the -race tests on sudoUIDAdoptionReporter).
type captureHandler struct {
	mu      sync.Mutex
	records []capturedRecord
	// handleErr, when non-nil, is returned from Handle. Tests use it to
	// simulate a handler that fails to write.
	handleErr error
}

func newCaptureHandler() *captureHandler {
	return &captureHandler{}
}

// Enabled always returns true so that no records are filtered by level.
func (h *captureHandler) Enabled(_ context.Context, _ slog.Level) bool {
	return true
}

// Handle stores a capturedRecord summarising the log record.
func (h *captureHandler) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	rec := capturedRecord{
		level:   r.Level,
		message: r.Message,
		attrs:   make(map[string]any, r.NumAttrs()),
	}
	r.Attrs(func(a slog.Attr) bool {
		rec.attrs[a.Key] = a.Value.Any()
		return true
	})
	h.records = append(h.records, rec)
	return h.handleErr
}

// WithAttrs and WithGroup return the same handler without recording the
// chained attrs or group. The tests in this package do not use chained
// loggers, so the simplification is safe here; if a future test starts
// using With(...), the captured records will silently drop the chained
// attributes, and the test author must replace the handler accordingly.
func (h *captureHandler) WithAttrs(_ []slog.Attr) slog.Handler {
	return h
}

func (h *captureHandler) WithGroup(_ string) slog.Handler {
	return h
}

// snapshot returns a copy of the captured records under the handler's lock
// so that callers can iterate without racing the handler.
func (h *captureHandler) snapshot() []capturedRecord {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]capturedRecord, len(h.records))
	copy(out, h.records)
	return out
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
