# A2: `internal/runner/base/executor/` セキュリティ監査所見

- 監査日: 2026-07-18
- 対象: `internal/runner/base/executor/` (非テストファイル中心: executor.go, environment.go, fdexec_linux.go, fdexec_other.go, interface.go, shell_escape.go, tempdir_manager.go, executor_privilege_check_unix.go / _windows.go)
- 方法: 静的コードレビュー（読み取り専用）

## 所見サマリ

| 重大度 | 件数 |
|---|---|
| 🔴 High | 0 |
| 🟡 Medium | 3 |
| 🟠 Low | 3 |
| 🔵 Info | 6 |

---

## 🟡 Medium

### M-1: 環境変数 denylist の抜け（`DYLD_*`、`GLIBC_TUNABLES` 等）

- 該当箇所: `environment.go:86-95`
- 問題: `BuildProcessEnvironment` は動的リンカ制御変数を denylist 方式で除去しているが、対象は `LD_` プレフィックスと固定リスト（`GCONV_PATH`, `LOCPATH`, `HOSTALIASES`, `NLSPATH`, `RES_OPTIONS`）のみ。以下が漏れている:
  - `DYLD_*`（macOS の dyld 挿入変数。本リポジトリは darwin ビルドをサポートしており（`fdexec_other.go` 参照）、macOS 上では `DYLD_INSERT_LIBRARIES` 等がそのまま子プロセスへ渡る）
  - `GLIBC_TUNABLES`（CVE-2023-4911 "Looney Tunables" で悪用された glibc 挙動制御変数）
  - シェル/インタプリタ系ラッパー実行時に効く `BASH_ENV` / `ENV` / `SHELLOPTS` / `PS4` / `PYTHONPATH` / `PERL5LIB` 等
- 悪用シナリオ: 攻撃者が `env_allowlist` に載っている変数名、あるいは設定ファイルの `vars`/`env_import` 経由でこれらの名前を注入できた場合（例: allowlist に `DYLD_INSERT_LIBRARIES` が誤って追加された、あるいは `env_import` 元が汚染された）、子プロセスへライブラリ挿入やインタプリタ初期化スクリプト実行の攻撃面が開く。denylist はこの誤設定に対する最終防衛線として意図されているのに、Linux glibc 中心の項目しかカバーしていない。
- 推奨対応: `DYLD_` プレフィックスと `GLIBC_TUNABLES` を最低限追加する。インタプリタ系（`BASH_ENV` 等）も脅威モデルに照らして追加を検討。denylist を `internal/runner/base/environment` 等の既存セキュリティ定義と一元化し（DRY）、docs/security の脅威モデルと突き合わせてテストで固定する。

### M-2: `plan` / 検証済み FD が無い場合にパス直接 exec へフォールバック（executor 単体では fail-open）

- 該当箇所: `executor.go:446-483`（`prepareExecCommand` の最終分岐）、`executor.go:264-286`（`executeNormal`）
- 問題: `plan == nil`、`plan.Identity == nil`、または `Identity.FD == nil` の場合、executor は検証済み inode へのバインドを一切行わず、渡されたパスをそのまま `exec.CommandContext` する。コメントは「evaluator の identity gate が未検証バイナリを事前に拒否する」ことを根拠にしているが、executor 自身は plan の存在も検証状態も強制しない。呼び出し側のバグ（plan の渡し忘れ、FD Close 後の再利用、テスト用経路の本番混入）が起きると、TOCTOU 保護（検証とexec の間のパス差し替え検出）が黙って消える。
- 悪用シナリオ: 将来のリファクタで `Execute(ctx, nil, cmd, ...)` を呼ぶ呼び出し元が追加された場合、ハッシュ検証済み inode と実際に exec される inode の同一性保証が失われ、検証後にシンボリックリンク/rename で差し替えられたバイナリが実行され得る。
- 推奨対応: 多層防御として、検証済み FD なしの実行を許すか否かを明示的なフラグ（例: `AllowUnboundExec`）にするか、少なくとも FD なしで exec する際に WARN レベルの監査ログを出す。ユニットテストで「plan なし実行はデフォルト拒否（または警告）」を固定する。

