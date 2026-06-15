# 実行時リスク判定の強化 — 実装計画書

## Document Status

| Item | Value |
|---|---|
| Status | `approved` |
| Created | 2026-06-15 |
| Review date | 2026-05-15 |
| Reviewer | isseis |
| Comments | 2026-06-15: main マージで取り込んだ 02 第7巡の変更へ追従 — `group_executor` を「`ResourceManager` 経由の呼び出し側に徹する（`EvaluateRisk`・比較・監査は manager 所有）」へ訂正（コンポーネント表・Step 2-3）、`xargs` をラッパー一覧から除外し子プロセス実行（find/xargs）ルールへ一本化、束縛可否チェックを副作用なし・実ステージング書込は normal の exec 直前のみ・dry-run は read-only 維持（AC-30/39）と Step 2-2 に明記。02 §1.2 概念モデル／§3.5 dry-run ハードエラー行の変更は 03 既存記述（評価器が resolve/verify/open・symlink 失敗=Blocking deny）と既に整合のため追加変更なし。2026-06-15: PR #727 自動レビュー（gemini 4件）を反映 — **fd 束縛実行の機構を是正**（runner はバッチ＝子プロセス起動・継続のため in-process `SYS_EXECVEAT` は runner 自身を置換し不可。検証済み fd を `ExtraFiles` で子へ継承し `os/exec` で `/proc/self/fd/<childfd>` を exec する方式へ。raw syscall/`unsafe`/`x/sys` direct 化を不要化。§1.3・Step 2-2・関連記述を更新）、systemctl argv 解析に `-n`/`--lines` 追加＋既知 verb 照合を主規則化（値オプション網羅漏れによる偽陽性 deny を抑制）、ローダ制御変数拒否に macOS `DYLD_*` を追加、シェル（`-c` のみ）とインタプリタ（`-c`/`-e`）の inline フラグ区別を明記（`bash -e script.sh` の誤解析回避）。2026-06-15: PR #727 自動レビュー第2巡（codex 9件・P1 3件含む）を反映 — **段階導入の feature gate を新設**（新強制経路は production 既定 OFF、有効化は fd 束縛 exec〔Phase 2〕＋構造化監査〔Phase 3〕が揃う Phase 3 まで行わない。中間 main が path exec で identity 強制／監査なし deny になるのを防止。P1 #136/#193）、**staging は保持中の検証済み fd を源泉とし path 再 open しない**（fd 未保持なら deny。P1 #230）、systemctl 解析に値オプション追加＋**未知オプションは High（hidden-verb-as-value バイパス防止）**（P1 #152）、`RiskAuditEntry`/`VerifiedFD` を `risktypes` 配置リスト＋Step 1-1 に追加（#258）、AC-37 静的 regex を陳腐化記述限定に絞り正当な medium を誤検出しない（#448）、`Classify` 完了条件の `-run` を実テスト名にマッチさせゼロマッチ素通りを防止（#106）、`cmd.Start` 失敗時の親 fd close ＋リークテスト追加（#231）、`CommandTemplate.RiskLevel` コメントの誤った「テンプレートへフォールバック」を訂正（#182）。2026-06-15: 【isseis 指示】mkplan2 で PR 境界を再設計（フェーズ 1:1 の 4 PR → 7 PR）。Phase 1 を「基盤（PR-1: 1-1〜1-3 types/config）」と「評価コア（PR-2: 1-4〜1-9）」に分割（`IsNetworkOperation`→`Classify` 削除が `EvaluateRisk` のコンパイルに直結する密結合のため評価コアは 1 単位）。fd 束縛 exec（PR-4）を高リスク隔離、文書を security/user（PR-6）と外部依存の command-risk-evaluation（PR-7、PR #724 後）に分離。Step を並べ替え renumber（F-015 を EvaluateRisk より前に＝1-7、EvaluateRisk=1-8）。§3.2 PR 構成表（依存列付き）・PR マーカー・§6 を PR ベース化。critical-review サブエージェント 2 パスで C-1（前方参照）/C-2（dry-run 非呼出前提）/C-3（feature flag 所有 Step 欠落）/M-2〜M-4 を解消。2026-06-15: 【isseis 指示】後方互換は考慮しないため **feature flag を撤廃**（C-3 で導入した flag は旧経路を削除する本タスクでは「OFF=旧挙動」が並列旧実装なしには成立せず不要。代わりに「各 PR は現行 main に対し単調改善で退行しない／完全な AC 充足主張・外部リリースは Phase 3 まで」で段階導入安全性を担保。codex PR #727 第3巡 P1 #207 解消）。同巡の他 5 件も反映 — PR-3（Step 2-1）に評価器順位 2 への配線＋e2e deny テストを追加し dead code 化を防止（P1 #239）、systemctl 未知オプションを結合形 `--opt=value`（安全スキップ）と分離形 `--opt`（High）で区別し read-only 整形オプションの偽陽性 deny を解消（#191）、`ReasonProfilePrivilege`/`ReasonProfileDestruction` を追加し各プロファイル要因に専用 reason code を割当（#92）、fd 所有権型を確定（`VerifiedIdentity.FD` を `*VerifiedFD`＝int 内包・`Close()`/`Fd()` 持ち。02 の `*int` スケッチを 03 で具体化、#279）、PR-6（ユーザー/セキュリティ文書）を PR-5 後マージの順序依存に変更し未実装挙動の誤記を防止（#451） |

本計画は `02_architecture.md`（status: `approved`）に基づき、`01_requirements.md` の F-001〜F-015 / AC-01〜AC-87（AC-52/53 は欠番）を実装するための作業手順・テスト対応・検証手段を定義する。設計判断・図・型定義は `02_architecture.md` を参照し、本書では重複させない。

---

## 1. 実装概要

### 1.1 目的

実行時リスク判定を設計意図どおり安全側（fail-safe / fail-closed）に動作させ、評価と実行を結合（`VerifiedCommandPlan`）し、監査可能性・dry-run の決定的予告・ドキュメント整合を確立する。詳細は `02_architecture.md` §1 を参照。

### 1.2 実装方針

- フェーズは `02_architecture.md` §8 の **縦切り** 構成（評価＋ゲート＋監査が各段で揃う）に従う。中間状態で identity 保証の無い実行束縛を作らない。
- 共有 DTO は `risk -> audit -> risk` 循環を避ける配置（§1.4 で確定）に置く。
- Go ソースのコメント・識別子・文字列リテラルは英語。計画本文（散文）は日本語。
- 各フェーズ完了時に `make fmt` → `make test` → `make lint` を通す（AC-21）。

### 1.3 既存コード調査結果

調査対象は `02_architecture.md` §3.4 の変更ファイル一覧。各箇所の現状と必要変更:

| 対象 | 現状 | 必要変更 |
|------|------|---------|
| [evaluator.go](../../../internal/runner/base/risk/evaluator.go) | `EvaluateRisk(cmd) (RiskLevel, error)`。privilege→destructive→coreutils→network→systemmod の **早期 return**（最大値でない）。プロファイル `BaseRiskLevel` を実行時に反映していない（F-001 未実装） | `VerifiedCommandPlan` 返却へ変更。全次元最大値・identity ゲート・間接実行解決・reason code 生成。早期 return を撤廃 |
| [network_analyzer.go](../../../internal/runner/base/security/network_analyzer.go) | `IsNetworkOperation(cmdName,args,contentHash) (bool,bool,error)`。プロファイル `NetworkType` 照合・引数 URL/SSH 検出・バイナリ解析を 1 関数に内包。`analyzeBinarySignals` は `contentHash==""` で `(true,true)`、レコード欠落/スキーマ不一致で `(true,true)` を返す（不確実と危険を 2値 bool で合流） | `Classify(cmdPath, contentHash) (BinaryAnalysisResult, error)` へ。プロファイル照合・引数検出を評価器へ移送。4区分（Clean/Network/HighRisk/Uncertain）＋根拠別 reason code を返す。`Uncertain` を導入し不確実を危険から分離 |
| [command_analysis.go](../../../internal/runner/base/security/command_analysis.go) | `IsDestructiveFileOperation(cmd,args)` / `IsSystemModification(cmd,args)` は basename リテラル一致（`extractAllCommandNames` で symlink 解決対応済みだが、これらの関数は素の `cmd` 文字列を map 照合）。`find -exec` は次要素のみ・`-execdir`/`-ok`/`-okdir` 未対応。`service` はプロファイルで High だが分類関数では一律扱い | 破壊/システム変更判定を basename・symlink 解決対応へ。危険引数パターン（`highRiskPatterns`/`mediumRiskPatterns`）を実行時評価へ統合。`find` 全実行アクション対応。`service`→High。symlink 解決失敗を Blocking 化（現状 §6 で High） |
| [coreutils.go](../../../internal/runner/base/security/coreutils.go) | `CoreutilsCommandRisk` は setuid→destructive→safe→Medium。未知サブコマンドは Medium。`findFirstSubcommand`（git オプション表流用）でサブコマンド抽出 | 未知/判別不能サブコマンドを High へ（AC-68）。バイナリ解析次元の抑制を評価器側で明示。識別境界（identity ゲートを coreutils より前段）に整合 |
| [command_risk_profile.go](../../../internal/runner/base/security/command_risk_profile.go) | `BaseRiskLevel()` は 5 要因の `max`。`SystemModRisk` は静的 | `SystemModRisk` を引数条件付きで導出する仕組みを評価器側に追加（プロファイル静的値を無条件に持ち込まない。F-011） |
| [config.go](../../../internal/runner/base/runnertypes/config.go) | `ParseRiskLevel("unknown")` → `(RiskLevelUnknown, nil)`（エラーにしない） | `"unknown"` を `ErrInvalidRiskLevel` でエラー化（AC-24）。`"critical"` は既にエラー |
| [spec.go](../../../internal/runner/base/runnertypes/spec.go) | `CommandTemplate.RiskLevel` コメント `nil: inherit from global default`、`CommandSpec.RiskLevel` コメント `nil=inherit default`（存在しない継承を示唆） | コメントを実態（`nil` はテンプレートフォールバック後 `low`）へ修正（AC-16） |
| [normal_manager.go](../../../internal/runner/resource/normal_manager.go) | `EvaluateRisk` の戻り値 `RiskLevel` で比較。`audit.Logger` を保持せず監査未出力 | `VerifiedCommandPlan` 利用・`audit.Logger` 注入・decision 記録・deny 重大度下限（AC-11/56/70） |
| [dryrun_manager.go](../../../internal/runner/resource/dryrun_manager.go) | `RiskEvaluator` を受け取らず、`AnalyzeCommandSecurity` ベースの表示のみ。実行可否予告なし（背景 I） | 同一評価器（read-only 解析）で実効リスク＋allow/deny 予告。検証不能 deny の運用区別（終了コード/CI オプション、AC-58） |
| [default_manager.go](../../../internal/runner/resource/default_manager.go) | dry-run に `RiskEvaluator`・`audit.Logger` を配線していない | dry-run マネージャへ評価器・監査ロガーを配線（AC-39/56） |
| [logger.go](../../../internal/runner/base/audit/logger.go) | `LogRiskProfile(ctx, commandName, baseRiskLevel, riskReasons, networkType)` のみ。相関フィールド（resolved_path/content_hash/decision/max_allowed_risk/reason_codes）なし。**本番未呼び出し（デッドコード、背景 B）** | `LogRiskProfile(ctx, RiskAuditEntry)` パラメータ構造体へ。相関フィールド・引数マスキング・連鎖監査・deny 重大度下限 |
| [group_executor.go](../../../internal/runner/group_executor.go) | `verifyGroupFiles` で `ResolvePath`＋ハッシュ伝播。`executeCommandInGroup`（:520-528）で **実行直前に再度 `ResolvePath` し `cmd.ExpandedCmd` を上書き**（TOCTOU 二重解決） | `ResourceManager` 経由で実行を委譲する呼び出し側に徹する（`EvaluateRisk`・比較・`LogRiskProfile` は **manager が所有**。group_executor は自前で呼ばない）。実行直前の独立した再 `ResolvePath`（二重解決）を廃止（AC-64/76） |
| [executor.go](../../../internal/runner/base/executor/executor.go) | `Execute(ctx, cmd, env, outputWriter)` が `cmd.ExpandedCmd`（パス）を exec。fd ベース実行なし | `VerifiedCommandPlan` のみ exec。fd 束縛実行（検証済み fd を子へ継承し `os/exec` で `/proc/self/fd/<childfd>` を exec、§1.3 参照）第一候補、read-only ステージングがフォールバック。元 argv/env 直接 exec を禁止（AC-76/79） |
| `security/indirect_execution.go` | **存在しない** | 新規作成。ラッパー/シェル/ローダ/find-exec/shebang/オプションの検出・抽出・ゲート・identity 束縛・拒否（F-013/F-014） |
| `docs/dev/architecture_design/command-risk-evaluation.{ja,md}` | **main に存在しない**（PR #724 にのみ存在。§3.4 注・付録参照） | PR #724 マージ後に AC-15/17/18 を反映（フェーズ 4） |
| [security-architecture.md](../../../docs/dev/architecture_design/security-architecture.md) / `.ja.md` | `:417` 付近に旧シグネチャ `EvaluateRisk(cmd *runnertypes.Command) (RiskLevel, error)`、`:1039` 付近に "Graceful degradation when security features are unavailable" | §5.3 の 2 例外（fail-closed 反転・シグネチャ更新）を反映 |
| [risk_assessment.ja.md](../../../docs/user/risk_assessment.ja.md) / `.md` | `dpkg` 記載・最大値記述・systemctl=medium 等が実装と乖離（背景 J） | F-010（AC-34〜38/50）に整合 |

