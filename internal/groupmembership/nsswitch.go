package groupmembership

import (
	"errors"
	"io/fs"
	"log/slog"
	"os"
	"runtime"
	"strings"
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

// completeNSSSources is the allowlist of source names whose enumeration is
// taken to be exhaustive from the configuration alone, on either build. It
// is an allowlist rather than a list of dangerous names so that a source
// neither build has heard of counts against completeness instead of for it.
// "compat" is deliberately absent: it pulls NIS entries in through the "+"
// and "-" lines, and neither build can confirm that those entries are
// enumerated in full.
//
// "systemd" is assumed rather than established: under cgo, nss-systemd
// answers getpwent and getgrent, but systemd-homed users appear dynamically.
var completeNSSSources = map[string]struct{}{
	"files":   {},
	"systemd": {},
}

// nssCompletenessMessage is the warning emitted once per process when this
// build cannot see every member of a group on this host.
const nssCompletenessMessage = "This build cannot enumerate every member of a group on this host"

// nssLineDefect says what the configuration lines for one database amount to.
// Its zero value says nothing was examined, so a database whose lines no
// branch classified never reads as usable.
type nssLineDefect int

const (
	// nssLineUnexamined is the zero value: the lines were not examined.
	nssLineUnexamined nssLineDefect = iota
	// nssLineWellFormed means exactly one line configures the database and
	// its source list could be read as written.
	nssLineWellFormed
	// nssLineMissing means no line configures the database.
	nssLineMissing
	// nssLineNoSources means the line configures the database but names no
	// source to look in.
	nssLineNoSources
	// nssLineUnbalancedBracket means an action token on the line was opened
	// and never closed, so where the source names end cannot be told.
	nssLineUnbalancedBracket
	// nssLineDuplicated means more than one line configures the database.
	nssLineDuplicated
)

// readNsswitchSnapshot reads /etc/nsswitch.conf. It reports the outcome
// through the returned snapshot and never returns an error of its own.
func readNsswitchSnapshot() nsswitchSnapshot {
	return readNsswitchSnapshotFrom(nsswitchPath)
}

// readNsswitchSnapshotFrom reads the nsswitch configuration at path. Only
// a file that does not exist yields nsswitchAbsent; every other failure
// yields nsswitchReadFailed, because a file that exists but cannot be read
// may well name a source whose enumeration cannot be confirmed exhaustive.
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

// classifyNSSCompleteness decides whether this host's user database
// configuration establishes that all members of a group are enumerated,
// given the contents of /etc/nsswitch.conf and the target platform. Both
// builds share this rule. It touches no files.
func classifyNSSCompleteness(snapshot nsswitchSnapshot, goos string) completenessVerdict {
	// No platform other than Linux exposes its user database configuration in
	// a form either build can classify.
	if goos != "linux" {
		return incompleteVerdict(causeUnsupportedPlatform, "goos="+goos)
	}

	switch snapshot.state {
	case nsswitchAbsent:
		// Without cgo this is airtight: no file, no way to name a source
		// other than the local files. Under cgo it is a judgement call,
		// since glibc falls back to a compile-time default not established
		// here. Both are answered "complete" because falling open needs no
		// configuration file AND a working compat or NIS setup AND a
		// group-writable protected file, while anything configuring such a
		// source writes this file -- whereas denying would reject every
		// minimal container image that ships without one.
		return completeVerdict()
	case nsswitchRead:
		return classifyNSSSources(snapshot.content)
	case nsswitchReadFailed:
		if snapshot.err != nil {
			// The error names the path itself, so it stands alone.
			return incompleteVerdict(causeNSSSources, snapshot.err.Error())
		}
		return incompleteVerdict(causeNSSSources, nsswitchPath+" could not be read")
	case nsswitchUnread:
		return incompleteVerdict(causeNSSSources, nsswitchPath+" was not read")
	default:
		// Any state a later change adds denies until it is classified here.
		return incompleteVerdict(causeNSSSources, "unrecognized outcome of reading "+nsswitchPath)
	}
}

// classifyNSSSources judges the passwd and group lines of an nsswitch
// configuration. Both databases must be configured by exactly one line that
// can be read as written and names at least one source, and every source
// they name must be one whose enumeration can be confirmed exhaustive from
// the configuration alone.
func classifyNSSSources(content string) completenessVerdict {
	for _, database := range []string{"passwd", "group"} {
		sources, defect := nssSources(content, database)
		switch defect {
		case nssLineWellFormed:
			for _, source := range sources {
				if _, ok := completeNSSSources[source]; !ok {
					return incompleteVerdict(causeNSSSources, database+": "+source)
				}
			}
		case nssLineMissing:
			return incompleteVerdict(causeNSSSources, database+": no line configures this database")
		case nssLineNoSources:
			return incompleteVerdict(causeNSSSources, database+": the line names no source")
		case nssLineUnbalancedBracket:
			return incompleteVerdict(causeNSSSources, database+": an action token on the line is never closed")
		case nssLineDuplicated:
			return incompleteVerdict(causeNSSSources, database+": more than one line configures this database")
		case nssLineUnexamined:
			return incompleteVerdict(causeNSSSources, database+": the line was not examined")
		default:
			// Any defect a later change adds denies until it is classified.
			return incompleteVerdict(causeNSSSources, database+": the line has a defect this build does not recognize")
		}
	}
	return completeVerdict()
}

// nssSources returns the source names listed for one database in the
// contents of /etc/nsswitch.conf, together with what stands in the way of
// reading them. A trailing "#" comment on the database line is stripped
// before tokenizing, and a bracketed action token -- including one
// containing internal whitespace, such as
// "[NOTFOUND=return UNAVAIL=continue]" -- is removed as a single unit rather
// than split on its interior spaces.
//
// Two shapes are reported as defects rather than repaired. A bracket that is
// never closed would otherwise swallow the rest of the line, hiding whatever
// source names follow it; and a second line for the same database would
// otherwise be decided by guessing which of the two the C library honours.
// Both are answered with a defect so that the caller denies instead.
func nssSources(content, database string) ([]string, nssLineDefect) {
	var (
		sources []string
		defect  = nssLineMissing
	)

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
		if defect != nssLineMissing {
			return nil, nssLineDuplicated
		}

		tokens, balanced := splitNSSTokens(rest)
		if !balanced {
			defect = nssLineUnbalancedBracket
			continue
		}
		for _, token := range tokens {
			// A bracketed token specifies how to act on the preceding
			// source's outcome; it is not itself a place to look.
			if strings.HasPrefix(token, "[") {
				continue
			}
			sources = append(sources, token)
		}
		if len(sources) == 0 {
			defect = nssLineNoSources
			continue
		}
		defect = nssLineWellFormed
	}

	if defect != nssLineWellFormed {
		return nil, defect
	}
	return sources, defect
}

