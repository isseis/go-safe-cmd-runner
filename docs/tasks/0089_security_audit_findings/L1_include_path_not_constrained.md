# L1: template include のパス正規化不足

- **重大度**: 🟠 Low
- **領域**: 設定ローダ (`internal/runner/config`)
- **影響コマンド**: `record`, `verify`, `runner`

## 問題

TOML 設定の template include 機構 ([internal/runner/config/path_resolver.go](../../../internal/runner/config/path_resolver.go)) において、include 対象のパスが設定ファイルの基底ディレクトリ配下に限定されていない。

### 挙動

- include パスが相対パスの場合、基底ディレクトリ (メイン config のディレクトリ) 起点で解決される。
- しかし `../` 成分を含む場合、解決後のパスが基底ディレクトリの外に出ることを防ぐチェックがない。
- 絶対パスが指定された場合もそのまま受け入れる。

## 影響

### 緩和要因 (大きい)

すべての include 対象ファイルは **事前にハッシュ検証対象** ([verification/manager.go](../../../internal/verification/manager.go) の `VerifyAndReadConfigFile`) となる。したがって:

1. 攻撃者が include パスを `../../../etc/passwd` に向けても、`/etc/passwd` のハッシュが `--hash-dir` に登録されていない限り検証エラーで停止する。
2. 検証された config ファイル自体の内容を攻撃者が制御できない限り、任意 include による情報漏洩経路は閉じている。

### 残存リスク

- ハッシュディレクトリに **意図せず** 登録された機密ファイル (例: 昔 `record` で登録した `/etc/hostname` のハッシュが残っている) があれば、悪意ある config により内容が読み出される。
- パス正規化の欠如自体は「単なる記述ミス」を早期に検出できず、設定ファイルの可読性・保守性を下げる。

## 修正方針

### 案 A: 基底ディレクトリに制約

include 解決後のパスを `filepath.Clean` し、`basedir` のプレフィックスであることを確認。

```go
resolved := filepath.Clean(filepath.Join(basedir, includePath))
rel, err := filepath.Rel(basedir, resolved)
if err != nil || strings.HasPrefix(rel, "..") {
    return fmt.Errorf("include path %q escapes basedir", includePath)
}
```

### 案 B: 絶対パスも許容するが明示的ルール化

絶対パスは許容するが、相対パスは basedir に制約。ドキュメントで使い分けを明示。

### 推奨

**案 A + 絶対パス拒否**。絶対パスで include する必然性は低く、明示的相対パスのみに絞ることで「どこを読むか」が config 階層から一目瞭然になる。

## 参考箇所

- [internal/runner/config/path_resolver.go](../../../internal/runner/config/path_resolver.go) — include パス解決
- [internal/verification/manager.go](../../../internal/verification/manager.go) — `VerifyAndReadConfigFile` (ハッシュ検証)
