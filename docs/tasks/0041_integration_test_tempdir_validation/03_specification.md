# 詳細仕様書：統合テストにおける一時ディレクトリ検証の強化

## 1. 概要

本ドキュメントは、`TestIntegration_TempDirHandling` 統合テストの改善に関する詳細仕様を記述します。

### 1.1 目的

- 一時ディレクトリの作成・削除動作を実際のファイルシステムで検証する
- `keepTempDirs` フラグの効果を確認する
- コマンド出力から `__runner_workdir` の値を抽出し、検証に利用する

### 1.2 前提ドキュメント

| ドキュメント | 参照目的 |
|----------|---------|
| `01_requirements.md` | 機能要件、非機能要件の確認 |
| `02_architecture.md` | アーキテクチャ設計、コンポーネント設計の理解 |

## 2. データ構造仕様

### 2.1 テストケース構造体

既存の構造体をそのまま使用します。

```go
// Test case structure for temp dir handling tests
type testCase struct {
    name          string  // Test case name
    keepTempDirs  bool    // --keep-temp-dirs flag value
    configContent string  // TOML configuration content
    expectTempDir bool    // Whether temp dir creation is expected
}
```

**フィールド仕様:**

| フィールド | 型 | 説明 | 使用目的 |
|----------|---|------|---------|
| `name` | `string` | テストケース名 | サブテストの識別 |
| `keepTempDirs` | `bool` | `--keep-temp-dirs` フラグの値 | Runner に渡すオプション |
| `configContent` | `string` | TOML 設定ファイルの内容 | 一時設定ファイルの生成 |
| `expectTempDir` | `bool` | 一時ディレクトリ作成の期待値 | 検証ロジックの制御 |

**expectTempDir の使用ルール:**

- `expectTempDir=true`: グループに `workdir` 指定がなく、自動的に一時ディレクトリが作成される
- `expectTempDir=false`: グループに `workdir` 固定指定があり、一時ディレクトリは作成されない

### 2.2 出力バッファ構造体

テスト専用の簡易的な出力キャプチャバッファを定義します。

```go
// testOutputBuffer captures command output for testing
type testOutputBuffer struct {
    stdout bytes.Buffer  // Standard output buffer
    stderr bytes.Buffer  // Standard error buffer (currently unused)
    mu     sync.Mutex    // Protects concurrent access
}

// Write implements io.Writer interface
func (b *testOutputBuffer) Write(p []byte) (n int, err error) {
    b.mu.Lock()
    defer b.mu.Unlock()
    return b.stdout.Write(p)
}

// String returns the captured output as a string
func (b *testOutputBuffer) String() string {
    b.mu.Lock()
    defer b.mu.Unlock()
    return b.stdout.String()
}
```

**責務:**
- コマンドの標準出力をメモリ上に保存する
- スレッドセーフなアクセスを提供する
- テスト終了時に出力内容を文字列として返す

**設計判断:**
- `bytes.Buffer` は無制限なので、サイズ制限は設けない（統合テストの出力は小規模なため）
- `stderr` は現状使用しないが、将来の拡張性のために定義
- `sync.Mutex` で並行アクセスを保護（Executor が並行してWrite を呼ぶ可能性に備える）

## 3. ヘルパー関数仕様

### 3.1 createRunnerWithOutputCapture

```go
// createRunnerWithOutputCapture creates a Runner with output capture enabled
func createRunnerWithOutputCapture(
    t *testing.T,
    configContent string,
    keepTempDirs bool,
) (*runner.Runner, *testOutputBuffer)
```

**目的:** 出力キャプチャを有効化した Runner を作成する

**パラメータ:**

| パラメータ | 型 | 説明 |
|----------|---|------|
| `t` | `*testing.T` | テストコンテキスト |
| `configContent` | `string` | TOML 設定ファイルの内容 |
| `keepTempDirs` | `bool` | 一時ディレクトリを保持するかどうか |

**戻り値:**

