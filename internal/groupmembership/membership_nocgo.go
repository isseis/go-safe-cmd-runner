//go:build !cgo

// Package groupmembership provides utilities for checking group membership
// and related user/group operations.
// This file provides fallback implementations when CGO is disabled by parsing /etc/group.
package groupmembership

import (
	"fmt"
	"log/slog"
	"runtime"
	"strings"
	"sync"
)

// userDatabaseSource names the user database this build consults for user
// lookups. Without cgo, os/user parses /etc/passwd only; directory-backed
// sources such as LDAP or SSSD are not consulted.
const userDatabaseSource = "passwd-file"

// getGroupMembers returns all members of a group given its GID by parsing
// /etc/group and /etc/passwd to find users with this GID as their primary
// group. The completeness it states combines what this build's environment
// already says about its ability to see every member with the lines the
// scans had to skip.
// This is a stateless function - caching is handled by the GroupMembership struct
func getGroupMembers(gid uint32) (groupEnumeration, error) {
	return enumerateFromFiles(gid, nsswitchVerdict())
}

// enumerateFromFiles lists the members of the group with the given GID from
// /etc/group and /etc/passwd. nssVerdict is what this build's environment
// already says about its ability to see every member; the returned
// completeness combines it with the lines the scans had to skip. Taking the
// verdict as a parameter is what lets a test drive the whole enumeration
// with a chosen classification.
func enumerateFromFiles(gid uint32, nssVerdict completenessVerdict) (groupEnumeration, error) {
	return enumerateFromSources(groupFileSource(), passwdFileSource(), gid, nssVerdict)
}

// enumerateFromSources is enumerateFromFiles over the given user database
// sources, so that a test can drive the whole enumeration -- both scans and
// the combination of every source of doubt -- from chosen contents.
func enumerateFromSources(groupSrc, passwdSrc dbSource, gid uint32, nssVerdict completenessVerdict) (groupEnumeration, error) {
	groupEntry, groupMalformed, err := findGroupByGID(groupSrc, gid)
	if err != nil {
		return groupEnumeration{}, err
	}

	// Both databases are scanned even when the group is absent from the
	// first. Completeness describes the files the enumeration had to read,
	// so it must not depend on which GID was asked about -- the same reason
	// the scans read past their matching entry. The members are a separate
	// question, decided below.
	primaryUsers, passwdMalformed, err := findUsersWithPrimaryGID(passwdSrc, gid)
	if err != nil {
		return groupEnumeration{}, fmt.Errorf("failed to find users with primary GID %d: %w", gid, err)
	}

	// The classification is evaluated first so that, when more than one of
	// these says incomplete, the cause reported is the environment one:
	// rebuilding with cgo is the remediation that covers all of them.
	verdict := nssVerdict.combine(groupMalformed.verdict()).combine(passwdMalformed.verdict())

	if groupEntry == nil {
		// A group with no entry in the group database has no members, even
		// if some user names it as their primary GID. The cgo build reports
		// the same empty set for an unknown group, and the two builds must
		// agree on the members they return.
		return groupEnumeration{members: []string{}, verdict: verdict}, nil
	}

	// Start with explicit members from /etc/group
	memberSet := make(map[string]struct{})
	if groupEntry.members != "" {
		for member := range strings.SplitSeq(groupEntry.members, ",") {
			member = strings.TrimSpace(member)
			if member != "" {
				memberSet[member] = struct{}{}
			}
		}
	}

	// Add the users holding this GID as their primary group.
	for _, user := range primaryUsers {
		memberSet[user] = struct{}{}
	}

	// Convert map to slice
	result := make([]string, 0, len(memberSet))
	for member := range memberSet {
		result = append(result, member)
	}

	return groupEnumeration{members: result, verdict: verdict}, nil
}

// precomputeEnumerationEnvironment resolves whatever environment facts this
// build needs before the first enumeration, so that a build unable to
// enumerate every member says so at startup rather than at the first
// group-writable file.
func precomputeEnumerationEnvironment() {
	nsswitchVerdict()
}

// processNSSCompletenessReporter is the single reporter instance shared by
// the whole process, so that the classification record is emitted at most
// once per process.
var processNSSCompletenessReporter nssCompletenessReporter

// The classification is settled once and reused for the lifetime of the
// process: a permission decision that changed halfway through a run because
// /etc/nsswitch.conf was edited would be a denial the operator cannot
// reproduce. The cost is that a change to that file goes unobserved until
// the process exits.
var (
	nsswitchVerdictMu       sync.Mutex
	nsswitchVerdictResolved bool
	nsswitchVerdictValue    completenessVerdict
)

// nsswitchVerdict returns the classification for this process. It reads and
// classifies on first call, records an incomplete classification once, and
// reuses the result thereafter.
func nsswitchVerdict() completenessVerdict {
	verdict, justSettled := settleNsswitchVerdict()
	if justSettled {
		// The record is emitted outside the lock: a log handler is
		// arbitrary code, and one that reached back into this package
		// would deadlock on a lock that is not reentrant.
		processNSSCompletenessReporter.report(slog.Default(), verdict)
	}
	return verdict
}

// settleNsswitchVerdict returns the classification for this process and
// whether this call is the one that settled it.
func settleNsswitchVerdict() (completenessVerdict, bool) {
	nsswitchVerdictMu.Lock()
	defer nsswitchVerdictMu.Unlock()

	if nsswitchVerdictResolved {
		return nsswitchVerdictValue, false
	}
	nsswitchVerdictValue = classifyNSSCompleteness(readNsswitchSnapshot(), runtime.GOOS)
	nsswitchVerdictResolved = true
	return nsswitchVerdictValue, true
}
