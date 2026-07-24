# 要件定義書: 環境変数 denylist の一元化と非対称・抜けの解消

## Document Status

| Item | Value |
|---|---|
| Status | `approved` |
| Created | 2026-07-23 |
| Review date | 2026-07-24 |
| Reviewer | isseis |
| Comments | - |

## 関連 Issue

- [#863 [Security][P4] 環境変数 denylist の非対称・抜けを解消](https://github.com/isseis/go-safe-cmd-runner/issues/863)
- 詳細所見:
  - [findings/A2_executor.md](../0149_security_code_smell_audit_fable/findings/A2_executor.md) M-1
  - [findings/A4_security.md](../0149_security_code_smell_audit_fable/findings/A4_security.md) M-1
  - [findings/B4_config.md](../0149_security_code_smell_audit_fable/findings/B4_config.md) L-1
  - 集約サマリ: [99_summary.md](../0149_security_code_smell_audit_fable/99_summary.md)（横断パターン P4）

## 背景

本コードベースには、子プロセスへ渡してはならない環境変数（動的ローダ制御変数・インタプリタ起動時コード注入変数）を拒否する処理が、独立に3箇所実装されている。

1. **実行層** `internal/runner/base/executor/environment.go:86-95`（`BuildProcessEnvironment`）: 最終的な子プロセス環境から、`LD_*` prefix と固定5個（`GCONV_PATH`, `LOCPATH`, `HOSTALIASES`, `NLSPATH`, `RES_OPTIONS`）を出所を問わず削除する（fail-silent スクラブ）。
2. **config 層** `internal/runner/config/expansion.go:279-304`（`isForbiddenEnvVar`）: `env_import` で取り込むシステム環境変数名を検証し、`LD_*` prefix と実行層と同じ固定5個を含む場合はロードエラーにする（fail-closed reject）。ただし `env_vars` に直書きされたKEYはこのチェックを経由しない。
3. **security 層** `internal/runner/base/security/indirect_execution.go:1908-1919`（`isLoaderControlVar`）: `env NAME=VALUE cmd ...` 形式の間接実行を静的リスク分類する際、`LD_*`/`DYLD_*` prefix の割り当てを Reject（Blocking）に倒す。

この3実装は次の点で非対称・不完全である。

- 実行層・config 層は `DYLD_*`（macOS dyld 挿入変数）と `GLIBC_TUNABLES`（CVE-2023-4911 "Looney Tunables"）を含んでいない。security 層のみ `DYLD_*` を持つが `GLIBC_TUNABLES` は無い。
- 3箇所とも、検証済みインタプリタへ同等のコード注入が可能な変数（`BASH_ENV`, `ENV`, `SHELLOPTS`, `PS4`, `PYTHONPATH`, `PYTHONSTARTUP`, `PERL5LIB`, `PERL5OPT`, `NODE_OPTIONS`, `RUBYOPT`, `GIT_SSH`, `GIT_EXTERNAL_DIFF` 等）を含んでいない。`LD_PRELOAD` は完全に拒否されるのに、同種のコード注入経路であるこれらは通過し得る。
- config 層は `env_import`（システム環境変数の取り込み）にはチェックがあるが、`env_vars`（ユーザーが TOML に直接書く `KEY=VALUE`）の KEY には同じチェックが適用されていない。結果として、実行層のスクラブで最終的に無害化されるものの、「設定した env が黙って剥がされる」という fail-silent な体験になっている。

リストの定義自体が3箇所に重複しており（DRY 違反）、一方を更新してももう一方に反映されない構造になっている。

## 目的

- 禁止環境変数名の判定（prefix + 完全一致リスト）を単一のパッケージに集約し、3つの呼び出し箇所がそれを参照するようにする。
- 集約後のリストに `DYLD_*` と `GLIBC_TUNABLES` を追加し、実行層・config 層のカバレッジを security 層と同水準にする。
- インタプリタ起動時コード注入変数（`BASH_ENV` 等）を新たに denylist へ追加し、`LD_*`/`DYLD_*` と同じ扱い（実行層は削除、config 層はロードエラー、security 層は Reject）にする。
- config 層で `env_vars` の KEY にも denylist チェックを適用し、`env_import` と同じ判定を通す。
- 各層の「検知後の挙動」（削除 / ロードエラー / リスク Reject）はそれぞれの既存設計を維持し、統一しない（DRY化するのは判定ロジックとリストのみ）。

## スコープ

### 対象（本タスクで対応する）

1. 禁止環境変数名の判定関数とリストを `internal/runner/base/environment` パッケージに一元化する。
2. リストに `DYLD_*`（prefix）、`GLIBC_TUNABLES`（完全一致）を追加する。
3. リストにインタプリタ起動時コード注入変数を追加する（下記「対象変数リスト」参照）。
4. `internal/runner/base/executor/environment.go` の `BuildProcessEnvironment` を一元化後の判定関数を使うようリファクタする。
5. `internal/runner/config/expansion.go` の `isForbiddenEnvVar` を一元化後の判定関数を使うようリファクタし、`env_vars` の KEY にも同じチェックを適用する（B4 L-1 の解消）。
6. `internal/runner/base/security/indirect_execution.go` の `isLoaderControlVar` を一元化後の判定関数を使うようリファクタする。
7. 上記変更に伴うテスト（3層それぞれの denylist 関連テスト、および一元化パッケージ自体の単体テスト）を追加・更新する。
8. 変更内容をセキュリティ関連ドキュメント（`docs/user/security-risk-assessment.md`/`.ja.md`, `docs/dev/architecture_design/security-architecture.md`/`.ja.md` 等、実際に denylist に言及する既存文書）に反映する。

### 対象外（別 Issue・別タスクとする、または本タスクでは対応しない）

- A4 M-2（`SanitizeEnvironmentVariables` が変数の「値」を検査せず「キー名」パターンのみで redaction する問題）: denylist（渡してはいけない変数名）とは異なる系統の所見（ログ／通知への値漏洩対策）であり、本 Issue の「該当箇所」にも含まれない。
- インタプリタ変数の denylist 化に対するエスケープハッチ（明示的な許可上書き機構）の実装: 現時点でそのような要求はなく、YAGNI に基づき見送る。将来的に正当な利用（例: ビルドツールが意図的に `PYTHONPATH` を必要とする）が問題になった場合は別タスクで検討する。
- `env` 経由の間接実行を許可リスト方式に切り替える設計変更（A4 M-1 の代替案として挙げられていたもの）: 本タスクは denylist 拡充で対応し、許可リスト方式への転換は行わない。
- 実行時（`os.Environ()` 変化等）の動的な denylist 更新機構: 対象変数リストは静的なコード内定義のままとする。TOML 等からの拡張はスコープ外。
- `internal/runner/base/environment` パッケージ内の既存 `Filter`（allowlist）の実装変更: 新しい denylist 機能は同パッケージに追加するのみで、既存 `Filter` の挙動・インターフェースは変更しない。

## 対象変数リスト（暫定）

集約後の denylist に含める変数を以下のカテゴリで整理する。実装時に脱漏がないか再確認すること。

**動的ローダ制御変数（prefix 一致）**
- `LD_*`（既存）
- `DYLD_*`（macOS dyld、security 層のみ既存 → 全層に拡大）

**動的ローダ制御変数（完全一致）**
- `GCONV_PATH`, `LOCPATH`, `HOSTALIASES`, `NLSPATH`, `RES_OPTIONS`（既存）
- `GLIBC_TUNABLES`（新規、CVE-2023-4911 対応）

**インタプリタ起動時コード注入変数（完全一致、新規）**
- シェル系: `BASH_ENV`, `ENV`, `SHELLOPTS`, `PS4`
- Python: `PYTHONPATH`, `PYTHONSTARTUP`
- Perl: `PERL5LIB`, `PERL5OPT`
- Node.js: `NODE_OPTIONS`
- Ruby: `RUBYOPT`
- Git（リモートヘルパー経由コード実行）: `GIT_SSH`, `GIT_SSH_COMMAND`, `GIT_EXTERNAL_DIFF`

> **実装時に採否を再検討する候補**（同種の既知ベクタ。本リストへの追加可否は実装時に判断する）:
> - `BASH_FUNC_*`（prefix、Shellshock 型のエクスポート関数注入 / CVE-2014-6271 系）
> - `RUBYLIB`（Ruby における `PYTHONPATH` 相当）
> - `PYTHONHOME`（Python 標準ライブラリ位置の乗っ取り）
> - `LESSOPEN` / `LESSCLOSE`（`less` の input preprocessor 経由のコード実行）

## Acceptance Criteria

#### F-001: 禁止環境変数名判定の一元化

`internal/runner/base/environment` パッケージに、禁止環境変数名を判定する公開関数（および prefix/完全一致リストの定義）を新設する。

- **AC-01**: `internal/runner/base/environment` パッケージに、変数名を受け取り denylist 該当有無を返す公開関数が存在する。
- **AC-02**: 該当関数の単体テストが、prefix 一致（`LD_*`, `DYLD_*`）・完全一致（`GLIBC_TUNABLES` 等）・非該当ケースをそれぞれ検証する。
- **AC-03**: `internal/runner/base/executor`, `internal/runner/config`, `internal/runner/base/security` の3パッケージが、それぞれ独自に定義していた禁止変数リスト（`forbiddenEnvVarPrefixes`/`forbiddenEnvVarExact`/`isLoaderControlVar` 等）を削除し、AC-01 の共通関数を呼び出すようリファクタされている（`grep` 等でリストの重複定義が残っていないことを確認できる）。

#### F-002: denylist 対象範囲の拡張

- **AC-04**: `DYLD_*` prefix が実行層（`BuildProcessEnvironment`）・config 層（`env_import`/`env_vars`）の両方で拒否される（既存は security 層のみ）。
- **AC-05**: `GLIBC_TUNABLES` が3層すべてで拒否される。
- **AC-06**: 「対象変数リスト（暫定）」節に列挙したインタプリタ起動時コード注入変数（同節を単一の典拠とする。ここでは再列挙しない）が3層すべてで拒否される。テストは同節のリストを直接 range して網羅検証し、リストへの追加が自動的にテスト対象へ反映される構成とする。

#### F-003: config 層における `env_vars` KEY チェックの追加

- **AC-07**: `env_vars` に denylist 該当の KEY（例: `LD_PRELOAD`, `PYTHONPATH`）を指定した設定は、config ロード時にエラーとなる（実行時の黙ったスクラブに委ねない）。
- **AC-08**: AC-07 のエラーは `env_import` の既存エラー（`isForbiddenEnvVar` 起因）と同一の判定関数・同系統のエラー型を用いる。

#### F-004: 各層の既存挙動（検知後の扱い）の維持

- **AC-09**: 実行層（`BuildProcessEnvironment`）は、denylist 該当変数を出所（system/vars/command）を問わず最終環境から削除する挙動を維持する（fail-silent スクラブは変更しない）。
- **AC-10**: security 層（`isLoaderControlVar` 相当）は、`env NAME=VALUE cmd ...` の間接実行解析において、denylist 該当の割り当てを Reject（Blocking）として分類する挙動を維持する。

#### F-005: ドキュメント整合

- **AC-11**: denylist に言及する既存セキュリティドキュメント（`docs/user/security-risk-assessment.md`/`.ja.md`、`docs/dev/architecture_design/security-architecture.md`/`.ja.md` 等、実装時に特定する）が、一元化後の対象変数リストと整合する内容に更新されている。

#### F-006: 変数名マッチングの正規化セマンティクスの明確化

一元化前の3実装は大文字小文字の扱いが非対称である（executor / config は case-sensitive、security 層は `strings.ToUpper` による case-insensitive）。一元化にあたり、共通判定関数の case セマンティクスを1つに定め、各層がそれに従うことを要件とする。

- **AC-12**: 共通判定関数の大文字小文字の扱い（case-sensitive か、正規化して case-insensitive か）が設計として明示的に選択され、その根拠（例: 環境変数名は Unix で case-sensitive でありローダは正確なスペルのみを解釈する／防御的に正規化する 等）が要件・設計文書に記載されている。
- **AC-13**: 選択したセマンティクスを検証する単体テストが存在する（例: case-sensitive を選んだ場合は `ld_preload` が非該当、case-insensitive を選んだ場合は該当、を明示的にテストする）。一元化により従来 case-sensitive だった executor / config 層の挙動が意図せず変化していないことを確認する。

## Success Criteria（要件レベル）

- AC-01〜AC-13 のすべてに対し、実装計画（`03_implementation_plan.md`）で具体的なテストまたは静的検証手段が対応付けられている。
- 既存の denylist 関連テスト（executor/config/security 各パッケージ）がリファクタ後も引き続き pass する。
- `make lint` / `make test` がグリーンである。
