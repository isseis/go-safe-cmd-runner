# Migration Guide: security.Validator Functional Options Pattern

## 概要

このガイドは、`security.Validator` の古いコンストラクタAPIから新しい Functional Options Pattern への移行方法を説明します。

## 変更の背景

`security.Validator` のコンストラクタは、複数の専用コンストラクタから単一の柔軟なコンストラクタへと変更されました。これにより、以下の利点が得られます：

- コードの保守性向上
- 将来的な拡張の容易さ
- `Runner` 型との一貫性
- より読みやすいコード

## 移行例

### パターン 1: デフォルトのValidator

変更なし。現在のコードはそのまま動作します。

```go
// Before (変更不要)
validator, err := security.NewValidator(nil)

// After (同じ)
validator, err := security.NewValidator(nil)
```

### パターン 2: カスタムFileSystemの使用

主にテストコードで使用されるパターン。

```go
// Before (非推奨)
validator, err := security.NewValidatorWithFS(config, mockFS)

// After (推奨)
validator, err := security.NewValidator(config,
    security.WithFileSystem(mockFS))
```

#### 実例: テストコードの移行

```go
// Before
func TestSomething(t *testing.T) {
    mockFS := &MockFileSystem{}
    validator, err := security.NewValidatorWithFS(nil, mockFS)
    require.NoError(t, err)
    // ...
}

// After
func TestSomething(t *testing.T) {
    mockFS := &MockFileSystem{}
    validator, err := security.NewValidator(nil,
        security.WithFileSystem(mockFS))
    require.NoError(t, err)
    // ...
}
```

### パターン 3: GroupMembershipの使用

ファイル権限検証のテストで使用されるパターン。

```go
// Before (非推奨)
validator, err := security.NewValidatorWithGroupMembership(config, gm)

// After (推奨)
validator, err := security.NewValidator(config,
    security.WithGroupMembership(gm))
```

#### 実例: 権限検証テストの移行

```go
// Before
func TestPermissionValidation(t *testing.T) {
    gm := groupmembership.NewMock(1000, 1000)
    validator, err := security.NewValidatorWithGroupMembership(nil, gm)
    require.NoError(t, err)
    // ...
}

// After
func TestPermissionValidation(t *testing.T) {
    gm := groupmembership.NewMock(1000, 1000)
    validator, err := security.NewValidator(nil,
        security.WithGroupMembership(gm))
    require.NoError(t, err)
    // ...
}
```

### パターン 4: 両方のオプションを使用

FileSystemとGroupMembershipの両方をカスタマイズする場合。

```go
// Before (非推奨)
validator, err := security.NewValidatorWithFSAndGroupMembership(config, mockFS, gm)

// After (推奨)
validator, err := security.NewValidator(config,
    security.WithFileSystem(mockFS),
    security.WithGroupMembership(gm))
```

#### 実例: 複合的なテストの移行

```go
// Before
func TestComplexScenario(t *testing.T) {
    mockFS := &MockFileSystem{}
    gm := groupmembership.NewMock(1000, 1000)
    validator, err := security.NewValidatorWithFSAndGroupMembership(nil, mockFS, gm)
    require.NoError(t, err)
    // ...
}

// After
func TestComplexScenario(t *testing.T) {
    mockFS := &MockFileSystem{}
    gm := groupmembership.NewMock(1000, 1000)
    validator, err := security.NewValidator(nil,
        security.WithFileSystem(mockFS),
        security.WithGroupMembership(gm))
    require.NoError(t, err)
    // ...
}
```

## 一括移行のためのsedコマンド

大量のファイルを一括で移行する場合、以下のsedコマンドが使用できます：

### FileSystemのみのケース

```bash
# NewValidatorWithFS(config, mockFS) -> NewValidator(config, security.WithFileSystem(mockFS))
find . -name "*.go" -type f -exec sed -i \
  's/security\.NewValidatorWithFS(\([^,]*\), \([^)]*\))/security.NewValidator(\1, security.WithFileSystem(\2))/g' {} +
```

