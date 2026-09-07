//go:build integration

// These tests need -tags "test integration": executor/testutil requires test.
package executor_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/audit"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/executor"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/executor/testutil"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/output"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/privilege"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/risktypes"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/runnertypes"
	"github.com/isseis/go-safe-cmd-runner/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	setuidReadyLine     = "READY"
	setuidEntryReadyEnv = "TEST_SETUID_ENTRY_READY_FILE"
	stagedFileMode      = 0o550
	stagedDirMode       = 0o710
)

var (
	setuidEntryManager  privilege.Manager
	setuidObserverEntry privilege.Manager
)

func TestMain(m *testing.M) {
	readyPath := os.Getenv(setuidEntryReadyEnv)
	if readyPath != "" {
		managerLogger := slog.New(slog.NewTextHandler(io.Discard, nil))
		setuidEntryManager = privilege.NewManager(managerLogger)
		setuidObserverEntry = privilege.NewManager(managerLogger)
		if !setuidEntryManager.IsPrivilegedExecutionSupported() || !setuidObserverEntry.IsPrivilegedExecutionSupported() {
			fmt.Fprintln(os.Stderr, "FATAL: privilege managers rejected setuid entry before readiness")
			os.Exit(2)
		}
		readyFile, err := os.OpenFile(readyPath, os.O_WRONLY|os.O_TRUNC, 0o600)
		if err != nil {
			fmt.Fprintf(os.Stderr, "FATAL: failed to open setuid entry marker: %v\n", err)
			os.Exit(2)
		}
		_, writeErr := readyFile.WriteString("ready\n")
		closeErr := readyFile.Close()
		if err := errors.Join(writeErr, closeErr); err != nil {
			fmt.Fprintf(os.Stderr, "FATAL: failed to write setuid entry marker: %v\n", err)
			os.Exit(2)
		}
	}
	os.Exit(m.Run())
}

type setuidFixture struct {
	executor     executor.CommandExecutor
	observer     privilege.Manager
	logger       *slog.Logger
	recorder     *tu.LogRecorder
	target       *user.User
	targetUID    int
	targetGID    int
	targetGroups []int
	invokerUID   int
}

func newSetuidFixture(t *testing.T, opts ...executor.Option) *setuidFixture {
	t.Helper()

	invokerUID := os.Getuid()
	entryEUID := os.Geteuid()
	targetName := os.Getenv("TEST_RUNAS_TARGET_USER")
	if invokerUID == 0 || entryEUID != 0 {
		t.Skipf("setuid entry requires ruid != 0 and euid == 0; got ruid=%d euid=%d", invokerUID, entryEUID)
	}

	target, err := user.Lookup(targetName)
	if err != nil {
		t.Skipf("target lookup failed for %q: %v", targetName, err)
	}
	targetUID, err := strconv.Atoi(target.Uid)
	if err != nil {
		t.Skipf("target %q has non-numeric uid %q: %v", targetName, target.Uid, err)
	}
	targetGID, err := strconv.Atoi(target.Gid)
	if err != nil {
		t.Skipf("target %q has non-numeric primary gid %q: %v", targetName, target.Gid, err)
	}
	if targetUID == 0 || targetUID == invokerUID {
		t.Skipf("target uid must differ from root and the invoker; target_uid=%d invoker_uid=%d", targetUID, invokerUID)
	}
	groupIDs, err := target.GroupIds()
	if err != nil {
		t.Skipf("target group lookup failed for %q (uid=%d gid=%d): %v", targetName, targetUID, targetGID, err)
	}
	targetGroups := parseNumericIDs(t, groupIDs)

	logger, recorder := tu.NewRecordingLogger()
	manager := setuidEntryManager
	observer := setuidObserverEntry
	if manager == nil {
		manager = privilege.NewManager(logger)
	}
	if observer == nil {
		observer = privilege.NewManager(slog.New(slog.NewTextHandler(io.Discard, nil)))
	}
	if !manager.IsPrivilegedExecutionSupported() {
		t.Skipf("privilege manager rejected setuid entry ruid=%d euid=%d target_uid=%d target_gid=%d", invokerUID, entryEUID, targetUID, targetGID)
	}

	// Every test starts execution de-escalated and restores its entry euid even
	// after a failed assertion; process-wide credential changes stay serial.
	t.Cleanup(func() {
		require.NoError(t, syscall.Seteuid(entryEUID))
	})
	if err := syscall.Seteuid(invokerUID); err != nil {
		t.Skipf("failed to de-escalate setuid test from euid=%d to ruid=%d: %v", entryEUID, invokerUID, err)
	}
	if got := os.Geteuid(); got != invokerUID {
		t.Skipf("setuid test did not remain de-escalated: ruid=%d euid=%d", invokerUID, got)
	}

	baseOpts := []executor.Option{
		executor.WithPrivilegeManager(manager),
		executor.WithLogger(logger),
		executor.WithAuditLogger(audit.NewAuditLoggerWithCustom(logger)),
	}
	baseOpts = append(baseOpts, opts...)

	return &setuidFixture{
		executor:     executor.NewDefaultExecutor(baseOpts...),
		observer:     observer,
		logger:       logger,
		recorder:     recorder,
		target:       target,
		targetUID:    targetUID,
		targetGID:    targetGID,
		targetGroups: targetGroups,
		invokerUID:   invokerUID,
	}
}

