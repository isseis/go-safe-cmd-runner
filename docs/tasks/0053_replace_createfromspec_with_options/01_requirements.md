# 要件定義書: CreateRuntimeCommandFromSpec の options パターンへの移行

## 概要

テストコード内の `CreateRuntimeCommandFromSpec` 呼び出しを、より読みやすく意図が明確な `CreateRuntimeCommand` + options パターンに移行する。

## 背景

現在、テストコードでは2つの RuntimeCommand 作成パターンが混在している:

1. **CreateRuntimeCommand + options**: 直接的でオプションの意図が明確
2. **CreateRuntimeCommandFromSpec**: CommandSpec を経由する冗長な方法

`CreateRuntimeCommandFromSpec` は CommandSpec をインラインで作成するケースが多く、中間オブジェクトの作成が冗長になっている。

## 目的

- テストコードの可読性向上
- 意図が明確なオプションパターンへの統一
- 不要な中間オブジェクト (CommandSpec) の削減
- テストヘルパーの使い方の一貫性向上

## 移行方針

### 基本原則

**インラインで CommandSpec を作成している箇所**: `CreateRuntimeCommand` + options に移行

**Before:**
```go
cmd := executortesting.CreateRuntimeCommandFromSpec(&runnertypes.CommandSpec{
    Name:       "test_cmd",
    Cmd:        "echo",
    Args:       []string{"test"},
    RunAsUser:  "testuser",
    RunAsGroup: "testgroup",
})
```

**After:**
```go
cmd := executortesting.CreateRuntimeCommand(
    "echo",
    []string{"test"},
    executortesting.WithName([]string{"test_cmd"}),
    executortesting.WithRunAsUser("testuser"),
    executortesting.WithRunAsGroup("testgroup"),
)
```

### 例外ケース

以下のケースでは `CreateRuntimeCommandFromSpec` を残す:

1. **テストテーブルで CommandSpec を使用**: `&tt.spec` のように、テストテーブルから spec を受け取る場合
2. **CommandSpec の再利用**: 同じ spec を複数箇所で使い回している場合
3. **複雑な spec の構築**: spec の構築ロジックが複雑で、options パターンでは表現しにくい場合

## 対象ファイル

### 移行対象 (インライン CommandSpec)

1. **internal/runner/resource/security_test.go**
   - 4箇所: 97行目, 190行目, 256行目, 321行目

2. **internal/runner/resource/error_scenarios_test.go**
   - 6箇所: 228行目, 305行目, 470行目, 622行目, 685行目, 722行目

3. **internal/runner/resource/usergroup_dryrun_test.go**
   - 6箇所: 27行目, 67行目, 105行目, 140行目, 175行目, 215行目

4. **internal/runner/resource/performance_test.go**
   - 3箇所: 31行目, 150行目, 196行目

5. **internal/runner/resource/dryrun_manager_test.go**
   - 2箇所: 292行目, 355行目
   - 例外: 315行目 (`&tt.spec` を使用)

6. **internal/runner/resource/integration_test.go**
   - 3箇所: 94行目, 146行目, 209行目

7. **internal/runner/resource/normal_manager_test.go**
   - 2箇所: 236行目, 329行目

8. **test/performance/output_capture_test.go**
   - 7箇所: 41行目, 118行目, 181行目, 235行目, 301行目, 326行目, 372行目

9. **test/security/output_security_test.go**
   - 8箇所: 79行目, 136行目, 212行目, 266行目, 312行目, 375行目, 439行目, 500行目

### 例外 (そのまま残す)

- **internal/runner/resource/dryrun_manager_test.go:315** - テストテーブルから `&tt.spec` を使用

## フィールドマッピング

CommandSpec のフィールドを options パターンに変換する際のマッピング:

| CommandSpec フィールド | options パターン |
|----------------------|-----------------|
| Cmd | 第1引数 (必須) |
| Args | 第2引数 (必須) |
| Name | `WithName(string)` |
| WorkDir | `WithWorkDir(string)` |
| Timeout | `WithTimeout(*int)` |
| OutputFile | `WithOutputFile(string)` |
| RunAsUser | `WithRunAsUser(string)` |
| RunAsGroup | `WithRunAsGroup(string)` |

## 期待される効果

### コードの可読性向上

**Before (冗長):**
```go
cmd := executortesting.CreateRuntimeCommandFromSpec(&runnertypes.CommandSpec{
    Name: "test",
    Cmd:  "echo",
    Args: []string{"hello"},
})
```

**After (簡潔):**
```go
cmd := executortesting.CreateRuntimeCommand(
    "echo",
    []string{"hello"},
    executortesting.WithName("test"),
)
```

### オプションの意図が明確

- `WithRunAsUser("testuser")` - ユーザー指定が一目瞭然
- `WithTimeout(&timeout)` - タイムアウト設定が明示的

### 中間オブジェクトの削減

- CommandSpec の作成が不要
- メモリ効率の向上

## 非機能要件

- **後方互換性**: `CreateRuntimeCommandFromSpec` は削除せず、必要な場所で引き続き使用可能
- **テストの動作**: 移行後も全てのテストが正常に動作すること
- **コードフォーマット**: `make fmt` でフォーマットを整えること
- **コード品質**: `make lint` でエラーが出ないこと

## 成功基準

- [ ] 対象の41箇所全てで移行完了
- [ ] 全てのテストが成功 (`make test`)
- [ ] Linter エラーなし (`make lint`)
- [ ] コードフォーマット適用済み (`make fmt`)
- [ ] `CreateRuntimeCommandFromSpec` は1箇所のみ残存 (dryrun_manager_test.go:315)

## リスク

### 低リスク

- 既存の `CreateRuntimeCommand` は十分にテストされている
- 移行は機械的な作業で、ロジック変更なし

### 軽減策

- ファイル単位で移行し、都度テスト実行
- 問題が発生した場合は即座にロールバック可能
