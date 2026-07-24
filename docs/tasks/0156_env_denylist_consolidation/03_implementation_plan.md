# 実装計画書: 環境変数 denylist の一元化と非対称・抜けの解消

## Document Status

| Item | Value |
|---|---|
| Status | `draft` |
| Created | 2026-07-24 |
| Review date | - |
| Reviewer | - |
| Comments | - |

## 関連文書

- 要件定義: [01_requirements.md](01_requirements.md)
- アーキテクチャ設計: [02_architecture.md](02_architecture.md)
- 要件プロセス: [requirements_process.md](../../dev/developer_guide/requirements_process.md)
- テスト構成ガイド: [test_organization.md](../../dev/developer_guide/test_organization.md)

---

## 1. 実装概要

### 1.1 目的

3箇所（実行層・config 層・security 層）に重複実装されている「禁止環境変数名の判定」を、`internal/runner/base/environment` パッケージの単一の公開関数 `IsForbiddenEnvVar` に集約する。あわせて denylist の対象範囲を拡張し（`DYLD_*`・`GLIBC_TUNABLES`・インタプリタ起動時コード注入変数）、config 層の `env_vars` KEY にも判定を適用する。設計の全体像・判定内訳・カバレッジ拡張の根拠は [02_architecture.md](02_architecture.md) を典拠とし、本書では作業手順と検証手段のみを記す。

### 1.2 実装原則

- **判定ロジックとリストのみ一元化**: 検知後の挙動（実行層は削除、config 層はロードエラー、security 層は Reject）は各層の既存責務として維持する（[02_architecture.md](02_architecture.md) §1.1）。
- **case-sensitive を採用**: 共有関数は変数名を正規化せず完全一致で照合する（[02_architecture.md](02_architecture.md) §6.2、AC-12）。
- **DRY / YAGNI**: 既存 `environment` パッケージに関数を追加するのみ。新規パッケージや設定駆動機構は設けない。
- **単一の典拠**: 対象変数の一覧は [01_requirements.md](01_requirements.md)「対象変数リスト（暫定）」を唯一の典拠とし、コードとテストがそれに追随する。
- **Go ソースは英語**: 追加・変更する Go のコメント・識別子・文字列リテラルはすべて英語で書く。

### 1.3 既存コード調査結果

実装前に対象箇所と全参照を調査した結果を示す。

**新設先パッケージ `internal/runner/base/environment`**

- 現状は `filter.go`（allowlist ベースの `Filter`）のみを持ち、非テストコードの import は `common` のみである。`runnertypes` はテスト `filter_test.go` からのみ参照する（[02_architecture.md](02_architecture.md) §2.1 は本番＋テストを合わせて `common`/`runnertypes` と記す）。`security`・`config`・`executor` を import しないため、これら3層のいずれから参照されても循環しない（config 層は既に `environment.NewFilter` を使用済み、[expansion.go:887](../../../internal/runner/config/expansion.go)）。
- denylist 定義（非公開リスト）と公開関数 `IsForbiddenEnvVar` を新規ファイルに追加する。網羅テストは非公開リストを直接 range するため、テストは white-box（`package environment`）とする。

**実行層 `internal/runner/base/executor/environment.go`**

- `BuildProcessEnvironment` の末尾（[environment.go:81-95](../../../internal/runner/base/executor/environment.go)）に inline スクラブがある。`strings.HasPrefix(key, "LD_")` と固定5個（`GCONV_PATH`, `LOCPATH`, `HOSTALIASES`, `NLSPATH`, `RES_OPTIONS`）を削除。
- 当パッケージは現状 `environment` を import していない → 新規 import が必要。
- コメント（[environment.go:85](../../../internal/runner/base/executor/environment.go)）に `docs/security/README.md` への参照があるが、当リポジトリに同ファイルは存在しない（`internal/dynlib` 内の他コメントにも同じ stale 参照あり。ただし本タスクのスコープは実行層のみとし、他ファイルの stale 参照修正は対象外）。本タスクで書き換えるこのコメントは、参照先を実在文書（`docs/dev/architecture_design/security-architecture.md`）へ差し替える。
- 影響を受ける既存テスト（[environment_test.go](../../../internal/runner/base/executor/environment_test.go)）: `TestBuildProcessEnvironment_DynamicLinkerVarsAlwaysRemoved`、`TestBuildProcessEnvironment_AllLDVarsRemoved`、`TestBuildProcessEnvironment_NonLDDangerousVarsRemoved`、`TestBuildProcessEnvironment_LegitimateVarsPreserved`。いずれもリファクタ後も pass する想定（削除挙動は不変）。

**config 層 `internal/runner/config/expansion.go`**

