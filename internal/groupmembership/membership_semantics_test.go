//go:build cgo

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

// TestGetGroupMembers_CGOAndNoCGOSemanticsMatch compares what this build
// enumerates against what the local files alone hold. The comparison only
// means anything where the files are the whole story, which is exactly what
// the production classification decides -- so it decides here too, rather
// than a copy of the rule kept in the test.
func TestGetGroupMembers_CGOAndNoCGOSemanticsMatch(t *testing.T) {
	verdict := classifyNSSCompleteness(readNsswitchSnapshot(), runtime.GOOS)
	if verdict.completeness != completenessComplete {
		t.Skipf("file-based enumeration is not exhaustive here: %s (%s)", verdict.cause, verdict.detail)
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
		assert.ElementsMatch(t, expected, cgoResult.members, "GID %d: CGO and file-based semantics differ", gid)
	}
}

func fileExpectedMembers(t *testing.T, gid uint32) []string {
	t.Helper()
	entry, _, err := findGroupByGID(groupFileSource(), gid)
	require.NoError(t, err, "failed to find group by GID %d", gid)
	if entry == nil {
		return []string{}
	}

	set := make(map[string]struct{})
	if entry.members != "" {
		for m := range strings.SplitSeq(entry.members, ",") {
			m = strings.TrimSpace(m)
			if m != "" {
				set[m] = struct{}{}
			}
		}
	}
	primaryUsers, _, err := findUsersWithPrimaryGID(passwdFileSource(), gid)
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
