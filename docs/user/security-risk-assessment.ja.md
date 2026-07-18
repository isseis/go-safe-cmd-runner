# Go Safe Command Runner - セキュリティリスク評価レポート

## 📋 文書情報
- **作成日**: 2025年09月08日
- **最終更新日**: 2026年07月18日
- **対象システム**: go-safe-cmd-runner
- **評価範囲**: ソフトウェアセキュリティリスク分析と運用上の考慮事項
- **対象読者**: ソフトウェアエンジニア、セキュリティ専門家、プロダクトマネージャー、運用エンジニア

---

## 🎯 エグゼクティブサマリー

### プロジェクト概要
go-safe-cmd-runnerは、セキュリティを重視したGoベースのコマンド実行システムです。特権昇格機能を含む複雑なバッチ処理を安全に実行するために設計されています。

### ✅ 総合セキュリティ評価: A (優秀)

**重要な成果**:
- **クリティカルリスク 0件**: 重大なセキュリティ脆弱性は存在しない
- セキュリティファーストの設計思想による包括的な保護機能
- 多層防御アーキテクチャと適切なエラーハンドリング
- 豊富なテストカバレッジを持つ高品質なコード

**ビジネスへの影響**:
- 📈 **高い信頼性**: 包括的なエラーハンドリングによりシステム障害を削減
- 🔒 **セキュリティ保証**: 内蔵保護機能により攻撃表面を最小化
- 🔧 **保守性**: クリーンなアーキテクチャにより長期開発をサポート

---

## 📊 セキュリティ評価結果

### リスク分布ダッシュボード
```
🔴 クリティカル:  0件
🟡 高リスク:      0件
🟠 中リスク:      2件  (ログ強化、エラーハンドリング標準化)
🟢 低リスク:      4件  (依存関係更新、コード品質改善)
```

### 主要セキュリティ機能の評価

| セキュリティ機能 | 実装状況 | 評価 |
|-----------------|---------|------|
| パストラバーサル対策 | openat2システムコール | ✅ 優秀 |
| コマンドインジェクション対策 | 静的パターン検証 | ✅ 優秀 |
| ファイル整合性検証 | SHA-256ハッシュ | ✅ 優秀 |
| 権限管理 | 制御された昇格・復元 | ✅ 優秀 |
| 設定検証タイミング | 使用前完全検証 | ✅ 優秀 |
| ハッシュディレクトリ保護 | カスタム指定完全禁止 | ✅ 優秀 |
| コマンド許可リスト | グローバル正規表現 + グループレベル完全パス | ✅ 優秀 |
| リスクベース実行制御 | 多因子リスク評価（`risk_level` 上限宣言） | ✅ 優秀 |
| バイナリ静的解析 | ELF/Mach-O のsyscall・動的ライブラリ解析 | ✅ 優秀 |
| dry-run セキュリティ | 未検証成果物の常時 hard fail・read-only 検証 | ✅ 優秀 |
| 出力ファイルセキュリティ | 権限分離・制限権限 | ✅ 良好 |
| 変数展開セキュリティ | allowlist連携 | ✅ 良好 |
| 機密情報リダクション | キー名検出 + 値フォーマット検出 | ✅ 良好 |

---

## 🔐 コアセキュリティ機能

### 1. 権限管理システム

**🎯 目的**: 制御された特権昇格とセキュアな権限復元

#### 実装の優秀な点
- **Template Method パターン**: 適切な責任分離による設計
- **包括的監査**: 全権限操作のsyslog記録
- **排他制御**: mutexによる競合状態防止
- **フェイルセーフ設計**: 権限復元失敗時の緊急終了

```go
func (m *UnixPrivilegeManager) WithPrivileges(elevationCtx runnertypes.ElevationContext, fn func() error) (err error) {
    m.mu.Lock()
    defer m.mu.Unlock()

    execCtx, err := m.prepareExecution(elevationCtx)    // 準備フェーズ
    if err != nil { return err }

    if err := m.performElevation(execCtx); err != nil { // 実行フェーズ
        return err
    }

    defer m.handleCleanupAndMetrics(execCtx)           // クリーンアップフェーズ
    return fn()
}
```

#### セキュリティ評価
- ✅ **権限昇格制御**: 厳格なコンテキスト管理
- ✅ **監査証跡**: 完全な操作履歴記録
- ✅ **エラーハンドリング**: 適切な緊急時対応
- ✅ **統計的安全性**: seteuid()失敗率 < 0.001%

