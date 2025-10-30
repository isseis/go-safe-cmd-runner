
# security.Validator コンストラクタの Functional Options Pattern への移行

## 背景

現在、`internal/runner/security` パッケージの `Validator` 型には4つのコンストラクタが存在し、オプショナルな依存関係の組み合わせごとに異なるコンストラクタが提供されている:

1. `NewValidator(config *Config) (*Validator, error)`
   - デフォルトのFileSystemを使用
   - GroupMembershipなし

2. `NewValidatorWithFS(config *Config, fs common.FileSystem) (*Validator, error)`
   - カスタムFileSystemを使用
   - GroupMembershipなし

3. `NewValidatorWithGroupMembership(config *Config, groupMembership *groupmembership.GroupMembership) (*Validator, error)`
   - デフォルトのFileSystemを使用
   - GroupMembershipあり

4. `NewValidatorWithFSAndGroupMembership(config *Config, fs common.FileSystem, groupMembership *groupmembership.GroupMembership) (*Validator, error)`
   - カスタムFileSystemを使用
   - GroupMembershipあり

### 問題点

1. **組み合わせ爆発の懸念**
   - 現在2つのオプショナルパラメータ(FileSystem, GroupMembership)で4つのコンストラクタが必要
   - 将来的にオプショナルパラメータが追加されると、コンストラクタ数が指数関数的に増加する

2. **可読性の低下**
   - コンストラクタ名が長くなり(`NewValidatorWithFSAndGroupMembership`)、可読性が低い
   - 使用箇所でどのコンストラクタを選ぶべきか判断が必要

3. **保守性の課題**
   - すべてのコンストラクタが最終的に `NewValidatorWithFSAndGroupMembership` を呼び出す構造
   - 4つのコンストラクタすべてを保守する必要がある

4. **コードベース内の不統一**
   - 同じコードベース内の `Runner` 型は既に Functional Options Pattern を採用している
   - パターンの不統一により、開発者の認知負荷が増加

### 現在の使用状況

検索結果から、以下の使用パターンが確認された:

- `NewValidator(nil)`: デフォルト設定での使用が多数(テスト、実装コード両方)
- `NewValidatorWithFS()`: テストでのモックFileSystem注入に使用(約13箇所)
- `NewValidatorWithGroupMembership()`: ファイル権限検証のテストに使用(約13箇所)
- `NewValidatorWithFSAndGroupMembership()`: 内部実装でのみ使用(他のコンストラクタから呼び出される)

## 目的

Functional Options Pattern を採用することで、以下を実現する:

1. **拡張性の向上**: 将来的なオプショナルパラメータの追加が容易
2. **可読性の向上**: オプション名で意図が明確になる
3. **保守性の向上**: 単一のコンストラクタのみを保守
4. **一貫性の確保**: `Runner` 型と同じパターンを採用し、コードベース全体の一貫性を向上

## 改善案

### 採用パターン: Functional Options Pattern

Go言語のベストプラクティスに従い、Functional Options Pattern を採用する。このパターンは以下の理由で選択した:

1. **Uber Go Style Guide推奨**: 業界標準のGoスタイルガイドで推奨されている
2. **後方互換性**: 既存のコンストラクタをラッパーとして残すことで段階的な移行が可能
3. **コードベース内での一貫性**: 既に `Runner` 型で使用されているパターン
4. **柔軟性**: オプションの追加・変更が容易

### 新しいAPI設計

```go
// 新しい単一コンストラクタ
func NewValidator(config *Config, opts ...Option) (*Validator, error)

// Option 関数
func WithFileSystem(fs common.FileSystem) Option
func WithGroupMembership(gm *groupmembership.GroupMembership) Option
```

### 使用例

```go
// デフォルト設定
validator, err := security.NewValidator(nil)

// カスタムFileSystem
validator, err := security.NewValidator(nil,
    security.WithFileSystem(mockFS))

// GroupMembership
validator, err := security.NewValidator(nil,
    security.WithGroupMembership(gm))

// 両方のオプション
validator, err := security.NewValidator(nil,
    security.WithFileSystem(mockFS),
    security.WithGroupMembership(gm))
```

## 移行戦略

### フェーズ1: 新しいAPIの追加

1. `Option` 型と option functions の追加
2. 新しい `NewValidator(config *Config, opts ...Option)` の実装
3. 既存の4つのコンストラクタを新しいAPIのラッパーとして実装し、Deprecated マークを追加

### フェーズ2: 段階的な置き換え

1. テストコードでの置き換え
2. 実装コードでの置き換え
3. ドキュメントの更新

### フェーズ3: クリーンアップ(将来的なマイルストーン)

1. Deprecated なコンストラクタの削除
2. 最終的な動作確認

## 影響範囲

### 変更が必要なファイル

#### 実装ファイル
- `internal/runner/security/validator.go`: 新しいAPIの実装

#### 使用箇所(実装コード)
- `internal/runner/runner.go`: `NewValidator(nil)` 使用
- `internal/runner/resource/default_manager.go`: `NewValidator(nil)` 使用
- `internal/runner/config/validator.go`: `NewValidator(secConfig)` 使用
- `internal/runner/security/environment_validation.go`: `NewValidator(nil)` 使用
- `internal/verification/manager.go`: `NewValidatorWithFS()` 使用

#### 使用箇所(テストコード)
- 約40個のテストファイルで使用されている
- 主にテスト用のモック注入のため `NewValidatorWithFS()` や `NewValidatorWithGroupMembership()` を使用

### 後方互換性

既存のコンストラクタをラッパーとして残すため、既存コードは引き続き動作する。段階的な移行が可能。

## 参考資料

### コードベース内の類似実装
- `internal/runner/runner.go`: 既に Functional Options Pattern を採用
  - `WithExecutor()`, `WithPrivilegeManager()`, `WithRunID()` など

### Go言語のベストプラクティス
- [Uber Go Style Guide - Functional Options](https://github.com/uber-go/guide/blob/master/style.md#functional-options)
- Rob Pike's blog: "Self-referential functions and the design of options"
