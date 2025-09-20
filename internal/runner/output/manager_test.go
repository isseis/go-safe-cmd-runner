package output

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/security"
)

// Test errors for manager_test
var (
	ErrPathTraversalDetected = errors.New("path traversal detected")
	ErrPermissionDenied      = errors.New("permission denied")
	ErrTempCreationFailed    = errors.New("temp creation failed")
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
			},
			wantErr: false,
			validateCapture: func(t *testing.T, capture *Capture) {
				assert.Equal(t, "/tmp/output.txt", capture.OutputPath)
				assert.NotNil(t, capture.Buffer)
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
			},
			wantErr: false,
			validateCapture: func(t *testing.T, capture *Capture) {
				assert.Equal(t, "/home/user/project/output/result.txt", capture.OutputPath)
				assert.Equal(t, int64(2048), capture.MaxSize)
			},
		},
		{
			name:       "path_validation_error",
			outputPath: "../../../etc/passwd",
			workDir:    "/home/user",
			maxSize:    1024,
			setupMocks: func(pv *MockPathValidator, _ *MockFileManager, _ *MockSecurityValidator) {
				pv.On("ValidateAndResolvePath", "../../../etc/passwd", "/home/user").Return("", ErrPathTraversalDetected)
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
				fm.On("EnsureDirectory", "/tmp/nonexistent/deeply/nested").Return(ErrPermissionDenied)
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
			// Create capture with initial state
			capture := &Capture{
				OutputPath:  "/tmp/test.txt",
				Buffer:      &bytes.Buffer{},
				CurrentSize: tt.initialSize,
				MaxSize:     tt.maxSize,
				StartTime:   time.Now(),
			}

			// Add some initial data if needed
			if tt.initialSize > 0 {
				initialData := make([]byte, tt.initialSize)
				capture.Buffer.Write(initialData)
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

				// Check if the new data was written to buffer
				if len(tt.expectedBuffer) > 0 {
					bufferData := capture.Buffer.Bytes()
					// The buffer should contain the initial data plus the new data
					if tt.initialSize > 0 {
						assert.True(t, len(bufferData) >= len(tt.expectedBuffer))
						// Check that the new data appears at the end
						assert.Equal(t, tt.expectedBuffer, bufferData[len(bufferData)-len(tt.expectedBuffer):])
					} else {
						assert.Equal(t, tt.expectedBuffer, bufferData)
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
				// Mock temp file creation
				mockFile, err := os.CreateTemp("", "test_temp_*.tmp")
				if err != nil {
					panic(err)
				}
				defer os.Remove(mockFile.Name())

				fm.On("CreateTempFile", mock.AnythingOfType("string"), "output_*.tmp").Return(mockFile, nil)
				fm.On("WriteToTemp", mockFile, mock.AnythingOfType("[]uint8")).Return(len([]byte("test output content\nline 2\n")), nil)
				fm.On("MoveToFinal", mock.AnythingOfType("string"), "/tmp/final.txt").Return(nil)
				fm.On("RemoveTemp", mock.AnythingOfType("string")).Return(nil)
			},
			wantErr: false,
		},
		{
			name:          "empty_buffer_finalization",
			bufferContent: []byte{},
			setupMocks: func(fm *MockFileManager) {
				mockFile, err := os.CreateTemp("", "test_temp_*.tmp")
				if err != nil {
					panic(err)
				}
				defer os.Remove(mockFile.Name())

				fm.On("CreateTempFile", mock.AnythingOfType("string"), "output_*.tmp").Return(mockFile, nil)
				fm.On("WriteToTemp", mockFile, mock.AnythingOfType("[]uint8")).Return(0, nil)
				fm.On("MoveToFinal", mock.AnythingOfType("string"), "/tmp/empty.txt").Return(nil)
				fm.On("RemoveTemp", mock.AnythingOfType("string")).Return(nil)
			},
			wantErr: false,
		},
		{
			name:          "temp_file_creation_error",
			bufferContent: []byte("content"),
			setupMocks: func(fm *MockFileManager) {
				fm.On("CreateTempFile", mock.AnythingOfType("string"), "output_*.tmp").Return((*os.File)(nil), ErrTempCreationFailed)
			},
			wantErr:    true,
			errMessage: "temp creation failed",
		},
		{
			name:          "file_move_error",
			bufferContent: []byte("content"),
			setupMocks: func(fm *MockFileManager) {
				mockFile, err := os.CreateTemp("", "test_temp_*.tmp")
				if err != nil {
					panic(err)
				}
				defer os.Remove(mockFile.Name())

				fm.On("CreateTempFile", mock.AnythingOfType("string"), "output_*.tmp").Return(mockFile, nil)
				fm.On("WriteToTemp", mockFile, mock.AnythingOfType("[]uint8")).Return(len([]byte("content")), nil)
				fm.On("MoveToFinal", mock.AnythingOfType("string"), "/tmp/error.txt").Return(ErrPermissionDenied)
				fm.On("RemoveTemp", mock.AnythingOfType("string")).Return(nil)
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

			// Create capture with buffer content
			buffer := &bytes.Buffer{}
			buffer.Write(tt.bufferContent)

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
				OutputPath:  outputPath,
				Buffer:      buffer,
				CurrentSize: int64(len(tt.bufferContent)),
				StartTime:   time.Now(),
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
	manager := &DefaultOutputCaptureManager{}

	// Create capture with some data
	buffer := &bytes.Buffer{}
	buffer.WriteString("test data to be cleaned")

	capture := &Capture{
		OutputPath:  "/tmp/test.txt",
		Buffer:      buffer,
		CurrentSize: 23,
		StartTime:   time.Now(),
	}

	// Verify initial state
	assert.Equal(t, int64(23), capture.CurrentSize)
	assert.Equal(t, 23, capture.Buffer.Len())

	// Call CleanupOutput
	err := manager.CleanupOutput(capture)

	// Validate results
	assert.NoError(t, err)
	assert.Equal(t, int64(0), capture.CurrentSize)
	assert.Equal(t, 0, capture.Buffer.Len())
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
				pv.On("ValidateAndResolvePath", "../../../etc/passwd", "/home/user").Return("", ErrPathTraversalDetected)
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
				sv.On("ValidateOutputWritePermission", "/etc/sensitive", mock.AnythingOfType("int")).Return(ErrPermissionDenied)
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

	// Verify buffer was cleaned
	assert.Equal(t, int64(0), capture.CurrentSize)
	assert.Equal(t, 0, capture.Buffer.Len())

	// Verify mock expectations
	mockPathValidator.AssertExpectations(t)
	mockSecurityValidator.AssertExpectations(t)
}