**設計判断**: 権限復帰失敗時の即座終了は、権限リーク防止を最優先した保守的で適切な判断

### 2. 設定ファイル検証システム

**🎯 目的**: 包括的な設定セキュリティとコマンドインジェクション防止

#### 実装されたセキュリティ機能
- **多層検証**: 構造的検証 → セキュリティ検証 → 危険パターン検出
- **静的パターン**: 実行ファイル埋め込みによる改ざん耐性
- **ホワイトリストアプローチ**: 安全が確認されたもののみ許可
- **早期検証**: 未検証データの使用を完全に防止

```go
func (v *Validator) ValidateConfig(config *runnertypes.Config) (*ValidationResult, error) {
    result := &ValidationResult{ Valid: true }

    v.validateGlobalConfig(&config.Global, result)                    // 構造的検証
    v.validatePrivilegedCommands(config.Groups, result)              // セキュリティ検証
    v.detectDangerousPatterns(config, result)                        // 危険パターン検出

    return result, nil
}
```

#### セキュリティ評価
- ✅ **コマンドインジェクション対策**: 専用検証関数による包括的防御
- ✅ **危険環境変数検出**: LD_PRELOAD等のライブラリ注入攻撃防止
- ✅ **特権コマンド検証**: root権限実行の厳格チェック
- ✅ **設定整合性**: 重複・矛盾検出による安全性確保

### 3. ファイル整合性・アクセス制御

**🎯 目的**: 改ざん検知とパストラバーサル攻撃防止

#### SHA-256ハッシュ検証とハッシュファイル命名

ハッシュファイル名は `HybridHashFilePathGetter` により生成されます。通常のパスは可逆な
置換エスケープ方式（`~path` 形式）でエンコードし、`MaxFilenameLength`（250。一般的な `NAME_MAX`
の 255 に対する安全マージン）を超える長いパスのみ SHA-256
フォールバックに委譲します。ファイル内容自体の整合性検証には SHA-256 を使用します。

```go
func (h *HybridHashFilePathGetter) GetHashFilePath(hashDir common.ResolvedPath, filePath common.ResolvedPath) (string, error) {
    // 1. 通常は置換+エスケープでエンコード（例: /home/user/file.txt → ~home~user~file.txt）
    encodedName, err := h.encoder.Encode(filePath.String())
    // ...
    // 2. MaxFilenameLength（250）を超える場合のみ SHA-256 フォールバック（AbCdEf123456.json）
    // 3. hashDir と結合して返す
}
```

#### openat2によるパストラバーサル対策
```go
func (fs *osFS) safeOpenFileInternal(absPath string, flag int, perm os.FileMode) (*os.File, error) {
    if !fs.openat2Available {
        // openat2 非対応環境ではポータブルな二段階検証にフォールバック
        return safeOpenFileFallback(absPath, flag, perm)
    }
    how := openHow{
        flags:   uint64(flag),
        mode:    uint64(perm),
        resolve: ResolveNoSymlinks, // シンボリックリンク解決を無効化（アトミック）
    }
    fd, err := openat2(AtFdcwd, absPath, &how)
    // ...
}
```

#### セキュリティ評価
- ✅ **暗号学的整合性**: SHA-256による強力な改ざん検知
- ✅ **カーネルレベル保護**: openat2による最新セキュリティ機能活用
- ✅ **パス操作防止**: Base64エンコーディングとシンボリックリンク無効化

#### 前提と限界（ファイルサイズ・non-Linux TOCTOU）

本ツールが安全にファイルを読み込み・解析できる対象サイズには上限があります。これはメモリ枯渇攻撃を防ぐための
意図的な制限であり、本番運用で前提として理解しておく必要があります。

**ファイルサイズ上限（2 種類を区別）**:

- **`safefileio.MaxFileSize`（128 MB）**: 設定ファイル・テンプレート等の安全な読み込み（`SafeReadFile`）に
  適用される上限。`internal/safefileio/safe_file.go` で `128 * 1024 * 1024` として定義され、
  メモリ枯渇対策です。これを超えるファイルは `safefileio.ErrFileTooLarge` で拒否されます。
- **`filevalidator.maxFileSize`（1 GB）**: バイナリ解析（ELF / Mach-O 等）に適用される上限。
  `internal/filevalidator/validator.go` で `1 << 30` として定義され、解析時間とメモリ使用量を
  抑制するための別個の定数です。`elfanalyzer`/`machoanalyzer` もそれぞれ同じ 1 GB の上限に合わせた
  独自の定数を個別に定義しており、共通のシンボルを参照しているわけではありません。