**確定済み事項（設計委譲の解消）**: systemctl サブコマンド分類は `02_architecture.md` §3.6.1、identity 束縛契約は §3.6.2、監査配線は §3.6.3、間接実行対象は §3.3/§3.6.4、リスク次元優先順位は §6.1 で確定済み。本計画はこれらを実装へ落とす。

**fd ベース実行の機構（重要・設計確定）**: runner は **バッチ実行プロセス**であり、各コマンドを **子プロセスとして起動して継続** する（現行 `executor.go` は `exec.CommandContext`＝fork+exec）。したがって、`execveat`/`execve` を **runner 自身のプロセスで直接呼んではならない**——それは runner プロセス自体を置換して終わらせてしまう。Go では、マルチスレッドランタイム上で fork した子プロセス側でのみ `execveat` を非同期シグナル安全に呼ぶことは現実的でなく（`os/exec`・`syscall.ForkExec` の子プロセス側実装はパスベースの `execve` 固定で `execveat` を差し込めない）、自前の in-process `SYS_EXECVEAT` 呼び出しは採らない。

したがって Linux での fd 束縛実行は、**保持している検証済み fd を子プロセスへ継承させ、`os/exec` に実行パスとして `/proc/self/fd/<childfd>`（同義 `/dev/fd/<childfd>`）を渡す**方式で実装する（glibc の `fexecve` が古いカーネルで用いるのと同じ手法）。fd は検証済み inode を指すため、元のパス名がすり替えられても子プロセスが exec するのはその inode であり、TOCTOU・symlink すり替えを閉じる（AC-76）。実装上は検証済み fd を `cmd.ExtraFiles` で子へ継承（CLOEXEC を解除）し、子側 fd 番号（`3 + index`）から `/proc/self/fd/<n>` を組み立てる。`os/exec` の堅牢な fork+exec をそのまま使え、`unsafe`/`unix.Syscall6`/GC 回避（`runtime.KeepAlive`）は不要になる。

- **前提・限界**: `/proc` がマウントされていること（Linux 前提）。exec 権限は fd の参照先に対して評価される。`#!` スクリプトや hidepid 構成の扱いは実装で確認する。
- **フォールバック**: `/proc/self/fd` 実行が不能な環境（非 Linux・`/proc` なし等）は **read-only ステージング**（§3.6.2）。双方不能なら **評価段階で Blocking deny**（§3.6.2 の可否判定。副作用なし）。
- **依存**: 本方式は標準ライブラリ（`os/exec`・`os`・`syscall`）のみで実現でき、`golang.org/x/sys` を **direct 依存へ昇格する必要はない**（`SYS_EXECVEAT` の自前ラッパーが不要になったため）。

### 1.4 共有 DTO の配置決定

`02_architecture.md` §3.1 / §3.4 が「03 で確定」とした共有 DTO の配置を以下に確定する:

- 配置先: 新規パッケージ `internal/runner/base/risktypes/`（`runnertypes` よりさらに下位に依存を持たない中立パッケージ）。
- 配置する型: `VerifiedCommandPlan`, `VerifiedIdentity`, `RiskAssessment`, `ExecutedArtifact`, `RiskAuditEntry`（監査エントリ DTO。`audit.LogRiskProfile` の引数型）, `VerifiedFD`（クローズ可能 fd ラッパー。型は Step 1-1 で宣言、実装/利用は Step 2-2）, `BinaryAnalysisClass`, `BinaryAnalysisResult`, `ReasonCode`（＋定数）, `ArtifactRole`（＋定数）, `ArtifactDisposition`（＋定数）, `ErrorClass`（＋定数）, `Decision`（＋定数）, `ExecutionMode`（＋定数）。`RiskAuditEntry` を `risktypes` に置くことで、`audit` が `risk` を import せずに監査エントリを受け取れる（循環回避）。
- 根拠: `risk`・`audit`・`security`・`resource` の全てがこのパッケージのみに依存し、相互の循環を回避する。`runnertypes.RiskLevel` は `risktypes` が import する（`runnertypes` は `risktypes` を import しない）。

> 注: `02_architecture.md` は `runnertypes` 同居案も併記したが、`runnertypes` は設定型の中核で広く import されるため、リスク評価専用 DTO を混在させると責務が肥大する。独立パッケージ `risktypes` を採用する（YAGNI に反しない最小の分離）。

---

## 2. 実装ステップ

各 Step は **対象ファイル**・**作業内容（チェックボックス）**・**完了条件** を持つ。設計詳細は `02_architecture.md` の該当節を参照。

## Phase 1 — 評価コア＋拒否ゲート（F-001/F-002/F-005/F-007/F-008/F-011）

縦切りの到達点: **評価コアと normal の deny ゲートのロジックが入る**（間接実行は Phase 2、fd 束縛 exec は Phase 2、構造化監査 `LogRiskProfile` は Phase 3）。

> **段階導入の安全性（後方互換は考慮しない / feature flag を設けない）**: 本タスクは旧挙動との後方互換を保持しない。各 PR で旧経路（旧 `EvaluateRisk` API・`IsNetworkOperation`・path-only executor）を置換・削除するため、「旧挙動へ戻す」フラグは並列の旧評価器/実行系を温存しない限り成立せず、それは不要なコストとなる。代わりに次の不変条件で段階導入の安全性を担保する: **(1) 各 PR は現行 main に対する単調改善である**（PR-2 は deny ゲートを追加するのみ。現行 main は deny ゲートも fd 束縛も持たないため、中間 main がどの時点でも現行より悪化しない。とくに PR-2〜PR-4 間の path 実行は現行 main と同じ TOCTOU 特性で、退行ではない）。**(2) 完全な AC 充足の主張・外部リリースは Phase 3（PR-5）完了まで行わない**（AC-64/76 の TOCTOU 封鎖は fd 束縛 exec が入る PR-4、AC-56/70 の構造化監査は PR-5 で初めて成立する。それまでは「実装途上」であり外部リリース可否ゲート未達）。各 Phase の完了ゲートは `make fmt && make test && make lint` を緑にする。

### Step 1-1: `risktypes` パッケージと中核型を新設

**対象ファイル**: `internal/runner/base/risktypes/types.go`（新規）, `internal/runner/base/risktypes/reason_codes.go`（新規）

- [x] `VerifiedCommandPlan` / `VerifiedIdentity` / `RiskAssessment` / `ExecutedArtifact` / `RiskAuditEntry` を定義（`02_architecture.md` §3.1/§3.2 の型に一致。`RiskAuditEntry` は `Mode`/`Decision`/`MaxAllowedRisk`/`Chain` 等を持ち、Phase 3 の `LogRiskProfile` がこの型を受ける）。
- [x] **fd 所有権の型を確定（02 §3.6.2 が 03 へ委譲）**: `VerifiedFD` を「生 fd（`int`）を内包し `Close() error` と raw fd 取得（例 `Fd() int`）を持つクローズ可能ラッパー」として定義する。`VerifiedIdentity` および各 `ExecutedArtifact` は **fd を `*VerifiedFD`（nil=fd なし）で保持**する（02 が `FD *int` とスケッチしていた箇所を本タスクでは `*VerifiedFD` に確定。所有権・close を一元化）。`/proc/self/fd/<n>` の `n` はこのラッパーの `Fd()` から得る（int を直接扱うコードも矛盾なく利用できる）。生 `int` フィールドは持たせない（close 漏れ・二重所有を防ぐ）。
- [x] `BinaryAnalysisClass`（`Uncertain=0` がゼロ値=fail-closed）＋ `BinaryAnalysisResult` を定義（§3.1）。
- [x] `ReasonCode`（string 派生型）＋全定数を定義。最低限以下を含む（網羅は AC-69 でテスト）: `ReasonDestructiveFileOperation`, `ReasonSystemModification`, `ReasonPrivilegeEscalation`, `ReasonCoreutilsClassification`, `ReasonProfilePrivilege`, `ReasonProfileDestruction`, `ReasonProfileDataExfil`, `ReasonProfileNetwork`, `ReasonProfileSystemMod`, `ReasonBinaryAnalysisNetwork`, `ReasonBinaryAnalysisDynamicLoad`, `ReasonBinaryAnalysisExec`, `ReasonBinaryAnalysisSVC`, `ReasonBinaryAnalysisMprotectExec`, `ReasonUncertainMissingRecord`, `ReasonUncertainSchemaMismatch`, `ReasonUncertainHashMismatch`, `ReasonUncertainUnsupportedFormat`, `ReasonUncertainUnverifiedIdentity`, `ReasonAnalysisDisabled`, `ReasonArbitraryCodeExecution`, `ReasonDangerousArgPattern`, `ReasonSymlinkResolutionFailed`, `ReasonIdentityUnbound`, `ReasonIndirectExecutionRejected`, `ReasonForbiddenEnvVar`。各定数の文字列値は snake_case の英語（例: `"destructive_file_operation"`）。
- [x] `ErrorClass`（string 派生型）＋定数: `ErrorClassSymlinkResolution`, `ErrorClassCoreutilsFileInfo`, `ErrorClassRecordLoad`, `ErrorClassPathResolution`。
- [x] `ArtifactRole` / `ArtifactDisposition` / `Decision`（`DecisionAllow`/`DecisionDeny`）/ `ExecutionMode`（`ModeNormal`/`ModeDryRun`）を定義（§3.2）。

**完了条件**: `go build -tags test ./internal/runner/base/risktypes/` が通る。型のゼロ値が fail-closed（`BinaryAnalysisClass` ゼロ値 = `Uncertain`）であることを単体テストで確認。

### Step 1-2: `ParseRiskLevel("unknown")` のエラー化

**対象ファイル**: [config.go](../../../internal/runner/base/runnertypes/config.go), [config_test.go](../../../internal/runner/base/runnertypes/config_test.go)

- [x] `ParseRiskLevel` に `case "unknown":` を追加し `ErrInvalidRiskLevel` を返す（`"critical"` と同様。AC-24）。`"low"/"medium"/"high"/省略/空文字` は不変（AC-26）。
- [x] `config_test.go` に `ParseRiskLevel("unknown")` エラーケースを追加。

**完了条件**: `go test -tags test ./internal/runner/base/runnertypes/ -run RiskLevel` が通る。

### Step 1-3: `spec.go` コメント修正

**対象ファイル**: [spec.go](../../../internal/runner/base/runnertypes/spec.go)

