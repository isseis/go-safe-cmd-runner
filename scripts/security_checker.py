"""
security_checker.py
Core security checking functionality for go-safe-cmd-runner
"""

import os
import sys
import subprocess
import shutil
import stat
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
        print(message)

    def run_command(self, cmd: List[str], capture_output: bool = True,
                                    check: bool = True) -> subprocess.CompletedProcess:
        """Run a shell command and return the CompletedProcess.

        Notes:
        - The underlying subprocess.run is always invoked with check=False so
            that this function can inspect the CompletedProcess regardless of
            the child process exit code.
        - If the caller passes check=True and the subprocess exit code is
            non-zero, this function will raise subprocess.CalledProcessError to
            signal failure. If check=False (the default behavior for the
            internal run), the CompletedProcess is returned even for non-zero
            exit codes.
        """
        result = subprocess.run(
                cmd,
                capture_output=capture_output,
                text=True,
                check=False
        )

        if check and result.returncode != 0:
                # Raise with stdout/stderr attached for caller convenience.
                raise subprocess.CalledProcessError(result.returncode, cmd, result.stdout, result.stderr)

        return result

    def extract_strings_from_binary(self, binary_path: str, min_length: int = 4) -> List[str]:
        """Extract strings from binary file."""
        try:
            with open(binary_path, 'rb') as f:
                binary_data = f.read()
        except (IOError, OSError):
            return []

        strings = []
        current_string = ""

        for byte in binary_data:
            # Check if byte is printable ASCII
            if 32 <= byte <= 126:  # Printable ASCII range
                current_string += chr(byte)
            else:
                # Non-printable byte, end current string if it meets min length
                if len(current_string) >= min_length:
                    strings.append(current_string)
                current_string = ""

        # Don't forget the last string if file doesn't end with non-printable
        if len(current_string) >= min_length:
            strings.append(current_string)

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

            # Check for user test files only (exclude debug symbols for production builds)
            user_test_patterns = [
                'test.go',
                'testing.go'
            ]

            user_test_files_found = self._contains_user_test_file(strings_output, user_test_patterns)

            # For production binaries, only fail on actual test functions and user test files,
            # not on Go runtime debug symbols which may be present even with -s -w flags
            if test_functions_found:
                return False
            elif user_test_files_found:
                self.print_warning(f"User test file references found in binary: {binary_name}")
                return False
            else:
                self.print_success(f"No test artifacts found in binary: {binary_name}")
                return True

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

    def _contains_any_pattern(self, strings_output: List[str], patterns: List[str]) -> bool:
        """Return True if any of the given patterns appear in any string in strings_output."""
        for s in strings_output:
            for pattern in patterns:
                if pattern in s:
                    return True
        return False

    def _contains_user_test_file(self, strings_output: List[str], user_test_patterns: List[str]) -> bool:
        """Return True if any user test file patterns are present in strings_output,
        excluding entries that look like Go toolchain/module paths.
        """
        for s in strings_output:
            # Skip entries that clearly come from the Go toolchain or module cache
            if 'toolchain@' in s or '/go/pkg/mod/' in s:
                continue
            for pattern in user_test_patterns:
                if pattern in s:
                    return True
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

        # Expect owner-executable and owner-readable (r-x for owner is sufficient)
        owner_mask = stat.S_IRWXU  # Owner read/write/execute mask (0o700)
        owner_perms = (mode & owner_mask) >> 6
        required_perms = stat.S_IRUSR | stat.S_IXUSR  # Read and execute for owner
        if not (mode & required_perms == required_perms):
            # Check that both read and execute bits are set
            self.print_warning(f"Binary owner permissions unexpected: {oct(owner_perms)} (expected at least r-x)")

        if not is_executable:
            self.print_error(f"Binary is not executable: {binary_path}")
            return False

        self.print_success("Binary permissions check passed")
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
            # Skip vendor directory, test files, and the legitimate definition file
            if ('vendor' in go_file.parts or
                go_file.name.endswith('_test.go') or
                str(go_file) == 'internal/cmdcommon/common.go' or
                'Makefile' in str(go_file)):
                continue

            try:
                with open(go_file, 'r', encoding='utf-8') as f:
                    content = f.read()
                    if 'go-safe-cmd-runner/hashes' in content:
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

        # Check production binaries if they exist
        binaries = ["build/prod/runner", "build/prod/record", "build/prod/verify"]
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

    def validate_production_binaries(self) -> bool:
        """Comprehensive validation for production binaries."""
        self.print_info("=== Production Binary Validation ===")

        build_dir = Path("build/prod")
        if not build_dir.exists():
            self.print_error("Production build directory not found. Run 'make build' first.")
            return False

        binaries = ["record", "verify", "runner"]
        all_passed = True

        for binary_name in binaries:
            binary_path = build_dir / binary_name

            self.print_info(f"\n--- Validating {binary_name} ---")

            # Check if binary exists
            if not binary_path.is_file():
                self.print_error(f"Binary not found: {binary_path}")
                all_passed = False
                continue

            # Check binary properties
            file_stat = binary_path.stat()
            size_mb = file_stat.st_size / (1024 * 1024)
            self.print_info(f"Binary size: {size_mb:.1f}MB ({file_stat.st_size} bytes)")

            # Check permissions
            if not self.check_binary_permissions(str(binary_path)):
                all_passed = False

            # Check security (test function exclusion)
            if not self.check_binary_security(str(binary_path)):
                all_passed = False

            # Additional checks for runner binary (should have setuid)
            if binary_name == "runner":
                mode = file_stat.st_mode
                if not (mode & stat.S_ISUID):
                    self.print_warning("Runner binary does not have setuid bit (this is expected in CI)")
                else:
                    self.print_success("Runner binary has setuid bit set")

        if all_passed:
            self.print_success("=== All production binaries passed validation ===")
        else:
            self.print_error("=== Some production binaries failed validation ===")

        return all_passed

    def run_build_security_check(self) -> bool:
        """Run comprehensive build and security check."""
        self.print_info("=== Build Security Check ===")

        success = True

        # Step 1: Check build environment
        self.print_info("\n--- Step 1: Build Environment ---")
        if not self.check_build_environment():
            success = False

        # Step 2: Check Go modules
        self.print_info("\n--- Step 2: Go Modules Verification ---")
        try:
            self.run_command(['go', 'mod', 'tidy'])
            # Check if go mod tidy changed anything
            result = self.run_command(['git', 'diff', '--name-only'], capture_output=True, check=False)
            if result.stdout.strip():
                self.print_error("go mod tidy resulted in changes. Please commit the changes first.")
                success = False
            else:
                self.print_success("Go modules are up to date")
        except subprocess.CalledProcessError as e:
            self.print_error(f"Go modules check failed: {e}")
            success = False

        # Step 3: Build tags compliance
        self.print_info("\n--- Step 3: Build Tags ---")
        if not self.check_build_tags():
            success = False

        # Step 4: Forbidden patterns
        self.print_info("\n--- Step 4: Code Patterns ---")
        if not self.check_forbidden_patterns():
            success = False

        # Step 5: Production binary validation (if binaries exist)
        self.print_info("\n--- Step 5: Production Binaries ---")
        if Path("build/prod").exists() and any(Path(f"build/prod/{b}").exists() for b in ["record", "verify", "runner"]):
            if not self.validate_production_binaries():
                success = False
        else:
            self.print_info("No production binaries found - skipping binary validation")

        # Summary
        self.print_info("\n=== Build Security Check Summary ===")
        if success:
            self.print_success("All build security checks passed")
        else:
            self.print_error("Some build security checks failed")

        return success