- 非公開の `forbiddenEnvVarPrefixes`（[expansion.go:282](../../../internal/runner/config/expansion.go)）、`forbiddenEnvVarExact`（:287）、`isForbiddenEnvVar`（:296）を定義。`isForbiddenEnvVar` の呼び出しは `ProcessEnvImport` 内の1箇所（:353）のみ。
- `ProcessEnv`（env_vars 処理、[expansion.go:806-852](../../../internal/runner/config/expansion.go)）は現状 `security.ValidateVariableName`（形式検査）のみで denylist 検査なし。ここに判定を1件追加する。
- `isForbiddenEnvVar` を直接参照するテスト（削除対象の private 関数）: [expansion_test.go](../../../internal/runner/config/expansion_test.go) の `TestIsForbiddenEnvVar_Prefix`（:261）・`TestIsForbiddenEnvVar_Exact`（:284）・`TestIsForbiddenEnvVar_SafeVarsAllowed`（:294）。これらが検証していたロジックは `environment` パッケージへ移るため、同パッケージの新規テストで置き換える（下記「削除テストの invariant 引き継ぎ」参照）。
- env_import の denylist 拒否テスト: [expansion_unit_test.go](../../../internal/runner/config/expansion_unit_test.go) の `TestProcessEnvImport_ForbiddenVariable`（:363）。env_vars 用の対になる拒否テストは未存在 → 新設が必要（AC-07）。

**security 層 `internal/runner/base/security/indirect_execution.go`**

- 非公開の `isLoaderControlVar`（[indirect_execution.go:1917-1920](../../../internal/runner/base/security/indirect_execution.go)）は `strings.ToUpper` で case-insensitive、`LD_`/`DYLD_` prefix のみを判定。呼び出しは `checkEnvAssignment`（:769）の1箇所のみ。
- 当パッケージは現状 `environment` を import していない → 新規 import が必要（循環なし、上記参照）。
- 影響を受ける既存テスト: [indirect_execution_test.go](../../../internal/runner/base/security/indirect_execution_test.go) の `TestIndirect_WrapperLoaderEnvRejected`（:349、`LD_PRELOAD`・`DYLD_*` を大文字綴りで検証）。end-to-end 回帰点は [evaluator_test.go](../../../internal/runner/base/risk/evaluator_test.go) の `TestEvaluateRisk_IndirectExecutionDeny`（:548、`env LD_PRELOAD` を Blocking として検証）。いずれも大文字綴りのみを検証しており、case-sensitive 化で失敗するアサーションは存在しない（[02_architecture.md](02_architecture.md) §6.2）。

**削除対象シンボルの全参照（`rg` 実測、`*.go`）**

- `forbiddenEnvVarPrefixes` / `forbiddenEnvVarExact` / `isForbiddenEnvVar`: `internal/runner/config/expansion.go` と `internal/runner/config/expansion_test.go` のみ。
- `isLoaderControlVar`: `internal/runner/base/security/indirect_execution.go` のみ（テスト参照なし）。

**ドキュメント（AC-11）**

- denylist に言及する既存文書: `docs/user/security-risk-assessment.md`/`.ja.md`（各 :123「LD_PRELOAD 等のライブラリ注入」）、`docs/dev/architecture_design/security-architecture.md`/`.ja.md`（:441「loader-control variables」を含む間接実行の記述、:1116「LD_PRELOAD 等」の脅威記述、および `.ja.md` の対応箇所）。

**テストヘルパー**

- 新規のクロスパッケージヘルパー・モックは不要。`environment` パッケージの網羅テストは非公開リストを range する white-box テストであり、通常の `_test.go`（`package environment`）で足りる。`testutil/` や `test_helpers.go` の追加は行わない。

---

## 2. 実装ステップ

作業はアーキテクチャ [02_architecture.md](02_architecture.md) §8 の優先順位に従い、フェーズ1（基盤）→2（実行層）→3（config 層）→4（security 層）→5（文書）→6（静的検証）の順に進める。フェーズ2〜4は互いに独立だが、いずれもフェーズ1の共有関数に依存するため、フェーズ1を先に完了させる。

### フェーズ1: `environment` パッケージへの判定関数新設（AC-01, AC-02, AC-06, AC-12, AC-13）

**対象ファイル**: `internal/runner/base/environment/denylist.go`（新規）、`internal/runner/base/environment/denylist_test.go`（新規）

