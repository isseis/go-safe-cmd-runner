# 実装計画書：統合テストにおける一時ディレクトリ検証の強化

## 1. 概要

本ドキュメントは、タスク0041「統合テストにおける一時ディレクトリ検証の強化」の実装作業計画を記述します。

### 1.1 前提ドキュメント

本実装計画は以下のドキュメントに基づいて作成されています：

| ドキュメント | 参照目的 |
|----------|---------|
| `01_requirements.md` | 機能要件、非機能要件、テスト要件の確認 |
| `02_architecture.md` | アーキテクチャ設計、コンポーネント設計の理解 |
| `03_specification.md` | 詳細仕様、API仕様、実装方法の確認 |

### 1.2 実装方針

- **TDD (Test-Driven Development)**: テストを先に実装し、実行して失敗を確認してからコミット
- **段階的な実装**: 3つのPhaseに分けて実装
- **既存コードの最小限の変更**: 統合テストの改善に集中し、プロダクションコードは変更しない
- **シンプルさの優先**: 複雑な仕組みを避け、理解しやすいコードを書く

### 1.3 実装スコープ

#### 対象機能（In Scope）

- ✅ `testOutputBuffer` 構造体の実装
- ✅ `executorWithOutput` 構造体の実装（Executor ラッパー）
- ✅ `createRunnerWithOutputCapture()` ヘルパー関数の実装
- ✅ `extractWorkdirFromOutput()` ヘルパー関数の実装
- ✅ `validateTempDirBehavior()` ヘルパー関数の実装
- ✅ `TestIntegration_TempDirHandling` の改善
- ✅ 3つのテストケースの検証強化

#### 対象外（Out of Scope）

- ❌ プロダクションコードの変更（`internal/runner/*` は変更しない）
- ❌ 他の統合テストの変更（`TestIntegration_DryRunWithTempDir` など）
- ❌ ユニットテストの追加（既存のユニットテストで十分）
- ❌ E2E テストの実装（統合テストで十分）

## 2. フェーズ別実装計画

### Phase 1: テストインフラの実装

**目的**: 出力キャプチャとヘルパー関数の基盤を実装する

**所要時間**: 約 2-3 時間

#### 作業項目

| ID | タスク | ファイル | 作業内容 | 所要時間 | 状態 |
|----|-------|---------|---------|---------|------|
| P1-1 | testOutputBuffer実装 | `cmd/runner/integration_workdir_test.go` | 出力バッファ構造体と `Write()`, `String()` メソッドを実装 | 30min | [x] |
| P1-2 | executorWithOutput実装 | `cmd/runner/integration_workdir_test.go` | Executor ラッパー構造体と `ExecuteCommand()` メソッドを実装 | 45min | [x] |
| P1-3 | buildEnvSlice実装 | `cmd/runner/integration_workdir_test.go` | 環境変数マップをスライスに変換するヘルパー関数 | 15min | [x] |
| P1-4 | 動作確認テスト | - | 簡易的なテストで出力キャプチャが動作することを確認 | 30min | [x] |
| P1-5 | コミット | - | Phase 1 の実装をコミット | 10min | [x] |

#### P1-1: testOutputBuffer 実装

**ファイル:** `cmd/runner/integration_workdir_test.go`

**追加するコード:**

```go
// testOutputBuffer captures command output for testing
type testOutputBuffer struct {
    stdout bytes.Buffer
    stderr bytes.Buffer
    mu     sync.Mutex
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

**完了条件:**
- [ ] 構造体が定義されている
- [ ] `Write()` メソッドが `io.Writer` を実装している
- [ ] `String()` メソッドがバッファの内容を返す
- [ ] `sync.Mutex` で並行アクセスが保護されている

#### P1-2: executorWithOutput 実装

**ファイル:** `cmd/runner/integration_workdir_test.go`

**追加するコード:**

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
```

**完了条件:**
- [ ] `executor.CommandExecutor` を埋め込んでいる
- [ ] `ExecuteCommand()` メソッドが実装されている
- [ ] `io.MultiWriter` で標準出力とキャプチャバッファに出力している
- [ ] エラーハンドリングが適切に実装されている

#### P1-3: buildEnvSlice 実装

**ファイル:** `cmd/runner/integration_workdir_test.go`

**追加するコード:**

```go
// buildEnvSlice converts environment map to slice format
func buildEnvSlice(env map[string]string) []string {
    result := make([]string, 0, len(env))
    for k, v := range env {
        result = append(result, k+"="+v)
    }
    return result
}
```

**完了条件:**
- [ ] 環境変数マップをスライスに変換している
- [ ] フォーマットが `"KEY=VALUE"` になっている

