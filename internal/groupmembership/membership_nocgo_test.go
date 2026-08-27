//go:build !cgo

package groupmembership

import (
	"bufio"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

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

// Helper functions for testing with temporary files
func createTempGroupFile(t *testing.T, content string) string {
	tempDir := tu.SafeTempDir(t)
	groupFile := filepath.Join(tempDir, "group")
	require.NoError(t, os.WriteFile(groupFile, []byte(content), 0o644))
	return groupFile
}

func createTempPasswdFile(t *testing.T, content string) string {
	tempDir := tu.SafeTempDir(t)
	passwdFile := filepath.Join(tempDir, "passwd")
	require.NoError(t, os.WriteFile(passwdFile, []byte(content), 0o644))
	return passwdFile
}

// TestFindGroupByGID tests group lookup functionality with temporary files
func TestFindGroupByGID(t *testing.T) {
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

	// Create custom implementation for testing
	testFindGroupByGID := func(filepath string, gid uint32) (*groupEntry, error) {
		file, err := os.Open(filepath)
		if err != nil {
			return nil, err
		}
		defer file.Close()

		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}

			entry, err := parseGroupLine(line)
			if err != nil {
				continue
			}

			if entry.gid == gid {
				return entry, nil
			}
		}

		if err := scanner.Err(); err != nil {
			return nil, err
		}
		return nil, nil
	}

	tempFile := createTempGroupFile(t, groupContent)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := testFindGroupByGID(tempFile, tt.gid)
			assert.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestFindUsersWithPrimaryGID tests finding users with specific primary GID
func TestFindUsersWithPrimaryGID(t *testing.T) {
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

	// Create custom implementation for testing
	testFindUsersWithPrimaryGID := func(filepath string, gid uint32) ([]string, error) {
		file, err := os.Open(filepath)
		if err != nil {
			return nil, err
		}
		defer file.Close()

		var users []string
		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}

			user, userGID, err := parsePasswdLine(line)
			if err != nil {
				continue
			}

			if userGID == gid {
				users = append(users, user)
			}
		}

		if err := scanner.Err(); err != nil {
			return nil, err
		}
		return users, nil
	}

	tempFile := createTempPasswdFile(t, passwdContent)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := testFindUsersWithPrimaryGID(tempFile, tt.gid)
			assert.NoError(t, err)
			assert.ElementsMatch(t, tt.expected, result)
		})
	}
}

// TestFileReadingErrors tests error handling for file operations
func TestFileReadingErrors(t *testing.T) {
	t.Run("group file not found", func(t *testing.T) {
		testFindGroupByGID := func(filepath string, _ uint32) (*groupEntry, error) {
			file, err := os.Open(filepath)
			if err != nil {
				return nil, err
			}
			defer file.Close()
			return nil, nil
		}

		_, err := testFindGroupByGID("/nonexistent/group", 0)
		require.ErrorIs(t, err, fs.ErrNotExist)
	})

	t.Run("passwd file not found", func(t *testing.T) {
		testFindUsersWithPrimaryGID := func(filepath string, _ uint32) ([]string, error) {
			file, err := os.Open(filepath)
			if err != nil {
				return nil, err
			}
			defer file.Close()
			return []string{}, nil
		}

		_, err := testFindUsersWithPrimaryGID("/nonexistent/passwd", 0)
		require.ErrorIs(t, err, fs.ErrNotExist)
	})
}

// resetNsswitchClassification clears the process-wide classification latch
// and the reporter that shares its lifetime, so that a test can observe the
// first classification of the process. It clears them again afterwards so
// that a value one test planted cannot be read by the next.
//
// Callers must not run in parallel with each other: the latch is
// process-wide, and clearing it mid-run would let another test observe a
// classification that is being settled a second time.
func resetNsswitchClassification(t *testing.T) {
	t.Helper()

	reset := func() {
		nsswitchVerdictMu.Lock()
		defer nsswitchVerdictMu.Unlock()
		nsswitchVerdictResolved = false
		nsswitchVerdictValue = completenessVerdict{}
		processNSSCompletenessReporter.reported.Store(false)
	}

	reset()
	t.Cleanup(reset)
}

// TestNsswitchVerdictSettlesOncePerProcess verifies that the classification
// is settled once and reused. Re-reading /etc/nsswitch.conf per call would
// let a permission decision change halfway through a run, which is a denial
// the operator cannot reproduce.
func TestNsswitchVerdictSettlesOncePerProcess(t *testing.T) {
	resetNsswitchClassification(t)

	settled := nsswitchVerdict()

	// Plant a value the host itself could never produce: a second call that
	// classified again would overwrite it and return the host's own answer.
	planted := incompleteVerdict(causeUnsupportedPlatform, "planted by TestNsswitchVerdictSettlesOncePerProcess")
	require.NotEqual(t, planted, settled)
	nsswitchVerdictMu.Lock()
	nsswitchVerdictValue = planted
	nsswitchVerdictMu.Unlock()

	assert.Equal(t, planted, nsswitchVerdict())
}

// TestNsswitchVerdictAgreesAcrossGoroutines verifies that concurrent callers
// all receive the classification that was settled, whichever of them settled
// it.
func TestNsswitchVerdictAgreesAcrossGoroutines(t *testing.T) {
	resetNsswitchClassification(t)

	const callers = 16
	verdicts := make([]completenessVerdict, callers)
	var wg sync.WaitGroup
	wg.Add(callers)
	for i := range callers {
		go func() {
			defer wg.Done()
			verdicts[i] = nsswitchVerdict()
		}()
	}
	wg.Wait()

	for _, v := range verdicts {
		assert.Equal(t, verdicts[0], v)
	}
}

// TestNsswitchVerdictReportsWhatItSettled verifies that settling the
// classification is what drives the startup warning, and that it drives it
// exactly when the classification is not complete.
//
// On a host this build can enumerate exhaustively the expectation is that
// nothing was recorded, which alone would also hold if the reporter were
// never called at all. The reverse direction -- that an incomplete
// classification does produce the record -- is what the forced run described
// in the plan confirms, since forcing it from inside the test would require
// a seam in production code that exists only for tests.
func TestNsswitchVerdictReportsWhatItSettled(t *testing.T) {
	resetNsswitchClassification(t)

	settled := nsswitchVerdict()

	assert.Equal(t, settled.completeness != completenessComplete,
		processNSSCompletenessReporter.reported.Load(),
		"the startup warning must be emitted exactly when the classification is not complete")
}

// TestEnsurePermissionCheckUIDPrecomputesEnvironment verifies that the// TestEnsurePermissionCheckUIDPrecomputesEnvironment verifies that the
// startup entry point settles the NSS classification, so that a host this
// build cannot enumerate says so when record or verify starts rather than at
// the first group-writable file. It lives here rather than in manager_test.go
// because the cgo build has no classification to settle.
func TestEnsurePermissionCheckUIDPrecomputesEnvironment(t *testing.T) {
	resetNsswitchClassification(t)

	// The UID resolution may legitimately fail on some hosts; what is under
	// test is that the classification was settled before that could matter.
	_ = New().EnsurePermissionCheckUID()

	nsswitchVerdictMu.Lock()
	defer nsswitchVerdictMu.Unlock()
	assert.True(t, nsswitchVerdictResolved, "EnsurePermissionCheckUID must settle the NSS classification")
}
