# 詳細設計書: Record スキーマ v22

## 1. 変更サマリー

1. Record のスキーマを 22 へ更新する
2. `deps` を `path` + `hash` のみへ縮退する
3. リスク判定入力を Record トップレベル解析結果に統一する
4. `runner` 側の dep 解析ループと shebang 解析追跡を削除する

## 2. データ仕様

### 2.1 Record 主要フィールド

| フィールド | 仕様 | 対応 AC |
|---|---|---|
| `schema_version` | 22 固定 | AC-001, AC-026 |
| `syscall_analysis` | 全解析対象の syscall 番号 dedup 統合結果 | AC-013, AC-017 |
| `symbol_analysis` | 全解析対象の symbol 名 dedup 統合結果 | AC-014, AC-017 |
| `analysis_warnings` | 非致命警告を統合・dedup して格納（必要時のみ） | AC-004 |
| `deps` | `path` + `hash` のみ。hash 検証用途 | AC-002, AC-006, AC-009 |
| `shebang_chain` | `ref?` `path` のみ | AC-003, AC-010, AC-011 |
| `debug.dep_sources` | `-debug-info` 時のみ出力 | AC-005 |

### 2.2 deps の仕様

1. 収集対象
   コマンド本体の依存共有ライブラリ
   shebang チェーン各バイナリの依存共有ライブラリ
   shebang チェーンのインタープリターバイナリ本体
2. dedup
   キーは `path`
   同一 path hash 不一致は致命エラーで Record 生成中断
3. 出力
   各エントリは `path` `hash` のみ

### 2.3 shebang_chain の仕様

各エントリの `ref` フィールドの内容で解決方法を分岐する:
- `ref` なし: 再解決不要（スキップ）
- `ref` が絶対パス（`filepath.IsAbs(ref) == true`）: `filepath.EvalSymlinks(ref)` の結果を `path` と比較
- `ref` がベア名（パス区切りなし）: `exec.LookPath(ref)` + `filepath.EvalSymlinks` の結果を `path` と比較

不一致または LookPath 失敗時は fail-closed

## 3. record 実装詳細

### 3.1 解析対象と統合ルール

1. コマンド本体を解析する
2. shebang チェーンがある場合、チェーン全バイナリを解析する
3. 1と2で得た依存共有ライブラリを解析する
4. VDSO は解析スキップし `deps` にも含めない（AC-016）。syscall wrapper ライブラリは解析スキップするが `deps` には path+hash を含める
5. syscall は番号で dedup する
6. symbol は名前で dedup する
7. `ArgEvalResults` は worst-case を採用して統合する
8. 非致命警告は統合・dedup して `analysis_warnings` に格納する

### 3.2 dynlib キャッシュの扱い

1. `record` は dynlib-analysis キャッシュを内部最適化として使用可能
2. `runner` は dynlib-analysis キャッシュを参照しない

### 3.3 削除対象

1. `saveInterpreterRecord` を削除する
2. dep 単位の解析結果出力を `deps` から削除する

## 4. runner / verification 実装詳細

### 4.1 AnalysisDeps

1. `AnalysisDeps` は `RecordStore` のみを保持する
2. `NetworkSymbolStore` `SyscallStore` `DynLibDepsStore` `LibAnalysisStore` `ShebangStore` は削除する

### 4.2 analyzeBinarySignals

1. Record をロードする
2. `record.syscall_analysis` と `record.symbol_analysis` のみで判定する
3. dep ごとの解析ループは持たない
4. shebang 解析追跡 (`followShebangChain`) は持たない

### 4.3 verifyShebangChain

```
for each entry in shebang_chain:
    if entry.Ref == "":
        continue
    if filepath.IsAbs(entry.Ref):
        resolved = filepath.EvalSymlinks(entry.Ref)
    else:
        found = exec.LookPath(entry.Ref)  // error → abort
        resolved = filepath.EvalSymlinks(found)
    if resolved != entry.Path:
        abort  // fail-closed
```

### 4.4 エラー方針

1. `ErrDepAnalysisNotEmbedded` は削除する
2. v21 以下は `SchemaVersionMismatchError`
3. 同一 path hash 不一致は `record` 側で即時失敗

## 5. verification.Manager 詳細

