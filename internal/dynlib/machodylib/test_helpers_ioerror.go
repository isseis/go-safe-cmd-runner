//go:build test && darwin

package machodylib

import (
	"errors"
	"io"
	"os"
	"sync"
	"time"
)

// errorFile is a test implementation of safefileio.File whose Seek and Read
// behavior can be configured to simulate I/O failures (fail-closed path tests).
//
// It is package-internal because the test wants to drive the production code
// through a controlled File implementation. Only the Seek and Read hooks are
// needed for HasDynamicLibDeps error-path tests; other File methods return
// inert values sufficient for that function's use.
type errorFile struct {
	mu sync.Mutex

	// data backs Read/ReadAt when readErr is nil.
	data []byte
	// pos is the current offset used by Read/Seek.
	pos int64

	// seekErr causes Seek to return it.
	seekErr error
	// readErr causes Read to return it (overrides EOF behaviour).
	readErr error
}

func newErrorFile(data []byte) *errorFile {
	return &errorFile{data: data}
}

// Read returns readErr when set, otherwise reads from data.
func (f *errorFile) Read(p []byte) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.readErr != nil {
		return 0, f.readErr
	}
	if f.pos >= int64(len(f.data)) {
		return 0, io.EOF
	}
	n := copy(p, f.data[f.pos:])
	f.pos += int64(n)
	return n, nil
}

// Write is unused by HasDynamicLibDeps but required by the File interface.
func (f *errorFile) Write(_ []byte) (int, error) {
	return 0, errors.New("errorFile: Write not supported")
}

// Seek returns seekErr when set, otherwise updates pos.
func (f *errorFile) Seek(offset int64, whence int) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.seekErr != nil {
		return 0, f.seekErr
	}
	var newPos int64
	switch whence {
	case io.SeekStart:
		newPos = offset
	case io.SeekCurrent:
		newPos = f.pos + offset
	case io.SeekEnd:
		newPos = int64(len(f.data)) + offset
	default:
		return 0, errors.New("invalid whence")
	}
	if newPos < 0 {
		return 0, errors.New("negative position")
	}
	f.pos = newPos
	return f.pos, nil
}

// ReadAt is required by the File interface but unused by HasDynamicLibDeps.
func (f *errorFile) ReadAt(p []byte, off int64) (int, error) {
	if off < 0 || off >= int64(len(f.data)) {
		return 0, io.EOF
	}
	n := copy(p, f.data[off:])
	if n < len(p) {
		return n, io.EOF
	}
	return n, nil
}

// Chmod is required by the File interface but unused by HasDynamicLibDeps.
func (f *errorFile) Chmod(_ os.FileMode) error { return nil }

// Close is required by the File interface.
func (f *errorFile) Close() error { return nil }

// Stat returns a synthetic file info with the data length as size.
func (f *errorFile) Stat() (os.FileInfo, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return &errorFileInfo{size: int64(len(f.data))}, nil
}

// Truncate is required by the File interface but unused by HasDynamicLibDeps.
func (f *errorFile) Truncate(_ int64) error { return nil }

// errorFileInfo is a minimal os.FileInfo for errorFile.
type errorFileInfo struct{ size int64 }

func (fi *errorFileInfo) Name() string       { return "errorFile" }
func (fi *errorFileInfo) Size() int64        { return fi.size }
func (fi *errorFileInfo) Mode() os.FileMode  { return 0o600 }
func (fi *errorFileInfo) ModTime() time.Time { return time.Time{} }
func (fi *errorFileInfo) IsDir() bool        { return false }
func (fi *errorFileInfo) Sys() any           { return nil }
