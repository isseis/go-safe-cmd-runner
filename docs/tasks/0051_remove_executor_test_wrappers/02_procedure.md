# 作業手順書: executor_test.go のラッパー関数削除

## 作業の流れ

1. 使用箇所の特定と分類
2. 各テストケースの更新
3. ラッパー関数の削除
4. 最終確認

## 前提条件

- `internal/runner/executor/testing/helpers.go` に Option パターンの `CreateRuntimeCommand` が実装済み
- テストが全て通る状態である (`make test` が成功する)

## 作業手順

### Phase 1: 使用箇所の特定

#### タスク 1-1: 使用箇所のリストアップ

```bash
grep -n "createRuntimeCommand" internal/runner/executor/executor_test.go | grep -v "^22:" | grep -v "^37:"
```

**結果**: 32箇所の使用を確認

- `createRuntimeCommand` 使用: 30箇所
- `createRuntimeCommandWithName` 使用: 2箇所

### Phase 2: テストケースの更新

各行を個別に更新し、更新後にテストを実行して動作を確認する。

#### グループ A: 基本的な使用 (workDir="", runAsUser="", runAsGroup="")

これらは最もシンプルな変換パターン。

- [ ] Line 65: `TestExecute/successful_execution`
  ```go
  // Before
  cmd: createRuntimeCommand("echo", []string{"hello"}, "", "", ""),
  // After
  cmd: executortesting.CreateRuntimeCommand("echo", []string{"hello"},
      executortesting.WithWorkDir("")),
  ```

- [ ] Line 83: `TestExecute/command_with_args`
  ```go
  // Before
  cmd: createRuntimeCommand("echo", []string{"-n", "test"}, "", "", ""),
  // After
  cmd: executortesting.CreateRuntimeCommand("echo", []string{"-n", "test"},
      executortesting.WithWorkDir("")),
  ```

- [ ] Line 141: `TestExecute_Error/command_not_found`
  ```go
  // Before
  cmd: createRuntimeCommand("nonexistentcommand12345", []string{}, "", "", ""),
  // After
  cmd: executortesting.CreateRuntimeCommand("nonexistentcommand12345", []string{},
      executortesting.WithWorkDir("")),
  ```

- [ ] Line 148: `TestExecute_Error/command_fails`
  ```go
  // Before
  cmd: createRuntimeCommand("sh", []string{"-c", "exit 1"}, "", "", ""),
  // After
  cmd: executortesting.CreateRuntimeCommand("sh", []string{"-c", "exit 1"},
      executortesting.WithWorkDir("")),
  ```

- [ ] Line 155: `TestExecute_Error/stderr_captured`
  ```go
  // Before
  cmd: createRuntimeCommand("sh", []string{"-c", "echo 'error message' >&2; exit 0"}, "", "", ""),
  // After
  cmd: executortesting.CreateRuntimeCommand("sh", []string{"-c", "echo 'error message' >&2; exit 0"},
      executortesting.WithWorkDir("")),
  ```

- [ ] Line 161: `TestExecute_Error/timeout`
  ```go
  // Before
  cmd: createRuntimeCommand("sleep", []string{"2"}, "", "", ""),
  // After
  cmd: executortesting.CreateRuntimeCommand("sleep", []string{"2"},
      executortesting.WithWorkDir("")),
  ```

- [ ] Line 226: `TestExecuteWithOutputWriter`
  ```go
  // Before
  cmd := createRuntimeCommand("sleep", []string{"10"}, "", "", "")
  // After
  cmd := executortesting.CreateRuntimeCommand("sleep", []string{"10"},
      executortesting.WithWorkDir(""))
  ```

- [ ] Line 253: `TestExecuteWithEnvironment`
  ```go
  // Before
  cmd := createRuntimeCommand("printenv", []string{}, "", "", "")
  // After
  cmd := executortesting.CreateRuntimeCommand("printenv", []string{},
      executortesting.WithWorkDir(""))
  ```

- [ ] Line 286: `TestValidate/valid_command_with_local_path`
  ```go
  // Before
  cmd: createRuntimeCommand("echo", []string{"hello"}, "", "", ""),
  // After
  cmd: executortesting.CreateRuntimeCommand("echo", []string{"hello"},
      executortesting.WithWorkDir("")),
  ```

- [ ] Line 460: `TestUserGroupExecution/no_privilege_manager`
  ```go
  // Before
  cmd := createRuntimeCommand("echo", []string{"test"}, "", "", "")
  // After
  cmd := executortesting.CreateRuntimeCommand("echo", []string{"test"},
      executortesting.WithWorkDir(""))
  ```

