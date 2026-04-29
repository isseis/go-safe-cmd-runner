# 要件定義: verification から runner 依存を除去する

## 背景

task 0115 において `internal/filevalidator` から `internal/runner/runnertypes` および
`internal/runner/privilege` への依存は解消された。
しかし `internal/verification/` は引き続き `internal/runner/` 以下に依存している。

```
internal/runner/ → internal/verification/  （runner が verification を使用）
internal/verification/ → internal/runner/runnertypes  （逆方向）
internal/verification/ → internal/runner/security     （逆方向）
```

この依存関係は循環インポートではない（Go はそれをコンパイルエラーとして検出する）が、
アーキテクチャ上の問題がある。`verification` はファイル整合性検証という基盤的な役割を担う
パッケージであり、`runner` はその上位レイヤーとして `verification` を利用する。
しかし現状では `verification` が `runner` 固有の型や実装を参照しており、
`runner` なしで `verification` を再利用することができない。

## 問題

```
go list -deps github.com/isseis/go-safe-cmd-runner/internal/verification | grep internal/runner
```

上記コマンドが `internal/runner/runnertypes` および `internal/runner/security` を返す。

## 原因分析

### 依存 1: `verification` → `internal/runner/runnertypes`

#### `interfaces.go` — `ManagerInterface` インターフェース

```go
type ManagerInterface interface {
    VerifyGroupFiles(runtimeGroup *runnertypes.RuntimeGroup) (*Result, error)
    // ...
}
```

- インターフェースのメソッドシグネチャが `*runnertypes.RuntimeGroup` を直接参照している

#### `manager.go` — `Manager` 本体

```go
func (m *Manager) VerifyGlobalFiles(runtimeGlobal *runnertypes.RuntimeGlobal) (*Result, error)
func (m *Manager) VerifyGroupFiles(runtimeGroup *runnertypes.RuntimeGroup) (*Result, error)
func (m *Manager) collectVerificationFiles(runtimeGroup *runnertypes.RuntimeGroup) map[string]struct{}
```

- メソッドシグネチャが `*runnertypes.RuntimeGlobal` / `*runnertypes.RuntimeGroup` を参照
- `runnertypes.ExtractGroupName(runtimeGroup)` を呼び出している
- これらが使用するのは以下のフィールドのみ:
  - `runtimeGlobal.ExpandedVerifyFiles []string`
  - `runtimeGroup.ExpandedVerifyFiles []string`
  - `runtimeGroup.Commands[].ExpandedCmd string`
  - `runtimeGroup.Spec.Name string`（`ExtractGroupName` 経由）

#### テストヘルパー（`//go:build test` タグ）

- `testing/testify_mocks.go`: `MockManager.VerifyGroupFiles(*runnertypes.RuntimeGroup)`
- `testing/helpers.go`: `MatchRuntimeGroupWithName` が `*runnertypes.RuntimeGroup` を参照

### 依存 2: `verification` → `internal/runner/security`

#### `path_resolver.go` — `PathResolver` 構造体（デッドコード）

```go
type PathResolver struct {
    pathEnv  string
    security *security.Validator  // ← 格納されるが一度も参照されない
    // ...
}

func NewPathResolver(pathEnv string, security *security.Validator) *PathResolver
```

- `security` フィールドはコンストラクタで設定されるが、いかなるメソッドでも使用されていない
- **デッドコード**

#### `manager.go` — `security.Validator` の生成と使用

```go
securityConfig := security.DefaultConfig()
securityValidator, err := security.NewValidator(securityConfig, security.WithFileSystem(opts.fs))
// ...
pathResolver = NewPathResolver(security.SecurePathEnv, securityValidator)
manager.security = securityValidator
```

- `manager.go` は `newManagerInternal` 内で `*security.Validator` を生成している
- `Manager.security` フィールドが `*security.Validator` 型
- `security.SecurePathEnv` 定数（文字列）を参照
- 実際に使用しているのは `manager.security.ValidateDirectoryPermissions(hashDir)` のみ

## 目標

`internal/verification` から `internal/runner/runnertypes` および `internal/runner/security` への
依存を除去し、以下の依存構造を実現する。

```
internal/runner/ → internal/verification/  （一方向のみ）
```

## 受け入れ基準

