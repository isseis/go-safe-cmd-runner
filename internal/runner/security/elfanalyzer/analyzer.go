package elfanalyzer

import "github.com/isseis/go-safe-cmd-runner/internal/security/binaryanalyzer"

// Compile-time check: StandardELFAnalyzer implements binaryanalyzer.BinaryAnalyzer.
var _ binaryanalyzer.BinaryAnalyzer = (*StandardELFAnalyzer)(nil)
