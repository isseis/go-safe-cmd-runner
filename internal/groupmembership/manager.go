package groupmembership

import (
	"errors"
	"fmt"
	"log/slog"
	"math"
	"os"
	"os/user"
	"slices"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
)

const (
	// DefaultCacheTimeout is the default timeout duration for cache entries
	DefaultCacheTimeout = 30 * time.Second
	// CleanupInterval defines how often to perform full cache cleanup (every N cache misses)
	CleanupInterval = 10
	// AllPermissionBits represents all possible permission and special bits
	AllPermissionBits = 0o7777
	// MaxAllowedReadPerms defines the maximum allowed file permissions for read operations
	MaxAllowedReadPerms = 0o6775 // rwsrwsr-x with setuid and setgid
	// MaxAllowedWritePerms defines the maximum allowed file permissions for write operations
	MaxAllowedWritePerms = 0o664 // rw-rw-r-- with group write allowed for write operations
)

// ErrUIDOutOfBounds is returned when a UID value is out of bounds for uint32
var ErrUIDOutOfBounds = errors.New("UID is out of bounds for uint32")

// ErrFileWorldWritable is returned when a file has world-writable permissions
var ErrFileWorldWritable = errors.New("file is world-writable")

// ErrFileNotWritable is returned when a file has no writable permissions for the user
var ErrFileNotWritable = errors.New("file has no writable permissions for user")

// ErrFileNotOwner is returned when a user does not own the file
var ErrFileNotOwner = errors.New("user does not own the file")

// ErrGroupWritableNonMember is returned when accessing group writable file with non-member user
var ErrGroupWritableNonMember = errors.New("group writable file with non-member user access")

// ErrPermissionsExceedMaximum is returned when file permissions exceed the maximum allowed for the operation
var ErrPermissionsExceedMaximum = errors.New("file permissions exceed maximum allowed for operation")

// ErrSudoUIDOutOfRange is returned when SUDO_UID value is out of range for uint32
var ErrSudoUIDOutOfRange = errors.New("SUDO_UID value out of range")

// ErrSudoUIDUserNotFound is returned when the SUDO_UID value is a valid
// number but no user with that UID exists in the user database.
var ErrSudoUIDUserNotFound = errors.New("SUDO_UID does not refer to an existing user")

// ErrSudoUIDUserLookupFailed is returned when the existence check for the
// SUDO_UID value could not be completed, so the user can neither be
// confirmed to exist nor confirmed to be absent (for example a transient
// NSS failure).
var ErrSudoUIDUserLookupFailed = errors.New("failed to verify that SUDO_UID refers to an existing user")

// ErrGroupMemberEnumeration is returned when group member enumeration fails
// due to NSS errors, buffer limit exhaustion, or memory allocation failure.
var ErrGroupMemberEnumeration = errors.New("group member enumeration failed")

// FileOperation represents the type of file operation being performed
type FileOperation int

const (
	// FileOpRead indicates a read operation
	FileOpRead FileOperation = iota
	// FileOpWrite indicates a write operation
	FileOpWrite
)

// GroupMembership provides group membership checking functionality with explicit cache management
type GroupMembership struct {
	// cache for group membership data with thread safety
	membershipCache map[uint32]groupMemberCache
	cacheMutex      sync.RWMutex
	// cleanupCounter tracks cache misses to trigger periodic cleanup
	cleanupCounter int

	// enumerateGroupMembers is the function used to list group members.
	// New() sets it to getGroupMembers (the build-specific implementation).
	// Tests may replace it to inject deterministic failures.
	enumerateGroupMembers func(gid uint32) (groupEnumeration, error)

	// policy is the instance-level permission check UID policy. Its zero
	// value is PolicyUnset, meaning the instance defers to the process-wide
	// default policy (see effectivePermissionCheckUIDPolicy).
	policy PermissionCheckUIDPolicy

	// sudoUIDExistence remembers which SUDO_UID values have already been
	// confirmed to exist, so repeated read-safety checks do not re-query the
	// user database.
	sudoUIDExistence sudoUIDExistenceMemo
}

// groupMemberCache holds a cached enumeration result with its expiry.
type groupMemberCache struct {
	enumeration groupEnumeration
	expiry      time.Time
}

