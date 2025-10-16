// Package config provides tests for verify_files path expansion functionality.
package config_test

import (
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/config"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/environment"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestVerifyFilesExpansion_Global tests global-level verify_files path expansion
func TestVerifyFilesExpansion_Global(t *testing.T) {
	tests := []struct {
		name        string
		verifyFiles []string
		vars        []string
		fromEnv     []string
		allowlist   []string
		systemEnv   map[string]string
		expected    []string
		expectError bool
	}{
		{
			name:        "Literal paths - no variable references",
			verifyFiles: []string{"/etc/config.toml", "/var/log/app.log"},
			vars:        []string{},
			fromEnv:     []string{},
			allowlist:   []string{},
			systemEnv:   map[string]string{},
			expected:    []string{"/etc/config.toml", "/var/log/app.log"},
			expectError: false,
		},
		{
			name:        "Single variable reference",
			verifyFiles: []string{"/path/%{dir}/file.txt"},
			vars:        []string{"dir=data"},
			fromEnv:     []string{},
			allowlist:   []string{},
			systemEnv:   map[string]string{},
			expected:    []string{"/path/data/file.txt"},
			expectError: false,
		},
		{
			name:        "Multiple variable references",
			verifyFiles: []string{"%{base}/%{subdir}/%{filename}"},
			vars:        []string{"base=/root", "subdir=config", "filename=app.toml"},
			fromEnv:     []string{},
			allowlist:   []string{},
			systemEnv:   map[string]string{},
			expected:    []string{"/root/config/app.toml"},
			expectError: false,
		},
		{
			name:        "Variable references from system environment",
			verifyFiles: []string{"%{home}/config.toml"},
			vars:        []string{},
			fromEnv:     []string{"home=HOME"},
			allowlist:   []string{"HOME"},
			systemEnv:   map[string]string{"HOME": "/home/user"},
			expected:    []string{"/home/user/config.toml"},
			expectError: false,
		},
		{
			name:        "Multiple paths with different variables",
			verifyFiles: []string{"%{path1}/file1", "%{path2}/file2"},
			vars:        []string{"path1=/etc", "path2=/var"},
			fromEnv:     []string{},
			allowlist:   []string{},
			systemEnv:   map[string]string{},
			expected:    []string{"/etc/file1", "/var/file2"},
			expectError: false,
		},
		{
			name:        "Relative paths",
			verifyFiles: []string{"./config/%{env}.toml", "../data/%{file}"},
			vars:        []string{"env=prod", "file=data.json"},
			fromEnv:     []string{},
			allowlist:   []string{},
			systemEnv:   map[string]string{},
			expected:    []string{"./config/prod.toml", "../data/data.json"},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up system environment variables
			for key, value := range tt.systemEnv {
				t.Setenv(key, value)
			}

			// Create global config
			global := &runnertypes.GlobalConfig{
				Vars:         tt.vars,
				FromEnv:      tt.fromEnv,
				VerifyFiles:  tt.verifyFiles,
				EnvAllowlist: tt.allowlist,
			}

			// Create environment filter
			filter := environment.NewFilter(global.EnvAllowlist)

			// Expand global config
			err := config.ExpandGlobalConfig(global, filter)

			if tt.expectError {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.expected, global.ExpandedVerifyFiles)
		})
	}
}

