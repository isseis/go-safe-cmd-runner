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

// DefaultPlaceholder is the text substituted for a redacted secret unless
// WithPlaceholder overrides it.
const DefaultPlaceholder = "[REDACTED]"

// Config controls how sensitive information is redacted.
//
// Every field is unexported and there is no way to populate one from outside
// this package except through NewConfig or DefaultConfig, so a Config that
// reaches RedactText has necessarily had its patterns validated. A malformed
// pattern fails open - it compiles to a regex matching nothing and leaves the
// secret in the clear - which is not a mistake worth leaving to convention.
type Config struct {
	// placeholder is the text substituted for a redacted secret.
	placeholder string
	// patterns contains the sensitive patterns to detect.
	patterns *SensitivePatterns
	// keyValuePatterns contains the patterns for key-name-based redaction, each
	// declaring what it redacts
	// e.g. [{"password", PatternKindKeyedValue}, {"Authorization", PatternKindHeaderValue}]
	keyValuePatterns []KeyValuePattern
	// compiled holds one ready-to-run rule per keyValuePatterns entry, built once
	// by NewConfig.
	compiled []compiledPattern
	// valueDetector detects sensitive values based on value format (e.g., AWS keys,
	// GitHub tokens, PEM blocks) independent of key-name context. When nil, value-based
	// detection is skipped.
	valueDetector *ValueDetector
	// validated records that this Config came from NewConfig.
	//
	// NewConfig never hands back an unvalidated Config - it returns nil and an
	// error instead - so the only way to hold one is to have skipped NewConfig
	// altogether. Unexported fields stop a caller populating a literal, but not
	// from leaving a Config at its zero value, and Config is usable by value:
	//
	//	var c redaction.Config                     // c.RedactText redacts nothing
	//	type svc struct{ redactor redaction.Config } // same, if never assigned
	//
	// A *Config left nil would panic and be noticed; these fail open silently,
	// which is why the redaction entry points check this flag and suppress their
	// output rather than pass the secret through.
	validated bool
}

// Placeholder returns the text this Config substitutes for a redacted secret.
func (c *Config) Placeholder() string {
	return c.placeholder
}

// compiledPattern is one key-name rule with everything that does not depend on
// the input text already worked out. Only the regex match itself is left for
// RedactText, which runs over every log line and every captured command output.
type compiledPattern struct {
	// keyedValue selects the rebuild-by-hand path over regex replacement. The
	// keyed-value rule copies the boundary, key, separator and quotes verbatim
	// around the value, which a replacement template cannot express.
	keyedValue bool
	re         *regexp.Regexp
	// replacement is the template handed to Regexp.ReplaceAllString, with the
	// placeholder's "$" already escaped. Empty for the keyed-value rule.
	replacement string
	// placeholder is the raw text, used by the keyed-value rule, which writes it
	// into a strings.Builder rather than through replacement expansion and so
	// must not have its "$" escaped.
	placeholder string
	// valueGroups holds the submatch indices of the keyed-value alternatives, so
	// the name lookups happen once here rather than per call.
	valueGroups []int
}

// compilePattern turns a validated pattern into the rule that runs against text.
//
// Compiling here rather than per call is what lets a bad pattern be an error at
// wiring time. That matters beyond tidiness: RedactText runs inside
// RedactingHandler.Handle, where slog.Default() is that same handler, so a
// failure reported from there would be redacted by the Config that produced it
// and recurse. There is nowhere useful to report a compile failure at run time,
// so it must not happen at run time.
func compilePattern(p KeyValuePattern, placeholder string) (compiledPattern, error) {
	escaped := regexp.QuoteMeta(p.Literal)
	escapedPlaceholder := escapeReplacementDollars(placeholder)

	var expr, replacement string
	keyedValue := false
	switch p.Kind {
	case PatternKindHeaderValue:
		// header + separator + optional auth scheme, then the value to end of line
		expr = `(?i)(` + escaped + `)([ \t]*:[ \t]*)((?:bearer |basic )?)[^\r\n]*`
		replacement = "${1}${2}${3}" + escapedPlaceholder
	case PatternKindNextToken:
		// literal followed by one or more non-whitespace characters
		expr = `(?i)(` + escaped + `)(\S+)`
		replacement = "${1}" + escapedPlaceholder
	case PatternKindKeyedValue:
		expr = buildKeyValueRegex(escaped, keyBoundaryGroup(p.Literal))
		keyedValue = true
	default:
		// Unreachable: NewConfig validates before compiling, and validate rejects
		// an unknown kind. Kept so this switch stays total.
		return compiledPattern{}, fmt.Errorf("%w: %d", ErrPatternKindUnknown, p.Kind)
	}

	re, err := regexp.Compile(expr)
	if err != nil {
		// Also unreachable today: every branch above builds its expression from
		// regexp.QuoteMeta-escaped input. Reported rather than assumed away, since
		// that is the whole point of compiling at construction.
		return compiledPattern{}, fmt.Errorf("compiling pattern %q: %w", p.Literal, err)
	}

	cp := compiledPattern{
		keyedValue:  keyedValue,
		re:          re,
		replacement: replacement,
		placeholder: placeholder,
	}
	if keyedValue {
		cp.valueGroups = keyValueValueGroups(re)
	}
	return cp, nil
}

