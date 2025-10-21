# Task 0039 Phase 1.1: 現状分析レポート

**作成日**: 2025-10-21
**作成者**: GitHub Copilot
**分析対象**: `internal/runner/runner_test.go`

## エグゼクティブサマリー

runner_test.go からビルドタグ `//go:build skip_integration_tests` を削除し、コンパイルエラーを分析した結果、**65個のエラー箇所**が確認されました。

### エラー内訳

| エラーカテゴリ | 箇所数 | 当初予想 | 差分 |
|-------------|-------|---------|------|
| EffectiveWorkdir | 35 | ~25 | +10 |
| SetupFailedMockExecution | 16 | ~8 | +8 |
| TempDir | 14 | ~10 | +4 |
| **合計** | **65** | **~43** | **+22** |

## 詳細分析

### 1. EffectiveWorkdir エラー（35箇所）

#### 問題の本質

`CommandSpec` に `EffectiveWorkdir` フィールドが存在しない。このフィールドは `RuntimeCommand` に属している。

#### エラーパターン

**パターン1: フィールドアクセス** (2箇所)
```go
// 行244, 246
expectedCmd.EffectiveWorkdir = config.Global.WorkDir
expectedCmd.EffectiveWorkdir = expectedCmd.WorkDir
```

**パターン2: 構造体リテラル** (33箇所)
```go
// 多数の箇所
runnertypes.CommandSpec{
    Name: "cmd-1",
    Cmd: "false",
    EffectiveWorkdir: "/tmp",  // ❌ このフィールドは存在しない
}
```

#### 影響するテスト関数

- TestNewRunner (行244, 246)
- TestRunner_ExecuteGroup (行294, 331, 335, 373, 377)
- TestRunner_ExecuteAll (行424, 425, 475, 479, 482, 532, 536, 540)
- TestRunner_ExecuteAllWithPriority (行584, 588, 593, 635, 639)
- その他多数

### 2. SetupFailedMockExecution エラー（16箇所）

#### 問題の本質

`MockResourceManager` に `SetupFailedMockExecution` メソッドが存在しない。

#### エラー例

```go
// 未定義メソッドの呼び出し
mockRM.SetupFailedMockExecution(errors.New("test error"))
```

#### 解決策の方向性

1. **Option 1**: `SetupFailedMockExecution` メソッドを `MockResourceManager` に追加
2. **Option 2**: 直接 `mockRM.On()` を使用して個別にモック設定

**推奨**: Option 2（直接モック設定）
- より柔軟
- テストケースごとにカスタマイズ可能
- 既存の testify/mock パターンに準拠

### 3. TempDir エラー（14箇所）

#### 問題の本質

`GroupSpec` に `TempDir` フィールドが存在しない。

#### エラー例

```go
GroupSpec{
    Name: "test",
    TempDir: true,  // ❌ このフィールドは存在しない
}
```

#### 解決策の方向性

1. **Option 1**: テストをスキップ（`t.Skip("TempDir feature not yet implemented")`）
2. **Option 2**: Go標準の `t.TempDir()` で代替
3. **Option 3**: `WorkDir` フィールドで代替実装
4. **Option 4**: `GroupSpec` に `TempDir` 機能を実装（Task 0040）

**推奨**: Option 2 または Option 3
- Option 2: 簡単で標準的
- Option 3: より現実的な実装

## Phase 1.1 完了時の作業内容

### 実施した作業

1. ✅ output_capture_integration_test.go のビルドタグ不足を発見・修正
   - `//go:build test` タグを追加
   - 2つのテスト関数がPASS
   - コミット完了

2. ✅ runner_test.go からビルドタグを削除
   - `//go:build skip_integration_tests` を削除
   - コンパイルエラーを確認

3. ✅ 全エラーを収集・分類
   - `-gcflags=-e` オプションで全エラーを収集
   - `/tmp/runner_test_errors_full.log` に保存
   - エラーカテゴリ別に分類

### 時間記録

- output_capture_integration_test.go 修正: 15分
- runner_test.go エラー分析: 15分
- **Phase 1.1 合計**: 約30分

