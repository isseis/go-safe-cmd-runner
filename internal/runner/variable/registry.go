package variable

import (
	"maps"
	"sort"
	"sync"
)

// VariableRegistry manages variable registration and resolution with scope enforcement.
// It separates global and local variables into distinct namespaces based on naming conventions.
//
// This interface is required by the requirements document (section 5.2) to enforce
// type safety and namespace separation between global and local variables.
//
//nolint:revive // VariableRegistry is descriptive and matches specification document naming
type VariableRegistry interface {
	// RegisterGlobal registers a global variable (must start with uppercase)
	// Returns error if the name doesn't follow global naming rules
	RegisterGlobal(name, value string) error

	// WithLocals creates a new registry with the current global variables and the provided local variables.
	// It validates that all local variable names follow local naming rules.
	// The returned registry is independent of the original one regarding local variables.
	WithLocals(locals map[string]string) (VariableRegistry, error)

	// Resolve resolves a variable name to its value
	// The scope is automatically determined from the variable name (F-003 requirement)
	// Returns error if the variable is not defined
	Resolve(name string) (string, error)

	// GlobalVars returns all global variables as a sorted slice of key-value pairs
	// Used for dry-run output to display variable state in a stable order
	GlobalVars() []VariableEntry

	// LocalVars returns all local variables as a sorted slice of key-value pairs
	// Used for dry-run output to display variable state in a stable order
	LocalVars() []VariableEntry
}

// VariableEntry represents a single variable for display purposes
//
//nolint:revive // VariableEntry is descriptive and matches specification document naming
type VariableEntry struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// variableRegistry is the concrete implementation of VariableRegistry
type variableRegistry struct {
	globals map[string]string
	locals  map[string]string
	mu      sync.RWMutex // Protects concurrent access to maps
}

// NewVariableRegistry creates a new variable registry
func NewVariableRegistry() VariableRegistry {
	return &variableRegistry{
		globals: make(map[string]string),
		locals:  make(map[string]string),
	}
}

func (r *variableRegistry) RegisterGlobal(name, value string) error {
	// Validate that the name is a valid global variable name
	if err := ValidateVariableNameForScope(name, ScopeGlobal, "[global.vars]"); err != nil {
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// No need to check for duplicates here because:
	// 1. The TOML parser guarantees no duplicate keys at parse time
	// 2. This registry is only populated from validated TOML content
	// If dynamic registration is needed in the future, add duplicate checking then.

	r.globals[name] = value
	return nil
}

func (r *variableRegistry) WithLocals(locals map[string]string) (VariableRegistry, error) {
	// Validate all local variable names first
	for name := range locals {
		if err := ValidateVariableNameForScope(name, ScopeLocal, "[local scope]"); err != nil {
			return nil, err
		}
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	// Create new registry with copy of globals
	newRegistry := &variableRegistry{
		globals: make(map[string]string, len(r.globals)),
		locals:  make(map[string]string, len(locals)),
	}

	maps.Copy(newRegistry.globals, r.globals)
	maps.Copy(newRegistry.locals, locals)

	return newRegistry, nil
}

func (r *variableRegistry) Resolve(name string) (string, error) {
	// Determine scope from name
	scope, err := DetermineScope(name)
	if err != nil {
		return "", err
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	switch scope {
	case ScopeGlobal:
		if value, exists := r.globals[name]; exists {
			return value, nil
		}
		return "", &ErrUndefinedGlobalVariable{
			Name: name,
		}

	case ScopeLocal:
		if value, exists := r.locals[name]; exists {
			return value, nil
		}
		return "", &ErrUndefinedLocalVariable{
			Name: name,
		}

	default:
		// This should never happen if DetermineScope is correct
		return "", &ErrInvalidVariableName{
			Name:   name,
			Reason: "unknown scope",
		}
	}
}

func (r *variableRegistry) GlobalVars() []VariableEntry {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Create sorted slice of variable entries
	entries := make([]VariableEntry, 0, len(r.globals))
	for name, value := range r.globals {
		entries = append(entries, VariableEntry{
			Name:  name,
			Value: value,
		})
	}

	// Sort by name for stable output
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name < entries[j].Name
	})

	return entries
}

func (r *variableRegistry) LocalVars() []VariableEntry {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Create sorted slice of variable entries
	entries := make([]VariableEntry, 0, len(r.locals))
	for name, value := range r.locals {
		entries = append(entries, VariableEntry{
			Name:  name,
			Value: value,
		})
	}

	// Sort by name for stable output
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name < entries[j].Name
	})

	return entries
}
