// Package cmdcommon provides common functionality for command-line tools.
package cmdcommon

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/isseis/go-safe-cmd-runner/internal/filevalidator"
)

var (
	// ErrFileArgumentRequired is returned when the required file argument is missing.
	ErrFileArgumentRequired = errors.New("file argument is required")
	// ErrGetCurrentDir is returned when there's an error getting the current directory.
	ErrGetCurrentDir = errors.New("error getting current directory")
	// ErrCreateHashDir is returned when there's an error creating or accessing the hash directory.
	ErrCreateHashDir = errors.New("error creating/accessing hash directory")
)

const (
	// HashDirPermissions defines the default permissions for the hash directory.
	HashDirPermissions = 0o750
)

// Build-time variables (set via ldflags)
var (
	DefaultHashDirectory = "/usr/local/etc/go-safe-cmd-runner/hashes" // fallback default
)

// Config holds the common configuration for commands.
type Config struct {
	File    string
	HashDir string
}

// ParseFlags parses the common command-line flags.
// It returns a Config and any error encountered.
func ParseFlags() (*Config, error) {
	var (
		file    = flag.String("file", "", "Path to the file to process")
		hashDir = flag.String("hash-dir", "", "Directory containing hash files (default: "+DefaultHashDirectory+")")
	)
	flag.Parse()

	if *file == "" {
		flag.Usage()
		return nil, ErrFileArgumentRequired
	}

	// Set default hash directory to current directory if not specified
	dir := *hashDir
	if dir == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("%w: %w", ErrGetCurrentDir, err)
		}
		dir = cwd
	}

	// Ensure the directory exists
	if err := os.MkdirAll(dir, HashDirPermissions); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrCreateHashDir, err)
	}

	return &Config{
		File:    *file,
		HashDir: dir,
	}, nil
}

// CreateValidator creates a new file validator with the specified hasher.
func CreateValidator(hashDir string) (*filevalidator.Validator, error) {
	return filevalidator.New(&filevalidator.SHA256{}, hashDir)
}

// PrintUsage prints the usage message for the command.
func PrintUsage() {
	fmt.Fprintf(os.Stderr, "Usage: %s -file <path> [-hash-dir <dir>]\n", filepath.Base(os.Args[0]))
	flag.PrintDefaults()
}
