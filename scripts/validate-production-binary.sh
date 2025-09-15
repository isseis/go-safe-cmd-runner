#!/bin/bash

# Production Binary Validation Script
# This script validates that production binaries do not contain test functions
# and meet security requirements.

set -euo pipefail

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Script configuration
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
BUILD_DIR="${PROJECT_ROOT}/build"

# Function to print colored messages
print_info() {
    echo -e "${YELLOW}[INFO]${NC} $1"
}

print_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

print_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Function to validate binary exists and is executable
validate_binary_exists() {
    local binary_path="$1"
    local binary_name="$(basename "$binary_path")"

    if [ ! -f "$binary_path" ]; then
        print_error "Binary not found: $binary_path"
        return 1
    fi

    if [ ! -x "$binary_path" ]; then
        print_error "Binary is not executable: $binary_path"
        return 1
    fi

    print_success "Binary exists and is executable: $binary_name"
    return 0
}

# Function to check for test functions in binary
check_test_functions() {
    local binary_path="$1"
    local binary_name="$(basename "$binary_path")"

    print_info "Checking $binary_name for test functions..."

    # Check for test-related symbols
    local test_symbols
    test_symbols=$(strings "$binary_path" | grep -E "(NewManagerForTest|testing\.)" | grep -v "runtime" || true)

    if [ -n "$test_symbols" ]; then
        print_error "Test functions found in $binary_name:"
        echo "$test_symbols" | head -10
        if [ $(echo "$test_symbols" | wc -l) -gt 10 ]; then
            echo "... and $(( $(echo "$test_symbols" | wc -l) - 10 )) more"
        fi
        return 1
    fi

    print_success "No test functions found in $binary_name"
    return 0
}

# Function to check binary size and basic properties
check_binary_properties() {
    local binary_path="$1"
    local binary_name="$(basename "$binary_path")"

    print_info "Checking properties of $binary_name..."

    local size_bytes
    size_bytes=$(stat -c%s "$binary_path")
    local size_mb=$((size_bytes / 1024 / 1024))

    echo "  Size: ${size_mb}MB (${size_bytes} bytes)"

    # Check if binary is stripped (should not be for debugging, but good to know)
    if file "$binary_path" | grep -q "not stripped"; then
        echo "  Debug info: Present (not stripped)"
    else
        echo "  Debug info: Stripped"
    fi

    # Check architecture
    local arch
    arch=$(file "$binary_path" | grep -o 'x86[_-]64\|aarch64\|arm64' || echo "unknown")
    echo "  Architecture: $arch"

    print_success "Properties check completed for $binary_name"
    return 0
}

# Function to run comprehensive validation
validate_production_binary() {
    local binary_path="$1"
    local binary_name="$(basename "$binary_path")"

    echo ""
    print_info "=== Validating $binary_name ==="

    # Step 1: Check if binary exists and is executable
    if ! validate_binary_exists "$binary_path"; then
        return 1
    fi

    # Step 2: Check for test functions
    if ! check_test_functions "$binary_path"; then
        return 1
    fi

    # Step 3: Check binary properties
    if ! check_binary_properties "$binary_path"; then
        return 1
    fi

    print_success "=== $binary_name validation completed successfully ==="
    echo ""
    return 0
}

# Main function
main() {
    echo "Production Binary Validation Script"
    echo "=================================="
    echo "Project: $(basename "$PROJECT_ROOT")"
    echo "Build directory: $BUILD_DIR"
    echo ""

    # Change to project root
    cd "$PROJECT_ROOT"

    # Check if build directory exists
    if [ ! -d "$BUILD_DIR" ]; then
        print_error "Build directory not found: $BUILD_DIR"
        print_info "Run 'make build' first to create production binaries"
        exit 1
    fi

    # Define binaries to validate
    local binaries=("record" "verify" "runner")
    local validation_failed=false

    # Validate each binary
    for binary in "${binaries[@]}"; do
        local binary_path="${BUILD_DIR}/${binary}"

        if ! validate_production_binary "$binary_path"; then
            validation_failed=true
        fi
    done

    # Summary
    echo ""
    echo "=== VALIDATION SUMMARY ==="
    if [ "$validation_failed" = true ]; then
        print_error "Production binary validation FAILED"
        print_info "Some binaries did not pass security validation"
        exit 1
    else
        print_success "All production binaries passed validation"
        print_info "Binaries are ready for production deployment"
        exit 0
    fi
}

# Help function
show_help() {
    echo "Usage: $0 [OPTIONS]"
    echo ""
    echo "Validates production binaries for security compliance."
    echo ""
    echo "OPTIONS:"
    echo "  -h, --help    Show this help message"
    echo ""
    echo "This script performs the following validations:"
    echo "  1. Checks if binaries exist and are executable"
    echo "  2. Verifies no test functions are present in production binaries"
    echo "  3. Analyzes binary properties (size, architecture, etc.)"
    echo ""
    echo "Run 'make build' before using this script to ensure binaries exist."
}

# Parse command line arguments
case "${1:-}" in
    -h|--help)
        show_help
        exit 0
        ;;
    "")
        main
        ;;
    *)
        print_error "Unknown option: $1"
        show_help
        exit 1
        ;;
esac
