# shebang スクリプトのネットワークリスク解析 実装計画書

## 進捗状況

- [ ] Phase 1: `ShebangInterpreterStore` の実装
- [ ] Phase 2: `NetworkAnalyzer` の拡張
- [ ] Phase 3: `NewNetworkAnalyzer` 呼び出し箇所の更新
- [ ] Phase 4: テスト実装
- [ ] Phase 5: 動作確認・品質チェック

---

## Phase 1: `ShebangInterpreterStore` の実装

**対象ファイル:** `internal/fileanalysis/shebang_store.go`（新規）

- [ ] 1-1. `ShebangInterpreterStore` インターフェースを定義
  - メソッド: `LoadInterpreterAnalysisPath(scriptPath, scriptContentHash string) (interpPath, interpContentHash string, err error)`

- [ ] 1-2. `shebangInterpreterStore` 構造体の実装
  - `store Store` フィールド
  - `NewShebangInterpreterStore(store Store) ShebangInterpreterStore` コンストラクタ

- [ ] 1-3. `LoadInterpreterAnalysisPath` の処理実装
  - [ ] スクリプトレコードのロード（`ErrRecordNotFound` はスキップ）
  - [ ] `contentHash` の検証（不一致は `ErrHashMismatch`）
  - [ ] `ShebangInterpreter == nil` チェック（スキップ）
  - [ ] インタープリタパスの決定（`ResolvedPath` 優先）
  - [ ] インタープリタレコードのロード（`ErrRecordNotFound` は `(interpPath, "", nil)`）
  - [ ] `(interpPath, interpRecord.ContentHash, nil)` を返す

---

## Phase 2: `NetworkAnalyzer` の拡張

**対象ファイル:** `internal/runner/base/security/network_analyzer.go`

- [ ] 2-1. `NetworkAnalyzer` 構造体に `shebangStore fileanalysis.ShebangInterpreterStore` フィールドを追加

- [ ] 2-2. `NewNetworkAnalyzer` の引数に `shebangStore fileanalysis.ShebangInterpreterStore` を追加し、フィールドに代入

- [ ] 2-3. `analyzeBinarySignals` に shebang チェーン追跡を追加（`return` 直前）
  - [ ] `shebangStore != nil && contentHash != ""` のガード
  - [ ] `LoadInterpreterAnalysisPath` 呼び出し
  - [ ] `ErrHashMismatch` → `return true, true`
  - [ ] その他エラー → `slog.Warn` + `return true, true`
  - [ ] `interpHash != ""` の場合のみ再帰呼び出し
  - [ ] 再帰結果を OR 結合

---

## Phase 3: `NewNetworkAnalyzer` 呼び出し箇所の更新

- [ ] 3-1. `grep -rn "NewNetworkAnalyzer"` で全呼び出し箇所を特定

- [ ] 3-2. 各呼び出し箇所に `shebangStore` 引数を追加
  - `internal/runner/base/risk/evaluator.go`: `NewNetworkAnalyzer` 呼び出しに `shebangStore` を渡す
  - テストコード等: 適切なモックまたは `nil` を渡す

---

## Phase 4: テスト実装

**対象ファイル（新規）:** `internal/fileanalysis/shebang_store_test.go`
**対象ファイル（追加）:** `internal/runner/base/security/network_analyzer_test.go`

- [ ] 4-1. `ShebangInterpreterStore` テスト（TC-01〜TC-07）
  - [ ] TC-01: direct 形式、両レコード存在 → `(interpPath, interpHash, nil)`
  - [ ] TC-02: env 形式、`ResolvedPath` が使用される
  - [ ] TC-03: スクリプトレコード不在 → `("", "", nil)`
  - [ ] TC-04: `contentHash` 不一致 → `ErrHashMismatch`
  - [ ] TC-05: `ShebangInterpreter == nil` → `("", "", nil)`
  - [ ] TC-06: インタープリタレコード不在 → `(interpPath, "", nil)`
  - [ ] TC-07: インタープリタロードエラー → error

- [ ] 4-2. `analyzeBinarySignals` shebang 拡張テスト（TC-11〜TC-18）
  - [ ] TC-11: インタープリタが `socket` シンボル → `isNetwork = true`
  - [ ] TC-12: インタープリタの共有ライブラリが mprotect リスクを持つ → `isHighRisk = true`
  - [ ] TC-13: インタープリタのライブラリが `dlopen` → `isHighRisk = true`
  - [ ] TC-14: インタープリタのハッシュ不明 → スキップ `(false, false)`
  - [ ] TC-15: `ErrHashMismatch` → `(true, true)`
  - [ ] TC-16: ロードエラー → `(true, true)`
  - [ ] TC-17: `shebangStore == nil` → 変更なし
  - [ ] TC-18: 非スクリプト（`ShebangInterpreter == nil`）→ 変更なし

---

## Phase 5: 動作確認・品質チェック

- [ ] 5-1. `make fmt` でフォーマット確認
- [ ] 5-2. `make test` で全テストが通ることを確認
- [ ] 5-3. `make lint` でリンターエラーがないことを確認
- [ ] 5-4. 既存テストのリグレッションがないことを確認
