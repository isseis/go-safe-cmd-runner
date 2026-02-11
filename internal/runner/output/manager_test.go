package output

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	commontesting "github.com/isseis/go-safe-cmd-runner/internal/common/testing"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/security"
	safefileiotesting "github.com/isseis/go-safe-cmd-runner/internal/safefileio/testutil"
)

// Test errors for manager_test
var (
	ErrTestPathTraversalDetected = errors.New("path traversal detected")
	ErrTestPermissionDenied      = errors.New("permission denied")
	ErrTestTempCreationFailed    = errors.New("temp creation failed")
)

// MockPathValidator for testing
type MockPathValidator struct {
	mock.Mock
}

func (m *MockPathValidator) ValidateAndResolvePath(outputPath, workDir string) (string, error) {
	args := m.Called(outputPath, workDir)
	return args.String(0), args.Error(1)
}

// MockFileManager for testing
type MockFileManager struct {
	mock.Mock
}

func (m *MockFileManager) CreateTempFile(dir string, pattern string) (*os.File, error) {
	args := m.Called(dir, pattern)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*os.File), args.Error(1)
}

// Helper function to create a real temporary file for testing
func createRealTempFile(t *testing.T) (*os.File, string) {
	tempFile, err := os.CreateTemp("/tmp", "test_output_*.tmp")
	require.NoError(t, err)
	return tempFile, tempFile.Name()
}

func (m *MockFileManager) WriteToTemp(file *os.File, data []byte) (int, error) {
	args := m.Called(file, data)
	return args.Int(0), args.Error(1)
}

func (m *MockFileManager) MoveToFinal(tempPath, finalPath string) error {
	args := m.Called(tempPath, finalPath)
	return args.Error(0)
}

func (m *MockFileManager) EnsureDirectory(path string) error {
	args := m.Called(path)
	return args.Error(0)
}

func (m *MockFileManager) RemoveTemp(path string) error {
	args := m.Called(path)
	return args.Error(0)
}

// MockSecurityValidator for testing
type MockSecurityValidator struct {
	mock.Mock
}

func (m *MockSecurityValidator) ValidateOutputWritePermission(outputPath string, realUID int) error {
	args := m.Called(outputPath, realUID)
	return args.Error(0)
}