| # | 基準 |
|---|------|
| AC-1 | `go list -deps github.com/isseis/go-safe-cmd-runner/internal/verification \| grep internal/runner` が 0 件 |
| AC-2 | `go build ./cmd/record ./cmd/verify ./cmd/runner` が成功 |
| AC-3 | `make test` が全件パス |
| AC-4 | `verification.ManagerInterface` のメソッドシグネチャが `runnertypes.RuntimeGroup` / `runnertypes.RuntimeGlobal` を参照しない |
| AC-5 | `cmd/runner` のグループファイル検証（`VerifyGroupFiles`）が既存または本タスクで追加したテストで引き続き検証される |
| AC-6 | task 0115 の AC-1・AC-2（`cmd/record` / `cmd/verify` が `internal/runner` に依存しない）が引き続き成立する |

## 設計方針

### 基本方針

- **YAGNI**: デッドコードは削除する
- **依存の方向を明確化**: 基盤パッケージ（`verification`）は上位パッケージ（`runner`）の型を参照しない
- **インターフェースによる分離**: 具体実装への依存をインターフェースに置き換える
- **境界での変換**: 型変換は依存の境界（上位レイヤー）で行う

---

### Step 1: DTO 型を `verification` パッケージに定義する

`verification` パッケージに、runner 固有の型に依存しない最小限の入力型（DTO）を定義する。

```go
// GroupVerificationInput はグループのファイル検証に必要な最小限の情報を保持する
type GroupVerificationInput struct {
    Name                string
    ExpandedVerifyFiles []string
    Commands            []CommandEntry
}

// CommandEntry は検証対象のコマンドエントリを表す
type CommandEntry struct {
    ExpandedCmd string
}

// GlobalVerificationInput はグローバルのファイル検証に必要な最小限の情報を保持する
type GlobalVerificationInput struct {
    ExpandedVerifyFiles []string
}
```

---

### Step 2: `VerifyGroupFiles` / `VerifyGlobalFiles` のシグネチャを変更する

`*runnertypes.RuntimeGroup` / `*runnertypes.RuntimeGlobal` を DTO に置き換える。

```go
// 変更前
func (m *Manager) VerifyGroupFiles(runtimeGroup *runnertypes.RuntimeGroup) (*Result, error)
func (m *Manager) VerifyGlobalFiles(runtimeGlobal *runnertypes.RuntimeGlobal) (*Result, error)

// 変更後
func (m *Manager) VerifyGroupFiles(input *GroupVerificationInput) (*Result, error)
func (m *Manager) VerifyGlobalFiles(input *GlobalVerificationInput) (*Result, error)
```

`ManagerInterface` の定義も合わせて変更する。

---

### Step 3: 呼び出し元で変換を行う

#### `internal/runner/group_executor.go`

`VerifyGroupFiles` の呼び出し前に `RuntimeGroup` から `GroupVerificationInput` への変換を行う。

```go
input := &verification.GroupVerificationInput{
    Name:                runnertypes.ExtractGroupName(runtimeGroup),
    ExpandedVerifyFiles: runtimeGroup.ExpandedVerifyFiles,
    Commands:            convertCommands(runtimeGroup.Commands),
}
result, err := ge.verificationManager.VerifyGroupFiles(input)
```

#### `cmd/runner/main.go`

`VerifyGlobalFiles` の呼び出し前に `RuntimeGlobal` から `GlobalVerificationInput` への変換を行う。

```go
input := &verification.GlobalVerificationInput{
    ExpandedVerifyFiles: runtimeGlobal.ExpandedVerifyFiles,
}
result, err := verificationManager.VerifyGlobalFiles(input)
```

---

### Step 4: `PathResolver` のデッドコードを除去する

`PathResolver.security` フィールドおよび `NewPathResolver` の `security` パラメータを削除する。

```go
// 変更前
func NewPathResolver(pathEnv string, security *security.Validator) *PathResolver

// 変更後
func NewPathResolver(pathEnv string) *PathResolver
```

---

### Step 5: `DirectoryValidator` インターフェースを定義し、依存注入に切り替える

`verification` パッケージにインターフェースを定義する。

```go
// DirectoryValidator はハッシュディレクトリのパーミッションを検証するインターフェースを定義する
type DirectoryValidator interface {
    ValidateDirectoryPermissions(dirPath string) error
}
```

