# 作業手順書: CreateRuntimeCommandFromSpec の options パターンへの移行

## 概要

このドキュメントでは、`CreateRuntimeCommandFromSpec` を `CreateRuntimeCommand` + options パターンに移行する具体的な手順を記載する。

## 作業の進め方

1. ファイル単位で移行
2. 各ファイル移行後にテスト実行
3. 問題があれば即座に修正
4. 全ファイル完了後に最終確認

## 事前準備

### 1. 現在の状態確認

```bash
# テストが全て成功することを確認
make test

# Linter エラーがないことを確認
make lint
```

- [ ] テスト成功確認
- [ ] Linter エラーなし確認

## 移行作業

### Phase 1: internal/runner/resource パッケージ

#### 1.1 security_test.go (4箇所)

**対象行**: 97, 190, 256, 321

**作業内容**:
- [ ] L97: `CreateRuntimeCommandFromSpec` → `CreateRuntimeCommand` + options
- [ ] L190: `CreateRuntimeCommandFromSpec` → `CreateRuntimeCommand` + options
- [ ] L256: `CreateRuntimeCommandFromSpec` → `CreateRuntimeCommand` + options
- [ ] L321: `CreateRuntimeCommandFromSpec` → `CreateRuntimeCommand` + options
- [ ] テスト実行: `go test -tags test -v ./internal/runner/resource -run TestSecurityValidation`
- [ ] `make fmt` 実行

**変換例**:
```go
// Before (L97)
cmd := executortesting.CreateRuntimeCommandFromSpec(&tt.spec)

// After
cmd := executortesting.CreateRuntimeCommand(
    tt.spec.Name,
    tt.spec.Cmd,
    executortesting.WithArgs(tt.spec.Args),
    // 他のフィールドも必要に応じて追加
)
```

#### 1.2 error_scenarios_test.go (6箇所)

**対象行**: 228, 305, 470, 622, 685, 722

**作業内容**:
- [ ] L228: `CreateRuntimeCommandFromSpec` → `CreateRuntimeCommand` + options
- [ ] L305: `CreateRuntimeCommandFromSpec` → `CreateRuntimeCommand` + options
- [ ] L470: `CreateRuntimeCommandFromSpec` → `CreateRuntimeCommand` + options
- [ ] L622: `CreateRuntimeCommandFromSpec` → `CreateRuntimeCommand` + options
- [ ] L685: `CreateRuntimeCommandFromSpec` → `CreateRuntimeCommand` + options
- [ ] L722: `CreateRuntimeCommandFromSpec` → `CreateRuntimeCommand` + options
- [ ] テスト実行: `go test -tags test -v ./internal/runner/resource -run TestErrorScenarios`
- [ ] `make fmt` 実行

#### 1.3 usergroup_dryrun_test.go (6箇所)

**対象行**: 27, 67, 105, 140, 175, 215

**作業内容**:
- [ ] L27: `CreateRuntimeCommandFromSpec` → `CreateRuntimeCommand` + options
- [ ] L67: `CreateRuntimeCommandFromSpec` → `CreateRuntimeCommand` + options
- [ ] L105: `CreateRuntimeCommandFromSpec` → `CreateRuntimeCommand` + options
- [ ] L140: `CreateRuntimeCommandFromSpec` → `CreateRuntimeCommand` + options
- [ ] L175: `CreateRuntimeCommandFromSpec` → `CreateRuntimeCommand` + options
- [ ] L215: `CreateRuntimeCommandFromSpec` → `CreateRuntimeCommand` + options
- [ ] テスト実行: `go test -tags test -v ./internal/runner/resource -run TestUserGroupDryRun`
- [ ] `make fmt` 実行

**変換例**:
```go
// Before (L27)
cmd := executortesting.CreateRuntimeCommandFromSpec(&runnertypes.CommandSpec{
    Name:       "test_user_group",
    Cmd:        "echo",
    Args:       []string{"test"},
    RunAsUser:  "testuser",
    RunAsGroup: "testgroup",
})

// After
cmd := executortesting.CreateRuntimeCommand(
    "test_user_group",
    "echo",
    executortesting.WithArgs([]string{"test"}),
    executortesting.WithRunAsUser("testuser"),
    executortesting.WithRunAsGroup("testgroup"),
)
```

#### 1.4 performance_test.go (3箇所)

**対象行**: 31, 150, 196

