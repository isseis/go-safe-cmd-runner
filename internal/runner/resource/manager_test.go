package resource

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
)

func TestNewManager(t *testing.T) {
	manager := NewManager("")
	if manager == nil {
		t.Fatal("NewManager() returned nil")
	}
	if manager.resources == nil {
		t.Error("Manager resources map is nil")
	}
	if manager.baseDir == "" {
		t.Error("Manager baseDir should be set to temp dir when empty")
	}

	// Test with custom base directory
	customDir := "/tmp/test"
	manager2 := NewManager(customDir)
	if manager2.baseDir != customDir {
		t.Errorf("Manager baseDir = %v, want %v", manager2.baseDir, customDir)
	}
}

func TestCreateTempDir(t *testing.T) {
	tempDir := t.TempDir()
	manager := NewManager(tempDir)

	resource, err := manager.CreateTempDir("test-command", true)
	if err != nil {
		t.Fatalf("CreateTempDir() failed: %v", err)
	}

	if resource == nil {
		t.Fatal("CreateTempDir() returned nil resource")
	}

	if resource.Type != TypeTempDir {
		t.Errorf("Resource type = %v, want %v", resource.Type, TypeTempDir)
	}

	if resource.Command != "test-command" {
		t.Errorf("Resource command = %v, want test-command", resource.Command)
	}

	if !resource.AutoCleanup {
		t.Error("Resource AutoCleanup should be true")
	}

	// Check that directory was actually created
	if _, err := os.Stat(resource.Path); os.IsNotExist(err) {
		t.Errorf("Temporary directory was not created: %s", resource.Path)
	}

	// Check that path is under the base directory
	if !strings.HasPrefix(resource.Path, tempDir) {
		t.Errorf("Resource path %s is not under base directory %s", resource.Path, tempDir)
	}
}

func TestGetResource(t *testing.T) {
	tempDir := t.TempDir()
	manager := NewManager(tempDir)

	// Create a resource
	resource, err := manager.CreateTempDir("test-command", true)
	if err != nil {
		t.Fatalf("CreateTempDir() failed: %v", err)
	}

	// Retrieve the resource
	retrieved, err := manager.GetResource(resource.ID)
	if err != nil {
		t.Errorf("GetResource() failed: %v", err)
	}

	if retrieved.ID != resource.ID {
		t.Errorf("Retrieved resource ID = %v, want %v", retrieved.ID, resource.ID)
	}

	// Try to get non-existent resource
	_, err = manager.GetResource("non-existent")
	if err == nil {
		t.Error("GetResource() should return error for non-existent resource")
	}
}

func TestListResources(t *testing.T) {
	tempDir := t.TempDir()
	manager := NewManager(tempDir)

	// Initially should be empty
	resources := manager.ListResources()
	if len(resources) != 0 {
		t.Errorf("ListResources() initial length = %d, want 0", len(resources))
	}

	// Create some resources
	_, err := manager.CreateTempDir("test-command-1", true)
	if err != nil {
		t.Fatalf("CreateTempDir() failed: %v", err)
	}

	_, err = manager.CreateTempDir("test-command-2", false)
	if err != nil {
		t.Fatalf("CreateTempDir() failed: %v", err)
	}

	resources = manager.ListResources()
	if len(resources) != 2 {
		t.Errorf("ListResources() length = %d, want 2", len(resources))
	}
}

func TestCleanupResource(t *testing.T) {
	tempDir := t.TempDir()
	manager := NewManager(tempDir)

	// Create a resource
	resource, err := manager.CreateTempDir("test-command", true)
	if err != nil {
		t.Fatalf("CreateTempDir() failed: %v", err)
	}

	// Verify directory exists
	if _, err := os.Stat(resource.Path); os.IsNotExist(err) {
		t.Fatalf("Temporary directory was not created: %s", resource.Path)
	}

	// Clean up the resource
	err = manager.CleanupResource(resource.ID)
	if err != nil {
		t.Errorf("CleanupResource() failed: %v", err)
	}

	// Verify directory was removed
	if _, err := os.Stat(resource.Path); !os.IsNotExist(err) {
		t.Errorf("Temporary directory was not removed: %s", resource.Path)
	}

	// Verify resource was removed from manager
	_, err = manager.GetResource(resource.ID)
	if err == nil {
		t.Error("Resource should be removed from manager after cleanup")
	}

	// Try to cleanup non-existent resource
	err = manager.CleanupResource("non-existent")
	if err == nil {
		t.Error("CleanupResource() should return error for non-existent resource")
	}
}

