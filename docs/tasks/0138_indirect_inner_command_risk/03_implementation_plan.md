# 間接実行インナーコマンドのリスク一律 High 化 — 実装計画書

## Document Status

| Item | Value |
|---|---|
| Status | `approved` |
| Created | 2026-06-17 |
| Review date | 2026-06-18 |
| Reviewer | isseis |
| Comments | - |

## 1. 実装概要

### 1.1 目的

ラッパー（`env`/`timeout`/`nice` 等）経由のインナーコマンド評価を、6 次元の細粒度算出からフラットな **High 下限** へ単純化する。設計の全体像・判定契約・残存リスクは [02_architecture.md](02_architecture.md) を参照し、本書では重複させない。

### 1.2 実装原則

- **変更は 1 関数に閉じる**: コード変更は `internal/runner/base/security/indirect_execution.go` の `evaluateInnerAs` の `RoleInner` 経路のみ（[02_architecture.md](02_architecture.md) §3.2/§3.3）。`RoleInterpreter`（shebang 直接スクリプト実行）経路の細粒度算出は不変に保つ。
- **後退させない**: Critical（特権昇格）・各種 Reject・無コマンド時の Medium 下限・インラインコード High・パッケージスクリプトランナー High・`service` High は保持（[02_architecture.md](02_architecture.md) §3.2 末尾）。
- **新規型・新規 reason code を導入しない**: ラッパーインナー Floor は既存の `risktypes.ReasonIndirectExecutionWrapper`（`"indirect_execution_wrapper"`）を再利用（[02_architecture.md](02_architecture.md) §3.1）。
- **Go ソースの英語規約**: 修正後のコメント・識別子・文字列リテラルは英語で記述する。
- **翻訳ワークフロー準拠**: ドキュメントは日本語版（`.ja.md`）を先に更新・コミットし、英語版（`.md`）は `/mktrans` で整合（[bilingual doc editing] 規約）。

### 1.3 既存コード調査結果

事前調査（symbol 確認・全インスタンス列挙・呼び出し影響トレース）の結果を以下に記録する。

#### 変更対象（コア）

