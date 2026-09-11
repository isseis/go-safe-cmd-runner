package bootstrap

import (
	"fmt"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
	"github.com/isseis/go-safe-cmd-runner/internal/logging"
	"github.com/isseis/go-safe-cmd-runner/internal/redaction"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/runnertypes"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/config"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/resource"
)

// ValidateIdentifierRedaction rejects a configuration whose group or command
// names the production redaction transformation would rewrite. A rewritten name
// reaches a notification as the redaction placeholder, so the scope no longer
// points at the configured group or command.
//
// redactionConfig must be the Config AddSlackHandlers built, which
// SetupSlackLogging returns. A nil value means Slack notifications are disabled
// and the handler chain runs with redaction.DefaultConfig(), so that default is
// used here too; nil does not mean "skip the check". The Config is deliberately
// not rebuilt: a second NewConfig call would agree today and could silently
// drift from the production one when either call site gains an option.
//
// The returned error is a PreExecutionError whose text names the position and
// the check but never the identifier value. The values this check rejects are
// exactly the ones redaction would rewrite, and HandlePreExecutionError writes
// its detail to stderr without redaction.
func ValidateIdentifierRedaction(cfg *runnertypes.ConfigSpec, redactionConfig *redaction.Config) error {
	if redactionConfig == nil {
		// Same meaning as NewRedactingHandler's nil handling: a disabled Slack
		// setup leaves no Config behind, and the handler chain keeps running
		// with the default transformation.
		redactionConfig = redaction.DefaultConfig()
	}

	for i := range cfg.Groups {
		group := &cfg.Groups[i]
		if redactionConfig.RewritesValue(group.Name) {
			return identifierRedactedError("group", fmt.Sprintf("groups[%d]", i))
		}
		for j := range group.Commands {
			if redactionConfig.RewritesValue(group.Commands[j].Name) {
				return identifierRedactedError("command", fmt.Sprintf("groups[%d].commands[%d]", i, j))
			}
		}
	}

	return nil
}

// identifierRedactedError builds the PreExecutionError for a rejected
// identifier. The message carries the kind, the position and the check name;
// the value itself is never included (see ValidateIdentifierRedaction). RunID
// is left empty because the report boundary in cmd/runner owns the process run
// ID and fills it in before HandlePreExecutionError.
func identifierRedactedError(kind, position string) error {
	return &logging.PreExecutionError{
		Type: logging.ErrorTypeConfigParsing,
		Message: fmt.Sprintf(
			"Identifier redaction validation failed for %s at %s: the name is rewritten by redaction and cannot identify the notification scope",
			kind, position),
		Component:           string(resource.ComponentConfig),
		NotificationContext: common.GlobalScope(),
		Err:                 config.ErrIdentifierRedacted,
	}
}