// New creates a new GroupMembership instance. If no options are given, the
// permission check UID policy follows the process-wide default policy.
func New(opts ...Option) *GroupMembership {
	gm := &GroupMembership{
		membershipCache:       make(map[uint32]groupMemberCache),
		enumerateGroupMembers: getGroupMembers,
		sudoUIDExistence:      sudoUIDExistenceMemo{confirmed: make(map[int]struct{})},
	}
	for _, opt := range opts {
		opt(gm)
	}
	return gm
}

// getGroupEnumeration returns the cached or freshly computed enumeration
// result for gid, including its completeness.
func (gm *GroupMembership) getGroupEnumeration(gid uint32) (groupEnumeration, error) {
	// Check cache first
	gm.cacheMutex.RLock()
	if cached, exists := gm.membershipCache[gid]; exists && time.Now().Before(cached.expiry) {
		gm.cacheMutex.RUnlock()
		return cached.enumeration, nil
	}
	gm.cacheMutex.RUnlock()

	// Cache miss or expired - acquire write lock and compute
	gm.cacheMutex.Lock()
	defer gm.cacheMutex.Unlock()

	// Double-check after acquiring write lock (another goroutine might have populated it)
	if cached, exists := gm.membershipCache[gid]; exists && time.Now().Before(cached.expiry) {
		return cached.enumeration, nil
	}

	// Increment cleanup counter and perform periodic cleanup
	gm.cleanupCounter++
	if gm.cleanupCounter >= CleanupInterval {
		gm.clearExpiredCache()
		gm.cleanupCounter = 0
	}

	// Get group members using the appropriate implementation (CGO or non-CGO)
	enumeration, err := gm.enumerateGroupMembers(gid)
	if err != nil {
		return groupEnumeration{}, err
	}

	// Cache the result
	gm.membershipCache[gid] = groupMemberCache{
		enumeration: enumeration,
		expiry:      time.Now().Add(DefaultCacheTimeout),
	}

	return enumeration, nil
}

// GetGroupMembers returns all members of a group given its GID.
// Results are cached for performance with the configured timeout.
func (gm *GroupMembership) GetGroupMembers(gid uint32) ([]string, error) {
	enumeration, err := gm.getGroupEnumeration(gid)
	if err != nil {
		return nil, err
	}
	return enumeration.members, nil
}

// IsUserInGroup checks if a user is a member of a group
func (gm *GroupMembership) IsUserInGroup(uid, gid uint32) (bool, error) {
	// Look up user by UID to get username and primary group
	userInfo, err := user.LookupId(strconv.FormatUint(uint64(uid), 10))
	if err != nil {
		return false, fmt.Errorf("failed to lookup user for UID %d: %w", uid, err)
	}

	// Check if this is the user's primary group
	userPrimaryGID, err := strconv.ParseUint(userInfo.Gid, 10, 32)
	if err != nil {
		return false, fmt.Errorf("failed to parse user's primary GID %s: %w", userInfo.Gid, err)
	}
	if uint32(userPrimaryGID) == gid {
		return true, nil
	}

	// Check secondary group memberships
	groupIDs, err := userInfo.GroupIds()
	if err != nil {
		return false, fmt.Errorf("failed to get user group memberships: %w", err)
	}

	targetGIDStr := strconv.FormatUint(uint64(gid), 10)
	if slices.Contains(groupIDs, targetGIDStr) {
		return true, nil
	}

	// Also check explicit group members (for completeness)
	members, err := gm.GetGroupMembers(gid)
	if err != nil {
		return false, fmt.Errorf("failed to get members of group GID %d: %w", gid, err)
	}

	// Check if the user is in the members list
	return slices.Contains(members, userInfo.Username), nil
}

// isUserOnlyGroupMember checks if the specified user is the only member of a group
// This is useful for security validation where group write permissions are acceptable
// only if the group has a single member who is the specified user
func (gm *GroupMembership) isUserOnlyGroupMember(userUID int, groupGID uint32) (bool, error) {
	user, err := user.LookupId(strconv.Itoa(userUID))
	if err != nil {
		return false, fmt.Errorf("failed to lookup user for UID %d: %w", userUID, err)
	}

	members, err := gm.GetGroupMembers(groupGID)
	if err != nil {
		return false, fmt.Errorf("failed to get group members for GID %d: %w", groupGID, err)
	}

	return len(members) == 1 && members[0] == user.Username, nil
}

