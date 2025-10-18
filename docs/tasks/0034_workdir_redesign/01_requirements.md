# 要件定義書: 作業ディレクトリ仕様の再設計

## 1. プロジェクト概要

### 1.1 プロジェクト名
作業ディレクトリ仕様の再設計 (Working Directory Specification Redesign)

### 1.2 プロジェクト目的
作業ディレクトリの設定階層を簡素化し、デフォルトで一時ディレクトリを使用することでセキュリティを向上させる。また、フィールド名を統一することで設定の直感性を高める。

### 1.3 背景と課題

**現在の状況**:
現在のgo-safe-cmd-runnerでは、作業ディレクトリが以下の3階層で設定可能である：
1. **グローバルレベル**: `Global.WorkDir`（デフォルト: `/tmp`）
2. **グループレベル**: `Group.TempDir` (bool)、`Group.WorkDir`
3. **コマンドレベル**: `Command.Dir`

優先順位は次の通り：
```
1. Command.Dir（最優先）
2. Group.TempDir=true の場合の一時ディレクトリ
3. Group.WorkDir
4. Global.WorkDir
5. カレントディレクトリ（実際には4で設定される）
```

**課題**:
- **階層が複雑**: 3階層の設定があり、優先順位の理解が困難
- **明示的な一時ディレクトリ指定**: `temp_dir=true` を明示的に指定する必要があり、デフォルトでは永続ディレクトリ（`/tmp`）を使用してしまう
- **セキュリティリスク**: デフォルトで `/tmp` を使用するため、ファイルが残留するリスクがある
- **命名の不統一**: コマンドレベルは `dir`、グループレベルは `workdir`、グローバルレベルも `workdir` と不統一
- **`temp_dir` と `workdir` の競合**: 両方指定可能だが、`temp_dir` が優先され `workdir` は無視されるという直感的でない動作

**典型的なユースケース**:
1. **バックアップ処理**: 一時ディレクトリでダンプ・圧縮・アップロードを実行
2. **ビルドパイプライン**: 固定ディレクトリでソースコードをビルド
3. **ログ分析**: 特定ディレクトリ（`/var/log`）からログを読み、一時ディレクトリで処理
4. **データ処理**: 一時ディレクトリでダウンロード・変換・検証を実行

これらのユースケースでは、**デフォルトで一時ディレクトリを使用**し、**必要な場合のみ固定ディレクトリを指定**する方が自然である。

## 2. 機能要件

### 2.1 作業ディレクトリの階層簡素化

#### F001: グローバルレベル `workdir` の廃止
**概要**: `Global.WorkDir` フィールドを削除する

**削除対象**:
- `runnertypes.GlobalConfig.WorkDir` フィールド
- グローバルレベルでの `workdir` TOML設定

**影響**:
- グローバルレベルでのデフォルト作業ディレクトリ設定が不可能になる
- 代わりに、グループごとの自動一時ディレクトリがデフォルトとなる

**破壊的変更**:
- 既存のTOMLファイルで `[global]` セクションに `workdir` を指定している場合、設定ロード時にエラーとなる
- 正式リリース前のため、マイグレーション支援は不要

#### F002: グループレベル `temp_dir` フィールドの廃止
**概要**: `Group.TempDir` (bool) フィールドを削除する

**削除対象**:
- `runnertypes.CommandGroup.TempDir` フィールド
- グループレベルでの `temp_dir` TOML設定

**理由**:
- デフォルトで一時ディレクトリを使用するため、明示的なフラグが不要になる
- 固定ディレクトリを使用したい場合は `workdir` を指定すればよい

**破壊的変更**:
- 既存のTOMLファイルで `temp_dir = true` を指定している場合、設定ロード時にエラーとなる
- `temp_dir = false` を指定していた場合も同様にエラーとなる

#### F003: コマンドレベル `dir` フィールドの名称変更
**概要**: `Command.Dir` フィールドを `Command.WorkDir` に名称変更する

**変更対象**:
- `runnertypes.Command.Dir` → `runnertypes.Command.WorkDir`
- TOML設定: `dir = "/path"` → `workdir = "/path"`

