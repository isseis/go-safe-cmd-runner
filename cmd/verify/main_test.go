package main

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseFlags_Success(t *testing.T) {
	tests := []struct {
		name         string
		args         []string
		wantFile     string
		checkHashDir bool
	}{
		{
			name:         "with required file argument",
			args:         []string{"-file", "test.txt"},
			wantFile:     "test.txt",
			checkHashDir: true,
		},
		{
			name:         "with file and hash-dir",
			args:         []string{"-file", "test.txt", "-hash-dir", "/tmp/hashes"},
			wantFile:     "test.txt",
			checkHashDir: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset flags for each test
			flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ContinueOnError)

			// Set os.Args to simulate command-line arguments
			oldArgs := os.Args
			os.Args = append([]string{"cmd"}, tt.args...)
			defer func() { os.Args = oldArgs }()

			cfg, err := parseFlags()
			require.NoError(t, err)

			assert.Equal(t, tt.wantFile, cfg.File)

			if tt.checkHashDir {
				// When hash-dir is not specified, it should default to current directory
				cwd, err := os.Getwd()
				require.NoError(t, err)
				assert.Equal(t, cwd, cfg.HashDir)
			} else {
				assert.Equal(t, "/tmp/hashes", cfg.HashDir)
			}
		})
	}
}

func TestParseFlags_MissingRequiredArg(t *testing.T) {
	// Reset flags
	flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ContinueOnError)

	// Set os.Args without -file argument
	oldArgs := os.Args
	os.Args = []string{"cmd"}
	defer func() { os.Args = oldArgs }()

	// Capture stderr to avoid cluttering test output
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w
	defer func() { os.Stderr = oldStderr }()

	cfg, err := parseFlags()

	// Close writer and restore stderr
	w.Close()
	os.Stderr = oldStderr

	assert.Nil(t, cfg)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrFileArgumentRequired)

	// Read captured output (just to consume it, we don't need to check it)
	buf := new(bytes.Buffer)
	buf.ReadFrom(r)
}

func TestParseFlags_InvalidHashDir(t *testing.T) {
	// Create a test directory with no write permissions
	tempDir := t.TempDir()
	noWriteDir := filepath.Join(tempDir, "no_write")

	// Create directory with read-only permissions
	require.NoError(t, os.Mkdir(noWriteDir, 0o444))
	defer os.Chmod(noWriteDir, 0o755) // Restore permissions for cleanup

	// Try to create a subdirectory that will fail due to permissions
	invalidHashDir := filepath.Join(noWriteDir, "hashes")

	// Reset flags
	flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ContinueOnError)

	oldArgs := os.Args
	os.Args = []string{"cmd", "-file", "test.txt", "-hash-dir", invalidHashDir}
	defer func() { os.Args = oldArgs }()

	cfg, err := parseFlags()

	assert.Nil(t, cfg)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrCreateHashDir)
}

func TestPrintUsage(t *testing.T) {
	// Capture stderr output
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	printUsage()

	// Close writer and restore stderr
	w.Close()
	os.Stderr = oldStderr

	// Read captured output
	buf := new(bytes.Buffer)
	buf.ReadFrom(r)
	output := buf.String()

	// Verify that usage message contains expected elements
	assert.Contains(t, output, "Usage:")
	assert.Contains(t, output, "-file")
	assert.Contains(t, output, "-hash-dir")
}
