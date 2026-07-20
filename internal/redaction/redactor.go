// Package redaction provides shared redaction functionality.
package redaction

import (
	"context"
	"fmt"
	"log/slog"
	"reflect"
	"regexp"
	"runtime/debug"
	"slices"
	"strings"
)

// Config controls how sensitive information is redacted
type Config struct {
	// Placeholder is the placeholder used for redaction (e.g., "[REDACTED]")
	// Unified from LogPlaceholder and TextPlaceholder
	Placeholder string
	// Patterns contains the sensitive patterns to detect
	Patterns *SensitivePatterns
	// KeyValuePatterns contains keys for key=value or header redaction
	// e.g. ["password", "token", "Authorization: "]
	KeyValuePatterns []string
	// ValueDetector detects sensitive values based on value format (e.g., AWS keys,
	// GitHub tokens, PEM blocks) independent of key-name context. When nil, value-based
	// detection is skipped. DefaultConfig sets this to a detector with the same placeholder.
	ValueDetector *ValueDetector
}

// DefaultConfig returns default redaction configuration
func DefaultConfig() *Config {
	placeholder := "[REDACTED]"
	return &Config{
		Placeholder:      placeholder,
		Patterns:         DefaultSensitivePatterns(),
		KeyValuePatterns: DefaultKeyValuePatterns(),
		ValueDetector:    NewValueDetector(placeholder),
	}
}

// redactionContext holds context information for recursive redaction
type redactionContext struct {
	depth int // Current recursion depth
}

// maxRedactionDepth is the maximum depth for recursive redaction
// to prevent infinite recursion and DoS attacks
const maxRedactionDepth = 10

// RedactionFailurePlaceholder is used when redaction itself fails
const RedactionFailurePlaceholder = "[REDACTION FAILED - OUTPUT SUPPRESSED]"

// ErrorCollector collects redaction failures for monitoring and debugging
type ErrorCollector interface {
	// RecordFailure records a redaction failure with the attribute key and error
	RecordFailure(key string, err error)
}

// RedactText removes or redacts potentially sensitive information from text.
// Applies both key-name-based patterns and value-format-based detection.
func (c *Config) RedactText(text string) string {
	if text == "" {
		return text
	}

	result := text

	// Apply key=value pattern redaction
	for _, key := range c.KeyValuePatterns {
		result = c.performKeyValueRedaction(result, key, c.Placeholder)
	}

	// Apply value-format-based detection (e.g., AWS keys, GitHub tokens, PEM blocks).
	// This runs after key=value redaction so that structured key=value pairs get
	// precise masking first, then bare secrets in the remaining text are caught.
	// When ValueDetector is nil, this step is skipped (backward compatible).
	if c.ValueDetector != nil {
		result = c.ValueDetector.Mask(result)
	}

	return result
}

// RedactLogAttribute redacts sensitive information from a log attribute
func (c *Config) RedactLogAttribute(attr slog.Attr) slog.Attr {
	key := attr.Key
	value := attr.Value

	// Check for sensitive patterns in the key
	if c.Patterns.IsSensitiveKey(key) {
		return slog.Attr{Key: key, Value: slog.StringValue(c.Placeholder)}
	}

	// Redact string values that match sensitive patterns
	if value.Kind() == slog.KindString {
		strValue := value.String()
		// First apply text-based redaction for key=value patterns within the string
		redactedText := c.RedactText(strValue)
		if redactedText != strValue {
			return slog.Attr{Key: key, Value: slog.StringValue(redactedText)}
		}
		// Then check if the entire value is sensitive (only if no key=value patterns were found)
		// This prevents strings like "password=secret" from being completely replaced with "[REDACTED]"
		if c.Patterns.IsSensitiveValue(strValue) {
			return slog.Attr{Key: key, Value: slog.StringValue(c.Placeholder)}
		}
	}

	// Handle group values recursively
	if value.Kind() == slog.KindGroup {
		groupAttrs := value.Group()
		redactedGroupAttrs := make([]slog.Attr, 0, len(groupAttrs))
		for _, groupAttr := range groupAttrs {
			redactedGroupAttrs = append(redactedGroupAttrs, c.RedactLogAttribute(groupAttr))
		}
		return slog.Attr{Key: key, Value: slog.GroupValue(redactedGroupAttrs...)}
	}

	return attr
}

