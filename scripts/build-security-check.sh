#!/bin/bash

# Build Security Check Script
# This script performs a complete build with security validation
# for both development and CI/CD environments.

set -euo pipefail

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Script configuration
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

# Default values
BUILD_TYPE="production"
VERBOSE=false
CLEANUP=true
RUN_TESTS=false

# Function to print colored messages
print_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

print_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

print_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

# Function to run command with optional verbosity
run_command() {
    local description="$1"
    shift

    print_info "$description"

    if [ "$VERBOSE" = true ]; then
        echo "Running: $*"
        "$@"
    else
        if ! "$@" >/dev/null 2>&1; then
            print_error "Failed: $description"
            return 1
        fi
    fi

    print_success "Completed: $description"
    return 0
}

# Function to check prerequisites
check_prerequisites() {
    print_info "Checking prerequisites..."

    # Check if Go is installed
    if ! command -v go >/dev/null 2>&1; then
        print_error "Go is not installed or not in PATH"
        return 1
    fi

    local go_version
    go_version=$(go version | cut -d' ' -f3)
    echo "  Go version: $go_version"

    # Check if make is available
    if ! command -v make >/dev/null 2>&1; then
        print_error "make is not installed or not in PATH"
        return 1
    fi

    # Check if we're in the correct directory
    if [ ! -f "$PROJECT_ROOT/go.mod" ]; then
        print_error "Not in a Go project directory (go.mod not found)"
        return 1
    fi

    print_success "Prerequisites check completed"
    return 0
}

# Function to perform clean build
perform_build() {
    local build_type="$1"

    print_info "Performing $build_type build..."

    # Change to project root
    cd "$PROJECT_ROOT"

    # Clean previous builds if requested
    if [ "$CLEANUP" = true ]; then
        if ! run_command "Cleaning previous builds" make clean; then
            return 1
        fi
    fi

    # Perform build based on type
    case "$build_type" in
        "production")
            if ! run_command "Building production binaries" make build-production; then
                return 1
            fi
            ;;
        "test")
            if ! run_command "Building test binaries" make build-test; then
                return 1
            fi
            ;;
        *)
            print_error "Unknown build type: $build_type"
            return 1
            ;;
    esac

    print_success "$build_type build completed"
    return 0
}

# Function to run security validation
run_security_validation() {
    print_info "Running security validation..."

    cd "$PROJECT_ROOT"

    # Run Makefile security check
    if ! run_command "Running Makefile security checks" make security-check; then
        return 1
    fi

    # Run our detailed validation script
    if [ -x "$SCRIPT_DIR/validate-production-binary.sh" ]; then
        print_info "Running detailed binary validation..."
        if [ "$VERBOSE" = true ]; then
            "$SCRIPT_DIR/validate-production-binary.sh"
        else
            "$SCRIPT_DIR/validate-production-binary.sh" >/dev/null 2>&1
        fi
        print_success "Detailed binary validation completed"
    else
        print_warning "Detailed validation script not found or not executable"
    fi

    print_success "Security validation completed"
    return 0
}

# Function to run tests
run_tests() {
    print_info "Running tests..."

    cd "$PROJECT_ROOT"

    if ! run_command "Running comprehensive test suite" make test; then
        return 1
    fi

    print_success "Tests completed"
    return 0
}

# Function to run linting
run_linting() {
    print_info "Running code quality checks..."

    cd "$PROJECT_ROOT"

    if ! run_command "Running golangci-lint" make lint; then
        return 1
    fi

    print_success "Code quality checks completed"
    return 0
}

# Main build function
main() {
    echo "Build Security Check Script"
    echo "=========================="
    echo "Project: $(basename "$PROJECT_ROOT")"
    echo "Build type: $BUILD_TYPE"
    echo "Verbose: $VERBOSE"
    echo "Run tests: $RUN_TESTS"
    echo "Cleanup: $CLEANUP"
    echo ""

    # Step 1: Check prerequisites
    if ! check_prerequisites; then
        exit 1
    fi

    # Step 2: Perform build
    if ! perform_build "$BUILD_TYPE"; then
        exit 1
    fi

    # Step 3: Run security validation (only for production builds)
    if [ "$BUILD_TYPE" = "production" ]; then
        if ! run_security_validation; then
            exit 1
        fi
    fi

    # Step 4: Run tests if requested
    if [ "$RUN_TESTS" = true ]; then
        if ! run_tests; then
            exit 1
        fi
    fi

    # Step 5: Run linting
    if ! run_linting; then
        exit 1
    fi

    # Success summary
    echo ""
    print_success "=== BUILD SECURITY CHECK COMPLETED ==="
    print_info "Build type: $BUILD_TYPE"
    if [ "$BUILD_TYPE" = "production" ]; then
        print_info "Security validation: PASSED"
    fi
    print_info "Code quality: PASSED"
    if [ "$RUN_TESTS" = true ]; then
        print_info "Tests: PASSED"
    fi
    echo ""
    print_success "All checks completed successfully!"

    exit 0
}

# Help function
show_help() {
    echo "Usage: $0 [OPTIONS]"
    echo ""
    echo "Performs secure build with validation for Go projects."
    echo ""
    echo "OPTIONS:"
    echo "  -t, --type TYPE       Build type: 'production' or 'test' (default: production)"
    echo "  -v, --verbose         Enable verbose output"
    echo "  --no-cleanup          Skip cleaning previous builds"
    echo "  --with-tests          Run test suite after build"
    echo "  -h, --help            Show this help message"
    echo ""
    echo "EXAMPLES:"
    echo "  $0                              # Production build with security checks"
    echo "  $0 --type test --with-tests     # Test build with test execution"
    echo "  $0 --verbose --with-tests       # Verbose production build with tests"
    echo ""
    echo "This script performs:"
    echo "  1. Prerequisites checking (Go, make, project structure)"
    echo "  2. Clean build (production or test)"
    echo "  3. Security validation (production builds only)"
    echo "  4. Optional test execution"
    echo "  5. Code quality checks (linting)"
}

# Parse command line arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        -t|--type)
            BUILD_TYPE="$2"
            if [ "$BUILD_TYPE" != "production" ] && [ "$BUILD_TYPE" != "test" ]; then
                print_error "Invalid build type: $BUILD_TYPE. Must be 'production' or 'test'"
                exit 1
            fi
            shift 2
            ;;
        -v|--verbose)
            VERBOSE=true
            shift
            ;;
        --no-cleanup)
            CLEANUP=false
            shift
            ;;
        --with-tests)
            RUN_TESTS=true
            shift
            ;;
        -h|--help)
            show_help
            exit 0
            ;;
        *)
            print_error "Unknown option: $1"
            show_help
            exit 1
            ;;
    esac
done

# Run main function
main