- [ ] Line 619: `TestUserGroupExecutionMixedPrivileges`
  ```go
  // Before
  cmd := createRuntimeCommand("echo", []string{"normal"}, "", "", "")
  // After
  cmd := executortesting.CreateRuntimeCommand("echo", []string{"normal"},
      executortesting.WithWorkDir(""))
  ```

#### グループ B: workDir 指定あり

- [ ] Line 74: `TestExecute/working_directory` (workDir=".")
  ```go
  // Before
  cmd: createRuntimeCommand("pwd", []string{}, ".", "", ""),
  // After
  cmd: executortesting.CreateRuntimeCommand("pwd", []string{},
      executortesting.WithWorkDir(".")),
  ```

- [ ] Line 291: `TestValidate/nonexistent_directory` (workDir="/nonexistent/directory")
  ```go
  // Before
  cmd: createRuntimeCommand("ls", []string{}, "/nonexistent/directory", "", ""),
  // After
  cmd: executortesting.CreateRuntimeCommand("ls", []string{},
      executortesting.WithWorkDir("/nonexistent/directory")),
  ```

- [ ] Line 488: `TestValidatePrivilegedCommand/valid_relative_workdir` (workDir="tmp")
  ```go
  // Before
  cmd: createRuntimeCommand("/bin/echo", []string{"test"}, "tmp", "testuser", "testgroup"),
  // After
  cmd: executortesting.CreateRuntimeCommand("/bin/echo", []string{"test"},
      executortesting.WithWorkDir("tmp"),
      executortesting.WithRunAsUser("testuser"),
      executortesting.WithRunAsGroup("testgroup")),
  ```

- [ ] Line 494: `TestValidatePrivilegedCommand/valid_absolute_workdir` (workDir="/tmp")
  ```go
  // Before
  cmd: createRuntimeCommand("/bin/echo", []string{"test"}, "/tmp", "testuser", "testgroup"),
  // After
  cmd: executortesting.CreateRuntimeCommand("/bin/echo", []string{"test"},
      executortesting.WithWorkDir("/tmp"),
      executortesting.WithRunAsUser("testuser"),
      executortesting.WithRunAsGroup("testgroup")),
  ```

#### グループ C: runAsUser/runAsGroup 指定あり

- [ ] Line 333: `TestUserGroupExecution/successful_user_group_execution`
  ```go
  // Before
  cmd := createRuntimeCommand("/bin/echo", []string{"test"}, "", "testuser", "testgroup")
  // After
  cmd := executortesting.CreateRuntimeCommand("/bin/echo", []string{"test"},
      executortesting.WithWorkDir(""),
      executortesting.WithRunAsUser("testuser"),
      executortesting.WithRunAsGroup("testgroup"))
  ```

- [ ] Line 351: `TestUserGroupExecution/privileged_execution_not_supported`
  ```go
  // Before
  cmd := createRuntimeCommand("/bin/echo", []string{"test"}, "", "testuser", "testgroup")
  // After
  cmd := executortesting.CreateRuntimeCommand("/bin/echo", []string{"test"},
      executortesting.WithWorkDir(""),
      executortesting.WithRunAsUser("testuser"),
      executortesting.WithRunAsGroup("testgroup"))
  ```

- [ ] Line 367: `TestUserGroupExecution/validation_fails_for_relative_path`
  ```go
  // Before
  cmd := createRuntimeCommand("echo", []string{"test"}, "", "testuser", "testgroup")
  // After
  cmd := executortesting.CreateRuntimeCommand("echo", []string{"test"},
      executortesting.WithWorkDir(""),
      executortesting.WithRunAsUser("testuser"),
      executortesting.WithRunAsGroup("testgroup"))
  ```

- [ ] Line 383: `TestUserGroupExecution/privilege_execution_fails`
  ```go
  // Before
  cmd := createRuntimeCommand("/bin/echo", []string{"test"}, "", "invaliduser", "invalidgroup")
  // After
  cmd := executortesting.CreateRuntimeCommand("/bin/echo", []string{"test"},
      executortesting.WithWorkDir(""),
      executortesting.WithRunAsUser("invaliduser"),
      executortesting.WithRunAsGroup("invalidgroup"))
  ```