// performKeyValueRedaction performs redaction on key=value patterns
func (c *Config) performKeyValueRedaction(text, key, placeholder string) string {
	if strings.Contains(key, ":") {
		// For header-like patterns such as "Authorization:" or "Authorization: "
		return c.performColonPatternRedaction(text, key, placeholder)
	}
	if strings.Contains(key, " ") {
		// For patterns like "Bearer ", "Basic " - replace the token after the space
		return c.performSpacePatternRedaction(text, key, placeholder)
	}
	// For regular key=value patterns
	return c.performKeyValuePatternRedaction(text, key, placeholder)
}

// compileRedactionRegex compiles a regex pattern with fail-secure error handling.
// Returns the compiled regex or nil if compilation fails.
// On failure, logs a warning and returns nil to signal the caller to use RedactionFailurePlaceholder.
func compileRedactionRegex(regexPattern string, contextInfo map[string]string) *regexp.Regexp {
	re, err := regexp.Compile(regexPattern)
	if err != nil {
		// Fail-secure: log warning and signal caller to use safe placeholder
		// This prevents potential sensitive information leakage
		logAttrs := []any{
			"error", err.Error(),
			"output_destination", "stderr, file, audit",
		}
		for k, v := range contextInfo {
			logAttrs = append(logAttrs, k, v)
		}
		slog.Warn("Regex compilation failed - using safe placeholder", logAttrs...)
		return nil
	}
	return re
}

// performSpacePatternRedaction handles patterns like "Bearer ", "Basic "
func (c *Config) performSpacePatternRedaction(text, pattern, placeholder string) string {
	// Escape pattern for regex and create case-insensitive pattern
	// Match: pattern followed by one or more non-whitespace characters
	escapedPattern := regexp.QuoteMeta(pattern)
	regexPattern := `(?i)(` + escapedPattern + `)(\S+)`

	re := compileRedactionRegex(regexPattern, map[string]string{
		"function": "performSpacePatternRedaction",
		"pattern":  pattern,
	})
	if re == nil {
		return RedactionFailurePlaceholder
	}

	// Replace matching tokens with pattern + placeholder
	return re.ReplaceAllStringFunc(text, func(match string) string {
		// Find the original pattern in the match (preserving case)
		patternLen := len(pattern)
		if len(match) < patternLen {
			return match
		}

		originalPattern := match[:patternLen]
		return originalPattern + placeholder
	})
}

// performColonPatternRedaction handles patterns like "Authorization:" or "Authorization: "
// It will redact everything after the pattern up to the end of line (or end of string).
func (c *Config) performColonPatternRedaction(text, pattern, placeholder string) string {
	// Escape pattern for regex and create case-insensitive pattern
	// Match: pattern + optional whitespace + optional auth scheme (Bearer/Basic) + value + line ending
	escapedPattern := regexp.QuoteMeta(pattern)
	regexPattern := `(?i)(` + escapedPattern + `)([ \t]*)((?:bearer |basic )?)[^\r\n]*`

	re := compileRedactionRegex(regexPattern, map[string]string{
		"function": "performColonPatternRedaction",
		"pattern":  pattern,
	})
	if re == nil {
		return RedactionFailurePlaceholder
	}

	// Replace matching headers with pattern + whitespace + scheme + placeholder
	return re.ReplaceAllStringFunc(text, func(match string) string {
		// Extract the original case-preserving parts using submatch
		submatches := re.FindStringSubmatch(match)
		const minSubmatchCount = 4
		if len(submatches) < minSubmatchCount {
			return match
		}

		originalPattern := submatches[1] // Original pattern preserving case
		whitespace := submatches[2]      // Whitespace after pattern
		scheme := submatches[3]          // Auth scheme (Bearer/Basic) if present

		return originalPattern + whitespace + scheme + placeholder
	})
}

