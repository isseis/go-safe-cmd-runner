# セキュリティクリティカル部 code smell 監査 実行計画（fable 使用）

## 0. 目的

Claude モデル **fable**（`model: "fable"`）をレビュアーとして使い、本システムのセキュリティクリティカルなパッケージを
1つずつ細かい単位で監査し、code smell / セキュリティリスクの所見を洗い出す。

- フルスペックの `01_requirements.md` / `02_architecture.md` は作成しない（本ドキュメントのみで進める）。
- 各コンポーネントを独立したタスク単位に分割し、チェックボックスで進捗を追跡する。
- 各タスクの成果物は `findings/<component>.md` に保存する（fable の生出力をほぼそのまま保存）。
- 全コンポーネント完了後、`99_summary.md` に重大度付きの一覧表として集約する。

過去の類似監査（`docs/tasks/0088_security_audit_findings/`）を参考にした所見フォーマット・重大度区分を踏襲する。

## 1. 前提条件・進め方

- **読み取り専用の静的監査**。コード修正はこのタスクの範囲外（修正は所見ごとに別 PR/別タスクで行う）。
- 1 コンポーネント = 1 チェック単位。逐次実行し、都度チェックボックスを更新する。
- 各コンポーネントのレビューは以下のいずれかで実行する:
  - `Agent` ツールを `model: "fable"`, `subagent_type: "general-purpose"` で呼び出す（推奨、下記テンプレート参照）
  - fable が直接利用できない環境では同等のプロンプトを人手で fable に投げてもよい
- fable への指示（プロンプト）は §3 のテンプレートを毎回使い回す。コンポーネントごとに対象パスと出力先だけ差し替える。
- 1コンポーネントのレビューが完了したら、出力ファイルを軽く確認し（明らかな誤り・幻覚がないか）、問題なければチェックを入れて次へ進む。
- 途中で会話が途切れても、本ドキュメントのチェック状態と `findings/` 配下のファイルだけで再開できるようにする。

## 2. 重大度区分（`docs/tasks/0088_security_audit_findings/` を踏襲）

| 記号 | 重大度 | 目安 |
|---|---|---|
| 🔴 | High | 権限昇格・認証バイパス・任意コード実行などに直結しうる |
| 🟡 | Medium | 悪用条件が限定的、または多層防御の一部が欠ける |
| 🟠 | Low | 直接の悪用は困難だが堅牢性・保守性上の懸念 |
| 🔵 | Info | 監視事項・将来の変更で注意すべき点（アクション不要な場合を含む） |

## 3. fable 呼び出しテンプレート

各コンポーネントのチェックボックス作業は、以下のプロンプトの `{{PACKAGE_PATHS}}` と `{{OUTPUT_FILE}}` を差し替えて `Agent` ツール
（`model: "fable"`, `subagent_type: "general-purpose"`, `run_in_background` は状況に応じて）に渡す。

```
このリポジトリ（go-safe-cmd-runner、Go製のセキュアコマンドランナー）の以下のパッケージについて、
セキュリティクリティカルな観点で code smell / セキュリティリスクの監査を行ってください。

対象パッケージ:
{{PACKAGE_PATHS}}

観点の例（網羅ではなく例示）:
- 権限管理・特権分離の不備（権限昇格/降格の失敗時の後始末漏れ等）
- パス検証・symlink attack・TOCTOU
- コマンドインジェクション・環境変数経由の攻撃面
- ハッシュ検証・改ざん検出のロジック不備
- allowlist/denylist の抜け漏れ、境界条件
- エラーハンドリングの握りつぶし、fail-open になっている箇所
- 機密情報のログ出力・redaction 漏れ
- 並行処理・競合状態
- 一般的な code smell（重複、過度な複雑さ、命名と実装の乖離、テスト不足な分岐 等）

作業方法:
- 対象パッケージのソースコード（*_test.go 以外を中心に、必要に応じてテストも参照）を読み込んで静的に分析してください。
- コードの修正は行わないでください（読み取り専用の監査です）。
- 所見ごとに 重大度（🔴High/🟡Medium/🟠Low/🔵Info）、該当箇所（file:line）、問題の説明、再現/悪用シナリオ、推奨対応 を記載してください。
- 既に実装されている良い防御機構があれば、所見とは別に「観察された良好な防御層」として記録してください。
- 出力は日本語の Markdown とし、以下のファイルに書き込んでください: {{OUTPUT_FILE}}
- 所見が無い場合も「所見なし」と明記してファイルを作成してください。
```