func TestDefaultOutputCaptureManager_PrepareOutput(t *testing.T) {
	tests := []struct {
		name            string
		outputPath      string
		workDir         string
		maxSize         int64
		setupMocks      func(*MockPathValidator, *MockFileManager, *MockSecurityValidator)
		wantErr         bool
		errMessage      string
		validateCapture func(t *testing.T, capture *Capture)
	}{
		{
			name:       "successful_preparation_absolute_path",
			outputPath: "/tmp/output.txt",
			workDir:    "/home/user",
			maxSize:    1024 * 1024,
			setupMocks: func(pv *MockPathValidator, fm *MockFileManager, sv *MockSecurityValidator) {
				pv.On("ValidateAndResolvePath", "/tmp/output.txt", "/home/user").Return("/tmp/output.txt", nil)
				sv.On("ValidateOutputWritePermission", "/tmp/output.txt", mock.AnythingOfType("int")).Return(nil)
				fm.On("EnsureDirectory", "/tmp").Return(nil)
				// Create a real temp file for testing
				tempFile, _ := createRealTempFile(t)
				fm.On("CreateTempFile", "/tmp", "output_*.tmp").Return(tempFile, nil)
			},
			wantErr: false,
			validateCapture: func(t *testing.T, capture *Capture) {
				assert.Equal(t, "/tmp/output.txt", capture.OutputPath)
				assert.NotNil(t, capture.FileHandle)
				assert.NotEmpty(t, capture.TempFilePath)
				assert.Equal(t, int64(0), capture.CurrentSize)
				assert.Equal(t, int64(1024*1024), capture.MaxSize)
				assert.False(t, capture.StartTime.IsZero())
			},
		},
		{
			name:       "successful_preparation_relative_path",
			outputPath: "output/result.txt",
			workDir:    "/home/user/project",
			maxSize:    2048,
			setupMocks: func(pv *MockPathValidator, fm *MockFileManager, sv *MockSecurityValidator) {
				pv.On("ValidateAndResolvePath", "output/result.txt", "/home/user/project").Return("/home/user/project/output/result.txt", nil)
				sv.On("ValidateOutputWritePermission", "/home/user/project/output/result.txt", mock.AnythingOfType("int")).Return(nil)
				fm.On("EnsureDirectory", "/home/user/project/output").Return(nil)
				// Create a real temp file for testing
				tempFile, _ := createRealTempFile(t)
				fm.On("CreateTempFile", "/home/user/project/output", "output_*.tmp").Return(tempFile, nil)
			},
			wantErr: false,
			validateCapture: func(t *testing.T, capture *Capture) {
				assert.Equal(t, "/home/user/project/output/result.txt", capture.OutputPath)
				assert.NotNil(t, capture.FileHandle)
				assert.NotEmpty(t, capture.TempFilePath)
				assert.Equal(t, int64(2048), capture.MaxSize)
			},
		},
		{
			name:       "path_validation_error",
			outputPath: "../../../etc/passwd",
			workDir:    "/home/user",
			maxSize:    1024,
			setupMocks: func(pv *MockPathValidator, _ *MockFileManager, _ *MockSecurityValidator) {
				pv.On("ValidateAndResolvePath", "../../../etc/passwd", "/home/user").Return("", ErrTestPathTraversalDetected)
			},
			wantErr:    true,
			errMessage: "path traversal detected",
		},
		{
			name:       "permission_validation_error",
			outputPath: "/etc/sensitive",
			workDir:    "/home/user",
			maxSize:    1024,
			setupMocks: func(pv *MockPathValidator, _ *MockFileManager, sv *MockSecurityValidator) {
				pv.On("ValidateAndResolvePath", "/etc/sensitive", "/home/user").Return("/etc/sensitive", nil)
				sv.On("ValidateOutputWritePermission", "/etc/sensitive", mock.AnythingOfType("int")).Return(security.ErrInvalidFilePermissions)
			},
			wantErr:    true,
			errMessage: "invalid file permissions",
		},
		{
			name:       "directory_creation_error",
			outputPath: "/tmp/nonexistent/deeply/nested/output.txt",
			workDir:    "/home/user",
			maxSize:    1024,
			setupMocks: func(pv *MockPathValidator, fm *MockFileManager, sv *MockSecurityValidator) {
				pv.On("ValidateAndResolvePath", "/tmp/nonexistent/deeply/nested/output.txt", "/home/user").Return("/tmp/nonexistent/deeply/nested/output.txt", nil)
				sv.On("ValidateOutputWritePermission", "/tmp/nonexistent/deeply/nested/output.txt", mock.AnythingOfType("int")).Return(nil)
				fm.On("EnsureDirectory", "/tmp/nonexistent/deeply/nested").Return(ErrTestPermissionDenied)
			},
			wantErr:    true,
			errMessage: "permission denied",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup mocks
			mockPathValidator := &MockPathValidator{}
			mockFileManager := &MockFileManager{}
			mockSecurityValidator := &MockSecurityValidator{}

			if tt.setupMocks != nil {
				tt.setupMocks(mockPathValidator, mockFileManager, mockSecurityValidator)
			}

			// Create manager with mocks
			manager := &DefaultOutputCaptureManager{
				pathValidator:     mockPathValidator,
				fileManager:       mockFileManager,
				securityValidator: mockSecurityValidator,
			}

			// Call PrepareOutput
			capture, err := manager.PrepareOutput(tt.outputPath, tt.workDir, tt.maxSize)

			// Validate results
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errMessage != "" {
					assert.Contains(t, err.Error(), tt.errMessage)
				}
				assert.Nil(t, capture)
			} else {
				require.NoError(t, err)
				require.NotNil(t, capture)
				if tt.validateCapture != nil {
					tt.validateCapture(t, capture)
				}
			}

			// Verify mock expectations
			mockPathValidator.AssertExpectations(t)
			mockFileManager.AssertExpectations(t)
			mockSecurityValidator.AssertExpectations(t)
		})
	}
}