- [ ] Line 399: `TestUserGroupExecution/user_only_execution`
  ```go
  // Before
  cmd := createRuntimeCommand("/bin/echo", []string{"test"}, "", "testuser", "")
  // After
  cmd := executortesting.CreateRuntimeCommand("/bin/echo", []string{"test"},
      executortesting.WithWorkDir(""),
      executortesting.WithRunAsUser("testuser"))
  ```

- [ ] Line 418: `TestUserGroupExecution/group_only_execution`
  ```go
  // Before
  cmd := createRuntimeCommand("/bin/echo", []string{"test"}, "", "", "testgroup")
  // After
  cmd := executortesting.CreateRuntimeCommand("/bin/echo", []string{"test"},
      executortesting.WithWorkDir(""),
      executortesting.WithRunAsGroup("testgroup"))
  ```

- [ ] Line 440: `TestUserGroupExecution/basic_validation_fails`
  ```go
  // Before
  cmd := createRuntimeCommand("/bin/echo", []string{"test"}, "", "testuser", "testgroup")
  // After
  cmd := executortesting.CreateRuntimeCommand("/bin/echo", []string{"test"},
      executortesting.WithWorkDir(""),
      executortesting.WithRunAsUser("testuser"),
      executortesting.WithRunAsGroup("testgroup"))
  ```

- [ ] Line 483: `TestValidatePrivilegedCommand/valid_absolute_path`
  ```go
  // Before
  cmd: createRuntimeCommand("/bin/echo", []string{"test"}, "", "testuser", "testgroup"),
  // After
  cmd: executortesting.CreateRuntimeCommand("/bin/echo", []string{"test"},
      executortesting.WithWorkDir(""),
      executortesting.WithRunAsUser("testuser"),
      executortesting.WithRunAsGroup("testgroup")),
  ```

- [ ] Line 499: `TestValidatePrivilegedCommand/invalid_path_with_parent_reference`
  ```go
  // Before
  cmd: createRuntimeCommand("/bin/../bin/echo", []string{"test"}, "", "testuser", "testgroup"),
  // After
  cmd: executortesting.CreateRuntimeCommand("/bin/../bin/echo", []string{"test"},
      executortesting.WithWorkDir(""),
      executortesting.WithRunAsUser("testuser"),
      executortesting.WithRunAsGroup("testgroup")),
  ```

- [ ] Line 599: `TestUserGroupExecutionIntegration`
  ```go
  // Before
  cmd := createRuntimeCommand("/bin/echo", []string{"test"}, "", "root", "wheel")
  // After
  cmd := executortesting.CreateRuntimeCommand("/bin/echo", []string{"test"},
      executortesting.WithWorkDir(""),
      executortesting.WithRunAsUser("root"),
      executortesting.WithRunAsGroup("wheel"))
  ```

- [ ] Line 647: `TestExecuteSudoCommand/successful_sudo_execution`
  ```go
  // Before
  cmd: createRuntimeCommand("/usr/bin/whoami", []string{}, "", "root", ""),
  // After
  cmd: executortesting.CreateRuntimeCommand("/usr/bin/whoami", []string{},
      executortesting.WithWorkDir(""),
      executortesting.WithRunAsUser("root")),
  ```

- [ ] Line 655: `TestExecuteSudoCommand/sudo_execution_without_manager`
  ```go
  // Before
  cmd: createRuntimeCommand("/usr/bin/whoami", []string{}, "", "root", ""),
  // After
  cmd: executortesting.CreateRuntimeCommand("/usr/bin/whoami", []string{},
      executortesting.WithWorkDir(""),
      executortesting.WithRunAsUser("root")),
  ```

- [ ] Line 664: `TestExecuteSudoCommand/sudo_execution_not_supported`
  ```go
  // Before
  cmd: createRuntimeCommand("/usr/bin/whoami", []string{}, "", "root", ""),
  // After
  cmd: executortesting.CreateRuntimeCommand("/usr/bin/whoami", []string{},
      executortesting.WithWorkDir(""),
      executortesting.WithRunAsUser("root")),
  ```

- [ ] Line 673: `TestExecuteSudoCommand/normal_execution_without_privilege`
  ```go
  // Before
  cmd: createRuntimeCommand("/bin/echo", []string{"test"}, "", "", ""),
  // After
  cmd: executortesting.CreateRuntimeCommand("/bin/echo", []string{"test"},
      executortesting.WithWorkDir("")),
  ```

#### グループ D: エッジケース

- [ ] Line 281: `TestValidate/empty_command`
  ```go
  // Before
  cmd: createRuntimeCommand("", []string{}, "", "", ""),
  // After
  cmd: executortesting.CreateRuntimeCommand("", []string{},
      executortesting.WithWorkDir("")),
  ```

