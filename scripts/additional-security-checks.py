#!/usr/bin/env python3
"""
additional-security-checks.py
Supplementary security validation script for go-safe-cmd-runner
This script provides additional security checks beyond golangci-lint forbidigo

This is a Python port of the original bash script.
"""

import sys
import argparse
import os

# Add the scripts directory to the Python path if not already there
script_dir = os.path.dirname(os.path.abspath(__file__))
if script_dir not in sys.path:
    sys.path.insert(0, script_dir)

from security_checker import SecurityChecker


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