func TestDefaultOutputCaptureManager_WriteOutput(t *testing.T) {
	tests := []struct {
		name           string
		initialSize    int64
		maxSize        int64
		writeData      []byte
		wantErr        bool
		errMessage     string
		expectedSize   int64
		expectedBuffer []byte
	}{
		{
			name:           "successful_write_small_data",
			initialSize:    0,
			maxSize:        1024,
			writeData:      []byte("test data\n"),
			wantErr:        false,
			expectedSize:   10,
			expectedBuffer: []byte("test data\n"),
		},
		{
			name:           "successful_write_binary_data",
			initialSize:    5,
			maxSize:        1024,
			writeData:      []byte{0x00, 0x01, 0x02, 0xFF},
			wantErr:        false,
			expectedSize:   9,
			expectedBuffer: []byte{0x00, 0x01, 0x02, 0xFF},
		},
		{
			name:           "write_empty_data",
			initialSize:    10,
			maxSize:        1024,
			writeData:      []byte{},
			wantErr:        false,
			expectedSize:   10,
			expectedBuffer: []byte{},
		},
		{
			name:        "size_limit_exceeded",
			initialSize: 1020,
			maxSize:     1024,
			writeData:   []byte("this data exceeds limit"),
			wantErr:     true,
			errMessage:  "output size limit exceeded",
		},
		{
			name:           "write_at_exact_limit",
			initialSize:    1020,
			maxSize:        1024,
			writeData:      []byte("1234"),
			wantErr:        false,
			expectedSize:   1024,
			expectedBuffer: []byte("1234"),
		},
		{
			name:           "unlimited_size_zero_max",
			initialSize:    1000000,
			maxSize:        0, // No limit
			writeData:      []byte("large data can be written"),
			wantErr:        false,
			expectedSize:   1000025,
			expectedBuffer: []byte("large data can be written"),
		},
	}

	manager := &DefaultOutputCaptureManager{}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a real temporary file for testing
			tempFile, tempPath := createRealTempFile(t)
			defer os.Remove(tempPath)

			// Create capture with initial state
			capture := &Capture{
				OutputPath:   "/tmp/test.txt",
				TempFilePath: tempPath,
				FileHandle:   tempFile,
				CurrentSize:  tt.initialSize,
				MaxSize:      tt.maxSize,
				StartTime:    time.Now(),
			}

			// Add some initial data if needed
			if tt.initialSize > 0 {
				initialData := make([]byte, tt.initialSize)
				_, err := tempFile.Write(initialData)
				require.NoError(t, err)
			}

			// Call WriteOutput
			err := manager.WriteOutput(capture, tt.writeData)

			// Validate results
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errMessage != "" {
					assert.Contains(t, err.Error(), tt.errMessage)
				}
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedSize, capture.CurrentSize)

				// Check if the new data was written to file
				if len(tt.expectedBuffer) > 0 {
					// Seek to the end of the file to verify the last written data
					_, err := tempFile.Seek(-int64(len(tt.expectedBuffer)), 2)
					if err == nil {
						lastBytes := make([]byte, len(tt.expectedBuffer))
						n, readErr := tempFile.Read(lastBytes)
						if readErr == nil && n == len(tt.expectedBuffer) {
							// The file should contain the expected buffer data at the end
							assert.Equal(t, tt.expectedBuffer, lastBytes)
						}
					}
				}
			}
		})
	}
}

