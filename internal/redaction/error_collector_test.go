//go:build test

package redaction

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

func TestInMemoryErrorCollector_RecordFailure(t *testing.T) {
	collector := NewInMemoryErrorCollector(0) // Unlimited

	// Record a failure
	testErr := errors.New("test error")
	collector.RecordFailure("test_key", testErr)

	// Verify it was recorded
	failures := collector.GetFailures()
	require.Len(t, failures, 1)
	assert.Equal(t, "test_key", failures[0].Key)
	assert.Equal(t, testErr, failures[0].Err)
	assert.WithinDuration(t, time.Now(), failures[0].Timestamp, time.Second)
}

func TestInMemoryErrorCollector_GetFailures_ReturnsCopy(t *testing.T) {
	collector := NewInMemoryErrorCollector(0)

	// Record a failure
	collector.RecordFailure("test_key", errors.New("test error"))

	// Get failures and modify the returned slice
	failures1 := collector.GetFailures()
	failures1[0].Key = "modified"

	// Get failures again and verify original is unchanged
	failures2 := collector.GetFailures()
	assert.Equal(t, "test_key", failures2[0].Key)
}

func TestInMemoryErrorCollector_Clear(t *testing.T) {
	collector := NewInMemoryErrorCollector(0)

	// Record failures
	collector.RecordFailure("key1", errors.New("error1"))
	collector.RecordFailure("key2", errors.New("error2"))

	// Verify they were recorded
	assert.Equal(t, 2, collector.Count())

	// Clear
	collector.Clear()

	// Verify cleared
	assert.Equal(t, 0, collector.Count())
	assert.False(t, collector.HasFailures())
}

func TestInMemoryErrorCollector_MaxSize(t *testing.T) {
	const maxSize = 3
	collector := NewInMemoryErrorCollector(maxSize)

	// Record more failures than maxSize
	for i := 0; i < 5; i++ {
		collector.RecordFailure("key", errors.New("error"))
	}

	// Verify only maxSize failures are kept
	assert.Equal(t, maxSize, collector.Count())
}

func TestInMemoryErrorCollector_MaxSize_KeepsNewest(t *testing.T) {
	const maxSize = 2
	collector := NewInMemoryErrorCollector(maxSize)

	// Record failures with different errors
	err1 := errors.New("error1")
	err2 := errors.New("error2")
	err3 := errors.New("error3")

	collector.RecordFailure("key1", err1)
	time.Sleep(10 * time.Millisecond) // Ensure different timestamps
	collector.RecordFailure("key2", err2)
	time.Sleep(10 * time.Millisecond)
	collector.RecordFailure("key3", err3)

	// Verify oldest (err1) was removed, keeping err2 and err3
	failures := collector.GetFailures()
	require.Len(t, failures, maxSize)
	assert.Equal(t, err2, failures[0].Err)
	assert.Equal(t, err3, failures[1].Err)
}

func TestInMemoryErrorCollector_ConcurrentAccess(t *testing.T) {
	collector := NewInMemoryErrorCollector(100)
	const goroutines = 10
	const recordsPerGoroutine = 10

	var wg sync.WaitGroup
	wg.Add(goroutines)

	// Record failures concurrently
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < recordsPerGoroutine; j++ {
				collector.RecordFailure("key", errors.New("error"))
			}
		}()
	}

	wg.Wait()

	// Verify all were recorded
	assert.Equal(t, goroutines*recordsPerGoroutine, collector.Count())
}

func TestInMemoryErrorCollector_ConcurrentReadWrite(_ *testing.T) {
	collector := NewInMemoryErrorCollector(0)
	const goroutines = 5
	const iterations = 20

	var wg sync.WaitGroup
	wg.Add(goroutines * 2) // Writers + readers

	// Writers
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				collector.RecordFailure("key", errors.New("error"))
			}
		}()
	}

	// Readers
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				_ = collector.GetFailures()
				_ = collector.Count()
				_ = collector.HasFailures()
			}
		}()
	}

	wg.Wait()

	// No assertion needed - test passes if no race condition detected
}

func TestRedactingHandler_WithErrorCollector(t *testing.T) {
	// Create a mock handler
	mockHandler := &mockHandler{records: make([]slog.Record, 0)}
	collector := NewInMemoryErrorCollector(0)

	// Create redacting handler with error collector
	handler := NewRedactingHandler(mockHandler, DefaultConfig(), nil)
	handlerWithCollector := handler.WithErrorCollector(collector)

	// Verify the collector is set
	assert.NotNil(t, handlerWithCollector.errorCollector)
	assert.Same(t, collector, handlerWithCollector.errorCollector)
}

