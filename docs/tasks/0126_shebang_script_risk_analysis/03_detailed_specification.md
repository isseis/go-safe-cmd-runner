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
    //   - The interpreter's record is not found (ErrRecordNotFound for interpreter)
    //   - The interpreter's content hash is empty
    //
    // Returns error when:
    //   - The script's record cannot be loaded (non-ErrRecordNotFound errors)
    //   - scriptContentHash does not match stored hash (ErrHashMismatch)
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
    store Store  // 既存の Record ストア
}

// NewShebangInterpreterStore creates a ShebangInterpreterStore backed by store.
func NewShebangInterpreterStore(store Store) ShebangInterpreterStore {
    return &shebangInterpreterStore{store: store}
}
```

#### 処理手順

1. `validatePath(scriptPath)` → `common.ResolvedPath`
2. `store.Load(scriptTarget)` でスクリプトのレコードをロード
   - `ErrRecordNotFound` → `return "", "", nil`
   - 他のエラー → `return "", "", err`
3. `scriptRecord.ContentHash != scriptContentHash` → `return "", "", ErrHashMismatch`
4. `scriptRecord.ShebangInterpreter == nil` → `return "", "", nil`
5. インタープリタパスの決定:
   - `si.ResolvedPath != ""` → `interpPath = si.ResolvedPath`
   - それ以外 → `interpPath = si.InterpreterPath`
6. `validatePath(interpPath)` → `common.ResolvedPath`
7. `store.Load(interpTarget)` でインタープリタのレコードをロード
   - `ErrRecordNotFound` → `return interpPath, "", nil`（ハッシュなし=スキップ可能）
   - 他のエラー → `return "", "", err`
8. `return interpPath, interpRecord.ContentHash, nil`

### 2.3. エラー処理一覧

| 条件 | 返却値 | 意味 |
|------|--------|------|
| スクリプトレコード不在 | `("", "", nil)` | 解析スキップ |
| `contentHash` 不一致 | `("", "", ErrHashMismatch)` | ファイル変更検知 |
| `ShebangInterpreter == nil` | `("", "", nil)` | 非スクリプト |
| インタープリタレコード不在 | `(interpPath, "", nil)` | ハッシュ不明（スキップ）|
| その他のエラー | `("", "", err)` | fail-closed |

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

既存の解析（`SymbolAnalysis`・`SyscallAnalysis`・`DynLibDeps`）の実行後に shebang チェーン追跡を追加する。挿入位置は関数末尾の `return isNetwork, hasDynLoad` の直前。

```go
// Shebang chain: if this is a script, also analyze the interpreter binary.
// The interpreter's record (SymbolAnalysis, SyscallAnalysis, DynLibDeps)
// was written by `record` when the script was first recorded.
if a.shebangStore != nil && contentHash != "" {
    interpPath, interpHash, err := a.shebangStore.LoadInterpreterAnalysisPath(cmdPath, contentHash)
    switch {
    case err == nil:
        if interpPath != "" && interpHash != "" {
            interpNet, interpHigh := a.analyzeBinarySignals(interpPath, interpHash)
            isNetwork = isNetwork || interpNet
            hasDynLoad = hasDynLoad || interpHigh
        }
    case errors.Is(err, fileanalysis.ErrHashMismatch):
        slog.Warn("shebang interpreter script hash mismatch; treating as high risk",
            "path", cmdPath)
        return true, true
    default:
        slog.Warn("shebang interpreter lookup failed; treating as high risk",
            "path", cmdPath, "error", err)
        return true, true
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
| TC-03 | スクリプトレコード不在 | `("", "", nil)` |
| TC-04 | `contentHash` 不一致 | `("", "", ErrHashMismatch)` |
| TC-05 | `ShebangInterpreter == nil` | `("", "", nil)` |
| TC-06 | インタープリタレコード不在 | `(interpPath, "", nil)` |
| TC-07 | インタープリタロードエラー | `("", "", error)` |

### 5.2. `analyzeBinarySignals` shebang 拡張テスト

**ファイル:** `internal/runner/base/security/network_analyzer_test.go`

| テストケース | 条件 | 期待結果 |
|------------|------|---------|
| TC-11 | インタープリタが `socket` シンボルを持つ | `isNetwork = true` |
| TC-12 | インタープリタの共有ライブラリが mprotect リスクを持つ | `isHighRisk = true` |
| TC-13 | インタープリタの共有ライブラリが `dlopen` を持つ | `isHighRisk = true` |
| TC-14 | インタープリタレコードのハッシュが不明（`""`）| スキップ、`(false, false)` |
| TC-15 | `ErrHashMismatch` | `(true, true)` |
| TC-16 | ロードエラー | `(true, true)` |
| TC-17 | `shebangStore == nil` | 変更なし（既存動作）|
| TC-18 | 非スクリプト（`ShebangInterpreter == nil`）| 変更なし（AC-07）|

### 5.3. 対応受け入れ基準

| 受け入れ基準 | テスト |
|------------|-------|
| AC-01: bash の network シンボル | TC-11 |
| AC-02: env 形式 python3 | TC-02 |
| AC-03: インタープリタ共有ライブラリの mprotect リスク | TC-12 |
| AC-04: ライブラリの dynload シンボル | TC-13 |
| AC-05: インタープリタレコード不在はスキップ | TC-14, TC-06 |
| AC-06: ロードエラーで high risk | TC-15, TC-16 |
| AC-07: ELF バイナリの回帰 | TC-18 |
| AC-08: ハッシュ不明はスキップ | TC-14 |

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
