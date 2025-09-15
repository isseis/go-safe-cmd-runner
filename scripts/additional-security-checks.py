#!/usr/bin/env python3
"""
additional-security-checks.py
Supplementary security validation script for go-safe-cmd-runner
This script provides additional security checks beyond golangci-lint forbidigo

This is a Python port of the original bash script.
"""

import os
import sys
import subprocess
import shutil
import stat
import argparse
from pathlib import Path
from typing import List, Optional


class Colors:
    """ANSI color codes for terminal output."""
    RED = '\033[0;31m'
    GREEN = '\033[0;32m'
    YELLOW = '\033[1;33m'
    NC = '\033[0m'  # No Color


class SecurityChecker:
    """Security checker for go-safe-cmd-runner project."""

    def __init__(self):
        self.exit_code = 0

    def print_status(self, color: str, message: str) -> None:
        """Print colored status message."""
        print(f"{color}{message}{Colors.NC}")

    def print_error(self, message: str) -> None:
        """Print error message."""
        self.print_status(Colors.RED, f"ERROR: {message}")

    def print_success(self, message: str) -> None:
        """Print success message."""
        self.print_status(Colors.GREEN, f"PASS: {message}")

    def print_warning(self, message: str) -> None:
        """Print warning message."""
        self.print_status(Colors.YELLOW, f"WARNING: {message}")

    def print_info(self, message: str) -> None:
        """Print info message."""
        self.print_status(Colors.NC, f"INFO: {message}")

    def run_command(self, cmd: List[str], capture_output: bool = True,
                   check: bool = True) -> subprocess.CompletedProcess:
        """Run a shell command and return the result."""
        # Always run with check=False and handle errors explicitly so this
        # function consistently returns a CompletedProcess on success.
        result = subprocess.run(
            cmd,
            capture_output=capture_output,
            text=True,
            check=False
        )

        if check and result.returncode != 0:
            # Raise a CalledProcessError with stdout/stderr attached to match
            # prior behaviour when callers expect exceptions on failure.
            raise subprocess.CalledProcessError(
                result.returncode, cmd, output=result.stdout, stderr=result.stderr
            )

        return result

    def extract_strings_from_binary(self, binary_path: str, min_length: int = 4) -> List[str]:
        """Extract printable strings from a binary file using Python."""
        strings = []
        try:
            with open(binary_path, 'rb') as f:
                current_string = bytearray()

                while True:
                    byte = f.read(1)
                    if not byte:
                        break

                    byte_val = byte[0]
                    # Check if byte is printable ASCII (32-126)
                    if 32 <= byte_val <= 126:
                        current_string.append(byte_val)
                    else:
                        # End of string, add if long enough
                        if len(current_string) >= min_length:
                            try:
                                strings.append(current_string.decode('ascii'))
                            except UnicodeDecodeError:
                                pass
                        current_string = bytearray()

                # Handle final string if file doesn't end with non-printable
                if len(current_string) >= min_length:
                    try:
                        strings.append(current_string.decode('ascii'))
                    except UnicodeDecodeError:
                        pass

        except IOError as e:
            self.print_warning(f"Could not read binary file {binary_path}: {e}")
            return []

        return strings

    def check_binary_security(self, binary_path: str) -> bool:
        """Check if a binary contains test artifacts."""
        binary_name = Path(binary_path).name
        self.print_info(f"Checking binary security for: {binary_name}")

        if not Path(binary_path).is_file():
            self.print_error(f"Binary not found: {binary_path}")
            return False

        # Extract strings from binary using Python implementation
        try:
            strings_output = self.extract_strings_from_binary(binary_path)

            # Check for common test function patterns
            test_patterns = [
                'NewManagerForTest',
                'testing.T',
                '_test.go'
            ]

            test_functions_found = False
            for pattern in test_patterns:
                matches = [s for s in strings_output if pattern in s]
                if matches:
                    self.print_error(f"Test functions found in production binary: {binary_name}")
                    # Show first 5 matches
                    for match in matches[:5]:
                        print(match)
                    test_functions_found = True
                    break

            # Check for debug/development symbols
            runtime_caller_found = any('runtime.Caller' in s for s in strings_output)
            test_keyword_found = any('test' in s.lower() for s in strings_output)
            if runtime_caller_found and test_keyword_found:
                self.print_warning(f"Development debug symbols found in binary: {binary_name}")

            if not test_functions_found:
                self.print_success(f"No test artifacts found in binary: {binary_name}")
                return True
            else:
                return False

        except Exception as e:
            self.print_warning(f"Failed to analyze binary strings: {e}")
            return True

    def check_build_environment(self) -> bool:
        """Validate build environment integrity."""
        self.print_info("Checking build environment integrity")

        # Check Go version
        try:
            result = self.run_command(['go', 'version'])
            go_version = result.stdout.strip()
            self.print_info(f"Go version: {go_version}")
        except subprocess.CalledProcessError:
            self.print_error("Go is not installed or not in PATH")
            return False

        # Check for go.mod file
        if not Path('go.mod').is_file():
            self.print_error("go.mod file not found")
            return False

        # Verify module integrity
        try:
            self.run_command(['go', 'mod', 'verify'])
        except subprocess.CalledProcessError:
            self.print_error("go mod verify failed - module integrity check failed")
            return False

        self.print_success("Build environment integrity check passed")
        return True

    def check_build_tags(self) -> bool:
        """Validate build tag compliance."""
        self.print_info("Checking build tag compliance")

        files_without_test_tag = []

        # Find all .go files
        for go_file in Path('.').rglob('*.go'):
            # Skip vendor directory
            if 'vendor' in go_file.parts:
                continue

            filename = go_file.name
            if filename == 'manager_testing.go' or filename.endswith('_testing.go'):
                try:
                    with open(go_file, 'r', encoding='utf-8') as f:
                        lines = f.readlines()

                        # Find the package declaration index
                        package_line_idx = next((i for i, l in enumerate(lines) if l.strip().startswith('package ')), None)
                        check_until = package_line_idx if package_line_idx is not None else len(lines)

                        # Check for //go:build test constraint before package declaration
                        has_test_constraint = any(
                            l.strip().startswith('//go:build test') for l in lines[:check_until]
                        )

                        if not has_test_constraint:
                            files_without_test_tag.append(str(go_file))
                except (IOError, UnicodeDecodeError) as e:
                    self.print_warning(f"Could not read file {go_file}: {e}")

        if files_without_test_tag:
            self.print_error("Files with testing APIs missing '//go:build test' tag:")
            for file_path in files_without_test_tag:
                print(file_path)
            return False

        self.print_success("Build tag compliance check passed")
        return True

    def check_forbidden_patterns(self) -> bool:
        """Check for forbidden patterns in source code."""
        self.print_info("Checking for forbidden patterns in source code")

        patterns_found = False

        # Check for removed --hash-directory flag usage using Python search.
        hash_flag_matches: List[str] = []
        try:
            for go_file in Path('.').rglob('*.go'):
                if 'vendor' in go_file.parts:
                    continue
                try:
                    text = go_file.read_text(encoding='utf-8')
                except (IOError, UnicodeDecodeError):
                    continue
                if '--hash-directory' in text:
                    hash_flag_matches.append(str(go_file))

            if hash_flag_matches:
                self.print_error("Found forbidden --hash-directory flag usage:")
                for m in hash_flag_matches:
                    print(m)
                patterns_found = True
        except Exception as e:
            self.print_warning(f"Could not check for --hash-directory pattern: {e}")

        # Check for direct newManagerInternal usage outside verification package
        found_files = []
        for go_file in Path('.').rglob('*.go'):
            # Skip vendor directory, verification package, and test files
            if ('vendor' in go_file.parts or
                'internal/verification' in str(go_file) or
                go_file.name.endswith('_test.go')):
                continue

            try:
                with open(go_file, 'r', encoding='utf-8') as f:
                    content = f.read()
                    if 'newManagerInternal' in content:
                        found_files.append(str(go_file))
            except (IOError, UnicodeDecodeError):
                continue

        if found_files:
            self.print_error("Found forbidden direct newManagerInternal usage outside verification package:")
            for file_path in found_files:
                print(file_path)
            patterns_found = True

        # Check for hardcoded hash directories
        hardcoded_hash_dirs = []
        for go_file in Path('.').rglob('*.go'):
            # Skip vendor directory and test files
            if 'vendor' in go_file.parts or go_file.name.endswith('_test.go'):
                continue

            try:
                with open(go_file, 'r', encoding='utf-8') as f:
                    content = f.read()
                    if '.gocmdhashes' in content:
                        hardcoded_hash_dirs.append(str(go_file))
            except (IOError, UnicodeDecodeError):
                continue

        if hardcoded_hash_dirs:
            self.print_warning("Found potential hardcoded hash directory references:")
            for file_path in hardcoded_hash_dirs:
                print(file_path)

        if not patterns_found:
            self.print_success("No forbidden patterns found")
            return True
        else:
            return False

    def check_binary_permissions(self, binary_path: str) -> bool:
        """Check binary permissions and integrity."""
        binary_file = Path(binary_path)

        if not binary_file.is_file():
            return True  # Skip if binary doesn't exist

        self.print_info(f"Checking binary permissions for: {binary_file.name}")

        # Check file permissions using mode bits.
        file_stat = binary_file.stat()
        mode = file_stat.st_mode
        is_executable = bool(mode & (stat.S_IXUSR | stat.S_IXGRP | stat.S_IXOTH))

        # Expect owner-executable and owner-readable (rwx for owner is common for 755)
        owner_perms = (mode & 0o700) >> 6
        if owner_perms != 0o7:
            # owner_perms is small int 0-7; compare to 0o7 (7) for rwx
            self.print_warning(f"Binary owner permissions unexpected: {oct((mode & 0o700) >> 6)} (expected 0o7)")

        if not is_executable:
            self.print_error(f"Binary is not executable: {binary_path}")
            return False

        self.print_success("Binary permissions check passed")
        return True

    def run_all_checks(self) -> int:
        """Run all security checks."""
        self.print_info("Starting additional security checks for go-safe-cmd-runner")

        success = True

        # Check build environment
        if not self.check_build_environment():
            success = False

        # Check build tags
        if not self.check_build_tags():
            success = False

        # Check for forbidden patterns
        if not self.check_forbidden_patterns():
            success = False

        # Check binaries if they exist
        binaries = ["build/runner", "build/record", "build/verify"]
        for binary in binaries:
            if Path(binary).is_file():
                if not self.check_binary_security(binary):
                    success = False
                if not self.check_binary_permissions(binary):
                    success = False
            else:
                self.print_info(f"Binary not found (skipping): {binary}")

        # Final status
        if success:
            self.print_success("All additional security checks passed")
            return 0
        else:
            self.print_error("Some security checks failed")
            return 1