func parseNumericIDs(t *testing.T, values []string) []int {
	t.Helper()
	ids := make([]int, 0, len(values))
	for _, value := range values {
		id, err := strconv.Atoi(value)
		require.NoErrorf(t, err, "non-numeric identity %q", value)
		ids = append(ids, id)
	}
	return ids
}

type childCredentials struct {
	ruid   int
	euid   int
	egid   int
	groups []int
	pid    int
	ready  bool
}

func parseChildCredentials(t *testing.T, outputText string) childCredentials {
	t.Helper()
	values := make(map[string]string)
	ready := false
	for line := range strings.SplitSeq(strings.TrimSpace(outputText), "\n") {
		if line == setuidReadyLine {
			ready = true
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if ok {
			values[key] = value
		}
	}

	parseOne := func(key string) int {
		t.Helper()
		value, ok := values[key]
		require.Truef(t, ok, "child output has no %s field: %q", key, outputText)
		n, err := strconv.Atoi(value)
		require.NoErrorf(t, err, "child %s is not numeric: %q", key, value)
		return n
	}
	groupText, ok := values["GROUPS"]
	require.Truef(t, ok, "child output has no GROUPS field: %q", outputText)
	groups := parseNumericIDs(t, strings.Fields(groupText))

	return childCredentials{
		ruid:   parseOne("RUID"),
		euid:   parseOne("EUID"),
		egid:   parseOne("EGID"),
		groups: groups,
		pid:    parseOne("PID"),
		ready:  ready,
	}
}

func assertChildCredentials(t *testing.T, fixture *setuidFixture, got childCredentials) {
	t.Helper()
	require.True(t, got.ready, "child must report readiness after its credentials")
	assert.Equal(t, fixture.targetUID, got.ruid)
	assert.Equal(t, fixture.targetUID, got.euid)
	assert.Equal(t, fixture.targetGID, got.egid)
	assert.ElementsMatch(t, fixture.targetGroups, got.groups)
	assert.NotEqual(t, 0, got.ruid, "run-as child must not retain root credentials")
	assert.NotEqual(t, fixture.invokerUID, got.ruid, "run-as child must not retain invoker credentials")
}

func credentialShellCommand(t *testing.T, fixture *setuidFixture, tail string, preReady ...string) (string, []string) {
	t.Helper()
	idPath := executortestutil.ResolveCommand("id")
	lines := []string{
		fmt.Sprintf(`printf 'RUID=%%s\n' "$(%q -ru)"`, idPath),
		fmt.Sprintf(`printf 'EUID=%%s\n' "$(%q -u)"`, idPath),
		fmt.Sprintf(`printf 'EGID=%%s\n' "$(%q -g)"`, idPath),
		fmt.Sprintf(`printf 'GROUPS=%%s\n' "$(%q -G)"`, idPath),
		`printf 'PID=%s\n' "$$"`,
	}
	if len(preReady) > 0 && preReady[0] != "" {
		lines = append(lines, preReady[0])
	}
	lines = append(lines, "echo READY")
	if tail != "" {
		lines = append(lines, tail)
	}
	return executortestutil.ResolveCommand("sh"), []string{"-c", strings.Join(lines, "; ")}
}

func runtimeCommand(path string, args []string, fixture *setuidFixture) *runnertypes.RuntimeCommand {
	return executortestutil.CreateRuntimeCommand(
		path,
		args,
		executortestutil.WithWorkDir(""),
		executortestutil.WithRunAsUser(fixture.target.Username),
	)
}

type readyOutputWriter struct {
	mu     sync.Mutex
	stdout bytes.Buffer
	ready  chan struct{}
	once   sync.Once
}

func newReadyOutputWriter() *readyOutputWriter {
	return &readyOutputWriter{ready: make(chan struct{})}
}

func (w *readyOutputWriter) Write(stream executor.OutputStream, data []byte) error {
	if stream != executor.StdoutStream {
		return nil
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	_, _ = w.stdout.Write(data)
	if bytes.Contains(w.stdout.Bytes(), []byte(setuidReadyLine+"\n")) {
		w.once.Do(func() { close(w.ready) })
	}
	return nil
}

func (*readyOutputWriter) Close() error { return nil }

func (w *readyOutputWriter) output() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.stdout.String()
}

func waitForReady(t *testing.T, writer *readyOutputWriter, timeout time.Duration) {
	t.Helper()
	select {
	case <-writer.ready:
	case <-time.After(timeout):
		t.Fatalf("child did not report readiness within %s; output=%q", timeout, writer.output())
	}
}

type asyncOutcome struct {
	result *executor.Result
	err    error
}

func executeAsync(
	ctx context.Context,
	e executor.CommandExecutor,
	plan *risktypes.VerifiedCommandPlan,
	cmd *runnertypes.RuntimeCommand,
	writer executor.OutputWriter,
) <-chan asyncOutcome {
	done := make(chan asyncOutcome, 1)
	go func() {
		defer close(done)
		result, err := e.Execute(ctx, plan, cmd, nil, writer)
		done <- asyncOutcome{result: result, err: err}
	}()
	return done
}

func registerAsyncCleanup(t *testing.T, cancel context.CancelFunc, done <-chan asyncOutcome) {
	t.Helper()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Errorf("command execution did not stop during cleanup")
			// Do not let later cleanup restore process-wide credentials while
			// Execute can still be changing them. The harness timeout remains
			// the process-level bound if cancellation cannot stop the goroutine.
			<-done
		}
	})
}

func waitForOutcome(t *testing.T, done <-chan asyncOutcome, timeout time.Duration) asyncOutcome {
	t.Helper()
	select {
	case outcome := <-done:
		return outcome
	case <-time.After(timeout):
		t.Fatalf("command execution did not return within %s", timeout)
		return asyncOutcome{}
	}
}

func assertWindowAttrs(t *testing.T, record tu.RecordSnapshot, want ...runnertypes.Operation) {
	t.Helper()
	require.EqualValues(t, len(want), record.Attrs["elevation_count"])
	got := make([]string, 0, len(want))
	for key, value := range record.Attrs {
		if !strings.HasPrefix(key, "privilege_duration_") || !strings.HasSuffix(key, "_us") {
			continue
		}
		op := strings.TrimSuffix(strings.TrimPrefix(key, "privilege_duration_"), "_us")
		duration, ok := value.(int64)
		require.Truef(t, ok, "window duration %s has type %T", key, value)
		assert.GreaterOrEqual(t, duration, int64(0), "window duration %s", key)
		got = append(got, op)
	}
	wantStrings := make([]string, 0, len(want))
	for _, op := range want {
		wantStrings = append(wantStrings, string(op))
	}
	assert.ElementsMatch(t, wantStrings, got)
}

func assertAuditWindows(t *testing.T, recorder *tu.LogRecorder, want ...runnertypes.Operation) {
	t.Helper()
	record := recorder.RequireRecord(t, slog.LevelInfo, "User/group command executed successfully")
	assertWindowAttrs(t, record, want...)
}

func assertFailureWindows(t *testing.T, recorder *tu.LogRecorder, want ...runnertypes.Operation) {
	t.Helper()
	record := recorder.RequireRecord(t, slog.LevelError, "User/group privilege execution failed")
	assertWindowAttrs(t, record, want...)
}

type privilegedStatResult struct {
	infos []os.FileInfo
	err   error
}

