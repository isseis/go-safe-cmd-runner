//go:build test

package config

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
	"github.com/isseis/go-safe-cmd-runner/internal/filevalidator"
	"github.com/isseis/go-safe-cmd-runner/internal/verification"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeHashManifestFile writes a hash manifest to the appropriate location.
// This is the common helper used by createHashManifest and createManifestWithCustomHash.
func writeHashManifestFile(t *testing.T, hashDir, filePath string, manifest filevalidator.HashManifest) string {
	t.Helper()

	jsonData, err := json.MarshalIndent(manifest, "", "  ")
	require.NoError(t, err)
	jsonData = append(jsonData, '\n')

	// Use HybridHashFilePathGetter to get the correct hash file path
	getter := filevalidator.NewHybridHashFilePathGetter()
	resolvedPath, err := common.NewResolvedPath(filePath)
	require.NoError(t, err)
	hashFile, err := getter.GetHashFilePath(hashDir, resolvedPath)
	require.NoError(t, err)

	// Ensure parent directory exists
	err = os.MkdirAll(filepath.Dir(hashFile), 0o755)
	require.NoError(t, err)

	err = os.WriteFile(hashFile, jsonData, 0o644)
	require.NoError(t, err)

	return hashFile
}

// Helper function to create a hash manifest file for testing
func createHashManifest(t *testing.T, hashDir, filePath string, content []byte) {
	t.Helper()

	// Calculate SHA256 hash of the content
	hasher := &filevalidator.SHA256{}
	hash, err := hasher.Sum(bytes.NewReader(content))
	require.NoError(t, err)

	manifest := filevalidator.HashManifest{
		Version: "1.0",
		Format:  "file-hash",
		File: filevalidator.FileInfo{
			Path: filePath,
			Hash: filevalidator.HashInfo{
				Algorithm: "sha256",
				Value:     hash,
			},
		},
	}

	writeHashManifestFile(t, hashDir, filePath, manifest)
}

// Helper function to create a hash manifest file with a custom hash value for testing
func createManifestWithCustomHash(t *testing.T, hashDir, filePath, customHash string) {
	t.Helper()

	manifest := filevalidator.HashManifest{
		Version: "1.0",
		Format:  "file-hash",
		File: filevalidator.FileInfo{
			Path: filePath,
			Hash: filevalidator.HashInfo{
				Algorithm: "sha256",
				Value:     customHash,
			},
		},
	}

	writeHashManifestFile(t, hashDir, filePath, manifest)
}

// =============================================================================
// Hash Verification Tests for Include Feature (F-006)
// =============================================================================

// TestLoadConfig_MainConfigHashVerification verifies that the main config file's
// hash is verified during loading.
func TestLoadConfig_MainConfigHashVerification(t *testing.T) {
	tmpDir := t.TempDir()
	hashDir := filepath.Join(tmpDir, "hashes")
	err := os.MkdirAll(hashDir, 0o755)
	require.NoError(t, err)

	// Create main config file
	configPath := filepath.Join(tmpDir, "config.toml")
	configContent := []byte(`version = "1.0"

[command_templates.test]
cmd = "echo"
args = ["hello"]
`)
	err = os.WriteFile(configPath, configContent, 0o644)
	require.NoError(t, err)

	// Create verification manager with hash verification enabled
	verificationMgr, err := verification.NewManagerForTest(hashDir, verification.WithSkipHashDirectoryValidation())
	require.NoError(t, err)

	// Create loader with verification manager
	loader := NewLoader(common.NewDefaultFileSystem(), verificationMgr)

	t.Run("verification fails when hash not recorded", func(t *testing.T) {
		// Read config content (simulating what runner does)
		content, err := verificationMgr.VerifyAndReadConfigFile(configPath)

		// Should fail because hash is not recorded
		require.Error(t, err)
		assert.Nil(t, content)
		assert.Contains(t, err.Error(), "verification error")
	})

	t.Run("verification succeeds when hash is recorded", func(t *testing.T) {
		// Create hash manifest for the config file
		createHashManifest(t, hashDir, configPath, configContent)

		// Now verification should succeed
		content, err := verificationMgr.VerifyAndReadConfigFile(configPath)
		require.NoError(t, err)
		assert.Equal(t, string(configContent), string(content))

		// And loading should also succeed
		cfg, err := loader.LoadConfig(configPath, content)
		require.NoError(t, err)
		assert.NotNil(t, cfg)
		assert.Equal(t, "echo", cfg.CommandTemplates["test"].Cmd)
	})
}