**作業内容**:
- [ ] L31: `CreateRuntimeCommandFromSpec` → `CreateRuntimeCommand` + options
- [ ] L150: `CreateRuntimeCommandFromSpec` → `CreateRuntimeCommand` + options
- [ ] L196: `CreateRuntimeCommandFromSpec` → `CreateRuntimeCommand` + options
- [ ] テスト実行: `go test -tags test -v ./internal/runner/resource -run TestPerformance`
- [ ] `make fmt` 実行

#### 1.5 dryrun_manager_test.go (2箇所 + 1箇所例外)

**対象行**: 292, 355 (移行対象), 315 (例外)

**作業内容**:
- [ ] L292: `CreateRuntimeCommandFromSpec` → `CreateRuntimeCommand` + options
- [ ] L355: `CreateRuntimeCommandFromSpec` → `CreateRuntimeCommand` + options
- [ ] L315: **移行しない** (テストテーブルから `&tt.spec` を使用)
- [ ] テスト実行: `go test -tags test -v ./internal/runner/resource -run TestDryRunManager`
- [ ] `make fmt` 実行

**変換例**:
```go
// Before (L292)
cmd := executortesting.CreateRuntimeCommandFromSpec(&runnertypes.CommandSpec{
    Name: "setuid-chmod",
    Cmd:  "setuid-chmod",
    Args: []string{"777", "/tmp/test"},
})

// After
cmd := executortesting.CreateRuntimeCommand(
    "setuid-chmod",
    "setuid-chmod",
    executortesting.WithArgs([]string{"777", "/tmp/test"}),
)
```

#### 1.6 integration_test.go (3箇所)

**対象行**: 94, 146, 209

**作業内容**:
- [ ] L94: `CreateRuntimeCommandFromSpec` → `CreateRuntimeCommand` + options
- [ ] L146: `CreateRuntimeCommandFromSpec` → `CreateRuntimeCommand` + options
- [ ] L209: `CreateRuntimeCommandFromSpec` → `CreateRuntimeCommand` + options
- [ ] テスト実行: `go test -tags test -v ./internal/runner/resource -run TestIntegration`
- [ ] `make fmt` 実行

#### 1.7 normal_manager_test.go (2箇所)

**対象行**: 236, 329

**作業内容**:
- [ ] L236: `CreateRuntimeCommandFromSpec` → `CreateRuntimeCommand` + options
- [ ] L329: `CreateRuntimeCommandFromSpec` → `CreateRuntimeCommand` + options
- [ ] テスト実行: `go test -tags test -v ./internal/runner/resource -run TestNormalManager`
- [ ] `make fmt` 実行

#### Phase 1 完了確認

- [ ] internal/runner/resource パッケージの全テスト成功
- [ ] `go test -tags test -v ./internal/runner/resource`

---

### Phase 2: test/performance パッケージ

#### 2.1 output_capture_test.go (7箇所)

**対象行**: 41, 118, 181, 235, 301, 326, 372

**作業内容**:
- [ ] L41: `CreateRuntimeCommandFromSpec` → `CreateRuntimeCommand` + options
- [ ] L118: `CreateRuntimeCommandFromSpec` → `CreateRuntimeCommand` + options
- [ ] L181: `CreateRuntimeCommandFromSpec` → `CreateRuntimeCommand` + options
- [ ] L235: `CreateRuntimeCommandFromSpec` → `CreateRuntimeCommand` + options
- [ ] L301: `CreateRuntimeCommandFromSpec` → `CreateRuntimeCommand` + options
- [ ] L326: `CreateRuntimeCommandFromSpec` → `CreateRuntimeCommand` + options
- [ ] L372: `CreateRuntimeCommandFromSpec` → `CreateRuntimeCommand` + options
- [ ] テスト実行: `go test -tags test -v ./test/performance -run TestOutputCapture`
- [ ] `make fmt` 実行

**変換例**:
```go
// Before
runtimeCmd := executortesting.CreateRuntimeCommandFromSpec(cmdSpec)

// After (cmdSpec の内容に応じて)
runtimeCmd := executortesting.CreateRuntimeCommand(
    cmdSpec.Name,
    cmdSpec.Cmd,
    executortesting.WithArgs(cmdSpec.Args),
    executortesting.WithOutputFile(cmdSpec.OutputFile),
)
```

#### Phase 2 完了確認

- [ ] test/performance パッケージの全テスト成功
- [ ] `go test -tags test -v ./test/performance`

---

### Phase 3: test/security パッケージ

#### 3.1 output_security_test.go (8箇所)

