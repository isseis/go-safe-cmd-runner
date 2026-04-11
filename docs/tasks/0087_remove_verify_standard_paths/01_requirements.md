# verify_standard_paths 機能の完全削除 要件定義書

## 1. 概要

### 1.1 背景

過去の実装では、標準システムディレクトリ（`/bin/`、`/sbin/`、`/usr/bin/`、`/usr/sbin/`）に存在するコマンドについて、TOML 設定の `verify_standard_paths = false` によりハッシュ検証をスキップできる機能が存在した。

現在のセキュリティポリシーでは、**すべての実行ファイルにハッシュ検証が必要**となっており、`verify_standard_paths = false` の設定は許容されない。`verify_standard_paths` のデフォルト値は `true`（検証する）である一方、現状コードでは設定値に応じて `runtimeGlobal.SkipStandardPaths()` の結果が `PathResolver.skipStandardPaths` に反映されるため、常に `skipStandardPaths=false` が渡されるわけではない。本タスクでは、そのような設定依存の分岐自体を削除し、ハッシュ検証が常に全ファイルに対して行われる状態に統一する。

### 1.2 目的

`verify_standard_paths` 設定およびそれに関連するすべてのコード・テスト・ドキュメントを完全に削除し、ハッシュ検証が常に全ファイルに対して行われることを明確にする。

### 1.3 スコープ外

- ハッシュ検証そのものの動作は変更しない（常にすべてのファイルを検証する）
- 過去タスクのドキュメント（`docs/tasks/` 配下）は歴史的記録として保持する

## 2. 用語定義

| 用語 | 定義 |
|------|------|
| `verify_standard_paths` | TOML 設定フィールド。`false` にすると標準ディレクトリの実行ファイルのハッシュ検証をスキップできた（廃止対象） |
| 標準ディレクトリ | `/bin/`、`/sbin/`、`/usr/bin/`、`/usr/sbin/` |
| `SkipBinaryAnalysis` | `verify_standard_paths=false` かつ標準ディレクトリのコマンドに対して、バイナリ解析をスキップするフラグ（廃止対象） |

## 3. 機能要件

削除対象を層ごとに整理する。

### 3.1 TOML 設定・ランタイム型の削除

#### FR-3.1.1: `GlobalSpec.VerifyStandardPaths` フィールドの削除

- ファイル: `internal/runner/runnertypes/spec.go`
- 削除対象: ``VerifyStandardPaths *bool `toml:"verify_standard_paths"` `` フィールド

#### FR-3.1.2: デフォルト値定義の削除

- ファイル: `internal/runner/config/defaults.go`
- 削除対象:
  - `DefaultVerifyStandardPaths = true` 定数
  - `ApplyGlobalDefaults` 内の `VerifyStandardPaths` 設定ブロック

#### FR-3.1.3: ランタイム変換関数・メソッドの削除

- ファイル: `internal/runner/runnertypes/runtime.go`
- 削除対象:
  - `DetermineVerifyStandardPaths()` 関数
  - `RuntimeGlobal.SkipStandardPaths()` メソッド（`VerifyStandardPaths` の反転値を返す）

#### FR-3.1.4: `RuntimeCommand.SkipBinaryAnalysis` フィールドの削除

- ファイル: `internal/runner/runnertypes/runtime.go`
- 削除対象: `RuntimeCommand` 構造体の `SkipBinaryAnalysis bool` フィールドおよびそのコメント

### 3.2 PathResolver の削除

#### FR-3.2.1: `PathResolver` の関連フィールドとメソッドの削除

- ファイル: `internal/verification/path_resolver.go`
- 削除対象:
  - `skipStandardPaths bool` フィールド
  - `standardPaths []string` フィールド
  - `NewPathResolver` の `skipStandardPaths bool` パラメータ
  - コンストラクタ内の `skipStandardPaths` と `standardPaths` の初期化
  - `ShouldSkipVerification()` メソッド

### 3.3 検証マネージャの削除

#### FR-3.3.1: `Manager` 内のスキップロジックの削除

- ファイル: `internal/verification/manager.go`
- 削除対象:
  - `m.pathResolver.skipStandardPaths = runtimeGlobal.SkipStandardPaths()` の設定箇所
  - `VerifyGlobalFiles` 内の `ShouldSkipVerification` チェックとスキップ処理（`result.SkippedFiles` への追加、ログ出力、`continue` 文）
  - `VerifyGroupFiles` 内の同様のスキップ処理

### 3.4 Group Executor の削除

#### FR-3.4.1: スキップパスのビルドと `SkipBinaryAnalysis` 設定の削除

