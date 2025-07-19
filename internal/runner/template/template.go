// Package template provides functionality for template expansion and management
// in the command runner. It supports template definition, property merging,
// and dynamic parameter resolution.
package template

import (
	"errors"
	"fmt"
	"maps"
	"sort"
	"strings"
	"text/template"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
)

// Error definitions for the template package
var (
	// ErrTemplateNotFound is returned when a referenced template is not found
	ErrTemplateNotFound = errors.New("template not found")
	// ErrUndefinedVariable is returned when a template variable is not defined
	ErrUndefinedVariable = errors.New("undefined template variable")
	// ErrCircularDependency is returned when circular template dependency is detected
	ErrCircularDependency = errors.New("circular template dependency detected")
	// ErrInvalidTemplate is returned when template syntax is invalid
	ErrInvalidTemplate = errors.New("invalid template syntax")
	// ErrEmptyTemplateName is returned when template name is empty
	ErrEmptyTemplateName = errors.New("template name cannot be empty")
	// ErrNilTemplate is returned when template is nil
	ErrNilTemplate = errors.New("template cannot be nil")
)

// Template represents a template definition
type Template struct {
	Name        string            `toml:"name"`
	Description string            `toml:"description"`
	TempDir     bool              `toml:"temp_dir"`   // Auto-generate temporary directory
	Cleanup     bool              `toml:"cleanup"`    // Auto cleanup
	WorkDir     string            `toml:"workdir"`    // Working directory (supports "auto")
	Privileged  bool              `toml:"privileged"` // Default privileged execution
	Variables   map[string]string `toml:"variables"`  // Template variables
}

// Engine manages template expansion and application
type Engine struct {
	templates map[string]*Template
	variables map[string]string
}

// NewEngine creates a new template engine
func NewEngine() *Engine {
	return &Engine{
		templates: make(map[string]*Template),
		variables: make(map[string]string),
	}
}

// RegisterTemplate registers a template in the engine
func (e *Engine) RegisterTemplate(name string, tmpl *Template) error {
	if name == "" {
		return ErrEmptyTemplateName
	}
	if tmpl == nil {
		return ErrNilTemplate
	}

	// Set template name if not already set
	if tmpl.Name == "" {
		tmpl.Name = name
	}

	e.templates[name] = tmpl
	return nil
}

// GetTemplate retrieves a template by name
func (e *Engine) GetTemplate(name string) (*Template, error) {
	tmpl, exists := e.templates[name]
	if !exists {
		return nil, fmt.Errorf("%w: %s", ErrTemplateNotFound, name)
	}
	return tmpl, nil
}

// SetVariable sets a global template variable
func (e *Engine) SetVariable(key, value string) {
	if e.variables == nil {
		e.variables = make(map[string]string)
	}
	e.variables[key] = value
}

// SetVariables sets multiple global template variables
func (e *Engine) SetVariables(vars map[string]string) {
	if e.variables == nil {
		e.variables = make(map[string]string)
	}
	maps.Copy(e.variables, vars)
}

// ApplyTemplate applies a template to a command group
func (e *Engine) ApplyTemplate(group *runnertypes.CommandGroup, templateName string) (*runnertypes.CommandGroup, error) {
	if templateName == "" {
		// No template to apply, return as is
		return group, nil
	}

	tmpl, err := e.GetTemplate(templateName)
	if err != nil {
		return nil, fmt.Errorf("failed to get template %s: %w", templateName, err)
	}

	// Create a deep copy of the group to avoid modifying the original
	result := *group
	if group.Commands != nil {
		result.Commands = make([]runnertypes.Command, len(group.Commands))
		copy(result.Commands, group.Commands)
	}

	// Apply template properties to the group
	if err := e.applyTemplateToGroup(&result, tmpl); err != nil {
		return nil, fmt.Errorf("failed to apply template to group: %w", err)
	}

	// Apply template properties to each command in the group
	for i := range result.Commands {
		if err := e.applyTemplateToCommand(&result.Commands[i], tmpl); err != nil {
			return nil, fmt.Errorf("failed to apply template to command %s: %w", result.Commands[i].Name, err)
		}
	}

	return &result, nil
}

