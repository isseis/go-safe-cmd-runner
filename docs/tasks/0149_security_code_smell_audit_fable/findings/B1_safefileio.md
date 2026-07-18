# B1: internal/safefileio/ セキュリティ監査

- 監査日: 2026-07-18
- 対象: `internal/safefileio/` (safe_file.go, safe_file_linux.go, safe_file_nonlinux.go, errors.go, nofollow_error*.go)
- 方法: 静的コードレビュー（読み取り専用）。関連する `internal/common`（ResolvedPath / OS 管理 symlink allowlist）、呼び出し元（filevalidator, runner/base/output 等）も参照。

## サマリ

| 重大度 | 件数 |
|---|---|
| 🔴 High | 0 |
| 🟡 Medium | 2 |
| 🟠 Low | 3 |
| 🔵 Info | 4 |

---

## 所見

### F-1 🟡Medium: AtomicMoveFile — fd で検証したソースをパスで rename する TOCTOU

- 該当箇所: `internal/safefileio/safe_file.go:140-179` (`atomicMoveFileCore`)
- 問題: ソースファイルは `SafeOpenFile`（openat2 `RESOLVE_NO_SYMLINKS`）で開き、fchmod・所有権/権限検証もその **fd** に対して行う。しかし最終的な移動は `os.Rename(absSrc, absDst)` と **パス名** で実行するため、検証対象と移動対象が同一である保証がない。
- 悪用シナリオ: ソースの親ディレクトリに書き込める攻撃者が、検証完了後 `os.Rename` 実行前に `absSrc` を別ファイル（または symlink を含む経路）に差し替えると、未検証のファイルが `absDst`（例: 出力の最終位置）へ移動される。rename 時にはカーネルが `absSrc` の親コンポーネントの symlink を通常どおり解決するため、`ensureParentDirsNoSymlinks` は宛先側しかカバーしない。移動後に宛先を `SafeOpenFile` + `canSafelyAccessFile` で再検証する層があるため権限面の異常は検出されるが、内容の同一性（検証したファイル＝移動されたファイル）は保証されない。
- 緩和要因: 実運用の呼び出し元（`internal/runner/base/output/file.go:73`）ではソースは runner 自身が作成した一時ファイルであり、親ディレクトリが攻撃者書き込み可能な構成は通常ない。攻撃には親ディレクトリへの書き込み権限が必要。
- 推奨対応: Linux では `renameat2` を親ディレクトリ fd（`O_DIRECTORY|O_NOFOLLOW` で開いた dirfd）基準で行う、または移動後の宛先 fd と検証済みソース fd の `(dev, ino)` 一致を確認する。少なくとも関数コメントに「ソース親ディレクトリは信頼できるパスであること」という前提条件を明記する。

### F-2 🟡Medium: フォールバック経路（非 Linux / openat2 無効時）に残存する親ディレクトリ symlink TOCTOU

- 該当箇所: `internal/safefileio/safe_file.go:505-529` (`safeOpenFileFallback`)
- 問題: `O_NOFOLLOW` はリーフのみ保護し、中間コンポーネントの symlink は `ensureParentDirsNoSymlinks` の事前/事後チェックに依存する。チェック→open→再チェックの二相方式は競合窓を狭めるが排除しない。
- 悪用シナリオ: 攻撃者が (1) 事前チェック後に親ディレクトリを symlink に差し替え → (2) open が symlink 先で実行される → (3) 事後チェック前に symlink を実ディレクトリに戻す、という往復差し替えに成功すると両チェックを通過する。タイミングは厳しいが、inotify/kqueue 等で open を検知して往復させる攻撃は既知の手法。macOS（本開発環境）は常にこの経路を使う。
- 緩和要因: パッケージコメントおよび関数コメントで二相検証の限界が示唆されており、Linux 本番環境では openat2 によりこの窓は存在しない。攻撃には中間ディレクトリの書き換え権限が必要で、その場合そもそも脅威モデル上ほぼ敗北している。
- 推奨対応: 非 Linux でも `openat` を用いてルートからコンポーネント単位で `O_DIRECTORY|O_NOFOLLOW` を指定しながら降りていく方式（dirfd ウォーク）に置き換えると原理的に排除できる。少なくとも「フォールバック経路は best-effort」であることを公開 API のドキュメントに明記する。

### F-3 🟠Low: safeOpenFileFallback — 事後チェック失敗時にファイルディスクリプタがリークする

