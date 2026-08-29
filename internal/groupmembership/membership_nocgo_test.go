//go:build !cgo

package groupmembership

import (
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"testing/iotest"

	tu "github.com/isseis/go-safe-cmd-runner/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestUserDatabaseSource fixes the value of userDatabaseSource so that an
// accidental rewrite is caught. It does not prove which user database this
// build actually consults; that is determined by the build toolchain.
func TestUserDatabaseSource(t *testing.T) {
	assert.Equal(t, "passwd-file", userDatabaseSource)
}

// TestParseGroupLine is specific to the no-CGO implementation
func TestParseGroupLine(t *testing.T) {
	tests := []struct {
		name        string
		line        string
		expected    *groupEntry
		shouldError bool
	}{
		{
			name: "normal group with members",
			line: "adm:x:4:syslog,issei",
			expected: &groupEntry{
				name:    "adm",
				gid:     4,
				members: "syslog,issei",
			},
			shouldError: false,
		},
		{
			name: "group with no members",
			line: "root:x:0:",
			expected: &groupEntry{
				name:    "root",
				gid:     0,
				members: "",
			},
			shouldError: false,
		},
		{
			name:        "invalid line format",
			line:        "invalid:line",
			expected:    nil,
			shouldError: true,
		},
		{
			name:        "invalid GID",
			line:        "group:x:notanumber:members",
			expected:    nil,
			shouldError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parseGroupLine(tt.line)
			if tt.shouldError {
				assert.Error(t, err)
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

// TestParsePasswdLine tests the new passwd line parsing function
func TestParsePasswdLine(t *testing.T) {
	tests := []struct {
		name         string
		line         string
		expectedUser string
		expectedGID  uint32
		shouldError  bool
	}{
		{
			name:         "normal user",
			line:         "root:x:0:0:root:/root:/bin/bash",
			expectedUser: "root",
			expectedGID:  0,
			shouldError:  false,
		},
		{
			name:         "regular user",
			line:         "issei:x:1000:1000:Issei,,,:/home/issei:/bin/bash",
			expectedUser: "issei",
			expectedGID:  1000,
			shouldError:  false,
		},
		{
			name:         "system user",
			line:         "daemon:x:1:1:daemon:/usr/sbin:/usr/sbin/nologin",
			expectedUser: "daemon",
			expectedGID:  1,
			shouldError:  false,
		},
		{
			name:         "invalid line format",
			line:         "invalid:line",
			expectedUser: "",
			expectedGID:  0,
			shouldError:  true,
		},
		{
			name:         "invalid GID",
			line:         "user:x:1000:notanumber:User:/home/user:/bin/bash",
			expectedUser: "",
			expectedGID:  0,
			shouldError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			user, gid, err := parsePasswdLine(tt.line)
			if tt.shouldError {
				assert.Error(t, err)
				assert.Equal(t, "", user)
				assert.Equal(t, uint32(0), gid)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedUser, user)
				assert.Equal(t, tt.expectedGID, gid)
			}
		})
	}
}

// TestScanGroupFile tests group lookup against the production scan
func TestScanGroupFile(t *testing.T) {
	t.Parallel()

	groupContent := `# System groups
root:x:0:
daemon:x:1:
bin:x:2:
sys:x:3:
adm:x:4:syslog,john
tty:x:5:
users:x:100:alice,bob
docker:x:999:john,alice

# Invalid line should be skipped
invalid:line:format
# Comment line
staff:x:1000:
`

	tests := []struct {
		name     string
		gid      uint32
		expected *groupEntry
	}{
		{
			name: "find root group",
			gid:  0,
			expected: &groupEntry{
				name:    "root",
				gid:     0,
				members: "",
			},
		},
		{
			name: "find group with members",
			gid:  4,
			expected: &groupEntry{
				name:    "adm",
				gid:     4,
				members: "syslog,john",
			},
		},
		{
			name: "find users group with multiple members",
			gid:  100,
			expected: &groupEntry{
				name:    "users",
				gid:     100,
				members: "alice,bob",
			},
		},
		{
			name:     "group not found",
			gid:      9999,
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result, _, err := scanGroupFile(strings.NewReader(groupContent), "group", tt.gid)
			assert.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestScanPasswdFile tests finding users with a specific primary GID
// against the production scan
func TestScanPasswdFile(t *testing.T) {
	t.Parallel()

	passwdContent := `# System users
root:x:0:0:root:/root:/bin/bash
daemon:x:1:1:daemon:/usr/sbin:/usr/sbin/nologin
bin:x:2:2:bin:/bin:/usr/sbin/nologin
sys:x:3:3:sys:/dev:/usr/sbin/nologin
john:x:1001:100:John Doe:/home/john:/bin/bash
alice:x:1002:100:Alice Smith:/home/alice:/bin/bash
bob:x:1003:1003:Bob Jones:/home/bob:/bin/bash
charlie:x:1004:999:Charlie Brown:/home/charlie:/bin/bash

# Invalid line should be skipped
invalid:line:format
# Comment line
nobody:x:65534:65534:nobody:/nonexistent:/usr/sbin/nologin
`

	tests := []struct {
		name     string
		gid      uint32
		expected []string
	}{
		{
			name:     "find users with GID 0 (root)",
			gid:      0,
			expected: []string{"root"},
		},
		{
			name:     "find users with GID 100 (multiple users)",
			gid:      100,
			expected: []string{"john", "alice"},
		},
		{
			name:     "find single user with unique GID",
			gid:      1003,
			expected: []string{"bob"},
		},
		{
			name:     "find user in docker group",
			gid:      999,
			expected: []string{"charlie"},
		},
		{
			name:     "no users found for non-existent GID",
			gid:      9999,
			expected: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result, _, err := scanPasswdFile(strings.NewReader(passwdContent), "passwd", tt.gid)
			assert.NoError(t, err)
			assert.ElementsMatch(t, tt.expected, result)
		})
	}
}

// TestFileReadingErrors verifies that a failure part-way through reading the
// user database reaches the caller instead of being reported as a short but
// successful scan. The scans are given a reader that fails after its first
// line, which is the shape a truncated read or an I/O error takes; the thin
// wrappers around them open /etc/group and /etc/passwd, which exist on every
// host these tests run on, so an open failure cannot be induced there.
func TestFileReadingErrors(t *testing.T) {
	t.Parallel()

	readErr := errors.New("injected read failure")

	t.Run("group file read error", func(t *testing.T) {
		t.Parallel()

		entry, malformed, err := scanGroupFile(
			io.MultiReader(strings.NewReader("root:x:0:\nbroken\n"), iotest.ErrReader(readErr)), "group", 100)
		require.ErrorIs(t, err, readErr)
		assert.Nil(t, entry)
		// What the scan managed to record before failing is still returned,
		// so a caller that logs the error can say where it got to.
		assert.Equal(t, 1, malformed.count)
	})

	t.Run("passwd file read error", func(t *testing.T) {
		t.Parallel()

		users, malformed, err := scanPasswdFile(
			io.MultiReader(strings.NewReader("root:x:0:0:root:/root:/bin/bash\nbroken\n"), iotest.ErrReader(readErr)), "passwd", 0)
		require.ErrorIs(t, err, readErr)
		assert.Nil(t, users)
		assert.Equal(t, 1, malformed.count)
	})
}

// TestScanRecordsMalformedLines verifies that the count and the position of
// the first unparsable line reach the caller, so that a denial can name the
// line the operator has to look at.
func TestScanRecordsMalformedLines(t *testing.T) {
	t.Parallel()

	t.Run("group", func(t *testing.T) {
		t.Parallel()

		content := "root:x:0:\nbroken\nusers:x:100:alice\nalso:broken:notanumber:x\n"
		entry, malformed, err := scanGroupFile(strings.NewReader(content), "/etc/group", 100)
		require.NoError(t, err)
		require.NotNil(t, entry)
		assert.Equal(t, "users", entry.name)
		assert.Equal(t, 2, malformed.count)
		assert.Equal(t, "/etc/group:2", malformed.first)
	})

	t.Run("passwd", func(t *testing.T) {
		t.Parallel()

		content := "root:x:0:0:root:/root:/bin/bash\nbroken\njohn:x:1:100:John:/home/john:/bin/sh\n"
		users, malformed, err := scanPasswdFile(strings.NewReader(content), "/etc/passwd", 100)
		require.NoError(t, err)
		assert.Equal(t, []string{"john"}, users)
		assert.Equal(t, 1, malformed.count)
		assert.Equal(t, "/etc/passwd:2", malformed.first)
	})
}

// TestScanGroupFileReadsPastTheMatch verifies that a malformed line placed
// after the entry being looked for is still recorded. Returning at the match
// would make the same file complete for an early GID and incomplete for a
// late one, and would let a malformed line evade detection by sitting below
// the entry.
func TestScanGroupFileReadsPastTheMatch(t *testing.T) {
	t.Parallel()

	content := "users:x:100:alice\nbroken\n"
	entry, malformed, err := scanGroupFile(strings.NewReader(content), "/etc/group", 100)
	require.NoError(t, err)
	require.NotNil(t, entry)
	assert.Equal(t, "users", entry.name, "the matching entry must still be returned")
	assert.Equal(t, 1, malformed.count, "a malformed line after the match must be recorded")
	assert.Equal(t, "/etc/group:2", malformed.first)
}

// TestScanGroupFileKeepsTheFirstMatchingEntry verifies that when the group
// database holds more than one entry with the target GID -- which the format
// permits -- the first one wins, as it does for getgrgid. Reading on past
// the match is what makes this a choice, so the choice needs pinning: taking
// the last entry instead would give the no-cgo build a different member set
// than the cgo build for the same file, and could hand a write to whoever
// appended the later line.
func TestScanGroupFileKeepsTheFirstMatchingEntry(t *testing.T) {
	t.Parallel()

	content := "users:x:100:alice\nusurper:x:100:mallory\n"
	entry, malformed, err := scanGroupFile(strings.NewReader(content), "/etc/group", 100)
	require.NoError(t, err)
	require.NotNil(t, entry)
	assert.Equal(t, "users", entry.name)
	assert.Equal(t, "alice", entry.members)
	assert.Zero(t, malformed.count, "a second entry for the same GID is well formed, not malformed")
}

// TestEnumerateReportsAnUnreadableDatabase verifies that a database that
// cannot be opened fails the enumeration rather than yielding a short member
// set. Both databases are checked: a caller that saw only /etc/group open
// successfully must not be handed a set missing every primary member.
func TestEnumerateReportsAnUnreadableDatabase(t *testing.T) {
	t.Parallel()

	openErr := errors.New("injected open failure")
	failing := dbSource{
		name: "unreadable",
		open: func() (io.ReadCloser, error) { return nil, openErr },
	}
	good := stringSource(testGroupFile, "users:x:100:alice\n")
	goodPasswd := stringSource(testPasswdFile, "bob:x:1:100:Bob:/home/bob:/bin/sh\n")

	tests := []struct {
		name      string
		groupSrc  dbSource
		passwdSrc dbSource
	}{
		{name: "group database unreadable", groupSrc: failing, passwdSrc: goodPasswd},
		{name: "passwd database unreadable", groupSrc: good, passwdSrc: failing},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			enumeration, err := enumerateFromSources(tt.groupSrc, tt.passwdSrc, 100, completeVerdict())
			require.ErrorIs(t, err, openErr)
			// The failed enumeration states nothing, so a caller that
			// ignored the error still could not use it to grant access.
			assert.Equal(t, completenessUnstated, enumeration.verdict.completeness)
			assert.Empty(t, enumeration.members)
		})
	}
}

// TestScanIgnoresBlankAndCommentLines verifies that blank lines and comments
// are not counted as malformed. Counting them would make every stock
// /etc/group incomplete, and every enumeration on every host a denial.
func TestScanIgnoresBlankAndCommentLines(t *testing.T) {
	t.Parallel()

	t.Run("group", func(t *testing.T) {
		t.Parallel()

		content := "# a comment\n\n   \nusers:x:100:alice\n# trailing comment\n"
		entry, malformed, err := scanGroupFile(strings.NewReader(content), "/etc/group", 100)
		require.NoError(t, err)
		require.NotNil(t, entry)
		assert.Zero(t, malformed.count)
		assert.Empty(t, malformed.first)
		assert.Equal(t, completeVerdict(), malformed.verdict(),
			"blank and comment lines must leave the scan complete")
	})

	t.Run("passwd", func(t *testing.T) {
		t.Parallel()

		content := "# a comment\n\n   \njohn:x:1:100:John:/home/john:/bin/sh\n"
		users, malformed, err := scanPasswdFile(strings.NewReader(content), "/etc/passwd", 100)
		require.NoError(t, err)
		assert.Equal(t, []string{"john"}, users)
		assert.Zero(t, malformed.count)
		assert.Empty(t, malformed.first)
		assert.Equal(t, completeVerdict(), malformed.verdict(),
			"blank and comment lines must leave the scan complete")
	})
}

// TestScanCountsNISCompatibilityEntries verifies that the NIS compatibility
// entries the compat source understands are counted as malformed. This build
// cannot follow them to NIS, so a host that uses them really does have
// members these files do not list, and denying is the safe answer.
func TestScanCountsNISCompatibilityEntries(t *testing.T) {
	t.Parallel()

	t.Run("group", func(t *testing.T) {
		t.Parallel()

		content := "users:x:100:alice\n+:::\n+@netgroup\n-baduser\n"
		_, malformed, err := scanGroupFile(strings.NewReader(content), "/etc/group", 100)
		require.NoError(t, err)
		assert.Equal(t, 3, malformed.count)
		assert.Equal(t, "/etc/group:2", malformed.first)
	})

	t.Run("passwd", func(t *testing.T) {
		t.Parallel()

		content := "john:x:1:100:John:/home/john:/bin/sh\n+::::::\n+@netgroup\n-baduser\n"
		_, malformed, err := scanPasswdFile(strings.NewReader(content), "/etc/passwd", 100)
		require.NoError(t, err)
		assert.Equal(t, 3, malformed.count)
		assert.Equal(t, "/etc/passwd:2", malformed.first)
	})
}

// TestScanWarnsAboutMalformedLines verifies that every skipped line keeps
// producing its own warning with the file, the line number, and the parse
// error. The malformedLines record carries only the first position, so this
// per-line record is the only place the rest are listed.
//
// The scans call the package-level slog.Warn, so this test replaces the
// default logger and does not run in parallel.
func TestScanWarnsAboutMalformedLines(t *testing.T) {
	original := slog.Default()
	t.Cleanup(func() { slog.SetDefault(original) })

	handler := tu.NewLogRecorder(nil)
	slog.SetDefault(slog.New(handler))

	_, _, err := scanGroupFile(strings.NewReader("root:x:0:\nbroken\n"), "/etc/group", 0)
	require.NoError(t, err)
	_, _, err = scanPasswdFile(strings.NewReader("root:x:0:0:root:/root:/bin/bash\nbroken\n"), "/etc/passwd", 0)
	require.NoError(t, err)

	records := handler.RecordsAtLevel(slog.LevelWarn)
	require.Len(t, records, 2)

	assert.Equal(t, "/etc/group", records[0].Attrs["file"])
	assert.Equal(t, int64(2), records[0].Attrs["line"])
	assert.NotNil(t, records[0].Attrs["error"])

	assert.Equal(t, "/etc/passwd", records[1].Attrs["file"])
	assert.Equal(t, int64(2), records[1].Attrs["line"])
	assert.NotNil(t, records[1].Attrs["error"])
}

// TestMalformedLinesVerdict verifies the two directions of the record's own
// verdict: no skipped line is complete, and any skipped line is incomplete
// with the malformed-line cause.
func TestMalformedLinesVerdict(t *testing.T) {
	t.Parallel()

	assert.Equal(t, completeVerdict(), malformedLines{}.verdict())

	v := malformedLines{count: 2, first: "/etc/group:7"}.verdict()
	assert.Equal(t, completenessIncomplete, v.completeness)
	assert.Equal(t, causeMalformedLine, v.cause)
	assert.Equal(t, "2 line(s) skipped, first at /etc/group:7", v.detail)
}

// stringSource returns a dbSource that serves the given contents, so that a
// test can drive the enumeration over a user database it chose.
func stringSource(name, content string) dbSource {
	return dbSource{
		name: name,
		open: func() (io.ReadCloser, error) {
			return io.NopCloser(strings.NewReader(content)), nil
		},
	}
}

const (
	testGroupFile  = "/etc/group"
	testPasswdFile = "/etc/passwd"
)

// TestEnumerateCombinesEverySourceOfDoubt verifies that neither source of
// doubt can be overridden by the other: a complete classification does not
// rescue a scan that skipped a line, and a clean scan does not rescue an
// incomplete classification. It drives the real enumeration so that dropping
// either input from the combination fails here.
func TestEnumerateCombinesEverySourceOfDoubt(t *testing.T) {
	t.Parallel()

	const (
		cleanGroup   = "users:x:100:alice\n"
		cleanPasswd  = "bob:x:1:100:Bob:/home/bob:/bin/sh\n"
		brokenGroup  = "users:x:100:alice\nbroken\n"
		brokenPasswd = "bob:x:1:100:Bob:/home/bob:/bin/sh\nbroken\n"
	)

	nssIncomplete := incompleteVerdict(causeNSSSources, "group: sss")

	tests := []struct {
		name       string
		group      string
		passwd     string
		nssVerdict completenessVerdict
		want       completenessVerdict
	}{
		{
			name:       "nothing in doubt",
			group:      cleanGroup,
			passwd:     cleanPasswd,
			nssVerdict: completeVerdict(),
			want:       completeVerdict(),
		},
		{
			name:       "a skipped group line outweighs a complete classification",
			group:      brokenGroup,
			passwd:     cleanPasswd,
			nssVerdict: completeVerdict(),
			want:       incompleteVerdict(causeMalformedLine, "1 line(s) skipped, first at "+testGroupFile+":2"),
		},
		{
			name:       "a skipped passwd line outweighs a complete classification",
			group:      cleanGroup,
			passwd:     brokenPasswd,
			nssVerdict: completeVerdict(),
			want:       incompleteVerdict(causeMalformedLine, "1 line(s) skipped, first at "+testPasswdFile+":2"),
		},
		{
			name:       "a clean scan does not rescue an incomplete classification",
			group:      cleanGroup,
			passwd:     cleanPasswd,
			nssVerdict: nssIncomplete,
			want:       nssIncomplete,
		},
		{
			name:       "the classification cause survives both being incomplete",
			group:      brokenGroup,
			passwd:     brokenPasswd,
			nssVerdict: nssIncomplete,
			// The remediation for the environment covers both, so the
			// cause evaluated first is the one reported.
			want: nssIncomplete,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			enumeration, err := enumerateFromSources(
				stringSource(testGroupFile, tt.group),
				stringSource(testPasswdFile, tt.passwd),
				100, tt.nssVerdict)
			require.NoError(t, err)
			assert.Equal(t, tt.want, enumeration.verdict)
			// The members are returned either way: an incomplete verdict
			// denies write access, it does not empty the set.
			assert.ElementsMatch(t, []string{"alice", "bob"}, enumeration.members)
		})
	}
}

// TestEnumerateMissingGroupStatesTheEnvironmentVerdict verifies that a group
// absent from the database is an enumeration of zero members that still
// carries the environment's verdict, rather than a complete one.
//
// The member set stays empty even though a user names the GID as their
// primary group: the cgo build reports the same empty set for an unknown
// group, and the two builds must not disagree about members.
func TestEnumerateMissingGroupStatesTheEnvironmentVerdict(t *testing.T) {
	t.Parallel()

	nss := incompleteVerdict(causeNSSSources, "group: sss")
	enumeration, err := enumerateFromSources(
		stringSource(testGroupFile, "root:x:0:\n"),
		stringSource(testPasswdFile, "ghost:x:1:9999:Ghost:/home/ghost:/bin/sh\n"),
		9999, nss)
	require.NoError(t, err)

	assert.Empty(t, enumeration.members)
	assert.Equal(t, nss, enumeration.verdict)
}

// TestEnumerateMissingGroupStillReadsThePasswdDatabase verifies that an
// absent group does not shorten the scan. Completeness describes the files
// the enumeration read, so a skipped line in the passwd database must reach
// the verdict whether or not the group itself was found; otherwise the same
// host would call one GID complete and another incomplete.
func TestEnumerateMissingGroupStillReadsThePasswdDatabase(t *testing.T) {
	t.Parallel()

	enumeration, err := enumerateFromSources(
		stringSource(testGroupFile, "root:x:0:\n"),
		stringSource(testPasswdFile, "root:x:0:0:root:/root:/bin/sh\nbroken\n"),
		9999, completeVerdict())
	require.NoError(t, err)

	assert.Empty(t, enumeration.members)
	assert.Equal(t,
		incompleteVerdict(causeMalformedLine, "1 line(s) skipped, first at "+testPasswdFile+":2"),
		enumeration.verdict)
}
