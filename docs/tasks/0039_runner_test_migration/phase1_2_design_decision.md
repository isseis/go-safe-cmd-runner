# Task 0039 Phase 1.2: 設計方針決定書

**作成日**: 2025-10-21
**作成者**: GitHub Copilot
**目的**: runner_test.go 移行における3つの主要問題の解決方針を決定

## エグゼクティブサマリー

Phase 1.1 の分析で特定された65個のエラーに対し、以下の設計方針を決定しました：

1. **EffectiveWorkdir問題**: 既存のヘルパー関数パターンを踏襲
2. **SetupFailedMockExecution問題**: 既存の `setupFailedMockExecution` を活用
3. **TempDir問題**: Go標準の `t.TempDir()` を使用

## 問題1: EffectiveWorkdir（35箇所）

### 問題の詳細

`CommandSpec` に `EffectiveWorkdir` フィールドが存在しない。このフィールドは `RuntimeCommand` に属している。

### 既存のベストプラクティス

`internal/runner/executor/executor_test.go` に既に良いパターンが存在：

```go
func createRuntimeCommand(cmd string, args []string, workDir string, runAsUser, runAsGroup string) *runnertypes.RuntimeCommand {
	spec := &runnertypes.CommandSpec{
		Name:       "test-command",
		Cmd:        cmd,
		Args:       args,
		WorkDir:    workDir,
		Timeout:    0,
		RunAsUser:  runAsUser,
		RunAsGroup: runAsGroup,
	}
	return &runnertypes.RuntimeCommand{
		Spec:             spec,
		ExpandedCmd:      cmd,
		ExpandedArgs:     args,
		ExpandedEnv:      make(map[string]string),
		EffectiveWorkDir: workDir,
		EffectiveTimeout: 0,
	}
}
```

### 決定事項

**✅ 採用する解決策**: ヘルパー関数アプローチ

#### 1. runner_test.go にヘルパー関数を追加

```go
// createTestRuntimeCommand creates a RuntimeCommand for testing with minimal setup
func createTestRuntimeCommand(spec *runnertypes.CommandSpec, effectiveWorkDir string) *runnertypes.RuntimeCommand {
	return &runnertypes.RuntimeCommand{
		Spec:             spec,
		ExpandedCmd:      spec.Cmd,
		ExpandedArgs:     spec.Args,
		ExpandedEnv:      make(map[string]string),
		ExpandedVars:     make(map[string]string),
		EffectiveWorkDir: effectiveWorkDir,
		EffectiveTimeout: 30,
	}
}
```

#### 2. 使用例

**Before（エラー）**:
```go
mockResourceManager.On("ExecuteCommand",
    mock.Anything,
    runnertypes.CommandSpec{
        Name: "cmd-1",
        Cmd: "false",
        EffectiveWorkdir: "/tmp",  // ❌ エラー
    },
    &group,
    mock.Anything,
).Return(...)
```

**After（修正）**:
```go
expectedSpec := runnertypes.CommandSpec{
    Name: "cmd-1",
    Cmd: "false",
}
expectedCmd := createTestRuntimeCommand(&expectedSpec, "/tmp")

mockResourceManager.On("ExecuteCommand",
    mock.Anything,
    mock.MatchedBy(func(cmd *runnertypes.RuntimeCommand) bool {
        return cmd.Spec.Name == "cmd-1" && cmd.EffectiveWorkDir == "/tmp"
    }),
    &group,
    mock.Anything,
).Return(...)
```

または、よりシンプルに：

```go
mockResourceManager.On("ExecuteCommand",
    mock.Anything,
    mock.Anything,  // RuntimeCommand - 詳細チェックは不要な場合
    &group,
    mock.Anything,
).Return(...)
```

#### 3. フィールドアクセスの修正（2箇所）

**Before（エラー）**:
```go
expectedCmd.EffectiveWorkdir = config.Global.WorkDir
```

**After（修正）**:
```go
// RuntimeCommand を作成してから EffectiveWorkDir を設定
runtimeCmd := createTestRuntimeCommand(&expectedCmd, config.Global.WorkDir)
// または
runtimeCmd.EffectiveWorkDir = config.Global.WorkDir
```

### 実装の優先順位

