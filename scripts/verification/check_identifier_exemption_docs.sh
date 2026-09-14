#!/bin/sh
# Verify that the identifier exemption is documented.
#
# Each file listed below must contain all of its required words. Every word is
# checked independently so that a single missing word fails the script with a
# message naming the file and the word. "exempt" is matched as a stem, so
# "exemption" and "exempted" also satisfy it.
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

ARCH_JA="$PROJECT_ROOT/docs/dev/architecture_design/security-architecture.ja.md"
ARCH_EN="$PROJECT_ROOT/docs/dev/architecture_design/security-architecture.md"
RISK_JA="$PROJECT_ROOT/docs/user/security-risk-assessment.ja.md"
RISK_EN="$PROJECT_ROOT/docs/user/security-risk-assessment.md"
TASK_ARCH="$PROJECT_ROOT/docs/tasks/0173_identifier_redaction_exemption/02_architecture.md"
TASK_REQ="$PROJECT_ROOT/docs/tasks/0173_identifier_redaction_exemption/01_requirements.md"

check_word "$ARCH_JA" "識別子" "the identifier exemption in the redaction layer (AC-15)"
check_word "$ARCH_JA" "免除" "the identifier exemption in the redaction layer (AC-15)"
check_word "$ARCH_JA" "NewIdentifier" "the declared constructor (AC-15)"

check_word "$ARCH_EN" "identifier" "the identifier exemption in the redaction layer (AC-15)"
check_word "$ARCH_EN" "exempt" "the identifier exemption in the redaction layer (AC-15)"
check_word "$ARCH_EN" "NewIdentifier" "the declared constructor (AC-15)"

check_word "$RISK_JA" "識別子" "the identifier exemption in the limitations (AC-14, AC-16)"
check_word "$RISK_JA" "免除" "the identifier exemption in the limitations (AC-14, AC-16)"

check_word "$RISK_EN" "identifier" "the identifier exemption in the limitations (AC-14, AC-16)"
check_word "$RISK_EN" "exempt" "the identifier exemption in the limitations (AC-14, AC-16)"

check_word "$TASK_ARCH" "残余リスク" "the residual risk record (AC-13)"
check_word "$TASK_ARCH" "record.Message" "the residual risk record (AC-13)"

check_word "$TASK_REQ" "0172" "the reference replacing the Task 0172 residual risk (AC-17)"
check_word "$TASK_REQ" "置き換え" "the reference replacing the Task 0172 residual risk (AC-17)"

if [ "$status" -eq 0 ]; then
    echo "All identifier exemption documentation checks passed"
else
    echo "Identifier exemption documentation checks failed"
fi

exit "$status"