- [ ] 対象変数リストの範囲を確定する。[01_requirements.md](01_requirements.md)「対象変数リスト（暫定）」の確定分（`LD_*`, `DYLD_*` prefix / 完全一致 `GCONV_PATH`, `LOCPATH`, `HOSTALIASES`, `NLSPATH`, `RES_OPTIONS`, `GLIBC_TUNABLES` / インタプリタ変数 `BASH_ENV`, `ENV`, `SHELLOPTS`, `PS4`, `PYTHONPATH`, `PYTHONSTARTUP`, `PERL5LIB`, `PERL5OPT`, `PERL5DB`, `NODE_OPTIONS`, `NODE_PATH`, `RUBYOPT`, `RUBYLIB`, `GIT_SSH`, `GIT_SSH_COMMAND`, `GIT_EXTERNAL_DIFF`）を採用する。
- [ ] 採否候補（`BASH_FUNC_*` prefix, `PYTHONHOME`, `LESSOPEN`, `LESSCLOSE`）および `ENV` の採否を判断し（[02_architecture.md](02_architecture.md) §3.1, §6.7）、確定結果を [01_requirements.md](01_requirements.md) の対象変数リストへ反映する。判断理由をコミットメッセージまたは PR 説明に記録する。判断時の追加考慮点:
  - `BASH_FUNC_*` を採用する場合、実際のエクスポート関数 KEY（`BASH_FUNC_x%%` 等）は `security.ValidateVariableName`（`[A-Za-z_][A-Za-z0-9_]*` のみ許可）を通らないため、config 層（env_import/env_vars）では denylist 検査に達する前に形式エラーになる。一方、実行層のスクラブ（生の `os.Environ` KEY を対象）と security 層の `checkEnvAssignment`（形式検査なし）では prefix 一致する。この層間の非対称を許容できるか判断材料に含める。
  - `ENV` を採用する場合、`env_vars` に `ENV=production` を書いた既存設定・サンプルがロードエラー化する（[02_architecture.md](02_architecture.md) §6.7）。フェーズ3のテストデータ監査タスクと整合させる。
- [ ] `denylist.go` に非公開の prefix 一致リスト（`[]string`、`LD_`, `DYLD_`, 採用する場合 `BASH_FUNC_`）と完全一致リスト（`map[string]struct{}`）を定義する。各エントリの由来（ローダ制御 / インタプリタ注入）を英語のインラインコメントで簡潔に付す。
- [ ] `denylist.go` に公開関数 `IsForbiddenEnvVar(name string) bool` を実装する。prefix 一致（case-sensitive な `strings.HasPrefix`）→完全一致（map 参照）の順で判定し、正規化（`ToUpper` 等）は行わない。doc コメントは英語で、case-sensitive である旨と典拠（[01_requirements.md](01_requirements.md)）への参照を含める。
- [ ] `denylist_test.go`（`package environment`）に単体テストを追加する:
  - [ ] `TestIsForbiddenEnvVar_Prefix`: 代表的な prefix 一致（`LD_PRELOAD`, `LD_LIBRARY_PATH`, `LD_AUDIT`, `DYLD_INSERT_LIBRARIES`, `DYLD_LIBRARY_PATH`）が該当することを検証（AC-02）。**prefix リストに `BASH_FUNC_` 等を追加採用した場合は、その prefix の代表的該当ケース（例 `BASH_FUNC_foo`）と near-miss 非該当ケース（例 `BASH_FUNCTION`）を本テストへ必ず追加する**（完全一致リストの range 網羅は prefix を対象にしないため、prefix は自動網羅されない）。
  - [ ] `TestIsForbiddenEnvVar_Exact`: 非公開の完全一致リストを直接 range し、各エントリが `IsForbiddenEnvVar` で該当することを検証（AC-06 の網羅検証。完全一致リストへの追加が自動でテスト対象になる。prefix リストには適用されない点に注意）。
  - [ ] `TestIsForbiddenEnvVar_NonMatch`: 非該当ケースが非該当であることを検証（AC-02）。正当変数（`PATH`, `HOME`, `USER`, `LANG`, `TZ`, `TERM`, `LANGUAGE`）に加え、prefix 誤判定の境界（`LDFLAGS`〈`LD_` に見えるが非該当〉、bare `LD`〈`HasPrefix(name,"LD")` 誤実装を検出〉、`DYLDFOO`/bare `DYLD`〈`DYLD_` の near-miss〉、空文字列）を含める。
  - [ ] `TestIsForbiddenEnvVar_CaseSensitive`: `ld_preload` が非該当・`LD_PRELOAD` が該当、`glibc_tunables` が非該当・`GLIBC_TUNABLES` が該当であることを検証（AC-12, AC-13）。
- [ ] `make fmt` → `make test` → `make lint` を実行し green を確認する。

### フェーズ2: 実行層のリファクタ（AC-04, AC-05, AC-06, AC-09）

