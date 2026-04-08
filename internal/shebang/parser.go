package shebang

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/isseis/go-safe-cmd-runner/internal/safefileio"
)

const (
	// maxShebangBytes is the maximum number of bytes to read for shebang detection.
	// Matches Linux kernel's BINPRM_BUF_SIZE.
	maxShebangBytes = 256

	// shebangPrefixLen is the length of the "#!" shebang prefix.
	shebangPrefixLen = 2

	// shebangPrefix is the magic bytes for shebang detection.
	shebangPrefix = "#!"
)

// Info holds the parsed result of a shebang line.
type Info struct {
	// RawInterpreterPath is the interpreter path exactly as written in the
	// shebang line, before symlink resolution (e.g., "/bin/sh" or "/usr/bin/env").
	RawInterpreterPath string

	// InterpreterPath is the absolute path to the interpreter binary,
	// resolved via filepath.EvalSymlinks.
	// For env form (e.g., "#!/usr/bin/env python3"), this is the resolved
	// path of env (e.g., "/usr/bin/env").
	// For direct form (e.g., "#!/bin/sh"), this is the resolved path of
	// the interpreter (e.g., "/usr/bin/dash" if /bin/sh is a symlink).
	InterpreterPath string

	// CommandName is the command name passed to env (e.g., "python3").
	// Empty for direct form.
	CommandName string

	// ResolvedPath is the resolved absolute path of CommandName via
	// exec.LookPath + filepath.EvalSymlinks.
	// Empty for direct form.
	ResolvedPath string
}

// Parse reads the shebang line from the file at filePath and returns the
// parsed interpreter information. Returns (nil, nil) if the file does not
// start with "#!" (not a script).
//
// fs is used to open the file safely (symlink protection, permission checks).
// Pass safefileio.NewFileSystem(safefileio.FileSystemConfig{}) for production use; pass a mock for testing.
//
// Errors:
//   - ErrShebangLineTooLong: no newline within 256 bytes
//   - ErrShebangCR: carriage return found in shebang line
//   - ErrEmptyInterpreterPath: no interpreter path after "#!"
//   - ErrInterpreterNotAbsolute: interpreter path is relative
//   - ErrMissingEnvCommand: env used without command name
//   - ErrEnvFlagNotSupported: env argument starts with "-"
//   - ErrEnvAssignmentNotSupported: env argument contains "="
//   - ErrCommandNotFound: command not found via exec.LookPath
func Parse(filePath string, fs safefileio.FileSystem) (*Info, error) {
	absPath, err := filepath.Abs(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve path: %w", err)
	}
	f, err := fs.SafeOpenFile(absPath, os.O_RDONLY, 0)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}
	defer func() { _ = f.Close() }()

	buf := make([]byte, maxShebangBytes)
	n, err := io.ReadFull(f, buf)
	if err != nil {
		if errors.Is(err, io.EOF) {
			// Empty file: not a shebang script.
			return nil, nil
		}
		if !errors.Is(err, io.ErrUnexpectedEOF) {
			// Real I/O error.
			return nil, fmt.Errorf("failed to read shebang: %w", err)
		}
		// io.ErrUnexpectedEOF: file shorter than maxShebangBytes; continue with n bytes.
	}
	buf = buf[:n]

	// Check for "#!" prefix.
	if n < shebangPrefixLen || string(buf[:shebangPrefixLen]) != shebangPrefix {
		return nil, nil // Not a shebang script
	}

	// Find newline.
	line := buf[2:] // Skip "#!"
	nlIdx := -1
	for i, b := range line {
		if b == '\n' {
			nlIdx = i
			break
		}
	}
	if nlIdx == -1 {
		return nil, ErrShebangLineTooLong
	}
	line = line[:nlIdx]

	// Check for \r.
	if bytes.ContainsRune(line, '\r') {
		return nil, ErrShebangCR
	}

	// Skip leading whitespace and tokenize.
	content := strings.TrimLeft(string(line), " \t")
	tokens := strings.Fields(content)
	if len(tokens) == 0 {
		return nil, ErrEmptyInterpreterPath
	}

	interpreterPath := tokens[0]

	// Validate absolute path.
	if !filepath.IsAbs(interpreterPath) {
		return nil, fmt.Errorf("%w: %s", ErrInterpreterNotAbsolute, interpreterPath)
	}

	// Detect env-form based on the original shebang token, before symlink
	// resolution. Checking after EvalSymlinks would misclassify env-form
	// shebangs when /usr/bin/env is a symlink (e.g., to busybox).
	isEnvForm := filepath.Base(interpreterPath) == "env"

	// Resolve symlinks for interpreter path.
	resolvedInterpreter, err := filepath.EvalSymlinks(interpreterPath)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve interpreter path %s: %w",
			interpreterPath, err)
	}

	// Check if env form using original token, but pass resolved path.
	if isEnvForm {
		return parseEnvForm(interpreterPath, resolvedInterpreter, tokens[1:])
	}

	// Direct form.
	return &Info{
		RawInterpreterPath: interpreterPath,
		InterpreterPath:    resolvedInterpreter,
	}, nil
}

// parseEnvForm handles "#!/usr/bin/env <cmd>" shebangs.
func parseEnvForm(rawEnvPath, envPath string, args []string) (*Info, error) {
	if len(args) == 0 {
		return nil, ErrMissingEnvCommand
	}

	cmdArg := args[0]

	// Check for flags.
	if strings.HasPrefix(cmdArg, "-") {
		return nil, fmt.Errorf("%w: %s", ErrEnvFlagNotSupported, cmdArg)
	}

	// Check for variable assignments.
	if strings.Contains(cmdArg, "=") {
		return nil, fmt.Errorf("%w: %s", ErrEnvAssignmentNotSupported, cmdArg)
	}

	// Resolve command via PATH using the shared algorithm (see env_resolver.go).
	resolvedCmd, err := ResolveEnvCommand(cmdArg, os.Getenv("PATH"))
	if err != nil {
		return nil, err
	}

	return &Info{
		RawInterpreterPath: rawEnvPath,
		InterpreterPath:    envPath,
		CommandName:        cmdArg,
		ResolvedPath:       resolvedCmd,
	}, nil
}

// IsShebangScript checks if the file at filePath starts with "#!" magic bytes.
// Returns false, nil for files that are too small.
// Returns an error when the file cannot be opened.
//
// fs is used to open the file safely (symlink protection, permission checks).
// Pass safefileio.NewFileSystem(safefileio.FileSystemConfig{}) for production use; pass a mock for testing.
func IsShebangScript(filePath string, fs safefileio.FileSystem) (bool, error) {
	absPath, err := filepath.Abs(filePath)
	if err != nil {
		return false, fmt.Errorf("failed to resolve path: %w", err)
	}
	f, err := fs.SafeOpenFile(absPath, os.O_RDONLY, 0)
	if err != nil {
		return false, fmt.Errorf("failed to open file: %w", err)
	}
	defer func() { _ = f.Close() }()

	buf := make([]byte, shebangPrefixLen)
	n, err := f.Read(buf)
	if err != nil {
		if errors.Is(err, io.EOF) {
			// File is smaller than the shebang prefix; treat as "not a shebang".
			return false, nil
		}
		// Propagate non-EOF I/O errors so callers can distinguish real failures.
		return false, fmt.Errorf("failed to read file header: %w", err)
	}
	if n < shebangPrefixLen {
		// Too few bytes to contain a shebang prefix.
		return false, nil
	}
	return string(buf) == shebangPrefix, nil
}