// performKeyValuePatternRedaction handles patterns like "key=value"
func (c *Config) performKeyValuePatternRedaction(text, key, placeholder string) string {
	// Escape key for regex and create case-insensitive pattern
	// Match: key + optional equals sign + value (non-whitespace characters)
	escapedKey := regexp.QuoteMeta(key)
	var regexPattern string

	if strings.Contains(key, "=") {
		// Key already contains "=", match it exactly + value
		regexPattern = `(?i)(` + escapedKey + `)(\S+)`
	} else {
		// Key without "=", add it and match value
		regexPattern = `(?i)(` + escapedKey + `)(=)(\S+)`
	}

	re := compileRedactionRegex(regexPattern, map[string]string{
		"function": "performKeyValuePatternRedaction",
		"key":      key,
	})
	if re == nil {
		return RedactionFailurePlaceholder
	}

	// Replace matching key=value pairs with key=placeholder
	return re.ReplaceAllStringFunc(text, func(match string) string {
		submatches := re.FindStringSubmatch(match)
		const minSubmatchCount = 3
		if len(submatches) < minSubmatchCount {
			return match
		}

		if strings.Contains(key, "=") {
			// Key already contains "=" (e.g., "Authorization=")
			originalKey := submatches[1] // Original key preserving case
			return originalKey + placeholder
		}
		// Key without "=" (e.g., "password")
		originalKey := submatches[1] // Original key preserving case
		equals := submatches[2]      // The "=" character
		return originalKey + equals + placeholder
	})
}

// RedactingHandler is a decorator that redacts sensitive information before forwarding to the underlying handler
type RedactingHandler struct {
	handler slog.Handler
	config  *Config
	// failureLogger is used for logging within RedactingHandler to prevent recursive redaction.
	//
	// CRITICAL CONSTRAINT: failureLogger MUST NOT contain RedactingHandler in its handler chain.
	// Violating this constraint can cause circular dependencies during panic recovery:
	//   1. User code panics in LogValue() → RedactingHandler catches it
	//   2. RedactingHandler logs panic details to failureLogger
	//   3. If failureLogger uses RedactingHandler, it tries to redact the panic log
	//   4. This could trigger another redaction → infinite loop or stack overflow
	//
	// This constraint is enforced by NewRedactingHandler which validates the failureLogger
	// and emits a warning to stderr if RedactingHandler is detected in the chain.
	//
	// Recommended configuration (as in internal/runner/bootstrap/logger.go):
	//   - failureLogger: stderr/file handlers only (NO RedactingHandler, NO Slack)
	//   - Main logger: all handlers wrapped with RedactingHandler (includes Slack)
	//
	// Logging strategy:
	// - Use failureLogger for: depth limit warnings, internal state, detailed error information,
	//   panic values, and stack traces (these logs must NOT go through RedactingHandler to
	//   avoid recursion and must NOT go to Slack to prevent sensitive data leakage)
	// - Use slog.Default() for: safe summary messages that should reach all destinations
	//   including Slack (these logs intentionally go through RedactingHandler and must not
	//   contain sensitive data)
	failureLogger *slog.Logger
	// errorCollector optionally collects redaction failures for monitoring and debugging
	errorCollector ErrorCollector
}

// containsRedactingHandler checks if a handler chain contains a RedactingHandler.
// This is used to prevent circular dependencies where failureLogger itself
// uses RedactingHandler, which could cause infinite loops during panic recovery.
//
// The function recursively walks through the handler chain, checking:
// - Direct RedactingHandler instances
// - Handlers that expose their underlying handler via Handler() method
// - Handlers that wrap multiple handlers (like MultiHandler)
//
// Returns true if any RedactingHandler is found in the chain.
func containsRedactingHandler(h slog.Handler) bool {
	if h == nil {
		return false
	}

	// Check if this handler is a RedactingHandler
	if _, ok := h.(*RedactingHandler); ok {
		return true
	}

	// Check if the handler exposes an underlying handler
	type handlerGetter interface {
		Handler() slog.Handler
	}
	if hg, ok := h.(handlerGetter); ok {
		return containsRedactingHandler(hg.Handler())
	}

	// handlerChainProvider is an interface for handlers that wrap multiple other handlers.
	// This is used by containsRedactingHandler to inspect the full handler chain.
	type handlerChainProvider interface {
		Handlers() []slog.Handler
	}

	// Check if the handler is a multi-handler that exposes its children
	if hcp, ok := h.(handlerChainProvider); ok {
		if slices.ContainsFunc(hcp.Handlers(), containsRedactingHandler) {
			return true
		}
	}

	// Cannot determine if there's a RedactingHandler deeper in the chain
	return false
}

