#!/bin/sh
# Build and run the executor integration tests under the real setuid model.
set -eu

if [ "$#" -ne 0 ]; then
	echo "usage: TEST_RUNAS_TARGET_USER=<user> $0" >&2
	exit 2
fi

target_user=${TEST_RUNAS_TARGET_USER:-}
if [ -z "$target_user" ]; then
	echo "FATAL: TEST_RUNAS_TARGET_USER must name a non-root fixture user" >&2
	exit 1
fi
if ! target_uid=$(id -u "$target_user" 2>/dev/null); then
	echo "FATAL: target user $target_user does not exist" >&2
	exit 1
fi
if [ "$target_uid" -eq 0 ]; then
	echo "FATAL: target user must be non-root; target=$target_user uid=$target_uid" >&2
	exit 1
fi

invoker_uid=$(id -u)
if [ "$invoker_uid" -eq 0 ]; then
	echo "FATAL: setuid test must be launched by a non-root user; ruid=$invoker_uid" >&2
	exit 1
fi
if [ "$target_uid" -eq "$invoker_uid" ]; then
	echo "FATAL: target uid must differ from invoker uid; uid=$invoker_uid" >&2
	exit 1
fi

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
project_root=$(CDPATH= cd -- "$script_dir/../.." && pwd)
run_parent=${TMPDIR:-/var/tmp}
run_dir=$(mktemp -d "$run_parent/scr-setuid.XXXXXX")
binary="$run_dir/executor.test"
output="$run_dir/output.txt"

cleanup() {
	if [ -n "${run_dir:-}" ] && [ -d "$run_dir" ]; then
		sudo rm -rf -- "$run_dir"
	fi
}
trap cleanup EXIT HUP INT TERM

if ! command -v findmnt >/dev/null 2>&1; then
	echo "FATAL: findmnt is required to verify that $run_dir is not on a nosuid mount" >&2
	exit 1
fi
if findmnt -no OPTIONS -T "$run_dir" | tr ',' '\n' | grep -qx nosuid; then
	echo "FATAL: $run_dir is on a nosuid mount; the setuid model cannot be exercised" >&2
	exit 1
fi

cd "$project_root"
go test -tags "test integration" -c -o "$binary" ./internal/runner/base/executor/
sudo chown root:root "$binary"
sudo chmod 4755 "$binary"

owner_uid=$(stat -c %u "$binary")
mode=$(stat -c %a "$binary")
if [ "$owner_uid" -ne 0 ] || [ "$mode" != 4755 ]; then
	echo "FATAL: invalid setuid binary metadata; owner_uid=$owner_uid mode=$mode path=$binary" >&2
	exit 1
fi

test_filter='^(TestPrivilegeGap_.*|TestRunAsSupplementaryGroups_MatchTargetUser_NotRoot)$'
if ! env -i 	PATH=/usr/bin:/bin 	LANG=C 	TEST_RUNAS_TARGET_USER="$target_user" 	"$binary" -test.v -test.run "$test_filter" >"$output" 2>&1; then
	cat "$output"
	echo "FATAL: setuid executor test binary exited non-zero" >&2
	exit 1
fi
cat "$output"

pass_count=$(grep -c -- '^--- PASS:' "$output" || true)
skip_count=$(grep -c -- '^--- SKIP:' "$output" || true)
echo "setuid executor integration summary: PASS=$pass_count SKIP=$skip_count"

if [ "$skip_count" -ne 0 ]; then
	echo "FATAL: privileged tests skipped; privileged behavior was not verified" >&2
	exit 1
fi

for test_name in 	TestPrivilegeGap_ChildCredentialsMatchTarget 	TestPrivilegeGap_VerifiedFDExecutionUsesTargetCredentials 	TestPrivilegeGap_StagingCleanupUsesRealPrivileges 	TestPrivilegeGap_StagingCancellationCleansUp 	TestPrivilegeGap_StartWindowIndependentOfCommandDuration 	TestPrivilegeGap_TimeoutKillsChild 	TestPrivilegeGap_CancelKillsChild 	TestPrivilegeGap_OutputLimitAbortsRunningChild 	TestPrivilegeGap_RefusedElevationDoesNotRecordWindow 	TestRunAsSupplementaryGroups_MatchTargetUser_NotRoot
do
	if ! grep -q -- "^--- PASS: $test_name " "$output"; then
		echo "FATAL: required test did not pass: $test_name" >&2
		exit 1
	fi
done

echo "OK: all required privileged executor tests passed under env -i"
