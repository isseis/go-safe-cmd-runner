# 要件定義書: Record スキーマ v22 への移行

## 1. 背景

本タスクでは、Record を実行時に自己完結して参照できる形へ再設計する。主目的は次の3点。

1. `runner` のリスク判定入力を Record のトップレベル解析結果に一本化する
2. `deps` と `shebang_chain` を検証用途に限定し、責務を明確化する
3. スキーマを v22 に更新し、旧 Record（v21 以下）を明確に拒否する

## 2. 用語

| 用語 | 定義 |
|------|------|
| Record | `record` コマンドが生成する JSON ファイル |
| 統合解析結果 | `syscall_analysis` と `symbol_analysis` のトップレベル集約結果 |
| deps | `path` と `hash` のみを持つ依存物リスト |
| shebang_chain | shebang 解決過程を記録する識別情報リスト |
| analysis_warnings | 解析時の非致命警告を統合・dedup した配列 |

## 3. 機能要件

### FR-001: Record スキーマ v22

Record は次のトップレベル構造を持つ。

```json
{
   "schema_version": 22,
   "file_path": "...",
   "content_hash": "sha256:...",
   "syscall_analysis": {},
   "symbol_analysis": {},
   "analysis_warnings": [],
   "deps": [
      { "path": "...", "hash": "sha256:..." }
   ],
   "shebang_chain": [
      { "raw_path": "...", "path": "...", "command_name": "..." }
   ],
   "debug": { "dep_sources": {} }
}
```

Acceptance Criteria:

1. AC-001: `schema_version` は 22 固定である
2. AC-002: `deps` の各要素は `path` と `hash` のみを持つ
3. AC-003: `shebang_chain` の各要素は `raw_path`（任意）`path`（必須）`command_name`（任意）のみを持つ
4. AC-004: `analysis_warnings` は警告がある場合のみ格納され、警告文字列は統合・dedup される
5. AC-005: `debug` は `-debug-info` 指定時のみ出力される

### FR-002: deps の責務定義

`deps` はハッシュ整合性検証専用データであり、リスク判定には使用しない。

Acceptance Criteria:

1. AC-006: `deps` には以下を含む
    コマンド本体および shebang チェーン各バイナリの依存共有ライブラリ
    shebang チェーンのインタープリターバイナリ本体
2. AC-007: `deps` の dedup キーは `path` である
3. AC-008: 同一 `path` で `hash` が不一致の場合、`record` は致命的エラーで中断する
4. AC-009: `runner` は `deps` をハッシュ検証にのみ使用し、ネットワークリスク判定には使用しない

### FR-003: shebang_chain の責務定義

`shebang_chain` は実行時改ざん検出専用データとする。

Acceptance Criteria:

1. AC-010: `raw_path` があるエントリは実行時に再解決され、解決結果が `path` と一致しない場合は失敗する
2. AC-011: `command_name` があるエントリは実行時 PATH で再解決され、解決結果が `path` と一致しない場合は失敗する
3. AC-012: `shebang_chain` は hash や解析結果を保持しない

### FR-004: トップレベル解析結果の統合

`record` は解析対象全体の結果を集約し、`runner` はトップレベルのみを参照する。

Acceptance Criteria:

1. AC-013: `syscall_analysis` はコマンド本体、全 dep ライブラリ、shebang チェーン全バイナリ由来の結果を syscall 番号で統合・dedup する
2. AC-014: `symbol_analysis` は同対象を symbol 名で統合・dedup する
3. AC-015: `ArgEvalResults`（`mprotect` の `PROT_EXEC` 判定）は統合時に worst-case を採用する
4. AC-016: VDSO は解析をスキップし `deps` にも含めない（実ファイルが存在しないため hash 検証不可）。syscall wrapper ライブラリ（libc 等）は解析をスキップするが `deps` には `path` + `hash` で含める
5. AC-017: `runner` のリスク判定は `record.syscall_analysis` と `record.symbol_analysis` のみを参照する

### FR-005: runner / NetworkAnalyzer / verification.Manager の簡素化

Acceptance Criteria:

1. AC-018: `AnalysisDeps` は `RecordStore` のみを持つ
2. AC-019: `analyzeBinarySignals` は Record を読み込み、トップレベル解析結果のみでシグナル判定する
3. AC-020: `checkDepsSignals`（dep ごとの解析ループ）は削除される
4. AC-021: `followShebangChain`（解析目的）は削除される
5. AC-022: `verifyShebangChain` は存続し、改ざん検出に使用される
6. AC-023: `ErrDepAnalysisNotEmbedded` は削除される
7. AC-024: `verification.Manager.GetAnalysisDeps` は `AnalysisDeps{RecordStore: m.fileValidator}` を返す
8. AC-025: `verification.Manager` から `networkSymbolStore` `syscallAnalysisStore` `dynLibDepsStore` `dynlibAnalysisStore` `shebangStore` を削除する

### FR-006: スキーマバージョン互換性

Acceptance Criteria:

1. AC-026: `CurrentSchemaVersion` は 22 である
2. AC-027: v21 以下の Record 読み込み時は `SchemaVersionMismatchError` を返す
3. AC-028: v21 以下の Record が存在する場合、`--force` フラグなしで `record` を再実行すれば v22 形式へ上書き再生成できる

## 4. 非機能要件

1. NFR-001: `runner` 実行時の解析用外部ストア参照をなくし、Record 単体で判断できること
2. NFR-002: dedup と統合結果は再現可能であること（同入力で同一結果）
3. NFR-003: 既存 CLI 互換性を維持すること（主要フラグと入出力契約）
4. NFR-004: Record 保存は既存どおりアトミックに行うこと

## 5. テスト要求

1. 各 AC に対して少なくとも1件のテストを対応付ける
2. 廃止ロジックのテストは削除対象として明示する
3. 既存テストと重複するケースは統合または削減し、冗長な同義検証を避ける

## 6. 実装上の注意事項

1. 実装コード内ではコメントを含め日本語を使用しない
2. 変更後は 01〜04 の文書間で用語、AC番号、削除対象、テスト対応を一致させる

## 7. スコープ外

1. shebang 多段解決の新規対応
2. 推移的依存関係解析の新規導入
3. Record 以外のフォーマット追加