func TestDefaultOutputCaptureManager_FinalizeOutput(t *testing.T) {
	tests := []struct {
		name          string
		bufferContent []byte
		setupMocks    func(*MockFileManager)
		wantErr       bool
		errMessage    string
	}{
		{
			name:          "successful_finalization",
			bufferContent: []byte("test output content\nline 2\n"),
			setupMocks: func(fm *MockFileManager) {
				fm.On("MoveToFinal", mock.AnythingOfType("string"), "/tmp/final.txt").Return(nil)
			},
			wantErr: false,
		},
		{
			name:          "empty_buffer_finalization",
			bufferContent: []byte{},
			setupMocks: func(fm *MockFileManager) {
				fm.On("MoveToFinal", mock.AnythingOfType("string"), "/tmp/empty.txt").Return(nil)
			},
			wantErr: false,
		},
		{
			name:          "file_move_error",
			bufferContent: []byte("content"),
			setupMocks: func(fm *MockFileManager) {
				fm.On("MoveToFinal", mock.AnythingOfType("string"), "/tmp/error.txt").Return(ErrTestPermissionDenied)
			},
			wantErr:    true,
			errMessage: "permission denied",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup mocks
			mockFileManager := &MockFileManager{}
			if tt.setupMocks != nil {
				tt.setupMocks(mockFileManager)
			}

			manager := &DefaultOutputCaptureManager{
				fileManager: mockFileManager,
			}

			// Create temporary file with content
			tempFile, tempPath := createRealTempFile(t)
			defer os.Remove(tempPath)

			// Write buffer content to temp file
			if len(tt.bufferContent) > 0 {
				_, err := tempFile.Write(tt.bufferContent)
				require.NoError(t, err)
			}

			var outputPath string
			switch tt.name {
			case "empty_buffer_finalization":
				outputPath = "/tmp/empty.txt"
			case "file_move_error":
				outputPath = "/tmp/error.txt"
			default:
				outputPath = "/tmp/final.txt"
			}

			capture := &Capture{
				OutputPath:   outputPath,
				TempFilePath: tempPath,
				FileHandle:   tempFile,
				CurrentSize:  int64(len(tt.bufferContent)),
				StartTime:    time.Now(),
			}

			// Call FinalizeOutput
			err := manager.FinalizeOutput(capture)

			// Validate results
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errMessage != "" {
					assert.Contains(t, err.Error(), tt.errMessage)
				}
			} else {
				assert.NoError(t, err)
			}

			// Verify mock expectations
			mockFileManager.AssertExpectations(t)
		})
	}
}

func TestDefaultOutputCaptureManager_CleanupOutput(t *testing.T) {
	// Setup mocks
	mockSafeFS := safefileiotesting.NewMockFileSystem()
	mockCommonFS := commontesting.NewMockFileSystem()

	// Add a dummy file to mock FS
	tempPath := "/tmp/test_output_123.tmp"
	require.NoError(t, mockCommonFS.AddFile(tempPath, 0o600, []byte("test data to be cleaned")))

	// Create manager with mocks
	manager := &DefaultOutputCaptureManager{
		fileManager: NewSafeFileManagerWithFS(mockSafeFS, mockCommonFS),
	}

	// Create a dummy file handle that can be closed
	// We use a real pipe here just to have a valid *os.File that doesn't error on Close
	r, w, err := os.Pipe()
	require.NoError(t, err)
	require.NoError(t, r.Close()) // Close reader end

	capture := &Capture{
		OutputPath:   "/tmp/test.txt",
		TempFilePath: tempPath,
		FileHandle:   w, // Use writer end as file handle
		CurrentSize:  23,
		StartTime:    time.Now(),
	}

	// Verify initial state
	assert.Equal(t, int64(23), capture.CurrentSize)
	assert.NotNil(t, capture.FileHandle)

	// Call CleanupOutput
	err = manager.CleanupOutput(capture)

	// Validate results
	assert.NoError(t, err)
	assert.Equal(t, int64(0), capture.CurrentSize)
	assert.Nil(t, capture.FileHandle)
	assert.Empty(t, capture.TempFilePath)

	// Verify file is removed from mock FS
	exists, _ := mockCommonFS.FileExists(tempPath)
	assert.False(t, exists, "Temp file should be removed from mock FS")
}

