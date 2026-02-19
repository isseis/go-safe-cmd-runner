//go:build test

package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
	"github.com/isseis/go-safe-cmd-runner/internal/fileanalysis"
	"github.com/isseis/go-safe-cmd-runner/internal/filevalidator"
	"github.com/isseis/go-safe-cmd-runner/internal/verification"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// createHashRecord creates a FileAnalysisRecord for the given file using filevalidator.New().Record().
// The file must already exist on disk.
func createHashRecord(t *testing.T, hashDir, filePath string) {
	t.Helper()

	validator, err := filevalidator.New(&filevalidator.SHA256{}, hashDir)
	require.NoError(t, err, "Failed to create validator for createHashRecord")

	_, err = validator.Record(filePath, false)
	require.NoError(t, err, "Failed to record hash for %s", filePath)
}

// createRecordWithCustomHash creates a FileAnalysisRecord with a custom (potentially wrong)
// hash value. Used for testing hash mismatch scenarios.
func createRecordWithCustomHash(t *testing.T, hashDir, filePath, customHash string) {
	t.Helper()

	getter := filevalidator.NewHybridHashFilePathGetter()
	store, err := fileanalysis.NewStore(hashDir, getter)
	require.NoError(t, err, "Failed to create store for createRecordWithCustomHash")

	resolvedPath, err := common.NewResolvedPath(filePath)
	require.NoError(t, err, "Failed to resolve file path")

	err = store.Update(resolvedPath, func(record *fileanalysis.Record) error {
		record.ContentHash = "sha256:" + customHash
		return nil
	})
	require.NoError(t, err, "Failed to write custom hash record")
}

// writeFile writes content to a file in the specified directory and returns the full path.
func writeFile(t *testing.T, dir, name string, content []byte) string {
	t.Helper()
	path := filepath.Join(dir, name)
	err := os.WriteFile(path, content, 0o644)
	require.NoError(t, err)
	return path
}