| 戻り値 | 型 | 説明 |
|------|---|------|
| `runner` | `*runner.Runner` | 作成された Runner インスタンス |
| `outputBuf` | `*testOutputBuffer` | 出力キャプチャバッファ |

**実装詳細:**

```go
func createRunnerWithOutputCapture(
    t *testing.T,
    configContent string,
    keepTempDirs bool,
) (*runner.Runner, *testOutputBuffer) {
    // 1. 一時設定ファイルの作成
    tempConfigFile, err := os.CreateTemp("", "test_config_*.toml")
    require.NoError(t, err)
    defer os.Remove(tempConfigFile.Name())

    _, err = tempConfigFile.WriteString(configContent)
    require.NoError(t, err)
    tempConfigFile.Close()

    // 2. 設定のロード
    verificationManager, err := verification.NewManager()
    require.NoError(t, err)

    cfg, err := bootstrap.LoadAndPrepareConfig(verificationManager, tempConfigFile.Name(), "test-run-id")
    require.NoError(t, err)

    runtimeGlobal, err := config.ExpandGlobal(&cfg.Global)
    require.NoError(t, err)

    // 3. 出力バッファの作成
    outputBuf := &testOutputBuffer{}

    // 4. Executor の作成（出力リダイレクト付き）
    baseExec := executor.New()
    exec := &executorWithOutput{
        CommandExecutor: baseExec,
        output:          outputBuf,
    }

    // 5. Privilege Manager の初期化
    privMgr := privilege.NewManager(nil)

    // 6. Runner の作成
    runnerOptions := []runner.Option{
        runner.WithVerificationManager(verificationManager),
        runner.WithPrivilegeManager(privMgr),
        runner.WithRunID("test-run-id"),
        runner.WithRuntimeGlobal(runtimeGlobal),
        runner.WithKeepTempDirs(keepTempDirs),
        runner.WithExecutor(exec),  // カスタムExecutorを設定
    }

    r, err := runner.NewRunner(cfg, runnerOptions...)
    require.NoError(t, err)

    return r, outputBuf
}
```

**エラーハンドリング:**
- すべてのエラーは `require.NoError()` で即座にテスト失敗とする
- 一時ファイルは `defer` で確実にクリーンアップする

### 3.2 executorWithOutput（内部構造体）

```go
// executorWithOutput wraps an executor to capture command output
type executorWithOutput struct {
    executor.CommandExecutor
    output io.Writer
}

// ExecuteCommand executes a command and captures its output
func (e *executorWithOutput) ExecuteCommand(
    ctx context.Context,
    cmd *runnertypes.RuntimeCommand,
    group *runnertypes.GroupSpec,
    env map[string]string,
) (int, error) {
    // os/exec.Cmd の作成
    execCmd := exec.CommandContext(ctx, cmd.ExpandedCmd, cmd.ExpandedArgs...)
    execCmd.Env = buildEnvSlice(env)
    execCmd.Dir = cmd.EffectiveWorkDir

    // 出力を両方にリダイレクト（標準出力 + キャプチャバッファ）
    execCmd.Stdout = io.MultiWriter(os.Stdout, e.output)
    execCmd.Stderr = os.Stderr

    // コマンド実行
    err := execCmd.Run()

    // 終了コードの取得
    exitCode := 0
    if err != nil {
        if exitErr, ok := err.(*exec.ExitError); ok {
            exitCode = exitErr.ExitCode()
        } else {
            return 0, err
        }
    }

    return exitCode, nil
}

// buildEnvSlice converts environment map to slice format
func buildEnvSlice(env map[string]string) []string {
    result := make([]string, 0, len(env))
    for k, v := range env {
        result = append(result, k+"="+v)
    }
    return result
}
```

**設計判断:**
- `io.MultiWriter` を使用して、標準出力とキャプチャバッファの両方に出力
  - デバッグ時に出力が見える（標準出力に出力）
  - テスト検証用にバッファにも保存
