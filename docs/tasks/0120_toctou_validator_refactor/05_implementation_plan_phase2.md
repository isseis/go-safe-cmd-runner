# 実装計画書: TOCTOU チェックの実装重複を解消する（フェーズ 2）

## 現状確認

フェーズ 1 完了後の状態：

- `isec.ValidateDirectoryPermissionsWithOptions` が `internal/security/dir_permissions_core.go` に実装済み
- `isec.dirPermChecker.ValidateDirectoryPermissions` と `Validator.ValidateDirectoryPermissions` の双方が委譲済み ✓
- **残存する重複**: `file_validation.go` の private メソッド群が `dir_permissions_core.go` と同一ロジックを保持している

### 残存する重複メソッド（`internal/runner/security/file_validation.go`）

| メソッド | 呼び出し元 | `dir_permissions_core.go` の対応 |
|---|---|---|
| `validateCompletePath` | `validateOutputDirectoryAccess` | `validateDirectoryHierarchy` |
| `validateDirectoryComponentMode` | `validateCompletePath` | `validateDirectoryComponentMode` |
| `isStickyDirectory` | `validateDirectoryComponentPermissions` | `isStickyDirectoryMode` |
| `validateDirectoryComponentPermissions` | `validateCompletePath` | `validateDirectoryComponentPermissions` |
| `validateGroupWritePermissions` | `validateDirectoryComponentPermissions` | `validateGroupWritePermissions` |

これらは `validateOutputDirectoryAccess` → `validateCompletePath` の経路でのみ使われる。

---

## 変更対象の概要

| ファイル | 変更種別 | 変更内容 |
|---|---|---|
| `internal/runner/security/file_validation.go` | 修正 | `validateOutputDirectoryAccess` を `ValidateDirectoryPermissionsWithOptions` に切り替え、重複 private メソッドを削除 |
| `internal/runner/security/file_validation_test.go` | 修正 | private メソッドを直接呼ぶテストを public API 経由に移行、重複テストを削除 |

---

## タスクリスト

### Step 1: `file_validation.go` を修正する

#### 1-1: `buildDirPermOpts` ヘルパーを追加する

`ValidateDirectoryPermissions` と `validateOutputDirectoryAccess` の双方が同一の `DirectoryPermCheckOptions` を構築するため、ヘルパーメソッドとして抽出する。

```go
func (v *Validator) buildDirPermOpts(realUID int) isec.DirectoryPermCheckOptions {
    opts := isec.DirectoryPermCheckOptions{
        Lstat:              v.fs.Lstat,
        MaxPathLength:      v.config.MaxPathLength,
        RealUID:            realUID,
        TestPermissiveMode: v.config.testPermissiveMode,
        IsTrustedGroup: func(gid uint32) bool {
            return v.isTrustedGroup(gid)
        },
    }
    if v.groupMembership != nil {
        opts.CanUserSafelyWrite = func(uid int, ownerUID uint32, groupGID uint32, mode os.FileMode) (bool, error) {
            return v.groupMembership.CanUserSafelyWriteFile(uid, ownerUID, groupGID, mode)
        }
    }
    return opts
}
```

- [x] `buildDirPermOpts` ヘルパーを追加する
- [x] `ValidateDirectoryPermissions` がこのヘルパーを使うよう更新する

  ```go
  // before
  func (v *Validator) ValidateDirectoryPermissions(dirPath string) error {
      realUID := os.Getuid()
      opts := isec.DirectoryPermCheckOptions{ ... }
      ...
      return isec.ValidateDirectoryPermissionsWithOptions(dirPath, opts)
  }

  // after
  func (v *Validator) ValidateDirectoryPermissions(dirPath string) error {
      return isec.ValidateDirectoryPermissionsWithOptions(dirPath, v.buildDirPermOpts(os.Getuid()))
  }
  ```

#### 1-2: `validateOutputDirectoryAccess` の `validateCompletePath` 呼び出しを切り替える

- [x] `validateCompletePath(resolvedPath, currentPath, realUID)` の呼び出しを `isec.ValidateDirectoryPermissionsWithOptions(resolvedPath, v.buildDirPermOpts(realUID))` に置き換える

  ```go
  // before
  if err := v.validateCompletePath(resolvedPath, currentPath, realUID); err != nil {
      return fmt.Errorf("directory security validation failed for %s: %w", currentPath, err)
  }

  // after
  if err := isec.ValidateDirectoryPermissionsWithOptions(resolvedPath, v.buildDirPermOpts(realUID)); err != nil {
      return fmt.Errorf("directory security validation failed for %s: %w", currentPath, err)
  }
  ```

  `realUID` はテストで任意の値を注入できるため `ValidateDirectoryPermissions`（`os.Getuid()` を固定で使用する）への単純な置き換えはできない。

#### 1-3: 使用されなくなった private メソッドを削除する

- [x] `validateCompletePath` を削除する
- [x] `validateDirectoryComponentMode` を削除する
- [x] `isStickyDirectory` を削除する
- [x] `validateDirectoryComponentPermissions` を削除する
- [x] `validateGroupWritePermissions` を削除する

---

### Step 2: `file_validation_test.go` を修正する

#### 2-1: private メソッドを直接呼ぶテストを public API 経由に移行する

以下のテストは `validateCompletePath` を直接呼び出しており、メソッド削除後にコンパイルエラーになるため `ValidateDirectoryPermissions` 経由に移行する。

