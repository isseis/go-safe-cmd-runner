package template

import (
	"reflect"
	"strings"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
)

func TestNewEngine(t *testing.T) {
	engine := NewEngine()
	if engine == nil {
		t.Fatal("NewEngine() returned nil")
	}
	if engine.templates == nil {
		t.Error("Engine templates map is nil")
	}
	if engine.variables == nil {
		t.Error("Engine variables map is nil")
	}
}

func TestRegisterTemplate(t *testing.T) {
	engine := NewEngine()

	tmpl := &Template{
		Name:        "test-template",
		Description: "Test template",
		TempDir:     true,
		Cleanup:     true,
	}

	err := engine.RegisterTemplate("test-template", tmpl)
	if err != nil {
		t.Errorf("RegisterTemplate() failed: %v", err)
	}

	// Test retrieving the template
	retrieved, err := engine.GetTemplate("test-template")
	if err != nil {
		t.Errorf("GetTemplate() failed: %v", err)
	}
	if retrieved.Name != tmpl.Name {
		t.Errorf("Retrieved template name = %v, want %v", retrieved.Name, tmpl.Name)
	}
}

func TestRegisterTemplateErrors(t *testing.T) {
	engine := NewEngine()

	tests := []struct {
		name     string
		tmplName string
		tmpl     *Template
		wantErr  bool
	}{
		{
			name:     "empty template name",
			tmplName: "",
			tmpl:     &Template{},
			wantErr:  true,
		},
		{
			name:     "nil template",
			tmplName: "test",
			tmpl:     nil,
			wantErr:  true,
		},
		{
			name:     "valid template",
			tmplName: "test",
			tmpl:     &Template{Description: "Test"},
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := engine.RegisterTemplate(tt.tmplName, tt.tmpl)
			if (err != nil) != tt.wantErr {
				t.Errorf("RegisterTemplate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestGetTemplate(t *testing.T) {
	engine := NewEngine()

	// Test getting non-existent template
	_, err := engine.GetTemplate("non-existent")
	if err == nil {
		t.Error("GetTemplate() expected error for non-existent template")
	}
	if !strings.Contains(err.Error(), "template not found") {
		t.Errorf("GetTemplate() error should contain 'template not found', got: %v", err)
	}
}

func TestSetVariables(t *testing.T) {
	engine := NewEngine()

	variables := map[string]string{
		"var1": "value1",
		"var2": "value2",
	}

	engine.SetVariables(variables)

	for key, expectedValue := range variables {
		if actualValue, exists := engine.variables[key]; !exists || actualValue != expectedValue {
			t.Errorf("Variable %s = %v, want %v", key, actualValue, expectedValue)
		}
	}
}

func TestSetVariable(t *testing.T) {
	engine := NewEngine()

	engine.SetVariable("test_var", "test_value")

	if value, exists := engine.variables["test_var"]; !exists || value != "test_value" {
		t.Errorf("SetVariable() failed, got %v, want test_value", value)
	}
}

func TestExpandString(t *testing.T) {
	engine := NewEngine()

	variables := map[string]string{
		"name":    "test",
		"version": "1.0.0",
		"path":    "/usr/local/bin",
	}

	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{
			name:    "no template variables",
			input:   "simple string",
			want:    "simple string",
			wantErr: false,
		},
		{
			name:    "single variable",
			input:   "Hello {{.name}}",
			want:    "Hello test",
			wantErr: false,
		},
		{
			name:    "multiple variables",
			input:   "{{.name}} version {{.version}}",
			want:    "test version 1.0.0",
			wantErr: false,
		},
		{
			name:    "path construction",
			input:   "{{.path}}/{{.name}}",
			want:    "/usr/local/bin/test",
			wantErr: false,
		},
		{
			name:    "undefined variable",
			input:   "{{.undefined}}",
			want:    "",
			wantErr: true,
		},
		{
			name:    "invalid template syntax",
			input:   "{{.name",
			want:    "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := engine.expandString(tt.input, variables)
			if (err != nil) != tt.wantErr {
				t.Errorf("expandString() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("expandString() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestApplyTemplate(t *testing.T) {
	engine := NewEngine()

	// Register a test template
	tmpl := &Template{
		Name:        "test-template",
		Description: "Test template for {{.app}}",
		TempDir:     true,
		Cleanup:     true,
		WorkDir:     "/tmp/{{.app}}",
		Variables: map[string]string{
			"app":     "myapp",
			"version": "1.0.0",
		},
	}
	err := engine.RegisterTemplate("test-template", tmpl)
	if err != nil {
		t.Fatalf("RegisterTemplate() failed: %v", err)
	}

	// Create a test command group
	group := &runnertypes.CommandGroup{
		Name:        "test-group",
		Description: "Test group for {{.app}}",
		Commands: []runnertypes.Command{
			{
				Name:        "test-cmd",
				Description: "Test command for {{.app}} v{{.version}}",
				Cmd:         "echo",
				Args:        []string{"{{.app}}", "{{.version}}"},
				Env:         []string{"USER=test"},
				Dir:         "",
			},
		},
	}

	// Apply template
	result, err := engine.ApplyTemplate(group, "test-template")
	if err != nil {
		t.Fatalf("ApplyTemplate() failed: %v", err)
	}

	// Verify group properties
	expectedGroupDesc := "Test group for myapp"
	if result.Description != expectedGroupDesc {
		t.Errorf("Group description = %v, want %v", result.Description, expectedGroupDesc)
	}

	// Verify command properties
	cmd := result.Commands[0]
	expectedCmdDesc := "Test command for myapp v1.0.0"
	if cmd.Description != expectedCmdDesc {
		t.Errorf("Command description = %v, want %v", cmd.Description, expectedCmdDesc)
	}

	expectedArgs := []string{"myapp", "1.0.0"}
	if !reflect.DeepEqual(cmd.Args, expectedArgs) {
		t.Errorf("Command args = %v, want %v", cmd.Args, expectedArgs)
	}

	// Verify working directory was set from template
	expectedDir := "/tmp/myapp"
	if cmd.Dir != expectedDir {
		t.Errorf("Command dir = %v, want %v", cmd.Dir, expectedDir)
	}
}

func TestApplyTemplateNoTemplate(t *testing.T) {
	engine := NewEngine()

	group := &runnertypes.CommandGroup{
		Name: "test-group",
	}

	// Apply empty template name (should return original group)
	result, err := engine.ApplyTemplate(group, "")
	if err != nil {
		t.Errorf("ApplyTemplate() with empty template failed: %v", err)
	}
	if result != group {
		t.Error("ApplyTemplate() with empty template should return original group")
	}
}

func TestApplyTemplateNotFound(t *testing.T) {
	engine := NewEngine()

	group := &runnertypes.CommandGroup{
		Name: "test-group",
	}

	// Apply non-existent template
	_, err := engine.ApplyTemplate(group, "non-existent")
	if err == nil {
		t.Error("ApplyTemplate() with non-existent template should return error")
	}
}

func TestValidateTemplate(t *testing.T) {
	engine := NewEngine()

	// Register a template with valid variables
	validTmpl := &Template{
		Name: "valid",
		Variables: map[string]string{
			"simple": "value",
			"nested": "prefix-{{.simple}}-suffix",
		},
	}
	engine.RegisterTemplate("valid", validTmpl)

	// Register a template with invalid variables
	invalidTmpl := &Template{
		Name: "invalid",
		Variables: map[string]string{
			"broken": "{{.undefined}}",
		},
	}
	engine.RegisterTemplate("invalid", invalidTmpl)

	// Register a template with circular dependencies
	circularTmpl := &Template{
		Name: "circular",
		Variables: map[string]string{
			"var1": "{{.var2}}",
			"var2": "{{.var1}}",
		},
	}
	engine.RegisterTemplate("circular", circularTmpl)

	// Register a template with complex circular dependencies
	complexCircularTmpl := &Template{
		Name: "complex-circular",
		Variables: map[string]string{
			"a": "{{.b}}",
			"b": "{{.c}}",
			"c": "{{.a}}",
			"d": "independent",
		},
	}
	engine.RegisterTemplate("complex-circular", complexCircularTmpl)

	// Register a template with self-reference
	selfRefTmpl := &Template{
		Name: "self-ref",
		Variables: map[string]string{
			"self": "{{.self}}",
		},
	}
	engine.RegisterTemplate("self-ref", selfRefTmpl)

	tests := []struct {
		name     string
		tmplName string
		wantErr  bool
		errorMsg string
	}{
		{
			name:     "valid template",
			tmplName: "valid",
			wantErr:  false,
		},
		{
			name:     "invalid template",
			tmplName: "invalid",
			wantErr:  true,
		},
		{
			name:     "circular dependency",
			tmplName: "circular",
			wantErr:  true,
			errorMsg: "circular dependency",
		},
		{
			name:     "complex circular dependency",
			tmplName: "complex-circular",
			wantErr:  true,
			errorMsg: "circular dependency",
		},
		{
			name:     "self reference",
			tmplName: "self-ref",
			wantErr:  true,
			errorMsg: "circular dependency",
		},
		{
			name:     "non-existent template",
			tmplName: "non-existent",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := engine.ValidateTemplate(tt.tmplName)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateTemplate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && tt.errorMsg != "" && !strings.Contains(err.Error(), tt.errorMsg) {
				t.Errorf("ValidateTemplate() error = %v, should contain %v", err, tt.errorMsg)
			}
		})
	}
}

func TestListTemplates(t *testing.T) {
	engine := NewEngine()

	// Initially should be empty
	templates := engine.ListTemplates()
	if len(templates) != 0 {
		t.Errorf("ListTemplates() initial length = %d, want 0", len(templates))
	}

	// Add some templates
	tmpl1 := &Template{Name: "template1"}
	tmpl2 := &Template{Name: "template2"}

	engine.RegisterTemplate("template1", tmpl1)
	engine.RegisterTemplate("template2", tmpl2)

	templates = engine.ListTemplates()
	if len(templates) != 2 {
		t.Errorf("ListTemplates() length = %d, want 2", len(templates))
	}

	// Verify template names are present
	templateMap := make(map[string]bool)
	for _, name := range templates {
		templateMap[name] = true
	}

	if !templateMap["template1"] || !templateMap["template2"] {
		t.Errorf("ListTemplates() missing expected templates, got %v", templates)
	}
}

func TestExtractVariableReferences(t *testing.T) {
	engine := NewEngine()

	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{
			name:  "no variables",
			input: "simple string",
			want:  nil,
		},
		{
			name:  "single variable",
			input: "{{.var1}}",
			want:  []string{"var1"},
		},
		{
			name:  "multiple variables",
			input: "{{.var1}} and {{.var2}}",
			want:  []string{"var1", "var2"},
		},
		{
			name:  "variable with pipe function",
			input: "{{.var1 | upper}}",
			want:  []string{"var1"},
		},
		{
			name:  "mixed content",
			input: "prefix {{.var1}} middle {{.var2}} suffix",
			want:  []string{"var1", "var2"},
		},
		{
			name:  "nested template (should extract outer only)",
			input: "{{.outer}}",
			want:  []string{"outer"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := engine.extractVariableReferences(tt.input)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("extractVariableReferences() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDetectCircularDependencies(t *testing.T) {
	engine := NewEngine()

	tests := []struct {
		name      string
		variables map[string]string
		wantErr   bool
	}{
		{
			name: "no dependencies",
			variables: map[string]string{
				"var1": "value1",
				"var2": "value2",
			},
			wantErr: false,
		},
		{
			name: "valid chain dependency",
			variables: map[string]string{
				"var1": "{{.var2}}",
				"var2": "{{.var3}}",
				"var3": "value3",
			},
			wantErr: false,
		},
		{
			name: "simple circular dependency",
			variables: map[string]string{
				"var1": "{{.var2}}",
				"var2": "{{.var1}}",
			},
			wantErr: true,
		},
		{
			name: "self reference",
			variables: map[string]string{
				"var1": "{{.var1}}",
			},
			wantErr: true,
		},
		{
			name: "complex circular dependency",
			variables: map[string]string{
				"a": "{{.b}}",
				"b": "{{.c}}",
				"c": "{{.a}}",
				"d": "independent",
			},
			wantErr: true,
		},
		{
			name: "partial circular with independent vars",
			variables: map[string]string{
				"good1": "value1",
				"good2": "{{.good1}}",
				"bad1":  "{{.bad2}}",
				"bad2":  "{{.bad1}}",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := engine.detectCircularDependencies(tt.variables)
			if (err != nil) != tt.wantErr {
				t.Errorf("detectCircularDependencies() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
