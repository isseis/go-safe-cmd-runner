#!/bin/sh
# Self-test for check_structured_message_redaction_docs.sh.
#
# A fixture that contains every expected word must pass; removing a single
# word from one file must fail. This pins that the check is not a no-op that
# always exits 0, so a silently disabled check cannot pass the docs gate.
#
# This script is POSIX sh (no bash-only syntax), like the script it exercises.
# The Makefile target verify-docs-checks enumerates scripts/verification/
# check_*.sh and runs each with `sh "$script"`, so this self-test runs there too.

set -u

SCRIPT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"
CHECK="$SCRIPT_DIR/check_structured_message_redaction_docs.sh"

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' 0 HUP INT TERM

ARCH_JA="$TMP/docs/dev/architecture_design/security-architecture.ja.md"
ARCH_EN="$TMP/docs/dev/architecture_design/security-architecture.md"
RISK_JA="$TMP/docs/user/security-risk-assessment.ja.md"
RISK_EN="$TMP/docs/user/security-risk-assessment.md"
PKG="$TMP/docs/dev/developer_guide/package_reference.md"

mkdir -p "$TMP/docs/dev/architecture_design" "$TMP/docs/dev/developer_guide" "$TMP/docs/user"

# write_complete_fixture lays down files that contain every expected word.
write_complete_fixture() {
    cat > "$ARCH_JA" <<'EOF'
構造化メッセージ
役割
値全体置換
対象外
値全体置換は適用しません
Path
Text
構造を持たない
EOF
    cat > "$ARCH_EN" <<'EOF'
structured message
role
whole-value replacement
Path
Text
no structure
whole-value replacement is not applied
EOF
    cat > "$RISK_JA" <<'EOF'
識別子
免除
error_message
構造化メッセージ
Identifier
値全体置換を受けずに
EOF
    cat > "$RISK_EN" <<'EOF'
identifier
exempt
error_message
structured message
Identifier
without whole-value replacement
EOF
    cat > "$PKG" <<'EOF'
errmsg/
internal/errmsg
EOF
}

status=0

write_complete_fixture
if STRUCTURED_MSG_DOCS_ROOT="$TMP" sh "$CHECK" >/dev/null 2>&1; then
    echo "OK: the complete fixture passes"
else
    echo "FAIL: the complete fixture must pass"
    STRUCTURED_MSG_DOCS_ROOT="$TMP" sh "$CHECK" || true
    status=1
fi

# Drop one expected word from one file; the check must notice.
cat > "$ARCH_JA" <<'EOF'
役割
値全体置換
対象外
Path
Text
構造を持たない
EOF
if STRUCTURED_MSG_DOCS_ROOT="$TMP" sh "$CHECK" >/dev/null 2>&1; then
    echo "FAIL: a missing word must fail the check"
    status=1
else
    echo "OK: a missing word fails the check"
fi

# Put the old term back; the check must notice that too.
write_complete_fixture
printf '%s\n' '値まるごと判定' >> "$ARCH_JA"
if STRUCTURED_MSG_DOCS_ROOT="$TMP" sh "$CHECK" >/dev/null 2>&1; then
    echo "FAIL: the old term must fail the check"
    status=1
else
    echo "OK: the old term fails the check"
fi

if [ "$status" -eq 0 ]; then
    echo "Structured-message redaction documentation self-test passed"
else
    echo "Structured-message redaction documentation self-test failed"
fi

exit "$status"
