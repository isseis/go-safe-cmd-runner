package groupmembership

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	tu "github.com/isseis/go-safe-cmd-runner/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// nsswitchStateOutOfRange is an nsswitchState value that no constant defines.
// It reaches the default branch of the classification switch, which must deny
// exactly as the zero value does.
const nsswitchStateOutOfRange nsswitchState = 99

// TestClassifyNSSCompleteness covers every environment the classification has
// to tell apart: the platforms, the four read outcomes, and the source lists
// that decide between them.
func TestClassifyNSSCompleteness(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		snapshot  nsswitchSnapshot
		goos      string
		want      enumerationCompleteness
		wantCause incompletenessCause
	}{
		{
			name:      "darwin cannot be classified",
			snapshot:  nsswitchSnapshot{state: nsswitchAbsent},
			goos:      "darwin",
			want:      completenessIncomplete,
			wantCause: causeUnsupportedPlatform,
		},
		{
			name:      "other platforms cannot be classified",
			snapshot:  nsswitchSnapshot{state: nsswitchRead, content: "passwd: files\ngroup: files\n"},
			goos:      "freebsd",
			want:      completenessIncomplete,
			wantCause: causeUnsupportedPlatform,
		},
		{
			name:     "absent file leaves the local files as the only source",
			snapshot: nsswitchSnapshot{state: nsswitchAbsent},
			goos:     "linux",
			want:     completenessComplete,
		},
		{
			name:      "read failure is not absence",
			snapshot:  nsswitchSnapshot{state: nsswitchReadFailed, err: os.ErrPermission},
			goos:      "linux",
			want:      completenessIncomplete,
			wantCause: causeNSSSources,
		},
		{
			name:     "files only",
			snapshot: nsswitchSnapshot{state: nsswitchRead, content: "passwd: files\n\ngroup: files\n"},
			goos:     "linux",
			want:     completenessComplete,
		},
		{
			name:     "files and systemd",
			snapshot: nsswitchSnapshot{state: nsswitchRead, content: "passwd: files systemd\n\ngroup: files systemd\n"},
			goos:     "linux",
			want:     completenessComplete,
		},
		{
			name:      "sss source is not enumerable from files",
			snapshot:  nsswitchSnapshot{state: nsswitchRead, content: "passwd: files\n\ngroup: files sss\n"},
			goos:      "linux",
			want:      completenessIncomplete,
			wantCause: causeNSSSources,
		},
		{
			name:      "ldap source is not enumerable from files",
			snapshot:  nsswitchSnapshot{state: nsswitchRead, content: "passwd: files ldap\n\ngroup: files\n"},
			goos:      "linux",
			want:      completenessIncomplete,
			wantCause: causeNSSSources,
		},
		{
			name:      "compat may pull in NIS entries",
			snapshot:  nsswitchSnapshot{state: nsswitchRead, content: "passwd: compat\n\ngroup: compat\n"},
			goos:      "linux",
			want:      completenessIncomplete,
			wantCause: causeNSSSources,
		},
		{
			name:      "db source is not enumerable from files",
			snapshot:  nsswitchSnapshot{state: nsswitchRead, content: "passwd: files db\n\ngroup: files\n"},
			goos:      "linux",
			want:      completenessIncomplete,
			wantCause: causeNSSSources,
		},
		{
			name:      "nis source is not enumerable from files",
			snapshot:  nsswitchSnapshot{state: nsswitchRead, content: "passwd: files nis\n\ngroup: files\n"},
			goos:      "linux",
			want:      completenessIncomplete,
			wantCause: causeNSSSources,
		},
		{
			name:      "winbind source is not enumerable from files",
			snapshot:  nsswitchSnapshot{state: nsswitchRead, content: "passwd: files\n\ngroup: files winbind\n"},
			goos:      "linux",
			want:      completenessIncomplete,
			wantCause: causeNSSSources,
		},
		{
			name:      "a source name this build has never heard of counts against completeness",
			snapshot:  nsswitchSnapshot{state: nsswitchRead, content: "passwd: files\n\ngroup: files someFutureModule\n"},
			goos:      "linux",
			want:      completenessIncomplete,
			wantCause: causeNSSSources,
		},
		{
			name:      "missing passwd line",
			snapshot:  nsswitchSnapshot{state: nsswitchRead, content: "group: files\n"},
			goos:      "linux",
			want:      completenessIncomplete,
			wantCause: causeNSSSources,
		},
		{
			name:      "missing group line",
			snapshot:  nsswitchSnapshot{state: nsswitchRead, content: "passwd: files\n"},
			goos:      "linux",
			want:      completenessIncomplete,
			wantCause: causeNSSSources,
		},
		{
			name:     "action token beside a source name",
			snapshot: nsswitchSnapshot{state: nsswitchRead, content: "passwd: files\n\ngroup: files [NOTFOUND=continue]\n"},
			goos:     "linux",
			want:     completenessComplete,
		},
		{
			name:      "action token with no source name leaves nothing to look in",
			snapshot:  nsswitchSnapshot{state: nsswitchRead, content: "passwd: files\n\ngroup: [NOTFOUND=return]\n"},
			goos:      "linux",
			want:      completenessIncomplete,
			wantCause: causeNSSSources,
		},
		{
			name:      "a bracket that is never closed hides whatever follows it",
			snapshot:  nsswitchSnapshot{state: nsswitchRead, content: "passwd: files\n\ngroup: files [NOTFOUND=return sss\n"},
			goos:      "linux",
			want:      completenessIncomplete,
			wantCause: causeNSSSources,
		},
		{
			name:      "a second line for the same database is not silently resolved",
			snapshot:  nsswitchSnapshot{state: nsswitchRead, content: "passwd: files\n\ngroup: files\ngroup: sss\n"},
			goos:      "linux",
			want:      completenessIncomplete,
			wantCause: causeNSSSources,
		},
		{
			name:      "a second line naming the same sources is still two lines",
			snapshot:  nsswitchSnapshot{state: nsswitchRead, content: "passwd: files\npasswd: files\n\ngroup: files\n"},
			goos:      "linux",
			want:      completenessIncomplete,
			wantCause: causeNSSSources,
		},
		{
			name:      "the zero value means the file was never read",
			snapshot:  nsswitchSnapshot{state: nsswitchUnread},
			goos:      "linux",
			want:      completenessIncomplete,
			wantCause: causeNSSSources,
		},
		{
			name:      "an unrecognized read outcome denies",
			snapshot:  nsswitchSnapshot{state: nsswitchStateOutOfRange},
			goos:      "linux",
			want:      completenessIncomplete,
			wantCause: causeNSSSources,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			verdict := classifyNSSCompleteness(tt.snapshot, tt.goos)

			assert.Equal(t, tt.want, verdict.completeness)
			assert.Equal(t, tt.wantCause, verdict.cause)
			if tt.want == completenessIncomplete {
				assert.NotEmpty(t, verdict.detail, "an incomplete verdict must say what it was judged on")
			}
		})
	}
}

