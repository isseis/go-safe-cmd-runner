//go:build test || performance || integration

package tu

import (
	"context"
	"log/slog"
	"sync"
)

// RecordSnapshot is a captured log record.
type RecordSnapshot struct {
	Level   slog.Level
	Message string
	Attrs   map[string]any
}

// LogRecorder captures log records with their attributes for testing.
// If FailErr is non-nil, Handle returns it for every record instead of capturing,
// allowing tests to exercise handler failure paths.
type LogRecorder struct {
	mu        sync.Mutex
	records   []RecordSnapshot
	pendAttrs []slog.Attr
	pendGroup string
	FailErr   error
}

// NewLogRecorder returns a new LogRecorder. If failErr is non-nil,
// Handle returns it for every record instead of capturing.
func NewLogRecorder(failErr error) *LogRecorder {
	return &LogRecorder{
		records:   make([]RecordSnapshot, 0),
		pendAttrs: make([]slog.Attr, 0),
		FailErr:   failErr,
	}
}

// Enabled implements slog.Handler.
func (lr *LogRecorder) Enabled(context.Context, slog.Level) bool {
	return true
}

// Handle implements slog.Handler and captures record with accumulated attributes.
func (lr *LogRecorder) Handle(_ context.Context, r slog.Record) error {
	lr.mu.Lock()
	defer lr.mu.Unlock()

	if lr.FailErr != nil {
		return lr.FailErr
	}

	attrs := make(map[string]any)

	// First capture accumulated attributes from WithAttrs
	for _, a := range lr.pendAttrs {
		attrs[a.Key] = a.Value.Any()
	}

	// Then capture attributes from the record itself
	r.Attrs(func(a slog.Attr) bool {
		attrs[a.Key] = a.Value.Any()
		return true
	})

	snapshot := RecordSnapshot{
		Level:   r.Level,
		Message: r.Message,
		Attrs:   attrs,
	}

	lr.records = append(lr.records, snapshot)
	return nil
}

// WithAttrs implements slog.Handler and returns a new handler with accumulated attributes.
func (lr *LogRecorder) WithAttrs(attrs []slog.Attr) slog.Handler {
	lr.mu.Lock()
	defer lr.mu.Unlock()

	newRecorder := &LogRecorder{
		records:   lr.records,
		pendAttrs: append(lr.pendAttrs[:len(lr.pendAttrs):len(lr.pendAttrs)], attrs...),
		pendGroup: lr.pendGroup,
	}
	return newRecorder
}

// WithGroup implements slog.Handler and returns a new handler with the group name.
func (lr *LogRecorder) WithGroup(name string) slog.Handler {
	lr.mu.Lock()
	defer lr.mu.Unlock()

	newRecorder := &LogRecorder{
		records:   lr.records,
		pendAttrs: lr.pendAttrs,
		pendGroup: name,
	}
	return newRecorder
}

// Records returns a copy of all captured records.
func (lr *LogRecorder) Records() []RecordSnapshot {
	lr.mu.Lock()
	defer lr.mu.Unlock()

	result := make([]RecordSnapshot, len(lr.records))
	copy(result, lr.records)
	return result
}

// CallbackHandler calls a function for each handled record without capturing.
type CallbackHandler struct {
	onHandle func(slog.Record)
}

// NewCallbackHandler returns a new CallbackHandler.
func NewCallbackHandler(onHandle func(slog.Record)) *CallbackHandler {
	return &CallbackHandler{onHandle: onHandle}
}

// Enabled implements slog.Handler.
func (h *CallbackHandler) Enabled(context.Context, slog.Level) bool {
	return true
}

// Handle implements slog.Handler and calls the callback.
func (h *CallbackHandler) Handle(_ context.Context, r slog.Record) error {
	if h.onHandle != nil {
		h.onHandle(r)
	}
	return nil
}

// WithAttrs implements slog.Handler and returns itself.
func (h *CallbackHandler) WithAttrs([]slog.Attr) slog.Handler {
	return h
}

// WithGroup implements slog.Handler and returns itself.
func (h *CallbackHandler) WithGroup(string) slog.Handler {
	return h
}