- エラーハンドリングは元の `executor.CommandExecutor` と同じロジックを使用
- 構造体埋め込み（`executor.CommandExecutor`）により、他のメソッドは元の実装を継承

### 3.3 extractWorkdirFromOutput

```go
// extractWorkdirFromOutput extracts the __runner_workdir path from command output
func extractWorkdirFromOutput(t *testing.T, output string) string
```

**目的:** コマンド出力から `__runner_workdir` のパス文字列を抽出する

**パラメータ:**

| パラメータ | 型 | 説明 |
|----------|---|------|
| `t` | `*testing.T` | テストコンテキスト |
| `output` | `string` | コマンドの出力文字列全体 |

**戻り値:**

| 戻り値 | 型 | 説明 |
|------|---|------|
| `workdirPath` | `string` | 抽出されたワークディレクトリのパス |

**実装詳細:**

```go
func extractWorkdirFromOutput(t *testing.T, output string) string {
    t.Helper()

    // 正規表現パターン: "working in: <path>"
    // テスト設定でコマンドを以下のように定義することを想定:
    //   args = ["working in: %{__runner_workdir}"]
    pattern := regexp.MustCompile(`working in: (.+)`)
    matches := pattern.FindStringSubmatch(output)

    require.Len(t, matches, 2,
        "Failed to extract workdir from output. Expected 'working in: <path>', got: %s",
        output)

    workdirPath := strings.TrimSpace(matches[1])
    require.NotEmpty(t, workdirPath, "Extracted workdir path is empty")

    return workdirPath
}
```

**想定される入力例:**

```
working in: /tmp/scr-temp-abc123/group_test
```

**抽出結果:**

```
/tmp/scr-temp-abc123/group_test
```

**エラーハンドリング:**
- パターンマッチ失敗時: テスト失敗（`require.Len`）
- パスが空文字列: テスト失敗（`require.NotEmpty`）

**設計判断:**
- `t.Helper()` を呼び出してスタックトレースを適切に表示
- 正規表現は単純なパターンを使用（複雑なパス検証は不要）
- `strings.TrimSpace()` で前後の空白を除去

### 3.4 validateTempDirBehavior

```go
// validateTempDirBehavior validates temporary directory creation and cleanup behavior
func validateTempDirBehavior(
    t *testing.T,
    workdirPath string,
    expectTempDir bool,
    keepTempDirs bool,
    afterCleanup bool,
)
```

**目的:** 一時ディレクトリの作成・削除動作を検証する

**パラメータ:**

| パラメータ | 型 | 説明 |
|----------|---|------|
| `t` | `*testing.T` | テストコンテキスト |
| `workdirPath` | `string` | 検証対象のワークディレクトリパス |
| `expectTempDir` | `bool` | 一時ディレクトリ作成を期待するか |
| `keepTempDirs` | `bool` | 一時ディレクトリを保持するか |
| `afterCleanup` | `bool` | クリーンアップ後の検証か |

**実装詳細:**