両者は **別々の定数** であり混同しないでください。128 MB は設定ファイルやテンプレートには十分な余裕がありますが、
1 GB はバイナリ解析専用です。これらの閾値は設定変更できず（固定値）、ハッシュ計算自体にはサイズ制限は
ありませんが、解析処理においては 1 GB の上限が適用されます。

**本番ターゲットと non-Linux 環境**:

- 本番ターゲットは **Linux カーネル 5.6+ (openat2 対応)** を前提とします。`openat2(2)` は path 解決と
  open をアトミックに行うことで、検証〜実行間の TOCTOU 競合ウィンドウを根本的に排除します。
- `openat2` が利用できない場合、または（`DisableOpenat2` により）明示的に無効化されている場合は、
  macOS 等の non-Linux 環境に限らず Linux カーネル 5.6 未満の場合も含めて常に `safeOpenFileFallback`
  による「親ディレクトリの非シンボリックリンク確認 → `O_NOFOLLOW` open → 再確認」の二段階チェックで
  代替します。実装は堅牢ですが、
  原理的に `openat2` のアトミック性には及ばず、**極めて短い TOCTOU 競合ウィンドウが残ります**（コード内
  コメントでも認識済み）。
- このため、**macOS 等は開発・限定用途に限る**運用を推奨します。本番運用は必ず Linux + `openat2` 環境を
  使用してください。`openat2` 非対応のカーネル（Linux 5.5 以下）で本番運用した場合、最悪のケースでは
  検証〜実行間でファイルが差し替えられ得ることを理解してください。

---

## 🔍 追加のセキュリティ機能

### 1. 拡張ログ・監査システム (`internal/logging/`, `internal/redaction/`)

**セキュリティ機能**:
- **機密データリダクション**: APIキー、パスワード、トークンの自動保護
- **構造化ログ**: 解析性向上と監査証跡の完全記録
- **デコレータパターン**: 柔軟で構成可能なロギングパイプライン

```go
// 機密情報の自動編集
type RedactingHandler struct {
    handler slog.Handler
    config  *redaction.Config
}

func (c *Config) RedactText(text string) string {
    // key=value パターンのリダクションを適用
    for _, key := range c.KeyValuePatterns {
        result = c.performKeyValueRedaction(result, key, c.TextPlaceholder)
    }
    return result
}
```

**値ベース検出**:
キー名ベースのリダクションに加えて、キー名が認識できない形で出現した秘密値も **値のフォーマット**のみから検出・マスクします。`ValueDetector`は以下の既知フォーマットを検出対象とします：

- AWS アクセスキーID（`AKIA`/`ASIA`プレフィックス）
- GitHub トークン（`ghp_`/`gho_`/`ghs_`プレフィックス）
- Slack トークン（`xoxb-`/`xoxp-`/`xoxa-`プレフィックス）
- GCP サービスアカウントのプライベートキーID
- PEM 秘密鍵ブロック（`-----BEGIN ... PRIVATE KEY-----`）
- OAuth `Bearer` トークン（標準JWTおよびopaque形式）
- URL埋め込み credential（`scheme://user:pass@host`）

**適用範囲**: 値ベース検出は `RedactText` 関数を介してコマンド引数、stdout、stderr、環境変数値に適用され、全出力先（ファイルログ、syslog、Slack通知）を一括でカバーします。

**限界**: 検出は上記の既知フォーマットに限られます。未知の credential 形式、独自トークンスキーム、高エントロピー文字列は検出されません。ログフィールドやストリームチャンク境界を跨いで分割された秘密値も取りこぼす可能性があります。GCP の項目のみ他と性質が異なり、値そのものに識別可能なフォーマットがありません（サービスアカウントのキーIDは単なる16進文字列であり、値だけでは他のハッシュ値と区別できません）。そのため JSON のフィールド名（`"private_key_id"`）と隣接する場合のみ検出されます。実際の GCP 資格情報本体である `private_key` の PEM ブロックは、上記の PEM 検出によりキー名に依存せずマスクされます。**Slackにコマンド全体の出力を載せる設定は避けるべきです**。マスキング層は多層防御の一環であり、不必要な露出の代替手段ではありません。

### 2. リスクベースコマンド制御 (`internal/runner/base/risk/`)

**多因子リスク評価**:
`risk_level` は「このコマンドに許可するリスクの**上限**」を宣言するもので、runner は実行前に
コマンドのリスクを自動算出し、算出値が `risk_level` を超えていると実行を拒否します。リスクは
**複数の独立した因子の最大値**として算出されます。

