#!/bin/sh
# Build and run the executor integration tests under the real setuid model.
set -eu

if [ "$#" -ne 0 ]; then
	echo "usage: TEST_RUNAS_TARGET_USER=<user> $0" >&2
	exit 2
fi
if [ "$(uname -s)" != Linux ]; then
	echo "FATAL: executor setuid integration is supported only on Linux" >&2
	exit 1
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
# The binary is briefly root-setuid, so only the invoking user may reach it.
# chmod also removes effective access granted by a default ACL on run_parent.
chmod 0700 "$run_dir"
binary="$run_dir/executor.test"
output="$run_dir/output.txt"
entry_ready="$run_dir/entry-ready"
test_pid=
: >"$output"
: >"$entry_ready"

cleanup() {
	if [ -n "${test_pid:-}" ] && kill -0 "$test_pid" 2>/dev/null; then
		kill "$test_pid" 2>/dev/null || true
		wait "$test_pid" 2>/dev/null || true
	fi
	if [ -n "${binary:-}" ] && [ -e "$binary" ]; then
		sudo chmod 0755 "$binary" 2>/dev/null || true
	fi
	if [ -n "${run_dir:-}" ] && [ -d "$run_dir" ]; then
		sudo rm -rf -- "$run_dir"
	fi
}
trap cleanup EXIT
trap "exit 130" HUP INT TERM

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

dir_owner_uid=$(stat -c %u "$run_dir")
dir_mode=$(stat -c %a "$run_dir")
owner_uid=$(stat -c %u "$binary")
mode=$(stat -c %a "$binary")
if [ "$dir_owner_uid" -ne "$invoker_uid" ] || [ "$dir_mode" != 700 ]; then
	echo "FATAL: invalid artifact directory metadata; owner_uid=$dir_owner_uid mode=$dir_mode path=$run_dir" >&2
	exit 1
fi
if [ "$owner_uid" -ne 0 ] || [ "$mode" != 4755 ]; then
	echo "FATAL: invalid setuid binary metadata; owner_uid=$owner_uid mode=$mode path=$binary" >&2
	exit 1
fi

test_filter='^(TestPrivilegeGap_.*|TestRunAsSupplementaryGroups_MatchTargetUser_NotRoot)$'
env -i \
	PATH=/usr/bin:/bin \
	LANG=C \
	TEST_RUNAS_TARGET_USER="$target_user" \
	TEST_SETUID_ENTRY_READY_FILE="$entry_ready" \
	"$binary" -test.v -test.timeout=60s -test.run "$test_filter" >"$output" 2>&1 &
test_pid=$!

wait_count=0
while [ ! -s "$entry_ready" ]; do
	if ! kill -0 "$test_pid" 2>/dev/null; then
		break
	fi
	wait_count=$((wait_count + 1))
	if [ "$wait_count" -ge 500 ]; then
		break
	fi
	sleep 0.01
done
if [ ! -s "$entry_ready" ]; then
	kill "$test_pid" 2>/dev/null || true
	wait "$test_pid" 2>/dev/null || true
	cat "$output"
	echo "FATAL: setuid executor test binary did not confirm privileged entry" >&2
	exit 1
fi

sudo chmod 0755 "$binary"
mode=$(stat -c %a "$binary")
if [ "$mode" != 755 ]; then
	echo "FATAL: failed to clear setuid bit after entry; mode=$mode path=$binary" >&2
	exit 1
fi

if wait "$test_pid"; then
	test_status=0
else
	test_status=$?
fi
test_pid=
if [ "$test_status" -ne 0 ]; then
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

for test_name in \
	TestPrivilegeGap_ChildCredentialsMatchTarget \
	TestPrivilegeGap_VerifiedFDExecutionUsesTargetCredentials \
	TestPrivilegeGap_StagingCleanupUsesRealPrivileges \
	TestPrivilegeGap_StagingCancellationCleansUp \
	TestPrivilegeGap_StartWindowIndependentOfCommandDuration \
	TestPrivilegeGap_TimeoutKillsChild \
	TestPrivilegeGap_CancelKillsChild \
	TestPrivilegeGap_OutputLimitAbortsRunningChild \
	TestPrivilegeGap_RefusedElevationDoesNotRecordWindow \
	TestRunAsSupplementaryGroups_MatchTargetUser_NotRoot
do
	if ! grep -q -- "^--- PASS: $test_name " "$output"; then
		echo "FATAL: required test did not pass: $test_name" >&2
		exit 1
	fi
done

echo "OK: all required privileged executor tests passed under env -i"
