//go:build linux || freebsd || openbsd || netbsd

package verification

import (
	"os"
	"path/filepath"
	"testing"

	tu "github.com/isseis/go-safe-cmd-runner/internal/testutil"
	"github.com/stretchr/testify/require"
)

// TestNewManagerInternal_DryRun_HashDirParentUnreadable_RecordsPermissionDenied tests that
// when the hash directory does not exist and its parent cannot be traversed (Lstat returns
// a permission error rather than IsNotExist), the dry-run manager still constructs
// successfully and records the failure as ReasonPermissionDenied rather than aborting or
// falling back to skipped_no_validator.
//
// Like the filevalidator "unreadable directory" tests, this test is meaningless when run as
// root, since chmod 0o000 does not deny access to root. This is an existing constraint of
// this style of permission test, not one newly introduced here.
func TestNewManagerInternal_DryRun_HashDirParentUnreadable_RecordsPermissionDenied(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("Skipping privilege test when running as root")
	}

	tmpDir := tu.SafeTempDir(t)
	restrictedDir := filepath.Join(tmpDir, "restricted")
	require.NoError(t, os.Mkdir(restrictedDir, 0o755))

	// hashDir is a path under restrictedDir that is never created.
	hashDir := filepath.Join(restrictedDir, "hashes")

	require.NoError(t, os.Chmod(restrictedDir, 0o000))
	t.Cleanup(func() { _ = os.Chmod(restrictedDir, 0o755) })

	manager, err := newManagerInternal(hashDir,
		withDryRunModeInternal(),
		withSkipHashDirectoryValidationInternal(),
		withCreationMode(CreationModeTesting),
		withSecurityLevel(SecurityLevelRelaxed))
	require.NoError(t, err)
	require.NotNil(t, manager.fileValidator)

	configFile := createTestFile(t, tmpDir, "config.toml", []byte("test config"))
	_, err = manager.VerifyAndReadConfigFile(configFile)
	require.NoError(t, err, "dry-run mode should not return errors")

	summary := manager.GetVerificationSummary()
	require.NotNil(t, summary)
	require.NotEmpty(t, summary.Failures)
	require.Equal(t, ReasonPermissionDenied, summary.Failures[0].Reason)
}

// TestNewManagerInternal_DryRun_HashDirExistsButUnresolvable_RecordsPermissionDenied tests
// the case where the hash directory itself exists and is Lstat-able (a relative path resolves
// against the process's cwd without re-checking ancestor search permissions), but resolving its
// absolute, symlink-free path fails with a permission error because an ancestor of cwd is
// unreadable. This exercises the err == nil && info.IsDir() branch of filevalidator.NewReadOnly,
// distinct from the parent-unreadable/Lstat-failure case above: dry-run construction must still
// succeed and report the failure as ReasonPermissionDenied rather than aborting.
func TestNewManagerInternal_DryRun_HashDirExistsButUnresolvable_RecordsPermissionDenied(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("Skipping privilege test when running as root")
	}

	tmpDir := tu.SafeTempDir(t)
	restrictedDir := filepath.Join(tmpDir, "restricted")
	workDir := filepath.Join(restrictedDir, "workdir")
	hashDir := filepath.Join(workDir, "hashes")
	require.NoError(t, os.MkdirAll(hashDir, 0o755))

	t.Chdir(workDir)

	require.NoError(t, os.Chmod(restrictedDir, 0o000))
	t.Cleanup(func() { _ = os.Chmod(restrictedDir, 0o755) })

	// Relative hashDir: os.Lstat resolves it against cwd directly, so it
	// succeeds even though "restricted" (an ancestor of cwd) is unreadable.
	manager, err := newManagerInternal("hashes",
		withDryRunModeInternal(),
		withSkipHashDirectoryValidationInternal(),
		withCreationMode(CreationModeTesting),
		withSecurityLevel(SecurityLevelRelaxed))
	require.NoError(t, err)
	require.NotNil(t, manager.fileValidator)

	// Place the config file outside restrictedDir; a path inside it would be
	// unreadable via its absolute path once restrictedDir is chmod 0o000.
	configFile := createTestFile(t, tmpDir, "config.toml", []byte("test config"))
	_, err = manager.VerifyAndReadConfigFile(configFile)
	require.NoError(t, err, "dry-run mode should not return errors")

	summary := manager.GetVerificationSummary()
	require.NotNil(t, summary)
	require.NotEmpty(t, summary.Failures)
	require.Equal(t, ReasonPermissionDenied, summary.Failures[0].Reason)
}
