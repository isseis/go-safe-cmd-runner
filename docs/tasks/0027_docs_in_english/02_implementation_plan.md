# 実装計画書: 英語版ドキュメント作成

## 1. 概要

本文書は、英語版ドキュメント作成タスクの具体的な実装手順を定義します。

## 2. 前提条件

- [ ] 要件定義書([01_requirements.md](01_requirements.md))が承認されている
- [ ] 用語集([glossary.md](glossary.md))が作成されている
- [ ] 翻訳作業用のブランチが作成されている

## 3. 実装フェーズ

### フェーズ0: 準備作業

#### 0.1 既存英語版ドキュメントの確認と退避
- [x] 既存の `.md` ファイルをリストアップ
- [x] 有用な情報がある場合は別途保存
- [x] git status で削除予定のファイルを確認

#### 0.2 翻訳環境の準備
- [x] 用語集を確認し、主要な技術用語を把握
- [x] 翻訳ツール(機械翻訳+人間レビュー)の準備

### フェーズ1: ルートディレクトリ (1ファイル)

#### 1.1 README.md の翻訳
- [x] `README.ja.md` を読み込み、構造を確認
- [x] 章立てとセクション構成を把握
- [x] 機械翻訳による初期翻訳
- [x] 用語集に基づいて訳語を統一
- [x] リンク(`*.ja.md` → `*.md`)を修正
- [x] 人間レビューによる品質確認
- [x] `README.md` として保存
- [x] マークダウンのレンダリング確認

### フェーズ2: 開発者向けドキュメント (4ファイル)

#### 2.1 design-implementation-overview.md の翻訳
- [x] `design-implementation-overview.ja.md` の翻訳
- [x] 用語集の更新(新しい技術用語があれば)
- [x] リンクの修正
- [x] レビューと保存

#### 2.2 hash-file-naming-adr.md の翻訳
- [x] `hash-file-naming-adr.ja.md` の翻訳
- [x] ADR(Architecture Decision Record)特有の用語を確認
- [x] 用語集の更新
- [x] リンクの修正
- [x] レビューと保存

#### 2.3 security-architecture.md の翻訳
- [x] `security-architecture.ja.md` の翻訳
- [x] セキュリティ用語の正確性を重点的に確認
- [x] 用語集の更新
- [x] リンクの修正
- [x] レビューと保存

#### 2.4 terminal-capabilities.md の翻訳
- [x] `terminal-capabilities.ja.md` の翻訳
- [x] 用語集の更新
- [x] リンクの修正
- [x] レビューと保存

### フェーズ3: ユーザー向けドキュメント (5ファイル)

#### 3.1 docs/user/README.md の翻訳
- [x] `docs/user/README.ja.md` の翻訳
- [x] ユーザーガイド全体の目次として機能することを確認
- [x] すべての内部リンクを確認
- [x] 用語集の更新
- [x] レビューと保存

#### 3.2 record_command.md の翻訳
- [x] `record_command.ja.md` の翻訳
- [x] コマンドラインオプションの説明を正確に翻訳
- [x] コマンド例はそのまま保持
- [x] 用語集の更新
- [x] リンクの修正
- [x] レビューと保存

#### 3.3 runner_command.md の翻訳
- [x] `runner_command.ja.md` の翻訳
- [x] コマンドラインオプションの説明を正確に翻訳
- [x] コマンド例はそのまま保持
- [x] 用語集の更新
- [x] リンクの修正
- [x] レビューと保存

#### 3.4 verify_command.md の翻訳
- [x] `verify_command.ja.md` の翻訳
- [x] コマンドラインオプションの説明を正確に翻訳
- [x] コマンド例はそのまま保持
- [x] 用語集の更新
- [x] リンクの修正
- [x] レビューと保存

#### 3.5 security-risk-assessment.md の翻訳
- [x] `security-risk-assessment.ja.md` の翻訳
- [x] セキュリティリスク評価の用語を正確に翻訳
- [x] 用語集の更新
- [x] リンクの修正
- [x] レビューと保存

### フェーズ4: TOML設定ガイド (12ファイル)