// CanUserSafelyWriteFile checks if a user can safely write to a file based on file permissions, ownership and group membership.
// It implements the core security policy: deny if file has other writable permissions (world writable);
// if group writable, allow only if user owns file and is the only group member;
// if owner writable, allow only if user owns the file.
// Returns wrapped ErrUIDOutOfBounds, ErrFileWorldWritable, ErrFileNotOwner, ErrFileNotWritable, or errors from user/group lookups on failure.
func (gm *GroupMembership) CanUserSafelyWriteFile(userUID int, fileUID, fileGID uint32, filePerm os.FileMode) (bool, error) {
	// Validate userUID is within bounds for uint32 before conversion.
	// Reject negative UIDs to avoid underflow when converting to uint32.
	if userUID < 0 || userUID > math.MaxUint32 {
		return false, fmt.Errorf("%w: %d", ErrUIDOutOfBounds, userUID)
	}

	// Convert userUID to uint32 for comparison
	// #nosec G115 -- safe: `userUID` represents a system user ID (UID), which is
	// constrained by the operating system to fit within a 32-bit unsigned value on
	// supported platforms. We already validated bounds above.
	userUID32 := uint32(userUID) // #nosec G115

	perm := filePerm.Perm()

	// 1. Always forbid world writable (other writable)
	if perm&0o002 != 0 {
		return false, fmt.Errorf("%w with permissions %o", ErrFileWorldWritable, perm)
	}

	// 2. Check group writable permissions
	if perm&0o020 != 0 {
		// Group writable: allow only if user owns file AND user is the only member of the group
		if userUID32 != fileUID {
			return false, fmt.Errorf("%w with permissions %o", ErrFileNotOwner, perm) // User doesn't own the file, dangerous to write
		}
		// Check if user is the only member of the file's group
		return gm.isUserOnlyGroupMember(userUID, fileGID)
	}

	// 3. Check owner writable permissions
	if perm&0o200 != 0 {
		// Owner writable: allow only if user owns the file
		if userUID32 == fileUID {
			return true, nil
		}
	}

	// File is not writable by user, group, or others
	return false, fmt.Errorf("%w UID %d", ErrFileNotWritable, userUID)
}

// CanCurrentUserSafelyWriteFile is a convenience wrapper for the current user,
// using the same security policy as CanUserSafelyWriteFile.
// Returns wrapped ErrUIDOutOfBounds from getProcessRealUID, or errors from CanUserSafelyWriteFile.
func (gm *GroupMembership) CanCurrentUserSafelyWriteFile(fileUID, fileGID uint32, filePerm os.FileMode) (bool, error) {
	// For write operations, use the process's real UID (not SUDO_UID) to verify
	// that the running process has permission to write to the file.
	// This is important for hash files that should only be writable by root.
	currentUID, err := getProcessRealUID()
	if err != nil {
		return false, err
	}

	return gm.CanUserSafelyWriteFile(currentUID, fileUID, fileGID, filePerm)
}

// CanCurrentUserSafelyReadFile checks if the current user can safely read from a file
// with more relaxed permissions than write operations. It denies world writable files,
// denies group writable files if the current user is not in the group,
// and allows reading with permissions up to 0o6775.
// Returns wrapped ErrFileWorldWritable, ErrGroupWritableNonMember, ErrPermissionsExceedMaximum,
// or errors from getPermissionCheckUID and IsUserInGroup on check failure.
func (gm *GroupMembership) CanCurrentUserSafelyReadFile(fileGID uint32, filePerm os.FileMode) (bool, error) {
	permissionCheckUID, err := gm.getPermissionCheckUID()
	if err != nil {
		return false, err
	}

	// For reads with group-writable permissions: deny only if the permission-check user is NOT in the group
	// Convert the UID to uint32 for the IsUserInGroup call
	// #nosec G115 -- safe: `permissionCheckUID` represents a system user ID (UID), which is
	// non-negative and constrained by the operating system to fit within a 32-bit
	// unsigned value on supported platforms. It was already validated in getPermissionCheckUID().
	permissionCheckUID32 := uint32(permissionCheckUID) // #nosec G115

	perm := filePerm.Perm()

	// 1. Always forbid world writable (other writable) - same as write policy
	if perm&0o002 != 0 {
		return false, fmt.Errorf("%w with permissions %o", ErrFileWorldWritable, perm)
	}

	// 2. Check group writable permissions - more relaxed than write policy
	if perm&0o020 != 0 {

		isUserInGroup, err := gm.IsUserInGroup(permissionCheckUID32, fileGID)
		if err != nil {
			return false, fmt.Errorf("failed to check group membership: %w", err)
		}

		if !isUserInGroup {
			return false, fmt.Errorf("%w: current user not in file's group", ErrGroupWritableNonMember)
		}
		// If user is in group, allow read access
	}

	// 3. Allow reading with broader permissions
	// This is more permissive than write operations

	permMask := filePerm & AllPermissionBits
	disallowedBits := permMask &^ MaxAllowedReadPerms // Find bits that are set but not allowed
	if disallowedBits != 0 {
		return false, fmt.Errorf("%w: file permissions %o have disallowed bits %o, maximum allowed %o",
			ErrPermissionsExceedMaximum, permMask, disallowedBits, MaxAllowedReadPerms)
	}

	return true, nil
}

