package security

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
)

// Validation errors for CommandRiskProfile
var (
	ErrNetworkAlwaysRequiresMediumRisk      = errors.New("NetworkTypeAlways commands must have BaseRiskLevel >= Medium")
	ErrPrivilegeRequiresHighRisk            = errors.New("privilege escalation commands must have BaseRiskLevel >= High")
	ErrNetworkSubcommandsOnlyForConditional = errors.New("NetworkSubcommands should only be set for NetworkTypeConditional")
)

// NetworkOperationType indicates the type of network operation a command performs
type NetworkOperationType int

// Network operation type constants
const (
	NetworkTypeNone        NetworkOperationType = iota // Not a network command
	NetworkTypeAlways                                  // Always performs network operations
	NetworkTypeConditional                             // Conditional based on arguments
)

// CommandRiskProfile defines comprehensive risk information for a command
//
// Risk Level Determination:
// The BaseRiskLevel represents the inherent risk level of the command itself,
// independent of its arguments. The actual risk level used during execution
// may be elevated based on:
//   - Dangerous command patterns (e.g., "rm -rf")
//   - setuid/setgid bits on the executable
//   - Directory-based default risk (e.g., /tmp has higher risk than /usr/bin)
//   - Hash validation failures
//
// Network Operation Detection:
// NetworkType determines how network operations are detected for this command:
//   - NetworkTypeNone (0): Command never performs network operations
//   - NetworkTypeAlways (1): Command always performs network operations
//     (e.g., curl, wget, ssh). Network detection returns true regardless of arguments.
//   - NetworkTypeConditional (2): Command may perform network operations depending
//     on subcommands or arguments (e.g., git, rsync).
//
// For NetworkTypeConditional commands:
//   - If NetworkSubcommands is non-empty: The first argument (subcommand) is checked
//     against this list. If matched, the command is considered a network operation.
//     Example: git with NetworkSubcommands=["fetch","pull","push"] will detect
//     "git fetch" as a network operation even without a URL argument.
//   - If NetworkSubcommands is empty or no match: Falls back to argument-based
//     detection (checking for URLs "://" or SSH-style addresses "user@host:path").
//
// This design allows precise control over network operation detection while
// maintaining extensibility for commands with complex subcommand structures.
type CommandRiskProfile struct {
	BaseRiskLevel      runnertypes.RiskLevel // Base risk level for the command
	Reason             string                // Reason for the risk level
	IsPrivilege        bool                  // Is privilege escalation command
	NetworkType        NetworkOperationType  // Network operation type
	NetworkSubcommands []string              // Network operation subcommands (for conditional network commands)
}

// Validate checks the consistency of the CommandRiskProfile configuration
func (p CommandRiskProfile) Validate() error {
	// Rule 1: NetworkTypeAlways commands must have BaseRiskLevel >= Medium
	// Rationale: Any command that always performs network operations poses at least medium risk
	// due to potential data exfiltration, network attacks, or credential exposure
	if p.NetworkType == NetworkTypeAlways && p.BaseRiskLevel < runnertypes.RiskLevelMedium {
		return fmt.Errorf("%w (got %v)", ErrNetworkAlwaysRequiresMediumRisk, p.BaseRiskLevel)
	}

	// Rule 2: Privilege escalation commands must have BaseRiskLevel >= High
	// Rationale: Privilege escalation commands can compromise the entire system
	if p.IsPrivilege && p.BaseRiskLevel < runnertypes.RiskLevelHigh {
		return fmt.Errorf("%w (got %v)", ErrPrivilegeRequiresHighRisk, p.BaseRiskLevel)
	}

	// Rule 3: NetworkSubcommands should only be set for NetworkTypeConditional
	if len(p.NetworkSubcommands) > 0 && p.NetworkType != NetworkTypeConditional {
		return fmt.Errorf("%w (got NetworkType=%v)", ErrNetworkSubcommandsOnlyForConditional, p.NetworkType)
	}

	return nil
}