- ファイル: `internal/runner/group_executor.go`
- 削除対象:
  - `skippedPaths` マップの構築ループ
  - `cmd.SkipBinaryAnalysis = true` の設定箇所

### 3.5 リスク評価の削除

#### FR-3.5.1: `SkipBinaryAnalysis` 参照の削除と `IsNetworkOperation` 呼び出し変更

- ファイル: `internal/runner/risk/evaluator.go`
- 削除対象: `cmd.SkipBinaryAnalysis` を参照するコメントおよび条件分岐
- 変更内容: `IsNetworkOperation` の呼び出しから `skipBinaryAnalysis` 引数を削除（FR-3.6.5 に対応）

### 3.6 セキュリティ分析の削除

#### FR-3.6.1: `AnalysisOptions.VerifyStandardPaths` の削除

- ファイル: `internal/runner/security/command_analysis.go`
- 削除対象: `AnalysisOptions` 構造体の `VerifyStandardPaths bool` フィールドおよびそのコメント

#### FR-3.6.2: `shouldPerformHashValidation()`・`isStandardDirectory()`・`StandardDirectories` の削除

- ファイル: `internal/runner/security/hash_validation.go`
- 削除対象: `shouldPerformHashValidation()` 関数（`VerifyStandardPaths` の値を分岐する処理）
- 変更内容: `AnalyzeCommandSecurity` 内の `shouldPerformHashValidation()` 呼び出しと条件分岐を削除し、`validateFileHash()` を常に呼び出すように変更

- ファイル: `internal/runner/security/directory_risk.go`
- 削除対象:
  - `StandardDirectories` 変数（`shouldPerformHashValidation` から参照されていたスライス）
  - `isStandardDirectory()` 関数（`shouldPerformHashValidation` からのみ呼び出されており、削除後はデッドコードとなる）
- 注意: `getDefaultRiskByDirectory()` は `DefaultRiskLevels` マップを直接使用しており `isStandardDirectory()` に依存していないため、リスク評価ロジックへの影響はない

#### FR-3.6.3: Dry-run マネージャの削除

- ファイル: `internal/runner/resource/dryrun_manager.go`
- 削除対象: `AnalysisOptions` への `VerifyStandardPaths` フィールドの設定

#### FR-3.6.4: リソース型の削除

- ファイル: `internal/runner/resource/types.go`
- 削除対象: `VerifyStandardPaths bool` フィールド（`AnalysisOptions` 相当の型）

#### FR-3.6.5: `NetworkAnalyzer.IsNetworkOperation` の `skipBinaryAnalysis` パラメータ削除

- ファイル: `internal/runner/security/network_analyzer.go`
- 削除対象:
  - `IsNetworkOperation` の `skipBinaryAnalysis bool` パラメータ
  - `skipBinaryAnalysis` に関するコメント（55–60行目）
  - 関数本体内の `!skipBinaryAnalysis` 条件（111行目）
- 変更内容: 削除後はバイナリ解析を常に実行する（`skipBinaryAnalysis=false` と同等の動作に固定）

### 3.7 `SkippedFiles` の削除

`SkippedFiles` はスタンダードパスのスキップ専用であり、削除後は常に 0 となる。ユーザー向け出力（dry-run サマリー等）の変更を伴うが、クリーンな削除のため合わせて削除する。

#### FR-3.7.1: 内部結果型の `SkippedFiles` 削除

- ファイル: `internal/verification/errors.go`
- 削除対象: `GlobalVerificationError.SkippedFiles int` フィールドおよび `Result.SkippedFiles []string` フィールド

#### FR-3.7.2: 公開型の `SkippedFiles` 削除

- ファイル: `internal/verification/types.go`
- 削除対象: `FileVerificationSummary.SkippedFiles int` フィールド

#### FR-3.7.3: 結果収集の `SkippedFiles` 削除

- ファイル: `internal/verification/result_collector.go`
- 削除対象: `skippedFiles int` フィールド、`RecordSkip()` メソッド、`GetSummary()` 内の `SkippedFiles` 設定

#### FR-3.7.4: フォーマッタの `SkippedFiles` 削除

- ファイル: `internal/runner/resource/formatter.go`
- 削除対象: `"  Skipped: %d\n"` の出力行

#### FR-3.7.5: ログフォーマッタの `skipped_files` キー削除

- ファイル: `internal/logging/message_formatter.go`
- 削除対象: `shouldSkipInteractiveAttr()` 関数内のローカル変数 `skipKeys` スライスリテラルから `"skipped_files"` エントリを削除

### 3.8 ドキュメントの更新

