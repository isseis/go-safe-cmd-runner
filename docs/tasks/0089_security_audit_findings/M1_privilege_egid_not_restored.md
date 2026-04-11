# M1: 権限昇格失敗時に egid が元に戻らない

- **重大度**: 🔴 Medium-High
- **領域**: 権限管理 (`internal/runner/privilege`)
- **影響コマンド**: `runner`

## 問題

[internal/runner/privilege/unix.go:478-485](../../../internal/runner/privilege/unix.go#L478-L485) の `changeUserGroupInternal` 関数において、`Setegid` 成功後に `Seteuid` が失敗した場合のリカバリが不正。

```go
if err := syscall.Seteuid(targetUID); err != nil {
    // Try to restore original GID on failure
    if restoreErr := syscall.Setegid(syscall.Getegid()); restoreErr != nil {
        m.logger.Error("Failed to restore GID after UID change failure",
            "restore_error", restoreErr)
    }
    return fmt.Errorf("failed to set effective user ID to %d (user %s): %w", targetUID, userName, err)
}
```

`syscall.Getegid()` は **現在の egid**、すなわち直前の `Setegid(targetGID)` ([:474](../../../internal/runner/privilege/unix.go#L474)) で設定したばかりの `targetGID` を返す。それを再度 `Setegid` に渡しているため、この呼び出しは **実質的な no-op** であり、元の egid には復旧していない。

## 発生条件と呼び出し側の挙動

1. `performElevation` ([:113](../../../internal/runner/privilege/unix.go#L113)) が `changeUserGroupInternal` を呼び出し、上記の失敗が発生。
2. `WithPrivileges` ([:46](../../../internal/runner/privilege/unix.go#L46)) は `performElevation` が失敗した場合、`defer m.handleCleanupAndMetrics(execCtx)` を **セットする前にリターン** する (該当の defer は [:61](../../../internal/runner/privilege/unix.go#L61) にあり、`performElevation` の成功後に初めて登録される)。
3. 結果として `restoreUserGroupInternal` (本来の egid 復元ルート) は **呼ばれない**。
4. `performElevation` 内で `restorePrivileges` ([:121-127](../../../internal/runner/privilege/unix.go#L121-L127)) は呼ばれるが、これは euid のみを復元し egid には触れない。

## 影響

- `Seteuid` が失敗する現実的シナリオ:
  - ターゲット UID の NSS ルックアップ後、ユーザ削除や名前変更が発生した
  - ターゲット UID に対する `RLIMIT_NPROC` 超過
  - seccomp / LSM ポリシーによる拒否
- 失敗後、プロセスは **euid = 元のユーザ、egid = target group** という不整合状態で実行を継続する。
- 後続のファイル作成は意図しないグループ所有で行われる可能性があり、権限境界の前提が崩れる。

## 再現性

- 単体で直接再現させるのは難しい (カーネルに `Seteuid` を失敗させる条件を作る必要がある)。
- ただしコードパスの存在は明白であり、特定の運用環境 (コンテナ、seccomp プロファイル、cgroup ulimit 等) では現実に到達しうる。

## 修正方針

### 案 A (推奨): 呼び出し側から元の egid を渡す

`changeUserGroupInternal` に `originalEGID int` 引数を追加し、失敗時にその値で `Setegid` を呼ぶ。

```go
func (m *UnixPrivilegeManager) changeUserGroupInternal(userName, groupName string, originalEGID int, dryRun bool) error {
    // ...
    if err := syscall.Seteuid(targetUID); err != nil {
        if restoreErr := syscall.Setegid(originalEGID); restoreErr != nil {
            m.logger.Error("Failed to restore GID after UID change failure",
                "restore_error", restoreErr, "original_egid", originalEGID)
            // 復元失敗は致命的なので emergencyShutdown を検討
        }
        return fmt.Errorf(...)
    }
}
```

呼び出し側 ([performElevation](../../../internal/runner/privilege/unix.go#L120-L130)) は既に `execCtx.originalEGID` を保持しているためこれを渡せばよい。

### 案 B: `performElevation` 失敗時にも cleanup を確実に実行

`WithPrivileges` の制御フローを見直し、`performElevation` の内部で失敗時にも GID 復元を含む完全な cleanup を行う。

- 案 A の方がローカルな修正で済み、副作用リスクも小さい。

## 追加の要検討事項

- egid 復元にも失敗した場合、プロセスを `emergencyShutdown` で即時終了させるべきか。現状の `emergencyShutdown` ([:241](../../../internal/runner/privilege/unix.go#L241)) と同等の扱いが妥当と思われる。
- 関連テスト: `internal/runner/privilege/unix_privilege_test.go` に失敗パスのテストケース追加が必要。

## 参考箇所

- [internal/runner/privilege/unix.go:381-494](../../../internal/runner/privilege/unix.go#L381-L494) — `changeUserGroupInternal`
- [internal/runner/privilege/unix.go:113-133](../../../internal/runner/privilege/unix.go#L113-L133) — `performElevation`
- [internal/runner/privilege/unix.go:46-66](../../../internal/runner/privilege/unix.go#L46-L66) — `WithPrivileges`
