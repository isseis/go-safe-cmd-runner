package resource

import (
	"strings"
	"testing"
	"time"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
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

func TestNewManagerWithFS(t *testing.T) {
	mockFS := common.NewMockFileSystem()
	manager := NewManagerWithFS("/tmp", mockFS)

	if manager == nil {
		t.Fatal("NewManagerWithFS() returned nil")
	}
	if manager.fs != mockFS {
		t.Error("Manager filesystem should be set to provided filesystem")
	}
}

func TestCreateTempDir(t *testing.T) {
	mockFS := common.NewMockFileSystem()
	manager := NewManagerWithFS("/tmp", mockFS)

	resource, err := manager.CreateTempDir("test-command", true)
	if err != nil {
		t.Fatalf("CreateTempDir() failed: %v", err)
	}

	if resource == nil {
		t.Fatal("CreateTempDir() returned nil resource")
	}

	if resource.Command != "test-command" {
		t.Errorf("Resource command = %v, want test-command", resource.Command)
	}

	if !resource.AutoCleanup {
		t.Error("Resource AutoCleanup should be true")
	}

	// Check that directory was actually created in mock filesystem
	exists, err := mockFS.FileExists(resource.Path)
	if err != nil {
		t.Fatalf("FileExists failed: %v", err)
	}
	if !exists {
		t.Errorf("Temporary directory was not created: %s", resource.Path)
	}

	// Check that path is under the base directory
	if !strings.HasPrefix(resource.Path, "/tmp") {
		t.Errorf("Resource path %s is not under base directory %s", resource.Path, "/tmp")
	}
}

func TestGetResource(t *testing.T) {
	mockFS := common.NewMockFileSystem()
	manager := NewManagerWithFS("/tmp", mockFS)

	// Create a resource
	resource, err := manager.CreateTempDir("test-command", true)
	if err != nil {
		t.Fatalf("CreateTempDir() failed: %v", err)
	}

	// Retrieve the resource
	retrieved, err := manager.GetResource(resource.ID)
	if err != nil {
		t.Fatalf("GetResource() failed: %v", err)
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
	mockFS := common.NewMockFileSystem()
	manager := NewManagerWithFS("/tmp", mockFS)

	// Initially should be empty
	resources := manager.ListResources()
	if len(resources) != 0 {
		t.Errorf("ListResources() length = %d, want 0", len(resources))
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
	mockFS := common.NewMockFileSystem()
	manager := NewManagerWithFS("/tmp", mockFS)

	// Create a resource
	resource, err := manager.CreateTempDir("test-command", true)
	if err != nil {
		t.Fatalf("CreateTempDir() failed: %v", err)
	}

	// Verify directory exists
	exists, err := mockFS.FileExists(resource.Path)
	if err != nil {
		t.Fatalf("FileExists failed: %v", err)
	}
	if !exists {
		t.Fatalf("Temporary directory was not created: %s", resource.Path)
	}

	// Clean up the resource
	err = manager.CleanupResource(resource.ID)
	if err != nil {
		t.Errorf("CleanupResource() failed: %v", err)
	}

	// Verify directory was removed
	exists, err = mockFS.FileExists(resource.Path)
	if err != nil {
		t.Fatalf("FileExists failed: %v", err)
	}
	if exists {
		t.Errorf("Resource directory should have been removed: %s", resource.Path)
	}

	// Verify resource was removed from manager
	_, err = manager.GetResource(resource.ID)
	if err == nil {
		t.Error("Resource should have been removed from manager")
	}

	// Try to cleanup non-existent resource
	err = manager.CleanupResource("non-existent")
	if err == nil {
		t.Error("CleanupResource() should return error for non-existent resource")
	}
}

func TestCleanupAll(t *testing.T) {
	mockFS := common.NewMockFileSystem()
	manager := NewManagerWithFS("/tmp", mockFS)

	// Create multiple resources
	resource1, err := manager.CreateTempDir("test-command-1", true)
	if err != nil {
		t.Fatalf("CreateTempDir() failed: %v", err)
	}

	resource2, err := manager.CreateTempDir("test-command-2", false)
	if err != nil {
		t.Fatalf("CreateTempDir() failed: %v", err)
	}

	// Verify directories exist
	exists, err := mockFS.FileExists(resource1.Path)
	if err != nil {
		t.Fatalf("FileExists failed: %v", err)
	}
	if !exists {
		t.Errorf("Resource1 directory should exist: %s", resource1.Path)
	}

	exists, err = mockFS.FileExists(resource2.Path)
	if err != nil {
		t.Fatalf("FileExists failed: %v", err)
	}
	if !exists {
		t.Errorf("Resource2 directory should exist: %s", resource2.Path)
	}

	// Clean up all resources
	err = manager.CleanupAll()
	if err != nil {
		t.Errorf("CleanupAll() failed: %v", err)
	}

	// Verify directories were removed
	exists, err = mockFS.FileExists(resource1.Path)
	if err != nil {
		t.Fatalf("FileExists failed: %v", err)
	}
	if exists {
		t.Errorf("Resource1 directory should have been removed: %s", resource1.Path)
	}

	exists, err = mockFS.FileExists(resource2.Path)
	if err != nil {
		t.Fatalf("FileExists failed: %v", err)
	}
	if exists {
		t.Errorf("Resource2 directory should have been removed: %s", resource2.Path)
	}

	// Verify all resources were removed from manager
	resources := manager.ListResources()
	if len(resources) != 0 {
		t.Errorf("Resources should be empty after CleanupAll(), got %d", len(resources))
	}
}

func TestCleanupAutoCleanup(t *testing.T) {
	mockFS := common.NewMockFileSystem()
	manager := NewManagerWithFS("/tmp", mockFS)

	// Create resources with different auto-cleanup settings
	resource1, err := manager.CreateTempDir("test-command-1", true) // auto-cleanup
	if err != nil {
		t.Fatalf("CreateTempDir() failed: %v", err)
	}

	resource2, err := manager.CreateTempDir("test-command-2", false) // no auto-cleanup
	if err != nil {
		t.Fatalf("CreateTempDir() failed: %v", err)
	}

	// Clean up auto-cleanup resources
	err = manager.CleanupAutoCleanup()
	if err != nil {
		t.Errorf("CleanupAutoCleanup() failed: %v", err)
	}

	// Verify auto-cleanup resource was removed
	exists, err := mockFS.FileExists(resource1.Path)
	if err != nil {
		t.Fatalf("FileExists failed: %v", err)
	}
	if exists {
		t.Errorf("Auto-cleanup resource should have been removed: %s", resource1.Path)
	}

	// Verify non-auto-cleanup resource still exists
	exists, err = mockFS.FileExists(resource2.Path)
	if err != nil {
		t.Fatalf("FileExists failed: %v", err)
	}
	if !exists {
		t.Errorf("Non-auto-cleanup resource should still exist: %s", resource2.Path)
	}

	// Verify only non-auto-cleanup resource remains
	resources := manager.ListResources()
	if len(resources) != 1 {
		t.Errorf("Expected 1 resource after CleanupAutoCleanup(), got %d", len(resources))
	}
	if resources[0].ID != resource2.ID {
		t.Errorf("Wrong resource remaining after CleanupAutoCleanup()")
	}
}

func TestCleanupByCommand(t *testing.T) {
	mockFS := common.NewMockFileSystem()
	manager := NewManagerWithFS("/tmp", mockFS)

	// Create resources for different commands
	resource1, err := manager.CreateTempDir("command-a", true)
	if err != nil {
		t.Fatalf("CreateTempDir() failed: %v", err)
	}

	resource2, err := manager.CreateTempDir("command-b", true)
	if err != nil {
		t.Fatalf("CreateTempDir() failed: %v", err)
	}

	resource3, err := manager.CreateTempDir("command-a", false)
	if err != nil {
		t.Fatalf("CreateTempDir() failed: %v", err)
	}

	// Clean up resources for command-a
	err = manager.CleanupByCommand("command-a")
	if err != nil {
		t.Errorf("CleanupByCommand() failed: %v", err)
	}

	// Verify command-a resources were removed
	exists, err := mockFS.FileExists(resource1.Path)
	if err != nil {
		t.Fatalf("FileExists failed: %v", err)
	}
	if exists {
		t.Errorf("Command-a resource1 should have been removed: %s", resource1.Path)
	}

	exists, err = mockFS.FileExists(resource3.Path)
	if err != nil {
		t.Fatalf("FileExists failed: %v", err)
	}
	if exists {
		t.Errorf("Command-a resource3 should have been removed: %s", resource3.Path)
	}

	// Verify command-b resource still exists
	exists, err = mockFS.FileExists(resource2.Path)
	if err != nil {
		t.Fatalf("FileExists failed: %v", err)
	}
	if !exists {
		t.Errorf("Command-b resource should still exist: %s", resource2.Path)
	}
}

func TestGetResourcesForCommand(t *testing.T) {
	mockFS := common.NewMockFileSystem()
	manager := NewManagerWithFS("/tmp", mockFS)

	// Create resources for different commands
	_, err := manager.CreateTempDir("command-a", true)
	if err != nil {
		t.Fatalf("CreateTempDir() failed: %v", err)
	}

	_, err = manager.CreateTempDir("command-b", true)
	if err != nil {
		t.Fatalf("CreateTempDir() failed: %v", err)
	}

	_, err = manager.CreateTempDir("command-a", false)
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
	mockFS := common.NewMockFileSystem()
	manager := NewManagerWithFS("/tmp", mockFS)

	// Create a resource
	resource, err := manager.CreateTempDir("test-command", true)
	if err != nil {
		t.Fatalf("CreateTempDir() failed: %v", err)
	}

	// Simulate old resource by modifying creation time
	resource.Created = time.Now().Add(-2 * time.Hour)
	manager.resources[resource.ID] = resource

	// Clean up resources older than 1 hour
	err = manager.CleanupOldResources(1 * time.Hour)
	if err != nil {
		t.Errorf("CleanupOldResources() failed: %v", err)
	}

	// Verify old resource was removed
	exists, err := mockFS.FileExists(resource.Path)
	if err != nil {
		t.Fatalf("FileExists failed: %v", err)
	}
	if exists {
		t.Errorf("Old resource should have been removed: %s", resource.Path)
	}

	// Verify resource was removed from manager
	resources := manager.ListResources()
	if len(resources) != 0 {
		t.Errorf("Resources should be empty after CleanupOldResources(), got %d", len(resources))
	}
}
