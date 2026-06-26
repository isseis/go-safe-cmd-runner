# 監査・文書整合 — 実装計画書

## Document Status

| Item | Value |
|---|---|
| Status | `approved` |
| Created | 2026-06-26 |
| Review date | 2026-06-26 |
| Reviewer | isseis |
| Comments | - |

> 本書は [01_requirements.md](01_requirements.md)（`approved`）と [02_architecture.md](02_architecture.md)（`approved`）を
> 実装手順へ落とすものである。設計の根拠・データフロー・脅威モデルは [02_architecture.md](02_architecture.md) を参照し、
> 本書では繰り返さない。先行タスク 0141・0142・0144・0145 がマージされ `make test` が緑であることを着手の前提とする
> （[01_requirements.md](01_requirements.md) §3）。

---

## 1. 実装の全体像

### 1.1 目的

先行タスク（0141・0142・0144・0145）が確定させた分類・データ構造を、運用者が観測・理解できる形へ反映する。
本タスクは新しい分類ロジックを追加せず、次の 3 種の横断成果物を所有する（[02_architecture.md](02_architecture.md) §1.1）。

1. **監査出力**: `RiskAssessment.OperandZones`（0142 が格納済み）を `LogRiskProfile` から `operand_zones` フィールドとして
   出力し、秘匿情報を出力境界の redaction へ配線する（F-001/AC-01）。
2. **理由コード family 区別**: `ReasonFamily` 型・コード→family テーブル・`FamilyOf` を追加し、全 `ReasonCode` の family
   割当をテストで担保する（F-002/AC-03）。deny 時の理由コード記録を end-to-end で担保する（AC-02）。
3. **文書・config 追従**: 移行ノート（AC-04/AC-05/NF-004）・文書整合（AC-06）・sample/test config 追従（AC-07）・分類ガイド
   最終化（AC-08）。

### 1.2 実装原則

- **読み取り専用の反映**: carrier・分類・理由コード定数の値は新設・変更しない（[02_architecture.md](02_architecture.md) §1.1）。
- **既存の出力規約を範例とする（DRY）**: 存在条件は `len()>0` ガード、秘匿マスクは既存 `command_args` と同じ
  `argRedactor.RedactText` を再利用する。新しい redactor・パターンは追加しない（[02_architecture.md](02_architecture.md) §3.2）。
- **English ソース**: Go の識別子・コメント・文字列リテラル（テストソース・`//go:build test` ヘルパを含む）は英語で記述する
  （[01_requirements.md](01_requirements.md) §3）。
- **バイリンガル文書の編集順序**: 日本語版を先に編集・コミットし、英語版は `/mktrans` で反映する（直接両方を編集しない）。

### 1.3 既存コード調査結果

実装着手前に関連パッケージ・テスト・文書・config を調査した結果は次のとおり。

**コード（変更対象）**

- [audit/logger.go](../../../internal/runner/base/audit/logger.go) `LogRiskProfile`: 既存フィールド（`reason_codes`/
  `risk_factors`/`blocking_reason`/`command_args`/`chain`）は `len()>0` ガードで追加され、`command_args` は
  `argRedactor.RedactText`（`redaction.DefaultConfig()`、Placeholder=`[REDACTED]`）でマスク済み。`chain` は
  `[]map[string]string` を `slog.Any` で渡し、`Path` は未マスク。**`operand_zones` の出力・マスク配線は未実装**。
  追加位置は `chain` ブロックの直後が自然。
- [redaction/redactor.go](../../../internal/redaction/redactor.go) `RedactText`: `KeyValuePatterns`（`password=`/`token=`/
  `secret=`/`api_key=` 等・認証ヘッダ）のみ置換する。裸のパス成分に埋め込まれた資格情報は対象外
  （[02_architecture.md](02_architecture.md) §3.2 の限界に一致）。`RedactingHandler` は `slog.Any` のスライス／マップ要素へ
  再帰しないため、配列形 `operand_zones` は Layer 2 で保護されず、`LogRiskProfile` 内の境界 redaction が唯一の制御
  （[02_architecture.md](02_architecture.md) §4）。
- [risktypes/reason_codes.go](../../../internal/runner/base/risktypes/reason_codes.go): `ReasonCode` 定数は全 36 個。family は
  コメント上のグルーピングに留まり機械的強制なし。**`ReasonFamily`・`FamilyOf`・family テーブルは未定義**。
- [risktypes/operand_zone.go](../../../internal/runner/base/risktypes/operand_zone.go) `OperandZone`: サブフィールド
  Index(int)/Raw(string)/Resolved(string)/Zone(PathTrustZone)/Role(OperandRole)/MatchedCritical(string)/Trusted(bool)/
  UnresolvedErr(string)。0142 が定義済みで本タスクは読み取り専用。

**テスト（変更・追加対象）**

- [audit/logger_test.go](../../../internal/runner/base/audit/logger_test.go)（`package audit_test`）: ヘルパ
  `logRiskProfileEntry(t, entry)`（Debug レベル JSON へ出力し `map[string]any` を返す）と `strptr` が再利用可能。
  既存テストは `command_args` マスク（`TestLogRiskProfile_ArgMasking`、`[REDACTED]` を表明）・`chain` を表明済み。
  **operand_zones の存在/非存在/漏えい否定テスト（秘匿情報が出力に現れないことを表明するテスト）は未追加**。
- [risktypes/reason_codes_test.go](../../../internal/runner/base/risktypes/reason_codes_test.go)（`package risktypes`）:
  `TestReasonCodes_AllDistinct` が全 36 コードをハードコードした `all := []ReasonCode{...}` で列挙している。これを
  family テーブルのキー集合由来へ置換し、並行リストを廃する（[02_architecture.md](02_architecture.md) §3.3）。