func TestCleanupAll(t *testing.T) {
	tempDir := t.TempDir()
	manager := NewManager(tempDir)

	// Create multiple resources
	resource1, err := manager.CreateTempDir("test-command-1", true)
	if err != nil {
		t.Fatalf("CreateTempDir() failed: %v", err)
	}

	resource2, err := manager.CreateTempDir("test-command-2", false)
	if err != nil {
		t.Fatalf("CreateTempDir() failed: %v", err)
	}

	// Verify both directories exist
	if _, err := os.Stat(resource1.Path); os.IsNotExist(err) {
		t.Fatalf("Temporary directory 1 was not created: %s", resource1.Path)
	}
	if _, err := os.Stat(resource2.Path); os.IsNotExist(err) {
		t.Fatalf("Temporary directory 2 was not created: %s", resource2.Path)
	}

	// Clean up all resources
	err = manager.CleanupAll()
	if err != nil {
		t.Errorf("CleanupAll() failed: %v", err)
	}

	// Verify both directories were removed
	if _, err := os.Stat(resource1.Path); !os.IsNotExist(err) {
		t.Errorf("Temporary directory 1 was not removed: %s", resource1.Path)
	}
	if _, err := os.Stat(resource2.Path); !os.IsNotExist(err) {
		t.Errorf("Temporary directory 2 was not removed: %s", resource2.Path)
	}

	// Verify all resources were removed from manager
	resources := manager.ListResources()
	if len(resources) != 0 {
		t.Errorf("Resources should be empty after CleanupAll(), got %d", len(resources))
	}
}

func TestCleanupAutoCleanup(t *testing.T) {
	tempDir := t.TempDir()
	manager := NewManager(tempDir)

	// Create resources with different auto-cleanup settings
	resource1, err := manager.CreateTempDir("test-command-1", true) // auto-cleanup
	if err != nil {
		t.Fatalf("CreateTempDir() failed: %v", err)
	}

	resource2, err := manager.CreateTempDir("test-command-2", false) // no auto-cleanup
	if err != nil {
		t.Fatalf("CreateTempDir() failed: %v", err)
	}

	// Clean up auto-cleanup resources only
	err = manager.CleanupAutoCleanup()
	if err != nil {
		t.Errorf("CleanupAutoCleanup() failed: %v", err)
	}

	// Verify auto-cleanup resource was removed
	if _, err := os.Stat(resource1.Path); !os.IsNotExist(err) {
		t.Errorf("Auto-cleanup directory was not removed: %s", resource1.Path)
	}

	// Verify non-auto-cleanup resource still exists
	if _, err := os.Stat(resource2.Path); os.IsNotExist(err) {
		t.Errorf("Non-auto-cleanup directory was incorrectly removed: %s", resource2.Path)
	}

	// Verify only the non-auto-cleanup resource remains
	resources := manager.ListResources()
	if len(resources) != 1 {
		t.Errorf("Expected 1 resource after CleanupAutoCleanup(), got %d", len(resources))
	}
	if resources[0].ID != resource2.ID {
		t.Errorf("Wrong resource remaining after CleanupAutoCleanup()")
	}
}

func TestCleanupByCommand(t *testing.T) {
	tempDir := t.TempDir()
	manager := NewManager(tempDir)

	// Create resources for different commands
	resource1, err := manager.CreateTempDir("command-a", true)
	if err != nil {
		t.Fatalf("CreateTempDir() failed: %v", err)
	}

	resource2, err := manager.CreateTempDir("command-b", true)
	if err != nil {
		t.Fatalf("CreateTempDir() failed: %v", err)
	}

	resource3, err := manager.CreateTempDir("command-a", true)
	if err != nil {
		t.Fatalf("CreateTempDir() failed: %v", err)
	}

	// Clean up resources for command-a only
	err = manager.CleanupByCommand("command-a")
	if err != nil {
		t.Errorf("CleanupByCommand() failed: %v", err)
	}

	// Verify command-a resources were removed
	if _, err := os.Stat(resource1.Path); !os.IsNotExist(err) {
		t.Errorf("Command-a resource 1 was not removed: %s", resource1.Path)
	}
	if _, err := os.Stat(resource3.Path); !os.IsNotExist(err) {
		t.Errorf("Command-a resource 3 was not removed: %s", resource3.Path)
	}

	// Verify command-b resource still exists
	if _, err := os.Stat(resource2.Path); os.IsNotExist(err) {
		t.Errorf("Command-b resource was incorrectly removed: %s", resource2.Path)
	}
}