**理由**:
- グループレベルの `workdir` と命名を統一
- より明確な名称（`dir` は汎用的すぎる）

**破壊的変更**:
- 既存のTOMLファイルで `dir` を指定している場合、設定ロード時にエラーとなる

### 2.2 デフォルト動作の変更

#### F004: グループごとの自動一時ディレクトリ生成
**概要**: グループレベルで `workdir` が指定されていない場合、自動的に一時ディレクトリを生成する

**動作**:
```toml
[[groups]]
name = "backup"
# workdir 未指定 → 自動的に一時ディレクトリを生成

[[groups.commands]]
name = "dump"
# /tmp/scr-backup-XXXXXX で実行される
```

**一時ディレクトリ命名規則**:
- プレフィックス: `scr-<グループ名>-`
- サフィックス: ランダム文字列（OSの`TempDir`関数による）
- 例: `/tmp/scr-backup-a1b2c3d4/`

**生成タイミング**:
- グループ実行開始時（最初のコマンド実行前）

**ライフサイクル**:
- グループ内の最後のコマンド実行終了後に自動削除
- **エラー時も削除**（セキュリティ優先）
- `--keep-temp-dirs` フラグ指定時は削除しない（F007参照）

**グループごとの独立性**:
```toml
[[groups]]
name = "group1"
# → /tmp/scr-group1-XXXXXX

[[groups]]
name = "group2"
# → /tmp/scr-group2-YYYYYY

# 各グループで異なる一時ディレクトリが作成される
```

#### F005: 作業ディレクトリの優先順位
**概要**: 作業ディレクトリは以下の優先順位で決定される

**優先順位**:
```
1. コマンドレベル workdir（最優先）
2. グループレベル workdir
3. グループごとの自動一時ディレクトリ（デフォルト）
```

**動作例**:

| グループ workdir | コマンド workdir | 実際の動作 |
|-----------------|-----------------|----------|
| 未指定 | 未指定 | `/tmp/scr-<group>-XXXXXX` |
| 未指定 | `/opt/app` | `/opt/app` |
| `/var/data` | 未指定 | `/var/data` |
| `/var/data` | `/opt/app` | `/opt/app` |

**設定例**:
```toml
# パターン1: すべてデフォルト（一時ディレクトリ）
[[groups]]
name = "backup"

[[groups.commands]]
name = "dump"
# /tmp/scr-backup-XXXXXX で実行

[[groups.commands]]
name = "compress"
# /tmp/scr-backup-XXXXXX で実行（同じディレクトリ）

# パターン2: グループで固定ディレクトリを指定
[[groups]]
name = "build"
workdir = "/opt/project"

[[groups.commands]]
name = "compile"
# /opt/project で実行

[[groups.commands]]
name = "test"
# /opt/project で実行

# パターン3: 一部のコマンドのみ異なるディレクトリ
[[groups]]
name = "log_analysis"
# デフォルトは一時ディレクトリ

[[groups.commands]]
name = "find_logs"
workdir = "/var/log"
# /var/log で実行

[[groups.commands]]
name = "analyze"
# /tmp/scr-log_analysis-XXXXXX で実行

[[groups.commands]]
name = "save_report"
workdir = "/var/reports"
# /var/reports で実行
```

### 2.3 自動変数による一時ディレクトリアクセス

#### F006: `%{__runner_workdir}` 自動変数
**概要**: グループの作業ディレクトリパスを参照する予約変数を提供する

**変数名**: `%{__runner_workdir}`

**値**:
- グループレベルで `workdir` が指定されている場合: その `workdir` のパス
- グループレベルで `workdir` が未指定の場合: 自動生成された一時ディレクトリのパス

**用途**:
- 一時ディレクトリ内のサブディレクトリを参照
- コマンド間でファイルパスを共有

