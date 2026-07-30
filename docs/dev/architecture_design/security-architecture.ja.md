# Go Safe Command Runner - セキュリティアーキテクチャ技術文書

## 概要

本文書は、Go Safe Command Runnerプロジェクトに実装されたセキュリティ対策の包括的な技術解析を提供します。システムの設計原則、実装詳細、およびセキュリティ保証について理解が必要なソフトウェアエンジニアやセキュリティ専門家を対象としています。

## 要約

Go Safe Command Runnerは、特権操作の安全な委譲と自動化されたバッチ処理を可能にするため、複数層のセキュリティ制御を実装しています。セキュリティモデルは多層防御の原則に基づいて構築され、ファイル整合性検証、ELFバイナリ静的解析、環境変数分離、特権管理、および安全なファイル操作を組み合わせています。

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
// 場所: internal/filevalidator/validator.go の Verify() メソッド
func (v *Validator) Verify(filePath string) error {
    // 1. ファイルパスの検証と解決
    targetPath, err := validatePath(filePath)

    // 2. 現在のファイルハッシュを計算
    actualHash, err := v.calculateHash(targetPath.String())

    // 3. verifyHash() が解析レコードのハッシュと比較
    return v.verifyHash(targetPath, actualHash)
}
```

`verifyHash()` は `internal/fileanalysis` の解析レコード（ハッシュマニフェスト）を `v.store.Load()` で読み取り、記録済みのファイルパスとの一致を確認（ハッシュ衝突検出）した上で、`アルゴリズム:ハッシュ値` 形式の期待値を記録済みの `ContentHash` と比較し、不一致の場合は `ErrMismatch` を返します。

**record 時点の読み取り安全性チェック**:

`calculateHash()`（`internal/filevalidator/validator.go`）はファイルを `v.fileSystem.SafeOpenFile()` 経由で開きます。この呼び出しは内部で `internal/groupmembership` の読み取り安全性チェック（グループ書き込み可能なファイルについて、権限チェックの基準UID（`record` は基準UID決定方針として `SudoUIDAware` を宣言しているため、実UIDが0かつ`SUDO_UID`が有効な値であればその値を、それ以外は実UIDを採用する）がそのグループに属しているかを確認するもの）を通過して初めて成功し、通過できなければ失敗します。

この読み取り安全性チェックが必要なのは、`verify`/`runner` が後で行う一致確認が「`record` 時点のハッシュと現在のハッシュが一致するか」しか見ないためです。仮に `record` がこのチェックを経ずにファイルを読み取ったとします。そのファイルが属するグループの信頼できないメンバーが、その時点ですでに内容を書き換えていた場合、`record` はその改ざん済みの内容をハッシュ化して「正しい基準」として記録してしまいます。以後 `verify`/`runner` は改ざん済みの内容と記録済みハッシュが一致し続けるため、この改ざんを検出できません。すなわちこのチェックは、`record` が信頼の基準点（`ContentHash`）を確定させる、まさにその瞬間にのみ有効な防御であり、実行時の再検証では代替できません（§2 の解析結果キャッシュについても同様の理由が当てはまります）。

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

### 2. ELFバイナリ静的解析とインタープリタ追跡

#### 目的
`record` コマンド実行時に ELF および Mach-O バイナリを静的解析し、危険なシステムコールパターン、ネットワーク機能の使用、動的ライブラリ依存関係、スクリプトインタープリタを記録します。runner は記録済みデータを用いて動的ライブラリの整合性を検証し、実行時の ELF 再解析を不要にします。

ここでの責務分担は次のとおりです。`record` はバイナリの静的解析結果を runner が利用しやすい正規化済み特徴量として保存します。たとえばネットワーク関連シンボル解析では `networkSymbols` を用いて保存対象のシンボル名集合を絞り込みますが、これは保存形式の正規化であり、実行可否や `risk_level` の最終判断ではありません。`runner` は保存済みの `detected_symbols`、`dynamic_load_symbols`、`known_network_lib_deps` を読み取り、必要に応じてカテゴリを再導出して実行時のリスク判定を行います。

#### 実装詳細

**record コマンドでの解析フロー** (`cmd/record/main.go`):

```go
// filevalidator.ValidatorConfig 構造体リテラルで各解析コンポーネントを注入する
vCfg := filevalidator.ValidatorConfig{
    BinaryAnalyzer:    security.NewBinaryAnalyzer(runtime.GOOS),      // ネットワークシンボル検出（socket, connect, bind など）
    SyscallAnalyzer:   libccache.NewSyscallAdapter(syscallAnalyzer),  // syscall パターン解析（x86_64 / arm64 対応）
    LibcCache:         libccache.NewCacheAdapter(cacheMgr, syscallAnalyzer),        // Linux: libc syscall ラッパーシンボルキャッシュ
    LibSystemCache:    libccache.NewMachoLibSystemAdapter(machoCacheMgr, fs),       // macOS: libSystem syscall シンボルキャッシュ
    MachoSyscallTable: libccache.MacOSSyscallTable{},
    DebugInfo:         debugInfo,
}
// 動的ライブラリ依存関係の再帰解析（条件を満たす場合のみ設定）
vCfg.ELFDynLibAnalyzer = d.elfDynlibAnalyzerFactory()
vCfg.MachODynLibAnalyzer = d.machoDynlibAnalyzerFactory()