**対象行**: 79, 136, 212, 266, 312, 375, 439, 500

**作業内容**:
- [ ] L79: `CreateRuntimeCommandFromSpec` → `CreateRuntimeCommand` + options
- [ ] L136: `CreateRuntimeCommandFromSpec` → `CreateRuntimeCommand` + options
- [ ] L212: `CreateRuntimeCommandFromSpec` → `CreateRuntimeCommand` + options
- [ ] L266: `CreateRuntimeCommandFromSpec` → `CreateRuntimeCommand` + options
- [ ] L312: `CreateRuntimeCommandFromSpec` → `CreateRuntimeCommand` + options
- [ ] L375: `CreateRuntimeCommandFromSpec` → `CreateRuntimeCommand` + options
- [ ] L439: `CreateRuntimeCommandFromSpec` → `CreateRuntimeCommand` + options
- [ ] L500: `CreateRuntimeCommandFromSpec` → `CreateRuntimeCommand` + options
- [ ] テスト実行: `go test -tags test -v ./test/security -run TestOutputSecurity`
- [ ] `make fmt` 実行

**変換例**:
```go
// Before (L79)
cmdSpec := &runnertypes.CommandSpec{
    Name:       "path_traversal_test",
    Cmd:        "echo",
    Args:       []string{"test output"},
    OutputFile: tc.outputPath,
}
runtimeCmd := executortesting.CreateRuntimeCommandFromSpec(cmdSpec)

// After
runtimeCmd := executortesting.CreateRuntimeCommand(
    "path_traversal_test",
    "echo",
    executortesting.WithArgs([]string{"test output"}),
    executortesting.WithOutputFile(tc.outputPath),
)
```

#### Phase 3 完了確認

- [ ] test/security パッケージの全テスト成功
- [ ] `go test -tags test -v ./test/security`

---

## 最終確認

### 統合テスト

- [ ] 全テスト実行: `make test`
- [ ] Linter チェック: `make lint`
- [ ] コードフォーマット: `make fmt`

### 移行完了確認

- [ ] 全41箇所の移行完了
- [ ] 例外1箇所 (dryrun_manager_test.go:315) は残存していることを確認

### 残存確認

```bash
# CreateRuntimeCommandFromSpec の残存確認
grep -rn "CreateRuntimeCommandFromSpec" internal/runner/resource/*.go test/**/*.go
```

**期待される結果**:
- `internal/runner/resource/dryrun_manager_test.go:315` のみ残存
- 定義ファイル `internal/runner/executor/testing/helpers.go` は除外

- [ ] 残存確認完了

## トラブルシューティング

### テスト失敗時

1. **エラーメッセージを確認**
   - フィールドの欠落がないか
   - options の指定ミスがないか

2. **元のコードと比較**
   ```bash
   git diff <file>
   ```

3. **必要に応じてロールバック**
   ```bash
   git checkout <file>
   ```

### よくあるエラー

#### 1. Args の指定漏れ

**エラー**: テストで期待する引数が渡されていない

**対処**:
```go
// 忘れずに WithArgs を追加
executortesting.WithArgs([]string{"arg1", "arg2"})
```

#### 2. OutputFile の指定漏れ

**エラー**: 出力ファイルが作成されない

**対処**:
```go
// OutputFile がある場合は必ず追加
executortesting.WithOutputFile(outputPath)
```

#### 3. RunAsUser/RunAsGroup の指定漏れ

**エラー**: 権限関連のテストが失敗

**対処**:
```go
executortesting.WithRunAsUser("testuser"),
executortesting.WithRunAsGroup("testgroup"),
```

## 完了報告

全ての作業が完了したら、以下を確認:

- [ ] 全41箇所の移行完了
- [ ] `make test` 成功
- [ ] `make lint` エラーなし
- [ ] `make fmt` 適用済み
- [ ] git status で変更ファイル確認

## 進捗トラッキング

### 全体進捗

- Phase 1 (internal/runner/resource): 0/26 (0%)
  - security_test.go: 0/4
  - error_scenarios_test.go: 0/6
  - usergroup_dryrun_test.go: 0/6
  - performance_test.go: 0/3
  - dryrun_manager_test.go: 0/2
  - integration_test.go: 0/3
  - normal_manager_test.go: 0/2

- Phase 2 (test/performance): 0/7 (0%)
  - output_capture_test.go: 0/7

- Phase 3 (test/security): 0/8 (0%)
  - output_security_test.go: 0/8

**総計**: 0/41 (0%)
