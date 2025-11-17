package cli

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
)

// GroupNamePattern defines the naming rule for groups.
// Allowed characters follow the environment variable convention: [A-Za-z_][A-Za-z0-9_]*.
var GroupNamePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// Error definitions for group filtering.
var (
	ErrInvalidGroupName = errors.New("invalid group name")
	ErrGroupNotFound    = errors.New("group not found")
	ErrNilConfig        = errors.New("configuration must not be nil")
)

// ParseGroupNames parses the --groups CLI flag and returns a slice of group names.
// It splits the input by comma, trims whitespace, and drops empty entries.
// Returns nil when the input is empty or resolves to no valid values.
func ParseGroupNames(groupsFlag string) []string {
	if strings.TrimSpace(groupsFlag) == "" {
		return nil
	}

	parts := strings.Split(groupsFlag, ",")
	result := make([]string, 0, len(parts))

	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}
		result = append(result, trimmed)
	}

	if len(result) == 0 {
		return nil
	}

	return result
}

// ValidateGroupName verifies that a single group name complies with the naming convention.
func ValidateGroupName(name string) error {
	if !GroupNamePattern.MatchString(name) {
		return fmt.Errorf("%w: %q must match pattern [A-Za-z_][A-Za-z0-9_]*", ErrInvalidGroupName, name)
	}
	return nil
}

// ValidateGroupNames verifies that all provided group names are valid.
func ValidateGroupNames(names []string) error {
	for _, name := range names {
		if err := ValidateGroupName(name); err != nil {
			return err
		}
	}
	return nil
}

// CheckGroupsExist ensures that every group name exists in the provided configuration.
func CheckGroupsExist(names []string, config *runnertypes.ConfigSpec) error {
	if len(names) == 0 {
		return nil
	}
	if config == nil {
		return fmt.Errorf("%w: %w", ErrGroupNotFound, ErrNilConfig)
	}

	// Build a map of existing group names for O(1) lookup
	// This reduces time complexity from O(N*M) to O(N+M)
	existingGroups := make(map[string]struct{}, len(config.Groups))
	for _, group := range config.Groups {
		existingGroups[group.Name] = struct{}{}
	}

	// Check each requested group against the map
	var (
		missing    []string
		missingSet = make(map[string]struct{})
	)

	for _, name := range names {
		if _, exists := existingGroups[name]; !exists {
			// Track missing groups with deduplication
			if _, alreadyAdded := missingSet[name]; !alreadyAdded {
				missingSet[name] = struct{}{}
				missing = append(missing, name)
			}
		}
	}

	if len(missing) > 0 {
		// Build available groups list (deduplicated)
		available := make([]string, 0, len(existingGroups))
		for groupName := range existingGroups {
			available = append(available, groupName)
		}

		return fmt.Errorf("%w: group(s) %v specified in --groups do not exist in configuration\nAvailable groups: %v",
			ErrGroupNotFound, missing, available)
	}

	return nil
}

// FilterGroups validates and filters the configuration based on the requested names.
// When names is nil or empty, it returns all group names from the configuration.
func FilterGroups(names []string, config *runnertypes.ConfigSpec) ([]string, error) {
	if config == nil {
		return nil, ErrNilConfig
	}

	if len(names) == 0 {
		allGroups := make([]string, len(config.Groups))
		for i, group := range config.Groups {
			allGroups[i] = group.Name
		}
		return allGroups, nil
	}

	if err := ValidateGroupNames(names); err != nil {
		return nil, err
	}

	if err := CheckGroupsExist(names, config); err != nil {
		return nil, err
	}

	return append([]string(nil), names...), nil
}