def main():
    """Main entry point."""
    parser = argparse.ArgumentParser(
        description="Additional security checks for go-safe-cmd-runner"
    )
    parser.add_argument(
        'command',
        nargs='?',
        default='all',
        choices=['all', 'build-env', 'build-tags', 'patterns', 'binary'],
        help='Security check to run'
    )
    parser.add_argument(
        'binary_path',
        nargs='?',
        help='Binary path (required for binary command)'
    )

    args = parser.parse_args()
    checker = SecurityChecker()

    try:
        if args.command == 'build-env':
            success = checker.check_build_environment()
        elif args.command == 'build-tags':
            success = checker.check_build_tags()
        elif args.command == 'patterns':
            success = checker.check_forbidden_patterns()
        elif args.command == 'binary':
            if not args.binary_path:
                checker.print_error("Binary path required for binary check")
                return 1
            success = (checker.check_binary_security(args.binary_path) and
                      checker.check_binary_permissions(args.binary_path))
        else:  # 'all' or default
            return checker.run_all_checks()

        return 0 if success else 1

    except KeyboardInterrupt:
        checker.print_error("Interrupted by user")
        return 130
    except Exception as e:
        checker.print_error(f"Unexpected error: {e}")
        return 1


if __name__ == '__main__':
    sys.exit(main())
