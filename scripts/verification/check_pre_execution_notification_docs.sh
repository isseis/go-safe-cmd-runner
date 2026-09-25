#!/bin/sh
# Verify that the group pre-execution failure notification is documented.
#
# Each error_type constant must be declared with its documented string value in
# internal/logging/pre_execution_error.go, and each value must be named in both
# the Japanese and the English runner_command guide. The new section itself must
# be present in both guides too (a plain pre_execution_error search would
# already pass on the older message-type table). This pins the documented
# values to the code; whether the prose describing each value is correct is
# left to human review (Task 0176 AC-20).
#
# This script is POSIX sh (no bash-only syntax). The Makefile target
# verify-docs-checks enumerates scripts/verification/check_*.sh and runs each
# with `sh "$script"`, so the executable bit is not required.

set -u

SCRIPT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"
PROJECT_ROOT="$(CDPATH= cd -- "$SCRIPT_DIR/../.." && pwd)"

status=0

# check_word <file> <word> <description>
check_word() {
    file="$1"
    word="$2"
    description="$3"

    if [ ! -f "$file" ]; then
        echo "FAIL: $file is missing (required for $description)"
        status=1
        return 0
    fi

    if ! grep -F -e "$word" "$file" >/dev/null 2>&1; then
        echo "FAIL: $file does not contain '$word' ($description)"
        status=1
        return 0
    fi

    echo "OK: $file contains '$word'"
    return 0
}

ERRORS_GO="$PROJECT_ROOT/internal/logging/pre_execution_error.go"
RUNNER_JA="$PROJECT_ROOT/docs/user/runner_command.ja.md"
RUNNER_EN="$PROJECT_ROOT/docs/user/runner_command.md"

# check_declaration <constant> <value> <description>
# The constant must be declared with exactly this string value, so swapping two
# constants' values cannot pass while both names and both values still exist.
check_declaration() {
    constant="$1"
    value="$2"
    description="$3"

    if [ ! -f "$ERRORS_GO" ]; then
        echo "FAIL: $ERRORS_GO is missing (required for $description)"
        status=1
        return 0
    fi

    pattern="${constant}[[:space:]]+ErrorType[[:space:]]*=[[:space:]]*\"${value}\""
    if ! grep -E -e "$pattern" "$ERRORS_GO" >/dev/null 2>&1; then
        echo "FAIL: $ERRORS_GO does not declare '$constant' as \"$value\" ($description)"
        status=1
        return 0
    fi

    echo "OK: $ERRORS_GO declares '$constant' as \"$value\""
    return 0
}

# check_error_type <constant> <value> <description>
check_error_type() {
    constant="$1"
    value="$2"
    description="$3"

    check_declaration "$constant" "$value" "$description constant (Task 0176)"
    check_word "$RUNNER_JA" "$value" "$description in the Japanese guide (Task 0176)"
    check_word "$RUNNER_EN" "$value" "$description in the English guide (Task 0176)"
}

check_error_type "ErrorTypeGroupPreparation" "group_preparation_failed" "group preparation failure"
check_error_type "ErrorTypeGroupDirPermissionViolation" "group_dir_permission_violation" "group directory permission violation"
check_error_type "ErrorTypeCommandVerification" "command_verification_failed" "command verification failure"
check_error_type "ErrorTypeGroupPreExecution" "group_pre_execution_failed" "generic group pre-execution failure"
# The new section points at the pre-existing value for group file verification,
# so it must stay declared and documented as well.
check_error_type "ErrorTypeGroupFileVerification" "group_file_verification_failed" "existing group file verification failure"

# The new section must exist in both guides (the values above could otherwise be
# documented in an unrelated section). The notification type is held to the same
# text.
check_word "$RUNNER_JA" "group 実行前段の失敗の通知" "the new group pre-execution section in the Japanese guide (Task 0176)"
check_word "$RUNNER_EN" "Group Pre-Execution Stage Failure Notification" "the new group pre-execution section in the English guide (Task 0176)"
check_word "$RUNNER_JA" "pre_execution_error" "the notification type in the Japanese guide (Task 0176)"
check_word "$RUNNER_EN" "pre_execution_error" "the notification type in the English guide (Task 0176)"

if [ "$status" -eq 0 ]; then
    echo "All pre-execution notification documentation checks passed"
else
    echo "Pre-execution notification documentation checks failed"
fi

exit "$status"
