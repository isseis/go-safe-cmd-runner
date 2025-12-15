# ドキュメント検証クイックスタート

## 最速で始める

```bash
# プロジェクトルートから実行
make verify-docs
```

これだけで、すべての検証が実行され、レポートが `build/verification-reports/` に生成されます。

## レポートの確認

```bash
# テキストレポートを閲覧
cat build/verification-reports/toml_keys_report.txt
cat build/verification-reports/cli_args_report.txt
cat build/verification-reports/structure_comparison_report.txt
cat build/verification-reports/links_report.txt
```

## よく使うコマンド

### 基本的な検証（推奨）

```bash
make verify-docs
```

または

```bash
./scripts/verification/run_all.sh
```

### 詳細な検証（外部リンクチェック含む）

```bash
make verify-docs-full
```

または

```bash
./scripts/verification/run_all.sh -v -e
```

### 個別スクリプトの実行

```bash
# TOML設定キー検証のみ
cd scripts/verification
go run verify_toml_keys.go --source=../../internal --docs=../../docs/user --verbose

# CLI引数検証のみ
go run verify_cli_args.go --source=../../cmd --docs=../../docs/user --verbose

# ドキュメント構造比較のみ
go run compare_doc_structure.go --docs=../../docs/user --verbose

# リンク検証のみ
go run verify_links.go --docs=../../docs --verbose
```

## レポートの見方

### ✅ 成功

```
✓ TOML keys verification passed
```

問題なし。ドキュメントと実装が整合しています。

### ⚠️ 警告

```
⚠️  Keys in CODE but NOT in DOCS (1):
  - command_templates (struct: ConfigSpec, file: spec.go:75)
```

アクションが必要です。この例では、`command_templates`キーがコードに存在しますが、ドキュメントに記載されていません。

## 次のステップ

1. レポートを確認
2. 不一致を修正
3. 再度検証を実行
4. すべて✅になるまで繰り返し

## 詳細情報

- [automation_scripts_summary.md](automation_scripts_summary.md) - 自動化スクリプトの詳細
- [scripts/verification/README.md](../../../scripts/verification/README.md) - 各スクリプトの使用方法
- [execution_plan.md](execution_plan.md) - 検証プロジェクトの全体計画

## トラブルシューティング

### スクリプトが見つからない

```bash
# 実行権限を付与
chmod +x scripts/verification/run_all.sh
```

### ビルドエラー

```bash
# 依存関係を更新
go mod tidy

# クリーンビルド
rm -rf build/verification-reports
make verify-docs
```

### 詳しいヘルプ

```bash
./scripts/verification/run_all.sh --help
```