- [x] `CommandTemplate.RiskLevel`（:46 付近）コメントを `// nil: inherit from global default, otherwise must be one of: "low", "medium", "high"` から `// nil: no template-level default (a command that omits risk_level falls back here; if this is also nil, GetRiskLevel() yields "low"); otherwise must be one of: "low", "medium", "high"` へ変更（テンプレート自身が「テンプレートへフォールバック」する誤記を避け、テンプレート nil＝テンプレート既定なし、と明示。AC-16）。
- [x] `CommandSpec.RiskLevel`（:259 付近）の行末コメントを `// Maximum allowed risk level (nil=inherit default, otherwise: low, medium, high)` から `// Maximum allowed risk level (nil defaults to "low" after template fallback; otherwise: low, medium, high)` へ変更。

**完了条件**: `rg -n "inherit from global default|nil=inherit default" internal/runner/base/runnertypes/spec.go` が 0 件。

### PR-1 作成ポイント: shared types and config foundation

**対象ステップ**: 1-1 / 1-2 / 1-3

**推奨タイトル**: `feat(0136): risktypes package and risk_level config foundation`

**レビュー観点**: `risktypes` のゼロ値 fail-closed（`BinaryAnalysisClass`=Uncertain）/ 共有 DTO の最下位中立パッケージ配置（循環回避）/ `ParseRiskLevel("unknown")` のエラー化と既存値の不変 / 評価器・実行系へのシグネチャ波及が無い（本 PR は新規型・config・コメントのみで既存挙動を変えない）

- [x] グリーンゲート（`_context.md` の "Green gate" 参照）がパスしていることを確認した
- [x] PR を作成した（https://github.com/isseis/go-safe-cmd-runner/pull/728）
- [x] PR がマージされた
- [x] 次のブランチへ切り替えた（次ステップは新しいブランチで作業する）

### Step 1-4: `NetworkAnalyzer.Classify` へリファクタ

**対象ファイル**: [network_analyzer.go](../../../internal/runner/base/security/network_analyzer.go), [network_analyzer_test.go](../../../internal/runner/base/security/network_analyzer_test.go)

- [x] `IsNetworkOperation` を `Classify(cmdPath string, contentHash string) (risktypes.BinaryAnalysisResult, error)` に置換。プロファイル `NetworkType` 照合・引数 URL/SSH 検出を **削除**（評価器へ移送）。
- [x] `analyzeBinarySignals` の 2値 bool 合流を 4区分へ分解: 危険シグナル（dlopen/exec/svc/mprotect）→ `BinaryAnalysisHighRisk`＋該当 reason code、ネットワークのみ → `BinaryAnalysisNetwork`、シグナルなし → `BinaryAnalysisClean`、レコード欠落/スキーマ不一致/ハッシュ不一致/非対応/想定外/`contentHash==""` → `BinaryAnalysisUncertain`＋該当 reason code。
- [x] `handleAnalysisOutput` の `NotSupportedBinary`/`StaticBinary` 分岐を見直し: 解析データを入手できないケースは `Uncertain`（fail-open で Clean に落とさない。AC-45）。解析成功・危険なしのみ `Clean`。
- [x] 真の I/O 障害（分類不能なレコード読込エラー）は `error` 返却を維持（§4(3)）。
- [x] `network_analyzer_test.go` を `Classify` の 4区分・reason code 網羅へ更新。

**完了条件**: `go test -tags test -v ./internal/runner/base/security/ -run 'TestClassify'` が通り、**`TestClassify_AllResultClasses` と `TestClassify_DistinctReasonCodes` が実際に実行された**ことを `-v` 出力で確認する（`go test -run` はマッチ 0 件でも成功するため、テスト名にマッチするパターンを用い、実行ログで両テストの `--- PASS` を確認する）。4区分すべてと各 reason code が網羅される。

### Step 1-5: `command_analysis.go` を解決済み絶対パス対応へ

**対象ファイル**: [command_analysis.go](../../../internal/runner/base/security/command_analysis.go), [command_analysis_test.go](../../../internal/runner/base/security/command_analysis_test.go), [command_analysis_dangerous_test.go](../../../internal/runner/base/security/command_analysis_dangerous_test.go)

- [x] `IsDestructiveFileOperation` / `IsSystemModification` を basename・symlink 解決対応へ（`extractAllCommandNames` を用い、絶対パス `/usr/bin/rm` でも検出。AC-06/07/08）。部分一致誤検出を防ぐ（`lsrm`/`systemctl-helper` 非該当。AC-09）。
- [x] `find` の実行アクションを `-exec`/`-execdir`/`-ok`/`-okdir` 全対応に（AC-62）。対象コマンドにも basename 正規化・symlink 解決を適用。
- [x] `service`→High（プロファイルと分類関数の双方で一貫。AC-22/75）。
- [x] 危険引数パターン（`highRiskPatterns`/`mediumRiskPatterns`: `rm -rf`/`dd if=`/`chmod 777`/`chown root` 等）を実行時評価へ統合できるよう、評価器から呼べる純関数として公開（`checkCommandPatterns` の利用境界を整理。AC-47）。
- [x] symlink 解決失敗（深度超過・リンク先取得失敗・循環・解決不能）を、High ではなく **Blocking deny シグナル**として呼び出し元（評価器）へ伝える経路を用意（`ErrorClassSymlinkResolution`。AC-54/55）。現状 `AnalyzeCommandSecurity` Step 2 の深度超過 High 扱いは dry-run 表示用途のため、評価器パスでは Blocking に倒す（§4(1)）。

**完了条件**: `go test -tags test ./internal/runner/base/security/ -run 'CommandAnalysis|Destructive|SystemMod'` が通る。絶対パス入力ケースを含む。

### Step 1-6: `coreutils.go` の未知サブコマンド High 化

**対象ファイル**: [coreutils.go](../../../internal/runner/base/security/coreutils.go), [coreutils_test.go](../../../internal/runner/base/security/coreutils_test.go)

- [x] `CoreutilsCommandRisk` の未知/判別不能サブコマンド既定を Medium → **High**（AC-68）。`safeCoreutilsCommands` に明示された安全サブコマンドのみ Low。
- [x] `findFirstSubcommand`（git オプション表流用）を coreutils 文脈で正しく動くサブコマンド抽出へ。判別不能時は High（安全側）。
- [x] `coreutils_test.go` に未知サブコマンド High・`coreutils <未解析>` が Medium 通過しないケースを追加。

**完了条件**: `go test -tags test ./internal/runner/base/security/ -run Coreutils` が通る。

### Step 1-7: F-015 任意コード実行ランナー分類

**対象ファイル**: [evaluator.go](../../../internal/runner/base/risk/evaluator.go) もしくは `internal/runner/base/security/arbitrary_code.go`（新規・小規模）, 対応テスト

- [x] 任意コード実行コマンドの確定リストを定義（`02_architecture.md` §6.1 順位 7）: シェル（`bash`/`sh`/`dash`/`zsh` 等）、インタプリタ（`python`/`node`/`ruby`/`perl`/`php`/`lua`/`java`/`dotnet`/`pwsh` 等）、ビルド/タスクランナー（`make`/`cmake`/`ninja`/`gradle`/`mvn`/`bazel`/`rake`/`just`/`task`）。リストは英語識別子の `map[string]struct{}`。
- [x] basename・symlink 解決で照合し、該当なら引数によらず High を最大値へ合流＋`ReasonArbitraryCodeExecution`（AC-73/74）。`--version`/`--help` も既定 High（注 1）。
- [x] パッケージスクリプトランナー（`npm run`/`npx`/`yarn <script>`/`pnpm run`）の High は間接実行と重なるため、引数形態判定を含めて Phase 2 の `indirect_execution.go` 側に置く（AC-85）。本 Step は引数非依存の一律 High 群を担当。

**完了条件**: `go test -tags test ./internal/runner/base/risk/ -run ArbitraryCode` が通る。

### Step 1-8: `EvaluateRisk` を最大値モデル＋identity ゲートへ

**対象ファイル**: [evaluator.go](../../../internal/runner/base/risk/evaluator.go), [evaluator_test.go](../../../internal/runner/base/risk/evaluator_test.go), [coreutils_consistency_test.go](../../../internal/runner/base/risk/coreutils_consistency_test.go)

- [x] `Evaluator` インターフェースと `StandardEvaluator.EvaluateRisk` の戻り値を `(risktypes.VerifiedCommandPlan, error)` へ変更。
- [x] `02_architecture.md` §6.1 のリスク次元優先順位を実装:
  - [x] 順位 1: identity ゲート（content_hash 有・検証/解析有効・identity 束縛可）。不成立で `Blocking=true`＋`ReasonUncertainUnverifiedIdentity`/`ReasonAnalysisDisabled`/`ReasonIdentityUnbound`（AC-51/65）。**このゲートは順位 4〜8 の全次元算出より前に短絡する**ため、未検証ハッシュのコマンドが coreutils 判定・破壊判定・プロファイル・F-015 の **いずれの経路でも** Low/High-allowable に確定する前に Blocking deny へ帰着する（F-014/AC-65 の全経路一貫適用）。実装上は「ハッシュ未確定なら順位 4 以降に進ませない」単一ゲートで全経路を覆う（経路ごとに個別チェックを散らさない＝取りこぼし防止）。Phase 1 では fd 束縛可否は「検証済みハッシュ有 ＋ 解析有効」までを判定し、fd 取得は Phase 2 で接続（Phase 1 完了時は path ベース実行のままだが deny ゲートは機能する）。
  - [x] 順位 2: 間接実行の拒否/解決（抽出不能・identity 束縛不能・禁止 env 等 → `Blocking`＋reason code）。**実装は Phase 2（Step 2-1）で行う**ため Phase 1 のこの Step では未実装（順位だけ 02 §6.1 と揃えて明示。Phase 1 完了時点では順位 1・3〜8 が機能し、順位 2 は Phase 2 で接続）。
  - [x] 順位 3: 特権昇格 → `Level=Critical`＋`ReasonPrivilegeEscalation`（§3.1 の Critical 一貫扱い）。
  - [x] 順位 4: coreutils 分類（バイナリ解析次元を除外して算出。識別ゲートの後段）。
  - [x] 順位 5: プロファイル要因（引数評価後の SystemModRisk を含む最大値。`BaseRiskLevel` を SystemModRisk 差し替え後に計算）。**昇格させた要因に応じた専用 reason code を付す**（`PrivilegeRisk`→`ReasonProfilePrivilege` / `DestructionRisk`→`ReasonProfileDestruction` / `NetworkRisk`→`ReasonProfileNetwork` / `DataExfilRisk`→`ReasonProfileDataExfil` / `SystemModRisk`→`ReasonProfileSystemMod`）。引数/パターン由来のランタイム code（例 `ReasonDestructiveFileOperation`）を流用しない（プロファイル起因か引数起因かを監査で区別。AC-12/69）。
  - [x] 順位 6: 危険引数パターン（Step 1-5 の公開関数）。
  - [x] 順位 7: F-015 任意コード実行ランナー（Step 1-7、前出の分類関数を利用）。
  - [x] 順位 8: バイナリ解析分類（`Classify` 結果を Clean→Low/Network→Medium/HighRisk→High/Uncertain→Blocking に写像し最大値へ合流。reason code を `RiskAssessment.ReasonCodes` に転記）。