// TestVerifyFilesExpansion_Group tests group-level verify_files path expansion
func TestVerifyFilesExpansion_Group(t *testing.T) {
	tests := []struct {
		name          string
		verifyFiles   []string
		groupVars     []string
		globalVars    []string
		globalFromEnv []string
		allowlist     []string
		systemEnv     map[string]string
		expected      []string
		expectError   bool
	}{
		{
			name:          "Group vars used for path expansion",
			verifyFiles:   []string{"%{grp_dir}/file.txt"},
			groupVars:     []string{"grp_dir=/group/data"},
			globalVars:    []string{},
			globalFromEnv: []string{},
			allowlist:     []string{},
			systemEnv:     map[string]string{},
			expected:      []string{"/group/data/file.txt"},
			expectError:   false,
		},
		{
			name:          "Inherited global vars for path expansion",
			verifyFiles:   []string{"%{global_base}/%{filename}"},
			groupVars:     []string{"filename=app.conf"},
			globalVars:    []string{"global_base=/etc"},
			globalFromEnv: []string{},
			allowlist:     []string{},
			systemEnv:     map[string]string{},
			expected:      []string{"/etc/app.conf"},
			expectError:   false,
		},
		{
			name:          "Group vars override global vars",
			verifyFiles:   []string{"%{dir}/file.txt"},
			groupVars:     []string{"dir=/group/override"},
			globalVars:    []string{"dir=/global/base"},
			globalFromEnv: []string{},
			allowlist:     []string{},
			systemEnv:     map[string]string{},
			expected:      []string{"/group/override/file.txt"},
			expectError:   false,
		},
		{
			name:          "Expansion from system environment variables",
			verifyFiles:   []string{"%{user_home}/config/%{app_name}.toml"},
			groupVars:     []string{"app_name=myapp"},
			globalVars:    []string{},
			globalFromEnv: []string{"user_home=HOME"},
			allowlist:     []string{"HOME"},
			systemEnv:     map[string]string{"HOME": "/home/testuser"},
			expected:      []string{"/home/testuser/config/myapp.toml"},
			expectError:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up system environment variables
			for key, value := range tt.systemEnv {
				t.Setenv(key, value)
			}

			// Create config
			cfg := &runnertypes.Config{
				Global: runnertypes.GlobalConfig{
					Vars:         tt.globalVars,
					FromEnv:      tt.globalFromEnv,
					EnvAllowlist: tt.allowlist,
				},
				Groups: []runnertypes.CommandGroup{
					{
						Name:        "test_group",
						Vars:        tt.groupVars,
						VerifyFiles: tt.verifyFiles,
					},
				},
			}

			// Create environment filter
			filter := environment.NewFilter(cfg.Global.EnvAllowlist)

			// Expand global config first
			err := config.ExpandGlobalConfig(&cfg.Global, filter)
			require.NoError(t, err)

			// Expand group config
			err = config.ExpandGroupConfig(&cfg.Groups[0], &cfg.Global, filter)

			if tt.expectError {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.expected, cfg.Groups[0].ExpandedVerifyFiles)
		})
	}
}

// TestVerifyFilesExpansion_NestedReferences tests nested variable references
func TestVerifyFilesExpansion_NestedReferences(t *testing.T) {
	tests := []struct {
		name        string
		verifyFiles []string
		vars        []string
		fromEnv     []string
		allowlist   []string
		systemEnv   map[string]string
		expected    []string
	}{
		{
			name:        "2-level nested references",
			verifyFiles: []string{"%{path}/file.txt"},
			vars:        []string{"base=/root", "path=%{base}/subdir"},
			fromEnv:     []string{},
			allowlist:   []string{},
			systemEnv:   map[string]string{},
			expected:    []string{"/root/subdir/file.txt"},
		},
		{
			name:        "3-level nested references",
			verifyFiles: []string{"%{full_path}"},
			vars:        []string{"base=/etc", "file=config.toml", "mid=%{base}/subdir", "full_path=%{mid}/%{file}"},
			fromEnv:     []string{},
			allowlist:   []string{},
			systemEnv:   map[string]string{},
			expected:    []string{"/etc/subdir/config.toml"},
		},
		{
			name:        "Nested references including system env variables",
			verifyFiles: []string{"%{app_path}/config.toml"},
			vars:        []string{"app_path=%{home}/apps/myapp"},
			fromEnv:     []string{"home=HOME"},
			allowlist:   []string{"HOME"},
			systemEnv:   map[string]string{"HOME": "/home/user"},
			expected:    []string{"/home/user/apps/myapp/config.toml"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up system environment variables
			for key, value := range tt.systemEnv {
				t.Setenv(key, value)
			}

			// Create global config
			global := &runnertypes.GlobalConfig{
				Vars:         tt.vars,
				FromEnv:      tt.fromEnv,
				VerifyFiles:  tt.verifyFiles,
				EnvAllowlist: tt.allowlist,
			}

			// Create environment filter
			filter := environment.NewFilter(global.EnvAllowlist)

			// Expand global config
			err := config.ExpandGlobalConfig(global, filter)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, global.ExpandedVerifyFiles)
		})
	}
}

// TestVerifyFilesExpansion_SpecialCharacters tests paths with special characters
func TestVerifyFilesExpansion_SpecialCharacters(t *testing.T) {
	tests := []struct {
		name        string
		verifyFiles []string
		vars        []string
		expected    []string
	}{
		{
			name:        "Path containing spaces",
			verifyFiles: []string{"%{dir}/my file.txt"},
			vars:        []string{"dir=/path/with spaces"},
			expected:    []string{"/path/with spaces/my file.txt"},
		},
		{
			name:        "Path containing special characters",
			verifyFiles: []string{"%{base}/@file-name_123.conf"},
			vars:        []string{"base=/etc/app"},
			expected:    []string{"/etc/app/@file-name_123.conf"},
		},
		{
			name:        "Path containing Japanese characters",
			verifyFiles: []string{"%{dir}/設定.toml"},
			vars:        []string{"dir=/home/ユーザー"},
			expected:    []string{"/home/ユーザー/設定.toml"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create global config
			global := &runnertypes.GlobalConfig{
				Vars:        tt.vars,
				VerifyFiles: tt.verifyFiles,
			}

			// Create environment filter
			filter := environment.NewFilter([]string{})

			// Expand global config
			err := config.ExpandGlobalConfig(global, filter)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, global.ExpandedVerifyFiles)
		})
	}
}