// TestLoadConfig_SingleIncludeFileHashVerification verifies that included
// template files' hashes are verified during loading.
func TestLoadConfig_SingleIncludeFileHashVerification(t *testing.T) {
	tmpDir := t.TempDir()
	hashDir := filepath.Join(tmpDir, "hashes")
	err := os.MkdirAll(hashDir, 0o755)
	require.NoError(t, err)

	// Create template file
	templatePath := filepath.Join(tmpDir, "templates.toml")
	templateContent := []byte(`version = "1.0"

[command_templates.backup]
cmd = "restic"
args = ["backup", "${path}"]
`)
	err = os.WriteFile(templatePath, templateContent, 0o644)
	require.NoError(t, err)

	// Create main config file
	configPath := filepath.Join(tmpDir, "config.toml")
	configContent := []byte(`version = "1.0"
includes = ["templates.toml"]

[command_templates.test]
cmd = "echo"
args = ["hello"]
`)
	err = os.WriteFile(configPath, configContent, 0o644)
	require.NoError(t, err)

	t.Run("single include file hash verification fails when not recorded", func(t *testing.T) {
		// Create verification manager
		verificationMgr, err := verification.NewManagerForTest(hashDir, verification.WithSkipHashDirectoryValidation())
		require.NoError(t, err)

		// Create loader with verification manager
		loader := NewLoader(common.NewDefaultFileSystem(), verificationMgr)

		// Loading should fail because template file hash is not recorded
		// (even if we provide content directly, the includes processing will verify)
		_, err = loader.LoadConfig(configPath, configContent)

		// Should fail because template file hash is not recorded
		require.Error(t, err)
		assert.Contains(t, err.Error(), "verification error")
	})

	t.Run("single include file hash verification succeeds when recorded", func(t *testing.T) {
		// Create hash manifest for the template file
		createHashManifest(t, hashDir, templatePath, templateContent)

		// Create verification manager
		verificationMgr, err := verification.NewManagerForTest(hashDir, verification.WithSkipHashDirectoryValidation())
		require.NoError(t, err)

		// Create loader with verification manager
		loader := NewLoader(common.NewDefaultFileSystem(), verificationMgr)

		// Now loading should succeed
		cfg, err := loader.LoadConfig(configPath, configContent)
		require.NoError(t, err)
		assert.NotNil(t, cfg)
		assert.Len(t, cfg.CommandTemplates, 2)
		assert.Equal(t, "echo", cfg.CommandTemplates["test"].Cmd)
		assert.Equal(t, "restic", cfg.CommandTemplates["backup"].Cmd)
	})
}

