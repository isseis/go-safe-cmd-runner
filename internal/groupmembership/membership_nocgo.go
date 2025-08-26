//go:build !cgo

// Package groupmembership provides utilities for checking group membership
// and related user/group operations.
// This file provides fallback implementations when CGO is disabled by parsing /etc/group.
package groupmembership

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// getGroupMembers returns all members of a group given its GID by parsing /etc/group
func getGroupMembers(gid uint32) ([]string, error) {
	groupEntry, err := findGroupByGID(gid)
	if err != nil {
		return nil, err
	}
	if groupEntry == nil {
		return []string{}, nil // Group not found
	}

	// Parse members from the group entry
	if groupEntry.members == "" {
		return []string{}, nil
	}

	members := strings.Split(groupEntry.members, ",")
	// Filter out empty strings
	result := make([]string, 0, len(members))
	for _, member := range members {
		member = strings.TrimSpace(member)
		if member != "" {
			result = append(result, member)
		}
	}
	return result, nil
}

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
	defer file.Close()

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
	if len(fields) < 4 {
		return nil, fmt.Errorf("invalid group line format")
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
