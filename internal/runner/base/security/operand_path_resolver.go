package security

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/risktypes"
)

// ErrOperandResolution is returned when an operand path cannot be resolved to a
// trustworthy absolute path. The caller folds it into ZoneUnresolved (fail-closed:
// write/delete High, read Medium) rather than guessing a zone.
var ErrOperandResolution = errors.New("operand path resolution failed")

// lstatFunc and readlinkFunc are the read-only filesystem primitives the resolver
// depends on. They are injected so tests can substitute counting stubs (to assert
// the resolution cost stays linear) and so the resolver provably never follows
// a symlink implicitly: os.Stat (which resolves the final component) is
// deliberately NOT part of this set, because a stat that follows the leaf symlink
// would let a symlinked path element be classified by its target's ownership and
// defeat the zoning judgment.
type (
	lstatFunc    func(string) (fs.FileInfo, error)
	readlinkFunc func(string) (string, error)
)

// operandResolver resolves operands to canonical absolute paths and answers the
// Trusted predicate. It memoizes resolution within its own lifetime; callers
// create one instance per classification of a single command (one RunAsIdent), so
// the memo never mixes identities or base directories. It holds no live identity.
type operandResolver struct {
	lstat    lstatFunc
	readlink readlinkFunc
	// memo maps a confirmed-real node's pre-follow absolute path to itself. Only
	// existing, non-symlink nodes are memoized; folding shared parent chains to a
	// single lstat per node is what keeps resolution cost linear. Identity
	// dependent results (the Trusted predicate) are never cached here.
	memo map[string]string
}

// newOperandResolver builds a resolver over the given read-only primitives. The
// production path passes os.Lstat/os.Readlink; tests pass counting or scripted
// stubs.
func newOperandResolver(lstat lstatFunc, readlink readlinkFunc) *operandResolver {
	return &operandResolver{lstat: lstat, readlink: readlink, memo: make(map[string]string)}
}

// ResolveOperandPath resolves an operand to a normalized absolute path with the
// symlink chain followed (leaf and parents). A non-existent leaf is resolved to
// its deepest existing parent and the trailing components are folded in, so a
// not-yet-created destination still classifies by its real parent directory. A
// relative operand is resolved against base (the link's parent for `ln -s`
// relative targets, EffectiveWorkDir otherwise). It fails closed: a cycle, a
// depth-limit overflow, or a mid-chain readlink/lstat failure returns an error so
// the caller records ZoneUnresolved. It is read-only (lstat/readlink only).
//
// This is the single-operand convenience over a fresh resolver; the classifier
// reuses one operandResolver across a command's operands to share the memo.
func ResolveOperandPath(operand, base string, maxHops int) (string, error) {
	return newOperandResolver(os.Lstat, os.Readlink).resolve(operand, base, maxHops)
}

// resolve canonicalizes operand into an absolute, symlink-followed path.
//
// Invariant: the returned path's existing prefix contains no unresolved symlink
// element: every existing path element has been followed. Trailing components
// that do not exist are folded onto their deepest existing (real) ancestor, so a
// not-yet-created leaf is never placed under an unresolved symlink. The judgment
// is read-only and deterministic given a fixed filesystem state.
func (r *operandResolver) resolve(operand, base string, maxHops int) (string, error) {
	if operand == "" {
		return "", fmt.Errorf("%w: empty operand", ErrOperandResolution)
	}

	path := operand
	if !filepath.IsAbs(path) {
		// A relative operand is anchored at base (caller supplies an absolute
		// EffectiveWorkDir or, for an `ln -s` relative target, the link's parent).
		path = filepath.Join(base, path)
	}
	path = filepath.Clean(path)
	if !filepath.IsAbs(path) {
		return "", fmt.Errorf("%w: %q is not absolute (base %q)", ErrOperandResolution, operand, base)
	}

	// components are the path elements below root; a cleaned absolute path has no
	// "." or ".." elements to special-case.
	components := splitAbs(path)
	dest := string(filepath.Separator)
	visited := make(map[string]struct{})
	hops := 0

	for i := 0; i < len(components); i++ {
		name := components[i]
		if name == "" {
			continue
		}
		node := filepath.Join(dest, name)

		if cached, ok := r.memo[node]; ok {
			dest = cached
			continue
		}

		info, err := r.lstat(node)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				// node and every remaining component are non-existent; fold them
				// literally onto dest, which is a resolved real directory.
				parts := make([]string, 0, len(components)-i+1)
				parts = append(parts, dest, name)
				parts = append(parts, components[i+1:]...)
				return filepath.Join(parts...), nil
			}
			return "", fmt.Errorf("%w: lstat %q: %w", ErrOperandResolution, node, err)
		}

		if info.Mode()&fs.ModeSymlink == 0 {
			r.memo[node] = node
			dest = node
			continue
		}

		// Symlink: follow it. Count the hop and guard against cycles and runaway
		// chains so the hot path cannot be driven into unbounded filesystem I/O.
		hops++
		if hops > maxHops {
			return "", fmt.Errorf("%w: symlink depth exceeded at %q", ErrOperandResolution, node)
		}
		if _, seen := visited[node]; seen {
			return "", fmt.Errorf("%w: symlink cycle at %q", ErrOperandResolution, node)
		}
		visited[node] = struct{}{}

		target, err := r.readlink(node)
		if err != nil {
			return "", fmt.Errorf("%w: readlink %q: %w", ErrOperandResolution, node, err)
		}

		var newAbs string
		if filepath.IsAbs(target) {
			newAbs = filepath.Clean(target)
		} else {
			// A relative target is resolved against the link's own parent (dest),
			// not the original base, so a chain cannot be redirected by the caller's
			// working directory.
			newAbs = filepath.Clean(filepath.Join(dest, target))
		}

		// Re-process from root with the target spliced in front of the not-yet-
		// resolved suffix. The memo makes the already-resolved prefix replay
		// without new lstat calls.
		components = append(splitAbs(newAbs), components[i+1:]...)
		dest = string(filepath.Separator)
		i = -1
	}

	return dest, nil
}

