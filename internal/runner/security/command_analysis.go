package security

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
)

// Pre-sorted patterns by risk level for efficient lookup
var (
	highRiskPatterns   []DangerousCommandPattern
	mediumRiskPatterns []DangerousCommandPattern
)

// privilegeCommands is a pre-defined list of privilege escalation commands.
var privilegeCommands = []string{"sudo", "su", "doas"}

// Network command sets for efficient lookup
var (
	alwaysNetworkCommands = map[string]struct{}{
		"curl":   {},
		"wget":   {},
		"nc":     {},
		"netcat": {},
		"telnet": {},
		"ssh":    {},
		"scp":    {},
	}

	conditionalNetworkCommands = map[string]struct{}{
		"rsync": {},
		"git":   {},
	}
)

// init initializes the pre-sorted pattern lists for efficient lookup
func init() {
	patterns := GetDangerousCommandPatterns()
	for _, p := range patterns {
		switch p.RiskLevel {
		case runnertypes.RiskLevelHigh:
			highRiskPatterns = append(highRiskPatterns, p)
		case runnertypes.RiskLevelMedium:
			mediumRiskPatterns = append(mediumRiskPatterns, p)
		case runnertypes.RiskLevelLow, runnertypes.RiskLevelUnknown:
			// Skip low and none risk patterns as they don't need checking
			continue
		default:
			// Skip invalid risk levels
			continue
		}
	}
}

// GetDangerousCommandPatterns returns a list of dangerous command patterns for security analysis
func GetDangerousCommandPatterns() []DangerousCommandPattern {
	return []DangerousCommandPattern{
		// File system destruction
		{[]string{"rm", "-rf"}, runnertypes.RiskLevelHigh, "Recursive file removal"},
		{[]string{"sudo", "rm"}, runnertypes.RiskLevelHigh, "Privileged file removal"},
		{[]string{"format"}, runnertypes.RiskLevelHigh, "Disk formatting"},
		{[]string{"mkfs"}, runnertypes.RiskLevelHigh, "File system creation"},
		{[]string{"fdisk"}, runnertypes.RiskLevelHigh, "Disk partitioning"},

		// Data manipulation
		{[]string{"dd", "if="}, runnertypes.RiskLevelHigh, "Low-level disk operations"},
		{[]string{"chmod", "777"}, runnertypes.RiskLevelMedium, "Overly permissive file permissions"},
		{[]string{"chown", "root"}, runnertypes.RiskLevelMedium, "Ownership change to root"},

		// Network operations
		{[]string{"wget"}, runnertypes.RiskLevelMedium, "File download"},
		{[]string{"curl"}, runnertypes.RiskLevelMedium, "Network request"},
		{[]string{"nc", "-"}, runnertypes.RiskLevelMedium, "Network connection"},
		{[]string{"netcat"}, runnertypes.RiskLevelMedium, "Network connection"},
	}
}

// ValidateCommand validates that a command is allowed according to the whitelist
func (v *Validator) ValidateCommand(command string) error {
	if command == "" {
		return fmt.Errorf("%w: empty command", ErrCommandNotAllowed)
	}

	// Check against compiled allowed command patterns
	for _, re := range v.allowedCommandRegexps {
		if re.MatchString(command) {
			return nil
		}
	}

	return fmt.Errorf("%w: command %s does not match any allowed pattern", ErrCommandNotAllowed, command)
}

// IsDangerousPrivilegedCommand checks if a command path is potentially dangerous when run with privileges
func (v *Validator) IsDangerousPrivilegedCommand(cmdPath string) bool {
	_, exists := v.dangerousPrivilegedCommands[cmdPath]
	return exists
}

// IsShellCommand checks if a command is a shell command
func (v *Validator) IsShellCommand(cmdPath string) bool {
	_, exists := v.shellCommands[cmdPath]
	return exists
}

// HasShellMetacharacters checks if any argument contains shell metacharacters
func (v *Validator) HasShellMetacharacters(args []string) bool {
	for _, arg := range args {
		for _, meta := range v.config.ShellMetacharacters {
			if strings.Contains(arg, meta) {
				return true
			}
		}
	}
	return false
}