// ValidateRequestedPermissions validates that requested permissions don't exceed security limits
// for the specified operation type. Returns ErrPermissionsExceedMaximum if they do.
func (gm *GroupMembership) ValidateRequestedPermissions(perm os.FileMode, operation FileOperation) error {
	// Select maximum allowed permissions based on operation type
	var maxAllowedPerms os.FileMode
	switch operation {
	case FileOpRead:
		maxAllowedPerms = MaxAllowedReadPerms
	case FileOpWrite:
		maxAllowedPerms = MaxAllowedWritePerms
	default:
		return fmt.Errorf("%w: unknown file operation", common.ErrInvalidFileOperation)
	}

	// Check if requested permissions exceed the maximum allowed
	// Use full mode to include setuid/setgid/sticky bits, not just Perm()
	fullMode := perm & AllPermissionBits // Include all permission and special bits
	disallowedBits := fullMode &^ maxAllowedPerms
	if disallowedBits != 0 {
		return fmt.Errorf("%w: requested permissions %o exceed maximum allowed %o for %v operation",
			ErrPermissionsExceedMaximum, fullMode, maxAllowedPerms, operation)
	}

	return nil
}

// ClearCache manually clears all cached group membership data
func (gm *GroupMembership) ClearCache() {
	gm.cacheMutex.Lock()
	defer gm.cacheMutex.Unlock()
	gm.membershipCache = make(map[uint32]groupMemberCache)
	gm.cleanupCounter = 0
}

// CacheStats represents cache statistics in a type-safe manner
type CacheStats struct {
	TotalEntries   int           `json:"total_entries"`
	ExpiredEntries int           `json:"expired_entries"`
	CacheTimeout   time.Duration `json:"cache_timeout"`
}

// GetCacheStats returns cache statistics for monitoring and debugging
func (gm *GroupMembership) GetCacheStats() CacheStats {
	gm.cacheMutex.RLock()
	defer gm.cacheMutex.RUnlock()

	totalEntries := len(gm.membershipCache)
	expiredEntries := 0
	now := time.Now()

	for _, entry := range gm.membershipCache {
		if now.After(entry.expiry) {
			expiredEntries++
		}
	}

	return CacheStats{
		TotalEntries:   totalEntries,
		ExpiredEntries: expiredEntries,
		CacheTimeout:   DefaultCacheTimeout,
	}
}

// clearExpiredCache removes expired cache entries (must be called with write lock held)
func (gm *GroupMembership) clearExpiredCache() {
	now := time.Now()
	for gid, entry := range gm.membershipCache {
		if now.After(entry.expiry) {
			delete(gm.membershipCache, gid)
		}
	}
}

// sudoUIDEnvVar is the name of the environment variable consulted by the
// SudoUIDAware policy.
const sudoUIDEnvVar = "SUDO_UID"

// sudoUIDAdoptionReporter emits the record that SUDO_UID was adopted as the
// permission check UID. It emits at most one record for its lifetime, so a
// single instance shared by the whole process satisfies "once per process".
// It is the only place that builds the record's message and attributes.
type sudoUIDAdoptionReporter struct {
	reported atomic.Bool
}