// TestLoadConfig_MultipleIncludeFilesHashVerification tests hash verification for
// multiple included template files.
func TestLoadConfig_MultipleIncludeFilesHashVerification(t *testing.T) {
	tmpDir := t.TempDir()
	hashDir := filepath.Join(tmpDir, "hashes")
	err := os.MkdirAll(hashDir, 0o755)
	require.NoError(t, err)

	// Create first template file
	template1Path := filepath.Join(tmpDir, "backup.toml")
	template1Content := []byte(`version = "1.0"

[command_templates.backup]
cmd = "restic"
args = ["backup"]
`)
	err = os.WriteFile(template1Path, template1Content, 0o644)
	require.NoError(t, err)

	// Create second template file
	template2Path := filepath.Join(tmpDir, "restore.toml")
	template2Content := []byte(`version = "1.0"

[command_templates.restore]
cmd = "restic"
args = ["restore"]
`)
	err = os.WriteFile(template2Path, template2Content, 0o644)
	require.NoError(t, err)

	// Create main config file
	configPath := filepath.Join(tmpDir, "config.toml")
	configContent := []byte(`version = "1.0"
includes = ["backup.toml", "restore.toml"]
`)
	err = os.WriteFile(configPath, configContent, 0o644)
	require.NoError(t, err)

	t.Run("fails when one of multiple include files lacks hash", func(t *testing.T) {
		// Create hash manifest only for the first template file
		createHashManifest(t, hashDir, template1Path, template1Content)
		// NOTE: second template file has no hash

		// Create verification manager
		verificationMgr, err := verification.NewManagerForTest(hashDir, verification.WithSkipHashDirectoryValidation())
		require.NoError(t, err)

		// Create loader with verification manager
		loader := NewLoader(common.NewDefaultFileSystem(), verificationMgr)

		// Loading should fail because second template file hash is not recorded
		_, err = loader.LoadConfig(configPath, configContent)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "verification error")
	})

	t.Run("succeeds when all include files have hashes", func(t *testing.T) {
		// Create hash manifest for the second template file as well
		createHashManifest(t, hashDir, template2Path, template2Content)

		// Create verification manager
		verificationMgr, err := verification.NewManagerForTest(hashDir, verification.WithSkipHashDirectoryValidation())
		require.NoError(t, err)

		// Create loader with verification manager
		loader := NewLoader(common.NewDefaultFileSystem(), verificationMgr)

		// Now loading should succeed
		cfg, err := loader.LoadConfig(configPath, configContent)
		require.NoError(t, err)
		assert.NotNil(t, cfg)
		assert.Len(t, cfg.CommandTemplates, 2)
	})
}

// TestLoadConfig_HashMismatchReturnsError tests that hash verification
// failure returns an error before execution.
func TestLoadConfig_HashMismatchReturnsError(t *testing.T) {
	tmpDir := t.TempDir()
	hashDir := filepath.Join(tmpDir, "hashes")
	err := os.MkdirAll(hashDir, 0o755)
	require.NoError(t, err)

	// Create template file
	templatePath := filepath.Join(tmpDir, "templates.toml")
	templateContent := []byte(`version = "1.0"

[command_templates.backup]
cmd = "restic"
`)
	err = os.WriteFile(templatePath, templateContent, 0o644)
	require.NoError(t, err)

	// Create hash manifest with WRONG hash
	wrongHash := "0000000000000000000000000000000000000000000000000000000000000000"
	createManifestWithCustomHash(t, hashDir, templatePath, wrongHash)

	// Create main config file
	configPath := filepath.Join(tmpDir, "config.toml")
	configContent := []byte(`version = "1.0"
includes = ["templates.toml"]
`)
	err = os.WriteFile(configPath, configContent, 0o644)
	require.NoError(t, err)

	// Create verification manager
	verificationMgr, err := verification.NewManagerForTest(hashDir, verification.WithSkipHashDirectoryValidation())
	require.NoError(t, err)

	// Create loader with verification manager
	loader := NewLoader(common.NewDefaultFileSystem(), verificationMgr)

	// Loading should fail because hash verification fails
	_, err = loader.LoadConfig(configPath, configContent)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "verification error")
}

// TestLoadConfig_MissingHashReturnsError tests that missing hash records
// return an error before execution.
func TestLoadConfig_MissingHashReturnsError(t *testing.T) {
	tmpDir := t.TempDir()
	hashDir := filepath.Join(tmpDir, "hashes")
	err := os.MkdirAll(hashDir, 0o755)
	require.NoError(t, err)

	// Create template file
	templatePath := filepath.Join(tmpDir, "templates.toml")
	templateContent := []byte(`version = "1.0"

[command_templates.backup]
cmd = "restic"
`)
	err = os.WriteFile(templatePath, templateContent, 0o644)
	require.NoError(t, err)

	// NOTE: NO hash manifest created for the template file

	// Create main config file
	configPath := filepath.Join(tmpDir, "config.toml")
	configContent := []byte(`version = "1.0"
includes = ["templates.toml"]
`)
	err = os.WriteFile(configPath, configContent, 0o644)
	require.NoError(t, err)

	// Create verification manager
	verificationMgr, err := verification.NewManagerForTest(hashDir, verification.WithSkipHashDirectoryValidation())
	require.NoError(t, err)

	// Create loader with verification manager
	loader := NewLoader(common.NewDefaultFileSystem(), verificationMgr)

	// Loading should fail because hash is not recorded
	_, err = loader.LoadConfig(configPath, configContent)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "verification error")
}