func TestRedactingHandler_ErrorCollectorRecordsLogValuerPanic(t *testing.T) {
	mockHandler := &mockHandler{records: make([]slog.Record, 0)}
	collector := NewInMemoryErrorCollector(0)
	handler := NewRedactingHandler(mockHandler, DefaultConfig(), nil).WithErrorCollector(collector)

	// Create a LogValuer that panics
	panicValuer := &panicLogValuer{panicValue: "test panic"}

	// Create and handle a log record
	record := slog.NewRecord(time.Now(), slog.LevelInfo, "test message", 0)
	record.AddAttrs(slog.Any("panic_attr", panicValuer))

	err := handler.Handle(context.Background(), record)
	require.NoError(t, err)

	// Verify error was collected
	require.True(t, collector.HasFailures())
	failures := collector.GetFailures()
	require.Len(t, failures, 1)

	assert.Equal(t, "panic_attr", failures[0].Key)

	// Verify error is of correct type
	var panicErr *ErrLogValuePanic
	require.ErrorAs(t, failures[0].Err, &panicErr)
	assert.Equal(t, "panic_attr", panicErr.Key)
	assert.Equal(t, "test panic", panicErr.PanicValue)
}

func TestRedactingHandler_ErrorCollectorRecordsSlicePanic(t *testing.T) {
	mockHandler := &mockHandler{records: make([]slog.Record, 0)}
	collector := NewInMemoryErrorCollector(0)
	handler := NewRedactingHandler(mockHandler, DefaultConfig(), nil).WithErrorCollector(collector)

	// Create a slice with a LogValuer that panics
	panicValuer := &panicLogValuer{panicValue: "slice panic"}
	slice := []slog.LogValuer{panicValuer}

	// Create and handle a log record
	record := slog.NewRecord(time.Now(), slog.LevelInfo, "test message", 0)
	record.AddAttrs(slog.Any("slice_attr", slice))

	err := handler.Handle(context.Background(), record)
	require.NoError(t, err)

	// Verify error was collected
	require.True(t, collector.HasFailures())
	failures := collector.GetFailures()
	require.Len(t, failures, 1)

	// The error should be for the slice attribute
	assert.Equal(t, "slice_attr", failures[0].Key)

	// Verify error is of correct type
	var panicErr *ErrLogValuePanic
	require.ErrorAs(t, failures[0].Err, &panicErr)
	assert.Equal(t, "slice_attr[0]", panicErr.Key) // Note: processSlice creates key with index
	assert.Equal(t, "slice panic", panicErr.PanicValue)
}

func TestRedactingHandler_ErrorCollectorNotSetByDefault(t *testing.T) {
	mockHandler := &mockHandler{records: make([]slog.Record, 0)}
	handler := NewRedactingHandler(mockHandler, DefaultConfig(), nil)

	// Verify error collector is nil by default
	assert.Nil(t, handler.errorCollector)

	// Create a LogValuer that panics
	panicValuer := &panicLogValuer{panicValue: "test panic"}

	// Create and handle a log record
	record := slog.NewRecord(time.Now(), slog.LevelInfo, "test message", 0)
	record.AddAttrs(slog.Any("panic_attr", panicValuer))

	// Should not panic even without error collector
	err := handler.Handle(context.Background(), record)
	require.NoError(t, err)
}

func TestRedactingHandler_WithAttrs_PropagatesErrorCollector(t *testing.T) {
	mockHandler := &mockHandler{records: make([]slog.Record, 0)}
	collector := NewInMemoryErrorCollector(0)
	handler := NewRedactingHandler(mockHandler, DefaultConfig(), nil).WithErrorCollector(collector)

	// Call WithAttrs
	newHandler := handler.WithAttrs([]slog.Attr{slog.String("key", "value")})

	// Verify error collector is propagated
	redactingHandler, ok := newHandler.(*RedactingHandler)
	require.True(t, ok)
	assert.Same(t, collector, redactingHandler.errorCollector)
}

func TestRedactingHandler_WithGroup_PropagatesErrorCollector(t *testing.T) {
	mockHandler := &mockHandler{records: make([]slog.Record, 0)}
	collector := NewInMemoryErrorCollector(0)
	handler := NewRedactingHandler(mockHandler, DefaultConfig(), nil).WithErrorCollector(collector)

	// Call WithGroup
	newHandler := handler.WithGroup("group")

	// Verify error collector is propagated
	redactingHandler, ok := newHandler.(*RedactingHandler)
	require.True(t, ok)
	assert.Same(t, collector, redactingHandler.errorCollector)
}

// panicLogValuer is a test helper that panics when LogValue is called
type panicLogValuer struct {
	panicValue any
}

func (p *panicLogValuer) LogValue() slog.Value {
	panic(p.panicValue)
}
