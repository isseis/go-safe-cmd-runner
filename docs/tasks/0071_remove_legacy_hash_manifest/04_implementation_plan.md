# Legacy HashManifest 削除 — 実装手順書

## 目的

`filevalidator` パッケージから Legacy HashManifest フォーマットサポートを削除し、すべてのコードパスを FileAnalysisRecord（`fileanalysis.Store`）フォーマットに統一する。

## 背景

現在 `Validator` には2つのフォーマットが共存している:

- **新形式** (`NewWithAnalysisStore`): `fileanalysis.Store` を使い、ファイルパスをキーとして `Record` を JSON で保存
- **旧形式** (`New`): `HashManifest` 構造体を JSON ファイルとして直接書き込み

`store != nil` による分岐が `Record()` と `Verify()` に存在し、`VerifyFromHandle`・`VerifyWithPrivileges`・`VerifyAndRead`・`VerifyAndReadWithPrivileges` は旧形式のみ対応。この二重構造を解消する。

## 影響範囲の整理

### 削除対象コード

| ファイル | 対象 | 説明 |
|---|---|---|
| `validator.go` | `New()` | 旧形式コンストラクタ |
| `validator.go` | `recordWithHashManifest()` | 旧形式 Record 実装 |
| `validator.go` | `verifyWithHashManifest()` | 旧形式 Verify 実装 |
| `validator.go` | `readAndParseHashFile()` | 旧形式ハッシュファイル読み込み |
| `validator.go` | `parseAndValidateHashFile()` | 旧形式バリデーション |
| `validator.go` | `writeHashManifest()` | 旧形式書き込み |
| `validator.go` | `Record()` / `Verify()` 内の `if v.store != nil` 分岐 | 旧形式フォールバック |
| `hash_manifest.go` | ファイル全体 | `HashManifest` 型・ユーティリティ関数 |
| `errors.go` | `ErrHashCollision` | 旧形式でのみ使用 |
| `errors.go` | `ErrInvalidManifestFormat` | 旧形式でのみ使用 |
| `errors.go` | `ErrUnsupportedVersion` | 旧形式でのみ使用 |
| `errors.go` | `ErrJSONParseError` | 旧形式でのみ使用 |

### 修正が必要なメソッド（新形式対応が未実装）

| メソッド | 現状 | 修正内容 |
|---|---|---|
| `VerifyFromHandle()` | `readAndParseHashFile()` 依存（旧形式のみ） | `store.Load()` で期待ハッシュを取得するよう変更 |
| `VerifyWithPrivileges()` | `VerifyFromHandle()` 経由で旧形式 | `VerifyFromHandle()` 修正で自動対応 |
| `VerifyAndRead()` | `verifyAndReadContent()` → `readAndParseHashFile()` | `store.Load()` で期待ハッシュを取得するよう変更 |
| `VerifyAndReadWithPrivileges()` | 同上 | 同上 |

### 呼び出し元の移行（`filevalidator.New` → `filevalidator.NewWithAnalysisStore`）

| ファイル | 行 | 用途 |
|---|---|---|
| `internal/cmdcommon/common.go:15` | `CreateValidator()` | CLI 共通ユーティリティ |
| `internal/runner/security/hash_validation.go:27` | `validateFileHash()` | セキュリティバリデーション |
| `internal/verification/manager.go:448` | `Manager` 初期化 | ファイル検証マネージャ |

### テストの修正

| ファイル | 修正内容 |
|---|---|
| `validator_test.go` | `newValidator()` 使用テスト → `NewWithAnalysisStore()` に移行。HashManifest 固有テスト（マニフェストフォーマット検証、ハッシュ衝突テスト等）を削除 |
| `benchmark_test.go` | `VerifyFromHandle` ベンチマーク — 新形式対応 |
| `validator_error_test.go` | 旧形式エラー（`ErrHashCollision` 等）の参照を削除 |
| `test/security/hash_bypass_test.go` | `filevalidator.New` → `NewWithAnalysisStore` |
| `cmd/runner/integration_security_test.go` | `filevalidator.New` → `NewWithAnalysisStore` |
| `internal/runner/bootstrap/config_test.go` | `filevalidator.New` → `NewWithAnalysisStore` |
| `internal/cmdcommon/common_test.go` | `CreateValidator` テスト（内部変更で自動対応の可能性あり） |