// report emits the adoption record once unless already emitted.
// A failure to record must not change the read-safety verdict.
func (r *sudoUIDAdoptionReporter) report(logger *slog.Logger, policy PermissionCheckUIDPolicy, realUID, permissionCheckUID int) {
	if !r.reported.CompareAndSwap(false, true) {
		return
	}
	logger.Warn(
		"Permission check UID taken from SUDO_UID instead of the real UID; if this process was not started via sudo, SUDO_UID may be a stale value inherited from the environment",
		slog.Int("permission_check_uid", permissionCheckUID),
		slog.Int("real_uid", realUID),
		slog.String("source_env_var", sudoUIDEnvVar),
		slog.String("permission_check_uid_policy", policy.String()),
		slog.String("user_database_source", userDatabaseSource),
	)
}

// processSudoUIDAdoptionReporter is the single reporter instance shared by
// the whole process, so that the adoption record is emitted at most once per
// process.
var processSudoUIDAdoptionReporter sudoUIDAdoptionReporter

// sudoUIDExistenceMemo remembers the UIDs whose existence has already been
// confirmed, so that repeated read-safety checks do not re-query the user
// database. Only confirmations are remembered; a failed check is always
// re-queried. Callers pass a single UID per process (the value of SUDO_UID,
// which does not change during the lifetime of a record or verify process),
// so in practice it holds one entry; the memo itself imposes no bound.
type sudoUIDExistenceMemo struct {
	mu        sync.Mutex
	confirmed map[int]struct{}
}

// verify returns nil if uid has already been confirmed; otherwise it calls
// lookup and records uid as confirmed. The lock is held across lookup to
// single-flight concurrent queries.
func (m *sudoUIDExistenceMemo) verify(uid int, lookup func(uid int) error) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.confirmed[uid]; ok {
		return nil
	}
	if err := lookup(uid); err != nil {
		return err
	}
	m.confirmed[uid] = struct{}{}
	return nil
}

// permissionCheckUIDDeps bundles the external dependencies that the
// SudoUIDAware branch consults while resolving the permission check UID.
// getPermissionCheckUID supplies the production values; in-package tests
// replace each field. All fields must be non-nil; the RealUIDOnly branch
// reads none of them.
type permissionCheckUIDDeps struct {
	// getenv reads an environment variable (os.Getenv in production).
	getenv func(name string) string

	// verifyUserExists reports whether a user with the given UID exists. A
	// nil error means the user exists. It passes through whatever os/user
	// returns so that the caller can classify the failure.
	verifyUserExists func(uid int) error

	// reportAdoption records that the permission check UID was taken from
	// SUDO_UID. It is the single seam for the record: production binds both
	// the destination logger and the once-per-process guard into it.
	reportAdoption func(policy PermissionCheckUIDPolicy, realUID, permissionCheckUID int)
}

// lookupUserByUID reports whether a user with the given UID exists in the
// user database. It returns the error from os/user unchanged so that the
// caller can distinguish "no such user" from a lookup failure.
func lookupUserByUID(uid int) error {
	_, err := user.LookupId(strconv.Itoa(uid))
	return err
}

// getPermissionCheckUID returns the user ID to use for permission checks,
// resolved according to gm's effective policy (see effectivePermissionCheckUIDPolicy);
// SUDO_UID is only consulted when that policy is SudoUIDAware.
// It is primarily used for read operations to verify the original user has access to the file being read.
func (gm *GroupMembership) getPermissionCheckUID() (int, error) {
	realUID, err := getProcessRealUID()
	if err != nil {
		return 0, err
	}
	return resolvePermissionCheckUID(gm.effectivePermissionCheckUIDPolicy(), realUID, permissionCheckUIDDeps{
		getenv: os.Getenv,
		verifyUserExists: func(uid int) error {
			return gm.sudoUIDExistence.verify(uid, lookupUserByUID)
		},
		reportAdoption: func(policy PermissionCheckUIDPolicy, realUID, permissionCheckUID int) {
			processSudoUIDAdoptionReporter.report(slog.Default(), policy, realUID, permissionCheckUID)
		},
	})
}