// IsDangerousRootCommand checks if a command contains dangerous patterns when running as root
func (v *Validator) IsDangerousRootCommand(cmdPath string) bool {
	cmdBase := filepath.Base(cmdPath)
	cmdLower := strings.ToLower(cmdBase)

	for _, dangerous := range v.config.DangerousRootPatterns {
		if strings.Contains(cmdLower, dangerous) {
			return true
		}
	}
	return false
}

// HasDangerousRootArgs checks if any argument contains dangerous patterns for root commands
func (v *Validator) HasDangerousRootArgs(args []string) []int {
	var dangerousIndices []int

	for i, arg := range args {
		argLower := strings.ToLower(arg)
		for _, dangerousPattern := range v.config.DangerousRootArgPatterns {
			if strings.Contains(argLower, dangerousPattern) {
				dangerousIndices = append(dangerousIndices, i)
				break
			}
		}
	}
	return dangerousIndices
}

// HasWildcards checks if any argument contains wildcards
func (v *Validator) HasWildcards(args []string) []int {
	var wildcardIndices []int

	for i, arg := range args {
		if strings.Contains(arg, "*") || strings.Contains(arg, "?") {
			wildcardIndices = append(wildcardIndices, i)
		}
	}
	return wildcardIndices
}

// HasSystemCriticalPaths checks if any argument targets system-critical paths
func (v *Validator) HasSystemCriticalPaths(args []string) []int {
	var criticalIndices []int

	for i, arg := range args {
		for _, criticalPath := range v.config.SystemCriticalPaths {
			if strings.HasPrefix(arg, criticalPath) &&
				(len(arg) == len(criticalPath) || arg[len(criticalPath)] == '/') {
				criticalIndices = append(criticalIndices, i)
				break
			}
		}
	}
	return criticalIndices
}

// checkCommandPatterns checks if a command matches any patterns in the given list
func checkCommandPatterns(cmdName string, cmdArgs []string, patterns []DangerousCommandPattern) (runnertypes.RiskLevel, string, string) {
	for _, pattern := range patterns {
		if matchesPattern(cmdName, cmdArgs, pattern.Pattern) {
			displayPattern := strings.Join(pattern.Pattern, " ")
			return pattern.RiskLevel, displayPattern, pattern.Reason
		}
	}
	return runnertypes.RiskLevelUnknown, "", ""
}

// IsSudoCommand checks if the given command is sudo, considering symbolic links
// Returns (isSudo, error) where error indicates if symlink depth was exceeded
// Deprecated: Use IsPrivilegeEscalationCommand instead
func IsSudoCommand(cmdName string) (bool, error) {
	return IsPrivilegeEscalationCommand(cmdName)
}

// IsPrivilegeEscalationCommand checks if the given command is a privilege escalation command
// (sudo, su, doas), considering symbolic links
// Returns (isPrivilegeEscalation, error) where error indicates if symlink depth was exceeded
func IsPrivilegeEscalationCommand(cmdName string) (bool, error) {
	commandNames, exceededDepth := extractAllCommandNames(cmdName)
	if exceededDepth {
		return false, ErrSymlinkDepthExceeded
	}

	// Check for any privilege escalation commands
	for _, cmd := range privilegeCommands {
		if _, exists := commandNames[cmd]; exists {
			return true, nil
		}
	}

	return false, nil
}

// IsNetworkOperation checks if the command performs network operations
// This function considers symbolic links to detect network commands properly
// Returns (isNetwork, isHighRisk) where isHighRisk indicates symlink depth exceeded
func IsNetworkOperation(cmdName string, args []string) (bool, bool) {
	// Extract all possible command names including symlink targets
	commandNames, exceededDepth := extractAllCommandNames(cmdName)

	// If symlink depth exceeded, this is a high risk security concern
	if exceededDepth {
		return false, true
	}

	// Check if any of the command names match always-network commands
	for name := range commandNames {
		if _, exists := alwaysNetworkCommands[name]; exists {
			return true, false
		}
	}

	// Check if any command name matches conditional network commands
	hasConditionalNetworkCommand := false
	for name := range commandNames {
		if _, exists := conditionalNetworkCommands[name]; exists {
			hasConditionalNetworkCommand = true
			break
		}
	}

	if hasConditionalNetworkCommand {
		// Check for network-related arguments
		allArgs := strings.Join(args, " ")
		if strings.Contains(allArgs, "://") || // URLs
			containsSSHStyleAddress(args) { // SSH-style user@host:path addresses
			return true, false
		}
		return false, false
	}

	// Check for network-related arguments in any command
	allArgs := strings.Join(args, " ")
	if strings.Contains(allArgs, "://") { // URLs
		return true, false
	}

	return false, false
}