### M-3: コマンドの stderr 全文・コマンドラインを redaction なしでログ出力

- 該当箇所: `executor.go:400-406`（失敗時に `"stderr", string(stderr)` を Error ログへ）、`executor.go:303-308`（`FormatCommandForLog` による全引数の Debug ログ）
- 問題: コマンド失敗時、stderr の全文がそのままログに記録される。stderr にはパスワードプロンプトのエコー、接続文字列、トークンを含む URL などの機密が含まれ得る。また Debug レベルでは展開後の全引数（`--password=...` 等の機密を含み得る）がログに出る。このパッケージ内には redaction 層が無く、上位の logging/redaction 層に完全依存している（slog ハンドラが redact する保証はこのコードからは読み取れない）。
- 悪用シナリオ: ログ収集基盤（syslog 転送先、ログファイル）の閲覧権限しか持たない者が、失敗したコマンドの stderr / 引数から DB パスワード等を取得する。
- 推奨対応: stderr のログ出力を先頭 N バイトに制限する、または redaction フィルタ経由でのみ出力する。引数ログは既存の redaction 機構（`internal/logging` 等）を通すことを型レベルで強制する。少なくとも stderr 全文出力は監査ログ（アクセス制御された経路）に限定する。

---

## 🟠 Low

### L-1: run-as 実行中、親プロセスが子の終了まで root 権限を保持し続ける

- 該当箇所: `executor.go:236-240`（`WithPrivileges` のクロージャ内で `executeCommandWithPath` → `execCmd.Run()` まで実行）
- 問題: 権限昇格が必要なのは fork/exec（`SysProcAttr.Credential` 設定のための setuid 相当）とステージングの chgrp までであり、子プロセスの実行完了を待つ間まで親が euid=0 を保持する必要はない。長時間走るコマンドでは、昇格ウィンドウがコマンド実行時間全体に広がり、親プロセスが攻撃（シグナル、ptrace は不可だが他のバグとの複合）を受けた際の影響が大きくなる。
- 推奨対応: `Start()` までを `WithPrivileges` 内で行い、`Wait()` は権限復元後に行う構造を検討する（`exec.Cmd.Run()` を `Start()`+`Wait()` に分割）。実装コストと現在の privilege manager の設計を踏まえた上でのハードニング項目。

### L-2: 出力キャプチャが無制限にメモリへ蓄積される（DoS）

- 該当箇所: `executor.go:381-387`（`outputWriter == nil` 時の `execCmd.Output()`）、`executor.go:660-687`（`outputWrapper` の無制限 `bytes.Buffer`）
- 問題: `outputWriter` が nil の場合、子プロセスの stdout/stderr は上限なしでメモリに読み込まれる。`outputWriter` がある場合も、writer 側がサイズ上限でエラーを返すまでの間、`outputWrapper.buffer` は並行して全出力を複製保持する（writer の上限＝バッファの上限にはなるが、writer 実装に上限が無ければ無制限）。さらに `Result.Stdout/Stderr` として文字列化され、audit ログにも全文が渡る（`executor.go:253-257`）。巨大出力を生成するコマンド（意図的・偶発的）でランナー自体が OOM し、後続グループの実行やクリーンアップが道連れになる。
- 推奨対応: キャプチャバッファに上限（例: 数 MiB、超過分は切り捨て＋切り捨てフラグ）を設ける。`Result` / audit へ渡す文言も同じ上限を適用する。

### L-3: `syscall.Dup` の複製 FD に `FD_CLOEXEC` が無く、子プロセスへ元番号でも漏れる

