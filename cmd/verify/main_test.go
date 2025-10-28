package main

import (
	"bytes"
	"errors"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
			if err != nil {
				t.Fatalf("parseFlags() error = %v, want nil", err)
			}

			if cfg.File != tt.wantFile {
				t.Errorf("parseFlags() File = %v, want %v", cfg.File, tt.wantFile)
			}

			if tt.checkHashDir {
				// When hash-dir is not specified, it should default to current directory
				cwd, err := os.Getwd()
				if err != nil {
					t.Fatalf("os.Getwd() error = %v", err)
				}
				if cfg.HashDir != cwd {
					t.Errorf("parseFlags() HashDir = %v, want %v (current directory)", cfg.HashDir, cwd)
				}
			} else if cfg.HashDir != "/tmp/hashes" {
				t.Errorf("parseFlags() HashDir = %v, want %v", cfg.HashDir, "/tmp/hashes")
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

	if cfg != nil {
		t.Errorf("parseFlags() config = %v, want nil", cfg)
	}

	if err == nil {
		t.Fatal("parseFlags() error = nil, want error")
	}

	if !errors.Is(err, ErrFileArgumentRequired) {
		t.Errorf("parseFlags() error = %v, want ErrFileArgumentRequired", err)
	}

	// Read captured output (just to consume it, we don't need to check it)
	buf := new(bytes.Buffer)
	buf.ReadFrom(r)
}

func TestParseFlags_InvalidHashDir(t *testing.T) {
	// Create a test directory with no write permissions
	tempDir := t.TempDir()
	noWriteDir := filepath.Join(tempDir, "no_write")

	// Create directory with read-only permissions
	if err := os.Mkdir(noWriteDir, 0o444); err != nil {
		t.Fatalf("Failed to create test directory: %v", err)
	}
	defer os.Chmod(noWriteDir, 0o755) // Restore permissions for cleanup

	// Try to create a subdirectory that will fail due to permissions
	invalidHashDir := filepath.Join(noWriteDir, "hashes")

	// Reset flags
	flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ContinueOnError)

	oldArgs := os.Args
	os.Args = []string{"cmd", "-file", "test.txt", "-hash-dir", invalidHashDir}
	defer func() { os.Args = oldArgs }()

	cfg, err := parseFlags()

	if cfg != nil {
		t.Errorf("parseFlags() config = %v, want nil", cfg)
	}

	if err == nil {
		t.Fatal("parseFlags() error = nil, want error")
	}

	if !errors.Is(err, ErrCreateHashDir) {
		t.Errorf("parseFlags() error = %v, want ErrCreateHashDir", err)
	}
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
	if !strings.Contains(output, "Usage:") {
		t.Error("printUsage() output does not contain 'Usage:'")
	}

	if !strings.Contains(output, "-file") {
		t.Error("printUsage() output does not contain '-file' flag")
	}

	if !strings.Contains(output, "-hash-dir") {
		t.Error("printUsage() output does not contain '-hash-dir' flag")
	}
}