1. `GetAnalysisDeps` は `AnalysisDeps{RecordStore: m.fileValidator}` のみを返す
2. `networkSymbolStore` `syscallAnalysisStore` `dynLibDepsStore` `dynlibAnalysisStore` `shebangStore` フィールドを削除する

## 6. AC とテスト対応表

| AC | テスト観点 | 追加/更新テスト候補 |
|---|---|---|
| AC-001, AC-026 | schema_version が 22 | `internal/fileanalysis/schema_test.go` |
| AC-002 | deps が path/hash のみ | `cmd/record/*integration*_test.go` |
| AC-003, AC-011 | shebang_chain の項目制約（`ref` + `path` のみ） | `internal/fileanalysis/schema_test.go` |
| AC-004 | analysis_warnings の統合・dedup | `internal/filevalidator/*analysis*_test.go` |
| AC-005 | debug の omitempty | `cmd/record/main_test.go` |
| AC-006 | deps 収集範囲 | `internal/filevalidator/validator_shebang_test.go` |
| AC-007 | path dedup | `internal/filevalidator/validator_dedup_test.go` |
| AC-008 | path 同一 hash 不一致で失敗 | `internal/filevalidator/validator_dedup_test.go` |
| AC-009 | deps をリスク判定に使わない | `internal/runner/base/security/network_analyzer_test.go` |
| AC-010 | ref（絶対パス）→ EvalSymlinks 比較 / ref（ベア名）→ LookPath+EvalSymlinks 比較 | `internal/verification/shebang_chain_verifier_test.go` |
| AC-013 | syscall 統合 dedup | `internal/filevalidator/*syscall*_test.go` |
| AC-014 | symbol 統合 dedup | `internal/filevalidator/*symbol*_test.go` |
| AC-015 | ArgEvalResults worst-case 統合 | `internal/filevalidator/*syscall*_test.go` |
| AC-016 | VDSO 解析スキップかつ deps 除外 / wrapper 解析スキップのみ（deps に残す） | `internal/filevalidator/validator_library_analysis_test.go` |
| AC-017 | runner はトップレベルのみ参照 | `internal/runner/base/security/network_analyzer_test.go` |
| AC-018, AC-024 | AnalysisDeps/Manager の新構造 | `internal/verification/manager_test.go` |
| AC-019 | analyzeBinarySignals 新仕様 | `internal/runner/base/security/network_analyzer_test.go` |
| AC-020 | checkDepsSignals 削除確認 | `internal/runner/base/security` のコンパイル/参照テスト |
| AC-021 | followShebangChain 削除確認 | `internal/runner/base/security` のコンパイル/参照テスト |
| AC-022 | verifyShebangChain 存続 | `internal/verification/shebang_chain_verifier_test.go` |
| AC-023 | ErrDepAnalysisNotEmbedded 削除確認 | 参照消滅テスト or grep ベース検証 |
| AC-025 | Manager 不要フィールド削除確認 | `internal/verification/manager_test.go` |
| AC-027 | v21 以下読み込み拒否 | `internal/fileanalysis/store_test.go` |
| AC-028 | 再記録で v22 生成 | `cmd/record/main_test.go` |

## 7. 削除対象テスト一覧

1. `ErrAnalysisNotFound` を高リスクフォールバックとして検証するテスト
2. dep ごと解析ロード（`checkDepsSignals` 前提）のテスト
3. `followShebangChain` による解析追跡を前提にしたテスト
4. `ErrDepAnalysisNotEmbedded` の発生を期待するテスト
5. `ShebangStore` `DynLibDepsStore` `LibAnalysisStore` 依存を前提にしたテスト
6. `deps` 内に解析フィールドが存在する前提のテスト

## 8. 重複テスト整理方針

1. schema のフィールド存在確認は `internal/fileanalysis/schema_test.go` に集約する
2. dedup 失敗パスは `internal/filevalidator/validator_dedup_test.go` に集約する
3. shebang 実行時再解決は `internal/verification/shebang_chain_verifier_test.go` に集約する
4. runner の入力源確認は `internal/runner/base/security/network_analyzer_test.go` に集約する

## 9. 実装上の注意事項

1. コメントを含むソースコード内で日本語を使用しない
2. 既存公開 API の変更は最小限に留める
3. 01〜04 文書間で AC 番号と用語を一致させる
