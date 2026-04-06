// Package shebang provides utilities for parsing and validating shebang lines
// in script files to enable interpreter tracking and verification.
package shebang

import "errors"

var (
	// ErrShebangLineTooLong is returned when no newline is found within
	// maxShebangBytes (256) bytes.
	ErrShebangLineTooLong = errors.New("shebang line exceeds 256-byte limit")

	// ErrShebangCR is returned when the shebang line contains a carriage
	// return (\r) character.
	ErrShebangCR = errors.New("shebang line contains carriage return")

	// ErrEmptyInterpreterPath is returned when the shebang line has no
	// interpreter path (e.g., "#!\n").
	ErrEmptyInterpreterPath = errors.New("empty interpreter path in shebang")

	// ErrInterpreterNotAbsolute is returned when the interpreter path is
	// not an absolute path (e.g., "#!python3").
	ErrInterpreterNotAbsolute = errors.New("interpreter path is not absolute")

	// ErrMissingEnvCommand is returned when env is used without a command
	// name (e.g., "#!/usr/bin/env\n").
	ErrMissingEnvCommand = errors.New("missing command name after env")

	// ErrEnvFlagNotSupported is returned when the env command has flags
	// (e.g., "#!/usr/bin/env -S python3").
	ErrEnvFlagNotSupported = errors.New("env flags are not supported")

	// ErrEnvAssignmentNotSupported is returned when the env command has
	// environment variable assignments (e.g., "#!/usr/bin/env PYTHONPATH=. python3").
	ErrEnvAssignmentNotSupported = errors.New("env variable assignments are not supported")

	// ErrCommandNotFound is returned when the command name cannot be
	// resolved via PATH (e.g., "#!/usr/bin/env nonexistent_cmd").
	ErrCommandNotFound = errors.New("command not found in PATH")
)