validator, _ := d.validatorFactory(cfg.hashDir, vCfg)
```

**解析内容**:
- **syscall 解析** (internal/security/elfanalyzer/): x86_64 と arm64 の両アーキテクチャに対応。SYSCALL 命令 (0F 05) / SVC #0 を列挙し、逆方向スキャンで syscall 番号を特定。mprotect/pkey_mprotect + PROT_EXEC の組み合わせ（JIT コード実行相当）を危険パターンとして検出。exec 関連 syscall（Linux: execve/execveat、macOS: execve/__mac_execve）を検出し、実行時に高リスク判定へマッピングする。Go ラッパー呼び出し（syscall.Syscall 等）も Pass 2 で解析（pclntab 解析のため Go 1.18 以降が必要）。分岐先収束（Branch Convergence）解析によりレジスタコピーチェーンを追跡し、条件分岐を跨ぐ syscall 番号の特定にも対応。キャッシュミス（ErrNoSyscallAnalysis 等）時はライブ解析へフォールバックするが、スキーマ不一致（SchemaVersionMismatchError）時は再記録が必要。
- **ネットワーク機能検出** (internal/security/binaryanalyzer/, internal/security/elfanalyzer/): socket, connect, bind 等のシンボル名を networkSymbols で正規化し、runner が後続で参照する detected_symbols / dynamic_load_symbols を生成。未定義シンボル (SHN_UNDEF) がない ELF の場合、StaticBinary ではなく NoNetworkSymbols を返却。
- **動的ライブラリ依存解析** (`internal/dynlib/elfdynlib/`, `internal/dynlib/machodylib/`): ELF の DT_NEEDED / Mach-O の LC_LOAD_DYLIB を再帰解析し、すべての依存ライブラリのパスとハッシュを記録
- **libc syscall キャッシュ** (`internal/libccache/`): Linux では libc の syscall ラッパーシンボルをキャッシュし、macOS では libSystem の syscall シンボルをキャッシュすることで、間接的な syscall 呼び出しを解析
- **shebang 追跡** (`internal/shebang/`): `#!/bin/sh`（直接形式）/ `#!/usr/bin/env python3`（env 形式）等のインタープリタパスを解析・記録

**解析結果の永続化** (`internal/fileanalysis/`):

```
fileanalysis.Record（SchemaVersion = fileanalysis.CurrentSchemaVersion、現在 23）
  ├── ContentHash           // ファイルの SHA-256 ハッシュ
  ├── DynLibDeps            // 依存ライブラリのパスとハッシュ一覧（[]LibEntry）
  ├── SyscallAnalysis       // syscall 解析結果
  │     ├── DetectedSyscalls   // 検出された syscall 一覧（番号・名称・出現箇所・判定方法）
  │     ├── AnalysisWarnings   // 解析警告（mprotect+PROT_EXEC 検出など）
  │     ├── ArgEvalResults     // syscall 引数評価結果（mprotect の PROT_EXEC フラグ判定）
  │     └── DeterminationStats // syscall 番号判定方法の診断統計
  ├── SymbolAnalysis        // ネットワークシンボル解析結果
  ├── ShebangChain          // shebang チェーンのインタープリタ情報（[]ShebangChainEntry、スクリプトの場合）
  └── AnalysisWarnings      // dynlib 解析の非致命的警告
```

**runner 実行時の検証** (`internal/verification/manager.go` の `verifyDynLibDeps()` メソッド):

`verifyDynLibDeps()` は概ね次の流れで動作します。
1. 記録済みレコードを `LoadRecord()` で読み取る。`ErrRecordNotFound` や、`record` 作成時点で dynlib 未対応だったことを示す `SchemaVersionMismatchError` などのエラーを個別にハンドリングする。
2. `DynLibDeps` が記録済みであれば、ハッシュ検証後にさらに `verifyDynLibDepsResolution()` を呼び出し、検索パスのシャドーイングによる差し替えがないかも確認する（検証済みハッシュはキャッシュされる）。
3. ELF だけでなく Mach-O バイナリについても `hasMachODynamicLibraryDeps()` で動的ライブラリ依存の有無を確認する。
4. 動的リンクバイナリで `DynLibDeps` が未記録の場合は、`dynlib.ErrDynLibDepsRequired` を返して再 `record` を要求する。

DynLibDeps が記録済みのバイナリに対しては、実行時に ELF を再解析せず、記録済みのハッシュ一覧を照合することで検証コストを最適化しています。

同様に、ネットワーク機能判定でも runner は ELF を再解析せず、`record` が保存した `detected_symbols` と `known_network_lib_deps` を読み込み、シンボル名からカテゴリを再導出してポリシー判定を行います。

#### セキュリティ保証
- 動的ライブラリの改ざん検出（依存ライブラリのハッシュ照合）
- 動的リンクバイナリの依存関係が未記録の場合は実行前に再 record を要求
- 危険な syscall パターン（mprotect+PROT_EXEC）の事前検出と警告
- exec 関連 syscall の事前検出と高リスクへの自動分類
- ネットワーク機能を持つバイナリの識別と可視化
- スクリプトインタープリタの改ざん検出（shebang 追跡）
- libc 経由の間接 syscall 呼び出しの解析対応（libccache）

### 3. 環境変数分離

#### 目的
環境変数の厳格な許可リストベースのフィルタリングを実装し、環境操作による情報漏洩やコマンドインジェクション攻撃を防止します。

#### 実装詳細

**許可リストの実際の適用箇所**:
- `config.ProcessEnvImport`（`internal/runner/config/expansion.go`）: `env_import` 宣言に対する許可リスト照合を設定展開時に行う
- `executor.BuildProcessEnvironment`（`internal/runner/base/executor/environment.go`）: 子プロセス環境の組み立て時に許可リストで絞り込みを行う
- `internal/runner/base/environment` パッケージはシステム環境の列挙（`system_env.go`）と denylist 判定（`denylist.go`）を提供し、許可リストは扱わない

**3レベル継承モデル**:

1. **グローバル許可リスト**: すべてのグループで利用可能な基本環境変数
2. **グループオーバーライド**: グループが独自の許可リストを定義し、グローバル設定を完全にオーバーライド
3. **継承制御**: 明示的な許可リストを持たないグループはグローバル設定を継承

**継承モード**:
- `InheritanceModeInherit`: グローバル許可リストを使用
- `InheritanceModeExplicit`: グループ固有の許可リストのみを使用
- `InheritanceModeReject`: 環境変数を許可しない（空の許可リスト）

