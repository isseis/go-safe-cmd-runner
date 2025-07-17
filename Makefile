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

.PHONY: all lint build run clean test hash integration-test

all: build

lint:
	$(GOLINT)

# The phony 'build' target now depends on the actual binary file.
build: $(BINARY_RECORD) $(BINARY_VERIFY) $(BINARY_RUNNER)

# This rule tells make how to build the binary from the source files.
# It will only run if the binary doesn't exist or if a .go file has changed.
$(BINARY_RECORD): $(GO_SOURCES)
	@mkdir -p $(@D)
	$(GOBUILD) $(BUILD_FLAGS) -o build/record -v cmd/record/main.go

$(BINARY_VERIFY): $(GO_SOURCES)
	@mkdir -p $(@D)
	$(GOBUILD) $(BUILD_FLAGS) -o build/verify -v cmd/verify/main.go

$(BINARY_RUNNER): $(GO_SOURCES)
	@mkdir -p $(@D)
	$(GOBUILD) $(BUILD_FLAGS) -o build/runner -v cmd/runner/main.go

clean:
	$(GOCLEAN)
	rm -f $(BINARY_RECORD) $(BINARY_VERIFY) $(BINARY_RUNNER)

hash: $(BINARY_RECORD) ./sample/test.toml ./sample/config.toml
	$(SUDOCMD) $(BINARY_RECORD) -force -file ./sample/test.toml -hash-dir $(DEFAULT_HASH_DIRECTORY)
	$(SUDOCMD) $(BINARY_RECORD) -force -file ./sample/config.toml -hash-dir $(DEFAULT_HASH_DIRECTORY)

test: $(BINARY_RUNNER)
	$(GOTEST) -v ./...
	$(ENVCMD) -i $(BINARY_RUNNER) -dry-run -config ./sample/config.toml --disable-verification

integration-test: $(BINARY_RUNNER)
	$(ENVCMD) -i PATH=/bin:/sbin/usr/bin:/usr/sbin LANG=C $(BINARY_RUNNER) -config ./sample/test.toml