`Manager.security` フィールドの型を `*security.Validator` から `DirectoryValidator` に変更する。

`ValidateHashDirectory` のnilチェックを調整し、スキップモードを先に確認する:

```go
func (m *Manager) ValidateHashDirectory() error {
    // スキップモード（dry-run または明示的スキップ）では検証不要
    if m.skipHashDirectoryValidation || m.isDryRun {
        return nil
    }
    // 非スキップモードでバリデータが未設定はプログラミングエラー
    if m.security == nil {
        return ErrSecurityValidatorNotInitialized
    }
    return m.security.ValidateDirectoryPermissions(m.hashDir)
}
```

---

### Step 6: `SecurePathEnv` 定数を `verification` パッケージ内に定義する

`security.SecurePathEnv` を参照する代わりに、`verification` パッケージ内に同値の定数を定義する。

```go
// secureDefaultPath はコマンド解決に使用するセキュアな固定 PATH
// internal/runner/security.SecurePathEnv と同じ値を保つこと
const secureDefaultPath = "/sbin:/usr/sbin:/bin:/usr/bin"
```

将来的に定義箇所を一元化する場合は `internal/common` に移動する（別タスク）。

---

### Step 7: `NewManager` を `internal/runner/bootstrap` に移動し、`NewManagerForDryRun` は留置する

`verification.Manager` の生成において `*security.Validator` が必要なのは、
ハッシュディレクトリ検証を行う **本番モード** のみである。
dry-run モードは `skipHashDirectoryValidation == true` / `isDryRun == true` のため、
`ValidateDirectoryPermissions` が呼ばれず `DirectoryValidator` が不要である。

したがって、`*security.Validator` の生成を必要とする `NewManager()` のみを
`verification` パッケージ外に移動し、`NewManagerForDryRun()` は `verification` に留置する。

#### `verification` パッケージに追加する内部オプション関数

```go
func withDirectoryValidatorInternal(v DirectoryValidator) InternalOption {
    return func(opts *managerInternalOptions) {
        opts.directoryValidator = v
    }
}
```

#### `verification/manager_production.go` の変更

`NewManager()` を削除し、代わりに `NewManagerForProduction(validator DirectoryValidator)` を追加する。
`NewManagerForDryRun()` はシグネチャを変えずに留置するが、
`security` import を削除するため内部実装から `*security.Validator` の生成を除去する。

```go
// 追加: 本番用ファクトリ（DirectoryValidator を外部から注入）
// security.Validator の生成は呼び出し元（runner/bootstrap）の責任
func NewManagerForProduction(validator DirectoryValidator) (*Manager, error) {
    logProductionManagerCreation()
    return newManagerInternal(cmdcommon.DefaultHashDirectory,
        withCreationMode(CreationModeProduction),
        withSecurityLevel(SecurityLevelStrict),
        withDirectoryValidatorInternal(validator),
    )
}

// 留置: dry-run ファクトリ（DirectoryValidator 不要）
// dry-run モードはディレクトリ検証をスキップするため security import が不要になる
func NewManagerForDryRun() (*Manager, error) {
    logDryRunManagerCreation()
    return newManagerInternal(cmdcommon.DefaultHashDirectory,
        withCreationMode(CreationModeProduction),
        withSecurityLevel(SecurityLevelStrict),
        withSkipHashDirectoryValidationInternal(),
        withDryRunModeInternal(),
        // DirectoryValidator なし（dry-run は検証をスキップ）
    )
}
```

#### `internal/runner/bootstrap` にファクトリ関数を追加する

```go
// NewVerificationManager は本番用の検証マネージャを生成する。
// security.Validator の生成はここで行い、verification パッケージに注入する。
func NewVerificationManager() (*verification.Manager, error) {
    secConfig := security.DefaultConfig()
    secValidator, err := security.NewValidator(secConfig)
    if err != nil {
        return nil, fmt.Errorf("failed to initialize security validator: %w", err)
    }
    return verification.NewManagerForProduction(secValidator)
}
```

#### `cmd/runner/main.go` の変更

