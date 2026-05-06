# shebang スクリプトのネットワークリスク解析 実装計画書

## 進捗状況

- [x] Phase 1: `ShebangInterpreterStore` の実装
- [x] Phase 2: `NetworkAnalyzer` の拡張
- [x] Phase 3: `NewNetworkAnalyzer` 呼び出し箇所の更新
- [ ] Phase 4: テスト実装
- [ ] Phase 5: 動作確認・品質チェック

---

## Phase 1: `ShebangInterpreterStore` の実装

**対象ファイル:** `internal/fileanalysis/shebang_store.go`（新規）

- [x] 1-1. `ShebangInterpreterStore` インターフェースを定義
  - メソッド: `LoadInterpreterAnalysisPath(scriptPath, scriptContentHash string) (interpPath, interpContentHash string, err error)`

- [x] 1-2. `shebangInterpreterStore` 構造体の実装
  - `store Store` フィールド
  - `NewShebangInterpreterStore(store Store) ShebangInterpreterStore` コンストラクタ

- [x] 1-3. `ErrInterpreterRecordMissing` sentinel error を定義

- [x] 1-4. `LoadInterpreterAnalysisPath` の処理実装
  - [x] スクリプトレコードのロード（`ErrRecordNotFound` は `ErrRecordNotFound` を返す）
  - [x] `contentHash` の検証（不一致は `ErrHashMismatch`）
  - [x] `ShebangInterpreter == nil` チェック（スキップ）
  - [x] インタープリタパスの決定（`ResolvedPath` 優先）
  - [x] インタープリタレコードのロード（`ErrRecordNotFound` は `ErrInterpreterRecordMissing` でエラー返却）
  - [x] `interpRecord.ContentHash == ""` も `ErrInterpreterRecordMissing` でエラー返却
  - [x] `(interpPath, interpRecord.ContentHash, nil)` を返す

---

## Phase 2: `NetworkAnalyzer` の拡張

**対象ファイル:** `internal/runner/base/security/network_analyzer.go`

- [x] 2-1. `NetworkAnalyzer` 構造体に `shebangStore fileanalysis.ShebangInterpreterStore` フィールドを追加

- [x] 2-2. `NewNetworkAnalyzer` の引数に `shebangStore fileanalysis.ShebangInterpreterStore` を追加し、フィールドに代入

- [x] 2-3. `analyzeBinarySignals` に shebang チェーン追跡を追加（`return` 直前）
  - [x] `shebangStore != nil && contentHash != ""` のガード
  - [x] `LoadInterpreterAnalysisPath` 呼び出し
  - [x] `ErrInterpreterRecordMissing` → エラーを呼び出し元へ返却（実行中止）
  - [x] `ErrHashMismatch` → エラーを呼び出し元へ返却（実行中止）
  - [x] その他エラー（インタープリタレコードロード失敗を含む）→ エラーを呼び出し元へ返却（実行中止）
  - [x] `interpHash != ""` の場合のみ再帰呼び出し
  - [x] 再帰結果を OR 結合

---

## Phase 3: `NewNetworkAnalyzer` 呼び出し箇所の更新

- [x] 3-1. `grep -rn "NewNetworkAnalyzer"` で全呼び出し箇所を特定

- [x] 3-2. 各呼び出し箇所に `shebangStore` 引数を追加
  - `internal/runner/base/risk/evaluator.go`: `NewNetworkAnalyzer` 呼び出しに `shebangStore` を渡す
  - テストコード等: 適切なモックまたは `nil` を渡す

---

## Phase 4: テスト実装

**対象ファイル（新規）:** `internal/fileanalysis/shebang_store_test.go`
**対象ファイル（追加）:** `internal/runner/base/security/network_analyzer_test.go`

  - [x] 4-1. `ShebangInterpreterStore` テスト（TC-01〜TC-08）
    - [x] TC-01: direct 形式、両レコード存在 → `(interpPath, interpHash, nil)`
    - [x] TC-02: env 形式、`ResolvedPath` が使用される
    - [x] TC-03: スクリプトレコード不在 → `("", "", ErrRecordNotFound)`
    - [x] TC-04: `contentHash` 不一致 → `ErrHashMismatch`
    - [x] TC-05: `ShebangInterpreter == nil` → `("", "", nil)`
    - [x] TC-06: インタープリタレコード不在 → `("", "", ErrInterpreterRecordMissing)`
    - [x] TC-07: インタープリタロードエラー → error
    - [x] TC-08: インタープリタ `ContentHash` 空 → `("", "", ErrInterpreterRecordMissing)`

  - [x] 4-2. `analyzeBinarySignals` shebang 拡張テスト（TC-11〜TC-18）
    - [x] TC-11: インタープリタが `socket` シンボル → `isNetwork = true`
    - [x] TC-12: インタープリタの共有ライブラリが mprotect リスクを持つ → `isHighRisk = true`
    - [x] TC-13: インタープリタのライブラリが `dlopen` → `isHighRisk = true`
    - [x] TC-14: インタープリタレコード不在（`ErrInterpreterRecordMissing`）→ エラー返却 + グループ実行中止
    - [x] TC-15: `ErrHashMismatch` → エラー返却 + グループ実行中止
    - [x] TC-16: ロードエラー → エラー返却 + グループ実行中止
    - [x] TC-17: `shebangStore == nil` → 変更なし
    - [x] TC-18: 非スクリプト（`ShebangInterpreter == nil`）→ 変更なし

---

## Phase 5: 動作確認・品質チェック

  - [x] 5-1. `make fmt` でフォーマット確認
  - [x] 5-2. `make test` で全テストが通ることを確認
  - [x] 5-3. `make lint` でリンターエラーがないことを確認
  - [x] 5-4. 既存テストのリグレッションがないことを確認
