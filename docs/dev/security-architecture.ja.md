# Go Safe Command Runner - セキュリティアーキテクチャ技術文書

## 概要

本文書は、Go Safe Command Runnerプロジェクトに実装されたセキュリティ対策の包括的な技術解析を提供します。システムの設計原則、実装詳細、およびセキュリティ保証について理解が必要なソフトウェアエンジニアやセキュリティ専門家を対象としています。

## 要約

Go Safe Command Runnerは、特権操作の安全な委譲と自動化されたバッチ処理を可能にするため、複数層のセキュリティ制御を実装しています。セキュリティモデルは多層防御の原則に基づいて構築され、ファイル整合性検証、環境変数分離、特権管理、および安全なファイル操作を組み合わせています。

## 主要なセキュリティ機能

### 1. ファイル整合性検証

#### 目的
実行前に実行ファイルや重要なファイルが改ざんされていないことを確認し、侵害されたバイナリの実行を防止します。システムは現在、`internal/verification/` パッケージによる一元化された検証管理を提供します。

#### 実装詳細

**ハッシュアルゴリズム**: SHA-256暗号化ハッシュ
- 場所: `internal/filevalidator/hash_algo.go`
- Go標準の`crypto/sha256`ライブラリを使用
- 強力な衝突耐性のための256ビットハッシュ値を提供

**ハッシュストレージシステム**:
- ハッシュファイルは専用ディレクトリにJSONマニフェストとして保存
- 特殊文字を処理するためBase64 URL-safe encodingを使用してファイルパスをエンコード
- マニフェスト形式にはファイルパス、ハッシュ値、アルゴリズム、タイムスタンプが含まれる
- 衝突検出により、パスのハッシュが衝突した場合に、異なるファイルパスが同じハッシュマニフェストファイルにマッピングされるのを防止

**検証プロセス**:
```go
// 場所: internal/filevalidator/validator.go:169-197
func (v *Validator) Verify(filePath string) error {
    // 1. ファイルパスの検証と解決
    targetPath, err := validatePath(filePath)

    // 2. 現在のファイルハッシュを計算
    actualHash, err := v.calculateHash(targetPath.String())

    // 3. マニフェストから保存されたハッシュを読み取り
    _, expectedHash, err := v.readAndParseHashFile(targetPath)

    // 4. ハッシュを比較
    if expectedHash != actualHash {
        return ErrMismatch
    }
    return nil
}
```


**一元化検証管理**:
- 場所: `internal/verification/manager.go`
- すべてのファイル検証操作のための統一インターフェース
- 権限制限ファイルに対する自動特権昇格フォールバック
- 標準システムパススキップ機能

**特権ファイルアクセス**:
- 権限により通常の検証が失敗した場合、特権昇格にフォールバック
- 安全な特権管理を使用（特権管理セクション参照）
- 場所: `internal/filevalidator/privileged_file.go`

#### セキュリティ保証
- 実行ファイル、設定ファイルの不正な変更を検出
- 改ざんされたバイナリの実行を防止
- 暗号学的に強力なハッシュアルゴリズム（SHA-256）
- 原子的ファイル操作により競合状態を防止

### 2. 環境変数分離

#### 目的
環境変数の厳格な許可リストベースのフィルタリングを実装し、環境操作による情報漏洩やコマンドインジェクション攻撃を防止します。

#### 実装詳細

**許可リストアーキテクチャ**:
```go
// 場所: internal/runner/environment/filter.go:31-50
type Filter struct {
    config          *runnertypes.Config
    globalAllowlist map[string]bool // O(1)検索パフォーマンス
}
```

**3レベル継承モデル**:

1. **グローバル許可リスト**: すべてのグループで利用可能な基本環境変数
2. **グループオーバーライド**: グループが独自の許可リストを定義し、グローバル設定を完全にオーバーライド
3. **継承制御**: 明示的な許可リストを持たないグループはグローバル設定を継承

**継承モード**:
- `InheritanceModeInherit`: グローバル許可リストを使用
- `InheritanceModeExplicit`: グループ固有の許可リストのみを使用
- `InheritanceModeReject`: 環境変数を許可しない（空の許可リスト）

**変数検証**:
```go
// 場所: internal/runner/config/validator.go
func (v *Validator) validateVariableValue(value string) error {
    // 一元化されたセキュリティ検証を使用
    if err := security.IsVariableValueSafe(value); err != nil {
        // 一貫性のため検証エラー型でセキュリティエラーをラップ
        return fmt.Errorf("%w: %s", ErrDangerousPattern, err.Error())
    }
    return nil
}
```

**危険パターン検出**:
- コマンド区切り文字: `;`, `|`, `&&`, `||`
- コマンド置換: `$(...)`, バッククォート
- ファイル操作: `>`, `<`, `rm `, `dd if=`, `dd of=`
- コード実行: `exec `, `system `, `eval `

#### セキュリティ保証
- ゼロトラスト環境変数モデル（許可リストのみ）
- 環境ベースのコマンドインジェクションを防止
- 機密変数のグループレベル分離
- 危険なパターンに対する変数名と値の検証

### 3. 安全なファイル操作

#### 目的
シンボリックリンク攻撃、TOCTOU（Time-of-Check-Time-of-Use）競合状態、パストラバーサル攻撃を防ぐため、シンボリックリンク安全なファイルI/O操作を提供します。

#### 実装詳細

**最新Linuxセキュリティ（openat2）**:
```go
// 場所: internal/safefileio/safe_file.go:99-122
func openat2(dirfd int, pathname string, how *openHow) (int, error) {
    // RESOLVE_NO_SYMLINKSフラグを使用してシンボリックリンクの追跡を原子的に防止
    pathBytes, err := syscall.BytePtrFromString(pathname)
    fd, _, errno := syscall.Syscall6(SysOpenat2, ...)
    return int(fd), nil
}
```

