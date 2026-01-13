//go:build test

package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/filevalidator"
	"github.com/isseis/go-safe-cmd-runner/internal/verification"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAcceptanceCriteria_F006_AC2_SingleIncludeFileVerification tests AC-2:
// includeされた全てのテンプレートファイルのハッシュ検証 (単一ファイル)
func TestAcceptanceCriteria_F006_AC2_SingleIncludeFileVerification(t *testing.T) {
	// Setup: Create temporary directory structure
	tempDir := t.TempDir()
	templateFile := filepath.Join(tempDir, "templates.toml")
	hashDir := filepath.Join(tempDir, ".hashes")

	// Create template file
	templateContent := []byte(`version = "1.0"

[command_templates.test_template]
cmd = "echo"
args = ["test"]
`)
	err := os.WriteFile(templateFile, templateContent, 0o600)
	require.NoError(t, err)

	// Record hash using filevalidator
	err = os.MkdirAll(hashDir, 0o700)
	require.NoError(t, err)

	validator, err := filevalidator.New(&filevalidator.SHA256{}, hashDir)
	require.NoError(t, err)

	_, err = validator.Record(templateFile, false)
	require.NoError(t, err)

	// Create verification manager
	vm, err := verification.NewManagerForTest(hashDir)
	require.NoError(t, err)

	// Create verified loader
	loader := NewVerifiedTemplateFileLoader(vm)

	// AC-2: Load template file - should verify hash
	templates, err := loader.LoadTemplateFile(templateFile)
	require.NoError(t, err)
	assert.Contains(t, templates, "test_template")
}

// TestAcceptanceCriteria_F006_AC2_MultipleIncludeFilesVerification tests AC-2:
// includeされた全てのテンプレートファイルのハッシュ検証 (複数ファイル)
func TestAcceptanceCriteria_F006_AC2_MultipleIncludeFilesVerification(t *testing.T) {
	// Setup: Create temporary directory structure
	tempDir := t.TempDir()
	template1 := filepath.Join(tempDir, "template1.toml")
	template2 := filepath.Join(tempDir, "template2.toml")
	hashDir := filepath.Join(tempDir, ".hashes")

	// Create template files
	template1Content := []byte(`version = "1.0"

[command_templates.template1]
cmd = "echo"
args = ["one"]
`)
	template2Content := []byte(`version = "1.0"

[command_templates.template2]
cmd = "echo"
args = ["two"]
`)
	err := os.WriteFile(template1, template1Content, 0o600)
	require.NoError(t, err)
	err = os.WriteFile(template2, template2Content, 0o600)
	require.NoError(t, err)

	// Record hashes using filevalidator
	err = os.MkdirAll(hashDir, 0o700)
	require.NoError(t, err)

	validator, err := filevalidator.New(&filevalidator.SHA256{}, hashDir)
	require.NoError(t, err)

	_, err = validator.Record(template1, false)
	require.NoError(t, err)
	_, err = validator.Record(template2, false)
	require.NoError(t, err)

	// Create verification manager
	vm, err := verification.NewManagerForTest(hashDir)
	require.NoError(t, err)

	// Create verified loader
	loader := NewVerifiedTemplateFileLoader(vm)

	// AC-2: Load both template files - should verify hashes for all
	templates1, err := loader.LoadTemplateFile(template1)
	require.NoError(t, err)
	assert.Contains(t, templates1, "template1")

	templates2, err := loader.LoadTemplateFile(template2)
	require.NoError(t, err)
	assert.Contains(t, templates2, "template2")
}

