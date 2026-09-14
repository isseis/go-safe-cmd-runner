package common

import (
	"log/slog"

	"github.com/isseis/go-safe-cmd-runner/internal/identifier"
)

// NotificationScope identifies the origin level a notification context
// declares. ScopeGlobal is the zero value so an omitted context still encodes
// as a valid global scope; the record still has to carry the attribute
// explicitly.
type NotificationScope int

const (
	// ScopeGlobal means the notification has no group or command origin.
	ScopeGlobal NotificationScope = iota
	// ScopeGroup means the notification originated in a group.
	ScopeGroup
	// ScopeCommand means the notification originated in a command.
	ScopeCommand
)

// MaxIdentifierBytes bounds the UTF-8 byte length of a single identifier (a
// group or command name) that may be embedded in a notification. Configuration
// validation enforces it; the interpolation contract does not truncate
// identifiers.
const MaxIdentifierBytes = 128

// NotificationContext carries where a notification originated. Its fields are
// unexported, so values can only be built through GlobalScope, GroupScope and
// CommandScope. The zero value is a valid global scope and encodes exactly like
// GlobalScope().
type NotificationContext struct {
	scope   NotificationScope
	group   string
	command string
}

// Compile-time guard to ensure NotificationContext implements slog.LogValuer.
var _ slog.LogValuer = GlobalScope()

// GlobalScope returns the notification context of an error that is not tied to
// a group or command.
func GlobalScope() NotificationContext {
	return NotificationContext{}
}

// GroupScope returns the notification context of an error that occurred in the
// named group. The name is not validated here: construction must not interrupt
// the log path. The display boundary reports an empty or unprintable name as an
// invalid scope.
func GroupScope(group string) NotificationContext {
	return NotificationContext{scope: ScopeGroup, group: group}
}

// CommandScope returns the notification context of an error that occurred in
// the named command inside the named group. The names are not validated here,
// for the same reason as GroupScope.
func CommandScope(group, command string) NotificationContext {
	return NotificationContext{scope: ScopeCommand, group: group, command: command}
}

// Scope returns the declared origin level.
func (c NotificationContext) Scope() NotificationScope {
	return c.scope
}

// GroupName returns the group name, or an empty string for the global scope.
func (c NotificationContext) GroupName() string {
	return c.group
}

// CommandName returns the command name, or an empty string when the context has
// none.
func (c NotificationContext) CommandName() string {
	return c.command
}

// LogValue encodes the context in the fixed record form: scope and group are
// always present, and command is present only when it is not empty.
func (c NotificationContext) LogValue() slog.Value {
	attrs := []slog.Attr{
		slog.String(NotificationContextAttrs.Scope, c.scopeName()),
		slog.Any(NotificationContextAttrs.Group, identifier.NewIdentifier(c.group)),
	}
	if c.command != "" {
		attrs = append(attrs, slog.Any(NotificationContextAttrs.Command, identifier.NewIdentifier(c.command)))
	}
	return slog.GroupValue(attrs...)
}

// LogAttr returns the record attribute that carries the context. The value is
// left as a slog.LogValuer so a logging handler resolves it through LogValue;
// by the time the encoded form reaches the Slack handler it is a group.
func (c NotificationContext) LogAttr() slog.Attr {
	return slog.Any(NotificationContextAttrs.Key, c)
}

// DecodeNotificationContext reconstructs a NotificationContext from the fixed
// group encoding produced by LogValue and validates every part of it. It
// returns ErrInvalidNotificationContext unless the input is exactly a group
// with a string-valued scope sub-key, a group sub-key that is either a string
// or an identifier.Identifier, an optional command sub-key of either form, a
// known scope name, and names consistent with that scope. A redacting handler
// normalizes a declared identifier to a string, so both forms reach the
// decoder.
func DecodeNotificationContext(value slog.Value) (NotificationContext, error) {
	if value.Kind() != slog.KindGroup {
		return NotificationContext{}, ErrInvalidNotificationContext
	}

	scopeName, group, command, ok := decodeNotificationContextParts(value.Group())
	if !ok {
		return NotificationContext{}, ErrInvalidNotificationContext
	}

	var scope NotificationScope
	switch scopeName {
	case NotificationScopeNames.Global:
		scope = ScopeGlobal
	case NotificationScopeNames.Group:
		scope = ScopeGroup
	case NotificationScopeNames.Command:
		scope = ScopeCommand
	default:
		return NotificationContext{}, ErrInvalidNotificationContext
	}

	// Keep every decoded name while validating so a scope that carries more
	// information than it declares is rejected instead of silently dropped.
	ctx := NotificationContext{scope: scope, group: group, command: command}
	if !ctx.hasConsistentNames() || !ctx.hasDisplayableNames() {
		return NotificationContext{}, ErrInvalidNotificationContext
	}
	return ctx, nil
}