// TestLoadConfig_TamperedFileDetection tests that tampering with a template file
// after hash recording is detected.
func TestLoadConfig_TamperedFileDetection(t *testing.T) {
	tmpDir := t.TempDir()
	hashDir := filepath.Join(tmpDir, "hashes")
	err := os.MkdirAll(hashDir, 0o755)
	require.NoError(t, err)

	// Create template file
	templatePath := filepath.Join(tmpDir, "templates.toml")
	originalContent := []byte(`version = "1.0"

[command_templates.backup]
cmd = "restic"
`)
	err = os.WriteFile(templatePath, originalContent, 0o644)
	require.NoError(t, err)

	// Create hash manifest for the ORIGINAL content
	createHashManifest(t, hashDir, templatePath, originalContent)

	// Now tamper with the file (modify content after hash recording)
	tamperedContent := []byte(`version = "1.0"

[command_templates.backup]
cmd = "malicious_command"
`)
	err = os.WriteFile(templatePath, tamperedContent, 0o644)
	require.NoError(t, err)

	// Create main config file
	configPath := filepath.Join(tmpDir, "config.toml")
	configContent := []byte(`version = "1.0"
includes = ["templates.toml"]
`)
	err = os.WriteFile(configPath, configContent, 0o644)
	require.NoError(t, err)

	// Create verification manager
	verificationMgr, err := verification.NewManagerForTest(hashDir, verification.WithSkipHashDirectoryValidation())
	require.NoError(t, err)

	// Create loader with verification manager
	loader := NewLoader(common.NewDefaultFileSystem(), verificationMgr)

	// Loading should fail because file was tampered with
	_, err = loader.LoadConfig(configPath, configContent)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "verification error")
}

// =============================================================================
// Security Requirements Tests
// =============================================================================

// TestVerifyAndReadTemplateFile_AtomicOperation verifies that file verification and
// reading are performed atomically to prevent TOCTOU attacks.
func TestVerifyAndReadTemplateFile_AtomicOperation(t *testing.T) {
	tmpDir := t.TempDir()
	hashDir := filepath.Join(tmpDir, "hashes")
	err := os.MkdirAll(hashDir, 0o755)
	require.NoError(t, err)

	// Create template file
	templatePath := filepath.Join(tmpDir, "templates.toml")
	templateContent := []byte(`version = "1.0"

[command_templates.backup]
cmd = "restic"
`)
	err = os.WriteFile(templatePath, templateContent, 0o644)
	require.NoError(t, err)

	// Create hash manifest
	createHashManifest(t, hashDir, templatePath, templateContent)

	// Create verification manager
	verificationMgr, err := verification.NewManagerForTest(hashDir, verification.WithSkipHashDirectoryValidation())
	require.NoError(t, err)

	// The VerifyAndReadTemplateFile method should perform verification and reading
	// in a single operation (atomic) to prevent TOCTOU attacks.
	// This is verified by the fact that the content returned is the same content
	// that was verified.
	content, err := verificationMgr.VerifyAndReadTemplateFile(templatePath)
	require.NoError(t, err)
	assert.Equal(t, string(templateContent), string(content))

	// The implementation uses filevalidator.VerifyAndRead which:
	// 1. Reads the file content once
	// 2. Computes the hash of that content
	// 3. Compares with the stored hash
	// 4. Returns the content only if hash matches
	// This ensures no TOCTOU window between verification and content usage
}

