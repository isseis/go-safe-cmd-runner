# L3: 将来の ulid バージョン追従時の監視事項

- **重大度**: 🔵 Info
- **領域**: 依存ライブラリ (`internal/logging`)
- **影響コマンド**: `record`, `verify`, `runner`

## 概要

本プロジェクトは実行 ID (run ID) 生成に `github.com/oklog/ulid/v2 v2.1.1` を使用している ([internal/logging/safeopen.go:57-60](../../../internal/logging/safeopen.go#L57-L60))。現バージョンでは以下の実装になっており、**暗号論的に安全な乱数源** (`crypto/rand`) を使っているため問題はない。

```go
entropy := ulid.Monotonic(rand.Reader, 0) // rand は crypto/rand
id := ulid.MustNew(ulid.Timestamp(time.Now()), entropy)
```

## 監視事項 (なぜこれを記録するか)

- ulid ライブラリは歴史的に `math/rand` を使うサンプルコードや helper を提供していた時期があった。
- ライブラリのバージョンアップに伴いサンプルや API の **推奨利用方法** が変わる可能性がある。
- 依存更新時に誤って `math/rand` を使う経路に切り替えると、run ID が予測可能になり **ログファイル名の衝突/上書き** や、ログ追跡情報の予測による攻撃 (run ID を推測して特定のログ出力を狙った競合) が理論的に可能になる。

## 対応

### 現時点ではアクション不要

- 現コードは `crypto/rand` を明示指定。
- このドキュメントの目的は「将来の依存バージョンアップ時にチェックする」ためのメモ。

### バージョンアップ時のチェックリスト

- [ ] `ulid.Monotonic` の第一引数が `rand.Reader` (crypto/rand パッケージ) であることを確認
- [ ] `import "math/rand"` が `internal/logging` に混入していないことを確認
- [ ] ulid 本体の CHANGELOG で entropy source のデフォルトが変わっていないことを確認
- [ ] `go test -run TestGenerateRunID ./internal/logging/...` で run ID の一意性テストが通ることを確認

## 参考箇所

- [internal/logging/safeopen.go:57-60](../../../internal/logging/safeopen.go#L57-L60) — `GenerateRunID` の実装
- [docs/tasks/0088_reduce_external_dependencies/01_requirements.md](../0088_reduce_external_dependencies/01_requirements.md) — ulid 維持の判断根拠