**フォールバックセキュリティ（従来システム）**:
```go
// 場所: internal/safefileio/safe_file.go:409-433
func ensureParentDirsNoSymlinks(absPath string) error {
    // ルートからターゲットまでのステップバイステップパス検証
    for _, component := range components {
        fi, err := os.Lstat(currentPath) // シンボリックリンクを追跡しない
        if fi.Mode()&os.ModeSymlink != 0 {
            return fmt.Errorf("%w: %s", ErrIsSymlink, currentPath)
        }
    }
    return nil
}
```

**ファイルサイズ保護**:
- 最大ファイルサイズ制限: 128 MB
- メモリ枯渇攻撃を防止
- カスタムサイズ制限ライターによる書き込みサイズの制御

**パス検証**:
- 絶対パス要求
- パス長制限（設定可能、デフォルト4096文字）
- 通常ファイルタイプの検証
- デバイスファイル、パイプ、特殊ファイルは許可しない

#### セキュリティ保証
- 最新Linux上での原子的シンボリックリンク安全操作（openat2）
- 包括的パストラバーサル保護
- TOCTOU競合状態の排除
- メモリ枯渇攻撃に対する保護
- 安全なファイルタイプ検証

### 4. 特権管理

#### 目的
最小特権の原則を維持しながら特定の操作に対する制御された特権昇格を可能にし、包括的な監査証跡を提供します。

#### 実装詳細

**Unix特権アーキテクチャ**:
```go
// 場所: internal/runner/privilege/unix.go:18-25
type UnixPrivilegeManager struct {
    logger             *slog.Logger
    originalUID        int
    privilegeSupported bool
    metrics            Metrics
    mu                 sync.Mutex  // 競合状態を防止
}
```

**特権昇格プロセス**:
```go
// 場所: internal/runner/privilege/unix.go:36-87
func (m *UnixPrivilegeManager) WithPrivileges(elevationCtx runnertypes.ElevationContext, fn func() error) (err error) {
    m.mu.Lock()  // スレッドセーフティのためのグローバルロック
    defer m.mu.Unlock()

    // 1. 特権を昇格
    if err := m.escalatePrivileges(elevationCtx); err != nil {
        return err
    }

    // 2. deferベースのクリーンアップで操作を実行
    defer func() {
        if err := m.restorePrivileges(); err != nil {
            m.emergencyShutdown(err, shutdownContext) // 失敗時に終了
        }
    }()

    return fn()
}
```

**実行モード**:

1. **ネイティブルート実行**: ルートユーザー（UID 0）として実行
   - 特権昇格は不要
   - 完全な特権での直接実行

2. **setuidバイナリ実行**: setuidビット設定とルート所有権を持つバイナリ
   - 特権昇格に`syscall.Seteuid(0)`を使用
   - 操作後の自動特権復元

**セキュリティ検証**:
```go
// 場所: internal/runner/privilege/unix.go:232-294
func isRootOwnedSetuidBinary(logger *slog.Logger) bool {
    // setuidビットが設定されていることを検証
    hasSetuidBit := fileInfo.Mode()&os.ModeSetuid != 0

    // ルート所有権を検証（setuidが動作するために不可欠）
    isOwnedByRoot := stat.Uid == 0

    // 非ルート実UID を検証（真のsetuidシナリオ）
    isValidSetuid := hasSetuidBit && isOwnedByRoot && originalUID != 0

    return isValidSetuid
}
```

**緊急シャットダウンプロトコル**:
- 特権復元失敗時の即座のプロセス終了
- マルチチャンネルログ（構造化ログ、syslog、stderr）
- 完全なコンテキストでのセキュリティイベント記録
- 侵害された状態での継続実行防止

#### セキュリティ保証
- グローバルmutexによるスレッドセーフな特権操作
- パニック保護付きの自動特権復元
- すべての特権操作の包括的監査ログ
- セキュリティ障害時の緊急シャットダウン
- ネイティブルートとsetuidバイナリ実行モデルの両方をサポート

### 5. コマンドパス検証

#### 目的
設定可能な許可リストに対してコマンドパスを検証し、危険なバイナリの実行を防ぐことで、認可されたコマンドのみが実行できることを確保します。環境変数の継承を停止し、セキュアな固定PATHを使用します。

#### 実装詳細

**セキュアPATH環境の強制**:
```go
// 場所: internal/verification/manager.go
const securePathEnv = "/sbin:/usr/sbin:/bin:/usr/bin"

// 環境変数PATHを継承せず、セキュアな固定PATHを使用
pathResolver := NewPathResolver(securePathEnv, securityValidator, false)
```

**パス解決**:
```go
// 場所: internal/verification/path_resolver.go
type PathResolver struct {
    pathEnv            string    // セキュア固定PATH使用
    securityValidator  *security.Validator
    skipStandardPaths  bool
}
```

**コマンド検証プロセス**:
1. PATH環境変数を使用してコマンドを完全なパスに解決
2. 許可リストパターン（正規表現ベース）に対して検証
3. 危険な特権コマンドをチェック
4. ハッシュが利用可能な場合はファイル整合性を検証

**デフォルト許可パターン**:
```go
// 場所: internal/runner/security/types.go:147-154
AllowedCommands: []string{
    "^/bin/.*",
    "^/usr/bin/.*",
    "^/usr/sbin/.*",
    "^/usr/local/bin/.*",
},
```

**危険コマンド検出**:
- シェル実行ファイル: `/bin/bash`, `/bin/sh`
- 特権昇格ツール: `sudo`, `su`, `doas`
- システム管理: `rm`, `dd`, `mount`, `umount`
- パッケージ管理: `apt`, `yum`, `dnf`
- サービス管理: `systemctl`, `service`