### GroupMembershipのみのケース

```bash
# NewValidatorWithGroupMembership(config, gm) -> NewValidator(config, security.WithGroupMembership(gm))
find . -name "*.go" -type f -exec sed -i \
  's/security\.NewValidatorWithGroupMembership(\([^,]*\), \([^)]*\))/security.NewValidator(\1, security.WithGroupMembership(\2))/g' {} +
```

### 両方のケース

```bash
# NewValidatorWithFSAndGroupMembership(config, mockFS, gm) ->
# NewValidator(config, security.WithFileSystem(mockFS), security.WithGroupMembership(gm))
find . -name "*.go" -type f -exec sed -i \
  's/security\.NewValidatorWithFSAndGroupMembership(\([^,]*\), \([^,]*\), \([^)]*\))/security.NewValidator(\1, security.WithFileSystem(\2), security.WithGroupMembership(\3))/g' {} +
```

**注意**: 必ず以下を実行してください：

1. 変更前にgitコミットまたはバックアップを作成
2. sedコマンド実行後に `make test` でテストが通ることを確認
3. `make lint` でコード品質チェックを実行
4. 手動で変更内容を確認

## 非推奨API一覧

以下のコンストラクタは非推奨ですが、後方互換性のため現在も使用可能です：

| 非推奨API | 推奨される代替API |
|----------|------------------|
| `NewValidatorWithFS(config, fs)` | `NewValidator(config, WithFileSystem(fs))` |
| `NewValidatorWithGroupMembership(config, gm)` | `NewValidator(config, WithGroupMembership(gm))` |
| `NewValidatorWithFSAndGroupMembership(config, fs, gm)` | `NewValidator(config, WithFileSystem(fs), WithGroupMembership(gm))` |

## 移行のタイムライン

- **Phase 1 (完了)**: 新しいAPIの追加、既存APIを非推奨としてマーク
- **Phase 2 (完了)**: 既存コードの段階的な移行
- **Phase 3 (完了)**: ドキュメントとマイグレーションガイドの更新
- **Phase 4 (将来)**: 非推奨APIの削除（十分な猶予期間後）

## よくある質問

### Q1: 既存のコードを移行する必要がありますか？

A: 現時点では必須ではありません。既存の非推奨APIは後方互換性のため継続して動作します。ただし、新しいコードでは新しいAPIを使用することを推奨します。

### Q2: 移行のメリットは何ですか？

A:
- より読みやすく、保守しやすいコード
- 将来的な拡張への対応が容易
- `Runner` 型との一貫性により、コードベース全体の学習コストが低減
- オプションの順序が自由（可読性向上）

### Q3: 複数のオプションを使う場合、順序は重要ですか？

A: いいえ。オプションは任意の順序で指定でき、最終的な結果は同じです：

```go
// これらは同じ結果
validator, _ := security.NewValidator(config,
    security.WithFileSystem(mockFS),
    security.WithGroupMembership(gm))

validator, _ := security.NewValidator(config,
    security.WithGroupMembership(gm),
    security.WithFileSystem(mockFS))
```

### Q4: 新しいオプションを追加する予定はありますか？

A: 現時点では具体的な計画はありませんが、Functional Options Patternにより、将来的な拡張が容易になっています。新しいオプションが追加されても、既存のコードには影響しません。

### Q5: テスト以外で WithFileSystem や WithGroupMembership を使う必要はありますか？

A: 通常はありません。これらのオプションは主にテストでモックを注入するために設計されています。本番コードでは通常 `NewValidator(config)` または `NewValidator(nil)` で十分です。

## サポート

質問や問題がある場合は、プロジェクトのIssue trackerで報告してください。

## 参考資料

- [実装計画書](./02_implementation_plan.md)
- [要件定義書](./01_requirements.md)
- [Uber Go Style Guide - Functional Options](https://github.com/uber-go/guide/blob/master/style.md#functional-options)
