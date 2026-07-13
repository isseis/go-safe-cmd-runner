# go-safe-cmd-runner セキュリティリスク評価レポート

- 対象リポジトリ: `go-safe-cmd-runner`（ブランチ `main`, commit `a5b82894`）
- 評価種別: レビューベースのソースコード監査（静的レビュー中心。動的検証・ファジングは未実施）
- 評価範囲: 実行エンジン、権限管理、ファイル完全性検証、環境変数処理、設定展開、機微情報のマスキング、外部通知（Slack）
- 日付: 2026-07-14

---

## 0. 総評

本アプリケーションは「非特権ユーザーに特権操作を安全に委譲する」ことを目的とした、
多層防御（defense-in-depth）が非常に丁寧に設計されたコードベースです。特に以下は
高品質に実装されており、一般的な攻撃ベクトルの多くは既に塞がれています。

- 事前ハッシュ検証（設定ファイル・env ファイル・実行バイナリ）を実行前に強制
- 本番ビルドではハッシュディレクトリを固定（`/usr/local/etc/go-safe-cmd-runner/hashes`）し、任意ハッシュDI攻撃を排除
- 固定 PATH・シンボリックリンク攻撃対策（`openat2 RESOLVE_NO_SYMLINKS` + フォールバック）・TOCTOU 対策
- fd バインド実行（検証済み inode を `/proc/self/fd` 経由で実行）による検証〜実行間 TOCTOU の遮断
- リスク評価器による「identity ゲート → 間接実行 → 特権昇格 → 各次元の最大値」というフェイルクローズ設計
- 権限復元後の EUID==UID / EGID==GID 不変条件チェックと、失敗時の即時プロセス停止

以下に挙げるのは、上記の堅牢な基盤の上に残る **残余リスク／堅牢化の余地** が中心であり、
即時に悪用可能な致命的欠陥を多数発見したという趣旨ではありません。重要度は
`High / Medium / Low / Info` で示します。

---

## 1. 検出事項サマリ

| # | 重要度 | 概要 | 該当箇所 |
|---|--------|------|----------|
| F-1 | **High** | run-as ユーザー切替時に補助グループ（supplementary groups）を破棄していない | `internal/runner/base/privilege/unix.go` |
| F-2 | Medium | ユーザー/グループ切替が `seteuid/setegid` のみで、saved-set-uid を明示的に落としていない | `internal/runner/base/privilege/unix.go` |
| F-3 | Medium | 機微情報マスキングがキー名パターン依存で、コマンド出力・引数中の生の秘密値を取りこぼす可能性 | `internal/redaction/sensitive_patterns.go` |
| F-4 | Low | dry-run 時に検証失敗した設定ファイルを `os.ReadFile` で読み直し、未検証内容を解析対象にする | `internal/verification/manager.go` |
| F-5 | Low | `record` コマンドの TOCTOU/ディレクトリ権限チェックが警告のみで、信頼の起点であるハッシュDB生成を止めない | `cmd/record/main.go` |
| F-6 | Info | 128MB のファイルサイズ上限により大容量バイナリの検証・解析が不可（可用性の観点） | `internal/safefileio/safe_file.go`, `filevalidator` |
| F-7 | Info | non-Linux（openat2 非対応）環境では二段階チェックによる TOCTOU 残余ウィンドウが存在 | `internal/safefileio/safe_file.go` |

---

## 2. 詳細

### F-1 [High] run-as ユーザー切替で補助グループを破棄していない

**概要**
`UnixPrivilegeManager.changeUserGroupInternal` は対象ユーザー/グループへの切替を
`syscall.Setegid(targetGID)` と `syscall.Seteuid(targetUID)` のみで行っており、
`setgroups(2)` を呼び出して補助グループ（supplementary groups）をクリア／再設定していません。
コードベース全体を検索しても `Setgroups` の呼び出しは存在しません。

**問題**
setuid-root バイナリとして配備した場合、プロセス起動時の補助グループは root の
補助グループ集合（例: `wheel`, `sudo`, `docker`, `adm` など）です。`run_as_user` で
一般ユーザーに切り替えても補助グループは root のものが残るため、起動された子プロセスは
「対象ユーザー + root の補助グループ」という、本来意図しない権限で動作します。
`docker` グループが残れば実質 root 相当、group 読み取り可能な機微ファイルへのアクセスなど、
権限分離の前提が崩れます。

**修正方針（案）**
- ユーザー切替時に、対象ユーザーの初期補助グループを `initgroups`(=`getgrouplist`+`setgroups`)
  相当で設定し、それ以外を破棄する。Go では `syscall.Setgroups()` を利用。