// apply runs this rule over the text.
func (cp *compiledPattern) apply(text string) string {
	if cp.keyedValue {
		return replaceKeyValueMatches(cp.re, cp.valueGroups, text, cp.placeholder)
	}
	return cp.re.ReplaceAllString(text, cp.replacement)
}

// Option customizes the Config built by NewConfig.
type Option func(*Config)

// WithPlaceholder replaces the text substituted for a redacted secret. The
// value-format detector is built from the same placeholder, so this applies to
// both redaction layers.
func WithPlaceholder(placeholder string) Option {
	return func(c *Config) {
		c.placeholder = placeholder
	}
}

// WithAdditionalKeyValuePatterns appends patterns to the default key-name set.
// This is the supported way to declare that a key name marks a secret in a
// particular deployment; the added patterns are validated along with the
// defaults, and they are classified into boundary groups by the same rules (a
// key added here gets the loose boundary - see commonWordKeys).
func WithAdditionalKeyValuePatterns(patterns ...KeyValuePattern) Option {
	return func(c *Config) {
		c.keyValuePatterns = append(c.keyValuePatterns, patterns...)
	}
}

// NewConfig returns the default redaction configuration as modified by opts,
// with every key-name pattern validated. It is the only way to obtain a usable
// Config from outside this package.
func NewConfig(opts ...Option) (*Config, error) {
	c := &Config{
		placeholder:      DefaultPlaceholder,
		patterns:         DefaultSensitivePatterns(),
		keyValuePatterns: DefaultKeyValuePatterns(),
	}
	for _, opt := range opts {
		opt(c)
	}

	// Built after the options so that WithPlaceholder reaches the value-format
	// layer too.
	c.valueDetector = NewValueDetector(c.placeholder)

	c.compiled = make([]compiledPattern, 0, len(c.keyValuePatterns))
	for _, p := range c.keyValuePatterns {
		if err := p.validate(); err != nil {
			return nil, fmt.Errorf("invalid redaction pattern: %w", err)
		}
		cp, err := compilePattern(p, c.placeholder)
		if err != nil {
			return nil, err
		}
		c.compiled = append(c.compiled, cp)
	}

	c.validated = true
	return c, nil
}

// DefaultConfig returns the default redaction configuration.
//
// It panics rather than returning an error: with no options in play the only
// way to fail is for the defaults themselves to be malformed, which the tests
// catch (TestDefaultKeyValuePatterns_AreValid) and which no caller could
// meaningfully recover from. This mirrors DefaultSensitivePatterns.
func DefaultConfig() *Config {
	c, err := NewConfig()
	if err != nil {
		panic(fmt.Sprintf("failed to create default redaction config: %v", err))
	}
	return c
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
	if !c.validated {
		// Suppress rather than return the input unchanged, which is what a Config
		// that skipped NewConfig would otherwise do (see the field). One boolean
		// per call, not a re-validation: the patterns cannot change after
		// NewConfig has returned.
		return RedactionFailurePlaceholder
	}

	result := text

	// Apply key-name-based redaction
	for i := range c.compiled {
		result = c.compiled[i].apply(result)
	}

	// Apply value-format-based detection (e.g., AWS keys, GitHub tokens, PEM blocks).
	// This runs after key=value redaction so that structured key=value pairs get
	// precise masking first, then bare secrets in the remaining text are caught.
	// When ValueDetector is nil, this step is skipped (backward compatible).
	if c.valueDetector != nil {
		result = c.valueDetector.Mask(result)
	}

	return result
}

