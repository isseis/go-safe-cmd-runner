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
GOLINT=golangci-lint run --build-tags test
SUDOCMD=sudo
GOFUMPTCMD=gofumpt

PYTHON=python3

# Common gofumpt check and error message
define check_gofumpt
	@if ! command -v $(GOFUMPTCMD) >/dev/null 2>&1; then \
		echo "Error: $(GOFUMPTCMD) is required but not found in PATH"; \
		echo "Please install gofumpt: go install mvdan.cc/gofumpt@latest"; \
		exit 1; \
	fi
endef

# Check for Slack webhook URL environment variable
define check_slack_webhook
	@if [ -z "$$GSCR_SLACK_WEBHOOK_URL_SUCCESS" -o -z "$$GSCR_SLACK_WEBHOOK_URL_ERROR" ]; then \
		echo "Warning: GSCR_SLACK_WEBHOOK_URL_SUCCESS and GSCR_SLACK_WEBHOOK_URL_ERROR environment variables must be set"; \
		echo "Slack notifications will be disabled during this test"; \
		echo "To enable notifications, set:"; \
		echo "  export GSCR_SLACK_WEBHOOK_URL_SUCCESS=your_webhook_url"; \
		echo "  export GSCR_SLACK_WEBHOOK_URL_ERROR=your_webhook_url"; \
		echo ""; \
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


ENVSET=$(ENVCMD) -i \
	HOME=$(HOME) \
	USER=$(USER) \
	PATH=/bin:/sbin:/usr/bin:/usr/sbin:/usr/local/go/bin \
	LANG=C \
	TERM=$(TERM) \
	TEST_GLOBAL_VAR=global_test_value \
	COMPREHENSIVE_TEST=enabled \
	NODE_ENV=test \
	DEBUG_MODE=true \
	FINAL_TEST_VAR=comprehensive_success \
	TEST_SECURITY_VAR=security_value

# Configuration paths
DEFAULT_HASH_DIRECTORY=/usr/local/etc/go-safe-cmd-runner/hashes

BINARY_NAME=go-safe-cmd-runner
# Production binaries
BINARY_RECORD=build/prod/record
BINARY_VERIFY=build/prod/verify
BINARY_RUNNER=build/prod/runner
# Test binaries
BINARY_TEST_RECORD=build/test/record
BINARY_TEST_VERIFY=build/test/verify
BINARY_TEST_RUNNER=build/test/runner

# Build flags to embed configuration values
BUILD_FLAGS=-ldflags "-s -w -X main.DefaultHashDirectory=$(DEFAULT_HASH_DIRECTORY)"

# Find all Go source files to use as dependencies for the build
GO_SOURCES := $(shell find . -type f -name '*.go' -not -name '*_test.go')

HASH_TARGETS := \
	/etc/passwd \
	./sample/comprehensive.toml \
	./sample/slack-notify.toml \
	./sample/slack-group-notification-test.toml

.PHONY: all lint build run clean test benchmark coverage coverage-internal hash hash-integration-test integration-test integration-test-success slack-notify-test slack-group-notification-test fmt fmt-all security-check build-security-check performance-test additional-test deadcode generate-perf-configs verify-docs verify-docs-full e2e-test

all: security-check

lint:
	$(GOLINT)

# Build production binaries only
build: $(BINARY_RECORD) $(BINARY_VERIFY) $(BINARY_RUNNER)

# Security check: Verify production binaries exclude test functions
security-check: build
	$(PYTHON) scripts/additional-security-checks.py production-validation

# Build with comprehensive security validation
build-security-check:
	$(PYTHON) scripts/additional-security-checks.py build-security

# This rule tells make how to build the binary from the source files.
# It will only run if the binary doesn't exist or if a .go file has changed.
# Production binary build rules
$(BINARY_RECORD): $(GO_SOURCES)
	@$(MKDIR) $(@D)
	$(GOBUILD) $(BUILD_FLAGS) -o $@ -v cmd/record/main.go

$(BINARY_VERIFY): $(GO_SOURCES)
	@$(MKDIR) $(@D)
	$(GOBUILD) $(BUILD_FLAGS) -o $@ -v cmd/verify/main.go

$(BINARY_RUNNER): $(GO_SOURCES)
	@$(MKDIR) $(@D)
	$(GOBUILD) $(BUILD_FLAGS) -o $@ -v cmd/runner/main.go
	$(SUDOCMD) $(CHOWN) root:root $@
	$(SUDOCMD) $(CHMOD) u+s $@

# Test binary build rules
$(BINARY_TEST_RECORD): $(GO_SOURCES)
	@$(MKDIR) $(@D)
	$(GOBUILD) $(BUILD_FLAGS) -tags test -o $@ -v cmd/record/main.go

$(BINARY_TEST_VERIFY): $(GO_SOURCES)
	@$(MKDIR) $(@D)
	$(GOBUILD) $(BUILD_FLAGS) -tags test -o $@ -v cmd/verify/main.go

$(BINARY_TEST_RUNNER): $(GO_SOURCES)
	@$(MKDIR) $(@D)
	$(GOBUILD) $(BUILD_FLAGS) -tags test -o $@ -v cmd/runner/main.go

clean:
	$(GOCLEAN)
	rm -f $(BINARY_RECORD) $(BINARY_VERIFY) $(BINARY_RUNNER)
	rm -f $(BINARY_TEST_RECORD) $(BINARY_TEST_VERIFY) $(BINARY_TEST_RUNNER)
	rm -f coverage.out coverage.html
	rm -f test/performance/medium_scale.toml test/performance/large_scale.toml

