# shebang スクリプトのネットワークリスク解析 詳細仕様書

## 1. 変更対象ファイル

| ファイル | 変更種別 |
|--------|---------|
| `internal/fileanalysis/shebang_store.go` | 新規（`ShebangInterpreterStore` インターフェース・実装）|
| `internal/runner/base/security/network_analyzer.go` | 変更（`shebangStore` フィールド追加・`analyzeBinarySignals` 拡張）|
| `internal/runner/base/security/network_analyzer_test.go` | テスト追加 |
| `internal/runner/base/risk/evaluator.go` | 変更（`NewNetworkAnalyzer` 呼び出しに `shebangStore` を追加）|

---

## 2. `ShebangInterpreterStore` インターフェースと実装

### 2.1. インターフェース定義

**ファイル:** `internal/fileanalysis/shebang_store.go`

```go
package fileanalysis

// ShebangInterpreterStore provides the interpreter binary path and content hash
// for a shebang script, enabling the runner to follow the shebang chain for
// risk assessment without re-analyzing the interpreter binary.
type ShebangInterpreterStore interface {
    // LoadInterpreterAnalysisPath returns the effective interpreter binary path
    // and its content hash for the shebang script at scriptPath.
    //
    // scriptContentHash is validated against the stored record to detect
    // script file changes (ErrHashMismatch).
    //
    // Returns ("", "", nil) when:
    //   - The script has no ShebangInterpreter (not a shebang script)
    //
    // Returns error when:
    //   - The script's record cannot be loaded (non-ErrRecordNotFound errors)
    //   - scriptContentHash does not match stored hash (ErrHashMismatch)
    //   - The interpreter's record is not found (ErrInterpreterRecordMissing)
    //   - The interpreter's content hash is empty (ErrInterpreterRecordMissing)
    //   - The interpreter's record cannot be loaded (non-ErrRecordNotFound errors)
    LoadInterpreterAnalysisPath(scriptPath, scriptContentHash string) (interpPath, interpContentHash string, err error)
}
```

### 2.2. 実装: `shebangInterpreterStore`

**ファイル:** `internal/fileanalysis/shebang_store.go`

```go
// shebangInterpreterStore implements ShebangInterpreterStore using the file
// analysis record store.
type shebangInterpreterStore struct {
    store *Store  // 既存の Record ストア（他のストア実装と同様にポインタを使用）
}

// NewShebangInterpreterStore creates a ShebangInterpreterStore backed by store.
func NewShebangInterpreterStore(store *Store) ShebangInterpreterStore {
    return &shebangInterpreterStore{store: store}
}
```

#### 処理手順

1. `common.NewResolvedPath(scriptPath)` → `common.ResolvedPath`（`networkSymbolStore` 等の既存実装と同じパターン）
2. `store.Load(scriptTarget)` でスクリプトのレコードをロード
   - `ErrRecordNotFound` → `return "", "", fileanalysis.ErrRecordNotFound`（呼び出し元で panic）
   - 他のエラー → `return "", "", err`
3. `scriptRecord.ContentHash != scriptContentHash` → `return "", "", ErrHashMismatch`
4. `scriptRecord.ShebangInterpreter == nil` → `return "", "", nil`
5. インタープリタパスの決定:
   - `si.ResolvedPath != ""` → `interpPath = si.ResolvedPath`
   - それ以外 → `interpPath = si.InterpreterPath`
6. `common.NewResolvedPath(interpPath)` → `common.ResolvedPath`
7. `store.Load(interpTarget)` でインタープリタのレコードをロード
   - `ErrRecordNotFound` → `return "", "", fmt.Errorf("interpreter record not found: %w", ErrInterpreterRecordMissing)`
   - 他のエラー → `return "", "", err`
8. `interpRecord.ContentHash == ""` → `return "", "", fmt.Errorf("interpreter record has empty content hash: %w", ErrInterpreterRecordMissing)`
9. `return interpPath, interpRecord.ContentHash, nil`

### 2.3. エラー処理一覧

| 条件 | 返却値 | 呼び出し側の扱い |
|------|--------|----------------|
| スクリプトレコード不在 | `("", "", ErrRecordNotFound)` | panic（`analyzeBinarySignals` で検出）|
| スクリプト `contentHash` 不一致 | `("", "", ErrHashMismatch)` | エラー返却（当該グループ実行中止）|
| `ShebangInterpreter == nil` | `("", "", nil)` | スキップ（非スクリプト）|
| インタープリタレコード不在 | `("", "", ErrInterpreterRecordMissing)` | エラー返却（当該グループ実行中止）|
| インタープリタ `ContentHash` 空 | `("", "", ErrInterpreterRecordMissing)` | エラー返却（当該グループ実行中止）|
| その他のエラー（インタープリタレコードロード失敗含む） | `("", "", err)` | エラー返却（当該グループ実行中止）|

