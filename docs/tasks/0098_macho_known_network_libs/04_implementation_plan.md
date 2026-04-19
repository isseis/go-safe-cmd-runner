# Mach-O 既知ネットワークライブラリ検出 実装計画書

## 1. 実装の進め方

本タスクの変更は `internal/filevalidator/validator.go` の1箇所のみ。
`filepath.Base()` を追加してベース名を正規化してから `IsKnownNetworkLibrary()` に渡す。

### 実装ステップ概要

1. **Step 1**: `validator.go` の `KnownNetworkLibDeps` 導出ロジック修正
2. **Step 2**: テストの追加
3. **Step 3**: ビルド・テスト・リント確認

---

## 2. Step 1: `validator.go` の修正

**対象ファイル**: `internal/filevalidator/validator.go`

### 2.1 実装チェックリスト

- [ ] `"path/filepath"` のインポートが既に存在することを確認する（なければ追加する）
- [ ] `KnownNetworkLibDeps` 導出ループを以下のように修正する:

```go
// 変更前
for _, lib := range record.DynLibDeps {
    if binaryanalyzer.IsKnownNetworkLibrary(lib.SOName) {
        matched = append(matched, lib.SOName)
    }
}

// 変更後
for _, lib := range record.DynLibDeps {
    base := filepath.Base(lib.SOName)
    if binaryanalyzer.IsKnownNetworkLibrary(base) {
        matched = append(matched, lib.SOName)
    }
}
```

**注意**: `matched` に記録する値は `lib.SOName`（インストール名）のまま変更しない。

---

## 3. Step 2: テストの追加

**対象ファイル**: `internal/filevalidator/validator_test.go`

既存の `KnownNetworkLibDeps` テスト群（`TestRecord_KnownNetworkLibDeps_*`）の末尾に追加する。

### 3.1 テストチェックリスト

- [ ] `TestRecord_KnownNetworkLibDeps_MachoInstallNameRuby`
  - `DynLibDeps: [{SOName: "/usr/local/opt/ruby/lib/libruby.3.2.dylib", ...}]` でインストール名がそのまま `KnownNetworkLibDeps` に記録される
- [ ] `TestRecord_KnownNetworkLibDeps_MachoInstallNameCurl`
  - `DynLibDeps: [{SOName: "/usr/local/lib/libcurl.4.dylib", ...}]` で記録される（AC-2）
- [ ] `TestRecord_KnownNetworkLibDeps_MachoInstallNamePython`
  - `DynLibDeps: [{SOName: "/usr/local/opt/python/lib/libpython3.11.dylib", ...}]` で記録される（AC-3）
- [ ] `TestRecord_KnownNetworkLibDeps_MachoRpathInstallName`
  - `DynLibDeps: [{SOName: "@rpath/libcurl.dylib", ...}]` で記録される（`@rpath/` プレフィックス付き）
- [ ] `TestRecord_KnownNetworkLibDeps_MachoNonNetworkLib`
  - `DynLibDeps: [{SOName: "/usr/lib/libz.1.dylib", ...}]` で `KnownNetworkLibDeps` が空（AC-5）
- [ ] `TestRecord_KnownNetworkLibDeps_MachoFalsePositivePrefix`
  - `DynLibDeps: [{SOName: "/usr/local/lib/libpythonista.dylib", ...}]` で記録されない（AC-6）
- [ ] 既存の ELF テスト（`TestRecord_KnownNetworkLibDeps_CurlDetected` 等）が引き続きパスする（AC-7）

**テストヘルパー**: 既存の `recordWithDynLibDepsAndBinaryAnalyzer()` を利用する。`DynLibDeps` の `Path` と `Hash` はダミー値で可（照合ロジックは `SOName` のみ参照）。

**実行コマンド**:
```
go test -tags test -v ./internal/filevalidator/ -run TestRecord_KnownNetworkLibDeps
```

---

## 4. Step 3: ビルド・テスト・リント確認

### 4.1 確認チェックリスト

- [ ] `make fmt` でフォーマット適用後に変更差分なし
- [ ] `make build` でビルドエラーなし
- [ ] `make test` で全テストパス
- [ ] `make lint` でリントエラーなし

---

## 5. リスクと対策

| リスク | 影響 | 対策 |
|-------|------|------|
| `filepath.Base()` が ELF SOName に副作用を与える | 既存の ELF 検出が壊れる | ELF SOName はスラッシュを含まないため `filepath.Base()` で変化しない。AC-7 のテストで回帰確認 |
| `matched` に記録する値の変更 | runner ログや比較ロジックへの影響 | インストール名をそのまま記録する方針を維持。`network_analyzer.go` の判定は非空チェックのみで値の形式に依存しない |