// containsSSHStyleAddress checks if any argument contains SSH-style addresses (user@host:path)
// This is more specific than just checking for "@" to avoid false positives with email addresses
func containsSSHStyleAddress(args []string) bool {
	for _, arg := range args {
		// Look for pattern: [user@]host:path
		// Must contain both @ and : with @ appearing before :
		atIndex := strings.Index(arg, "@")
		colonIndex := strings.Index(arg, ":")

		// SSH-style address requires both @ and : with @ before :
		if atIndex != -1 && colonIndex != -1 && atIndex < colonIndex {
			// Additional validation: ensure there's content before @, between @ and :, and after :
			if atIndex > 0 && colonIndex > atIndex+1 && colonIndex < len(arg)-1 {
				// More specific validation: check if the part after : looks like a path
				pathPart := arg[colonIndex+1:]
				// SSH-style paths typically start with / or ~ or contain /
				if strings.HasPrefix(pathPart, "/") || strings.HasPrefix(pathPart, "~") || strings.Contains(pathPart, "/") {
					return true
				}
			}
		}

		// Also check for host:path pattern (without user@)
		if colonIndex != -1 && atIndex == -1 {
			// Ensure there's content before and after :
			if colonIndex > 0 && colonIndex < len(arg)-1 {
				// Simple heuristic: if it looks like a path (contains / or ~) after :, it's likely SSH-style
				pathPart := arg[colonIndex+1:]
				if strings.Contains(pathPart, "/") || strings.HasPrefix(pathPart, "~") {
					return true
				}
			}
		}
	}
	return false
}

// IsDestructiveFileOperation checks if the command performs destructive file operations
func IsDestructiveFileOperation(cmd string, args []string) bool {
	destructiveCommands := map[string]bool{
		"rm":     true,
		"rmdir":  true,
		"unlink": true,
		"shred":  true,
		"dd":     true, // Can be dangerous when used incorrectly
	}

	if destructiveCommands[cmd] {
		return true
	}

	// Check for destructive flags in common commands
	if cmd == "find" {
		for i, arg := range args {
			if arg == "-delete" {
				return true
			}
			if arg == "-exec" && i+1 < len(args) {
				// Check if the command following -exec is destructive
				execCmd := args[i+1]
				if destructiveCommands[execCmd] {
					return true
				}
			}
		}
	}

	if cmd == "rsync" {
		for _, arg := range args {
			if arg == "--delete" || arg == "--delete-before" || arg == "--delete-after" {
				return true
			}
		}
	}

	return false
}

// IsSystemModification checks if the command modifies system settings
func IsSystemModification(cmd string, args []string) bool {
	systemCommands := map[string]bool{
		"systemctl":   true,
		"service":     true,
		"chkconfig":   true,
		"update-rc.d": true,
		"mount":       true,
		"umount":      true,
		"fdisk":       true,
		"parted":      true,
		"mkfs":        true,
		"fsck":        true,
		"crontab":     true,
		"at":          true,
		"batch":       true,
	}

	if systemCommands[cmd] {
		return true
	}

	// Check for package management commands
	packageManagers := map[string]bool{
		"apt":     true,
		"apt-get": true,
		"yum":     true,
		"dnf":     true,
		"zypper":  true,
		"pacman":  true,
		"brew":    true,
		"pip":     true,
		"npm":     true,
		"yarn":    true,
	}

	if packageManagers[cmd] {
		// Only consider install/remove operations as medium risk
		for _, arg := range args {
			if arg == "install" || arg == "remove" || arg == "uninstall" ||
				arg == "upgrade" || arg == "update" {
				return true
			}
		}
	}

	return false
}