- [resource/audit_wiring_test.go](../../../internal/runner/resource/audit_wiring_test.go)（`package resource`）: e2e ハーネス
  `newAuditingNormalManager(evaluator)`・`findRiskProfileEntry(t, buf)` が再利用可能。ただし `fixedPlanEvaluator`（preset
  Assessment）駆動であり、実分類器を通さない。`TestExecute_SystemModificationDenyAuditable`（`system_modification`）・
  `TestExecute_RejectedCommandAuditable`（`destructive_file_operation`）が Assessment→entry の配線を担保済み。
- [risk/evaluator_test.go](../../../internal/runner/base/risk/evaluator_test.go)（`package risk`）: 既存の理由コード表明は
  **`dpkg`/`systemctl`=`ReasonSystemModification`** に限られる（`TestEvaluateRisk_SystemModificationReasonCode`）。一方、
  **`insmod`=`system_modification`・`dd if=...of=/dev/sdb`=`dangerous_arg_pattern`・trust-critical 書込=`trust_boundary_write`
  の理由コードは評価器レベルでは未表明**である（`insmod`/`dd` は Level==High のみを表明する＝
  `TestEvaluateRisk_SystemModHighNotPrivilegeCritical`／`TestEvaluateRisk_DangerousArgPatternsRuntime`）。
  このため Step 1.5 の e2e が、これら 3 リンクの**初の表明**となる。実評価器の構築ヘルパ
  `newVerifiedEvaluator()`・`newZoningEvaluator(workdir, ident)` は
  [risk/test_helpers.go](../../../internal/runner/base/risk/test_helpers.go)（`//go:build test`）に**非公開**で存在する。
  → AC-02 の「実分類→監査出力」の seam（実分類器と監査出力の接合点）を閉じる e2e テストは、これらを使うため
  **`package risk`** に置く必要がある（評価器レベルの重複ユニットテストは追加せず、e2e が seam と未表明リンクの両方を兼ねる）。

**文書（変更・新規対象）**

- [risk_assessment.ja.md](../../../docs/user/risk_assessment.ja.md): 「§8 移行ノート」が既存（line 242〜）。引き上げ記述は
  あるが、本タスクの **引き下げ（D7）独立ブロック**・**0145 fail-closed 化**・**NF-004 ログレベル運用注記**は未記載。
- [command-risk-evaluation.ja.md](../../../docs/dev/architecture_design/command-risk-evaluation.ja.md) と英語版
  [command-risk-evaluation.md](../../../docs/dev/architecture_design/command-risk-evaluation.md): システム管理系コマンドを
  **Medium** とする記述（`mount`/`umount`/`parted`/`fsck`/`crontab`/`at`/`batch`/`chkconfig`/`update-rc.d`）が残存し、
  0141 が `parted`/`fsck` を High へ引き上げた確定分類と齟齬する（**AC-06 の除去確認対象**）。検出限界の節は旧
  「コマンドごとの独自パーサ」前提のままで 0144/0145 の現行アーキテクチャ未反映。監査セクション（既存）に
  `operand_zones` の記述は未追加。
- [risk-level-classification-guide.ja.md](../../../docs/dev/architecture_design/risk-level-classification-guide.ja.md):
  Status=`draft`。英語版 `risk-level-classification-guide.md` は**未作成**。
- [translation_glossary.md](../../../docs/translation_glossary.md): `パス信頼区分`・`trust-critical`・`safe-zone`・
  `オペランド毎`(per-operand) は登録済み。**`移行ノート`（migration note）は単一エントリ未登録**（`移行`/`移行ガイド` のみ）。

**sample/test config（追従対象）**

- [sample/risk-based-control.toml](../../../sample/risk-based-control.toml): `rm`（line 58・`-rf`・`risk_level="high"` 設定済み）。
- [sample/comprehensive.toml](../../../sample/comprehensive.toml): `touch`（line 88/112/130・素のファイル名引数で 0145 の `-p`
  無効フラグ形ではない＝非該当）。
- [sample/variable_expansion_advanced.toml](../../../sample/variable_expansion_advanced.toml): `/bin/mkdir`（line 71・`-a` 等の
  0145 無効フラグ形ではない）。
- [sample/variable_expansion_basic.toml](../../../sample/variable_expansion_basic.toml): `/bin/cp`（line 34・判断軸2 は宛先パス
  依存のため、宛先ゾーン次第で判定が変わる）。
- 上記が `cmd` ベースの該当ヒットの全件であり、引き上げ・fail-closed 化対象コマンドの**網羅的な再列挙**を Phase 2 で grep に
  より行う（下記 AC-07 タスク）。判断軸1 で High 化した名前（`ln`・`insmod` 等）を `cmd` に持つ既存 config は現時点で存在しない。

---

## 2. 実装ステップ

各フェーズの順序は [02_architecture.md](02_architecture.md) §8 の優先順位に一致する。コード（Phase 1）→ config（Phase 2）→
日本語文書（Phase 3）→ 英語版（Phase 4）。

### Phase 1 — コード（監査出力・family 区別・deny 理由コード e2e）

#### Step 1.1: `operand_zones` の監査出力と redaction 配線（F-001/AC-01）

**対象ファイル**: [internal/runner/base/audit/logger.go](../../../internal/runner/base/audit/logger.go)

- [x] `LogRiskProfile` 内の `chain` ブロック直後に `operand_zones` 構築ブロックを追加する。存在条件は
      `len(entry.Assessment.OperandZones) > 0`（[02_architecture.md](02_architecture.md) §3.2）。空・nil ではキーを付けない。
- [x] 各 `OperandZone` を 1 要素 `map[string]any` として構築する（`Index`(int)/`Trusted`(bool) を型保持するため
      `chain` の `map[string]string` ではなく `map[string]any` を用いる）。キーは `index`/`raw`/`resolved`/`zone`/`role`/
      `matched_critical`/`trusted`/`unresolved_err`（snake_case 英語）。
