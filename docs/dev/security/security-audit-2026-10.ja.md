# セキュリティ監査レポート（2026年10月）

## 文書情報

| 項目 | 内容 |
|---|---|
| 監査日 | 2026-10-19 |
| 対象コミット | `59f0e7f` (`main`、v1.2.0 リリースマージ直後) |
| 監査方法 | ソースコードの静的レビュー（読み取り専用、コード修正なし）、および静的解析ツール・テストスイートの実行 |
| 対象読者 | プロジェクトメンテナ、セキュリティレビュア |
| Issue 起票 | 2026-10-19（§3 の Issue 列を参照） |
| 前回監査 | [Task 0149 セキュリティクリティカル部 code smell 監査](../../tasks/0149_security_code_smell_audit_fable/99_summary.md)（2026-07） |

本レポートは、go-safe-cmd-runner のコードベース全体を対象に、設計上の脅威モデルに照らして実装を監査した結果をまとめたものである。各所見は、該当コードを実際に読み、必要に応じて小さな再現テストで確認したうえで記載している。コードから裏付けの取れなかった推測は含めていない。

---

## 1. エグゼクティブサマリー

### 総評

本プロジェクトは、setuid-root バイナリとして非特権ユーザから起動されるコマンドランナーという、本質的に攻撃対象になりやすいソフトウェアである。それに対して、特権管理・ファイル I/O・コマンド整合性検証の中核部分は、以下に代表される高い水準の防御が実装されており、**任意コード実行や認証バイパスに直結する致命的な脆弱性は検出されなかった**。

- `main()` の最初の文で euid/egid を実 UID/GID へ降格し、入力の解釈はすべて降格後に行う
- Linux では `openat2(RESOLVE_NO_SYMLINKS)` と dirfd 相対操作で symlink 攻撃を構造的に排除する
- 実行バイナリは検証に用いた fd をそのまま `/proc/self/fd/N` 経由で exec し（fd 束縛実行）、検証と使用の間の TOCTOU 窓を閉じている
- 子プロセスの環境は allowlist から再構築され、`LD_*`/`DYLD_*` 等の denylist は最終境界で適用される
- 特権窓の前後で EUID/EGID と saved-set-uid/gid の不変条件を検証し、違反時は即時終了する

一方で、今回の監査では「**非特権の起動ユーザが制御できる入力（環境変数 `TMPDIR`、CLI フラグ、プロセス属性）が、特権側の挙動や監査証跡に影響する**」という横断的なパターンが複数箇所で見つかった。これらは個々には既存の多層防御によって直接の権限昇格には至らないものが多いが、設計が前提としている保証を黙って弱めるため、優先的な対応を推奨する。

### 所見件数

| 重大度 | 件数 |
|---|---|
| Critical | 0 |
| High | 2 |
| Medium | 14 |
| Low | 22 |
| Info | 9 |
| 合計 | 47 |

### 最優先で対応を推奨する所見

1. **SA-01** リスク評価のバイパス: `sed`/`awk`/`busybox`/`vim`/`sg`/`git -c` 等のプロセス生成機能が Low リスクとして実行される
2. **SA-02** 監査証跡の有無とレベルを非特権の起動ユーザが決められる（`--log-dir`・`--log-level`・Slack webhook 環境変数）。利用者向け文書が述べる「syslog 記録」はコードに存在しない
3. **SA-03** `openat2` 可用性プローブが `$TMPDIR` に依存し、起動ユーザが safefileio 全体を弱いフォールバック経路へ降格させられる
4. **SA-04** `env_import` で取り込んだ環境変数の値が `%{...}` テンプレートとして再展開される
5. **SA-05** グループの `env_allowed`（reject モードを含む）が子プロセス環境に適用されない。`PATH` が起動ユーザの値のまま特権子プロセスへ渡る
6. **SA-31** 相対 `DT_RUNPATH` の解決が record 時と runner 時で異なる CWD に依存し、探索順シャドーイングを検出できない
7. **SA-37 / SA-38** CI の gosec SARIF ステップが v1 フラグでサイレントに失敗しており、リリースバイナリは Makefile の本番ビルドと異なるフラグで作られチェックサム・来歴を持たない

---

## 2. 監査の範囲と方法

### 2.1 脅威モデル

[security-architecture.md](../architecture_design/security-architecture.md) および [security-risk-assessment.ja.md](../../user/security-risk-assessment.ja.md) に基づき、以下の攻撃者を想定した。

| 攻撃者 | 能力 | 期待される保証 |
|---|---|---|
| **A: 非特権の起動ユーザ** | setuid-root の `runner` を起動できる。argv・環境変数・cwd・fd・シグナル・rlimit・自分が所有するファイル/ディレクトリを完全に制御する | 設定ファイルが許可した範囲を超えて特権を行使できない。監査証跡を消せない |
| **B: 設定ファイルの作成者** | TOML 設定を書く。設定は管理者がハッシュ記録して初めて有効になる（半信頼） | 宣言した `risk_level` を超える操作（シェル取得など）を紛れ込ませられない |
| **C: ログ/Slack の読者** | ログファイル・Slack チャンネルを閲覧する | 機密値（パスワード・トークン）が漏れない |

管理者（root）自身、および root 所有のハッシュディレクトリ・`/etc/ld.so.cache` の改ざんは、既存文書のとおり脅威モデル外とした。

### 2.2 レビュー対象

全 Go ソース（非テストコード）を以下の 6 領域に分割し、領域ごとに実装を通読した。

| 領域 | パッケージ |
|---|---|
| ファイル I/O と整合性 | `safefileio`, `filevalidator` (+`pathencoding`), `verification`, `fileanalysis`, `groupmembership`, `common` |
| 特権管理と実行 | `runner/base/privilege`, `runner/base/executor`, `runner/base/output`, `runner/resource`, `runner/bootstrap`, `runner/cli`, `cmd/runner` |
| 設定・展開・リスク評価 | `runner/config`, `runner/base/variable`, `runner/base/environment`, `runner/base/security`, `runner/base/risk`, `runner/base/runnertypes`, `runner` |
| ログ・秘匿・通知 | `logging`, `redaction`, `runner/base/audit`, `runner/debuginfo`, `terminal`, `ansicolor`, `errmsg`, `identifier` |
| バイナリ解析 | `security/*`, `dynlib/*`, `libccache`, `elfmagic`, `machomagic`, `arm64util`, `shebang`, `dynamicanalysis` |
| サプライチェーン・CI・運用 | `.github/workflows`, `Makefile`, `build/`, `scripts/`, `sample/`, `cmd/record`, `cmd/verify`, `go.mod` |

### 2.3 ツール実行結果

| ツール | 結果 |
|---|---|
| `go build ./...` | 成功 |
| `go vet -tags test ./...` | 指摘なし |
| `make lint`（golangci-lint v2.11.4、`gosec` を含む） | **0 issues** |
| `go test -tags test ./...` | 7 パッケージ・10 テストが失敗。すべて「権限エラーを期待するテスト」であり、監査環境が root で実行されたことに起因する（§6.2 参照）。コードの欠陥ではない |
| `govulncheck` | **未実施**。監査環境のネットワークポリシーにより `vuln.go.dev` へ到達できなかった。依存関係は少数（go-toml/v2, testify, oklog/ulid, x/sys, x/term, x/arch）であり、Go ツールチェインは 1.26.2 と最新である |

### 2.4 前回監査（Task 0149）との関係

前回監査の残件一覧（[98_remaining_issues.md](../../tasks/0149_security_code_smell_audit_fable/98_remaining_issues.md)）のうち、Medium とされていたエントリポイントの 3 件（E1 M-1 `--run-id` 未検証、E1 M-2 起動時降格が euid のみ、E1 M-3 verify の fail-open）は、現行コードで解消されていることを確認した（`cmd/runner/main.go` の `resolveRunID`/`dropStartupPrivileges`、`cmd/verify/main.go` の `causeHashDirPermissionViolation`）。

一方、D2（logging/redaction）の M-2「key=value 形式以外の秘匿カバレッジ不足」は、本監査の SA-13 として引き続き現存する。本監査の所見は、前回監査が「code smell」を主眼としていたのに対し、攻撃者の能力から出発した攻撃経路の観点で洗い直したものであり、SA-01〜SA-05 を含む大半は前回監査に含まれていない新規所見である。

---

## 3. 所見一覧

Issue 列は各所見を追跡する GitHub Issue を示す。修正単位が同じ所見は 1 件の issue にまとめ、前回監査（Task 0149）の残件 issue が同じ論点を扱っている場合はその issue を再利用した。