// commandProfileDefinitions defines command risk profiles using the new builder pattern
// This uses CommandRiskProfileNew with explicit risk factor separation
var commandProfileDefinitions = []CommandProfileDef{
	// Phase 2.2.1: Privilege escalation commands
	NewProfile("sudo", "su", "doas").
		PrivilegeRisk(runnertypes.RiskLevelCritical, "Allows execution with elevated privileges, can compromise entire system").
		Build(),

	// Phase 2.2.6: System modification commands
	NewProfile("systemctl", "service").
		SystemModRisk(runnertypes.RiskLevelHigh, "Can modify system services and configuration").
		Build(),

	// Phase 2.2.4: Destructive operations - separate definitions for different risk levels
	NewProfile("rm").
		DestructionRisk(runnertypes.RiskLevelHigh, "Can delete files and directories").
		Build(),
	NewProfile("dd").
		DestructionRisk(runnertypes.RiskLevelCritical, "Can overwrite entire disks, potential data loss").
		Build(),

	// Phase 2.2.5: AI service commands with multiple risk factors
	NewProfile("claude", "gemini", "chatgpt", "gpt", "openai", "anthropic").
		NetworkRisk(runnertypes.RiskLevelHigh, "Always communicates with external AI API").
		DataExfilRisk(runnertypes.RiskLevelHigh, "May send sensitive data to external service").
		AlwaysNetwork().
		Build(),

	// Phase 2.2.2: Network commands (always)
	NewProfile("curl", "wget").
		NetworkRisk(runnertypes.RiskLevelMedium, "Always performs network operations").
		AlwaysNetwork().
		Build(),
	NewProfile("nc", "netcat", "telnet").
		NetworkRisk(runnertypes.RiskLevelMedium, "Establishes network connections").
		AlwaysNetwork().
		Build(),
	NewProfile("ssh", "scp").
		NetworkRisk(runnertypes.RiskLevelMedium, "Remote operations via network").
		AlwaysNetwork().
		Build(),
	NewProfile("aws").
		NetworkRisk(runnertypes.RiskLevelMedium, "Cloud service operations via network").
		AlwaysNetwork().
		Build(),

	// Phase 2.2.3: Network commands (conditional)
	NewProfile("git").
		NetworkRisk(runnertypes.RiskLevelMedium, "Network operations for clone/fetch/pull/push/remote").
		ConditionalNetwork("clone", "fetch", "pull", "push", "remote").
		Build(),
	NewProfile("rsync").
		NetworkRisk(runnertypes.RiskLevelMedium, "Network operations when using remote sources/destinations").
		ConditionalNetwork().
		Build(),
}

// commandRiskProfilesNew is built from commandProfileDefinitions (new structure)
var commandRiskProfilesNew = buildCommandRiskProfilesNew()

func buildCommandRiskProfilesNew() map[string]CommandRiskProfileNew {
	profiles := make(map[string]CommandRiskProfileNew)
	for _, def := range commandProfileDefinitions {
		// Profile is already validated in Build()
		for _, cmd := range def.Commands() {
			profiles[cmd] = def.Profile()
		}
	}
	return profiles
}

// convertNewProfileToOld converts CommandRiskProfileNew to CommandRiskProfile for backward compatibility
func convertNewProfileToOld(newProfile CommandRiskProfileNew) CommandRiskProfile {
	// Get the base risk level (max of all risk factors)
	baseRisk := newProfile.BaseRiskLevel()

	// Get the first risk reason (for backward compatibility)
	var reason string
	reasons := newProfile.GetRiskReasons()
	if len(reasons) > 0 {
		reason = reasons[0]
	}

	return CommandRiskProfile{
		BaseRiskLevel:      baseRisk,
		Reason:             reason,
		IsPrivilege:        newProfile.IsPrivilege(),
		NetworkType:        newProfile.NetworkType,
		NetworkSubcommands: newProfile.NetworkSubcommands,
	}
}

// commandRiskProfiles is built from commandGroupDefinitions (old structure)
// This is kept for backward compatibility during migration
var commandRiskProfiles = buildCommandRiskProfiles()

func buildCommandRiskProfiles() map[string]CommandRiskProfile {
	profiles := make(map[string]CommandRiskProfile)

	// Use new profiles and convert to old structure
	for cmd, newProfile := range commandRiskProfilesNew {
		profiles[cmd] = convertNewProfileToOld(newProfile)
	}

	return profiles
}

// Pre-sorted patterns by risk level for efficient lookup
var (
	highRiskPatterns   []DangerousCommandPattern
	mediumRiskPatterns []DangerousCommandPattern
)