- [x] `raw`/`resolved`/`unresolved_err` の文字列値は `argRedactor.RedactText(...)` を**経由してから**格納する。
      `zone`/`role`/`matched_critical`/`index`/`trusted` はマスクしない（[02_architecture.md](02_architecture.md) §3.2）。
- [x] 構築した配列を `attrs = append(attrs, slog.Any("operand_zones", zones))` で追加する。
- [x] シグネチャは変更しない。`make fmt` → `make test` → `make lint` を緑に保つ。

**成功基準**: Step 1.4 のテストが緑。`LogRiskProfile` の他フィールド出力は不変。

#### Step 1.2: `ReasonFamily`・family テーブル・`FamilyOf` の追加（F-002/AC-03）

**対象ファイル**: [internal/runner/base/risktypes/reason_codes.go](../../../internal/runner/base/risktypes/reason_codes.go)

- [x] `type ReasonFamily string` と 6 個の family 定数を追加する（[02_architecture.md](02_architecture.md) §3.3）:
      `FamilyNameClassification`=`"name_classification"`、`FamilyPrivilege`=`"privilege"`、
      `FamilyBinaryAnalysis`=`"binary_analysis"`、`FamilyUncertain`=`"uncertain"`、
      `FamilyRuntimeArgument`=`"runtime_argument"`、`FamilyPathTrustZone`=`"path_trust_zone"`。
- [x] 全 36 `ReasonCode` を family へ対応づける `map[ReasonCode]ReasonFamily`（非公開・パッケージ変数）を定義する。割当は
      [02_architecture.md](02_architecture.md) §3.3 の family 割当表（全件）に従う。このテーブルを「コードの正準な列挙」とする。
- [x] `func FamilyOf(code ReasonCode) (ReasonFamily, bool)` を追加する。テーブルに無いコードは `("", false)` を返す。
- [x] 既存 `ReasonCode` 定数の文字列値・意味・並びは変更しない。

**成功基準**: Step 1.3 のテストが緑。

#### Step 1.3: 理由コード網羅・family テストの更新（F-002/AC-03/NF-001/S-2）

**対象ファイル**: [internal/runner/base/risktypes/reason_codes_test.go](../../../internal/runner/base/risktypes/reason_codes_test.go)

- [x] `TestReasonCodes_AllDistinct` のハードコード `all := []ReasonCode{...}`（36 件）を削除し、family テーブルのキー集合
      （`slices.Collect(maps.Keys(...))` 等）から導出するよう書き換える。並行リストを廃する
      （[02_architecture.md](02_architecture.md) §3.3、メモリ「リスト漏れはソース集合の range で検証」方針）。文字列値の
      非空・一意（重複した string 値が無い）の表明は維持する。
- [x] `TestReasonFamily_AllCodesAssigned` を追加: family テーブルの各値が定義済み 6 family のいずれかであり、空でないこと
      を表明する（family 割当の網羅。exhaustive/distinct とは別の保証＝S-2）。
- [x] ground-truth アンカーを追加: 同テストで family テーブルの要素数が期待値（現状 36。確定分類の全 `ReasonCode` 数）に
      等しいことを表明する。Go は const ブロックをリフレクション列挙できず、新規 `ReasonCode` をテーブルへ未追記のままにすると
      table 由来のテストでは検出できないため（旧ハードコード `all` リストが担っていた独立証跡の代替）、件数アンカーで
      テーブルへの追記漏れを検出する。期待値はコメントで根拠（reason_codes.go の定数数）を併記する。
- [x] `TestReasonFamily_OfReturnsAssignedFamily` を追加: テーブルの各コードに対し `FamilyOf(code)` が `ok=true` と
      テーブル値どおりの family を返すこと、未知コード（例 `ReasonCode("__nonexistent__")`）に対し `ok=false` を返すことを
      表明する。

**成功基準**: 上記 3 テストが緑。`go test -tags test ./internal/runner/base/risktypes/` 成功。

#### Step 1.4: `operand_zones` 出力・漏えい否定の単体テスト（F-001/AC-01/S-1）

**対象ファイル**: [internal/runner/base/audit/logger_test.go](../../../internal/runner/base/audit/logger_test.go)

- [x] `TestLogRiskProfile_OperandZones` を追加: 複数オペランド（write/read 混在、symlink 解決済み・unresolved を含む）の
      `OperandZones` を持つ entry を既存ヘルパ `logRiskProfileEntry` で出力し、`operand_zones` 配列の各要素の
      `index`/`raw`/`resolved`/`zone`/`role`/`trusted` が carrier の値どおりであることを表明する。
- [x] 同テストで `ZoneUnresolved` の要素について、`resolved` が**空**であり `unresolved_err` が（redaction 後の）原因
      メッセージを保持することを表明する（「適用済みだが解決不能」を「非適用」から区別する load-bearing な信号
      ＝[02_architecture.md](02_architecture.md) §2.2・§3.1）。
- [x] 同テストに「carrier 空（`nil`/`len()==0`）では `operand_zones` キーが**無い**」ケースと、「deny（`DecisionDeny`）
      経路でも carrier があれば出力される」ケースを追加する。
- [x] `TestLogRiskProfile_OperandZoneMasking` を追加（漏えい否定／S-1）: `Raw`/`Resolved`/`UnresolvedErr` に
      `key=value` 形式の秘匿トークン（例 `/data/dump?token=supersecretvalue`、`UnresolvedErr` に `password=...`）を持つ
      オペランドを与え、出力で (i) 当該秘匿値が**現れず** `[REDACTED]` に置換されていること、(ii) 秘匿でないパス成分
      （例 `/data/dump`）は**保持される**こと（マスクが外科的で、フィールド全体を落とさないこと）を表明する。既存
      `TestLogRiskProfile_ArgMasking`（負＋正の両表明）の様式を範例とする。

**成功基準**: 追加テストが緑。`go test -tags test ./internal/runner/base/audit/` 成功。