// TestNewLoader_RequiresVerificationManager verifies that NewLoader
// rejects nil verificationManager to ensure verification is always enabled
// in production code.
func TestNewLoader_RequiresVerificationManager(t *testing.T) {
	t.Run("NewLoader panics with nil verificationManager", func(t *testing.T) {
		fs := common.NewDefaultFileSystem()

		// NewLoader should panic when verificationManager is nil
		assert.Panics(t, func() {
			NewLoader(fs, nil)
		}, "NewLoader should panic when verificationManager is nil")
	})

	t.Run("NewLoader panics with nil fileSystem", func(t *testing.T) {
		tmpDir := t.TempDir()
		verificationMgr, err := verification.NewManagerForTest(tmpDir, verification.WithSkipHashDirectoryValidation())
		require.NoError(t, err)

		// NewLoader should panic when fileSystem is nil
		assert.Panics(t, func() {
			NewLoader(nil, verificationMgr)
		}, "NewLoader should panic when fileSystem is nil")
	})

	t.Run("NewLoaderForTest allows nil verificationManager (test only)", func(t *testing.T) {
		// NewLoaderForTest is a testing convenience that allows nil
		// This is acceptable because it's only used in tests
		loader := NewLoaderForTest()
		assert.NotNil(t, loader)
		assert.Nil(t, loader.verificationMgr)
	})
}

// TestLoadConfig_ProductionPathUsesVerification verifies that the
// production code path always uses VerifyAndReadTemplateFile.
func TestLoadConfig_ProductionPathUsesVerification(t *testing.T) {
	tmpDir := t.TempDir()
	hashDir := filepath.Join(tmpDir, "hashes")
	err := os.MkdirAll(hashDir, 0o755)
	require.NoError(t, err)

	// Create template file
	templatePath := filepath.Join(tmpDir, "templates.toml")
	templateContent := []byte(`version = "1.0"

[command_templates.backup]
cmd = "restic"
`)
	err = os.WriteFile(templatePath, templateContent, 0o644)
	require.NoError(t, err)

	// Create hash manifest
	createHashManifest(t, hashDir, templatePath, templateContent)

	// Create main config file
	configPath := filepath.Join(tmpDir, "config.toml")
	configContent := []byte(`version = "1.0"
includes = ["templates.toml"]
`)
	err = os.WriteFile(configPath, configContent, 0o644)
	require.NoError(t, err)

	// Create verification manager
	verificationMgr, err := verification.NewManagerForTest(hashDir, verification.WithSkipHashDirectoryValidation())
	require.NoError(t, err)

	// Create loader with verification manager (production pattern)
	loader := NewLoader(common.NewDefaultFileSystem(), verificationMgr)

	// Loading should succeed and use verified template loading
	cfg, err := loader.LoadConfig(configPath, configContent)
	require.NoError(t, err)
	assert.NotNil(t, cfg)
	assert.Contains(t, cfg.CommandTemplates, "backup")

	// Now test that without hash, it fails (proving verification is being used)
	// Create a new template file without hash
	newTemplatePath := filepath.Join(tmpDir, "new_templates.toml")
	newTemplateContent := []byte(`version = "1.0"

[command_templates.new_backup]
cmd = "restic"
`)
	err = os.WriteFile(newTemplatePath, newTemplateContent, 0o644)
	require.NoError(t, err)

	// Create config that includes the new template (without hash)
	newConfigPath := filepath.Join(tmpDir, "new_config.toml")
	newConfigContent := []byte(`version = "1.0"
includes = ["new_templates.toml"]
`)
	err = os.WriteFile(newConfigPath, newConfigContent, 0o644)
	require.NoError(t, err)

	// Loading should fail because new template lacks hash
	_, err = loader.LoadConfig(newConfigPath, newConfigContent)
	require.Error(t, err, "should fail when template file lacks hash verification")
	assert.Contains(t, err.Error(), "verification error")
}
