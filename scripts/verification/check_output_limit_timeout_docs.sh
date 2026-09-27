#!/bin/sh
# Verify that the output_size_limit and timeout behavior is documented.
#
# Both the Japanese and the English global-level configuration guide must state
# the facts below. Each fact is matched by a fixed anchor string that only a
# sentence carrying that fact contains, so deleting the sentence fails this
# script even if the topic is mentioned elsewhere:
#   (a) output_size_limit = 0 means unlimited
#   (b) negative output_size_limit values are rejected while loading
#   (c) subsequent groups run after a command times out
#   (d) a timed-out command's process (including grandchildren) may remain
#   (e) retained output is bounded to a leading window, and an output file is
#       needed for the full output
#
# The factual content is pinned by the tests named in the implementation plan
# (TestCapture_WriteOutput, TestRunner_ZeroOutputSizeLimitIntegration,
# TestValidateOutputSizeLimits, TestRunner_CommandTimeoutNotifiesSubsequentGroups,
# TestBoundedBuffer_KeepsCompletePrefixLines, TestExecute_OutputWriterReceivesAllBytes)
# and, for fact (d), by the accepted limitation in 02_architecture.md §4.4.
# Whether the prose around each anchor is correct is left to human review
# (Task 0177 AC-26, AC-30, AC-31).
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

GLOBAL_JA="$PROJECT_ROOT/docs/user/toml_config/04_global_level.ja.md"
GLOBAL_EN="$PROJECT_ROOT/docs/user/toml_config/04_global_level.md"

# (a) 0 means unlimited.
check_word "$GLOBAL_JA" '`output_size_limit = 0` は**無制限**を表します' "0 means unlimited in the Japanese guide (AC-26)"
check_word "$GLOBAL_EN" '`output_size_limit = 0` means **unlimited**' "0 means unlimited in the English guide (AC-26)"

# (b) Negative values are rejected at load time.
check_word "$GLOBAL_JA" '負の値は設定の読み込み時に拒否され' "negative values rejected at load time in the Japanese guide (AC-26, AC-27)"
check_word "$GLOBAL_EN" 'Negative values are rejected when the configuration is loaded' "negative values rejected at load time in the English guide (AC-26, AC-27)"

# (c) Subsequent groups run after a command timeout.
check_word "$GLOBAL_JA" '後続の group は実行されます' "subsequent groups run after a command timeout in the Japanese guide (AC-30)"
check_word "$GLOBAL_EN" 'subsequent groups are executed' "subsequent groups run after a command timeout in the English guide (AC-30)"

# (d) A timed-out command's process (including grandchildren) may remain.
check_word "$GLOBAL_JA" 'タイムアウトしたコマンドのプロセスは、そのプロセスが起動した孫プロセスを含め、残り続けることがあります' "leftover timed-out process in the Japanese guide (AC-30)"
check_word "$GLOBAL_EN" 'The process of a timed-out command may remain, including any grandchildren it started' "leftover timed-out process in the English guide (AC-30)"

# (e) Retained output is bounded to a leading window, and the full output needs
# an output file.
check_word "$GLOBAL_JA" '先頭の 64 KiB（先頭の窓）だけが保持されます' "bounded retained output in the Japanese guide (AC-28, AC-31)"
check_word "$GLOBAL_EN" 'up to the leading 64 KiB (the leading window)' "bounded retained output in the English guide (AC-28, AC-31)"
check_word "$GLOBAL_JA" 'コマンドに出力ファイルを指定してください' "output file for the full output in the Japanese guide (AC-31)"
check_word "$GLOBAL_EN" 'specify an output file for the command' "output file for the full output in the English guide (AC-31)"

if [ "$status" -eq 0 ]; then
    echo "All output_size_limit and timeout documentation checks passed"
else
    echo "output_size_limit and timeout documentation checks failed"
fi

exit "$status"
