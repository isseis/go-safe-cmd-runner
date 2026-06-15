//go:build test

package risk

import (
	"errors"
	"path/filepath"
	"runtime"

	"github.com/isseis/go-safe-cmd-runner/internal/fileanalysis"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/runnertypes"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/security"
)

// errUnexpectedIO is an unclassifiable record-load failure used to exercise the
// error-return (not deny) path.
var errUnexpectedIO = errors.New("unexpected record-load I/O error")

// fileErrNotFound returns the sentinel signalling a missing analysis record.
func fileErrNotFound() error { return fileanalysis.ErrRecordNotFound }

// testContentHash is the verified content hash attached to test commands so they
// pass the evaluator's identity gate.
const testContentHash = "sha256:testhash"

// fakeRecordStore is a configurable RecordStore for risk evaluation tests. By
// default LoadRecord returns a clean record (no signals) whose hash matches
// testContentHash, so binary analysis classifies as Clean. Per-path records or
// errors can be injected to exercise the network/high-risk/uncertain branches.
type fakeRecordStore struct {
	records map[string]*fileanalysis.Record
	errs    map[string]error
}

func (s fakeRecordStore) LoadRecord(path string) (*fileanalysis.Record, error) {
	if err, ok := s.errs[path]; ok {
		return nil, err
	}
	if r, ok := s.records[path]; ok {
		return r, nil
	}
	return &fileanalysis.Record{ContentHash: testContentHash}, nil
}

// newVerifiedEvaluator returns an evaluator backed by a clean record store, so
// the identity gate is satisfied (analysis enabled) and unconfigured paths
// classify as Clean.
func newVerifiedEvaluator() Evaluator {
	return newEvaluatorWithStore(fakeRecordStore{})
}

// newEvaluatorWithStore returns an evaluator using the given record store.
func newEvaluatorWithStore(store security.RecordStore) Evaluator {
	deps := security.AnalysisDeps{RecordStore: store}
	return NewStandardEvaluator(security.NewNetworkAnalyzer(runtime.GOOS, deps))
}

// newAnalysisDisabledEvaluator returns an evaluator with binary analysis disabled
// (no record store), so the identity gate denies every command.
func newAnalysisDisabledEvaluator() Evaluator {
	return NewStandardEvaluator(security.NewNetworkAnalyzer(runtime.GOOS, security.AnalysisDeps{}))
}

// testBinDir is the synthetic absolute directory used to turn a bare command
// name into an absolute path, since the evaluator requires absolute paths
// (production always resolves them). It does not need to exist on disk.
const testBinDir = "/runner-test-bin"

// absCmd makes a bare command name absolute so it satisfies the evaluator's
// absolute-path requirement; an already-absolute path is returned unchanged.
func absCmd(cmd string) string {
	if cmd == "" || filepath.IsAbs(cmd) {
		return cmd
	}
	return filepath.Join(testBinDir, cmd)
}

// verifiedCmd builds a RuntimeCommand carrying a verified content hash so it
// passes the identity gate. A bare command name is made absolute.
func verifiedCmd(cmd string, args []string) *runnertypes.RuntimeCommand {
	return &runnertypes.RuntimeCommand{
		ExpandedCmd:            absCmd(cmd),
		ExpandedArgs:           args,
		ExpandedCmdContentHash: testContentHash,
	}
}

// dlopenRecord builds a record whose symbol analysis carries a dynamic-load
// (dlopen) signal, classified as high risk.
func dlopenRecord() *fileanalysis.Record {
	return &fileanalysis.Record{
		ContentHash: testContentHash,
		SymbolAnalysis: &fileanalysis.SymbolAnalysisData{
			DynamicLoadSymbols: []fileanalysis.DetectedSymbol{{Name: "dlopen"}},
		},
	}
}
