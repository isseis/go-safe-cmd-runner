//go:build test || performance || integration

package commontestutil

import (
	"github.com/isseis/go-safe-cmd-runner/internal/common"
)

// NewUnsetOutputSizeLimit creates an unset OutputSizeLimit (will use default or inherit from parent).
func NewUnsetOutputSizeLimit() common.OutputSizeLimit {
	return common.OutputSizeLimit{OptionalValue: common.NewUnsetOptionalValue[int64]()}
}