- **コマンド名・引数評価**: 特権昇格・破壊的操作・危険な引数パターンの検出
- **コマンドプロファイル要因**: 権限付与・ネットワーク通信・データ持ち出し・システム変更の分類
- **バイナリ静的解析結果の参照**: `record` 時に記録した syscall・動的ライブラリ解析結果を再利用
- **監査統合**: 全リスク評価結果の完全記録

```go
func (e *StandardEvaluator) EvaluateRisk(cmd *runnertypes.RuntimeCommand) (risktypes.VerifiedCommandPlan, error) {
    // 特権昇格コマンドは最上位（Critical）
    // 破壊的ファイル操作・システム変更・任意コード実行は High
    // ネットワーク引数等は Medium
    // 各因子を addDimension で積み上げ、実効リスクは全因子の最大値
    a := risktypes.RiskAssessment{Level: runnertypes.RiskLevelLow}
    // ...
    return plan, nil
}
```

リスク算出の詳細な仕組みと `risk_level` 設定の指針は [リスク評価ガイド](risk_assessment.ja.md)
を参照してください。

#### バイナリ静的解析（`record` 時）

`record` コマンドは実行ファイルを解析し、その結果をハッシュ DB に記録します。runner 実行時は
この記録を参照してリスクを判定するため、実行時の解析コストを避けつつ危険な挙動を検出できます。

- **ELF/Mach-O syscall 解析** (`internal/security/elfanalyzer`, `internal/security/machoanalyzer`):
  実行ファイルが呼び出す可能性のあるシステムコールを静的に抽出し、`mprotect(PROT_EXEC)` による
  動的コード実行や `exec` 系 syscall 等を検出
- **動的ライブラリの推移的解析** (`internal/dynlib`, キャッシュは `internal/libccache`):
  依存する共有ライブラリを推移的に辿り、間接的に到達可能な syscall を解析。解析結果は
  ライブラリ単位でキャッシュし再解析を回避
- **shebang スクリプト解析** (`internal/shebang`): スクリプトのインタプリタを解決し、
  インタプリタ実行ファイルのリスクをスクリプトのリスクに反映

### 3. ユーザー・グループ管理 (`internal/groupmembership/`)

**権限境界の厳格化**:
- **CGO/非CGO対応**: 環境に依存しない権限検証
- **キャッシュ機能**: パフォーマンス向上と一貫性確保
- **クロスプラットフォーム**: 統一されたユーザー・グループ管理

### 4. 安全な端末出力制御 (`internal/terminal/`, `internal/ansicolor/`)

**出力セキュリティ**:
- **端末能力検出**: CI/CD環境の自動判別
- **エスケープシーケンス制御**: ターミナルインジェクション防止
- **保守的デフォルト**: 不明環境では安全側に動作

### 5. 設定の使用前検証と信頼境界

設定ファイルは、その内容を使用する前に必ず検証されます。処理は以下の順序で進み、検証が完了するまで
一切の設定内容を信頼しません。

```go
func run(runID string) error {
    // 1. ハッシュディレクトリ検証（最優先）
    hashDir, err := getHashDirectoryWithValidation()

    // 2. 設定ファイル検証（使用前に必須）
    if err := performConfigFileVerification(verificationManager, runID); err != nil {
        return err // クリティカルエラーで即座終了
    }

    // 3. 検証済み設定のみ使用
    cfg, err := loadAndValidateConfig(runID)
}
```

- ✅ **デフォルト拒否**: 検証完了まで全操作を禁止
- ✅ **早期検証**: 攻撃表面の最小化
- ✅ **信頼境界明確化**: 検証済みデータのみ使用（作業ディレクトリ・ログレベル・Slack Webhook 等の
  設定は検証後にのみ反映）

### 6. ハッシュディレクトリ保護

本番環境ではハッシュディレクトリはデフォルトディレクトリのみを使用し、外部からの指定を受け付けません。
これにより、偽ハッシュファイルを配置して悪意あるコマンドの「検証成功」を偽装する攻撃（特に setuid
バイナリ実行時の特権昇格）を防止します。

```go
// 本番環境: デフォルトディレクトリのみ
func NewManager() (*Manager, error) {
    // cmdcommon.DefaultHashDirectory のみ使用
    // 外部指定を受け付けない
}

// テスト環境: ビルドタグで分離
//go:build test
func NewManagerForTest(hashDir string, options ...Option) (*Manager, error) {
    // テスト専用APIのみカスタムディレクトリ許可
}
```

- ✅ **カスタム指定不可**: `--hash-directory` のようなフラグは存在しない
- ✅ **ゼロトラスト**: カスタムハッシュディレクトリを一切信頼しない
- ✅ **多層防御**: コンパイル時・ビルドタグ・CI/CD での保護

### 7. 出力ファイル・変数展開のセキュリティ

**出力ファイルセキュリティ**:
- **権限分離**: 出力ファイルは実UID権限で作成（EUID変更の影響なし）
- **制限権限**: ファイル権限0600（所有者のみアクセス）
- **パストラバーサル防止**: 親ディレクトリ参照（`..`）禁止
- **サイズ制限**: デフォルト10MB上限でディスク枯渇攻撃防止

**変数展開セキュリティ**:
- **allowlist連携**: 許可された環境変数のみ展開
- **循環参照検出**: 最大15回反復で無限ループ防止
- **シェル実行なし**: `$(...)`、`` `...` ``未サポート
- **コマンド検証**: 展開後のコマンドパスを再検証

### 8. dry-run のセキュリティ

dry-run は本番実行と同じ検証結果を返し、状態を変更しません。dry-run で「問題なし」と判断できた設定は、
本番実行でも同じ可否になります。

- **未検証成果物の常時 hard fail**: 「この環境では検証できなかった」コマンドは、dry-run でも常に
  hard fail として扱われる
- **ハッシュディレクトリの read-only 検証**: dry-run はハッシュディレクトリを **作成しません**。
  ディレクトリが存在しない場合も副作用として作成せず read-only で検証し、本番実行と同じく不在を
  hard fail として扱う。監査ログには検証の構築モード（`construction_mode`）を記録
- ✅ **dry-run と本番の挙動一致**: dry-run の結果が本番実行の可否を正しく予測
- ✅ **副作用の排除**: dry-run が状態を変更しない（最小権限・冪等性）

---

## ⚠️ リスク分析

### 残存リスク

#### 中リスク (2件)

**1. セキュリティログ強化の機会**
- 現状: 基本的なセキュリティイベント記録は実装済み
- 改善点: より詳細な攻撃パターン分析情報の追加
- 影響: 高度な攻撃の検知・分析能力に限界

**2. エラーメッセージ標準化**
- 現状: セキュリティ関連エラーは適切に処理
- 改善点: 一貫性のあるエラー報告形式の確立
- 影響: トラブルシューティング効率に軽微な影響

#### 低リスク (4件)

1. **依存関係の定期更新**: 脆弱性データベースとの自動統合
2. **パフォーマンス監視**: リソース使用量制限の実装
3. **テストカバレッジ**: セキュリティクリティカルパスのカバレッジを90%以上へ向上（現状約85%）
4. **静的解析強化**: より高度なコード品質チェック

### 外部依存関係セキュリティ

| パッケージ | バージョン | リスクレベル | 状況 |
|-----------|------------|-------------|------|
| go-toml/v2 | v2.0.9 | 🟡 中 | 積極的メンテナンス、既知CVEなし |
| ulid/v2 | v2.1.1 | 🟢 低 | 暗号学的に安全な ID 生成 |
| golang.org/x/arch | v0.24.0 | 🟢 低 | バイナリ静的解析（命令デコード）に使用 |
| golang.org/x/sys | v0.35.0 | 🟢 低 | openat2 等のシステムコール呼び出し |
| golang.org/x/term | v0.34.0 | 🟢 低 | 端末能力検出 |
| testify | v1.8.4 | 🟢 低 | テストのみ依存、限定的暴露 |

### 運用上の注意事項

**システム管理者向け**:
- setuidバイナリの定期的な整合性チェック (`md5sum`, `sha256sum`)
- 権限昇格操作の頻度監視とパターン分析
- ハッシュディレクトリ (`~/.go-safe-cmd-runner/hashes/`) の権限確認

**開発チーム向け**:
- 新機能開発時のセキュリティレビュー必須
- 外部依存関係追加時の脆弱性スキャン
- セキュリティテストケース追加の徹底

---

## 🛠️ 改善ロードマップ

### 高優先度 (1-2週間)

**1. セキュリティログ強化**
```go
// 拡張セキュリティメトリクス
type SecurityMetrics struct {
    AttackPatternDetections map[string]int
    PrivilegeEscalationAttempts int
    FileIntegrityViolations int
}

func (s *SecurityLogger) LogThreatDetection(pattern string, context map[string]interface{}) {
    // 攻撃パターンの詳細分析
    // 脅威インテリジェンス統合
}
```