// NewRedactingHandler creates a new redacting handler that wraps the given handler.
//
// IMPORTANT: The failureLogger MUST NOT contain a RedactingHandler in its handler chain.
// If failureLogger uses RedactingHandler, it can cause circular dependencies during panic
// recovery in processLogValuer:
//  1. User code panics in LogValue()
//  2. RedactingHandler catches panic and logs to failureLogger
//  3. If failureLogger uses RedactingHandler, it tries to redact the panic log
//  4. This could trigger another redaction → infinite loop or stack overflow
//
// This function validates the failureLogger and logs a warning if a RedactingHandler
// is detected in the chain. The warning is logged to stderr to ensure visibility even
// if the logging system is misconfigured.
func NewRedactingHandler(handler slog.Handler, config *Config, failureLogger *slog.Logger) *RedactingHandler {
	if config == nil {
		config = DefaultConfig()
	}
	if failureLogger == nil {
		// Default to slog.Default() if not provided
		failureLogger = slog.Default()
	}

	// Validate that failureLogger does not contain RedactingHandler
	// Use failureLogger.Handler() to get the handler chain
	if containsRedactingHandler(failureLogger.Handler()) {
		// This is a fatal configuration error that must be caught immediately.
		// Continuing execution with RedactingHandler in failureLogger's chain will
		// cause infinite loops during panic recovery in processLogValuer.
		// We panic here to fail fast and prevent runtime circular dependency bugs.
		panic("FATAL: failureLogger contains RedactingHandler in its handler chain.\n" +
			"This will cause circular dependencies during panic recovery in redaction.\n" +
			"The failureLogger MUST be configured to exclude RedactingHandler.\n" +
			"See internal/redaction/redactor.go RedactingHandler.failureLogger documentation for details.")
	}

	return &RedactingHandler{
		handler:        handler,
		config:         config,
		failureLogger:  failureLogger,
		errorCollector: nil, // No error collector by default
	}
}

// WithErrorCollector returns a new RedactingHandler with the given error collector
func (r *RedactingHandler) WithErrorCollector(collector ErrorCollector) *RedactingHandler {
	return &RedactingHandler{
		handler:        r.handler,
		config:         r.config,
		failureLogger:  r.failureLogger,
		errorCollector: collector,
	}
}

// Enabled reports whether the handler handles records at the given level
func (r *RedactingHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return r.handler.Enabled(ctx, level)
}

// Handler returns the underlying handler
func (r *RedactingHandler) Handler() slog.Handler {
	return r.handler
}

// Handle redacts the log record and forwards it to the underlying handler
func (r *RedactingHandler) Handle(ctx context.Context, record slog.Record) error {
	// Create a new record with redacted message and attributes
	redactedMessage := r.config.RedactText(record.Message)
	newRecord := slog.NewRecord(record.Time, record.Level, redactedMessage, record.PC)

	record.Attrs(func(attr slog.Attr) bool {
		// Use redactLogAttributeWithContext for full redaction support
		redactedAttr := r.redactLogAttributeWithContext(attr, redactionContext{depth: 0})
		newRecord.AddAttrs(redactedAttr)
		return true
	})

	return r.handler.Handle(ctx, newRecord)
}

// WithAttrs returns a new RedactingHandler with the given attributes
func (r *RedactingHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	redactedAttrs := make([]slog.Attr, 0, len(attrs))
	for _, attr := range attrs {
		// Use redactLogAttributeWithContext for full redaction support
		redactedAttrs = append(redactedAttrs, r.redactLogAttributeWithContext(attr, redactionContext{depth: 0}))
	}
	return &RedactingHandler{
		handler:        r.handler.WithAttrs(redactedAttrs),
		config:         r.config,
		failureLogger:  r.failureLogger,
		errorCollector: r.errorCollector,
	}
}

// WithGroup returns a new RedactingHandler with the given group name
func (r *RedactingHandler) WithGroup(name string) slog.Handler {
	return &RedactingHandler{
		handler:        r.handler.WithGroup(name),
		config:         r.config,
		failureLogger:  r.failureLogger,
		errorCollector: r.errorCollector,
	}
}