#### P1-4: 動作確認テスト

簡易的なテストで出力キャプチャが動作することを確認します。

```go
// 一時的な動作確認テスト（Phase 1 完了後に削除）
func TestOutputCapture(t *testing.T) {
    buf := &testOutputBuffer{}

    n, err := buf.Write([]byte("test output\n"))
    require.NoError(t, err)
    require.Equal(t, 12, n)

    output := buf.String()
    require.Equal(t, "test output\n", output)
}
```

**完了条件:**
- [ ] テストが成功する
- [ ] 出力が正しくキャプチャされている

---

### Phase 2: ヘルパー関数の実装

**目的**: テストで使用するヘルパー関数を実装する

**所要時間**: 約 2-3 時間

#### 作業項目

| ID | タスク | ファイル | 作業内容 | 所要時間 | 状態 |
|----|-------|---------|---------|---------|------|
| P2-1 | createRunnerWithOutputCapture実装 | `cmd/runner/integration_workdir_test.go` | 出力キャプチャ付き Runner を作成するヘルパー関数 | 60min | [x] |
| P2-2 | extractWorkdirFromOutput実装 | `cmd/runner/integration_workdir_test.go` | 出力から `__runner_workdir` の値を抽出する関数 | 30min | [x] |
| P2-3 | validateTempDirBehavior実装 | `cmd/runner/integration_workdir_test.go` | 一時ディレクトリの動作を検証する関数 | 60min | [x] |
| P2-4 | ヘルパー関数のテスト | - | 各ヘルパー関数が正しく動作することを確認 | 30min | [x] |
| P2-5 | コミット | - | Phase 2 の実装をコミット | 10min | [ ] |

#### P2-1: createRunnerWithOutputCapture 実装

**ファイル:** `cmd/runner/integration_workdir_test.go`

**追加するコード:**

```go
// createRunnerWithOutputCapture creates a Runner with output capture enabled
func createRunnerWithOutputCapture(
    t *testing.T,
    configContent string,
    keepTempDirs bool,
) (*runner.Runner, *testOutputBuffer) {
    t.Helper()

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
        runner.WithExecutor(exec),
    }

    r, err := runner.NewRunner(cfg, runnerOptions...)
    require.NoError(t, err)

    return r, outputBuf
}
```

**完了条件:**
- [ ] 一時設定ファイルが作成され、クリーンアップされる
- [ ] 設定が正しくロードされる
- [ ] 出力バッファが作成される
- [ ] カスタム Executor が設定される
- [ ] Runner が正しく初期化される

#### P2-2: extractWorkdirFromOutput 実装

**ファイル:** `cmd/runner/integration_workdir_test.go`

**追加するコード:**

