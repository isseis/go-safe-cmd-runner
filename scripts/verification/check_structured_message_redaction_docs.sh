#!/bin/sh
# Verify that the structured-message redaction boundary is documented.
#
# The two records' error_message is carried as a structured message whose parts
# declare their redaction role by type. The documentation must describe the
# role-based redaction, the Path boundary, the fail-closed treatment of
# unstructured errors, and the extended identifier exemption, in both the
# Japanese and the English security documents. It must not keep the old term
# for the value-based layer (glossary: whole-value replacement).
#
# Each expected word is checked independently, so one missing word fails with a
# message naming the file and the word. Set STRUCTURED_MSG_DOCS_ROOT to point
# the checks at another tree (the self-test uses this); the default is the
# repository root.
#
# This script is POSIX sh (no bash-only syntax). The Makefile target
# verify-docs-checks enumerates scripts/verification/check_*.sh and runs each
# with `sh "$script"`, so the executable bit is not required.

set -u

SCRIPT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"
PROJECT_ROOT="${STRUCTURED_MSG_DOCS_ROOT:-$(CDPATH= cd -- "$SCRIPT_DIR/../.." && pwd)}"

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

# check_absent <file> <word> <description>
check_absent() {
    file="$1"
    word="$2"
    description="$3"

    if [ ! -f "$file" ]; then
        echo "FAIL: $file is missing (required for $description)"
        status=1
        return 0
    fi

    if grep -F -e "$word" "$file" >/dev/null 2>&1; then
        echo "FAIL: $file still contains the old term '$word' ($description)"
        status=1
        return 0
    fi

    echo "OK: $file does not contain '$word'"
    return 0
}

ARCH_JA="$PROJECT_ROOT/docs/dev/architecture_design/security-architecture.ja.md"
ARCH_EN="$PROJECT_ROOT/docs/dev/architecture_design/security-architecture.md"
RISK_JA="$PROJECT_ROOT/docs/user/security-risk-assessment.ja.md"
RISK_EN="$PROJECT_ROOT/docs/user/security-risk-assessment.md"
PKG="$PROJECT_ROOT/docs/dev/developer_guide/package_reference.md"

# Architecture (Japanese): roles, the Path boundary and the fail-closed Text
# treatment.
check_word "$ARCH_JA" "構造化メッセージ" "the structured message design (AC-26)"
check_word "$ARCH_JA" "役割" "the per-role redaction (AC-26)"
check_word "$ARCH_JA" "値全体置換" "the whole-value replacement layer (AC-26)"
check_word "$ARCH_JA" "対象外" "the Path boundary outside whole-value replacement (AC-26)"
check_word "$ARCH_JA" "Path" "the Path role (AC-26)"
check_word "$ARCH_JA" "Text" "the fail-closed Text role (AC-26)"
check_word "$ARCH_JA" "構造を持たない" "the fail-closed treatment of unstructured errors (AC-26)"
check_word "$ARCH_JA" "値全体置換は適用しません" "the Path boundary sentence (AC-26)"

# Architecture (English) mirrors the Japanese content.
check_word "$ARCH_EN" "structured message" "the structured message design (AC-26)"
check_word "$ARCH_EN" "role" "the per-role redaction (AC-26)"
check_word "$ARCH_EN" "whole-value replacement" "the whole-value replacement layer (AC-26)"
check_word "$ARCH_EN" "Path" "the Path role (AC-26)"
check_word "$ARCH_EN" "Text" "the fail-closed Text role (AC-26)"
check_word "$ARCH_EN" "no structure" "the fail-closed treatment of unstructured errors (AC-26)"
check_word "$ARCH_EN" "whole-value replacement is not applied" "the Path boundary sentence (AC-26)"

# The old term for the value-based layer must not come back.
check_absent "$ARCH_JA" "値まるごと判定" "the renamed value-based layer (AC-26)"
check_absent "$ARCH_EN" "whole-value detection" "the renamed value-based layer (AC-26)"

# Risk assessment (Japanese): the exemption must not read as if the structured
# error_message were not exempt.
check_word "$RISK_JA" "識別子" "the identifier exemption (AC-26)"
check_word "$RISK_JA" "免除" "the identifier exemption (AC-26)"
check_word "$RISK_JA" "error_message" "the structured error_message boundary (AC-26)"
check_word "$RISK_JA" "構造化メッセージ" "the structured error_message boundary (AC-26)"
check_word "$RISK_JA" "Identifier" "the declared identifier part (AC-26)"
check_word "$RISK_JA" "値全体置換を受けずに" "the structured error_message exemption statement (AC-26)"

# Risk assessment (English) mirrors the Japanese content.
check_word "$RISK_EN" "identifier" "the identifier exemption (AC-26)"
check_word "$RISK_EN" "exempt" "the identifier exemption (AC-26)"
check_word "$RISK_EN" "error_message" "the structured error_message boundary (AC-26)"
check_word "$RISK_EN" "structured message" "the structured error_message boundary (AC-26)"
check_word "$RISK_EN" "Identifier" "the declared identifier part (AC-26)"
check_word "$RISK_EN" "without whole-value replacement" "the structured error_message exemption statement (AC-26)"

# Package reference: the new leaf package and its responsibility.
check_word "$PKG" "errmsg/" "the internal/errmsg package listing (AC-26)"
check_word "$PKG" "internal/errmsg" "the internal/errmsg responsibility (AC-26)"

if [ "$status" -eq 0 ]; then
    echo "All structured-message redaction documentation checks passed"
else
    echo "Structured-message redaction documentation checks failed"
fi

exit "$status"
