# 要件定義書: RuntimeCommand 直接作成からヘルパー関数への移行

## 概要

テストファイル内で `RuntimeCommand` 構造体を直接作成している箇所を、共通ヘルパー関数 `executortesting.CreateRuntimeCommand` または `executortesting.CreateRuntimeCommandFromSpec` を使用する形に移行する。

## 背景

### 現状の問題

1. **重複した初期化コード**: 複数のテストファイルで `RuntimeCommand` 構造体を直接作成する独自のヘルパー関数が存在し、初期化ロジックが重複している
2. **不完全な初期化**: 一部のテストでは `TimeoutResolution` などの重要なフィールドが初期化されていない
3. **保守性の低下**: タイムアウト解決ロジックなどの共通処理が各所で重複実装されており、変更時の修正箇所が増える
4. **テストの一貫性欠如**: 同じ目的のテスト用 `RuntimeCommand` 作成でも、ファイルごとに異なる実装が使われている

### 既存の共通ヘルパー関数

`internal/runner/executor/testing/helpers.go` には以下のヘルパー関数が提供されている:

- **`CreateRuntimeCommand(cmd, args, opts...)`**: コマンドと引数から `RuntimeCommand` を作成
  - Option パターンで柔軟な設定が可能
  - タイムアウト解決ロジックを自動的に適用
  - デフォルトで適切な初期化を実行

- **`CreateRuntimeCommandFromSpec(spec)`**: 既存の `CommandSpec` から `RuntimeCommand` を作成
  - `CommandSpec` をパラメータとして受け取る既存のヘルパー関数の置き換えに最適

## 目的

1. **コードの一元化**: テスト用 `RuntimeCommand` の生成を共通ヘルパー関数に統一する
2. **保守性の向上**: 初期化ロジックを1箇所に集約し、将来的な修正を容易にする
3. **テストの品質向上**: 適切な初期化（タイムアウト解決など）を確実に実行する
4. **重複コードの削除**: 各テストファイル内の独自ヘルパー関数を削除する

## 対象ファイルと分類

### 優先度 高: ヘルパー関数を持つファイル（即座に移行すべき）

1. **test/security/output_security_test.go**
   - 対象: `createRuntimeCommand()` ヘルパー関数（L24-33）
   - 移行方法: `executortesting.CreateRuntimeCommandFromSpec()` に置き換え
   - 影響範囲: 同ファイル内の全テストケース

2. **test/performance/output_capture_test.go**
   - 対象: `createRuntimeCommand()` ヘルパー関数（L26-35）
   - 移行方法: `executortesting.CreateRuntimeCommandFromSpec()` に置き換え
   - 影響範囲: 同ファイル内の全テストケース

3. **internal/runner/resource/normal_manager_test.go**
   - 対象: `createTestCommand()` ヘルパー関数（L177-186）
   - 移行方法: `executortesting.CreateRuntimeCommand()` または `CreateRuntimeCommandFromSpec()` に置き換え
   - 影響範囲: 同ファイル内の全テストケース

### 優先度 中: 複雑なセットアップを持つテスト（段階的に移行推奨）

4. **internal/runner/audit/logger_test.go**
   - 対象: テストケース定義内の直接作成（3箇所: L40-48, L62-70, L84-91）
   - 移行方法: `executortesting.CreateRuntimeCommand()` + Option パターン
   - 使用オプション: `WithName()`, `WithRunAsUser()`, `WithRunAsGroup()`

5. **internal/runner/group_executor_test.go**
   - 対象: 完全な `RuntimeCommand` 作成箇所（4箇所: L1203-1213, L1270-1278, L1361-1372, L1505-1512）
   - 移行方法: `executortesting.CreateRuntimeCommand()` + 各種オプション
   - 使用オプション: `WithName()`, `WithExpandedCmd()`, `WithExpandedArgs()`, `WithExpandedEnv()`

6. **internal/runner/executor/environment_bench_test.go**
   - 対象: ベンチマーク内の直接作成（2箇所: L54-59, L90-97）
   - 移行方法: `executortesting.CreateRuntimeCommand()` + オプション
   - 使用オプション: `WithName()`, `WithExpandedEnv()`

7. **cmd/runner/integration_security_test.go**
   - 対象: 統合テスト内の直接作成（3箇所: L267-275, L285-293, L304-311）
   - 移行方法: `executortesting.CreateRuntimeCommand()` + オプション

### 優先度 低: 現状維持を推奨（移行しない）

以下のファイルは、最小限のフィールド設定でテストの意図を明確にする目的で直接作成を使用しており、移行は不適切:

- **internal/runner/runnertypes/runtime_test.go**
  - 理由: `RuntimeCommand` 自体のメソッド（`Name()`, `RunAsUser()` など）をテストするユニットテスト
  - 判断: 最小限のフィールドのみを設定する必要があり、ヘルパー関数は不要な初期化を行うため不適切

- **internal/runner/executor/executor_validation_test.go**
  - 理由: バリデーションロジックのテスト
  - 判断: テスト対象のフィールドのみを設定することで、テストケースの意図が明確になる

- **internal/runner/group_executor_test.go** の一部
  - 理由: タイムアウト処理や作業ディレクトリ解決など、特定のフィールドのみをテストする焦点の絞られたテスト
  - 対象箇所: L74-79, L120-125, L151-156, L944-950, L2334-2339
  - 判断: 最小限のフィールド設定でテストの意図を明確にするため、直接作成が適切

## 変更方針

