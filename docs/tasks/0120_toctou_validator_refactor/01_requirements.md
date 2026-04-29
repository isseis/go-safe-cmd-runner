# TOCTOU Validator リファクタリング要件書

## 概要

`security.NewValidator(nil, WithGroupMembership(...))` で初期化される`Validator` 全体から、TOCTOU権限チェック専用の軽量な実装を切り出し、不要な初期化処理（200+ 行）を削減する。

## 背景

### 現在の問題

1. **不要な初期化コスト**
   [cmd/runner/main.go:339](cmd/runner/main.go#L339) で TOCTOU権限チェック専用に `NewValidator(nil, WithGroupMembership(...))` を呼び出している

2. **初期化時に実行されるが未使用の処理**
   - AllowedCommands 正規表現コンパイル（70+ 行）
   - SensitiveEnvVars 正規表現コンパイル（70+ 行）
   - DangerousPrivilegedCommands マップ作成
   - ShellCommands マップ作成
   - DangerousRootPatterns 検証
   - redaction.Config / SensitivePatterns 初期化

3. **本番コードで必要なのは**
   - ファイルシステム操作（Lstat, stat）
   - グループメンバーシップ検証（`CanUserSafelyWriteFile()`）
   - 小数の設定値（trustedGIDs, testPermissiveMode）

### `NewValidator(nil)` の呼び出し箇所

#### 本番コード
- `internal/runner/security/toctou_check.go:12` - **TOCTOU チェック専用** ← 切り出し対象

#### テストコード（21 箇所）
- `internal/runner/security/` 配下のテスト（6 ファイル）
- `test/security/` 配下のテスト（3 ファイル）
- これらはすべて `DefaultConfig()` を活用した統合テスト

**結論**: 本番コードでは **TOCTOU チェックの 1 箇所だけ** で `nil` が渡されている

## 受理基準

### 1. 新しい軽量な DirectoryPermChecker 実装の作成

- [ ] `internal/security/directory_perm_checker.go` を新規作成
  - `NewDirectoryPermCheckerForTOCTOU()` ファクトリ関数を提供
  - `DirectoryValidator` インタフェース（`internal/verification/types.go:96`）を実装
  - 以下のフィールドのみを保持
    - `fs`: `common.FileSystem`
    - `groupMembership`: `*groupmembership.GroupMembership`
    - `trustedGIDs`: `map[uint32]struct{}`
    - `testPermissiveMode`: `bool`

- [ ] `internal/security/directory_perm_checker.go` で実装する処理
  - `ValidateDirectoryPermissions(dirPath string) error` メソッド
  - struct フィールドへのアクセスのため、ヘルパー関数を呼び出す

- [ ] 共通のロジック関数を `internal/security/` にパッケージレベル関数として抽出
  - `validateCompletePath(fs FileSystem, groupMembership *GroupMembership, config *permValidationConfig, cleanPath, originalPath string, realUID int) error`
  - `validateDirectoryComponentMode(fs FileSystem, dirPath string, info os.FileInfo) error`
  - `validateDirectoryComponentPermissions(fs FileSystem, groupMembership *GroupMembership, config *permValidationConfig, dirPath string, info os.FileInfo, realUID int) error`
  - `validateGroupWritePermissions(groupMembership *GroupMembership, config *permValidationConfig, dirPath string, info os.FileInfo, realUID int) error`
  - `isStickyDirectory(info os.FileInfo) bool`
  - `isTrustedGroup(config *permValidationConfig, gid uint32) bool`

- [ ] 設定構造体 `permValidationConfig` を定義
  ```go
  type permValidationConfig struct {
      trustedGIDs       map[uint32]struct{}
      testPermissiveMode bool
  }
  ```

### 2. `toctou_check.go` の更新

- [ ] `NewValidatorForTOCTOU()` を新しい `NewDirectoryPermCheckerForTOCTOU()` に置き換え
- [ ] 戻り値型を `(*Validator, error)` から `(DirectoryValidator, error)` に変更
- [ ] 下位互換性のため、古い `NewValidatorForTOCTOU()` は **廃止予定 (deprecated)** とマーク

### 3. `cmd/runner/main.go` の更新

- [ ] `security.NewValidatorForTOCTOU()` の呼び出しを新しい実装に置き換え
- [ ] 変数型を必要に応じて `DirectoryValidator` インタフェース型に調整

### 4. `Validator.NewValidator()` の簡略化

- [ ] 本番コードで `config = nil` が渡されなくなったことを確認
- [ ] `newValidatorCore()` で `if config == nil { config = DefaultConfig() }` の条件を削除
- [ ] **前提**: `NewValidator()` は常に `config != nil` で呼び出されることを保証

### 5. テストの更新

- [ ] `internal/runner/security/toctou_check_test.go` を新規作成（必要な場合）
  - `NewDirectoryPermCheckerForTOCTOU()` のテストケース
  - `DirectoryValidator` インタフェースの実装確認

- [ ] 既存のテストコードは変更なし
  - `NewValidator(nil)` は引き続き機能する（互換性維持）

## 実装の成果

| 項目 | 効果 |
|-----|------|
| 初期化処理の削減 | 200+ 行の不要なコンパイル処理を回避 |
| コードの明確性 | TOCTOU チェック専用の軽量な実装で意図が明確 |
| メモリ使用量 | 不要なフィールド（regex 配列、map）の割り当てを削減 |
| 保守性 | `Validator` の責務がより明確になる |
| 互換性 | テストコードへの影響なし、既存の `DirectoryValidator` インタフェースを活用 |

## スコープ（対象外）

- Validator 自体の機能削減（他の用途で必要な AllowedCommands, SensitiveEnvVars チェックは維持）
- 既存のテストコードの大規模な変更
- ドキュメント更新（必要に応じて docs/ へのコメント追加のみ）

## 設計上の決定

### 1. 既存の `DirectoryValidator` インタフェースを再利用

```go
// internal/verification/types.go:96
type DirectoryValidator interface {
    ValidateDirectoryPermissions(dirPath string) error
}
```

- 既に `internal/verification/manager.go` で使用されている
- 新しい実装も同じインタフェースを実装することで統一性を確保

### 2. ロジック重複の回避（共通関数への抽出）

- `internal/runner/security/file_validation.go` の既存メソッドロジックを、パッケージレベルの共通関数として抽出
- `Validator` と `dirPermChecker` の両方で同じ関数を呼び出す
- 利点:
  - コード保守性向上（バグ修正は 1 箇所で完結）
  - 一貫性確保（両実装で同じセキュリティチェック）
  - テスト効率化（共通関数単位でテスト可能）

### 3. テストコードへの影響最小化

- `NewValidator(nil)` は引き続き機能（互換性維持）
- テストコードは既存のまま
- 新しい実装用のテストは別途追加

## 実装のステップ

1. **Phase 1**: `internal/runner/security/file_validation.go` から共通ロジック関数を抽出
   - パッケージレベル関数として定義
   - `permValidationConfig` 構造体を定義
   - 既存の `Validator` メソッドを新しい共通関数で置き換え（リファクタリング）

2. **Phase 2**: `internal/security/directory_perm_checker.go` の新規作成
   - `dirPermChecker` struct 定義
   - `ValidateDirectoryPermissions()` メソッド実装
   - Phase 1 で抽出した共通関数を呼び出す

3. **Phase 3**: `toctou_check.go` の更新
   - `NewDirectoryPermCheckerForTOCTOU()` を実装
   - 戻り値型を `DirectoryValidator` に変更
   - 古い `NewValidatorForTOCTOU()` は deprecated とマーク

4. **Phase 4**: `cmd/runner/main.go` の更新
   - `NewDirectoryPermCheckerForTOCTOU()` に切り替え

5. **Phase 5**: `Validator.NewValidator()` の簡略化
   - `config = nil` チェック削除

6. **Phase 6**: テスト追加・検証