## 次のステップ（Phase 1.2）

### 設計方針の決定事項

以下の設計決定を行う必要があります：

#### 1. EffectiveWorkdir の扱い方

**決定すべき事項**:
- `CommandSpec` から `RuntimeCommand` への変換方法
- テストコードでの `EffectiveWorkdir` の設定方法
- ヘルパー関数の実装方針

**候補案**:
```go
// 案1: ヘルパー関数を作成
func createRuntimeCommand(spec *runnertypes.CommandSpec) *runnertypes.RuntimeCommand {
    return &runnertypes.RuntimeCommand{
        Spec:             spec,
        ExpandedCmd:      spec.Cmd,
        ExpandedArgs:     spec.Args,
        ExpandedEnv:      make(map[string]string),
        ExpandedVars:     make(map[string]string),
        EffectiveWorkDir: spec.WorkDir,  // または適切なデフォルト値
        EffectiveTimeout: 30,
    }
}

// 案2: モック設定で RuntimeCommand を直接使用
mockRM.On("ExecuteCommand", mock.Anything, mock.MatchedBy(func(cmd *runnertypes.RuntimeCommand) bool {
    return cmd.Spec.Name == "cmd-1" && cmd.EffectiveWorkDir == "/tmp"
}), mock.Anything, mock.Anything).Return(...)
```

#### 2. SetupFailedMockExecution の実装方針

**決定すべき事項**:
- メソッドを追加するか、直接モック設定するか
- エラーパターンの標準化

**推奨**: 直接モック設定
```go
// Before
mockRM.SetupFailedMockExecution(errors.New("test error"))

// After
mockRM.On("ExecuteCommand", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
    Return(nil, errors.New("test error"))
```

#### 3. TempDir の代替実装

**決定すべき事項**:
- どのオプションを採用するか
- 全テストで統一した方法を使うか

**推奨**: Option 2（Go標準のt.TempDir()）
```go
// Before
group := runnertypes.GroupSpec{
    Name: "test",
    TempDir: true,
}

// After
group := runnertypes.GroupSpec{
    Name: "test",
    WorkDir: t.TempDir(),  // Go標準のTempDirを使用
}
```

## 推定工数の見直し

### 当初見積もり vs 実態

| Phase | 当初見積もり | 実績/修正見積もり | 備考 |
|-------|------------|---------------|------|
| Phase 1.1 | 1-1.5h | 0.5h (実績) | output_capture修正含む |
| Phase 1.2 | 0.5-1h | 1-1.5h (修正) | エラー数増加のため |
| Phase 1.3 | 0.5h | 0.5h | 変更なし |
| Phase 2 | 2-4h | 3-5h (修正) | エラー数増加のため |
| Phase 3 | 10-16h | 12-20h (修正) | エラー数増加のため |
| Phase 4 | 2-3h | 2-3h | 変更なし |
| **合計** | **16-26h** | **19-31h** | +3-5h |

### リスク評価

#### 新たに特定されたリスク

1. **EffectiveWorkdir エラーが予想より多い**
   - 影響: 作業時間が増加
   - 対策: ヘルパー関数で一括変換

2. **SetupFailedMockExecution の使用箇所が多い**
   - 影響: 個別にモック設定が必要
   - 対策: パターン化してコピー&ペースト

3. **TempDir の代替実装が必要**
   - 影響: 設計判断が必要
   - 対策: Option 2（t.TempDir()）を標準とする

## 結論

Phase 1.1 は予定より早く完了しましたが、エラー数が予想を上回ったため、Phase 1.2 以降の作業時間を見直す必要があります。

**重要な発見**:
- output_capture_integration_test.go のビルドタグ不足を発見・修正
- runner_test.go のエラーは 65箇所（当初予想 ~48箇所から+17箇所）
- 3つのエラーカテゴリがほぼ均等に分布

**Phase 1.2 への準備完了**:
- 全エラーの詳細リスト作成済み（/tmp/runner_test_errors_full.log）
- エラーカテゴリ別の分類完了
- 解決策の方向性を明確化

---

**次のアクション**: Phase 1.2（設計方針の決定）を開始