#### 4.1 docs/user/toml_config/README.md の翻訳
- [x] `docs/user/toml_config/README.ja.md` の翻訳
- [x] TOML設定ガイド全体の目次として機能することを確認
- [x] すべての内部リンク(`01_*.md` 〜 `10_*.md`)を確認
- [x] 用語集の更新
- [x] レビューと保存

#### 4.2 01_introduction.md の翻訳
- [x] `01_introduction.ja.md` の翻訳
- [x] 導入セクションとして適切な表現を使用
- [x] 用語集の更新
- [x] リンクの修正
- [x] レビューと保存

#### 4.3 02_hierarchy.md の翻訳
- [x] `02_hierarchy.ja.md` の翻訳
- [x] 階層構造の説明を正確に翻訳
- [x] 図表がある場合は保持
- [x] 用語集の更新
- [x] リンクの修正
- [x] レビューと保存

#### 4.4 03_root_level.md の翻訳
- [x] `03_root_level.ja.md` の翻訳
- [x] TOML設定例はそのまま保持
- [x] 用語集の更新
- [x] リンクの修正
- [x] レビューと保存

#### 4.5 04_global_level.md の翻訳
- [x] `04_global_level.ja.md` の翻訳
- [x] TOML設定例はそのまま保持
- [x] 用語集の更新
- [x] リンクの修正
- [x] レビューと保存

#### 4.6 05_group_level.md の翻訳
- [x] `05_group_level.ja.md` の翻訳
- [x] TOML設定例はそのまま保持
- [x] 用語集の更新
- [x] リンクの修正
- [x] レビューと保存

#### 4.7 06_command_level.md の翻訳
- [x] `06_command_level.ja.md` の翻訳
- [x] TOML設定例はそのまま保持
- [x] 用語集の更新
- [x] リンクの修正
- [x] レビューと保存

#### 4.8 07_variable_expansion.md の翻訳
- [x] `07_variable_expansion.ja.md` の翻訳
- [x] 変数展開の説明を正確に翻訳
- [x] TOML設定例はそのまま保持
- [x] 用語集の更新
- [x] リンクの修正
- [x] レビューと保存

#### 4.9 08_practical_examples.md の翻訳
- [x] `08_practical_examples.ja.md` の翻訳
- [x] 実践例として適切な表現を使用
- [x] TOML設定例はそのまま保持
- [x] 用語集の更新
- [x] リンクの修正
- [x] レビューと保存

#### 4.10 09_best_practices.md の翻訳
- [x] `09_best_practices.ja.md` の翻訳
- [x] ベストプラクティスとして適切な表現を使用
- [x] 用語集の更新
- [x] リンクの修正
- [x] レビューと保存

#### 4.11 10_troubleshooting.md の翻訳
- [x] `10_troubleshooting.ja.md` の翻訳
- [x] トラブルシューティングとして適切な表現を使用
- [x] エラーメッセージはそのまま保持
- [x] 用語集の更新
- [x] リンクの修正
- [x] レビューと保存

#### 4.12 appendix.md の翻訳
- [x] `appendix.ja.md` の翻訳
- [x] 付録として適切な表現を使用
- [x] 用語集の更新
- [x] リンクの修正
- [x] レビューと保存

### フェーズ5: 品質保証

#### 5.1 全体レビュー
- [x] すべての英語版ドキュメント(21ファイル)が存在することを確認
- [x] 各ドキュメントの章立てが日本語版と一致することを確認
- [x] 訳語の統一性を確認(用語集と照合)
- [x] 内部リンクがすべて機能することを確認
- [x] 外部リンクがすべて機能することを確認

#### 5.2 マークダウン検証
- [x] すべてのマークダウンファイルが正しくレンダリングされることを確認
- [x] コードブロックの構文ハイライトが機能することを確認
- [x] テーブルが正しく表示されることを確認
- [x] リストの階層構造が保持されることを確認

#### 5.3 コンテンツ検証
- [x] 各セクションの内容が日本語版と一致することを確認
- [x] 省略や追加がないことを確認
- [x] コードブロック、コマンド例が正しく保持されていることを確認
- [x] ファイルパス、変数名、関数名が変更されていないことを確認

