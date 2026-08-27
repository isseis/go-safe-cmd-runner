//go:build !cgo || test

package groupmembership

import (
	"bufio"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strconv"
	"strings"
)

// Paths of the user database files the file-based enumeration parses.
const (
	groupFilePath  = "/etc/group"
	passwdFilePath = "/etc/passwd"
)

// dbSource is one user database file: its name, and how to open it. The
// enumeration takes its inputs this way rather than as fixed paths so that
// the scans and the combination of every source of doubt can be driven from
// chosen contents; a host's own /etc/group is well formed, so a malformed
// line could otherwise never be exercised through the enumeration itself.
type dbSource struct {
	name string
	open func() (io.ReadCloser, error)
}

// fileSource returns the source that reads the file at path.
func fileSource(path string) dbSource {
	return dbSource{
		name: path,
		//nolint:gosec // G304: the only call sites pass this package's two constant user-database paths
		open: func() (io.ReadCloser, error) { return os.Open(path) },
	}
}

// groupFileSource returns the group database this build reads.
func groupFileSource() dbSource { return fileSource(groupFilePath) }

// passwdFileSource returns the passwd database this build reads.
func passwdFileSource() dbSource { return fileSource(passwdFilePath) }

// groupEntry represents a parsed line from /etc/group
type groupEntry struct {
	name    string
	gid     uint32
	members string
}

// malformedLines records the lines a scan skipped as unparsable. Only the
// position of the first one is kept: an error message needs to point the
// operator at the line to fix first, while the full list is what the
// per-line slog.Warn records already carry.
type malformedLines struct {
	count int
	first string // "file:line" of the first skipped line; empty when count is 0
}

// record notes one skipped line at the given position.
func (m *malformedLines) record(source string, lineNum int) {
	m.count++
	if m.first == "" {
		m.first = fmt.Sprintf("%s:%d", source, lineNum)
	}
}

// maxDBLineLen bounds one user-database line. A group with many members can
// exceed bufio.Scanner's default 64KB limit, which would abort the scan with
// bufio.ErrTooLong and hide members rather than report them.
const maxDBLineLen = 1024 * 1024

// newDBScanner returns a scanner over r that tolerates such long lines.
func newDBScanner(r io.Reader) *bufio.Scanner {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), maxDBLineLen) //nolint:mnd
	return scanner
}

// scanGroupFile searches r, whose contents are in /etc/group format, for the
// entry with the given GID. source names r in log records and in the
// recorded position of skipped lines. It reads r to the end so that the
// skipped-line record does not depend on where the entry appears; stopping
// at the match would make the same file complete for an early GID and
// incomplete for a late one.
func scanGroupFile(r io.Reader, source string, gid uint32) (*groupEntry, malformedLines, error) {
	var (
		found     *groupEntry
		malformed malformedLines
	)

	scanner := newDBScanner(r)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue // Skip empty lines and comments
		}

		entry, err := parseGroupLine(line)
		if err != nil {
			slog.Warn("skipping malformed line while searching the group database for group membership",
				slog.String("file", source),
				slog.Int("line", lineNum),
				slog.Any("error", err),
			)
			malformed.record(source, lineNum)
			continue // Skip malformed lines
		}

		// The first entry with the matching GID wins, as it does for
		// getgrgid. Reading on past the match is what makes this a
		// decision at all, so it is stated rather than left to the loop.
		if found == nil && entry.gid == gid {
			found = entry
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, malformed, fmt.Errorf("error reading %s: %w", source, err)
	}

	return found, malformed, nil
}

// findGroupByGID searches src, a file in /etc/group format, for the entry
// with the given GID.
func findGroupByGID(src dbSource, gid uint32) (*groupEntry, malformedLines, error) {
	file, err := src.open()
	if err != nil {
		return nil, malformedLines{}, fmt.Errorf("failed to open %s: %w", src.name, err)
	}
	defer file.Close() //nolint:errcheck

	return scanGroupFile(file, src.name, gid)
}

// parseGroupLine parses a single line from /etc/group
// Format: groupname:password:gid:member1,member2,member3
func parseGroupLine(line string) (*groupEntry, error) {
	fields := strings.Split(line, ":")
	if len(fields) < 4 { //nolint:mnd
		return nil, fmt.Errorf("invalid group line format") //nolint:err113
	}

	gid, err := strconv.ParseUint(fields[2], 10, 32)
	if err != nil {
		return nil, fmt.Errorf("invalid GID: %w", err)
	}

	return &groupEntry{
		name:    fields[0],
		gid:     uint32(gid),
		members: fields[3],
	}, nil
}

// scanPasswdFile returns the users in r, whose contents are in /etc/passwd
// format, whose primary GID is gid. source names r in log records and in the
// recorded position of skipped lines.
func scanPasswdFile(r io.Reader, source string, gid uint32) ([]string, malformedLines, error) {
	var (
		users     []string
		malformed malformedLines
	)

	scanner := newDBScanner(r)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue // Skip empty lines and comments
		}

		user, userGID, err := parsePasswdLine(line)
		if err != nil {
			slog.Warn("skipping malformed line while searching the passwd database for primary group members",
				slog.String("file", source),
				slog.Int("line", lineNum),
				slog.Any("error", err),
			)
			malformed.record(source, lineNum)
			continue // Skip malformed lines
		}

		if userGID == gid {
			users = append(users, user)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, malformed, fmt.Errorf("error reading %s: %w", source, err)
	}

	return users, malformed, nil
}

// findUsersWithPrimaryGID returns the users in src, a file in /etc/passwd
// format, whose primary group is the specified GID.
func findUsersWithPrimaryGID(src dbSource, gid uint32) ([]string, malformedLines, error) {
	file, err := src.open()
	if err != nil {
		return nil, malformedLines{}, fmt.Errorf("failed to open %s: %w", src.name, err)
	}
	defer file.Close() //nolint:errcheck

	return scanPasswdFile(file, src.name, gid)
}

// parsePasswdLine parses a single line from /etc/passwd and returns username and primary GID
// Format: username:password:uid:gid:gecos:home:shell
func parsePasswdLine(line string) (string, uint32, error) {
	fields := strings.Split(line, ":")
	if len(fields) < 4 { //nolint:mnd
		return "", 0, fmt.Errorf("invalid passwd line format") //nolint:err113
	}

	gid, err := strconv.ParseUint(fields[3], 10, 32)
	if err != nil {
		return "", 0, fmt.Errorf("invalid GID: %w", err)
	}

	return fields[0], uint32(gid), nil
}