func TestDefaultOutputCaptureManager_AnalyzeOutput(t *testing.T) {
	tests := []struct {
		name             string
		outputPath       string
		workDir          string
		setupMocks       func(*MockPathValidator, *MockSecurityValidator)
		wantErr          bool
		errMessage       string
		validateAnalysis func(t *testing.T, analysis *Analysis)
	}{
		{
			name:       "successful_analysis_with_permissions",
			outputPath: "/tmp/output.txt",
			workDir:    "/home/user",
			setupMocks: func(pv *MockPathValidator, sv *MockSecurityValidator) {
				pv.On("ValidateAndResolvePath", "/tmp/output.txt", "/home/user").Return("/tmp/output.txt", nil)
				sv.On("ValidateOutputWritePermission", "/tmp/output.txt", mock.AnythingOfType("int")).Return(nil)
			},
			wantErr: false,
			validateAnalysis: func(t *testing.T, analysis *Analysis) {
				assert.Equal(t, "/tmp/output.txt", analysis.OutputPath)
				assert.Equal(t, "/tmp/output.txt", analysis.ResolvedPath)
				assert.True(t, analysis.WritePermission)
				assert.Equal(t, RiskLevelMedium, analysis.SecurityRisk) // Default for /tmp
			},
		},
		{
			name:       "path_validation_failure",
			outputPath: "../../../etc/passwd",
			workDir:    "/home/user",
			setupMocks: func(pv *MockPathValidator, _ *MockSecurityValidator) {
				pv.On("ValidateAndResolvePath", "../../../etc/passwd", "/home/user").Return("", ErrTestPathTraversalDetected)
			},
			wantErr: false, // AnalyzeOutput doesn't fail, it reports the problem
			validateAnalysis: func(t *testing.T, analysis *Analysis) {
				assert.Equal(t, "../../../etc/passwd", analysis.OutputPath)
				assert.Equal(t, "", analysis.ResolvedPath)
				assert.False(t, analysis.WritePermission)
				assert.Equal(t, RiskLevelCritical, analysis.SecurityRisk)
				assert.Contains(t, analysis.ErrorMessage, "path traversal detected")
			},
		},
		{
			name:       "permission_check_failure",
			outputPath: "/etc/sensitive",
			workDir:    "/home/user",
			setupMocks: func(pv *MockPathValidator, sv *MockSecurityValidator) {
				pv.On("ValidateAndResolvePath", "/etc/sensitive", "/home/user").Return("/etc/sensitive", nil)
				sv.On("ValidateOutputWritePermission", "/etc/sensitive", mock.AnythingOfType("int")).Return(ErrTestPermissionDenied)
			},
			wantErr: false,
			validateAnalysis: func(t *testing.T, analysis *Analysis) {
				assert.Equal(t, "/etc/sensitive", analysis.OutputPath)
				assert.Equal(t, "/etc/sensitive", analysis.ResolvedPath)
				assert.False(t, analysis.WritePermission)
				assert.Equal(t, RiskLevelHigh, analysis.SecurityRisk) // /etc/ is high risk
				assert.Contains(t, analysis.ErrorMessage, "permission denied")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup mocks
			mockPathValidator := &MockPathValidator{}
			mockSecurityValidator := &MockSecurityValidator{}

			if tt.setupMocks != nil {
				tt.setupMocks(mockPathValidator, mockSecurityValidator)
			}

			// Create manager with mocks
			manager := &DefaultOutputCaptureManager{
				pathValidator:     mockPathValidator,
				securityValidator: mockSecurityValidator,
			}

			// Call AnalyzeOutput
			analysis, err := manager.AnalyzeOutput(tt.outputPath, tt.workDir)

			// Validate results
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errMessage != "" {
					assert.Contains(t, err.Error(), tt.errMessage)
				}
			} else {
				assert.NoError(t, err)
				require.NotNil(t, analysis)
				if tt.validateAnalysis != nil {
					tt.validateAnalysis(t, analysis)
				}
			}

			// Verify mock expectations
			mockPathValidator.AssertExpectations(t)
			mockSecurityValidator.AssertExpectations(t)
		})
	}
}

