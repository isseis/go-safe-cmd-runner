CHOWN=chown
CHMOD=chmod
MKDIR=mkdir -p
RM=rm
ENVCMD=env

GITCMD=git
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOLINT=golangci-lint run
SUDOCMD=sudo
GOFUMPTCMD=gofumpt

# Common gofumpt check and error message
define check_gofumpt
	@if ! command -v $(GOFUMPTCMD) >/dev/null 2>&1; then \
		echo "Error: $(GOFUMPTCMD) is required but not found in PATH"; \
		echo "Please install gofumpt: go install mvdan.cc/gofumpt@latest"; \
		exit 1; \
	fi
endef

# Format files from a list and display what was formatted
# Usage: $(call format_files_from_list,file_list_command)
define format_files_from_list
	TEMP_FILE=$$(mktemp); \
	trap "rm -f \"$$TEMP_FILE\"" EXIT; \
	$(1) | while IFS= read -r file; do \
		if [ -f "$$file" ] && $(GOFUMPTCMD) -d "$$file" | grep -q .; then \
			printf '%s\n' "$$file"; \
		fi; \
	done > "$$TEMP_FILE"; \
	if [ -s "$$TEMP_FILE" ]; then \
		echo "Formatting files:"; \
		while IFS= read -r file; do \
			printf '  %s\n' "$$file"; \
		done < "$$TEMP_FILE"; \
		while IFS= read -r file; do \
			if ! $(GOFUMPTCMD) -w "$$file"; then \
				echo "Error: $(GOFUMPTCMD) failed on $$file"; \
				exit 1; \
			fi; \
		done < "$$TEMP_FILE"; \
	fi
endef



ENVSET=$(ENVCMD) -i HOME=$(HOME) USER=$(USER) PATH=/bin:/sbin:/usr/bin:/usr/sbin LANG=C

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

.PHONY: all lint build run clean test benchmark coverage hash integration-test integration-test-success slack-notify-test slack-group-notification-test fmt fmt-all

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
	rm -f coverage.out coverage.html

hash:
	$(foreach file, $(HASH_TARGETS), \
		$(SUDOCMD) $(BINARY_RECORD) -force -file $(file) -hash-dir $(DEFAULT_HASH_DIRECTORY);)

test: $(BINARY_RUNNER)
	$(ENVSET) CGO_ENABLED=1 $(GOTEST) -race -p 2 -v ./...
	$(ENVSET) CGO_ENABLED=0 $(GOTEST) -p 2 -v ./...
	$(ENVSET) $(BINARY_RUNNER) -dry-run -config ./sample/comprehensive.toml

fmt:
	$(call check_gofumpt)
	@TEMP_CHANGED="/tmp/fmt-changed-$$$$.tmp"; \
	{ git diff --name-only HEAD; git diff --name-only --cached; } | grep '\.go$$' | sort -u > "$$TEMP_CHANGED"; \
	if [ -s "$$TEMP_CHANGED" ]; then \
		$(call format_files_from_list,cat "$$TEMP_CHANGED"); \
	else \
		echo "No changed Go files to format"; \
	fi; \
	rm -f "$$TEMP_CHANGED"

fmt-all:
	$(call check_gofumpt)
	@$(call format_files_from_list,find . -name '*.go' -not -path './vendor/*')

benchmark:
	$(GOTEST) -bench=. -benchmem ./internal/runner/resource/

coverage:
	$(ENVSET) $(GOTEST) -coverprofile=coverage.out ./...
	$(GOCMD) tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

integration-test: $(BINARY_RUNNER)
	$(MKDIR) /tmp/cmd-runner-comprehensive /tmp/custom-workdir-test
	@EXIT_CODE=0; \
	$(ENVSET) $(BINARY_RUNNER) -config ./sample/comprehensive.toml -log-level warn -env-file $(PWD)/sample/.env || EXIT_CODE=$$?; \
	$(RM) -r /tmp/cmd-runner-comprehensive /tmp/custom-workdir-test; \
	echo "Integration test completed with exit code: $$EXIT_CODE"; \
	exit $$EXIT_CODE

slack-notify-test: $(BINARY_RUNNER)
	$(MKDIR) /tmp/cmd-runner-slack-test
	@EXIT_CODE=0; \
	$(ENVSET) $(BINARY_RUNNER) -config ./sample/slack-notify.toml -log-level warn -env-file $(PWD)/sample/.env || EXIT_CODE=$$?; \
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
	$(ENVSET) SLACK_WEBHOOK_URL="$$SLACK_WEBHOOK_URL" \
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