- **`internal/runner/base/security/indirect_execution.go`**
  - `evaluateInner`（[indirect_execution.go:746](../../../internal/runner/base/security/indirect_execution.go#L746)）は `evaluateInnerAs(..., RoleInner)`（[indirect_execution.go:756](../../../internal/runner/base/security/indirect_execution.go#L756)）へ委譲。`analyzeShebang`（[indirect_execution.go:415](../../../internal/runner/base/security/indirect_execution.go#L415)）は `evaluateInnerAs(..., RoleInterpreter)` を呼ぶ。両経路が同一関数を共有するため、**`role` による分岐**が必須（§3.3）。
  - 撤廃対象（`RoleInner` のみ）: 再帰後の `level`/`codes`/`reasons` 算出ブロック（[indirect_execution.go:785-846](../../../internal/runner/base/security/indirect_execution.go#L785-L846)）— `IsDestructiveFileOperation` / `CoreutilsCommandRisk`（**coreutils stat 失敗時の fail-closed Reject 分岐 [indirect_execution.go:798-805](../../../internal/runner/base/security/indirect_execution.go#L798-L805) を含む**）/ `SystemModificationRisk` / `IsArbitraryCodeExecutionRunner` / `CheckDangerousArgPatterns` / `ResolveProfile`+`ProfileFactorRisk`。これらの関数はトップレベル評価器・`RoleInterpreter` 経路でも使われるため**デッドコードにはならない**（削除ではなく `RoleInner` 経路からの呼び出しのみ撤廃）。
  - 保持: 名前解決（`ResolveCommandNames`）→ symlink 失敗 Reject、特権判定（`isPrivilegeCommand`）→ Critical、再帰（`analyzeIndirect`）→ Critical/Reject 伝播、`artifact` の記録。

- **AC-08 対象コメント（[02_architecture.md](02_architecture.md) §3.4「AC-08 対象」の 5 箇所）**: いずれも現状コードに存在することを確認済み。
  - `wrapperSpec` 型コメント（[indirect_execution.go:66-69](../../../internal/runner/base/security/indirect_execution.go#L66-L69)）「The runner re-implements these wrappers (it execs the extracted inner command itself)…」
  - `wrapperSpecs` 変数コメント（[indirect_execution.go:79-85](../../../internal/runner/base/security/indirect_execution.go#L79-L85)）「the curated set of wrappers whose inner command the runner can extract and exec directly」
  - `IndirectExecutionResult` 型コメント（[indirect_execution.go:44-51](../../../internal/runner/base/security/indirect_execution.go#L44-L51)）「The actual fd binding and hash gating … is wired in the execution layer」
  - `analyzeIndirect` 内ラッパー分岐コメント（[indirect_execution.go:380-383](../../../internal/runner/base/security/indirect_execution.go#L380-L383)）「Other wrappers the runner re-implements…」
  - `IndirectFloor` 定数コメント（[indirect_execution.go:38-42](../../../internal/runner/base/security/indirect_execution.go#L38-L42)）「a wrapped dangerous inner command -> their level」
  - `analyzeService` 内コメント（[indirect_execution.go:920-921](../../../internal/runner/base/security/indirect_execution.go#L920-L921)）「identity binding / disposition is populated when artifact gating is wired in the execution layer」。これは [02_architecture.md](02_architecture.md) §3.4 の 5 箇所には未列挙だが、**同じ「実行層で fd 束縛・ゲートを配線する」前提**を持ち（[02_architecture.md](02_architecture.md) §3.1「成果物に identity / disposition を付与する後続ステップは存在しない」と矛盾）、AC-08 の検証 `rg`（`"wired in the execution layer"` を含む）にも合致するため、本タスクで併せて修正する。

#### 重要: `role` を再帰へ伝播させる必要がある（env ベース shebang の取り扱い）

[02_architecture.md](02_architecture.md) §3.3 が示す「`evaluateInnerAs` 内で `role` により分岐」という素朴な実装だけでは §3.3 の保証（`RoleInterpreter` 経路＝直接スクリプト実行の shebang インタプリタ評価は不変）を**満たせない**ことを、`analyzeShebang` の再帰経路を追って確認した。

- `#!/usr/bin/env <interp>`（最も一般的な shebang の慣用形）の直接スクリプト実行では、`analyzeShebang` が `evaluateInnerAs("/usr/bin/env", ["<interp>", script], RoleInterpreter)` を呼ぶが、その内部の再帰 `analyzeIndirect` は `env` ラッパー経路（`analyzeEnv` → `resolveInner` → `evaluateInner`）を辿り、**実インタプリタ `<interp>` を `RoleInner` で評価する**（[indirect_execution.go:649-654](../../../internal/runner/base/security/indirect_execution.go#L649-L654)・[:746-748](../../../internal/runner/base/security/indirect_execution.go#L746-L748)）。
- 素朴な分岐では `<interp>` が一律 High に潰れるため、`<interp>` が arbitrary-code ランナーでない無害インタプリタ（例 `#!/usr/bin/env cat`。`cat` は [arbitrary_code.go:17-36](../../../internal/runner/base/security/arbitrary_code.go#L17-L36) の集合に無い）の場合、**現状の非昇格（Low/Unknown）から High へ変化**してしまい、スコープ外（shebang 直接スクリプト実行）の挙動を変えてしまう。`#!/usr/bin/env python`／`bash` 等は `IsArbitraryCodeExecutionRunner` で既に High のため変化しないが、無害インタプリタの env-shebang のみが退行する。
- **対策**: `role` を `analyzeIndirect` とその下位ラッパーヘルパ（`analyzeEnv`/`analyzeWrapper`/`analyzeTaskset`/`analyzeEnvSplitString`/`resolveInner`/`evaluateInner`）へ引数として伝播させ、`evaluateInnerAs` の再帰 `analyzeIndirect` 呼び出しが**現在の `role` をそのまま渡す**ようにする。これにより shebang インタプリタ連鎖（`env` を経由しても）は終始 `RoleInterpreter`（細粒度算出）で評価され、ラッパー経由インナーのみが `RoleInner`（一律 High）になる。トップレベル公開 API `AnalyzeIndirectExecution` は既定で `RoleInner` を渡す（ラッパーインナー文脈）。

#### 影響を受ける既存テスト（`internal/runner/base/security/indirect_execution_test.go`）

[02_architecture.md](02_architecture.md) §3.4 の更新方針表に加え、実コードを確認して**フラット High 化で実際に挙動が変わる（= 現状アサーションが失敗する）テスト**を以下に確定した。

| テスト関数 | 現状アサーション | フラット High 化後 | 対応 |
|---|---|---|---|
| `TestIndirect_WrapperProfileFactors`（[:141](../../../internal/runner/base/security/indirect_execution_test.go#L141)） | `env curl`=Medium / `timeout wget`=Medium / `env claude`=High | すべて High | `want` を High に更新し、コメントを「プロファイル要因の合流は行わず一律 High」へ修正 |
| `TestIndirect_WrappedProfileReasonsCarried`（[:347](../../../internal/runner/base/security/indirect_execution_test.go#L347)） | `env curl` の `Reasons` に "Always performs network operations" を含む | `RoleInner` は profile reason を収集しない → `Reasons` 空 | **削除**（撤廃された `RoleInner` 挙動の検証のため）。ただし削除すると `.Reasons` を検証するテストが一つも残らず、§3.3 で**維持する** `RoleInterpreter` の Reasons 収集が無防備になるため、後述の新規テスト `TestIndirect_InterpreterReasonsCollected` で当該経路の Reasons 収集を locking する |
| `TestIndirect_CoreutilsInnerFolded`（[:321](../../../internal/runner/base/security/indirect_execution_test.go#L321)） | `env mysteryutil`=High かつ reason=`ReasonCoreutilsClassification`／ghost=Reject（coreutils fail-closed） | `env mysteryutil`=High だが reason=`ReasonIndirectExecutionWrapper`／ghost=Floor High（fail-closed 分岐が吸収） | 一律 High・reason=`indirect_execution_wrapper`・ghost も Floor High を期待する形へ更新（§3.2 の fail-closed 吸収） |
| `TestIndirect_WrappedRunnerReasonCodesDeduped`（[:195](../../../internal/runner/base/security/indirect_execution_test.go#L195)） | `env bash -c` の `ReasonArbitraryCodeExecution` が 1 個 | `RoleInner` は単一 `indirect_execution_wrapper` を返す | `ReasonCodes == [indirect_execution_wrapper]` を期待する形へ更新 |
| `TestIndirect_InnerCommandGated`（[:209](../../../internal/runner/base/security/indirect_execution_test.go#L209)） | `env rm`=Floor High+artifact／`env find -exec`=Reject／`env sudo`=Critical+artifact | いずれも不変（**現状のまま緑**） | コメントのみ「allowlist/ハッシュゲートではなく一律 High。Reject/Critical の伝播と artifact 記録は維持」へ更新（0136 AC-77 の再定義を反映） |

> **緑のまま不変だが注意が必要なテスト**:
> - `TestIndirect_WrapperDestructive`（[:116](../../../internal/runner/base/security/indirect_execution_test.go#L116)）: `Kind==Floor && Level==High` のみをアサートし reason code を見ないため**そのまま緑**。AC-01 の「インナー内容によらず High／reason=`indirect_execution_wrapper`」を確実に検証するため、reason code 検証は新規テスト（後述 `TestIndirect_WrapperInnerFlatHigh`）で担保する。
> - `TestIndirect_Taskset`（[:78](../../../internal/runner/base/security/indirect_execution_test.go#L78)）: `taskset 0x3 ls` 等の benign ケースは `Kind` のみ検証のため緑だが、**[:99](../../../internal/runner/base/security/indirect_execution_test.go#L99) のコメント「A benign inner command stays low」が陳腐化**する（フラット High 化後は High）。当該コメントを「benign でも一律 High。`Kind` のみ確認し level は本ケースでは検証しない」旨へ修正する。

#### 影響を受ける既存テスト（`internal/runner/base/risk/evaluator_test.go`）— アーキテクチャ §3.4 表に未記載

呼び出し影響トレースの結果、`risk` パッケージ側にもフラット High 化で失敗するテストが 1 件あることを確認した（[02_architecture.md](02_architecture.md) §3.4 の表は `indirect_execution_test.go` のみを列挙しており、本テストは未記載）。

| テスト関数 | 現状アサーション | フラット High 化後 | 対応 |
|---|---|---|---|
| `TestEvaluateRisk_WrappedProfileReasonsFolded`（[evaluator_test.go:249](../../../internal/runner/base/risk/evaluator_test.go#L249)） | `env curl` の `plan.Assessment.Reasons` に "Always performs network operations" を含む | `RoleInner` が profile reason を折り込まない → 含まれない | **削除**（撤廃された挙動の検証。level アサーションは無いため他の回帰なし） |

> 影響なしを確認したテスト: `TestEvaluateRisk_FloorReasonCodesDeduped`（[evaluator_test.go:233](../../../internal/runner/base/risk/evaluator_test.go#L233)）は `bash -c`（トップレベルのインラインコード検出、`RoleInner` 経由ではない）のため不変。`TestEvaluateRisk_IndirectExecutionDeny`（[evaluator_test.go:494](../../../internal/runner/base/risk/evaluator_test.go#L494)）の env sudo=Critical／env LD_PRELOAD=Blocking／env rm -rf=High も不変。

#### 影響を受けるドキュメント

- **ユーザー向け（AC-09）**: `docs/user/risk_assessment{.ja,}.md`（§8 移行ノート [risk_assessment.ja.md:253](../../../docs/user/risk_assessment.ja.md#L253) の「ラップされた内部コマンドが評価・ゲートされます」記述／§3 間接実行）、`docs/user/toml_config/04_global_level{.ja,}.md`（§4.6 [04_global_level.ja.md:950](../../../docs/user/toml_config/04_global_level.ja.md#L950) の「コマンドは自動的にハッシュ検証の対象となります」記述）、`docs/user/toml_config/05_group_level{.ja,}.md`・`docs/user/toml_config/06_command_level{.ja,}.md`・`README{.ja,}.md`（同趣旨記述があれば整合）。
- **開発者向け（AC-10）**: `docs/dev/architecture_design/security-architecture.md`（間接実行リゾルバ記述、[security-architecture.md:433](../../../docs/dev/architecture_design/security-architecture.md#L433) 周辺）。
  - **AC-10 の文言には未記載だが、本変更で陳腐化する開発者ドキュメント**: `docs/dev/architecture_design/command-risk-evaluation{.ja,}.md` は「内側コマンドの評価（`evaluateInnerAs`）では、特権・破壊操作・coreutils・システム変更・任意コード実行ランナー・危険引数パターン・リスクプロファイル要因をすべて折り込み…」と**撤廃される RoleInner 挙動を明記**している。AC-10 の趣旨（開発者ドキュメントを本方式へ整合）に従い Phase 3 で併せて更新する（§3.3 区別を反映し「`RoleInner` は一律 High、`RoleInterpreter`（shebang）のみ細粒度算出を維持」とする）。これは AC-10 の文言を超える追加更新である旨を本計画に明記する。
- **0136 注記（AC-11）**: `docs/tasks/0136_runtime_risk_evaluation_enforcement/02_architecture.md`（§3.3 ラッパー行 [02_architecture.md:475](../../../docs/tasks/0136_runtime_risk_evaluation_enforcement/02_architecture.md#L475)／§5.2 残存制約）、`docs/tasks/0136_runtime_risk_evaluation_enforcement/03_implementation_plan.md`（Step 2-2 の `[-]` 項目 [03_implementation_plan.md:288](../../../docs/tasks/0136_runtime_risk_evaluation_enforcement/03_implementation_plan.md#L288)／AC-60 行 [:574](../../../docs/tasks/0136_runtime_risk_evaluation_enforcement/03_implementation_plan.md#L574)／AC-77 行 [:591](../../../docs/tasks/0136_runtime_risk_evaluation_enforcement/03_implementation_plan.md#L591)）。

#### テストヘルパー

新規テストヘルパー・モックは不要。既存ヘルパー（`analyzeIndirectCmd`/`hasReason`/`hasArtifactPath`、`evalLevel`/`verifiedCmd`/`newVerifiedEvaluator`）を再利用する。

## 2. 実装ステップ

### Phase 1: コア実装・コメント修正・単体テスト更新（F-001/F-003/AC-01〜08）

**対象ファイル**: `internal/runner/base/security/indirect_execution.go`, `internal/runner/base/security/indirect_execution_test.go`, `internal/runner/base/risk/evaluator_test.go`

#### コア実装

- [x] `role` を再帰経路へ伝播させる（§1.3「重要: `role` を再帰へ伝播させる必要がある」）。`analyzeIndirect`・`analyzeEnv`・`analyzeWrapper`・`analyzeTaskset`・`analyzeEnvSplitString`・`resolveInner`・`evaluateInnerAs` に `role risktypes.ArtifactRole` 引数を追加し、各ラッパーヘルパは受け取った `role` で `evaluateInnerAs` を呼ぶ。`evaluateInnerAs` の再帰 `analyzeIndirect(inner, innerArgs, depth+1, role)` は**現在の `role` をそのまま渡す**。公開 API `AnalyzeIndirectExecution` は `analyzeIndirect(cmdPath, args, 0, risktypes.RoleInner)` を呼ぶ。`analyzeShebang` はインタプリタ連鎖を `RoleInterpreter` で評価する（`evaluateInnerAs(interp, ..., RoleInterpreter)` とその再帰）。（当初計画では `RoleInner` を固定する薄い `evaluateInner` ラッパーを残す想定だったが、`role` 引数化により同一シグネチャの単純な委譲となり冗長なため、レビュー指摘を受けて削除し呼び出し側は `evaluateInnerAs` を直接呼ぶ形へ変更した。）
- [x] `evaluateInnerAs`（[indirect_execution.go:756](../../../internal/runner/base/security/indirect_execution.go#L756)）に `role` による分岐を追加する。名前解決・特権判定・再帰（Critical/Reject 伝播）は現状のまま保持し、再帰後に `role == RoleInner` のときは細粒度算出ブロックを実行せず、`IndirectExecutionResult{Kind: IndirectFloor, Level: runnertypes.RiskLevelHigh, ReasonCodes: []risktypes.ReasonCode{risktypes.ReasonIndirectExecutionWrapper}, Artifacts: append(nested.Artifacts, artifact)}` を返す。**`Artifacts` は必ず `nested.Artifacts` に `artifact` を追記したものとする**（既存の細粒度算出ブロックの末尾 [indirect_execution.go:845](../../../internal/runner/base/security/indirect_execution.go#L845) と同様）。`floor(...)` ヘルパは `Artifacts` を空で返すため使用しない（ネストしたラッパー `env timeout nice ls` 等で内側のチェーンアーティファクトが消失するのを防ぐ）。
- [x] `role == RoleInterpreter` のときは既存の細粒度算出ブロック（[indirect_execution.go:785-846](../../../internal/runner/base/security/indirect_execution.go#L785-L846)、coreutils fail-closed Reject 分岐を含む）をそのまま実行する。`role` 伝播と併せ、`#!/usr/bin/env <interp>` のように `env` を経由する shebang インタプリタ連鎖も終始 `RoleInterpreter` で細粒度評価され、直接スクリプト実行の挙動を不変に保つ（§3.3）。

> **実装上の差分（`role` 伝播の副作用）**: `role` の再帰伝播により、`#!/usr/bin/env <interp>` の env ベース shebang では内側インタプリタ（例 `python`）の連鎖アーティファクトの `Role` が従来の `RoleInner` から `RoleInterpreter` に変わる（`env` 自身に加えインタプリタ本体も `RoleInterpreter` で記録される）。アーティファクトの `Role` は監査ログ専用メタデータであり（fd 束縛・identity 束縛は 0138 で取り下げ済み・production ロジックは `Role` で分岐しない）、リスク Level/Kind（=スコープ外のセキュリティ挙動）は不変。`#!/usr/bin/env python` を実際に実行するのは `python` であるため、これはより正確なラベル付けである。この副作用により §3.4「維持されるテスト」に挙げた `TestIndirect_ShebangInterpreterGated` の `RoleInterpreter` アーティファクト数アサーションのみ更新した（`env python`=2、`bin sh`/`bin bash`=1。結果 Level は High のまま不変）。

#### コメント修正（AC-08、英語で記述）

- [x] `wrapperSpec` 型コメントを修正する。`The runner re-implements these wrappers (it execs the extracted inner command itself), so the inner command is identity-bindable; that is why wrappers resolve rather than reject.` を「runner はラッパーを再実装せず・インナーを fd 束縛しない。ラッパーを解析するのはインナーを抽出してリスク評価（一律 High／特権=Critical／禁止形態=Reject）するためである」旨の英語コメントへ置換する。
- [x] `wrapperSpecs` 変数コメントの `the curated set of wrappers whose inner command the runner can extract and exec directly` を「インナーを抽出してリスク評価するための curated set（runner が直接 exec するのではない）」旨へ修正する。
- [x] `IndirectExecutionResult` 型コメントの `The actual fd binding and hash gating of each artifact ... is wired in the execution layer; here Artifacts carry their path and role for audit and for that later binding step.` を「`Artifacts` は監査用に path/role を保持する。ラッパーのインナーに対する fd 束縛・ハッシュゲートは行わない（0138 で取り下げ。実体固定が必要なら利用者が `verify_files` に明示登録）」旨へ修正する。
- [x] `analyzeIndirect` 内ラッパー分岐コメントの `Other wrappers the runner re-implements: extract the inner command and evaluate it` を「runner が再実装するのではなく、インナーを抽出してリスク評価する」旨へ修正する。
- [x] `IndirectFloor` 定数コメントの `a wrapped dangerous inner command -> their level` を「a wrapped extractable inner command -> High（一律下限）」旨へ修正する。
- [x] `analyzeService` 内コメント（[indirect_execution.go:920-921](../../../internal/runner/base/security/indirect_execution.go#L920-L921)）の `identity binding / disposition is populated when artifact gating is wired in the execution layer` を「`Artifacts` は監査用に init スクリプトの path/role を記録する。fd 束縛・ハッシュゲートは行わない（0138 で取り下げ）」旨へ修正する（`service` 自体の High 分類は不変）。

#### 単体テスト更新（`indirect_execution_test.go`）

- [x] 新規 `TestIndirect_WrapperInnerFlatHigh` を追加し、AC-01 を直接検証する: 無害インナー `env echo hi` / `timeout 5 echo hi` / `nice -n 10 build.sh` が `Kind==IndirectFloor` かつ `Level==RiskLevelHigh` かつ `ReasonCodes == []risktypes.ReasonCode{risktypes.ReasonIndirectExecutionWrapper}` であることをアサートする。
- [x] `TestIndirect_WrapperProfileFactors` の `want` を `env curl`/`timeout wget` とも `RiskLevelHigh` に更新し、関数コメントを「プロファイル要因は合流せず一律 High」へ書き換える。
- [x] `TestIndirect_WrappedProfileReasonsCarried` を削除する（撤廃された `RoleInner` 挙動の検証のため）。
- [x] 新規 `TestIndirect_InterpreterReasonsCollected` を追加し、§3.3 で維持する `RoleInterpreter` 経路の Reasons 収集が回帰しないことを locking する: プロファイルを持つコマンド名（例 `curl`）に対し `evaluateInnerAs("curl", []string{"https://example.com"}, 0, risktypes.RoleInterpreter)` を呼び、`Kind==IndirectFloor` かつ `Reasons` に当該プロファイル理由（"Always performs network operations"）を含むことをアサートする（`curl` は bare name のため `ResolveCommandNames` は自身へ解決し、`ResolveProfile` が name でマッチする）。
- [x] 新規 `TestIndirect_EnvShebangInterpreterNotFlattened` を追加し、`role` 伝播により env ベース shebang のスコープ外挙動が不変であることを検証する（§1.3）: `#!/usr/bin/env cat` の直接スクリプト実行が High に昇格しないこと（`Level < RiskLevelHigh`。無害インタプリタは細粒度評価のまま）、および `#!/usr/bin/env python` の直接スクリプト実行が従来どおり High であること（`IsArbitraryCodeExecutionRunner` 経由）を `t.TempDir()` にスクリプトを書いてアサートする。
- [x] `TestIndirect_CoreutilsInnerFolded` を更新する: `env <unknown applet>` の期待 reason を `risktypes.ReasonCoreutilsClassification` から `risktypes.ReasonIndirectExecutionWrapper` へ変更し、ghost（coreutils dir 配下の非存在ファイル = [02_architecture.md](02_architecture.md) §3.2 の「coreutils 分類失敗」ケース）の期待を `IndirectReject` から `IndirectFloor`/`RiskLevelHigh` へ変更する。関数コメントを「ラップされた coreutils インナーも一律 High に吸収（coreutils 固有の合流・fail-closed Reject は RoleInner では行わない）」へ更新する。
- [x] `TestIndirect_WrappedRunnerReasonCodesDeduped` を更新する: `env bash -c echo hi` の `ReasonCodes` が `[]risktypes.ReasonCode{risktypes.ReasonIndirectExecutionWrapper}` と等しいことをアサートする形へ書き換える。関数コメントの「重複 reason code を蓄積しない」という dedup 前提は単一 reason code 化により無意味になるため、「ラッパーインナーは単一の `indirect_execution_wrapper` を返す」旨へ書き換える（テスト名も実態に合えば見直す）。
- [x] `TestIndirect_InnerCommandGated` の関数コメントを「インナーは allowlist/ハッシュゲートされず一律 High。Reject/Critical の伝播と artifact 記録は維持（0136 AC-77 の再定義）」へ更新する（アサーション本体は不変で緑のまま）。
- [x] `TestIndirect_Taskset` の [:99](../../../internal/runner/base/security/indirect_execution_test.go#L99) コメント「A benign inner command stays low; the -p/--pid form runs no command.」を「benign インナーも一律 High（本ケースは `Kind` のみ検証）。`-p`/`--pid` 形態はコマンドを実行しない」旨へ修正する。

#### 単体テスト更新（`evaluator_test.go`）

- [x] `TestEvaluateRisk_WrappedProfileReasonsFolded`（[evaluator_test.go:249](../../../internal/runner/base/risk/evaluator_test.go#L249)）を削除する（`RoleInner` が profile reason を折り込まなくなるため。撤廃された挙動の検証）。

#### 完了ゲート

- [x] `make fmt && make test && make lint` が緑（NF-001）。`go test -tags test ./internal/runner/base/security/... ./internal/runner/base/risk/...` を含む全テストが通過。

### PR-1 作成ポイント: risk-evaluation core change (flat-High wrapper inner)

**対象ステップ**: Phase 1

**推奨タイトル**: `feat(0138): flatten wrapper inner command risk to a High floor`

**レビュー観点**: `role` の再帰伝播で shebang（`RoleInterpreter`）経路が不変か / フラット High の `Artifacts` が `nested.Artifacts` を保持するか（ネストしたラッパー） / Critical・Reject 伝播の維持と AC-08 コメント 6 箇所の網羅 / 影響テスト（`security`・`risk` 両パッケージ）の更新・削除と全テスト緑

- [x] グリーンゲート（`_context.md` の "Green gate" 参照）がパスしていることを確認した
- [x] PR を作成した（https://github.com/isseis/go-safe-cmd-runner/pull/743）
- [x] PR がマージされた
- [x] 次のブランチへ切り替えた（次ステップは新しいブランチで作業する）

### Phase 2: ユーザー向けドキュメント更新（F-004/AC-09）

**対象ファイル**: `docs/user/risk_assessment{.ja,}.md`, `docs/user/toml_config/04_global_level{.ja,}.md`, `docs/user/toml_config/05_group_level{.ja,}.md`, `docs/user/toml_config/06_command_level{.ja,}.md`, `README{.ja,}.md`

- [x] `risk_assessment.ja.md` §8 移行ノートのラッパー記述（[:253](../../../docs/user/risk_assessment.ja.md#L253)）を「ラッパー経由のインナーは一律 **High**（明示的に `risk_level = "high"` が必要）。特権昇格は **Critical**、一部形態（ローダ制御変数・`env -C`/解釈不能 `env -S`・find/xargs・動的ローダ・remote-shell・抽出不能・深さ超過・symlink 失敗）は依然 **Blocking**。インナーは自動的なハッシュ検証・identity 束縛の対象外（監査チェーンには成果物として記録されるが実体固定ではない。固定が必要なら `record` で記録し `verify_files` に明示登録）」へ書き換える。§5.2 の TOCTOU 残存リスク（[02_architecture.md](02_architecture.md) §5.2）も明記する。
- [x] `risk_assessment.ja.md` §3（間接実行）に同趣旨の記述があれば整合させる。→ §3 は名前・引数ベース／coreutils／バイナリ解析／フェイルクローズのみで、ラッパーインナー評価の専用記述は存在しないため変更不要（grep 確認済み）。
- [x] `04_global_level.ja.md` §4.6 の `verify_files`／「コマンドは自動的にハッシュ検証の対象となります」記述（[:950](../../../docs/user/toml_config/04_global_level.ja.md#L950)）に「ラッパー（`env`/`timeout` 等）のインナーコマンドは自動的なハッシュ検証・identity 束縛の対象外であり（監査チェーンには記録されるが実体固定ではない）、固定が必要なら `record` + `verify_files` に明示登録する」注記を追加する。
- [x] `05_group_level.ja.md`（`verify_files`/`cmd_allowed`）・`06_command_level.ja.md`（`risk_level`）・`README.ja.md`（セキュリティ機能概説）に同趣旨の記述があれば整合させる（無ければ変更不要、その旨を確認）。→ いずれもラッパーインナーに関する記述は無し（grep 確認済み）。変更不要。
- [x] 上記すべての `.ja.md` をコミット後、対応する英語版（`.md`）を `/mktrans` で整合させる（日本語版を正とする翻訳ワークフロー）。

### PR-2 作成ポイント: user-facing documentation (AC-09)

**対象ステップ**: Phase 2

**推奨タイトル**: `docs(0138): document flat-High wrapper inner behavior for users`

**レビュー観点**: ラッパーインナー=High／特権=Critical／一部 Blocking／自動検証・自動記録なしの記述が正確か / TOCTOU 残存リスク（§5.2）の明記 / `.ja.md` を先行コミットし `.md` を `/mktrans` で整合（翻訳ワークフロー）

- [x] グリーンゲート（`_context.md` の "Green gate" 参照）がパスしていることを確認した（docs-only のため自明に緑だが、ja/en 乖離は検知できない点に注意）
- [x] `.ja.md` を先行コミット後 `/mktrans` で `.md` を再生成し、AC-09 の `rg`（ja/en 双方）＋ ja↔en 章構成の目視で整合を確認した（本 PR の実質ゲート）
- [x] 条件付きステップ（`05_group_level`/`06_command_level`/`README`）は該当記述の有無を grep で確認し、無変更の場合はその旨を PR 説明に記録した
- [x] PR を作成した（https://github.com/isseis/go-safe-cmd-runner/pull/745）
- [x] PR がマージされた
- [x] 次のブランチへ切り替えた（次ステップは新しいブランチで作業する）

### Phase 3: 開発者向けドキュメント更新（F-004/AC-10）

**対象ファイル**: `docs/dev/architecture_design/security-architecture.md`, `docs/dev/architecture_design/command-risk-evaluation{.ja,}.md`

- [x] `security-architecture.md` の間接実行リゾルバ記述（[:433](../../../docs/dev/architecture_design/security-architecture.md#L433) 周辺）を本方式へ更新する: 抽出は維持／Critical・拒否を優先／通常インナーは一律 High／インナーの fd 束縛・ラッパー再実装はしない。→ `EvaluateRisk` コードブロック直後に「間接実行リゾルバ（ラッパー経由インナー）」段落を新設（一律 High／抽出維持／Critical・Reject 優先／fd 束縛・再実装なし）。`security-architecture` も `.ja.md`/`.md` の対なので ja を先に編集し `/mktrans` で en を整合（翻訳ワークフロー）。
- [x] `command-risk-evaluation.ja.md` の「内側コマンドの評価（`evaluateInnerAs`）では…すべて折り込み…」記述を「`RoleInner`（ラッパーインナー）は一律 High 下限（細粒度算出なし）。`RoleInterpreter`（shebang 直接スクリプト実行）は従来どおり細粒度算出を維持」へ書き換える（AC-10 の文言を超える追加更新。§1.3 参照）。→ 同記述に加え、同ファイル内の関連する陳腐化記述（`IndirectFloor` 説明・成果物記録の「後段の同一性束縛」・ラッパー項の「再実装し exec／同一性束縛できる」・設計意図ノート・まとめ）も「runner はインナーを再実装・exec・fd 束縛しない（外側コマンドの fd 束縛は不変）」へ併せて修正（AC-08 と同種の陳腐化修正。AC-10 の文言を超える追加更新）。
- [x] `command-risk-evaluation.ja.md` を更新後、英語版 `command-risk-evaluation.md` を `/mktrans` で整合させる。→ `security-architecture.md` も併せて `/mktrans` で整合（両ペアとも翻訳レビュー subagent で Critical/Major なし。1 件の Major〔`抽出不能`→`抽出不能ラッパー`〕は ja を明示化して解消）。

### PR-3 作成ポイント: developer architecture documentation (AC-10)

**対象ステップ**: Phase 3

**推奨タイトル**: `docs(0138): update developer architecture docs for flat-High`

**レビュー観点**: `security-architecture.md` の間接実行リゾルバ記述が本方式（抽出維持／Critical・拒否優先／一律 High／fd 束縛・再実装なし）へ更新されたか / `command-risk-evaluation` の「すべて折り込み」記述が `RoleInterpreter` 限定へ修正されたか / ja↔en 整合

- [ ] グリーンゲート（`_context.md` の "Green gate" 参照）がパスしていることを確認した（docs-only のため自明に緑だが、ja/en 乖離は検知できない点に注意）
- [ ] `command-risk-evaluation.ja.md` 更新後 `/mktrans` で `.md` を再生成し、AC-10 の `rg`（ja/en 双方）＋ ja↔en 章構成の目視で整合を確認した（本 PR の実質ゲート）
- [ ] PR を作成した
- [ ] PR がマージされた
- [ ] 次のブランチへ切り替えた（次ステップは新しいブランチで作業する）

### Phase 4: 0136 ドキュメントへの参照注記追加（F-003/AC-11）

**対象ファイル**: `docs/tasks/0136_runtime_risk_evaluation_enforcement/02_architecture.md`, `docs/tasks/0136_runtime_risk_evaluation_enforcement/03_implementation_plan.md`

- [ ] `0136/02_architecture.md` §3.3 ラッパー行（[:475](../../../docs/tasks/0136_runtime_risk_evaluation_enforcement/02_architecture.md#L475)）に、後続タスク 0138 によりラッパーインナーは一律 High・fd 束縛/再実装は取り下げへ変更された旨の 1〜2 行注記と 0138 ドキュメントへの参照を追加する（既存記述は詳細改訂しない）。
- [ ] `0136/02_architecture.md` §5.2 残存制約（許可ラッパーのインナーコマンド fd 束縛）に同様の注記を追加する。
- [ ] `0136/03_implementation_plan.md` Step 2-2 の保留 `[-]` 項目（[:288](../../../docs/tasks/0136_runtime_risk_evaluation_enforcement/03_implementation_plan.md#L288)）に同様の注記を追加する。
- [ ] `0136/03_implementation_plan.md` AC-60 行（[:574](../../../docs/tasks/0136_runtime_risk_evaluation_enforcement/03_implementation_plan.md#L574)）・AC-77 行（[:591](../../../docs/tasks/0136_runtime_risk_evaluation_enforcement/03_implementation_plan.md#L591)）に、0138 による改訂／再定義の旨の注記と 0138 参照を追加する。

### PR-4 作成ポイント: supersede notes in task 0136 docs (AC-11)

**対象ステップ**: Phase 4

**推奨タイトル**: `docs(0138): add 0138 supersede notes to task 0136 documents`

**レビュー観点**: 注記が最小限（1〜2 行＋0138 参照）で 0136 の既存記述を詳細改訂していないか / §3.3・§5.2・Step 2-2 `[-]`・AC-60・AC-77 の全該当箇所を網羅 / スナップショット方針（作業用ドキュメントは改訂しない）の遵守

- [ ] グリーンゲート（`_context.md` の "Green gate" 参照）がパスしていることを確認した
- [ ] PR を作成した
- [ ] PR がマージされた
- [ ] 次のブランチへ切り替えた（次ステップは新しいブランチで作業する）

## 3. 実装順序とマイルストーン

### 3.1 マイルストーン

| マイルストーン | フェーズ | 成果物 | 完了条件 |
|---|---|---|---|
| M1: コア確定 | Phase 1 | フラット High 化＋コメント修正＋単体テスト更新 | `make fmt && make test && make lint` 緑、AC-01〜08 検証通過 |
| M2: ユーザー文書 | Phase 2 | `risk_assessment`/`04_global_level`/関連ページ（ja+md） | AC-09 検証通過、ja/en 整合 |
| M3: 開発者文書 | Phase 3 | `security-architecture.md`／`command-risk-evaluation`（ja+md） | AC-10 検証通過 |
| M4: 0136 注記 | Phase 4 | 0136 ドキュメント 2 ファイルへの参照注記 | AC-11 検証通過 |

フェーズ間に強い依存はないが、コア（M1）を先に確定させてからドキュメント（M2〜M4）を整合させる。Phase 2/3 は各ファイルとも日本語版を先にコミットしてから英語版を `/mktrans` で整合させる。

### 3.2 PR 構成

各 PR は 1 フェーズに 1:1 対応する。PR-1 のみがコード変更（唯一のリスク PR）で、PR-2〜4 はドキュメント変更（相互に独立し、グリーンゲートへ影響しない）。コードを先行させ、PR-1 マージ後にドキュメント PR を作成する。

| PR | 対象ステップ | 主な変更内容 |
|---|---|---|
| PR-1 | Phase 1 | `evaluateInnerAs` の `role` 伝播＋`RoleInner` フラット High 化、AC-08 コメント 6 箇所、`security`・`risk` 両パッケージの単体テスト更新（F-001/F-003/AC-01〜08） |
| PR-2 | Phase 2 | ユーザー向け文書（`risk_assessment`／`04_global_level`／関連ページ）を本方式へ更新、ja→en（AC-09） |
| PR-3 | Phase 3 | 開発者向け文書（`security-architecture.md`／`command-risk-evaluation`）を本方式へ更新、ja→en（AC-10） |
| PR-4 | Phase 4 | タスク 0136 の該当箇所へ 0138 supersede 参照注記を追加（AC-11） |

## 4. テスト戦略

- **単体テスト（`indirect_execution_test.go`）**: AC-01 を新規 `TestIndirect_WrapperInnerFlatHigh` で直接検証（無害インナーが Floor/High／reason=`indirect_execution_wrapper`）。AC-02〜05 は既存テストの回帰確認（結果値不変）。
- **回帰確認（`evaluator_test.go`）**: 評価器レベルで env sudo=Critical／env LD_PRELOAD=Blocking／env rm -rf=High が不変であること（`TestEvaluateRisk_IndirectExecutionDeny`）。
- **スコープ外不変確認（§3.3）**: shebang（`RoleInterpreter`）テスト（`TestIndirect_ShebangInterpreterGated`/`TestIndirect_ShebangLongLineNotTruncated`/`TestIndirect_ShebangFifoNotRead`）が従来どおり通過すること（細粒度算出の維持を担保）。さらに新規 `TestIndirect_InterpreterReasonsCollected` で `RoleInterpreter` 経路の Reasons 収集が回帰しないことを、新規 `TestIndirect_EnvShebangInterpreterNotFlattened` で env ベース shebang（`#!/usr/bin/env cat`）が `role` 伝播により High に昇格しないこと（無害インタプリタ）を locking する。
- **セキュリティ回帰**: `TestIndirect_BypassAttackerScenarios` でフラット High が Critical/Reject を上書きしないこと。
- **後方互換テスト**: 本タスクは挙動を安全側へ強化する（無害インナーが Medium/Low → High）。これは意図した非互換であり、移行ノート（AC-09）で利用者に明示する。
- **ドキュメント検証（AC-07/09/10/11）**: §6 の AC 検証表に記載した `rg` コマンドで文言の存在／不在を確認し、ja/en 整合は目視確認で補う。

## 5. リスク管理

| リスク | 区分 | 緩和策 |
|---|---|---|
| `RoleInterpreter` 経路を誤って一律 High 化し、スコープ外（shebang）の挙動を変えてしまう | 技術 | `role == RoleInner` でのみ分岐し、`role` を再帰へ伝播させて env ベース shebang（`#!/usr/bin/env <interp>`）のインタプリタ連鎖も `RoleInterpreter` を維持（§1.3）。shebang テスト群＋新規 `TestIndirect_EnvShebangInterpreterNotFlattened`（§4）が緑であることを Phase 1 完了ゲートで確認 |
| アーキテクチャ §3.4 表に未記載の `evaluator_test.go` テスト失敗を見落とす | 技術 | §1.3 で `risk` パッケージの影響テストを列挙済み。Phase 1 で `make test` 全体を実行 |
| `command-risk-evaluation` 文書の陳腐化を放置 | スケジュール | §1.3／Phase 3 で AC-10 の追加更新として明示。レビューで指摘されても計画済み |
| ja/en ドキュメントの乖離 | 品質 | 各ファイル ja を先にコミットし `/mktrans` で en を生成する翻訳ワークフローを徹底 |

## 6. Acceptance Criteria 検証

各 AC を検証タスク・テスト位置にマップする。ラベル: `test`=実行可能テスト、`static`=`rg`/コンパイル、`manual`=目視/PR 観察（補助）。

| AC | ラベル | 検証位置 / コマンド | 期待結果 |
|---|---|---|---|
| AC-01 | test | `internal/runner/base/security/indirect_execution_test.go::TestIndirect_WrapperInnerFlatHigh` | 無害インナー（`env echo hi`/`timeout 5 echo hi`/`nice -n 10 build.sh`）が Floor/High／`ReasonCodes==[indirect_execution_wrapper]` |
| AC-01 | test | `...::TestIndirect_WrapperProfileFactors` / `...::TestIndirect_WrapperDestructive` | curl/wget/claude/rm 等いずれのインナーも High |
| AC-02 | test | `...::TestIndirect_WrapperSudoCritical` / `...::TestIndirect_NestedWrapperAndDepthGuard` / `...::TestIndirect_EnvSplitString` | 特権トークン（ネスト・`env -S` 隠蔽含む）が Critical |
| AC-03 | test | `...::TestIndirect_WrapperLoaderEnvRejected` / `...::TestIndirect_EnvChdirRejected` / `...::TestIndirect_EnvSplitString` / `...::TestIndirect_FindXargsTargetGated` / `...::TestIndirect_DynamicLoaderGated` / `...::TestIndirect_CommandExecOptionsGated` / `...::TestIndirect_NestedWrapperAndDepthGuard` / `...::TestIndirect_BrokenSymlinkChainFailsClosed` | 各拒否形態が Blocking（High に緩和されない） |
| AC-04 | test | `...::TestIndirect_UnextractableWrapperRejected` | 抽出不能ラッパーが Reject |
| AC-05 | test | `...::TestIndirect_WrapperNoCommandMedium` | コマンドを伴わないラッパー単体が Medium |
| AC-06 | test | `internal/runner/base/security/indirect_execution_test.go::TestIndirect_CoreutilsInnerFolded` / `...::TestIndirect_WrapperProfileFactors` | フラット High 化後はインナーの coreutils 分類・プロファイル要因が結果へ反映されない（一律 High）= インナーに対する細粒度の解決・検証処理を行わないことを挙動として実証。`ResolveCommandNames`（リスク評価用の名前解決のみ）は残る |
| AC-06 | static | `rg -n "role == risktypes.RoleInterpreter\|role != risktypes.RoleInner" internal/runner/base/security/indirect_execution.go` | `evaluateInnerAs` に role 分岐ガードが存在し、細粒度算出ブロックが `RoleInterpreter` 経路に限定されていることを構造的に確認（1 件以上） |
| AC-06 | manual | `evaluateInnerAs` のコードレビュー | `RoleInner` 経路が `ResolveCommandNames`（名前解決）・`isPrivilegeCommand`・再帰のみを呼び、ハッシュ検証・fd 束縛・allowlist 照合の API を呼ばないこと |
| AC-07 | static | `rg -n "AnalyzeIndirectExecution\|wrapperSpec\|inner" cmd/record/ internal/filevalidator/` | `record` 経路にラッパーインナー抽出・自動記録のロジックが存在しない（no matches）。インナーの自動記録を行わないことを確認 |
| AC-08 | static | `rg -n "re-implements these wrappers\|extract and exec directly\|wired in the execution layer\|Other wrappers the runner re-implements\|a wrapped dangerous inner command\|identity binding / disposition" internal/runner/base/security/indirect_execution.go` | 旧コメント文言が 0 件（6 箇所すべて修正済み。`identity binding / disposition` は `analyzeService` の 6 箇所目を独立に捕捉。§1.3 L44 参照 = architecture §3.4 の 5 箇所＋`analyzeService` 1 箇所） |
| AC-09 | static | `rg -n "identity 束縛の対象外" docs/user/risk_assessment.ja.md`（新規追記する固有アンカー文） | 1 件以上（ラッパーインナー=High／特権=Critical／一部 Blocking／インナーは自動ハッシュ検証・identity 束縛の対象外〔監査チェーン記録は実体固定ではない〕、を説明する新規文の存在を固有フレーズで確認）。英語版 `.md` も対応する固有フレーズ（例: "not automatically hash-verified or identity-bound"）で 1 件以上 |
| AC-09 | static | `rg -n "identity 束縛の対象外" docs/user/toml_config/04_global_level.ja.md` および対応 `.md` | §4.6 にラッパーインナーが自動ハッシュ検証・identity 束縛の対象外である旨の新規注記が 1 件以上 |
| AC-09 | static | `rg -n "評価・ゲートされます" docs/user/risk_assessment.ja.md` | 旧「ラップされた内部コマンドが評価・ゲートされます」記述が残っていない（no matches） |
| AC-09 | manual | `docs/user/*` の ja/en 目視 | 日本語版と英語版が構造・内容で整合（翻訳ワークフロー） |
| AC-10 | static | `rg -n "RoleInterpreter" docs/dev/architecture_design/command-risk-evaluation.ja.md` および対応 `.md` | 細粒度算出が `RoleInterpreter`（shebang）限定である旨の新規記述が 1 件以上（本方式では `RoleInner` は一律 High）。`security-architecture.md` も間接実行リゾルバ記述に「一律 High」相当の固有フレーズが 1 件以上 |
| AC-10 | static | `rg -n "すべて折り込み" docs/dev/architecture_design/command-risk-evaluation.ja.md`／`rg -n "folds in all of" docs/dev/architecture_design/command-risk-evaluation.md` | 旧「（RoleInner を含め無条件に）すべて折り込み／folds in all of」記述が `RoleInterpreter` 限定へ書き換えられ、無条件の折り込み記述が残っていない（該当行が `RoleInterpreter` 限定の文言に変わっていることを目視で確認） |
| AC-11 | static | `rg -n "0138" docs/tasks/0136_runtime_risk_evaluation_enforcement/02_architecture.md docs/tasks/0136_runtime_risk_evaluation_enforcement/03_implementation_plan.md` | §3.3／§5.2／Step 2-2 `[-]`／AC-60／AC-77 の各該当箇所に 0138 への参照注記が存在 |

## 7. 実装チェックリスト

- [ ] PR-1 マージ済み（対象ステップ: Phase 1）— コア（`role` 伝播＋`role` 分岐）＋ AC-08 コメント 6 箇所 ＋ `indirect_execution_test.go`（新規 3・アサーション更新 3・コメント修正 2・削除 1）＋ `evaluator_test.go` 削除 1 ＋ 完了ゲート緑（AC-01〜08）
- [ ] PR-2 マージ済み（対象ステップ: Phase 2）— ユーザー文書（ja → en）AC-09
- [ ] PR-3 マージ済み（対象ステップ: Phase 3）— 開発者文書（`security-architecture.md` ＋ `command-risk-evaluation` ja → en）AC-10
- [ ] PR-4 マージ済み（対象ステップ: Phase 4）— 0136 注記（02/03 各該当箇所）AC-11
- [ ] **全体**: §6 の全 AC 検証タスクが期待結果を満たす

## 8. クロスサーチチェックリスト

`make lint`/`make test` で検出できない残存参照・整合性のみを対象とする（§6 の AC 表と重複するコマンドは AC 表側に集約済み）。

- [ ] 旧 AC-08 コメント文言の残存がコード全体に無いこと（AC-08 の `rg` で担保）。
- [ ] `RoleInner` の細粒度算出撤廃に伴い、`Reasons` フィールドを参照するテスト／アサーションが `RoleInner` 前提で残っていないこと（`rg -n "\.Reasons" internal/runner/base/security/ internal/runner/base/risk/` で確認し、残るのは `RoleInterpreter`/shebang 由来のもののみ）。

## 9. Success Criteria

- ラッパー経由インナーが一律 High（特権=Critical／一部 Blocking）に確定し、無害インナーも明示 `risk_level = "high"` なしには実行されない（AC-01〜05）。
- 既存の Critical/Reject ディスポジションが（ネスト含め）後退なく維持される（AC-02〜04、`TestIndirect_BypassAttackerScenarios` 緑）。
- インナーの自動検証・fd 束縛・ラッパー再実装が導入されないことが、コード（コメント含む）と 0138 ドキュメントで一貫する（AC-06〜08）。
- 0136 の保留項目・supersede 対象 AC に 0138 への参照注記が付与される（AC-11）。
- ユーザー向け・開発者向けドキュメントが本方式へ更新され、ja/en が整合する（AC-09/10）。
- `make fmt` / `make test` / `make lint` が緑（NF-001）、ラッパーセマンティクスの runner 再実装を導入しない（NF-002）。

## 10. Next Steps

- 本実装計画（03）のレビューと `approved` への更新（人間レビュアー）。
- `approved` 後、Phase 1 から実装を開始する。