**対象ファイル**: `internal/runner/base/executor/environment.go`、`internal/runner/base/executor/environment_test.go`

- [ ] `environment.go` に `internal/runner/base/environment` を import する。
- [ ] `BuildProcessEnvironment` 末尾の inline スクラブ（`LD_` prefix ループ + 固定5個の削除ループ、[environment.go:86-95](../../../internal/runner/base/executor/environment.go)）を、マージ結果の各 KEY に対する `if environment.IsForbiddenEnvVar(key) { delete(result, key) }` の単一ループに置換する。マージ後に一括削除する順序は維持する（AC-09）。
- [ ] 不要になった `strings` import が残る場合は削除する（`make lint` で検出）。
- [ ] コメントの stale 参照を修正する。`// See docs/security/README.md for the threat model.`（[environment.go:85](../../../internal/runner/base/executor/environment.go)）を `// See docs/dev/architecture_design/security-architecture.md for the threat model.` に変更する。
- [ ] `environment_test.go` の削除ケースを拡張する:
  - [ ] `TestBuildProcessEnvironment_NonLDDangerousVarsRemoved` の対象リストに `GLIBC_TUNABLES` を追加する（AC-05）。
  - [ ] `DYLD_*` prefix の削除を検証する専用ケース `TestBuildProcessEnvironment_DYLDVarsRemoved` を新設する（`DYLD_INSERT_LIBRARIES`, `DYLD_LIBRARY_PATH`）。§9 の AC-04 検証もこのテスト名を典拠とする（AC-04）。
  - [ ] インタプリタ起動時コード注入変数の削除を検証するケースを追加する（代表として `BASH_ENV`, `PYTHONPATH`, `NODE_OPTIONS`, `PERL5LIB`）（AC-06）。
  - [ ] case-sensitive 化を実行層でも固定する。小文字綴り `ld_preload` を vars 経由で注入した場合に `BuildProcessEnvironment` の結果に保持される（削除されない）ことを検証するケースを追加する（AC-13 の「executor の従来 case-sensitive 挙動が不変」を実測で担保）。
  - [ ] `TestBuildProcessEnvironment_LegitimateVarsPreserved` は変更不要だが、`ENV` を採用した場合に正当変数リストへ影響しないことを確認する（`ENV` は保持対象リストに含めない）。
- [ ] 既存の `TestBuildProcessEnvironment_DynamicLinkerVarsAlwaysRemoved`・`TestBuildProcessEnvironment_AllLDVarsRemoved` が引き続き pass することを確認する（AC-09 の回帰）。
- [ ] `make fmt` → `make test` → `make lint` を実行し green を確認する。

### フェーズ3: config 層のリファクタ（AC-04, AC-05, AC-06, AC-07, AC-08）

**対象ファイル**: `internal/runner/config/expansion.go`、`internal/runner/config/expansion_test.go`、`internal/runner/config/expansion_unit_test.go`

- [ ] `expansion.go` から `forbiddenEnvVarPrefixes`・`forbiddenEnvVarExact`・`isForbiddenEnvVar`（[:279-304](../../../internal/runner/config/expansion.go)）を削除する。
- [ ] `ProcessEnvImport` 内の呼び出し（[:353](../../../internal/runner/config/expansion.go)）を `environment.IsForbiddenEnvVar(systemVarName)` に置換する。エラー返却（`ErrForbiddenEnvVar`、メッセージ `%w: %s cannot be imported via env_import (level: %s)`）は変更しない。
- [ ] `ProcessEnv`（env_vars、[:806-852](../../../internal/runner/config/expansion.go)）に denylist 検査を追加する。`security.ValidateVariableName` による形式検査（:824-831）の直後、重複検査の前に、`if environment.IsForbiddenEnvVar(envVarName) { return nil, fmt.Errorf("%w: %s cannot be set via env_vars (level: %s)", ErrForbiddenEnvVar, envVarName, level) }` を挿入する（AC-07, AC-08）。既存の形式検査の順序・エラー型は変えない。
  - エラー生成に構造化 detail 型（`ErrInvalidEnvKeyDetail` 等）ではなく bare `fmt.Errorf` を用いるのは、既存の env_import 禁止変数エラー（`ProcessEnvImport` :354-355 も同じく bare `fmt.Errorf` で `ErrForbiddenEnvVar` をラップ）と様式を揃えるためであり、意図的である（[02_architecture.md](02_architecture.md) §4）。
