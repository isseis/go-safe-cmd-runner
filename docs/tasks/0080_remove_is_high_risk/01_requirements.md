# `SyscallSummary.IsHighRisk` 廃止・`HighRiskReasons` リネーム 要件定義書

## 1. 概要

### 1.1 背景

`record` コマンドはバイナリを静的解析し、その事実（syscall の有無・種類・引数評価結果など）を
JSON ファイルとして記録する。後に `runner` が記録を参照してリスク判断を行う。

現在の `common.SyscallSummary` には `IsHighRisk bool` フィールドが存在し、
`record` 側の `filevalidator` パッケージがこのフィールドに値を書き込んで保存している。
しかし「高リスクかどうか」の判断は runner の責務であり、record の責務は事実の記録に留まるべきである。

この混入により以下の問題が生じている：

- `common.SyscallSummary`（共有型）がリスク判断ロジックを内包し、責務が不明確になっている
- `IsHighRisk` は既存の `HasUnknownSyscalls`（bool）と `ArgEvalResults`（`EvalMprotectRisk` による評価）から
  完全に再計算可能であり、冗長なキャッシュフィールドに過ぎない
- リスク判定基準を変更するたびに、`common` パッケージの変更と JSON スキーマのバージョン更新が
  発生するというコスト構造になっている

あわせて、`SyscallAnalysisResultCore` の `HighRiskReasons []string` フィールドも問題を抱えている。
このフィールドが保持する内容は「mprotect PROT_EXEC を検出した」「syscall 番号が確定できなかった」
という**解析上の観察事実**であるにもかかわらず、フィールド名に "HighRisk"（リスク評価語）が含まれており、
責務分離の観点で不整合がある。

なお、タスク 0078 では「`IsHighRisk` 廃止は変更コストが大きく現タスクには不均衡」として
選択肢 D を不採用とした（`01_requirements.md` §5「選択肢 D」参照）。
本タスクは 0078 の完了後に蓄積した知見を活かし、改めてこの廃止を専任タスクとして実施する。

### 1.2 目的

1. `common.SyscallSummary` から `IsHighRisk` フィールドを削除し、
   リスク判定ロジックを `elfanalyzer` パッケージ（runner 側）に完全に閉じ込める。
2. `HighRiskReasons` フィールドを `AnalysisWarnings` にリネームし、
   フィールド名を「解析上の観察事実・注意事項」という実態に合わせる。

これら2つの変更により、事実の記録とリスク判断を明確に分離する。

### 1.3 スコープ

- **対象**: `common.SyscallSummary.IsHighRisk` フィールドの削除
- **対象**: `filevalidator` パッケージでの `IsHighRisk` 設定ロジックの削除
- **対象**: `elfanalyzer` パッケージでの `IsHighRisk` 参照箇所の置き換え
- **対象**: `HighRiskReasons` → `AnalysisWarnings` へのリネーム（Go フィールド名・JSON キー）
- **対象**: JSON スキーマバージョンの更新
- **対象外**: リスク判定ロジック自体の変更（判断基準は変えない）
- **対象外**: `AnalysisWarnings` に格納する内容・生成ロジックの変更
- **対象外**: `HasUnknownSyscalls` の意味・内容の変更

## 2. 用語定義

| 用語 | 定義 |
|------|------|
| 事実の記録（fact recording） | バイナリ解析で観察された事実をそのまま保存すること。例: `HasUnknownSyscalls`、`AnalysisWarnings`、`ArgEvalResults` |
| リスク判断（risk judgment） | 記録された事実をもとに「高リスクか否か」を評価すること。runner の責務 |
| `IsHighRisk` | `common.SyscallSummary` の bool フィールド。廃止対象 |
| `HighRiskReasons` | `SyscallAnalysisResultCore` の `[]string` フィールド。`AnalysisWarnings` にリネーム対象 |
| `AnalysisWarnings` | `HighRiskReasons` のリネーム後の名称。解析上の観察事実・注意事項を保持する |
| スキーマバージョン | `fileanalysis` パッケージが管理する JSON 解析結果ファイルのバージョン番号 |