```go
func validateTempDirBehavior(
    t *testing.T,
    workdirPath string,
    expectTempDir bool,
    keepTempDirs bool,
    afterCleanup bool,
) {
    t.Helper()

    // Case 1: 固定ワークディレクトリの場合（expectTempDir=false）
    if !expectTempDir {
        // 一時ディレクトリパターンではないことを確認
        assert.NotContains(t, workdirPath, "scr-temp-",
            "Expected fixed workdir, but got temp dir: %s", workdirPath)

        // 固定ワークディレクトリは削除されないことを確認
        info, err := os.Stat(workdirPath)
        assert.NoError(t, err, "Fixed workdir should exist: %s", workdirPath)
        assert.True(t, info.IsDir(), "Fixed workdir should be a directory: %s", workdirPath)

        return
    }

    // Case 2: 一時ディレクトリの場合（expectTempDir=true）

    // 一時ディレクトリの命名規則に従っているか確認
    assert.Contains(t, workdirPath, "scr-temp-",
        "Expected temp dir pattern 'scr-temp-*', but got: %s", workdirPath)

    // システムの一時ディレクトリ配下にあるか確認（セキュリティチェック）
    tempRoot := os.TempDir()
    absPath, err := filepath.Abs(workdirPath)
    require.NoError(t, err)
    assert.True(t, strings.HasPrefix(absPath, tempRoot),
        "Temp dir should be under system temp dir %s, but got: %s", tempRoot, absPath)

    if afterCleanup {
        // クリーンアップ後の検証
        _, err := os.Stat(workdirPath)

        if keepTempDirs {
            // keepTempDirs=true の場合、ディレクトリは保持される
            assert.NoError(t, err,
                "Temp dir should exist after cleanup with keepTempDirs=true: %s", workdirPath)

            // （オプション）テスト終了時に手動でクリーンアップ
            t.Cleanup(func() {
                os.RemoveAll(workdirPath)
            })
        } else {
            // keepTempDirs=false の場合、ディレクトリは削除される
            assert.True(t, os.IsNotExist(err),
                "Temp dir should be deleted after cleanup with keepTempDirs=false, but exists: %s",
                workdirPath)
        }
    } else {
        // クリーンアップ前の検証
        info, err := os.Stat(workdirPath)
        require.NoError(t, err, "Temp dir should exist before cleanup: %s", workdirPath)
        require.True(t, info.IsDir(), "Path should be a directory: %s", workdirPath)

        // （オプション）パーミッションの検証（Linux/Unix のみ）
        if runtime.GOOS != "windows" {
            mode := info.Mode()
            assert.Equal(t, os.FileMode(0700), mode.Perm(),
                "Temp dir permissions should be 0700, got: %o", mode.Perm())
        }
    }
}
```

**検証パターン表:**

| expectTempDir | keepTempDirs | afterCleanup | 検証内容 |
|--------------|--------------|-------------|---------|
| `false` | - | - | 固定ワークディレクトリであることを確認、存在を確認 |
| `true` | - | `false` | 一時ディレクトリが存在し、パーミッションが正しい |
| `true` | `false` | `true` | 一時ディレクトリが削除されている |
| `true` | `true` | `true` | 一時ディレクトリが保持されている |

**エラーハンドリング:**
- ファイルシステムエラーは適切にハンドリングし、診断に役立つメッセージを提供
- `os.IsNotExist()` で「存在しない」と「その他のエラー」を区別

## 4. テストフロー仕様

### 4.1 TestIntegration_TempDirHandling の全体フロー

```go
func TestIntegration_TempDirHandling(t *testing.T) {
    tests := []struct {
        name          string
        keepTempDirs  bool
        configContent string
        expectTempDir bool
    }{
        {
            name:         "Auto temp dir without keep flag",
            keepTempDirs: false,
            configContent: `
[[groups]]
name = "test_group"

[[groups.commands]]
name = "test_cmd"
cmd = "echo"
args = ["working in: %{__runner_workdir}"]
`,
            expectTempDir: true,
        },
        {
            name:         "Auto temp dir with keep flag",
            keepTempDirs: true,
            configContent: `
[[groups]]
name = "test_group"

[[groups.commands]]
name = "test_cmd"
cmd = "echo"
args = ["working in: %{__runner_workdir}"]
`,
            expectTempDir: true,
        },
        {
            name:         "Fixed workdir",
            keepTempDirs: false,
            // configContent は各テスト内で動的に生成（固定ワークディレクトリを含む）
            configContent: "",  // この後、テスト内で生成
            expectTempDir: false,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // TC-003 の場合、固定ワークディレクトリを作成
            var fixedWorkdir string
            if tt.name == "Fixed workdir" {
                var err error
                fixedWorkdir, err = os.MkdirTemp("", "test-fixed-workdir-*")
                require.NoError(t, err)
                defer os.RemoveAll(fixedWorkdir)

                // configContent を動的に生成
                tt.configContent = fmt.Sprintf(`
