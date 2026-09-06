// Package badpositionalfield is negative test data for the privilege window
// guard: the function value a window runs is held in a struct field, and the
// field is bound through a positional composite literal, which names no field
// for the guard to check its value against. The guard must report the binding
// rather than walk the literal its indirection table names while the window
// runs this one.
//
// The keyed nil binding in clear exists so the package also holds a binding
// the guard accepts: without it the report would come from the stale-entry
// fallback, not from the positional-literal branch under test.
//
// This directory is under testdata/, so the Go toolchain never builds it as
// part of the executor package; the guard parses and type-checks it on its
// own.
package badpositionalfield

import "os"

// manager stands in for the privilege manager: what the guard looks for is a
// call named WithPrivileges taking a function literal, not a particular type.
type manager struct{}

func (m *manager) WithPrivileges(fn func() error) error { return fn() }

type prepared struct {
	cleanup func() error
}

func (p *prepared) runCleanup() error {
	if p.cleanup == nil {
		return nil
	}
	return p.cleanup()
}

type runner struct {
	mgr *manager
	pc  *prepared
}

// startWindowHolder is named in the guard's root table as the function that
// opens the start window here.
func (r *runner) startWindowHolder() error {
	return r.mgr.WithPrivileges(func() error {
		return r.pc.runCleanup()
	})
}

// stage declares the literal the guard's indirection table names as the one
// the field carries.
func (r *runner) stage() func() error {
	cleanup := func() error { return nil }
	return cleanup
}

// clear binds the field with nil through a keyed literal, a binding the guard
// accepts, so the stale-entry fallback does not fire instead of the branch
// under test.
func (r *runner) clear() {
	r.pc = &prepared{cleanup: nil}
}

// rebind binds the field through a positional composite literal, which the
// guard cannot trace and must report.
func (r *runner) rebind() {
	r.pc = &prepared{func() error { return os.Remove("/tmp/decoy") }}
}
