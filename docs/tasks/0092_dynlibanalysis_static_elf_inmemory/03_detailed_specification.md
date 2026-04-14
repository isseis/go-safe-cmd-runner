# 詳細仕様書: TestAnalyze_StaticELF のインメモリ ELF 生成への移行

## 0. 変更方針

変更対象は `internal/dynlibanalysis/analyzer_test.go` の `TestAnalyze_StaticELF` 関数の本体のみ。
既存の `buildTestELFWithDeps` ヘルパーを再利用し、新たなヘルパー追加や他テスト関数への影響はない。

---

## 1. 変更対象ファイル一覧

| ファイル | 変更種別 | 概要 |
|---------|---------|------|
| `internal/dynlibanalysis/analyzer_test.go` | 変更 | `TestAnalyze_StaticELF` の本体をインメモリ生成方式に書き換え |

---

## 2. 変更詳細

### 2.1 `TestAnalyze_StaticELF` (`internal/dynlibanalysis/analyzer_test.go`)

**変更前**:

```go
// TestAnalyze_StaticELF verifies that Analyze returns nil for a static ELF
// (no DT_NEEDED entries).
func TestAnalyze_StaticELF(t *testing.T) {
	staticELF := "../runner/security/elfanalyzer/testdata/static.elf"
	if _, err := os.Stat(staticELF); err != nil {
		t.Skipf("static.elf testdata not accessible: %v", err)
	}

	a := newTestAnalyzer(t)
	result, err := a.Analyze(staticELF)
	require.NoError(t, err)
	assert.Nil(t, result, "static ELF with no DT_NEEDED should return nil")
}
```

**変更後**:

```go
// TestAnalyze_StaticELF verifies that Analyze returns nil for a static ELF
// (no DT_NEEDED entries).
func TestAnalyze_StaticELF(t *testing.T) {
	tmpDir := t.TempDir()
	// sonames=nil produces an ELF with no DT_NEEDED entries.
	staticELF := buildTestELFWithDeps(t, tmpDir, "static.elf", nil, "")

	a := newTestAnalyzer(t)
	result, err := a.Analyze(staticELF)
	require.NoError(t, err)
	assert.Nil(t, result, "static ELF with no DT_NEEDED should return nil")
}
```

### 2.2 削除する要素

| 要素 | 削除理由 |
|------|---------|
| `staticELF := "../runner/security/elfanalyzer/testdata/static.elf"` | インメモリ生成へ移行するため不要 |
| `os.Stat(staticELF)` によるファイル存在確認 | インメモリ生成ではファイルの事前存在が不要 |
| `t.Skipf(...)` によるスキップ処理 | 外部ファイル依存がなくなりスキップ条件が消滅 |

### 2.3 `os` パッケージのインポート

`os` パッケージは同ファイル内の `TestAnalyze_NonELF` で引き続き使用されるため、インポート宣言の変更は不要。

---

## 3. `buildTestELFWithDeps` の動作仕様（静的 ELF 生成に関する部分）

`buildTestELFWithDeps(t, dir, fileName, nil, "")` を呼び出したとき:

- `sonames` が `nil`（または空スライス）の場合、DT_NEEDED エントリは生成されない
- `runpath` が空文字列の場合、DT_RUNPATH エントリは生成されない
- `.dynamic` セクションには `DT_STRTAB`、`DT_STRSZ`、`DT_NULL` の 3 エントリのみが書き込まれる
- `DynLibAnalyzer.Analyze()` はこの ELF を解析し、DT_NEEDED が存在しないため `nil` を返す

---

## 4. テストと受け入れ基準の対応

| 受け入れ基準 | 対応箇所 |
|------------|---------|
| AC-1: `elfanalyzer/testdata` パス参照の除去 | `staticELF := "../runner/security/elfanalyzer/testdata/static.elf"` を削除 |
| AC-1: `t.Skip` / `t.Skipf` の除去 | `t.Skipf(...)` を削除 |
| AC-2: インメモリ ELF による検証 | `buildTestELFWithDeps(t, tmpDir, "static.elf", nil, "")` を使用 |
| AC-2: `Analyze()` が nil を返すことの検証 | `assert.Nil(t, result, ...)` で検証（変更なし） |
| AC-3: `make elfanalyzer-testdata` 不要での PASS | 外部ファイル依存が解消されるため、実装後に達成予定 |
| AC-3: `make test` がすべてパス | テストロジックの変更は最小限であり、実装後に確認 |
| AC-3: `make lint` がエラーなし | 削除した変数 `err` はスコープが消えるため未使用変数エラーは発生しない想定。実装後に確認 |
