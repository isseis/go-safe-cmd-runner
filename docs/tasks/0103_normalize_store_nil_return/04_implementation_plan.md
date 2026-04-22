# 実装計画書: 解析ストア API の正規化（`(nil, nil)` 返却）

## 概要

センチネルエラー `ErrNoSyscallAnalysis` および `ErrNoNetworkSymbolAnalysis` を削除し、
「解析済み・未検出」を `(nil, nil)` で返す API に統一する。

## 変更対象ファイル

| ファイル | 変更内容 |
|---------|---------|
| `internal/fileanalysis/errors.go` | `ErrNoSyscallAnalysis`、`ErrNoNetworkSymbolAnalysis` の定義を削除 |
| `internal/fileanalysis/schema.go` | `ErrNoSyscallAnalysis` に言及するコメントを `(nil, nil)` 返却に合わせて更新 |
| `internal/fileanalysis/syscall_store.go` | `LoadSyscallAnalysis` の `nil` フィールド時の返却値を `(nil, nil)` に変更。コメント更新 |
| `internal/fileanalysis/network_symbol_store.go` | `LoadNetworkSymbolAnalysis` の `nil` フィールド時の返却値を `(nil, nil)` に変更。コメント更新 |
| `internal/fileanalysis/syscall_store_test.go` | `ErrNoSyscallAnalysis` を使うテストを `(nil, nil)` に更新 |
| `internal/fileanalysis/network_symbol_store_test.go` | `ErrNoNetworkSymbolAnalysis` を使うテストを `(nil, nil)` に更新 |
| `internal/runner/security/network_analyzer.go` | switch 文から `ErrNoNetworkSymbolAnalysis`・`ErrNoSyscallAnalysis` ケースを削除 |
| `internal/runner/security/elfanalyzer/standard_analyzer.go` | switch 文から `ErrNoSyscallAnalysis` ケースを削除 |
| `internal/runner/security/syscall_store_adapter_test.go` | `ErrNoSyscallAnalysis` を使うテストを更新 |
| `internal/runner/security/command_analysis_test.go` | `ErrNoNetworkSymbolAnalysis` を使うテストを更新 |

## 実装ステップ

### Step 1: ストア実装の変更

- [x] **1.1** `internal/fileanalysis/syscall_store.go`
  - `LoadSyscallAnalysis` 内の `return nil, ErrNoSyscallAnalysis` を `return nil, nil` に変更
  - 関数コメントの `(nil, ErrNoSyscallAnalysis)` 記述を `(nil, nil)` に更新（2 箇所）

- [x] **1.2** `internal/fileanalysis/network_symbol_store.go`
  - `LoadNetworkSymbolAnalysis` 内の `return nil, ErrNoNetworkSymbolAnalysis` を `return nil, nil` に変更
  - インターフェースおよび関数コメントの `(nil, ErrNoNetworkSymbolAnalysis)` 記述を `(nil, nil)` に更新（2 箇所）

### Step 2: センチネルエラーと関連コメントの削除

- [x] **2.1** `internal/fileanalysis/errors.go`
  - `ErrNoSyscallAnalysis` の定義と関連コメントを削除
  - `ErrNoNetworkSymbolAnalysis` の定義と関連コメントを削除

- [x] **2.2** `internal/fileanalysis/schema.go`
  - `ErrNoSyscallAnalysis` に言及するコメントを更新
    （「解析済み・syscall 未検出」は `(nil, nil)` で返すように変更された旨に修正）

### Step 3: 呼び出し元の修正

- [x] **3.1** `internal/runner/security/network_analyzer.go`
  - `case errors.Is(err, fileanalysis.ErrNoNetworkSymbolAnalysis):` ブロックを削除
    - `data == nil && err == nil` となった場合でも、既存の `case err == nil:` ブロックで
      処理される。その後の `if data == nil { return false, false }` チェックが機能するため
      動作変化なし
  - `case errors.Is(svcErr, fileanalysis.ErrNoSyscallAnalysis):` ブロックを削除
    - `svcResult == nil && svcErr == nil` となった場合は `case svcErr == nil:` で処理される。
      `syscallAnalysisHasSVCSignal(nil)` が `false` を返すため fall-through となり動作変化なし