- 該当箇所: `internal/safefileio/safe_file.go:523-526`
- 問題: `os.OpenFile` 成功後、2 回目の `ensureParentDirsNoSymlinks` がエラーを返すと `file` を Close せずに `nil, err` を返す。fd がリークする。
- 悪用シナリオ: 攻撃者が親ディレクトリの symlink 差し替えを繰り返して事後チェックを意図的に失敗させ続けると、長時間動作するプロセスで fd を枯渇させられる（DoS）。通常運用でもエラー経路で fd が漏れるのは資源リーク。加えて、`O_CREATE` で作成済みのファイルが symlink 先に残置される可能性がある（作成の取り消しもない）。
- 推奨対応: エラー時に `file.Close()`（および必要に応じて作成したファイルの削除）を行う。

### F-4 🟠Low: atomicMoveFileCore — 失敗時の副作用（chmod 済み・移動済み）が巻き戻されない

- 該当箇所: `internal/safefileio/safe_file.go:162-197`
- 問題: 2 つの fail 時残留がある。
  1. `srcFile.Chmod(requiredPerm)`（162 行）を検証（167 行）より先に実行するため、検証失敗時にはソースの権限が既に変更されている。requiredPerm が元より広い場合、失敗したのに緩い権限が残る。
  2. `os.Rename` 成功後の宛先検証（182-195 行）が失敗してもファイルは宛先に移動済みのまま。呼び出し元がエラーを見て中断しても、宛先に「検証に失敗したファイル」が残置される。
- 悪用シナリオ: 直接の権限昇格ではないが、「エラーを返したのに状態は成功時と同じ」という half-done 状態は上位層の想定（エラー＝出力ファイルは作られていない）を裏切り、検証失敗ファイルの残置・後続処理での誤使用につながる。
- 推奨対応: 宛先検証失敗時は宛先ファイルを削除（ロールバック）するか、少なくとも関数コメントで「失敗時も移動が完了している場合がある」ことを契約として明記する。chmod は検証成功後に行う（fchmod なので順序変更のリスクは低い）。

### F-5 🟠Low: FileSystem.Remove が無検査の os.Remove 直通で、パッケージの安全性契約と乖離

- 該当箇所: `internal/safefileio/safe_file.go:96-98`
- 問題: `SafeOpenFile` / `AtomicMoveFile` が symlink・権限検査を行うのに対し、同じ `FileSystem` インターフェースの `Remove` は `os.Remove(name)` をそのまま呼ぶ。unlink 時のパス解決で親コンポーネントの symlink は通常どおり辿られるため、「safefileio の FileSystem を使っているから安全」という呼び出し側の期待（命名と実装の乖離）に反する。
- 悪用シナリオ: 攻撃者が制御できるパスが `Remove` に渡る場合、親ディレクトリ symlink の差し替えで意図しないファイルを削除させられる。現状の呼び出し元は自プロセスが作成した一時ファイルの削除が中心でリスクは低い。
- 推奨対応: `ensureParentDirsNoSymlinks` を `Remove` にも適用するか、メソッドコメントに「安全性検査なし。信頼済みパス専用」と明記する。

### F-6 🔵Info: openat2 の mode に os.FileMode をそのまま渡している

- 該当箇所: `internal/safefileio/safe_file_linux.go:103-108` (`mode: uint64(perm)`)
- 問題: Go の `os.FileMode` は setuid/setgid/sticky を独自ビット（1<<23 等）で表現するため、`os.OpenFile` 内部の `syscallMode()` 相当の変換なしに渡すと、これらのビットを含む perm はカーネルへ誤った mode として届く。また openat2(2) は `O_CREAT`/`O_TMPFILE` なしで mode≠0 を渡すと `EINVAL` を返す仕様であり、非ゼロ perm + 読み取りオープンの組み合わせは fallback 経路と挙動が食い違う。現状の呼び出し元は 0o777 以下のビットと読み取り時 perm=0 のみを渡しているため実害はないが、将来の呼び出しで静かに壊れる罠。
- 推奨対応: perm を `perm & os.ModePerm` に正規化し、特殊ビットが渡された場合は明示的にエラーにする。`O_CREATE` なしのときは mode を 0 に落とす。

### F-7 🔵Info: safeWriteFileCommon はインプレース書き込みで、クラッシュ時に部分書き込みが残る