// TestNSSSources covers the ways a source list carries text that is not a
// source name -- a comment running to the end of the line, a bracketed action
// token whose interior spaces must not split it -- and the two shapes that
// are reported as defects rather than read as written.
func TestNSSSources(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		content    string
		database   string
		want       []string
		wantDefect nssLineDefect
	}{
		{
			name:       "trailing comment is not a source",
			content:    "passwd: files systemd # local users only\n",
			database:   "passwd",
			want:       []string{"files", "systemd"},
			wantDefect: nssLineWellFormed,
		},
		{
			name:       "action token with interior spaces stays whole",
			content:    "group: files [NOTFOUND=return UNAVAIL=continue] systemd\n",
			database:   "group",
			want:       []string{"files", "systemd"},
			wantDefect: nssLineWellFormed,
		},
		{
			name:       "commented-out line is not a database line",
			content:    "# group: files sss\ngroup: files\n",
			database:   "group",
			want:       []string{"files"},
			wantDefect: nssLineWellFormed,
		},
		{
			name:       "database without a line",
			content:    "passwd: files\n",
			database:   "group",
			wantDefect: nssLineMissing,
		},
		{
			name:       "line naming no source at all",
			content:    "group: [NOTFOUND=return]\n",
			database:   "group",
			wantDefect: nssLineNoSources,
		},
		{
			// Splitting this on whitespace would leave the trailing source
			// inside the unterminated action token, hiding it from the
			// allowlist.
			name:       "bracket that is never closed",
			content:    "group: files [NOTFOUND=return sss\n",
			database:   "group",
			wantDefect: nssLineUnbalancedBracket,
		},
		{
			name:       "two lines for one database",
			content:    "group: files\ngroup: sss\n",
			database:   "group",
			wantDefect: nssLineDuplicated,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			sources, defect := nssSources(tt.content, tt.database)

			assert.Equal(t, tt.want, sources)
			assert.Equal(t, tt.wantDefect, defect)
		})
	}
}

