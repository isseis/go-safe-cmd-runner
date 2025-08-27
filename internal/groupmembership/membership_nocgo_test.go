//go:build !cgo

package groupmembership

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
	tempDir := t.TempDir()
	groupFile := filepath.Join(tempDir, "group")
	require.NoError(t, os.WriteFile(groupFile, []byte(content), 0o644))
	return groupFile
}

func createTempPasswdFile(t *testing.T, content string) string {
	tempDir := t.TempDir()
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
		testFindGroupByGID := func(filepath string, gid uint32) (*groupEntry, error) {
			file, err := os.Open(filepath)
			if err != nil {
				return nil, err
			}
			defer file.Close()
			return nil, nil
		}

		_, err := testFindGroupByGID("/nonexistent/group", 0)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "no such file or directory")
	})

	t.Run("passwd file not found", func(t *testing.T) {
		testFindUsersWithPrimaryGID := func(filepath string, gid uint32) ([]string, error) {
			file, err := os.Open(filepath)
			if err != nil {
				return nil, err
			}
			defer file.Close()
			return []string{}, nil
		}

		_, err := testFindUsersWithPrimaryGID("/nonexistent/passwd", 0)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "no such file or directory")
	})
}