- 該当箇所: `fdexec_linux.go:42-46`
- 問題: `syscall.Dup` は `FD_CLOEXEC` を設定しない。`ExtraFiles` 経由で子の fd 3 に dup2 されるのは意図どおりだが、複製元の記述子番号（親でのランダムな番号 j）も CLOEXEC が無いため exec 後の子プロセスにそのまま残る。結果として子は fd 3 と fd j の 2 本で自分のバイナリへの読み取り記述子を持つ。実害は小さい（読み取り専用の自バイナリ）が、意図しない fd 継承は fd 枯渇や将来の変更（書き込み可能 fd の追加等）で問題化し得る典型的な穴。なお `stageFromFD`（`executor.go:546`）側の dup は exec 前に close されるため漏れない。
- 推奨対応: `syscall.Dup` の代わりに `unix.FcntlInt(fd, unix.F_DUPFD_CLOEXEC, 0)` を使う（`ExtraFiles` に渡した *os.File は os/exec が子側で正しく fd 3 に継承させるため、CLOEXEC を付けても意図した継承は損なわれない）。

---

## 🔵 Info

### I-1: nil ガードの不整合（`runAsResolver` のみ fallback、`identityChecker`/`osExit` は nil panic）

- 該当箇所: `executor.go:192-195`（resolver は nil fallback あり）vs `executor.go:135`, `executor.go:141`（`identityChecker`/`osExit` は `&DefaultExecutor{}` リテラル構築時に nil panic）
- 問題: `runAsResolver` には「bare literal 構築でも fail-closed」のための nil fallback があるが、同じ理由が当てはまる `identityChecker` と `osExit` には無い。panic は fail-closed ではあるものの、privilege leak 検出（Error ログ + os.Exit(1)）という設計された経路を通らずに落ちる。設計意図の一貫性の問題。
- 推奨対応: `Execute` 冒頭で 3 つとも default へ fallback するか、逆に「必ず `NewDefaultExecutor` を使う」ことをコンストラクタ非公開化などで強制する。

### I-2: `Validate` と実行時チェックの不整合・重複

- 該当箇所: `executor.go:611-637`（`Validate` は `filepath.IsLocal` な相対パスを許容）、`executor.go:280-283`（`executeNormal` は絶対パスを要求）、`executor.go:180-183` / `executor.go:272-275`（`Validate` 直後の重複した空コマンドチェック）
- 問題: `Validate` 単体では相対パスを合格させるが、実行経路は必ず絶対パスを要求するため、インターフェースとしての `Validate` の保証が実行時の要件より弱い。外部から `Validate` だけを信頼した呼び出し元が生まれると齟齬になる。また `ErrEmptyCommand` チェックが `Validate` と各 execute 関数で二重化している（dead code）。
- 推奨対応: `Validate` で絶対パスを要求するよう統一し、重複チェックを削除する。

### I-3: `/proc/self/fd/3` のハードコードは `ExtraFiles` 構成との暗黙結合

- 該当箇所: `fdexec_linux.go:52`、`executor.go:461-463`
- 問題: 子パス `/proc/self/fd/3` は「`ExtraFiles` の唯一の要素が検証済み FD である」ことを前提にしている。将来 `ExtraFiles` に別の fd を先頭追加する変更が入ると、検証していない fd を exec するという重大バグに静かに化ける。現状は正しいが、結合が暗黙的。
- 推奨対応: `fdExecExtraFile` が index を返す、あるいは `prepareExecCommand` 側で `ExtraFiles` 構築と childPath 生成を単一関数に閉じ込め、コメントではなくコードで不変条件を表現する。

### I-4: `outputWrapper.Write` の io.Writer 契約違反気味の挙動

- 該当箇所: `executor.go:668-687`
- 問題: 内部バッファへは常に全量書き込んだ後、`OutputWriter.Write` が失敗すると `(0, err)` を返す。バッファには「書き込めなかったはずのデータ」が残り、`Result.Stdout` と writer 出力（ファイル等）が不一致になる。また n=0 と実消費量の不一致は `io.Writer` 契約上グレー。実害は「エラー後の捕捉出力が writer 出力より多い」程度。
- 推奨対応: writer エラー後はバッファへの追記も止める、またはエラー時点でバッファを切り詰めて一致させる。

### I-5: `TempDirManager` の軽微な事項

