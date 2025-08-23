package security

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
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
		case RiskLevelHigh:
			highRiskPatterns = append(highRiskPatterns, p)
		case RiskLevelMedium:
			mediumRiskPatterns = append(mediumRiskPatterns, p)
		case RiskLevelLow, RiskLevelNone:
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
		{[]string{"rm", "-rf"}, RiskLevelHigh, "Recursive file removal"},
		{[]string{"sudo", "rm"}, RiskLevelHigh, "Privileged file removal"},
		{[]string{"format"}, RiskLevelHigh, "Disk formatting"},
		{[]string{"mkfs"}, RiskLevelHigh, "File system creation"},
		{[]string{"fdisk"}, RiskLevelHigh, "Disk partitioning"},

		// Data manipulation
		{[]string{"dd", "if="}, RiskLevelHigh, "Low-level disk operations"},
		{[]string{"chmod", "777"}, RiskLevelMedium, "Overly permissive file permissions"},
		{[]string{"chown", "root"}, RiskLevelMedium, "Ownership change to root"},

		// Network operations
		{[]string{"wget"}, RiskLevelMedium, "File download"},
		{[]string{"curl"}, RiskLevelMedium, "Network request"},
		{[]string{"nc", "-"}, RiskLevelMedium, "Network connection"},
		{[]string{"netcat"}, RiskLevelMedium, "Network connection"},
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

// checkCommandPatterns checks if a command matches any patterns in the given list
func checkCommandPatterns(cmdName string, cmdArgs []string, patterns []DangerousCommandPattern) (RiskLevel, string, string) {
	for _, pattern := range patterns {
		if matchesPattern(cmdName, cmdArgs, pattern.Pattern) {
			displayPattern := strings.Join(pattern.Pattern, " ")
			return pattern.RiskLevel, displayPattern, pattern.Reason
		}
	}
	return RiskLevelNone, "", ""
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
			strings.Contains(allArgs, "@") { // Email or SSH-style addresses
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

// AnalyzeCommandSecurity analyzes a command with its arguments for dangerous patterns
func AnalyzeCommandSecurity(cmdName string, args []string) (riskLevel RiskLevel, detectedPattern string, reason string) {
	// First, check if symlink depth is exceeded (highest priority security concern)
	if _, exceededDepth := extractAllCommandNames(cmdName); exceededDepth {
		return RiskLevelHigh, cmdName, "Symbolic link depth exceeds security limit (potential symlink attack)"
	}

	// Check high risk patterns
	if riskLevel, pattern, reason := checkCommandPatterns(cmdName, args, highRiskPatterns); riskLevel != RiskLevelNone {
		return riskLevel, pattern, reason
	}

	// Then check medium risk patterns
	if riskLevel, pattern, reason := checkCommandPatterns(cmdName, args, mediumRiskPatterns); riskLevel != RiskLevelNone {
		return riskLevel, pattern, reason
	}

	return RiskLevelNone, "", ""
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
