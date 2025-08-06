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
	/bin/ls \
	/bin/sleep \
	/bin/mkdir \
	/bin/touch \
	/bin/id \
	/bin/sh \
	/bin/echo \
	/bin/env \
	/bin/pwd \
	/bin/printenv \
	/usr/bin/whoami \
	./sample/comprehensive.toml

.PHONY: all lint build run clean test hash integration-test

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
	$(ENVCMD) -i PATH=/bin:/sbin:/usr/bin:/usr/sbin LANG=C $(BINARY_RUNNER) -config ./sample/comprehensive.toml
	$(RM) -r /tmp/cmd-runner-comprehensive /tmp/custom-workdir-test
