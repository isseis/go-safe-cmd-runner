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

// logRecord stores the captured details of a single slog log record.
type logRecord struct {
	Level   slog.Level
	Message string
	Attrs   map[string]any
}

// logCaptureHandler is an slog.Handler that captures log records for
// inspection in tests. It is safe for concurrent use: Handle and
// Records are guarded by a mutex.
type logCaptureHandler struct {
	mu          sync.Mutex
	records     []logRecord
	handleError error
}

// newLogCaptureHandler creates a logCaptureHandler that captures every
// record and always succeeds in Handle.
func newLogCaptureHandler() *logCaptureHandler {
	return &logCaptureHandler{}
}

// newFailingLogCaptureHandler creates a logCaptureHandler whose Handle
// always returns the given error, so that tests can verify that a
// recording failure does not change the read-safety verdict.
func newFailingLogCaptureHandler(err error) *logCaptureHandler { //nolint:unused
	return &logCaptureHandler{handleError: err}
}

// Enabled always returns true so every log level is captured.
func (h *logCaptureHandler) Enabled(context.Context, slog.Level) bool { return true }

// Handle captures the record. A copy of the attrs map is stored so that
// callers can inspect the values after Handle returns.
func (h *logCaptureHandler) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	attrs := make(map[string]any, r.NumAttrs())
	r.Attrs(func(a slog.Attr) bool {
		attrs[a.Key] = a.Value.Any()
		return true
	})
	h.records = append(h.records, logRecord{
		Level:   r.Level,
		Message: r.Message,
		Attrs:   attrs,
	})
	return h.handleError
}

// WithAttrs returns a new handler with the given attrs. The capture
// handler does not support pre-resolved attrs and panics to fail loudly
// on misuse rather than silently dropping attributes.
func (h *logCaptureHandler) WithAttrs(_ []slog.Attr) slog.Handler {
	panic("logCaptureHandler does not support WithAttrs; use slog.New(handler) and add attrs at the call site")
}

// WithGroup returns a new handler with the given group. The capture
// handler does not support groups and panics to fail loudly on misuse.
func (h *logCaptureHandler) WithGroup(_ string) slog.Handler {
	panic("logCaptureHandler does not support WithGroup")
}

// recordsCopy returns a copy of the captured records under the mutex.
func (h *logCaptureHandler) recordsCopy() []logRecord {
	h.mu.Lock()
	defer h.mu.Unlock()
	cp := make([]logRecord, len(h.records))
	copy(cp, h.records)
	return cp
}
