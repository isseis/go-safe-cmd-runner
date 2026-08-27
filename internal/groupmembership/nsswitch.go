//go:build !cgo || test

package groupmembership

import (
	"errors"
	"io/fs"
	"log/slog"
	"os"
	"strings"
	"sync/atomic"
	"unicode"
)

// nsswitchPath is the configuration file that names, per database, where the
// C library looks users and groups up.
const nsswitchPath = "/etc/nsswitch.conf"

// nsswitchState says what happened when /etc/nsswitch.conf was read.
type nsswitchState int

const (
	// nsswitchUnread is the zero value: the file was not read. It is never
	// classified as complete.
	nsswitchUnread nsswitchState = iota
	// nsswitchAbsent means the file does not exist.
	nsswitchAbsent
	// nsswitchRead means the file was read successfully.
	nsswitchRead
	// nsswitchReadFailed means the file could not be read for any reason
	// other than not existing.
	nsswitchReadFailed
)

// nsswitchSnapshot is what one read of /etc/nsswitch.conf produced.
type nsswitchSnapshot struct {
	state   nsswitchState
	content string
	err     error
}

// completeNSSSources is the allowlist of source names a build reading the
// user and group databases from files alone can enumerate exhaustively. It
// is an allowlist rather than a list of dangerous names so that a source
// this build has never heard of counts against completeness instead of for
// it. "compat" is deliberately absent: it pulls NIS entries in through the
// "+" and "-" lines, which this build cannot resolve.
var completeNSSSources = map[string]struct{}{
	"files":   {},
	"systemd": {},
}

// nssCompletenessMessage is the warning emitted once per process when this
// build cannot see every member of a group on this host.
const nssCompletenessMessage = "This build cannot enumerate every member of a group on this host, so write access to group-writable files will be denied"

// readNsswitchSnapshot reads /etc/nsswitch.conf. It reports the outcome
// through the returned snapshot and never returns an error of its own.
func readNsswitchSnapshot() nsswitchSnapshot {
	return readNsswitchSnapshotFrom(nsswitchPath)
}

// readNsswitchSnapshotFrom reads the nsswitch configuration at path. Only
// a file that does not exist yields nsswitchAbsent; every other failure
// yields nsswitchReadFailed, because a file that exists but cannot be read
// may well name a source this build cannot consult.
func readNsswitchSnapshotFrom(path string) nsswitchSnapshot {
	content, err := os.ReadFile(path) //nolint:gosec // the path is a fixed system configuration file
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nsswitchSnapshot{state: nsswitchAbsent}
		}
		return nsswitchSnapshot{state: nsswitchReadFailed, err: err}
	}
	return nsswitchSnapshot{state: nsswitchRead, content: string(content)}
}

// classifyNSSCompleteness decides whether a build that reads the user and
// group databases from files alone can enumerate all members, given the
// contents of /etc/nsswitch.conf and the target platform. It touches no
// files.
func classifyNSSCompleteness(snapshot nsswitchSnapshot, goos string) completenessVerdict {
	// Only Linux configures its user databases through /etc/nsswitch.conf in
	// a form this build reads, so every other platform is unclassifiable and
	// therefore incomplete.
	if goos != "linux" {
		return incompleteVerdict(causeUnsupportedPlatform, "goos="+goos)
	}

	switch snapshot.state {
	case nsswitchAbsent:
		// With no configuration file there is no way to configure a source
		// other than the local files, so the local files are all there is.
		return completeVerdict()
	case nsswitchRead:
		return classifyNSSSources(snapshot.content)
	case nsswitchReadFailed:
		detail := nsswitchPath + " could not be read"
		if snapshot.err != nil {
			detail += ": " + snapshot.err.Error()
		}
		return incompleteVerdict(causeNSSSources, detail)
	case nsswitchUnread:
		return incompleteVerdict(causeNSSSources, nsswitchPath+" was not read")
	default:
		// Any state a later change adds denies until it is classified here.
		return incompleteVerdict(causeNSSSources, "unrecognized outcome of reading "+nsswitchPath)
	}
}

// classifyNSSSources judges the passwd and group lines of an nsswitch
// configuration. Both databases must name at least one source, and every
// source they name must be one this build can enumerate exhaustively.
func classifyNSSSources(content string) completenessVerdict {
	for _, database := range []string{"passwd", "group"} {
		sources := nssSources(content, database)
		if len(sources) == 0 {
			return incompleteVerdict(causeNSSSources, database+": no source names configured")
		}
		for _, source := range sources {
			if _, ok := completeNSSSources[source]; !ok {
				return incompleteVerdict(causeNSSSources, database+": "+source)
			}
		}
	}
	return completeVerdict()
}

// nssSources returns the source names listed for one database in the
// contents of /etc/nsswitch.conf. A trailing "#" comment on the database
// line is stripped before tokenizing, and a bracketed action token --
// including one containing internal whitespace, such as
// "[NOTFOUND=return UNAVAIL=continue]" -- is removed as a single unit rather
// than split on its interior spaces.
func nssSources(content, database string) []string {
	for line := range strings.SplitSeq(content, "\n") {
		// A "#" starts a comment that runs to the end of the line, whether
		// it opens the line or follows a source list. Bracketed action
		// tokens never contain one, so the first "#" always ends the
		// meaningful part of the line.
		line, _, _ = strings.Cut(line, "#")
		name, rest, found := strings.Cut(line, ":")
		if !found || strings.TrimSpace(name) != database {
			continue
		}

		var sources []string
		for _, token := range splitNSSTokens(rest) {
			// A bracketed token specifies how to act on the preceding
			// source's outcome; it is not itself a place to look.
			if strings.HasPrefix(token, "[") {
				continue
			}
			sources = append(sources, token)
		}
		return sources
	}
	return nil
}

// splitNSSTokens splits a source list on whitespace, keeping each bracketed
// action token whole even when it contains spaces of its own.
func splitNSSTokens(sourceList string) []string {
	var (
		tokens  []string
		current strings.Builder
		depth   int
	)
	flush := func() {
		if current.Len() > 0 {
			tokens = append(tokens, current.String())
			current.Reset()
		}
	}

	for _, r := range sourceList {
		switch {
		case r == '[':
			depth++
			current.WriteRune(r)
		case r == ']':
			if depth > 0 {
				depth--
			}
			current.WriteRune(r)
		case unicode.IsSpace(r) && depth == 0:
			flush()
		default:
			current.WriteRune(r)
		}
	}
	flush()

	return tokens
}

// nssCompletenessReporter emits the record that this build cannot enumerate
// every group member on this host. It emits at most one record for its
// lifetime, so a single instance shared by the whole process satisfies
// "once per process". It is the only place that builds the record's message
// and attributes.
type nssCompletenessReporter struct {
	reported atomic.Bool
}

// report emits the classification record once unless already emitted, and
// emits nothing at all for a classification that covers every member. The
// cause and detail go in as separate attributes: completenessVerdict has no
// exported field, so a handler that walks structs would replace the whole
// value with its redaction placeholder and the diagnosis would be lost.
func (r *nssCompletenessReporter) report(logger *slog.Logger, v completenessVerdict) {
	if v.completeness == completenessComplete {
		return
	}
	if !r.reported.CompareAndSwap(false, true) {
		return
	}
	logger.Warn(
		nssCompletenessMessage,
		slog.String("user_database_source", userDatabaseSource),
		slog.String("cause", v.cause.String()),
		slog.String("detail", v.detail),
	)
}