#### セキュリティ保証
- 許可リストベースのコマンド実行
- 任意のコマンド実行の防止
- 危険な特権操作の検出
- パス解決セキュリティ検証
- 環境変数PATH継承の完全排除
- セキュアな固定PATH（/sbin:/usr/sbin:/bin:/usr/bin）の強制使用

### 6. リスクベースコマンド制御

#### 目的
コマンドリスク評価に基づくインテリジェントなセキュリティ制御を実装し、高リスク操作を自動的にブロックしながら安全なコマンドの正常実行を可能にします。

#### 実装詳細

**リスク評価エンジン**:
```go
// 場所: internal/runner/risk/evaluator.go
type StandardEvaluator struct{}

func (e *StandardEvaluator) EvaluateRisk(cmd *runnertypes.Command) (runnertypes.RiskLevel, error) {
    // 特権昇格コマンドをチェック（クリティカルリスク - ブロックされるべき）
    isPrivEsc, err := security.IsPrivilegeEscalationCommand(cmd.Cmd)
    if err != nil {
        return runnertypes.RiskLevelUnknown, err
    }
    if isPrivEsc {
        return runnertypes.RiskLevelCritical, nil
    }
    // ... 追加のリスク評価ロジック
}
```

**コマンドリスク分析**:
- 低リスク: 標準システムユーティリティ（ls、cat、grep）
- 中リスク: ファイル変更コマンド（cp、mv、chmod）、パッケージ管理（apt、yum）
- 高リスク: システム管理コマンド（mount、systemctl）、破壊的操作（rm -rf）
- クリティカルリスク: 特権昇格コマンド（sudo、su）- 自動的にブロック

**リスクレベル設定**:
```go
// 場所: internal/runner/runnertypes/config.go
type Command struct {
    RiskLevel string `toml:"risk_level"` // コマンドのリスクレベル
}
```

#### セキュリティ保証
- 特権昇格試行の自動ブロック
- コマンド毎の設定可能リスク閾値
- 包括的コマンドパターンマッチング
- リスクベース監査ログ

### 7. リソース管理セキュリティ

#### 目的
通常実行とdry-runモードの両方でセキュリティ境界を維持する安全なリソース管理を提供します。

#### 実装詳細

**統一リソースインターフェース**:
```go
// 場所: internal/runner/resource/manager.go
type ResourceManager interface {
    ExecuteCommand(ctx context.Context, cmd runnertypes.Command, group *runnertypes.CommandGroup, env map[string]string) (*ExecutionResult, error)
    WithPrivileges(ctx context.Context, fn func() error) error
    SendNotification(message string, details map[string]any) error
}
```

**実行モードセキュリティ**:
- 通常モード: 完全な特権管理とコマンド実行
- dry-runモード: 実際の実行なしでのセキュリティ分析
- 両モード間での一貫したセキュリティ検証

#### セキュリティ保証
- モードに依存しないセキュリティ検証
- 特権境界執行
- 安全な通知処理
- リソースライフサイクル管理

### 8. セキュアログと機密データ保護

#### 目的
パスワード、APIキー、トークンなどの機密情報がログファイルに露出することを防ぎ、機密データを侵害することなく安全な監査証跡を提供します。専用の編集サービスで強化され、多層防御アプローチにより包括的な保護を実現します。

#### 実装詳細

##### 現在の実装

**一元化データ編集基盤**:
```go
// 場所: internal/redaction/redactor.go
type Config struct {
    LogPlaceholder   string
    TextPlaceholder  string
    Patterns         *SensitivePatterns
    KeyValuePatterns []string
}

func (c *Config) RedactText(text string) string {
    // 設定されたすべての編集パターンを適用
}

func (c *Config) RedactLogAttribute(attr slog.Attr) slog.Attr {
    // ログ属性の機密情報を編集
}
```

**RedactingHandler（ログレベル編集）**:
```go
// 場所: internal/redaction/redactor.go:200-259
type RedactingHandler struct {
    handler slog.Handler
    config  *Config
}

// 場所: internal/runner/bootstrap/logger.go:138
redactedHandler := redaction.NewRedactingHandler(multiHandler, nil)
logger := slog.New(redactedHandler)
```
- ログ出力時に自動的に機密情報を編集
- すべてのログハンドラー（ファイル、syslog、Slack）をラップ
- `slog.KindGroup`を含む構造化ログの再帰的処理
- key=value形式と認証ヘッダーパターンの両方をサポート

**現在のSlack通知実装**:
```go
// 場所: internal/logging/slack_handler.go:64-73
type SlackHandler struct {
    webhookURL    string
    runID         string
    httpClient    *http.Client
    level         slog.Level
    attrs         []slog.Attr
    groups        []string
    backoffConfig BackoffConfig
}
```
- RedactingHandlerによってラップされているため、基本的な編集が適用される
- コマンド出力の長さ制限（stdout: 1000文字、stderr: 500文字）

##### 計画中の拡張（task 0055）

**多層防御による機密情報保護の強化**:

システムは二重の防御層アプローチを採用し、一方の対策が失敗しても他方が保護します：

1. **第1層：CommandResult作成時の編集**（計画中）
   - 場所: `internal/runner/group_executor.go`（変更予定）
   - コマンド出力を`CommandResult`に格納する際に事前編集
   - `security.Validator.SanitizeOutputForLogging()`を使用
   - 現在の実装: 編集なしで生の出力を格納

2. **第2層：RedactingHandlerでの編集**（実装済み）
   - 場所: `internal/redaction/redactor.go:200-259`
   - ログ出力時に`slog.KindAny`型と`LogValuer`インターフェースを処理
   - 第1層で漏れた機密情報もここでキャッチ
   - 再帰深度制限により無限再帰を防止