## 4. コンポーネント別チェックリスト

> 各行が 1 タスク単位。`[ ]` → 実施 → `findings/<file>` 確認 → `[x]`。

### Phase A: 権限・実行コア

- [x] A1. `internal/runner/base/privilege/` — 権限昇格・降格管理
      → `findings/A1_privilege.md`（🟡2/🟠5/🔵4）
- [x] A2. `internal/runner/base/executor/` — コマンド実行・環境変数構築・パス検証
      → `findings/A2_executor.md`（🟡3/🟠3/🔵6）
- [x] A3. `internal/runner/base/environment/` — 環境変数処理・フィルタリング
      → `findings/A3_environment.md`（🟡2/🟠2/🔵3）
- [x] A4. `internal/runner/base/security/` — コマンド/環境変数 allowlist 検証フレームワーク
      → `findings/A4_security.md`（🟡2/🟠4/🔵3）
- [x] A5. `internal/runner/base/risk/` — リスクベースのコマンド評価
      → `findings/A5_risk.md`（🟡1/🟠4/🔵3）
- [x] A6. `internal/runner/base/output/` — 出力先パス検証
      → `findings/A6_output.md`（🟡2/🟠3/🔵3）
- [x] A7. `internal/runner/base/audit/` — セキュリティ監査ログ
      → `findings/A7_audit.md`（🟡3/🟠2/🔵5）

### Phase B: ファイル整合性・検証

- [x] B1. `internal/safefileio/` — symlink-safe ファイル I/O（openat2 + フォールバック）
      → `findings/B1_safefileio.md`（🟡2/🟠3/🔵4）
- [x] B2. `internal/filevalidator/`, `internal/filevalidator/pathencoding/` — ハッシュベース整合性検証
      → `findings/B2_filevalidator.md`（🟡2/🟠6/🔵5）
- [x] B3. `internal/verification/` — config/binary 統合検証マネージャ
      → `findings/B3_verification.md`（🟡2/🟠4/🔵4）
- [x] B4. `internal/runner/config/` — TOML 設定ロード・変数展開・テンプレート include
      → `findings/B4_config.md`（🟡2/🟠4/🔵3）

### Phase C: バイナリ・依存解析

- [x] C1. `internal/security/` 配下（`binaryanalyzer/`, `elfanalyzer/`, `machoanalyzer/`）, `internal/arm64util/`
      → `findings/C1_binary_analysis.md`（🟡2/🟠4/🔵5）
- [x] C2. `internal/dynlib/` 配下（`elfdynlib/`, `machodylib/`）, `internal/libccache/`
      → `findings/C2_dynlib.md`（🟡3/🟠3/🔵5）
- [x] C3. `internal/shebang/`, `internal/fileanalysis/`
      → `findings/C3_shebang_fileanalysis.md`（🟡3/🟠5/🔵3）

### Phase D: 周辺セキュリティ機能

- [x] D1. `internal/groupmembership/` — CGO 経由 `getgrgid_r` 呼び出し
      → `findings/D1_groupmembership.md`（🔴1/🟡4/🟠4/🔵2）
- [x] D2. `internal/logging/`, `internal/redaction/` — 構造化ログ、Slack webhook、機密情報 redaction
      → `findings/D2_logging_redaction.md`（🔴1/🟡5/🟠6/🔵6）

### Phase E: エントリポイント・起動シーケンス

- [x] E1. `cmd/runner/`, `cmd/record/`, `cmd/verify/`, `internal/runner/bootstrap/`, `internal/runner/cli/`
      → `findings/E1_entrypoints.md`（🟡3/🟠7/🔵6）

## 5. 集約フェーズ

- [x] 全 Phase A〜E のチェックが完了していることを確認
- [x] `findings/*.md` を通読し、所見一覧表（ID・重大度・領域・概要・ファイルへのリンク）を作成
- [x] コンポーネント横断で繰り返し出現するパターン（例: 同種のエラー握りつぶしが複数箇所にある等）があれば別枠でまとめる
- [x] 「観察された良好な防御層」を集約・重複排除して記録
- [x] 対応優先度（推奨）を重大度と修正コストから暫定順位付け
- [x] `99_summary.md` として保存

## 6. 進捗ログ（追記用）

- 実施日:
- 実施者:
- 完了 Phase/コンポーネント:
- 所見件数（重大度別）:
- 課題/ブロッカー:
- 次アクション:
