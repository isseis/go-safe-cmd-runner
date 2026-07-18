# A6: internal/runner/base/output/ セキュリティ監査所見

- 監査日: 2026-07-18
- 対象: `internal/runner/base/output/`（capture.go, manager.go, path.go, file.go, errors.go, interfaces.go, types.go）
- 方法: ソースコードの静的読解（依存先 `internal/common`, `internal/safefileio`, `internal/runner/base/security`, 呼び出し元 `internal/runner/resource/normal_manager.go` も参照）

## 所見サマリ

| 重大度 | 件数 |
|---|---|
| 🔴 High | 0 |
| 🟡 Medium | 2 |
| 🟠 Low | 3 |
| 🔵 Info | 3 |

---

## 🟡 Medium

### A6-1: `Capture.WriteOutput` が MaxSize=0（無制限設定）を「一切書き込み禁止」と解釈する

- 該当箇所: `internal/runner/base/output/capture.go:36`
- 関連箇所: `internal/runner/resource/normal_manager.go:246-250`, `internal/runner/base/output/manager.go:142`

**問題**: 呼び出し元 `executeCommandWithOutput` は `EffectiveOutputSizeLimit.IsUnlimited()` のとき `maxSize = 0`（コメントに "0 means unlimited in output manager"）として `PrepareOutput` に渡す。しかし実行時に出力を受け取る `Capture.WriteOutput` のサイズ判定は

```go
if c.CurrentSize+int64(len(data)) > c.MaxSize {
```

であり、`MaxSize > 0` のガードがない。`MaxSize=0` では長さ 1 バイト以上のあらゆる書き込みが `ErrOutputSizeExceeded` で失敗する。つまり「無制限」を設定したユーザーのコマンドは、出力を 1 バイトでも出した時点で失敗する。

一方 `DefaultOutputCaptureManager.WriteOutput`（manager.go:142）は `capture.MaxSize > 0 && ...` と正しくガードしているが、こちらは本番コードから呼ばれていない（後述 A6-2）。テスト（capture_test.go）はすべて `MaxSize > 0` のケースのみで、無制限ケースのテストが欠如している。

**悪用/障害シナリオ**: `output_size_limit = 0`（無制限）を設定した設定ファイルで output 付きコマンドを実行すると、出力キャプチャが常に失敗する。fail-closed のためセキュリティバイパスにはならないが、意図した機能が動作しない（可用性障害）うえ、2 つの実装のセマンティクス乖離は将来の改修時に fail-open 方向へ倒れるリスクを孕む。

**推奨対応**: `Capture.WriteOutput` に `c.MaxSize > 0 &&` ガードを追加し、無制限（MaxSize=0）ケースの単体テスト・統合テストを追加する。あわせて A6-2 の重複解消でセマンティクスを一本化する。

### A6-2: サイズ制限判定ロジックの重複と本番未使用コード（`Manager.WriteOutput`）

- 該当箇所: `internal/runner/base/output/manager.go:136-154`（`WriteOutput`）, `manager.go:17-19`（`ErrOutputSizeLimitExceeded`）

**問題**: `DefaultOutputCaptureManager.WriteOutput` と `Capture.WriteOutput` はほぼ同一の責務（サイズ検査＋一時ファイル書き込み）を別実装で持ち、判定条件（`MaxSize > 0` ガードの有無）と返すエラー型（`ErrOutputSizeLimitExceeded` vs `CaptureError{ErrorTypeSizeLimit}`）が乖離している。リポジトリ全体を検索した限り、本番コードパスは executor → `Capture.Write` → `Capture.WriteOutput` のみを使用し、`Manager.WriteOutput` と `ErrOutputSizeLimitExceeded` を参照する本番コードは存在しない（テストのみ）。

**リスク**: セキュリティ制御（出力サイズ上限＝ディスク枯渇 DoS 対策）の実装が二重化していると、片方だけ修正・監査される事故が起きやすい。実際に A6-1 の乖離が発生している。

**推奨対応**: `Manager.WriteOutput` を削除して `Capture.WriteOutput` に委譲する（または逆に一方を薄いラッパにする）。`ErrOutputSizeLimitExceeded` と `ErrOutputSizeExceeded`（errors.go:135）の同義エラー 2 本立ても統合する。

---

## 🟠 Low

### A6-3: `evaluateSecurityRisk` の denylist に抜けがある（dry-run 表示のみのため Low）

- 該当箇所: `internal/runner/base/output/manager.go:22-37, 277-320`

**問題**: リスク分類の正規表現に以下の抜けがある。

- Critical 対象の抜け: `/etc/sudoers.d/`（`/etc/` により High 止まり。sudoers 本体は Critical なのに drop-in は High）、`/etc/cron.d/`・`/etc/crontab`、`/etc/ld.so.preload`、`/etc/systemd/`、`/root/`、`/dev/`
- High 対象の抜け: `/usr/local/bin/`・`/usr/local/sbin/`、`/bin/`・`/sbin/`（symlink でないディストリでは実体）、`/lib/`・`/lib64/`
- 鍵ファイル名の抜け: `id_ed25519_sk`・`id_ecdsa_sk`（FIDO 鍵）、`*.pem`、`id_rsa.bak` 等の変形（完全一致のみ）
- `.ssh`/`.gnupg` はディレクトリ区切り `/` を伴う場合のみ High 判定のため、`/home/user/.ssh` というパス文字列そのもの（末尾スラッシュなし）への書き込みは Low 判定になる

**影響範囲の限定**: この分類は `AnalyzeOutput`（dry-run 解析）での表示にのみ使われ、実行時の書き込み許可は `SecurityValidator.ValidateOutputWritePermission` と OS パーミッションで強制される。したがってバイパスではなく「dry-run のリスク表示が過小になる」問題。

**推奨対応**: 上記パターンの追加。可能なら `internal/runner/base/risk` 等の既存リスク分類と定義を共有し、分類表の一元管理を検討する（DRY）。

### A6-4: 検証→一時ファイル作成間の TOCTOU 残余（実害は限定的）

- 該当箇所: `internal/runner/base/output/manager.go:109-119`（PrepareOutput）, `file.go:34-43`（CreateTempFile）

**問題**: `PrepareOutput` は (1) `ValidateOutputWritePermission` で親ディレクトリの symlink・権限を検査した後、(2) `EnsureDirectory`、(3) `os.CreateTemp`（`commonFS.CreateTemp` 経由）を行う。(1) と (3) の間にディレクトリを symlink に差し替えられた場合、`os.CreateTemp` は symlink を追従するため一時ファイルが攻撃者制御の場所に作られうる。

**緩和要因**: 最終的な配置は `AtomicMoveFile` が `openat2(RESOLVE_NO_SYMLINKS)` と `ensureParentDirsNoSymlinks` で守っており、(1) の検査で親ディレクトリが他ユーザー書き込み可能でないことも確認済み。攻撃には検証済みディレクトリ自体の書き換え権限が必要で、一時ファイルは 0600・ランダム名。実害はほぼ「一時ファイルの置き場所がずれる」に留まる。

**推奨対応**: 現状の多層防御で実用上は許容範囲。厳密化するなら `O_DIRECTORY|O_NOFOLLOW` で開いた dirfd に対する `openat` ベースの一時ファイル作成を `safefileio` に追加する。

### A6-5: `Capture` の全フィールドが exported で、非公開 mutex による保護と矛盾

- 該当箇所: `internal/runner/base/output/capture.go:13-22`, `manager.go:157-166`

**問題**: `Capture` は `FileHandle`, `CurrentSize`, `TempFilePath`, `MaxSize` 等をすべて export しつつ、排他は unexported の `mutex` に依存する。パッケージ外のコードはロックなしでこれらを読み書きできてしまう（例: `FinalizeOutput` は `capture.Close()` でロックを取った後、`capture.TempFilePath`/`OutputPath` をロックなしで読む。`CleanupOutput` はロック下で `FileHandle`/`TempFilePath` を書き換える）。現在の呼び出しシーケンス（書き込み終了後に Finalize/Cleanup）では実害となる競合はないが、構造として不変条件をコンパイラで守れない。

**推奨対応**: フィールドを unexported にしてアクセサ経由にする、または「Finalize/Cleanup は全 Write 完了後にのみ呼ぶ」という契約を interface ドキュメントに明記する。

---

## 🔵 Info

### A6-6: `AnalyzeOutput` の `DirectoryExists` 判定が実行時ポリシーと不整合

- 該当箇所: `internal/runner/base/output/manager.go:250`

`os.Lstat(dir)` で symlink を除外するため、macOS の `/tmp`（→ `/private/tmp` への symlink）配下では実際には書き込めるのに `DirectoryExists=false` と報告される。実行時検査（`ValidateOutputWritePermission`）は root 所有の OS symlink を許可しており、dry-run と実行時で判定基準が食い違う。また `stat.Mode()&os.ModeSymlink == 0` は `Lstat` が symlink を返した時点で `IsDir()` が false になるため冗長。

### A6-7: `cleanupTempFile` がインスタンスの `m.logger` でなくグローバル `slog` を使用

- 該当箇所: `internal/runner/base/output/manager.go:201, 208`

`DefaultOutputCaptureManager` は `logger` フィールドを持つのに、cleanup 時の警告のみ `slog.Warn` 直呼びで一貫性がなく、テストでのログ捕捉も困難。機密情報は含まれないためセキュリティ影響はない。

### A6-8: 軽微な code smell（重複・冗長）

- `PrepareOutput`（manager.go:111）と `MoveToFinal`（file.go:62）で `EnsureDirectory` を二重に実行。
- `highConfigDirsRegex`（manager.go:36）で `(?i)` を同一パターン内に 2 回記述。
- `types.go:24-34` の `RiskLevel` エイリアスと定数再エクスポートは `runnertypes` の単純な二重化で、利用側の import 混在を招く。
- `errors.go` の `ErrOutputSizeExceeded` と `manager.go` の `ErrOutputSizeLimitExceeded` が同義（A6-2 参照）。

---

## 観察された良好な防御層

1. **パストラバーサル検査のセグメント単位判定**（path.go:56, common/filesystem.go:203）: `..` をパスセグメントとして検査し、`archive..zip` のような正当なファイル名を誤検知しない。相対パスは `filepath.Rel` による work directory 逸脱チェックと二重化。
2. **危険文字 denylist**（path.go:111-186）: シェルメタ文字・グロブ・制御文字・全空白文字（`unicode.IsSpace`）・紛らわしい通貨記号を単一走査で拒否。出力パスが後段でシェルやログに渡った場合のインジェクション面を先回りで縮小している。
3. **temp file + atomic rename パターン**（manager.go:88-187）: 出力は同一ディレクトリの 0600 一時ファイルに書き、`safefileio.AtomicMoveFile` で原子的に配置。部分書き込みファイルが最終パスに現れない。
4. **symlink 攻撃対策の多層化**: 事前の `ValidateOutputWritePermission`（realUID ベースの権限検査＋symlink 検査）、`AtomicMoveFile` 内の `openat2(RESOLVE_NO_SYMLINKS)`・`ensureParentDirsNoSymlinks`・`fchmod`（ファイルハンドル経由の chmod で TOCTOU 回避）。file.go:66-72 のコメントは「事前に symlink を解決してはならない」理由を明記しており、防御の意図がコードに残されている。
5. **保守的なパーミッション**: 出力ファイル 0600（機密出力を想定した根拠コメント付き、manager.go:179-182）、ディレクトリ 0750。
6. **パス包含判定の境界処理**（common/filesystem.go:215）: 末尾セパレータ付与により `/home/user-evil` が `/home/user` 内と誤判定されない。
7. **fail-closed な初期値**: `AnalyzeOutput` は `SecurityRisk` を Critical で初期化し、検証を通過した場合のみ緩和（manager.go:237）。
8. **冪等な後始末**: `Capture.Close()` の冪等化、`RemoveTemp` の存在チェック付き冪等削除、呼び出し元（normal_manager.go）の defer によるエラー時 temp cleanup。
