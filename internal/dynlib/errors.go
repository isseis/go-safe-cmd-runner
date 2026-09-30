// Package dynlib provides error types shared between ELF and Mach-O dynamic
// library analysis packages (elfdynlib and machodylib).
//
// These error types are format-independent and represent failure modes that
// occur during dependency resolution and verification regardless of whether
// the binary is an ELF or Mach-O file.
package dynlib

import (
	"strconv"

	"github.com/isseis/go-safe-cmd-runner/internal/errmsg"
)

// ErrRecursionDepthExceeded indicates that dependency resolution exceeded the
// maximum allowed depth. This typically indicates an abnormal library configuration
// or a missed circular dependency.
type ErrRecursionDepthExceeded struct {
	Depth    int
	MaxDepth int
	SOName   string
}

func (e *ErrRecursionDepthExceeded) Error() string {
	return e.StructuredMessage().String()
}

// StructuredMessage declares the SOName as a path and the depth numbers as free
// text, so a library name that contains a sensitive word is not replaced whole.
func (e *ErrRecursionDepthExceeded) StructuredMessage() errmsg.Message {
	return errmsg.NewMessage(
		errmsg.Const("dependency resolution depth exceeded: "),
		errmsg.Path(e.SOName),
		errmsg.Const(" at depth "),
		errmsg.Text(strconv.Itoa(e.Depth)),
		errmsg.Const(" (max "),
		errmsg.Text(strconv.Itoa(e.MaxDepth)),
		errmsg.Const(")"),
	)
}

// ErrLibraryHashMismatch indicates that a library's hash does not match the recorded value
// (Stage 1 verification failure).
type ErrLibraryHashMismatch struct {
	SOName       string
	Path         string
	ExpectedHash string
	ActualHash   string
}

func (e *ErrLibraryHashMismatch) Error() string {
	return e.StructuredMessage().String()
}

// StructuredMessage declares the SOName and the library path as paths and the
// hashes as free text.
func (e *ErrLibraryHashMismatch) StructuredMessage() errmsg.Message {
	return errmsg.NewMessage(
		errmsg.Const("dynamic library hash mismatch: "),
		errmsg.Path(e.SOName),
		errmsg.Const("\n  path: "),
		errmsg.Path(e.Path),
		errmsg.Const("\n  expected hash: "),
		errmsg.Text(e.ExpectedHash),
		errmsg.Const("\n  actual hash: "),
		errmsg.Text(e.ActualHash),
		errmsg.Const("\n  please re-run 'record' command"),
	)
}

// ErrEmptyLibraryPath indicates that a LibEntry has an empty path,
// which should never happen in valid records (defensive check).
type ErrEmptyLibraryPath struct {
	SOName string
}

func (e *ErrEmptyLibraryPath) Error() string {
	return e.StructuredMessage().String()
}

// StructuredMessage declares the SOName as a path.
func (e *ErrEmptyLibraryPath) StructuredMessage() errmsg.Message {
	return errmsg.NewMessage(
		errmsg.Const("incomplete record: empty path for library "),
		errmsg.Path(e.SOName),
		errmsg.Const("\n  please re-run 'record' command"),
	)
}

// ErrDynLibDepsRequired indicates that a DynLibDeps record is required
// but not present for a binary.
type ErrDynLibDepsRequired struct {
	BinaryPath string
}

func (e *ErrDynLibDepsRequired) Error() string {
	return e.StructuredMessage().String()
}

// StructuredMessage declares the binary path as a path.
func (e *ErrDynLibDepsRequired) StructuredMessage() errmsg.Message {
	return errmsg.NewMessage(
		errmsg.Const("dynamic library dependencies not recorded for binary: "),
		errmsg.Path(e.BinaryPath),
		errmsg.Const("\n  please re-run 'record' command"),
	)
}