#### グループ E: createRuntimeCommandWithName の使用

- [ ] Line 553: `TestUserGroupAuditLogging/audit_user_group_execution`
  ```go
  // Before
  cmd := createRuntimeCommandWithName("test_audit_user_group", "/bin/echo", []string{"test"}, "", "testuser", "testgroup")
  // After
  cmd := executortesting.CreateRuntimeCommand("/bin/echo", []string{"test"},
      executortesting.WithName("test_audit_user_group"),
      executortesting.WithWorkDir(""),
      executortesting.WithRunAsUser("testuser"),
      executortesting.WithRunAsGroup("testgroup"))
  ```

- [ ] Line 578: `TestUserGroupAuditLogging/no_audit_logger`
  ```go
  // Before
  cmd := createRuntimeCommandWithName("test_no_audit", "/bin/echo", []string{"test"}, "", "testuser", "testgroup")
  // After
  cmd := executortesting.CreateRuntimeCommand("/bin/echo", []string{"test"},
      executortesting.WithName("test_no_audit"),
      executortesting.WithWorkDir(""),
      executortesting.WithRunAsUser("testuser"),
      executortesting.WithRunAsGroup("testgroup"))
  ```

#### テスト実行タイミング

各グループの更新後にテストを実行:
- [ ] グループ A 更新後: `go test -tags test -v ./internal/runner/executor -run "TestExecute|TestValidate/valid_command_with_local_path|TestUserGroupExecution/no_privilege_manager|TestUserGroupExecutionMixedPrivileges"`
- [ ] グループ B 更新後: `go test -tags test -v ./internal/runner/executor -run "TestExecute/working_directory|TestValidate/nonexistent_directory|TestValidatePrivilegedCommand"`
- [ ] グループ C 更新後: `go test -tags test -v ./internal/runner/executor -run "TestUserGroupExecution|TestValidatePrivilegedCommand|TestUserGroupExecutionIntegration|TestExecuteSudoCommand"`
- [ ] グループ D 更新後: `go test -tags test -v ./internal/runner/executor -run "TestValidate/empty_command"`
- [ ] グループ E 更新後: `go test -tags test -v ./internal/runner/executor -run "TestUserGroupAuditLogging"`

### Phase 3: ラッパー関数の削除

- [ ] `createRuntimeCommand` 関数を削除 (Line 22-35)
- [ ] `createRuntimeCommandWithName` 関数を削除 (Line 37-51)
- [ ] 削除後にテスト実行: `go test -tags test -v ./internal/runner/executor`

### Phase 4: 最終確認

- [ ] 全体テスト実行: `make test`
- [ ] リンター実行: `make lint`
- [ ] 変更内容のレビュー: `git diff internal/runner/executor/executor_test.go`

## 作業時の注意事項

1. **workDir="" の扱い**
   - 必ず `executortesting.WithWorkDir("")` を明示的に指定する
   - 省略すると `os.TempDir()` が使用され、既存のテスト動作が変わる

2. **runAsUser/runAsGroup の扱い**
   - 空文字列の場合はオプション自体を省略する
   - 値がある場合のみ `WithRunAsUser` / `WithRunAsGroup` を使用

3. **段階的な作業**
   - 1つのグループを更新したら必ずテストを実行
   - 問題があればすぐに修正してから次のグループへ進む

4. **コード整形**
   - 変更後は `make fmt` を実行してコードを整形

## トラブルシューティング

### テストが失敗する場合

1. workDir の指定が正しいか確認
   - `WithWorkDir("")` が必要な箇所で省略していないか
   - `WithWorkDir("")` を指定すべきでない箇所で指定していないか

2. runAsUser/runAsGroup の指定が正しいか確認
   - 空文字列の場合はオプションを省略しているか
   - 値がある場合はオプションを指定しているか

3. 元のコードと比較
   - 変更前後でパラメータが一致しているか確認

### リンターエラーが発生する場合

1. `make fmt` を実行してコードを整形
2. import の整理が必要な場合は `goimports` を実行

## 完了基準

- [ ] 全32箇所の使用箇所が更新されている
- [ ] ラッパー関数2つ (`createRuntimeCommand`, `createRuntimeCommandWithName`) が削除されている
- [ ] `make test` が成功する
- [ ] `make lint` が成功する
- [ ] git diff で変更内容を確認し、意図通りの変更であることを確認