func statWithPrivileges(t *testing.T, fixture *setuidFixture, paths ...string) []os.FileInfo {
	t.Helper()
	observed := make(chan privilegedStatResult, 1)
	elevationCtx := runnertypes.ElevationContext{
		Operation: runnertypes.OperationStagingCleanup,
		FilePath:  strings.Join(paths, ","),
	}
	err := fixture.observer.WithPrivileges(elevationCtx, func() error {
		infos := make([]os.FileInfo, 0, len(paths))
		for _, path := range paths {
			info, statErr := os.Stat(path)
			if statErr != nil {
				observed <- privilegedStatResult{err: statErr}
				return statErr
			}
			infos = append(infos, info)
		}
		observed <- privilegedStatResult{infos: infos}
		return nil
	})
	require.NoError(t, err)
	result := <-observed
	require.NoError(t, result.err)
	return result.infos
}

func matchingOpenFDs(t *testing.T, path string) []string {
	t.Helper()
	wantInfo, err := os.Stat(path)
	require.NoError(t, err)
	entries, err := os.ReadDir("/proc/self/fd")
	require.NoError(t, err)
	var matching []string
	for _, entry := range entries {
		fdPath := filepath.Join("/proc/self/fd", entry.Name())
		info, statErr := os.Stat(fdPath)
		if errors.Is(statErr, os.ErrNotExist) {
			continue
		}
		require.NoErrorf(t, statErr, "stat parent fd %s", fdPath)
		if os.SameFile(wantInfo, info) {
			matching = append(matching, entry.Name())
		}
	}
	return matching
}

func stagedPathFromRecorder(t *testing.T, recorder *tu.LogRecorder) string {
	t.Helper()
	record := recorder.RequireRecord(t, slog.LevelDebug, "Staged verified command copy")
	path, ok := record.Attrs["staged_path"].(string)
	require.Truef(t, ok, "staged_path has type %T", record.Attrs["staged_path"])
	return path
}

func registerStagedCleanupSafetyNet(t *testing.T, fixture *setuidFixture) func(string) {
	t.Helper()
	var stagedPath string
	t.Cleanup(func() {
		if stagedPath == "" {
			return
		}
		elevationCtx := runnertypes.ElevationContext{
			Operation: runnertypes.OperationStagingCleanup,
			FilePath:  stagedPath,
		}
		err := fixture.observer.WithPrivileges(elevationCtx, func() error {
			return os.RemoveAll(filepath.Dir(stagedPath))
		})
		require.NoError(t, err)
	})
	return func(path string) { stagedPath = path }
}

func uniqueMarkerPath(t *testing.T) string {
	t.Helper()
	path := filepath.Join(os.TempDir(), fmt.Sprintf("scr-setuid-marker-%d-%d", os.Getpid(), time.Now().UnixNano()))
	require.NoFileExists(t, path)
	t.Cleanup(func() { _ = os.Remove(path) })
	return path
}

func TestPrivilegeGap_StartWindowIndependentOfCommandDuration(t *testing.T) {
	requireSetuidModel(t)
	fixture := newSetuidFixture(t)
	var durations []int64
	for _, seconds := range []string{"1", "5"} {
		cmd := runtimeCommand(executortestutil.ResolveCommand("sleep"), []string{seconds}, fixture)
		result, err := fixture.executor.Execute(context.Background(), nil, cmd, nil, nil)
		require.NoError(t, err)
		require.Equal(t, 0, result.ExitCode)
	}
	records := fixture.recorder.FindRecords(slog.LevelInfo, "User/group command executed successfully")
	require.Len(t, records, 2)
	for _, record := range records {
		duration, ok := record.Attrs["privilege_duration_user_group_execution_us"].(int64)
		require.True(t, ok, "start-window metric must be present")
		assert.Positive(t, duration)
		assert.Less(t, duration, int64(5000))
		durations = append(durations, duration)
	}
	difference := durations[1] - durations[0]
	if difference < 0 {
		difference = -difference
	}
	assert.Less(t, difference, int64(2000))
	t.Logf("start windows: %v us; difference: %d us", durations, difference)
}

