# 要件定義書: 英語版ドキュメント作成

## 1. プロジェクト概要

### 1.1 プロジェクト名
英語版ドキュメント作成 (English Documentation Creation)

### 1.2 プロジェクト目的
リポジトリ内の各種ドキュメントが日本語・マークダウン形式で作成されている。
国際的な利用者に向けて、これらのドキュメントの英語版を作成する。

### 1.3 背景
現在、プロジェクトのドキュメントは主に日本語で記述されており、英語圏のユーザーや貢献者にとって参入障壁となっている。
英語版ドキュメントを整備することで、プロジェクトの国際的な認知度向上と貢献者の拡大を目指す。

## 2. 要件

### 2.1 翻訳対象ファイル

以下のファイルを翻訳対象とする:

1. **ルートディレクトリ**
   - `README.ja.md` → `README.md`

2. **開発者向けドキュメント** (`docs/dev/`)
   - `design-implementation-overview.ja.md` → `design-implementation-overview.md`
   - `hash-file-naming-adr.ja.md` → `hash-file-naming-adr.md`
   - `security-architecture.ja.md` → `security-architecture.md`
   - `terminal-capabilities.ja.md` → `terminal-capabilities.md`

3. **ユーザー向けドキュメント** (`docs/user/`)
   - `README.ja.md` → `README.md`
   - `record_command.ja.md` → `record_command.md`
   - `runner_command.ja.md` → `runner_command.md`
   - `verify_command.ja.md` → `verify_command.md`
   - `security-risk-assessment.ja.md` → `security-risk-assessment.md`

4. **TOML設定ガイド** (`docs/user/toml_config/`)
   - `README.ja.md` → `README.md`
   - `01_introduction.ja.md` → `01_introduction.md`
   - `02_hierarchy.ja.md` → `02_hierarchy.md`
   - `03_root_level.ja.md` → `03_root_level.md`
   - `04_global_level.ja.md` → `04_global_level.md`
   - `05_group_level.ja.md` → `05_group_level.md`
   - `06_command_level.ja.md` → `06_command_level.md`
   - `07_variable_expansion.ja.md` → `07_variable_expansion.md`
   - `08_practical_examples.ja.md` → `08_practical_examples.md`
   - `09_best_practices.ja.md` → `09_best_practices.md`
   - `10_troubleshooting.ja.md` → `10_troubleshooting.md`
   - `appendix.ja.md` → `appendix.md`

**合計: 21ファイル**

### 2.2 翻訳除外対象

以下のファイルは翻訳対象外とする:

- `docs/tasks/` 以下のすべてのドキュメント(開発タスク管理用)
- `.md` 拡張子を持たない既存の英語版ドキュメント
- `CLAUDE.md`、`LICENSE` などのプロジェクトメタファイル

### 2.3 ファイル命名規則

- 英語版ドキュメントは日本語版と同じディレクトリに配置する
- ファイル名は `.ja.md` を `.md` に置換する
- 例: `README.ja.md` → `README.md`

### 2.4 翻訳方針

#### 2.4.1 基本方針

- **忠実な翻訳**: 内容を意訳せず、原文に忠実な翻訳とする
- **訳語の統一**: 技術用語や専門用語の訳語を統一する
- **構造の保持**: 章立て、見出しレベル、リスト構造を完全に保持する
- **リンクの維持**: 内部リンク、外部リンクをすべて維持する
- **コードブロックの保持**: コード例、コマンド例は原文のまま保持する

#### 2.4.2 訳語管理

- 訳語を管理するための用語集ファイルを `docs/tasks/0027_docs_in_english/glossary.md` に作成する
- フォーマット: マークダウンテーブル形式
  ```markdown
  | 日本語 | English | 備考 |
  |--------|---------|------|
  | 用語1  | term1   | 説明 |
  ```
- 翻訳作業中に新しい技術用語が出現した場合、用語集に追加する

#### 2.4.3 品質基準

- 章立てが日本語版と完全に一致すること
- 内容の省略や追加がないこと
- マークダウン記法が正しく動作すること
- リンクが正しく機能すること

### 2.5 既存英語版ドキュメントの扱い

- 現在、一部の英語版ドキュメント(`.md`)が既に存在する
- これらは本タスクで作成する新しい英語版で上書きする
- 上書き前に、既存ドキュメントの内容を確認し、有用な情報があれば統合する

### 2.6 今後のメンテナンス方針

本タスクの範囲外だが、将来的には以下の運用を検討する:

- 日本語版ドキュメント更新時に英語版も同時更新する
- 日本語版と英語版の同期状態を管理する仕組みを導入する
- CI/CDで両言語のドキュメント整合性を検証する

## 3. 成功基準

- [ ] すべての日本語版ドキュメント(21ファイル)に対応する英語版ドキュメントが存在する
- [ ] 日本語版ドキュメントと英語版ドキュメントは章立てが完全に一致し、内容も省略や齟齬がない
- [ ] 訳語管理ファイル(`glossary.md`)が作成され、主要な技術用語が登録されている
- [ ] すべての内部リンクが正しく機能する
- [ ] マークダウン記法が正しくレンダリングされる
- [ ] コードブロック、コマンド例が正しく保持されている

## 4. 制約事項

- 機械翻訳の使用は可とするが、必ず人間によるレビューと修正を行う
- 翻訳の正確性よりも、原文への忠実性を優先する
- 文化的な表現の違いは最小限に留め、技術的な正確性を重視する

## 5. 想定される課題とリスク

| 課題/リスク | 対策 |
|------------|------|
| 技術用語の訳語が不統一になる | 用語集を作成し、翻訳前に主要な用語を定義する |
| 既存の英語版ドキュメントとの整合性 | 上書き前に既存内容を確認し、有用な情報を統合する |
| 日本語特有の表現の翻訳が困難 | 技術的な意味を損なわない範囲で、英語として自然な表現に調整する |
| 翻訳量が多く、時間がかかる | ファイル単位で段階的に翻訳を進める |