- 併せて **`os/exec` の `SysProcAttr.Credential{Uid, Gid, Groups, NoSetGroups:false}` を使う実装への移行を推奨**。
  これによりカーネルが exec 時に uid/gid/補助グループをアトミックに設定し、
  「プロセス全体を seteuid する」現行方式より安全（後述 F-2 も同時に解決）。
- 補助グループの決定に失敗した場合はフェイルクローズ（実行拒否）とする。

---

### F-2 [Medium] seteuid ベースの切替で saved-set-uid を明示的に落としていない

**概要**
特権実行は「プロセスの effective UID を一時的に変更 → コマンドを同期実行 → 復元」という
方式です（`WithPrivileges` 内で `execCmd.Run()`）。切替に `Seteuid`/`Setegid` を用いており、
real / saved-set-uid は明示的に変更していません。

**問題**
setuid-root 配備時、`run_as` 実行中の親プロセスは `euid=target, ruid=originalUser, suid=0`
となる瞬間があります。実行対象が非 setuid の検証済みバイナリであれば `execve` が
子プロセスの suid を euid に揃えるため、子から `seteuid(0)` で root へ戻ることは通常できません
（この点は現状の設計で緩和されています）。しかし、
- saved-set-uid=0 を保持したまま同一プロセス内で処理が進む区間があること
- 復元ロジックの一貫性を人手の順序制御に依存していること（`restoreUserGroupInternal` →
  `restorePrivileges` の二段構え）

は、将来の改修時に権限リークを生みやすい構造です。実際、`restorePrivilegesAndMetrics` は
複数の復元経路と emergency shutdown を組み合わせた複雑なロジックになっています。

**修正方針（案）**
- F-1 と同様、コマンド実行に限っては `SysProcAttr.Credential` によるカーネル側アトミック設定へ
  移行し、「親プロセスを seteuid する」区間そのものを排除する。
  ファイル検証など親プロセス内で特権が必要な処理のみ現行の `WithPrivileges` を残す。
- どうしても現行方式を維持する場合は、切替時に `setresuid`/`setresgid` を用いて
  real/effective/saved を一括設定し、権限復元後の不変条件チェック（既存の identityVerifier）に
  saved-set-uid の検証も加える。

---

### F-3 [Medium] マスキングがキー名パターン依存で、値の取りこぼしがある

**概要**
`redaction` パッケージの機微判定は、主にキー名／変数名に対する正規表現
（`password|token|secret|key|api_key` 等）で行われます。値に対する
`IsSensitiveValue` も同じキー向けパターンを流用しており、任意形式の秘密値
（例: 高エントロピー文字列、`AKIA...` 形式、JWT、private key ブロック）を
値そのものから検出する仕組みは限定的です。

**問題**
コマンドの引数・stdout/stderr・環境変数値に秘密情報が「キー名を伴わず」現れた場合、
ログや **Slack 通知** に平文で流出する可能性があります。Slack はホスト検証
（`slack_allowed_host`）で送信先は制限されますが、送信「内容」のマスキングは
上記パターンに依存します。加えて `--show-sensitive` 指定時は意図的に平文化されます。

**修正方針（案）**
- 値ベースの検出器を追加する: よく知られたトークン形式（AWS/GitHub/Slack/GCP 等の
  プレフィックス）、PEM/private key ブロック、`Bearer <token>`、URL 中の credential
  （`https://user:pass@host`）などのパターンマッチ。
- 高エントロピー文字列のヒューリスティック（長さ + base64/hex 比率 + Shannon エントロピー閾値）を
  Slack など外部送信経路に限って適用する。
- Slack 送信ペイロードは「許可リストされたフィールドのみ送る」ホワイトリスト方式にし、
  自由記述の本文へ生データを載せない。
- ドキュメントで「Slack にコマンド出力を載せる設定は避ける」旨を明示する。

---

### F-4 [Low] dry-run で未検証の設定ファイルを読み直して解析する

**概要**
`readAndVerifyFileWithReadFallback` は dry-run モードで検証に失敗した場合でも、
`os.ReadFile` で内容を読み直して処理を継続します（失敗は ResultCollector に記録）。

**問題**
dry-run は本番実行前のプレビュー用途であり、実行そのものは行わないため直接の被害は
限定的です。ただし、未検証（改ざんされ得る）内容が変数展開・リスク評価・
デバッグ出力（環境変数の由来表示等）に流れ込むため、
「dry-run では問題なし」と誤認させる／誤った安心を与えるリスクがあります。

**修正方針（案）**
- フォールバック読み込みで得た内容には「UNVERIFIED」ラベルを付し、dry-run 出力上でも
  明確に区別する。