#### Step 1.5: deny 時の理由コード記録の end-to-end テスト（F-002/AC-02/S-3）

**対象ファイル**: 新規 [internal/runner/base/risk/audit_reason_codes_test.go](../../../internal/runner/base/risk/audit_reason_codes_test.go)（`package risk`）

> 配置理由（[02_architecture.md](02_architecture.md) §7.2）: 実分類器の構築ヘルパ（`newVerifiedEvaluator`/`newZoningEvaluator`）が
> `package risk` の非公開 `//go:build test` ヘルパであり、`resource` パッケージからは到達できない。本テストは実分類器の
> プランを `audit.LogRiskProfile` へ渡し、`command_risk_profile` 出力エントリの `reason_codes`/`blocking_reason` に正しい
> コードが現れることを表明して「実分類→監査出力」の seam を閉じる。配線（Assessment→entry）の一般形は既存
> `audit_wiring_test.go`（`fixedPlanEvaluator` 駆動）が担保済みのため重複させない。一方、本テストが対象とする 3 リンク
> （`insmod`=`system_modification`・`dd`=`dangerous_arg_pattern`・trust-critical 書込=`trust_boundary_write`）は §1.3 のとおり
> 評価器レベルでも未表明であり、本 e2e がその初の表明を兼ねる（別途の評価器ユニットテストは追加しない）。
> なお `newVerifiedEvaluator`/`newZoningEvaluator` は descriptor-free な `fakeIdentityOpener` を用いるため、生成される
> プランは OS リソースを保持せず、`VerifiedCommandPlan.Close`/`t.Cleanup` は不要である。

- [x] `TestLogRiskProfile_DenyReasonCodes_EndToEnd` を追加し、3 代表 deny を実評価器で生成して `audit.LogRiskProfile`
      （`audit.NewAuditLoggerWithCustom` + Debug レベル JSON バッファ）へ流し、各ケースで `reason_codes` に対応コードが
      現れることを表明する。3 代表はいずれも High の ceiling deny（非 Blocking）であり `BlockingReason` を持たないため、
      `blocking_reason` キーが付かないことも併せて確認する。Blocking/Critical deny の `blocking_reason` 出力は既存
      `audit_wiring_test.go`（`fixedPlanEvaluator` 駆動）が担保済みのため重複させない:
  - [x] 判断軸1 由来: `/sbin/insmod` を `newVerifiedEvaluator()` で評価し、`reason_codes` に `system_modification`
        （`ReasonSystemModification`）が含まれることを表明。
  - [x] 判断軸2 由来: trust-critical への書込（例 `cp` の宛先が `SystemCriticalPaths` 配下）を `newZoningEvaluator(...)` で
        評価し、`reason_codes` に `trust_boundary_write`（`ReasonTrustBoundaryWrite`）が含まれることを表明（S-3: axis 2 ブロックの
        定数名を正確に引く）。
  - [x] 危険引数パターン由来: `dd if=...of=/dev/sdb` を評価し、`reason_codes` に `dangerous_arg_pattern`
        （`ReasonDangerousArgPattern`）が含まれることを表明。
  - [x] entry の `Decision` は `DecisionDeny` を設定し、`MaxAllowedRisk=Low` で「High が Low 上限で deny される」ceiling
        deny をモデル化する（3 代表は非 Blocking のため deny はマネージャの上限判定で生じる）。
- [x] 各ケースで使用する `ReasonCode` 定数は [reason_codes.go](../../../internal/runner/base/risktypes/reason_codes.go) の実在
      シンボルを参照する（文字列リテラル直書きを避ける）。

**成功基準**: 追加テストが緑。`go test -tags test ./internal/runner/base/risk/` 成功。

#### Step 1.6: Phase 1 完了ゲート

- [x] `make fmt` 実行後の差分なし。
- [x] `make test` 全件緑。
- [x] `make lint` エラーなし。

### Phase 2 — sample/test config 追従（F-005/AC-07）

**対象**: [sample/](../../../sample/) 配下および全 testdata の `.toml`

- [x] 引き上げ・fail-closed 化対象コマンドを使う config を grep で網羅列挙する。対象コマンド名は次を含む（判断軸1 High/Medium
      化対象＝`ln`/`insmod`/`fdisk`/`mkfs`/`parted`/`fsck` 等、判断軸2 trust-critical 書込形＝`cp`/`mv`/`rsync`/`chmod`/`chown` 等、
      0145 で fail-closed 化した無効フラグ形＝`sponge`/`mkdir`/`touch`/`unlink`/`rmdir` 等）
      （[01_requirements.md](01_requirements.md) AC-07 の列挙に準拠）。
      列挙コマンド（静的検証）:
      `rg -n --glob '*.toml' -e 'cmd\s*=\s*"(/[^"]*/)?(rm|dd|shred|unlink|rmdir|mkdir|touch|mv|sponge|ln|fdisk|mkfs|insmod|parted|fsck|cp|rsync|chmod|chown)"' sample cmd internal`
      判断軸2 の `cp`/`mv`/`rsync`/`chmod`/`chown` は**宛先パス依存**で deny するため、名前ヒットしても宛先ゾーンに応じて
      意図結果を個別確認する（一律 deny にはならない）。
- [x] 列挙された各 config について、次のいずれかを満たすことを確認・対応する:
  - [x] 新分類で deny され得るものは適切な `risk_level` を付与する（例: 判断軸1 で High 化した名前を `cmd` に持つ config を新設・
        変更する場合は `risk_level="high"` を付与）、または
  - [x] 新分類下でも意図した結果（allow/deny）になることを確認する。意図的に deny を検証する config はその旨をコメント等で明示する。
  - [x] 0145 の無効フラグ形（`sponge -r`/`mkdir -a`/`touch -p`/`unlink -r`/`rmdir -r`/`mv -s` 等）を使う config が**無い**こと、
        または存在する場合は fail-closed（High deny）が意図どおりであることを確認する。