[[groups]]
name = "test_group"
workdir = "%s"

[[groups.commands]]
name = "test_cmd"
cmd = "echo"
args = ["working in: %%{__runner_workdir}"]
`, fixedWorkdir)
            }

            // 1. Runner作成（出力キャプチャ有効）
            r, outputBuf := createRunnerWithOutputCapture(t, tt.configContent, tt.keepTempDirs)

            // 2. システム環境変数のロード
            err := r.LoadSystemEnvironment()
            require.NoError(t, err)

            // 3. すべてのグループを実行
            ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
            defer cancel()

            err = r.ExecuteAll(ctx)
            require.NoError(t, err)

            // 4. 出力から __runner_workdir の値を抽出
            output := outputBuf.String()
            workdirPath := extractWorkdirFromOutput(t, output)

            // 5. TC-003 の場合、固定ワークディレクトリが使用されていることを確認
            if tt.name == "Fixed workdir" {
                assert.Equal(t, fixedWorkdir, workdirPath,
                    "Expected fixed workdir to be used: %s, got: %s", fixedWorkdir, workdirPath)
            }

            // 6. クリーンアップ前の検証
            validateTempDirBehavior(t, workdirPath, tt.expectTempDir, tt.keepTempDirs, false)

            // 7. リソースのクリーンアップ
            err = r.CleanupAllResources()
            require.NoError(t, err)

            // 8. クリーンアップ後の検証
            validateTempDirBehavior(t, workdirPath, tt.expectTempDir, tt.keepTempDirs, true)
        })
    }
}
```

**ステップ詳細:**

0. **固定ワークディレクトリの準備**（TC-003 のみ）: テスト用の一時ディレクトリを作成し、`configContent` に埋め込む
1. **Runner作成**: `createRunnerWithOutputCapture()` で出力キャプチャを有効化
2. **環境変数ロード**: システム環境変数を Runner に読み込む
3. **グループ実行**: タイムアウト付きコンテキストで `ExecuteAll()` を実行
4. **出力抽出**: バッファから `__runner_workdir` の値を取得
5. **固定ワークディレクトリ確認**（TC-003 のみ）: 抽出したパスが期待される固定ワークディレクトリと一致するか確認
6. **クリーンアップ前検証**: 一時ディレクトリが正しく作成されているか確認
7. **クリーンアップ**: `CleanupAllResources()` で一時ディレクトリを削除
8. **クリーンアップ後検証**: `keepTempDirs` フラグに応じた動作を確認

### 4.2 実行タイムライン

```
時刻 0ms:   テスト開始
時刻 10ms:  Runner作成、一時設定ファイル作成
時刻 20ms:  LoadSystemEnvironment()
時刻 30ms:  ExecuteAll() 開始
時刻 50ms:  一時ディレクトリ作成（expectTempDir=true の場合）
時刻 60ms:  コマンド実行、出力キャプチャ
時刻 70ms:  ExecuteAll() 完了
時刻 80ms:  extractWorkdirFromOutput()
時刻 90ms:  validateTempDirBehavior(afterCleanup=false)
時刻 100ms: CleanupAllResources()
時刻 110ms: 一時ディレクトリ削除（keepTempDirs=false の場合）
時刻 120ms: validateTempDirBehavior(afterCleanup=true)
時刻 130ms: テスト完了
```

## 5. テストケース詳細仕様

### 5.1 TC-001: Auto temp dir without keep flag

**目的:** 一時ディレクトリが自動作成され、クリーンアップ時に削除されることを確認

**設定:**
- `keepTempDirs`: `false`
- `expectTempDir`: `true`
- `groups.workdir`: 未指定

**期待される動作:**

| タイミング | 期待される状態 |
|----------|-------------|
| ExecuteAll 前 | 一時ディレクトリは存在しない |
| ExecuteAll 中 | `/tmp/scr-temp-<random>/` が作成される |
| ExecuteAll 後 | 一時ディレクトリが存在する（パーミッション 0700） |
| CleanupAllResources 後 | 一時ディレクトリが削除される |

