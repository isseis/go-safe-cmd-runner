package cli

import (
	"github.com/isseis/go-safe-cmd-runner/internal/runner/resource"
)

// ParseDryRunDetailLevel converts string to DetailLevel enum
func ParseDryRunDetailLevel(level string) (resource.DryRunDetailLevel, error) {
	switch level {
	case "summary":
		return resource.DetailLevelSummary, nil
	case "detailed":
		return resource.DetailLevelDetailed, nil
	case "full":
		return resource.DetailLevelFull, nil
	default:
		return resource.DetailLevelSummary, ErrInvalidDetailLevel
	}
}

// ParseDryRunOutputFormat converts string to OutputFormat enum
func ParseDryRunOutputFormat(format string) (resource.OutputFormat, error) {
	switch format {
	case "text":
		return resource.OutputFormatText, nil
	case "json":
		return resource.OutputFormatJSON, nil
	default:
		return resource.OutputFormatText, ErrInvalidOutputFormat
	}
}