**Slack通知への機密情報保護の強化**（計画中）:
```go
// 計画中の実装: internal/logging/slack_handler.go
type SlackHandler struct {
    webhookURL string
    redactor   *redaction.Redactor  // 追加予定
}
```
- Slack通知送信前の明示的な編集処理追加
- `[]common.CommandResult`スライス内の機密情報の完全な編集
- 外部通信における追加の保護層

**ログセキュリティ設定**:
```go
// 場所: internal/runner/security/types.go:92-107
type LoggingOptions struct {
    // IncludeErrorDetails は完全なエラーメッセージをログに含めるかを制御
    IncludeErrorDetails bool `json:"include_error_details"`

    // MaxErrorMessageLength はログ内のエラーメッセージの長さを制限
    MaxErrorMessageLength int `json:"max_error_message_length"`

    // RedactSensitiveInfo は機密パターンの自動編集を有効化
    RedactSensitiveInfo bool `json:"redact_sensitive_info"`

    // TruncateStdout はエラーログでstdoutを切り詰めるかを制御
    TruncateStdout bool `json:"truncate_stdout"`

    // MaxStdoutLength はエラーログ内のstdoutの長さを制限
    MaxStdoutLength int `json:"max_stdout_length"`
}
```

**機密パターン検出と編集**:
```go
// 場所: internal/runner/security/logging_security.go:49-52
func (v *Validator) redactSensitivePatterns(text string) string {
    sensitivePatterns := []struct {
        pattern     string
        replacement string
    }{
        // APIキー、トークン、パスワード（一般的なパターン）
        {"password=", "password=[REDACTED]"},
        {"token=", "token=[REDACTED]"},
        {"key=", "key=[REDACTED]"},
        {"secret=", "secret=[REDACTED]"},
        {"api_key=", "api_key=[REDACTED]"},

        // 機密を含む可能性のある環境変数代入
        {"_PASSWORD=", "_PASSWORD=[REDACTED]"},
        {"_TOKEN=", "_TOKEN=[REDACTED]"},
        {"_KEY=", "_KEY=[REDACTED]"},
        {"_SECRET=", "_SECRET=[REDACTED]"},

        // 一般的な認証情報パターン
        {"Bearer ", "Bearer [REDACTED]"},
        {"Basic ", "Basic [REDACTED]"},
    }
    // パターンマッチングと置換ロジック
}
```

**エラーメッセージのサニタイズ**:
```go
// 場所: internal/runner/security/logging_security.go:4-26
func (v *Validator) SanitizeErrorForLogging(err error) string {
    if err == nil {
        return ""
    }

    errMsg := err.Error()

    // エラー詳細を含めるべきでない場合、汎用メッセージを返す
    if !v.config.LoggingOptions.IncludeErrorDetails {
        return "[error details redacted for security]"
    }

    // 有効化されている場合、機密情報を編集
    if v.config.LoggingOptions.RedactSensitiveInfo {
        errMsg = v.redactSensitivePatterns(errMsg)
    }

    // 長すぎる場合は切り詰め
    if len(errMsg) > v.config.LoggingOptions.MaxErrorMessageLength {
        errMsg = errMsg[:v.config.LoggingOptions.MaxErrorMessageLength] + "...[truncated]"
    }

    return errMsg
}
```

**出力のサニタイズ**:
- 認証情報漏洩を防ぐコマンド出力のサニタイズ
- 設定可能な出力長の切り詰め
- 機密情報の自動パターンベース編集
- key=value形式と認証ヘッダーパターンの両方をサポート

**セーフログ関数**:
- `CreateSafeLogFields()`: サニタイズされたログフィールドマップを作成
- `LogFieldsWithError()`: ベースフィールドとサニタイズされたエラー情報を結合
- 構造化ログでの機密パターンの自動検出と編集

#### セキュリティ保証

##### 現在提供されている保証
- RedactingHandlerによる全ログ出力での自動編集
- 一般的な機密パターン（パスワード、トークン、APIキー）の検出と編集
- 異なるセキュリティ環境に対応する設定可能なログ詳細レベル
- エラーメッセージとコマンド出力による認証情報露出からの保護
- ログファイルの肥大化と潜在的DoSを防ぐ長さベースの切り詰め
- 環境変数パターンの検出とサニタイズ
- key=value形式と認証ヘッダーパターン（Bearer、Basic）の両方をサポート

##### task 0055実装後に追加される保証
- CommandResult作成時点での事前編集による二重防御
- Slack通知での明示的な編集処理による外部通信の追加保護層
- 第1層で編集漏れがあっても第2層（RedactingHandler）でキャッチ

### 9. 端末能力検出 (`internal/terminal/`)

#### 目的
端末の色彩サポートと対話的実行環境を検出し、適切な出力形式を選択するための端末能力判定機能を提供します。

#### 実装詳細

**端末能力検出インターフェース**:
```go
// 場所: internal/terminal/capabilities.go
type Capabilities interface {
    IsInteractive() bool
    SupportsColor() bool
    HasExplicitUserPreference() bool
}
```

**対話的環境検出**:
```go
// 場所: internal/terminal/detector.go
type InteractiveDetector interface {
    IsInteractive() bool
    IsTerminal() bool // TTY環境または端末類似環境をチェック
    IsCIEnvironment() bool
}
```

**実装機能**:
- **CI/CD環境検出**: GitHub Actions、Travis CI、Jenkins等の自動判定
- **TTY検出**: stdout/stderrのTTY接続状況確認
- **端末環境ヒューリスティック**: TERM環境変数による端末類似環境判定
- **色彩サポート検出**: TERM値に基づく色彩対応端末識別
- **ユーザー設定優先順位**: コマンドライン引数、環境変数の優先順位制御