#### FR-3.8.1: TOML 設定リファレンスの更新

- ファイル: `docs/user/toml_config/04_global_level.md`（および対応する日本語版）
- 内容: `verify_standard_paths` フィールドの説明を削除

#### FR-3.8.2: その他ユーザードキュメントの更新

- 対象: `docs/user/` 配下で `verify_standard_paths`/`skip_standard_paths` を参照しているドキュメント
  - `docs/user/runner_command.md`（および日本語版）
  - `docs/user/toml_config/09_practical_examples.md`（および日本語版）
  - `docs/user/toml_config/10_best_practices.md`（および日本語版）
  - `docs/user/toml_config/11_troubleshooting.md`（および日本語版）
  - `docs/user/toml_config/appendix.md`（および日本語版）
- 内容: 当該フィールドへの言及を削除

#### FR-3.8.5: 開発者ドキュメントの更新

- 対象:
  - `docs/dev/security-architecture.md`（および日本語版）: `PathResolver` 構造体の `skipStandardPaths` フィールドを示すコードスニペット（277行目）を削除
  - `docs/dev/config-inheritance-behavior.md`（および日本語版）: `skip_standard_paths` の行（29行目）を削除
  - `docs/translation_glossary.md`: `標準パス検証 / verify_standard_paths` の行（559行目）を削除

#### FR-3.8.3: dry-run JSON スキーマドキュメントの更新

- ファイル: `docs/user/dry_run_json_schema.md`（および日本語版）
- 削除対象: `skipped_files` フィールドの行（`SkippedFiles` 削除に伴う）

#### FR-3.8.4: CHANGELOG への変更エントリ追加

- ファイル: `CHANGELOG.md`
- 内容: `verify_standard_paths` 機能の完全削除を Breaking Changes として追記
  - 削除されたフィールド・メソッド・パラメータの一覧
  - `skipped_files` 出力の削除
  - `verify_standard_paths` を TOML に記述するとパースエラーになる旨
- 注意: 過去エントリ（`verify_standard_paths` に関する既存の記述）は歴史的記録として変更しない

### 3.9 サンプルファイルの更新

#### FR-3.9.1: サンプル TOML ファイルの `verify_standard_paths` 削除

- 対象: `sample/` 配下のすべての TOML ファイル（14ファイル）
  - `sample/starter.toml`
  - `sample/comprehensive.toml`
  - `sample/variable_expansion_basic.toml`
  - `sample/variable_expansion_advanced.toml`
  - `sample/variable_expansion_security.toml`
  - `sample/variable_expansion_test.toml`
  - `sample/auto_env_test.toml`
  - `sample/auto_env_group.toml`
  - `sample/slack-notify.toml`
  - `sample/slack-group-notification-test.toml`
  - `sample/risk-based-control.toml`
  - `sample/output_capture_basic.toml`
  - `sample/output_capture_too_large_error.toml`
  - `sample/workdir_examples.toml`
- 削除対象: `verify_standard_paths = false` の行
- 理由: コードから当該フィールドが削除されるため、サンプルに残すと誤解を招く。また go-toml の strict モードではパースエラーになる可能性がある。

### 3.10 エントリポイントとランナーの変更

#### FR-3.10.1: `cmd/runner/main.go` の変更

- ファイル: `cmd/runner/main.go`
- 削除対象:
  - `SkippedFiles` を参照するログ出力（274行目: `"skipped", len(result.SkippedFiles)`）
  - `DryRunOptions.VerifyStandardPaths` フィールドの設定（322行目: `VerifyStandardPaths: runnertypes.DetermineVerifyStandardPaths(...)`）

#### FR-3.10.2: `internal/runner/runner.go` の変更

- ファイル: `internal/runner/runner.go`
- 削除対象: `verErr.SkippedFiles` を参照するエラーメッセージ内の `Skipped` フィールド（401行目）

## 4. 非機能要件

### 4.1 動作の変更

以下は、`verify_standard_paths` キーを TOML から削除した後、または未指定だった場合の**実行時の検証動作**について述べる。

- `verify_standard_paths = false` を TOML に設定していたユーザーは、キー削除後、すべての標準ディレクトリのコマンドもハッシュ検証対象となる（セキュリティ強化方向の変更）
- `verify_standard_paths = true` を TOML に設定していたユーザーは、キー削除後の検証動作自体は変更なし（もともとデフォルトと同じく、標準ディレクトリのコマンドも検証対象）

### 4.2 TOML 互換性

`verify_standard_paths` フィールドが TOML に記述されている場合は、値が `true` / `false` のいずれであっても **パースエラー** とする（unknown field として明示的に失敗させる）。これは既知の破壊的変更として受け入れる。