- [x] 上記 config が `make test` 内でロード・評価でき、テスト用 config が新分類で**意図せず** deny されないことを確認する。

**成功基準**: 列挙 grep の各ヒットに対応済み。`make test` 全件緑。

### Phase 3 — 文書（日本語版）

> バイリンガル編集順序に従い、本フェーズでは日本語版のみを編集・コミットする。英語版は Phase 4 で `/mktrans` により反映する。

#### Step 3.1: 移行ノート（F-003/AC-04/AC-05/NF-004）

**対象ファイル**: [docs/user/risk_assessment.ja.md](../../../docs/user/risk_assessment.ja.md) の「§8 移行ノート」

- [x] **引き上げ（AC-04）**: Low/Medium→High への引き上げ（0141 の判断軸1 High 化・0142 の trust-critical 書込）により、
      従来許可していた config がブロックされ得ることを記載する。安全運用（allowlist + ハッシュ固定 + 明示的 `risk_level`）を
      併記する。（§8.5・§8.6 に記載）
- [x] **0145 fail-closed 化の周知（AC-04 側）**: 実 CLI に無いフラグの受理を除去した結果、当該フラグ形を recognized のまま
      通していた config が新たに High で deny され得ること（`sponge -r`/`mkdir -a`/`touch -p`/`unlink -r`/`rmdir -r`/`mv -s` 等）を
      **引き上げ側の周知**として明記する（緩和方向ではないため AC-05 の独立警告ブロックには含めない）。（§8.7 に記載）
- [x] **引き下げ（AC-05）**: High→Medium/Low に引き下がるコマンド（`rm`/`rmdir`/`shred`/`unlink`/`dd` の safe-zone/ordinary
      ケース＝0142 の D7）を **セキュリティ緩和方向の変更（security relaxation）** として、引き上げリストへ埋没させず
      **独立した見出し／警告ブロック**で提示する。対象コマンド・緩和条件（safe-zone/ordinary）・baseline（直近リリース挙動）を明記する。（§8.8 に独立見出し＋警告ブロックで記載）
- [x] **NF-004 運用注記**: 引き下げ対象の allow-Low は `riskLogLevel` により Debug 出力となり、本番ログ設定では
      `operand_zones` を含むエントリごと失われ得ること、緩和コマンドの監査証跡を残すにはログレベルを Debug 以下にする回避策を、
      AC-05 ブロック内に運用注記として記載する。（§8.8 の運用注記に記載）
- [x] **0139 との上書き関係（AC-06(c)）**: 0139 と本一連で記述が衝突する箇所は、本一連の分類が上書きする関係を移行ノート側で明示する。（§8.9 に記載）

**成功基準**: 引き上げ・引き下げが同一移行ノート内に存在し、引き下げが独立見出し／警告ブロックとして示され、引き上げ記述に
紛れて見落とされないこと（Step 3.5 の静的検証）。

#### Step 3.2: ユーザー/開発者文書の整合（F-004/AC-06）

**対象ファイル**: [docs/user/risk_assessment.ja.md](../../../docs/user/risk_assessment.ja.md)・
[docs/dev/architecture_design/command-risk-evaluation.ja.md](../../../docs/dev/architecture_design/command-risk-evaluation.ja.md)

- [x] **(a) 追加の網羅**: 実装で High/Medium/Low に確定した全コマンド分類を各文書の分類表へ反映する。（user §3.1/§3.5、
      dev `SystemModificationRisk`・新設「宛先パス信頼区分（判断軸2）」節に反映）
- [x] **(b) 除去の確認**: 旧記述を除去する。最低限、(i) 旧「fdisk/mkfs=Medium」、(ii) 旧「`rm`/`dd` の無条件 High」、
      (iii) `command-risk-evaluation.ja.md` のシステム管理系コマンドを Medium とする行から、**確定分類で High となる名前を削除**して
      High 側へ移すこと。
      **【計画からの修正・ground-truth 整合】**: 本項 (iii) は当初 `parted`/`fsck` のみを削除し `crontab`/`at`/`batch`/
      `chkconfig`/`update-rc.d` は Medium のままと想定していたが、実装（`internal/runner/base/security/command_analysis.go`
      の `highSystemModificationNames`）を確認した結果、`fdisk`/`parted`/`mkfs`/`fsck` に加え `crontab`/`at`/`batch`/
      `chkconfig`/`update-rc.d` も **High** であった（分類ガイド §4 とも一致）。よって Medium 行に残すのは `mount`/`umount`
      （＋ネットワーク設定 `ip`/`ifconfig` 系・LVM 作成系）のみとし、上記を High 側へ移した。確定分類との整合を優先する。
      旧記述がいずれの文書にも残っていないことを確認した（Step 3.5 grep ヒット 0）。
- [x] **0144/0145 の反映**: 検出限界・フラグ解析の節を、(i) 現行アーキテクチャ（宣言的フラグ仕様 `flag_spec.go` ＋単一 getopt
      パーサ＋完全性メタテスト）を反映し、(ii) 0145 で解消した過剰認識（実 CLI に無いフラグの受理）を旧制約として残さない
      よう更新する。fail-closed `Recognized` contract が安全保証を担う点は不変として明記する。（dev 新設節「宣言的フラグ仕様と
      単一 getopt パーサ（0144／0145）」に記載）
- [x] **operand_zones の追記**: `command-risk-evaluation.ja.md` の監査セクションへ `operand_zones` フィールドの説明と、
      [02_architecture.md](02_architecture.md) §2.2 の「3 状態デコード規則」（キー無し・error_class 有無による判別）を反映する。

**成功基準**: Step 3.5 の除去確認 grep が成功（旧記述ヒット 0）。分類表が確定分類と一致。

#### Step 3.3: 用語集の整合（F-004/AC-06）