**使用例**:
```toml
[[groups]]
name = "build"
# workdir 未指定 → /tmp/scr-build-XXXXXX が自動生成される

[[groups.commands]]
name = "checkout"
cmd = "git"
args = ["clone", "https://github.com/example/repo.git", "%{__runner_workdir}/project"]
# /tmp/scr-build-XXXXXX/project にクローン

[[groups.commands]]
name = "build"
cmd = "make"
workdir = "%{__runner_workdir}/project"
# /tmp/scr-build-XXXXXX/project で実行

[[groups.commands]]
name = "test"
cmd = "make"
args = ["test"]
workdir = "%{__runner_workdir}/project"
# /tmp/scr-build-XXXXXX/project で実行
```

**固定ディレクトリでの使用例**:
```toml
[[groups]]
name = "deploy"
workdir = "/opt/staging"

[[groups.commands]]
name = "install_deps"
cmd = "npm"
args = ["install", "--prefix", "%{__runner_workdir}"]
# /opt/staging を参照

[[groups.commands]]
name = "run_app"
cmd = "%{__runner_workdir}/bin/start.sh"
# /opt/staging/bin/start.sh を実行
```

**スコープ**:
- グループ内のすべてのコマンドから参照可能
- 各グループで独立した値を持つ（グループAの `%{__runner_workdir}` とグループBの `%{__runner_workdir}` は異なる）

**命名規則の遵守**:
- プレフィックス `__runner_` は予約（Task 0033で定義）
- ユーザー定義の内部変数では `__runner_` で始まる名前は使用不可

### 2.4 コマンドラインオプション

#### F007: `--keep-temp-dirs` フラグ
**概要**: 一時ディレクトリを削除せずに残すオプション

**フラグ名**: `--keep-temp-dirs`

**動作**:
- 指定なし（デフォルト）: グループ実行後に一時ディレクトリを自動削除
- 指定あり: グループ実行後も一時ディレクトリを削除しない

**削除タイミング（デフォルト動作）**:
- グループ内の最後のコマンド実行終了後
- **成功時も失敗時も削除**（セキュリティ優先）

**ユースケース**:
- デバッグ時に一時ディレクトリの内容を確認したい
- 一時ディレクトリに生成されたファイルを手動で確認・保存したい

**使用例**:
```bash
# 通常実行（一時ディレクトリは自動削除）
./runner --config backup.toml

# デバッグ用（一時ディレクトリを保持）
./runner --config backup.toml --keep-temp-dirs

# 実行後に一時ディレクトリの内容を確認
ls -la /tmp/scr-backup-*/
```

**ログ出力**:
- 一時ディレクトリ作成時: `Created temporary directory: /tmp/scr-backup-XXXXXX`
- 一時ディレクトリ削除時: `Cleaned up temporary directory: /tmp/scr-backup-XXXXXX`
- `--keep-temp-dirs` 指定時: `Keeping temporary directory (--keep-temp-dirs): /tmp/scr-backup-XXXXXX`

## 3. 非機能要件

### 3.1 セキュリティ要件

#### NF001: 一時ディレクトリのパーミッション
**要件**: 一時ディレクトリは所有者のみアクセス可能とする

**実装**:
- パーミッション: `0700`（所有者のみ読み書き実行可能）
- OSの`TempDir`関数のデフォルト動作に従う

#### NF002: 一時ディレクトリの確実な削除
**要件**: エラー時も含めて一時ディレクトリを確実に削除する

**重要性**: 一時ディレクトリの削除失敗はセキュリティリスクが高い
- 機密情報が残留する可能性
- ディスク容量の枯渇
- 他のユーザーによる情報漏洩

**実装**:
- `defer` を使用して確実に削除処理を実行
- 削除失敗時は `ERROR` レベルでログ出力し、確実にユーザーに通知
- 削除失敗時も処理は継続（エラーにはしない）
- `--keep-temp-dirs` フラグ指定時のみ削除をスキップ

**通知要件**:
- 削除失敗は標準エラー出力に出力
- ログレベル `ERROR` で記録
- 削除失敗したディレクトリパスを明記

#### NF003: パス検証の強化
**要件**: 絶対パスのみを許可し、相対パス・シンボリックリンクは禁止