func TestGetResourcesForCommand(t *testing.T) {
	tempDir := t.TempDir()
	manager := NewManager(tempDir)

	// Create resources for different commands
	_, err := manager.CreateTempDir("command-a", true)
	if err != nil {
		t.Fatalf("CreateTempDir() failed: %v", err)
	}

	_, err = manager.CreateTempDir("command-b", true)
	if err != nil {
		t.Fatalf("CreateTempDir() failed: %v", err)
	}

	_, err = manager.CreateTempDir("command-a", true)
	if err != nil {
		t.Fatalf("CreateTempDir() failed: %v", err)
	}

	// Get resources for command-a
	resourcesA := manager.GetResourcesForCommand("command-a")
	if len(resourcesA) != 2 {
		t.Errorf("GetResourcesForCommand(command-a) length = %d, want 2", len(resourcesA))
	}

	// Get resources for command-b
	resourcesB := manager.GetResourcesForCommand("command-b")
	if len(resourcesB) != 1 {
		t.Errorf("GetResourcesForCommand(command-b) length = %d, want 1", len(resourcesB))
	}

	// Get resources for non-existent command
	resourcesC := manager.GetResourcesForCommand("command-c")
	if len(resourcesC) != 0 {
		t.Errorf("GetResourcesForCommand(command-c) length = %d, want 0", len(resourcesC))
	}
}

func TestCleanupOldResources(t *testing.T) {
	tempDir := t.TempDir()
	manager := NewManager(tempDir)

	// Create a resource
	resource, err := manager.CreateTempDir("test-command", true)
	if err != nil {
		t.Fatalf("CreateTempDir() failed: %v", err)
	}

	// Manually set creation time to past
	resource.Created = time.Now().Add(-2 * time.Hour)

	// Clean up resources older than 1 hour
	err = manager.CleanupOldResources(1 * time.Hour)
	if err != nil {
		t.Errorf("CleanupOldResources() failed: %v", err)
	}

	// Verify old resource was removed
	if _, err := os.Stat(resource.Path); !os.IsNotExist(err) {
		t.Errorf("Old resource was not removed: %s", resource.Path)
	}
}

func TestSanitizeName(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "simple name",
			input: "test",
			want:  "test",
		},
		{
			name:  "name with spaces",
			input: "test command",
			want:  "test_command",
		},
		{
			name:  "name with special characters",
			input: "test/command!@#",
			want:  "test_command___",
		},
		{
			name:  "empty name",
			input: "",
			want:  "unnamed",
		},
		{
			name:  "name with hyphens and underscores",
			input: "test-command_name",
			want:  "test-command_name",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sanitizeName(tt.input)
			if got != tt.want {
				t.Errorf("sanitizeName() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestApplyResourceToCommand(t *testing.T) {
	tempDir := t.TempDir()
	manager := NewManager(tempDir)

	cmd := &runnertypes.Command{
		Name: "test-command",
		Cmd:  "echo",
		Args: []string{"hello"},
		Dir:  "",
	}

	// Test with temp dir enabled
	err := manager.ApplyResourceToCommand(cmd, true)
	if err != nil {
		t.Errorf("ApplyResourceToCommand() failed: %v", err)
	}

	// Verify command directory was set
	if cmd.Dir == "" {
		t.Error("Command directory should be set when temp dir is enabled")
	}

	// Verify TEMP_DIR environment variable was added
	hasTempDir := false
	for _, env := range cmd.Env {
		if len(env) > 9 && env[:9] == "TEMP_DIR=" {
			hasTempDir = true
			break
		}
	}
	if !hasTempDir {
		t.Error("TEMP_DIR environment variable should be added")
	}

	// Test with temp dir disabled
	cmd2 := &runnertypes.Command{
		Name: "test-command-2",
		Cmd:  "echo",
		Args: []string{"hello"},
		Dir:  "",
	}

	err = manager.ApplyResourceToCommand(cmd2, false)
	if err != nil {
		t.Errorf("ApplyResourceToCommand() with disabled temp dir failed: %v", err)
	}

	// Verify no changes were made
	if cmd2.Dir != "" {
		t.Error("Command directory should not be changed when temp dir is disabled")
	}
}