- [ ] テストデータ・サンプル TOML の監査を行う。拡張後 denylist（特に `ENV` を採用する場合）に該当する KEY を `env_vars`/`env_import` に持つ既存 TOML を検出し、テストの green ゲートを壊さないか確認する。`rg -n "env_vars|env_import" --glob '*.toml'` の該当行を [01_requirements.md](01_requirements.md) 対象変数リストと突き合わせる。既知例: `sample/includes_example.toml` の `ENV=production`（`ENV` 採用時に該当）。テストで実ロードされる testdata が該当する場合は、当該テストデータを安全な変数名へ修正するか、`ENV` 採否判断（フェーズ1）へ差し戻す。
- [ ] `expansion_test.go` の private 関数直接参照テストを置き換える。ロジックの検証は `environment` パッケージへ移ったため、以下3件を削除する:
  - [ ] `TestIsForbiddenEnvVar_Prefix`（[:261](../../../internal/runner/config/expansion_test.go)）を削除。
  - [ ] `TestIsForbiddenEnvVar_Exact`（[:284](../../../internal/runner/config/expansion_test.go)）を削除。
  - [ ] `TestIsForbiddenEnvVar_SafeVarsAllowed`（[:294](../../../internal/runner/config/expansion_test.go)）を削除。
  - （これら3件の invariant は「§削除テストの invariant 引き継ぎ」の通りフェーズ1の環境パッケージテストが継承する。）
- [ ] `expansion_unit_test.go` の env_import 拒否テストを拡張する。`TestProcessEnvImport_ForbiddenVariable`（[:363](../../../internal/runner/config/expansion_unit_test.go)）に `DYLD_INSERT_LIBRARIES`・`GLIBC_TUNABLES`・`PYTHONPATH`（代表インタプリタ変数）のケースを追加し、いずれも `ErrForbiddenEnvVar` を返すことを `assert.ErrorIs` で検証する（AC-04, AC-05, AC-06）。
- [ ] `expansion_unit_test.go` に env_vars 拒否テスト `TestProcessEnv_ForbiddenVariable` を新設する。`ProcessEnv` に `LD_PRELOAD`・`PYTHONPATH`・`DYLD_LIBRARY_PATH`・`GLIBC_TUNABLES` を KEY とする env_vars を渡し、いずれも `ErrForbiddenEnvVar` を返すことを `assert.ErrorIs` で検証する（AC-07, AC-08）。
- [ ] `make fmt` → `make test` → `make lint` を実行し green を確認する。

### フェーズ4: security 層のリファクタ（AC-05, AC-06, AC-10, case 変更）

**対象ファイル**: `internal/runner/base/security/indirect_execution.go`、`internal/runner/base/security/indirect_execution_test.go`、`internal/runner/base/risk/evaluator_test.go`

- [ ] `indirect_execution.go` に `internal/runner/base/environment` を import する。
- [ ] `isLoaderControlVar`（[:1908-1920](../../../internal/runner/base/security/indirect_execution.go)）を削除する。
- [ ] `checkEnvAssignment` 内の呼び出し（[:769](../../../internal/runner/base/security/indirect_execution.go)）を `environment.IsForbiddenEnvVar(name)` に置換する。Reject/Blocking 分類（`rejectClass(risktypes.ReasonForbiddenEnvVar, "")`）は維持する（AC-10）。
- [ ] `checkEnvAssignment` の doc コメント（[:765-766](../../../internal/runner/base/security/indirect_execution.go)、"rejects loader-control assignments (LD_*/DYLD_*)"）を、拡張後の対象（loader-control と interpreter startup code-injection variables）と case-sensitive 化を反映した英語の文へ更新する。
- [ ] 不要になった `strings` import が残らないか `make lint` で確認する（`indirect_execution.go` は他所でも `strings` を使うため残存見込みだが、lint 結果で判断する）。
- [ ] `indirect_execution_test.go` の Reject テストを拡張する:
  - [ ] `TestIndirect_WrapperLoaderEnvRejected`（[:349](../../../internal/runner/base/security/indirect_execution_test.go)）に完全一致リスト（`GCONV_PATH`, `GLIBC_TUNABLES`）とインタプリタ変数（`BASH_ENV`, `PYTHONPATH`）の `env NAME=VALUE cmd` ケースを追加し、`ReasonForbiddenEnvVar` を伴う Reject を検証する（AC-05, AC-06, AC-10）。
  - [ ] case-sensitive 化を検証するケースを追加する。`env ld_preload=/tmp/evil.so ls`（小文字綴り）が Reject **されない**ことを明示的に検証する（[02_architecture.md](02_architecture.md) §6.2 の意図的挙動変更）。