**対象ファイル**: [docs/translation_glossary.md](../../../docs/translation_glossary.md)

- [x] `移行ノート`（English: `migration note`）を canonical エントリとして追加する（changelog の和語は「移行ノート」で統一）。
- [x] 本一連で用いた用語のうち未登録のものを追加する（`オペランド毎判定` 等）。`パス信頼区分`/`trust-critical`/`safe-zone`/
      `オペランド毎`(per-operand) は登録済みのため訳語の整合のみ確認する。

**成功基準**: Step 3.5 の用語集 grep が成功。

#### Step 3.4: 分類ガイドの最終化（F-006/AC-08, 日本語版）

**対象ファイル**: [docs/dev/architecture_design/risk-level-classification-guide.ja.md](../../../docs/dev/architecture_design/risk-level-classification-guide.ja.md)

- [x] ガイドの分類記述を確定挙動（判断軸1/判断軸2・max 合成・Critical 限定・safe-zone の安全要件）へ改訂し、AC-06 の分類表と
      齟齬が無いようにする。（§4 の High/Medium ファミリ・§5 ゾーン・§6 相互作用が実装と一致。`chroot`/`unshare`/`ip netns exec`
      等のラッパも実装に存在することを確認）
- [x] 冒頭の Document Status の Status を `draft` から確定状態（`approved`）へ遷移させる。

**成功基準**: Status が遷移済み。分類記述が AC-06 分類表と一致（Step 3.5 の静的確認＋目視）。

#### Step 3.5: Phase 3 静的検証（AC-04/AC-05/AC-06/AC-08）

- [x] 引き下げ独立ブロックの存在確認:
      `rg -n -e 'security relaxation' -e '引き下げ' docs/user/risk_assessment.ja.md`（独立見出し／警告ブロックを目視確認）。（§8.8 ✓）
- [x] 除去確認（旧記述ヒット 0 を期待）:
      `rg -n -e 'fdisk' -e 'mkfs' docs/user/risk_assessment.ja.md docs/dev/architecture_design/command-risk-evaluation.ja.md | rg -i 'medium'`
      および `parted`/`fsck` がシステム管理系→Medium 行に残らないことの確認。（0 ヒット ✓）
- [x] 用語集確認: `rg -n '移行ノート' docs/translation_glossary.md`（ヒット有りを期待）。（✓）
- [x] ガイド Status 確認: `rg -n 'Status' docs/dev/architecture_design/risk-level-classification-guide.ja.md`（`draft` でないことを確認）。（`approved` ✓）
- [x] 日本語版をコミットする（英語版は Phase 4）。

### Phase 4 — 英語版反映（F-004/F-006/AC-06/AC-08）

> 各日本語版の確定・コミット後に `/mktrans` で英語版へ反映する。

- [x] [risk_assessment.md](../../../docs/user/risk_assessment.md) を `/mktrans` で日本語版から反映する。
- [x] [command-risk-evaluation.md](../../../docs/dev/architecture_design/command-risk-evaluation.md) を `/mktrans` で反映する。
- [x] 英語版 `risk-level-classification-guide.md` を `/mktrans` で**新規作成**する（実装・日本語確定前には作成しない条件を満たす）。
- [x] 各英語版が日本語版と構造一致（見出し・段落構造）であることを確認する。

**成功基準**: AC-08 完了条件 (c)（英語版が存在し日本語版と構造一致）を満たす。

---

## 3. 実装順序とマイルストーン

| マイルストーン | 内容 | 対応 AC | 完了条件 |
|---|---|---|---|
| M1 | Phase 1（コード: 監査出力・family・deny e2e） | AC-01/AC-02/AC-03/NF-001 | `make test`/`make lint`/`make fmt` 緑 |
| M2 | Phase 2（sample/test config 追従） | AC-07 | 列挙 grep 対応済み・`make test` 緑 |
| M3 | Phase 3（日本語文書: 移行ノート・整合・用語集・ガイド） | AC-04/AC-05/AC-06/AC-08/NF-004 | Phase 3 静的検証成功・日本語版コミット |
| M4 | Phase 4（英語版反映） | AC-06/AC-08 | 英語版作成・構造一致 |

### PR 作成ポイント

#### PR-1 作成ポイント: audit operand_zones output and reason-code families

**対象ステップ**: Phase 1（Step 1.1〜1.6）
**推奨タイトル**: `feat(0143): emit operand_zones audit field and add reason-code families`
**レビュー観点**: `operand_zones` の存在条件（`len()>0`）・redaction 配線が唯一の制御である点の担保（漏えい否定テスト）／
family テーブルを唯一の列挙源とした並行リスト廃止／AC-02 e2e の配置（`package risk`）と既存テストとの非重複。

#### PR-2 作成ポイント: follow sample/test config to the new classification

**対象ステップ**: Phase 2
**推奨タイトル**: `feat(0143): follow sample and test config to the finalized risk classification`
**レビュー観点**: 引き上げ・fail-closed 化対象コマンドの grep 網羅性（判断軸1 High 名・判断軸2 書込形・0145 無効フラグ形）／意図せぬ deny が無いこと。

#### PR-3 作成ポイント: align docs and finalize classification guide (Japanese)

**対象ステップ**: Phase 3
**推奨タイトル**: `docs(0143): align risk docs, add migration note, finalize guide (ja)`
**レビュー観点**: 引き下げの独立提示（fail-silent 防止）／旧記述（fdisk/mkfs=Medium・rm/dd 無条件 High・parted/fsck=Medium）の
除去確認／NF-004 運用注記／用語集の canonical 統一。

#### PR-4 作成ポイント: english translations via mktrans

**対象ステップ**: Phase 4
**推奨タイトル**: `docs(0143): add english translations for risk docs and guide`
**レビュー観点**: `/mktrans` 由来で日英構造一致／英語版が日本語確定後に作成されている。

