// Package resource provides functionality for managing resources like
// temporary directories, file cleanup, and resource lifecycle management.
package resource

import (
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/isseis/go-safe-cmd-runner/internal/common"
)

// Error definitions for the resource package
var (
	// ErrResourceNotFound is returned when a requested resource is not found
	ErrResourceNotFound = errors.New("resource not found")
	// ErrResourceAlreadyExists is returned when trying to create a resource that already exists
	ErrResourceAlreadyExists = errors.New("resource already exists")
	// ErrCleanupFailed is returned when resource cleanup fails
	ErrCleanupFailed = errors.New("resource cleanup failed")
)

// Resource represents a managed resource
type Resource struct {
	ID          string    `json:"id"`
	Path        string    `json:"path"`
	Created     time.Time `json:"created"`
	AutoCleanup bool      `json:"auto_cleanup"`
	Command     string    `json:"command"` // Associated command name
}

// Manager handles resource lifecycle management
type Manager struct {
	resources map[string]*Resource
	mu        sync.RWMutex
	baseDir   string
	fs        common.FileSystem
}

// NewManager creates a new resource manager
func NewManager(baseDir string) *Manager {
	return NewManagerWithFS(baseDir, common.NewDefaultFileSystem())
}

// NewManagerWithFS creates a new resource manager with a custom FileSystem
func NewManagerWithFS(baseDir string, fs common.FileSystem) *Manager {
	if baseDir == "" {
		baseDir = os.TempDir()
	}
	return &Manager{
		resources: make(map[string]*Resource),
		baseDir:   baseDir,
		fs:        fs,
	}
}

// CreateTempDir creates a temporary directory for a command
func (m *Manager) CreateTempDir(commandName string, autoCleanup bool) (*Resource, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Generate a unique ID for the resource using UUID
	uuidStr := uuid.New().String()
	resourceID := fmt.Sprintf("tempdir_%s_%s", commandName, uuidStr)

	// Check if resource already exists
	if _, exists := m.resources[resourceID]; exists {
		return nil, fmt.Errorf("%w: %s", ErrResourceAlreadyExists, resourceID)
	}

	// Create the directory
	tempDirPath, err := m.fs.CreateTempDir(m.baseDir, resourceID)
	if err != nil {
		return nil, fmt.Errorf("failed to create temporary directory: %w", err)
	}

	// Create the resource
	resource := &Resource{
		ID:          resourceID,
		Path:        tempDirPath,
		Created:     time.Now(),
		AutoCleanup: autoCleanup,
		Command:     commandName,
	}

	m.resources[resourceID] = resource
	return resource, nil
}

// GetResource retrieves a resource by ID
func (m *Manager) GetResource(id string) (*Resource, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	resource, exists := m.resources[id]
	if !exists {
		return nil, fmt.Errorf("%w: %s", ErrResourceNotFound, id)
	}

	return resource, nil
}

// ListResources returns all managed resources
func (m *Manager) ListResources() []*Resource {
	m.mu.RLock()
	defer m.mu.RUnlock()

	resources := make([]*Resource, 0, len(m.resources))
	for _, resource := range m.resources {
		resources = append(resources, resource)
	}

	return resources
}

// cleanupResources is a helper function that cleans up resources based on a filter function
// The filter function should return true for resources that should be cleaned up
func (m *Manager) cleanupResources(filter func(id string, r *Resource) bool, errorMsg string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	var errs []error
	for id, resource := range m.resources {
		if filter == nil || filter(id, resource) {
			if err := m.cleanupResourceUnsafe(id); err != nil {
				errs = append(errs, err)
			}
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("%w: %s: %d resources failed to cleanup", ErrCleanupFailed, errorMsg, len(errs))
	}

	return nil
}

// CleanupResource cleans up a specific resource
func (m *Manager) CleanupResource(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if err := m.cleanupResourceUnsafe(id); err != nil {
		if errors.Is(err, ErrResourceNotFound) {
			return err
		}
		return fmt.Errorf("%w: %v", ErrCleanupFailed, err)
	}
	return nil
}

// CleanupAll cleans up all managed resources
func (m *Manager) CleanupAll() error {
	return m.cleanupResources(nil, "")
}

// cleanupResourceUnsafe cleans up a resource without locking (internal use)
func (m *Manager) cleanupResourceUnsafe(id string) error {
	resource, exists := m.resources[id]
	if !exists {
		return fmt.Errorf("%w: %s", ErrResourceNotFound, id)
	}

	// Since all resources are temporary directories, use RemoveAll
	err := m.fs.RemoveAll(resource.Path)
	if err != nil {
		return fmt.Errorf("failed to cleanup resource %s: %w", id, err)
	}

	delete(m.resources, id)
	return nil
}
