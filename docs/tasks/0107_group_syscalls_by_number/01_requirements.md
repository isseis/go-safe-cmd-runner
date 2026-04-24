# 要件定義書: detected_syscalls のシステムコール番号単位グループ化

## 1. 概要

### 1.1 背景

現行の `detected_syscalls` フィールドはフラットなリスト構造であり、同一のシステムコール（同一番号）が複数箇所で検出された場合、`number`・`name`・`is_network` という共通属性を各エントリが個別に保持する。

例：`ioctl`（番号 54）が 3 箇所で検出された場合の現行 JSON

```json
"detected_syscalls": [
  {"number": 54, "name": "ioctl", "is_network": false, "location": 4305769496, "determination_method": "go_wrapper"},
  {"number": 54, "name": "ioctl", "is_network": false, "location": 4305769628, "determination_method": "go_wrapper"},
  {"number": 54, "name": "ioctl", "is_network": false, "location": 4305769824, "determination_method": "go_wrapper"}
]
```

この構造には以下の問題がある。

**問題 1: リスク判定と無関係な情報の混在**

`location`（命令アドレス）と `determination_method`（番号解決手法）はデバッグ用ヒント情報であり、リスク判定に使用されない。一方、`number`・`name`・`is_network` はリスク判定の根拠である。この 2 種類の情報が同一レベルに混在しており、スキーマ上の意味的分離ができていない。

**問題 2: 同一番号の重複**

同一の `number`・`name`・`is_network` を持つエントリが出現箇所の数だけ繰り返されるため、JSONのサイズがバイナリ内の出現回数に比例して増大する。リスク判定上はシステムコール番号の「集合」のみが必要であり、出現箇所の多寡は関係しない。

### 1.2 目的

- `detected_syscalls` をシステムコール番号単位にグループ化し、リスク判定根拠（`number`・`name`・`is_network`）と出現箇所のヒント情報（`location`・`determination_method`・`source`）を分離する
- JSON の空間効率と意味的明確さを向上させる

### 1.3 変更後の JSON 構造

```json
"detected_syscalls": [
  {
    "number": 1,
    "name": "exit",
    "is_network": false,
    "occurrences": [
      {"location": 4295490180, "determination_method": "immediate"}
    ]
  },
  {
    "number": 54,
    "name": "ioctl",
    "is_network": false,
    "occurrences": [
      {"location": 4305769496, "determination_method": "go_wrapper"},
      {"location": 4305769628, "determination_method": "go_wrapper"},
      {"location": 4305769824, "determination_method": "go_wrapper"}
    ]
  }
]
```

同一番号が複数箇所で検出された場合でも、`number`・`name`・`is_network` は 1 回だけ記録される。出現箇所の詳細は `occurrences` 配列にまとめられる。

### 1.4 スコープ

#### 対象

- `internal/common/syscall_types.go`：`SyscallInfo` 型の再設計、新型 `SyscallOccurrence` の追加
- `internal/fileanalysis/syscall_store.go`：保存時のグループ化ロジック（`SaveSyscallAnalysis` 内のソート処理を含む）
- `internal/filevalidator/validator.go`：`buildMachoSyscallData` の `DeterminationMethod` 参照をグループ化後の構造に合わせた修正
- `internal/runner/security/network_analyzer.go`：`syscallAnalysisHasSVCSignal` の `DeterminationMethod` 参照をグループ化後の構造に合わせた修正
- `internal/runner/security/elfanalyzer/`・`internal/runner/security/machoanalyzer/`：内部中間型（`SyscallInfo` を使用している箇所）の更新
- スキーマバージョンの更新（v16 → v17）
- 上記に伴うテストの更新

#### 対象外

- リスク判定ロジック（`IsNetwork` や未解決 svc の判定方針）の変更。本タスクは構造の再編であり、判定結果は変わらない
- `SymbolAnalysis`・`DynLibDeps` など他のフィールドの変更
- `libccache` パッケージのスキーマ（`LibcCacheFile`）の変更

## 2. 用語定義

| 用語 | 定義 |
|------|------|
| グループエントリ | 同一の `number` を持つシステムコールをまとめた `SyscallInfo` 1 件。`number`・`name`・`is_network` を持つ |
| 出現（Occurrence） | 1 つのグループエントリ内の個別検出箇所。`location`・`determination_method`・`source` を持つ |
| 未解決 svc | `Number == -1` かつ出現の `determination_method == "direct_svc_0x80"` であるエントリ。リスク判定上の高リスクシグナル |
| 解決済み svc | `Number != -1` かつ出現の `determination_method == "direct_svc_0x80"` であるエントリ |

## 3. 機能要件

### FR-1: `SyscallOccurrence` 型の新設

`internal/common/syscall_types.go` に新型 `SyscallOccurrence` を追加する。