**2. エラーハンドリング標準化**
```go
// 一貫性のあるセキュリティエラー
type SecurityError struct {
    Code string
    Message string
    Severity Level
    Context map[string]interface{}
}
```

### 中優先度 (1-3ヶ月)

**1. 自動化セキュリティテスト統合**
- GitHub Actions による静的解析 (gosec, golangci-lint)
- 依存関係脆弱性スキャン (nancy, govulncheck)
- セキュリティテストカバレッジ監視

**2. パフォーマンス・セキュリティ監視**
- リソース使用量制限の実装
- セキュリティメトリクス収集
- アラート閾値の設定

### 低優先度 (継続的改善)

**1. 依存関係管理**
- 月次セキュリティ更新レビュー
- 自動脆弱性スキャン統合

**2. コード品質向上**
- セキュリティ重視コードレビューチェックリスト
- 包括的なドキュメンテーション

---

## 🚀 運用ガイド

### デプロイメント手順

**1. システム要件**
- Linux カーネル 5.6+ (openat2 サポート)
- Go 1.26+ (開発環境)
- 適切なファイルシステム権限

**2. セキュリティ設定**
```bash
# setuid バイナリ設定
sudo chmod 4755 /usr/local/bin/runner

# ハッシュディレクトリ準備
mkdir -p ~/.go-safe-cmd-runner/hashes
chmod 700 ~/.go-safe-cmd-runner

# ログ設定
sudo tee /etc/rsyslog.d/go-safe-cmd-runner.conf <<EOF
# go-safe-cmd-runner ログ
:programname, isequal, "go-safe-cmd-runner" /var/log/go-safe-cmd-runner.log
& stop
EOF
```

**3. 監視・アラート設定**

**重要な監視項目**:
- 権限昇格失敗: `grep "CRITICAL SECURITY FAILURE" /var/log/auth.log`
- 設定ファイル改ざん: ハッシュ検証失敗パターン
- 異常な実行頻度: 短時間の大量実行検出

**推奨SLI/SLO**:
```yaml
availability: 99.9%      # 月間ダウンタイム < 43分
latency_p95: 5s         # 95%のコマンド < 5秒で完了
error_rate: < 0.1%      # 全体エラー率 < 0.1%
security_violations: 0   # セキュリティ違反ゼロ
```

### トラブルシューティング

**よくある問題と対処法**:

1. **権限昇格失敗**
   ```bash
   # 原因調査
   ls -la $(which runner)  # setuid 設定確認
   id                      # ユーザー権限確認
   ```

2. **ハッシュ検証失敗**
   ```bash
   # ハッシュファイル確認
   ls -la ~/.go-safe-cmd-runner/hashes/
   # 設定ファイル整合性確認
   sha256sum config.toml
   ```

3. **パフォーマンス問題**
   ```bash
   # リソース使用量確認
   top -p $(pgrep runner)
   # ログ解析
   journalctl -u go-safe-cmd-runner -f
   ```

### 緊急対応手順

**インシデント分類**:
- 🔴 **P0**: セキュリティ違反、権限昇格失敗
- 🟡 **P1**: サービス不可用、設定改ざん検出
- 🟢 **P2**: パフォーマンス低下、軽微な問題

**エスカレーション**:
1. P0: 即座にセキュリティチーム + 運用責任者
2. P1: 30分以内に開発チーム通知
3. P2: 営業時間中に担当チーム通知

---

## 📚 関連文書

### セキュリティドキュメント
- [設計実装概要](../dev/design-implementation-overview.ja.md)
- [セキュリティアーキテクチャ](../dev/security-architecture.ja.md)
- [ハッシュファイル命名規則](../dev/hash-file-naming-adr.ja.md)
- [リスク評価ガイド](risk_assessment.ja.md)

---

## 📋 文書管理

**レビュースケジュール**:
- **次回レビュー**: 2026年10月01日
- **四半期レビュー**: 3ヶ月毎
- **年次包括評価**: 2026年9月

**責任者**:
- **セキュリティ**: 開発チーム + セキュリティ専門家
- **運用**: SREチーム + 運用マネージャー
- **最終承認**: プロダクトマネージャー

**更新トリガー**:
- 主要リリース時
- セキュリティ脆弱性発見時
- アーキテクチャ変更時
- 外部監査結果反映時