## 3. 機能要件

### 3.1 `IsHighRisk` フィールドの廃止

#### FR-3.1.1: `common.SyscallSummary` からの削除

`internal/common/syscall_types.go` の `SyscallSummary` 構造体から
`IsHighRisk bool` フィールドを削除すること。

#### FR-3.1.2: `filevalidator` での参照削除

`internal/filevalidator/validator.go` において `IsHighRisk` への代入を削除すること。
変更後、`filevalidator` が `SyscallSummary` に設定するフィールドは
`HasNetworkSyscalls`・`TotalDetectedEvents`・`NetworkSyscallCount` の3つのみとなる。
`HasUnknownSyscalls` は `SyscallSummary` ではなく `SyscallAnalysisResultCore` レベルの
フィールドであり、引き続き `filevalidator` が設定するが、これはリスク判断ではなく事実の記録である。

### 3.2 `HighRiskReasons` → `AnalysisWarnings` リネーム

#### FR-3.2.1: Go フィールド名の変更

`internal/common/syscall_types.go` の `SyscallAnalysisResultCore` 構造体において、
`HighRiskReasons []string` フィールドを `AnalysisWarnings []string` にリネームすること。

JSON タグも合わせて `json:"high_risk_reasons,omitempty"` から
`json:"analysis_warnings,omitempty"` に変更すること。

フィールドの意味・`omitempty` 動作・nil と空スライスの区別は変更しないこと。

#### FR-3.2.2: 全参照箇所の更新

コードベース全体で `HighRiskReasons` を参照している箇所（代入・読み取り・テストのアサーション等）を
すべて `AnalysisWarnings` に更新すること。

### 3.3 `elfanalyzer` でのリスク判定の置き換え

#### FR-3.3.1: `convertSyscallResult` の変更

`internal/runner/security/elfanalyzer/standard_analyzer.go` の `convertSyscallResult` 関数において、
`result.Summary.IsHighRisk` の参照を以下の導出条件に置き換えること：

```
result.HasUnknownSyscalls || EvalMprotectRisk(result.ArgEvalResults)
```

`AnalysisWarnings` は人間可読の説明文であり判定条件には使わないこと。
説明文の整理・変更がリスク判定の挙動に影響しないよう、判定は一次事実（`HasUnknownSyscalls` および `ArgEvalResults`）のみに依拠すること。

この条件は現行の `IsHighRisk` と意味的に同値である（下表参照）。

| 要因 | 現行 `IsHighRisk = true` | 置き換え後の条件 |
|------|--------------------------|-----------------|
| syscall 番号不明 | `HasUnknownSyscalls = true` により設定 | `HasUnknownSyscalls` |
| mprotect `exec_confirmed` / `exec_unknown` | `EvalMprotectRisk` → `true` → `IsHighRisk = true` | `EvalMprotectRisk(ArgEvalResults)` |

あわせて、`convertSyscallResult` 関数のドキュメントコメント（`standard_analyzer.go` L347–352）に
ある `IsHighRisk` の説明行（「`IsHighRisk: true if any syscall number could not be determined ...`」）を
削除し、判定ロジックをコメントで正確に記述すること。

#### FR-3.3.2: `analyzeSyscallsInCode` での `IsHighRisk` 設定削除とコメント修正

`internal/runner/security/elfanalyzer/syscall_analyzer.go` の `analyzeSyscallsInCode` 関数において、
`result.Summary.IsHighRisk` への代入をすべて削除すること。

`EvalMprotectRisk` 関数はリスクの有無を返す補助ロジックであるが、
その結果を `IsHighRisk` に格納するのではなく、`AnalysisWarnings` への追加のみに使用するよう変更する。
ただし `EvalMprotectRisk` 関数自体の削除・改名は本タスクのスコープ外とする。

