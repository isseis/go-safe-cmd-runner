//go:build test

package risktypes

import (
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/testutil/identitymutationguard"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// This guard protects the risktypes package, a separate boundary from the
// existing identity-mutation guard in internal/runner/base/privilege
// (identity_mutation_guard_test.go). That guard allows two specific call
// sites (root escalation and restoration); this one allows none.
// OriginalExecutionIdentity and ResolveRunAsIdent/ResolveRunAsIdentStrict
// (runas_ident.go) only ever read identity via os.Getuid/os.Getgid/
// os.Getgroups and the os/user database lookups, and must never change
// process identity. A single hit here is a failure with no exceptions.
//
// The AST-scanning primitives are shared with the sibling guard in
// internal/runner/resource via identitymutationguard, since both packages
// apply the same empty-allow-list policy; only the privilege package
// (which does escalate/restore, and so needs an allow-list) keeps its own
// copy.

// TestRisktypesPackageDoesNotMutateIdentity statically verifies that the
// risktypes package never calls, or references as a value, a syscall/unix
// identity-mutation function. The allow-list here is empty: this package
// resolves and reports identity, it never changes it.
func TestRisktypesPackageDoesNotMutateIdentity(t *testing.T) {
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

	// Control 5: os.Getuid/os.Getgid/os.Getgroups (used by
	// OriginalExecutionIdentity) are read-only and are not in the tracked
	// function set, so they never trip this guard.
	readOnlySrc := "package p\n" +
		"import \"os\"\n" +
		"func f() (int, int) { return os.Getuid(), os.Getgid() }\n"
	readOnlySites := identitymutationguard.CallSitesInSource(t, "readonly.go", readOnlySrc)
	assert.Empty(t, readOnlySites, "os.Getuid/os.Getgid must not be tracked as identity-mutation calls")

	sites, valueRefs := identitymutationguard.FindRefs(t, ".")

	for _, ref := range valueRefs {
		t.Errorf("identity-mutation function %s referenced as a value in function %s; the risktypes package must never reference it, called or not",
			ref.Expr, ref.FuncName)
	}
	for _, site := range sites {
		t.Errorf("unexpected identity-mutation call %s in function %s: the risktypes package must never change process identity",
			site.CallExpr, site.FuncName)
	}
}