**実装**:
- `workdir` は絶対パスのみ許可（現状維持）
- `%{__runner_workdir}` の値は常に絶対パス
- パストラバーサル攻撃を防ぐための検証（現状維持）

### 3.2 後方互換性

#### NF004: 破壊的変更の容認
**方針**: 正式リリース前のため、後方互換性は考慮しない

**影響範囲**:
- `Global.WorkDir` を使用している設定ファイル → TOMLパースエラー（未知のフィールド）
- `Group.TempDir` を使用している設定ファイル → TOMLパースエラー（未知のフィールド）
- `Command.Dir` を使用している設定ファイル → TOMLパースエラー（未知のフィールド）

**エラー処理**:
- 特別なエラーメッセージは不要
- TOMLパーサーの標準エラーメッセージで対応

### 3.3 パフォーマンス要件

#### NF005: 一時ディレクトリ生成のオーバーヘッド最小化
**要件**: 一時ディレクトリの生成・削除がパフォーマンスに悪影響を与えない

**実装**:
- グループごとに1回のみ生成
- 削除は`defer`による非同期処理

### 3.4 運用要件

#### NF006: ログ出力
**要件**: 一時ディレクトリの生成・削除をログに記録する

**ログレベル**:
- 一時ディレクトリ生成: `INFO`
- 一時ディレクトリ削除成功: `DEBUG`
- 削除失敗: `ERROR`（セキュリティリスクのため）
- `--keep-temp-dirs` 指定時: `INFO`

**ログ形式**:
```
INFO  Created temporary directory for group 'backup': /tmp/scr-backup-a1b2c3d4
DEBUG Cleaned up temporary directory: /tmp/scr-backup-a1b2c3d4
ERROR Failed to cleanup temporary directory: /tmp/scr-backup-a1b2c3d4: permission denied
INFO  Keeping temporary directory (--keep-temp-dirs): /tmp/scr-backup-a1b2c3d4
```

**削除失敗時の追加要件**:
- ログレベルを `ERROR` にして確実にユーザーに通知
- 標準エラー出力にも出力
- 削除失敗したディレクトリのパスを明記

## 4. 実装スコープ

### 4.1 実装対象（コア機能）

- [x] `Global.WorkDir` フィールドの削除
- [x] `Group.TempDir` フィールドの削除
- [x] `Command.Dir` → `Command.WorkDir` への名称変更
- [x] グループごとの自動一時ディレクトリ生成
- [x] 一時ディレクトリの自動削除（エラー時含む）
- [x] `%{__runner_workdir}` 自動変数の実装
- [x] `--keep-temp-dirs` コマンドラインフラグの実装
- [x] ログ出力の実装
- [x] 既存テストの更新
- [x] 新規テストケースの追加

### 4.2 実装対象外（将来の拡張）

以下は今回のスコープ外とし、必要に応じて別タスクで実装する：

- [ ] `--keep-temp-dirs-on-error` フラグ（エラー時のみ保持）
- [ ] 設定ファイル自動変換ツール（正式リリース前のため不要）
- [ ] 相対パス対応（`workdir = "./subdir"`）
- [ ] 一時ディレクトリのベースパス指定（環境変数 `TMPDIR` は既にOSレベルで対応済み）
- [ ] 一時ディレクトリの容量制限

## 5. ユースケース

### 5.1 ユースケース1: データベースバックアップ
**シナリオ**: 一時ディレクトリでDB内容をダンプ・圧縮・アップロード

**設定例**:
```toml
[[groups]]
name = "db_backup"
# workdir 未指定 → 一時ディレクトリ自動生成

[[groups.commands]]
name = "dump"
cmd = "pg_dump"
args = ["mydb", "-f", "%{__runner_workdir}/dump.sql"]

[[groups.commands]]
name = "compress"
cmd = "gzip"
args = ["%{__runner_workdir}/dump.sql"]

[[groups.commands]]
name = "upload"
cmd = "aws"
args = ["s3", "cp", "%{__runner_workdir}/dump.sql.gz", "s3://backups/"]
```