- 該当箇所: `internal/safefileio/safe_file.go:217-243`
- 問題: `O_WRONLY|O_CREATE` → 検証 → `Truncate(0)` → `Write` の順で既存ファイルを直接上書きする。Write 中のクラッシュ・ディスク満杯で、切り詰め済み/部分内容のファイルが残る。ハッシュマニフェスト等の完全性が重要なファイルでは、破損ファイルが後段の検証で「改ざん」と区別できない。fail-closed（検証が失敗する方向）なのでセキュリティ上の実害はないが、耐障害性の観点では temp+`AtomicMoveFile` が既に同パッケージにあるのに使われていない。
- 推奨対応: 完全性が重要な書き込みは temp ファイル + `AtomicMoveFile` パターンへ寄せることを検討。

### F-8 🔵Info: 読み取り権限検査が GID とモードのみで、所有者 UID を考慮しない

- 該当箇所: `internal/safefileio/safe_file.go:442, 489`（`CanCurrentUserSafelyReadFile(stat.Gid, fileInfo.Mode())`）
- 問題: 書き込み検査は `(uid, gid, mode)` を見るのに対し、読み取り検査は `(gid, mode)` のみ。悪意ある一般ユーザーが所有するファイルでも、group/other のビットが基準を満たせば「安全に読める」と判定される。これは groupmembership 側のポリシー設計であり、読み取り対象は別途ハッシュ検証されるため許容と考えられるが、対称性の欠如は意図的である旨のコメントがあるとよい。詳細は D1（groupmembership 監査）で扱う。
- 推奨対応: D1 で読み取りポリシーの意図を確認し、`canSafelyReadFromFile` のコメントに設計意図を明記。

### F-9 🔵Info: openat2 呼び出しに EINTR リトライがない

- 該当箇所: `internal/safefileio/safe_file_linux.go:68-90`
- 問題: 生の `syscall.Syscall6` 呼び出しのため、シグナル到着時に `EINTR` がそのまま呼び出し元へ返る（Go ランタイムの非同期プリエンプションシグナル SIGURG 等）。失敗は fail-closed（操作が拒否される方向）だが、まれな偽陰性エラーの原因になり得る。
- 推奨対応: `errno == syscall.EINTR` のときのリトライループを追加。

---

## 観察された良好な防御層

1. **openat2(RESOLVE_NO_SYMLINKS) による原子的 symlink 排除**（safe_file_linux.go）: Linux ではパス解決全体からの symlink 排除がカーネル内で原子的に行われ、TOCTOU が原理的に存在しない。起動時に実プローブ（実際にファイルを作って確認）を行い、利用不可なら自動フォールバックする設計も堅実。
2. **型レベルの契約強制**: `SafeWriteFileOverwrite` が `common.ResolvedPath.IsParentOnly()` を要求し（safe_file.go:208-210）、リーフ symlink を事前解決してしまった `NewResolvedPath` 由来のパスを拒否する。「安全な呼び方」をコンパイル時/実行時に強制する良いパターン。
3. **OS 管理 symlink の厳格な allowlist**（common/osmanaged_symlink_darwin.go）: `/tmp` 等の macOS firmlink のみを許可し、`os.Readlink` の実ターゲットが期待値と完全一致することまで検証。攻撃者が置いた同名 symlink を信頼しない。エラー時は false（reject-by-default）。
4. **fchmod の採用**（safe_file.go:159-164): パスベースの `os.Chmod`（symlink を辿る）ではなく、開いた fd への Chmod で権限設定し、chmod 系 TOCTOU を回避。コメントで理由も明記。
5. **多層のサイズ制限**（safe_file.go:365-386): Stat による事前チェックに加え `io.LimitReader(MaxFileSize+1)` + 読み取り後の再チェックで、Stat と Read の間にファイルが成長する競合にも対処。
6. **regular file 検証と権限検証の一元化**: すべての読み書き経路が `getFileStatInfo`（regular file 強制）と groupmembership による権限検証を通る。デバイスファイル・FIFO 経由の攻撃を遮断。
7. **エラーの型付けと fail-closed 姿勢**: `ErrIsSymlink` / `ErrFileExists` 等の sentinel エラーで `errors.Is` 判定が可能。ELOOP/EEXIST/ENOENT のマッピング、NetBSD の EFTYPE 差異までビルドタグで吸収。曖昧なケースは一貫して拒否方向。
8. **テストの充実**: symlink 攻撃・クリーンアップ・openat2 有効/無効の両経路を含む約 1,360 行のテストがあり、フォールバック経路も `DisableOpenat2` で明示的にテスト可能。