- [x] `TestValidator_ValidateCompletePath_SymlinkProtection`
  - `testValidator.validateCompletePath(cleanPath, originalPath, realUID)` → `testValidator.ValidateDirectoryPermissions(tt.path)`
  - `ValidateDirectoryPermissions` 内で `ValidateDirectoryPermissionsWithOptions` → `validateDirectoryHierarchy` → `validateDirectoryComponentMode` を経由するため、シンボリックリンク検出ロジックは同様に動作する

- [x] `TestValidator_ValidatePathComponents_EdgeCases`
  - `testValidator.validateCompletePath(cleanPath, originalPath, realUID)` → `testValidator.ValidateDirectoryPermissions(tt.path)`
  - パス正規化のエッジケース（`//`、末尾 `/` 等）は `ValidateDirectoryPermissionsWithOptions` 内の `filepath.Clean` で処理される

- [x] `TestValidator_validateCompletePath`
  - 2 件のテストケース（UID コンテキスト、所有権ミスマッチ）を `ValidateDirectoryPermissions` 経由に移行する
  - 移行後は `ValidateDirectoryPermissions` が `os.Getuid()` を使用するため、テスト環境でのUID を前提としたケース設計に修正する

#### 2-2: 上位テストで網羅済みの private メソッドテストを削除する

以下のテストは削除するメソッドを直接テストしているが、それぞれ上位レベルのテストで同等のシナリオが検証されている。

- [x] `TestValidator_validateDirectoryComponentPermissions_WithRealUID` を削除する

  | シナリオ | 上位テストでの対応 |
  |---|---|
  | owner_write_permission_with_matching_uid | `TestValidator_ValidateDirectoryPermissions_CompletePath` "directory owned by current user" |
  | owner_write_permission_with_non_matching_uid | 移行済み `TestValidator_validateCompletePath` の ownership mismatch ケース |
  | root_owned_directory_always_allowed | `TestValidator_ValidateDirectoryPermissions_CompletePath` 複数ケース |
  | group_write_permission_with_single_group_member | `TestValidator_ValidateDirectoryPermissions` sticky ケース |
  | world_writable_directory_rejected | `TestValidator_ValidateDirectoryPermissions` "directory with excessive permissions" |

- [x] `TestValidator_validateGroupWritePermissions_AllScenarios` を削除する

  | シナリオ | 上位テストでの対応 |
  |---|---|
  | root_owned_trusted_group_allowed | `TestValidator_ValidateDirectoryPermissions_CompletePath` "directory with root group write owned by root" |
  | group_write_non_trusted / gm_nil / gm_unsafe | `TestValidator_ValidateDirectoryPermissions_CompletePath` "group-writable intermediate directory owned by non-root" |
  | gm_safe_write_passes | `TestRunTOCTOUPermissionCheck_*` in `internal/security/toctou_test.go`（実FSで同等） |

- [x] `TestValidator_validateGroupWritePermissions_TrustedOwnershipScenarios` を削除する

  | シナリオ | 上位テストでの対応 |
  |---|---|
  | root_owned_root_group_allowed | `TestValidator_ValidateDirectoryPermissions_CompletePath` "directory with root group write owned by root" |
  | root_owned_non_root_non_trusted_group_rejected | `TestValidator_ValidateDirectoryPermissions_CompletePath` "non-root group write owned by root" |

---

### Step 3: ビルド・テスト・lint の確認

- [ ] `go build ./cmd/record ./cmd/verify ./cmd/runner` が成功することを確認する（AC-8）
- [ ] `make test` が全件パスすることを確認する（AC-9）
- [ ] `make lint` がエラーなしで完了することを確認する（AC-10）

---

## 補足

### AC-7 の充足確認

フェーズ 2 完了後、`file_validation.go` にディレクトリ権限検証の重複ロジックは存在しない。
コアロジックは `internal/security/dir_permissions_core.go` の `ValidateDirectoryPermissionsWithOptions` に一元化される。

| パス | 使用する共通関数 |
|---|---|
| `isec.dirPermChecker.ValidateDirectoryPermissions` | `ValidateDirectoryPermissionsWithOptions` |
| `Validator.ValidateDirectoryPermissions` | `ValidateDirectoryPermissionsWithOptions`（`buildDirPermOpts` 経由） |
| `Validator.validateOutputDirectoryAccess` | `ValidateDirectoryPermissionsWithOptions`（`buildDirPermOpts` 経由） |

### テスト移行方針

テスト数の変化：

- 削除: `TestValidator_validateDirectoryComponentPermissions_WithRealUID`、`TestValidator_validateGroupWritePermissions_AllScenarios`、`TestValidator_validateGroupWritePermissions_TrustedOwnershipScenarios`（3 件）
- 移行（関数名変更なし、呼び出しを public API に変更）: `TestValidator_ValidateCompletePath_SymlinkProtection`、`TestValidator_ValidatePathComponents_EdgeCases`、`TestValidator_validateCompletePath`（3 件）

シンボリックリンク保護・パスエッジケース・所有権ミスマッチのシナリオは public API 経由で引き続き検証される。

### `buildDirPermOpts` について

`ValidateDirectoryPermissions` と `validateOutputDirectoryAccess` で同一の `DirectoryPermCheckOptions` 構築ロジックが必要なため抽出する。テストコードからは不可視（private メソッド）のため専用テストは追加しない。