- [x] 早期 return を撤廃し、順位 4〜8 を **全次元最大値**（順序非依存、F-001/AC-63）で算出。
- [x] `RiskAssessment` に `Level`/`Blocking`/`BlockingReason`/`ErrorClass`/`ReasonCodes`/`Reasons`/`NetworkType` を設定。`Blocking=true` または `Level=Critical` の拒否では必ず `BlockingReason` を設定（§3.1）。
- [x] F-011 サブコマンド条件付き SystemModRisk 導出（`02_architecture.md` §3.6.1 の確定リスト: 変更系→High、読み取り専用→Medium 下限、未知→High、`service` は全アクション High）。
- [x] **systemctl argv 解析規則の確定（§3.6.1 M-9 が 03 へ委譲）**: 現行 `findFirstSubcommand`（git オプション表流用）を systemctl 用パーサ `firstSystemctlSubcommand(args)` に置換する。規則:
  - 値を取るオプション（次トークンを consume）: `-H`/`--host`, `-M`/`--machine`, `-t`/`--type`, `--state`, `-p`/`--property`, `-P`, `--what`, `--job-mode`, `--root`, `--image`, `--drop-in`, `--when`, `--kill-whom`, `-s`/`--signal`, `--timestamp`, `--output`/`-o`, `-n`/`--lines`。これらの直後トークンはサブコマンドとして扱わない。
  - 結合形 `--opt=value` は 1 トークンで完結（次トークンを consume しない）。
  - 真偽オプション（値なし、例 `--now`/`--no-pager`/`--quiet`/`-q`/`--user`/`--system`）は単純スキップ。
  - **未知オプションの扱いは結合形か分離形かで区別する**（hidden-verb-as-value バイパスは「分離形 `--opt value` の value が verb 名になる」場合のみ起きる点に基づく）:
    - 未知の **結合形 `--opt=value`**（`=` を含む）は次トークンを consume しないため verb を隠せない。**安全にスキップ**してよい（`systemctl --legend=false status` の `status` を正しく verb 認識でき、`--legend`/`--check-inhibitors=` 等の整形・問い合わせ系オプションが read-only コマンドを誤って High にしない。AC-49 の Medium 下限を保つ）。
    - 未知の **分離形 `--opt`**（`=` を含まない `-`/`--` トークン）は値を取るか不明で、取る場合に次トークン（verb 名）を隠し得る。**verb 照合に頼らず High（安全側）** に倒す（値オプション表の網羅漏れを fail-safe に閉じる）。
    - 既知の値/真偽オプションは従来どおり（値オプションは次トークン consume、真偽はスキップ）。`systemctl --help` に列挙される値オプションは可能な範囲で既知表へ追加し、分離形での偽陽性 High を減らす。
  - オプション終端 `--`: 以降の最初のトークンを無条件にサブコマンドとして採用。
  - **既知オプションのスキップ後、最初の非オプショントークンを既知 verb 集合とマッチ**: systemctl の既知サブコマンド（`status`/`show`/`start`/`stop`/`restart`/`reload`/`enable`/`disable`/`mask`/`is-active`/… 変更系・読み取り系の確定集合は §3.6.1）に一致すればそれを採用。一意に判別できない（空・解析破綻・既知 verb 不一致）場合は High（安全側、§3.6.1）。
  - 既知 verb 集合・値/真偽オプション表は実装で保守する。網羅漏れ・未知オプション・未知 verb のいずれも **High（fail-safe、偽陰性なし）** に倒れる設計を維持する。
- [x] `evaluator_test.go` を `VerifiedCommandPlan` 返却・最大値・identity ゲートへ全面更新。`02_architecture.md` §5.3 の移行影響（`claude` Medium→High 等）に合わせ期待値を更新。
- [x] `coreutils_consistency_test.go` を新シグネチャへ追従（実行時/ dry-run 一致は Phase 3 で完結）。

**完了条件**: `go test -tags test ./internal/runner/base/risk/` が通る。AC-01/03/04/05/22/63 のテストが緑。

### Step 1-9: `normal_manager` の最大値ゲート＋監査骨組み

**対象ファイル**: [normal_manager.go](../../../internal/runner/resource/normal_manager.go), 対応テスト

- [x] `EvaluateRisk` の戻り値 `VerifiedCommandPlan` を受け取り、`plan.Assessment.Level`／`plan.Assessment.Blocking` で比較ゲート（`effectiveRisk > maxAllowed || Blocking`）。旧リスク比較は本経路に置換する（後方互換は保持しない。§2 Phase 1 注）。
- [x] 拒否時に `runnertypes.ErrCommandSecurityViolation` を返す（既存）。Blocking/Critical 由来も同経路（§4）。
- [x] 監査ロガー注入の受け皿を追加（構造化 `LogRiskProfile` の実配線は Phase 3。Phase 1 では deny が既存 `n.logger.Error` で記録されることのみ確認）。Phase 1 時点は構造化監査（reason code・`max_allowed_risk` 等、AC-56/70）と fd 束縛 exec（AC-64/76）が未了のため、**完全な AC 充足の主張・外部リリースは Phase 3 まで行わない**（中間 main は現行より退行しない＝§2 Phase 1 注の単調改善）。

**完了条件**: `go test -tags test ./internal/runner/resource/ -run Normal` が通る。Phase 1 完了ゲート: `make fmt && make test && make lint`。

### PR-2 作成ポイント: runtime risk evaluation core

**対象ステップ**: 1-4 / 1-5 / 1-6 / 1-7 / 1-8 / 1-9

**推奨タイトル**: `feat(0136): max-of-dimensions risk evaluation core with identity gate and deny`

**レビュー観点**: `EvaluateRisk` の `VerifiedCommandPlan` 返却・全次元最大値の順序非依存（早期 return 撤廃）/ identity ゲートが coreutils 抑制より前段で短絡 / `Classify` 4 区分・F-015 一律 High・systemctl verb 解析（未知オプション→High）/ 旧リスク比較の完全置換（後方互換なし）。**本 PR 時点は path 実行・構造化監査未了のため外部リリース可否ゲート未達**（完全な AC 充足主張は Phase 3 まで。中間 main は現行より退行しない＝§2 Phase 1 注）

> **本 PR を 1 単位とする理由（密結合・ビルド制約）**: ハードな制約は **Step 1-4 が現行 `EvaluateRisk` の呼び出し先 `IsNetworkOperation` を削除し `Classify`（戻り値型も変更）に置換する**点である。これにより `IsNetworkOperation` を呼ぶ現行 `EvaluateRisk` は 1-4 単独ではコンパイル不能となるため、**1-4（Classify）と 1-8（EvaluateRisk）は同一 PR でなければグリーンゲートを満たせない**。加えて評価器テストは `command_analysis`/`coreutils`/F-015 の挙動に依存するため、これらの挙動変更（1-5/1-6/1-7）も評価器テスト更新（1-8）と同 PR に置く（インターフェース＋実装＋テストを分割しない原則）。`EvaluateRisk` の戻り値型変更の波及先は **唯一 `normal_manager`（1-9）** であり、`dryrun_manager` は本タスク前は `risk.Evaluator` を呼ばない（dry-run への評価器配線は PR-5 で初めて行う。§1.3）。したがって PR-2 マージ時に未コンパイルの呼び出し元は残らない。
>
> **レビュー上の注意（高複雑度の内訳）**: 本 PR は大きいため、(a) identity ゲートの短絡（順位 1、F-014 の要）、(b) systemctl の手書き argv パーサ（fail-safe・未知オプション→High）、(c) 8 段優先順位の最大値合成、の 3 点を個別に重点レビューすること（機械的リファクタ 1-5/1-6 とは分けて確認）。

- [x] グリーンゲート（`_context.md` の "Green gate" 参照）がパスしていることを確認した
- [ ] PR を作成した
- [ ] PR がマージされた
- [ ] 次のブランチへ切り替えた（次ステップは新しいブランチで作業する）

## Phase 2 — 実行束縛＋間接実行（F-013/F-014/F-015 一部）

縦切りの到達点: **間接実行のゲート・拒否と fd ベース実行束縛が揃う**。

### Step 2-1: `indirect_execution.go` 新規作成＋評価器への配線

**対象ファイル**: `internal/runner/base/security/indirect_execution.go`（新規）, `internal/runner/base/security/indirect_execution_test.go`（新規）, [evaluator.go](../../../internal/runner/base/risk/evaluator.go)（順位 2 を配線）, [evaluator_test.go](../../../internal/runner/base/risk/evaluator_test.go), 対応する resource-manager の deny テスト

- [ ] `02_architecture.md` §3.3 の形態表を実装。各形態で「実行/ロードされる全成果物を抽出 → allowlist/ハッシュゲート → identity 束縛 → 不能なら拒否（`Blocking`＋reason code）」。
- [ ] ラッパー（`env`/`nice`/`ionice`/`timeout`/`nohup`/`stdbuf`/`setsid`/`time`/`chrt`/`taskset`。**runner 自身が抽出した実コマンドを exec する形態**）: インナーコマンド抽出→再帰評価＋ゲート。COMMAND ありで抽出不能は `ReasonIndirectExecutionRejected`（AC-60/77/84）。**`xargs` はここに含めない**（helper を exec するのは runner ではなく xargs 子プロセスのため、下記 find/xargs の子プロセス実行ルールで扱う）。
- [ ] 特権昇格トークン（`sudo`/`su`/`doas`）が独立トークンで出現 → 抽出可否によらず Critical（AC-59）。
- [ ] ラッパー供給環境変数を既存 [environment_validation.go](../../../internal/runner/base/security/environment_validation.go) で検証。ローダ制御変数を拒否（AC-80）: Linux 系 `LD_PRELOAD`/`LD_LIBRARY_PATH`/`LD_AUDIT`、および **macOS（dyld）系 `DYLD_INSERT_LIBRARIES`/`DYLD_LIBRARY_PATH`/`DYLD_FALLBACK_LIBRARY_PATH`** 等の `DYLD_*`（macOS でのライブラリインジェクション対策。OS によらず拒否リストに含める）。
- [ ] `env -S`（split-string）分割後 argv 解釈。`sudo` 等を Critical、解釈不能は拒否（AC-81）。
- [ ] シェル/インタプリタの inline コード実行 → High 下限（AC-61。文字列内 sudo の Critical 化は保証しない＝限界明記）。**inline 実行フラグはコマンド種別で区別する**: シェル（`bash`/`sh`/`zsh` 等）は `-c` のみ（`-e` は errexit の真偽オプションで inline コードではない。`bash -e script.sh` を inline 文字列と誤解析しない）。インタプリタ（`node`/`ruby`/`perl` 等）は `-e`（eval）と `-c` を inline コードとして扱う。なお F-015 によりシェル/インタプリタは引数によらず High のため、この区別は inline 文字列か否か（＝内側スクリプトをファイル成果物としてゲートできるか）の判定に用いる。
- [ ] 実行解決すり替え（`env PATH=…`）→ 検証済み絶対パスで実行 or 拒否（AC-79）。
- [ ] `find`/`xargs` 実行アクション → 対象を破壊判定＋allowlist/ハッシュゲート＋検証済み絶対パス実行。fd 束縛不能なら拒否（AC-62/82。残存制約は §5.2）。
- [ ] サービス管理（`service <name> <action>`）→ `/etc/init.d/<name>` を成果物として検証＋ゲート＋identity 束縛、不能なら拒否（AC-75/82）。
- [ ] 直接スクリプト/shebang（`./deploy.sh`/`#!/usr/bin/env python`）→ インタプリタ連鎖を評価＋ゲート＋identity 束縛（AC-86）。
- [ ] コマンド実行オプション（`rsync -e`/`--rsh`、`tar --to-command`/checkpoint アクション）→ helper をゲート or 拒否（AC-87）。
- [ ] 動的ローダ直接起動（`ld-linux*.so --preload`/`--library-path`/`--inhibit-cache` 等）→ EXECUTABLE・preload・探索パス配下を load-time ゲート、不能なら拒否（AC-83）。
- [ ] パッケージスクリプトランナー（`npm run`/`npx`/`yarn <script>`/`pnpm run`）→ High（AC-85）。
- [ ] ラッパー無コマンド起動（`env` 単体）→ 抽出不能と区別し Medium 以上（AC-78）。
- [ ] §3.3 表注のとおり、列挙したフラグは代表例。同一クラスの他オプションにも一般原則を適用する実装（網羅列挙はスコープ外）。
- [ ] **評価器への配線（dead code 化の防止）**: Step 1-8 で順位 2（間接実行）を未実装としていた箇所を、本 Step の検出器を呼び出すよう `EvaluateRisk` に配線する（§6.1 順位 2 が実効リスク・`Blocking` に反映される）。検出器を追加するだけで `EvaluateRisk` から呼ばれない状態を残さない。
- [ ] **エンドツーエンドの risk/deny テスト**: `evaluator_test.go`／resource-manager テストで、`env sudo`／`bash -c`／`find -exec`／ローダ起動 等が **`EvaluateRisk` 経由で** Critical/Blocking/High に評価され実行拒否されることを検証する（security パッケージ単体の `-run Indirect` だけで終えない。間接実行が実効リスク計算に確かに参加することを示す）。

**完了条件**: `go test -tags test ./internal/runner/base/security/ -run Indirect` が緑、かつ `go test -tags test ./internal/runner/base/risk/ ./internal/runner/resource/` の間接実行 e2e ケースが緑（`EvaluateRisk` が検出器を呼ぶことをテストで保証）。攻撃者視点ケース（AC-71）を含む。