- [ ] `evaluator_test.go` の `TestEvaluateRisk_IndirectExecutionDeny`（[:548](../../../internal/runner/base/risk/evaluator_test.go)）に拡張分の Blocking ケース（`env GLIBC_TUNABLES=... ls`, `env BASH_ENV=... ls`）を追加し、`EvaluateRisk` 経由の end-to-end で `ReasonForbiddenEnvVar` の Blocking になることを検証する（AC-05, AC-06, AC-10）。
- [ ] `make fmt` → `make test` → `make lint` を実行し green を確認する。

### フェーズ5: ドキュメント整合（AC-11）

**対象ファイル**: `docs/user/security-risk-assessment.md`/`.ja.md`、`docs/dev/architecture_design/security-architecture.md`/`.ja.md`

- [ ] 各文書内で denylist（危険環境変数）に言及する箇所を、拡張後の対象範囲（`LD_*`/`DYLD_*` prefix、完全一致リスト、インタプリタ起動時コード注入変数）に整合させる。少なくとも次を更新する:
  - [ ] `docs/user/security-risk-assessment.md` の危険環境変数検出の記述（:123 付近）。`LD_PRELOAD` 単独の例示を、対象カテゴリ（ローダ制御 + インタプリタ注入）を示す記述に拡張する。
  - [ ] `docs/dev/architecture_design/security-architecture.md` の間接実行 Reject の記述（:441 付近の "loader-control variables"）と脅威記述（:1116 付近）を拡張後の denylist に整合させる。
  - [ ] [02_architecture.md](02_architecture.md) §6.7 の破壊的変更（`env_vars`/`env_import` の一部設定が改修後ロードエラーになる点）と dry-run による事前検知手順を、利用者向け文書に移行ノートとして追記する。
- [ ] `.ja.md` を先に編集し、対応する英語版（`.md`）へ `/mktrans` で反映する（バイリンガル文書の編集順序）。対象4文書はいずれも `.ja.md` が日本語原本・`.md` が英訳である（各ファイル冒頭見出しで確認済み）。
- [ ] `docs/translation_glossary.md` に「denylist」等の新規用語が必要か確認し、必要なら追記する。
- [ ] 追記・変更した記述が拡張後の実装（対象変数リスト）と一致することを、[01_requirements.md](01_requirements.md) の対象変数リストと突き合わせて確認する。

### フェーズ6: 静的検証と全体の green 化（AC-03）

- [ ] 削除シンボルの残存参照がないことを確認する（下記「4. 横断検索チェックリスト」）。
- [ ] `make test && make lint` が green であることを確認する（green ゲート）。

---

## 3. 削除テストの invariant 引き継ぎ

フェーズ3で削除する config 層の3テストが検証していた invariant が、フェーズ1の `environment` パッケージテストで確実に継承されることを対応付ける。

| 削除テスト（config） | 検証していた invariant | 引き継ぎ先（environment） |
|---|---|---|
| `TestIsForbiddenEnvVar_Prefix` | `LD_*` prefix が該当 | `TestIsForbiddenEnvVar_Prefix`（`LD_*` に加え `DYLD_*` も検証） |
| `TestIsForbiddenEnvVar_Exact` | 固定5個が該当 | `TestIsForbiddenEnvVar_Exact`（完全一致リストを range し、5個 + `GLIBC_TUNABLES` + インタプリタ変数を網羅） |
| `TestIsForbiddenEnvVar_SafeVarsAllowed` | 正当変数が非該当 | `TestIsForbiddenEnvVar_NonMatch` |

---

## 4. 横断検索チェックリスト（`make lint`/`make test` で検出できない項目）

- [ ] 削除した private シンボルの残存参照がないこと（AC-03）:
  - `rg -n "isForbiddenEnvVar|forbiddenEnvVarPrefixes|forbiddenEnvVarExact" --glob '*.go'` → 期待: マッチなし。
  - `rg -n "isLoaderControlVar" --glob '*.go'` → 期待: マッチなし。
- [ ] 拡張後のドキュメントに旧来の狭い記述（`LD_PRELOAD` のみを唯一の危険変数とする表現）が残っていないこと（AC-11）:
  - `rg -n "LD_PRELOAD" docs/user/security-risk-assessment.md docs/user/security-risk-assessment.ja.md docs/dev/architecture_design/security-architecture.md docs/dev/architecture_design/security-architecture.ja.md` → 各マッチが拡張後の文脈（カテゴリの例示）になっていることを目視確認する。
  - 拡張カテゴリが実際に追記されたことを positive 検索で確認する: `rg -n "GLIBC_TUNABLES|DYLD_|BASH_ENV|PYTHONPATH" docs/user/security-risk-assessment.md docs/user/security-risk-assessment.ja.md docs/dev/architecture_design/security-architecture.md docs/dev/architecture_design/security-architecture.ja.md` → 各文書で1件以上マッチすること（旧来の `LD_PRELOAD` 生存確認だけに依存しない）。