- 該当箇所: `tempdir_manager.go:72-93`
- 問題:
  - `os.MkdirTemp` は既に 0700 で作成するため、直後の `Chmod(0700)` は冗長（umask で狭まることはあっても広がることはない）。
  - prefix にグループ名をそのまま埋め込む（`scr-<group>-`）。パス区切りを含む名前は `MkdirTemp` がエラーで fail-closed になるため脆弱性ではないが、グループ名検証への暗黙依存。
  - `DefaultTempDirManager` は並行安全でない（`tempDirPath` へのレースガード無し）。現在の利用パターン（グループ単位・単一ゴルーチン）では問題ないが、インターフェースにはその制約が明記されていない。
- 推奨対応: 冗長 Chmod の削除またはコメントでの意図明記、インターフェースコメントへの並行性制約の追記。

### I-6: `ShellEscape` は制御文字をエスケープしない（ログインジェクション面）

- 該当箇所: `shell_escape.go:30-33`
- 問題: 改行・ESC 等の制御文字はシングルクォート内にそのまま残る。用途はログ整形のみであり、slog のハンドラ側でクォートされることが多いが、raw テキストハンドラや `fmt.Fprintf(os.Stderr, ...)` 経由に流れた場合、引数に含まれる改行/ANSI エスケープでログ行偽造・端末エスケープ注入が可能。「copy-paste 可能な文字列を返す」というコメント上の保証も、制御文字を含む場合は成立しない。
- 推奨対応: 制御文字（< 0x20, 0x7f）を含む場合は `$'...'` 形式または `\xNN` 表記に落とす。

---

## 観察された良好な防御層

1. **実行後の権限リーク不変条件チェック**（`executor.go:131-142`, `executor_privilege_check_unix.go`）: 全実行後に EUID==UID / EGID==GID を検証し、違反時は即 `os.Exit(1)`。privilege manager の復元ロジックから独立した defense-in-depth。テスト用に `osExit`/`identityChecker` が注入可能で、テスト網羅もされている（`executor_privilege_check_test.go`）。
2. **fail-closed な run-as 解決**（`executor.go:196-213`）: 識別子解決の失敗でコマンドを実行せずエラー返却。特に `ResolveRunAsIdent` が補助グループ列挙失敗時に nil Groups を黙って返す仕様を把握した上で、nil Groups を明示的に拒否している点は API の落とし穴を正しく塞いでいる。
3. **カーネルアトミックな資格情報設定**（`executor.go:217-222`, `applyCredential`）: setuid/setgid/setgroups を親プロセスで順次呼ぶのではなく `SysProcAttr.Credential` により execve 時にカーネルが一括設定。順序ミス（setgroups 忘れ等）による権限残留の余地がない。`NoSetGroups: false` の明示も適切。
4. **fd-bound exec による TOCTOU 閉鎖**（`fdexec_linux.go`, `executor.go:454-479`）: 検証済み記述子を `/proc/self/fd/3` 経由で exec し、検証〜exec 間のパス差し替えを無効化。`/proc` 非搭載環境の probe（`sync.OnceValue`）とステージングへのフォールバックも用意されている。
5. **ステージングコピーの所有権設計**（`executor.go:485-608`）: chown ではなく chgrp（uid=-1）で run-as ユーザーに owner 権限（chmod/chown 権）を渡さない。ディレクトリ 0710 / ファイル 0550・`O_EXCL` 作成・umask 非依存の明示的 chmod・エラー時の named-return による確実なクリーンアップ、検証済み fd からの `SectionReader`(pread) コピー（共有オフセット非破壊、パス再 open なし）と、細部まで一貫している。
6. **環境変数の allowlist 方式 + ローダ変数の強制除去**（`environment.go`）: システム環境は allowlist 通過分のみ、その上で `LD_*` 等を出所を問わず除去（除去リストの不足は M-1 参照だが、方式自体は健全）。
7. **stdin の /dev/null 固定**（`executor.go:328-337`）: 子プロセスへの意図しない入力継承を遮断。
8. **fd 所有権の明確化**: 検証済み FD の複製と close 責務が「original は VerifiedFD、duplicate は *os.File」と明文化され、double-close/leak を防ぐ規律がコメントとコードの両方で維持されている。
9. **出力サイズ超過エラーの優先**（`executor.go:371-380`）: writer エラー時に SIGPIPE 由来の "broken pipe" ではなく根本原因（サイズ上限超過等）を返す配慮。
