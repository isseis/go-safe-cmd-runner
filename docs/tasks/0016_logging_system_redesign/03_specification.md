# Detailed Specification for Logging System Redesign

## 1. Overview

This document provides the detailed design and implementation plan for the logging system redesign, based on the architecture defined in `02_architecture.md`.

## 2. File and Module Structure

A new package will be created to house the custom logging components.

- `internal/log/`: New package for logging utilities.
  - `multihandler.go`: Contains the implementation of the `MultiHandler`.
  - `multihandler_test.go`: Unit tests for the `MultiHandler`.

The main application logic will be updated:

- `cmd/runner/main.go`: Will be modified to initialize and configure the new logging system.

## 3. `MultiHandler` Implementation (`internal/log/multihandler.go`)

The `MultiHandler` will implement the `slog.Handler` interface.

```go
package log

import (
	"context"
	"log/slog"
)

// MultiHandler is a slog.Handler that dispatches log records to multiple handlers.
type MultiHandler struct {
	handlers []slog.Handler
}

// NewMultiHandler creates a new MultiHandler that wraps the given handlers.
func NewMultiHandler(handlers ...slog.Handler) *MultiHandler {
	return &MultiHandler{
		handlers: handlers,
	}
}

// Enabled reports whether the handler handles records at the given level.
// The handler is enabled if at least one of its underlying handlers is enabled.
func (h *MultiHandler) Enabled(ctx context.Context, level slog.Level) bool {
	for _, handler := range h.handlers {
		if handler.Enabled(ctx, level) {
			return true
		}
	}
	return false
}

// Handle handles the log record by passing it to all underlying handlers.
func (h *MultiHandler) Handle(ctx context.Context, r slog.Record) error {
	// We don't need to check Enabled here because the slog.Logger front-end
	// already does that. We just pass the record to all handlers.
	// Each underlying handler will do its own Enabled check.
	for _, handler := range h.handlers {
        // Although the top-level logger checks Enabled, each handler might have a different level.
        // So we must check if the specific handler is enabled for the record's level.
		if handler.Enabled(ctx, r.Level) {
			if err := handler.Handle(ctx, r.Clone()); err != nil {
				// In a real-world scenario, you might want to handle this error differently,
				// e.g., log it to a fallback logger. For now, we return the first error.
				return err
			}
		}
	}
	return nil
}

// WithAttrs returns a new MultiHandler whose handlers have the given attributes.
func (h *MultiHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	newHandlers := make([]slog.Handler, len(h.handlers))
	for i, handler := range h.handlers {
		newHandlers[i] = handler.WithAttrs(attrs)
	}
	return &MultiHandler{handlers: newHandlers}
}

// WithGroup returns a new MultiHandler whose handlers have the given group name.
func (h *MultiHandler) WithGroup(name string) slog.Handler {
	newHandlers := make([]slog.Handler, len(h.handlers))
	for i, handler := range h.handlers {
		newHandlers[i] = handler.WithGroup(name)
	}
	return &MultiHandler{handlers: newHandlers}
}
```

## 4. `main.go` Modifications (`cmd/runner/main.go`)

The `main` function will be updated to set up the new logging system.

### 4.1. New Command-Line Flags

Two new flags will be introduced.

```go
// In main function, before flag.Parse()
var (
    // ... existing flags
    logLevel = flag.String("log-level", "info", "Log level (debug, info, warn, error)")
    logFile  = flag.String("log-file", "", "Path to the machine-readable log file (JSON format). If empty, disabled.")
)
```
The default for `log-level` is changed to `"info"`. The existing `logLevel` flag will be repurposed.

### 4.2. Logger Initialization Logic

A new function `setupLogger` will be created and called at the beginning of `run`.

```go
// In cmd/runner/main.go

import (
    // ... other imports
    "os"
    "log/slog"
    "github.com/isseis/go-safe-cmd-runner/internal/log" // New import
)

func run() error {
    // ... flag parsing happens in main, values are passed to run or accessed globally

    // Setup logger at the beginning of the execution
    if err := setupLogger(*logLevel, *logFile); err != nil {
        // Fallback to standard logger if setup fails
        slog.Error("Failed to setup logger", "error", err)
    }

    // ... rest of the run function
}

func setupLogger(level, filePath string) error {
    var handlers []slog.Handler

    // 1. Human-readable summary handler (to stdout)
    // This handler only logs Info level and above.
    textHandler := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
        Level: slog.LevelInfo,
    })
    handlers = append(handlers, textHandler)

    // 2. Machine-readable log handler (to file)
    if filePath != "" {
        logF, err := os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
        if err != nil {
            return fmt.Errorf("failed to open log file %s: %w", filePath, err)
        }
        // Note: The file will be closed by the OS on process exit.
        // For a long-running service, you'd want to manage its lifecycle more carefully.

        var slogLevel slog.Level
        if err := slogLevel.UnmarshalText([]byte(level)); err != nil {
            slogLevel = slog.LevelInfo // Default to info on parse error
            slog.Warn("Invalid log level provided, defaulting to INFO", "provided", level)
        }

        jsonHandler := slog.NewJSONHandler(logF, &slog.HandlerOptions{
            Level: slogLevel,
        })
        handlers = append(handlers, jsonHandler)
    }

    // 3. Create and set the default logger
    multiHandler := log.NewMultiHandler(handlers...)
    logger := slog.New(multiHandler)
    slog.SetDefault(logger)

    slog.Info("Logger initialized", "log-level", level, "log-file", filePath)
    return nil
}
```

### 4.3. Replacing `log` package calls

All existing calls to the standard `log` package (`log.Printf`, `log.Fatalf`, etc.) must be replaced with corresponding `slog` calls.

- `log.Printf(...)` -> `slog.Info(...)` or `slog.Debug(...)`
- `log.Fatalf(...)` -> `slog.Error(...); os.Exit(1)`

**Example Replacement:**
```go
// Before
if err != nil {
    log.Fatalf("Failed to drop privileges: %v", err)
}

// After
if err != nil {
    slog.Error("Failed to drop privileges", "error", err)
    os.Exit(1)
}
```

## 5. Testing Plan

- **Unit Tests for `MultiHandler`**:
  - Test `Enabled()` returns true if any handler is enabled.
  - Test `Handle()` calls `Handle` on all enabled handlers.
  - Test `WithAttrs()` and `WithGroup()` correctly propagate attributes/groups to all handlers.
- **Integration Test in `main_test.go`**:
  - Write a test that executes the `runner` command with different `--log-level` and `--log-file` flags.
  - Verify the contents of the standard output (should be summary).
  - Verify the contents of the generated log file (should be detailed JSON).

This detailed specification provides a clear path for implementation, ensuring all requirements are met.