`ErrInterpreterRecordMissing` は `fileanalysis` パッケージに新規追加する sentinel error。

---

## 3. `NetworkAnalyzer` の変更

### 3.1. フィールド追加

**ファイル:** `internal/runner/base/security/network_analyzer.go`

```go
type NetworkAnalyzer struct {
    goos             string
    store            fileanalysis.NetworkSymbolStore
    syscallStore     fileanalysis.SyscallAnalysisStore
    depsStore        fileanalysis.DynLibDepsStore
    libAnalysisStore dynamicanalysis.Store
    shebangStore     fileanalysis.ShebangInterpreterStore  // 追加（nil で無効化）
}
```

### 3.2. `NewNetworkAnalyzer` の変更

引数に `shebangStore fileanalysis.ShebangInterpreterStore` を追加:

```go
func NewNetworkAnalyzer(
    goos string,
    symStore fileanalysis.NetworkSymbolStore,
    svcStore fileanalysis.SyscallAnalysisStore,
    depsStore fileanalysis.DynLibDepsStore,
    libAnalysisStore dynamicanalysis.Store,
    shebangStore fileanalysis.ShebangInterpreterStore,  // 追加
) *NetworkAnalyzer {
    return &NetworkAnalyzer{
        // ...
        shebangStore: shebangStore,
    }
}
```

### 3.3. `analyzeBinarySignals` の変更

既存の解析（`SymbolAnalysis`・`SyscallAnalysis`・`DynLibDeps`）の実行後に shebang チェーン追跡を追加する。インタープリタレコード不在は high risk に丸めず、エラーとして呼び出し元へ返却する。

> **設計上の意図:** このタスクは意図的に `analyzeBinarySignals` の戻り値に `error` を追加し、ランナーの契約を変更する。既存の `ErrHashMismatch` 等の処理（`return true, true`）とは異なり、shebang インタープリタのレコード不在・ハッシュ不一致はエラーとして伝播させ、グループ実行を中止する。これは AC-06・AC-07 で定義された fail-closed ポリシーに従ったものであり、「解析結果が得られない場合は実行を継続しない」という設計方針の一貫性を確保するためである。

このため、`analyzeBinarySignals` は `error` を返せる形に拡張し、`IsNetworkOperation` → `EvaluateRisk` → グループ実行層へ伝播させる。
`ErrHashMismatch` とインタープリタレコードロードエラーは high risk へ丸めず、実行中止エラーとして扱う。

```go
// Shebang chain: if this is a script, also analyze the interpreter binary.
// The interpreter's record (SymbolAnalysis, SyscallAnalysis, DynLibDeps)
// was written by `record` when the script was first recorded.
if a.shebangStore != nil && contentHash != "" {
    interpPath, interpHash, err := a.shebangStore.LoadInterpreterAnalysisPath(cmdPath, contentHash)
    switch {
    case err == nil:
        if interpPath != "" {
            interpNet, interpHigh := a.analyzeBinarySignals(interpPath, interpHash)
            isNetwork = isNetwork || interpNet
            hasDynLoad = hasDynLoad || interpHigh
        }
    case errors.Is(err, fileanalysis.ErrRecordNotFound):
        // Script record must exist at this point (LoadNetworkSymbolAnalysis already
        // succeeded). A missing record now indicates a programming error.
        panic(fmt.Sprintf("shebang store: script record disappeared for %q: %v", cmdPath, err))
    case errors.Is(err, fileanalysis.ErrHashMismatch):
        return false, false, fmt.Errorf("shebang script hash mismatch for %q: %w", cmdPath, err)
    case errors.Is(err, fileanalysis.ErrInterpreterRecordMissing):
        return false, false, fmt.Errorf("shebang interpreter record missing for %q: %w", cmdPath, err)
    default:
        return false, false, fmt.Errorf("shebang interpreter lookup failed for %q: %w", cmdPath, err)
    }
}
```

---

## 4. `NewNetworkAnalyzer` 呼び出し箇所の変更

### 4.1. `internal/runner/base/risk/evaluator.go`

`NewNetworkAnalyzer` の呼び出しに `shebangStore` を追加する。

```go
networkAnalyzer := security.NewNetworkAnalyzer(
    runtime.GOOS,
    symStore,
    syscallStore,
    depsStore,
    libAnalysisStore,
    shebangStore, // 追加
)
```

### 4.2. その他の `NewNetworkAnalyzer` 呼び出し箇所

`grep -rn "NewNetworkAnalyzer"` で全呼び出し箇所を確認し、`nil` またはストア実装を適切に渡す。

---

## 5. テスト仕様

### 5.1. `ShebangInterpreterStore` ユニットテスト

**ファイル:** `internal/fileanalysis/shebang_store_test.go`

| テストケース | 条件 | 期待結果 |
|------------|------|---------|
| TC-01 | direct 形式、両レコード存在 | `(interpPath, interpHash, nil)` |
| TC-02 | env 形式、`ResolvedPath` が使用される | `ResolvedPath` のレコードハッシュを返す |
| TC-03 | スクリプトレコード不在 | `("", "", ErrRecordNotFound)` |
| TC-04 | `contentHash` 不一致 | `("", "", ErrHashMismatch)` |
| TC-05 | `ShebangInterpreter == nil` | `("", "", nil)` |
| TC-06 | インタープリタレコード不在 | `("", "", ErrInterpreterRecordMissing)` |
| TC-07 | インタープリタロードエラー | `("", "", error)` |
| TC-08 | インタープリタ `ContentHash` 空 | `("", "", ErrInterpreterRecordMissing)` |

### 5.2. `analyzeBinarySignals` shebang 拡張テスト

**ファイル:** `internal/runner/base/security/network_analyzer_test.go`

| テストケース | 条件 | 期待結果 |
|------------|------|---------|
| TC-11 | インタープリタが `socket` シンボルを持つ | `isNetwork = true` |
| TC-12 | インタープリタの共有ライブラリが mprotect リスクを持つ | `isHighRisk = true` |
| TC-13 | インタープリタの共有ライブラリが `dlopen` を持つ | `isHighRisk = true` |
| TC-14 | インタープリタレコード不在（`ErrInterpreterRecordMissing`）| エラーを返し、グループ実行を中止 |
| TC-15 | `ErrHashMismatch` | エラーを返し、グループ実行を中止 |
| TC-16 | ロードエラー | エラーを返し、グループ実行を中止 |
| TC-17 | `shebangStore == nil` | 変更なし（既存動作）|
| TC-18 | 非スクリプト（`ShebangInterpreter == nil`）| 変更なし（AC-07）|

### 5.3. 対応受け入れ基準

| 受け入れ基準 | テスト |
|------------|-------|
| AC-01: bash の network シンボル | TC-11 |
| AC-02: env 形式 python3 | TC-02 |
| AC-03: インタープリタ共有ライブラリの mprotect リスク | TC-12 |
| AC-04: ライブラリの dynload シンボル | TC-13 |
| AC-05: スクリプトレコード不在は panic | TC-03 |
| AC-06: インタープリタレコード不在は実行中止エラー | TC-06, TC-14 |
| AC-07: ハッシュ不一致またはロードエラーは実行中止エラー | TC-15, TC-16 |
| AC-08: ELF バイナリの回帰 | TC-18 |

---

## 6. 実装上の注意点

### 6.1. 再帰の安全性

`analyzeBinarySignals(interpPath, interpHash)` の再帰呼び出しにおいて:
- `interpPath` はネイティブバイナリ（ELF/Mach-O）のパス
- ネイティブバイナリの record には `ShebangInterpreter = nil` が格納される
- `LoadInterpreterAnalysisPath` は `ShebangInterpreter == nil` の場合 `("", "", nil)` を返す
- 再帰の深さは最大 1 段

### 6.2. `shebangStore` の nil 許容

`shebangStore == nil` または `contentHash == ""` の場合、shebang チェーン追跡全体をスキップする。
これにより、テスト環境やストアが未設定の環境でも既存の動作を維持できる。

### 6.3. `NewNetworkAnalyzer` のシグネチャ変更

`NewNetworkAnalyzer` の引数追加は破壊的変更のため、全呼び出し箇所を更新する必要がある。
`grep -rn "NewNetworkAnalyzer"` でカバレッジを確認すること。

### 6.4. ログ出力方針

エラーケースは既存の `slog.Warn` パターンに従う（`analyzeBinarySignals` 内の他の警告と統一）。
正常時の shebang チェーン追跡はログ出力しない（過剰なログを避ける）。
