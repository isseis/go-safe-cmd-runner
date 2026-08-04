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

// logStep is one recorded WithAttrs or WithGroup call, in call order, so
// Handle can tell whether attrs were added before or after a given group opened.
type logStep struct {
	group string      // non-empty for a WithGroup step
	attrs []slog.Attr // non-nil for a WithAttrs step
}

// LogRecorder captures log records with their attributes for testing.
// If FailErr is non-nil, Handle returns it for every record instead of capturing,
// allowing tests to exercise handler failure paths.
type LogRecorder struct {
	mu      *sync.Mutex
	records *[]RecordSnapshot
	steps   []logStep
	FailErr error
}

// NewLogRecorder returns a new LogRecorder. If failErr is non-nil,
// Handle returns it for every record instead of capturing.
func NewLogRecorder(failErr error) *LogRecorder {
	return &LogRecorder{
		mu:      &sync.Mutex{},
		records: &[]RecordSnapshot{},
		FailErr: failErr,
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

	root := make(map[string]any)
	current := root // namespace currently open, per the most recent WithGroup step

	// Replay steps in order so attrs added before a WithGroup stay at root,
	// and attrs added after it nest under the group.
	for _, step := range lr.steps {
		if step.group != "" {
			next := make(map[string]any)
			current[step.group] = next
			current = next
			continue
		}
		for _, a := range step.attrs {
			current[a.Key] = a.Value.Any()
		}
	}

	// Record-level attrs from the log call itself also belong under the
	// currently open group, matching slog.Handler semantics.
	r.Attrs(func(a slog.Attr) bool {
		current[a.Key] = a.Value.Any()
		return true
	})

	snapshot := RecordSnapshot{
		Level:   r.Level,
		Message: r.Message,
		Attrs:   root,
	}

	*lr.records = append(*lr.records, snapshot)
	return nil
}

// WithAttrs implements slog.Handler and returns a new handler with accumulated attributes.
func (lr *LogRecorder) WithAttrs(attrs []slog.Attr) slog.Handler {
	lr.mu.Lock()
	defer lr.mu.Unlock()

	newRecorder := &LogRecorder{
		mu:      lr.mu,
		records: lr.records,
		steps:   append(lr.steps[:len(lr.steps):len(lr.steps)], logStep{attrs: attrs}),
		FailErr: lr.FailErr,
	}
	return newRecorder
}

// WithGroup implements slog.Handler and returns a new handler with the group name.
func (lr *LogRecorder) WithGroup(name string) slog.Handler {
	lr.mu.Lock()
	defer lr.mu.Unlock()

	newRecorder := &LogRecorder{
		mu:      lr.mu,
		records: lr.records,
		steps:   append(lr.steps[:len(lr.steps):len(lr.steps)], logStep{group: name}),
		FailErr: lr.FailErr,
	}
	return newRecorder
}

// Records returns a copy of all captured records.
func (lr *LogRecorder) Records() []RecordSnapshot {
	lr.mu.Lock()
	defer lr.mu.Unlock()

	result := make([]RecordSnapshot, len(*lr.records))
	for i, record := range *lr.records {
		result[i] = record // copy the struct wholesale so future fields aren't dropped
		if record.Attrs != nil {
			result[i].Attrs = deepCopyAttrs(record.Attrs)
		}
	}
	return result
}

// deepCopyAttrs recursively copies a possibly-nested Attrs map to ensure
// independence from internal state (nested maps come from WithGroup).
func deepCopyAttrs(m map[string]any) map[string]any {
	cp := make(map[string]any, len(m))
	for key, value := range m {
		if nested, ok := value.(map[string]any); ok {
			cp[key] = deepCopyAttrs(nested)
		} else {
			cp[key] = value
		}
	}
	return cp
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