あわせて、`analyzeSyscallsInCode` 末尾のビルドサマリーコメントブロック（`syscall_analyzer.go` L355 付近）から
`IsHighRisk` への言及を削除すること。

### 3.4 JSON スキーマの更新

#### FR-3.4.1: スキーマバージョンの更新

`is_high_risk` フィールドの削除および `high_risk_reasons` → `analysis_warnings` への
JSON キー変更に伴い、`fileanalysis` パッケージの `CurrentSchemaVersion` を
現在の値（5）から 6 にインクリメントすること。

#### FR-3.4.2: 旧バージョンの無効化

スキーマバージョン不一致時の既存動作（解析結果を無効化して再解析を要求する
`SchemaVersionMismatchError`）をそのまま維持すること。
旧バージョンの JSON に `is_high_risk` / `high_risk_reasons` フィールドが含まれていても、
再解析によって正しい結果が得られること。

## 4. 非機能要件

### 4.1 後方互換性

#### NFR-4.1.1: リスク判定の同値性

変更前後でリスク判定の結果が変わらないこと。すなわち、
変更前に `IsHighRisk = true` となっていたすべてのケースで、
変更後の `convertSyscallResult` も `AnalysisError` を返すこと。

#### NFR-4.1.2: テスト継続パス

`make test` がすべてパスすること。

### 4.2 コードの明確性

#### NFR-4.2.1: 責務の分離

変更後のコードを読んだとき、`filevalidator`（record 側）が
「事実のみを記録している」ことが明確であること。
具体的には、`filevalidator` の `SyscallSummary` 設定箇所に
リスク判断に関連する変数名・コメントが残らないこと。

#### NFR-4.2.2: フィールド名と実態の一致

`AnalysisWarnings` というフィールド名を見たとき、
「解析中に観察された注意事項（事実の記述）」であることが名前から直接読み取れること。

## 5. 変更ファイル一覧

### 5.1 本体ファイル

| ファイル | 変更内容 |
|----------|----------|
| `internal/common/syscall_types.go` | `SyscallSummary.IsHighRisk` フィールドを削除。`HighRiskReasons` フィールドを `AnalysisWarnings` にリネーム（JSON タグも `analysis_warnings` に変更） |
| `internal/filevalidator/validator.go` | `IsHighRisk` への代入を削除。`HighRiskReasons` → `AnalysisWarnings` に更新 |
| `internal/runner/security/elfanalyzer/syscall_analyzer.go` | `result.Summary.IsHighRisk` への代入を全削除。`HighRiskReasons` → `AnalysisWarnings` に更新。L355 のビルドサマリーコメントブロックから `IsHighRisk` への言及を削除 |
| `internal/runner/security/elfanalyzer/standard_analyzer.go` | `result.Summary.IsHighRisk` の参照を `HasUnknownSyscalls \|\| EvalMprotectRisk(ArgEvalResults)` に置き換え。`HighRiskReasons` → `AnalysisWarnings` に更新。`convertSyscallResult` 関数のドキュメントコメント（L347–352）から `IsHighRisk` の説明行を削除 |
| `internal/runner/security/elfanalyzer/mprotect_risk.go` | `EvalMprotectRisk` 関数のコメント（L6）を「`IsHighRisk` に設定すべきか」から「mprotect 由来のリスクが存在するか（`AnalysisWarnings` への追加判断および `convertSyscallResult` でのリスク判定に使用）」に更新 |
| `internal/fileanalysis/schema.go` | `CurrentSchemaVersion` を 5 → 6 にインクリメント |

### 5.2 テストファイル