**検証ポイント:**
- `workdirPath` に `"scr-temp-"` が含まれる
- クリーンアップ前: `os.Stat(workdirPath)` が成功する
- クリーンアップ後: `os.IsNotExist(os.Stat(workdirPath))` が `true`

### 5.2 TC-002: Auto temp dir with keep flag

**目的:** `--keep-temp-dirs` フラグが有効な場合、一時ディレクトリが保持されることを確認

**設定:**
- `keepTempDirs`: `true`
- `expectTempDir`: `true`
- `groups.workdir`: 未指定

**期待される動作:**

| タイミング | 期待される状態 |
|----------|-------------|
| ExecuteAll 前 | 一時ディレクトリは存在しない |
| ExecuteAll 中 | `/tmp/scr-temp-<random>/` が作成される |
| ExecuteAll 後 | 一時ディレクトリが存在する |
| CleanupAllResources 後 | 一時ディレクトリが**保持される** |

**検証ポイント:**
- `workdirPath` に `"scr-temp-"` が含まれる
- クリーンアップ前: `os.Stat(workdirPath)` が成功する
- クリーンアップ後: `os.Stat(workdirPath)` が成功する（削除されていない）

**クリーンアップ:**
- テスト終了時に `t.Cleanup()` で手動削除

### 5.3 TC-003: Fixed workdir

**目的:** 固定ワークディレクトリが指定されている場合、一時ディレクトリが作成されないことを確認

**設定:**
- `keepTempDirs`: `false`
- `expectTempDir`: `false`
- `groups.workdir`: テスト内で作成した一時ディレクトリ（例: `/tmp/test-fixed-workdir-123456`）

**セットアップ:**
```go
// テスト開始時に固定ワークディレクトリを作成
fixedWorkdir, err := os.MkdirTemp("", "test-fixed-workdir-*")
require.NoError(t, err)
defer os.RemoveAll(fixedWorkdir)

// configContent に fixedWorkdir を埋め込む
configContent := fmt.Sprintf(`
[[groups]]
name = "test_group"
workdir = "%s"

[[groups.commands]]
name = "test_cmd"
cmd = "echo"
args = ["working in: %%{__runner_workdir}"]
`, fixedWorkdir)
```

**期待される動作:**

| タイミング | 期待される状態 |
|----------|-------------|
| ExecuteAll 前 | 固定ワークディレクトリが存在する（テストで作成済み） |
| ExecuteAll 中 | 固定ワークディレクトリを使用、Runner の一時ディレクトリは作成しない |
| ExecuteAll 後 | 固定ワークディレクトリが存在する |
| CleanupAllResources 後 | 固定ワークディレクトリが存在する（削除されない） |
| テスト終了後 | 固定ワークディレクトリをテストがクリーンアップ |

**検証ポイント:**
- `workdirPath` に `"scr-temp-"` が含まれない（Runner が作成する一時ディレクトリではない）
- `workdirPath == fixedWorkdir`（テストで作成した固定ワークディレクトリと一致）
- クリーンアップ前: `os.Stat(workdirPath)` が成功する
- クリーンアップ後: `os.Stat(workdirPath)` が成功する（Runner はこのディレクトリを削除しない）
- テスト終了時: `defer os.RemoveAll(fixedWorkdir)` でテストがクリーンアップ

**注意点:**
- `/tmp` のようなシステムのグローバルなディレクトリを直接使用しない
- テストごとに独立した一時ディレクトリを使用することで、テストの独立性を保証する
- テスト終了時に必ず `defer os.RemoveAll()` でクリーンアップする

## 6. エラーハンドリング仕様

### 6.1 出力パース失敗

**シナリオ:** コマンド出力に期待されるパターン `"working in: <path>"` が含まれない

