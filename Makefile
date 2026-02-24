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

# Check for Slack webhook URL environment variables
# Note: The application supports ERROR-only configuration (success notifications disabled),
# but these tests require both URLs for full notification coverage.
define check_slack_webhook
	@if [ -z "$$GSCR_SLACK_WEBHOOK_URL_SUCCESS" ] || [ -z "$$GSCR_SLACK_WEBHOOK_URL_ERROR" ]; then \
		echo "Warning: For full test coverage, both Slack webhook environment variables should be set"; \
		echo "Currently missing:"; \
		[ -z "$$GSCR_SLACK_WEBHOOK_URL_SUCCESS" ] && echo "  - GSCR_SLACK_WEBHOOK_URL_SUCCESS (success notifications)"; \
		[ -z "$$GSCR_SLACK_WEBHOOK_URL_ERROR" ] && echo "  - GSCR_SLACK_WEBHOOK_URL_ERROR (error notifications)"; \
		echo ""; \
		echo "To enable all notifications, set:"; \
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

.PHONY: all lint build run clean test test-ci test-all benchmark coverage coverage-internal hash hash-integration-test integration-test slack-notify-test slack-group-notification-test fmt fmt-all security-check build-security-check performance-test unit-test e2e-test security-test deadcode generate-perf-configs verify-docs verify-docs-full elfanalyzer-testdata elfanalyzer-testdata-verify elfanalyzer-testdata-clean elfanalyzer-integration-test

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

# =============================================================================
# ELF Analyzer Test Data Generation
# =============================================================================
# Generates test binaries for elfanalyzer package unit tests.
# Prerequisites: GCC, libssl-dev

ELFANALYZER_TESTDATA_DIR := internal/runner/security/elfanalyzer/testdata

# List of required test binaries
ELFANALYZER_TEST_BINARIES := \
	$(ELFANALYZER_TESTDATA_DIR)/with_socket.elf \
	$(ELFANALYZER_TESTDATA_DIR)/with_ssl.elf \
	$(ELFANALYZER_TESTDATA_DIR)/no_network.elf \
	$(ELFANALYZER_TESTDATA_DIR)/static.elf \
	$(ELFANALYZER_TESTDATA_DIR)/corrupted.elf \
	$(ELFANALYZER_TESTDATA_DIR)/script.sh

# Verify elfanalyzer test binaries by running elfanalyzer tests
# This ensures the generated binaries have the expected symbols
elfanalyzer-testdata-verify: $(ELFANALYZER_TEST_BINARIES)
	@echo "Verifying elfanalyzer test binaries..."
	@TEMP_FILE=$$(mktemp); \
	if $(GOTEST) -tags test -v ./internal/runner/security/elfanalyzer/ -run TestStandardELFAnalyzer_AnalyzeNetworkSymbols > "$$TEMP_FILE" 2>&1; then \
		rm -f "$$TEMP_FILE"; \
	else \
		cat "$$TEMP_FILE"; \
		rm -f "$$TEMP_FILE"; \
		echo "ERROR: elfanalyzer test binaries verification failed"; \
		exit 1; \
	fi
	@echo "elfanalyzer test binaries verified successfully"

# Individual test binary targets
$(ELFANALYZER_TESTDATA_DIR)/with_socket.elf:
	@echo "Generating $@..."
	@echo '#include <sys/socket.h>\n#include <netinet/in.h>\nint main() { int fd = socket(AF_INET, SOCK_STREAM, 0); struct sockaddr_in addr = {0}; connect(fd, (struct sockaddr*)&addr, sizeof(addr)); return 0; }' | \
		gcc -x c -o $@ -

$(ELFANALYZER_TESTDATA_DIR)/with_ssl.elf:
	@echo "Generating $@..."
	@echo '#include <openssl/ssl.h>\nint main() { SSL_CTX *ctx = SSL_CTX_new(TLS_client_method()); SSL_CTX_free(ctx); return 0; }' | \
		gcc -x c -o $@ - -lssl -lcrypto

$(ELFANALYZER_TESTDATA_DIR)/no_network.elf:
	@echo "Generating $@..."
	@echo '#include <stdio.h>\nint main() { printf("Hello, World!\\n"); return 0; }' | \
		gcc -x c -o $@ -

$(ELFANALYZER_TESTDATA_DIR)/static.elf:
	@echo "Generating $@..."
	@echo '#include <stdio.h>\nint main() { printf("Hello, World!\\n"); return 0; }' | \
		gcc -x c -static -o $@ -