1. **高**: ヘルパー関数の実装（Phase 2.2）
2. **高**: フィールドアクセスの修正（2箇所）
3. **中**: モック設定の修正（33箇所）

### 推定作業時間

- ヘルパー関数実装: 0.2時間
- フィールドアクセス修正: 0.1時間
- モック設定修正: 2-3時間（テストケースごとに検証が必要）
- **合計**: 2.3-3.3時間

---

## 問題2: SetupFailedMockExecution（16箇所）

### 問題の詳細

テストコードで `mockRM.SetupFailedMockExecution(errors.New("test error"))` を呼び出しているが、このメソッドは存在しない。

### 既存のベストプラクティス

**✅ 既に解決済み！**

`internal/runner/test_helpers.go` に `setupFailedMockExecution` 関数が既に存在：

```go
// setupFailedMockExecution sets up mock for failed command execution with custom error
func setupFailedMockExecution(m *MockResourceManager, err error) {
	m.On("ValidateOutputPath", mock.AnythingOfType("string"), mock.AnythingOfType("string")).Return(nil)
	m.On("ExecuteCommand", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil, err)
}
```

### 決定事項

**✅ 採用する解決策**: 既存の `setupFailedMockExecution` を使用

#### 使用方法

**Before（エラー）**:
```go
mockRM.SetupFailedMockExecution(errors.New("test error"))  // ❌ メソッドが存在しない
```

**After（修正）**:
```go
setupFailedMockExecution(mockRM, errors.New("test error"))  // ✅ 関数呼び出し
```

### 実装の優先順位

**高**: 全16箇所を機械的に置換（Phase 3で実施）

### 推定作業時間

- 0.5-1時間（検索置換 + 動作確認）

---

## 問題3: TempDir（14箇所）

### 問題の詳細

`GroupSpec` に `TempDir` フィールドが存在しない。

### 検討した選択肢

#### Option 1: テストをスキップ

```go
t.Skip("TempDir feature not yet implemented (Task 0040)")
```

**利点**: 実装不要
**欠点**: テストカバレッジが低下

#### Option 2: Go標準の `t.TempDir()` を使用 ✅

```go
group := runnertypes.GroupSpec{
    Name: "test",
    WorkDir: t.TempDir(),  // Go標準の TempDir を使用
}
```

**利点**:
- 標準的で安全
- 自動クリーンアップ
- 実装が簡単

**欠点**:
- 元の `TempDir: true` の意図と若干異なる可能性

#### Option 3: カスタム実装

```go
tempDir := filepath.Join(os.TempDir(), fmt.Sprintf("test_%d", time.Now().UnixNano()))
os.MkdirAll(tempDir, 0755)
t.Cleanup(func() { os.RemoveAll(tempDir) })
```

**利点**: 柔軟性が高い
**欠点**: 複雑、標準から逸脱

### 決定事項

**✅ 採用する解決策**: Option 2（Go標準の `t.TempDir()` を使用）

#### 使用方法

**Before（エラー）**:
```go
group := runnertypes.GroupSpec{
    Name: "test",
    TempDir: true,  // ❌ フィールドが存在しない
    Commands: []runnertypes.CommandSpec{...},
}
```

**After（修正）**:
```go
group := runnertypes.GroupSpec{
    Name: "test",
    WorkDir: t.TempDir(),  // ✅ Go標準の TempDir
    Commands: []runnertypes.CommandSpec{...},
}
```

#### 特殊ケース

**ケース1**: `TempDir: false` の場合

```go
// Before
group := runnertypes.GroupSpec{
    Name: "test",
    TempDir: false,  // 明示的に false
}

// After
group := runnertypes.GroupSpec{
    Name: "test",
    // WorkDir は設定しない（デフォルト動作）
}
```

**ケース2**: WorkDir と TempDir の両方が指定されている場合

```go
// Before
group := runnertypes.GroupSpec{
    Name: "test",
    WorkDir: "/custom",
    TempDir: true,  // 競合
}

// After - ケースバイケースで判断
// TempDir が優先される場合:
group := runnertypes.GroupSpec{
    Name: "test",
    WorkDir: t.TempDir(),
}
```

### 実装の優先順位

**高**: 全14箇所を個別に確認して修正（Phase 3で実施）

### 推定作業時間

- 1-2時間（各ケースを個別に確認）

---

