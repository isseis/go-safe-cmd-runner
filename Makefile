# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOLINT=golangci-lint run
BINARY_NAME=go-safe-cmd-runner
BINARY_PATH=build/record build/verify

# Find all Go source files to use as dependencies for the build
GO_SOURCES := $(shell find . -type f -name '*.go' -not -name '*_test.go')

.PHONY: all lint build run clean test

all: build

lint:
	$(GOLINT)

# The phony 'build' target now depends on the actual binary file.
build: $(BINARY_PATH)

# This rule tells make how to build the binary from the source files.
# It will only run if the binary doesn't exist or if a .go file has changed.
$(BINARY_PATH): $(GO_SOURCES)
	@mkdir -p $(@D)
	$(GOBUILD) -o build/record -v cmd/record/main.go
	$(GOBUILD) -o build/verify -v cmd/verify/main.go

run: $(BINARY_PATH)
	./$(BINARY_PATH)

clean:
	$(GOCLEAN)
	rm -f $(BINARY_PATH)

test:
	$(GOTEST) -v ./...
