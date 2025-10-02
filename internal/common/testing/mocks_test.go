package testing

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMockFileSystem_AddFile_ErrorHandling(t *testing.T) {
	tests := []struct {
		name          string
		setupFunc     func(*MockFileSystem) error
		path          string
		mode          os.FileMode
		content       []byte
		expectedError error
	}{
		{
			name:          "create new file should succeed",
			path:          "/test/newfile.txt",
			mode:          0o644,
			content:       []byte("test content"),
			expectedError: nil,
		},
		{
			name: "create file on existing path should fail",
			setupFunc: func(fs *MockFileSystem) error {
				return fs.AddFile("/test/existing.txt", 0o644, []byte("existing"))
			},
			path:          "/test/existing.txt",
			mode:          0o644,
			content:       []byte("new content"),
			expectedError: os.ErrExist,
		},
		{
			name: "create file where directory already exists should fail",
			setupFunc: func(fs *MockFileSystem) error {
				return fs.AddDir("/test/dir", 0o755)
			},
			path:          "/test/dir",
			mode:          0o644,
			content:       []byte("content"),
			expectedError: os.ErrExist,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fs := NewMockFileSystem()

			// Setup if needed
			if tt.setupFunc != nil {
				err := tt.setupFunc(fs)
				require.NoError(t, err)
			}

			// Test AddFile
			err := fs.AddFile(tt.path, tt.mode, tt.content)

			if tt.expectedError != nil {
				assert.Error(t, err)
				assert.ErrorIs(t, err, tt.expectedError)
			} else {
				assert.NoError(t, err)

				// Verify file was created
				exists, err := fs.FileExists(tt.path)
				require.NoError(t, err)
				assert.True(t, exists)

				// Verify it's not a directory
				isDir, err := fs.IsDir(tt.path)
				require.NoError(t, err)
				assert.False(t, isDir)
			}
		})
	}
}

func TestMockFileSystem_AddDir_ErrorHandling(t *testing.T) {
	tests := []struct {
		name          string
		setupFunc     func(*MockFileSystem) error
		path          string
		mode          os.FileMode
		expectedError error
	}{
		{
			name:          "create new directory should succeed",
			path:          "/test/newdir",
			mode:          0o755,
			expectedError: nil,
		},
		{
			name: "create directory on existing path should fail",
			setupFunc: func(fs *MockFileSystem) error {
				return fs.AddDir("/test/existing", 0o755)
			},
			path:          "/test/existing",
			mode:          0o755,
			expectedError: os.ErrExist,
		},
		{
			name: "create directory where file already exists should fail",
			setupFunc: func(fs *MockFileSystem) error {
				return fs.AddFile("/test/file.txt", 0o644, []byte("content"))
			},
			path:          "/test/file.txt",
			mode:          0o755,
			expectedError: os.ErrExist,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fs := NewMockFileSystem()

			// Setup if needed
			if tt.setupFunc != nil {
				err := tt.setupFunc(fs)
				require.NoError(t, err)
			}

			// Test AddDir
			err := fs.AddDir(tt.path, tt.mode)

			if tt.expectedError != nil {
				assert.Error(t, err)
				assert.ErrorIs(t, err, tt.expectedError)
			} else {
				assert.NoError(t, err)

				// Verify directory was created
				exists, err := fs.FileExists(tt.path)
				require.NoError(t, err)
				assert.True(t, exists)

				// Verify it's a directory
				isDir, err := fs.IsDir(tt.path)
				require.NoError(t, err)
				assert.True(t, isDir)
			}
		})
	}
}

func TestMockFileSystem_AddSymlink_ErrorHandling(t *testing.T) {
	tests := []struct {
		name          string
		setupFunc     func(*MockFileSystem) error
		linkPath      string
		targetPath    string
		expectedError error
	}{
		{
			name:          "create new symlink should succeed",
			linkPath:      "/test/link",
			targetPath:    "/test/target",
			expectedError: nil,
		},
		{
			name: "create symlink on existing path should fail",
			setupFunc: func(fs *MockFileSystem) error {
				return fs.AddFile("/test/existing", 0o644, []byte("content"))
			},
			linkPath:      "/test/existing",
			targetPath:    "/test/target",
			expectedError: os.ErrExist,
		},
		{
			name: "create symlink where directory already exists should fail",
			setupFunc: func(fs *MockFileSystem) error {
				return fs.AddDir("/test/dir", 0o755)
			},
			linkPath:      "/test/dir",
			targetPath:    "/test/target",
			expectedError: os.ErrExist,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fs := NewMockFileSystem()

			// Setup if needed
			if tt.setupFunc != nil {
				err := tt.setupFunc(fs)
				require.NoError(t, err)
			}

			// Test AddSymlink
			err := fs.AddSymlink(tt.linkPath, tt.targetPath)

			if tt.expectedError != nil {
				assert.Error(t, err)
				assert.ErrorIs(t, err, tt.expectedError)
			} else {
				assert.NoError(t, err)

				// Verify symlink was created
				exists, err := fs.FileExists(tt.linkPath)
				require.NoError(t, err)
				assert.True(t, exists)

				// Verify it's not a directory
				isDir, err := fs.IsDir(tt.linkPath)
				require.NoError(t, err)
				assert.False(t, isDir)

				// Verify symlink target
				target, err := fs.Readlink(tt.linkPath)
				require.NoError(t, err)
				assert.Equal(t, tt.targetPath, target)

				// Verify file info shows it's a symlink
				info, err := fs.Lstat(tt.linkPath)
				require.NoError(t, err)
				assert.True(t, info.Mode()&os.ModeSymlink != 0)
			}
		})
	}
}

func TestMockFileSystem_ConsistentBehavior(t *testing.T) {
	t.Run("all Add functions should behave consistently with existing paths", func(t *testing.T) {
		fs := NewMockFileSystem()

		// Create a file first
		err := fs.AddFile("/test/path", 0o644, []byte("content"))
		require.NoError(t, err)

		// All subsequent operations on the same path should fail
		err = fs.AddFile("/test/path", 0o644, []byte("new content"))
		assert.ErrorIs(t, err, os.ErrExist)

		err = fs.AddDir("/test/path", 0o755)
		assert.ErrorIs(t, err, os.ErrExist)

		err = fs.AddSymlink("/test/path", "/target")
		assert.ErrorIs(t, err, os.ErrExist)
	})

	t.Run("all Add functions should behave consistently with directory paths", func(t *testing.T) {
		fs := NewMockFileSystem()

		// Create a directory first
		err := fs.AddDir("/test/dir", 0o755)
		require.NoError(t, err)

		// All subsequent operations on the same path should fail
		err = fs.AddFile("/test/dir", 0o644, []byte("content"))
		assert.ErrorIs(t, err, os.ErrExist)

		err = fs.AddDir("/test/dir", 0o755)
		assert.ErrorIs(t, err, os.ErrExist)

		err = fs.AddSymlink("/test/dir", "/target")
		assert.ErrorIs(t, err, os.ErrExist)
	})
}
