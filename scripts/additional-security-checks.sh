#!/bin/bash

# additional-security-checks.sh
# Supplementary security validation script for go-safe-cmd-runner
# This script provides additional security checks beyond golangci-lint forbidigo

set -euo pipefail

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Function to print colored output
print_status() {
    local color=$1
    local message=$2
    echo -e "${color}${message}${NC}"
}

print_error() {
    print_status "$RED" "ERROR: $1"
}

print_success() {
    print_status "$GREEN" "PASS: $1"
}

print_warning() {
    print_status "$YELLOW" "WARNING: $1"
}

print_info() {
    print_status "$NC" "INFO: $1"
}

# Function to check if a binary contains test artifacts
check_binary_security() {
    local binary_path=$1
    local binary_name=$(basename "$binary_path")

    print_info "Checking binary security for: $binary_name"

    if [[ ! -f "$binary_path" ]]; then
        print_error "Binary not found: $binary_path"
        return 1
    fi

    # Check for test function symbols in the binary
    if command -v strings >/dev/null 2>&1; then
        local test_functions_found=0

        # Check for common test function patterns
        if strings "$binary_path" | grep -q "NewManagerForTest\|testing\.T\|_test\.go"; then
            print_error "Test functions found in production binary: $binary_name"
            strings "$binary_path" | grep -E "(NewManagerForTest|testing\.T|_test\.go)" | head -5
            test_functions_found=1
        fi

        # Check for debug/development symbols
        if strings "$binary_path" | grep -q "runtime\.Caller.*test"; then
            print_warning "Development debug symbols found in binary: $binary_name"
        fi

        if [[ $test_functions_found -eq 0 ]]; then
            print_success "No test artifacts found in binary: $binary_name"
        else
            return 1
        fi
    else
        print_warning "strings command not available, skipping binary artifact check"
    fi

    return 0
}

# Function to validate build environment integrity
check_build_environment() {
    print_info "Checking build environment integrity"

    # Check Go version
    if ! go version >/dev/null 2>&1; then
        print_error "Go is not installed or not in PATH"
        return 1
    fi

    local go_version=$(go version | awk '{print $3}')
    print_info "Go version: $go_version"

    # Check for go.mod file
    if [[ ! -f "go.mod" ]]; then
        print_error "go.mod file not found"
        return 1
    fi

    # Verify module integrity
    if ! go mod verify >/dev/null 2>&1; then
        print_error "go mod verify failed - module integrity check failed"
        return 1
    fi

    print_success "Build environment integrity check passed"
    return 0
}

