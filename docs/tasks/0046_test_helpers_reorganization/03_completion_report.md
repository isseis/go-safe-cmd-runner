# テストヘルパファイル統一化 完了レポート

## 実施日
2025-10-29

## 概要
テストヘルパファイルの命名規則と配置場所を統一し、コードベースの保守性を向上させた。

## 採用したルール

### 分類A: `testing/` サブディレクトリに配置
**対象**: パッケージ横断で使用される汎用ヘルパとモック実装

**既に存在していたファイル(変更なし)**:
- `internal/common/testing/mocks.go`
- `internal/runner/executor/testing/testify_mocks.go`
- `internal/runner/privilege/testing/mocks.go`
- `internal/runner/security/testing/testify_mocks.go`
- `internal/runner/testing/mocks.go`
- `internal/verification/testing/testify_mocks.go`

**ビルドタグのみ修正**:
- `internal/runner/executor/testing/mocks.go`
  - 変更前: `//go:build test || !prod`
  - 変更後: `//go:build test`

**新規作成**:
- `internal/runner/testing/helpers.go`
  - `setupTestEnv`, `setupSafeTestEnv`, `setupFailedMockExecution` などのヘルパ関数を集約

### 分類B: パッケージ内に残す(test_helpers.go に統一)
**対象**: パッケージ内部型にメソッドを追加するファイル、または非公開APIを使用するファイル

**リネーム実施**:
1. `internal/verification/manager_testing.go` → `internal/verification/test_helpers.go`
   - パッケージ内部の `NewManagerForTest` などを定義

2. `internal/runner/runnertypes/config_test_helpers.go` → `internal/runner/runnertypes/test_helpers.go`
   - `AllowlistResolutionBuilder` へのメソッド追加

3. `internal/runner/variable/auto_test_helper.go` → `internal/runner/variable/test_helpers.go`
   - `NewAutoVarProviderWithClock` 関数

4. `internal/runner/group_executor_test_helpers.go` → `internal/runner/test_helpers_group.go`
   - `NewTestGroupExecutor` などの関数

5. `internal/runner/bootstrap/verification_test_helper.go` → `internal/runner/bootstrap/test_helpers.go`
   - `InitializeVerificationManagerForTest` 関数

**既存ファイル維持**:
- `internal/runner/test_helpers.go`
  - `testing/helpers.go` のラッパー関数として機能

### 削除・統合したファイル
1. `cmd/runner/main_test_helper.go` - 削除
   - 内容を `cmd/runner/main_test.go` に直接実装

## 実施した変更

### Phase 1: ディレクトリ構造の準備
- 不要(既存のディレクトリ構造で対応可能と判断)

### Phase 2: ファイル移動とリネーム
- 4ファイルをリネーム
- 1ファイルを削除して内容を統合
- 1ファイルのビルドタグを修正

### Phase 3: インポートパスの更新
- `cmd/runner/main_test.go` のインポートと関数呼び出しを更新

### Phase 4: 動作確認
- `make test` - 成功
- `make build` - 成功

### Phase 5: クリーンアップ
- 空の testing ディレクトリを削除
  - `internal/runner/runnertypes/testing/`
  - `internal/runner/variable/testing/`

## 最終的なファイル配置

### testing/ サブディレクトリのファイル
```
internal/common/testing/
├── mocks.go
└── mocks_test.go

internal/runner/executor/testing/
├── testify_mocks.go
└── mocks.go (ビルドタグ修正)

internal/runner/privilege/testing/
└── mocks.go

internal/runner/security/testing/
├── testify_mocks.go
└── testify_mocks_test.go

internal/runner/testing/
├── mocks.go
└── helpers.go (新規)

internal/verification/testing/
├── testify_mocks.go
└── testify_mocks_test.go
```

### パッケージ内の test_helpers.go
```
internal/verification/test_helpers.go
internal/runner/runnertypes/test_helpers.go
internal/runner/variable/test_helpers.go
internal/runner/test_helpers.go
internal/runner/test_helpers_group.go
internal/runner/bootstrap/test_helpers.go
```

## ビルドタグの統一
すべてのテストヘルパファイルで `//go:build test` を使用

## 完了基準の達成状況
- ✅ ファイル名が統一ルールに従っている
- ✅ すべてのテストが成功する(`make test`)
- ✅ ビルドが成功する(`make build`)
- ✅ CLAUDE.md との一貫性が保たれている

## 今後の推奨事項
1. 新しいテストヘルパを追加する際は、以下のルールに従う:
   - パッケージ横断で使用 → `testing/helpers.go` または `testing/mocks.go`
   - パッケージ内部のみ → `test_helpers.go`

2. すべてのファイルで `//go:build test` タグを必ず付与

3. パッケージ名は以下のいずれかを使用:
   - `testing/` サブディレクトリ: `package testing`
   - パッケージ内: 元のパッケージ名を維持

## 修正が必要だった設計判断

当初は全てのファイルを `testing/` サブディレクトリに移動する計画だったが、以下の理由で一部をパッケージ内に残す方針に変更:

1. **パッケージ型へのメソッド追加**
   - Go言語の制約により、別パッケージから型にメソッドを追加できない
   - 例: `runnertypes.AllowlistResolutionBuilder` へのメソッド追加

2. **非公開APIの使用**
   - テストヘルパが非公開関数や型を使用している場合、同じパッケージ内に留める必要がある
   - 例: `verification` パッケージの内部関数

3. **パッケージ名の競合**
   - 既存の `verificationtesting` パッケージとの競合を避けるため

この修正により、実用性と保守性のバランスが取れた設計となった。