| ファイル | 変更内容 |
|----------|----------|
| `internal/common/syscall_types_test.go` | `TestSyscallSummary_JSONRoundTrip`（L87–101）・`TestSyscallAnalysisResultCore_JSONRoundTrip`（L106–135）から `IsHighRisk` フィールドの設定・アサーションを削除。`HighRiskReasons` → `AnalysisWarnings` に更新 |
| `internal/filevalidator/validator_test.go` | `TestBuildSyscallAnalysisData` 内の `IsHighRisk` アサーション（L1314、L1326）を削除。`HighRiskReasons` → `AnalysisWarnings` に更新 |
| `internal/fileanalysis/syscall_store_test.go` | 保存・ロードのラウンドトリップテスト（L28–64、L151–193）および ArgEvalResults ラウンドトリップテスト（L328–358）から `IsHighRisk` フィールドの設定・アサーションを削除。`HighRiskReasons` → `AnalysisWarnings` に更新 |
| `internal/runner/security/elfanalyzer/syscall_analyzer_test.go` | `result.Summary.IsHighRisk` への参照（L163、L194 等）を `EvalMprotectRisk` または `HasUnknownSyscalls` を使った等価な確認に置き換え。`exec_not_set does not overwrite pre-existing IsHighRisk=true` テスト（L871–890）の検証方法を `HasUnknownSyscalls` のみの確認に変更。`HighRiskReasons` → `AnalysisWarnings` に更新 |
| `internal/runner/security/elfanalyzer/analyzer_test.go` | モックストアが返す `SyscallAnalysisResult` の `Summary.IsHighRisk` 設定（L376、L422、L593）を削除。`HighRiskReasons` → `AnalysisWarnings` に更新。`convertSyscallResult` がキャッシュ済みデータを読む経路のテストを、`HasUnknownSyscalls` / `ArgEvalResults` から正しくリスク判定されることの確認に更新（詳細は §5.3 参照）|
| `internal/fileanalysis/file_analysis_store_test.go` | L143 の `HighRiskReasons` フィールドを `AnalysisWarnings` に更新 |
| `internal/runner/security/elfanalyzer/syscall_analyzer_integration_test.go` | L398 の `result.Summary.IsHighRisk` 参照を削除またはコメントアウト（ログ出力なので `HasUnknownSyscalls` への置き換えも可）|

### 5.3 キャッシュ済みデータ読み取り経路の考慮

`StandardELFAnalyzer.convertSyscallResult` はリアルタイム解析結果だけでなく、
ストアからロードしたキャッシュ済みデータも処理する経路（`lookupSyscallAnalysis` → `LoadSyscallAnalysis` → `convertSyscallResult`）で呼ばれる。

変更後、ロード済みの `SyscallAnalysisResult` には `IsHighRisk` フィールドが存在しない。
`convertSyscallResult` はロードされた `HasUnknownSyscalls` および `ArgEvalResults` のみを使って
リスク判定を行わなければならない。

`analyzer_test.go` のモックストア使用テストは、この経路を模倣する（ストアが返すデータを
`convertSyscallResult` が処理する）。したがって、モックデータ構築時に `IsHighRisk` を
設定しなくても `AnalysisError` が正しく返ることを検証するようテストを更新すること。

## 6. 受け入れ条件

### AC-1: `IsHighRisk` フィールド廃止（型定義）

- [ ] `common.SyscallSummary` に `IsHighRisk` フィールドが存在しないこと
  - `internal/common/syscall_types.go` の `SyscallSummary` 構造体を目視確認
- [ ] `make build` がエラーなく完了すること（コンパイルエラーがないこと）
- [ ] `TestSyscallSummary_JSONRoundTrip`（`syscall_types_test.go` L87–101）が `IsHighRisk` なしで通過すること
- [ ] `TestSyscallAnalysisResultCore_JSONRoundTrip`（`syscall_types_test.go` L106–135）が `IsHighRisk` なしで通過すること

### AC-2: `HighRiskReasons` → `AnalysisWarnings` リネーム

- [ ] `common.SyscallAnalysisResultCore` のフィールド名が `AnalysisWarnings` になっていること
- [ ] JSON キーが `analysis_warnings` になっていること
- [ ] **Go ソースおよびテストファイル**（`**/*.go`）に `HighRiskReasons` という識別子が残っていないこと
  - `grep -r HighRiskReasons --include='*.go' .` でヒットなしを確認
  - 注: Markdown ドキュメント（`docs/` 以下）は対象外。過去タスクの要件定義書等に残存しても不問