- [x] **3.2** `internal/runner/security/elfanalyzer/standard_analyzer.go`
  - switch 文から `errors.Is(err, fileanalysis.ErrNoSyscallAnalysis)` ケースを削除し、
    `ErrRecordNotFound` のケースのみを残す
  - **重要:** `if err != nil` ブロックの直後（`return a.convertSyscallResult(result)` の前）に
    `result == nil` チェックを追加する。
    現在のコードは `if err != nil` ブロックを抜けた後に直接 `a.convertSyscallResult(result)` を呼ぶため、
    `result == nil` だと nil ポインタ参照パニックが発生する。

    ```go
    // 変更後（err == nil ブロックの後に追加）
    if result == nil {
        // Syscall analysis not stored for this file. Fall back silently.
        return binaryanalyzer.AnalysisOutput{Result: binaryanalyzer.StaticBinary}
    }
    return a.convertSyscallResult(result)
    ```

### Step 4: テストの更新

- [x] **4.1** `internal/fileanalysis/syscall_store_test.go`
  - `TestSyscallAnalysisStore_NoSyscallAnalysis`（相当するテスト）
    - `assert.ErrorIs(t, err, ErrNoSyscallAnalysis, ...)` を `assert.NoError(t, err)` に変更
    - `assert.Nil(t, loadedResult)` はそのまま維持

- [x] **4.2** `internal/fileanalysis/network_symbol_store_test.go`
  - `TestNetworkSymbolStore_LoadNetworkSymbolAnalysis_NilSymbolAnalysis`（相当するテスト）
    - `assert.ErrorIs(t, err, ErrNoNetworkSymbolAnalysis, ...)` を `assert.NoError(t, err)` に変更
    - `assert.Nil(t, loaded)` はそのまま維持
  - `assert.NotErrorIs(t, err, ErrNoNetworkSymbolAnalysis, ...)` の行を削除
    （削除されたエラーへの参照のため）

- [x] **4.3** `internal/runner/security/syscall_store_adapter_test.go`
  - `TestNewELFSyscallStoreAdapter_PassesThroughErrors` のセンチネルリストから
    `fileanalysis.ErrNoSyscallAnalysis` を削除

- [x] **4.4** `internal/runner/security/command_analysis_test.go`
  - テスト `"ErrNoNetworkSymbolAnalysis (no syscallStore) → false, false (static binary)"` を
    `"nil (no syscallStore) → false, false (static binary)"` に変更し、
    `stubNetworkSymbolStore{err: fileanalysis.ErrNoNetworkSymbolAnalysis}` を
    `stubNetworkSymbolStore{data: nil}` に変更

### Step 5: ビルドと確認

- [x] **5.1** `go build ./...` でコンパイルエラーがないことを確認
- [x] **5.2** `go test -tags test ./...` で全テストが通ることを確認
- [x] **5.3** `make lint` でリントエラーがないことを確認
- [x] **5.4** `make fmt` でフォーマットを適用

## 受け入れ条件との対応

| AC | 対応ステップ |
|----|------------|
| AC-1: `ErrNoSyscallAnalysis` の削除 | Step 1.1, 2.1, 3.1, 3.2, 4.1, 4.3 |
| AC-2: `ErrNoNetworkSymbolAnalysis` の削除 | Step 1.2, 2.1, 3.1, 4.2, 4.4 |
| AC-3: `(nil, nil)` の返却 | Step 1.1, 1.2 |
| AC-4: 動作の非変更 | Step 5.2, 5.3 |

## 注意事項

- `network_analyzer.go` 削除ケースのコメント（文脈説明）は `case err == nil:` 側のコメントに吸収すること
- `standard_analyzer.go` では `if err != nil` ブロックを抜けた後に `result == nil` となる
  新ケースが生じるため、`convertSyscallResult` 呼び出し前に必ず nil チェックを追加すること
  （nil ポインタパニック防止）