## 全体の実装戦略

### Phase 2: 基盤整備（推定: 3-5時間）

#### 2.1 モックの拡張（不要）

- ✅ `setupFailedMockExecution` は既に存在
- ✅ `setupSuccessfulMockExecution` も存在
- ✅ `setupDefaultMockBehavior` も存在

#### 2.2 ヘルパー関数の実装（0.5-1時間）

`internal/runner/runner_test.go` に追加:

```go
// createTestRuntimeCommand creates a RuntimeCommand for testing
func createTestRuntimeCommand(spec *runnertypes.CommandSpec, effectiveWorkDir string) *runnertypes.RuntimeCommand {
	return &runnertypes.RuntimeCommand{
		Spec:             spec,
		ExpandedCmd:      spec.Cmd,
		ExpandedArgs:     spec.Args,
		ExpandedEnv:      make(map[string]string),
		ExpandedVars:     make(map[string]string),
		EffectiveWorkDir: effectiveWorkDir,
		EffectiveTimeout: 30,
	}
}
```

#### 2.3 テスト用ユーティリティの整備（不要）

- ✅ TempDir は Go標準の `t.TempDir()` を使用

### Phase 3: 段階的移行（推定: 10-16時間 → 修正: 8-12時間）

エラー数は増えたが、解決策が明確になったため、作業時間は短縮可能。

#### 3.1 簡単なテストから開始（3-4時間）

1. TestNewRunner (行114-178)
   - EffectiveWorkdir: 2箇所（フィールドアクセス）
   - 作業: ヘルパー関数使用に変更

2. TestNewRunnerWithSecurity (行180-221)
   - 比較的シンプル

3. TestRunner_ExecuteCommand (行989-1097)
   - 標準的なテストケース

#### 3.2 中程度のテスト（4-5時間）

4-8. 各テスト関数で:
- SetupFailedMockExecution → setupFailedMockExecution に置換
- TempDir → t.TempDir() に変更
- EffectiveWorkdir → createTestRuntimeCommand 使用

#### 3.3 複雑なテスト（1-3時間）

9-21. 残りのテスト関数

---

## 成功基準

### Phase 1.2 完了基準

- [x] EffectiveWorkdir の解決方針決定
- [x] SetupFailedMockExecution の解決方針決定
- [x] TempDir の解決方針決定
- [x] 設計ドキュメント作成
- [ ] チーム/レビュアーの承認（該当する場合）

### 全体の成功基準

1. ✅ 全65箇所のエラーが解決
2. ✅ 全21個のテスト関数がPASS
3. ✅ コンパイルエラー 0件
4. ✅ `make test` で全テストPASS
5. ✅ `make lint` でエラーなし

---

## リスクと対策

### リスク1: EffectiveWorkdir の設定値が不適切

**対策**: 各テストケースで期待値を確認し、適切なデフォルト値を設定

### リスク2: TempDir の動作が元の意図と異なる

**対策**: テスト失敗時に元のテストの意図を確認し、必要に応じて調整

### リスク3: モック設定の変更でテストが失敗

**対策**: 段階的に移行し、各テストを個別に確認

---

## 推定工数の更新

| Phase | 当初見積もり | Phase 1.1修正 | Phase 1.2修正 | 理由 |
|-------|------------|--------------|--------------|------|
| Phase 1.1 | 1-1.5h | 0.5h (実績) | - | 効率的に実施 |
| Phase 1.2 | 0.5-1h | 1-1.5h | 1h (実績) | エラー数増加 |
| Phase 1.3 | 0.5h | 0.5h | 0.5h | 変更なし |
| Phase 2 | 2-4h | 3-5h | 1-2h | モック既存のため短縮 |
| Phase 3 | 10-16h | 12-20h | 8-12h | 解決策明確化で短縮 |
| Phase 4 | 2-3h | 2-3h | 2-3h | 変更なし |
| **合計** | **16-26h** | **19-31h** | **13-19h** | 設計明確化で短縮 |

---

## 次のステップ（Phase 1.3）

1. 21個のテスト関数を優先順位付け
2. 各テストの複雑度を評価
3. 移行順序を確定

---

**承認**: GitHub Copilot
**日付**: 2025-10-21
**次のアクション**: Phase 1.3（移行計画の詳細化）
