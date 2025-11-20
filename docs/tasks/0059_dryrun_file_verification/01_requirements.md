# Dry-Run モードでのファイル検証機能 - 要件定義書

## 1. 概要

### 1.1 背景

現在、runner の `-dry-run` モードでは実行計画の表示のみを行い、ファイルのハッシュ検証は完全にスキップされる。これは以下の実装によるものである：

**現在の実装** ([internal/verification/manager.go:330-336](internal/verification/manager.go#L330-L336)):
```go
func (m *Manager) verifyFileWithFallback(filePath string) error {
    if m.fileValidator == nil {
        // File validator is disabled (e.g., in dry-run mode) - skip verification
        return nil
    }
    return m.fileValidator.Verify(filePath)
}
```

**初期化処理** ([internal/verification/manager_production.go:28-42](internal/verification/manager_production.go#L28-L42)):
```go
func NewManagerForDryRun() (*Manager, error) {
    // ...
    return newManagerInternal(hashDir,
        withCreationMode(CreationModeProduction),
        withSecurityLevel(SecurityLevelStrict),
        withSkipHashDirectoryValidationInternal(),
        withFileValidatorDisabledInternal(),  // ← ファイル検証を無効化
        withDryRunModeInternal(),
    )
}
```

この設計により、dry-run 実行時には以下の問題が発生する：

1. **検証エラーの見逃し**: 本番実行前にハッシュファイルの不在やハッシュ値の不一致を発見できない
2. **セキュリティリスクの可視化不足**: ファイルの改ざんや不正な変更を事前に検出できない
3. **dry-run の精度低下**: 「本番実行のシミュレーション」としての価値が低い

### 1.2 目的

dry-run モードでもファイルのハッシュ検証を実行し、検証結果を WARNING/ERROR レベルでログ出力することで、以下を実現する：

1. **検証の完全性向上**: 本番実行前のハッシュ検証により、実行時エラーを事前に発見
2. **セキュリティの可視化**: ファイル改ざん検出結果を dry-run 出力に含める
3. **dry-run の価値向上**: より現実的な実行計画の提示

### 1.3 スコープ

#### 対象範囲

- **dry-run モードでのファイル検証**: 検証を実行するが、エラーで終了しない（warn-only モード）
- **検証結果の記録**: `DryRunResult` 構造体への検証結果の追加
- **ログ出力**: 検証失敗時の WARNING/ERROR レベルのログ出力
- **JSON/TEXT フォーマッタ**: 検証結果の表示対応

#### 対象外

- **コマンドライン引数の追加**: 明示的な検証モード指定フラグは追加しない
- **複数の検証モード**: warn-only モードのみをサポート（disabled/strict モードは実装しない）
- **通常実行モードの変更**: 通常実行モードでは引き続き厳格な検証を維持

## 2. 機能要件

### 2.1 ファイル検証動作

#### 2.1.1 検証対象ファイル

dry-run モードで検証する対象ファイルは、通常実行モードと同じ：

1. **設定ファイル** (`VerifyConfigFile`):
   - TOML 設定ファイル自体

2. **グローバル検証ファイル** (`VerifyGlobalFiles`):
   - `Global.VerifyFiles` に指定されたファイル

3. **グループ検証ファイル** (`VerifyGroupFiles`):
   - `Group.VerifyFiles` に指定されたファイル
   - 各コマンドの実行ファイル（`Command.Path`）

4. **環境変数ファイル** (`VerifyEnvFile`):
   - `Global.FromEnv.File` に指定されたファイル（存在する場合）

#### 2.1.2 検証方法

各ファイルに対して以下の検証を実行：

1. **ハッシュファイルの存在確認**:
   - ハッシュディレクトリ（`/etc/runner/hashes`）内の対応するハッシュファイルの存在確認
   - ファイル名は `encoding` パッケージによりエンコードされたパス

2. **ハッシュ値の検証**:
   - ファイルの SHA-256 ハッシュ値とハッシュファイルの内容を比較

3. **標準システムパスの扱い**:
   - `/bin`, `/usr/bin` 等の標準パスは `VerifyStandardPaths` 設定に従ってスキップ可能

#### 2.1.3 検証失敗時の動作

**通常実行モードとの違い**:

| 項目 | 通常実行モード | dry-run モード（本要件） |
|------|--------------|----------------------|
| 検証失敗時の終了 | プログラムを終了（exit 1） | **継続実行** |
| エラーログ | ERROR レベル | ERROR/WARN レベル |
| 検証結果 | `VerificationError` を返す | **検証結果を記録して継続** |

**検証失敗の分類**:

| 失敗理由 | ログレベル | 説明 |
|---------|----------|------|
| ハッシュディレクトリ不在 | **INFO** | ハッシュディレクトリ自体が存在しない（開発環境等） |
| ハッシュファイル不在 | **WARN** | 特定ファイルのハッシュファイルが存在しない |
| ハッシュ値不一致 | **ERROR** | ハッシュファイルは存在するが、値が一致しない（改ざんの可能性） |
| ファイル読み込み失敗 | **ERROR** | 検証対象ファイルの読み込みに失敗（権限エラー等） |
| 標準パススキップ | **INFO** | 標準システムパスのため検証をスキップ |

### 2.2 検証結果の記録

#### 2.2.1 DryRunResult 構造体の拡張

`DryRunResult` ([internal/runner/resource/types.go:147-158](internal/runner/resource/types.go#L147-L158)) に検証結果フィールドを追加：

```go
type DryRunResult struct {
    Metadata         *ResultMetadata       `json:"metadata"`
    Status           ExecutionStatus       `json:"status"`
    Phase            ExecutionPhase        `json:"phase"`
    Error            *ExecutionError       `json:"error,omitempty"`
    Summary          *ExecutionSummary     `json:"summary"`
    ResourceAnalyses []ResourceAnalysis    `json:"resource_analyses"`
    SecurityAnalysis *SecurityAnalysis     `json:"security_analysis"`
    EnvironmentInfo  *EnvironmentInfo      `json:"environment_info"`
    FileVerification *FileVerificationSummary `json:"file_verification,omitempty"` // 新規追加
    Errors           []DryRunError         `json:"errors"`
    Warnings         []DryRunWarning       `json:"warnings"`
}
```

#### 2.2.2 FileVerificationSummary 構造体

```go
type FileVerificationSummary struct {
    TotalFiles      int                        `json:"total_files"`
    VerifiedFiles   int                        `json:"verified_files"`
    SkippedFiles    int                        `json:"skipped_files"`
    FailedFiles     int                        `json:"failed_files"`
    Duration        time.Duration              `json:"duration"`
    HashDirStatus   HashDirectoryStatus        `json:"hash_dir_status"`
    Failures        []FileVerificationFailure  `json:"failures,omitempty"`
}

type HashDirectoryStatus struct {
    Path      string `json:"path"`
    Exists    bool   `json:"exists"`
    Validated bool   `json:"validated"`
}

type FileVerificationFailure struct {
    Path     string              `json:"path"`
    Reason   VerificationFailureReason `json:"reason"`
    Level    string              `json:"level"` // "info", "warn", "error"
    Message  string              `json:"message"`
    Context  string              `json:"context"` // "config", "global", "group:<name>"
}

type VerificationFailureReason string

const (
    ReasonHashDirNotFound    VerificationFailureReason = "hash_directory_not_found"
    ReasonHashFileNotFound   VerificationFailureReason = "hash_file_not_found"
    ReasonHashMismatch       VerificationFailureReason = "hash_mismatch"
    ReasonFileReadError      VerificationFailureReason = "file_read_error"
    ReasonPermissionDenied   VerificationFailureReason = "permission_denied"
    ReasonStandardPathSkipped VerificationFailureReason = "standard_path_skipped"
)
```

### 2.3 ログ出力

#### 2.3.1 検証開始時のログ

```
INFO  Dry-run mode: File verification enabled (warn-only mode)
      hash_directory=/etc/runner/hashes
```

#### 2.3.2 ハッシュディレクトリ不在時

```
INFO  Hash directory not found - skipping all file verification
      hash_directory=/etc/runner/hashes
      reason="Directory does not exist (acceptable in development environments)"
```

#### 2.3.3 ハッシュファイル不在時

```
WARN  File verification failed: hash file not found
      file=/usr/local/bin/myapp
      context=group:build
      hash_file=/etc/runner/hashes/usr_local_bin_myapp.sha256
```

#### 2.3.4 ハッシュ値不一致時

```
ERROR File verification failed: hash mismatch
      file=/usr/local/bin/myapp
      context=group:build
      expected=abc123...
      actual=def456...
      security_risk=high
```

#### 2.3.5 検証完了時のサマリー

```
INFO  File verification completed (dry-run mode)
      total_files=10
      verified_files=8
      skipped_files=0
      failed_files=2
      duration_ms=150
```

### 2.4 出力フォーマット

#### 2.4.1 TEXT フォーマット

dry-run の TEXT 出力に検証結果セクションを追加：

```
=== File Verification Summary ===
Total Files:      10
Verified:         8
Skipped:          0
Failed:           2
Duration:         150ms

Hash Directory:   /etc/runner/hashes
Status:           Exists

=== Verification Failures ===
[WARN] /opt/app/config.json
  Reason:   Hash file not found
  Context:  global

[ERROR] /usr/local/bin/myapp
  Reason:   Hash mismatch
  Context:  group:build
  Expected: abc123def456...
  Actual:   def456abc123...
```

#### 2.4.2 JSON フォーマット

`DryRunResult` の JSON 出力に `file_verification` フィールドが含まれる：

```json
{
  "metadata": { ... },
  "status": "success",
  "file_verification": {
    "total_files": 10,
    "verified_files": 8,
    "skipped_files": 0,
    "failed_files": 2,
    "duration": 150000000,
    "hash_dir_status": {
      "path": "/etc/runner/hashes",
      "exists": true,
      "validated": true
    },
    "failures": [
      {
        "path": "/opt/app/config.json",
        "reason": "hash_file_not_found",
        "level": "warn",
        "message": "Hash file not found",
        "context": "global"
      },
      {
        "path": "/usr/local/bin/myapp",
        "reason": "hash_mismatch",
        "level": "error",
        "message": "Hash value mismatch",
        "context": "group:build"
      }
    ]
  }
}
```

## 3. 非機能要件

### 3.1 パフォーマンス

- **検証時間**: ファイル検証による dry-run 実行時間の増加は許容範囲内であること
- **並列処理**: 大量のファイル検証でも実用的な速度を維持すること（既存実装を活用）

### 3.2 後方互換性

- **dry-run の基本動作**: 検証失敗時もプログラムは正常終了（exit 0）すること
- **既存の出力形式**: JSON/TEXT フォーマットに検証結果を追加しても、既存フィールドは変更しない

### 3.3 セキュリティ

- **センシティブ情報**: `--show-sensitive` フラグが無効の場合、検証結果にセンシティブ情報を含めない
- **検証バイパス**: 通常実行モードでは引き続き厳格な検証を維持すること

### 3.4 保守性

- **コードの整合性**: 通常実行モードと dry-run モードで検証ロジックを共有すること
- **テスタビリティ**: 検証結果の記録と判定を独立してテスト可能にすること

## 4. 制約事項

### 4.1 技術的制約

- **ハッシュディレクトリ**: デフォルトは `/etc/runner/hashes`（変更不可）
- **ハッシュアルゴリズム**: SHA-256 を使用（既存実装に従う）
- **ファイル名エンコーディング**: `filevalidator/encoding` パッケージを使用

### 4.2 運用上の制約

- **開発環境**: ハッシュディレクトリが存在しない環境でも dry-run を実行可能であること
- **CI/CD 環境**: CI パイプラインでの dry-run 実行をブロックしないこと

## 5. 想定ユースケース

### 5.1 本番実行前の検証確認

**シナリオ**: 本番環境でコマンドを実行する前に、ファイル検証が通るか確認したい

```bash
runner -c production.toml --dry-run
```

**期待結果**:
- ハッシュファイル不在やハッシュ値不一致を事前に発見
- 検証失敗があっても dry-run は正常終了し、結果を確認可能

### 5.2 開発環境での動作確認

**シナリオ**: ハッシュディレクトリが存在しない開発環境で dry-run を実行したい

```bash
runner -c dev.toml --dry-run
```

**期待結果**:
- ハッシュディレクトリ不在は INFO レベルで通知
- 検証をスキップして、他の dry-run 結果（コマンド実行計画等）を確認可能

### 5.3 セキュリティ監査

**シナリオ**: 実行予定のファイルが改ざんされていないか確認したい

```bash
runner -c audit.toml --dry-run --dry-run-format=json | jq '.file_verification.failures'
```

**期待結果**:
- ハッシュ値不一致を ERROR レベルで検出
- JSON 出力から検証失敗の詳細を取得可能

## 6. 成功基準

### 6.1 機能面

- [ ] dry-run モードでファイル検証が実行される
- [ ] 検証失敗時も dry-run は継続実行される（exit 0）
- [ ] 検証結果が `DryRunResult` に記録される
- [ ] TEXT/JSON フォーマットで検証結果が出力される

### 6.2 品質面

- [ ] ユニットテストのカバレッジが 80% 以上
- [ ] 統合テストで各検証失敗パターンをカバー
- [ ] 既存の dry-run テストが全て成功

### 6.3 性能面

- [ ] ファイル検証による実行時間増加が 30% 以内
- [ ] 大量ファイル（100 ファイル以上）でも 5 秒以内に検証完了

## 7. リスクと対応策

### 7.1 パフォーマンス劣化

**リスク**: ファイル検証により dry-run の実行時間が大幅に増加する

**対応策**:
- 既存の検証ロジックを活用し、追加の I/O を最小化
- 検証失敗時のログ出力を効率化

### 7.2 下位互換性

**リスク**: JSON 出力形式の変更により、既存のツールが動作しなくなる

**対応策**:
- `file_verification` フィールドはオプショナル（`omitempty`）
- 既存フィールドは変更しない

### 7.3 誤解を招く出力

**リスク**: 検証失敗があっても exit 0 のため、エラーを見逃す可能性

**対応策**:
- 検証失敗を目立たせる出力（色付け、明確なセクション）
- JSON の `status` フィールドで検証失敗を示す（`status: "success_with_warnings"`）

## 8. 関連タスク

- **0007_verify_hash_all**: ファイル検証の基本実装
- **0017_realistic_dry_run**: dry-run モードの Resource Manager Pattern
- **0030_verify_files_variable_expansion**: verify_files の変数展開
- **0057_group_filtering**: グループフィルタリング（dry-run 対応）

## 9. 参考資料

### 9.1 関連ファイル

- [internal/verification/manager.go](../../internal/verification/manager.go) - 検証マネージャー
- [internal/verification/manager_production.go](../../internal/verification/manager_production.go) - dry-run 用初期化
- [internal/runner/resource/types.go](../../internal/runner/resource/types.go) - DryRunResult 定義
- [cmd/runner/main.go](../../cmd/runner/main.go) - dry-run 実行フロー

### 9.2 設計原則

本タスクは以下のプロジェクト設計原則に従う：

- **YAGNI**: 必要最小限の機能（warn-only モードのみ）を実装
- **DRY**: 既存の検証ロジックを再利用
- **Security First**: 検証失敗の分類と適切なログレベル
- **Interface-based Design**: 検証結果の記録と表示を分離
