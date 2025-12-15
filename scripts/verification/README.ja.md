# Documentation Verification Scripts

このディレクトリには、日本語ドキュメントと実装コードの整合性を検証するための自動化スクリプトが含まれています。

## Overview

これらのスクリプトは、[docs/tasks/0064_ja_docs_implementation_verification/execution_plan.md](../../docs/tasks/0064_ja_docs_implementation_verification/execution_plan.md) の「8. 自動化の検討」セクションで提案された自動化を実装したものです。

## Scripts

### 1. verify_toml_keys.go

TOMLの設定キーをGoのソースコードから抽出し、ドキュメントに記載されているキーと比較します。

**機能:**
- Goコードからstructタグの`toml:"key"`を抽出
- ドキュメント内のTOMLキーを抽出
- 両者を比較して、不足しているキーや余分なキーを検出

**使用方法:**
```bash
go run verify_toml_keys.go \
  --source=../../internal \
  --docs=../../docs/user \
  --verbose \
  --output=toml_report.json
```

**オプション:**
- `--source`: Goソースコードのルートディレクトリ（デフォルト: `.`）
- `--docs`: ドキュメントのルートディレクトリ（デフォルト: `docs/user`）
- `--verbose`: 詳細な出力
- `--output`: JSON形式のレポート出力先（オプション）

### 2. verify_cli_args.go

コマンドライン引数をGoのflagパッケージの呼び出しから抽出し、ドキュメントと比較します。

**機能:**
- `flag.String()`, `flag.Bool()` などの呼び出しを解析
- コマンド名、引数名、型、デフォルト値、説明を抽出
- ドキュメント内の引数記述と比較

**使用方法:**
```bash
go run verify_cli_args.go \
  --source=../../cmd \
  --docs=../../docs/user \
  --verbose \
  --output=cli_report.json
```

**オプション:**
- `--source`: コマンドソースコードのディレクトリ（デフォルト: `cmd`）
- `--docs`: ドキュメントのルートディレクトリ（デフォルト: `docs/user`）
- `--verbose`: 詳細な出力
- `--output`: JSON形式のレポート出力先（オプション）

### 3. compare_doc_structure.go

日本語ドキュメント（.ja.md）と英語ドキュメント（.md）の構造を比較します。

**機能:**
- 見出しレベルと数の比較
- コードブロック、テーブル、リストの数を比較
- セクション構造の差異を検出

**使用方法:**
```bash
go run compare_doc_structure.go \
  --docs=../../docs/user \
  --verbose \
  --output=structure_report.json
```

**オプション:**
- `--docs`: ドキュメントのルートディレクトリ（デフォルト: `docs/user`）
- `--verbose`: 詳細な出力
- `--output`: JSON形式のレポート出力先（オプション）

### 4. verify_links.go

ドキュメント内のリンク（内部リンクと外部リンク）を検証します。

**機能:**
- Markdown形式のリンクを抽出
- 内部リンク（ファイルパス）の存在確認
- 外部リンク（HTTP/HTTPS）のアクセス確認（オプション）

**使用方法:**
```bash
go run verify_links.go \
  --docs=../../docs \
  --verbose \
  --external \
  --output=links_report.json
```

**オプション:**
- `--docs`: ドキュメントのルートディレクトリ（デフォルト: `docs`）
- `--external`: 外部リンクも確認（時間がかかる可能性あり）
- `--verbose`: 詳細な出力
- `--timeout`: 外部リンク確認のタイムアウト秒数（デフォルト: 10）
- `--output`: JSON形式のレポート出力先（オプション）

### 5. run_all.sh

すべての検証スクリプトを一括実行するオーケストレーションスクリプト。

**機能:**
- すべての検証ツールをビルド
- 各検証を順次実行
- レポートを統一された場所に出力
- サマリーを表示

**使用方法:**
```bash
./run_all.sh [OPTIONS]
```

