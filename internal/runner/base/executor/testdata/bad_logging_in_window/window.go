// Package badlogginginwindow is negative test data for the privilege window
// guard: it logs from inside a privilege window. The prohibition on logging is
// expressed by the allowlist not naming any Logger method, i.e. by an absence,
// and an absence cannot be seen to break -- hence this package.
//
// The log record is reached through a same-package hop and then through a
// function value, so the guard's reachability and its indirection table both
// have to work for it to be found.
//
// This directory is under testdata/, so the Go toolchain never builds it as
// part of the executor package; the guard parses and type-checks it on its
// own.
package badlogginginwindow

import "log/slog"

// manager stands in for the privilege manager: what the guard looks for is a
// call named WithPrivileges taking a function literal, not a particular type.
type manager struct{}

func (m *manager) WithPrivileges(fn func() error) error { return fn() }

type runner struct {
	mgr    *manager
	logger *slog.Logger
}

// startWindowHolder is named in the guard's root table as the function that
// opens the start window here.
func (r *runner) startWindowHolder() error {
	return r.mgr.WithPrivileges(func() error {
		return r.report()
	})
}

func (r *runner) report() error {
	// A slog handler is free to open a file, and inside the window that open
	// would happen at euid 0.
	warn := func() {
		r.logger.Warn("staging directory could not be removed")
	}
	warn()
	return nil
}