```go
// 変更前
verificationManager, err = verification.NewManager()

// 変更後
verificationManager, err = bootstrap.NewVerificationManager()

// NewManagerForDryRun は変更なし（引き続き verification から呼び出せる）
verificationManager, err = verification.NewManagerForDryRun()
```

---

### Step 8: テストヘルパーを更新する

#### `verification/testing/testify_mocks.go`

`MockManager.VerifyGroupFiles` のシグネチャを更新する。

```go
// 変更前
func (m *MockManager) VerifyGroupFiles(runtimeGroup *runnertypes.RuntimeGroup) (*verification.Result, error)

// 変更後
func (m *MockManager) VerifyGroupFiles(input *verification.GroupVerificationInput) (*verification.Result, error)
```

#### `verification/testing/helpers.go`

`MatchRuntimeGroupWithName` を `GroupVerificationInput` 対応に変更する。

```go
// 変更後: GroupVerificationInput の Name フィールドでマッチング
func MatchRuntimeGroupWithName(expectedName string) any {
    return mock.MatchedBy(func(input *verification.GroupVerificationInput) bool {
        return input != nil && input.Name == expectedName
    })
}
```

#### `internal/runner/test_helpers.go`

`matchRuntimeGroupWithName` を `GroupVerificationInput` 対応に変更する。
（`VerifyGroupFiles` モックへの引数が `*runnertypes.RuntimeGroup` から `*verification.GroupVerificationInput` に変わるため）

```go
// 変更後
func matchRuntimeGroupWithName(expectedName string) any {
    return mock.MatchedBy(func(input *verification.GroupVerificationInput) bool {
        return input != nil && input.Name == expectedName
    })
}
```

#### `internal/verification/manager_test.go` / `internal/verification/testing/testify_mocks_test.go`

テストヘルパー関数（`createRuntimeGroup`, `createRuntimeGlobal` 等）を、変更後の DTO 型を使うよう更新する。
`runnertypes` import を削除する。

---

### 影響範囲

| コンポーネント | 変更内容 |
|---|---|
| `internal/verification/manager.go` | `VerifyGroupFiles` / `VerifyGlobalFiles` / `collectVerificationFiles` のシグネチャを DTO に変更、`DirectoryValidator` インターフェース追加、`runnertypes` / `security` import 削除 |
| `internal/verification/interfaces.go` | `ManagerInterface.VerifyGroupFiles` のシグネチャ変更、`runnertypes` import 削除 |
| `internal/verification/path_resolver.go` | `PathResolver.security` フィールドおよび `NewPathResolver` の `security` パラメータ削除、`security` import 削除 |
| `internal/verification/manager_production.go` | `NewManager` を削除し `NewManagerForProduction(DirectoryValidator)` を追加、`NewManagerForDryRun` は留置（`security` import を削除） |
| `internal/verification/types.go` または新規ファイル | `GroupVerificationInput` / `GlobalVerificationInput` / `CommandEntry` / `DirectoryValidator` を追加 |
| `internal/verification/testing/testify_mocks.go` | `MockManager.VerifyGroupFiles` のシグネチャ変更、`runnertypes` import 削除 |
| `internal/verification/testing/helpers.go` | `MatchRuntimeGroupWithName` を `GroupVerificationInput` 対応に変更、`runnertypes` import 削除 |
| `internal/verification/testing/testify_mocks_test.go` | `runnertypes.RuntimeGroup` → `verification.GroupVerificationInput` に変更 |
| `internal/verification/manager_test.go` | テストヘルパーを DTO 型対応に変更、`runnertypes` import 削除 |
| `internal/runner/test_helpers.go` | `matchRuntimeGroupWithName` を `GroupVerificationInput` 対応に変更 |
| `internal/runner/group_executor.go` | `VerifyGroupFiles` 呼び出し前に `RuntimeGroup` → `GroupVerificationInput` 変換を追加 |
| `internal/runner/bootstrap/` | `NewVerificationManager()` を追加 |
| `cmd/runner/main.go` | `verification.NewManager()` → `bootstrap.NewVerificationManager()` に変更（`NewManagerForDryRun` は変更なし） |

### スコープ外

- `internal/runner/security` パッケージ自体の `runnertypes` 依存整理
- `SecurePathEnv` 定数の一元化（`internal/common` への移動）
- その他の `internal/runner/` 外パッケージの runner 依存解消
