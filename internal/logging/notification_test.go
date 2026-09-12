package logging

import (
	"log/slog"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fixtureForMessageType returns the representative record for a registered
// type. The fixtures live in slack_handler_test.go; the second return value is
// false when a registered type has no fixture, which fails the range tests
// rather than silently skipping it.
func fixtureForMessageType(messageType string) (slog.Record, bool) {
	for _, fixture := range notificationFixtures() {
		if fixture.token.typeName() == messageType {
			return fixture.record, true
		}
	}
	return slog.Record{}, false
}

// TestNotificationFixturesCoverEveryRegisteredType makes the fixture table the
// prerequisite of the range tests: a type added to the registry without a
// fixture fails here instead of being skipped by every other test.
func TestNotificationFixturesCoverEveryRegisteredType(t *testing.T) {
	require.NotEmpty(t, notificationDefinitions)
	for _, definition := range notificationDefinitions {
		_, ok := fixtureForMessageType(definition.messageType)
		assert.True(t, ok, "no fixture for registered type %q", definition.messageType)
	}
}

// TestNotificationDefinitions_UniqueTypesAndTokens verifies each type is
// registered once, each definition is complete, and the public accessors
// return those registered definitions rather than copies or new ones.
func TestNotificationDefinitions_UniqueTypesAndTokens(t *testing.T) {
	require.NotEmpty(t, notificationDefinitions, "the registry must register at least one type")

	registered := make(map[string]*messageTypeDefinition, len(notificationDefinitions))
	for _, definition := range notificationDefinitions {
		require.NotEmpty(t, definition.messageType)
		require.NotNil(t, definition.build)
		_, duplicate := registered[definition.messageType]
		require.False(t, duplicate, "type %q is registered twice", definition.messageType)
		registered[definition.messageType] = definition
	}

	tokens := []Notification{
		CommandGroupSummaryNotification(),
		PreExecutionErrorNotification(),
		UserGroupCommandFailureNotification(),
	}
	require.Len(t, tokens, len(notificationDefinitions),
		"each registered type must have exactly one public accessor")

	for _, token := range tokens {
		require.NotNil(t, token.definition, "an accessor returned a zero token")
		assert.Same(t, registered[token.definition.messageType], token.definition,
			"accessor for %q must return the registered definition", token.typeName())
	}
}

// TestNotificationDefinitions_TextLineFormat verifies every registered type
// renders the unified Text line:
//
//	[<product name>] <emoji> *<STATUS>* — <Scope> : <headline>
func TestNotificationDefinitions_TextLineFormat(t *testing.T) {
	handler := &SlackHandler{runID: "run-1"}
	const pattern = `^\[go-safe-cmd-runner\] (✅|⚠️|❌) \*(SUCCESS|WARNING|ERROR)\* — .+ : .+$`

	for _, definition := range notificationDefinitions {
		t.Run(definition.messageType, func(t *testing.T) {
			record, ok := fixtureForMessageType(definition.messageType)
			require.True(t, ok)

			message := handler.buildEnvelope(slog.LevelInfo, "(global)", definition.build(record))
			assert.Regexp(t, pattern, message.Text)
			assert.True(t, strings.HasPrefix(message.Text, "["+productName+"] "),
				"the Text line must start with the product name: %q", message.Text)
		})
	}
}

// TestNotificationDefinitions_TrailingFieldsOrder verifies every type ends
// with Scope, Hostname and Run ID in that order, after its own fields.
func TestNotificationDefinitions_TrailingFieldsOrder(t *testing.T) {
	handler := &SlackHandler{runID: "run-1"}
	for _, definition := range notificationDefinitions {
		t.Run(definition.messageType, func(t *testing.T) {
			record, ok := fixtureForMessageType(definition.messageType)
			require.True(t, ok)

			message := handler.buildEnvelope(slog.LevelInfo, "(global)", definition.build(record))
			require.Len(t, message.Attachments, 1)
			assertTrailingEnvelopeFields(t, message.Attachments[0])
		})
	}
}

// TestNotificationDefinitions_BuildersAvoidReservedFieldTitles verifies no
// type-specific builder uses the titles the envelope appends.
func TestNotificationDefinitions_BuildersAvoidReservedFieldTitles(t *testing.T) {
	for _, definition := range notificationDefinitions {
		t.Run(definition.messageType, func(t *testing.T) {
			record, ok := fixtureForMessageType(definition.messageType)
			require.True(t, ok)

			for _, field := range definition.build(record).fields {
				_, reserved := reservedFieldTitles[field.Title]
				assert.False(t, reserved,
					"builder for %q must not use the reserved field title %q", definition.messageType, field.Title)
			}
		})
	}
}

// TestNotificationDefinitions_EnvelopeHasNoHeadingMarkup verifies the static
// parts of the envelope — the Text line skeleton and the field titles — carry
// no "###".
func TestNotificationDefinitions_EnvelopeHasNoHeadingMarkup(t *testing.T) {
	handler := &SlackHandler{runID: "run-1"}
	assert.NotContains(t, productName, "###")

	for _, definition := range notificationDefinitions {
		t.Run(definition.messageType, func(t *testing.T) {
			record, ok := fixtureForMessageType(definition.messageType)
			require.True(t, ok)

			message := handler.buildEnvelope(slog.LevelInfo, "(global)", definition.build(record))
			for _, field := range message.Attachments[0].Fields {
				assert.NotContains(t, field.Title, "###", "field titles are static envelope parts")
			}
			// The dynamic values (scope and headline) sit after " : " and in
			// the field values; the static skeleton must not contain "###".
			static := message.Text
			if marker := strings.Index(static, " : "); marker >= 0 {
				static = static[:marker]
			}
			assert.NotContains(t, static, "###")
		})
	}
}

// TestNotificationDefinitions_FieldsAreDeclaredInInventory verifies every
// builder-produced field title is one the dynamic-value inventory declares.
// A builder that adds a field without an inventory entry fails here.
func TestNotificationDefinitions_FieldsAreDeclaredInInventory(t *testing.T) {
	inventory := map[string]struct{}{
		"Command Count":         {},
		"Duration":              {},
		"Command":               {},
		arrowIndent + " Output": {},
		arrowIndent + " Error":  {},
		"Error Message":         {},
		"Component":             {},
		"Exit Code":             {},
		"Output":                {},
		"Error Output":          {},
	}

	for _, definition := range notificationDefinitions {
		t.Run(definition.messageType, func(t *testing.T) {
			record, ok := fixtureForMessageType(definition.messageType)
			require.True(t, ok)

			fields := definition.build(record).fields
			require.NotEmpty(t, fields, "every registered type must contribute at least one field")
			for _, field := range fields {
				_, known := inventory[field.Title]
				assert.True(t, known,
					"field %q is not declared in the dynamic-value inventory", field.Title)
			}
		})
	}
}