**動作**:
1. `/tmp/scr-db_backup-XXXXXX/` を自動生成
2. ダンプファイルを一時ディレクトリに作成
3. 圧縮・アップロードを実行
4. グループ実行後、一時ディレクトリを自動削除

### 5.2 ユースケース2: 差分バックアップ（キャッシュあり）
**シナリオ**: 固定ディレクトリでキャッシュを使用した差分バックアップ

**設定例**:
```toml
[[groups]]
name = "incremental_backup"
workdir = "/var/backup/cache"

[[groups.commands]]
name = "backup"
cmd = "restic"
args = ["backup", "/data", "--cache-dir", "%{__runner_workdir}"]
```

**動作**:
1. `/var/backup/cache` で実行
2. キャッシュファイルは永続化される
3. 一時ディレクトリは生成されない

### 5.3 ユースケース3: ビルドパイプライン
**シナリオ**: 一時ディレクトリでソースコードをチェックアウト・ビルド

**設定例**:
```toml
[[groups]]
name = "build"
# workdir 未指定 → 一時ディレクトリ自動生成

[[groups.commands]]
name = "checkout"
cmd = "git"
args = ["clone", "https://github.com/example/repo.git", "%{__runner_workdir}/project"]

[[groups.commands]]
name = "install_deps"
cmd = "npm"
args = ["install"]
workdir = "%{__runner_workdir}/project"

[[groups.commands]]
name = "build"
cmd = "npm"
args = ["run", "build"]
workdir = "%{__runner_workdir}/project"

[[groups.commands]]
name = "upload_artifacts"
cmd = "aws"
args = ["s3", "cp", "%{__runner_workdir}/project/dist/", "s3://artifacts/", "--recursive"]
```

**動作**:
1. `/tmp/scr-build-XXXXXX/` を自動生成
2. ソースコードをクローン
3. 依存関係インストール・ビルドを実行
4. 成果物をアップロード
5. 一時ディレクトリを自動削除

### 5.4 ユースケース4: ログ分析とレポート生成
**シナリオ**: 特定ディレクトリからログを読み、一時ディレクトリで処理

**設定例**:
```toml
[[groups]]
name = "log_analysis"
# デフォルトは一時ディレクトリ

[[groups.commands]]
name = "find_logs"
cmd = "find"
args = ["/var/log", "-name", "*.log", "-mtime", "-1"]
workdir = "/var/log"
# このコマンドのみ /var/log で実行

[[groups.commands]]
name = "analyze"
cmd = "python"
args = ["analyze.py", "--output", "%{__runner_workdir}/report.json"]
# 一時ディレクトリで実行

[[groups.commands]]
name = "generate_report"
cmd = "python"
args = ["report.py", "--input", "%{__runner_workdir}/report.json"]
# 一時ディレクトリで実行

[[groups.commands]]
name = "save_report"
cmd = "cp"
args = ["%{__runner_workdir}/report.html", "/var/reports/daily_report.html"]
# 一時ディレクトリで実行、出力先は /var/reports
```

**動作**:
1. `/tmp/scr-log_analysis-XXXXXX/` を自動生成
2. `/var/log` でログを検索
3. 一時ディレクトリで分析・レポート生成
4. レポートを `/var/reports` にコピー
5. 一時ディレクトリを自動削除

### 5.5 ユースケース5: デバッグ時の一時ディレクトリ確認
**シナリオ**: エラー時に一時ディレクトリの内容を確認

**コマンド例**:
```bash
# 通常実行（エラー時も一時ディレクトリは削除される）
./runner --config backup.toml
# エラー発生 → /tmp/scr-backup-XXXXXX は既に削除済み

# デバッグ用（一時ディレクトリを保持）
./runner --config backup.toml --keep-temp-dirs
# エラー発生 → /tmp/scr-backup-XXXXXX が残る

# 一時ディレクトリの内容を確認
ls -la /tmp/scr-backup-*/
cat /tmp/scr-backup-*/dump.sql
```

## 6. テスト要件