// redactLogAttributeWithContext is the internal implementation with full redaction support
// This method supports LogValuer and slice processing with recursion depth tracking
func (r *RedactingHandler) redactLogAttributeWithContext(attr slog.Attr, ctx redactionContext) slog.Attr {
	key := attr.Key
	value := attr.Value

	// Check for sensitive patterns in the key
	if r.config.Patterns.IsSensitiveKey(key) {
		return slog.Attr{Key: key, Value: slog.StringValue(r.config.Placeholder)}
	}

	// Process based on value kind
	switch value.Kind() {
	case slog.KindString:
		// Handle string values - apply text-based redaction
		strValue := value.String()
		// First apply text-based redaction for key=value patterns within the string
		redactedText := r.config.RedactText(strValue)
		if redactedText != strValue {
			return slog.Attr{Key: key, Value: slog.StringValue(redactedText)}
		}
		// Then check if the entire value is sensitive (only if no key=value patterns were found)
		// This prevents strings like "password=secret" from being completely replaced with "[REDACTED]"
		if r.config.Patterns.IsSensitiveValue(strValue) {
			return slog.Attr{Key: key, Value: slog.StringValue(r.config.Placeholder)}
		}
		return attr

	case slog.KindGroup:
		// Handle group values recursively
		if ctx.depth >= maxRedactionDepth {
			r.failureLogger.Debug("redaction depth limit reached for group, returning placeholder", "key", key, "depth", ctx.depth)
			return slog.Attr{Key: key, Value: slog.StringValue(RedactionFailurePlaceholder)}
		}
		groupAttrs := value.Group()
		redactedGroupAttrs := make([]slog.Attr, 0, len(groupAttrs))
		nextCtx := redactionContext{depth: ctx.depth + 1}
		for _, groupAttr := range groupAttrs {
			redactedGroupAttrs = append(redactedGroupAttrs, r.redactLogAttributeWithContext(groupAttr, nextCtx))
		}
		return slog.Attr{Key: key, Value: slog.GroupValue(redactedGroupAttrs...)}

	case slog.KindLogValuer:
		// Handle LogValuer with panic recovery
		logValuer, ok := value.Any().(slog.LogValuer)
		if !ok {
			// Should never happen, but handle gracefully
			return attr
		}
		processedAttr, err := r.processLogValuer(key, logValuer, ctx)
		if err != nil {
			// Record error for monitoring
			if r.errorCollector != nil {
				r.errorCollector.RecordFailure(key, err)
			}
			// On error, return safe placeholder
			return slog.Attr{Key: key, Value: slog.StringValue(RedactionFailurePlaceholder)}
		}
		return processedAttr

	case slog.KindAny:
		// NEW: Handle KindAny (LogValuer, slices, etc.)
		processedAttr, err := r.processKindAny(key, value, ctx)
		if err != nil {
			// Record error for monitoring
			if r.errorCollector != nil {
				r.errorCollector.RecordFailure(key, err)
			}
			// On error, return safe placeholder
			return slog.Attr{Key: key, Value: slog.StringValue(RedactionFailurePlaceholder)}
		}
		return processedAttr

	default:
		// Other types: pass through
		return attr
	}
}

// processKindAny processes slog.KindAny values
func (r *RedactingHandler) processKindAny(key string, value slog.Value, ctx redactionContext) (slog.Attr, error) {
	anyValue := value.Any()

	// Nil check
	if anyValue == nil {
		return slog.Attr{Key: key, Value: value}, nil
	}

	// 1. Check for LogValuer interface
	if logValuer, ok := anyValue.(slog.LogValuer); ok {
		return r.processLogValuer(key, logValuer, ctx)
	}

	// 2. Determine type and dispatch to appropriate handler
	rv := reflect.ValueOf(anyValue)
	switch rv.Kind() {
	case reflect.Slice:
		return r.processSlice(key, anyValue, ctx)
	case reflect.Map:
		return r.processMap(key, anyValue, ctx)
	case reflect.Struct:
		return r.processStruct(key, anyValue, ctx)
	case reflect.Ptr:
		// Dereference pointer and process recursively
		if !rv.IsNil() {
			dereferenced := rv.Elem().Interface()
			return r.processKindAny(key, slog.AnyValue(dereferenced), ctx)
		}
		return slog.Attr{Key: key, Value: value}, nil
	case reflect.Interface:
		// Extract concrete value and process recursively
		if !rv.IsNil() {
			concrete := rv.Elem().Interface()
			return r.processKindAny(key, slog.AnyValue(concrete), ctx)
		}
		return slog.Attr{Key: key, Value: value}, nil
	case reflect.Func, reflect.Chan, reflect.UnsafePointer:
		// Unsupported types: fail-secure
		return slog.Attr{Key: key, Value: slog.StringValue(RedactionFailurePlaceholder)}, nil
	default:
		// Primitive types (int, bool, string, etc.) and other basic types: pass through as-is
		return slog.Attr{Key: key, Value: value}, nil
	}
}