**変数値の安全性検証**:
```go
// 場所: internal/runner/base/security/environment_validation.go
// コマンドはシェルを介さず直接実行されるため、; | $( > < 等のシェルメタ文字は
// インジェクションリスクがなく、検証対象外とする。
func (v *Validator) ValidateEnvironmentValue(key, value string) error {
    if strings.ContainsRune(value, '\x00') {
        return fmt.Errorf("%w: environment variable %s contains null byte",
            ErrUnsafeEnvironmentVar, key)
    }
    if strings.ContainsAny(value, "\n\r") {
        return fmt.Errorf("%w: environment variable %s contains newline or carriage return character",
            ErrUnsafeEnvironmentVar, key)
    }
    return nil
}
```

**変数値の検証対象**:
コマンドはシェルを介さず直接実行されるため、シェルメタ文字（`;`、`|`、`$(...)`、`>`、`<` 等）はインジェクションリスクを持たず検証対象外とする。以下の文字のみを拒否する：
- ヌルバイト: `\x00`（ヘッダーインジェクションや構造化出力の破壊に悪用可能）
- 改行文字: `\n`、`\r`（同上）

#### セキュリティ保証
- ゼロトラスト環境変数モデル（許可リストのみ）
- 環境ベースのコマンドインジェクションを防止
- 機密変数のグループレベル分離
- 変数名の検証（POSIX 準拠・予約プレフィックスチェック）
- ヌルバイト・改行文字による変数値の検証（コマンドはシェル経由ではなく直接実行されるため、シェルメタ文字は検証対象外）

### 4. 安全なファイル操作

#### 目的
シンボリックリンク攻撃、TOCTOU（Time-of-Check-Time-of-Use）競合状態、パストラバーサル攻撃を防ぐため、シンボリックリンクに対して安全なファイルI/O操作を提供します。

#### 実装詳細

**最新Linuxセキュリティ（openat2）**:
```go
// 場所: internal/safefileio/safe_file_linux.go（Linux 専用ビルドファイル）の openat2() 関数
func openat2(dirfd int, pathname string, how *openHow) (int, error) {
    // RESOLVE_NO_SYMLINKSフラグを使用してシンボリックリンクの追跡を原子的に防止
    pathBytes, err := syscall.BytePtrFromString(pathname)
    fd, _, errno := syscall.Syscall6(SysOpenat2, ...)
    return int(fd), nil
}
```

**フォールバックセキュリティ（従来システム）**:
```go
// 場所: internal/safefileio/safe_file.go の ensureParentDirsNoSymlinks() 関数
func ensureParentDirsNoSymlinks(absPath string) error {
    // ルートからターゲットまでのステップバイステップパス検証
    for _, component := range components {
        fi, err := os.Lstat(currentPath) // シンボリックリンクを追跡しない
        if fi.Mode()&os.ModeSymlink != 0 {
            // OS が管理する既知のシンボリックリンク（例: /etc/mtab 等）は例外的に許可し、
            // EvalSymlinks で解決した上で検証を継続する
            if !common.IsAllowedOSManagedSymlink(currentPath) {
                return fmt.Errorf("%w: %s", ErrIsSymlink, currentPath)
            }
            resolved, err := filepath.EvalSymlinks(currentPath)
            // 以降、resolved を使って検証を継続
        }
    }
    return nil
}
```

シンボリックリンクは原則として拒否しますが、OS がインストール時から管理する既知のシンボリックリンク（`common.IsAllowedOSManagedSymlink()` で判定）のみ例外的に許可し、解決先を検証対象として扱います。

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
- 最新Linux上でのシンボリックリンクに対する原子的な安全操作（openat2）
- 包括的パストラバーサル保護
- TOCTOU競合状態の排除
- メモリ枯渇攻撃に対する保護
- 安全なファイルタイプ検証

### 5. 特権管理

#### 目的
最小特権の原則を維持しながら、特定の操作に対して制御された特権昇格を可能にします。あわせて、包括的な監査証跡と復元後の二重の防御的検証を提供します。

#### 実装詳細

**Unix特権アーキテクチャ**:
```go
// 場所: internal/runner/base/privilege/unix.go
type UnixPrivilegeManager struct {
    logger             *slog.Logger
    originalUID        int
    privilegeSupported bool
    metrics            Metrics
    mu                 sync.Mutex  // 競合状態を防止
    osExit             func(code int)                      // テスト用に注入可能なos.Exit
    identityVerifier   func() error                         // EUID==UID / EGID==GID の検証（テスト用に注入可能）
    readSavedIDs       func() (suid, sgid int, err error)   // saved-set-uid/gidの読み取り（テスト用に注入可能）
}
```

**特権昇格プロセス**:
`WithPrivileges`は、実行前の準備・昇格・後始末の3段階に分かれています。

```go
// 場所: internal/runner/base/privilege/unix.go
func (m *UnixPrivilegeManager) WithPrivileges(elevationCtx runnertypes.ElevationContext, fn func() error) (err error) {
    m.mu.Lock()  // スレッドセーフティのためのグローバルロック
    defer m.mu.Unlock()

    // 1. saved-set-uid/gidを記録し、operationの種別から昇格要否を決定
    execCtx, err := m.prepareExecution(elevationCtx)
    if err != nil {
        return err
    }

    // 2. 昇格が必要なoperationのみsyscall.Seteuid(0)を実行
    if err := m.performElevation(execCtx); err != nil {
        return err
    }

    // 3. deferで復元・検証・メトリクス記録をまとめて実行
    defer m.handleCleanupAndMetrics(execCtx)
    return fn()
}
```

昇格要否は`elevationCtx.Operation`によって決まり、`OperationUserGroupExecution`と`OperationFileValidation`のみ昇格します。それ以外の操作種別が渡された場合、`prepareExecution`は`ErrUnsupportedOperationType`エラーを返し、`WithPrivileges`はそのエラーをそのまま呼び出し元に返します（`fn()`は呼び出されません）。

**実行モード**:

1. **ネイティブルート実行**: ルートユーザー（UID 0）として実行
   - 特権昇格は不要
   - 完全な特権での直接実行

2. **setuidバイナリ実行**: setuidビット設定とルート所有権を持つバイナリ
   - 特権昇格に`syscall.Seteuid(0)`を使用
   - 操作後の自動特権復元

