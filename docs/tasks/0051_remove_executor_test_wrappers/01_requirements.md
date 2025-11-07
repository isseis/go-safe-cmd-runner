# 要件定義書: executor_test.go のラッパー関数削除

## 概要

`internal/runner/executor/executor_test.go` に定義されている `createRuntimeCommand` および `createRuntimeCommandWithName` ラッパー関数を削除し、共通ヘルパー関数 `executortesting.CreateRuntimeCommand` を直接使用するようテストコードを更新する。

## 背景

### 現状の問題

1. **重複コードの存在**: `executor_test.go` 内に独自のラッパー関数が定義されており、共通ヘルパー `executortesting.CreateRuntimeCommand` と機能が重複している
2. **保守性の低下**: 同じ目的のコードが複数箇所に存在することで、変更時の修正箇所が増える
3. **可読性への影響が限定的**: Option パターン導入により、ラッパー関数を経由しても直接呼び出しても可読性に大きな差がない

### 変更の契機

`executortesting.CreateRuntimeCommand` が Option パターンを採用したことにより:
- 必須パラメータ (`cmd`, `args`) と任意パラメータを明確に区別できるようになった
- 呼び出し側で必要なパラメータのみを指定できるようになった
- ラッパー関数を経由する必要性が低下した

## 目的

1. **コードの一元化**: テスト用 `RuntimeCommand` の生成を共通ヘルパー関数に統一する
2. **保守性の向上**: 変更箇所を1箇所に集約し、将来的な修正を容易にする
3. **可読性の維持**: Option パターンにより、直接呼び出しでも意図が明確なコードを保つ

## 対象ファイル

- `internal/runner/executor/executor_test.go`
  - 削除対象: `createRuntimeCommand` 関数
  - 削除対象: `createRuntimeCommandWithName` 関数
  - 更新対象: 上記関数を使用している全てのテストケース

## 変更方針

### 1. 関数マッピング

#### `createRuntimeCommand(cmd, args, workDir, runAsUser, runAsGroup)` の置き換え

**Before:**
```go
createRuntimeCommand("echo", []string{"hello"}, "", "", "")
```

**After:**
```go
executortesting.CreateRuntimeCommand("echo", []string{"hello"},
    executortesting.WithWorkDir(""))
```

**パラメータ対応:**
- `cmd` → 第1引数 (必須)
- `args` → 第2引数 (必須)
- `workDir` → `executortesting.WithWorkDir(workDir)` オプション
- `runAsUser` → `executortesting.WithRunAsUser(runAsUser)` オプション (空文字列の場合は省略可)
- `runAsGroup` → `executortesting.WithRunAsGroup(runAsGroup)` オプション (空文字列の場合は省略可)

#### `createRuntimeCommandWithName(name, cmd, args, workDir, runAsUser, runAsGroup)` の置き換え

**Before:**
```go
createRuntimeCommandWithName("test-cmd", "echo", []string{"hello"}, "/tmp", "", "")
```

**After:**
```go
executortesting.CreateRuntimeCommand("echo", []string{"hello"},
    executortesting.WithName("test-cmd"),
    executortesting.WithWorkDir("/tmp"))
```

**パラメータ対応:**
- `name` → `executortesting.WithName(name)` オプション
- その他は `createRuntimeCommand` と同じ

### 2. 空文字列パラメータの扱い

- `workDir=""` の場合: `executortesting.WithWorkDir("")` を明示的に指定
  - 理由: デフォルトでは `os.TempDir()` が使用されるため、既存のテスト動作を保持するには明示的な指定が必要
- `runAsUser=""`, `runAsGroup=""` の場合: オプション自体を省略
  - 理由: 空文字列は「指定なし」を意味するため、オプションを付けない方が意図が明確

### 3. 段階的な移行

1. 全ての使用箇所を特定
2. 各テストケースを個別に更新
3. 各更新後にテストを実行し、動作を確認
4. 全ての更新が完了したらラッパー関数を削除
5. 最終的な全体テストを実行

## 成功基準

1. `createRuntimeCommand` および `createRuntimeCommandWithName` が完全に削除されている
2. 全てのテストケースが `executortesting.CreateRuntimeCommand` を使用している
3. 全てのテストが成功する (`make test` が通る)
4. リンターエラーが発生しない (`make lint` が通る)

## リスクと対策

### リスク1: workDir のデフォルト動作の違い

- **リスク**: ラッパー関数では `workDir=""` は空文字列として扱われるが、`CreateRuntimeCommand` では `os.TempDir()` がデフォルト
- **対策**: 全ての `workDir=""` のケースで `executortesting.WithWorkDir("")` を明示的に指定

### リスク2: テスト動作の変更

- **リスク**: 変更により既存のテスト動作が意図せず変わる可能性
- **対策**: 各変更後に個別にテストを実行し、動作を確認する

### リスク3: 見落とし

- **リスク**: 一部の使用箇所を見落とす可能性
- **対策**: grep で全ての使用箇所を特定し、チェックリストで管理する

## 参考情報

- Option パターンの実装: `internal/runner/executor/testing/helpers.go`
- 関連タスク: 0050 (testify migration) - 同様のリファクタリング作業