- `--dry-run-fail-unverified` の既定挙動・ドキュメントを見直し、CI 用途では
  未検証を hard fail にする運用を推奨として明記する。

---

### F-5 [Low] record のディレクトリ権限チェックが警告のみ

**概要**
`cmd/record/main.go` は TOCTOU/ディレクトリ権限チェック
（`RunTOCTOUPermissionCheck`）を実行しますが、結果は警告ログのみで、
違反があっても処理を続行します。ハッシュディレクトリは `os.MkdirAll(dir, 0o750)` で作成。

**問題**
ハッシュDB は本システムの **信頼の起点** です。生成時に、書き込み可能な親ディレクトリや
world/group-writable なハッシュディレクトリが放置されると、攻撃者がハッシュレコードを
差し替え、後段の runner に「改ざんバイナリを検証済み」と誤認させる余地が生じます。
検証（runner）側は本番で固定ディレクトリ + strict 検証を行う一方、
生成（record）側は緩めのため、運用ミスを検知しても止めません。

**修正方針（案）**
- record 側でも、ハッシュディレクトリおよびその祖先に対する権限違反を検出した場合は
  `--force` などの明示指定がない限りフェイルクローズ（非ゼロ終了）にする。
- ハッシュディレクトリのパーミッションを `0o700`（または所有者 root + group 読み取り専用）に
  し、world/group-writable を明示的に拒否する。
- ドキュメントに「record は信頼できる管理者権限・クリーンな環境で実行すること」を明記。

---

### F-6 [Info] 128MB のファイルサイズ上限（可用性）

`SafeReadFile` / 各アナライザは 128MB（`MaxFileSize`）を超えるファイルを拒否します。
セキュリティ上はメモリ枯渇対策として妥当ですが、大型インタプリタや巨大バイナリを
検証対象にできない可用性上の制約になります。閾値の設定可能化、またはハッシュ計算
（ストリーミング済み）と解析（サイズ制限）で上限を分離する検討を推奨します。

---

### F-7 [Info] non-Linux 環境の TOCTOU 残余ウィンドウ

`openat2` 非対応環境では `safeOpenFileFallback` が
「親ディレクトリの非シンボリックリンク確認 → `O_NOFOLLOW` open → 再確認」の
二段階チェックで代替しています。実装は堅牢ですが、原理的に openat2 の原子性には
及ばず、極めて短い競合ウィンドウが残ります（コード内コメントでも認識済み）。
本番ターゲットを Linux + openat2 前提とする旨をドキュメントで明確化し、
macOS 等は開発・限定用途に限る運用ガイドを推奨します。

---

## 3. 良好に対策されている点（確認済み）

- **任意ハッシュディレクトリ攻撃**: 本番では `NewManagerForProduction` が固定ディレクトリを強制。
- **PATH 操作攻撃**: 固定 PATH。実行前にコマンドパスは絶対・シンボリックリンク解決済みを要求。
- **検証〜実行間 TOCTOU**: fd バインド実行（Linux）／検証済み inode のステージングコピー（非 Linux）。
- **危険な環境変数**: `LD_*` 全般、`GCONV_PATH`/`LOCPATH`/`HOSTALIASES`/`NLSPATH`/`RES_OPTIONS` を
  経路によらず最終環境から削除（`BuildProcessEnvironment`）。env_import でも拒否。
- **環境変数値のインジェクション**: null / 改行 / CR を拒否。コマンドは shell を介さず直接 exec のため
  shell メタ文字はリスクにならない設計。
- **変数展開の DoS/循環**: 再帰深さ・変数数・配列長・文字列長の上限、循環参照検出。
- **特権リーク検知**: 実行後・復元後の EUID==UID / EGID==GID 不変条件チェックと即時停止。
- **Slack 送信先**: `slack_allowed_host` によるホスト名検証。
- **間接実行の検知**: ラッパー・インラインシェル・動的ローダ・リモートシェル等をリスク評価で分類。

---

## 4. 推奨対応順序

1. **F-1（補助グループ破棄）** — run-as を setuid-root で使う場合の実害が大きく、最優先。
   `SysProcAttr.Credential` 移行で F-1・F-2 を同時に解決するのが望ましい。
2. **F-3（値ベースのマスキング強化）** — 外部（Slack）流出面を持つため次点。
3. **F-5 / F-4** — 信頼の起点（record）と dry-run の安全側デフォルトを固める。
4. **F-6 / F-7** — 運用ドキュメントでの前提明確化と設定可能化。

> 注記: 本レポートは静的レビューに基づく指摘であり、F-1/F-2 の実害範囲は配備形態
> （setuid-root か native-root か、run_as の使用有無）に依存します。実際の修正着手前に、
> 対象環境での挙動を PoC で確認することを推奨します。
