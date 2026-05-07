# 実装計画書: Record スキーマ v22

## 進捗

- [x] Phase 1: スキーマ更新
- [x] Phase 2: record 実装更新
- [x] Phase 3: runner / verification 更新
- [x] Phase 4: テスト整理と追加
- [ ] Phase 5: 品質確認と文書整合

## Phase 1: スキーマ更新

### 1-1. バージョン更新

- [x] `CurrentSchemaVersion` を 22 に更新する（AC-001, AC-026）
- [x] v21 以下読み込み時に `SchemaVersionMismatchError` を返すことを確認する（AC-027）

### 1-2. Record フィールド更新

- [x] `deps` を `path` `hash` のみの構造に更新する（AC-002）
- [x] `shebang_chain` を `ref?` `path` のみに更新する（AC-003, AC-011）
- [x] `analysis_warnings` をトップレベルで統合出力する（AC-004）
- [x] `debug` を `omitempty` で保持する（AC-005）
- [x] `internal/fileanalysis/schema_test.go` を新規作成し、`deps`・`shebang_chain` の JSON シリアライズ仕様（AC-002, AC-003, AC-011）をテストする

### 1-3. 参照コード修正

- [x] 旧 dep 解析フィールド参照を全て削除する
- [x] 旧 shebang 解析埋め込み前提の参照を全て削除する

## Phase 2: record 実装更新

### 2-1. deps 収集

- [x] コマンド本体の依存共有ライブラリを収集する（AC-006）
- [x] shebang チェーン各バイナリの依存共有ライブラリを収集する（AC-006）
- [x] shebang チェーンのインタープリターバイナリ本体を deps に含める（AC-006）

### 2-2. dedup とエラー

- [x] `path` をキーに dedup する（AC-007）
- [x] 同一 path hash 不一致で致命エラーにする（AC-008）

### 2-3. 解析統合

- [x] コマンド本体、dep ライブラリ、shebang チェーン全バイナリを対象に syscall を統合する（AC-013）
- [x] コマンド本体、dep ライブラリ、shebang チェーン全バイナリを対象に symbol を統合する（AC-014）
- [x] ArgEvalResults は worst-case 統合を実装する（AC-015）
- [x] VDSO と syscall wrapper を解析スキップする（AC-016）

### 2-4. 出力更新

- [x] 統合結果を `record.syscall_analysis` と `record.symbol_analysis` に書き込む（AC-017）
- [x] `analysis_warnings` を統合・dedup して書き込む（AC-004）
- [x] `-debug-info` 指定時のみ `debug.dep_sources` を書き込む（AC-005）

### 2-5. 削除作業

- [x] `saveInterpreterRecord` を削除する

## Phase 3: runner / verification 更新

### 3-1. AnalysisDeps の単純化

- [x] `AnalysisDeps` を `RecordStore` のみに変更する（AC-018）

### 3-2. NetworkAnalyzer 更新

- [x] `analyzeBinarySignals` を Record ロード + トップレベル解析参照に置換する（AC-019, AC-017）
- [x] `checkDepsSignals` を削除する（AC-020）
- [x] `followShebangChain`（解析目的）を削除する（AC-021）
- [x] `ErrDepAnalysisNotEmbedded` を削除する（AC-023）

### 3-3. shebang 実行時検証

- [x] `verifyShebangChain` を実装する: `ref` が絶対パスなら EvalSymlinks、ベア名なら LookPath+EvalSymlinks で解決し `path` と比較する（AC-010, AC-022）

### 3-4. verification.Manager 更新

- [x] `GetAnalysisDeps` を `AnalysisDeps{RecordStore: m.fileValidator}` に変更する（AC-024）
- [x] `networkSymbolStore` `syscallAnalysisStore` `dynLibDepsStore` `dynlibAnalysisStore` `shebangStore` を削除する（AC-025）

## Phase 4: テスト整理と追加

### 4-1. 追加・更新テスト

- [x] `internal/fileanalysis/file_analysis_store_test.go`（既存）を更新: schema_version 22 検証（AC-001, AC-026, AC-027）
- [x] `internal/fileanalysis/schema_test.go`（新規作成）: deps・shebang_chain の JSON 構造制約（AC-002, AC-003, AC-011）
- [x] `internal/filevalidator/validator_dedup_test.go`（新規作成）: path dedup と hash 不一致エラー（AC-007, AC-008）
- [x] `internal/verification/shebang_chain_verifier_test.go`（新規作成）: ref 再解決検証（AC-010, AC-022）
- [x] `internal/filevalidator/validator_test.go`（既存）を更新: 統合 syscall/symbol/ArgEvalResults・deps 収集範囲（AC-006, AC-013, AC-014, AC-015, AC-016, AC-017）
- [x] `cmd/record/main_test.go`（既存）を更新: debug omitempty・再記録（AC-005, AC-028）

### 4-2. 削除テスト

- [x] `ErrAnalysisNotFound` フォールバック前提テストを削除する
- [x] `checkDepsSignals` 前提テストを削除する
- [x] `followShebangChain` 前提テストを削除する
- [x] `ErrDepAnalysisNotEmbedded` 前提テストを削除する
- [x] `ShebangStore` `DynLibDepsStore` `LibAnalysisStore` 前提テストを削除する
- [x] deps 内解析フィールド前提テストを削除する

### 4-3. 重複排除

- [x] schema 形状検証を単一テスト群に集約する
- [x] dedup 異常系検証を単一テスト群に集約する
- [x] shebang 再解決検証を単一テスト群に集約する

## Phase 5: 品質確認と文書整合

### 5-1. コード品質

- [ ] `make build` を実行してビルドが通ることを確認する
- [ ] `make fmt` を実行する
- [ ] `make test` を実行する
- [ ] `make lint` を実行する

### 5-2. 実装ルール確認

- [ ] コメントを含むコード中に日本語がないことを確認する

### 5-3. 文書整合

- [ ] 01〜04 の AC 番号整合を確認する
- [ ] 01〜04 の削除対象ロジック記述整合を確認する
- [ ] 01〜04 のテスト対応記述整合を確認する