// processLogValuer processes a LogValuer value and recursively redacts it
func (r *RedactingHandler) processLogValuer(key string, logValuer slog.LogValuer, ctx redactionContext) (slog.Attr, error) {
	// 1. Check recursion depth
	if ctx.depth >= maxRedactionDepth {
		// Depth limit reached: return placeholder to prevent information leakage
		// Log at Debug level
		r.failureLogger.Debug(
			"Recursion depth limit reached - returning placeholder for security",
			"attribute_key", key,
			"depth", maxRedactionDepth,
			"note", "This is not an error - DoS prevention measure",
		)
		return slog.Attr{Key: key, Value: slog.StringValue(RedactionFailurePlaceholder)}, nil
	}

	// 2. Call LogValue() with panic recovery
	var resolvedValue slog.Value
	var panicOccurred bool
	var panicValue any
	var panicStackTrace string

	func() {
		defer func() {
			if rec := recover(); rec != nil {
				panicOccurred = true
				panicValue = rec
				resolvedValue = slog.StringValue(RedactionFailurePlaceholder)
				panicStackTrace = string(debug.Stack())

				// 1. Log detailed information to file/stderr only (excludes Slack)
				// This uses failureLogger which was configured to exclude Slack handler
				r.failureLogger.Warn(
					"Redaction failed - detailed log",
					"attribute_key", key,
					"panic_value", rec,
					"panic_type", fmt.Sprintf("%T", rec),
					"stack_trace", panicStackTrace,
					"log_category", "redaction_failure_detail",
				)

				// 2. Log safe summary to all destinations (includes Slack)
				// This uses slog.Default() which goes through RedactingHandler
				slog.Warn(
					"Redaction failed - see logs for details",
					"attribute_key", key,
					"panic_type", fmt.Sprintf("%T", rec),
					"log_category", "redaction_failure_summary",
					"details_in_log", true,
				)
			}
		}()
		resolvedValue = logValuer.LogValue()
	}()

	if panicOccurred {
		return slog.Attr{Key: key, Value: resolvedValue}, &ErrLogValuePanic{
			Key:        key,
			PanicValue: panicValue,
			StackTrace: panicStackTrace,
		}
	}

	// 3. Recursively redact the resolved value
	resolvedAttr := slog.Attr{Key: key, Value: resolvedValue}
	nextCtx := redactionContext{depth: ctx.depth + 1}
	return r.redactLogAttributeWithContext(resolvedAttr, nextCtx), nil
}

// processMap processes a map value and recursively redacts keys and values
func (r *RedactingHandler) processMap(key string, mapValue any, ctx redactionContext) (slog.Attr, error) {
	// 1. Check recursion depth
	if ctx.depth >= maxRedactionDepth {
		r.failureLogger.Debug(
			"Recursion depth limit reached for map - returning placeholder for security",
			"attribute_key", key,
			"depth", maxRedactionDepth,
		)
		return slog.Attr{Key: key, Value: slog.StringValue(RedactionFailurePlaceholder)}, nil
	}

	// 2. Wrap in defer/recover for panic safety
	defer func() {
		if rec := recover(); rec != nil {
			// Panic occurred during map processing
			r.failureLogger.Warn(
				"Redaction failed for map - detailed log",
				"attribute_key", key,
				"panic_value", rec,
				"panic_type", fmt.Sprintf("%T", rec),
				"stack_trace", string(debug.Stack()),
				"log_category", "redaction_failure_detail",
			)
		}
	}()

	// 3. Use reflection to get map entries
	rv := reflect.ValueOf(mapValue)
	if rv.Kind() != reflect.Map {
		return slog.Attr{Key: key, Value: slog.AnyValue(mapValue)}, nil
	}

	// 4. Collect and sort keys for deterministic output
	var keys []string
	for _, k := range rv.MapKeys() {
		keys = append(keys, fmt.Sprint(k.Interface()))
	}
	slices.Sort(keys)

	// 5. Process each entry
	result := make(map[string]any)
	nextCtx := redactionContext{depth: ctx.depth + 1}

	for _, keyStr := range keys {
		// Find original key value for map lookup
		var originalKey reflect.Value
		for _, k := range rv.MapKeys() {
			if fmt.Sprint(k.Interface()) == keyStr {
				originalKey = k
				break
			}
		}

		if !originalKey.IsValid() {
			continue
		}

		mapEntryValue := rv.MapIndex(originalKey).Interface()

		// Check if key is sensitive - if so, mask the value
		if r.config.Patterns.IsSensitiveKey(keyStr) {
			result[keyStr] = r.config.Placeholder
		} else {
			// Recursively redact the value
			redactedAttr := r.redactLogAttributeWithContext(
				slog.Attr{Key: keyStr, Value: slog.AnyValue(mapEntryValue)},
				nextCtx,
			)
			result[keyStr] = redactedAttr.Value.Any()
		}
	}

	return slog.Attr{Key: key, Value: slog.AnyValue(result)}, nil
}