// EnsurePermissionCheckUID resolves the permission check UID at startup,
// failing closed if SUDO_UID is unverifiable. This must be called once before processing files,
// since record reads through safefileio.SafeOpenFile without read-safety checks.
func (gm *GroupMembership) EnsurePermissionCheckUID() error {
	precomputeEnumerationEnvironment()
	_, err := gm.getPermissionCheckUID()
	return err
}

// resolvePermissionCheckUID resolves the UID to use for permission checks from
// the effective policy, the real UID, and dependencies bundled in deps.
// Under RealUIDOnly, it always returns realUID.
// Under SudoUIDAware, when realUID is 0 and SUDO_UID is set, it validates and adopts the value,
// returning ErrSudoUIDUserNotFound or ErrSudoUIDUserLookupFailed on failure (fails closed).
func resolvePermissionCheckUID(policy PermissionCheckUIDPolicy, realUID int, deps permissionCheckUIDDeps) (int, error) {
	if policy != SudoUIDAware || realUID != 0 {
		return realUID, nil
	}
	sudoUID := deps.getenv(sudoUIDEnvVar)
	if sudoUID == "" {
		return realUID, nil
	}
	parsedUID, err := parseSudoUID(sudoUID)
	if err != nil {
		return 0, err
	}
	// verifyUserExists passes through whatever os/user returns so that the
	// classification below can distinguish "no such user" from a failed
	// lookup. Both fail closed; the distinction is diagnostic only.
	if err := deps.verifyUserExists(parsedUID); err != nil {
		if _, ok := errors.AsType[user.UnknownUserIdError](err); ok {
			return 0, fmt.Errorf("SUDO_UID %s does not exist in the user database (user_database_source=%s); check whether SUDO_UID is a stale value inherited from the environment, then re-run from an interactive sudo session: %w: %w", echoSudoUID(sudoUID), userDatabaseSource, ErrSudoUIDUserNotFound, err)
		}
		return 0, fmt.Errorf("could not verify SUDO_UID %s against the user database (user_database_source=%s); check the state of the user database, then re-run: %w: %w", echoSudoUID(sudoUID), userDatabaseSource, ErrSudoUIDUserLookupFailed, err)
	}
	if parsedUID != realUID {
		// Bare statement: a failure to record must not change the verdict.
		deps.reportAdoption(policy, realUID, parsedUID)
	}
	return parsedUID, nil
}

// maxEchoedSudoUIDLen bounds the length of the raw SUDO_UID string echoed
// into an error message. A value that passes parseSudoUID is at most 10
// digits, so anything longer is padding (leading zeros, which strconv.Atoi
// accepts without limit) that carries no diagnostic value. The bound matters
// because the value the existence-check errors echo comes from the
// environment, so without it the message length would be caller-controlled.
const maxEchoedSudoUIDLen = 16

// echoSudoUID returns raw for display in an error message, truncated if it is
// longer than maxEchoedSudoUIDLen. Truncation is marked so that the displayed
// value is never mistaken for the value that was actually set.
func echoSudoUID(raw string) string {
	if len(raw) <= maxEchoedSudoUIDLen {
		return raw
	}
	return raw[:maxEchoedSudoUIDLen] + "...(truncated)"
}

// parseSudoUID parses and validates a SUDO_UID string value.
// It is separated from resolvePermissionCheckUID to allow independent testing.
// Returns an error if the value is not a number, is negative, or exceeds uint32.
func parseSudoUID(sudoUID string) (int, error) {
	parsedUID, err := strconv.Atoi(sudoUID)
	if err != nil {
		return 0, fmt.Errorf("failed to parse SUDO_UID %s: %w", sudoUID, err)
	}
	if parsedUID < 0 || parsedUID > math.MaxUint32 {
		return 0, fmt.Errorf("SUDO_UID value out of range %s: %w", sudoUID, ErrSudoUIDOutOfRange)
	}
	return parsedUID, nil
}

// getProcessRealUID returns the process's real UID without considering SUDO_UID.
// It reads from the kernel (os.Getuid) without consulting /etc/passwd or NSS.
// The bounds check is kept for CanCurrentUserSafelyReadFile's gosec G115 suppression.
func getProcessRealUID() (int, error) {
	currentUID := os.Getuid()

	if currentUID < 0 || currentUID > math.MaxUint32 {
		return 0, fmt.Errorf("%w: %d", ErrUIDOutOfBounds, currentUID)
	}

	return currentUID, nil
}