```go
// extractWorkdirFromOutput extracts the __runner_workdir path from command output
func extractWorkdirFromOutput(t *testing.T, output string) string {
    t.Helper()

    // 正規表現パターン: "working in: <path>"
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

**完了条件:**
- [ ] 正規表現でパターンマッチングしている
- [ ] パス抽出に失敗した場合、テストが失敗する
- [ ] 前後の空白が除去されている

#### P2-3: validateTempDirBehavior 実装

**ファイル:** `cmd/runner/integration_workdir_test.go`

**追加するコード:**

```go
// validateTempDirBehavior validates temporary directory creation and cleanup behavior
func validateTempDirBehavior(
    t *testing.T,
    workdirPath string,
    expectTempDir bool,
    keepTempDirs bool,
    afterCleanup bool,
) {
    t.Helper()

    // Case 1: 固定ワークディレクトリの場合
    if !expectTempDir {
        assert.NotContains(t, workdirPath, "scr-temp-",
            "Expected fixed workdir, but got temp dir: %s", workdirPath)

        info, err := os.Stat(workdirPath)
        assert.NoError(t, err, "Fixed workdir should exist: %s", workdirPath)
        assert.True(t, info.IsDir(), "Fixed workdir should be a directory: %s", workdirPath)

        return
    }

    // Case 2: 一時ディレクトリの場合
    assert.Contains(t, workdirPath, "scr-temp-",
        "Expected temp dir pattern 'scr-temp-*', but got: %s", workdirPath)

    // セキュリティチェック: システムの一時ディレクトリ配下にあるか
    tempRoot := os.TempDir()
    absPath, err := filepath.Abs(workdirPath)
    require.NoError(t, err)
    assert.True(t, strings.HasPrefix(absPath, tempRoot),
        "Temp dir should be under system temp dir %s, but got: %s", tempRoot, absPath)

    if afterCleanup {
        // クリーンアップ後の検証
        _, err := os.Stat(workdirPath)

        if keepTempDirs {
            assert.NoError(t, err,
                "Temp dir should exist after cleanup with keepTempDirs=true: %s", workdirPath)

            // テスト終了時に手動でクリーンアップ
            t.Cleanup(func() {
                os.RemoveAll(workdirPath)
            })
        } else {
            assert.True(t, os.IsNotExist(err),
                "Temp dir should be deleted after cleanup with keepTempDirs=false, but exists: %s",
                workdirPath)
        }
    } else {
        // クリーンアップ前の検証
        info, err := os.Stat(workdirPath)
        require.NoError(t, err, "Temp dir should exist before cleanup: %s", workdirPath)
        require.True(t, info.IsDir(), "Path should be a directory: %s", workdirPath)

        // パーミッションの検証（Linux/Unix のみ）
        if runtime.GOOS != "windows" {
            mode := info.Mode()
            assert.Equal(t, os.FileMode(0700), mode.Perm(),
                "Temp dir permissions should be 0700, got: %o", mode.Perm())
        }
    }
}
```

**完了条件:**
- [ ] 固定ワークディレクトリの検証ロジックが実装されている
- [ ] 一時ディレクトリの検証ロジックが実装されている
- [ ] セキュリティチェック（パストラバーサル対策）が実装されている
- [ ] クリーンアップ前後の検証が正しく実装されている
- [ ] `keepTempDirs` フラグに応じた検証が実装されている

---

### Phase 3: 統合テストの改善

**目的**: `TestIntegration_TempDirHandling` を改善し、実際の検証を実施する

**所要時間**: 約 1-2 時間

#### 作業項目

| ID | タスク | ファイル | 作業内容 | 所要時間 | 状態 |
|----|-------|---------|---------|---------|------|
| P3-1 | テストコードの書き換え | `cmd/runner/integration_workdir_test.go` | `TestIntegration_TempDirHandling` を改善 | 60min | [x] |
| P3-2 | テスト実行 | - | 改善したテストを実行し、成功を確認 | 15min | [x] |
| P3-3 | コミット（テスト） | - | Phase 3 のテストをコミット | 10min | [ ] |
| P3-4 | テスト実行（確認） | - | すべてのテストが成功することを確認 | 15min | [x] |
| P3-5 | 最終コミット | - | Phase 3 の完成版をコミット | 10min | [ ] |

#### P3-1: TestIntegration_TempDirHandling の改善

**ファイル:** `cmd/runner/integration_workdir_test.go`

**変更内容:**

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

**変更点:**
- 既存のコメント `// For this test, we primarily verify...` を削除
- TC-003 で固定ワークディレクトリを動的に作成するロジックを追加
- ステップ 4-8 を追加（出力抽出、固定ワークディレクトリ確認、検証）

**完了条件:**
- [ ] TC-003 で固定ワークディレクトリをテスト内で作成している
- [ ] `createRunnerWithOutputCapture()` を使用している
- [ ] `extractWorkdirFromOutput()` で出力をパースしている
- [ ] TC-003 で固定ワークディレクトリが正しく使用されていることを確認している
- [ ] クリーンアップ前の検証を実施している
- [ ] `CleanupAllResources()` を呼び出している
- [ ] クリーンアップ後の検証を実施している

#### P3-2: テスト実行

```bash
go test -v -run TestIntegration_TempDirHandling ./cmd/runner/
```

**期待される結果:**
- すべてのテストケース（TC-001, TC-002, TC-003）が成功する

**完了条件:**
- [ ] `TestIntegration_TempDirHandling` のすべてのサブテストが成功する
- [ ] 一時ディレクトリの作成・削除が正しく動作している

---

## 3. テスト戦略

### 3.1 単体テストレベル

**対象:** ヘルパー関数の個別動作

| 関数 | テスト方法 | 検証内容 |
|-----|----------|---------|
| `testOutputBuffer.Write()` | 簡易テスト | 出力が正しくバッファに書き込まれる |
| `testOutputBuffer.String()` | 簡易テスト | バッファの内容が文字列として取得できる |
| `buildEnvSlice()` | 簡易テスト | マップがスライスに正しく変換される |

**注:** これらは Phase 1 の動作確認テストで実施し、完了後は削除します。

### 3.2 統合テストレベル

**対象:** `TestIntegration_TempDirHandling` 全体

| テストケース | 検証内容 |
|------------|---------|
| TC-001 | 自動一時ディレクトリが作成され、クリーンアップ時に削除される |
| TC-002 | `keepTempDirs=true` の場合、一時ディレクトリが保持される |
| TC-003 | 固定ワークディレクトリが使用され、一時ディレクトリは作成されない |

