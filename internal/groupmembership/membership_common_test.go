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

// capturedRecord holds the parts of a slog.Record that tests assert on.
type capturedRecord struct {
	level slog.Level
	msg   string
	attrs map[string]slog.Value
}

// logCaptureHandler is a slog.Handler that keeps the records passed to it so
// that tests can assert on their level, message and attributes. When handleErr
// is non-nil, every Handle call also reports that error, which lets tests
// confirm that a failing log destination does not change the caller's result.
type logCaptureHandler struct {
	mu        sync.Mutex
	records   []capturedRecord
	handleErr error
}

func (h *logCaptureHandler) Enabled(context.Context, slog.Level) bool { return true }

func (h *logCaptureHandler) Handle(_ context.Context, record slog.Record) error {
	attrs := make(map[string]slog.Value, record.NumAttrs())
	record.Attrs(func(a slog.Attr) bool {
		attrs[a.Key] = a.Value
		return true
	})

	// Handle may run on several goroutines at once, so the captured slice is
	// guarded by a mutex and only ever handed out as a copy.
	h.mu.Lock()
	defer h.mu.Unlock()
	h.records = append(h.records, capturedRecord{level: record.Level, msg: record.Message, attrs: attrs})
	return h.handleErr
}

// WithAttrs and WithGroup are not supported: this handler captures only the
// attributes carried by the record itself. Panicking rather than silently
// dropping them keeps a wrapped logger from making a test under-verify.
func (h *logCaptureHandler) WithAttrs([]slog.Attr) slog.Handler {
	panic("logCaptureHandler does not support WithAttrs")
}

func (h *logCaptureHandler) WithGroup(string) slog.Handler {
	panic("logCaptureHandler does not support WithGroup")
}

// captured returns a copy of the captured slice. The per-record attribute maps
// are shared with the handler and must not be mutated by the caller.
func (h *logCaptureHandler) captured() []capturedRecord {
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
