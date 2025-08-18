package logging

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test errors
var (
	errHandler1 = errors.New("handler1 error")
	errHandler2 = errors.New("handler2 error")
)

// mockHandler is a test implementation of slog.Handler
type mockHandler struct {
	mu          sync.Mutex
	enabled     bool
	records     []slog.Record
	attrs       []slog.Attr
	groups      []string
	handleError error
}

func newMockHandler(enabled bool) *mockHandler {
	return &mockHandler{
		enabled: enabled,
		records: make([]slog.Record, 0),
		attrs:   make([]slog.Attr, 0),
		groups:  make([]string, 0),
	}
}

func (m *mockHandler) Enabled(_ context.Context, _ slog.Level) bool {
	return m.enabled
}

func (m *mockHandler) Handle(_ context.Context, r slog.Record) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.handleError != nil {
		return m.handleError
	}
	m.records = append(m.records, r.Clone())
	return nil
}

func (m *mockHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	m.mu.Lock()
	defer m.mu.Unlock()

	newHandler := &mockHandler{
		enabled:     m.enabled,
		records:     make([]slog.Record, 0),
		attrs:       append(m.attrs, attrs...),
		groups:      m.groups,
		handleError: m.handleError,
	}
	return newHandler
}

func (m *mockHandler) WithGroup(name string) slog.Handler {
	m.mu.Lock()
	defer m.mu.Unlock()

	newHandler := &mockHandler{
		enabled:     m.enabled,
		records:     make([]slog.Record, 0),
		attrs:       m.attrs,
		groups:      append(m.groups, name),
		handleError: m.handleError,
	}
	return newHandler
}

// getRecordCount returns the number of records in a thread-safe manner
func (m *mockHandler) getRecordCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.records)
}

func TestNewMultiHandler(t *testing.T) {
	handler1 := newMockHandler(true)
	handler2 := newMockHandler(false)

	multi := NewMultiHandler(handler1, handler2)

	assert.Len(t, multi.handlers, 2)
}

func TestMultiHandler_Enabled(t *testing.T) {
	tests := []struct {
		name     string
		handlers []slog.Handler
		expected bool
	}{
		{
			name:     "at least one handler enabled",
			handlers: []slog.Handler{newMockHandler(false), newMockHandler(true)},
			expected: true,
		},
		{
			name:     "no handlers enabled",
			handlers: []slog.Handler{newMockHandler(false), newMockHandler(false)},
			expected: false,
		},
		{
			name:     "all handlers enabled",
			handlers: []slog.Handler{newMockHandler(true), newMockHandler(true)},
			expected: true,
		},
		{
			name:     "no handlers",
			handlers: []slog.Handler{},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			multi := NewMultiHandler(tt.handlers...)
			result := multi.Enabled(context.Background(), slog.LevelInfo)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestMultiHandler_Handle(t *testing.T) {
	handler1 := newMockHandler(true)
	handler2 := newMockHandler(true)
	handler3 := newMockHandler(false) // disabled handler

	multi := NewMultiHandler(handler1, handler2, handler3)

	record := slog.NewRecord(time.Now(), slog.LevelInfo, "test message", 0)
	err := multi.Handle(context.Background(), record)
	assert.NoError(t, err)

	// Check that enabled handlers received the record
	assert.Equal(t, 1, handler1.getRecordCount(), "Handler1 should have received 1 record")
	assert.Equal(t, 1, handler2.getRecordCount(), "Handler2 should have received 1 record")
	assert.Equal(t, 0, handler3.getRecordCount(), "Handler3 (disabled) should have received 0 records")
}

func TestMultiHandler_HandleWithErrors(t *testing.T) {
	handler1 := newMockHandler(true)
	handler1.handleError = errHandler1

	handler2 := newMockHandler(true)
	handler2.handleError = errHandler2

	multi := NewMultiHandler(handler1, handler2)

	record := slog.NewRecord(time.Now(), slog.LevelInfo, "test message", 0)
	err := multi.Handle(context.Background(), record)

	require.Error(t, err, "Expected error, got nil")

	// Should contain both errors
	errStr := err.Error()
	assert.Contains(t, errStr, "handler1 error")
	assert.Contains(t, errStr, "handler2 error")
}

func TestMultiHandler_WithAttrs(t *testing.T) {
	handler1 := newMockHandler(true)
	handler2 := newMockHandler(true)

	multi := NewMultiHandler(handler1, handler2)
	attrs := []slog.Attr{slog.String("key", "value")}

	newMulti := multi.WithAttrs(attrs)

	// Verify it returns a new MultiHandler
	assert.NotSame(t, multi, newMulti, "WithAttrs should return a new MultiHandler instance")

	// Verify the new MultiHandler has the same number of handlers
	newMultiTyped := newMulti.(*MultiHandler)
	assert.Len(t, newMultiTyped.handlers, 2)
}

func TestMultiHandler_WithGroup(t *testing.T) {
	handler1 := newMockHandler(true)
	handler2 := newMockHandler(true)

	multi := NewMultiHandler(handler1, handler2)
	groupName := "testgroup"

	newMulti := multi.WithGroup(groupName)

	// Verify it returns a new MultiHandler
	assert.NotSame(t, multi, newMulti, "WithGroup should return a new MultiHandler instance")

	// Verify the new MultiHandler has the same number of handlers
	newMultiTyped := newMulti.(*MultiHandler)
	assert.Len(t, newMultiTyped.handlers, 2)
}

func TestMultiHandler_ConcurrentAccess(_ *testing.T) {
	handler := newMockHandler(true)
	multi := NewMultiHandler(handler)

	// Test concurrent access to Enabled
	done := make(chan bool, 10)
	for range 10 {
		go func() {
			multi.Enabled(context.Background(), slog.LevelInfo)
			done <- true
		}()
	}

	// Wait for all goroutines to complete
	for range 10 {
		<-done
	}

	// Test concurrent Handle calls
	for i := range 10 {
		go func(_ int) {
			record := slog.NewRecord(time.Now(), slog.LevelInfo, "test message", 0)
			multi.Handle(context.Background(), record)
			done <- true
		}(i)
	}

	// Wait for all goroutines to complete
	for range 10 {
		<-done
	}
}