// AnalyzeCommandSecurity analyzes a command with its arguments for dangerous patterns.
// This function expects a resolved absolute path for optimal security checking.
// Use this version when you have already resolved the command path through the unified path resolution system.
func AnalyzeCommandSecurity(resolvedPath string, args []string) (riskLevel runnertypes.RiskLevel, detectedPattern string, reason string, err error) {
	// Call the enhanced version with false skipStandardPaths and empty hashDir for backward compatibility
	return AnalyzeCommandSecurityWithConfig(resolvedPath, args, false, "")
}

// AnalyzeCommandSecurityWithConfig analyzes a command with its arguments for dangerous patterns
// with enhanced security validation including directory-based risk assessment and hash validation.
// This is the main implementation that supports both basic and enhanced security analysis.
func AnalyzeCommandSecurityWithConfig(resolvedPath string, args []string, skipStandardPaths bool, hashDir string) (riskLevel runnertypes.RiskLevel, detectedPattern string, reason string, err error) {
	// Step 1: Input validation
	if resolvedPath == "" {
		return runnertypes.RiskLevelUnknown, "", "", fmt.Errorf("%w: empty command path", ErrInvalidPath)
	}

	if !filepath.IsAbs(resolvedPath) {
		return runnertypes.RiskLevelUnknown, "", "", fmt.Errorf("%w: path must be absolute, got relative path: %s", ErrInvalidPath, resolvedPath)
	}

	// Step 2: Symbolic link depth check
	if _, exceededDepth := extractAllCommandNames(resolvedPath); exceededDepth {
		return runnertypes.RiskLevelHigh, resolvedPath, "Symbolic link depth exceeds security limit (potential symlink attack)", nil
	}

	// Step 3: Directory-based default risk assessment
	defaultRisk := getDefaultRiskByDirectory(resolvedPath)

	// Step 4: Hash validation (skip for standard paths when skipStandardPaths=true)
	if !shouldSkipHashValidation(resolvedPath, skipStandardPaths) {
		if err := validateFileHash(resolvedPath, hashDir); err != nil {
			return runnertypes.RiskLevelCritical, resolvedPath,
				fmt.Sprintf("Hash validation failed: %v", err), nil
		}
	}

	// Step 5: High-risk pattern analysis
	if riskLevel, pattern, reason := checkCommandPatterns(resolvedPath, args, highRiskPatterns); riskLevel != runnertypes.RiskLevelUnknown {
		return riskLevel, pattern, reason, nil
	}

	// Step 6: setuid/setgid check
	hasSetuidOrSetgid, setuidErr := hasSetuidOrSetgidBit(resolvedPath)
	if setuidErr != nil {
		return runnertypes.RiskLevelHigh, resolvedPath,
			fmt.Sprintf("Unable to check setuid/setgid status: %v", setuidErr), nil
	}
	if hasSetuidOrSetgid {
		return runnertypes.RiskLevelHigh, resolvedPath,
			"Executable has setuid or setgid bit set", nil
	}

	// Step 7: Medium-risk pattern analysis
	if riskLevel, pattern, reason := checkCommandPatterns(resolvedPath, args, mediumRiskPatterns); riskLevel != runnertypes.RiskLevelUnknown {
		return riskLevel, pattern, reason, nil
	}

	// Step 8: Individual command override application
	if overrideRisk, found := getCommandRiskOverride(resolvedPath); found {
		return overrideRisk, resolvedPath, "Explicit risk level override", nil
	}

	// Step 9: Apply default risk level
	if defaultRisk != runnertypes.RiskLevelUnknown {
		return defaultRisk, "", "Default directory-based risk level", nil
	}

	// Fallback: no specific risk identified
	return runnertypes.RiskLevelUnknown, "", "", nil
}