// applyTemplateToGroup applies template properties to a command group
func (e *Engine) applyTemplateToGroup(group *runnertypes.CommandGroup, tmpl *Template) error {
	// Merge template variables with engine variables
	variables := make(map[string]string)
	maps.Copy(variables, e.variables)
	maps.Copy(variables, tmpl.Variables)

	// Expand template variables in group properties
	if group.Description != "" {
		expanded, err := e.expandString(group.Description, variables)
		if err != nil {
			return fmt.Errorf("failed to expand group description: %w", err)
		}
		group.Description = expanded
	}

	return nil
}

// applyTemplateToCommand applies template properties to a command
func (e *Engine) applyTemplateToCommand(cmd *runnertypes.Command, tmpl *Template) error {
	// Merge template variables with engine variables
	variables := e.mergeVariables(tmpl.Variables)

	// Apply working directory from template
	if err := e.applyWorkingDirectory(cmd, tmpl, variables); err != nil {
		return err
	}

	// Set privileged flag from template if not explicitly set
	if tmpl.Privileged && !cmd.Privileged {
		cmd.Privileged = tmpl.Privileged
	}

	// Expand template variables in command properties
	return e.expandCommandProperties(cmd, variables)
}

// mergeVariables merges template variables with engine variables
func (e *Engine) mergeVariables(templateVars map[string]string) map[string]string {
	variables := make(map[string]string)
	maps.Copy(variables, e.variables)
	maps.Copy(variables, templateVars)
	return variables
}

// applyWorkingDirectory applies working directory from template
func (e *Engine) applyWorkingDirectory(cmd *runnertypes.Command, tmpl *Template, variables map[string]string) error {
	if cmd.Dir == "" && tmpl.WorkDir != "" {
		if tmpl.WorkDir == "auto" && tmpl.TempDir {
			// Will be handled by resource manager during execution
			cmd.Dir = "{{.temp_dir}}"
		} else {
			expanded, err := e.expandString(tmpl.WorkDir, variables)
			if err != nil {
				return fmt.Errorf("failed to expand working directory: %w", err)
			}
			cmd.Dir = expanded
		}
	}
	return nil
}

// expandCommandProperties expands template variables in command properties
func (e *Engine) expandCommandProperties(cmd *runnertypes.Command, variables map[string]string) error {
	// Expand template variables in command properties
	if cmd.Description != "" {
		expanded, err := e.expandString(cmd.Description, variables)
		if err != nil {
			return fmt.Errorf("failed to expand command description: %w", err)
		}
		cmd.Description = expanded
	}

	if cmd.Cmd != "" {
		expanded, err := e.expandString(cmd.Cmd, variables)
		if err != nil {
			return fmt.Errorf("failed to expand command: %w", err)
		}
		cmd.Cmd = expanded
	}

	// Expand template variables in command arguments
	for i, arg := range cmd.Args {
		expanded, err := e.expandString(arg, variables)
		if err != nil {
			return fmt.Errorf("failed to expand command argument %d: %w", i, err)
		}
		cmd.Args[i] = expanded
	}

	// Expand template variables in environment variables
	for i, env := range cmd.Env {
		expanded, err := e.expandString(env, variables)
		if err != nil {
			return fmt.Errorf("failed to expand environment variable %d: %w", i, err)
		}
		cmd.Env[i] = expanded
	}

	if cmd.Dir != "" {
		expanded, err := e.expandString(cmd.Dir, variables)
		if err != nil {
			return fmt.Errorf("failed to expand command directory: %w", err)
		}
		cmd.Dir = expanded
	}

	return nil
}

// expandString expands template variables in a string using Go's text/template
func (e *Engine) expandString(input string, variables map[string]string) (string, error) {
	// If there are no template markers, return as is for performance
	if !strings.Contains(input, "{{") {
		return input, nil
	}

	// Create a new template with a custom function for error handling
	tmpl := template.New("expand").Option("missingkey=error")
	tmpl, err := tmpl.Parse(input)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrInvalidTemplate, err)
	}

	// Execute template with variables
	var result strings.Builder
	err = tmpl.Execute(&result, variables)
	if err != nil {
		// Check if it's an undefined variable error
		if strings.Contains(err.Error(), "no value for key") || strings.Contains(err.Error(), "has no field or method") {
			return "", fmt.Errorf("%w: %v", ErrUndefinedVariable, err)
		}
		return "", fmt.Errorf("template execution failed: %w", err)
	}

	return result.String(), nil
}

