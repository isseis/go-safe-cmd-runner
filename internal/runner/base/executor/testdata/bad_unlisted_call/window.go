// Package badunlistedcall is negative test data for the privilege window
// guard: it opens a privilege window whose body reaches a file operation that
// the guard's allowlist does not name. The guard must reject it.
//
// This directory is under testdata/, so the Go toolchain never builds it as
// part of the executor package; the guard parses and type-checks it on its
// own.
package badunlistedcall

import "os"

// manager stands in for the privilege manager: what the guard looks for is a
// call named WithPrivileges taking a function literal, not a particular type.
type manager struct{}

func (m *manager) WithPrivileges(fn func() error) error { return fn() }

type runner struct {
	mgr *manager
}

// startWindowHolder is named in the guard's root table as the function that
// opens the start window here.
func (r *runner) startWindowHolder(dir string) error {
	return r.mgr.WithPrivileges(func() error {
		// os.Remove is not os.RemoveAll: it is tracked (package os) and absent
		// from the start window's allowlist, so the guard must report it.
		return os.Remove(dir)
	})
}
