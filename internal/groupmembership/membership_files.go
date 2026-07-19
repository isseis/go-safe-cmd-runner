//go:build !cgo || test

package groupmembership

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// groupEntry represents a parsed line from /etc/group
type groupEntry struct {
	name    string
	gid     uint32
	members string
}

// findGroupByGID searches for a group entry in /etc/group by GID
func findGroupByGID(gid uint32) (*groupEntry, error) {
	file, err := os.Open("/etc/group")
	if err != nil {
		return nil, fmt.Errorf("failed to open /etc/group: %w", err)
	}
	defer file.Close() //nolint:errcheck

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue // Skip empty lines and comments
		}

		entry, err := parseGroupLine(line)
		if err != nil {
			continue // Skip malformed lines
		}

		if entry.gid == gid {
			return entry, nil
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading /etc/group: %w", err)
	}

	return nil, nil // Group not found
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

// findUsersWithPrimaryGID finds all users that have the specified GID as their primary group
// by parsing /etc/passwd
func findUsersWithPrimaryGID(gid uint32) ([]string, error) {
	file, err := os.Open("/etc/passwd")
	if err != nil {
		return nil, fmt.Errorf("failed to open /etc/passwd: %w", err)
	}
	defer file.Close() //nolint:errcheck

	var users []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue // Skip empty lines and comments
		}

		user, userGID, err := parsePasswdLine(line)
		if err != nil {
			continue // Skip malformed lines
		}

		if userGID == gid {
			users = append(users, user)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading /etc/passwd: %w", err)
	}

	return users, nil
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