func TestDefaultOutputCaptureManager_Integration(t *testing.T) {
	// Integration test using real FileManager but mock validators
	tempDir := t.TempDir()
	outputPath := filepath.Join(tempDir, "integration_output.txt")

	// Setup mocks for validators
	mockPathValidator := &MockPathValidator{}
	mockSecurityValidator := &MockSecurityValidator{}

	mockPathValidator.On("ValidateAndResolvePath", outputPath, tempDir).Return(outputPath, nil)
	mockSecurityValidator.On("ValidateOutputWritePermission", outputPath, mock.AnythingOfType("int")).Return(nil)

	// Create manager with real file manager
	manager := &DefaultOutputCaptureManager{
		pathValidator:     mockPathValidator,
		fileManager:       NewSafeFileManager(),
		securityValidator: mockSecurityValidator,
	}

	// Test complete workflow
	maxSize := int64(1024)

	// 1. Prepare output
	capture, err := manager.PrepareOutput(outputPath, tempDir, maxSize)
	require.NoError(t, err)
	require.NotNil(t, capture)

	// 2. Write data multiple times
	testData1 := []byte("First line of output\n")
	err = manager.WriteOutput(capture, testData1)
	require.NoError(t, err)

	testData2 := []byte("Second line of output\n")
	err = manager.WriteOutput(capture, testData2)
	require.NoError(t, err)

	testData3 := []byte("Final line of output\n")
	err = manager.WriteOutput(capture, testData3)
	require.NoError(t, err)

	// Verify buffer state
	expectedTotalSize := int64(len(testData1) + len(testData2) + len(testData3))
	assert.Equal(t, expectedTotalSize, capture.CurrentSize)

	// 3. Finalize output
	err = manager.FinalizeOutput(capture)
	require.NoError(t, err)

	// 4. Verify final file
	finalContent, err := os.ReadFile(outputPath)
	require.NoError(t, err)

	expectedContent := append(append(testData1, testData2...), testData3...)
	assert.Equal(t, expectedContent, finalContent)

	// 5. Cleanup
	err = manager.CleanupOutput(capture)
	require.NoError(t, err)

	// Verify capture was cleaned
	assert.Equal(t, int64(0), capture.CurrentSize)
	assert.Nil(t, capture.FileHandle)
	assert.Empty(t, capture.TempFilePath)

	// Verify mock expectations
	mockPathValidator.AssertExpectations(t)
	mockSecurityValidator.AssertExpectations(t)
}

func TestIsPathWithinDirectory(t *testing.T) {
	tests := []struct {
		name       string
		targetPath string
		dirPath    string
		expected   bool
	}{
		// Valid cases - path within directory
		{
			name:       "file_in_directory",
			targetPath: "/home/user/app/file.txt",
			dirPath:    "/home/user/app",
			expected:   true,
		},
		{
			name:       "nested_path_in_directory",
			targetPath: "/home/user/app/subdir/file.txt",
			dirPath:    "/home/user/app",
			expected:   true,
		},
		{
			name:       "directory_with_trailing_slash",
			targetPath: "/home/user/app/file.txt",
			dirPath:    "/home/user/app/",
			expected:   true,
		},
		{
			name:       "relative_paths_within",
			targetPath: "app/subdir/file.txt",
			dirPath:    "app",
			expected:   true,
		},

		// Security vulnerability cases - false positives with old logic
		{
			name:       "similar_prefix_different_directory",
			targetPath: "/home/user/application/file.txt",
			dirPath:    "/home/user/app",
			expected:   false, // This would be true with strings.HasPrefix, but should be false
		},
		{
			name:       "longer_path_with_same_prefix",
			targetPath: "/home/user/app_backup/sensitive.txt",
			dirPath:    "/home/user/app",
			expected:   false, // This would be true with strings.HasPrefix, but should be false
		},
		{
			name:       "completely_different_path_same_start",
			targetPath: "/home/user/app2/config/secret.txt",
			dirPath:    "/home/user/app",
			expected:   false, // This would be true with strings.HasPrefix, but should be false
		},

		// Edge cases
		{
			name:       "exact_same_path",
			targetPath: "/home/user/app",
			dirPath:    "/home/user/app",
			expected:   false, // Directory itself is not "within" the directory
		},
		{
			name:       "empty_directory_path",
			targetPath: "/home/user/app/file.txt",
			dirPath:    "",
			expected:   false,
		},
		{
			name:       "root_directory",
			targetPath: "/home/user/app/file.txt",
			dirPath:    "/",
			expected:   true,
		},
		{
			name:       "path_with_dots",
			targetPath: "/home/user/app/../app/file.txt",
			dirPath:    "/home/user/app",
			expected:   true, // filepath.Clean will resolve the dots
		},

		// Invalid cases - path outside directory
		{
			name:       "parent_directory",
			targetPath: "/home/user/file.txt",
			dirPath:    "/home/user/app",
			expected:   false,
		},
		{
			name:       "sibling_directory",
			targetPath: "/home/user/other/file.txt",
			dirPath:    "/home/user/app",
			expected:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isPathWithinDirectory(tt.targetPath, tt.dirPath)
			assert.Equal(t, tt.expected, result,
				"isPathWithinDirectory(%q, %q) = %v, want %v",
				tt.targetPath, tt.dirPath, result, tt.expected)
		})
	}
}

