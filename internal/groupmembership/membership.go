package groupmembership

// IsCurrentUserOnlyGroupMember implements the common logic for checking if:
// 1. Current user is the file owner
// 2. Current user is a member of the file's group
// 3. Current user is the ONLY member of the file's group
// This function uses the default GroupMembership instance for backward compatibility
func IsCurrentUserOnlyGroupMember(fileUID, fileGID uint32) (bool, error) {
	return defaultManager.IsCurrentUserOnlyGroupMember(fileUID, fileGID)
}