したがって、既存ユーザーへの影響は以下の通りである。

- `verify_standard_paths = true` を設定していたユーザーは、キーが残っている間は設定読み込みに失敗する。キー削除後の検証動作は従来と同じである。
- `verify_standard_paths = false` を設定していたユーザーは、キーが残っている間は設定読み込みに失敗する。キー削除後は、従来スキップされていた標準ディレクトリのコマンドも検証対象となる。

サンプルファイル（`sample/` 配下）からも `verify_standard_paths = false` 行を削除することで、既存のサンプルがパースエラーにならないようにする（FR-3.9.1）。

### 4.3 出力フォーマットの変更

dry-run モードのサマリー出力から `Skipped: N` 行が削除される。これは外部ツールによる出力パースに影響する可能性がある既知の破壊的変更として受け入れる。

## 5. 受け入れ基準

### AC-1: TOML 設定フィールドの削除

- [ ] `GlobalSpec` に `VerifyStandardPaths` フィールドが存在しないこと
- [ ] `verify_standard_paths` を TOML に記述するとパースエラーになること（unknown field として明示的に失敗すること）

### AC-2: ランタイム型の削除

- [ ] `DetermineVerifyStandardPaths()` が存在しないこと
- [ ] `RuntimeGlobal.SkipStandardPaths()` が存在しないこと
- [ ] `RuntimeCommand.SkipBinaryAnalysis` フィールドが存在しないこと

### AC-3: PathResolver の削除

- [ ] `PathResolver` に `skipStandardPaths`、`standardPaths` フィールドが存在しないこと
- [ ] `ShouldSkipVerification()` メソッドが存在しないこと
- [ ] `NewPathResolver` の引数が `skipStandardPaths` パラメータなしになっていること

### AC-4: ハッシュ検証が常に実行されること

- [ ] 標準ディレクトリ（`/bin/ls` 等）に対してもハッシュ検証が実行されること
- [ ] `shouldPerformHashValidation()` が存在しないこと
- [ ] `isStandardDirectory()` が存在しないこと
- [ ] `StandardDirectories` 変数が存在しないこと

### AC-5: `SkipBinaryAnalysis` / `skipBinaryAnalysis` の削除

- [ ] `SkipBinaryAnalysis` / `skipBinaryAnalysis` 識別子への参照（フィールド・パラメータ・ローカル変数・コメント）がプロダクションコードおよびテストコードに存在しないこと

### AC-6: `SkippedFiles` の削除

- [ ] `SkippedFiles` フィールドへの参照がコードベースに存在しないこと
- [ ] dry-run サマリー出力に `Skipped:` 行が存在しないこと

### AC-7: ドキュメントの更新

- [ ] `docs/user/` 配下に `verify_standard_paths` への言及が存在しないこと
- [ ] `docs/user/dry_run_json_schema.md`（日英両方）から `skipped_files` フィールドの説明が削除されていること
- [ ] `sample/` 配下の TOML ファイルに `verify_standard_paths` の記述が存在しないこと
- [ ] `docs/dev/security-architecture.md`（日英両方）から `skipStandardPaths` フィールドを含むコードスニペットが削除されていること
- [ ] `docs/dev/config-inheritance-behavior.md`（日英両方）から `skip_standard_paths` の行が削除されていること
- [ ] `docs/translation_glossary.md` から `verify_standard_paths` の行が削除されていること

### AC-8: ビルドとテストの成功

- [ ] `make build` が成功すること
- [ ] `make test` がすべてパスすること
- [ ] `make lint` がエラーなく完了すること

### AC-9: `IsNetworkOperation` のシグネチャ変更

- [ ] `NetworkAnalyzer.IsNetworkOperation` から `skipBinaryAnalysis bool` パラメータが削除されていること
- [ ] 呼び出し元（`evaluator.go`）が新しいシグネチャに対応していること
- [ ] 削除後もバイナリ解析が標準ディレクトリ以外で実行されること

## 6. 削除対象ファイル・コード箇所一覧

