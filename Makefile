CHOWN=chown
CHMOD=chmod
MKDIR=mkdir -p
RM=rm
ENVCMD=env
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOLINT=golangci-lint run
SUDOCMD=sudo

# Configuration paths
DEFAULT_HASH_DIRECTORY=/usr/local/etc/go-safe-cmd-runner/hashes

BINARY_NAME=go-safe-cmd-runner
BINARY_RECORD=build/record
BINARY_VERIFY=build/verify
BINARY_RUNNER=build/runner

# Build flags to embed configuration values
BUILD_FLAGS=-ldflags "-X main.DefaultHashDirectory=$(DEFAULT_HASH_DIRECTORY)"

# Find all Go source files to use as dependencies for the build
GO_SOURCES := $(shell find . -type f -name '*.go' -not -name '*_test.go')

HASH_TARGETS := \
	/etc/passwd \
	./sample/.env \
	./sample/comprehensive.toml \
	./sample/slack-notify.toml \
	./sample/slack-group-notification-test.toml

.PHONY: all lint build run clean test hash integration-test integration-test-success slack-notify-test slack-group-notification-test

all: build

lint:
	$(GOLINT)

# The phony 'build' target now depends on the actual binary file.
build: $(BINARY_RECORD) $(BINARY_VERIFY) $(BINARY_RUNNER)

# This rule tells make how to build the binary from the source files.
# It will only run if the binary doesn't exist or if a .go file has changed.
$(BINARY_RECORD): $(GO_SOURCES)
	@$(MKDIR) $(@D)
	$(GOBUILD) $(BUILD_FLAGS) -o build/record -v cmd/record/main.go

$(BINARY_VERIFY): $(GO_SOURCES)
	@$(MKDIR) $(@D)
	$(GOBUILD) $(BUILD_FLAGS) -o build/verify -v cmd/verify/main.go

$(BINARY_RUNNER): $(GO_SOURCES)
	@$(MKDIR) $(@D)
	$(GOBUILD) $(BUILD_FLAGS) -o $@ -v cmd/runner/main.go
	$(SUDOCMD) $(CHOWN) root:root $@
	$(SUDOCMD) $(CHMOD) u+s $@

clean:
	$(GOCLEAN)
	rm -f $(BINARY_RECORD) $(BINARY_VERIFY) $(BINARY_RUNNER)

hash:
	$(foreach file, $(HASH_TARGETS), \
		$(SUDOCMD) $(BINARY_RECORD) -force -file $(file) -hash-dir $(DEFAULT_HASH_DIRECTORY);)

test: $(BINARY_RUNNER)
	$(GOTEST) -v ./...
	$(ENVCMD) -i $(BINARY_RUNNER) -dry-run -config ./sample/comprehensive.toml

integration-test: $(BINARY_RUNNER)
	$(MKDIR) /tmp/cmd-runner-comprehensive /tmp/custom-workdir-test
	@EXIT_CODE=0; \
	$(ENVCMD) -i PATH=/bin:/sbin:/usr/bin:/usr/sbin LANG=C $(BINARY_RUNNER) -config ./sample/comprehensive.toml -log-level warn -env-file $(PWD)/sample/.env || EXIT_CODE=$$?; \
	$(RM) -r /tmp/cmd-runner-comprehensive /tmp/custom-workdir-test; \
	echo "Integration test completed with exit code: $$EXIT_CODE"; \
	exit $$EXIT_CODE

slack-notify-test: $(BINARY_RUNNER)
	$(MKDIR) /tmp/cmd-runner-slack-test
	@EXIT_CODE=0; \
	$(ENVCMD) -i PATH=/bin:/sbin:/usr/bin:/usr/sbin LANG=C $(BINARY_RUNNER) -config ./sample/slack-notify.toml -log-level warn -env-file $(PWD)/sample/.env || EXIT_CODE=$$?; \
	$(RM) -r /tmp/cmd-runner-slack-test; \
	echo "Slack notification test completed with exit code: $$EXIT_CODE"; \
	exit $$EXIT_CODE

# Test the new group-level Slack notification functionality
# This target tests notifications sent after each command group execution
slack-group-notification-test: $(BINARY_RUNNER)
	@$(MKDIR) /tmp/slack-group-test
	@EXIT_CODE=0; \
	RUN_ID="slack-test-$$(date +%s)"; \
	echo "Running Slack group notification test with run ID: $$RUN_ID"; \
	$(ENVCMD) -i PATH=/bin:/sbin:/usr/bin:/usr/sbin LANG=C SLACK_WEBHOOK_URL="$$SLACK_WEBHOOK_URL" \
		$(BINARY_RUNNER) -config ./sample/slack-group-notification-test.toml -log-level info -run-id "$$RUN_ID" --env-file $(PWD)/sample/.env \
		2>&1 | tee /tmp/slack-group-test/test-output.log || EXIT_CODE=$$?; \
	echo ""; \
	echo "=== Test Results ==="; \
	echo "Expected notifications:"; \
	echo "  1. SUCCESS notification for 'success_group'"; \
	echo "  2. ERROR notification for 'failure_group' (this group is designed to fail)"; \
	echo "  3. SUCCESS notification for 'second_success_group'"; \
	echo "  4. ERROR notification for 'mixed_group' (ends with failure)"; \
	echo ""; \
	echo "Check the log output above for messages containing:"; \
	echo "  - 'slack_notify=true'"; \
	echo "  - 'message_type=command_group_summary'"; \
	echo "  - 'status=success' or 'status=error'"; \
	echo ""; \
	$(RM) -r /tmp/slack-group-test; \
	echo ""; \
	echo "Slack group notification test completed with exit code: $$EXIT_CODE"; \
	exit $$EXIT_CODE
