# 要件定義書: デバッグ情報へのシンボル・syscall 呼び出し元帰属

## 1. 背景

`record --debug-info` で出力される Record JSON には、検出されたシステムコールや動的ロードシンボル（`socket`、`dlopen` など）の情報が含まれる。しかし現在のスキーマでは、これらがコマンド本体・shebang インタープリター・依存共有ライブラリのどれに由来するかを特定できない。

具体的には：

1. **`symbol_analysis.detected_symbols / dynamic_load_symbols`** は `[]string`（シンボル名のみ）であり、呼び出し元の情報を持たない。
2. **`syscall_analysis.detected_syscalls[].occurrences[].location`** は libc インポート経由で検出された syscall では常に `0`（番兵値）となり、どのバイナリがそのシンボルをインポートしているかを示す情報がない。
3. **`determination_stats`** はすべてのバイナリの統計値が合算されており、バイナリ別内訳を確認できない。

本タスクでは `--debug-info` 指定時に限り、各シンボル・syscall の呼び出し元バイナリを特定できるよう、スキーマと集約アーキテクチャを拡張する。

## 2. 用語

| 用語 | 定義 |
|---|---|
| 呼び出し元（source）| あるシンボルまたは syscall がどのバイナリ/ライブラリに由来するかを示すパス |
| source attribution | 各 occurrence またはシンボルエントリに呼び出し元パスを付与すること |
| in-place attribution | 集約済みフィールド（`syscall_analysis`, `symbol_analysis`）の各エントリに `source_path` を埋め込む方式 |
| `SourceRole` | 集約パイプライン内でバイナリの役割を表す内部型（`internal/filevalidator/validator.go` で定義）。値は `roleMain` / `roleShebangInterpreter` / `roleDynLib` |
| `DetectedSymbol` | シンボル名と呼び出し元パスを持つ構造体。`SymbolAnalysisData` の要素型（`internal/fileanalysis/schema.go` で定義） |

## 3. 機能要件

### FR-001: `SyscallOccurrence` への `source_path` 追加

`SyscallOccurrence` に `source_path` フィールドを追加する。

Acceptance Criteria:

1. AC-001: `--debug-info` 指定時、各 `SyscallOccurrence` に `source_path`（そのシンボルを使用しているバイナリの絶対パス、symlink 解決済み）が格納される
2. AC-002: `--debug-info` 非指定時、`source_path` は JSON に出力されない（`omitempty`）
3. AC-003: libc インポート経由（`location=0`）の occurrence でも `source_path` は正しく設定される

`source_path` のセマンティクス：

- 直接 syscall 命令検出時：その命令が存在するバイナリのパス
- libc インポート経由検出時：そのシンボルをインポートしているバイナリのパス（libc 自体ではない）
- dynlib 経由の検出時：そのシンボルを持つ dynlib のパス

### FR-002: `SymbolAnalysisData` の `DetectedSymbol` 型への変更

`detected_symbols` および `dynamic_load_symbols` フィールドの要素型を `string` から `DetectedSymbol` 構造体に変更する。

```go
type DetectedSymbol struct {
    Name       string `json:"name"`
    SourcePath string `json:"source_path,omitempty"`
}
```

Acceptance Criteria:

1. AC-004: `--debug-info` 指定時、各 `DetectedSymbol` に `source_path` が格納される
2. AC-005: `--debug-info` 非指定時、`source_path` は JSON に出力されない
3. AC-006: `--debug-info` 指定時、同一シンボル名でも呼び出し元が異なる場合は別エントリとして記録される（`(name, source_path)` 単位で重複除去）
4. AC-007: `--debug-info` 非指定時、同一シンボル名は1エントリに集約される（`name` 単位で重複除去、従来動作を維持）

### FR-003: 集約パイプラインへの source パス伝達

`analysisAggregate.addRecord` がソースパスを受け取り、各 occurrence とシンボルエントリに `source_path` を付与する。

Acceptance Criteria:

1. AC-008: `addRecord(record, sourcePath, role)` は `record` 内の全 `SyscallOccurrence.SourcePath` にまだ設定されていない場合のみ `sourcePath` をセットする
2. AC-009: `addRecord` はコマンド本体（main）、shebang インタープリター（shebang_interpreter）、dynlib（dynlib）の 3 種の role を区別する
3. AC-010: `addSymbolAnalysis` は `--debug-info` 時に `(name, source_path)` キーで重複除去、非 debug 時は `name` キーで重複除去する

### FR-004: スキーマバージョン更新

Acceptance Criteria:

1. AC-011: `CurrentSchemaVersion` を 22 から 23 に更新する
2. AC-012: v22 以下の Record 読み込み時は `SchemaVersionMismatchError` を返す
3. AC-013: v22 以下の Record に対して `--force` なしで `record` を再実行すると v23 形式に上書き再生成できる

## 4. 非機能要件

1. NFR-001: `--debug-info` 非指定時、`source_path` は JSON に出力されない（`omitempty`）。ただし `detected_symbols` / `dynamic_load_symbols` の型が `string[]` から `DetectedSymbol[]` に変わるため、構造変更に伴うサイズ増分は許容する
2. NFR-002: `--debug-info` 指定時の追加データは source attribution の文字列フィールドのみとし、データ重複を持ち込まない
3. NFR-003: セキュリティポリシー評価（ネットワークリスク判定）に使用される集約フィールドの動作は変更しない
4. NFR-004: `network_analyzer` はシンボル名の有無でリスク判定するため、`DetectedSymbol` への型変更後も判定ロジックを維持する

## 5. テスト要求

1. 各 AC に対して少なくとも1件のテストを対応付ける
2. `SymbolAnalysisData` を参照する既存テストは `DetectedSymbol` 型に更新する
3. `--debug-info` あり／なしの両方で source attribution の有無を検証する

## 6. 実装上の注意事項

1. 実装コード内ではコメントを含め日本語を使用しない
2. `network_analyzer` および `verification.Manager` が `DetectedSymbol.Name` を参照する箇所を漏れなく更新する

## 7. スコープ外

1. `debug.per_source_analysis`（バイナリ別生データ）の追加（別タスクで検討）
2. `--debug-info` 非指定時への source attribution 適用
3. `determination_stats` のバイナリ別内訳出力