func TestPrivilegeGap_ChildCredentialsMatchTarget(t *testing.T) {
	requireSetuidModel(t)
	fixture := newSetuidFixture(t)
	path, args := credentialShellCommand(t, fixture, "")
	cmd := runtimeCommand(path, args, fixture)

	before := os.Geteuid()
	result, err := fixture.executor.Execute(context.Background(), nil, cmd, nil, nil)
	require.NoError(t, err)
	require.Equal(t, before, os.Geteuid())
	require.Equal(t, 0, result.ExitCode)
	assertChildCredentials(t, fixture, parseChildCredentials(t, result.Stdout))
	assertAuditWindows(t, fixture.recorder, runnertypes.OperationUserGroupExecution)
}

func TestPrivilegeGap_TimeoutKillsChild(t *testing.T) {
	requireSetuidModel(t)
	fixture := newSetuidFixture(t)
	sleepPath := executortestutil.ResolveCommand("sleep")
	path, args := credentialShellCommand(t, fixture, fmt.Sprintf("exec %q 30", sleepPath))
	cmd := runtimeCommand(path, args, fixture)
	writer := newReadyOutputWriter()
	ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer cancel()

	before := os.Geteuid()
	start := time.Now()
	done := executeAsync(ctx, fixture.executor, nil, cmd, writer)
	registerAsyncCleanup(t, cancel, done)
	waitForReady(t, writer, time.Second)
	outcome := waitForOutcome(t, done, 3*time.Second)

	assert.Equal(t, before, os.Geteuid())
	require.ErrorIs(t, outcome.err, context.DeadlineExceeded)
	require.NotNil(t, outcome.result)
	_, exited := errors.AsType[*exec.ExitError](outcome.err)
	require.True(t, exited, "child must have started and exited")
	assert.NotErrorIs(t, outcome.err, executor.ErrChildNotReaped)
	assert.NotErrorIs(t, outcome.err, executor.ErrKillAfterCancel)
	assert.Less(t, time.Since(start), 3*time.Second)
	assertChildCredentials(t, fixture, parseChildCredentials(t, outcome.result.Stdout))
	assertFailureWindows(t, fixture.recorder,
		runnertypes.OperationUserGroupExecution,
		runnertypes.OperationKillAfterCancel,
	)
}

func TestPrivilegeGap_CancelKillsChild(t *testing.T) {
	requireSetuidModel(t)
	fixture := newSetuidFixture(t)
	sleepPath := executortestutil.ResolveCommand("sleep")
	path, args := credentialShellCommand(t, fixture, fmt.Sprintf("exec %q 30", sleepPath))
	cmd := runtimeCommand(path, args, fixture)
	writer := newReadyOutputWriter()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	before := os.Geteuid()
	start := time.Now()
	done := executeAsync(ctx, fixture.executor, nil, cmd, writer)
	registerAsyncCleanup(t, cancel, done)
	waitForReady(t, writer, time.Second)
	cancel()
	outcome := waitForOutcome(t, done, 2*time.Second)

	assert.Equal(t, before, os.Geteuid())
	require.ErrorIs(t, outcome.err, context.Canceled)
	require.NotNil(t, outcome.result)
	_, exited := errors.AsType[*exec.ExitError](outcome.err)
	require.True(t, exited, "child must have started and exited")
	assert.NotErrorIs(t, outcome.err, executor.ErrChildNotReaped)
	assert.NotErrorIs(t, outcome.err, executor.ErrKillAfterCancel)
	assert.Less(t, time.Since(start), 2*time.Second)
	assertChildCredentials(t, fixture, parseChildCredentials(t, outcome.result.Stdout))
	assertFailureWindows(t, fixture.recorder,
		runnertypes.OperationUserGroupExecution,
		runnertypes.OperationKillAfterCancel,
	)
}

func TestPrivilegeGap_OutputLimitAbortsRunningChild(t *testing.T) {
	requireSetuidModel(t)
	fixture := newSetuidFixture(t)
	file, err := os.CreateTemp(t.TempDir(), "capture-")
	require.NoError(t, err)
	capture := &output.Capture{FileHandle: file, MaxSize: 1024}
	t.Cleanup(func() { require.NoError(t, capture.Close()) })
	// A direct yes child writes indefinitely; closing its pipe must stop it.
	cmd := runtimeCommand(executortestutil.ResolveCommand("yes"), []string{"x"}, fixture)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	start := time.Now()
	result, err := fixture.executor.Execute(ctx, nil, cmd, nil, capture)
	require.ErrorIs(t, err, output.ErrOutputSizeExceeded)
	require.NotNil(t, result)
	assert.NotErrorIs(t, err, context.DeadlineExceeded)
	assert.Less(t, time.Since(start), 2*time.Second)
}

