# M2: バイナリ検証と exec の間の TOCTOU ウィンドウ

- **重大度**: 🟡 Medium
- **領域**: 検証 ↔ 実行 (`internal/verification`, `internal/runner/executor`)
- **影響コマンド**: `runner`

## 問題

コマンドバイナリのハッシュ検証とその後の `exec.CommandContext` によるロードの間に、ファイル内容を差し替え可能な TOCTOU ウィンドウが存在する。

### 処理フロー

1. [group_executor.go:344](../../../internal/runner/group_executor.go#L344) で `VerifyGroupFiles` がバイナリをハッシュ検証 (ファイルを open → 読込 → ハッシュ計算 → close)。
2. 検証成功後、[executor.go:196](../../../internal/runner/executor/executor.go#L196) で `exec.CommandContext(ctx, path, ...)` が **別の open(2) / execve(2)** を発行してバイナリをロード。
3. ステップ 1 の `close` とステップ 2 の `open` の間に、パス上のファイルが差し替えられる可能性がある。

### ハッシュ検証側の詳細

[filevalidator/validator.go:429-446](../../../internal/filevalidator/validator.go#L429-L446) の `Verify` は:

```go
func (v *Validator) Verify(filePath string) error {
    targetPath, err := validatePath(filePath)       // stat
    ...
    actualHash, err := v.calculateHash(targetPath)  // SafeReadFile (open → read → close)
    ...
    return v.verifyHash(targetPath, actualHash)     // compare
}
```

ファイルディスクリプタは **保持されない**。`SafeReadFile` は `openat2(RESOLVE_NO_SYMLINKS)` を使うため symlink swap は防げるが、**同じパスに対する通常ファイルの inode 置き換え** (別ファイルを rename でそのパスに移動) は防げない。

## 影響

攻撃者がバイナリの存在するディレクトリに対する書き込み権限を持つ場合 (例: `rename`/`unlink`+`create` が可能)、以下が成立する:

1. 本物のバイナリ `A` をハッシュ検証 → OK
2. 検証と exec の間に `A` を悪意あるバイナリ `A'` に差し替え
3. runner が `A'` を exec、検証をバイパスして任意コード実行

## 緩和要因

- 典型的な運用では検証対象は `/bin`, `/usr/bin`, `/usr/local/bin` 等の **root 所有かつ非特権ユーザ書込不可** のディレクトリに配置される。
- `record` で生成されるハッシュディレクトリも同様に保護される前提。
- 動的共有ライブラリは [dynlibVerifier](../../../internal/verification/manager.go#L473) で別途検証。
- shebang interpreter も [VerifyCommandShebangInterpreter](../../../internal/verification/manager.go#L681) で別途検証。

これらの前提 (= **攻撃者がバイナリのあるディレクトリに書き込めない**) が成立する限り、攻撃経路は実質的に閉じている。

## 根本対策 (中長期課題)

TOCTOU を完全に排除するには、検証で開いた FD から直接 exec する必要がある。

### `fexecve(2)` / `execveat(2)` の採用

Linux では `execveat(fd, "", argv, envp, AT_EMPTY_PATH)` により、ファイルディスクリプタから直接 exec 可能。検証フローを以下に変更:

1. `openat2(RESOLVE_NO_SYMLINKS)` で FD を取得
2. FD からハッシュを計算し検証
3. 同じ FD を `execveat` に渡して実行

### Go 側の課題

- Go 標準ライブラリ `os/exec` は `fexecve` をサポートしない。
- `syscall.Syscall6` で直接呼び出す必要がある (`safefileio/safe_file_linux.go` と同様のスタイル)。
- プロセス生成 (`fork` + `execveat`) を自前実装する必要があり、`os/exec` の便利機能 (stdin/stdout リダイレクト、context キャンセル、プロセスグループ管理) を再実装する必要がある。
- 実装コストが中程度で、運用での現実的脅威は低いため短期対応には含めない。

## 短期的対策

### 運用ドキュメントでの要件明記

`docs/security/README.md` または運用ガイドに以下を必須要件として明記する:

- `verify_files` で指定するパスは、既存のディレクトリパーミッション検査 (`ValidateDirectoryPermissions` / `validateCompletePath`) が通過するディレクトリ配下であること
  - 具体的には: other 書込不可 (sticky bit ディレクトリを除く)、group 書込は root 所有または実行ユーザが唯一のグループメンバである場合のみ許可、owner 書込は root または実行ユーザ所有の場合のみ許可
- ディレクトリ自体の rename/unlink 権限 (親ディレクトリの書込権限) も同様に制限すること
- ハッシュディレクトリ (`--hash-dir`) 自体も同様の要件

### 自動検査の追加検討

`security.Validator` で `verify_files` / コマンドパスの親ディレクトリの所有者・パーミッションを検査し、非 root 書込可なら警告/エラーを発する機能の追加を検討。

## 参考箇所

- [internal/runner/group_executor.go:344](../../../internal/runner/group_executor.go#L344) — `VerifyGroupFiles` 呼び出し
- [internal/runner/executor/executor.go:196](../../../internal/runner/executor/executor.go#L196) — `exec.CommandContext`
- [internal/filevalidator/validator.go:429-446](../../../internal/filevalidator/validator.go#L429-L446) — `Verify` 実装
- [internal/safefileio/safe_file_linux.go](../../../internal/safefileio/safe_file_linux.go) — `openat2` ラッパ