#### 5.4 用語集の最終確認
- [x] 用語集に登録されているすべての用語が正しく使用されていることを確認
- [x] 新しい技術用語が用語集に追加されていることを確認
- [x] 訳語の統一が保たれていることを確認

### フェーズ6: コミットとプルリクエスト

#### 6.1 変更のステージング
- [ ] `git status` で変更内容を確認
- [ ] 21個の新しい `.md` ファイルが追加されていることを確認
- [ ] 既存の `.md` ファイルが削除されていることを確認(git status に表示)
- [ ] `git add` で変更をステージング

#### 6.2 コミット
- [ ] 適切なコミットメッセージでコミット
  ```
  docs: Add English translations for user and developer documentation

  - Translate 21 documentation files from Japanese to English
  - Add glossary for translation consistency
  - Update all internal links to reference .md files

  Files translated:
  - Root: README.md
  - Dev docs: design-implementation-overview.md, hash-file-naming-adr.md,
    security-architecture.md, terminal-capabilities.md
  - User docs: README.md, record_command.md, runner_command.md,
    verify_command.md, security-risk-assessment.md
  - TOML config guide: README.md, 01-10 chapters, appendix.md
  ```

#### 6.3 プルリクエスト作成
- [ ] ブランチを GitHub にプッシュ
- [ ] プルリクエストを作成
- [ ] プルリクエストの説明を記載
  - 翻訳対象ファイル一覧
  - 翻訳方針
  - レビューポイント
- [ ] レビュワーをアサイン

## 4. 作業見積もり

| フェーズ | ファイル数 | 見積もり時間 | 備考 |
|---------|-----------|-------------|------|
| 0. 準備作業 | - | 0.5時間 | 既存ファイル確認、環境準備 |
| 1. ルート | 1 | 1.0時間 | README は比較的大きいファイル |
| 2. 開発者向け | 4 | 4.0時間 | 技術的な内容、用語の正確性が重要 |
| 3. ユーザー向け | 5 | 5.0時間 | コマンドラインオプションの説明が多い |
| 4. TOML設定 | 12 | 8.0時間 | 設定ガイドは比較的定型的 |
| 5. 品質保証 | - | 3.0時間 | 全体レビュー、リンク確認 |
| 6. PR作成 | - | 0.5時間 | コミット、プルリクエスト |
| **合計** | **21** | **22.0時間** | 約3日間(1日8時間作業) |

## 5. リスクと対策

| リスク | 影響度 | 対策 |
|--------|-------|------|
| 機械翻訳の品質が低い | 高 | 人間レビューを必ず実施、用語集で訳語を統一 |
| 内部リンクの修正漏れ | 中 | 品質保証フェーズで全リンクを確認 |
| 訳語の不統一 | 中 | 用語集を随時更新、最終レビューで確認 |
| 作業時間の超過 | 中 | ファイル単位で段階的に進める、優先度の高いファイルから着手 |
| マークダウン記法の崩れ | 低 | 各ファイル翻訳後にレンダリング確認 |

## 6. 優先順位

以下の順序で翻訳を進めることを推奨します:

1. **最優先**: `README.md` (プロジェクトの顔)
2. **高優先**: ユーザー向けドキュメント (ユーザーが最初に読む)
   - `docs/user/README.md`
   - `docs/user/runner_command.md`
   - `docs/user/record_command.md`
   - `docs/user/verify_command.md`
3. **中優先**: TOML設定ガイド (ユーザーが設定を理解するために必要)
4. **通常**: 開発者向けドキュメント (貢献者向け)

## 7. 完了条件

- [ ] すべてのチェックボックス(上記すべてのフェーズ)がチェックされている
- [ ] 成功基準([01_requirements.md](01_requirements.md) セクション3)がすべて満たされている
- [ ] プルリクエストが作成され、レビュー待ち状態になっている

## 8. 関連ドキュメント

- [要件定義書](01_requirements.md)
- [用語集](glossary.md)
- [CLAUDE.md](../../CLAUDE.md) - プロジェクト全体のドキュメントガイドライン