// splitNSSTokens splits a source list on whitespace, keeping each bracketed
// action token whole even when it contains spaces of its own. It reports
// whether every bracket it opened was closed; when it was not, the tokens it
// returns are not what the line says.
func splitNSSTokens(sourceList string) ([]string, bool) {
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

	return tokens, depth == 0
}

// nssCompletenessReporter emits the record that this build cannot enumerate
// every group member on this host. It emits at most one record for its
// lifetime, so a single instance shared by the whole process satisfies
// "once per process". It is the only place that builds the record's message
// and attributes.
type nssCompletenessReporter struct {
	reported bool
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
	if r.reported {
		return
	}
	r.reported = true
	logger.Warn(
		nssCompletenessMessage,
		slog.String("user_database_source", userDatabaseSource),
		slog.String("cause", v.cause.String()),
		slog.String("detail", v.detail),
	)
}

// processNSSCompletenessReporter is the single reporter instance shared by
// the whole process, so that the classification record is emitted at most
// once per process.
var processNSSCompletenessReporter nssCompletenessReporter

// nsswitchVerdictValue is the classification for this process. It is written
// exactly once, by precomputeEnumerationEnvironment during startup while the
// process is still single-threaded, and only read from then on; that is what
// makes it safe to hold without a lock. Until startup writes it, it holds its
// zero value, whose completeness is unstated and which therefore denies.
//
// The classification is settled once and reused for the lifetime of the
// process: a permission decision that changed halfway through a run because
// /etc/nsswitch.conf was edited would be a denial the operator cannot
// reproduce. The cost is that a change to that file goes unobserved until
// the process exits.
var nsswitchVerdictValue completenessVerdict

// nsswitchVerdict returns the classification settled for this process. A
// caller that reaches it before startup settled anything receives the
// unstated zero value, which denies rather than grants.
func nsswitchVerdict() completenessVerdict {
	return nsswitchVerdictValue
}

// precomputeEnumerationEnvironment settles the completeness verdict before
// the first enumeration, so that a build that cannot enumerate every member
// on this host says so at startup rather than at the first group-writable
// file. It both classifies and records. A call made after the verdict is
// settled keeps the first classification, so a mid-run edit of
// /etc/nsswitch.conf cannot change a decision.
func precomputeEnumerationEnvironment() {
	if nsswitchVerdictValue.completeness != completenessUnstated {
		return
	}
	nsswitchVerdictValue = classifyNSSCompleteness(readNsswitchSnapshot(), runtime.GOOS)
	processNSSCompletenessReporter.report(slog.Default(), nsswitchVerdictValue)
}

// PrecomputeEnumerationEnvironment settles the completeness verdict for
// this process, so that a build that cannot enumerate every member on this
// host says so at startup rather than at the first group-writable path.
// It resolves no UID and returns no error; EnsurePermissionCheckUID calls
// it too, so record and verify need no change.
func PrecomputeEnumerationEnvironment() {
	precomputeEnumerationEnvironment()
}