func TestPrivilegeGap_VerifiedFDExecutionUsesTargetCredentials(t *testing.T) {
	requireSetuidModel(t)
	if runtime.GOOS != "linux" {
		t.Skip("verified fd execution inspection requires /proc")
	}
	fixture := newSetuidFixture(t)
	marker := uniqueMarkerPath(t)
	sleepPath := executortestutil.ResolveCommand("sleep")
	statPath := executortestutil.ResolveCommand("stat")
	fdIdentity := fmt.Sprintf(`%q -Lc 'FD_ID=%%d:%%i' /proc/self/fd/3`, statPath)
	tail := fmt.Sprintf("while [ ! -e %q ]; do %q 0.05; done", marker, sleepPath)
	path, args := credentialShellCommand(t, fixture, tail, fdIdentity)
	cmd := runtimeCommand(path, args, fixture)
	plan := openVerifiedPlan(t, path, args)
	t.Cleanup(func() { require.NoError(t, plan.Close()) })
	parentFDsBefore := matchingOpenFDs(t, path)
	require.NotEmpty(t, parentFDsBefore, "verified plan must keep its source descriptor open")
	writer := newReadyOutputWriter()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	before := os.Geteuid()
	done := executeAsync(ctx, fixture.executor, plan, cmd, writer)
	registerAsyncCleanup(t, cancel, done)
	waitForReady(t, writer, time.Second)
	assert.ElementsMatch(t, parentFDsBefore, matchingOpenFDs(t, path),
		"executor-owned duplicate of the verified fd must close immediately after start")
	credentials := parseChildCredentials(t, writer.output())
	assertChildCredentials(t, fixture, credentials)

	verifiedInfo, err := os.Stat(path)
	require.NoError(t, err)
	verifiedStat, ok := verifiedInfo.Sys().(*syscall.Stat_t)
	require.True(t, ok)
	require.Contains(t, writer.output(), fmt.Sprintf("FD_ID=%d:%d\n", verifiedStat.Dev, verifiedStat.Ino))

	require.NoError(t, os.WriteFile(marker, []byte("done"), 0o600))
	outcome := waitForOutcome(t, done, 2*time.Second)
	require.NoError(t, outcome.err)
	require.Equal(t, 0, outcome.result.ExitCode)
	assert.Equal(t, before, os.Geteuid())
	assertAuditWindows(t, fixture.recorder, runnertypes.OperationUserGroupExecution)
}

func TestPrivilegeGap_StagingCleanupUsesRealPrivileges(t *testing.T) {
	requireSetuidModel(t)
	fixture := newSetuidFixture(t, executor.WithFdExecDisabled())
	rememberStagedPath := registerStagedCleanupSafetyNet(t, fixture)
	marker := uniqueMarkerPath(t)
	sleepPath := executortestutil.ResolveCommand("sleep")
	tail := fmt.Sprintf("while [ ! -e %q ]; do %q 0.05; done", marker, sleepPath)
	path, args := credentialShellCommand(t, fixture, tail)
	cmd := runtimeCommand(path, args, fixture)
	plan := openVerifiedPlan(t, path, args)
	t.Cleanup(func() { _ = plan.Close() })
	writer := newReadyOutputWriter()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	before := os.Geteuid()
	done := executeAsync(ctx, fixture.executor, plan, cmd, writer)
	registerAsyncCleanup(t, cancel, done)
	waitForReady(t, writer, time.Second)
	assertChildCredentials(t, fixture, parseChildCredentials(t, writer.output()))

	stagedPath := stagedPathFromRecorder(t, fixture.recorder)
	rememberStagedPath(stagedPath)
	infos := statWithPrivileges(t, fixture, stagedPath, filepath.Dir(stagedPath))
	fileInfo, dirInfo := infos[0], infos[1]
	assert.Equal(t, os.FileMode(stagedFileMode), fileInfo.Mode().Perm())
	assert.Equal(t, os.FileMode(stagedDirMode), dirInfo.Mode().Perm())
	fileStat, ok := fileInfo.Sys().(*syscall.Stat_t)
	require.True(t, ok)
	dirStat, ok := dirInfo.Sys().(*syscall.Stat_t)
	require.True(t, ok)
	assert.EqualValues(t, 0, fileStat.Uid)
	assert.EqualValues(t, fixture.targetGID, fileStat.Gid)
	assert.EqualValues(t, 0, dirStat.Uid)
	assert.EqualValues(t, fixture.targetGID, dirStat.Gid)

	require.NoError(t, os.WriteFile(marker, []byte("done"), 0o600))
	outcome := waitForOutcome(t, done, 2*time.Second)
	require.NoError(t, outcome.err)
	require.Equal(t, 0, outcome.result.ExitCode)
	assert.NoDirExists(t, filepath.Dir(stagedPath))
	assert.Equal(t, before, os.Geteuid())
	assertAuditWindows(t, fixture.recorder,
		runnertypes.OperationUserGroupExecution,
		runnertypes.OperationStagingCleanup,
	)
}