- [ ] 追加した Go コード・テストに日本語のコメント・識別子・文字列リテラルが混入していないこと（目視 + `make lint`）。

---

## 5. 実装順序とマイルストーン

| マイルストーン | 成果物 | 完了条件 |
|---|---|---|
| M1: 基盤 | `environment.IsForbiddenEnvVar` + 単体テスト | フェーズ1完了、AC-01/02/06/12/13 の環境パッケージ分が green |
| M2: 3層委譲 | 実行層・config 層・security 層のリファクタ | フェーズ2〜4完了、AC-03/04/05/07/08/09/10 が green |
| M3: 文書・静的検証 | セキュリティ文書更新、重複ゼロ確認 | フェーズ5〜6完了、AC-11 と全体 green ゲート達成 |

---

## 6. テスト戦略

[02_architecture.md](02_architecture.md) §7 のテスト戦略に従う。要点:

- **単体テスト（environment）**: prefix / 完全一致（range 網羅）/ 非該当 / case-sensitive を検証（フェーズ1）。
- **各層の統合的挙動テスト**: 実行層は削除、config 層は env_import/env_vars 双方のロードエラー、security 層は Reject を、拡張分（`DYLD_*`, `GLIBC_TUNABLES`, インタプリタ変数）を含めて検証（フェーズ2〜4）。
- **回帰**: 既存の削除テスト・Reject テストがリファクタ後も pass すること（AC-09/AC-10 の回帰点）。
- **静的検証**: 重複定義ゼロの `rg` 確認と `make test && make lint`（フェーズ6）。

境界値・異常系として、`LD_` に見えて非該当の `LDFLAGS`、小文字綴り `ld_preload`（case-sensitive 化の境界）を明示的に含める。

---

## 7. リスク管理

| リスク | 影響 | 緩和策 |
|---|---|---|
| security 層の case-sensitive 化が既存挙動を意図せず後退させる | 小文字綴りの Reject 喪失 | [02_architecture.md](02_architecture.md) §6.2 の根拠（ローダ・インタプリタは正確なスペルのみ解釈）に基づき意図的変更と確認済み。共有関数・実行層・security 層それぞれに case-sensitive 検証テスト（小文字非該当）を新設して固定する。既存 executor/config テストは大文字綴りのみで case 境界を突かないため、不変の根拠には用いない |
| denylist 拡張が既存設定を破壊（`env_vars`/`env_import` のロードエラー化） | 一部設定がロード失敗 | 破壊的変更として文書化（[02_architecture.md](02_architecture.md) §6.7）。dry-run による事前検知手順を利用者文書へ記載（フェーズ5） |
| `ENV` / 採否候補の採用判断が要件と乖離 | 過剰・過少ブロック | フェーズ1で採否を確定し [01_requirements.md](01_requirements.md) へ反映、判断理由を記録 |
| security 層への import 追加が循環を生む | ビルド不能 | `environment` は `common` のみ依存で `security` を import しないため循環しないことを調査済み（§1.3） |

---

## 8. 実装チェックリスト（フェーズ別サマリ）

- [ ] フェーズ1: `environment` パッケージ新設 + 単体テスト green
- [ ] フェーズ2: 実行層委譲 + 削除テスト拡張 green
- [ ] フェーズ3: config 層委譲 + env_vars 検査追加 + 拒否テスト green
- [ ] フェーズ4: security 層委譲 + case-sensitive 化 + Reject テスト拡張 green
- [ ] フェーズ5: セキュリティ文書整合（ja→en 反映）
- [ ] フェーズ6: 重複ゼロの静的確認 + `make test && make lint` を green にする

---

## 9. Acceptance Criteria 検証

各 AC を検証手段（`test` / `static` / `manual`）とともに対応付ける。