### 6.1 単体テスト

#### T001: 一時ディレクトリ生成のテスト
- グループ実行時に一時ディレクトリが生成されること
- ディレクトリ名が `scr-<group>-` プレフィックスを持つこと
- パーミッションが `0700` であること

#### T002: 一時ディレクトリ削除のテスト
- グループ実行後に一時ディレクトリが削除されること
- エラー時も削除されること
- `--keep-temp-dirs` フラグ指定時は削除されないこと
- 削除失敗時に `ERROR` レベルでログ出力されること
- 削除失敗時に標準エラー出力に出力されること
- 削除失敗したディレクトリパスがログに記録されること

#### T003: `%{__runner_workdir}` 変数のテスト
- 一時ディレクトリのパスが正しく展開されること
- グループレベル `workdir` 指定時もそのパスが展開されること
- グループごとに異なる値を持つこと

#### T004: workdir 優先順位のテスト
- コマンドレベル `workdir` が最優先されること
- グループレベル `workdir` がデフォルトとして使用されること
- 両方未指定時に一時ディレクトリが使用されること

#### T005: 廃止フィールドのバリデーション
- `Global.WorkDir` を含む設定ファイルがTOMLパースエラーになること
- `Group.TempDir` を含む設定ファイルがTOMLパースエラーになること
- `Command.Dir` を含む設定ファイルがTOMLパースエラーになること

### 6.2 統合テスト

#### T006: エンドツーエンドテスト
- 複数コマンドが同じ一時ディレクトリで実行されること
- 一時ディレクトリ内でファイルを作成・参照できること
- グループ実行後に一時ディレクトリが削除されること

#### T007: 複数グループのテスト
- 各グループで独立した一時ディレクトリが作成されること
- グループAの `%{__runner_workdir}` とグループBの `%{__runner_workdir}` が異なること

#### T008: エラーハンドリングのテスト
- 一時ディレクトリ生成失敗時のエラーハンドリング
- 一時ディレクトリ削除失敗時の `ERROR` レベルログ出力
- 削除失敗時の標準エラー出力への出力
- 削除失敗時にディレクトリパスが記録されること
- 削除失敗時も処理が継続すること（エラー終了しない）
- 存在しない `workdir` 指定時のエラー

### 6.3 パフォーマンステスト

#### T009: 一時ディレクトリ生成のオーバーヘッド測定
- 一時ディレクトリ生成が100ms以内に完了すること
- 多数のグループ実行時もパフォーマンス劣化がないこと

## 7. ドキュメント更新

### 7.1 ユーザードキュメント

#### D001: グループレベル設定ドキュメント
**ファイル**: `docs/user/toml_config/05_group_level.ja.md`

**更新内容**:
- `temp_dir` フィールドの削除（廃止）
- `workdir` のデフォルト動作変更（未指定時は自動一時ディレクトリ）
- `%{__runner_workdir}` 変数の説明追加

#### D002: コマンドレベル設定ドキュメント
**ファイル**: `docs/user/toml_config/06_command_level.ja.md`

**更新内容**:
- `dir` フィールドを `workdir` に変更
- 「実装されていない」という記述を削除
- 実装済みである旨を明記

#### D003: グローバルレベル設定ドキュメント
**ファイル**: `docs/user/toml_config/04_global_level.ja.md`

**更新内容**:
- `workdir` フィールドの削除（廃止）

#### D004: 変数展開ドキュメント
**ファイル**: `docs/user/toml_config/07_variable_expansion.ja.md`（該当ファイルが存在する場合）

**更新内容**:
- `%{__runner_workdir}` 予約変数の説明追加
- 使用例の追加

#### D005: 実用例ドキュメント
**ファイル**: `docs/user/toml_config/08_practical_examples.ja.md`

**更新内容**:
- 一時ディレクトリを使用した例の追加
- ユースケース別の設定例の追加

### 7.2 開発者ドキュメント

#### D006: アーキテクチャドキュメント
**ファイル**: `docs/dev/design-implementation-overview.ja.md`

