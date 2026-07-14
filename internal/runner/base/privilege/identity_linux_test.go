//go:build linux && test

package privilege

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// readSavedIDsFromProcStatus reads the saved-set-uid/gid from /proc/self/status
// for cross-checking against readSavedIDs(). The Uid: and Gid: lines contain
// four tab-separated fields: real, effective, saved, filesystem. The saved-set
// is the third field (index 2).
func readSavedIDsFromProcStatus() (suid, sgid int, err error) {
	f, err := os.Open("/proc/self/status")
	if err != nil {
		return 0, 0, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case strings.HasPrefix(line, "Uid:"):
			fields := strings.Fields(line)
			if len(fields) < 4 {
				return 0, 0, fmt.Errorf("unexpected Uid line format: %s", line)
			}
			// fields[0]=real, fields[1]=effective, fields[2]=saved, fields[3]=filesystem
			n, err := strconv.Atoi(fields[2])
			if err != nil {
				return 0, 0, fmt.Errorf("parse Uid saved (field 2): %w", err)
			}
			suid = n
		case strings.HasPrefix(line, "Gid:"):
			fields := strings.Fields(line)
			if len(fields) < 4 {
				return 0, 0, fmt.Errorf("unexpected Gid line format: %s", line)
			}
			// fields[0]=real, fields[1]=effective, fields[2]=saved, fields[3]=filesystem
			n, err := strconv.Atoi(fields[2])
			if err != nil {
				return 0, 0, fmt.Errorf("parse Gid saved (field 2): %w", err)
			}
			sgid = n
		}
	}
	if err := scanner.Err(); err != nil {
		return 0, 0, err
	}
	return suid, sgid, nil
}

// TestReadSavedIDs_MatchesProcStatus verifies that readSavedIDs() returns the
// same saved-set-uid/gid as /proc/self/status. This is a Linux-specific test
// that exercises the production code path.
func TestReadSavedIDs_MatchesProcStatus(t *testing.T) {
	suid, sgid := readSavedIDs()
	require.NotZero(t, suid, "saved-set-uid should be non-zero on Linux")
	require.NotZero(t, sgid, "saved-set-gid should be non-zero on Linux")

	procSuid, procSgid, err := readSavedIDsFromProcStatus()
	require.NoError(t, err, "should read /proc/self/status")

	assert.Equal(t, procSuid, suid, "saved-set-uid matches /proc/self/status")
	assert.Equal(t, procSgid, sgid, "saved-set-gid matches /proc/self/status")
}