```go
// SyscallOccurrence represents a single detected location of a syscall.
// It holds diagnostic hint information that is not used for risk assessment.
type SyscallOccurrence struct {
    // Location is the virtual address of the syscall instruction.
    Location uint64 `json:"location"`

    // DeterminationMethod describes how the syscall number was determined.
    DeterminationMethod string `json:"determination_method"`

    // Source describes how this syscall was detected.
    // Empty string (omitted in JSON) means detection via direct syscall instruction.
    Source string `json:"source,omitempty"`
}
```

### FR-2: `SyscallInfo` 型の再設計

`internal/common/syscall_types.go` の `SyscallInfo` から `Location`・`DeterminationMethod`・`Source` フィールドを削除し、代わりに `Occurrences []SyscallOccurrence` を追加する。

変更前：

```go
type SyscallInfo struct {
    Number              int    `json:"number"`
    Name                string `json:"name,omitempty"`
    IsNetwork           bool   `json:"is_network"`
    Location            uint64 `json:"location"`
    DeterminationMethod string `json:"determination_method"`
    Source              string `json:"source,omitempty"`
}
```

変更後：

```go
// SyscallInfo represents a unique syscall detected in a binary.
// Each entry corresponds to a single syscall number; all occurrences at
// different addresses are collected in Occurrences.
type SyscallInfo struct {
    // Number is the syscall number. -1 if unresolved.
    Number int `json:"number"`

    // Name is the human-readable syscall name. Empty if unknown.
    Name string `json:"name,omitempty"`

    // IsNetwork indicates whether this syscall is network-related.
    IsNetwork bool `json:"is_network"`

    // Occurrences holds the individual detected locations.
    // This is hint information for debugging and is not used for risk assessment.
    Occurrences []SyscallOccurrence `json:"occurrences,omitempty"`
}
```

### FR-3: 保存時のグループ化ロジック

`internal/fileanalysis/syscall_store.go` の `SaveSyscallAnalysis` において、アナライザーが出力した `[]SyscallInfo`（各エントリが 1 出現を表す従来形式）を番号単位にグループ化してから保存する。

グループ化の規則：

1. 同一 `Number` のエントリをまとめ、1 つのグループエントリとする
2. グループエントリの `Name`・`IsNetwork` は最初のエントリの値を使用する（同一番号のエントリ間では一致することが保証される）
3. 各エントリの `Location`・`DeterminationMethod`・`Source` は `SyscallOccurrence` に変換して `Occurrences` に格納する
4. グループエントリは `Number` 昇順でソートする（`Number == -1` は末尾）
5. 各グループ内の `Occurrences` は `Location` 昇順でソートする

アナライザー（elfanalyzer・machoanalyzer）は引き続き出現ごとに 1 エントリを生成する中間形式を使用しても構わない。グループ化は保存ゲートウェイである `SaveSyscallAnalysis` でのみ行う。

> **設計上の注意**：FR-2 適用後、アナライザーが `SyscallInfo` を中間型として使い続ける場合の表現は「出現ごとに 1 `SyscallInfo`」とし、その `Occurrences` には当該出現を表す 1 要素のみを格納する。すなわち、グループ化前の中間データでも `Occurrences` を空にしてはならない。各要素の `Location`・`DeterminationMethod`・`Source` などの出現単位情報は、その 1 要素の `Occurrences` に保持する。`SaveSyscallAnalysis` はこれらの中間エントリを syscall 番号（および同一 syscall を識別する共通属性）で束ね、永続化用のグループ化済み `SyscallInfo` に変換する。

### FR-4: `validator.go` の `buildMachoSyscallData` 修正

`DeterminationMethod` が `SyscallInfo` から削除されるため、`internal/filevalidator/validator.go` の `buildMachoSyscallData` を修正する。

#### FR-4.1: `AnalysisWarnings` 生成ロジックの修正

現行は `SyscallInfo.DeterminationMethod` と `SyscallInfo.Number` を直接参照して未解決 svc を検出している。グループ化後は各グループエントリの `Occurrences` を走査して判定する。

変更前:

```go
for _, s := range merged {
    if s.DeterminationMethod == common.DeterminationMethodDirectSVC0x80 && s.Number == -1 {
        warnings = ...
        break
    }
}
```

変更後:

```go
for _, s := range merged {
    if s.Number == -1 {
        for _, occ := range s.Occurrences {
            if occ.DeterminationMethod == common.DeterminationMethodDirectSVC0x80 {
                warnings = ...
                break
            }
        }
    }
}
```

#### FR-4.2: ソート条件の修正

`Location` フィールドが `SyscallInfo` から削除されるため、`merged` のソート条件から `Location` 参照を削除する。グループ化後はグループエントリを `Number` でソートし、`Occurrences` 内を `Location` でソートする（FR-3 のグループ化ロジックに委ねる）。

### FR-5: `network_analyzer.go` の `syscallAnalysisHasSVCSignal` 修正

`internal/runner/security/network_analyzer.go` の `syscallAnalysisHasSVCSignal` を、グループ化後の構造に対して動作するよう修正する。

変更前:

```go
for _, s := range result.DetectedSyscalls {
    if s.DeterminationMethod == common.DeterminationMethodDirectSVC0x80 && s.Number == -1 {
        return true
    }
}
```