// decodeNotificationContextParts enforces the fixed key set, the accepted
// value forms and the key cardinality of the encoding and returns the scope
// name, the group name and the command name. The scope value must be a string;
// the group and command values must be either a string or an
// identifier.Identifier, the two forms LogValue and a redacting handler
// produce. Every other value, including a LogValuer that resolves to a string,
// is rejected. The command name is empty when the command key is absent.
func decodeNotificationContextParts(attrs []slog.Attr) (scopeName, group, command string, ok bool) {
	var (
		scopeValue   slog.Value
		groupValue   slog.Value
		commandValue slog.Value
		scopeCount   int
		groupCount   int
		commandCount int
	)
	for _, attr := range attrs {
		switch attr.Key {
		case NotificationContextAttrs.Scope:
			scopeCount++
			scopeValue = attr.Value
		case NotificationContextAttrs.Group:
			groupCount++
			groupValue = attr.Value
		case NotificationContextAttrs.Command:
			commandCount++
			commandValue = attr.Value
		default:
			// The encoding fixes the key set, so an unknown key is not
			// produced by LogValue and is rejected rather than ignored.
			return "", "", "", false
		}
	}

	// The scope value must be a string; the zero Value of a missing key is not
	// a string, so this check is guarded by presence. The group and command
	// values are checked by decodeNotificationContextName while extracting
	// them; the cardinality checks below are what reject a missing key.
	if scopeCount > 0 && scopeValue.Kind() != slog.KindString {
		return "", "", "", false
	}
	// scope and group appear exactly once in the encoding; command is optional
	// and appears at most once.
	if scopeCount != 1 || groupCount != 1 || commandCount > 1 {
		return "", "", "", false
	}

	if groupCount > 0 {
		name, named := decodeNotificationContextName(groupValue)
		if !named {
			return "", "", "", false
		}
		group = name
	}
	if commandCount > 0 {
		name, named := decodeNotificationContextName(commandValue)
		if !named {
			return "", "", "", false
		}
		command = name
	}
	return scopeValue.String(), group, command, true
}

// decodeNotificationContextName reads a group or command name from the two
// forms the encoding produces: the string a redacting handler writes after
// normalizing a declared identifier, and the raw identifier.Identifier. It
// does not accept a *identifier.Identifier: the redaction layer normalizes a
// non-nil pointer to a string before decoding and refuses a typed nil one. It
// deliberately does not resolve other LogValuer values either; accepting every
// value that resolves to a string would widen the encoding contract beyond
// those two forms.
func decodeNotificationContextName(value slog.Value) (string, bool) {
	if value.Kind() == slog.KindString {
		return value.String(), true
	}
	if id, ok := value.Any().(identifier.Identifier); ok {
		return id.Name(), true
	}
	return "", false
}

// hasConsistentNames reports whether the group and command names match the
// scope the context declares: global carries neither name, group carries only
// the group name, and command carries both.
func (c NotificationContext) hasConsistentNames() bool {
	switch c.scope {
	case ScopeGlobal:
		return c.group == "" && c.command == ""
	case ScopeGroup:
		return c.group != "" && c.command == ""
	case ScopeCommand:
		return c.group != "" && c.command != ""
	default:
		return false
	}
}

// hasDisplayableNames reports whether every name the context carries survives
// the interpolation contract with a visible character left.
func (c NotificationContext) hasDisplayableNames() bool {
	if c.group != "" && !HasDisplayableContent(c.group) {
		return false
	}
	if c.command != "" && !HasDisplayableContent(c.command) {
		return false
	}
	return true
}

// scopeName returns the encoded name of the scope. An unknown scope encodes as
// the empty string, which the decoder then rejects, rather than as a valid
// global scope.
func (c NotificationContext) scopeName() string {
	switch c.scope {
	case ScopeGlobal:
		return NotificationScopeNames.Global
	case ScopeGroup:
		return NotificationScopeNames.Group
	case ScopeCommand:
		return NotificationScopeNames.Command
	default:
		return ""
	}
}