// ValidateTemplate validates a template for syntax errors and circular dependencies
func (e *Engine) ValidateTemplate(name string) error {
	tmpl, err := e.GetTemplate(name)
	if err != nil {
		return err
	}

	// Check for circular dependencies in template variables
	if err := e.detectCircularDependencies(tmpl.Variables); err != nil {
		return err
	}

	// Validate template variables for syntax errors
	for key, value := range tmpl.Variables {
		// Create a temporary variable map for validation
		tempVars := make(map[string]string)
		maps.Copy(tempVars, tmpl.Variables)

		_, err := e.expandString(value, tempVars)
		if err != nil {
			return fmt.Errorf("invalid template variable %s: %w", key, err)
		}
	}

	return nil
}

// ListTemplates returns a list of all registered template names
func (e *Engine) ListTemplates() []string {
	names := make([]string, 0, len(e.templates))
	for name := range e.templates {
		names = append(names, name)
	}
	sort.Strings(names) // Ensure deterministic order
	return names
}

// detectCircularDependencies detects circular dependencies in template variables
func (e *Engine) detectCircularDependencies(variables map[string]string) error {
	// Use DFS to detect cycles in the variable dependency graph
	visiting := make(map[string]bool) // Currently being visited (gray nodes)
	visited := make(map[string]bool)  // Completely processed (black nodes)

	for varName := range variables {
		if !visited[varName] {
			if err := e.dfsVisit(varName, variables, visiting, visited); err != nil {
				return err
			}
		}
	}

	return nil
}

// dfsVisit performs depth-first search to detect cycles
func (e *Engine) dfsVisit(varName string, variables map[string]string, visiting, visited map[string]bool) error {
	// Mark as currently being visited
	visiting[varName] = true

	// Get variable value
	varValue, exists := variables[varName]
	if !exists {
		// Variable doesn't exist, mark as visited and return
		visited[varName] = true
		delete(visiting, varName)
		return nil
	}

	// Find all variable references in the value
	dependencies := e.extractVariableReferences(varValue)

	// Visit each dependency
	for _, depName := range dependencies {
		if visiting[depName] {
			// Found a back edge - circular dependency detected
			return fmt.Errorf("%w: variable '%s' has circular dependency with '%s'", ErrCircularDependency, varName, depName)
		}
		if !visited[depName] {
			if err := e.dfsVisit(depName, variables, visiting, visited); err != nil {
				return err
			}
		}
	}

	// Mark as completely processed
	visited[varName] = true
	delete(visiting, varName)
	return nil
}

// Constants for template parsing
const (
	templatePrefix = "{{." // Prefix for template variable references
	templateSuffix = "}}"  // Suffix for template variable references
)

// extractVariableReferences extracts variable names referenced in a template string
func (e *Engine) extractVariableReferences(input string) []string {
	var references []string
	start := 0

	// Find all {{.variableName}} patterns
	for {
		openIndex := strings.Index(input[start:], templatePrefix)
		if openIndex == -1 {
			break
		}
		openIndex += start

		closeIndex := strings.Index(input[openIndex:], templateSuffix)
		if closeIndex == -1 {
			break
		}
		closeIndex += openIndex

		// Extract variable name (remove {{. and }})
		varRef := input[openIndex+len(templatePrefix) : closeIndex]
		// Remove any trailing spaces or pipes (for template functions)
		if pipeIndex := strings.Index(varRef, "|"); pipeIndex != -1 {
			varRef = varRef[:pipeIndex]
		}
		varRef = strings.TrimSpace(varRef)

		if varRef != "" {
			references = append(references, varRef)
		}

		start = closeIndex + len(templateSuffix)
	}

	return references
}