func TestPrivilegeGap_StagingCancellationCleansUp(t *testing.T) {
	requireSetuidModel(t)
	fixture := newSetuidFixture(t, executor.WithFdExecDisabled())
	rememberStagedPath := registerStagedCleanupSafetyNet(t, fixture)
	sleepPath := executortestutil.ResolveCommand("sleep")
	path, args := credentialShellCommand(t, fixture, fmt.Sprintf("exec %q 30", sleepPath))
	cmd := runtimeCommand(path, args, fixture)
	plan := openVerifiedPlan(t, path, args)
	t.Cleanup(func() { _ = plan.Close() })
	writer := newReadyOutputWriter()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	before := os.Geteuid()
	done := executeAsync(ctx, fixture.executor, plan, cmd, writer)
	registerAsyncCleanup(t, cancel, done)
	waitForReady(t, writer, time.Second)
	assertChildCredentials(t, fixture, parseChildCredentials(t, writer.output()))
	stagedPath := stagedPathFromRecorder(t, fixture.recorder)
	rememberStagedPath(stagedPath)
	_ = statWithPrivileges(t, fixture, stagedPath)

	cancel()
	outcome := waitForOutcome(t, done, 2*time.Second)
	require.ErrorIs(t, outcome.err, context.Canceled)
	require.NotNil(t, outcome.result)
	_, exited := errors.AsType[*exec.ExitError](outcome.err)
	require.True(t, exited, "staged child must be killed and reaped")
	assert.NotErrorIs(t, outcome.err, executor.ErrChildNotReaped)
	assert.NotErrorIs(t, outcome.err, executor.ErrKillAfterCancel)
	assert.NoDirExists(t, filepath.Dir(stagedPath))
	assert.Equal(t, before, os.Geteuid())
	assertFailureWindows(t, fixture.recorder,
		runnertypes.OperationUserGroupExecution,
		runnertypes.OperationKillAfterCancel,
		runnertypes.OperationStagingCleanup,
	)
}

var errElevationRefusedBeforeWindow = errors.New("test elevation refused before window")

type refusingPrivilegeManager struct{}

func (refusingPrivilegeManager) IsPrivilegedExecutionSupported() bool { return true }

func (refusingPrivilegeManager) WithPrivileges(runnertypes.ElevationContext, func() error) error {
	return errElevationRefusedBeforeWindow
}

func TestPrivilegeGap_RefusedElevationDoesNotRecordWindow(t *testing.T) {
	requireSetuidModel(t)
	fixture := newSetuidFixture(t)
	e := executor.NewDefaultExecutor(
		executor.WithPrivilegeManager(refusingPrivilegeManager{}),
		executor.WithLogger(fixture.logger),
		executor.WithAuditLogger(audit.NewAuditLoggerWithCustom(fixture.logger)),
	)
	path, args := credentialShellCommand(t, fixture, "")
	cmd := runtimeCommand(path, args, fixture)

	result, err := e.Execute(context.Background(), nil, cmd, nil, nil)
	require.ErrorIs(t, err, errElevationRefusedBeforeWindow)
	require.Nil(t, result)
	assert.Empty(t, fixture.recorder.FindRecords(slog.LevelInfo, "Privileges elevated"))
	assertFailureWindows(t, fixture.recorder)
	assert.Empty(t, fixture.recorder.FindRecords(slog.LevelInfo, "User/group command executed successfully"))
}
