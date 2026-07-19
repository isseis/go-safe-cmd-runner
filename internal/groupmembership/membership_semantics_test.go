//go:build cgo && test

package groupmembership

import (
	"bufio"
	"os"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func shouldSkipSemanticsTest(nsswitchContent string, goos string) (skip bool, reason string) {
	if goos == "darwin" {
		return true, "macOS uses OpenDirectory; cannot guarantee files-only NSS"
	}
	if goos != "linux" {
		return true, "only Linux is supported for semantics equivalence testing"
	}

	if nsswitchContent == "" {
		return false, ""
	}

	for _, db := range []string{"passwd", "group"} {
		sources := nssSources(nsswitchContent, db)
		for _, src := range sources {
			if src != "files" && src != "systemd" {
				return true, "nsswitch.conf source " + src + " for " + db + " is not files/systemd"
			}
		}
	}
	return false, ""
}

func nssSources(content, database string) []string {
	scanner := bufio.NewScanner(strings.NewReader(content))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) < 2 {
			continue
		}
		if strings.TrimSpace(parts[0]) != database {
			continue
		}
		sourceList := strings.TrimSpace(parts[1])
		var sources []string
		for _, src := range strings.Fields(sourceList) {
			if strings.HasPrefix(src, "[") {
				continue
			}
			sources = append(sources, src)
		}
		return sources
	}
	return nil
}

func TestShouldSkipSemanticsTest(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		goos     string
		wantSkip bool
	}{
		{
			name:     "darwin always skips",
			goos:     "darwin",
			content:  "",
			wantSkip: true,
		},
		{
			name:     "nsswitch.conf absent (files assumed)",
			goos:     "linux",
			content:  "",
			wantSkip: false,
		},
		{
			name:     "files only",
			goos:     "linux",
			content:  "passwd: files\n\ngroup: files\n",
			wantSkip: false,
		},
		{
			name:     "files and systemd",
			goos:     "linux",
			content:  "passwd: files systemd\n\ngroup: files systemd\n",
			wantSkip: false,
		},
		{
			name:     "sss source triggers skip",
			goos:     "linux",
			content:  "passwd: files\n\ngroup: files sss\n",
			wantSkip: true,
		},
		{
			name:     "ldap source triggers skip",
			goos:     "linux",
			content:  "passwd: files ldap\n\ngroup: files\n",
			wantSkip: true,
		},
		{
			name:     "db with actions (brackets) stripped",
			goos:     "linux",
			content:  "passwd: files\n\ngroup: files [NOTFOUND=continue]\n",
			wantSkip: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			skip, reason := shouldSkipSemanticsTest(tt.content, tt.goos)
			assert.Equal(t, tt.wantSkip, skip)
			if skip {
				assert.NotEmpty(t, reason)
			} else {
				assert.Empty(t, reason)
			}
		})
	}
}

func TestGetGroupMembers_CGOAndNoCGOSemanticsMatch(t *testing.T) {
	var nsswitchContent string
	if runtime.GOOS != "darwin" {
		data, err := os.ReadFile("/etc/nsswitch.conf")
		if err == nil {
			nsswitchContent = string(data)
		}
	}

	skip, reason := shouldSkipSemanticsTest(nsswitchContent, runtime.GOOS)
	if skip {
		t.Skip(reason)
	}

	file, err := os.Open("/etc/group")
	require.NoError(t, err)
	defer file.Close()

	scanner := bufio.NewScanner(file)
	var gids []uint32
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		entry, err := parseGroupLine(line)
		if err != nil {
			continue
		}
		gids = append(gids, entry.gid)
	}
	require.NoError(t, scanner.Err())

	for _, gid := range gids {
		cgoResult, err := getGroupMembers(gid)
		require.NoError(t, err, "CGO getGroupMembers(%d) failed", gid)

		expected := fileExpectedMembers(t, gid)
		assert.ElementsMatch(t, expected, cgoResult, "GID %d: CGO and file-based semantics differ", gid)
	}
}

func fileExpectedMembers(t *testing.T, gid uint32) []string {
	t.Helper()
	entry, err := findGroupByGID(gid)
	require.NoError(t, err, "failed to find group by GID %d", gid)
	if entry == nil {
		return []string{}
	}

	set := make(map[string]struct{})
	if entry.members != "" {
		for _, m := range strings.Split(entry.members, ",") {
			m = strings.TrimSpace(m)
			if m != "" {
				set[m] = struct{}{}
			}
		}
	}
	primaryUsers, err := findUsersWithPrimaryGID(gid)
	require.NoError(t, err, "failed to find users with primary GID %d", gid)
	for _, u := range primaryUsers {
		set[u] = struct{}{}
	}
	result := make([]string, 0, len(set))
	for m := range set {
		result = append(result, m)
	}
	return result
}