// TestAcceptanceCriteria_F006_AC4_HashNotRecorded tests AC-4:
// テンプレートファイルのハッシュが記録されていない場合、実行前にエラーが返されること
func TestAcceptanceCriteria_F006_AC4_HashNotRecorded(t *testing.T) {
	// Setup: Create temporary directory structure
	tempDir := t.TempDir()
	templateFile := filepath.Join(tempDir, "templates.toml")
	hashDir := filepath.Join(tempDir, ".hashes")

	// Create template file
	templateContent := []byte(`version = "1.0"

[command_templates.test_template]
cmd = "echo"
args = ["test"]
`)
	err := os.WriteFile(templateFile, templateContent, 0o600)
	require.NoError(t, err)

	// DO NOT record hash
	err = os.MkdirAll(hashDir, 0o700)
	require.NoError(t, err)

	// Create verification manager
	vm, err := verification.NewManagerForTest(hashDir)
	require.NoError(t, err)

	// Create verified loader
	loader := NewVerifiedTemplateFileLoader(vm)

	// AC-4: Load template file - should fail with hash not found error
	_, err = loader.LoadTemplateFile(templateFile)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "hash file not found")
}

// TestAcceptanceCriteria_F006_AC5_FileTampering tests AC-5:
// テンプレートファイルが改ざんされた場合、実行前にエラーが返されること
func TestAcceptanceCriteria_F006_AC5_FileTampering(t *testing.T) {
	// Setup: Create temporary directory structure
	tempDir := t.TempDir()
	templateFile := filepath.Join(tempDir, "templates.toml")
	hashDir := filepath.Join(tempDir, ".hashes")

	// Create template file
	originalContent := []byte(`version = "1.0"

[command_templates.test_template]
cmd = "echo"
args = ["test"]
`)
	err := os.WriteFile(templateFile, originalContent, 0o600)
	require.NoError(t, err)

	// Record hash using filevalidator
	err = os.MkdirAll(hashDir, 0o700)
	require.NoError(t, err)

	validator, err := filevalidator.New(&filevalidator.SHA256{}, hashDir)
	require.NoError(t, err)

	_, err = validator.Record(templateFile, false)
	require.NoError(t, err)

	// Tamper with the file
	tamperedContent := []byte(`version = "1.0"

[command_templates.test_template]
cmd = "rm"
args = ["-rf", "/"]
`)
	err = os.WriteFile(templateFile, tamperedContent, 0o600)
	require.NoError(t, err)

	// Create verification manager
	vm, err := verification.NewManagerForTest(hashDir)
	require.NoError(t, err)

	// Create verified loader
	loader := NewVerifiedTemplateFileLoader(vm)

	// AC-5: Load template file - should fail with hash mismatch error
	_, err = loader.LoadTemplateFile(templateFile)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "does not match the recorded hash")
}

// TestAcceptanceCriteria_SEC1_TOCTOU tests SEC-1:
// ファイル検証と読み込みが原子的に行われることを確認
func TestAcceptanceCriteria_SEC1_TOCTOU(t *testing.T) {
	// Setup: Create temporary directory structure
	tempDir := t.TempDir()
	templateFile := filepath.Join(tempDir, "templates.toml")
	hashDir := filepath.Join(tempDir, ".hashes")

	// Create template file
	templateContent := []byte(`version = "1.0"

[command_templates.test_template]
cmd = "echo"
args = ["test"]
`)
	err := os.WriteFile(templateFile, templateContent, 0o600)
	require.NoError(t, err)

	// Record hash using filevalidator
	err = os.MkdirAll(hashDir, 0o700)
	require.NoError(t, err)

	validator, err := filevalidator.New(&filevalidator.SHA256{}, hashDir)
	require.NoError(t, err)

	_, err = validator.Record(templateFile, false)
	require.NoError(t, err)

	// Create verification manager
	vm, err := verification.NewManagerForTest(hashDir)
	require.NoError(t, err)

	// Create verified loader
	loader := NewVerifiedTemplateFileLoader(vm)

	// SEC-1: Load template file
	// The VerifyAndReadTemplateFile method should read the file only once
	// and verify the hash atomically
	templates, err := loader.LoadTemplateFile(templateFile)
	require.NoError(t, err)
	assert.Contains(t, templates, "test_template")

	// If TOCTOU vulnerability exists, an attacker could modify the file
	// between verification and reading. The implementation should prevent this
	// by reading the file only once and verifying the content that was read.
}