#### セキュリティ特性
- **保守的なデフォルト**: 不明な端末では色彩出力を無効化
- **環境変数検証**: CI環境変数の適切な解析
- **設定の優先順位制御**: セキュリティに配慮した設定継承

### 10. 色彩管理 (`internal/color/`)

#### 目的
端末の色彩サポート能力に基づいて安全な色付き出力を提供し、色彩制御シーケンスの適切な管理を行います。

#### 実装詳細

**色彩管理インターフェース**:
```go
// 場所: internal/color/color.go
type ColorManager interface {
    Enable() bool
    Colorize(text string, color ColorCode) string
}
```

**色彩サポート検出**:
```go
// 場所: internal/terminal/color.go
type ColorDetector interface {
    SupportsColor() bool
}
```

**実装機能**:
- **既知端末パターンマッチング**: xterm、screen、tmux等の色彩対応端末識別
- **保守的なフォールバック**: 不明な端末での色彩出力無効化
- **TERM環境変数解析**: 端末タイプに基づく色彩サポート判定
- **ユーザー設定統合**: 端末能力とユーザー設定の優先順位制御

#### セキュリティ特性
- **保守的なアプローチ**: 不明な端末では色彩出力を無効化してエスケープシーケンス出力を防止
- **検証済みパターン**: 既知の色彩対応端末のみでの色彩有効化
- **安全なデフォルト**: 色彩サポートが不明な場合の安全な動作保証

### 11. 共通ユーティリティ (`internal/common/`, `internal/cmdcommon/`)

#### 目的
パッケージ横断の基盤機能を提供し、テスト可能で再現性のある安全な実装を保証します。

#### 実装詳細

**ファイルシステム抽象**:
```go
// 場所: internal/common/filesystem.go
type FileSystem interface {
    CreateTempDir(dir string, prefix string) (string, error)
    FileExists(path string) (bool, error)
    Lstat(path string) (fs.FileInfo, error)
    IsDir(path string) (bool, error)
}
```

**モック実装**:
- テスト用のモックファイルシステムを提供し、本番と同等のセキュリティ特性でテスト可能にする
- エラー条件や境界ケースのテストをサポート

#### セキュリティ保証
- 実装間での一貫したセキュリティ挙動
- セキュリティパスの包括的なテストカバレッジ
- 型安全なインターフェース契約
- モック実装はセキュリティプロパティを保持

### 12. ユーザーとグループ実行セキュリティ

#### 目的
厳格なセキュリティ境界と包括的な監査証跡を維持しながら、安全なユーザーとグループ切り替え機能を提供します。

#### 実装詳細

**ユーザー・グループ設定**:
```go
// 場所: internal/runner/runnertypes/config.go
type Command struct {
    RunAsUser    string `toml:"run_as_user"`    // コマンドを実行するユーザー
    RunAsGroup   string `toml:"run_as_group"`   // コマンドを実行するグループ
    RiskLevel    string `toml:"risk_level"`     // コマンドの最大リスクレベル
}
```

**グループメンバーシップ検証**:
```go
// 場所: internal/groupmembership/membership.go
type GroupMembershipChecker interface {
    IsUserInGroup(username, groupname string) (bool, error)
    GetGroupMembers(groupname string) ([]string, error)
}
```

**セキュリティ検証フロー**:
1. ユーザー存在と権限の検証
2. グループが指定されている場合のグループメンバーシップ確認
3. 特権昇格要件のチェック
4. リスクベース制限の適用
5. 適切な特権でのコマンド実行

#### セキュリティ保証
- 包括的ユーザーとグループ検証
- 特権昇格境界執行
- グループメンバーシップ確認
- ユーザー・グループ切り替えの完全監査証跡

### 13. マルチチャンネル通知セキュリティ

#### 目的
外部通信で機密情報を保護しながら、重要なセキュリティイベントに対する安全な通知機能を提供します。

#### 実装詳細

**Slack統合**:
```go
// 場所: internal/logging/slack_handler.go
type SlackHandler struct {
    webhookURL string
    redactor   *redaction.Redactor
}
```

**安全な通知処理**:
- 送信前の機密データ自動編集
- 設定可能な通知チャンネル
- レート制限とエラー処理
- 安全なWebhook URL管理

#### セキュリティ保証
- 外部通知での機密データ保護
- 安全な通信チャンネル管理
- 悪用を防ぐレート制限
- 包括的エラー処理

### 14. コマンド実行環境の分離

#### 目的
子プロセスが予期しない入力を読み取ることを防ぎ、実行環境を明示的に制御することで、セキュリティと安定性を向上させます。

#### 実装詳細

**標準入力の無効化**:
```go
// 場所: internal/runner/executor/executor.go:210-224
// Set up stdin to null device to prevent issues with commands that expect stdin
// This prevents "exit status 255" errors from docker-compose exec and similar commands
// that try to allocate a pseudo-TTY when stdin is nil (file descriptor -1)
devNull, err := os.Open(os.DevNull)
if err != nil {
    return nil, fmt.Errorf("failed to open null device for stdin: %w", err)
}
defer func() {
    if closeErr := devNull.Close(); closeErr != nil {
        e.Logger.Warn("Failed to close null device", "error", closeErr)
    }
}()
execCmd.Stdin = devNull
```

**セキュリティ上の利点**:
- 子プロセスがstdinから予期しない入力を読み取ることを防止
- 対話型プロンプトによる処理の停止を防止
- バッチ処理環境における一貫した動作を保証
- 悪意のある入力注入攻撃のリスクを軽減

**安定性の向上**:
- stdinがnilの場合に疑似TTYを割り当てようとするコマンド（docker-compose execなど）のエラーを防止
- プラットフォーム間での一貫した動作（`os.DevNull`を使用）