- [ ] `TestSyscallAnalysisResultCore_JSONRoundTrip` の `"high_risk_reasons omitted when nil"` サブテストが
  `analysis_warnings` キーを対象として通過すること

### AC-3: `filevalidator` の責務限定

- [ ] `internal/filevalidator/validator.go` において `IsHighRisk` という識別子が一切使われないこと
- [ ] `filevalidator` が構築する `SyscallSummary` に含まれるフィールドが
  `HasNetworkSyscalls`・`TotalDetectedEvents`・`NetworkSyscallCount` のみであること
- [ ] `TestBuildSyscallAnalysisData`（`validator_test.go` L1314、L1326）において
  `IsHighRisk` に対するアサーションが存在せず、かつテストが通過すること

### AC-4: リスク判定の同値性（`elfanalyzer` リアルタイム解析経路）

- [ ] `HasUnknownSyscalls = true` かつ `ArgEvalResults` 空のケースで
  `analyzeSyscallsInCode` の結果が `HasUnknownSyscalls = true` となること
  - `TestSyscallAnalyzer_*` の該当テストが引き続きパスすること
- [ ] `EvalMprotectRisk(ArgEvalResults) = true`（mprotect `exec_confirmed` / `exec_unknown`）のケースで
  `AnalysisWarnings` にエントリが追加されていること
  - `TestSyscallAnalyzer_MultipleMprotect` 等が引き続きパスすること
- [ ] `exec_not_set` のみのケースで `HasUnknownSyscalls = false` かつ `AnalysisWarnings` が空であること
  - `syscall_analyzer_test.go` L860–869 相当のテストが通過すること
- [ ] `exec_not_set` のみで、かつ別要因（`HasUnknownSyscalls`）で高リスクのケースでは
  `HasUnknownSyscalls = true` が維持されること
  - `syscall_analyzer_test.go` L871–890（`exec_not_set does not overwrite`）相当のテストが通過すること

### AC-5: リスク判定の同値性（`elfanalyzer` キャッシュ読み取り経路）

`StandardELFAnalyzer.lookupSyscallAnalysis` → `convertSyscallResult` の経路において、
ストアから読み込んだ `SyscallAnalysisResult`（`IsHighRisk` フィールドなし）を正しく処理できること。

- [ ] `HasUnknownSyscalls = true`・`ArgEvalResults` 空のデータを返すモックストアを使った場合に
  `AnalysisError` が返ること
  - `analyzer_test.go` L355–389（`TestStandardELFAnalyzer_SyscallLookup_HighRisk*`）相当が
    `Summary.IsHighRisk` 設定なしのモックデータで通過すること
- [ ] `HasUnknownSyscalls = true`・ネットワーク syscall ありのデータを返すモックの場合に
  `AnalysisError`（`ErrSyscallAnalysisHighRisk`）が返ること（ネットワーク検出より高リスク優先）
  - `analyzer_test.go` L391–436（`TestStandardELFAnalyzer_SyscallLookup_HighRiskTakesPrecedenceOverNetwork`）
    相当が `Summary.IsHighRisk` 設定なしのモックデータで通過すること
- [ ] `ArgEvalResults` に `exec_confirmed` を含むデータを返すモックの場合に
  `AnalysisError` が返ること
  - `analyzer_test.go` L571–593（mprotect 高リスクのモック）相当が通過すること
- [ ] `HasUnknownSyscalls = false`・`ArgEvalResults` 空のデータでは `AnalysisError` が返らないこと
  - ネットワーク syscall ありなら `NetworkDetected`、なしなら `StaticBinary` が返ること

### AC-6: JSON スキーマ更新

- [ ] `CurrentSchemaVersion` が 6 になっていること
- [ ] 旧バージョンの JSON を読み込んだとき `SchemaVersionMismatchError` が返されること
  - 既存テスト `TestStore_SchemaVersionMismatch` が引き続きパスすること