**更新内容**:
- 一時ディレクトリ管理の説明追加
- `%{__runner_workdir}` の実装方針

### 7.3 サンプルファイル

#### D007: サンプル設定ファイルの更新
**対象ファイル**: `sample/*.toml`

**更新内容**:
- `Global.WorkDir` の削除
- `Group.TempDir` の削除
- `Command.Dir` → `Command.WorkDir` への変更
- `%{__runner_workdir}` を使用した例の追加

## 8. リスク管理

### 8.1 技術リスク

#### R001: 一時ディレクトリ容量不足
**リスク**: `/tmp` の容量が不足する可能性

**影響度**: 中

**対策**:
- ログに一時ディレクトリパスを出力し、手動で確認可能にする
- 将来的に `TMPDIR` 環境変数でベースパスを変更可能（OSレベルで既に対応済み）

#### R002: 一時ディレクトリ削除失敗
**リスク**: パーミッション等の理由で削除が失敗する可能性

**影響度**: 高（セキュリティリスク）

**セキュリティへの影響**:
- 機密情報（データベースダンプ、認証情報等）の残留
- 他のユーザーによる情報漏洩の可能性
- ディスク容量の枯渇

**対策**:
- 削除失敗時は `ERROR` レベルでログ出力
- 標準エラー出力にも出力し、確実にユーザーに通知
- 削除失敗したディレクトリパスを明記
- 削除失敗時も処理は継続（エラーにはしない）
- `--keep-temp-dirs` フラグで意図的に保持可能

### 8.2 運用リスク

#### R003: デフォルト動作変更による混乱
**リスク**: デフォルトが `/tmp` → 一時ディレクトリに変わることで、ユーザーが混乱する可能性

**影響度**: 中

**対策**:
- ドキュメントを更新
- 正式リリース前のため、影響は限定的

## 9. 成功基準

### 9.1 機能面
- [x] すべてのコア機能（F001-F007）が実装されている
- [x] すべてのテスト（T001-T009）が通過する
- [x] 廃止フィールドを含む設定ファイルが適切にエラーを返す

### 9.2 品質面
- [x] コードカバレッジが80%以上
- [x] `make lint` が通過する
- [x] すべてのドキュメントが更新されている

### 9.3 セキュリティ面
- [x] 一時ディレクトリのパーミッションが `0700`
- [x] エラー時も一時ディレクトリが確実に削除される
- [x] パストラバーサル攻撃への対策が維持されている

## 10. スケジュール

### 10.1 実装フェーズ
1. **Phase 1: 型定義とバリデーション** (優先度: 高)
   - `Global.WorkDir` の削除
   - `Group.TempDir` の削除
   - `Command.Dir` → `Command.WorkDir` への名称変更
   - 廃止フィールドのバリデーション追加

2. **Phase 2: 一時ディレクトリ機能** (優先度: 高)
   - グループごとの自動一時ディレクトリ生成
   - 一時ディレクトリの自動削除
   - `--keep-temp-dirs` フラグの実装

3. **Phase 3: 変数展開** (優先度: 中)
   - `%{__runner_workdir}` 自動変数の実装
   - 変数展開処理の統合

4. **Phase 4: テストとドキュメント** (優先度: 中)
   - 単体テスト・統合テストの実装
   - ドキュメントの更新
   - サンプルファイルの更新

### 10.2 完了定義
- すべてのテストが通過
- `make lint` が通過
- ドキュメントが更新されている
- サンプルファイルが更新されている

## 11. 参考資料

### 11.1 関連タスク
- Task 0010: テンプレート機能の削除（`TempDir`、`WorkDir`フィールドの追加）
- Task 0033: 内部変数とプロセス環境変数の分離（`%{}`変数展開、予約プレフィックス）

### 11.2 実装参考箇所
- `internal/runner/runner.go`: グループ実行ロジック
- `internal/runner/resource/normal_manager.go`: 一時ディレクトリ管理
- `internal/runner/config/loader.go`: 設定ファイル読み込み
- `internal/runner/config/expansion.go`: 変数展開処理