#### セキュリティ保証
- すべての子プロセスでstdin入力を明示的に無効化
- 予期しない入力による処理の停止や改ざんを防止
- クロスプラットフォーム対応（Linuxでは`/dev/null`、Windowsでは`NUL`）

### 15. 出力サイズ制限によるリソース保護

#### 目的
コマンド出力サイズを制限することで、メモリ枯渇攻撃やディスク容量の枯渇を防ぎ、システムの安定性とセキュリティを確保します。

#### 実装詳細

**階層的な出力サイズ制限**:
```go
// 場所: internal/common/output_size_limit.go
func ResolveOutputSizeLimit(commandLimit OutputSizeLimit, globalLimit OutputSizeLimit) OutputSizeLimit {
    // 1. コマンドレベルのoutput_size_limit（設定されている場合）
    // 2. グローバルレベルのoutput_size_limit（設定されている場合）
    // 3. デフォルト出力サイズ制限（10MB）
}
```

**デフォルト設定**:
```go
// 場所: internal/common/output_size_limit_type.go:20-21
// DefaultOutputSizeLimit is the default output size limit when not specified (10MB)
const DefaultOutputSizeLimit = 10 * 1024 * 1024
```

**制限の適用**:
- 場所: `internal/runner/output/capture.go`
- カスタムサイズ制限ライターによる出力サイズの制限
- 書き込み前のサイズチェックにより制限超過を防止
- 制限超過時のエラー検出と報告
- コマンド単位での柔軟な制限設定

**設定階層**:
1. **コマンドレベル**: 個別コマンドごとに`output_size_limit`を設定可能
2. **グローバルレベル**: すべてのコマンドに適用される既定値
3. **デフォルト**: 10MB（設定がない場合）
4. **無制限**: 値を0に設定することで制限を無効化可能（注意が必要）

#### セキュリティ保証
- メモリ枯渇攻撃（DoS）からの保護
- 過大な出力によるディスク容量枯渇の防止
- 出力サイズ制限超過時の明確なエラーメッセージ
- コマンド単位での柔軟な制限設定によるきめ細かな制御

### 16. 設定セキュリティ

#### 目的
設定ファイルと全体的なシステム設定が改ざんされないことを確保し、セキュリティのベストプラクティスに従います。

#### 実装詳細

**ファイル権限検証**:
```go
// 場所: internal/runner/security/file_validation.go:44-75
func (v *Validator) ValidateFilePermissions(filePath string) error {
    // ワールド書き込み可能ファイルをチェック
    disallowedBits := perm &^ requiredPerms
    if disallowedBits != 0 {
        return ErrInvalidFilePermissions
    }
    return nil
}
```

**ハッシュディレクトリセキュリティ強化（コマンドライン引数削除）**:
```go
// 場所: cmd/runner/main.go (変更後)
func getHashDir() string {
    // プロダクション環境では常にデフォルトディレクトリのみ使用
    // --hash-directoryフラグは完全削除（セキュリティ脆弱性対策）
    return cmdcommon.DefaultHashDirectory
}
```

**設定ファイル事前検証**:
```go
// 場所: cmd/runner/main.go (変更後)
// 設定ファイル読み込み前にハッシュ検証を実行
if err := verificationManager.VerifyConfigFile(configPath); err != nil {
    // 未検証データによるシステム動作を完全排除
    return &logging.PreExecutionError{
        Type:      logging.ErrorTypeConfigValidation,
        Message:   fmt.Sprintf("Configuration file verification failed: %s", err),
        Component: "config",
        RunID:     runID,
    }
}
```

**早期パス検証**:
```go
// 場所: cmd/runner/main.go:188-199
hashDir := getHashDir()
if !filepath.IsAbs(hashDir) {
    return &logging.PreExecutionError{
        Type:      logging.ErrorTypeFileAccess,
        Message:   fmt.Sprintf("Hash directory must be absolute path, got relative path: %s", hashDir),
        Component: "file",
        RunID:     runID,
    }
}
```

**ディレクトリセキュリティ検証**:
- ルートからターゲットまでの完全パストラバーサル
- パスコンポーネントでのシンボリックリンク検出
- ワールド書き込み可能ディレクトリ検出
- グループ書き込み制限（ルート所有権が必要）

**設定検証タイミングの改善**:
- 設定ファイル読み込み前のハッシュ検証実行
- 未検証データによるシステム動作の完全排除
- 検証失敗時の強制stderr出力（ログレベル設定に依存しない）

**ハッシュディレクトリ設定のセキュリティ強化**:
- `--hash-directory`コマンドライン引数の完全削除
- プロダクション環境では常にデフォルトディレクトリのみ使用
- カスタムハッシュディレクトリによる攻撃経路の完全排除
- テスト環境専用APIによるテスタビリティ維持

**設定整合性**:
- TOML形式検証
- 必須フィールド検証
- 型安全性の強制
- 重複グループ名検出と環境変数継承分析

#### セキュリティ保証
- 設定改ざんの防止
- 安全なファイルとディレクトリ権限
- パストラバーサル攻撃の防止
- 設定形式検証
- 設定ファイル事前検証による改ざん検出
- ハッシュディレクトリ攻撃経路の完全排除
- 絶対パス要求による早期検証強化

## セキュリティアーキテクチャパターン

### 多層防御

システムは複数のセキュリティレイヤを実装します：