$(ELFANALYZER_TESTDATA_DIR)/corrupted.elf:
	@echo "Generating $@..."
	@/usr/bin/printf '\x7fELF' > $@
	@dd if=/dev/urandom bs=100 count=1 >> $@ 2>/dev/null

$(ELFANALYZER_TESTDATA_DIR)/script.sh:
	@echo "Generating $@..."
	@echo '#!/bin/bash\necho "Hello"' > $@
	@chmod +x $@

# Convenience target to generate all test binaries
elfanalyzer-testdata: $(ELFANALYZER_TEST_BINARIES)
	@echo "elfanalyzer test binaries generated successfully"

# Clean elfanalyzer test binaries
elfanalyzer-testdata-clean:
	rm -f $(ELFANALYZER_TEST_BINARIES)

hash:
	$(foreach file, $(HASH_TARGETS), \
		$(SUDOCMD) $(BINARY_RECORD) -force -hash-dir $(DEFAULT_HASH_DIRECTORY) $(file);)

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
		$(SUDOCMD) $(BINARY_RECORD) -force -hash-dir $(DEFAULT_HASH_DIRECTORY) $(file);)

# Test build with test tags enabled
build-test: $(BINARY_TEST_RECORD) $(BINARY_TEST_VERIFY) $(BINARY_TEST_RUNNER)

# =============================================================================
# Test Targets
# =============================================================================
# Individual test targets:
#   unit-test              - Unit tests (race detection enabled and disabled)
#   integration-test       - Integration tests with runner binary
#   e2e-test               - End-to-end tests (dry-run validation + security checks)
#   security-test          - Security-focused tests
#   performance-test       - Performance and benchmark tests
#   slack-notify-test      - Slack notification tests
#   slack-group-notification-test - Slack group notification tests
#
# Composite test targets:
#   test                   - Tests for pre-commit (unit-test only)
#   test-ci                - Tests for CI environments (no sudo required)
#   test-all               - All tests including integration (requires sudo)
# =============================================================================

# Unit tests - core functionality tests
# Runs twice: with race detection (CGO_ENABLED=1) and without (CGO_ENABLED=0)
unit-test: build-test elfanalyzer-testdata-verify
	$(ENVSET) CGO_ENABLED=1 $(GOTEST) -tags test -race -p 2 -v ./...
	$(ENVSET) CGO_ENABLED=0 $(GOTEST) -tags test -p 2 -v ./...

# End-to-end tests - validates binary execution and security checks
e2e-test: build-test
	$(ENVSET) $(BINARY_TEST_RUNNER) -dry-run -config ./sample/comprehensive.toml
	$(PYTHON) scripts/test_additional_security_checks.py

# Pre-commit test target - runs unit tests only
# This is the default test target for daily development
test: unit-test

# ELF analyzer integration tests - runs integration-tagged tests for elfanalyzer package
# Requires: gcc (for TestSyscallAnalyzer_RealCBinary), amd64 arch
# Tests gracefully skip if requirements are not met (t.Skip)
elfanalyzer-integration-test:
	$(ENVSET) CGO_ENABLED=1 $(GOTEST) -tags integration -v ./internal/runner/security/elfanalyzer/

# CI test target - tests that can run without sudo or external services
# Suitable for GitHub Actions and other CI environments
test-ci: unit-test e2e-test security-test performance-test elfanalyzer-integration-test

# All tests - comprehensive test suite (requires sudo for integration-test)
# Excludes Slack notification tests (require external webhook configuration)
test-all: unit-test integration-test e2e-test security-test performance-test

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

# Integration tests - tests with actual runner binary execution
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

# =============================================================================
# Slack Notification Tests (require external webhook configuration)
# =============================================================================

# Slack notification test - tests basic Slack notification functionality
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

# Slack group notification test - tests notifications sent after each command group execution
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

# Performance tests - performance and memory usage tests
performance-test: generate-perf-configs
	$(ENVSET) $(GOTEST) -tags performance -v ./test/performance/

# Security tests - security-focused test cases
security-test:
	$(ENVSET) $(GOTEST) -tags test -v ./test/security/

deadcode:
	deadcode ./cmd/record ./cmd/runner ./cmd/verify

# Documentation verification targets
verify-docs:
	@./scripts/verification/run_all.sh

verify-docs-full:
	@./scripts/verification/run_all.sh -v -e
