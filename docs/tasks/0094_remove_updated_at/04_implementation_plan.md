# 実装計画書: updated_at フィールドの削除

## 変更ファイル一覧

### プロダクションコード

| ファイル | 変更内容 |
|---|---|
| `internal/fileanalysis/schema.go` | `UpdatedAt` フィールド削除、`time` インポート削除、`CurrentSchemaVersion` を 13 に更新、スキーマ履歴コメント追加、v9 コメント修正 |
| `internal/fileanalysis/file_analysis_store.go` | `record.UpdatedAt = time.Now().UTC()` 行削除、`time` インポート削除 |

### テストコード

| ファイル | 変更内容 |
|---|---|
| `internal/fileanalysis/file_analysis_store_test.go` | `UpdatedAt.IsZero()` アサーション削除（4 箇所）、`updated_at` キー削除（4 箇所）、`time` インポート削除 |
| `internal/fileanalysis/network_symbol_store_test.go` | `updated_at` キー削除（1 箇所）、`time` インポート削除 |
| `internal/filevalidator/validator_test.go` | `UpdatedAt.IsZero()` アサーション削除（2 箇所） |
| `internal/verification/manager_test.go` | `updated_at` キー削除（2 箇所） |

## 実装手順

- [ ] `internal/fileanalysis/schema.go` を修正する
  - `CurrentSchemaVersion` を 13 に更新
  - スキーマ履歴コメントに `Version 13 removes UpdatedAt field` を追加
  - v9 コメント (`use Record.UpdatedAt instead`) を修正
  - `UpdatedAt` フィールドと対応コメントを削除
  - `time` インポートを削除
- [ ] `internal/fileanalysis/file_analysis_store.go` を修正する
  - `record.UpdatedAt = time.Now().UTC()` 行を削除
  - `time` インポートを削除
- [ ] `internal/fileanalysis/file_analysis_store_test.go` を修正する
  - `UpdatedAt.IsZero()` アサーションを削除
  - テスト用 JSON マップから `updated_at` キーを削除
  - `time` インポートを削除（他で使用していなければ）
- [ ] `internal/fileanalysis/network_symbol_store_test.go` を修正する
  - テスト用 JSON マップから `updated_at` キーを削除
  - `time` インポートを削除（他で使用していなければ）
- [ ] `internal/filevalidator/validator_test.go` を修正する
  - `UpdatedAt.IsZero()` アサーションを削除
- [ ] `internal/verification/manager_test.go` を修正する
  - テスト用 JSON マップから `updated_at` キーを削除
- [ ] `make fmt` を実行してフォーマットを確認する
- [ ] `make test` を実行して全テストが通ることを確認する
- [ ] `make lint` を実行して lint エラーがないことを確認する
