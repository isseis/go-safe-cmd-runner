// Package badreboundfield is negative test data for the privilege window
// guard: the function value a window runs is held in a struct field, and the
// field is bound to something other than the literal the guard's indirection
// table names for it. The guard must report that, because the table is the one
// place the analysis takes a claim on faith -- if it did not check the claim,
// it would keep walking the literal the table names while the window ran this
// one.
//
// This directory is under testdata/, so the Go toolchain never builds it as
// part of the executor package; the guard parses and type-checks it on its
// own.
package badreboundfield

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

// rebind binds the field through a composite literal instead, which is the
// form that used to slip past the check.
func (r *runner) rebind() {
	r.pc = &prepared{cleanup: func() error { return os.Remove("/tmp/decoy") }}
}
