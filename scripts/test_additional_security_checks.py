#!/usr/bin/env python3
"""
test_additional_security_checks.py
Unit tests for additional-security-checks.py
"""

import unittest
import tempfile
import os
import subprocess
from unittest.mock import patch, MagicMock, mock_open
from pathlib import Path

# Import the security checker classes directly from the module
from security_checker import SecurityChecker, Colors


class TestSecurityChecker(unittest.TestCase):
    """Test cases for SecurityChecker class."""

    def setUp(self):
        """Set up test fixtures."""
        self.checker = SecurityChecker()

    def test_init(self):
        """Test SecurityChecker initialization."""
        self.assertEqual(self.checker.exit_code, 0)

    def test_print_methods(self):
        """Test print methods don't raise exceptions."""
        # These methods should not raise exceptions
        self.checker.print_error("test error")
        self.checker.print_success("test success")
        self.checker.print_warning("test warning")
        self.checker.print_info("test info")
        self.checker.print_status(Colors.RED, "test status")

    def test_run_command_success(self):
        """Test run_command with successful command."""
        result = self.checker.run_command(['echo', 'hello'])
        self.assertEqual(result.returncode, 0)
        self.assertEqual(result.stdout.strip(), 'hello')

    def test_run_command_failure_with_check_false(self):
        """Test run_command with failing command and check=False."""
        result = self.checker.run_command(['false'], check=False)
        self.assertNotEqual(result.returncode, 0)

    def test_run_command_failure_with_check_true(self):
        """Test run_command with failing command and check=True raises exception."""
        with self.assertRaises(subprocess.CalledProcessError):
            self.checker.run_command(['false'], check=True)

    def test_extract_strings_from_binary_with_temp_file(self):
        """Test extract_strings_from_binary with a temporary binary file."""
        # Create a temporary file with some binary data and strings
        with tempfile.NamedTemporaryFile(delete=False) as temp_file:
            # Write some binary data with embedded strings
            binary_data = b'\x00\x01\x02hello\x00world\x03\x04test123\xff\xfe'
            temp_file.write(binary_data)
            temp_file.flush()

            try:
                strings = self.checker.extract_strings_from_binary(temp_file.name)

                # Check that we extracted the expected strings
                self.assertIn('hello', strings)
                self.assertIn('world', strings)
                self.assertIn('test123', strings)

            finally:
                os.unlink(temp_file.name)

    def test_extract_strings_from_binary_nonexistent_file(self):
        """Test extract_strings_from_binary with non-existent file."""
        strings = self.checker.extract_strings_from_binary('/nonexistent/file')
        self.assertEqual(strings, [])

    def test_extract_strings_from_binary_min_length(self):
        """Test extract_strings_from_binary respects minimum length."""
        with tempfile.NamedTemporaryFile(delete=False) as temp_file:
            # Write data with short and long strings
            binary_data = b'\x00ab\x00\x01hello\x00x\x02world123\xff'
            temp_file.write(binary_data)
            temp_file.flush()

            try:
                # Test with default min_length (4)
                strings = self.checker.extract_strings_from_binary(temp_file.name)
                self.assertIn('hello', strings)
                self.assertIn('world123', strings)
                self.assertNotIn('ab', strings)  # Too short
                self.assertNotIn('x', strings)   # Too short

                # Test with min_length of 2
                strings_short = self.checker.extract_strings_from_binary(temp_file.name, min_length=2)
                self.assertIn('ab', strings_short)

            finally:
                os.unlink(temp_file.name)

    def test_check_binary_security_file_not_found(self):
        """Test check_binary_security with non-existent file."""
        result = self.checker.check_binary_security('/nonexistent/binary')
        self.assertFalse(result)

    def test_check_binary_security_with_test_artifacts(self):
        """Test check_binary_security detects test artifacts."""
        with tempfile.NamedTemporaryFile(delete=False) as temp_file:
            # Create binary with test artifacts
            binary_data = b'\x00\x01NewManagerForTest\x00some other data\x02testing.T\xff'
            temp_file.write(binary_data)
            temp_file.flush()

            try:
                result = self.checker.check_binary_security(temp_file.name)
                self.assertFalse(result)  # Should fail because test artifacts found

            finally:
                os.unlink(temp_file.name)

    def test_check_binary_security_clean_binary(self):
        """Test check_binary_security with clean binary."""
        with tempfile.NamedTemporaryFile(delete=False) as temp_file:
            # Create binary without test artifacts
            binary_data = b'\x00\x01production code\x00regular function\x02normal data\xff'
            temp_file.write(binary_data)
            temp_file.flush()

            try:
                result = self.checker.check_binary_security(temp_file.name)
                self.assertTrue(result)  # Should pass because no test artifacts

            finally:
                os.unlink(temp_file.name)

    def test_check_binary_security_go_runtime_internals_ignored(self):
        """Test that Go runtime internals are not flagged as debug symbols."""
        with tempfile.NamedTemporaryFile(delete=False) as temp_file:
            # Create binary with Go runtime strings that should be ignored
            binary_data = (
                b'\x00\x01runtime.CallersFrames\x00'
                b'/home/issei/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.24.6.linux-arm64/src/runtime/synctest.go\x00'
                b'synctest\x00writeString\x00WriteString\x00'
                b'production code\x02normal data\xff'
            )
            temp_file.write(binary_data)
            temp_file.flush()

            try:
                result = self.checker.check_binary_security(temp_file.name)
                self.assertTrue(result)  # Should pass because these are Go runtime internals

            finally:
                os.unlink(temp_file.name)

    def test_check_binary_security_user_debug_symbols_detected(self):
        """Test that user debug symbols are properly detected."""
        with tempfile.NamedTemporaryFile(delete=False) as temp_file:
            # Create binary with actual debug symbols that should be flagged
            binary_data = (
                b'\x00\x01testing.T\x00'
                b'debug.Stack\x00'
                b'TestMain\x00'
                b'/home/user/project/test.go\x00'  # User test file, not toolchain
                b'production code\xff'
            )
            temp_file.write(binary_data)
            temp_file.flush()

            try:
                result = self.checker.check_binary_security(temp_file.name)
                self.assertFalse(result)  # Should fail because user debug symbols found

            finally:
                os.unlink(temp_file.name)

    def test_check_binary_security_mixed_runtime_and_user_paths(self):
        """Test binary with mix of runtime and user paths - only user paths should be flagged."""
        with tempfile.NamedTemporaryFile(delete=False) as temp_file:
            # Create binary with both runtime paths (ignored) and user paths (flagged)
            binary_data = (
                b'\x00\x01'
                b'/go/pkg/mod/golang.org/toolchain@v0.0.1/src/runtime/test.go\x00'  # Should be ignored
                b'/home/user/myproject/helper_testing.go\x00'  # Should be flagged
                b'runtime.CallersFrames\x00'  # Runtime internal
                b'production code\xff'
            )
            temp_file.write(binary_data)
            temp_file.flush()

            try:
                result = self.checker.check_binary_security(temp_file.name)
                self.assertFalse(result)  # Should fail because user test file found

            finally:
                os.unlink(temp_file.name)

    @patch.object(SecurityChecker, 'run_command')
    def test_check_build_environment_go_not_found(self, mock_run_command):
        """Test check_build_environment when go command fails."""
        mock_run_command.side_effect = subprocess.CalledProcessError(1, ['go', 'version'])

        result = self.checker.check_build_environment()
        self.assertFalse(result)

    @patch('pathlib.Path.is_file')
    @patch.object(SecurityChecker, 'run_command')
    def test_check_build_environment_no_go_mod(self, mock_run_command, mock_is_file):
        """Test check_build_environment when go.mod is missing."""
        # Mock successful go version
        mock_process = MagicMock()
        mock_process.returncode = 0
        mock_process.stdout = "go version go1.21.0 linux/amd64"
        mock_run_command.return_value = mock_process

        # Mock go.mod not found
        mock_is_file.return_value = False

        result = self.checker.check_build_environment()
        self.assertFalse(result)

    def test_check_binary_permissions_file_not_found(self):
        """Test check_binary_permissions with non-existent file."""
        result = self.checker.check_binary_permissions('/nonexistent/binary')
        self.assertTrue(result)  # Should return True if file doesn't exist

    def test_check_binary_permissions_with_temp_file(self):
        """Test check_binary_permissions with actual file."""
        with tempfile.NamedTemporaryFile(delete=False) as temp_file:
            try:
                # Make file executable
                os.chmod(temp_file.name, 0o755)

                result = self.checker.check_binary_permissions(temp_file.name)
                self.assertTrue(result)

            finally:
                os.unlink(temp_file.name)

    @patch('pathlib.Path.rglob')
    def test_check_build_tags_clean_file(self, mock_rglob):
        """Test check_build_tags with files that don't need test tags."""
        # Mock Path.rglob to return a regular go file
        mock_file = MagicMock()
        mock_file.name = 'main.go'
        mock_file.parts = ('src', 'main.go')
        mock_rglob.return_value = [mock_file]

        result = self.checker.check_build_tags()
        self.assertTrue(result)

    @patch('builtins.open', mock_open(read_data='package main\n\nfunc NewManagerForTest() {}'))
    @patch('pathlib.Path.rglob')
    def test_check_build_tags_missing_constraint(self, mock_rglob):
        """Test check_build_tags with testing file missing build constraint."""
        # Mock Path.rglob to return a testing go file
        mock_file = MagicMock()
        mock_file.name = 'manager_testing.go'
        mock_file.parts = ('src', 'manager_testing.go')
        mock_rglob.return_value = [mock_file]

        result = self.checker.check_build_tags()
        self.assertFalse(result)

    @patch('pathlib.Path.rglob')
    def test_check_forbidden_patterns_clean_code(self, mock_rglob):
        """Test check_forbidden_patterns with clean code."""
        # Mock Path.rglob to return empty list
        mock_rglob.return_value = []

        result = self.checker.check_forbidden_patterns()
        self.assertTrue(result)


class TestColors(unittest.TestCase):
    """Test cases for Colors class."""

    def test_color_constants(self):
        """Test that color constants are defined."""
        self.assertTrue(hasattr(Colors, 'RED'))
        self.assertTrue(hasattr(Colors, 'GREEN'))
        self.assertTrue(hasattr(Colors, 'YELLOW'))
        self.assertTrue(hasattr(Colors, 'NC'))

        # Test that they are strings
        self.assertIsInstance(Colors.RED, str)
        self.assertIsInstance(Colors.GREEN, str)
        self.assertIsInstance(Colors.YELLOW, str)
        self.assertIsInstance(Colors.NC, str)


if __name__ == '__main__':
    # Run the tests
    unittest.main(verbosity=2)