// dangerousCommandPatterns contains the static list of dangerous command patterns
var dangerousCommandPatterns = []DangerousCommandPattern{
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

// init initializes the pre-sorted pattern lists for efficient lookup
func init() {
	for _, p := range dangerousCommandPatterns {
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
		for _, criticalPath := range v.config.GetSystemCriticalPaths() {
			if strings.HasPrefix(arg, criticalPath) &&
				(len(arg) == len(criticalPath) || arg[len(criticalPath)] == '/') {
				criticalIndices = append(criticalIndices, i)
				break
			}
		}
	}
	return criticalIndices
}

// getCommandRiskOverride retrieves the risk override for a specific command
// It now uses command name (basename) instead of full path
func getCommandRiskOverride(cmdPath string) (runnertypes.RiskLevel, bool) {
	// Extract command name from path
	cmdName := filepath.Base(cmdPath)

	// Look up in new unified profiles
	if profile, exists := commandRiskProfiles[cmdName]; exists {
		return profile.BaseRiskLevel, true
	}

	return runnertypes.RiskLevelUnknown, false
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

	// Check for any privilege escalation commands using unified profiles
	for cmdName := range commandNames {
		if profile, exists := commandRiskProfiles[cmdName]; exists && profile.IsPrivilege {
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

	// Check command profiles for network type using unified profiles
	var conditionalProfile *CommandRiskProfile
	for name := range commandNames {
		if profile, exists := commandRiskProfiles[name]; exists {
			switch profile.NetworkType {
			case NetworkTypeAlways:
				return true, false
			case NetworkTypeConditional:
				conditionalProfile = &profile
			}
		}
	}

	if conditionalProfile != nil {
		// Check for network subcommands (e.g., git fetch, git push)
		// Skip command-line options to find the actual subcommand
		if len(conditionalProfile.NetworkSubcommands) > 0 {
			subcommand := findFirstSubcommand(args)
			if subcommand != "" && slices.Contains(conditionalProfile.NetworkSubcommands, subcommand) {
				return true, false
			}
		}

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
	if strings.Contains(allArgs, "://") || // URLs
		containsSSHStyleAddress(args) { // SSH-style user@host:path addresses
		return true, false
	}

	return false, false
}

// findFirstSubcommand returns the first non-option argument from args.
// It skips arguments starting with "-" or "--" to find the actual subcommand.
// Also skips option arguments (e.g., for "-c value", skip both "-c" and "value").
// Returns empty string if no subcommand is found.
func findFirstSubcommand(args []string) string {
	// Common git options that take a value (not exhaustive, but covers common cases)
	optionsWithValue := map[string]bool{
		"-c": true, "-C": true, "--work-tree": true, "--git-dir": true,
		"--config": true, "--namespace": true,
	}

	skipNext := false
	for _, arg := range args {
		// If previous argument was an option that takes a value, skip this arg
		if skipNext {
			skipNext = false
			continue
		}

		// Skip options (starting with - or --)
		if strings.HasPrefix(arg, "-") {
			// Check if it's an option with embedded value (e.g., --config=value)
			if strings.Contains(arg, "=") {
				continue
			}

			// Check if this option takes a value
			if optionsWithValue[arg] {
				skipNext = true
			}
			continue
		}

		// Found the first non-option argument
		return arg
	}
	return ""
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

// AnalysisOptions contains configuration options for command security analysis
type AnalysisOptions struct {
	// SkipStandardPaths determines whether to skip hash validation for standard system paths
	SkipStandardPaths bool
	// HashDir specifies the directory containing hash files for validation
	HashDir string
	// Config provides access to security configuration including test settings
	Config *Config
}

// AnalyzeCommandSecurity analyzes a command with its arguments for dangerous
// patterns with enhanced security validation including directory-based risk
// assessment and hash validation.
//
// This is the primary entry point for command security analysis. It performs
// comprehensive security checks including:
//   - Pattern-based dangerous command detection
//   - setuid/setgid bit analysis
//   - Directory-based default risk assessment
//   - Optional hash validation for executable integrity
//
// Usage examples:
//
//	// Basic analysis (no hash validation)
//	risk, pattern, reason, err := AnalyzeCommandSecurity("/bin/rm", []string{"-rf", "/"}, nil)
//
//	// Analysis with hash validation
//	opts := &AnalysisOptions{
//		SkipStandardPaths: false,
//		HashDir:          "/path/to/hashes",
//	}
//	risk, pattern, reason, err := AnalyzeCommandSecurity("/usr/local/bin/custom", []string{}, opts)
//
//	// Analysis skipping standard paths (useful for system commands)
//	opts := &AnalysisOptions{
//		SkipStandardPaths: true,
//		HashDir:          "/path/to/hashes",
//	}
//	risk, pattern, reason, err := AnalyzeCommandSecurity("/bin/ls", []string{"-la"}, opts)
//
// Parameters:
//   - resolvedPath: Absolute path to the command executable
//   - args: Command line arguments
//   - opts: Configuration options (nil is acceptable for default behavior)
//
// Returns:
//   - riskLevel: Security risk level (Unknown, Low, Medium, High, Critical)
//   - detectedPattern: Matched dangerous pattern (if any)
//   - reason: Human-readable explanation of the risk assessment
//   - err: Error if analysis fails
func AnalyzeCommandSecurity(resolvedPath string, args []string, opts *AnalysisOptions) (riskLevel runnertypes.RiskLevel, detectedPattern string, reason string, err error) {
	// Handle nil options
	if opts == nil {
		opts = &AnalysisOptions{}
	}

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

	// Step 4: Hash validation (skip for standard paths when SkipStandardPaths=true)
	if !shouldSkipHashValidation(resolvedPath, opts.SkipStandardPaths) && opts.HashDir != "" {
		if err := validateFileHash(resolvedPath, opts.HashDir, opts.Config); err != nil {
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