// processStruct processes a struct value and recursively redacts its exported fields
func (r *RedactingHandler) processStruct(key string, structValue any, ctx redactionContext) (slog.Attr, error) {
	// 1. Check recursion depth
	if ctx.depth >= maxRedactionDepth {
		r.failureLogger.Debug(
			"Recursion depth limit reached for struct - returning placeholder for security",
			"attribute_key", key,
			"depth", maxRedactionDepth,
		)
		return slog.Attr{Key: key, Value: slog.StringValue(RedactionFailurePlaceholder)}, nil
	}

	// 2. Wrap in defer/recover for panic safety
	defer func() {
		if rec := recover(); rec != nil {
			r.failureLogger.Warn(
				"Redaction failed for struct - detailed log",
				"attribute_key", key,
				"panic_value", rec,
				"panic_type", fmt.Sprintf("%T", rec),
				"stack_trace", string(debug.Stack()),
				"log_category", "redaction_failure_detail",
			)
		}
	}()

	// 3. Get struct type information via reflection
	rv := reflect.ValueOf(structValue)
	if rv.Kind() != reflect.Struct {
		return slog.Attr{Key: key, Value: slog.AnyValue(structValue)}, nil
	}

	// 4. Process exported fields
	result := make(map[string]any)
	nextCtx := redactionContext{depth: ctx.depth + 1}
	exportedFieldCount := 0

	for i := 0; i < rv.NumField(); i++ {
		field := rv.Type().Field(i)
		// Skip unexported fields
		if !field.IsExported() {
			continue
		}

		exportedFieldCount++

		// Determine field key name from json tag or field name
		fieldKey := field.Name
		if jsonTag := field.Tag.Get("json"); jsonTag != "" {
			// Parse json tag to handle options like "omitempty", "string"
			if jsonTag == "-" {
				// Skip fields with json:"-" tag
				continue
			}
			// Extract field name from tag (before any comma)
			if tagName, _, found := strings.Cut(jsonTag, ","); found && tagName != "" {
				fieldKey = tagName
			} else if jsonTag != "" {
				fieldKey = jsonTag
			}
			// If tagName is empty string (e.g., json:",omitempty"), fall back to field name
		}

		fieldValue := rv.Field(i).Interface()

		// Recursively redact the field value
		redactedAttr := r.redactLogAttributeWithContext(
			slog.Attr{Key: fieldKey, Value: slog.AnyValue(fieldValue)},
			nextCtx,
		)
		result[fieldKey] = redactedAttr.Value.Any()
	}

	// 5. If no exported fields, return placeholder (fail-secure)
	if exportedFieldCount == 0 {
		return slog.Attr{Key: key, Value: slog.StringValue(RedactionFailurePlaceholder)}, nil
	}

	return slog.Attr{Key: key, Value: slog.AnyValue(result)}, nil
}