- [ ] 新バージョンの JSON に `is_high_risk` フィールドが含まれないこと
- [ ] 新バージョンの JSON で `analysis_warnings` キーが使われ、`high_risk_reasons` キーが存在しないこと
  - `syscall_store_test.go` L28–64（基本ラウンドトリップ）が通過すること
  - `syscall_store_test.go` L151–193（`AnalysisWarnings` のラウンドトリップ）が通過すること
  - `syscall_store_test.go` L328–358（ArgEvalResults ラウンドトリップ）が通過すること

### AC-7: 全テスト通過

- [ ] `make test` がすべてパスすること
- [ ] `make lint` がエラーなく完了すること

## 7. 設計上の考慮事項

### 7.1 `EvalMprotectRisk` 関数の扱い

`elfanalyzer.EvalMprotectRisk` 関数は「`ArgEvalResults` を受け取り mprotect 由来のリスク有無を返す」
ものである。`IsHighRisk` 廃止後も、この関数の戻り値は `AnalysisWarnings` へのエントリ追加判断に
引き続き使える。

関数の削除・改名は本タスクのスコープ外とするが、現行コメント（`mprotect_risk.go` L6）：

```
Returns true if IsHighRisk should be set based on mprotect detection.
```

は `IsHighRisk` フィールド削除後に不正確になるため、
「mprotect 由来のリスクが存在するか（`AnalysisWarnings` への追加判断および `convertSyscallResult` でのリスク判定に使用）」
を表すコメントに更新すること。

### 7.2 `IsHighRisk` の意味的同値性の証明

#### 7.2.1 `elfanalyzer` 側

現行の `IsHighRisk` は以下の式で設定される（`analyzeSyscallsInCode` 末尾）：

```
IsHighRisk = EvalMprotectRisk(ArgEvalResults) || HasUnknownSyscalls
```

置き換え後の `convertSyscallResult` の条件もまったく同じ式になる：

```
HasUnknownSyscalls || EvalMprotectRisk(result.ArgEvalResults)
```

`AnalysisWarnings` は現行実装では `EvalMprotectRisk` が `true` になるケースと
連動してエントリが追加されるが、これは偶然の一致であって設計上の保証ではない。
`AnalysisWarnings` を判定条件に使うと「説明文の整理 → 判定変化」という壊れやすい設計になるため、
判定は一次事実（`ArgEvalResults` を直接評価する `EvalMprotectRisk` と `HasUnknownSyscalls`）のみに依拠する。

#### 7.2.2 `filevalidator` 側の不一致（削除によって解消される既存問題）

`filevalidator`（`validator.go:778`）は現行、`IsHighRisk = hasUnknown`（`HasUnknownSyscalls` のみ）で設定しており、
`elfanalyzer` 側の `EvalMprotectRisk(ArgEvalResults) || HasUnknownSyscalls` とは同値でない。
すなわち `filevalidator` が生成する `IsHighRisk` は mprotect リスクを反映しておらず、
`elfanalyzer` が解析したキャッシュと内容が食い違う状態が既に存在している。

`IsHighRisk` を廃止することでこの不整合も同時に解消される。
本タスクで新たに `filevalidator` に mprotect 判定を追加する必要はない（スコープ外）。

### 7.3 スキーマバージョンについて

`is_high_risk` フィールドの削除および `high_risk_reasons` → `analysis_warnings` への JSON キー変更は
いずれも後方非互換変更である。スキーマバージョンを更新し、旧バージョンのキャッシュを
強制的に再生成させる必要がある。
ただし `is_high_risk` は `HasUnknownSyscalls` と `ArgEvalResults`（`EvalMprotectRisk` による評価）から
再計算可能であるため、再解析なしに移行できる可能性もあるが、コードの単純性のため
既存の「バージョン不一致 → 再解析」の仕組みをそのまま利用する。