// RedactLogAttribute redacts sensitive information from a log attribute
func (c *Config) RedactLogAttribute(attr slog.Attr) slog.Attr {
	key := attr.Key
	value := attr.Value

	if !c.validated {
		// As in RedactText, though here an unbuilt Config would nil-panic on
		// patterns below rather than pass the text through. Fail secure either way.
		return slog.Attr{Key: key, Value: slog.StringValue(RedactionFailurePlaceholder)}
	}

	// Check for sensitive patterns in the key
	if c.patterns.IsSensitiveKey(key) {
		return slog.Attr{Key: key, Value: slog.StringValue(c.placeholder)}
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
		if c.patterns.IsSensitiveValue(strValue) {
			return slog.Attr{Key: key, Value: slog.StringValue(c.placeholder)}
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

// escapeReplacementDollars makes a placeholder safe to embed in a
// Regexp.ReplaceAllString replacement, where "$0"/"$1"/etc. expand to the match
// and its capture groups. Without this, a placeholder configured with "$1"-like
// text would re-inject the matched (secret) text it was meant to hide.
func escapeReplacementDollars(placeholder string) string {
	return strings.ReplaceAll(placeholder, "$", "$$")
}

// boundaryGroup classifies a key by how strict a boundary must precede the key
// name before a separator-based or quoted-value match is allowed. It applies
// only to PatternKindKeyedValue patterns.
//
// Unlike PatternKind, the group is derived from the key rather than declared.
// Which words are frequent in English prose is a property of the language, not
// of the pattern's author, so there is nothing for an author to declare: see
// commonWordKeys.
type boundaryGroup int

const (
	// boundaryGroupSpecific covers keys specific enough that a match at any
	// non-alphanumeric position is more likely to mark a secret than to be prose
	// (e.g. "password", "api_key").
	boundaryGroupSpecific boundaryGroup = iota
	// boundaryGroupCommonWord covers keys that are ordinary English words and so
	// occur in diagnostic prose ("Primary key: id", "unexpected token: '}'").
	// Only identifier-internal characters and quotes count as a boundary, which
	// leaves prose intact while still matching structured data such as
	// "aws_secret_access_key: ...".
	boundaryGroupCommonWord
	// boundaryGroupPrefixed covers keys that already begin with a
	// non-alphanumeric character (e.g. "_TOKEN"). The key carries its own
	// boundary, so no additional one is required.
	boundaryGroupPrefixed
)

// commonWordKeys holds the lower-cased keys classified as
// boundaryGroupCommonWord. It is deliberately not a configuration item:
// whether a word is frequent in English prose is a property of the language,
// not of a deployment. Adding a pattern to KeyValuePatterns is instead read as a
// declaration that the key marks a secret, which is why user-added keys default
// to the loose boundary. Read-only after init.
var commonWordKeys = map[string]struct{}{
	"key":    {},
	"token":  {},
	"secret": {},
}

const (
	// keyValueSeparator matches "=" or ":" with optional surrounding spaces and
	// tabs. Newlines are excluded so that a key at the end of one line never
	// consumes the content of the next line.
	keyValueSeparator = `[ \t]*[:=][ \t]*`
	// keyNameClosingQuote matches the optional quote that closes a quoted key
	// name, as in JSON's "password": "x".
	keyNameClosingQuote = `["']?`
	// looseKeyBoundary matches the start of the text or any non-alphanumeric
	// character.
	looseKeyBoundary = `(?:^|[^A-Za-z0-9])`
	// strictKeyBoundary matches only characters that appear inside an identifier
	// or that quote a key name. A space deliberately does not qualify: allowing
	// it would redact ordinary diagnostics such as "Primary key: id".
	strictKeyBoundary = `["'_.\-]`
	// unquotedValue matches an unquoted value, ending at the first whitespace. It
	// refuses to start on a "{" or "[" so that a nested object or array value -
	// which has no whitespace to stop at on a compact JSON line - does not
	// swallow every sibling field and leave unbalanced brackets behind.
	unquotedValue = `[^\s{\[]\S*`
)

// isASCIIAlphanumeric reports whether c is one of the characters that
// looseKeyBoundary negates.
func isASCIIAlphanumeric(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9'
}

// keyBoundaryGroup derives the boundary group from the key string.
func keyBoundaryGroup(key string) boundaryGroup {
	if key == "" {
		return boundaryGroupSpecific
	}
	// Classify on the first byte against the same set looseKeyBoundary negates,
	// so the classification and the boundary character class cannot disagree. A
	// leading multi-byte rune starts outside that set and is therefore treated as
	// already carrying a boundary.
	if !isASCIIAlphanumeric(key[0]) {
		return boundaryGroupPrefixed
	}
	if _, ok := commonWordKeys[strings.ToLower(key)]; ok {
		return boundaryGroupCommonWord
	}
	return boundaryGroupSpecific
}

// keyBoundaryPattern returns the ready-to-embed regex fragment matching the
// single character that must precede the key name, or "" when the group needs
// no boundary.
func keyBoundaryPattern(group boundaryGroup) string {
	switch group {
	case boundaryGroupCommonWord:
		return strictKeyBoundary
	case boundaryGroupPrefixed:
		return ""
	case boundaryGroupSpecific:
		return looseKeyBoundary
	default:
		return looseKeyBoundary
	}
}

// buildKeyValueRegex builds the alternation used for keys that do not contain
// "=" themselves. The alternatives are ordered double-quoted value ->
// single-quoted value -> separated value -> adjacent "=" value. Go's regexp
// prefers the earliest alternative that matches at a given position, so a
// quoted value is always consumed up to its closing quote instead of being
// truncated at the first space by the adjacent alternative. Only the first
// three alternatives carry a boundary; the adjacent form keeps its existing
// unrestricted behavior.
//
// Each alternative captures its value and nothing else: everything else in the
// match lies before or after the value, so replaceKeyValueMatches can copy it
// verbatim. One group per alternative also keeps the submatch bookkeeping out of
// the regex engine's per-byte work, which matters because RedactText runs over
// whole command outputs.
func buildKeyValueRegex(escapedKey string, group boundaryGroup) string {
	boundary := keyBoundaryPattern(group)

	// quoted builds one quoted-value alternative, taking its boundary as an
	// argument because the two quote kinds differ (see loosenedQuotedBoundary).
	// The opening quote is matched literally rather than captured because RE2 has
	// no backreference to require the closing quote to be of the same kind, so
	// each quote character needs its own alternative. When the closing quote is
	// missing (a truncated log line) the value group runs to the end of the line,
	// redacting everything after the opening quote.
	quoted := func(name, quotedBoundary, quote string) string {
		return quotedBoundary + escapedKey + keyNameClosingQuote + keyValueSeparator +
			quote + `(?P<` + name + `>[^` + quote + `\r\n]*)` + quote + `?`
	}

	doubleQuoted := quoted("dqValue", loosenedQuotedBoundary(group), `"`)
	singleQuoted := quoted("sqValue", boundary, `'`)
	separated := boundary + escapedKey + keyNameClosingQuote + keyValueSeparator + `(?P<sepValue>` + unquotedValue + `)`
	adjacent := escapedKey + `=(?P<adjValue>\S+)`

	return `(?i)(?:` + doubleQuoted + `|` + singleQuoted + `|` + separated + `|` + adjacent + `)`
}

// loosenedQuotedBoundary returns the boundary used by the double-quoted
// alternative: common-word keys get the loose boundary there, every other group
// keeps its own. A double quote right after the separator is a structural signal
// prose does not produce, and the strict boundary would leave half of
// TOKEN="abc def" - the shape an environment dump takes - in the clear. The
// single-quoted alternative is deliberately not loosened, because a
// single-quoted value after a common-word key is exactly the shape of the
// diagnostic "unexpected token: '}'".
func loosenedQuotedBoundary(group boundaryGroup) string {
	if group == boundaryGroupCommonWord {
		return looseKeyBoundary
	}
	return keyBoundaryPattern(group)
}

// keyValueValueGroups returns the value submatch indices of the alternatives of
// the regex built by buildKeyValueRegex. The listing order is for readability
// only; matchedValueSpan does not depend on it.
func keyValueValueGroups(re *regexp.Regexp) []int {
	return []int{
		re.SubexpIndex("dqValue"),
		re.SubexpIndex("sqValue"),
		re.SubexpIndex("sepValue"),
		re.SubexpIndex("adjValue"),
	}
}

// matchedValueSpan returns the byte range of the value of whichever alternative
// participated in the match. Exactly one alternative participates per match, so
// the first value group with a non-negative start identifies it.
func matchedValueSpan(valueGroups []int, m []int) (start, end int, ok bool) {
	for _, g := range valueGroups {
		if g >= 0 && 2*g+1 < len(m) && m[2*g] >= 0 {
			return m[2*g], m[2*g+1], true
		}
	}
	return 0, 0, false
}

// replaceKeyValueMatches rebuilds every match, replacing only the value and
// copying the rest of the match - boundary, key, key quote, separator and value
// quotes - verbatim from the input.
//
// Matches are rebuilt from FindAllStringSubmatchIndex rather than from
// ReplaceAllStringFunc because that callback receives only the matched text:
// re-running the regex on that text alone can select a different alternative
// than the one that matched in context. A match that fell through to the
// adjacent alternative because the preceding character failed the boundary
// would match a quoted or separated alternative once that context is gone.
func replaceKeyValueMatches(re *regexp.Regexp, valueGroups []int, text, placeholder string) string {
	matches := re.FindAllStringSubmatchIndex(text, -1)
	if len(matches) == 0 {
		return text
	}

	var b strings.Builder
	// The placeholder is normally longer than the value it replaces, so reserving
	// only len(text) would force a reallocation on almost every call.
	b.Grow(len(text) + len(matches)*len(placeholder))
	last := 0
	for _, m := range matches {
		valueStart, valueEnd, ok := matchedValueSpan(valueGroups, m)
		if !ok {
			// Defensive: no alternative claimed the match. Leaving last untouched
			// makes the next copy (or the final one after the loop) emit this match
			// verbatim, so skipping it cannot drop or duplicate any input.
			continue
		}
		b.WriteString(text[last:valueStart])
		b.WriteString(placeholder)
		b.WriteString(text[valueEnd:m[1]])
		last = m[1]
	}
	b.WriteString(text[last:])
	return b.String()
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
	if !config.validated {
		// The handler reaches into config.patterns directly, so an unbuilt Config
		// would nil-panic mid-log rather than at wiring time. Refuse it here, where
		// the message can say what to do about it.
		panic("FATAL: redaction Config was not built by NewConfig or DefaultConfig.\n" +
			"A Config assembled any other way holds no patterns and redacts nothing.\n" +
			"Use redaction.NewConfig(...) or redaction.DefaultConfig().")
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
	if r.config.patterns.IsSensitiveKey(key) {
		return slog.Attr{Key: key, Value: slog.StringValue(r.config.placeholder)}
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
		if r.config.patterns.IsSensitiveValue(strValue) {
			return slog.Attr{Key: key, Value: slog.StringValue(r.config.placeholder)}
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

// isRecursivelyRedactableKind reports whether a reflect.Kind holds nested data that
// must be walked for redaction (as opposed to an opaque primitive that can be passed
// through unchanged). This is the single source of truth for that classification,
// shared between processKindAny's top-level dispatch and processSlice's per-element
// handling, so the two lists cannot drift apart (this previously happened: one had
// reflect.Array and the other did not, letting nested arrays skip redaction).
func isRecursivelyRedactableKind(k reflect.Kind) bool {
	switch k {
	case reflect.Slice, reflect.Array, reflect.Map, reflect.Struct, reflect.Ptr, reflect.Interface,
		reflect.Func, reflect.Chan, reflect.UnsafePointer:
		// Func, Chan, and UnsafePointer are not "recursively redactable" in the sense of
		// containing nested data, but they are unsupported/opaque kinds that must be
		// routed through redactLogAttributeWithContext (which fails secure to
		// RedactionFailurePlaceholder via processKindAny) rather than passed through
		// as-is, matching the top-level (processKindAny) and map (processMap) handling.
		return true
	default:
		return false
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

	// 2. Check for the error interface, after LogValuer so a type that implements
	// both is still logged the way it asks to be (see processError for why errors
	// cannot go through the struct walk below).
	if errValue, ok := anyValue.(error); ok {
		return r.processError(key, errValue, ctx)
	}

	// 3. Determine type and dispatch to appropriate handler
	rv := reflect.ValueOf(anyValue)
	switch rv.Kind() {
	case reflect.Slice:
		return r.processSlice(key, anyValue, ctx)
	case reflect.Array:
		return r.processSlice(key, anyValue, ctx)
	case reflect.Map:
		return r.processMap(key, anyValue, ctx)
	case reflect.Struct:
		return r.processStruct(key, anyValue, ctx)
	case reflect.Ptr:
		// Dereference pointer and process recursively
		if !rv.IsNil() {
			// Check recursion depth before dereferencing to prevent infinite recursion
			if ctx.depth >= maxRedactionDepth {
				r.failureLogger.Debug("recursion depth limit reached for pointer - returning placeholder for security", "key", key, "depth", ctx.depth)
				return slog.Attr{Key: key, Value: slog.StringValue(RedactionFailurePlaceholder)}, nil
			}
			dereferenced := rv.Elem().Interface()
			nextCtx := redactionContext{depth: ctx.depth + 1}
			return r.processKindAny(key, slog.AnyValue(dereferenced), nextCtx)
		}
		return slog.Attr{Key: key, Value: value}, nil
	case reflect.Interface:
		// Extract concrete value and process recursively
		if !rv.IsNil() {
			// Check recursion depth before dereferencing to prevent infinite recursion
			if ctx.depth >= maxRedactionDepth {
				r.failureLogger.Debug("recursion depth limit reached for interface - returning placeholder for security", "key", key, "depth", ctx.depth)
				return slog.Attr{Key: key, Value: slog.StringValue(RedactionFailurePlaceholder)}, nil
			}
			concrete := rv.Elem().Interface()
			nextCtx := redactionContext{depth: ctx.depth + 1}
			return r.processKindAny(key, slog.AnyValue(concrete), nextCtx)
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

// processError resolves an error to its Error() string and redacts that string,
// so an error attribute is redacted the same way the equivalent string attribute
// is. An error carries its message behind Error(), not in its fields: neither
// *errorString (errors.New) nor *fmt.wrapError (fmt.Errorf) has an exported
// field, so the struct walk would find nothing to redact and fail secure,
// replacing every such message with RedactionFailurePlaceholder.
func (r *RedactingHandler) processError(key string, errValue error, ctx redactionContext) (attr slog.Attr, err error) {
	// 1. Check recursion depth
	if ctx.depth >= maxRedactionDepth {
		r.failureLogger.Debug(
			"Recursion depth limit reached for error - returning placeholder for security",
			"attribute_key", key,
			"depth", maxRedactionDepth,
		)
		return slog.Attr{Key: key, Value: slog.StringValue(RedactionFailurePlaceholder)}, nil
	}

	// 2. A nil pointer stored in a non-nil error interface is left alone rather
	// than called, matching how processKindAny passes a nil pointer through.
	if rv := reflect.ValueOf(errValue); rv.Kind() == reflect.Ptr && rv.IsNil() {
		return slog.Attr{Key: key, Value: slog.AnyValue(errValue)}, nil
	}

	// 3. Error() is caller-supplied code, so it is called under recovery for the
	// same reason LogValue() is.
	defer func() {
		if rec := recover(); rec != nil {
			r.failureLogger.Warn(
				"Redaction failed for error - detailed log",
				"attribute_key", key,
				"panic_value", rec,
				"panic_type", fmt.Sprintf("%T", rec),
				"stack_trace", string(debug.Stack()),
				"log_category", "redaction_failure_detail",
			)
			// Log safe summary to all destinations
			slog.Warn(
				"Redaction failed for error - see logs for details",
				"attribute_key", key,
				"panic_type", fmt.Sprintf("%T", rec),
				"log_category", "redaction_failure_summary",
				"details_in_log", true,
			)
			// Fail secure: return the placeholder instead of a partial message
			attr = slog.Attr{Key: key, Value: slog.StringValue(RedactionFailurePlaceholder)}
			err = nil
		}
	}()

	// 4. Redact the message through the string path
	message := slog.Attr{Key: key, Value: slog.StringValue(errValue.Error())}
	nextCtx := redactionContext{depth: ctx.depth + 1}
	return r.redactLogAttributeWithContext(message, nextCtx), nil
}

// processMap processes a map value: keys are stringified (via fmt.Sprint, not redacted)
// and used only to check IsSensitiveKey, while values are recursively redacted.
func (r *RedactingHandler) processMap(key string, mapValue any, ctx redactionContext) (attr slog.Attr, err error) {
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
			// Log safe summary to all destinations
			slog.Warn(
				"Redaction failed for map - see logs for details",
				"attribute_key", key,
				"panic_type", fmt.Sprintf("%T", rec),
				"log_category", "redaction_failure_summary",
				"details_in_log", true,
			)
			// Fail secure: return the placeholder instead of zero values
			attr = slog.Attr{Key: key, Value: slog.StringValue(RedactionFailurePlaceholder)}
			err = nil
		}
	}()

	// 3. Use reflection to get map entries
	rv := reflect.ValueOf(mapValue)
	if rv.Kind() != reflect.Map {
		return slog.Attr{Key: key, Value: slog.AnyValue(mapValue)}, nil
	}

	// 4. Collect and sort keys for deterministic output
	type mapEntry struct {
		keyStr string
		keyVal reflect.Value
	}
	mapKeys := rv.MapKeys()
	entries := make([]mapEntry, 0, len(mapKeys))
	for _, k := range mapKeys {
		entries = append(entries, mapEntry{
			keyStr: fmt.Sprint(k.Interface()),
			keyVal: k,
		})
	}
	slices.SortFunc(entries, func(a, b mapEntry) int {
		return strings.Compare(a.keyStr, b.keyStr)
	})

	// 5. Process each entry
	result := make(map[string]any)
	nextCtx := redactionContext{depth: ctx.depth + 1}

	for _, entry := range entries {
		keyStr := entry.keyStr
		mapEntryValue := rv.MapIndex(entry.keyVal).Interface()

		// Check if key is sensitive - if so, mask the value
		if r.config.patterns.IsSensitiveKey(keyStr) {
			result[keyStr] = r.config.placeholder
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
func (r *RedactingHandler) processStruct(key string, structValue any, ctx redactionContext) (attr slog.Attr, err error) {
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
			// Panic occurred during struct processing
			r.failureLogger.Warn(
				"Redaction failed for struct - detailed log",
				"attribute_key", key,
				"panic_value", rec,
				"panic_type", fmt.Sprintf("%T", rec),
				"stack_trace", string(debug.Stack()),
				"log_category", "redaction_failure_detail",
			)
			// Log safe summary to all destinations
			slog.Warn(
				"Redaction failed for struct - see logs for details",
				"attribute_key", key,
				"panic_type", fmt.Sprintf("%T", rec),
				"log_category", "redaction_failure_summary",
				"details_in_log", true,
			)
			// Fail secure: return the placeholder instead of zero values
			attr = slog.Attr{Key: key, Value: slog.StringValue(RedactionFailurePlaceholder)}
			err = nil
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

		// Determine field key name from json tag or field name
		fieldKey := field.Name
		if jsonTag := field.Tag.Get("json"); jsonTag != "" {
			// Parse json tag to handle options like "omitempty", "string"
			if jsonTag == "-" {
				// Skip fields with json:"-" tag (don't count as exported)
				continue
			}
			// Extract field name from tag (before any comma)
			if tagName, _, _ := strings.Cut(jsonTag, ","); tagName != "" {
				fieldKey = tagName
			}
		}

		exportedFieldCount++

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

// processSlice processes slice and array values and recursively redacts all elements.
// Despite the name, this function handles both reflect.Slice and reflect.Array kinds,
// as both support the same reflection API (Len(), Index()) needed for element processing.
//
// Element Processing:
// LogValuer elements are resolved via LogValue() and then redacted. Non-LogValuer
// elements (strings, maps, structs, etc.) are passed through redactLogAttributeWithContext
// for recursive redaction, enabling redaction of nested structures (e.g., []map[string]string).
//
// Type Conversion Behavior:
// This function converts all typed slices and arrays ([]string, [10]int, []MyStruct, etc.)
// to []any in the returned slog.Value. This is necessary because:
//  1. We process each element individually (resolving LogValuer, applying redaction)
//  2. The processed elements are collected into a new slice
//  3. Go does not allow creating []T dynamically without complex reflection
//
// Example:
//
//	Input:  []string{"alice", "bob"}          -> Kind: KindAny, Type: []string
//	Output: []any{"alice", "bob"}             -> Kind: KindAny, Type: []any
//	Input:  [3]string{"x", "y", "z"}          -> Kind: KindAny, Type: [3]string
//	Output: []any{"x", "y", "z"}              -> Kind: KindAny, Type: []any
//
// Implications:
//   - Type assertions like value.Any().([]string) will fail after processing
//   - Use value.Any().([]any) instead to access processed slices/arrays
//   - For logging purposes this is typically transparent as handlers (JSON, text)
//     serialize the slice regardless of element type
//   - This differs from non-slice values which preserve their original types
//
// Rationale:
// Preserving the original type would require reflect.MakeSlice and complex
// type checking for every element, adding significant overhead and complexity.
// Since this is a logging system where handlers serialize to JSON/text anyway,
// the semantic content is what matters, not the Go type. The []any conversion
// maintains all actual values while keeping the implementation simple and efficient.
func (r *RedactingHandler) processSlice(key string, sliceValue any, ctx redactionContext) (attr slog.Attr, err error) {
	// 1. Check recursion depth
	if ctx.depth >= maxRedactionDepth {
		r.failureLogger.Debug(
			"Recursion depth limit reached for slice - returning placeholder for security",
			"attribute_key", key,
			"depth", maxRedactionDepth,
		)
		return slog.Attr{Key: key, Value: slog.StringValue(RedactionFailurePlaceholder)}, nil
	}

	// 2. Wrap in defer/recover for panic safety, matching processMap/processStruct.
	// The per-element LogValuer call below has its own recovery; this guards a panic
	// anywhere else in the function body (reflection dispatch, recursive redaction) so
	// the whole slice fails secure instead of letting the panic escape to the caller.
	defer func() {
		if rec := recover(); rec != nil {
			r.failureLogger.Warn(
				"Redaction failed for slice - detailed log",
				"attribute_key", key,
				"panic_value", rec,
				"panic_type", fmt.Sprintf("%T", rec),
				"stack_trace", string(debug.Stack()),
				"log_category", "redaction_failure_detail",
			)
			// Log safe summary to all destinations
			slog.Warn(
				"Redaction failed for slice - see logs for details",
				"attribute_key", key,
				"panic_type", fmt.Sprintf("%T", rec),
				"log_category", "redaction_failure_summary",
				"details_in_log", true,
			)
			// Fail secure: return the placeholder instead of a partial/zero-value slice
			attr = slog.Attr{Key: key, Value: slog.StringValue(RedactionFailurePlaceholder)}
			err = nil
		}
	}()

	// 3. Use reflection to get slice elements
	rv := reflect.ValueOf(sliceValue)
	if rv.Kind() != reflect.Slice && rv.Kind() != reflect.Array {
		// Not a slice or array (should not happen)
		return slog.Attr{Key: key, Value: slog.AnyValue(sliceValue)}, nil
	}

	// 4. Process each element
	processedElements := make([]any, 0, rv.Len())
	nextCtx := redactionContext{depth: ctx.depth + 1}
	var firstError error

	// Fast-path detection: when the slice/array has a concrete (non-interface) element
	// type whose reflect.Kind is never recursively redactable (e.g. []int, []bool), the
	// per-element reflect.ValueOf(element) + isRecursivelyRedactableKind check below is
	// redundant for every element - the static element type already answers the question.
	// This does NOT skip the LogValuer type assertion or the string fast path above; it
	// only elides the reflection work in the final "non-LogValuer, non-string" branch.
	// Interface-typed slices ([]any, []error, etc.) must still be checked per-element
	// since each element's dynamic type can differ.
	elemKind := rv.Type().Elem().Kind()
	skipPerElementReflectCheck := elemKind != reflect.Interface && !isRecursivelyRedactableKind(elemKind)

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
			if str, ok := element.(string); ok {
				// String element: apply RedactText directly
				redactedStr := r.config.RedactText(str)
				processedElements = append(processedElements, redactedStr)
			} else if skipPerElementReflectCheck {
				// Fast path: static element type is a concrete primitive kind that is
				// never recursively redactable, so no per-element reflection is needed.
				processedElements = append(processedElements, element)
			} else {
				elementValue := reflect.ValueOf(element)
				if isRecursivelyRedactableKind(elementValue.Kind()) {
					// Complex types: process recursively
					elementKey := fmt.Sprintf("%s[%d]", key, i)
					redactedAttr := r.redactLogAttributeWithContext(
						slog.Attr{Key: elementKey, Value: slog.AnyValue(element)},
						nextCtx,
					)
					processedElements = append(processedElements, redactedAttr.Value.Any())
				} else {
					// Primitive types (int, bool, etc.) and nil elements (Invalid kind): keep as-is
					processedElements = append(processedElements, element)
				}
			}
		}
	}

	// 5. Return processed slice (converted to []any for compatibility)
	return slog.Attr{Key: key, Value: slog.AnyValue(processedElements)}, firstError
}