| ファイル | 削除対象 |
|---------|---------|
| `internal/runner/runnertypes/spec.go` | `VerifyStandardPaths *bool` フィールド |
| `internal/runner/config/defaults.go` | `DefaultVerifyStandardPaths` 定数・`ApplyGlobalDefaults` 内処理 |
| `internal/runner/runnertypes/runtime.go` | `DetermineVerifyStandardPaths()`、`SkipStandardPaths()`、`SkipBinaryAnalysis` |
| `internal/verification/path_resolver.go` | `skipStandardPaths`/`standardPaths` フィールド、`ShouldSkipVerification()`、`skipStandardPaths` パラメータ |
| `internal/verification/manager.go` | `skipStandardPaths` 設定・スキップロジック |
| `internal/runner/group_executor.go` | `skippedPaths` マップ・`SkipBinaryAnalysis` 設定 |
| `internal/runner/risk/evaluator.go` | `SkipBinaryAnalysis` 参照・`IsNetworkOperation` 呼び出しシグネチャ変更 |
| `internal/runner/security/network_analyzer.go` | `IsNetworkOperation` の `skipBinaryAnalysis` パラメータ・関連コメント |
| `internal/runner/security/command_analysis.go` | `AnalysisOptions.VerifyStandardPaths` フィールド |
| `internal/runner/security/hash_validation.go` | `shouldPerformHashValidation()` 関数 |
| `internal/runner/security/directory_risk.go` | `StandardDirectories` 変数・`isStandardDirectory()` 関数 |
| `internal/runner/resource/dryrun_manager.go` | `VerifyStandardPaths` 設定 |
| `internal/runner/resource/types.go` | `VerifyStandardPaths bool` フィールド |
| `internal/verification/errors.go` | `SkippedFiles` フィールド（2箇所） |
| `internal/verification/types.go` | `FileVerificationSummary.SkippedFiles` フィールド |
| `internal/verification/result_collector.go` | `skippedFiles` フィールド・`RecordSkip()` メソッド |
| `internal/runner/resource/formatter.go` | `"  Skipped: %d\n"` 出力行 |
| `internal/logging/message_formatter.go` | `skipKeys` の `"skipped_files"` エントリ |
| `cmd/runner/main.go` | `SkippedFiles` ログ出力・`DryRunOptions.VerifyStandardPaths` 設定・`DetermineVerifyStandardPaths` 呼び出し |
| `internal/runner/runner.go` | エラーメッセージ内の `verErr.SkippedFiles` 参照 |
| `docs/user/toml_config/04_global_level.md`（+日本語版） | `verify_standard_paths` の説明 |
| `docs/user/runner_command.md`（+日本語版）、`docs/user/toml_config/09〜11_*.md`（+日本語版）、`docs/user/toml_config/appendix.md`（+日本語版） | `verify_standard_paths` への言及 |
| `docs/user/dry_run_json_schema.md`（+日本語版） | `skipped_files` フィールドの行 |
| `docs/dev/security-architecture.md`（+日本語版） | `PathResolver` 構造体の `skipStandardPaths` フィールドを示すコードスニペット |
| `docs/dev/config-inheritance-behavior.md`（+日本語版） | `skip_standard_paths` の行 |
| `docs/translation_glossary.md` | `標準パス検証 / verify_standard_paths` の行 |
| `CHANGELOG.md` | Breaking Changes エントリの追加（過去エントリは変更しない） |
| `sample/*.toml`（14ファイル） | `verify_standard_paths = false` の行 |
| `internal/runner/runnertypes/spec_test.go` | `VerifyStandardPaths` を参照するテストケース |
| `internal/runner/runnertypes/runtime_test.go` | `SkipStandardPaths()`・`DetermineVerifyStandardPaths()` のテスト |
| `internal/runner/security/command_analysis_test.go` | `VerifyStandardPaths` を参照するテストケース |
| `internal/runner/security/hash_validation_test.go` | `shouldPerformHashValidation()` のテスト |
| `internal/runner/security/directory_risk_test.go` | `isStandardDirectory()` を参照するテストケース |
| `internal/verification/result_collector_test.go` | `RecordSkip()`・`SkippedFiles` のテスト |
| `internal/verification/errors_test.go` | `SkippedFiles` を参照するテストケース |
| `internal/runner/resource/formatter_test.go` | `Skipped:` 出力・`SkippedFiles` を参照するテストケース |
| `internal/runner/group_executor_test.go` | `SkipBinaryAnalysis` を参照するテストケース |
| `internal/runner/config/defaults_test.go`・`loader_defaults_test.go` | `VerifyStandardPaths`・`DefaultVerifyStandardPaths` を参照するテストケース |
| `cmd/runner/integration_security_test.go` | `verify_standard_paths = false`・`VerifyStandardPaths: false` を参照するテストケース |
| `cmd/runner/integration_dryrun_sensitive_test.go` | `DetermineVerifyStandardPaths` を参照するテストケース |
| `cmd/runner/integration_workdir_test.go` | `DetermineVerifyStandardPaths` を参照するテストケース |