func TestEvaluateSecurityRisk_PathBoundarySecurityFix(t *testing.T) {
	// This test specifically validates the security fix for path boundary checking
	manager := &DefaultOutputCaptureManager{}

	tests := []struct {
		name        string
		path        string
		workDir     string
		expected    runnertypes.RiskLevel
		description string
	}{
		{
			name:        "legitimate_path_within_workdir",
			path:        "/home/user/app/output.txt",
			workDir:     "/home/user/app",
			expected:    runnertypes.RiskLevelLow,
			description: "Legitimate file within working directory should be low risk",
		},
		{
			name:        "security_vulnerability_similar_prefix",
			path:        "/home/user/application/sensitive.txt",
			workDir:     "/home/user/app",
			expected:    runnertypes.RiskLevelMedium, // NOT low risk - this is the security fix
			description: "Path with similar prefix but outside workdir should NOT be low risk",
		},
		{
			name:        "security_vulnerability_prefix_with_suffix",
			path:        "/home/user/app_backup/config.txt",
			workDir:     "/home/user/app",
			expected:    runnertypes.RiskLevelMedium, // NOT low risk - this is the security fix
			description: "Path starting with workdir name + suffix should NOT be low risk",
		},
		{
			name:        "security_vulnerability_numeric_suffix",
			path:        "/home/user/app2/secrets.txt",
			workDir:     "/home/user/app",
			expected:    runnertypes.RiskLevelMedium, // NOT low risk - this is the security fix
			description: "Path with numeric suffix should NOT be low risk",
		},
		{
			name:        "nested_legitimate_path",
			path:        "/home/user/app/subdir/deep/file.txt",
			workDir:     "/home/user/app",
			expected:    runnertypes.RiskLevelLow,
			description: "Deeply nested legitimate path should be low risk",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := manager.evaluateSecurityRisk(tt.path, tt.workDir)
			assert.Equal(t, tt.expected, result,
				"evaluateSecurityRisk(%q, %q) = %v, want %v: %s",
				tt.path, tt.workDir, result, tt.expected, tt.description)

			// Additional validation for security-critical test cases
			if tt.name == "security_vulnerability_similar_prefix" ||
				tt.name == "security_vulnerability_prefix_with_suffix" ||
				tt.name == "security_vulnerability_numeric_suffix" {
				assert.NotEqual(t, runnertypes.RiskLevelLow, result,
					"SECURITY CRITICAL: Path %q should NOT be classified as low risk when workDir is %q. "+
						"This would be a false positive with vulnerable string prefix checking.",
					tt.path, tt.workDir)
			}
		})
	}
}