### PR-3 作成ポイント: indirect execution gating

**対象ステップ**: 2-1

**推奨タイトル**: `feat(0136): indirect execution detection and artifact gating`

**レビュー観点**: 抽出不能ラッパーの拒否（High に倒さず `ReasonIndirectExecutionRejected`）/ ローダ制御 env（`LD_*`/`DYLD_*`）・`env -S`・shebang・service init スクリプト・find/xargs 子プロセスの各形態ゲート / 攻撃者視点テスト（AC-71）の網羅 / 未知形態は安全側（一般原則＋backstop）/ **検出器が `EvaluateRisk` 順位 2 に確実に配線され dead code でない**（e2e で `env sudo`/`bash -c`/`find -exec` が評価経由で拒否される）

- [ ] グリーンゲート（`_context.md` の "Green gate" 参照）がパスしていることを確認した
- [ ] PR を作成した
- [ ] PR がマージされた
- [ ] 次のブランチへ切り替えた（次ステップは新しいブランチで作業する）

### Step 2-2: fd ベース実行と `VerifiedCommandPlan` 契約

**対象ファイル**: [executor.go](../../../internal/runner/base/executor/executor.go), [interface.go](../../../internal/runner/base/executor/interface.go), 対応テスト, fd 実行ヘルパ（`internal/runner/base/executor/fdexec_linux.go`＋非 Linux スタブ `fdexec_other.go`・新規）

