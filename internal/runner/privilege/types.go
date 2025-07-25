package privilege

import (
	"time"
)

// Operation represents different types of privileged operations
type Operation string

// Supported privileged operations
const (
	OperationFileHashCalculation Operation = "file_hash_calculation"
	OperationCommandExecution    Operation = "command_execution"
	OperationFileAccess          Operation = "file_access"
	OperationHealthCheck         Operation = "health_check"
)

// ElevationContext contains context information for privilege elevation
type ElevationContext struct {
	Operation   Operation
	CommandName string
	FilePath    string
	StartTime   time.Time
	OriginalUID int
	TargetUID   int
}

// Metrics for privilege operations
type Metrics struct {
	ElevationAttempts  int64
	ElevationSuccesses int64
	ElevationFailures  int64
	TotalElevationTime time.Duration
	LastElevationTime  time.Time
	LastError          string
}