**復元後の防御的検証**:
`handleCleanupAndMetrics`はパニック回復と時間計測を担い、実際の特権復元と2段階の不変条件チェックは内部で呼び出す`restorePrivilegesAndMetrics`が行います。いずれかのチェックが失敗すると`emergencyShutdown`が呼ばれます。

```go
// 場所: internal/runner/base/privilege/unix.go の restorePrivilegesAndMetrics() 関数
// 1. EUID==UID / EGID==GID を検証する（復元処理自体のバグを検出する独立したチェック）
if err := m.identityVerifier(); err != nil {
    m.emergencyShutdown(err, fmt.Sprintf("identity_verification_failure_%s", shutdownContext))
}

// 2. saved-set-uid/gidが復元前後で変化していないことを検証する
//    （非対応プラットフォームではoriginalSUID < 0のガードにより構造的にスキップされる）
if execCtx.originalSUID >= 0 {
    suid, sgid, err := m.getReadSavedIDs()()
    if err != nil {
        m.emergencyShutdown(fmt.Errorf("failed to read saved-set IDs after restore: %w", err),
            fmt.Sprintf("saved_set_read_failure_%s", shutdownContext))
    }
    if suid != execCtx.originalSUID || sgid != execCtx.originalSGID {
        err := fmt.Errorf("saved-set-uid/gid changed after restore: "+
            "original suid=%d, sgid=%d; post-restore suid=%d, sgid=%d: %w",
            execCtx.originalSUID, execCtx.originalSGID, suid, sgid, ErrIdentityLeak)
        m.emergencyShutdown(err, fmt.Sprintf("saved_set_identity_verification_failure_%s", shutdownContext))
    }
}
```

saved-set-uid/gidの確認は、EUID一致確認よりも強い不変条件です。EUIDが正しく復元されていても、部分的な`seteuid`呼び出しでsaved-setが以前の実効UIDのまま壊れているケースを検出できます。

**セキュリティ検証**:
```go
// 場所: internal/runner/base/privilege/unix.go
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
- 特権復元失敗、またはEUID/EGID一致確認・saved-set-uid/gid不変条件チェックのいずれかの失敗時に即座のプロセス終了
- 構造化ログとstderrの両方への記録
- 完全なコンテキストでのセキュリティイベント記録
- 侵害された状態での継続実行防止

#### セキュリティ保証
- グローバルmutexによるスレッドセーフな特権操作
- パニック保護付きの自動特権復元
- 復元後のEUID/EGID一致検証とsaved-set-uid/gid不変条件チェックによる二重防御
- すべての特権操作の包括的監査ログ
- セキュリティ障害時の緊急シャットダウン
- ネイティブルートとsetuidバイナリ実行モデルの両方をサポート
- dry-run など昇格を要しない操作では特権を一切取得しない

### 6. コマンドパス検証

#### 目的
設定可能な許可リストに対してコマンドパスを検証し、危険なバイナリの実行を防ぐことで、認可されたコマンドのみが実行できることを確保します。環境変数の継承を停止し、セキュアな固定PATHを使用します。

#### 実装詳細

**セキュアPATH環境の強制**:
```go
// 場所: internal/common/secure_path.go
// common.SecurePathEnv = "/sbin:/usr/sbin:/bin:/usr/bin:" + CoreutilsDir

// 環境変数PATHを継承せず、セキュアな固定PATHを使用
pathResolver := NewPathResolver(common.SecurePathEnv)
```

**パス解決**:
```go
// 場所: internal/verification/path_resolver.go
type PathResolver struct {
    pathEnv string          // セキュア固定PATH使用
    cache   map[string]string
    mu      sync.RWMutex
}
```

**コマンド検証プロセス**:
1. PATH環境変数を使用してコマンドを完全なパスに解決
2. 許可リストパターン（正規表現ベース）に対して検証
3. 危険な特権コマンドをチェック
4. ハッシュが利用可能な場合はファイル整合性を検証

**デフォルト許可パターン**:
```go
// 場所: internal/runner/base/security/types.go
// DefaultConfig() が GenerateAllowedCommandsFromPath() を呼び出し、
// common.SecurePathEnv（セキュア固定PATH）からデフォルトの許可パターンを動的に生成する
allowedCommands, err := GenerateAllowedCommandsFromPath(common.SecurePathEnv)
```

デフォルトの許可コマンドパターンは固定リストではなく、セキュア固定PATHのディレクトリ一覧から実行時に動的生成されます（生成に失敗した場合は panic）。

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

### 7. リスクベースコマンド制御

#### 目的
コマンドリスク評価に基づくインテリジェントなセキュリティ制御を実装し、高リスク操作を自動的にブロックしながら安全なコマンドの正常実行を可能にします。

#### 実装詳細

**リスク評価エンジン**:
```go
// 場所: internal/runner/base/risk/evaluator.go
// StandardEvaluator は networkAnalyzer（ネットワークシンボル解析）、openIdentity
// （fd束縛実行のための検証済み同一性オープナー）、zoning、resolveRunAs
// （実行ユーザー解決）などのフィールドを持つ
type StandardEvaluator struct {
    networkAnalyzer *security.NetworkAnalyzer
    openIdentity    identityOpener
    zoning          *zoningParams
    resolveRunAs    runAsResolver
}

