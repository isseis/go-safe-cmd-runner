package redaction

import (
	"fmt"
	"io"
	"log/slog"
	"sort"
)

// ShutdownReporter reports collected redaction failures on shutdown
type ShutdownReporter struct {
	collector ErrorCollector
	writer    io.Writer
	logger    *slog.Logger
}

// NewShutdownReporter creates a new shutdown reporter
func NewShutdownReporter(collector ErrorCollector, writer io.Writer, logger *slog.Logger) *ShutdownReporter {
	return &ShutdownReporter{
		collector: collector,
		writer:    writer,
		logger:    logger,
	}
}

// Report outputs a summary of redaction failures
// This should be called during application shutdown
func (r *ShutdownReporter) Report() error {
	// Check if collector implements the InMemoryErrorCollector interface
	memCollector, ok := r.collector.(*InMemoryErrorCollector)
	if !ok {
		// Collector doesn't support retrieval, skip reporting
		return nil
	}

	failures := memCollector.GetFailures()
	if len(failures) == 0 {
		// No failures, nothing to report
		return nil
	}

	// Log summary to structured logger
	if r.logger != nil {
		r.logger.Warn("Redaction failures summary",
			"total_failures", len(failures),
			"first_failure_key", failures[0].Key,
			"last_failure_key", failures[len(failures)-1].Key,
		)
	}

	// Write detailed report to writer (stderr)
	if r.writer != nil {
		if err := r.writeReport(failures); err != nil {
			return fmt.Errorf("failed to write redaction report: %w", err)
		}
	}

	return nil
}

// writeReport writes a formatted report to the writer
func (r *ShutdownReporter) writeReport(failures []Failure) error {
	// Group failures by key
	failuresByKey := make(map[string][]Failure)
	for _, f := range failures {
		failuresByKey[f.Key] = append(failuresByKey[f.Key], f)
	}

	// Write header
	if err := r.writeHeader(len(failures), len(failuresByKey)); err != nil {
		return err
	}

	// Sort keys for consistent output
	keys := make([]string, 0, len(failuresByKey))
	for key := range failuresByKey {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	// Write details for each key
	if err := r.writeFailureDetails(keys, failuresByKey); err != nil {
		return err
	}

	// Write footer
	return r.writeFooter()
}

// writeHeader writes the report header
func (r *ShutdownReporter) writeHeader(totalFailures, affectedAttrs int) error {
	lines := []string{
		"\n",
		"REDACTION FAILURES DETECTED:\n",
		fmt.Sprintf("  Total failures: %d\n", totalFailures),
		fmt.Sprintf("  Affected attributes: %d\n", affectedAttrs),
		"\n",
	}
	return r.writeLines(lines)
}

// writeFailureDetails writes details for each failure group
func (r *ShutdownReporter) writeFailureDetails(keys []string, failuresByKey map[string][]Failure) error {
	if _, err := fmt.Fprintf(r.writer, "Details:\n"); err != nil {
		return err
	}

	for _, key := range keys {
		keyFailures := failuresByKey[key]
		if err := r.writeKeyFailures(key, keyFailures); err != nil {
			return err
		}
	}

	return nil
}

// writeKeyFailures writes failures for a single key
func (r *ShutdownReporter) writeKeyFailures(key string, keyFailures []Failure) error {
	if _, err := fmt.Fprintf(r.writer, "\n"); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(r.writer, "  Attribute: %s\n", key); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(r.writer, "  Count: %d\n", len(keyFailures)); err != nil {
		return err
	}

	// Show first error as example
	firstFailure := keyFailures[0]
	if _, err := fmt.Fprintf(r.writer, "  Error: %v\n", firstFailure.Err); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(r.writer, "  First occurrence: %s\n", firstFailure.Timestamp.Format("2006-01-02 15:04:05")); err != nil {
		return err
	}

	// Show last occurrence if different
	if len(keyFailures) > 1 {
		lastFailure := keyFailures[len(keyFailures)-1]
		if _, err := fmt.Fprintf(r.writer, "  Last occurrence: %s\n", lastFailure.Timestamp.Format("2006-01-02 15:04:05")); err != nil {
			return err
		}
	}

	return nil
}

// writeFooter writes the report footer
func (r *ShutdownReporter) writeFooter() error {
	lines := []string{
		"\n",
		"Note: These failures indicate that some LogValuer implementations\n",
		"      are panicking during redaction. Please review the implementations\n",
		"      and ensure they handle edge cases properly.\n",
		"\n",
	}
	return r.writeLines(lines)
}

// writeLines writes multiple lines to the writer
func (r *ShutdownReporter) writeLines(lines []string) error {
	for _, line := range lines {
		if _, err := fmt.Fprint(r.writer, line); err != nil {
			return err
		}
	}
	return nil
}
