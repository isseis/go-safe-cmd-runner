package logging

import (
	"log/slog"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
)

// productName is the sender name every notification Text line starts with. It
// is defined once so no builder can name the product independently.
const productName = "go-safe-cmd-runner"

// Attribute keys of the notification trigger and the type name. They are the
// record contract between firing points and SlackHandler.
const (
	slackNotifyAttrKey = "slack_notify"
	msgTypeAttrKey     = "message_type"
)

// Reasons recorded when a record cannot be rendered as a well-formed
// notification. The order in the reasons list is fixed: unknown message type
// first, then the notification context reason.
const (
	reasonUnknownMessageType           = "unknown_message_type"
	reasonMissingNotificationContext   = "missing_notification_context"
	reasonDuplicateNotificationContext = "duplicate_notification_context"
	reasonInvalidNotificationContext   = "invalid_notification_context"
)

// notificationPriority is the finalized send-queue priority of a notification
// definition. Known types carry it in their definition; it is not derived from
// the log level.
type notificationPriority int

const (
	priorityNormal notificationPriority = iota
	priorityHigh
)

// messageDetails is the type-specific part of a notification: the headline for
// the Text line and the type-specific attachment fields. The common envelope
// (product name, level display, Scope, Hostname, Run ID) is not included; the
// handler adds it for every notification.
type messageDetails struct {
	headline string
	fields   []SlackAttachmentField
}

// messageBuilder builds only the type-specific part. It receives the record
// and nothing else, so a builder has no way to assemble envelope fields.
type messageBuilder func(slog.Record) messageDetails

// messageTypeDefinition is one notification type's definition. The fields are
// set once by registerNotification at package initialization and never
// mutated afterwards.
type messageTypeDefinition struct {
	messageType string
	priority    notificationPriority
	build       messageBuilder
}

// Notification is the opaque token a firing point passes to NotificationAttrs
// to name its notification type. The field is unexported, so outside the
// package only the zero value can be built.
type Notification struct {
	definition *messageTypeDefinition
}

// notificationDefinitions is the single source of notification types: the type
// list, the message builders and the queue priorities all come from here.
var notificationDefinitions []*messageTypeDefinition

// registerNotification appends a definition and returns the token naming it.
// Calls happen only in the var block below, so the set is fixed before any
// other package's init runs.
func registerNotification(messageType string, priority notificationPriority, build messageBuilder) Notification {
	definition := &messageTypeDefinition{
		messageType: messageType,
		priority:    priority,
		build:       build,
	}
	notificationDefinitions = append(notificationDefinitions, definition)
	return Notification{definition: definition}
}

var (
	commandGroupSummaryNotification = registerNotification(
		"command_group_summary", priorityNormal, buildCommandGroupSummary)
	preExecutionErrorNotification = registerNotification(
		"pre_execution_error", priorityHigh, buildPreExecutionError)
	userGroupCommandFailureNotification = registerNotification(
		"user_group_command_failure", priorityNormal, buildUserGroupCommandFailure)
)

// CommandGroupSummaryNotification returns the token for command group summary
// notifications.
func CommandGroupSummaryNotification() Notification {
	return commandGroupSummaryNotification
}

// PreExecutionErrorNotification returns the token for pre-execution error
// notifications.
func PreExecutionErrorNotification() Notification {
	return preExecutionErrorNotification
}

// UserGroupCommandFailureNotification returns the token for user/group command
// failure notifications.
func UserGroupCommandFailureNotification() Notification {
	return userGroupCommandFailureNotification
}

// NotificationAttrs returns the attributes a firing point attaches to the
// record: slack_notify=true, the type name, and the encoded notification
// context. This is the only production path allowed to set slack_notify true.
// The zero Notification yields an empty type name, which the handler treats as
// unknown, so a missing token fails loudly rather than dropping the
// notification silently.
func NotificationAttrs(notification Notification, notificationContext common.NotificationContext) []slog.Attr {
	messageType := ""
	if notification.definition != nil {
		messageType = notification.definition.messageType
	}
	return []slog.Attr{
		slog.Bool(slackNotifyAttrKey, true),
		slog.String(msgTypeAttrKey, messageType),
		notificationContext.LogAttr(),
	}
}

// preExecutionErrorSummaryStatus returns the registered pre-execution error
// type name, which is also the RUN_SUMMARY status word. Reading it from the
// registry keeps the type name defined in one place.
func preExecutionErrorSummaryStatus() string {
	return preExecutionErrorNotification.definition.messageType
}

// lookupNotification returns the definition registered for messageType.
func lookupNotification(messageType string) (*messageTypeDefinition, bool) {
	for _, definition := range notificationDefinitions {
		if definition.messageType == messageType {
			return definition, true
		}
	}
	return nil, false
}

// unknownTypePriority maps a log level to the queue priority of a record whose
// type is unknown: WARN and above are high, INFO and everything below it are
// normal. The threshold form keeps DEBUG out of the reserved lane.
func unknownTypePriority(level slog.Level) notificationPriority {
	if level >= slog.LevelWarn {
		return priorityHigh
	}
	return priorityNormal
}

// priorityForNotification returns the queue priority of a record: the
// registered priority for a known type, and the level-derived priority for an
// unknown one.
func priorityForNotification(definition *messageTypeDefinition, level slog.Level) notificationPriority {
	if definition == nil {
		return unknownTypePriority(level)
	}
	return definition.priority
}

// reservedFieldTitles are the attachment field titles the common envelope
// appends last. A type-specific builder must not use them, or a reader cannot
// tell a type-specific field from the envelope's Scope, Hostname or Run ID.
var reservedFieldTitles = map[string]struct{}{
	fieldTitleScope:    {},
	fieldTitleHostname: {},
	fieldTitleRunID:    {},
}
