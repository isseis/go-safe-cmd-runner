package executor_test

import (
	"context"
	"log"
	"os"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/executor"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/stretchr/testify/assert"
)

type mockFileSystem struct{}

func (m *mockFileSystem) CreateTempDir(prefix string) (string, error) {
	return os.MkdirTemp("", prefix)
}

func (m *mockFileSystem) RemoveAll(path string) error {
	return os.RemoveAll(path)
}

func (m *mockFileSystem) FileExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	return err == nil, err
}

type mockOutputWriter struct {
	outputs []string
}

func (m *mockOutputWriter) Write(_ string, data []byte) error {
	m.outputs = append(m.outputs, string(data))
	return nil
}

func (m *mockOutputWriter) Close() error {
	return nil
}

type mockEnvManager struct{}

func (m *mockEnvManager) LoadFromFile(_ string) (map[string]string, error) {
	return map[string]string{"FROM_FILE": "value"}, nil
}

func (m *mockEnvManager) Merge(envs ...map[string]string) map[string]string {
	result := make(map[string]string)
	for _, env := range envs {
		for k, v := range env {
			result[k] = v
		}
	}
	return result
}

func (m *mockEnvManager) Resolve(s string, _ map[string]string) (string, error) {
	return s, nil
}

func TestNewDefaultExecutor(t *testing.T) {
	exec := executor.NewDefaultExecutor()
	assert.NotNil(t, exec, "NewDefaultExecutor should return a non-nil executor")
}

func TestExecute_Success(t *testing.T) {
	tests := []struct {
		name    string
		cmd     runnertypes.Command
		env     map[string]string
		wantErr bool
	}{
		{
			name: "simple command",
			cmd: runnertypes.Command{
				Cmd:  "echo",
				Args: []string{"hello"},
			},
			env:     map[string]string{"TEST": "value"},
			wantErr: false,
		},
		{
			name: "command with working directory",
			cmd: runnertypes.Command{
				Cmd:  "pwd",
				Dir:  ".",
				Args: []string{},
			},
			env:     nil,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := &executor.DefaultExecutor{
				FS:  &mockFileSystem{},
				Out: &mockOutputWriter{},
				Env: &mockEnvManager{},
			}

			_, err := e.Execute(context.Background(), tt.cmd, tt.env)
			if tt.wantErr {
				assert.Error(t, err, "Expected error but got none")
			} else {
				assert.NoError(t, err, "Unexpected error")
			}
		})
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		cmd     runnertypes.Command
		wantErr bool
	}{
		{
			name: "empty command",
			cmd: runnertypes.Command{
				Cmd: "",
			},
			wantErr: true,
		},
		{
			name: "valid command",
			cmd: runnertypes.Command{
				Cmd:  "echo",
				Args: []string{"hello"},
			},
			wantErr: false,
		},
		{
			name: "invalid directory",
			cmd: runnertypes.Command{
				Cmd: "ls",
				Dir: "/nonexistent/directory",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := &executor.DefaultExecutor{
				FS:  &mockFileSystem{},
				Out: &mockOutputWriter{},
				Env: &mockEnvManager{},
			}

			err := e.Validate(tt.cmd)
			if tt.wantErr {
				assert.Error(t, err, "Expected error but got none")
			} else {
				assert.NoError(t, err, "Unexpected error")
			}
		})
	}
}

func TestMain(m *testing.M) {
	// Setup: Create a temporary directory for tests
	tempDir, err := os.MkdirTemp("", "executor-test-*")
	if err != nil {
		log.Fatalf("Failed to create temp dir: %v", err)
	}

	// Run tests
	code := m.Run()

	// Cleanup
	err = os.RemoveAll(tempDir)
	if err != nil {
		log.Printf("Failed to remove temp dir: %v", err)
	}

	os.Exit(code)
}