// splitAbs splits a cleaned absolute path into its components below root.
func splitAbs(path string) []string {
	trimmed := strings.TrimPrefix(path, string(filepath.Separator))
	if trimmed == "" {
		return nil
	}
	return strings.Split(trimmed, string(filepath.Separator))
}

// isTrustedOperand reports whether resolved is a Trusted safe-zone operand: it
// lies within one of trustedDirs, and every path element from the parent of
// origin up to the filesystem root is non-writable by the run-as identity. The
// reference identity is the injected RunAsIdent, never the live euid, so the
// verdict is deterministic and identical between dry-run and runtime.
//
// Elements at or below origin are intentionally NOT checked: origin is the
// safe-zone anchor (EffectiveWorkDir / dedicated temp) that the run legitimately
// writes to, so requiring it to be non-writable would make Low unreachable. The
// check on origin's parent and above prevents the run-as identity from repointing
// the anchor itself.
func (r *operandResolver) isTrustedOperand(resolved, origin string, trustedDirs []string, ident risktypes.RunAsIdent) bool {
	if resolved == "" || origin == "" {
		return false
	}
	if !withinAnyDir(resolved, trustedDirs) {
		return false
	}

	dir := filepath.Dir(origin)
	for {
		info, err := r.lstat(dir)
		if err != nil {
			// Cannot verify this ancestor's ownership: fail closed (not Trusted).
			return false
		}
		if isWritableByRunAs(info, ident) {
			return false
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break // reached the filesystem root
		}
		dir = parent
	}
	return true
}

// withinAnyDir reports whether path equals or is contained by any of dirs.
func withinAnyDir(path string, dirs []string) bool {
	clean := filepath.Clean(path)
	for _, d := range dirs {
		if d == "" {
			continue
		}
		if clean == filepath.Clean(d) || common.IsPathWithinDirectory(clean, d) {
			return true
		}
	}
	return false
}

// isWritableByRunAs reports whether the run-as identity could write to (create,
// rename, or delete entries in) the file described by info. It follows the
// group/other write-bit logic of checkWritePermission, including the sticky-bit
// exemption: a world-writable directory with the sticky bit set (e.g. /tmp) does
// not let the identity rename or delete entries it does not own, so it is not a
// repoint risk. Ownership by the identity is itself treated as writable. Group
// membership is read from the precomputed RunAsIdent (no live system lookup). If
// ownership cannot be determined the result is true (writable) so the caller fails
// closed to "not Trusted".
func isWritableByRunAs(info fs.FileInfo, ident risktypes.RunAsIdent) bool {
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return true
	}
	mode := info.Mode()

	// Ownership alone is a repoint risk: the owner can always chmod the entry to
	// grant itself write, so an ancestor owned by the run-as identity is treated
	// as writable regardless of the current write bits.
	if st.Uid == ident.UID {
		return true
	}
	if mode&0o020 != 0 && runAsInGroup(st.Gid, ident) {
		return true
	}
	if mode&0o002 != 0 {
		if mode.IsDir() && mode&fs.ModeSticky != 0 {
			return false
		}
		return true
	}
	return false
}

// runAsInGroup reports whether gid is the run-as primary group or one of its
// supplementary groups.
func runAsInGroup(gid uint32, ident risktypes.RunAsIdent) bool {
	return gid == ident.GID || slices.Contains(ident.Groups, gid)
}