---

## 4. テスト戦略

設計の詳細は [02_architecture.md](02_architecture.md) §7 を参照する。本タスクの新規・更新テストは次のとおり。

**単体テスト（Phase 1）**

- `operand_zones` 出力（AC-01）: 値の一致・存在条件（空でキー無し）・deny 経路での出力。
- 漏えい否定（AC-01/S-1）: `key=value` 形式秘匿トークンが `[REDACTED]` に置換されること。裸のパス成分は受容済み残存リスクとして
  対象外（[02_architecture.md](02_architecture.md) §3.2）。
- family 区別（AC-03/NF-001）: family 割当の網羅・`FamilyOf` の返値・既存一意性テストの table 由来化。

**統合（end-to-end）テスト（Phase 1）**

- deny 時の理由コード記録（AC-02/S-3）: 実分類器のプランを監査出力へ流し、判断軸1（`insmod`）・判断軸2
  （`trust_boundary_write`）・危険引数（`dd if=`）の各 deny で `reason_codes`／`blocking_reason` を表明。

**config テスト（Phase 2）**

- sample/test config（AC-07）: 列挙 config が `make test` でロード・評価でき、意図せぬ deny が無いこと。

**文書検証（Phase 3/4）**

- 文書系 AC（AC-04/AC-05/AC-06/AC-08）は textual presence の静的確認（rg）を主とし、目視で独立提示・構造一致を補完する。

**既存テストとの非重複**: 配線の一般形（`audit_wiring_test.go` の `fixedPlanEvaluator` 駆動）と、既に表明済みの評価器レベル
理由コード（`evaluator_test.go` の `dpkg`/`systemctl`=`system_modification`）は再利用し、これらを重複させない。AC-02 e2e が対象
とする 3 リンク（`insmod`/`dd`/trust-critical 書込）は評価器レベルでも未表明のため、e2e が初の表明を兼ねる（§1.3・Step 1.5）。

---

## 5. リスク管理

| リスク | 影響 | 緩和策 |
|---|---|---|
| `operand_zones` のマスク漏れ（単一障害点） | 監査ログへの秘匿情報漏えい | 漏えい否定テスト（S-1）を必須化し、Layer 2 非再帰の前提を [02_architecture.md](02_architecture.md) §4 に明記済み |
| family テーブルへの新規コード追記漏れ | family 区別の不整合 | テーブルを唯一の列挙源とし、網羅・`FamilyOf` テストで検出（Go は const ブロックをリフレクションで列挙不可のため、追記は規約として残る既知の限界） |
| AC-07 の対象 config 洗い出し漏れ（fail-silent） | 緩和が無監査で本番投入 | コマンド名 grep による網羅列挙を完了条件に含める（判断軸1 High 名・判断軸2 書込形・0145 無効フラグ形を含む） |
| 先行タスク未マージでの着手 | コードと文書の齟齬 | 着手前提（0141/0142/0144/0145 マージ・`make test` 緑）を §1 で明示 |

---

## 6. 実装チェックリスト

- [x] Step 1.1: `operand_zones` 出力・redaction 配線（logger.go）
- [x] Step 1.2: `ReasonFamily`・family テーブル・`FamilyOf`（reason_codes.go）
- [x] Step 1.3: 網羅・family テスト更新（reason_codes_test.go）
- [x] Step 1.4: `operand_zones` 出力・漏えい否定テスト（logger_test.go）
- [x] Step 1.5: deny 理由コード e2e テスト（新規 audit_reason_codes_test.go）
- [x] Step 1.6: Phase 1 完了ゲート（fmt/test/lint）
- [x] Phase 2: sample/test config 追従と検証
- [x] Step 3.1: 移行ノート（引き上げ・引き下げ独立ブロック・NF-004・0139 上書き）
- [x] Step 3.2: ユーザー/開発者文書の整合（除去確認・0144/0145 反映・operand_zones 追記）
- [x] Step 3.3: 用語集（移行ノート canonical 追加）
- [x] Step 3.4: 分類ガイド最終化・Status 遷移（日本語版）
- [x] Step 3.5: Phase 3 静的検証・日本語版コミット
- [x] Phase 4: 英語版 3 文書を `/mktrans` で反映・新規作成

---

## 7. 受け入れ基準の検証

> 各 AC を実装タスクと検証手段へ対応づける。`test`=実行可能テスト、`static`=rg/コンパイル等の静的確認、`manual`=PR 観測。