// createTestVerificationLoader creates a verification manager and loader for testing.
func createTestVerificationLoader(t *testing.T, hashDir string) (*verification.Manager, *Loader) {
	t.Helper()
	verificationMgr, err := verification.NewManagerForTest(hashDir, verification.WithSkipHashDirectoryValidation())
	require.NoError(t, err)
	loader := NewLoader(common.NewDefaultFileSystem(), verificationMgr)
	return verificationMgr, loader
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

	configContent := []byte(`version = "1.0"

[command_templates.test]
cmd = "echo"
args = ["hello"]
`)
	configPath := writeFile(t, tmpDir, "config.toml", configContent)

	verificationMgr, loader := createTestVerificationLoader(t, hashDir)

	t.Run("verification fails when hash not recorded", func(t *testing.T) {
		// Read config content (simulating what runner does)
		content, err := verificationMgr.VerifyAndReadConfigFile(configPath)

		// Should fail because hash is not recorded
		require.Error(t, err)
		assert.Nil(t, content)
		assert.Contains(t, err.Error(), configPath)
	})

	t.Run("verification succeeds when hash is recorded", func(t *testing.T) {
		// Create hash record for the config file
		createHashRecord(t, hashDir, configPath)

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

	templateContent := []byte(`version = "1.0"

[command_templates.backup]
cmd = "restic"
args = ["backup", "${path}"]
`)
	templatePath := writeFile(t, tmpDir, "templates.toml", templateContent)

	configContent := []byte(`version = "1.0"
includes = ["templates.toml"]

[command_templates.test]
cmd = "echo"
args = ["hello"]
`)
	configPath := writeFile(t, tmpDir, "config.toml", configContent)

	_, loader := createTestVerificationLoader(t, hashDir)

	t.Run("verification fails when hash not recorded", func(t *testing.T) {
		// Loading should fail because template file hash is not recorded
		// (even if we provide content directly, the includes processing will verify)
		_, err = loader.LoadConfig(configPath, configContent)

		// Should fail because template file hash is not recorded
		require.Error(t, err)
		assert.Contains(t, err.Error(), templatePath)
	})

	t.Run("verification succeeds when hash is recorded", func(t *testing.T) {
		// Create hash record for the template file
		createHashRecord(t, hashDir, templatePath)

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

	template1Content := []byte(`version = "1.0"

[command_templates.backup]
cmd = "restic"
args = ["backup"]
`)
	template1Path := writeFile(t, tmpDir, "backup.toml", template1Content)

	template2Content := []byte(`version = "1.0"

[command_templates.restore]
cmd = "restic"
args = ["restore"]
`)
	template2Path := writeFile(t, tmpDir, "restore.toml", template2Content)

	configContent := []byte(`version = "1.0"
includes = ["backup.toml", "restore.toml"]
`)
	configPath := writeFile(t, tmpDir, "config.toml", configContent)

	_, loader := createTestVerificationLoader(t, hashDir)

	t.Run("verification fails when one of multiple include files lacks hash", func(t *testing.T) {
		// Create hash record only for the first template file
		createHashRecord(t, hashDir, template1Path)
		// NOTE: second template file has no hash

		// Loading should fail because second template file hash is not recorded
		_, err = loader.LoadConfig(configPath, configContent)
		require.Error(t, err)
		assert.Contains(t, err.Error(), template2Path)
	})

	t.Run("verification succeeds when all include files have hashes", func(t *testing.T) {
		// Create hash record for the second template file as well
		createHashRecord(t, hashDir, template2Path)

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

	templateContent := []byte(`version = "1.0"

[command_templates.backup]
cmd = "restic"
`)
	templatePath := writeFile(t, tmpDir, "templates.toml", templateContent)

	// Create hash record with WRONG hash
	wrongHash := "0000000000000000000000000000000000000000000000000000000000000000"
	createRecordWithCustomHash(t, hashDir, templatePath, wrongHash)

	configContent := []byte(`version = "1.0"
includes = ["templates.toml"]
`)
	configPath := writeFile(t, tmpDir, "config.toml", configContent)

	_, loader := createTestVerificationLoader(t, hashDir)

	// Loading should fail because hash verification fails
	_, err = loader.LoadConfig(configPath, configContent)
	require.Error(t, err)
	assert.Contains(t, err.Error(), templatePath)
}

// TestLoadConfig_MissingHashReturnsError tests that missing hash records
// return an error before execution.
func TestLoadConfig_MissingHashReturnsError(t *testing.T) {
	tmpDir := t.TempDir()
	hashDir := filepath.Join(tmpDir, "hashes")
	err := os.MkdirAll(hashDir, 0o755)
	require.NoError(t, err)

	templateContent := []byte(`version = "1.0"

[command_templates.backup]
cmd = "restic"
`)
	templatePath := writeFile(t, tmpDir, "templates.toml", templateContent)

	// NOTE: NO hash manifest created for the template file

	configContent := []byte(`version = "1.0"
includes = ["templates.toml"]
`)
	configPath := writeFile(t, tmpDir, "config.toml", configContent)

	_, loader := createTestVerificationLoader(t, hashDir)

	// Loading should fail because hash is not recorded
	_, err = loader.LoadConfig(configPath, configContent)
	require.Error(t, err)
	assert.Contains(t, err.Error(), templatePath)
}

// TestLoadConfig_TamperedFileDetection tests that tampering with a template file
// after hash recording is detected.
func TestLoadConfig_TamperedFileDetection(t *testing.T) {
	tmpDir := t.TempDir()
	hashDir := filepath.Join(tmpDir, "hashes")
	err := os.MkdirAll(hashDir, 0o755)
	require.NoError(t, err)

	originalContent := []byte(`version = "1.0"

[command_templates.backup]
cmd = "restic"
`)
	templatePath := writeFile(t, tmpDir, "templates.toml", originalContent)

	// Create hash record for the ORIGINAL content
	createHashRecord(t, hashDir, templatePath)

	// Now tamper with the file (modify content after hash recording)
	tamperedContent := []byte(`version = "1.0"

[command_templates.backup]
cmd = "malicious_command"
`)
	err = os.WriteFile(templatePath, tamperedContent, 0o644)
	require.NoError(t, err)

	configContent := []byte(`version = "1.0"
includes = ["templates.toml"]
`)
	configPath := writeFile(t, tmpDir, "config.toml", configContent)

	_, loader := createTestVerificationLoader(t, hashDir)

	// Loading should fail because file was tampered with
	_, err = loader.LoadConfig(configPath, configContent)
	require.Error(t, err)
	assert.Contains(t, err.Error(), templatePath)
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

	templateContent := []byte(`version = "1.0"

[command_templates.backup]
cmd = "restic"
`)
	templatePath := writeFile(t, tmpDir, "templates.toml", templateContent)

	// Create hash record
	createHashRecord(t, hashDir, templatePath)

	verificationMgr, _ := createTestVerificationLoader(t, hashDir)

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

	templateContent := []byte(`version = "1.0"

[command_templates.backup]
cmd = "restic"
`)
	templatePath := writeFile(t, tmpDir, "templates.toml", templateContent)

	// Create hash record
	createHashRecord(t, hashDir, templatePath)

	configContent := []byte(`version = "1.0"
includes = ["templates.toml"]
`)
	configPath := writeFile(t, tmpDir, "config.toml", configContent)

	_, loader := createTestVerificationLoader(t, hashDir)

	// Loading should succeed and use verified template loading
	cfg, err := loader.LoadConfig(configPath, configContent)
	require.NoError(t, err)
	assert.NotNil(t, cfg)
	assert.Contains(t, cfg.CommandTemplates, "backup")

	// Now test that without hash, it fails (proving verification is being used)
	// Create a new template file without hash
	newTemplateContent := []byte(`version = "1.0"

[command_templates.new_backup]
cmd = "restic"
`)
	newTemplatePath := writeFile(t, tmpDir, "new_templates.toml", newTemplateContent)

	// Create config that includes the new template (without hash)
	newConfigContent := []byte(`version = "1.0"
includes = ["new_templates.toml"]
`)
	newConfigPath := writeFile(t, tmpDir, "new_config.toml", newConfigContent)

	// Loading should fail because new template lacks hash
	_, err = loader.LoadConfig(newConfigPath, newConfigContent)
	require.Error(t, err, "should fail when template file lacks hash verification")
	assert.Contains(t, err.Error(), newTemplatePath)
}