// processSlice processes a slice value and recursively redacts all elements.
//
// Element Processing:
// LogValuer elements are resolved via LogValue() and then redacted. Non-LogValuer
// elements (strings, maps, structs, etc.) are passed through redactLogAttributeWithContext
// for recursive redaction, enabling redaction of nested structures (e.g., []map[string]string).
//
// Type Conversion Behavior:
// This function converts all typed slices ([]string, []int, []MyStruct, etc.)
// to []any in the returned slog.Value. This is necessary because:
//  1. We process each element individually (resolving LogValuer, applying redaction)
//  2. The processed elements are collected into a new slice
//  3. Go does not allow creating []T dynamically without complex reflection
//
// Example:
//
//	Input:  []string{"alice", "bob"}          -> Kind: KindAny, Type: []string
//	Output: []any{"alice", "bob"}             -> Kind: KindAny, Type: []any
//
// Implications:
//   - Type assertions like value.Any().([]string) will fail after processing
//   - Use value.Any().([]any) instead to access processed slices
//   - For logging purposes this is typically transparent as handlers (JSON, text)
//     serialize the slice regardless of element type
//   - This differs from non-slice values which preserve their original types
//
// Rationale:
// Preserving the original slice type would require reflect.MakeSlice and complex
// type checking for every element, adding significant overhead and complexity.
// Since this is a logging system where handlers serialize to JSON/text anyway,
// the semantic content is what matters, not the Go type. The []any conversion
// maintains all actual values while keeping the implementation simple and efficient.
func (r *RedactingHandler) processSlice(key string, sliceValue any, ctx redactionContext) (slog.Attr, error) {
	// 1. Check recursion depth
	if ctx.depth >= maxRedactionDepth {
		r.failureLogger.Debug(
			"Recursion depth limit reached for slice - returning placeholder for security",
			"attribute_key", key,
			"depth", maxRedactionDepth,
		)
		return slog.Attr{Key: key, Value: slog.StringValue(RedactionFailurePlaceholder)}, nil
	}

	// 2. Use reflection to get slice elements
	rv := reflect.ValueOf(sliceValue)
	if rv.Kind() != reflect.Slice {
		// Not a slice (should not happen)
		return slog.Attr{Key: key, Value: slog.AnyValue(sliceValue)}, nil
	}

	// 3. Process each element
	processedElements := make([]any, 0, rv.Len())
	nextCtx := redactionContext{depth: ctx.depth + 1}
	var firstError error

	for i := 0; i < rv.Len(); i++ {
		element := rv.Index(i).Interface()

		// Check if element is LogValuer
		if logValuer, ok := element.(slog.LogValuer); ok {
			// Call LogValue() and redact
			var resolvedValue slog.Value
			var panicOccurred bool
			var panicValue any
			var panicStackTrace string

			func() {
				defer func() {
					if rec := recover(); rec != nil {
						panicOccurred = true
						panicValue = rec
						resolvedValue = slog.StringValue(RedactionFailurePlaceholder)
						panicStackTrace = string(debug.Stack())
						elementKey := fmt.Sprintf("%s[%d]", key, i)

						// 1. Log detailed information to file/stderr only (excludes Slack)
						r.failureLogger.Warn(
							"Redaction failed for slice element - detailed log",
							"attribute_key", elementKey,
							"element_index", i,
							"panic_value", rec,
							"panic_type", fmt.Sprintf("%T", rec),
							"stack_trace", panicStackTrace,
							"log_category", "redaction_failure_detail",
						)

						// 2. Log safe summary to all destinations (includes Slack)
						slog.Warn(
							"Redaction failed for slice element - see logs for details",
							"attribute_key", elementKey,
							"element_index", i,
							"panic_type", fmt.Sprintf("%T", rec),
							"log_category", "redaction_failure_summary",
							"details_in_log", true,
						)
					}
				}()
				resolvedValue = logValuer.LogValue()
			}()

			if !panicOccurred {
				// Redact the resolved value
				elementKey := fmt.Sprintf("%s[%d]", key, i)
				redactedAttr := r.redactLogAttributeWithContext(
					slog.Attr{Key: elementKey, Value: resolvedValue},
					nextCtx,
				)
				processedElements = append(processedElements, redactedAttr.Value.Any())
			} else {
				processedElements = append(processedElements, resolvedValue.Any())
				// Record first error only
				if firstError == nil {
					firstError = &ErrLogValuePanic{
						Key:        fmt.Sprintf("%s[%d]", key, i),
						PanicValue: panicValue,
						StackTrace: panicStackTrace,
					}
				}
			}
		} else {
			// Non-LogValuer element: handle based on type
			elementValue := reflect.ValueOf(element)
			// For string elements, apply RedactText; for other types, process recursively
			if str, ok := element.(string); ok {
				// String element: apply RedactText directly
				redactedStr := r.config.RedactText(str)
				processedElements = append(processedElements, redactedStr)
			} else if elementValue.Kind() == reflect.Ptr || elementValue.Kind() == reflect.Map ||
				elementValue.Kind() == reflect.Struct || elementValue.Kind() == reflect.Slice ||
				elementValue.Kind() == reflect.Interface {
				// Complex types: process recursively
				elementKey := fmt.Sprintf("%s[%d]", key, i)
				redactedAttr := r.redactLogAttributeWithContext(
					slog.Attr{Key: elementKey, Value: slog.AnyValue(element)},
					nextCtx,
				)
				processedElements = append(processedElements, redactedAttr.Value.Any())
			} else {
				// Primitive types (int, bool, etc.): keep as-is
				processedElements = append(processedElements, element)
			}
		}
	}

	// 4. Return processed slice (converted to []any for compatibility)
	return slog.Attr{Key: key, Value: slog.AnyValue(processedElements)}, firstError
}