# Function to validate build tags
check_build_tags() {
    print_info "Checking build tag compliance"

    # Check that testing files have proper build tags
    local files_without_test_tag=()

    while IFS= read -r -d '' file; do
        local filename=$(basename "$file")
        if [[ "$filename" == "manager_testing.go" ]] || [[ "$filename" =~ _testing\.go$ ]]; then
            if ! head -1 "$file" | grep -q "//go:build test"; then
                files_without_test_tag+=("$file")
            fi
        fi
    done < <(find . -name "*.go" -not -path "./vendor/*" -print0)

    if [[ ${#files_without_test_tag[@]} -gt 0 ]]; then
        print_error "Files with testing APIs missing '//go:build test' tag:"
        printf '%s\n' "${files_without_test_tag[@]}"
        return 1
    fi

    print_success "Build tag compliance check passed"
    return 0
}

# Function to check for forbidden patterns in source code
check_forbidden_patterns() {
    print_info "Checking for forbidden patterns in source code"

    local patterns_found=0

    # Check for removed hash-directory flag usage
    if grep -r "--hash-directory" . --include="*.go" --exclude-dir=vendor >/dev/null 2>&1; then
        print_error "Found forbidden --hash-directory flag usage:"
        grep -r "--hash-directory" . --include="*.go" --exclude-dir=vendor
        patterns_found=1
    fi

    # Check for direct newManagerInternal usage outside verification package (excluding test files)
    local found_files
    found_files=$(find . -name "*.go" -not -path "./vendor/*" -not -path "./internal/verification/*" -not -name "*_test.go" -exec grep -l "newManagerInternal" {} \; 2>/dev/null)
    if [[ -n "$found_files" ]]; then
        print_error "Found forbidden direct newManagerInternal usage outside verification package:"
        find . -name "*.go" -not -path "./vendor/*" -not -path "./internal/verification/*" -not -name "*_test.go" -exec grep -H "newManagerInternal" {} \; 2>/dev/null
        patterns_found=1
    fi

    # Check for hardcoded hash directories
    if grep -r "\.gocmdhashes" . --include="*.go" --exclude-dir=vendor --exclude="*_test.go" >/dev/null 2>&1; then
        print_warning "Found potential hardcoded hash directory references:"
        grep -r "\.gocmdhashes" . --include="*.go" --exclude-dir=vendor --exclude="*_test.go"
    fi

    if [[ $patterns_found -eq 0 ]]; then
        print_success "No forbidden patterns found"
    else
        return 1
    fi

    return 0
}

# Function to check binary permissions and integrity
check_binary_permissions() {
    local binary_path=$1

    if [[ ! -f "$binary_path" ]]; then
        return 0  # Skip if binary doesn't exist
    fi

    print_info "Checking binary permissions for: $(basename "$binary_path")"

    # Check file permissions
    local perms=$(stat -c "%a" "$binary_path" 2>/dev/null || stat -f "%A" "$binary_path" 2>/dev/null || echo "unknown")
    if [[ "$perms" != "755" ]] && [[ "$perms" != "0755" ]]; then
        print_warning "Binary has non-standard permissions: $perms (expected 755)"
    fi

    # Check if binary is executable
    if [[ ! -x "$binary_path" ]]; then
        print_error "Binary is not executable: $binary_path"
        return 1
    fi

    print_success "Binary permissions check passed"
    return 0
}

# Main function
main() {
    print_info "Starting additional security checks for go-safe-cmd-runner"

    local exit_code=0

    # Check build environment
    if ! check_build_environment; then
        exit_code=1
    fi

    # Check build tags
    if ! check_build_tags; then
        exit_code=1
    fi

    # Check for forbidden patterns
    if ! check_forbidden_patterns; then
        exit_code=1
    fi

    # Check binaries if they exist
    local binaries=("build/runner" "build/record" "build/verify")
    for binary in "${binaries[@]}"; do
        if [[ -f "$binary" ]]; then
            if ! check_binary_security "$binary"; then
                exit_code=1
            fi
            if ! check_binary_permissions "$binary"; then
                exit_code=1
            fi
        else
            print_info "Binary not found (skipping): $binary"
        fi
    done

    # Final status
    if [[ $exit_code -eq 0 ]]; then
        print_success "All additional security checks passed"
    else
        print_error "Some security checks failed"
    fi

    return $exit_code
}

# Parse command line arguments
case "${1:-all}" in
    "build-env")
        check_build_environment
        ;;
    "build-tags")
        check_build_tags
        ;;
    "patterns")
        check_forbidden_patterns
        ;;
    "binary")
        if [[ -z "${2:-}" ]]; then
            print_error "Binary path required for binary check"
            exit 1
        fi
        check_binary_security "$2"
        check_binary_permissions "$2"
        ;;
    "all"|"")
        main
        ;;
    *)
        echo "Usage: $0 {all|build-env|build-tags|patterns|binary <path>}"
        echo "  all        - Run all security checks (default)"
        echo "  build-env  - Check build environment integrity"
        echo "  build-tags - Check build tag compliance"
        echo "  patterns   - Check for forbidden code patterns"
        echo "  binary     - Check specific binary for security issues"
        exit 1
        ;;
esac