## 4. 完了基準

### 4.1 コード完了基準

- [ ] Phase 1-3 のすべてのタスクが完了している
- [ ] すべてのチェックボックスがチェックされている
- [ ] コードレビューが完了している

### 4.2 テスト完了基準

- [ ] `TestIntegration_TempDirHandling` のすべてのサブテストが成功する
- [ ] `make test` が成功する
- [ ] `make lint` が成功する

### 4.3 ドキュメント完了基準

- [ ] 要件定義書が作成されている
- [ ] アーキテクチャ設計書が作成されている
- [ ] 詳細仕様書が作成されている
- [ ] 実装計画書が作成されている（本ドキュメント）

## 5. リスク管理

### 5.1 技術的リスク

| リスク | 影響度 | 発生確率 | 対策 |
|-------|-------|---------|------|
| 出力キャプチャが動作しない | 高 | 低 | Phase 1 で早期に動作確認 |
| 正規表現パースの失敗 | 中 | 低 | 明確なエラーメッセージで診断可能にする |
| パーミッション問題（CI環境） | 中 | 中 | Windows では検証をスキップ |
| 並行実行時の競合 | 低 | 低 | `sync.Mutex` で保護済み |

### 5.2 スケジュールリスク

| リスク | 影響度 | 発生確率 | 対策 |
|-------|-------|---------|------|
| 実装時間の見積もり誤り | 低 | 中 | 段階的な実装で早期に問題を発見 |
| テスト失敗の原因調査に時間がかかる | 中 | 低 | 明確なエラーメッセージで診断を支援 |

## 6. 実装チェックリスト

### Phase 1: テストインフラの実装
- [x] `testOutputBuffer` 構造体を実装
- [x] `executorWithOutput` 構造体を実装
- [x] `buildEnvSlice()` ヘルパー関数を実装
- [x] 動作確認テストを実行

### Phase 2: ヘルパー関数の実装
- [x] `createRunnerWithOutputCapture()` を実装
- [x] `extractWorkdirFromOutput()` を実装
- [x] `validateTempDirBehavior()` を実装
- [x] ヘルパー関数の動作確認

### Phase 3: 統合テストの改善
- [x] `TestIntegration_TempDirHandling` を改善
- [x] テスト実行して成功を確認
- [x] `make test` と `make lint` を実行して成功を確認

## 7. 見積もり

### 7.1 作業時間見積もり

| Phase | 作業内容 | 見積もり時間 |
|-------|---------|------------|
| Phase 1 | テストインフラの実装 | 2-3 時間 |
| Phase 2 | ヘルパー関数の実装 | 2-3 時間 |
| Phase 3 | 統合テストの改善 | 1-2 時間 |
| **合計** | | **5-8 時間** |

### 7.2 スケジュール

**推奨スケジュール:**
- Day 1 午前: Phase 1（2-3時間）
- Day 1 午後: Phase 2（2-3時間）
- Day 2 午前: Phase 3（1-2時間）
- Day 2 午後: レビュー、調整、最終テスト

**最短スケジュール:**
- 1日で完了可能（集中して作業する場合）

## 8. 参考資料

### 8.1 関連ドキュメント

- `01_requirements.md`: 要件定義書
- `02_architecture.md`: アーキテクチャ設計書
- `03_specification.md`: 詳細仕様書
- `docs/tasks/0034_workdir_redesign/`: 親タスクのドキュメント

### 8.2 参照コード

- `cmd/runner/integration_workdir_test.go`: 改善対象のテストコード
- `internal/runner/executor/executor.go`: Executor インターフェース
- `internal/runner/resource/temp_dir_manager.go`: 一時ディレクトリ管理

### 8.3 外部ライブラリ

- `github.com/stretchr/testify`: テストフレームワーク
- `regexp`: 正規表現パッケージ
- `io`: I/O インターフェース
- `os/exec`: コマンド実行

## まとめ

本実装計画書は、タスク0041「統合テストにおける一時ディレクトリ検証の強化」を3つのPhaseに分けて段階的に実装するための詳細な計画を提供します。

**重要なポイント:**
- **段階的な実装**: 3つのPhaseに分割し、各Phaseで動作確認
- **TDD**: テストを先に実装し、実行して失敗を確認
- **シンプルさ**: 複雑な仕組みを避け、理解しやすいコードを優先
- **既存コードへの影響なし**: 統合テストのみを変更、プロダクションコードは変更しない

**推定期間**: 約 5-8 時間（1-2日）

**次のステップ**: Phase 1（テストインフラの実装）から開始してください。