- [ ] `EvaluateRisk`（評価器）が検証時に開いた fd を `VerifiedIdentity.FD`（`*VerifiedFD`。Step 1-1）に格納する経路を実装（パス解決・検証・open を評価器が一度だけ。§3.6.2）。
- [ ] executor の実行入口を `VerifiedCommandPlan` 受領へ変更。fd 束縛実行は `//go:build linux` のヘルパで、検証済み fd（`Identity.FD.Fd()` で得た生 fd）を `cmd.ExtraFiles` で子へ継承させ（CLOEXEC 解除）、`os/exec` の `cmd.Path` に `/proc/self/fd/<childfd>`（`childfd = 3 + ExtraFiles index`）を設定して exec する（§1.3。標準 fork+exec を利用、raw syscall/`unsafe` 不要）。第一候補。不能環境は read-only ステージング、双方不能なら評価段階で Blocking 済み（executor はセキュリティ拒否判定を持たない。§3.6.2）。
- [ ] **再ハッシュのみの path exec（rehash-then-path-exec）を実装しない**（§3.6.2 / AC-76）。
- [ ] **検証済み fd は常に保持する**（評価時の open 成功＝fd 取得。fd を保持できない＝open 失敗なら deny）。fd 束縛 exec も **ステージングも、この保持中の検証済み fd を源泉**とする（ステージングは fd から複製＝`/proc/self/fd/<fd>` 経由のコピー等。**パスから再 open/再コピーしない**）。これにより、fd 実行不能環境でも allow 判定後にパス再オープンする TOCTOU 窓を作らない（§3.6.2 / AC-76）。源泉となる検証済み fd を保持できない場合は deny。
- [ ] 束縛可否の判定は **副作用なし**で行う（§3.6.2）: 評価段階では「検証済み fd を保持できているか」「ステージング先（書込不能領域）が**利用可能か**」のみを確認し、**実際のステージング複製（fd からのファイル書き込み）は normal の exec 直前にのみ行う**。dry-run は同じ副作用なし可否チェックで allow/deny を決め、ステージング書き込みは行わない（read-only 維持。AC-30/39 の normal/dry-run 整合を保ち、normal が staging で許可する実体を dry-run が誤って deny しない）。
- [ ] fd 所有権と close 機構（型は Step 1-1 で確定した `VerifiedFD`＝`*VerifiedFD` を `VerifiedIdentity`/各 `ExecutedArtifact` が保持）: `VerifiedCommandPlan` に `Close() error`（保持する全 `VerifiedFD` を集約 close、`errors.Join`）を実装する。close 呼び出し箇所を明示: (a) 許可 plan は子プロセス起動（`cmd.Start`、fd は `ExtraFiles` で子へ複製済み）成功後に親側の検証済み fd を close、(a') **`cmd.Start` がエラーを返した場合**（子が走らず親が全 fd を保持したまま）は executor が `defer plan.Close()`／エラー時クリーンアップで親側 fd を必ず close（長時間 runner で exec 失敗が続いても fd を漏らさない）、(b) 拒否 plan・dry-run preview・exec されない副成果物は ResourceManager が監査出力後に `plan.Close()`。`EvaluateRisk` 内で fd を開いた後にエラーで早期 return する場合も、その時点までに開いた fd を defer で close する。
- [ ] 元 argv/env の直接 exec を禁止（型契約＋コードレビュー観点）。
- [ ] 非対応 OS（`//go:build !linux`）の `fdexec_other.go` は fd 実行不能を返すスタブとし、ステージング/拒否のみ。

**完了条件**:
- Linux ビルドの実コンパイル: `go build -tags test ./internal/runner/base/executor/`（`//go:build linux` の `fdexec_linux.go` が型/シグネチャ込みでコンパイルされる。CI は linux/amd64 を前提・要確認）。
- 非 Linux スタブのクロスコンパイル: `GOOS=darwin go build -tags test ./internal/runner/base/executor/`（`fdexec_other.go` がコンパイルされる）。
- fd 実行・ステージング双方の単体テスト、および **拒否/preview plan・および `cmd.Start` 失敗時の許可 plan が fd を漏らさない**ことを検証するテスト（`/proc/self/fd` のカウント or fake fd で `Close` 呼び出しを確認。Start 失敗パスを含む）が緑。

### Step 2-3: `group_executor` の二重解決廃止

**対象ファイル**: [group_executor.go](../../../internal/runner/group_executor.go), [group_executor_test.go](../../../internal/runner/group_executor_test.go)

- [ ] `executeCommandInGroup`（:520-528）の実行直前 `ResolvePath`＋`cmd.ExpandedCmd` 上書きを **廃止**（TOCTOU 二重解決の解消。AC-64/76）。
- [ ] `group_executor` は `ResourceManager` 経由で実行を委譲する呼び出し側に徹する（`EvaluateRisk`・risk 比較・`LogRiskProfile` は manager が所有。§2.3。`EvaluateRisk` を呼ぶのは `normal_manager` で Step 1-9 に計上済み）。group_executor 自身は評価・ゲート・監査を持たない。
- [ ] ラッパー/ローダ成果物の検証連携は plan 経由で行う（評価器が生成した `VerifiedCommandPlan.Artifacts` を manager→executor が実行・監査へ引き渡す。§3.3 の連鎖）。

**完了条件**: `go test -tags test ./internal/runner/ -run GroupExecutor` が緑。Phase 2 完了ゲート: `make fmt && make test && make lint`。

### PR-4 作成ポイント: fd-bound execution and identity binding

**対象ステップ**: 2-2 / 2-3

**推奨タイトル**: `feat(0136): fd-bound execution via /proc/self/fd and TOCTOU-safe identity binding`

**レビュー観点**: `VerifiedCommandPlan` のみ exec（元 argv/env 直接 exec 禁止）/ `/proc/self/fd` 経由 fd 束縛・staging は保持 fd 源泉・再ハッシュ path exec 不可 / fd 所有権と close（`cmd.Start` 失敗パス含むリーク無し）/ `group_executor` の二重解決廃止（manager 委譲）。**高リスク（TOCTOU・実行系）の隔離 PR**として重点レビュー

- [ ] グリーンゲート（`_context.md` の "Green gate" 参照）がパスしていることを確認した
- [ ] PR を作成した
- [ ] PR がマージされた
- [ ] 次のブランチへ切り替えた（次ステップは新しいブランチで作業する）

## Phase 3 — dry-run preview＋監査拡張（F-003/F-006/F-009）

縦切りの到達点: **normal deny ＋ 監査出力 ＋ dry-run preview が揃う（外部リリース可否ゲート）**。

### Step 3-1: `LogRiskProfile` をパラメータ構造体へ拡張

**対象ファイル**: [logger.go](../../../internal/runner/base/audit/logger.go), [logger_test.go](../../../internal/runner/base/audit/logger_test.go)

- [ ] `LogRiskProfile(ctx, entry risktypes.RiskAuditEntry)` へシグネチャ変更（`02_architecture.md` §3.2）。相関フィールド（`resolved_path`/`content_hash`/レコード識別/`max_allowed_risk`/`decision`/`reason_codes`/`risk_factors`）を出力。
- [ ] 取得不能値は在/不在を明示（`*string` nil = 省略）。値フィールドにセンチネル文字列を入れない。固定マーカー（`n/a` 等）はログ出力境界のみ（AC-56）。
- [ ] decision に基づく重大度下限（deny は Warn 以上）を、リスクレベル対応ログレベル（AC-13）と独立に適用（AC-70）。
- [ ] 引数マスキング（既存 redaction 機構と整合。AC-57）。
- [ ] `logger_test.go` で deny 出力・相関フィールド・在不在表現・重大度下限・連鎖カバレッジを検証。

**完了条件**: `go test -tags test ./internal/runner/base/audit/` が緑。

### Step 3-2: 監査ロガーを ResourceManager へ配線

**対象ファイル**: [default_manager.go](../../../internal/runner/resource/default_manager.go), [normal_manager.go](../../../internal/runner/resource/normal_manager.go), [runner.go](../../../internal/runner/runner.go), 対応テスト

- [ ] `Config` に `AuditLogger *audit.Logger` を追加し、`normal_manager`・`dryrun_manager` 双方へ注入（§3.6.3）。注入経路（Config フィールド・コンストラクタ）を確定。
- [ ] `normal_manager` が判定後に `LogRiskProfile`（allow/deny・`decision`・`max_allowed_risk`）を出力。`error` 返却経路（§4(3)）でも中止前に最小限の監査エントリ（`decision=deny`+`ErrorClass`+path）を出力。
- [ ] `runner.go` で生成済みの `audit.Logger` を `Config.AuditLogger` 経由で渡す。

**完了条件**: `go test -tags test ./internal/runner/resource/ -run 'Normal|Audit'` が緑。AC-11/56/70 のテストが緑。

### Step 3-3: dry-run の allow/deny preview

**対象ファイル**: [dryrun_manager.go](../../../internal/runner/resource/dryrun_manager.go), [default_manager.go](../../../internal/runner/resource/default_manager.go), [formatter.go](../../../internal/runner/resource/formatter.go), 対応テスト

- [ ] `DryRunResourceManager` に `risk.Evaluator` を注入（`NewDryRunResourceManager` 系のシグネチャ拡張）。
- [ ] 同一評価器（read-only 解析）で実効リスク＋`risk_level` 比較を行い、**allow / deny の 2 区分**を preview 出力（AC-30/31/58）。`unknown` 区分は設けない。
- [ ] バイナリ解析シグナル由来 High/Medium を dry-run 表示へ反映（解析利用可能時。AC-32）。
- [ ] 失敗の 2 系統（ポリシー拒否 = deny 予告 / ハードエラー = error 返却）を実装（AC-18/33）。`(3)` 予期しない内部エラーは dry-run でも `error`（§4）。
- [ ] 検証不能 deny（解析/検証無効環境）に専用終了コードと CI オプションを付与（AC-46/58）。
- [ ] dry-run 監査ログ出力（dry-run 旨を含む）。
- [ ] `coreutils_consistency_test.go`（risk パッケージ）を拡張し、実行時/dry-run の実効リスク一致を検証（AC-20/27/28/39）。
- [ ] 本 PR 完了をもって fd 束縛 exec（PR-4）＋構造化監査（本 PR）が揃い、AC-64/76（TOCTOU）・AC-56/70（監査）を充足する。ここで初めて完全な AC 充足を主張でき、外部リリース可否ゲートに到達する（フラグ等の切替は不要＝後方互換を保持しないため新経路が常時有効）。

**完了条件**: `go test -tags test ./internal/runner/resource/ -run DryRun` が緑。Phase 3 完了ゲート: `make fmt && make test && make lint`。**外部リリース可否ゲート達成**。

### PR-5 作成ポイント: audit wiring and dry-run preview

**対象ステップ**: 3-1 / 3-2 / 3-3

**推奨タイトル**: `feat(0136): structured risk audit logging and dry-run allow/deny preview`

**レビュー観点**: `LogRiskProfile` の `RiskAuditEntry` 化（在/不在のセンチネル排除）/ ResourceManager（normal・dry-run 双方）への監査配線・deny 重大度下限 / dry-run と normal の判定一致・検証不能 deny の終了コード / **本 PR 完了で fd 束縛 exec＋構造化監査が揃い AC 充足＝外部リリース可否ゲート達成**

- [ ] グリーンゲート（`_context.md` の "Green gate" 参照）がパスしていることを確認した
- [ ] PR を作成した
- [ ] PR がマージされた
- [ ] 次のブランチへ切り替えた（次ステップは新しいブランチで作業する）

## Phase 4 — ドキュメント（F-004/F-010、AC-19、§5.3 例外）

### Step 4-1: `security-architecture.{md,ja.md}` の 2 例外反映

**対象ファイル**: [security-architecture.md](../../../docs/dev/architecture_design/security-architecture.md), `security-architecture.ja.md`

- [ ] `:1039` 付近の "Graceful degradation when security features are unavailable" を「解析/検証が無効な場合は実行を拒否する（dry-run は可）」へ改訂（§5.3 例外 1。F-005/AC-51）。
- [ ] `:417` 付近の旧シグネチャ `EvaluateRisk(cmd *runnertypes.Command) (runnertypes.RiskLevel, error)` を目標シグネチャ `EvaluateRisk(cmd *runnertypes.RuntimeCommand) (risktypes.VerifiedCommandPlan, error)` へ更新（§5.3 例外 2。引数型の陳腐化も是正）。
- [ ] 解析無効時に Low 通過/実行継続を期待する既存テストの有無を洗い出し、あれば常時拒否へ更新（§5.3 例外 1(3)）。

**完了条件**: `rg -n "Graceful degradation when security features are unavailable" docs/dev/architecture_design/security-architecture.md` が 0 件。`rg -n "cmd \*runnertypes\.Command\) \(runnertypes\.RiskLevel" docs/dev/architecture_design/security-architecture.md` が 0 件。

### Step 4-2: `risk_assessment.{ja.md,md}` をユーザー向け整合

**対象ファイル**: [risk_assessment.ja.md](../../../docs/user/risk_assessment.ja.md), `risk_assessment.md`

- [ ] §3.1 の表（[risk_assessment.ja.md:71](../../../docs/user/risk_assessment.ja.md#L71) の `| \`systemctl\`/\`apt\`/\`dpkg\` 等のシステム変更コマンド | \`medium\` |` 行）を書き換える。この 1 行が **AC-34（dpkg 削除）と AC-37（systemctl レベル是正）の双方**に関わるため、同時に改訂する: `dpkg` を除去し、`systemctl` 変更系=High / 読み取り専用=Medium 下限・`service`=High・`apt` install/remove=Medium を反映した記述へ分解する（旧 `systemctl … medium` の単一行を残さない）。`.md` 版の対応行も同様に修正。
- [ ] ネットワーク系（`curl`/`wget`/`ssh`）= medium、シェル/インタプリタ/ビルドランナー（`bash`/`python`/`node`/`make`）= high を説明（AC-35）。
- [ ] coreutils 単一バイナリ分類（Low/Medium/High 3 区分）を説明（AC-36）。
- [ ] 「最終リスクはすべての因子の最大値」をプロファイル要因含む最大値へ整合（AC-38）。
- [ ] §3.3 の挙動表を F-005 の deny/error 2 系統へ改訂（AC-17 のユーザー向け部分）。
- [ ] §5 設定例を修正後実装で動作する例へ（恒久拒否される例を残さない。AC-50）。
- [ ] 脅威モデル（ブロックリスト方式・allowlist/ハッシュ固定前提・basename 限界・output_file 対象外・root 判定系との関係）を明記（AC-66/67/29）。
- [ ] 移行ノートを追記（`claude`/`systemctl`/`service`/絶対パス破壊/インタプリタ/ビルド/`unknown` 設定エラー化/解析無効/ラッパー。AC-19）。

**完了条件**: 下記 AC 検証表の static rg がすべて期待どおり。

### PR-6 作成ポイント: documentation alignment (security/user docs)

**対象ステップ**: 4-1 / 4-2

**推奨タイトル**: `docs(0136): align security and user docs with enforced runtime risk evaluation`

**レビュー観点**: `security-architecture` の 2 例外（fail-closed 反転・シグネチャ更新）反映 / `risk_assessment` ユーザー文書の整合（dpkg 削除・systemctl レベル是正・移行ノート AC-19）/ 設定例が修正後実装で動作。**Step 4-3 は外部依存（PR #724）のため本 PR に含めず PR-7 に分離**。**マージ順序の依存: PR-5 完了後（実装が live）にマージする** — 本 PR は新しい強制挙動（`unknown` 拒否・systemctl/service レベル・解析無効 fail-closed・dry-run preview）を現在形で記述するため、PR-5 より前に main へ入るとユーザー文書が未実装挙動を既成事実として誤記することになる（ビルドは通るが内容が main と不整合）。

- [ ] グリーンゲート（`_context.md` の "Green gate" 参照）がパスしていることを確認した
- [ ] PR を作成した
- [ ] PR がマージされた
- [ ] 次のブランチへ切り替えた（次ステップは新しいブランチで作業する）

### Step 4-3: `command-risk-evaluation.{ja,md}`（PR #724 マージ後）

**対象ファイル**: `docs/dev/architecture_design/command-risk-evaluation.ja.md` / `.md`（**PR #724 マージ後に存在**）

- [ ] PR #724 マージ後、AC-15（`risk_level` スコープ明記）・AC-17（deny/error/High 許可の 3 区別）・AC-18（dry-run 失敗時挙動）・AC-29（重複定義の優先順位・root 判定系との関係）を反映。
- [ ] AC-66/67 の脅威モデルを開発者向けにも明記。

**完了条件（PR #724 マージ後）**: AC 検証表の該当 static rg。**依存**: PR #724 未マージのため、本 Step はマージ完了まで未着手（`02_architecture.md` §3.4 注・付録）。Phase 1〜3 の完了はこの Step に依存しない。

### PR-7 作成ポイント: command-risk-evaluation doc (depends on PR #724)

**対象ステップ**: 4-3

**推奨タイトル**: `docs(0136): align command-risk-evaluation guide with enforced runtime risk evaluation`

**レビュー観点**: AC-15/17/18/29 の反映（`risk_level` スコープ・deny/error/High 許可の区別・dry-run 失敗時挙動・重複定義の優先順位）/ AC-66/67 脅威モデルの開発者向け明記。**前提**: PR #724 がマージ済みで対象ファイルが存在すること（未マージなら本 PR は着手しない）

- [ ] グリーンゲート（`_context.md` の "Green gate" 参照）がパスしていることを確認した
- [ ] PR を作成した
- [ ] PR がマージされた
- [ ] 次のブランチへ切り替えた（次ステップは新しいブランチで作業する）

---

## 3. 実装順序と PR 構成

### 3.1 フェーズ・マイルストーン

| マイルストーン | 対象 Step | 成果物 |
|---|---|---|
| M1: 評価コア＋normal deny | 1-1〜1-9 | 最大値モデル・identity ゲート・`unknown` 拒否・normal の deny が機能 |
| M2: 実行束縛＋間接実行 | 2-1〜2-3 | `indirect_execution.go`・fd 束縛実行・二重解決廃止 |
| M3: 監査＋dry-run preview（リリース可否ゲート） | 3-1〜3-3 | `LogRiskProfile` 拡張・配線・dry-run allow/deny preview |
| M4: ドキュメント | 4-1〜4-3 | security-architecture/risk_assessment 整合、移行ノート、（PR #724 後）command-risk-evaluation |

### 3.2 PR 構成

各 PR の詳細（推奨タイトル・レビュー観点・チェックリスト）は §2 の各グループ末尾の `### PR-N 作成ポイント` を参照。各 PR は単独でグリーンゲート（`_context.md` の "Green gate"）を満たし、依存 PR より後にマージする順序とする（本タスクは `cmd/` 変更を持たず、変更は `internal/` とドキュメントのみ）。

| PR | 対象ステップ | 依存 PR / 提供物 | 主な変更内容 |
|---|---|---|---|
| PR-1 | 1-1 / 1-2 / 1-3 | なし。提供: `risktypes`（以降の全 PR が依存） | 共有型 `risktypes` 新設、`ParseRiskLevel("unknown")` エラー化、`spec.go` コメント修正（基盤。既存挙動への波及なし） |
| PR-2 | 1-4 / 1-5 / 1-6 / 1-7 / 1-8 / 1-9 | PR-1。提供: `VerifiedCommandPlan`/`EvaluateRisk` | 評価コア: `Classify` 4 区分・`command_analysis`/`coreutils`・F-015・`EvaluateRisk` 最大値＋identity ゲート・`normal_manager` deny（`IsNetworkOperation`→`Classify` 削除が `EvaluateRisk` のコンパイルに直結する密結合の 1 単位。旧比較は置換＝後方互換なし） |
| PR-3 | 2-1 | PR-2。提供: 間接実行解決（`Artifacts` を生成・`EvaluateRisk` 順位 2 へ配線） | 間接実行の検出・抽出・ゲート・拒否（`indirect_execution.go` 新規）＋評価器配線＋e2e deny テスト |
| PR-4 | 2-2 / 2-3 | PR-3（executor が `Artifacts` 連鎖を実行/監査）・PR-2 | fd 束縛実行（`/proc/self/fd`）・`VerifiedCommandPlan` 契約・`group_executor` 二重解決廃止（高リスク隔離 PR） |
| PR-5 | 3-1 / 3-2 / 3-3 | PR-4（fd 束縛 exec が揃う）・PR-2 | 構造化監査 `LogRiskProfile`・ResourceManager 配線・dry-run allow/deny preview。**本 PR 完了で AC 充足＝外部リリース可否ゲート達成** |
| PR-6 | 4-1 / 4-2 | **PR-5 完了後にマージ**（新挙動を現在形で記述するため実装が live になってから。ビルド非依存だが内容整合のため順序依存） | security-architecture/risk_assessment 文書整合・移行ノート |
| PR-7 | 4-3 | **外部 PR #724 マージ後**（対象ファイルが存在すること） | command-risk-evaluation ガイドの整合（AC-15/17/18/29・脅威モデル） |

---

## 4. テスト戦略

`02_architecture.md` §7 を踏襲する。

- **ユニットテスト**: 各 AC に最低 1 つ。絶対パス入力（AC-06/07/08/44）、全次元最大値・順序非依存（AC-63）、バイナリ解析 4 区分（AC-45）と reason code 網羅（AC-69）、coreutils 優先（AC-72）・未知サブコマンド High（AC-68）、`ParseRiskLevel("unknown")`（AC-24）。
- **バイパス系（攻撃者視点）テスト**（AC-71）: `env sudo`/`env rm`/`env PATH=`/`env LD_PRELOAD=`/`env -S`、`bash -c`、`find -exec`/`-execdir`、shebang、`rsync -e`/`tar --to-command`、ld-linux。正常系の直接呼び出しで終えない。
- **整合性テスト**: 実行時/dry-run の実効リスク一致（AC-20/27/28/39）。`risk/coreutils_consistency_test.go` を維持・拡張。
- **監査テスト**: deny 出力・相関フィールド・在不在表現・重大度下限・連鎖（AC-11/56/57/70）。
- **後方互換テスト**: basename 入力の検出維持（AC-10）。
- **fd 束縛テスト**: `/proc/self/fd` 実行経路とステージング経路の双方、双方不能時の Blocking deny（AC-76）。
- **品質ゲート**: 各フェーズで `make fmt`/`make test`/`make lint`（AC-21）。

### テストヘルパー方針（`docs/dev/developer_guide/test_organization.md` 準拠）

- `risktypes` のテスト用ファクトリ（plan/identity 構築）は、公開 API のみ使用するため必要時 `internal/runner/base/risktypes/testutil/helpers.go`（`package risktypestestutil`）に置く。
- `risk`/`security` パッケージ内で非公開 API を使うヘルパーは各パッケージの `test_helpers.go`（`//go:build test`）。既存 [network_analyzer_test_helpers.go](../../../internal/runner/base/security/network_analyzer_test_helpers.go) / [test_helpers.go](../../../internal/runner/base/security/test_helpers.go) を再利用・拡張する（新規ファイルは必要時のみ）。
- fd 束縛実行のテストでテスト専用バイナリを書込不能領域へ配置する等の OS 依存セットアップは、`security`-linter（gosec）指摘が出る場合のみ最小スコープ `//nolint` をテスト/ヘルパー双方に付す（`testutil/` の `//go:build test` ファイルにも適用。`_test.go` 限定の免除は効かない）。

---

## 5. リスク管理

| リスク | 影響 | 緩和策 |
|---|---|---|
| `/proc/self/fd` 実行が一部環境で不可（`/proc` なし等） | fd 束縛が成立せず実行不能 | read-only ステージングへフォールバック、双方不能なら Blocking deny（§3.6.2）。実装環境可否は Step 2-2 ゲートで確認 |
| 間接実行の網羅不能性 | 未知ベクトルの素通り | 一般原則（全成果物ゲート＋identity 束縛、不能なら拒否、未知は安全側）＋ allowlist/ハッシュ固定 backstop（§3.6.4） |
| 移行影響の広さ（High 化多数） | 既存設定の拒否 | 移行ノート（AC-19）で明記、dry-run で事前確認可能に |
| PR #724 未マージ依存 | command-risk-evaluation 文書を更新できない | Phase 4 の Step 4-3 をマージ後着手に分離。Phase 1〜3 はこの依存を持たない |
| 共有 DTO の循環依存 | コンパイル不能 | `risktypes` を最下位中立パッケージに分離（§1.4） |

---

## 6. 実装チェックリスト（PR 単位）

- [ ] PR-1 マージ済み（対象ステップ: 1-1 / 1-2 / 1-3）— 共有型基盤・config、グリーンゲート緑
- [ ] PR-2 マージ済み（対象ステップ: 1-4 / 1-5 / 1-6 / 1-7 / 1-8 / 1-9）— 評価コア＋normal deny、グリーンゲート緑
- [ ] PR-3 マージ済み（対象ステップ: 2-1）— 間接実行ゲート、グリーンゲート緑
- [ ] PR-4 マージ済み（対象ステップ: 2-2 / 2-3）— fd 束縛実行・二重解決廃止、グリーンゲート緑
- [ ] PR-5 マージ済み（対象ステップ: 3-1 / 3-2 / 3-3）— 監査＋dry-run preview、AC 充足（**外部リリース可否ゲート達成**）
- [ ] PR-6 マージ済み（対象ステップ: 4-1 / 4-2）— security/user 文書整合・移行ノート
- [ ] PR-7 マージ済み（対象ステップ: 4-3）— command-risk-evaluation 整合（**PR #724 マージ後**）
- [ ] 全 AC が §7 の検証表で `test` または `static` により充足
- [ ] §8 クロス検索チェックリスト完了

---

## 7. 受入基準検証（Acceptance Criteria Verification）

各 AC を `test`（実行可能テスト）/ `static`（rg/grep/コンパイル）/ `manual`（PR 観察）で検証する。テスト名は計画上の配置（実装時に確定）。すべての AC は最低 1 つの `test` または `static` を持つ。

| AC | 種別 | 検証場所 / コマンド | 期待 |
|----|------|--------------------|------|
| AC-01 | test | `internal/runner/base/risk/evaluator_test.go::TestEvaluateRisk_ProfileMaxClaude` | `claude` の実効リスク = High |
| AC-02 | test | `internal/runner/base/risk/evaluator_test.go::TestEvaluateRisk_ProfileFactorFloor` | 任意要因 High 宣言コマンドが High 以上 |
| AC-03 | test | `internal/runner/base/risk/evaluator_test.go::TestEvaluateRisk_ProfileSafeSideOnly` | プロファイルが他ステップ結果を下げない |
| AC-04 | test | `internal/runner/base/risk/evaluator_test.go::TestEvaluateRisk_ProfileStepNoChangeWithoutProfile` | プロファイル無しコマンドはプロファイル反映で不変 |
| AC-05 | test | `internal/runner/base/risk/evaluator_test.go::TestEvaluateRisk_SymlinkChainMaxAndFailSafe` | リンク連鎖の最大値、解決失敗は F-012 優先 |
| AC-06 | test | `internal/runner/base/security/command_analysis_test.go::TestIsDestructive_AbsolutePath` | `/usr/bin/rm` が破壊的と分類 |
| AC-07 | test | `internal/runner/base/security/command_analysis_test.go::TestIsSystemModification_AbsolutePath` | `/usr/sbin/systemctl restart` が分類 |
| AC-08 | test | `internal/runner/base/risk/evaluator_test.go::TestEvaluateRisk_AbsoluteRmRfHigh` | `/usr/bin/rm -rf /tmp/x` = High |
| AC-09 | test | `internal/runner/base/security/command_analysis_test.go::TestIsDestructive_NoSubstringMatch` | `lsrm`/`systemctl-helper` 非該当 |
| AC-10 | test | `internal/runner/base/security/command_analysis_test.go::TestIsDestructive_BasenameBackwardCompat` | basename `rm` が引き続き検出 |
| AC-11 | test | `internal/runner/resource/normal_manager_test.go::TestExecute_EmitsRiskProfileAudit` ＋ `internal/runner/base/audit/logger_test.go::TestLogRiskProfile_Chain` | `command_risk_profile` 出力・連鎖成果物追跡可能 |
| AC-12 | test | `internal/runner/base/audit/logger_test.go::TestLogRiskProfile_ReasonCodesAndFactors` | reason code＋risk_factors を含む |
| AC-13 | test | `internal/runner/base/audit/logger_test.go::TestLogRiskProfile_LogLevelByRisk` | Critical→Error/High→Warn/Medium→Info/他→Debug |
| AC-14 | test | `internal/runner/resource/normal_manager_test.go::TestExecute_RejectedCommandAuditable` | 拒否コマンドも監査可能 |
| AC-15 | static | （PR #724 マージ後）`rg -n "command(-| )level.*template" docs/dev/architecture_design/command-risk-evaluation.ja.md` | scope 記述あり（グループ/グローバル非対応を明記） |
| AC-16 | static | `rg -n "inherit from global default\|nil=inherit default" internal/runner/base/runnertypes/spec.go` | 0 件 |
| AC-17 | test/static | static: `rg -n "deny\|error\|high" docs/user/risk_assessment.ja.md`（§3.3 改訂確認）＋ test: `internal/runner/base/risk/evaluator_test.go::TestEvaluateRisk_DenyVsErrorClassification` | deny/error/High 許可の 3 区別が文書＋挙動で成立 |
| AC-18 | static | （PR #724 マージ後）`rg -n "High として表示継続\|deny 予告" docs/dev/architecture_design/command-risk-evaluation.ja.md` | dry-run 失敗時挙動を正確に記述 |
| AC-19 | static | `rg -n "移行ノート\|migration" docs/user/risk_assessment.ja.md` | 代表ケース・unknown エラー化・解析無効を記載 |
| AC-20 | test | `internal/runner/base/risk/coreutils_consistency_test.go::TestConsistency_DestructiveAbsolutePath` | 実行時/dry-run が同じ High |
| AC-21 | static | `make test && make lint` | 成功（既存無関係指摘除く） |
| AC-22 | test | `internal/runner/base/risk/evaluator_test.go::TestEvaluateRisk_SystemctlChangeAndServiceHigh` | systemctl 変更系・service 全アクション High |
| AC-23 | test | AC-06/07/08/44 の各テスト（`TestIsDestructive_AbsolutePath` / `TestIsSystemModification_AbsolutePath` / `TestEvaluateRisk_AbsoluteRmRfHigh` / `TestEvaluateRisk_NoProfileAbsolutePath`）を正準証拠とする | 破壊/システム変更/プロファイル無しの各表ケースが `/usr/...` 絶対パス入力で実行時の振る舞いを検証 |
| AC-24 | test | `internal/runner/base/runnertypes/config_test.go::TestParseRiskLevel_UnknownError` | `ParseRiskLevel("unknown")` がエラー |
| AC-25 | test | `internal/runner/base/runnertypes/config_test.go::TestParseRiskLevel_UnknownConfigRejected` | `risk_level="unknown"` 設定がエラー通知 |
| AC-26 | test | `internal/runner/base/runnertypes/config_test.go::TestParseRiskLevel_ValidValues` | low/medium/high/省略/空 が従来どおり |
| AC-27 | test | `internal/runner/base/risk/coreutils_consistency_test.go::TestConsistency_Systemctl` | systemctl で実行時/dry-run 一致 |
| AC-28 | test | `internal/runner/base/risk/coreutils_consistency_test.go::TestConsistency_RmAllForms` | basename/絶対/coreutils で双方 High |
| AC-29 | static | `rg -n "完全一致\|部分一致\|root" docs/user/risk_assessment.ja.md`（＋PR#724 後 command-risk-evaluation） | 優先順位・root 判定系との関係を明記 |
| AC-30 | test | `internal/runner/resource/dryrun_manager_test.go::TestDryRun_EffectiveRiskShown` | dry-run に実効リスク含む |
| AC-31 | test | `internal/runner/resource/dryrun_manager_test.go::TestDryRun_AllowDenyPreview` | risk_level 比較の許可/拒否予告含む |
| AC-32 | test | `internal/runner/resource/dryrun_manager_test.go::TestDryRun_BinaryAnalysisReflected` | 解析レコード利用可能時に解析シグナル High/Medium が dry-run 実効リスクへ反映（解析/検証不能環境は AC-46/51 の deny 予告で対象外） |
| AC-33 | test | `internal/runner/resource/dryrun_manager_test.go::TestDryRun_DenyVsHardError` | deny 予告 / error の 2 系統 |
| AC-34 | static | `rg -n "dpkg" docs/user/risk_assessment.ja.md docs/user/risk_assessment.md` | 0 件 |
| AC-35 | static | `rg -n "bash.*high\|python.*high\|make.*high\|curl.*medium" docs/user/risk_assessment.ja.md` | 該当記述あり |
| AC-36 | static | `rg -n "coreutils" docs/user/risk_assessment.ja.md` | Low/Medium/High 3 区分の説明あり |
| AC-37 | static | `rg -n "systemctl[^\|]*medium\|システム変更コマンド[^\|]*medium" docs/user/risk_assessment.ja.md` | 0 件（旧 §3.1 :71 行の `systemctl/apt/dpkg … medium`〔システム変更コマンド=medium〕の陳腐化記述のみを対象に検出。ネットワーク系の正当な `medium` 記述〔AC-35〕や一般レベル表の `medium` は対象外。systemctl 変更系=High / 読み取り=Medium 下限・`service`=High に整合した記述へ書き換え済み） |
| AC-38 | static | `rg -n "最大値\|maximum" docs/user/risk_assessment.ja.md` | プロファイル要因含む最大値の記述 |
| AC-39 | test | `internal/runner/base/risk/coreutils_consistency_test.go::TestConsistency_ProfileCommands` | claude/systemctl/curl で実行時/dry-run 一致 |
| AC-40 | test | `internal/runner/base/risk/evaluator_test.go::TestEvaluateRisk_UncertainBlockedEvenAtHigh` | 各不確実ケースが risk_level=high でも非実行 |
| AC-41 | test | `internal/runner/base/audit/logger_test.go::TestLogRiskProfile_UncertainReason` | 不確実中止が理由とともに監査追跡可能 |
| AC-42 | test | `internal/runner/base/risk/evaluator_test.go::TestEvaluateRisk_DangerousSignalsHighAllowable` | 危険シグナル High は risk_level=high で実行可 |
| AC-43 | test | `internal/runner/base/risk/coreutils_consistency_test.go::TestConsistency_UncertainCases` | normal/dry-run で不確実判定一致 |
| AC-44 | test | `internal/runner/base/risk/evaluator_test.go::TestEvaluateRisk_NoProfileAbsolutePath` | `rmdir`/`shred`/`mount`/`crontab` 絶対パスで正しいリスク |
| AC-45 | test | `internal/runner/base/security/network_analyzer_test.go::TestClassify_AllResultClasses` | 全結果区分が Uncertain/HighRisk/Network/Clean に正しく対応 |
| AC-46 | test | `internal/runner/resource/dryrun_manager_test.go::TestDryRun_AnalysisUnavailableDenyPreview` | 解析無効環境で deny 予告＋運用注記 |
| AC-47 | test | `internal/runner/base/risk/evaluator_test.go::TestEvaluateRisk_DangerousArgPatternsRuntime` | chmod -R 777 /・dd if= 等が実行時に該当リスク |
| AC-48 | test | `internal/runner/base/audit/logger_test.go::TestLogRiskProfile_NoProfileReasonCode` | プロファイル無しコマンドも reason code 出力 |
| AC-49 | test | `internal/runner/base/risk/evaluator_test.go::TestEvaluateRisk_SystemctlSubcommandConditional` | status/show=Medium 下限、restart/未知=High |
| AC-50 | test | `internal/runner/base/risk/evaluator_test.go::TestEvaluateRisk_RiskAssessmentDocExamples` ＋ static `rg -n "systemctl status" docs/user/risk_assessment.ja.md` | §5 設定例が修正後実装で動作 |
| AC-51 | test | `internal/runner/base/risk/evaluator_test.go::TestEvaluateRisk_AnalysisDisabledAlwaysDeny` | 解析無効で常時拒否（coreutils 含む）、オプトインなし |
| AC-54 | test | `internal/runner/base/risk/evaluator_test.go::TestEvaluateRisk_SymlinkResolutionFailureBlocking` | 解決失敗で部分継続せず Blocking |
| AC-55 | test | `internal/runner/base/risk/evaluator_test.go::TestEvaluateRisk_SymlinkFailureNotLow` | 解決失敗が Low 側に落ちない（リンク先取得失敗含む） |
| AC-56 | test | `internal/runner/base/audit/logger_test.go::TestLogRiskProfile_CorrelationFieldsAndAbsence` | 相関フィールド・在/不在明示・センチネル文字列なし・deny でも出力 |
| AC-57 | test | `internal/runner/base/audit/logger_test.go::TestLogRiskProfile_ArgMasking` | 引数マスキング適用 |
| AC-58 | test | `internal/runner/resource/dryrun_manager_test.go::TestDryRun_VerificationUnavailableExitCode` | 検証不能 deny を専用終了コードで区別 |
| AC-59 | test | `internal/runner/base/security/indirect_execution_test.go::TestIndirect_WrapperSudoCritical` | env sudo / timeout sudo / xargs sudo が Critical |
| AC-60 | test | `internal/runner/base/security/indirect_execution_test.go::TestIndirect_WrapperDestructive` | env rm -rf / timeout systemctl stop 等がラップなし同等以上 |
| AC-61 | test | `internal/runner/base/security/indirect_execution_test.go::TestIndirect_ShellInlineHigh` | bash -c / python -c が High 以上 |
| AC-62 | test | `internal/runner/base/security/command_analysis_test.go::TestFindExecAllActions` | -exec/-execdir/-ok/-okdir・coreutils 配下対象を破壊判定 |
| AC-63 | test | `internal/runner/base/risk/evaluator_test.go::TestEvaluateRisk_MaxOfDimensionsOrderIndependent` | 複数次元該当で最大値、順序非依存 |
| AC-64 | test | `internal/runner/group_executor_test.go::TestGroupExecutor_IdentityBoundNoReResolve` | 検証〜判定が同一実体、再解決なし |
| AC-65 | test | `internal/runner/base/risk/evaluator_test.go::TestEvaluateRisk_UnverifiedHashUncertainAllPaths`（table-driven で各経路を 1 ケースずつ: coreutils-safe `echo` / coreutils-destructive `rm` / プロファイル `claude` / F-015 `python` / プロファイル無し破壊 `/usr/bin/rmdir`） | 未検証ハッシュの各経路すべてが Blocking deny へ帰着（Low/High-allowable に確定しない） |
| AC-66 | static | `rg -n "ブロックリスト\|allowlist\|ハッシュ固定" docs/user/risk_assessment.ja.md` | ブロックリスト方式・allowlist+ハッシュ固定前提を明記 |
| AC-67 | static | `rg -n "ハードリンク\|output_file\|root" docs/user/risk_assessment.ja.md` | basename 限界・output_file 対象外・root 判定系を明記 |
| AC-68 | test | `internal/runner/base/security/coreutils_test.go::TestCoreutils_UnknownSubcommandHigh` | 未知/判別不能サブコマンドが High |
| AC-69 | test | `internal/runner/base/security/network_analyzer_test.go::TestClassify_DistinctReasonCodes` ＋ `internal/runner/base/risktypes/reason_codes_test.go::TestReasonCodes_AllDistinct` | 根拠別に異なる reason code、全種網羅 |
| AC-70 | test | `internal/runner/base/audit/logger_test.go::TestLogRiskProfile_DenySeverityFloor` | 全 deny が Warn 以上で相関検索可能 |
| AC-71 | test | `internal/runner/base/security/indirect_execution_test.go::TestIndirect_BypassAttackerScenarios` | AC-59〜62 の攻撃者視点ケース網羅 |
| AC-72 | test | `internal/runner/base/risk/evaluator_test.go::TestEvaluateRisk_CoreutilsPriorityOverBinaryAnalysis` | echo は dlopen/レコード欠落でも Low、rm は High |
| AC-73 | test | `internal/runner/base/risk/evaluator_test.go::TestEvaluateRisk_ShellInterpreterHigh` | bash/python/node/ruby/perl が High 以上 |
| AC-74 | test | `internal/runner/base/risk/evaluator_test.go::TestEvaluateRisk_BuildRunnerHigh` | make/cmake/gradle が High 以上 |
| AC-75 | test | `internal/runner/base/risk/evaluator_test.go::TestEvaluateRisk_ServiceAllActionsHigh` ＋ `internal/runner/base/security/indirect_execution_test.go::TestIndirect_ServiceInitScriptGated` | service が読み取りアクションでも High、init スクリプトをゲート |
| AC-76 | test | `internal/runner/base/executor/executor_test.go::TestExecute_FdBoundOrStaging` ＋ `internal/runner/group_executor_test.go::TestGroupExecutor_ExecIdentityBound` | 検証〜exec の全区間で同一 identity、再ハッシュ path exec なし |
| AC-77 | test | `internal/runner/base/security/indirect_execution_test.go::TestIndirect_InnerCommandGated` | 抽出インナーが allowlist/ハッシュゲート、通せなければ拒否 |
| AC-78 | test | `internal/runner/base/security/indirect_execution_test.go::TestIndirect_WrapperNoCommandMedium` | env 単体は Medium 以上、抽出不能と区別 |
| AC-79 | test | `internal/runner/base/security/indirect_execution_test.go::TestIndirect_EnvPathResolutionSwap` | env PATH= で /tmp/rm が実行されない |
| AC-80 | test | `internal/runner/base/security/indirect_execution_test.go::TestIndirect_WrapperLoaderEnvRejected` | env LD_PRELOAD= 等が拒否 |
| AC-81 | test | `internal/runner/base/security/indirect_execution_test.go::TestIndirect_EnvSplitString` | env -S 'sudo …' が Critical、解釈不能は拒否 |
| AC-82 | test | `internal/runner/base/security/indirect_execution_test.go::TestIndirect_FindXargsTargetGated` | find/xargs 対象が allowlist/ハッシュ＋検証済み絶対パス実行 |
| AC-83 | test | `internal/runner/base/security/indirect_execution_test.go::TestIndirect_DynamicLoaderGated` | ld-linux の EXECUTABLE/preload/library-path をゲート/load-time 束縛 or 拒否 |
| AC-84 | test | `internal/runner/base/security/indirect_execution_test.go::TestIndirect_UnextractableWrapperRejected` | 抽出不能ラッパーは High でなく拒否、AC-78 と区別 |
| AC-85 | test | `internal/runner/base/security/indirect_execution_test.go::TestIndirect_PackageScriptRunnerHigh` | npm run/npx/yarn/pnpm が High 以上 |
| AC-86 | test | `internal/runner/base/security/indirect_execution_test.go::TestIndirect_ShebangInterpreterGated` | shebang 連鎖を評価＋ゲート＋identity 束縛 or 拒否 |
| AC-87 | test | `internal/runner/base/security/indirect_execution_test.go::TestIndirect_CommandExecOptionsGated` | rsync -e / tar --to-command の helper をゲート or 拒否 |

> 注: AC-15/AC-18 と AC-29/AC-17 の command-risk-evaluation 部分は PR #724 マージ後に検証可能（Step 4-3）。Phase 1〜3 の `test` 系 AC はこの依存を持たない。AC-52/AC-53 は欠番（`01_requirements.md` 前提・依存節）。

---

## 8. クロス検索チェックリスト

`make lint`/`make test` が検出できない残存参照・命名衝突・文書整合のみを対象とする（AC 検証表と重複する rg はここに再掲しない）。

- [ ] `rg -n "IsNetworkOperation" --type go`（削除する旧 API の残存参照）→ テスト/呼出が `Classify` へ移行済み、0 件。
- [ ] `rg -n "LogRiskProfile\(" --type go`（旧シグネチャ呼出）→ 全て新パラメータ構造体形式、旧 4 引数形式 0 件。
- [ ] `rg -n "func .*EvaluateRisk" --type go` ＋ `rg -n "EvaluateRisk\(" --type go`（戻り値型）→ `VerifiedCommandPlan` 返却に統一、旧 `(RiskLevel, error)` 期待のテスト 0 件。
- [ ] `rg -n "ExpandedCmd = resolvedPath" internal/runner/group_executor.go`（廃止した二重解決の残存）→ 0 件。
- [ ] `rg -n "risktypes" --type go`（新パッケージ名の衝突）→ import パスが一意。共有 DTO（`VerifiedCommandPlan`/`RiskAssessment` 等）が `runnertypes` 側に定義されていない（§1.4 で却下した配置の残存なし）こと: `rg -n "VerifiedCommandPlan\|RiskAssessment" internal/runner/base/runnertypes/` が 0 件。
- [ ] `rg -n "ReasonCode\b" --type go`（cross-package の汎用識別子衝突）→ `risktypes.ReasonCode` のみ。
- [ ] 用語整合: `02_architecture.md` と本書で「実効リスク」「最大許容リスク」「不確実ケース」「無条件拒否（Blocking）」「リスク昇格」の訳語が一致（`docs/translation_glossary.md` 参照、必要なら追記）。
- [ ] `rg -n "dpkg" docs/`（ユーザー文書からの削除確認、AC-34 と別に docs 全体）→ 残存は許容される歴史的注記のみ。
- [ ] PR 構成の三者一致: 各 `### PR-N 作成ポイント` の `対象ステップ` ＝ §3.2 PR 構成表 ＝ §6 PR チェックリスト（再編時はこの 3 箇所を同時更新）。

---

## 9. Success Criteria

- **機能完全性**: AC-01〜AC-87（52/53 欠番、15/18 と 29/17 の一部は PR #724 後）がすべて §7 の `test`/`static` で充足。
- **品質**: `make test` / `make lint` がパス（AC-21）。新規 production コードは導入フェーズの完了ゲートで使用タグ込みコンパイル確認（Step 2-2）。
- **セキュリティ検証**: バイパス系テスト（AC-71）、identity 束縛（AC-76）、解析無効常時拒否（AC-51）、不確実中止（AC-40）が明示的にテストされる。
- **ドキュメント**: 移行ノート・脅威モデル・security-architecture 例外・risk_assessment 整合が完了（command-risk-evaluation は PR #724 後）。

---

## 10. Next Steps

- [ ] 本実装計画書を人間レビューに提出し、`approved` を得る。
- [ ] `approved` 後、Phase 1（Step 1-1）から実装着手（`runplan` 手順）。
- [ ] PR #724 のマージ状況を監視し、マージ後に Step 4-3（command-risk-evaluation 文書）を着手。