変更後:

```go
for _, s := range result.DetectedSyscalls {
    if s.Number == -1 {
        for _, occ := range s.Occurrences {
            if occ.DeterminationMethod == common.DeterminationMethodDirectSVC0x80 {
                return true
            }
        }
    }
}
```

### FR-6: スキーマバージョンの更新

`internal/fileanalysis/schema.go` の `CurrentSchemaVersion` を 16 から 17 に更新する。

```go
const CurrentSchemaVersion = 17
```

`SyscallInfo` の構造変化（`location`・`determination_method` の移動）はフィールドの型変更に相当し、後方互換性がない。既存の v16 レコードは `SchemaVersionMismatchError` を返す。`--force` オプションで再 `record` することで v17 形式に移行できる。

バージョン履歴コメントに以下を追記する：

```
// Version 17 groups detected_syscalls by syscall number.
//   SyscallInfo.Location, DeterminationMethod, Source are moved to
//   a new SyscallOccurrence sub-type, collected in SyscallInfo.Occurrences.
```

## 4. 非機能要件

### NFR-1: JSON サイズの削減

同一システムコールが複数箇所で検出されるバイナリでは、`detected_syscalls` の JSON サイズが削減される。削減量は `(出現回数 - 1) × (number + name + is_network フィールドのバイト数)` に比例する。

### NFR-2: リスク判定結果の非変更

グループ化は記録形式の変更であり、`runner` のリスク判定結果は変わらない。同一バイナリに対して v17 形式のレコードで `verify` を実行した場合、v16 形式と同一のリスク判定を返すこと。

### NFR-3: アナライザー内部への影響の最小化

elfanalyzer・machoanalyzer の内部実装は、グループ化された `SyscallInfo` ではなく従来の出現ごと `SyscallInfo`（`Occurrences` が空）を生成しても構わない。グループ化の責務は `SaveSyscallAnalysis` に集約し、アナライザーの大規模改修を避ける。

## 5. 受け入れ基準

### AC-1: JSON 構造のグループ化

同一番号のシステムコールを持つバイナリに対して `record` を実行したとき、`detected_syscalls` で同一 `number` のエントリが 1 件のみ存在し、各出現が `occurrences` 配列に格納されること。

### AC-2: ソート順の維持

保存された `detected_syscalls` エントリが `number` 昇順（`-1` は末尾）でソートされていること。各エントリの `occurrences` が `location` 昇順でソートされていること。

### AC-3: `AnalysisWarnings` の正確な発出（Mach-O）

- 未解決 svc（`Number == -1` かつ出現に `determination_method: "direct_svc_0x80"` あり）を持つ Mach-O バイナリでは `analysis_warnings` に警告が記録されること
- 全 svc が解決済み（全出現の `Number != -1`）の Mach-O バイナリでは `analysis_warnings` に svc 関連警告が記録されないこと

### AC-4: `runner` の高リスク判定（未解決 svc）

- `Number == -1` のグループエントリを持ち、その `occurrences` に `determination_method: "direct_svc_0x80"` が存在するレコードで `runner` を実行すると、高リスク（`isHighRisk = true`）と判定されること
- `Number == -1` のグループエントリを持つが、`occurrences` に `determination_method: "direct_svc_0x80"` が存在しないレコードでは、高リスクと判定されないこと

### AC-5: `runner` のネットワーク判定

- `IsNetwork == true` のグループエントリを含むレコードで `runner` を実行すると、ネットワーク操作あり（`isNetwork = true`）と判定されること（`occurrences` の内容に関わらず）

### AC-6: スキーマバージョン 17 の強制

- `CurrentSchemaVersion` が 17 に更新されていること
- v16 形式のレコードをロードすると `SchemaVersionMismatchError` が返ること
- `make test` / `make lint` がエラーなしで通過すること

### AC-7: 既存テストの更新

以下のテストファイルが v17 形式の構造前提へ更新されていること：

- `internal/common/syscall_types_test.go`：`SyscallInfo`・`SyscallOccurrence` の構造テスト
- `internal/fileanalysis/syscall_store_test.go`：グループ化ロジックのテスト（同一番号の複数エントリが 1 グループに集約されること）
- `internal/filevalidator/validator_macho_test.go`：`buildMachoSyscallData` の `Occurrences` 参照前提へ更新
- `internal/runner/security/network_analyzer_test.go`：`syscallAnalysisHasSVCSignal` のテストが `Occurrences` を持つ構造前提へ更新

## 6. 先行タスクとの関係

| タスク | 関係 |
|-------|------|
| 0104 Mach-O システムコール番号解析 | `SyscallInfo.DeterminationMethod`・`Location` を生成するアナライザーを実装したタスク。本タスクでこれらのフィールドが `SyscallOccurrence` へ移動する |
| 0105 record フィルタリング削除 | `detected_syscalls` の件数が増加し、グループ化の効果が大きくなるタスク。本タスクはその後続として位置づける |