// TestReadNsswitchSnapshotFrom separates the three read outcomes. Mistaking a
// file that cannot be read for one that does not exist would turn a host this
// build cannot classify into one it calls complete, so the distinction is
// checked against real files rather than assumed.
func TestReadNsswitchSnapshotFrom(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	t.Run("absent", func(t *testing.T) {
		t.Parallel()

		snapshot := readNsswitchSnapshotFrom(filepath.Join(dir, "does-not-exist"))

		assert.Equal(t, nsswitchAbsent, snapshot.state)
		assert.Empty(t, snapshot.content)
	})

	t.Run("unreadable", func(t *testing.T) {
		t.Parallel()

		if os.Geteuid() == 0 {
			t.Skip("root bypasses the permission bits this case depends on")
		}
		path := filepath.Join(dir, "unreadable.conf")
		require.NoError(t, os.WriteFile(path, []byte("passwd: files\n"), 0o000))

		snapshot := readNsswitchSnapshotFrom(path)

		assert.Equal(t, nsswitchReadFailed, snapshot.state)
		assert.Error(t, snapshot.err)
	})

	t.Run("read", func(t *testing.T) {
		t.Parallel()

		content := "passwd: files systemd\ngroup: files systemd\n"
		path := filepath.Join(dir, "readable.conf")
		require.NoError(t, os.WriteFile(path, []byte(content), 0o600))

		snapshot := readNsswitchSnapshotFrom(path)

		assert.Equal(t, nsswitchRead, snapshot.state)
		assert.Equal(t, content, snapshot.content)
		assert.NoError(t, snapshot.err)
	})
}

// TestNSSCompletenessReporter_Report verifies that the cause and the detail
// reach the record as attributes of their own. Passing the verdict as one
// value would lose both: it has no exported field, and a handler that walks
// structs replaces such a value with its redaction placeholder.
func TestNSSCompletenessReporter_Report(t *testing.T) {
	t.Parallel()

	handler := tu.NewLogRecorder(nil)
	logger := slog.New(handler)

	var reporter nssCompletenessReporter
	reporter.report(logger, incompleteVerdict(causeNSSSources, "group: sss"))

	records := handler.Records()
	require.Len(t, records, 1)
	rec := records[0]

	assert.Equal(t, slog.LevelWarn, rec.Level)
	assert.Equal(t, nssCompletenessMessage, rec.Message)
	assert.Equal(t, map[string]any{
		"user_database_source": userDatabaseSource,
		"cause":                causeNSSSources.String(),
		"detail":               "group: sss",
	}, rec.Attrs)
}

// TestNSSCompletenessReporter_ReportsOnlyOnce verifies that a single reporter
// instance emits at most one record across repeated calls.
func TestNSSCompletenessReporter_ReportsOnlyOnce(t *testing.T) {
	t.Parallel()

	handler := tu.NewLogRecorder(nil)
	logger := slog.New(handler)

	var reporter nssCompletenessReporter
	for range 3 {
		reporter.report(logger, incompleteVerdict(causeNSSSources, "group: sss"))
	}

	assert.Len(t, handler.Records(), 1)
}

// TestNSSCompletenessReporter_SaysNothingWhenComplete verifies that a host
// this build can enumerate exhaustively produces no record at all, and that a
// complete classification does not consume the one record a later incomplete
// one would need.
func TestNSSCompletenessReporter_SaysNothingWhenComplete(t *testing.T) {
	t.Parallel()

	handler := tu.NewLogRecorder(nil)
	logger := slog.New(handler)

	var reporter nssCompletenessReporter
	reporter.report(logger, completeVerdict())
	assert.Empty(t, handler.Records())

	reporter.report(logger, incompleteVerdict(causeNSSSources, "group: sss"))
	assert.Len(t, handler.Records(), 1)
}