| AC | 概要 | 対応タスク | 検証種別 | 検証手段（テスト位置 / コマンド・期待値） |
|---|---|---|---|---|
| AC-01 | operand_zones の logger 出力・存在条件・deny 出力 | Step 1.1/1.4 | test | `internal/runner/base/audit/logger_test.go::TestLogRiskProfile_OperandZones` |
| AC-01 | 秘匿マスク（漏えい否定 S-1） | Step 1.1/1.4 | test | `internal/runner/base/audit/logger_test.go::TestLogRiskProfile_OperandZoneMasking`（秘匿値が `[REDACTED]` へ置換） |
| AC-02 | deny 時の理由コード記録（end-to-end, 3 代表/S-3） | Step 1.5 | test | `internal/runner/base/risk/audit_reason_codes_test.go::TestLogRiskProfile_DenyReasonCodes_EndToEnd`（3 サブケースが各々 `system_modification`／`trust_boundary_write`／`dangerous_arg_pattern` の正確な定数を出力エントリで表明＝S-3） |
| AC-03 | family 区別の機械的根拠（割当網羅 S-2） | Step 1.2/1.3 | test | `internal/runner/base/risktypes/reason_codes_test.go::TestReasonFamily_AllCodesAssigned` / `::TestReasonFamily_OfReturnsAssignedFamily` |
| AC-04 | 移行周知・引き上げ（0145 fail-closed 含む） | Step 3.1 | static | `rg -n -e '引き上げ' -e 'fail-closed' -e 'recognized' docs/user/risk_assessment.ja.md`（引き上げ群と 0145 無効フラグ形の記述がヒット） |
| AC-04 | 引き上げ記述の内容妥当性 | Step 3.1 | manual | PR で allowlist+ハッシュ固定+明示 `risk_level` の併記を確認 |
| AC-05 | 移行周知・引き下げ（独立提示 B-2・NF-004） | Step 3.1 | static | `rg -n -e 'security relaxation' -e 'safe-zone' docs/user/risk_assessment.ja.md`（独立見出し／警告ブロックがヒット）＋ NF-004 ログレベル注記がヒット |
| AC-05 | 引き下げの視認上の埋没なし | Step 3.1 | manual | PR で独立見出し／警告ブロックの視認性を確認 |
| AC-06 | (b) 旧記述の除去（fdisk/mkfs=Medium・rm/dd 無条件 High・parted/fsck=Medium） | Step 3.2 | static | `rg -n -e 'fdisk' -e 'mkfs' -e 'parted' -e 'fsck' docs/user/risk_assessment.ja.md docs/dev/architecture_design/command-risk-evaluation.ja.md \| rg -i 'medium'` 期待: 確定分類と矛盾するヒット 0 |
| AC-06 | (a) 確定分類の全コマンド反映 / (c) 0139 上書き明示 | Step 3.2/3.1 | manual | PR で分類表の網羅と移行ノートの 0139 上書き記述を確認 |
| AC-07 | sample/test config の追従と網羅列挙 | Phase 2 | static | `rg -n --glob '*.toml' -e 'cmd\s*=\s*"(/[^"]*/)?(rm&#124;dd&#124;shred&#124;unlink&#124;rmdir&#124;mkdir&#124;touch&#124;mv&#124;sponge&#124;ln&#124;fdisk&#124;mkfs&#124;insmod&#124;parted&#124;fsck&#124;cp&#124;rsync&#124;chmod&#124;chown)"' sample cmd internal` 期待: 各ヒットに `risk_level` 付与済み or 意図結果を確認済み |
| AC-07 | config がロード・評価でき意図せぬ deny なし | Phase 2 | test | `make test`（config をロードする統合テスト全体が緑） |
| AC-08 | (b) ガイド Status の確定遷移 | Step 3.4 | static | `rg -n 'Status' docs/dev/architecture_design/risk-level-classification-guide.ja.md` 期待: `draft` でない |
| AC-08 | (c) 英語版の存在 | Phase 4 | static | `test -f docs/dev/architecture_design/risk-level-classification-guide.md` 期待: 存在 |
| AC-08 | (a) ガイド分類記述が AC-06 と齟齬なし / (c) 構造一致 | Step 3.4/Phase 4 | manual | PR でガイドと分類表の突き合わせ・日英構造一致を確認 |
| NF-001 | 理由コード網羅・一意の最終化 | Step 1.3 | test | `internal/runner/base/risktypes/reason_codes_test.go::TestReasonCodes_AllDistinct`（table 由来） |
| NF-002 | `make test`/`make lint`/`make fmt` 成功 | Step 1.6/全体 | static | `make test && make lint`（緑）・`make fmt`（差分なし） |
| NF-003 | 監査経路に FS 副作用・live identity を持ち込まない | Step 1.1 | static | `rg -n -e 'os.Geteuid' -e 'os.Getuid' -e 'user.Current' internal/runner/base/audit/logger.go` 期待: `LogRiskProfile` の `operand_zones`／redaction 経路に live-identity 参照を新規追加していない（既存の `user_id`/`effective_user_id` 出力は不変） |
| NF-004 | 引き下げ allow-Low の監査証跡消失の運用注記 | Step 3.1 | static | `rg -n -e 'Debug' -e 'ログレベル' docs/user/risk_assessment.ja.md`（AC-05 ブロック内に運用注記がヒット） |
| AC-28 | runtime==dry-run の決定性 | — | — | N/A（本タスクは runtime のパス/identity 評価を追加せず監査出力・文書のみ。[02_architecture.md](02_architecture.md) §6.2・[01_requirements.md](01_requirements.md) NF-003 により自明に充足） |

---

## 8. 成功基準

- **機能完全性**: AC-01〜AC-08 がすべて §7 の検証手段で確認済み。
- **品質**: `make test`・`make lint` が緑、`make fmt` 差分なし（NF-002）。
- **セキュリティ**: 漏えい否定テスト（S-1）が緑で、`operand_zones` の秘匿パス文字列が境界 redaction を経由する。
- **文書完全性**: 日本語版確定後に英語版を `/mktrans` で反映し、日英構造一致。用語集に `移行ノート` を canonical 登録。

---

## 9. クロスサーチ・チェックリスト

`make lint`/`make test` が検出できない残存参照・整合のみを対象とする（§7 の AC 検証表と重複する rg は再掲しない）。

- [x] `TestReasonCodes_AllDistinct` のハードコード `all` リスト削除後、同リストへの残存参照が無いこと:
      `rg -n 'all := \[\]ReasonCode' internal/runner/base/risktypes/` 期待: ヒット 0。（✓ Phase 1）
- [x] 新規シンボル名の衝突確認（汎用名のため）:
      `rg -n -e 'ReasonFamily' -e 'func FamilyOf' --type go` 期待: 定義は reason_codes.go の 1 箇所のみ。（✓ Phase 1）
- [x] 文書・用語集間の訳語整合: `移行ノート`/`オペランド毎判定` が用語集と各文書で一致して使われていること（目視＋
      `rg -n 'changelog' docs/` で旧和語の混在が無いことを確認）。（✓ 公開文書に旧和語混在なし）

---

## 10. 次のステップ

- 本実装計画書のレビューと `approved` への更新（人手）。
- `approved` 後、Phase 1 から実装に着手する。