| AC | 内容 | 種別 | 検証手段 |
|---|---|---|---|
| AC-01 | 公開判定関数が存在 | static | `rg -n "func IsForbiddenEnvVar\(name string\) bool" internal/runner/base/environment/denylist.go` → マッチ1件 |
| AC-01 | 同上（挙動） | test | `internal/runner/base/environment/denylist_test.go::TestIsForbiddenEnvVar_Prefix` |
| AC-02 | prefix/完全一致/非該当を単体検証 | test | `internal/runner/base/environment/denylist_test.go::TestIsForbiddenEnvVar_Prefix`, `::TestIsForbiddenEnvVar_Exact`, `::TestIsForbiddenEnvVar_NonMatch` |
| AC-03 | 3層の独自リスト削除・重複なし | static | `rg -n "isForbiddenEnvVar\|forbiddenEnvVarPrefixes\|forbiddenEnvVarExact\|isLoaderControlVar" --glob '*.go'` → マッチなし |
| AC-04 | `DYLD_*` を実行層・config 層で拒否 | test | `internal/runner/base/executor/environment_test.go::TestBuildProcessEnvironment_DYLDVarsRemoved`; `internal/runner/config/expansion_unit_test.go::TestProcessEnvImport_ForbiddenVariable`, `::TestProcessEnv_ForbiddenVariable` |
| AC-05 | `GLIBC_TUNABLES` を3層で拒否 | test | `environment_test.go::TestBuildProcessEnvironment_NonLDDangerousVarsRemoved`; `expansion_unit_test.go::TestProcessEnvImport_ForbiddenVariable`, `::TestProcessEnv_ForbiddenVariable`; `internal/runner/base/security/indirect_execution_test.go::TestIndirect_WrapperLoaderEnvRejected`; `internal/runner/base/risk/evaluator_test.go::TestEvaluateRisk_IndirectExecutionDeny` |
| AC-06 | インタプリタ変数を3層で拒否 + range 網羅 | test | `environment/denylist_test.go::TestIsForbiddenEnvVar_Exact`（range 網羅）; `executor/environment_test.go`（BASH_ENV/PYTHONPATH 等の削除ケース）; `expansion_unit_test.go::TestProcessEnv_ForbiddenVariable`; `security/indirect_execution_test.go::TestIndirect_WrapperLoaderEnvRejected` |
| AC-07 | `env_vars` KEY をロード時に拒否 | test | `internal/runner/config/expansion_unit_test.go::TestProcessEnv_ForbiddenVariable` |
| AC-08 | AC-07 が同一判定関数・同系エラー型 | test | `expansion_unit_test.go::TestProcessEnv_ForbiddenVariable`（`assert.ErrorIs(err, config.ErrForbiddenEnvVar)`） |
| AC-09 | 実行層 fail-silent スクラブ維持 | test | `executor/environment_test.go::TestBuildProcessEnvironment_DynamicLinkerVarsAlwaysRemoved`, `::TestBuildProcessEnvironment_AllLDVarsRemoved` |
| AC-10 | security 層 Reject/Blocking 維持 | test | `security/indirect_execution_test.go::TestIndirect_WrapperLoaderEnvRejected`; `risk/evaluator_test.go::TestEvaluateRisk_IndirectExecutionDeny` |
| AC-11 | セキュリティ文書の整合 | static | (1) `rg -n "LD_PRELOAD" docs/user/security-risk-assessment.md docs/dev/architecture_design/security-architecture.md` → マッチが拡張後の文脈であること。(2) positive: `rg -n "GLIBC_TUNABLES\|DYLD_\|BASH_ENV\|PYTHONPATH" docs/user/security-risk-assessment.md docs/user/security-risk-assessment.ja.md docs/dev/architecture_design/security-architecture.md docs/dev/architecture_design/security-architecture.ja.md` → 各文書で1件以上マッチ（拡張カテゴリが実際に追記されたこと） |
| AC-11 | 文書内容がリストと一致 | manual | 更新文書を [01_requirements.md](01_requirements.md) 対象変数リストと突き合わせて PR レビューで確認 |
| AC-12 | case セマンティクスを明示選択・根拠記載 | static | `rg -n "case-sensitive" internal/runner/base/environment/denylist.go`（doc コメント）; 根拠は [02_architecture.md](02_architecture.md) §6.2・[01_requirements.md](01_requirements.md) AC-12 に記載済み |
| AC-13 | case セマンティクスの単体テスト + executor/config 不変 | test | `environment/denylist_test.go::TestIsForbiddenEnvVar_CaseSensitive`（共有関数の case 挙動を固定）; `executor/environment_test.go` の `ld_preload` 保持ケース（実行層の case-sensitive 挙動を実測で固定）。config 層は同じ共有関数へ委譲するため、共有関数テストで担保する（既存 config テストは大文字綴りのみ検証で case 境界を突かないため、これを不変の根拠にはしない） |

---

## 10. Success Criteria

- AC-01〜AC-13 のすべてに §9 で `test` または `static` の検証手段が対応付いている。
- 既存の denylist 関連テスト（executor/config/security 各パッケージ）がリファクタ後も pass する。
- `make test && make lint` が green である。
- 追加・変更した Go ソースのコメント・識別子・文字列リテラルがすべて英語である。

---

## 11. 次のステップ

- 本実装計画書のレビューと承認（Status を `approved` へ）。
- 承認後、フェーズ1から実装に着手する。
