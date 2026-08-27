//go:build !cgo

package groupmembership

import "fmt"

// This file holds the part of the file-based scan that only the no-cgo
// enumeration uses. It is separate from membership_files.go, which the cgo
// test build also compiles, because a function with no caller in that build
// is reported as unused there.

// verdict returns an incomplete verdict when any line was skipped, and a
// complete verdict otherwise. A skipped line makes the whole scan
// incomplete: the parse failure is usually in the GID field, so which group
// the line belonged to is by definition unknown.
func (m malformedLines) verdict() completenessVerdict {
	if m.count == 0 {
		return completeVerdict()
	}
	return incompleteVerdict(causeMalformedLine,
		fmt.Sprintf("%d line(s) skipped, first at %s", m.count, m.first))
}