## 実装ステップ

### Step 1: `VerifyFromHandle` / `verifyAndReadContent` を新形式対応

**対象ファイル**: `internal/filevalidator/validator.go`

**作業内容**:
- [x] `VerifyFromHandle()` を修正: `readAndParseHashFile()` の代わりに `store.Load()` で期待ハッシュを取得
- [x] `verifyAndReadContent()` を修正: 同上
- [x] `Record()` と `Verify()` から `if v.store != nil` 分岐を削除し、`store` を常に使用

**成功条件**: 既存テストがすべてパスすること（この時点では `New()` も `NewWithAnalysisStore()` も残す）

### Step 2: `New()` を `NewWithAnalysisStore()` に統合

**対象ファイル**: `internal/filevalidator/validator.go`

**作業内容**:
- [x] `New()` の実装を `NewWithAnalysisStore()` と同じにする（`store` を常に作成）
- [x] `NewWithAnalysisStore()` を削除し、`New()` に一本化
- [x] `Validator` 構造体の `store` フィールドのコメントから「If nil」の記述を削除

**成功条件**: `make test && make lint` がパスすること

### Step 3: 呼び出し元の移行

**対象ファイル**:
- `internal/cmdcommon/common.go`
- `internal/runner/security/hash_validation.go`
- `internal/verification/manager.go`

**作業内容**:
- [-] `filevalidator.NewWithAnalysisStore` → `filevalidator.New` に変更（Step 2 で `New` に統合済みのため）
- [x] `NewWithAnalysisStore` が残っている全参照を `New` に更新（テストコード内で更新済み）

**成功条件**: `make test && make lint` がパスすること

### Step 4: Legacy コードの削除

**対象ファイル**: `internal/filevalidator/`

**作業内容**:
- [x] `recordWithHashManifest()` を削除
- [x] `verifyWithHashManifest()` を削除
- [x] `readAndParseHashFile()` を削除
- [x] `parseAndValidateHashFile()` を削除
- [x] `writeHashManifest()` を削除
- [x] `hash_manifest.go` ファイル全体を削除
- [x] 不要になったエラー定数を削除: `ErrHashCollision`, `ErrInvalidManifestFormat`, `ErrUnsupportedVersion`, `ErrJSONParseError`
- [x] 不要になった import を整理

**成功条件**: `make test && make lint` がパスすること

### Step 5: テストの整理

**対象ファイル**:
- `internal/filevalidator/validator_test.go`
- `internal/filevalidator/benchmark_test.go`
- `test/security/hash_bypass_test.go`
- `cmd/runner/integration_security_test.go`
- `internal/runner/bootstrap/config_test.go`

**作業内容**:
- [x] `newValidator()` を直接使うテストを `New()` に移行
- [x] HashManifest 固有のテストを削除（マニフェストフォーマット検証、ハッシュ衝突テスト等）
- [x] `filevalidator.New` を使うテストのシグネチャ変更対応（`New` が `error` を返す形式は維持されるため軽微）
- [x] ベンチマークテストの更新

**成功条件**: `make test && make lint` がパスすること

### Step 6: ドキュメント・コメントの更新

**作業内容**:
- [ ] `validator.go` の `Record()`, `Verify()` 等の doc コメントから「legacy」「HashManifest」の記述を削除
- [ ] `FileValidator` インターフェースの doc コメント更新
- [ ] `Validator` 構造体の doc コメント更新

**成功条件**: `make test && make lint` がパスすること

## 注意事項

- **Step 1 → Step 2 の順序が重要**: `VerifyFromHandle` 等を先に新形式対応してから `New()` を統合しないと、`store` が nil の状態で新形式コードパスを通ることになる
- **`New()` のシグネチャ変更**: 現在 `New()` はディレクトリが事前に存在する必要があるが、`NewWithAnalysisStore()` は自動作成する。統合後の挙動を `NewWithAnalysisStore()` 側に合わせる（ディレクトリ自動作成）
- **`ErrHashCollision`**: 旧形式の `hash_manifest.go:86` と `validator.go:224` でのみ使用。新形式ではファイルパスをキーとするため衝突検出の概念自体がない
- **`CollidingHashAlgorithm` / `CollidingHashFilePathGetter`**: テストヘルパーで旧形式の衝突テスト用。削除対象
