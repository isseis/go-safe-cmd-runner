// Package bootstrap provides application initialization and setup functionality.
package bootstrap

import (
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
	"github.com/isseis/go-safe-cmd-runner/internal/logging"
	"github.com/isseis/go-safe-cmd-runner/internal/redaction"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/runnertypes"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/config"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/resource"
	"github.com/isseis/go-safe-cmd-runner/internal/verification"
)

// ErrInvalidSlackAllowedHost is a sentinel error returned when the slack_allowed_host value is invalid.
var ErrInvalidSlackAllowedHost = errors.New("slack_allowed_host must be a valid hostname or IP address without port or whitespace")

// normalizeSlackAllowedHost lower-cases host and strips the brackets from an
// IPv6 literal ("[2001:DB8::1]" becomes "2001:db8::1"), returning ("", nil) for
// an empty host (Slack disabled).
//
// What counts as a valid host is redaction.ValidateWebhookHost's decision, which
// compileWebhookHostPattern also uses, so the accepted value and the pattern
// built from it cannot drift apart. Label length limits are not enforced.
func normalizeSlackAllowedHost(host string) (string, error) {
	if host == "" {
		return "", nil
	}

	var bareHost string
	// IPv6 literal: "[<addr>]" — delegate bracket-stripping to url.Parse.
	if strings.HasPrefix(host, "[") {
		u, err := url.Parse("https://" + host + "/")
		// url.Parse splits "[::1]/path?q#f" into a host plus a path, query and
		// fragment, so checking Hostname alone would silently drop them and
		// accept the value. Path is "/" because of the slash appended above.
		if err != nil || u.Hostname() == "" || u.Port() != "" ||
			u.Path != "/" || u.RawQuery != "" || u.Fragment != "" || u.User != nil || u.Opaque != "" {
			return "", fmt.Errorf("%w (got %q)", ErrInvalidSlackAllowedHost, host)
		}
		bareHost = strings.ToLower(u.Hostname()) // bare address e.g. "::1"
	} else {
		bareHost = strings.ToLower(host)
	}

	if err := redaction.ValidateWebhookHost(bareHost); err != nil {
		return "", fmt.Errorf("%w (got %q)", ErrInvalidSlackAllowedHost, host)
	}
	return bareHost, nil
}

// LoadAndPrepareConfig verifies the configuration file's hash and loads it,
// returning a ConfigSpec ready for execution. Variable expansion (EnvVars, Cmd,
// Args) happens inside config.Loader.LoadConfig, not here.
func LoadAndPrepareConfig(verificationManager *verification.Manager, configPath, runID string) (*runnertypes.ConfigSpec, error) {
	if configPath == "" {
		return nil, &logging.PreExecutionError{
			Type:      logging.ErrorTypeRequiredArgumentMissing,
			Message:   "Config file path is required",
			Component: string(resource.ComponentConfig),
			RunID:     runID,
		}
	}

	// Read and verify in one step: a separate read would reopen the file and
	// reintroduce the TOCTOU window.
	content, err := verificationManager.VerifyAndReadConfigFile(configPath)
	if err != nil {
		return nil, &logging.PreExecutionError{
			Type:      logging.ErrorTypeFileAccess,
			Message:   "Failed to verify and read the configuration file",
			Component: string(resource.ComponentVerification),
			RunID:     runID,
			// Carried as an error, not flattened into Message, so callers can tell
			// the cause apart with errors.Is/As.
			Err: err,
		}
	}

	// The verification manager is passed on so included files are hash-verified too.
	cfgLoader := config.NewLoader(
		common.NewDefaultFileSystem(),
		verificationManager,
	)

	cfg, err := cfgLoader.LoadConfig(configPath, content)
	if err != nil {
		return nil, &logging.PreExecutionError{
			Type:      logging.ErrorTypeConfigParsing,
			Message:   "Failed to load the configuration",
			Component: string(resource.ComponentConfig),
			RunID:     runID,
			Err:       err,
		}
	}

	normalizedHost, err := normalizeSlackAllowedHost(cfg.Global.SlackAllowedHost)
	if err != nil {
		return nil, &logging.PreExecutionError{
			Type:      logging.ErrorTypeConfigParsing,
			Message:   "Invalid slack_allowed_host",
			Component: string(resource.ComponentConfig),
			RunID:     runID,
			Err:       err,
		}
	}
	cfg.Global.SlackAllowedHost = normalizedHost

	return cfg, nil
}
