# Task 0037: 残りの統合テストの型移行

## 概要

Task 0036 (`runner_test.go`) の成果を活用し、残りの統合テストファイルを新しい型システムに移行します。

## 進捗状況

### ✅ 完了
- **`internal/runner/output_capture_integration_test.go`** (227行)
  - `package runner_test` → `package runner` に変更
  - `Config`/`GlobalConfig`/`CommandGroup`/`Command` → `ConfigSpec`/`GlobalSpec`/`GroupSpec`/`CommandSpec` に移行
  - ヘルパー関数（`setupSafeTestEnv`, `MockResourceManager`）を追加
  - 全テスト PASS

### 🔄 残作業

1. **`test/performance/output_capture_test.go`** (411行)
   - 推定工数: 4-6時間
   - 複雑度: 中

2. **`test/security/output_security_test.go`** (535行)
   - 推定工数: 6-8時間
   - 複雑度: 高

## 詳細

### 完了したファイル: output_capture_integration_test.go

#### 実施した変更

1. **パッケージ変更**
```go
// Before
//go:build skip_integration_tests
package runner_test

// After
package runner
```

2. **型の移行**
```go
// Before
cfg := &runnertypes.Config{
	Global: runnertypes.GlobalConfig{...},
	Groups: []runnertypes.CommandGroup{...},
}

// After
cfg := &runnertypes.ConfigSpec{
	Version: "1.0",
	Global: runnertypes.GlobalSpec{...},
	Groups: []runnertypes.GroupSpec{...},
}
```

3. **ヘルパー関数の追加**
```go
// MockResourceManager alias
type MockResourceManager = runnertesting.MockResourceManager

// Test environment setup
func setupSafeTestEnv(t *testing.T) {
	// ...
}
```

4. **ExecuteGroup 呼び出しの修正**
```go
// Before
err = runner.ExecuteGroup(ctx, cfg.Groups[0])

// After
err = runner.ExecuteGroup(ctx, &cfg.Groups[0])
```

#### テスト結果
```
=== RUN   TestRunner_OutputCaptureIntegration
=== RUN   TestRunner_OutputCaptureIntegration/BasicOutputCapture
=== RUN   TestRunner_OutputCaptureIntegration/OutputCaptureError
--- PASS: TestRunner_OutputCaptureIntegration (0.00s)
=== RUN   TestRunner_OutputCaptureSecurityValidation
=== RUN   TestRunner_OutputCaptureSecurityValidation/PathTraversalAttempt
=== RUN   TestRunner_OutputCaptureSecurityValidation/AbsolutePathBlocked
=== RUN   TestRunner_OutputCaptureSecurityValidation/ValidOutputPath
--- PASS: TestRunner_OutputCaptureSecurityValidation (0.00s)
PASS
```

### 残作業1: test/performance/output_capture_test.go

#### 現状分析
- **行数**: 411行
- **テスト関数**: 5個
- **主な課題**:
  1. `PrepareCommand()` メソッドの削除への対応
  2. `Command` → `RuntimeCommand` への変換
  3. `CommandGroup` → `GroupSpec` への変換
  4. パフォーマンス測定コードの動作確認

#### 使用されている古い型・メソッド

**削除されたメソッド**:
```go
runnertypes.PrepareCommand(&cmd)  // 削除済み
```

**古い型**:
```go
cmd := runnertypes.Command{...}        // → CommandSpec or RuntimeCommand
group := &runnertypes.CommandGroup{...} // → GroupSpec
```

#### 移行パターン

**Before**:
```go
cmd := runnertypes.Command{
	Name:   "large_output_test",
	Cmd:    "sh",
	Args:   []string{"-c", "yes 'A' | head -c 10240"},
	Output: outputPath,
}
runnertypes.PrepareCommand(&cmd)

group := &runnertypes.CommandGroup{Name: "test_group"}

manager := resource.NewNormalResourceManager(exec, fs, privMgr, logger)
result, err := manager.ExecuteCommand(ctx, cmd, group, map[string]string{})
```

**After**:
```go
// CommandSpec を作成
cmdSpec := &runnertypes.CommandSpec{
	Name:   "large_output_test",
	Cmd:    "sh",
	Args:   []string{"-c", "yes 'A' | head -c 10240"},
	Output: outputPath,
}

// GroupSpec を作成
groupSpec := &runnertypes.GroupSpec{Name: "test_group"}

// RuntimeCommand に変換（変数展開が必要な場合）
runtimeCmd := &runnertypes.RuntimeCommand{
	Spec:         cmdSpec,
	ExpandedCmd:  cmdSpec.Cmd,  // 変数展開なしの場合はそのままコピー
	ExpandedArgs: cmdSpec.Args,
	ExpandedEnv:  make(map[string]string),
	ExpandedVars: make(map[string]string),
	EffectiveWorkDir: "",
	EffectiveTimeout: 30, // デフォルト値
}

manager := resource.NewNormalResourceManager(exec, fs, privMgr, logger)
result, err := manager.ExecuteCommand(ctx, runtimeCmd, groupSpec, map[string]string{})
```

#### 推奨手順

1. `PrepareCommand()` の削除に対応
2. 全テスト関数の型を移行
3. パフォーマンス測定が正しく動作することを確認
4. ベンチマークテストを実行

### 残作業2: test/security/output_security_test.go

#### 現状分析
- **行数**: 535行
- **推定テスト関数**: 8-10個
- **主な課題**:
  1. `Command` → `RuntimeCommand` への大規模変換
  2. セキュリティバリデーション APIの変更への対応
  3. パス検証ロジックの動作確認

#### 推定作業

1. **型変換**: 100-150箇所
2. **APIの更新**: セキュリティバリデーションメソッドの変更に対応
3. **テスト環境整備**: セキュリティテスト特有の設定

## 移行戦略

### オプション1: 段階的移行（推奨）

1. **Phase 1**: `test/performance/output_capture_test.go` の移行（4-6時間）
2. **Phase 2**: `test/security/output_security_test.go` の移行（6-8時間）
3. **Phase 3**: 全テスト実行と検証（2-3時間）

**合計推定時間**: 12-17時間（2日）

### オプション2: 並行作業

2つのファイルを別々のブランチで並行して移行することも可能。

## 成功基準

1. ✅ `output_capture_integration_test.go` の全テスト PASS（完了）
2. ⏳ `test/performance/output_capture_test.go` の全テスト PASS
3. ⏳ `test/security/output_security_test.go` の全テスト PASS
4. ⏳ `make test` で全テスト PASS
5. ⏳ `make lint` でエラーなし
6. ⏳ すべての `skip_integration_tests` タグが削除されている

## 次のステップ

### 推奨作業順序

1. **`test/performance/output_capture_test.go` の移行**
   - Task 0036 の移行パターンを適用
   - パフォーマンステストの動作確認

2. **`test/security/output_security_test.go` の移行**
   - セキュリティ API の変更に注意
   - セキュリティバリデーションの動作確認

3. **最終検証**
   - すべてのテスト実行
   - カバレッジレポート確認

## 参考資料

- [完了済み: output_capture_integration_test.go](../../internal/runner/output_capture_integration_test.go)
- [Task 0036: runner_test.go 移行ガイド](../0036_runner_test_migration/)
- [group_executor_test.go 移行例](../../internal/runner/group_executor_test.go)