// extractAllCommandNames extracts all possible command names for matching:
// 1. The original command name (could be full path or just filename)
// 2. Just the base filename from the original command
// 3. All symbolic link names in the chain (if any)
// 4. The final target filename after resolving all symbolic links
// Returns a map for O(1) lookup performance and a boolean indicating if symlink depth was exceeded.
func extractAllCommandNames(cmdName string) (map[string]struct{}, bool) {
	// Handle error case: empty command name (programming error or TOML file mistake)
	if cmdName == "" {
		return make(map[string]struct{}), false
	}

	seen := make(map[string]struct{})

	// Add original command name
	seen[cmdName] = struct{}{}

	// Add base filename (no-op if cmdName is already just a filename)
	seen[filepath.Base(cmdName)] = struct{}{}

	// Resolve symbolic links iteratively to handle multi-level links
	current := cmdName
	exceededDepth := false

	for depth := range MaxSymlinkDepth {
		// Check if current path is a symbolic link
		fileInfo, err := os.Lstat(current)
		if err != nil {
			// If we can't stat the file, stop here
			break
		}

		// If it's not a symbolic link, we're done
		if fileInfo.Mode()&os.ModeSymlink == 0 {
			break
		}

		// If we're at the last iteration and still have a symlink, we exceeded the limit
		if depth == MaxSymlinkDepth-1 {
			exceededDepth = true
			break
		}

		// Resolve the symbolic link
		target, err := os.Readlink(current)
		if err != nil {
			break
		}

		// If target is relative, make it relative to the current directory
		if !filepath.IsAbs(target) {
			current = filepath.Join(filepath.Dir(current), target)
		} else {
			current = target
		}

		// Add the target name (both full path and base name)
		seen[current] = struct{}{}
		seen[filepath.Base(current)] = struct{}{}
	}

	return seen, exceededDepth
}

// matchesPattern checks if the command matches the dangerous pattern.
//
// Pattern matching rules:
//  1. Empty commands are invalid (programming error) and always return false.
//  2. Empty patterns match all valid commands.
//  3. Command names (index 0): Matches against filename only, supporting full paths and symbolic links.
//  4. Argument matching is order-independent.
//  5. Argument count matching: Subset matching (command can have more arguments than pattern).
//  6. Argument patterns ending with "=": Use prefix matching (e.g., "if="
//     matches "if=/dev/zero").
//  7. Other arguments: Require exact string match.
func matchesPattern(cmdName string, cmdArgs []string, pattern []string) bool {
	// If command itself is empty, it's a programming error that should be caught early
	if cmdName == "" {
		return false
	}

	// Empty pattern never matches any command
	if len(pattern) == 0 {
		return false
	}

	// Extract all possible command names (original, base filename, symlink targets)
	commandNames, _ := extractAllCommandNames(cmdName)

	// Check if any of the extracted command names match the pattern[0]
	if _, exists := commandNames[pattern[0]]; !exists {
		return false
	}

	patternArgs := pattern[1:]

	// Default: subset match, require command to have at least as many args as pattern
	if len(cmdArgs) < len(patternArgs) {
		return false
	}

	// Order-independent matching with one-time use of command args
	matchedCommandArgs := make([]bool, len(cmdArgs))
	for _, patternArg := range patternArgs {
		foundMatch := false

		// Prefix pattern when ending with '=' (e.g., "if=")
		if strings.HasSuffix(patternArg, "=") {
			for i, commandArg := range cmdArgs {
				if matchedCommandArgs[i] {
					continue
				}
				if strings.HasPrefix(commandArg, patternArg) {
					matchedCommandArgs[i] = true
					foundMatch = true
					break
				}
			}
		} else {
			// Exact match
			for i, commandArg := range cmdArgs {
				if matchedCommandArgs[i] {
					continue
				}
				if commandArg == patternArg {
					matchedCommandArgs[i] = true
					foundMatch = true
					break
				}
			}
		}

		if !foundMatch {
			return false
		}
	}

	return true
}

// hasSetuidOrSetgidBit checks if the given command path has setuid or setgid bit set
// This function expects a resolved absolute path. Path resolution should be done
// by the caller using the unified path resolution system.
// Returns (hasSetuidOrSetgid, error)
func hasSetuidOrSetgidBit(cmdPath string) (bool, error) {
	if !filepath.IsAbs(cmdPath) {
		return false, fmt.Errorf("%w: path must be absolute, got relative path: %s", ErrInvalidPath, cmdPath)
	}
	// Get file information
	fileInfo, err := os.Stat(cmdPath)
	if err != nil {
		// If we can't stat the file, assume it's not setuid/setgid
		return false, err
	}

	// Check if it's a regular file
	if !fileInfo.Mode().IsRegular() {
		return false, nil
	}

	// Check for setuid or setgid bits
	mode := fileInfo.Mode()
	hasSetuidBit := mode&os.ModeSetuid != 0
	hasSetgidBit := mode&os.ModeSetgid != 0

	return hasSetuidBit || hasSetgidBit, nil
}