**オプション:**
- `-v, --verbose`: 詳細な出力
- `-e, --external`: 外部リンクもチェック（時間がかかる）
- `-n, --no-json`: JSONレポートを生成しない
- `-o, --output DIR`: 出力ディレクトリを指定（デフォルト: `build/verification-reports`）
- `-h, --help`: ヘルプメッセージを表示

**例:**
```bash
# デフォルト設定で実行
./run_all.sh

# 詳細出力と外部リンクチェック
./run_all.sh -v -e

# カスタム出力ディレクトリ
./run_all.sh -o /tmp/my-reports
```

## 出力形式

各スクリプトは以下の形式でレポートを出力します：

### テキストレポート

標準出力に人間が読みやすい形式でレポートを表示します。

例:
```
=== TOML Configuration Key Verification Report ===

Total keys in code: 45
Total keys in docs: 42
Keys in both: 40

⚠️  Keys in CODE but NOT in DOCS (3):
  - new_setting (struct: Config, file: config.go:123)
  ...

⚠️  Keys in DOCS but NOT in CODE (2):
  - deprecated_option
  ...
```

### JSONレポート

`--output` オプションでJSON形式のレポートを生成できます。これは後続の自動処理やCIパイプラインでの利用に適しています。

例:
```json
{
  "in_code_only": [
    {
      "key": "new_setting",
      "type": "string",
      "struct_tag": "toml:\"new_setting\"",
      "source_file": "internal/config/config.go",
      "line_number": 123,
      "parent_struct": "Config"
    }
  ],
  "in_docs_only": ["deprecated_option"],
  "in_both": [...],
  "code_key_count": 45,
  "doc_key_count": 42
}
```

## Makefile統合

プロジェクトのMakefileに以下のターゲットを追加することで、簡単に検証を実行できます：

```makefile
# ドキュメント検証
.PHONY: verify-docs
verify-docs:
	@./scripts/verification/run_all.sh

# 詳細な検証（外部リンクも含む）
.PHONY: verify-docs-full
verify-docs-full:
	@./scripts/verification/run_all.sh -v -e
```

使用方法:
```bash
make verify-docs      # 基本的な検証
make verify-docs-full # 完全な検証（外部リンク含む）
```

## CI/CD統合

これらのスクリプトはCI/CDパイプラインに統合できます：

```yaml
# GitHub Actions の例
- name: Verify Documentation
  run: |
    cd scripts/verification
    ./run_all.sh -e

- name: Upload Reports
  if: failure()
  uses: actions/upload-artifact@v3
  with:
    name: verification-reports
    path: build/verification-reports/
```

## 定期実行の推奨

ドキュメントと実装の整合性を保つため、以下のタイミングでの実行を推奨します：

1. **Pull Request時**: 新しいコード変更がドキュメントと整合しているか確認
2. **週次**: 定期的なチェックで問題の早期発見
3. **リリース前**: リリース前の最終確認

## トラブルシューティング

### 誤検出が多い場合

- `verify_toml_keys.go` や `verify_cli_args.go` の `isValidTOMLKey()` や `isValidArgName()` 関数で除外リストを調整
- 正規表現パターンを調整して精度を向上

### ビルドエラー

```bash
# 依存関係の確認
go mod tidy

# クリーンビルド
rm -rf build/verification-reports
./run_all.sh
```

### 外部リンクチェックが遅い

- `--timeout` オプションでタイムアウトを短縮
- 外部リンクチェックは必要な時のみ実行（`-e` オプションなし）

## 今後の改善案

- [ ] マルチスレッド対応による高速化
- [ ] より精密な構造解析（セクション順序の比較など）
- [ ] 差分ベースの検証（変更されたファイルのみ）
- [ ] HTMLレポート生成
- [ ] 自動修正機能（可能な範囲で）
- [ ] 翻訳用語集の自動整合性チェック

## 参考資料

- [execution_plan.md](../../docs/tasks/0064_ja_docs_implementation_verification/execution_plan.md) - 検証プロジェクトの全体計画
- [CLAUDE.md](../../CLAUDE.md) - プロジェクト全体のガイドライン