// EvaluateRisk は素の RiskLevel ではなく VerifiedCommandPlan を返す。評価した同一性と
// 実行する同一性を束縛し、executor は plan（検証済みの argv/env/identity）のみを実行し、
// 素の argv/env は決して実行しないようにするためである。実効リスクは、該当する全次元
// （プロファイル・破壊・システム変更・危険引数パターン・任意コード実行ランナー・バイナリ
// 解析）の最大値であり、フェイルクローズのゲートは最大値を取る前に短絡する。
func (e *StandardEvaluator) EvaluateRisk(cmd *runnertypes.RuntimeCommand) (risktypes.VerifiedCommandPlan, error) {
    // まず絶対パスであることを検証する（相対パスは即座に拒否）
    if !filepath.IsAbs(cmdPath) {
        return blockingPlan(...), nil
    }
    // 次に identity ゲート: 検証済みハッシュがない、またはバイナリ解析が無効の場合、
    // バイナリの同一性を確認できないため、設定された risk_level によらず拒否（Blocking）する。
    // これは全リスク次元より前に実行され、未検証バイナリに対していずれの経路でも
    // Low/High 許容のリスクを確定させないようにする。
    if blocked, ok := e.identityGate(cmd); ok {
        return blockingPlan(blocked), nil
    }
    // 間接実行の解決（ラッパー・インラインシェル・ローダ）はそれ自体で拒否や
    // Critical を強制し得る。次に特権昇格（sudo/su/doas）-> Critical（常にブロック）。
    // 最後に coreutils 分類、プロファイル要因、危険引数パターン、任意コード実行
    // ランナー、バイナリ解析を、次元の最大値へ合流させる。
}
```

**間接実行リゾルバ（ラッパー経由インナー）**: ラッパー（`env`/`timeout`/`nice` 等）はインナーコマンドを抽出してリスク評価する（抽出は維持する）。特権昇格トークンは Critical、禁止形態（ローダ制御変数およびインタプリタ起動時コード注入変数・`env -C`・解釈不能な `env -S`・find/xargs の子プロセス実行・動的ローダ直接起動・remote-shell ヘルパ・抽出不能ラッパー・深さ上限超過・symlink 解決失敗）は **Blocking 拒否**（`IndirectReject`）を優先し、それ以外の抽出可能な通常インナーは内容によらず一律 High（細粒度算出は行わない）。runner はラッパーセマンティクスを再実装せず、インナーの fd 束縛・自動ハッシュ検証も行わない（インナーは監査のために実行計画へ記録するが実体固定ではない。タスク 0138）。直接スクリプト実行の shebang インタプリタ連鎖のみ従来どおり細粒度算出を維持する。

**コマンドリスク分析**:
- 低リスク: 標準システムユーティリティ（ls、cat、grep）
- 中リスク: ファイル変更コマンド（cp、mv、chmod）、その他のシステム変更（mount、crontab）
- 高リスク: パッケージ管理（apt、yum、dpkg）、サービス/システム管理（systemctl、service）、破壊的操作（rm -rf）
- クリティカルリスク: 特権昇格コマンド（sudo、su）- 自動的にブロック

**リスクレベル設定**:
```go
// 場所: internal/runner/base/runnertypes/spec.go
type CommandSpec struct {
    RiskLevel *string `toml:"risk_level"` // コマンドのリスクレベル（未設定時は nil）
}