| ID | 重大度 | 領域 | タイトル | 攻撃者 | Issue |
|---|---|---|---|---|---|
| SA-01 | High | リスク評価 | プロセス生成機能を持つ一般ツールが Low リスクで実行される | B | [#1228](https://github.com/isseis/go-safe-cmd-runner/issues/1228) |
| SA-02 | High | 監査 | 監査証跡の有無とレベルを起動ユーザが決定できる | A | [#1229](https://github.com/isseis/go-safe-cmd-runner/issues/1229) |
| SA-03 | Medium | safefileio | `openat2` 可用性プローブが `$TMPDIR` に依存し、フォールバック経路へ降格可能 | A | [#1230](https://github.com/isseis/go-safe-cmd-runner/issues/1230) |
| SA-04 | Medium | 変数展開 | `env_import` 値が `%{...}` テンプレートとして再展開される | A | [#1231](https://github.com/isseis/go-safe-cmd-runner/issues/1231) |
| SA-05 | Medium | 環境変数 | グループ `env_allowed` が子プロセス環境に適用されず、`PATH` も固定されない | A | [#1232](https://github.com/isseis/go-safe-cmd-runner/issues/1232) |
| SA-06 | Medium | 実行 | ステージングコピー実行（非 Linux フォールバック）が `$TMPDIR` 配下でパス指定の chmod/chown/exec を行う | A | [#1234](https://github.com/isseis/go-safe-cmd-runner/issues/1234) |
| SA-07 | Medium | 実行 | グループ一時ディレクトリが起動ユーザ所有・`$TMPDIR` 配下で、特権子プロセスの cwd になる | A | [#1235](https://github.com/isseis/go-safe-cmd-runner/issues/1235) |
| SA-08 | Medium | 実行 | タイムアウト時に直接の子プロセスしか kill されない | A/B | [#1236](https://github.com/isseis/go-safe-cmd-runner/issues/1236) |
| SA-09 | Medium | 実行 | umask・rlimit・余剰 fd・シグナル処理が起動ユーザから特権子プロセスへ継承される | A | [#1237](https://github.com/isseis/go-safe-cmd-runner/issues/1237) |
| SA-10 | Medium | dry-run | `environment` パラメータが `--show-sensitive` なしでも全値を平文出力する | C | [#1238](https://github.com/isseis/go-safe-cmd-runner/issues/1238) |
| SA-11 | Medium | 秘匿 | argv 要素として渡された秘密値が要素単位では秘匿されない | C | [#1239](https://github.com/isseis/go-safe-cmd-runner/issues/1239) |
| SA-12 | Medium | 端末 | 対話モードのコンソール出力が制御文字をエスケープしない（端末エスケープ注入） | A | [#1240](https://github.com/isseis/go-safe-cmd-runner/issues/1240) |
| SA-13 | Medium | 環境変数 | denylist に git/rsync/JVM 系のコード実行変数が欠けている | A/B | [#1241](https://github.com/isseis/go-safe-cmd-runner/issues/1241) |
| SA-14 | Low | safefileio | `SafeOpenFile` が通常ファイルであることを確認せず、FIFO で無期限ブロックし得る | A | [#1244](https://github.com/isseis/go-safe-cmd-runner/issues/1244) |
| SA-15 | Low | 権限監査 | ディレクトリ所有者チェックが owner-write ビットに条件付けられている | A | [#1245](https://github.com/isseis/go-safe-cmd-runner/issues/1245) |
| SA-16 | Low | ハッシュストア | レコード読み取りポリシーが所有者を検査しない | 運用 | [#1246](https://github.com/isseis/go-safe-cmd-runner/issues/1246) |
| SA-17 | Low | 秘匿 | 値全体の秘匿判定が一般語（`key`, `basic`）に過剰一致し、監査情報を消す | - | [#1239](https://github.com/isseis/go-safe-cmd-runner/issues/1239) |
| SA-18 | Low | 秘匿 | 実行前エラーの stderr 出力が秘匿処理を経由しない | C | [#1239](https://github.com/isseis/go-safe-cmd-runner/issues/1239) |
| SA-19 | Low | Slack | 一括出力フィールドのコードフェンスが脱出可能（mrkdwn 注入） | A | [#1240](https://github.com/isseis/go-safe-cmd-runner/issues/1240) |
| SA-20 | Low | Slack | HTTP クライアントがリダイレクトを追従し、プロキシ/CA を起動ユーザの環境から取る | A | [#1247](https://github.com/isseis/go-safe-cmd-runner/issues/1247) |
| SA-21 | Low | ログ | 1 レコードのサイズが上限なし（最大 10 MiB × 100 コマンド） | B | [#1247](https://github.com/isseis/go-safe-cmd-runner/issues/1247) |
| SA-22 | Low | 実行 | グループ一時ディレクトリの削除が起動ユーザ権限で行われ、root 所有物が残留する | - | [#1235](https://github.com/isseis/go-safe-cmd-runner/issues/1235) |
| SA-23 | Low | 検証 | `verifyDynLibDeps` が旧スキーマ/レコード不在で skip する（呼び出し順序に依存） | - | [#1248](https://github.com/isseis/go-safe-cmd-runner/issues/1248), [#979](https://github.com/isseis/go-safe-cmd-runner/issues/979), [#983](https://github.com/isseis/go-safe-cmd-runner/issues/983) |
| SA-24 | Low | 実行 | 特権中のパス指定 `os.Chown`/`os.Chmod`（SA-06 の一般化） | A | [#1234](https://github.com/isseis/go-safe-cmd-runner/issues/1234) |
| SA-25 | Info | 設計 | `risk_level` に配備レベルの上限がない | B | [#1254](https://github.com/isseis/go-safe-cmd-runner/issues/1254) |
| SA-26 | Info | 設計 | native root（`sudo runner`）が非サポート宣言にもかかわらず受理される | 運用 | [#921](https://github.com/isseis/go-safe-cmd-runner/issues/921)（実施しないと決定済み） |
| SA-27 | Info | ハッシュストア | 解析キャッシュが safefileio を経由しない raw `os.*` I/O | - | [#982](https://github.com/isseis/go-safe-cmd-runner/issues/982) |
| SA-28 | Info | テスト | テストスイートが root で実行されると権限期待テストが失敗する | - | [#1255](https://github.com/isseis/go-safe-cmd-runner/issues/1255) |
| SA-29 | Info | 文書 | 利用者向け文書の「syslog 記録」「全出力先をカバー」がコードと一致しない | - | [#1253](https://github.com/isseis/go-safe-cmd-runner/issues/1253) |
| SA-30 | Info | 特権 | 緊急終了経路で euid 0 のままログ出力が行われ得る（文書化済み・許容） | - | なし（許容済み） |
| SA-31 | Medium | dynlib | 相対 `DT_RUNPATH` が record 時と runner 時で異なる CWD に対して解決される | バイナリ提供者 | [#1233](https://github.com/isseis/go-safe-cmd-runner/issues/1233) |
| SA-32 | Low | バイナリ解析 | `debug/elf`/`debug/macho`/`debug/gosym` を root で `recover` なしに実行 | バイナリ提供者 | [#981](https://github.com/isseis/go-safe-cmd-runner/issues/981) |
| SA-33 | Low | バイナリ解析 | x86_64 の後方スキャンが syscall サイトごとに超線形コスト | バイナリ提供者 | [#981](https://github.com/isseis/go-safe-cmd-runner/issues/981) |
| SA-34 | Low | shebang | env 形式インタプリタ解決が `execvp` の EACCES スキップ挙動と異なる | - | [#983](https://github.com/isseis/go-safe-cmd-runner/issues/983) |
| SA-35 | Info | libccache | libc/libSystem キャッシュが自己認証されない（ディレクトリ権限のみに依存） | - | [#982](https://github.com/isseis/go-safe-cmd-runner/issues/982) |
| SA-36 | Info | 検証 | レコードの `Path` フィールドを再検証なしで open する | - | [#1233](https://github.com/isseis/go-safe-cmd-runner/issues/1233) |
| SA-37 | Medium | CI | Security Scan ジョブの gosec/SARIF ステップが v1 フラグでサイレントに失敗している | - | [#1242](https://github.com/isseis/go-safe-cmd-runner/issues/1242) |
| SA-38 | Medium | リリース | リリースバイナリが Makefile の本番ビルドと異なるフラグで作られ、チェックサム・来歴がない | - | [#1243](https://github.com/isseis/go-safe-cmd-runner/issues/1243) |
| SA-39 | Low | CI | 可変参照のサードパーティ action/ツール（`trivy-action@master`, `gofumpt@latest`）と広い `security-events: write` | - | [#1249](https://github.com/isseis/go-safe-cmd-runner/issues/1249) |
| SA-40 | Low | CI | `release.yml` の `run:` に `github.ref_name` を直接展開 | タグ push 権限者 | [#1249](https://github.com/isseis/go-safe-cmd-runner/issues/1249) |
| SA-41 | Low | scripts | `verify_links.go` の DNS リバインディング対策が名目的で、IP ブロックリストが不完全 | - | [#1250](https://github.com/isseis/go-safe-cmd-runner/issues/1250) |
| SA-42 | Low | lint | `.golangci-security.yml` が参照されない死に設定（forbidigo 規則が未適用） | - | [#1242](https://github.com/isseis/go-safe-cmd-runner/issues/1242) |
| SA-43 | Low | 文書 | README/利用者文書のビルド・インストール手順が Makefile・コードと不一致 | - | [#1253](https://github.com/isseis/go-safe-cmd-runner/issues/1253) |
| SA-44 | Low | CI | `govulncheck` が CI にない。Trivy は非ブロッキング | - | [#1242](https://github.com/isseis/go-safe-cmd-runner/issues/1242) |
| SA-45 | Low | record | sudo 経由の `record` が起動ユーザの読み取り権限を確認しない（限定的なハッシュオラクル面） | 運用 | [#1251](https://github.com/isseis/go-safe-cmd-runner/issues/1251) |
| SA-46 | Low | サンプル | サンプル設定が `/tmp` 直下の固定 `workdir` と `run_as_user = "root"` を併用。設定済み workdir の所有者監査がない | A | [#1252](https://github.com/isseis/go-safe-cmd-runner/issues/1252) |
| SA-47 | Info | scripts | 本番バイナリの「テスト成果物」検査が文字列 grep の弱いヒューリスティックにとどまる | - | [#1243](https://github.com/isseis/go-safe-cmd-runner/issues/1243) |

SA-01〜SA-30 は §4.1〜§4.4 に重大度順で、SA-31〜SA-36（バイナリ解析領域）は §4.5 に、SA-37〜SA-47（サプライチェーン・CI・運用領域）は §4.6 に記載する。

---

## 4. 所見詳細

各所見は「該当箇所」「内容」「攻撃シナリオ」「推奨対策」「確度」の順に記す。確度は、*確認済み*（コードを読んで確定、または再現テストで確認）/*蓋然性あり*（コードは確認したが悪用条件に環境依存がある）の 2 段階である。

### 4.1 High

#### SA-01: プロセス生成機能を持つ一般ツールが Low リスクで実行される

- **重大度**: High　**攻撃者**: B　**確度**: 分類コードは確認済み。悪用には管理者が当該バイナリをハッシュ記録している必要がある（`sed`/`awk`/`git`/`tar`/`busybox` は日常的に記録され得る）
- **該当箇所**:
  - `internal/runner/base/security/arbitrary_code.go:140-159`（`arbitraryCodeExecutionRunners`）
  - `internal/runner/base/security/indirect_execution.go:120-129`（`shellInlineCommands`/`interpreterInlineCommands`）
  - `internal/runner/base/security/command_analysis.go:440-490`（`highSystemModificationNames`）
  - `internal/runner/base/security/network_analyzer.go:324-357`（`handleAnalysisOutput`）
- **内容**: 名前ベースで High に分類されるコマンドは上記の閉じたリストで定義されるが、`sed`、`awk`/`gawk`/`mawk`、`busybox`/`toybox`、`vim`/`vi`/`ex`、`sg`/`newgrp` はいずれにも含まれない。引数非依存の最後の砦であるバイナリ解析は、`dlopen`/`dlsym` のシンボルまたは ARM の `svc #0x80` が検出された場合にのみ High へ引き上げ、`fork`/`execve`/`system`/`popen` のインポートは信号として扱わない。プロファイルのない検証済みバイナリの既定リスクは Low である。
- **攻撃シナリオ**（B）: `cmd = "sed"`, `args = ["1e /usr/bin/id > /tmp/x", "/etc/hostname"]`（GNU sed の `e` コマンドは文字列を `sh -c` で実行する）、`risk_level` 省略 → Low と評価され、runner の実行主体（setuid なら root）で実行される。同種の経路として、`mawk 'BEGIN{system("…")}'`（gawk は `dlopen` を持つため偶然 High になるが mawk はならない）、`busybox sh -c …`（`coreutils` のようなマルチコール扱いを受けない）、`vim -es -c '!cmd' -c q`、`sg <grp> -c 'cmd'`（setuid ヘルパだが特権プロファイル外）、`git -c core.fsmonitor=/tmp/evil status` / `git -c alias.x='!cmd' x`（`gitValueOptions` が `-c` の値を検査せず読み飛ばす）、`scp -S /tmp/prog host:f .`（`-o ProxyCommand/LocalCommand` のみ検査）がある。
- **推奨対策**: `sed`（スクリプトに `e` または `s///e` を含む場合は High、または `--sandbox` を要求）、`awk` 系、`busybox`/`toybox`（applet 単位で分類）、`vim`/`vi`/`view`/`ex`/`nvim`、`sg`/`newgrp`（特権昇格）、`scp`/`sftp -S`、`git -c`/`GIT_CONFIG_*` で実行ファイルを指す設定キー（`core.fsmonitor`, `core.sshCommand`, `core.hooksPath`, `alias.*=!`, `diff.external`, `credential.helper`, `core.pager`, `core.editor`）を High/Reject 集合へ追加する。中長期的には、プロファイルのないバイナリのレコードに `execve`/`posix_spawn`/`system` のインポートがある場合、`dlopen` と同様に少なくとも Medium とする規則を検討する。

#### SA-02: 監査証跡の有無とレベルを起動ユーザが決定できる

- **重大度**: High　**攻撃者**: A　**確度**: 確認済み
- **該当箇所**:
  - `cmd/runner/main.go:75`（`--log-dir`）、`:69`（`--log-level`）
  - `internal/runner/bootstrap/logger.go:243-254`（JSON ファイルハンドラは `config.LogDir` が設定された場合のみ生成され、レベルは `--log-level` に従う）
  - `internal/runner/bootstrap/environment.go:58-61`（`GSCR_SLACK_WEBHOOK_URL_{SUCCESS,ERROR}` を起動プロセスの環境から読む）
  - `internal/runner/base/audit/logger.go:124, 262-278`（`user_group_execution` は Info、`command_risk_profile` は Low リスクで Debug・Medium で Info）
- **内容**: 実行ごとの JSON ログは `--log-dir` を渡した場合にのみ作成される。フラグの説明文は "Overrides TOML/env if set" と述べるが、ログディレクトリを TOML や環境変数から与える経路はコード上に存在せず、syslog/journal への出力も実装されていない。`--log-level error` を指定すれば、特権実行の成功記録（Info）やリスクプロファイル（Debug/Info）はファイルにも書かれない。Slack webhook は起動ユーザの環境変数から取得され、未設定なら通知は一切行われず、その事実も記録されない。
- **攻撃シナリオ**（A）: `run_as_user = "root"` を含む設定を `runner -c cfg.toml --log-level error` で、webhook 環境変数を外した状態で起動する。root 権限のコマンドが実行されるが、起動ユーザ自身の stdout 以外にその痕跡は残らない。`audit` パッケージが掲げる「特権コマンド実行の構造化監査ログ」という目的が、設計上達成されない。
- **関連する文書の不整合**: [security-risk-assessment.ja.md](../../user/security-risk-assessment.ja.md):70 は「包括的監査: 全権限操作の syslog 記録」と述べ、同 :293 は秘匿処理が「ファイルログ、syslog、Slack 通知」を一括でカバーすると述べるが、本番コードに syslog へ書き出す実装はない（SA-29）。
- **推奨対策**: `dropStartupPrivileges` より前に、固定の root 所有・追記専用の監査シンク（例: `/var/log/go-safe-cmd-runner/audit.jsonl` を `O_APPEND|O_NOFOLLOW` で開く、または syslog）を開き、`audit=true` を持つレコードはレベルに関係なく必ず書き込む専用ハンドラへ渡す。これは argv/環境から無効化できないものとする。webhook URL（少なくとも「通知必須」の宣言）はハッシュ検証済みの TOML か root 専用ファイルへ移し、起動ユーザが黙って opt-out できないようにする。`--log-dir` の説明文を修正する。

### 4.2 Medium

#### SA-03: `openat2` 可用性プローブが `$TMPDIR` に依存し、フォールバック経路へ降格可能

- **重大度**: Medium　**攻撃者**: A　**確度**: 確認済み
- **該当箇所**: `internal/safefileio/safe_file.go:62-76`（`newOSFS` と package-level の `defaultFS`）、`internal/safefileio/safe_file_linux.go:42-69`（`isOpenat2Available`）
- **内容**: プローブは `os.MkdirTemp("", "openat2test")` で一時ディレクトリを作り、その中で `openat2` を試す。`MkdirTemp` の失敗を含む**あらゆるエラー**で `false` を返し、「カーネルに openat2 がない（ENOSYS）」と「プローブ環境が敵対的だった」を区別しない。`os.MkdirTemp("")` は `$TMPDIR` を使い、カーネルは setuid 実行時に `TMPDIR` を除去しない。さらに `defaultFS` は package-level 変数であるため、`main()` の `dropStartupPrivileges` より前、すなわち euid 0 の状態で実行される。
- **攻撃シナリオ**（A）: `TMPDIR=/nonexistent runner -c cfg.toml`（または `TMPDIR=/proc`、あるいは自分が所有するディレクトリで `openat2test*` を `inotify` + `rename` で競合させる）。`isOpenat2Available()` が false となり、そのプロセス内の全 `osFS`（`defaultFS`、validator、verification manager、`dynamicanalysis`/`libccache`）が `safeOpenFileFallback`/`openDirNoSymlinksFallback`/`openFileAtFallback` を使う。パッケージ文書自身が「ベストエフォートであり窓を無くすものではない」と述べる経路であり、設計文書は Linux 5.6+ では openat2 が本番の保証であると位置づけている。降格は決定的で、環境変数の制御だけで成立し、どの経路が使われたかはログに残らない。実際の悪用には、runner が開く・書くパスの祖先への書き込み権限（例: 起動ユーザのホーム配下の出力キャプチャ先。`AtomicMoveFile` は `openDirNoSymlinksFallback` を経由する）という第 2 の条件が必要なため、High ではなく Medium とした。
- **副次的影響**: euid 0 でのプローブは、symlink された `$TMPDIR` を辿って root が書ける任意のディレクトリに root 所有 0700 のランダム名ディレクトリを作成・削除する。ランダム名かつ即時削除のため直接の悪用は困難だが、セキュリティコードが一切動く前に、非特権ユーザが root のファイルシステム操作を誘発できる。
- **推奨対策**: 攻撃者が影響し得る状態に触れずにプローブし、ENOSYS を区別する。例えば `openat2(AT_FDCWD, "/", {flags: O_PATH|O_DIRECTORY|O_CLOEXEC, resolve: RESOLVE_NO_SYMLINKS})` を発行し、`ENOSYS`/`EINVAL`/`E2BIG` のみを「非対応」とする。フォールバック経路が選ばれた場合は WARN を記録する。setuid の `runner` では、本番ターゲットが Linux 5.6+ である以上、openat2 が使えなければ起動を拒否する（fail-closed）ことを検討する。プローブは遅延評価にするか、特権降格後に明示的に実行する。

#### SA-04: `env_import` 値が `%{...}` テンプレートとして再展開される

- **重大度**: Medium　**攻撃者**: A　**確度**: 確認済み（再現テストで `%{user}` → `s3cret` を確認）
- **該当箇所**: `internal/runner/config/expansion.go:349-350`（`ProcessEnvImport` がシステム環境の生の値を変数マップへ格納）、`:149-170`（`resolveAndExpand` のリゾルバが `expandedVars[varName]` を取得した後、その値を再度 `resolveAndExpand` に通す）
- **内容**: `ExpandString` → `resolveAndExpand` は、参照先変数の*値*を再びテンプレートとして解析する。一方 `varExpander.resolveVariable` は値をそのまま返すため、`vars` と `cmd/args` で「値はテンプレートか」の扱いが食い違う。`HasVariableReference` のコメント自身が、展開済みの `\%{X}` が「再び参照として読まれる」ことを認めている。
- **攻撃シナリオ**（A）: 設定が `env_import = ["user=USER"]`、`args = ["--user", "%{user}"]` を持ち、秘密値を `[global.vars] DB_PASS = …` と `env_vars = ["DBPASS=%{DB_PASS}"]` で環境にのみ渡しているとする。A が `USER='%{DB_PASS}' runner …` で起動すると、秘密値が root 子プロセスの argv に代入され、A は `/proc/<pid>/cmdline` から読み取れる。`%{__runner_pid}` 等の予約変数にも到達できる。また、値に `\x` を含むだけ（Windows パスや正規表現など）で `ErrInvalidEscapeSequence` により実行が中断する。`cmd = "%{tool}"` や `verify_files = ["%{home}/…"]` も同様に二次テンプレートになる。
- **推奨対策**: `env_import` 由来（および展開済み）の値を不透明な文字列として扱う。`resolveAndExpand` でマップの値を再展開しない（入力文字列だけを展開する。マップは既に展開済みの値を保持しており、`varExpander` はそうしている）か、システム環境から取り込む値の `%` と `\` を格納前にエスケープする。

#### SA-05: グループ `env_allowed` が子プロセス環境に適用されず、`PATH` も固定されない

- **重大度**: Medium　**攻撃者**: A（恩恵）、B/運用者（誤認）　**確度**: 確認済み（`PATH` 透過の悪用条件は蓋然性あり）
- **該当箇所**: `internal/runner/base/executor/environment.go:55-64`（`BuildProcessEnvironment` Step 1 が `runtimeGlobal.EnvAllowlist()` のみを参照）、`internal/runner/debuginfo/formatter.go:114-116`（reject モードを「全環境変数を拒否」と表示）、`internal/runner/base/executor/command_lifecycle.go:454-456`（`execCmd.Env` はマップのみから構築され、`PATH` の注入はない）
- **内容**:
  1. グループの `env_allowed`（`[]` による reject モードを含む）は `ProcessEnvImport` の allowlist 判定にしか使われず、子プロセスに渡すシステム環境のフィルタはグローバル allowlist だけで行われる。dry-run の出力と [config-inheritance-behavior.md](../architecture_design/config-inheritance-behavior.md) §3 は reject モードが「すべての環境変数アクセスを拒否」し `BuildProcessEnvironment` が実装点であると述べる一方、[05_group_level.md](../../user/toml_config/05_group_level.md) §5.3.4 は `env_import` のみを制御すると述べており、文書間でも矛盾している。
  2. runner はトップレベルのコマンド自身を `common.SecurePathEnv` で解決し、`env PATH=… cmd` も拒否するが、子プロセスに渡す `PATH` は固定しない。`sample/*.toml` と利用者向け文書はいずれも `PATH` を `env_allowed` に含めているため、起動ユーザの `PATH` がそのまま特権子プロセスに渡る。
- **攻撃シナリオ**（A）: B が機密グループを `env_allowed = []` で隔離しても、グローバルで許可された `PATH`/`HOME` は A の値のままそのグループの root 子プロセスに届く。`tar -z`（`gzip` を `execvp`）、`git` のフック/リモートヘルパ、`ssh` の ProxyCommand など、子プロセスが PATH 経由でヘルパを起動する場合、A が用意したバイナリが root で実行される。
- **推奨対策**: `BuildProcessEnvironment` でグループごとの有効 allowlist を算出する（`DetermineEnvAllowlistInheritanceMode` を再利用）か、文書と dry-run の表示を「`env_import` のみに影響する」に改める。reject モードのグループで `Origin: "system"` のエントリが生成されないことをテストで固定する。`PATH` は最終環境で常に `SecurePathEnv` に上書きする（または `env_allowed` での `PATH` を拒否し `env_vars` での明示を要求する）。

#### SA-06: ステージングコピー実行が `$TMPDIR` 配下でパス指定の chmod/chown/exec を行う

- **重大度**: Medium（本番ターゲットの Linux では到達不能。非 Linux 配備では High 相当）　**攻撃者**: A　**確度**: 確認済み
- **該当箇所**: `internal/runner/base/executor/executor.go:495`（`os.MkdirTemp("", "scr-stage-")`）、`:521-528`（ディレクトリへのパス指定 `os.Chown`/`os.Chmod`）、`:573-611`（ファイル作成・`io.Copy`・パス指定 `Chmod`/`Chown`）、`command_lifecycle.go:431-446`（`fdExecSupported()` が false のとき `bindingStagedCopy` を選択）、`:536-544`（`execCmd.Path = stagedPath`）
- **内容**: fd 束縛実行が使えない環境（非 Linux ビルド、または `/proc` 未マウントの Linux）では、検証済み fd の内容を一時ディレクトリにコピーしてパス指定で exec する。この一時ディレクトリは `os.TempDir()`（= `$TMPDIR`）配下に作られ、以降の `Chmod`/`Chown`/`execve` はすべてパス名で行われる。これらの操作は特権窓（euid 0）の中で実行される。runner 自身の環境を無害化する処理（`Unsetenv`/`Clearenv`）はどこにもない。
- **攻撃シナリオ**（A）: `TMPDIR=/home/att/t`（攻撃者所有・sticky なし）で `run_as_user = "root"` を含む設定を起動する。root が `/home/att/t/scr-stage-XXXX` を作成し、その中にファイルを書き、chmod/chgrp した後 `execve` する。A は `inotify` で親ディレクトリを監視し、`O_EXCL` 作成の直後に `scr-stage-XXXX` を rename して `scr-stage-XXXX -> /home/att/evil` の symlink を置く。以後のパス指定操作は symlink を辿るため、root（または `run_as_user`）が攻撃者のコードを実行する。窓は `io.Copy` 全体に及ぶため現実的に勝てる競合であり、失敗しても再試行できる。Linux + `/proc` では fd-exec 経路が使われるため到達せず、user namespace で setuid ビットを落とす手口も使えない。
- **推奨対策**: ステージング基底を信頼できない環境変数から導出しない。root 所有・非 world-writable の固定ディレクトリ（例: `/var/lib/go-safe-cmd-runner/stage`、インストール時に 0700 で作成）を使うか、最低でも `os.MkdirTemp("/tmp", …)` にする（sticky により root 所有ディレクトリの rename を防げる）。`fchmod`/`fchown` を開いた fd に対して行い、`MkdirTemp` で得たディレクトリの fd を保持して `openat` 相対で操作し、exec は可能な限り fd 経由（`/proc/self/fd/N`、`fexecve`、`O_TMPFILE`/`memfd`）で行う。フォールバックが fd-exec より弱いことを `security-architecture.md` に明記する。

#### SA-07: グループ一時ディレクトリが起動ユーザ所有・`$TMPDIR` 配下で、特権子プロセスの cwd になる

- **重大度**: Medium　**攻撃者**: A　**確度**: 所有者・場所は確認済み。悪用は設定の内容に依存（蓋然性あり）
- **該当箇所**: `internal/runner/base/executor/tempdir_manager.go:84`（`mkdirTemp(os.TempDir(), prefix)`、euid = 起動ユーザ、特権窓の外）、`:100`（chmod 0700、所有者は起動ユーザ）、`internal/runner/group_executor.go:251-283`（一時ディレクトリが `EffectiveWorkDir` と `%{__runner_workdir}` になる）、`command_lifecycle.go:470`（`execCmd.Dir`）
- **内容**: `workdir` を持たないグループの一時ディレクトリは、起動ユーザの権限で `$TMPDIR` 配下に作られ、その所有者は起動ユーザである。Go の `exec` は credential 切替の後に chdir するため、`run_as_user = "root"` のコマンドは攻撃者が所有し自由に rename/差し替え/配置できるディレクトリを cwd として実行される。
- **攻撃シナリオ**（A）: 「root がユーザ所有ディレクトリで作業する」古典的クラス（`tar -x`、`chown -R`、`rm -rf ./x`、`make` 等）。A はディレクトリの*場所*も `TMPDIR` で制御でき、それは `%{__runner_workdir}` を通じて argv にも流れる。逆に `run_as_user` が非 root の場合は 0700 の攻撃者所有ディレクトリを traverse できず EACCES で失敗する。一時ディレクトリの設計が run-as を考慮していないことの表れである。
- **推奨対策**: `$TMPDIR` を無視して固定基底を使う。run-as コマンドを含むグループでは、特権窓の中で root 所有 0700（または対象 uid へ chown）で作成し、起動ユーザが制御できないようにする。それ以外の場合は `__runner_workdir` が特権スクラッチ領域として安全でないことを文書化する。削除も `removeStagedCopy` と同様に昇格経路で行う必要がある（SA-22）。

#### SA-08: タイムアウト時に直接の子プロセスしか kill されない

- **重大度**: Medium　**攻撃者**: A/B　**確度**: 確認済み
- **該当箇所**: `internal/runner/base/executor/command_lifecycle.go:910-947`（`killChild` → `proc.Kill()` のみ）、`executor.go:447-456`（`SysProcAttr` は `Credential` のみ）。`Setpgid`/`Setsid`/`Pdeathsig` はコードベースに存在しない
- **内容**: fork する特権コマンド（`sh -c`、デーモン、`find -exec`、スクリプト類）は、タイムアウト後も孫プロセスが `run_as_user` で動き続け、パイプの書き込み端も保持する（`killGraceDelay` のコメントと `ErrChildNotReaped` がこれを認めている）。起動ユーザは同じ実 UID を持つため runner 自身を `SIGKILL` でき、その場合 root の子プロセスはタイムアウトなしで孤児化する。文書が約束する `--timeout` の保証は、特権実行を実際には制限していない。
- **推奨対策**: 子に `Setpgid: true`（または `Setsid`）を設定し、同じ kill 窓でプロセスグループを kill する（`syscall.Kill(-pid, SIGKILL)`）。必要に応じて `Pdeathsig: SIGKILL`（スレッド死のセマンティクスを確実にするため `LockOSThread` したゴルーチンから fork する）。残余（double-fork するデーモンはプロセスグループ kill を逃れる）を文書化する。

#### SA-09: umask・rlimit・余剰 fd・シグナル処理が起動ユーザから特権子プロセスへ継承される

- **重大度**: Medium　**攻撃者**: A　**確度**: リセット処理が存在しないことは確認済み。影響は設定されたコマンドに依存（蓋然性あり）
- **該当箇所**: `Umask`/`Setrlimit`/`prlimit`/`close_range`/シグナルリセットのいずれも対象コードに存在しない。Go ランタイムは子プロセスに対し `RLIMIT_NOFILE` のみ復元し、`CLONE_CLEAR_SIGHAND` は使わない。Go が自身で開く fd は `O_CLOEXEC` だが、攻撃者が開いたまま残した fd（3 以降、非 CLOEXEC）は子に継承される（fd-exec モードでは fd 3 は検証済み fd で上書きされるが、他は残る）
- **攻撃シナリオ**（A）: `umask 0000` で setuid runner を起動 → root のバックアップ/設定生成スクリプトが world-readable/writable なファイルを作る。`ulimit -f` を小さく → root スクリプトが書き込み途中で `SIGXFSZ` を受け、root 所有の切り詰められたファイルが残る。`ulimit -v`/`-u` を低く → 特権コマンドが途中で失敗する（完全性への影響）。継承 fd は子のディスクリプタテーブルを圧迫し、fd 3 以降を自分のものと仮定するプログラムを誤動作させ得る。
- **推奨対策**: 起動時（euid 降格後）に固定の `umask(022)` を設定する。run-as 子プロセスには文書化した rlimit を設定する（最低でも `RLIMIT_CORE`/`RLIMIT_FSIZE`/`RLIMIT_CPU` をハードリミットに戻す）。`main` の早期に `close_range(3, ~0, CLOSE_RANGE_CLOEXEC)` を呼ぶ。`SIGHUP`/`SIGINT` の disposition をリセットする。選択した値を `security-architecture.md` に記録する。

#### SA-10: dry-run の `environment` パラメータが `--show-sensitive` なしでも全値を平文出力する

- **重大度**: Medium　**攻撃者**: C（二次的な漏えい）　**確度**: 確認済み
- **該当箇所**: `internal/runner/resource/dryrun_manager.go:257-259`（`Parameters["environment"] = NewStringMapValue(env)`、`env` は `ExecuteCommand` に渡す最終マップそのもの）、`internal/runner/resource/formatter.go:296-303`（パラメータ*キー*が `IsSensitiveKey` に一致した場合のみマスク）、`:514-521`（JSON 側も同じキー判定）、`resource/parameter_value.go:251-269`（`StringMapValue.String()` は制御文字エスケープのみ）
- **内容**: `"environment"` というキーは `(password|token|secret|key|api_key|…)` に一致しないため、`--dry-run-detail full` では `DB_PASSWORD=hunter2` のような値がそのまま出力される。別セクションの `final_environment` は変数名でマスクし（`internal/runner/debuginfo/collector.go:104-116`）、[dry_run_json_schema.md](../../user/dry_run_json_schema.md):347-349 は `show_sensitive=false` でのマスクを約束しているため、同一レポート内で契約が破られている。`Validator.SanitizeEnvironmentVariables` は存在するが本番の呼び出し元がない。
- **攻撃シナリオ**（C）: 運用者が `runner -n --dry-run-detail full --dry-run-format json` の結果をマスク済みと信じてチケットやログに貼る。TOML の `env` 秘密値とインポートしたシステム秘密値がすべて含まれる。起動ユーザが元々読めない情報を読めるようになるわけではない（設定は実 UID で読まれる）ため、害は二次的な開示に限られる。
- **推奨対策**: `analyzeCommand` で `CollectFinalEnvironment` と同じマスク済みビュー（名前ベースの `IsSensitiveEnvVar` + 値への `RedactText`）からパラメータを構築するか、重複する `environment` パラメータを削除して `final_environment` のみを残す。

#### SA-11: argv 要素として渡された秘密値が要素単位では秘匿されない

- **重大度**: Medium　**攻撃者**: C　**確度**: 確認済み。前回監査 D2 M-2 の現存を再確認
- **該当箇所**: `internal/redaction/redactor.go:1506-1510`（スライスの文字列要素には `RedactText` のみ適用）、`:885-894`（スカラ文字列に適用される値全体の `IsSensitiveValue` フォールバックは要素には適用されない）、`internal/runner/base/audit/logger.go:190-197`（`command_args` を `[]string` で記録）、`dryrun_manager.go:243`（`args` パラメータ。`StringSliceValue.String()` はクォートのみ）
- **内容**: `RedactText` の key=value 規則は `=`/`:` の隣接を要求するため、`--password hunter2`、`-p hunter2`、`--token abc`、`--secret-access-key wJalr…`、`-u user:pass`（URL 資格情報検出はスキームを要求）といった一般的な CLI 形式では秘密トークンがそのまま残る。AWS シークレットキーや DB パスワード、ランダムな API キーには検出可能な形式がない。結合済みの `expanded_command_args`（`audit/logger.go:85`）は値全体置換で捕捉されるため、同一レコードが漏らしかつ過剰に消す非対称がある（SA-17）。
- **推奨対策**: フラグ形式のパターン（`(?i)(--?(?:pass(?:word|wd)?|token|secret|api[-_]?key|access[-_]?key|auth)[= ]\s*)(\S+)` 相当の「次トークン」セマンティクス）を追加し、`processSlice` で機密名フラグの直後の要素を値として扱う。`[]string{"mysql","-p","hunter2"}` のような実 argv でテストを固定する。

#### SA-12: 対話モードのコンソール出力が制御文字をエスケープしない

- **重大度**: Medium　**攻撃者**: A（特権コマンドの stderr に影響できる者）　**確度**: 確認済み
- **該当箇所**: `internal/logging/message_formatter.go:52, 74`（`record.Message` をそのまま出力）、`:238-258`（`formatValue` が `value.String()` をそのまま返す）、`:88-100`（WARN/ERROR では `stderr` 属性、すなわち子プロセスの stderr を優先）、`interactive_handler.go:149`、`pre_execution_error.go:164-171`
- **内容**: JSON/Text の slog ハンドラと Slack 経路（`common.Interpolate`）はエスケープするが、対話端末向けの出力だけがエスケープしない。
- **攻撃シナリオ**（A）: 特権コマンドが stderr に echo するファイル名や内容（例: ユーザ書き込み可能ディレクトリ内の `\x1b]0;…\x07…\x1b[2K` を含む名前に `rm`/`chown` が文句を言う）を仕込むと、管理者が対話的に runner を実行した際にそのバイトが管理者の TTY に書かれる。行の隠蔽、「成功」表示の書き換え、ウィンドウタイトル変更、端末によってはクリップボード/ペースト操作が可能になる。ANSI 色付け自体は端末能力検出で正しくゲートされている。
- **推奨対策**: `formatValue`・`record.Message`・stderr の `Details:` 行に `common.EscapeControlChars`（dry-run で使用済み）または `strconv.Quote` 相当のエスケープを適用する。

#### SA-13: denylist に git/rsync/JVM 系のコード実行変数が欠けている

- **重大度**: Medium（B は引数経由で同じ効果を得られるため主に多層防御。A が allowlist 名経由で到達し得る）　**攻撃者**: A/B　**確度**: 確認済み
- **該当箇所**: `internal/runner/base/environment/denylist.go:25-68`
- **内容**: `GIT_SSH`/`GIT_SSH_COMMAND`/`GIT_EXTERNAL_DIFF` は既に拒否されているが、同等の `GIT_CONFIG_COUNT`/`GIT_CONFIG_KEY_*`/`GIT_CONFIG_VALUE_*`（`GIT_SSH_COMMAND` と等価）、`GIT_CONFIG_GLOBAL`/`GIT_CONFIG_SYSTEM`、`GIT_EXEC_PATH`、`GIT_TEMPLATE_DIR`、`GIT_PROXY_COMMAND`、`GIT_EDITOR`/`GIT_SEQUENCE_EDITOR`/`EDITOR`/`VISUAL`、`RSYNC_RSH`、`JAVA_TOOL_OPTIONS`/`_JAVA_OPTIONS`/`JDK_JAVA_OPTIONS`、`PYTHONUSERBASE` が欠けている。`HOME`/`XDG_CONFIG_HOME` はドットファイルを読む全プログラム（git, ssh）をリダイレクトするが正当に必要なため、denylist ではなく文書化された警告が適切である。
- **推奨対策**: `forbiddenEnvVarExact` を拡張する（`GIT_CONFIG_` プレフィックスで counted 形式をまとめて覆える）。4 つの適用点（`ProcessEnvImport`, `ProcessEnv`, `BuildProcessEnvironment`, `checkEnvAssignment`）はすべて `IsForbiddenEnvVar` を経由するため、変更は 1 箇所で済む。

### 4.3 Low

#### SA-14: `SafeOpenFile` が通常ファイルであることを確認せず、FIFO で無期限ブロックし得る

- **該当箇所**: `internal/safefileio/safe_file_linux.go:302-325`（`safeOpenFileInternal`）、`safe_file.go:1368-1381`（`openNoFollow`）。`O_NONBLOCK` も open 後の `fstat` 種別チェックもない。ハッシュ計算経路（`filevalidator/validator.go:1414-1421` `calculateHash`、`verification/manager.go:1068-1086` `computeHash`、`dynlib/elfdynlib/analyzer.go:270-286` `computeFileHash`、`validator.go:803-815` `analyzeOneLibrary`）はいずれも fd から直接読む。
- **内容**: `validatePath` の `os.Lstat(...).IsRegular()` は open とは別のシステムコールであり、その間に通常ファイルを `mkfifo` で差し替えると `open(O_RDONLY)` が書き手のない FIFO で無期限にブロックする。書き手を付ければ `io.Copy` は上限なしに読み続ける（`SafeReadFile` の 128 MB 上限はこの経路にない）。root で動く `record` も同様にハングし得る。影響は DoS のみで、通常は攻撃者自身の起動がハングするため Low。`risk/evaluator.go:600-626` と `security/indirect_execution.go:606-612` は正しい手順（`O_NONBLOCK` で open → `fstat` → 非通常ファイルを拒否 → `O_NONBLOCK` 解除）を実装しているが、`safefileio` にはない。
- **推奨対策**: `safeOpenFileInternal`/`openNoFollow` で `O_NONBLOCK` を付けて open し、`fstat` で `S_IFREG`（`O_DIRECTORY` 指定時はディレクトリ）以外をセンチネルエラーで拒否してから `O_NONBLOCK` を外す。これにより全呼び出し元が個別に意識せずとも安全になる。

#### SA-15: ディレクトリ所有者チェックが owner-write ビットに条件付けられている

- **該当箇所**: `internal/security/dir_permissions_unix.go:197-201`
- **内容**: `perm&0o200 != 0` の場合にのみ所有者（root または実 UID）を検査する。第三者の非 root ユーザが所有する `0555` のディレクトリは通過するが、所有者はいつでも `chmod u+w` してエントリを rename/差し替えできる。`docs/dev/security/README.md` の「Restricted owner-write」はまさにこの条件付き規則を記述しており設計どおりだが、「モードビットが所有者を拘束する」という前提は POSIX では成り立たない。`safefileio/safe_file.go:1148-1152` はこれを「多層防御であって保証ではない」と認めている。コマンドバイナリについては fd 束縛実行と再ハッシュで下流が閉じているため Low。
- **推奨対策**: 非 root・非実 UID の所有者に対する検査を無条件にし（モードビットは group/other の分岐のみに影響させる）、README の表を更新する。

#### SA-16: レコード読み取りポリシーが所有者を検査しない

- **該当箇所**: `internal/fileanalysis/file_analysis_store.go:110`（`SafeReadFile` で読む）、`internal/groupmembership/manager.go:333-380`（`CanCurrentUserSafelyReadFile`: world-writable 拒否、group-writable は permission-check UID がグループ非所属の場合のみ拒否、所有者チェックなし）
- **内容**: runner の permission-check UID は起動ユーザ（RealUIDOnly）なので、`-rw-rw---- root:staff` のレコードは起動ユーザが `staff` に属していれば受理される。レコードは `0o600`、ディレクトリは `0o700` で作成されるため運用ミスでしか到達しないが、ハッシュストアは信頼の根であり、ポリシーが「読んでよいか」用に設計されたもので「信頼してよいか」用ではない点を記す。
- **推奨対策**: `Store.Load` には一般の読み取りより厳しいポリシー（所有者は root またはハッシュディレクトリ所有者、group/other write なし）を与える。最低でも group-writable なレコードは所属に関係なく拒否する。

#### SA-17: 値全体の秘匿判定が一般語に過剰一致し、監査情報を消す

- **該当箇所**: `internal/redaction/sensitive_patterns.go:40-53`（`IsSensitiveValue` は `(?i)(password|token|secret|key|…|bearer|basic|authorization)` の部分一致）、`redactor.go:417, 892`
- **内容**: `command_path=/usr/bin/ssh-keygen`、`/usr/bin/apt-key`、`working_directory=/srv/keys`、`ls /etc/ssh` の stdout、`authorized_keys` に言及するエラー文字列などが `[REDACTED]` になる。`audit/logger.go:82-83` では「何が root で実行されたか」を記録するはずのレコードからコマンドパスが消える。機密性の問題ではないが、監査の証拠価値を下げ、運用者がプレースホルダを無視する習慣を作る。
- **推奨対策**: パス型属性に役割を宣言し（`errmsg.Path`/`identifier.Identifier` の仕組みは既にある）、値全体の置換には秘密らしいトークン（形式検出または key=value 一致）を要求する。

#### SA-18: 実行前エラーの stderr 出力が秘匿処理を経由しない

- **該当箇所**: `internal/logging/pre_execution_error.go:158-171`（`handleErrorCommon` が `record.errorMsg.String()` をそのまま stderr へ出力。構造化レコードのみ `RedactingHandler` を通る）。`RedactMessage`（`redaction/message.go:22`）は未使用。
- **内容**: 読者は起動ユーザであり設定は起動ユーザの UID で読まれるため影響は限定的だが、展開済みの値を引用する原因エラー（変数展開、環境検証）は平文で出力される。将来 root 専用の設定読み取りを導入すると実害になる。
- **推奨対策**: 出力前に `redactionConfig.RedactMessage(record.errorMsg)` を通す（bootstrap は設定を持っているのでレポート経路へ渡す）。

#### SA-19: Slack 一括出力フィールドのコードフェンスが脱出可能

- **該当箇所**: `internal/logging/slack_handler.go:814, 823, 1056, 1063`（stdout/stderr を `common.Interpolate` を通さず ```` ```\n%s\n``` ```` に埋め込む。`InterpolationRoleBulkOutput` はパススルー）
- **内容**: 子プロセスの出力に ```` ``` ```` が含まれるとフェンスが閉じ、以降が mrkdwn として解釈される。`<!channel>`、`<!here>`、偽リンク、偽の「✅ SUCCESS」行を、特権コマンドの stdout に影響できる者（例: ユーザ制御ファイルの `cat`）が管理チャンネルに流せる。
- **推奨対策**: 一括出力の `&`, `<`, `>` をエスケープし、```` ``` ```` を無害化する（ゼロ幅接合子の挿入や `'''` への置換）。

#### SA-20: Slack HTTP クライアントがリダイレクトを追従し、プロキシ/CA を起動ユーザの環境から取る

- **該当箇所**: `internal/logging/slack_handler.go:188-217`（`validateWebhookURL`。https 強制・完全一致ホスト検証は良好。userinfo は拒否されない）、`:229`（`&http.Client{Timeout: 5s}`）
- **内容**: 既定トランスポートは `HTTPS_PROXY`/`HTTP_PROXY` を、x509 は `SSL_CERT_FILE`/`SSL_CERT_DIR` を尊重し、307/308 は本文を再 POST して `Location` 先を再検証しない。環境は起動ユーザの制御下なので、Slack 通信の経路変更・MITM が可能。ただし SA-02 のとおり起動ユーザは URL 自体を選べるため、実質的な追加影響は小さい。
- **推奨対策**: `CheckRedirect` で `http.ErrUseLastResponse` を返す。`parsedURL.User != nil` を拒否する。`Transport{Proxy: nil}` と `TLSClientConfig{MinVersion: TLS1.2}` を検討する。

#### SA-21: 1 ログレコードのサイズに上限がない

- **該当箇所**: `internal/common/logschema.go:150-185`（`CommandResults.LogValue` が最大 `MaxLoggedCommands=100` 件の `Output`/`Stderr` 全文を埋め込む）、`common/output_size_limit_type.go:19`（キャプチャ上限 10 MiB）
- **内容**: グループサマリ 1 行が理論上約 1 GiB に達し得る。Slack 側は 1000/500 バイトで切り詰めるが、ファイルは切り詰めない。SA-02 を踏まえると大半は自業自得だが、運用者が `--log-dir` を `/var/log` に向けていれば設定作成者がディスクを埋められる。
- **推奨対策**: キャプチャ上限とは独立に、ログレコード内のコマンド出力を切り詰める（例: 64 KiB + `truncated` フラグ）。

#### SA-22: グループ一時ディレクトリの削除が起動ユーザ権限で行われる

- **該当箇所**: `internal/runner/base/executor/tempdir_manager.go:127`（`os.RemoveAll`、特権窓の外）、`group_executor.go:205-211`（警告のみ）
- **内容**: `run_as_user = "root"` のコマンドが一時ディレクトリに書いたファイルは root 所有となり、起動ユーザは削除できず `/tmp` に蓄積する。衛生/DoS の問題で特権への影響はない（`RemoveAll` は `unlinkat`/`O_NOFOLLOW` により symlink 安全）。
- **推奨対策**: SA-07 の対応（root 所有の一時ディレクトリを昇格窓で削除）に含める。

#### SA-23: `verifyDynLibDeps` が旧スキーマ/レコード不在で skip する

- **該当箇所**: `internal/verification/manager.go:666-676`（`ErrRecordNotFound` で nil）、`:689-696`（旧スキーマで WARN + nil）。`verifyShebangInterpreter`（`:933-937`）は同じ条件でエラーを返しており非一貫。
- **内容**: 現状は `VerifyGroupFiles`（`group_executor.go:463`）が先に走り、`store.Load` が `schema_version != 23` を拒否するため安全だが、将来 `VerifyCommandDependencies` を単独で呼ぶ呼び出し元は依存検証を黙って skip する。また `file_analysis_store.go:192-195` の `Update` は `RecordCorruptedError` を「新規」として扱い `--force` なしで上書きする。
- **推奨対策**: 旧スキーマでは skip ではなく失敗にし `verifyShebangInterpreter` と揃える。破損レコードの上書きには `--force` を要求する。

#### SA-24: 特権中のパス指定 `os.Chown`/`os.Chmod`

- **該当箇所**: `internal/runner/base/executor/executor.go:521-528, 592-606`
- **内容**: SA-06 の基底ディレクトリ問題を解消した後も、`os.Chown`/`os.Chmod` は symlink を辿る。開いた fd に対する `dst.Chmod()`/`Fchown` とステージングディレクトリ fd 相対の `openat`+`O_NOFOLLOW` を使えば、ディレクトリの場所に関係なくこのクラス全体が消える。

### 4.4 Info

#### SA-25: `risk_level` に配備レベルの上限がない

`internal/runner/base/runnertypes/spec.go:293-298`、`resource/normal_manager.go:116-127`。B が全コマンドに `risk_level = "high"` を書くことを阻むのは、設定ファイル（と include）を管理者がハッシュ記録しなければならないという点だけである。設計とは整合しているが、リスクゲートは「申告されたリスクの過小評価」を防ぐものであり「high と申告する作成者」を防ぐものではないことを明記すべきである。`[security] max_risk_level` のような配備側の上限があれば、設定が要求できる範囲を運用者が制限できる。

#### SA-26: native root（`sudo runner`）が非サポート宣言にもかかわらず受理される

`cmd/runner/main.go:83-90` のコメントは「sudo 起動はサポート対象の配備ではない」と述べるが、`privilege/unix.go:419-426` は `uid==euid==0` をサポート済みとして扱い、`RealUIDOnly` ポリシーは `SUDO_UID` を無視する。sudo 配下では「起動ユーザ」に関する全チェック（出力パスの書き込み権限、ディレクトリ監査）が root として評価され、出力ファイルは設定指定パスに root で作成され、`executeNormal` の子も root で動く。宣言どおり `Getuid()==0 && SUDO_UID` 設定時は拒否するか、少なくとも強く警告することを検討する。

#### SA-27: 解析キャッシュが safefileio を経由しない raw `os.*` I/O

`internal/dynamicanalysis/store.go:35,117,182-196`、`internal/libccache/cache.go:38,61,126-140`、`macho_cache.go:28,53`。ディレクトリ `0o755`・ファイル `0o644` で、`fileanalysis` が `0o700` で作るハッシュディレクトリ内に置かれる。読み書きとも `record`（root）のみで、`record` は world-writable なハッシュディレクトリを拒否するため脅威モデル内では悪用不能。ただし「本番 I/O はすべて safefileio を通す」という原則が成り立たない唯一の箇所であり、ハッシュディレクトリを README が推奨する `755` に緩めた場合に意味を持つ。`SafeReadFile`/`SafeWriteFileOverwrite` へ寄せ、`0o700`/`0o600` に揃えることを推奨する。

#### SA-28: テストスイートが root で実行されると権限期待テストが失敗する

本監査環境（uid 0）で `go test -tags test ./...` を実行したところ、`cmd/runner`, `internal/filevalidator`, `internal/libccache`, `internal/runner`, `internal/runner/base/executor`, `internal/runner/base/security`, `internal/runner/bootstrap` の 7 パッケージで 10 テストが失敗した。いずれも「書き込み不可ディレクトリでエラーになること」を期待するテストで、root では書き込みが成功するため失敗する。`internal/filevalidator/validator_error_test.go:144` では、`Verify` が nil を返すと `assert.ErrorAs` が型付き nil の `*os.PathError` を残し、`os.IsPermission(perr)` がそれを参照して panic する（`os.IsPermission(nil)` 自体は false を返す）。`TestDropStartupPrivileges_*` のように `Geteuid()==0` で `t.Skip` する流儀を他の権限期待テストにも揃えることを推奨する。コードの欠陥ではない。

#### SA-29: 利用者向け文書の記述がコードと一致しない

- [security-risk-assessment.ja.md](../../user/security-risk-assessment.ja.md):70「全権限操作の syslog 記録」、:293「ファイルログ、syslog、Slack 通知を一括でカバー」— 本番コードに syslog シンクは存在しない（SA-02）
- [config-inheritance-behavior.md](../architecture_design/config-inheritance-behavior.md) §3 と [05_group_level.md](../../user/toml_config/05_group_level.md) §5.3.4 — reject モードの効果範囲について矛盾（SA-05）
- `--log-dir` フラグの説明文「Overrides TOML/env if set」— TOML/環境変数からの供給経路はない（SA-02）
- [dry_run_json_schema.md](../../user/dry_run_json_schema.md):347-349 — `show_sensitive=false` でのマスクの約束が `parameters.environment` では守られない（SA-10）

#### SA-30: 緊急終了経路で euid 0 のままログ出力が行われ得る

`privilege/unix.go:382-401`（`emergencyShutdown` は復元失敗時、euid 0 のまま `slog` へ書く）。kill/ステージング後始末の窓は出力ポンプのゴルーチンが動作中に開く（`output_pump.go:100-106`）。書き込み先は既に開いている fd（起動ユーザ権限で開いたログファイル、キャプチャ一時ファイル）であり、root として新たにファイルを開くことはない。Slack ハンドラはキューに積むだけ。`security-architecture.md:1224-1225` で許容済みと記されており、対応不要。

### 4.5 バイナリ解析領域（`record` が root で攻撃者提供のバイナリを解析する面）

#### SA-31: 相対 `DT_RUNPATH` が record 時と runner 時で異なる CWD に対して解決される

- **重大度**: Medium　**攻撃者**: 解析対象バイナリの提供者　**確度**: 確認済み
- **該当箇所**: `internal/dynlib/elfdynlib/resolver.go:47-52`（`candidate := filepath.Join(expanded, soname)`。`$ORIGIN` 展開後のエントリが絶対パスかを確認しない）、`internal/dynlib/resolve.go:16-26`（`ResolveRealPath` は「相対候補を絶対に強制しない」と明記し、`os.Lstat(candidate)` は実行プロセスの CWD に対して解決される）、`internal/dynlib/elfdynlib/analyzer.go:150-154`（`DT_RPATH` は `ErrDTRPATHNotSupported` で拒否するが `DT_RUNPATH` は非空エントリをそのまま受理）、`internal/verification/manager.go:759-806`（`verifyDynLibDepsResolution` は runner プロセスの CWD で再解決する）
- **内容**: glibc の ld.so は secure-exec モードでない場合、相対 RUNPATH エントリ（`.` や `lib`）をプロセスの CWD 基準で探索する。record 時の管理者の CWD には通常ライブラリがないため ld.so.cache/既定ディレクトリへフォールスルーしてシステムライブラリが記録される。runner 時、コマンドの作業ディレクトリ（コマンド単位で設定可能で、攻撃者が書けることも多い）が ld.so の最初の探索先になるが、再解決チェックは別の CWD（runner 自身の CWD）で走るため記録と一致したままになる。記録されたシステムライブラリのハッシュ検証は通るが、実際にロードされるのは別のファイルである。`$LIB`/`$PLATFORM` トークンも `expandOrigin` が `$ORIGIN` しか置換しないため、`$LIB` で始まる*相対*パスとして同じ CWD 依存を持つ。
- **推奨対策**: `$ORIGIN` 展開後に絶対パスでない RUNPATH エントリ、または `$` トークンを含むエントリは `DT_RPATH` と同様に record 時に専用エラーで拒否する（「Reject, don't normalize」）。[elf-rpath-runpath-inheritance.md](../binary_analysis/elf-rpath-runpath-inheritance.md) に相対 RUNPATH と `$LIB`/`$PLATFORM` が非サポートであることを明記する。

#### SA-32: `debug/elf`/`debug/macho`/`debug/gosym` を root で `recover` なしに実行

- **重大度**: Low（管理ツールの可用性のみ。完全性への影響は見つからなかった）　**確度**: 確認済み
- **該当箇所**: `internal/security/elfanalyzer/standard_analyzer.go:150`、`internal/dynlib/elfdynlib/analyzer.go:300,335`、`internal/libccache/cache.go:73`（`elf.NewFile`/`DynamicSymbols`/`Section.Data`）、`internal/security/machoanalyzer/standard_analyzer.go`、`internal/dynlib/machodylib/analyzer.go`（`macho.NewFile`/`NewFatFile`）、`internal/security/elfanalyzer/pclntab_parser.go:52-57`、`internal/security/machoanalyzer/pclntab_macho.go:96-100`（`gosym.NewLineTable`/`NewTable`）。対象コードに `recover()` はない
- **内容**: 標準ライブラリの `debug/elf`/`debug/macho` は「敵対的入力に対して堅牢化する設計ではない」と明記され、Go のセキュリティポリシー外である。`debug/gosym` は内部で recover するが、`go12Funcs` の割り当ては与えられた `.gopclntab` サイズに比例する（16 バイト入力あたり約 100 バイト、6〜7 倍の増幅）。ファイル全体の上限は 1 GB（`standard_analyzer.go:21`）なので、細工ファイルは他の検査が拒否する前に数 GB のヒープを要求し得る。`record` の panic/OOM は root プロセスのクラッシュだが、ストアへの書き込みは解析後なのでレコードは書かれず、影響は記録作業の失敗にとどまる。
- **推奨対策**: 各パーサ入口を `defer recover → err` の小さな関数で包み、不正入力を `AnalysisError`（fail-closed、レコード拒否）に変換する。`.gopclntab`/`__gopclntab`/`.text` には 1 GB のファイル上限より厳しいセクション単位の予算を設ける。

#### SA-33: x86_64 の後方スキャンが syscall サイトごとに超線形コスト

- **重大度**: Low　**確度**: 確認済み
- **該当箇所**: `internal/security/elfanalyzer/syscall_analyzer.go` の `backwardScanX86WithRegCopy`（約 L760-820）→ `x86_copy_chain_unresolved` 時に `resolveX86CopyChainTailConsensus` がウィンドウを再デコードし、最大 128 の候補開始オフセット + 15 のプローブについて worklist データフロー（16 レジスタ × 最大 750 命令）を再実行する
- **内容**: `mov eax, <reg>; syscall` の組を大量に詰めたバイナリは全サイトで高コスト経路を踏む。影響は管理者の `record` 起動が遅延/ハングするのみで、分類は変わらない。
- **推奨対策**: コンセンサス/プローブ処理を受ける syscall サイト数（またはファイルあたりのデコード予算）に上限を設け、超過時は `AnalysisError` を返す。未解決の syscall 番号は既に高リスク扱いなので予算上限は fail-closed である。

#### SA-34: env 形式インタプリタ解決が `execvp` の EACCES スキップ挙動と異なる

- **重大度**: Low　**確度**: 蓋然性あり
- **該当箇所**: `internal/shebang/env_resolver.go:60-86`（`LookPathInEnv` は PATH エントリのうち*いずれかの*実行ビットを持つ最初のファイルを選ぶ）
- **内容**: coreutils `env` が使う `execvp` は `execve` が `EACCES` を返した候補をスキップして次の PATH ディレクトリへ進む。優先度の高い PATH ディレクトリに別ユーザのみ実行可能（例: 他人所有の 0700）な同名ファイルがあると、record/verify（root で動くため実行ビット検査は無意味）はそのファイルを選び、コマンドの uid で動く `env` はそれをスキップして次を実行する。文書化された環境隔離の下では到達しにくく、主に正確性の乖離である。
- **推奨対策**: 対象 uid が分かる場合はその uid での実行可能性を要求する（`faccessat(..., X_OK)` 相当）か、先行 PATH ディレクトリに同名の非実行候補がある曖昧な場合を拒否する。

#### SA-35: libc/libSystem キャッシュが自己認証されない

- **重大度**: Info　**確度**: 確認済み
- **該当箇所**: `internal/libccache/cache.go:17-19`（`cacheDirPerm 0o755`, `cacheFilePerm 0o644`）、`cmd/record/main.go:313`（キャッシュは `<hashDir>/libc_cache` 配下）、`cache.go:63-70`（ヒット条件は `SchemaVersion` 一致かつ `LibHash == libcHash`）
- **内容**: エンコード済み libc パスをキーとし libc の内容ハッシュで検証されるため、libc の更新でエントリは正しく無効化される。ハッシュディレクトリは `record` が `0o700` で作るため `0o755` のサブディレクトリにも非 root は到達できず、脅威モデルと整合する。ハッシュディレクトリを緩めた場合、改ざんされたキャッシュはラッパ→syscall 番号の対応を書き換えられ、MAC/署名を持たないため検出できない。現行の脅威モデルでは所見ではないが、`schema.go` にコメントとして残す価値がある（SA-27 と同根）。

#### SA-36: レコードの `Path` フィールドを再検証なしで open する

- **重大度**: Info　**確度**: 確認済み
- **該当箇所**: `internal/dynlib/elfdynlib/verifier.go:34-57`（レコードの `entry.Path` を直接 open。空パスは拒否するが絶対パスか、同じ SOName の再解決パスと一致するかは確認しない）
- **内容**: `manager.go:759-806` が記録集合と再解決集合をパスで比較するため、改ざんされたレコードは検証*失敗*しか引き起こせず、記録パス上のファイルも攻撃者が制御していなければバイパスにはならない。shebang チェーンの `Path` は verify 時に再導出されており良好。

**設計上の制約として文書化を推奨する事項**: `int 0x80`/`sysenter`（x86 デコーダは `SYSCALL` のみを syscall と見なし `INT` は制御フロー終端扱い）、`.text`/`__TEXT,__text` 以外の実行可能セクションや自己書き換え/パック済みコード（`mprotect(PROT_EXEC)` 検出で部分的に補完）、libc ラッパキャッシュの `MaxWrapperFunctionSize = 256` フィルタは、現在 `elfanalyzer/doc.go` の「Limitations」に記されていない。妥当なスコープ判断だが、レビュアが見落としと誤解しないよう列挙することを推奨する。

### 4.6 サプライチェーン・CI・運用領域

#### SA-37: Security Scan ジョブの gosec/SARIF ステップが v1 フラグでサイレントに失敗している

- **重大度**: Medium　**確度**: 確認済み（ローカルの golangci-lint v2 で同じフラグを実行し `unknown flag: --disable-all` を確認）
- **該当箇所**: `.github/workflows/ci.yml:76-96`, `:285`
- **内容**: `golangci-lint/v2@v2.11.4` をインストールした後に `--disable-all` と `--out-format sarif:…` という v1 のフラグで実行している。`|| true` と `continue-on-error: true` がエラーを飲み込み、`if: hashFiles('golangci-security.sarif') != ''` によりアップロードはサイレントにスキップされる。その後 `deployment-validation` ジョブはセキュリティレポートに "Security Scans: Gosec + Trivy completed" と書く。gosec SARIF が Code Scanning に届いたことは一度もなく、存在しないゲートがあると信じられている。緩和要因として、`gosec` は `.golangci.yml:19` でも有効で `lint` ジョブ経由で PR をブロックしている（本監査の `make lint` でも実行された）。失われているのは Code Scanning での可視性とレポートの真実性である。
- **推奨対策**: `golangci-lint run --default=none --enable=gosec --build-tags test --output.sarif.path=golangci-security.sarif --timeout=5m ./...` に修正し、`|| true` を外し、SARIF が無ければジョブを失敗させる。

#### SA-38: リリースバイナリが Makefile の本番ビルドと異なるフラグで作られ、チェックサム・来歴がない

- **重大度**: Medium　**確度**: 確認済み
- **該当箇所**: `.github/workflows/release.yml:33-53, 87-94`、`Makefile:111, 171-181`、`internal/cmdcommon/common.go:10`
- **内容**: `release.yml` は `CGO_ENABLED=0` で素の `go build -o dist/${binary}` を実行する。Makefile の本番ルールは `-ldflags "-s -w -X …DefaultHashDirectory=/usr/local/etc/go-safe-cmd-runner/hashes"` を使う。`common.go:10` のソース既定値が Makefile の値と一致するため挙動は偶然一致するが、リリース成果物は strip されず、ビルドパスを含み（`-trimpath` なし、`-buildvcs=auto`）、`scripts/additional-security-checks.py production-validation` を通らず（このゲートは `ci.yml` の `make build` 出力にしか走らない）、`SHA256SUMS`・`actions/attest-build-provenance`・署名もない。利用者はダウンロードした tarball がこのワークフローから来たことを検証できず、「本番検証」ゲートは誰も配布しないバイナリを検証している。将来 `BUILD_FLAGS` にセキュリティ用の `-X` やビルドタグが追加されてもリリースには伝播しない。両ビルドとも `-tags test` を渡していない点は良好で、`release` ジョブへの `contents: write` の限定も正しい。
- **推奨対策**: `release.yml` から `make build` を呼ぶ（または `BUILD_FLAGS` を共有変数に括り出す）。`-trimpath` を追加し、リリース出力に `production-validation` を実行し、`SHA256SUMS` を生成し、`release` ジョブのみに `id-token: write`/`attestations: write` を与えて `actions/attest-build-provenance` を追加する。

#### SA-39: 可変参照のサードパーティ action/ツールと広い `security-events: write`

- **重大度**: Low　**確度**: 確認済み
- **該当箇所**: `.github/workflows/ci.yml:99`（`aquasecurity/trivy-action@master`）、`:136, :183`（`go install mvdan.cc/gofumpt@latest`）、`:331`（`devcontainers/ci@v0.3`）、`:200`（`codecov/codecov-action@v5`）、`:7-10`（ワークフロー全体への `security-events: write`）。`actions/*` と `github/codeql-action/*` はメジャータグのみでピン
- **内容**: `trivy-action` の `master` が侵害されれば、`security-events: write` を持つジョブで任意コードが動く。`@latest` の gofumpt はビルドの再現性を損ね、サプライチェーン面を広げる。
- **推奨対策**: すべての `uses:` をフルコミット SHA でピンし（バージョンをコメントで併記。Dependabot/Renovate で更新）、`gofumpt@vX.Y.Z` にピンし、`security-events: write` を `security-scan` ジョブに下ろす。

#### SA-40: `release.yml` の `run:` に `github.ref_name` を直接展開

- **重大度**: Low　**確度**: パターンは確認済み。悪用はタグ push 権限者に限られる
- **該当箇所**: `.github/workflows/release.yml:47, 75, 83, 91-93`
- **内容**: タグ名はシェル解釈の前に文字列展開される。Git の refname は `$`, `(`, `)`, `;`, `|`, 引用符を含み得る（`v1$(id)` は有効なタグ）。ジョブは `contents: write` を持つ。`v*` タグを push できるユーザしかトリガできないため主に衛生の問題だが、スクリプトインジェクションの典型形である。
- **推奨対策**: `env: TAG: ${{ github.ref_name }}` 経由で渡し `"$TAG"` として参照する。

#### SA-41: `verify_links.go` の DNS リバインディング対策が名目的で、IP ブロックリストが不完全

- **重大度**: Low（完全一致ホスト allowlist により強く緩和。CI からは呼ばれていない）　**確度**: 確認済み
- **該当箇所**: `scripts/verification/verify_links.go:297-328, 347, 362-391`、`scripts/verification/run_all.sh:155-163`、`Makefile:786-790`
- **内容**: `DialContext` は `hostname:port` を受け取り、`isPrivateIP(host)` が独自に `net.LookupIP` した後、`net.Dialer.DialContext` が**再度**名前解決する典型的な TOCTOU で、リバインディングする resolver は 1 回目にパブリック、2 回目にプライベートを返せる。コメントの「接続時に IP を再検証」は実装と一致しない。`isPrivateIP` は `IsLoopback || IsPrivate || IsLinkLocalUnicast` のみで、`IsUnspecified`（`0.0.0.0`, `::`）、マルチキャスト、100.64.0.0/10、192.0.0.0/24、198.18.0.0/15 を含まない（IPv4-mapped IPv6 は Go の `net.IP` が `To4()` するため対応済み）。リダイレクトは allowlist に対して再検証されるが、allowlist ホストへの `https→http` ダウングレードは受理される。影響が低い理由: `isAllowedHost` は `u.Host`（ポート含む）が約 15 のリテラルホスト名と完全一致することを要求するため IP リテラルはいかなる符号化でも通らず、攻撃者はランナー上で `github.com` 等の DNS を制御する必要がある。`-external` は既定で無効で、**どのワークフローも `verify-docs`/`run_all.sh` を呼んでいない**（ローカル専用ツール）。文書のドリフト: `verify_links.go:13` と `run_all.sh:43` は `docs/security/SSRF-001-…` を参照する（実際は `docs/dev/security/`）。[SSRF-001-external-link-verification.md](SSRF-001-external-link-verification.md):8 は "Status: Identified (Awaiting Mitigation)" のままで、[SSRF-001-implementation-summary.md](SSRF-001-implementation-summary.md):4 の "Implemented and Tested" と矛盾する。
- **推奨対策**: 1 回だけ名前解決し、返された全 IP をフィルタし、選んだ IP に `ServerName` を設定してダイヤルする（または `net.Dialer.Control` で実際の sockaddr を検査する）。`IsUnspecified`/マルチキャスト/CGNAT を追加し、全ホップで `https` を要求し、文書のパスと状態を修正する。

#### SA-42: `.golangci-security.yml` が参照されない死に設定

- **重大度**: Low　**確度**: 確認済み
- **該当箇所**: `.golangci-security.yml:1,13`（`version: "2"` に v1 の `linters-settings:`/`issues:` キー）。`Makefile`・`.github/`・`.pre-commit-config.yaml` のいずれからも参照されない（`docs/tasks/0022_*` のみ）
- **内容**: 本番コードでの `NewManagerForTest`、`--hash-directory`、`flag.String.*hash.*directory` を禁じる `forbidigo` 規則が機械的に適用されていない。メインの `.golangci.yml` は `depguard` と `gosec` を適用している。また `.golangci.yml:92,110` は `go.mod` に存在しない `github.com/joho/godotenv` を allowlist している（陳腐化）。
- **推奨対策**: forbidigo 規則を `.golangci.yml`（v2 の `linters.settings.forbidigo`）へ移植するかファイルを削除する。陳腐化した `godotenv` エントリを削る。

#### SA-43: README/利用者文書のビルド・インストール手順が Makefile・コードと不一致

- **重大度**: Low　**確度**: 確認済み
- **該当箇所**: `README.md:557-559, 577, 589-596`、`Makefile:102-104, 159`、`internal/fileanalysis/file_analysis_store.go:17-20`、`docs/user/record_command.md:222, 557, 597`、`docs/user/README.md:304`、`cmd/record/main.go:374`、`internal/common/secure_path.go:13`
- **内容**: README は `make build/runner|record|verify` と `make coverage` を案内するが、いずれも存在しない（ターゲットは `build/prod/*`。`install` ターゲットはなく `setuid:` のみ）。インストール例は `build/runner` をインストールする（正しくは `build/prod/runner`）。README と `docs/user/README.md` はハッシュディレクトリに `chmod 755` を指示するが、`record` は `0o700` で作成し、`record_command.md:222` は `chmod 700`、同文書のトラブルシューティング（`:597`）は `chmod 755` と述べて相互に矛盾する（root 所有 755 自体は脆弱性ではない）。`record -hash-dir` のヘルプは "default: current working directory" と言うが既定は `cmdcommon.DefaultHashDirectory`。README の「ハードコードされた `/sbin:/usr/sbin:/bin:/usr/bin`」は `/usr/lib/cargo/bin/coreutils` を省いている（root 所有で無害だが不正確）。「本番ビルドではカスタムディレクトリなし」は `runner` には当てはまるが、`record`/`verify` は `-hash-dir` を受け付ける（設計どおりだが README の文は無条件）。
- **推奨対策**: パス/ターゲットを修正し、ハッシュディレクトリのモードを 1 つに決めて一貫して記述し（`record_command.md:557` の分離運用 0750 の注記を含む）、フラグのヘルプを修正し、「カスタムディレクトリなし」の主張を `runner` に限定する。

#### SA-44: `govulncheck` が CI にない。依存関係スキャンが非ブロッキング

- **重大度**: Low　**確度**: 確認済み
- **該当箇所**: `.github/workflows/ci.yml:98-111`（Trivy fs スキャン、`continue-on-error: true`、SARIF のみ）、`go.mod`、`go.sum`
- **内容**: `govulncheck` はどこにもない。直接依存は `go-toml/v2 v2.0.9`（2023-07）、`testify v1.8.4`（2023-05）、`oklog/ulid/v2 v2.1.1`、`x/sys v0.35.0`、`x/term v0.34.0`、`x/arch v0.24.0`、間接依存に `yaml.v3 v3.0.1`（CVE-2022-28948 修正後）。これらの正確なバージョンに影響する公表済みアドバイザリは確信をもって特定できなかったため個別には挙げないが、単純に古い。`go.sum` は完全（全モジュールに `h1:` と `/go.mod` 行）。ツールのピンは重要箇所では良好（`golangci-lint` は `go run …@v2.11.4` を Makefile・pre-commit・CI で共有）で、`gofumpt@latest` が例外。
- **推奨対策**: 到達可能な脆弱性で失敗する `govulncheck ./...` ステップを追加する。gofumpt をピンする。Trivy を HIGH/CRITICAL でブロッキングにすることを検討する。

#### SA-45: sudo 経由の `record` が起動ユーザの読み取り権限を確認しない

- **重大度**: Low　**確度**: 蓋然性あり（設計は文書化済み。オラクルの完全な連鎖は推測を含む）
- **該当箇所**: `internal/groupmembership/manager.go:331-360`、`internal/safefileio/safe_file.go:1210-1233`、`cmd/record/main.go:28-35, 108-165`、`internal/security/dir_permissions_unix.go:181-183`
- **内容**: `CanCurrentUserSafelyReadFile` は world-writable / group-writable 非所属のみを拒否し、所有者や読み取りビットは見ない（分離運用のための意図的な設計と `safe_file.go` のコメントが述べる）。したがって `sudo record /etc/shadow` は成功する。悪用を制限するもの: レコードは root 所有 `0700` ディレクトリ内の `0600` ファイル、`record` はダイジェストではなくハッシュ*ファイルパス*のみを表示、`verify` は OK/FAIL のみ、`record` 自身の対象ディレクトリ監査は祖先が root/実 UID 以外の所有で owner-write ビットがあれば fail-closed。推測オラクルには非既定ホスト（`fs.protected_hardlinks=0`）、`record` と `verify` 双方を許す sudoers 規則、監査を避けるため owner-write ビットを落とした対象ディレクトリが必要となる（§4.7 の「特権読み取りオラクル」の評価は、これらの条件が揃わない既定環境を前提としている）。
- **推奨対策**: `SudoUIDAware` の下では permission-check UID が対象に読み取り権限を持つことを追加で要求する（既存のグループ所属機構で owner/group/other の読み取りビットを判定）か、`record` は管理者にのみ委譲すべきと文書化する。

#### SA-46: サンプル設定が `/tmp` 直下の固定 `workdir` と `run_as_user = "root"` を併用

- **重大度**: Low　**攻撃者**: A　**確度**: 蓋然性あり
- **該当箇所**: `sample/comprehensive.toml:129, 261`、`sample/output_capture_advanced.toml:103`、`sample/command_template_example.toml:118`（`workdir = "/tmp/…"`）、`internal/runner/base/executor/executor.go:633-638`（`EffectiveWorkDir` の存在のみ確認）
- **内容**: 設定済み `workdir` について所有者や world-writable の監査は見つからなかった（`verify_files`/`commands`/ハッシュディレクトリには監査がある）。`/tmp` は sticky だが、任意のローカルユーザが runner より先に `/tmp/custom-workdir-test` のような*新しい*名前を作成でき、以後、相対パスを使うコマンドは攻撃者所有のディレクトリ内で動作する。サンプルは運用者がコピーする対象である（SA-07 と同じクラス）。
- **推奨対策**: サンプルでは自動一時 workdir または `/var/lib/…` 配下の root 所有パスを使い、コメントで注意する。設定済み workdir を既存の `security.AuditDirectoryPermissions` で監査することを検討する。

#### SA-47: 本番バイナリの「テスト成果物」検査が文字列 grep の弱いヒューリスティックにとどまる

- **重大度**: Info　**該当箇所**: `scripts/security_checker.py:103-146`
- **内容**: バイナリ文字列を `NewManagerForTest`、`testing.T`、`_test.go` で grep する。`-s -w` でほとんどのシンボル名は消えるため不在は何も証明せず、例外時は `True` を返す。実際の保証はビルドタグ設計（§5.3）から来ている。`go version -m`/ビルドタグ検査に置き換えるか、主張を取り下げることを検討する。

### 4.7 検討したが問題なしと判断した事項

以下は監査中に攻撃経路として検討し、コードを確認したうえで問題なしと判断した。再確認の手間を省くために記す。

- **特権読み取りオラクル**（`/etc/shadow` の内容を推測する）: `record`（root）は管理者が指定したファイルをハッシュし `0o600`/`0o700` のレコードに保存する。非特権ユーザはレコードを読めず、`verify`/`runner` は自分の euid でハッシュ計算するため root 専用ファイルは EACCES となり、保存済みハッシュと比較できるだけで推測値との比較はできない。オラクルは存在しない。
- **ハッシュストアのファイル名衝突**: 置換符号化 `{'/'→'~', '~'→'##', '#'→'#1'}` は prefix-free で単射。通常名は `~` で始まり、SHA-256 フォールバック名は base64url + `.json` で名前空間が重ならない。72 ビット切り詰めの衝突は 2^36 のランダム長パスでようやく起きる水準で、しかも `SaveRecord` と `verifyHash` が保存済み `file_path` と要求パスを比較して `ErrHashFilePathCollision` で失敗する。大文字小文字を区別しないファイルシステムでの衝突も同じ検査で防がれる。
- **`SUDO_UID`**: `SudoUIDAware` ポリシー（`record`/`verify` のみ宣言）かつ実 UID が 0 の場合に限り参照され、範囲検査・実在確認を行い、lookup 失敗で fail-closed。書き込みポリシーは常にカーネルの実 UID を使う。
- **group-writable の書き込みポリシー**: 列挙の完全性が明示されない限り拒否。完全性は `/etc/nsswitch.conf` から導出され、不明/読めない状態はすべて「不完全」→拒否。
- **ハードリンク**: `fs.protected_hardlinks` はカーネル既定で有効であり、移動経路は `/proc/self/fd` 経由で*検証済み inode* をリンクするため、差し替えられたソースは移動されない。
- **`--show-sensitive` による情報の特権昇格**: 設定と include は euid 降格後に読まれ、`internal/verification` に `WithPrivileges` の使用はないため、root 専用 TOML は単に読めない。`env_import` は起動ユーザ自身の環境を読む。`.env` ファイルローダは存在しない。
- **ハッシュ不一致エラー**: パスのみを含みハッシュ値は含まない。
- **ログファイル作成**: `SafeFileOpener` が `safefileio`（`O_NOFOLLOW`、Linux では openat2、`O_EXCL` プローブ）を使い、mode 0600、ディレクトリ 0750、euid == 実 UID の状態で開くため、攻撃者ディレクトリの symlink で root の書き込みを誘導できない。
- **ULID/run ID**: `ulid.Monotonic(crypto/rand.Reader, 0)`。`--run-id` は `[A-Za-z0-9_-]{1,64}` に検証され、拒否された値はエコーされない。
- **Slack webhook ホスト検証**: https 強制、正規化済みホストとの大文字小文字を無視した完全一致、サフィックス一致なし。
- **設定ロード**: メイン・テンプレートファイルとも `DisallowUnknownFields`。go-toml v2.0.9 は重複キー/テーブルを拒否する（実機確認）。`env_allowed = []` は非 nil の空スライスにデコードされ nil/空の意味が安定。include はハッシュ検証済み・単段・`version`+`command_templates` のみ。
- **変数展開の上限**: 深さ 100、循環検出、レベルごとの変数数/配列/サイズ上限、`__runner_`/`__` 予約プレフィックスの拒否、テンプレート定義はグローバル（大文字）変数のみ参照可、`${@param}` は `cmd`/`workdir` で禁止。
- **リスク評価の順序**: `${}` と `%{}` の展開後の `ExpandedCmd`/`ExpandedArgs` で評価されるため「echo → /bin/sh」型の展開トリックは効かない。symlink チェーンは厳密に解決され全名前が照合されるため `ln -s sudo foo` も Critical。

---

## 5. 良好な点（維持すべき防御）

監査を通じて確認した、特筆すべき防御実装を記す。これらは本レポートの所見を評価する際の文脈でもある。

### 5.1 特権管理

- 起動時の特権降格が `main()` の最初の文であり（`cmd/runner/main.go:161-169`）、`Setegid` → `Seteuid` の順で実 ID へ降格してから `flag.Parse`・設定読み込み・ログ初期化に進む。`init()` で I/O を行うものはない
- 昇格プリミティブは Go として正しい: `syscall.Seteuid` は `AllThreadsSyscall(SYS_SETRESUID, -1, euid, -1)` でプロセス全体に作用し、スレッド単位の raw syscall や `LockOSThread` への依存はない
- saved-set-uid は設計上 0 のまま保持され、**毎回の窓の後で不変であることを検証**し（`unix.go:280-292`）、EUID==UID / EGID==GID は 2 箇所で検査され、失敗は `emergencyShutdown` でプロセス終了
- `handleCleanup` が panic を回復 → 復元 → 検証 → 再 panic し、昇格は deferred な復元の直前の最後の失敗し得るステップに置かれている。再入は `ErrReentrantPrivilegeCall` で拒否
- 対象 ID の切替はカーネルアトミックで順序も正しい: `syscall.Credential{Uid,Gid,Groups}` → 子で `setgroups` → `setgid` → `setuid`。補助グループ列挙の失敗は fail-closed
- 特権窓は最小: `startPrepared`（fork/exec とステージングコピー）のみが昇格して動き、prepare/supervise は非特権。窓の中でログを出さないことは静的ガードテストで強制されている

### 5.2 ファイル I/O と整合性

- Linux 経路は単一の `openat2(RESOLVE_NO_SYMLINKS)` と `O_PATH|O_DIRECTORY` の dirfd で、以降の `openat`/`linkat`/`renameat`/`unlinkat`/`fstatat` はすべて fd アンカーかつ単一コンポーネント名を `validateOpenAtName` で検証。本番ビルドにはこれらの経路に差し替え可能な関数値がない（`overrides*.go` は `!test` タグ）
- アトミックなレコード書き込み: 宛先 dirfd 内に `O_EXCL` で一時ファイル → `fsync` → `fchmod` → `/proc/self/fd` 経由で検証済み inode をハードリンク → `renameat` → 事後の同一性確認 → ディレクトリ `fsync`。後戻りできない地点以後の失敗は `ErrDestinationCommitted` で区別し、後始末は inode を確認したエントリしか削除しない
- ハッシュは常に実際に消費したバイト列で計算される: `VerifyAndRead` は返すバッファをハッシュし、`SaveRecord` は shebang/ハッシュ/ELF/Mach-O 解析を 1 つの共有 fd から行い、実行時は `execve` する fd を `/proc/self/fd/N` 経由で再ハッシュしサイズ変化を拒否する。これにより SA-15 のようなディレクトリ監査の隙間は主防御ではなく多層防御の位置づけになる
- fail-closed 既定の徹底: ゼロ値の enum は拒否（`PolicyUnset`, `completenessUnstated`, `nssLineUnexamined`, `resolveMode`）、`canSafelyAccessFile` は未知の操作を拒否、`ErrHashFileNotFound`/スキーマ不一致/衝突はすべて検証を中断、本番 `Manager` は nil の `DirectoryValidator` で panic しハッシュディレクトリをコンパイル時既定に固定
- `record` はハッシュディレクトリに独自の厳しい規則（sticky な world-writable ディレクトリと symlink された作成位置を拒否）を加え、共有ディレクトリ検査の sticky 例外が残す `/tmp` 型の事前配置穴を閉じている

### 5.3 実行環境

- 子プロセスの環境は allowlist 済みシステム環境 + 設定変数のみから構築され、親環境は継承されない。プレフィックス一致の `LD_*`/`DYLD_*`/`BASH_FUNC_*` と、完全一致の `GLIBC_TUNABLES`/`GCONV_PATH`/`BASH_ENV`/`ENV`/`PYTHONPATH`/`PYTHONSTARTUP`/`PYTHONHOME`/`PERL5LIB`/`PERLLIB`/`PERL5OPT`/`PERL5DB`/`NODE_OPTIONS`/`NODE_PATH`/`RUBYOPT`/`RUBYLIB`/`GIT_SSH`/`GIT_SSH_COMMAND`/`LESSOPEN` 等からなる広い denylist が**全マージの後**に適用されるため、`vars`/`env_import`/テンプレート経由でバイパスできない（`PYTHONUSERBASE` は拒否されない。SA-13 参照）
- `__*` 名は予約され設定ロード時に拒否。stdin は `/dev/null`
- 検証→exec の TOCTOU は Linux で閉じている: パスを 1 回だけ解決し、識別を `O_RDONLY|O_CLOEXEC|O_NONBLOCK` で再 open、`fstat` で通常ファイル確認、`pread` で再ハッシュ、サイズ再確認、`/proc/self/fd/3` として exec。コマンドは固定 `SecurePathEnv` のみで解決し相対結果を拒否。特権コマンドは絶対パスの cmd と workdir を要求
- 出力キャプチャは非特権で安全に行われる: 宛先ディレクトリ内に `os.CreateTemp`（O_EXCL, 0600）→ `safefileio.AtomicMoveFile`（dirfd ベース、ユーザ所有 symlink を拒否）、最終 mode 0600、*実 UID* での書き込み権限検証、パストラバーサル/メタ文字拒否、既定 10 MB 上限、メモリ保持 64 KiB 上限
- 広範な fail-closed のラッパ解析（env -S, timeout/nice, chroot/unshare/nsenter, flock/watch, ip netns exec, find -exec, xargs, tar --to-command, rsync -e, ssh -o ProxyCommand）。未知オプションの arity は信頼できないとして拒否
- CLI/bootstrap: runner にはハッシュディレクトリ上書き・検証スキップ・「unsafe 許可」のフラグが存在せず、ハッシュディレクトリは `-ldflags -X` で焼き込まれ絶対パスを要求。テスト専用コードは `//go:build test` の背後。Go ランタイムにより setuid バイナリの閉じた fd 0-2 は `/dev/null` で再オープンされ、プロセスは non-dumpable

### 5.4 ログ・秘匿

- Slack を含む全 slog シンクが `RedactingHandler` の背後にあり、`LogValuer` の panic・深さ上限・不透明な kind で fail-closed、スライス/マップ/構造体/エラーへ再帰、失敗時ロガーは閉じた型集合で Slack 非含有が検証されている
- 秘匿 `Config` は `NewConfig` 経由の構築を強制（非公開フィールド + `validated` フラグ）、パターンは配線時に検証・コンパイル、PEM ブロック（未終端含む）・AWS/GitHub/Slack/JWT/URL 資格情報の形式を検出、webhook パスは許可ホスト確定後にマスク
- `errmsg` の役割と `identifier.Identifier` により、秘匿判定を内容検査ではなく宣言された型で行う（「Declare, don't infer」の実践）
- Slack: 完全一致ホスト allowlist、HTTPS のみ、有界な非同期配送（送信 40 秒/フラッシュ 15 秒、実行をブロックしない）、自由テキストと識別子は表示安全な補間契約（制御/書式文字、`<>&`、500 バイト上限）を通り、予約済みエンベロープフィールドで Scope/Hostname/Run ID の偽装を防ぐ
- 監査エントリは実 UID・euid・pid・終了コード・所要時間・操作ごとの特権窓・解決済みパス・内容ハッシュを記録する。失敗経路では子プロセスの起動後に限り記録され（起動前の失敗と `AuditLogger` が nil の場合は記録されない）、ファイルに残るかどうかは `--log-dir` とログレベルに依存する（SA-02）

### 5.5 バイナリ解析

- 解析前に通常ファイルとサイズを確認し、`ReadAt`/`SectionReader` で 1 つの fd を共有して読むため、ハッシュと解析の間の TOCTOU は閉じている（`AnalyzeFromReader`、`dynamicanalysis/store.go:63-85`）
- fail-closed 既定: 未解決の syscall 番号 → `-1` → `AnalysisError`。`DT_RPATH` は即時拒否。`ErrLibraryNotResolved` は記録を中断。`ErrSyscallHashMismatch` は「ネットワークなし」ではなく高リスク扱い
- 依存関係の BFS は明示的な深さ上限（`MaxRecursionDepth = 20`）と訪問済み集合を持ち、循環する `DT_NEEDED`/`LC_LOAD_DYLIB` でも終了する。vDSO 名は明示的な許可集合でスキップ
- `ld.so.cache` パーサはサイズ計算を `uint64` で行い `dataEnd` を境界検査し、文字列オフセットを `extractCString` 前にクランプする。Mach-O のロードコマンド解析も名前オフセットを `len(raw)` に対して検査し、`Iundefsym`/`Nundefsym` を uint32 ラップに対してガードする。arm64 スキャナは毎回の `Uint32` 前に `off+4 <= len(code)` を確認
- Fat Mach-O は全スライスを解析して最悪の結果を採用し、良性スライスで悪性スライスを隠すことを防ぐ
- shebang: 512 バイト読み取りで改行必須、CR 拒否、`-S`/フラグ/`VAR=` 拒否、`env` は 2 つの固定信頼パスのみ認識、スクリプトであるインタプリタは `ErrRecursiveShebang` で拒否（深さ 1）、env コマンドは verify 時に実行時 PATH で再解決

### 5.6 ビルド・CI・配布

- `pull_request_target` は使われず、ワークフロー既定の `permissions: contents: read`。`codeql.yml` と `release.yml` は昇格権限をジョブ単位に限定。CodeQL は Go・Python・GitHub Actions をカバー
- テスト用の差し替え口が出荷され得ない設計: 本番ファイル（`safefileio/overrides.go`, `overrides_linux.go`, `security/getwd.go`）は `//go:build !test` で**差し替え可能な関数値を持たない**。`testPermissiveMode` は `//go:build test` でのみ設定される非公開フィールド。Makefile の本番ルールも `release.yml` も `-tags test` を渡さない
- `record`/`verify` は fail-closed: 解決済みのハッシュディレクトリパスを検査にも使用にも用いる。sticky 付きでも world-writable なハッシュディレクトリを拒否。ハッシュディレクトリはサブストアがより広い権限で作る前に `0700` で作成。`verify` はディレクトリを作成せず、機械可読な `verify-error=` トークン付きの終了コードを返す
- `golangci-lint` は Makefile・pre-commit・CI で `go run …@v2.11.4` によりピン（PATH 上の版を無視）。CI は `go mod tidy` のドリフトで失敗。`CGO_ENABLED=0/1` の両マトリクスを実行し CGO 側は `-race` 付き。`go.sum` は完全
- 出力キャプチャファイルは README の主張どおり `0600`（`output/manager.go:189`）。固定 PATH は root 所有ディレクトリのコンパイル時定数
- `run_executor_setuid_integration.sh` は一時的に setuid するテストバイナリを慎重に扱う（0700 の私有ディレクトリ、`nosuid` マウント検査、root 起動の拒否、chmod 後に rm）
- `.env`/`.envrc` は git-crypt フィルタと git-ignore の対象。`verify_links` の完全一致ホスト allowlist・リダイレクト再検証・既定無効の外部チェックにより SSRF 面は小さい

---

## 6. 補足

### 6.1 govulncheck 未実施について

監査環境のネットワークポリシーにより `https://vuln.go.dev` へ到達できず、`govulncheck` は実行できなかった。直接依存は `github.com/pelletier/go-toml/v2 v2.0.9`、`github.com/stretchr/testify v1.8.4`、`github.com/oklog/ulid/v2 v2.1.1`、`golang.org/x/sys v0.35.0`、`golang.org/x/term v0.34.0`、`golang.org/x/arch v0.24.0` であり、ツールチェインは Go 1.26.2 である。CI で `govulncheck` を定期実行することを推奨する（SA-44 参照）。

### 6.2 テスト失敗の内訳（root 実行時）

| パッケージ | テスト |
|---|---|
| `cmd/runner` | `TestSetupLoggerWithConfig_IntegrationWithNewHandlers/invalid_log_directory`, `TestErrorHandling/nonexistent_log_directory` |
| `internal/filevalidator` | `TestErrorCases/permission_denied`, `TestFilesystemEdgeCases/unreadable_directory`（後者は型付き nil の `*os.PathError` に対する `os.IsPermission(perr)` で panic。SA-28 参照） |
| `internal/libccache` | `TestLibcCacheManager_WriteFailure` |
| `internal/runner` | `TestRunner_OutputCaptureErrorScenarios` |
| `internal/runner/base/executor` | `TestDefaultExecutor_ExecuteUserGroupPrivileges_AuditLogging` |
| `internal/runner/base/security` | `TestValidator_EvaluateOutputSecurityRisk` |
| `internal/runner/bootstrap` | `TestSetupLoggerWithConfig_InvalidLogDirectory`, `TestSetupLogging_InvalidConfig` |

残る 47 パッケージはすべて成功した。

---

## 7. 推奨する対応順序

1. **即時（次リリースまで）**
   - SA-03: openat2 プローブの修正（`$TMPDIR` 非依存、ENOSYS の区別、フォールバック時の WARN）。変更量が小さく効果が大きい
   - SA-04: 変数値の再展開停止
   - SA-05: `PATH` の固定と、グループ `env_allowed` の子プロセス環境への適用（または文書の是正）
   - SA-01: `sed`/`awk`/`busybox`/`vim`/`sg`/`git -c`/`scp -S` の分類追加
   - SA-31: 相対 `DT_RUNPATH` と `$LIB`/`$PLATFORM` の record 時拒否
   - SA-37: CI の gosec SARIF ステップのフラグ修正（1 行の修正で Code Scanning への可視性が戻る）
2. **短期**
   - SA-02: 起動ユーザが無効化できない監査シンクの導入。文書の syslog 記述の是正（SA-29）
   - SA-06/SA-07/SA-22/SA-24: 一時ディレクトリ基底の固定化と fd ベース操作への移行
   - SA-08/SA-09: プロセスグループ kill、umask/rlimit/fd のリセット
   - SA-10/SA-11/SA-12: dry-run `environment` のマスク、argv 要素の秘匿、端末出力のエスケープ
   - SA-38: リリースビルドの Makefile 本番ビルドへの統一、`SHA256SUMS` と build provenance の追加
   - SA-39/SA-40/SA-44: action の SHA ピン、`ref_name` の env 経由渡し、`govulncheck` の CI 追加
3. **中期（ハードニング）**
   - SA-13〜SA-21、SA-23、SA-32〜SA-34、SA-41〜SA-43、SA-45、SA-46
4. **設計判断として記録**
   - SA-25、SA-26、SA-27、SA-28、SA-30、SA-35、SA-36、SA-47

各所見は、[requirements_process.md](../developer_guide/requirements_process.md) に従い、所見 ID を起点に要件定義書を作成してタスク化することを推奨する。