// TestVerifyFilesExpansion_ErrorHandling tests error cases
func TestVerifyFilesExpansion_ErrorHandling(t *testing.T) {
	tests := []struct {
		name        string
		verifyFiles []string
		vars        []string
		fromEnv     []string
		allowlist   []string
		systemEnv   map[string]string
		expectError bool
		errorCheck  func(*testing.T, error)
	}{
		{
			name:        "Undefined variable reference",
			verifyFiles: []string{"/path/%{undefined}/file"},
			vars:        []string{},
			fromEnv:     []string{},
			allowlist:   []string{},
			systemEnv:   map[string]string{},
			expectError: true,
			errorCheck: func(t *testing.T, err error) {
				assert.ErrorIs(t, err, config.ErrUndefinedVariable)
			},
		},
		{
			name:        "Unclosed variable reference",
			verifyFiles: []string{"/path/%{unclosed/file"},
			vars:        []string{},
			fromEnv:     []string{},
			allowlist:   []string{},
			systemEnv:   map[string]string{},
			expectError: true,
			errorCheck: func(t *testing.T, err error) {
				assert.ErrorIs(t, err, config.ErrUnclosedVariableReference)
			},
		},
		{
			name:        "Empty path",
			verifyFiles: []string{""},
			vars:        []string{},
			fromEnv:     []string{},
			allowlist:   []string{},
			systemEnv:   map[string]string{},
			expectError: false, // Empty path is allowed (will be validated elsewhere)
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up system environment variables
			for key, value := range tt.systemEnv {
				t.Setenv(key, value)
			}

			// Create global config
			global := &runnertypes.GlobalConfig{
				Vars:         tt.vars,
				FromEnv:      tt.fromEnv,
				VerifyFiles:  tt.verifyFiles,
				EnvAllowlist: tt.allowlist,
			}

			// Create environment filter
			filter := environment.NewFilter(global.EnvAllowlist)

			// Expand global config
			err := config.ExpandGlobalConfig(global, filter)

			if tt.expectError {
				require.Error(t, err)
				if tt.errorCheck != nil {
					tt.errorCheck(t, err)
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestVerifyFilesExpansion_EdgeCases tests edge cases
func TestVerifyFilesExpansion_EdgeCases(t *testing.T) {
	tests := []struct {
		name        string
		verifyFiles []string
		vars        []string
		expected    []string
	}{
		{
			name:        "Extremely long path",
			verifyFiles: []string{"%{base}/very/long/path/with/many/directories/and/subdirectories/file.txt"},
			vars:        []string{"base=/root"},
			expected:    []string{"/root/very/long/path/with/many/directories/and/subdirectories/file.txt"},
		},
		{
			name: "Multiple verify_files entries",
			verifyFiles: []string{
				"%{base}/file1.txt",
				"%{base}/file2.txt",
				"%{base}/file3.txt",
				"%{base}/file4.txt",
				"%{base}/file5.txt",
			},
			vars: []string{"base=/data"},
			expected: []string{
				"/data/file1.txt",
				"/data/file2.txt",
				"/data/file3.txt",
				"/data/file4.txt",
				"/data/file5.txt",
			},
		},
		{
			name:        "Path composed only of variables",
			verifyFiles: []string{"%{full_path}"},
			vars:        []string{"full_path=/etc/config.toml"},
			expected:    []string{"/etc/config.toml"},
		},
		{
			name:        "Concatenated variable references",
			verifyFiles: []string{"%{dir1}%{dir2}/file"},
			vars:        []string{"dir1=/root", "dir2=/subdir"},
			expected:    []string{"/root/subdir/file"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create global config
			global := &runnertypes.GlobalConfig{
				Vars:        tt.vars,
				VerifyFiles: tt.verifyFiles,
			}

			// Create environment filter
			filter := environment.NewFilter([]string{})

			// Expand global config
			err := config.ExpandGlobalConfig(global, filter)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, global.ExpandedVerifyFiles)
		})
	}
}