// GetRiskLevel() は RiskLevel が nil の場合 RiskLevelLow をデフォルトとして返す
func (c *CommandSpec) GetRiskLevel() (RiskLevel, error)
```

#### セキュリティ保証
- 特権昇格試行の自動ブロック
- コマンド毎の設定可能リスク閾値
- 包括的コマンドパターンマッチング
- リスクベース監査ログ

### 8. リソース管理セキュリティ

#### 目的
通常実行とdry-runモードの両方でセキュリティ境界を維持する安全なリソース管理を提供します。

#### 実装詳細

**統一リソースインターフェース**:
```go
// 場所: internal/runner/resource/manager.go
// インターフェース名は Manager（ResourceManager ではない）
type Manager interface {
    ExecuteCommand(ctx context.Context, cmd *runnertypes.RuntimeCommand, group *runnertypes.GroupSpec, env map[string]string) (CommandToken, *ExecutionResult, error)
    WithPrivileges(ctx context.Context, fn func() error) error
    SendNotification(message string, details map[string]any) error
    // 他、ValidateOutputPath / CreateTempDir / CleanupTempDir / CleanupAllTempDirs /
    // GetDryRunResults など、出力パス検証・一時ディレクトリ管理・dry-run結果取得のための
    // メソッドを追加で持つ
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

### 9. セキュアログと機密データ保護

#### 目的
パスワード、APIキー、トークンなどの機密情報がログファイルに露出することを防ぎ、機密データを侵害することなく安全な監査証跡を提供します。専用の編集サービスで強化され、多層防御アプローチにより包括的な保護を実現します。

#### 実装詳細

**一元化データ編集基盤**:
```go
// 場所: internal/redaction/redactor.go
type Config struct {
    Placeholder      string  // LogPlaceholder / TextPlaceholder は統一され単一フィールドに
    Patterns         *SensitivePatterns
    KeyValuePatterns []string
    ValueDetector    *ValueDetector // AWSキー・GitHubトークン・PEM形式などの値ベース検出
}

func (c *Config) RedactText(text string) string {
    // 設定されたすべての編集パターンを適用
}

func (c *Config) RedactLogAttribute(attr slog.Attr) slog.Attr {
    // ログ属性の機密情報を編集
}
```

**二層防御アーキテクチャ**:

機密データ保護は、一方の層に漏れが生じても他方がキャッチする二重防御で実装されています。

**第1層：CommandResult作成時の編集**（`internal/runner/group_executor.go`）:
```go
// 場所: internal/runner/group_executor.go の CommandResult 生成処理
// コマンド出力を CommandResult に格納する前に機密情報を編集
sanitizedStdout := ge.validator.SanitizeOutputForLogging(stdout)
sanitizedStderr := ge.validator.SanitizeOutputForLogging(stderr)
```
- `SanitizeOutputForLogging()` は `internal/runner/base/security/logging_security.go` に実装
- コマンド出力を格納する時点で機密情報を編集し、Slack 通知等への流出を防止

**第2層：RedactingHandlerでの編集**（`internal/redaction/redactor.go`）:
```go
// 場所: internal/redaction/redactor.go の RedactingHandler 型
type RedactingHandler struct {
    handler       slog.Handler
    config        *Config
    failureLogger *slog.Logger // 編集処理自体がパニックした場合の再帰防止用ロガー
}

// 場所: internal/runner/bootstrap/logger.go
// NewRedactingHandler は failureLogger を含む3引数を取り、WithErrorCollector で
// エラーコレクタをチェインする
redactedHandler := redaction.NewRedactingHandler(multiHandler, nil, failureLogger).WithErrorCollector(collector)
logger := slog.New(redactedHandler)
```
- ログ出力時に自動的に機密情報を編集
- すべてのログハンドラー（ファイル、syslog、Slack）をラップ
- `slog.KindGroup`を含む構造化ログの再帰的処理
- key=value形式と認証ヘッダーパターンの両方をサポート

**Slack通知実装**:
```go
// 場所: internal/logging/slack_handler.go の SlackHandler 型
type SlackHandler struct {
    webhookURL    string
    runID         string
    httpClient    *http.Client
    level         slog.Level
    attrs         []slog.Attr
    groups        []string
    backoffConfig BackoffConfig
    isDryRun      bool                  // dry-run実行時はSlack通知を送信しない
    levelMode     SlackHandlerLevelMode // ログレベルによる通知要否の制御モード
}
```
- RedactingHandlerによってラップされているため、第2層の編集が適用される
- 第1層（CommandResult作成時）の編集により、コマンド出力は格納前に編集済み
- コマンド出力の長さ制限（stdout: 1000文字、stderr: 500文字）

**ログセキュリティ設定**:
```go
// 場所: internal/runner/base/security/types.go の LoggingOptions 型
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
// 場所: internal/runner/base/security/logging_security.go の redactSensitivePatterns() メソッド
// パターン定義とマッチング・置換ロジックは internal/redaction パッケージへ一元化されており、
// このメソッドは単純に委譲するだけになっている
func (v *Validator) redactSensitivePatterns(text string) string {
    return v.redactionConfig.RedactText(text)
}
```

実際のパスワード・トークン・APIキー等のパターン定義（`password=`、`token=`、`key=`、`secret=`、`api_key=`、`_PASSWORD=` 等の環境変数代入、`Bearer `/`Basic ` 認証ヘッダーなど）は `internal/redaction/redactor.go` の `SensitivePatterns`／`Config.RedactText()` に集約されています（「9. セキュアログと機密データ保護」の一元化データ編集基盤を参照）。

**エラーメッセージのサニタイズ**:
```go
// 場所: internal/runner/base/security/logging_security.go の SanitizeErrorForLogging() 関数
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

    // 長さ制限が設定されており、かつ長すぎる場合のみ切り詰め
    if v.config.LoggingOptions.MaxErrorMessageLength > 0 && len(errMsg) > v.config.LoggingOptions.MaxErrorMessageLength {
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
- CommandResult作成時（第1層）とRedactingHandler（第2層）による二重防御
- 第1層で編集漏れがあっても第2層（RedactingHandler）でキャッチ
- 一般的な機密パターン（パスワード、トークン、APIキー）の検出と編集
- 異なるセキュリティ環境に対応する設定可能なログ詳細レベル
- エラーメッセージとコマンド出力による認証情報露出からの保護
- ログファイルの肥大化と潜在的DoSを防ぐ長さベースの切り詰め
- 環境変数パターンの検出とサニタイズ
- key=value形式と認証ヘッダーパターン（Bearer、Basic）の両方をサポート

### 10. 端末能力検出 (`internal/terminal/`)

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

### 11. 色彩管理 (`internal/ansicolor/`)

#### 目的
端末の色彩サポート能力に基づいて安全な色付き出力を提供し、色彩制御シーケンスの適切な管理を行います。

#### 実装詳細

**色彩関数型**:
```go
// 場所: internal/ansicolor/color.go
// Color は ANSI エスケープシーケンスでテキストをラップする関数型
type Color func(text string) string

// NewColor は指定した ANSI コードで色彩関数を生成する
func NewColor(ansiCode string) Color {
    return func(text string) string {
        return ansiCode + text + resetCode
    }
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

### 12. 共通ユーティリティ (`internal/common/`, `internal/cmdcommon/`)

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
    TempDir() string
    RemoveAll(path string) error
    Remove(path string) error
    CreateTemp(dir, pattern string) (*os.File, error)
    MkdirAll(path string, perm fs.FileMode) error
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

### 13. ユーザーとグループ実行セキュリティ

#### 目的
厳格なセキュリティ境界と包括的な監査証跡を維持しながら、安全なユーザーとグループ切り替え機能を提供します。

#### 実装詳細

**ユーザー・グループ設定**:
```go
// 場所: internal/runner/base/runnertypes/spec.go
type CommandSpec struct {
    RunAsUser    string  `toml:"run_as_user"`    // コマンドを実行するユーザー
    RunAsGroup   string  `toml:"run_as_group"`   // コマンドを実行するグループ
    RiskLevel    *string `toml:"risk_level"`     // コマンドのリスクレベル（未設定時は nil）
}
```

**グループメンバーシップ検証**:
```go
// 場所: internal/groupmembership/manager.go
// インターフェースではなく具象構造体。ユーザー名/グループ名ではなく uid/gid（数値）を受け取る
type GroupMembership struct{ /* ... */ }

func New(opts ...Option) *GroupMembership

func (gm *GroupMembership) IsUserInGroup(uid, gid uint32) (bool, error)
func (gm *GroupMembership) GetGroupMembers(gid uint32) ([]string, error)
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

### 14. マルチチャンネル通知セキュリティ

#### 目的
外部通信で機密情報を保護しながら、重要なセキュリティイベントに対する安全な通知機能を提供します。

#### 実装詳細

**Slack統合**:
```go
// 場所: internal/logging/slack_handler.go
type SlackHandler struct {
    webhookURL    string
    runID         string
    httpClient    *http.Client
    level         slog.Level
    attrs         []slog.Attr
    groups        []string
    backoffConfig BackoffConfig
    isDryRun      bool                  // dry-run実行時はSlack通知を送信しない
    levelMode     SlackHandlerLevelMode // ログレベルによる通知要否の制御モード
}
```

**安全な通知処理**:
- RedactingHandlerによるラップで機密データを自動編集（第2層）
- CommandResult格納時点での事前編集により、コマンド出力は通知前に編集済み（第1層）
- 設定可能な通知チャンネル
- レート制限とエラー処理
- 安全なWebhook URL管理

#### セキュリティ保証
- 外部通知での機密データ保護（二層防御）
- 安全な通信チャンネル管理
- 悪用を防ぐレート制限
- 包括的エラー処理

### 15. コマンド実行環境の分離

#### 目的
子プロセスが予期しない入力を読み取ることを防ぎ、実行環境を明示的に制御することで、セキュリティと安定性を向上させます。

#### 実装詳細

**標準入力の無効化**:
```go
// 場所: internal/runner/base/executor/executor.go の標準入力セットアップ処理
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

### 16. 出力サイズ制限によるリソース保護

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
// 場所: internal/common/output_size_limit_type.go の DefaultOutputSizeLimit 定数
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

### 17. 設定セキュリティ

#### 目的
設定ファイルと全体的なシステム設定が改ざんされないことを確保し、セキュリティのベストプラクティスに従います。

#### 実装詳細

**ファイル権限検証**:
```go
// 場所: internal/runner/base/security/file_validation.go の ValidateFilePermissions() 関数
// （実際にはこの他、通常ファイルタイプの検証や構造化ログ（slog.Debug/Warn）出力も行う）
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
- `--hash-directory`フラグは完全削除されており、`getHashDir()`のようなラッパー関数も存在しない
- `cmd/runner/main.go`は`cmdcommon.DefaultHashDirectory`を各使用箇所で直接参照する（プロダクション環境では常にデフォルトディレクトリのみ使用）

**設定ファイル事前検証**:

`cmd/runner/main.go` の `main()` は、設定ファイルの読み込みに先立って `bootstrap.LoadAndPrepareConfig(verificationManager, configPath, runID)` を呼び出す。この関数は内部で `verificationManager.VerifyAndReadConfigFile(configPath)` を呼び、ハッシュ検証とファイル読み込みを一度の読み取りでアトミックに行うことで TOCTOU 攻撃を防止する。

```go
// 場所: internal/runner/bootstrap/config.go の LoadAndPrepareConfig() 関数
// 設定ファイルの検証と読み込みをアトミックに実行し、TOCTOU 攻撃を防止する
content, err := verificationManager.VerifyAndReadConfigFile(configPath)
if err != nil {
    return nil, &logging.PreExecutionError{
        Type:      logging.ErrorTypeFileAccess,
        Message:   err.Error(),
        Component: string(resource.ComponentVerification),
        RunID:     runID,
    }
}

// 検証済みの content を用いて設定を読み込む
cfg, err := cfgLoader.LoadConfig(configPath, content)
```

なお、`verificationManager.VerifyGlobalFiles()` は設定ファイル自体ではなく、設定ファイルの読み込み・展開後に判明する `verify_files` などのグローバルファイル群を対象として、`main()` から別途呼び出される。

**早期パス検証**:
```go
// 場所: cmd/runner/main.go の main() 関数
if !filepath.IsAbs(cmdcommon.DefaultHashDirectory) {
    logging.HandlePreExecutionError(logging.ErrorTypeBuildConfig,
        fmt.Sprintf("Hash directory must be absolute path, got relative path: %s", cmdcommon.DefaultHashDirectory),
        "file", runID)
    os.Exit(1)
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
2. **ELFバイナリ静的解析**: record コマンドによる危険 syscall・ネットワーク機能の事前検出、動的ライブラリ依存関係の追跡とハッシュ検証
3. **事前検証**: 設定ファイルの使用前ハッシュ検証
4. **パスセキュリティ**: 包括的なパス検証とシンボリックリンク保護、セキュア固定PATH使用
5. **ファイル整合性**: すべての重要ファイル（設定、実行ファイル、依存ライブラリ）のハッシュベース検証
6. **特権制御**: 制御された昇格による最小特権原則
7. **環境分離**: 厳格な許可リストベースの環境フィルタリング、PATH継承の排除
8. **コマンド検証**: 許可リスト検証を伴うリスクベースコマンド実行制御
9. **データ保護**: CommandResult作成時（第1層）とRedactingHandler（第2層）による機密情報の二重防御編集
10. **ユーザー・グループセキュリティ**: メンバーシップ検証を伴う安全なユーザー・グループ切り替え
11. **ハッシュディレクトリセキュリティ**: カスタムハッシュディレクトリ攻撃の完全防止
12. **実行環境分離**: stdin無効化による予期しない入力の防止
13. **リソース保護**: 出力サイズ制限によるメモリ・ディスク枯渇攻撃の防止

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
- バイナリ解析・ファイル検証が利用できない場合は実行を拒否する（優雅な劣化ではなくフェイルクローズ）。同一性を確認できないバイナリは決して実行しない。dry-run のプレビューは引き続き利用可能。

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

### 危険なバイナリ実行

**脅威**:
- mprotect+PROT_EXEC を使用した動的コード実行（JIT コードインジェクション相当）
- exec 関連 syscall（execve 系）によるプロセス置換・実行
- ネットワーク通信機能を持つバイナリの予期しない外部通信
- 動的ライブラリ（.so / dylib）の置き換えによる動作改ざん
- スクリプトインタープリタの改ざんによる任意コード実行

**対策**:
- record コマンドによる ELF 静的解析と危険 syscall パターンの事前検出
- exec 関連 syscall の事前検出と実行時ポリシーでの高リスク昇格
- ネットワークシンボル解析による通信能力の可視化
- 動的ライブラリ依存関係のハッシュ記録と実行前照合
- shebang インタープリタのハッシュ記録と実行前検証
- 動的リンクバイナリで DynLibDeps 未記録の場合は実行前に再 record を要求

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
- 動的ローダ制御変数（`LD_PRELOAD`、`LD_LIBRARY_PATH`、`DYLD_INSERT_LIBRARIES`、`GLIBC_TUNABLES` 等）およびインタプリタ起動時コード注入変数（`BASH_ENV`、`PYTHONPATH`、`NODE_OPTIONS` 等）による特権昇格

**対策**:
- 厳格な許可リストベースフィルタリング
- グループレベル環境分離
- 変数名の検証（POSIX 準拠）
- ヌルバイト・改行文字に対する変数値の検証（コマンドはシェル経由で実行されないため、シェルメタ文字によるインジェクション不成立）

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
- CommandResult作成時（第1層）とRedactingHandler（第2層）による二層防御
- 機密データの事前コンパイルパターン
- 通常操作への最小パフォーマンス影響
- 設定可能な編集ポリシー

### ELFバイナリ解析
- record コマンド実行時のみ解析（runner 実行時は記録済みデータを参照）
- DynLibDeps 記録済みの場合: 実行時の ELF 再解析不要（記録済みハッシュ一覧の照合のみ）
- libc syscall ラッパーのキャッシュによる重複解析の回避

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

#### fd束縛実行（fexecve相当）による解消

コマンドパス検証（`ValidateCommandAllowed`）と実際のコマンド実行の間のTOCTOU競合状態は、主要な実行経路については**fd束縛実行により構造的に解消されています**。以下、現在の実装が採用している方式を説明します。

パスの解決は実行フロー全体を通じて一度だけ行われます。`internal/runner/group_executor.go`の`verifyGroupFiles`が`verificationManager.ResolvePath()`でコマンドパスをシンボリックリンク解決した絶対パスに解決し、`cmd.ExpandedCmd`にその解決済みパスを固定します。以降、実行直前の`executeCommandInGroup`では再解決を行いません。実行直前の再解決は検証と実行の間にTOCTOU再解決の窓を生むため、意図的に避けられています。

`internal/runner/base/security/validator.go`の`ValidateCommandAllowed`は、この時点で渡されるパスがすでに`PathResolver.ResolvePath()`により解決済みであることを前提としており、`filepath.EvalSymlinks`は呼び出しません（許可リストの正規表現照合のみを行います）。

その後、`internal/runner/base/risk/evaluator.go`の`openVerifiedIdentity`が解決済みパスを`O_RDONLY|O_CLOEXEC`で一度だけオープンし、そのファイルディスクリプタの内容をハッシュ再計算して検証時のハッシュと照合します（ファイルディスクリプタ経由でのTOCTOU安全な同一性検証）。得られたファイルディスクリプタは`internal/runner/base/executor/fdexec_linux.go`の`fdExecExtraFile`で子プロセスのfd 3として複製され、`/proc/self/fd/3`を実行対象として`exec`します。つまりカーネルは検証済みのinodeそのものを実行するため、検証後にパス文字列側でシンボリックリンクやファイルを差し替えても実行対象には影響しません。

#### 残存する既知の制限（shebangインタープリタ）

上記のfd束縛実行は、直接実行されるコマンドバイナリの経路を対象としています。一方、スクリプトのshebang行が指す**インタープリタ**については、`verifyInterpreterSymlinkTarget`が検証時点でシンボリックリンクの解決先を確認するものの、実際にスクリプトを実行する際にインタープリタパスを再解決するのはカーネル自身であり、この窓をアプリケーション側のGoコードから閉じることはできません（インタープリタ自体をfd束縛するには`execveat`相当の仕組みが必要で、現時点ではスコープ外としています）。

この残存ギャップを悪用するには、攻撃者が検証と実際のスクリプト実行の間の正確なタイミングでインタープリタのシンボリックリンクを差し替えられる、ファイルシステム書き込み権限が必要です。適切に権限を制限した環境ではこの前提自体が成立しません。

#### 参考資料

- [Safe programming. How to avoid TOCTOU vulnerability](https://stackoverflow.com/questions/41069166/)
- [CERT C Coding Standard: POS35-C](https://wiki.sei.cmu.edu/confluence/display/c/POS35-C.+Avoid+race+conditions+while+checking+for+the+existence+of+a+symbolic+link)
- [Wikipedia: Symlink race](https://en.wikipedia.org/wiki/Symlink_race)
- [Star Lab Software: Linux Symbolic Links Security](https://www.starlab.io/blog/linux-symbolic-links-convenient-useful-and-a-whole-lot-of-trouble)
- 関連タスク: `docs/tasks/0090_toctou_fexecve/`, `docs/tasks/0155_toctou_verify_use_residual_gaps/`

## 結論

Go Safe Command Runnerは、特権委譲による安全なコマンド実行のための包括的なセキュリティフレームワークを提供します。多層アプローチは、最新のセキュリティプリミティブ（openat2）と実証済みのセキュリティ原則（多層防御、ゼロトラスト、フェイルセーフ設計）を組み合わせて、セキュリティを重視する環境での本番使用に適した堅牢なシステムを作成します。

実装は、包括的な入力検証、ELFバイナリ静的解析、リスクベースコマンド制御、安全な特権管理、自動機密データ保護、広範な監査機能を含むセキュリティエンジニアリングのベストプラクティスを実証しています。システムは安全に失敗し、セキュリティ関連操作への完全な可視性を提供するよう設計されています。

主要なセキュリティ機能には、以下が含まれます：
- record コマンドによる ELF バイナリ静的解析（危険 syscall・ネットワーク機能の検出、動的ライブラリ依存関係のハッシュ記録）
- コマンド実行のためのインテリジェントリスク評価
- 一貫したセキュリティ境界を持つ統一リソース管理
- CommandResult作成時（第1層）とRedactingHandlerによる全ログ出力（第2層）での自動機密データ編集による二重防御
- 安全なユーザー・グループ実行機能
- セキュリティ対応メッセージングを伴う包括的マルチチャンネル通知
- stdin無効化による実行環境の明示的制御
- 出力サイズ制限によるリソース枯渇攻撃の防止

システムは、運用の柔軟性と透明性を維持しながら、エンタープライズグレードのセキュリティ制御を提供します。ELFバイナリ静的解析と機密データの二重防御編集により、包括的なセキュリティ対策が実現されています。