1. **入力検証**: すべての入力がエントリポイントで検証（絶対パス要求を含む）
2. **事前検証**: 設定ファイルの使用前ハッシュ検証
3. **パスセキュリティ**: 包括的なパス検証とシンボリックリンク保護、セキュア固定PATH使用
4. **ファイル整合性**: すべての重要ファイル（設定、実行ファイル）のハッシュベース検証
5. **特権制御**: 制御された昇格による最小特権原則
6. **環境分離**: 厳格な許可リストベースの環境フィルタリング、PATH継承の排除
7. **コマンド検証**: 許可リスト検証を伴うリスクベースコマンド実行制御
8. **データ保護**: RedactingHandlerによる全ログ出力での機密情報の自動編集（task 0055でCommandResult作成時の事前編集を追加し多層防御を強化予定）
9. **ユーザー・グループセキュリティ**: メンバーシップ検証を伴う安全なユーザー・グループ切り替え
10. **ハッシュディレクトリセキュリティ**: カスタムハッシュディレクトリ攻撃の完全防止
11. **実行環境分離**: stdin無効化による予期しない入力の防止
12. **リソース保護**: 出力サイズ制限によるメモリ・ディスク枯渇攻撃の防止

### ゼロトラストモデル

- システム環境への暗黙の信頼なし
- すべてのファイルは使用前に検証
- 環境変数は許可リストでフィルタリング
- コマンドは既知の良好なパターンに対して検証
- 特権は必要時のみ付与され、即座に取り消し

### フェイルセーフ設計

- すべての操作でデフォルト拒否
- セキュリティ障害時の緊急シャットダウン
- 包括的エラー処理とログ
- セキュリティ機能が利用できない場合の優雅な劣化

### 監査と監視

- セキュリティコンテキストでの構造化ログ
- 特権操作メトリクスと追跡
- セキュリティイベント記録
- 重大エラーのマルチチャンネル報告

## 脅威モデルと対策

### ファイルシステム攻撃

**脅威**:
- シンボリックリンク攻撃
- パストラバーサル
- TOCTOU競合状態
- ファイル改ざん
- 悪意のある設定ファイルによるシステム動作操作
- カスタムハッシュディレクトリによる検証迂回

**対策**:
- RESOLVE_NO_SYMLINKSでのopenat2
- ステップバイステップパス検証
- SHA-256ハッシュ検証
- 原子的ファイル操作
- 設定ファイルの事前ハッシュ検証
- ハッシュディレクトリのデフォルト値固定（カスタム指定完全禁止）

### 特権昇格

**脅威**:
- 不正な特権取得
- 特権の永続化
- 特権処理での競合状態

**対策**:
- 制御された特権昇格
- 自動特権復元
- スレッドセーフ操作
- 失敗時の緊急シャットダウン

### 環境操作

**脅威**:
- 環境変数によるコマンドインジェクション
- 環境による情報漏洩
- LD_PRELOADなどによる特権昇格

**対策**:
- 厳格な許可リストベースフィルタリング
- 危険パターン検出
- グループレベル環境分離
- 変数名と値の検証

### コマンドインジェクション

**脅威**:
- 任意のコマンド実行
- シェルメタ文字の悪用
- PATH操作
- コマンド操作による特権昇格
- 環境変数PATHを通じた悪意のあるバイナリ実行
- stdin経由での予期しない入力注入

**対策**:
- 許可リスト執行を伴うリスクベースコマンド検証
- セキュリティ検証を伴う完全パス解決
- シェルメタ文字検出
- コマンドパス検証
- リスクレベル執行とブロック
- ユーザー・グループ実行検証
- 環境変数PATH継承の完全排除
- セキュア固定PATH（/sbin:/usr/sbin:/bin:/usr/bin）の強制使用
- stdin無効化による入力注入攻撃の防止

### リソース枯渇攻撃

**脅威**:
- メモリ枯渇によるDoS攻撃
- 過大な出力によるディスク容量枯渇
- ログファイルの肥大化
- 長時間実行コマンドによるシステムリソースの独占

**対策**:
- 出力サイズ制限（デフォルト10MB、設定可能）
- タイムアウト設定による長時間実行の防止
- ログ切り詰め設定（MaxStdoutLength、MaxErrorMessageLength）
- 階層的な制限設定（グローバル、グループ、コマンドレベル）
- リソース使用量の監視とアラート

## パフォーマンス考慮事項

### ハッシュ計算
- 効率的なストリーミングハッシュ計算
- リソース枯渇を防ぐファイルサイズ制限

### 環境処理
- マップ構造を使用したO(1)許可リスト検索
- パターンマッチングのためのコンパイル済み正規表現
- 最小限の文字列操作

### 特権操作
- グローバルmutexが競合状態を防ぐが特権操作を直列化
- システムコールを使用した高速特権昇格/復元
- パフォーマンス監視のためのメトリクス収集

### リスク評価
- 効率的コマンド分析のための事前コンパイル正規表現パターン
- 事前コンパイルパターンを使用したO(1)リスクレベル検索
- リスク評価の最小オーバーヘッド
- 繰り返しコマンド分析の結果キャッシュ

### データ編集
- RedactingHandlerによるログ出力時の編集（現在実装済み）
- 機密データの事前コンパイルパターン
- 通常操作への最小パフォーマンス影響
- 設定可能な編集ポリシー
- CommandResult作成時の事前編集追加による多層防御（task 0055で実装予定）

### リソース管理
- 出力サイズ制限によるメモリ使用量の制御
- カスタムサイズ制限ライターによる効率的な制限実装
- 書き込み前のサイズチェックによる制限超過の防止
- コマンド単位での柔軟な制限設定
- 制限超過時の早期検出とエラー報告

## デプロイメントセキュリティ

### バイナリ配布
- 特権昇格のためにバイナリにsetuidビットを設定する必要
- setuid機能にはルート所有権が必要
- デプロイメント前にバイナリ整合性を検証すべき

### 設定管理
- ハッシュディレクトリは安全な権限（755以下）を持つ必要
- 設定ファイルは書き込み保護すべき
- 重要ファイルの定期的整合性検証