hash:
	$(foreach file, $(HASH_TARGETS), \
		$(SUDOCMD) $(BINARY_RECORD) -force -file $(file) -hash-dir $(DEFAULT_HASH_DIRECTORY);)

# Update hash for integration-test target
# Includes: config file and all files referenced in verify_files
HASH_INTEGRATION_TEST_TARGETS := \
	./sample/comprehensive.toml \
	/etc/passwd \
	/bin/sh \
	/bin/echo \
	/usr/bin/env

hash-integration-test: $(BINARY_RECORD)
	$(foreach file, $(HASH_INTEGRATION_TEST_TARGETS), \
		$(SUDOCMD) $(BINARY_RECORD) -force -file $(file) -hash-dir $(DEFAULT_HASH_DIRECTORY);)

# Test build with test tags enabled
build-test: $(BINARY_TEST_RECORD) $(BINARY_TEST_VERIFY) $(BINARY_TEST_RUNNER)

test: build-test
	$(ENVSET) CGO_ENABLED=1 $(GOTEST) -tags test -race -p 2 -v ./...
	$(ENVSET) CGO_ENABLED=0 $(GOTEST) -tags test -p 2 -v ./...

additional-test: test
	$(ENVSET) $(BINARY_TEST_RUNNER) -dry-run -config ./sample/comprehensive.toml
	$(PYTHON) scripts/test_additional_security_checks.py

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
	$(GOTEST) -tags test -bench=. -benchmem ./internal/runner/resource/ ./internal/runner/config

coverage:
	$(ENVSET) $(GOTEST) -tags test -coverprofile=coverage.out $$(go list ./internal/... | grep -v '/testing$$')
	$(GOCMD) tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

coverage-internal:
	$(ENVSET) $(GOTEST) -tags test -coverprofile=coverage_internal.out ./internal/...
	$(GOCMD) tool cover -html=coverage_internal.out -o coverage_internal.html
	@echo "Internal packages coverage report generated: coverage_internal.html"
	@$(GOCMD) tool cover -func=coverage_internal.out | tail -1

integration-test: $(BINARY_RUNNER) hash-integration-test
	$(call check_slack_webhook)
	$(MKDIR) /tmp/cmd-runner-comprehensive /tmp/custom-workdir-test /tmp/final-comprehensive-test
	@EXIT_CODE=0; \
	$(ENVSET) GSCR_SLACK_WEBHOOK_URL_SUCCESS="$$GSCR_SLACK_WEBHOOK_URL_SUCCESS" \
		GSCR_SLACK_WEBHOOK_URL_ERROR="$$GSCR_SLACK_WEBHOOK_URL_ERROR" \
		$(BINARY_RUNNER) -config ./sample/comprehensive.toml -log-level warn || EXIT_CODE=$$?; \
	$(RM) -r /tmp/cmd-runner-comprehensive /tmp/custom-workdir-test /tmp/final-comprehensive-test; \
	echo "Integration test completed with exit code: $$EXIT_CODE"; \
	exit $$EXIT_CODE

slack-notify-test: $(BINARY_RUNNER)
	$(call check_slack_webhook)
	$(MKDIR) /tmp/cmd-runner-slack-test
	@EXIT_CODE=0; \
	$(ENVSET) GSCR_SLACK_WEBHOOK_URL_SUCCESS="$$GSCR_SLACK_WEBHOOK_URL_SUCCESS" \
		GSCR_SLACK_WEBHOOK_URL_ERROR="$$GSCR_SLACK_WEBHOOK_URL_ERROR" \
		$(BINARY_RUNNER) -config ./sample/slack-notify.toml -log-level warn || EXIT_CODE=$$?; \
	$(RM) -r /tmp/cmd-runner-slack-test; \
	echo "Slack notification test completed with exit code: $$EXIT_CODE"; \
	exit $$EXIT_CODE

# Test the new group-level Slack notification functionality
# This target tests notifications sent after each command group execution
slack-group-notification-test: $(BINARY_RUNNER)
	$(call check_slack_webhook)
	@$(MKDIR) /tmp/slack-group-test
	@EXIT_CODE=0; \
	RUN_ID="slack-test-$$(date +%s)"; \
	echo "Running Slack group notification test with run ID: $$RUN_ID"; \
	$(ENVSET) GSCR_SLACK_WEBHOOK_URL_SUCCESS="$$GSCR_SLACK_WEBHOOK_URL_SUCCESS" \
		GSCR_SLACK_WEBHOOK_URL_ERROR="$$GSCR_SLACK_WEBHOOK_URL_ERROR" \
		$(BINARY_RUNNER) -config ./sample/slack-group-notification-test.toml -log-level info -run-id "$$RUN_ID" \
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

# Generate performance test configuration files
generate-perf-configs:
	@echo "Generating performance test configurations..."
	@cd test/performance && ./generate_medium.sh
	@cd test/performance && ./generate_large.sh
	@echo "Performance test configurations generated successfully"

performance-test: generate-perf-configs
	$(ENVSET) $(GOTEST) -tags performance -v ./test/performance/

security-test:
	$(ENVTEST) $(GOTEST) -tags test -v ./test/security/

deadcode:
	deadcode ./cmd/record ./cmd/runner ./cmd/verify

# Documentation verification targets
verify-docs:
	@./scripts/verification/run_all.sh

verify-docs-full:
	@./scripts/verification/run_all.sh -v -e
