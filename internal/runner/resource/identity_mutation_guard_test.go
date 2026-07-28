//go:build test

package resource

import (
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/testutil/identitymutationguard"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// This guard protects the resource package, a separate boundary from the
// existing identity-mutation guard in internal/runner/base/privilege
// (identity_mutation_guard_test.go). That guard allows two specific call
// sites (root escalation and restoration); this one allows none, because
// validateRunAsIdentity (dryrun_manager.go) only ever reads identity
// (risktypes.ResolveRunAsIdentStrict / OriginalExecutionIdentity) and must
// never change it. A single hit here is a failure with no exceptions.
//
// The AST-scanning primitives are shared with the sibling guard in
// internal/runner/base/risktypes via identitymutationguard, since both
// packages apply the same empty-allow-list policy; only the privilege
// package (which does escalate/restore, and so needs an allow-list) keeps
// its own copy.

// TestResourcePackageDoesNotMutateIdentity statically verifies that the
// resource package never calls, or references as a value, a syscall/unix
// identity-mutation function. Unlike the privilege package's guard, the
// allow-list here is empty: the resource package's job is to preview and
// validate, not to change what the process is running as.
func TestResourcePackageDoesNotMutateIdentity(t *testing.T) {
	// Control 1: an aliased import is still resolved to "syscall".
	aliasSrc := "package p\n" +
		"import sc \"syscall\"\n" +
		"func f() error { return sc.Seteuid(0) }\n"
	aliasSites := identitymutationguard.CallSitesInSource(t, "alias.go", aliasSrc)
	require.Len(t, aliasSites, 1, "an aliased syscall import must still be recognized")
	assert.Equal(t, "Seteuid", aliasSites[0].SyscallName)

	// Control 2: golang.org/x/sys/unix is matched by suffix.
	unixSrc := "package p\n" +
		"import \"golang.org/x/sys/unix\"\n" +
		"func f() error { return unix.Setgroups(nil) }\n"
	unixSites := identitymutationguard.CallSitesInSource(t, "unix.go", unixSrc)
	require.Len(t, unixSites, 1)
	assert.Equal(t, "Setgroups", unixSites[0].SyscallName)

	// Control 3: a raw syscall entry point is tracked in its own right.
	rawSrc := "package p\n" +
		"import \"syscall\"\n" +
		"func f() { syscall.RawSyscall(syscall.SYS_SETRESUID, 0, 0, 0) }\n"
	rawSites := identitymutationguard.CallSitesInSource(t, "raw.go", rawSrc)
	require.Len(t, rawSites, 1, "a raw syscall entry point must be detected")

	// Control 4: a function-value reference (not a direct call) is flagged
	// too, since it could be invoked later through the variable/field.
	valueRefSrc := "package p\n" +
		"import \"syscall\"\n" +
		"func f() func(int) error { return syscall.Seteuid }\n"
	valueRefCalls, valueRefSites := identitymutationguard.RefsInSource(t, "value_ref.go", valueRefSrc)
	assert.Empty(t, valueRefCalls, "a bare function-value reference must not be counted as a call")
	require.Len(t, valueRefSites, 1, "a bare function-value reference must be detected")

	// Control 5: a local identifier that merely shares the package's name
	// (not a real import) is not resolved to the tracked package.
	shadowSrc := "package p\n" +
		"type syscall struct{}\n" +
		"func (syscall) Seteuid(int) error { return nil }\n" +
		"func f() error { var syscall syscall; return syscall.Seteuid(1) }\n"
	shadowSites := identitymutationguard.CallSitesInSource(t, "shadow.go", shadowSrc)
	assert.Empty(t, shadowSites, "a local identifier that merely shares the package's name must not be treated as the syscall package")

	sites, valueRefs := identitymutationguard.FindRefs(t, ".")

	for _, ref := range valueRefs {
		t.Errorf("identity-mutation function %s referenced as a value in function %s; the resource package must never reference it, called or not",
			ref.Expr, ref.FuncName)
	}
	for _, site := range sites {
		t.Errorf("unexpected identity-mutation call %s in function %s: the resource package must never change process identity",
			site.CallExpr, site.FuncName)
	}
}