**原因:**
- コマンド設定が誤っている
- コマンド実行が失敗している
- 出力がキャプチャされていない

**エラーメッセージ:**
```
Failed to extract workdir from output. Expected 'working in: <path>', got: <actual output>
```

**対処:** テスト失敗（`require.Len` による）

### 6.2 ディレクトリ存在確認の失敗

**シナリオ:** クリーンアップ前に一時ディレクトリが存在しない

**原因:**
- 一時ディレクトリの作成に失敗している
- パスの抽出が間違っている

**エラーメッセージ:**
```
Temp dir should exist before cleanup: /tmp/scr-temp-abc123
```

**対処:** テスト失敗（`require.NoError` による）

### 6.3 クリーンアップ失敗

**シナリオ:** `CleanupAllResources()` 実行後も一時ディレクトリが残っている（`keepTempDirs=false` の場合）

**原因:**
- クリーンアップロジックのバグ
- パーミッションエラー

**エラーメッセージ:**
```
Temp dir should be deleted after cleanup with keepTempDirs=false, but exists: /tmp/scr-temp-abc123
```

**対処:** テスト失敗（`assert.True(os.IsNotExist(...))` による）

## 7. セキュリティ仕様

### 7.1 パストラバーサル対策

**要件:** 抽出したパスがシステムの一時ディレクトリ配下にあることを確認する

**実装:**

```go
tempRoot := os.TempDir()
absPath, err := filepath.Abs(workdirPath)
require.NoError(t, err)
assert.True(t, strings.HasPrefix(absPath, tempRoot),
    "Temp dir should be under system temp dir %s, but got: %s", tempRoot, absPath)
```

**防止する攻撃:**
- パストラバーサル（`../../etc/passwd` など）
- シンボリックリンク攻撃

### 7.2 パーミッション検証

**要件:** 一時ディレクトリのパーミッションが `0700` であることを確認する

**実装:**

```go
if runtime.GOOS != "windows" {
    mode := info.Mode()
    assert.Equal(t, os.FileMode(0700), mode.Perm(),
        "Temp dir permissions should be 0700, got: %o", mode.Perm())
}
```

**対象:** Linux/Unix のみ（Windows は対象外）

## 8. パフォーマンス仕様

### 8.1 実行時間

**目標:** 各テストケースが 1 秒以内に完了すること

**測定方法:** `go test -v -run TestIntegration_TempDirHandling` の実行時間

**現実的な実行時間:**
- TC-001: ~200ms
- TC-002: ~200ms
- TC-003: ~150ms（一時ディレクトリ作成・削除がないため高速）

### 8.2 メモリ使用量

**制約:** 出力バッファのサイズは無制限だが、統合テストの出力は小規模（< 1KB）なので問題なし

**監視:** 必要に応じて `testing.B` でベンチマークを追加可能

## 9. 互換性仕様

### 9.1 OS 互換性

| OS | サポート状況 | 備考 |
|----|----------|------|
| Linux | ✅ サポート | フルサポート、パーミッション検証あり |
| macOS | ✅ サポート | フルサポート、パーミッション検証あり |
| Windows | ⚠️ 部分サポート | パーミッション検証はスキップ |

### 9.2 Go バージョン

- **必須:** Go 1.23.10 以上
- **理由:** `os.MkdirTemp()` の使用、`context.Context` の標準ライブラリサポート

## 10. まとめ

本仕様書では、以下の実装詳細を定義しました：

1. **データ構造**: テストケース構造体、出力バッファ構造体
2. **ヘルパー関数**: Runner作成、出力抽出、検証ロジック
3. **テストフロー**: 7ステップの明確な検証プロセス
4. **テストケース**: 3つの主要シナリオ（自動一時ディレクトリ、保持、固定ワークディレクトリ）
5. **エラーハンドリング**: 各種エラーケースへの対応
6. **セキュリティ**: パストラバーサル対策、パーミッション検証

この仕様に基づいて実装することで、統合テストとしての価値を高めることができます。
