# Go Safe Command Runner - セキュリティアーキテクチャ技術文書

## 概要

本文書は、Go Safe Command Runnerプロジェクトに実装されたセキュリティ対策の包括的な技術解析を提供します。システムの設計原則、実装詳細、およびセキュリティ保証について理解が必要なソフトウェアエンジニアやセキュリティ専門家を対象としています。

## 要約

Go Safe Command Runnerは、特権操作の安全な委譲と自動化されたバッチ処理を可能にするため、複数層のセキュリティ制御を実装しています。セキュリティモデルは多層防御の原則に基づいて構築され、ファイル整合性検証、環境変数分離、特権管理、および安全なファイル操作を組み合わせています。

## 主要なセキュリティ機能

### 1. ファイル整合性検証

#### 目的
実行前に実行ファイルや重要なファイルが改ざんされていないことを確認し、侵害されたバイナリの実行を防止します。

#### 実装詳細

**ハッシュアルゴリズム**: SHA-256暗号化ハッシュ
- 場所: `internal/filevalidator/hash_algo.go`
- Go標準の`crypto/sha256`ライブラリを使用
- 強力な衝突耐性のための256ビットハッシュ値を提供

**ハッシュストレージシステム**:
- ハッシュファイルは専用ディレクトリにJSONマニフェストとして保存
- 特殊文字を処理するためBase64 URL-safe encodingを使用してファイルパスをエンコード
- マニフェスト形式にはファイルパス、ハッシュ値、アルゴリズム、タイムスタンプが含まれる
- 衝突検出により、異なるファイルが同じハッシュファイルを共有することを防止

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

**特権ファイルアクセス**:
- 権限により通常の検証が失敗した場合、特権昇格にフォールバック
- 安全な特権管理を使用（特権管理セクション参照）
- 場所: `internal/filevalidator/privileged_file.go`

#### セキュリティ保証
- 実行ファイルと設定ファイルの不正な変更を検出
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
// 場所: internal/runner/security/security.go:639-649
func (v *Validator) ValidateEnvironmentValue(key, value string) error {
    // コンパイルされた正規表現を使用して危険なパターンをチェック
    for _, re := range v.dangerousEnvRegexps {
        if re.MatchString(value) {
            return fmt.Errorf("%w: environment variable %s contains potentially dangerous pattern",
                ErrUnsafeEnvironmentVar, key)
        }
    }
    return nil
}
```

**危険パターン検出**:
- コマンド区切り文字: `;`, `|`, `&&`, `||`
- コマンド置換: `$(`, バッククォート
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
- 一貫した動作のため`io.LimitReader`を使用

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
func (m *UnixPrivilegeManager) WithPrivileges(elevationCtx runnertypes.ElevationContext, fn func() error) error {
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
設定可能な許可リストに対してコマンドパスを検証し、危険なバイナリの実行を防ぐことで、認可されたコマンドのみが実行できることを確保します。

#### 実装詳細

**パス解決**:
```go
// 場所: internal/verification/path_resolver.go
type PathResolver struct {
    pathEnv            string
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
// 場所: internal/runner/security/security.go:128-135
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

### 6. セキュアログと機密データ保護

#### 目的
パスワード、APIキー、トークンなどの機密情報がログファイルに露出することを防ぎ、機密データを侵害することなく安全な監査証跡を提供します。

#### 実装詳細

**ログセキュリティ設定**:
```go
// 場所: internal/runner/security/security.go:85-101
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
// 場所: internal/runner/security/security.go:500-531
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
// 場所: internal/runner/security/security.go:455-479
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
- 一般的な機密パターン（パスワード、トークン、APIキー）の自動編集
- 異なるセキュリティ環境に対応する設定可能なログ詳細レベル
- エラーメッセージとコマンド出力による認証情報露出からの保護
- ログファイルの肥大化と潜在的DoSを防ぐ長さベースの切り詰め
- 環境変数パターンの検出とサニタイズ

### 7. 設定セキュリティ

#### 目的
設定ファイルと全体的なシステム設定が改ざんされないことを確保し、セキュリティのベストプラクティスに従います。

#### 実装詳細

**ファイル権限検証**:
```go
// 場所: internal/runner/security/security.go:345-383
func (v *Validator) ValidateFilePermissions(filePath string) error {
    // ワールド書き込み可能ファイルをチェック
    disallowedBits := perm &^ requiredPerms
    if disallowedBits != 0 {
        return ErrInvalidFilePermissions
    }
    return nil
}
```

**ディレクトリセキュリティ検証**:
- ルートからターゲットまでの完全パストラバーサル
- パスコンポーネントでのシンボリックリンク検出
- ワールド書き込み可能ディレクトリ検出
- グループ書き込み制限（ルート所有権が必要）

**設定整合性**:
- TOML形式検証
- 必須フィールド検証
- 型安全性の強制
- セクション間のクロスリファレンス検証

#### セキュリティ保証
- 設定改ざんの防止
- 安全なファイルとディレクトリ権限
- パストラバーサル攻撃の防止
- 設定形式検証

## セキュリティアーキテクチャパターン

### 多層防御

システムは複数のセキュリティレイヤを実装します：

1. **入力検証**: すべての入力がエントリポイントで検証
2. **パスセキュリティ**: 包括的なパス検証とシンボリックリンク保護
3. **ファイル整合性**: すべての重要ファイルのハッシュベース検証
4. **特権制御**: 制御された昇格による最小特権原則
5. **環境分離**: 厳格な許可リストベースの環境フィルタリング
6. **コマンド検証**: 許可リストベースのコマンド実行制御

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

**対策**:
- RESOLVE_NO_SYMLINKSでのopenat2
- ステップバイステップパス検証
- SHA-256ハッシュ検証
- 原子的ファイル操作

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

**対策**:
- 許可リストベースのコマンド検証
- 完全パス解決
- シェルメタ文字検出
- コマンドパス検証

## パフォーマンス考慮事項

### ハッシュ計算
- 効率的なストリーミングハッシュ計算
- リソース枯渇を防ぐファイルサイズ制限
- 繰り返し検証のためのキャッシュメカニズム

### 環境処理
- マップ構造を使用したO(1)許可リスト検索
- パターンマッチングのためのコンパイル済み正規表現
- 最小限の文字列操作

### 特権操作
- グローバルmutexが競合状態を防ぐが特権操作を直列化
- システムコールを使用した高速特権昇格/復元
- パフォーマンス監視のためのメトリクス収集

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

## 結論

Go Safe Command Runnerは、特権委譲による安全なコマンド実行のための包括的なセキュリティフレームワークを提供します。多層アプローチは、最新のセキュリティプリミティブ（openat2）と実証済みのセキュリティ原則（多層防御、ゼロトラスト、フェイルセーフ設計）を組み合わせて、セキュリティを重視する環境での本番使用に適した堅牢なシステムを作成します。

実装は、包括的な入力検証、安全な特権管理、広範な監査機能を含むセキュリティエンジニアリングのベストプラクティスを実証しています。システムは安全に失敗し、セキュリティ関連操作への完全な可視性を提供するよう設計されています。