### 1. ヘルパー関数の置き換えパターン

#### パターンA: `CommandSpec` を受け取るヘルパー関数

**Before:**
```go
func createRuntimeCommand(spec *runnertypes.CommandSpec) *runnertypes.RuntimeCommand {
	return &runnertypes.RuntimeCommand{
		Spec:             spec,
		ExpandedCmd:      spec.Cmd,
		ExpandedArgs:     spec.Args,
		ExpandedEnv:      make(map[string]string),
		ExpandedVars:     make(map[string]string),
		EffectiveWorkDir: "",
		EffectiveTimeout: 30,
	}
}
```

**After:**
```go
import executortesting "github.com/isseis/go-safe-cmd-runner/internal/runner/executor/testing"

func createRuntimeCommand(spec *runnertypes.CommandSpec) *runnertypes.RuntimeCommand {
	return executortesting.CreateRuntimeCommandFromSpec(spec)
}
```

または、ヘルパー関数自体を削除し、直接呼び出しに変更:
```go
// 使用箇所で直接呼び出し
cmd := executortesting.CreateRuntimeCommandFromSpec(spec)
```

#### パターンB: テストケース内での直接作成

**Before:**
```go
cmd := &runnertypes.RuntimeCommand{
	Spec: &runnertypes.CommandSpec{
		Name:       "test-cmd",
		RunAsUser:  "testuser",
		RunAsGroup: "testgroup",
	},
	ExpandedCmd:  "/bin/echo",
	ExpandedArgs: []string{"test"},
}
```

**After:**
```go
cmd := executortesting.CreateRuntimeCommand("/bin/echo", []string{"test"},
	executortesting.WithName("test-cmd"),
	executortesting.WithRunAsUser("testuser"),
	executortesting.WithRunAsGroup("testgroup"))
```

### 2. Option パターンの使用ガイドライン

- **必須パラメータ**: `cmd`（コマンドパス）と `args`（引数）は関数の引数として渡す
- **任意パラメータ**: Option 関数で指定
  - `WithName(name)`: コマンド名
  - `WithWorkDir(workDir)`: 作業ディレクトリ（Spec.WorkDir と EffectiveWorkDir の両方を設定）
  - `WithEffectiveWorkDir(dir)`: EffectiveWorkDir のみを設定
  - `WithExpandedCmd(cmd)`: 展開後のコマンドパス（通常は不要）
  - `WithExpandedArgs(args)`: 展開後の引数（通常は不要）
  - `WithRunAsUser(user)`: 実行ユーザー
  - `WithRunAsGroup(group)`: 実行グループ
  - `WithOutputFile(path)`: 出力ファイルパス
  - `WithExpandedEnv(env)`: 展開後の環境変数
  - `WithTimeout(timeout)`: タイムアウト値（秒）

### 3. 段階的な移行手順

1. **優先度 高のファイルから順に移行**
   - ヘルパー関数を持つファイルから開始
   - 各ファイルごとに完結させる

2. **各ファイルの移行手順**
   1. import 文に `executortesting` を追加
   2. ヘルパー関数の実装を更新（または削除）
   3. テストを実行して動作確認（`make test`）
   4. コミット

3. **優先度 中のファイルを移行**
   - テストケース定義内の直接作成を更新
   - 各テストケースごとに動作確認

4. **最終確認**
   - 全体テストを実行（`make test`）
   - リンターを実行（`make lint`）

## 成功基準

1. 優先度 高・中のファイルで、独自のヘルパー関数が削除されている
2. 優先度 高・中のファイルで、`RuntimeCommand` の直接作成が `executortesting` のヘルパー関数に置き換えられている
3. 全てのテストが成功する（`make test` が通る）
4. リンターエラーが発生しない（`make lint` が通る）
5. 優先度 低のファイルは、直接作成のまま維持されている

## リスクと対策

### リスク1: デフォルト値の違いによるテスト動作の変更

- **リスク**: ヘルパー関数のデフォルト値（例: `EffectiveWorkDir` が `os.TempDir()`）が既存の直接作成と異なる場合、テスト動作が変わる可能性
- **対策**:
  - 各ファイルの既存実装を確認し、デフォルト値が異なる場合は明示的にオプション指定
  - 各変更後に必ずテストを実行して動作を確認

### リスク2: タイムアウト解決ロジックの影響

- **リスク**: `CreateRuntimeCommand` は自動的にタイムアウト解決ロジックを適用するため、既存の直接作成（タイムアウト解決なし）と動作が変わる可能性
- **対策**:
  - タイムアウトに依存するテストでは、既存の `EffectiveTimeout` 値と新しい解決結果を比較
  - 必要に応じて `WithTimeout()` オプションで明示的に指定

### リスク3: 過剰な移行によるテストの意図不明瞭化

- **リスク**: 優先度 低のファイル（ユニットテストやバリデーションテスト）まで移行すると、テストの意図が不明瞭になる
- **対策**:
  - 優先度 低のファイルは移行しない
  - 移行の判断基準を明確にし、文書化する

### リスク4: 見落とし

- **リスク**: 一部の使用箇所を見落とす可能性
- **対策**:
  - grep で全ての使用箇所を特定
  - チェックリスト（作業手順書）で進捗を管理

## 参考情報

- ヘルパー関数の実装: `internal/runner/executor/testing/helpers.go`
- 関連タスク: 0051 (remove executor test wrappers) - 同様のリファクタリング作業
- 調査結果: タスク作成前の分析で特定した全9ファイルの詳細