### 監視とアラート
- セキュリティイベントの構造化ログ
- 集中ログのためのsyslog統合
- 緊急シャットダウンイベントは即座の注意が必要
- リアルタイムセキュリティアラートのSlack統合
- 全監視チャンネルでの自動機密データ編集

## 既知のセキュリティ制限

### TOCTOU (Time-of-Check to Time-of-Use) 競合状態

#### 脆弱性の概要

コマンドパス検証（`ValidateCommandAllowed`）と実際のコマンド実行の間に、理論的なTOCTOU競合状態が存在します。ファイルシステムへの書き込み権限を持つ攻撃者は、これらの操作の間にシンボリックリンクのターゲットを置き換える可能性があります。

**脆弱性の場所**:
```go
// 場所: internal/runner/security/validator.go:255-295
func (v *Validator) ValidateCommandAllowed(cmdPath string, ...) error {
    // 1. シンボリックリンクを解決して検証（Check）
    resolvedCmd, err := filepath.EvalSymlinks(cmdPath)
    // パターンマッチング検証...
}

// 場所: internal/runner/group_executor.go:396-412
// 検証後、実際の実行までの間にTOCTOUウィンドウが存在
if err := ge.validator.ValidateCommandAllowed(...); err != nil {
    return nil, fmt.Errorf("command not allowed: %w", err)
}
// ... (ここで攻撃者がシンボリックリンクを変更可能)
token, resourceResult, err := ge.resourceManager.ExecuteCommand(...) // Use
```

#### 攻撃の要件

この脆弱性を悪用するには、攻撃者は以下を満たす必要があります：
1. ファイルシステムへの書き込み権限
2. 検証と実行の間の正確なタイミング
3. シンボリックリンクを配置・変更する能力

#### 緩和策

以下の多層防御メカニズムにより、攻撃の実現可能性と影響を大幅に軽減しています：

**1. ファイル整合性検証**:
- すべての実行ファイルはSHA-256ハッシュ検証により実行前に検証されます
- ハッシュ検証システムが改ざんされたバイナリの検出と実行防止を提供
- 場所: `internal/filevalidator/`, `internal/verification/`

**2. セキュリティモデル境界**:
- システムのセキュリティモデルは、ファイルシステムへの書き込み権限を持つ攻撃者を信頼境界外と定義
- 適切に設定されたシステムでは、実行ファイルディレクトリへの書き込み権限は制限されるべき

**3. デプロイメント推奨事項**:
高セキュリティ環境では、以下の追加対策を推奨：
- 実行ファイルディレクトリを読み取り専用ファイルシステムとしてマウント
- `nosymfollow`マウントオプションの使用（利用可能な場合）
- 厳格なファイルシステム権限の実施
- 定期的なファイル整合性監視

#### 技術的背景

**完全な対策の実現困難性**:
Goの標準`os/exec`パッケージは、TOCTOU攻撃を完全に防ぐ`fexecve()`システムコールをサポートしていません。完全な対策には以下が必要：
1. CGOを使用した低レベルシステムコール実装
2. ファイルディスクリプタベースの実行フロー
3. プラットフォーム固有のコード（Linux `fexecve()`、Windows代替）

このような実装は、以下の理由から現実的ではありません：
- 大幅なアーキテクチャ変更が必要
- プラットフォーム互換性の複雑化
- 保守性の低下
- 既存の多層防御で十分な保護を提供

#### 影響評価

**リスクレベル**: 低〜中
- **実現可能性**: 低（厳格な要件、正確なタイミング必要）
- **影響**: 中（ファイル整合性検証により制限）
- **検出可能性**: 高（監査ログ、ファイル整合性監視）

#### 参考資料

- [Safe programming. How to avoid TOCTOU vulnerability](https://stackoverflow.com/questions/41069166/)
- [CERT C Coding Standard: POS35-C](https://wiki.sei.cmu.edu/confluence/display/c/POS35-C.+Avoid+race+conditions+while+checking+for+the+existence+of+a+symbolic+link)
- [Wikipedia: Symlink race](https://en.wikipedia.org/wiki/Symlink_race)
- [Star Lab Software: Linux Symbolic Links Security](https://www.starlab.io/blog/linux-symbolic-links-convenient-useful-and-a-whole-lot-of-trouble)

## 結論

Go Safe Command Runnerは、特権委譲による安全なコマンド実行のための包括的なセキュリティフレームワークを提供します。多層アプローチは、最新のセキュリティプリミティブ（openat2）と実証済みのセキュリティ原則（多層防御、ゼロトラスト、フェイルセーフ設計）を組み合わせて、セキュリティを重視する環境での本番使用に適した堅牢なシステムを作成します。

実装は、包括的な入力検証、リスクベースコマンド制御、安全な特権管理、自動機密データ保護、広範な監査機能を含むセキュリティエンジニアリングのベストプラクティスを実証しています。システムは安全に失敗し、セキュリティ関連操作への完全な可視性を提供するよう設計されています。

主要なセキュリティ革新機能には、以下が含まれます：
- コマンド実行のためのインテリジェントリスク評価
- 一貫したセキュリティ境界を持つ統一リソース管理
- RedactingHandlerによる全ログ出力での自動機密データ編集（task 0055でCommandResult作成時の事前編集を追加し多層防御による二重保護を実現予定）
- 安全なユーザー・グループ実行機能
- セキュリティ対応メッセージングを伴う包括的マルチチャンネル通知
- stdin無効化による実行環境の明示的制御
- 出力サイズ制限によるリソース枯渇攻撃の防止

システムは、運用の柔軟性と透明性を維持しながら、エンタープライズグレードのセキュリティ制御を提供します。最近の改善により、実行環境の分離とリソース保護が強化され、より包括的なセキュリティ対策が実現されています。task 0055の実装により、機密データ保護がさらに強化される予定です。
