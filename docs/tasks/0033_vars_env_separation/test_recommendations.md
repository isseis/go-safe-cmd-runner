# テスト推奨事項: vars-env分離 (Task 0033)

**目的**: 検証で特定されたテストカバレッジギャップに対処するための詳細な推奨事項

---

## 優先度マトリクス

| 優先度 | テスト領域 | 件数 | 見積工数 | PRをブロックすべきか? |
|----------|-----------|-------|-------------|------------------|
| 🔴 **クリティカル** | Allowlist強制 | 5テスト | 3-4時間 | **はい** |
| 🔴 **クリティカル** | セキュリティ統合 | 3テスト | 4-5時間 | **はい** |
| 🔴 **クリティカル** | 環境変数優先順位E2E | 4テスト | 3-4時間 | **はい** |
| 🟡 **高** | コマンドenv展開 | 5テスト | 2-3時間 | **推奨** |
| 🟡 **高** | Verify Files展開 | 8テスト | 2-3時間 | **推奨** |
| 🟢 **中** | 自己参照 | 4テスト | 2-3時間 | **検討** |
| ⚫ **低** | パフォーマンスベンチマーク | 9ベンチマーク | 2-3時間 | **いいえ** |

**クリティカルパス合計**: 10-13時間
**高優先度合計**: 4-6時間
**中優先度合計**: 2-3時間

---

## クリティカル優先度: Allowlist強制テスト

### ギャップ分析

**削除されたファイル**: `internal/runner/config/allowlist_violation_test.go` (289行)
**削除されたテスト**: 5つのセキュリティクリティカルテスト
**現在のカバレッジ**: ❌ **なし**

### なぜクリティカルか

Allowlist強制は以下を行う**コアセキュリティ機能**:
- 機密環境変数への不正アクセスを防止
- 複数レベル(global、group、command)でセキュリティポリシーを強制
- verify_filesパスを不正な変数参照から保護
- エッジケース(空のallowlist、継承、上書き)を処理する必要がある

**これらのテストがないと**:
- セキュリティリグレッションが検出されない
- Allowlistバイパス脆弱性が存在する可能性
- 本番環境のデプロイがリスクにさらされる

### 推奨テスト

#### テスト1: `TestAllowlistViolation_Global`

**目的**: globalのfrom_envでのallowlist強制を検証

**テストシナリオ**:
```go
// テストケース:
1. 許可された変数参照 - 成功すべき
   from_env: [["SAFE_VAR", "HOME"]]
   env_allowlist: ["HOME", "USER"]
   期待: 成功

2. 禁止された変数参照 - 失敗すべき
   from_env: [["DANGER", "SECRET_KEY"]]
   env_allowlist: ["HOME", "USER"]
   期待: allowlist違反メッセージを含むエラー

3. 空のallowlistはすべてをブロック - 失敗すべき
   from_env: [["VAR", "HOME"]]
   env_allowlist: []
   期待: エラー

4. 許可された名前で未定義のシステム変数 - 異なる失敗
   from_env: [["VAR", "NONEXISTENT"]]
   env_allowlist: ["NONEXISTENT"]
   期待: エラー(システム変数が設定されていない)

5. 複数の参照、1つが禁止 - 失敗すべき
   from_env: [["A", "HOME"], ["B", "SECRET"]]
   env_allowlist: ["HOME"]
   期待: 2番目の参照でエラー
```

**ファイル場所**: `internal/runner/config/allowlist_test.go` (新規ファイル)

**依存関係**: %{VAR}構文を使用する必要がある

**見積時間**: 45分

---

#### テスト2: `TestAllowlistViolation_Group`

**目的**: 継承を伴うgroupのfrom_envでのallowlist強制を検証

**テストシナリオ**:
```go
// テストケース:
1. globalのallowlistを継承 - 許可
   group.from_env: [["VAR", "HOME"]]
   group.env_allowlist: nil (継承)
   global.env_allowlist: ["HOME", "USER"]
   期待: 成功

2. globalのallowlistを継承 - 禁止
   group.from_env: [["VAR", "SECRET"]]
   group.env_allowlist: nil
   global.env_allowlist: ["HOME"]
   期待: エラー

3. globalのallowlistを上書き - 今は許可
   group.from_env: [["VAR", "SECRET"]]
   group.env_allowlist: ["SECRET"]
   global.env_allowlist: ["HOME"]
   期待: 成功

4. globalのallowlistを上書き - 今は禁止
   group.from_env: [["VAR", "HOME"]]
   group.env_allowlist: ["SECRET"]
   global.env_allowlist: ["HOME", "SECRET"]
   期待: エラー

5. 空のgroupのallowlistはすべてをブロック
   group.from_env: [["VAR", "HOME"]]
   group.env_allowlist: []
   global.env_allowlist: ["HOME"]
   期待: エラー
```

**ファイル場所**: `internal/runner/config/allowlist_test.go`

**見積時間**: 1時間

---

#### テスト3: `TestAllowlistViolation_VerifyFiles`

**目的**: verify_filesパスでのallowlist強制を検証

**テストシナリオ**:
```go
// テストケース:
1. 許可された変数を持つglobalのverify_files
   global.verify_files: ["/path/to/%{HOME}/file"]
   global.from_env: [["HOME", "HOME"]]
   global.env_allowlist: ["HOME"]
   期待: 成功、パスが正しく展開される

2. 禁止された変数を持つglobalのverify_files
   global.verify_files: ["/path/%{SECRET}/file"]
   global.from_env: [["SECRET", "SECRET_KEY"]]
   global.env_allowlist: ["HOME"]
   期待: エラー

3. 継承されたallowlistを持つgroupのverify_files
   group.verify_files: ["/path/%{VAR}/file"]
   group.from_env: [["VAR", "USER"]]
   group.env_allowlist: nil
   global.env_allowlist: ["USER"]
   期待: 成功

4. 上書きされたallowlistを持つgroupのverify_files
   group.verify_files: ["/path/%{VAR}/file"]
   group.from_env: [["VAR", "SECRET"]]
   group.env_allowlist: ["SECRET"]
   global.env_allowlist: ["HOME"]
   期待: 成功

5. 複数のパス、1つに禁止された変数
   verify_files: ["/a/%{HOME}/f", "/b/%{SECRET}/f"]
   期待: 2番目のパスでエラー
```

**ファイル場所**: `internal/runner/config/allowlist_test.go`

**見積時間**: 1時間

---

#### テスト4: `TestAllowlistViolation_ProcessEnv`

**目的**: env値が禁止された変数を参照できないことを検証

**テストシナリオ**:
```go
// テストケース:
1. 許可された内部変数を参照するenv
   vars: [["myvar", "value"]]
   env: [["MY_ENV", "%{myvar}"]]
   期待: 成功

2. 許可されたシステムenvから来たvarsを参照するenv
   from_env: [["safe", "HOME"]]
   env_allowlist: ["HOME"]
   env: [["MY_ENV", "%{safe}"]]
   期待: 成功

3. システムenvを直接参照しようとするenv(展開で失敗すべき)
   env: [["MY_ENV", "%{HOME}"]]
   期待: 未定義変数エラー(HOMEは内部varsにない)

4. allowlistを尊重する複雑なチェーン
   from_env: [["a", "HOME"]]
   vars: [["b", "%{a}/subdir"]]
   env: [["MY_ENV", "%{b}"]]
   env_allowlist: ["HOME"]
   期待: 成功
```

**ファイル場所**: `internal/runner/config/allowlist_test.go`

**見積時間**: 45分

---

#### テスト5: `TestAllowlistViolation_EdgeCases`

**目的**: エッジケースと複雑なシナリオをテスト

**テストシナリオ**:
```go
// テストケース:
1. ワイルドカードを含むallowlist(サポートされている場合)
2. allowlistマッチングの大文字小文字区別
3. 特殊文字を含むallowlist
4. 非常に長いallowlist(パフォーマンス)
5. globalとgroupの間でallowlistが変更される
6. 同じシステム変数を参照する複数のfrom_env
7. 循環allowlistの影響(もしあれば)
```

**ファイル場所**: `internal/runner/config/allowlist_test.go`

**見積時間**: 30分

---

### 実装ガイド

**ファイル構造**:
```
internal/runner/config/
  allowlist_test.go  (新規ファイル)
```

**テストパターン**:
```go
func TestAllowlistViolation_Global(t *testing.T) {
    tests := []struct {
        name        string
        fromEnv     [][]string
        allowlist   []string
        expectError bool
        errorMatch  string // エラーメッセージの正規表現パターン
    }{
        {
            name: "許可された変数",
            fromEnv: [][]string{{"SAFE_VAR", "HOME"}},
            allowlist: []string{"HOME", "USER"},
            expectError: false,
        },
        {
            name: "禁止された変数",
            fromEnv: [][]string{{"DANGER", "SECRET_KEY"}},
            allowlist: []string{"HOME", "USER"},
            expectError: true,
            errorMatch: "not in allowlist",
        },
        // ... その他のテストケース
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // テスト環境のセットアップ
            t.Setenv("HOME", "/home/test")
            t.Setenv("SECRET_KEY", "secret")

            // configを作成
            cfg := &runnertypes.GlobalConfig{
                FromEnv: tt.fromEnv,
                EnvAllowlist: tt.allowlist,
            }

            // 展開を実行
            err := config.ProcessGlobalFromEnv(cfg, envVars)

            // アサート
            if tt.expectError {
                require.Error(t, err)
                if tt.errorMatch != "" {
                    require.Regexp(t, tt.errorMatch, err.Error())
                }
            } else {
                require.NoError(t, err)
            }
        })
    }
}
```

**依存関係**:
- 現在の`ProcessGlobalFromEnv`、`ProcessGroupFromEnv`関数を使用
- テストデータで%{VAR}構文を使用
- テスト環境セットアップに`t.Setenv()`を使用

**検証**:
- すべてのテストがパスする
- `-race`フラグで実行
- エラーメッセージが明確で実用的であることを確認
- `go test -cover`でテストカバレッジを検証

---

## クリティカル優先度: セキュリティ統合テスト

### ギャップ分析

**削除されたテスト**:
- `TestSecurityIntegration` (expansion_test.go)
- `TestSecurityAttackPrevention` (expansion_test.go)
- `TestRunner_SecurityIntegration` (runner_test.go)

**現在のカバレッジ**: ❌ **なし**

### なぜクリティカルか

セキュリティ統合テストは以下を検証:
- コンポーネント間のエンドツーエンドのセキュリティ動作
- 攻撃防止メカニズム(インジェクション、トラバーサルなど)
- 展開中にセキュリティ境界が維持される
- 複数のセキュリティ機能が正しく連携する

### 推奨テスト

#### テスト1: `TestSecurityIntegration_E2E`

**目的**: エンドツーエンドのセキュリティ検証

**テストシナリオ**:
```go
// テストケース:
1. Allowlist + Redaction統合
   - 許可されたenvからの変数は動作すべき
   - 機密値はログでredactされるべき
   - エラーは機密値を漏らさないべき

2. 変数展開 + コマンド実行セキュリティ
   - 展開されたパスはファイルシステム境界を尊重
   - 変数値を介したコマンドインジェクションなし
   - 変数を介したパストラバーサルなし

3. from_env + allowlist + vars + envチェーン
   - from_envはallowlistを尊重
   - varsはfrom_envを安全に参照可能
   - envはvarsを安全に参照可能
   - チェーン全体でセキュリティが維持される

4. 異なるallowlistを持つ複数のグループ
   - グループAはVAR_Aにアクセス可能
   - グループBはVAR_Bにアクセス可能
   - グループAはいかなる手段でもVAR_Bにアクセス不可
```

**ファイル場所**: `internal/runner/config/security_integration_test.go` (新規ファイル)

**見積時間**: 2時間

---

#### テスト2: `TestSecurityAttackPrevention`

**目的**: 一般的な攻撃ベクトルが防止されることを検証

**テストシナリオ**:
```go
// テストケース:
1. 変数を介したコマンドインジェクション
   vars: [["cmd", "rm -rf /"]]
   cmd: "echo %{cmd}"
   期待: 安全に実行(展開は行われるが、コマンドは単なるecho)

2. 変数を介したパストラバーサル
   vars: [["path", "../../etc/passwd"]]
   verify_files: ["/safe/dir/%{path}"]
   期待: パス解決がこれを安全に処理

3. Allowlistバイパス試行
   - varsチェーンを介して禁止された変数を間接的に参照しようとする
   - 自動変数を上書きしようとする
   - 類似した名前でallowlistをバイパスしようとする

4. 環境変数インジェクション
   - システムenvに悪意のある値
   - 展開が埋め込まれたコマンドを実行しないことを検証
   - 特殊文字が安全に処理されることを検証

5. Redactionバイパス試行
   - エラーメッセージを介して機密値を漏らそうとする
   - 変数名を介して漏らそうとする
   - 循環参照を介して漏らそうとする

6. 予約プレフィックス違反
   - __runner_*変数を上書きしようとする
   - 予約プレフィックスで変数を作成しようとする
```

**ファイル場所**: `internal/runner/config/security_integration_test.go`

**見積時間**: 2-3時間

---

#### テスト3: `TestRunner_SecurityIntegration`

**目的**: runnerレベルでのセキュリティ検証

**テストシナリオ**:
```go
// テストケース:
1. セキュリティ機能を持つ完全なconfig
   - from_env、vars、envを持つconfigをロード
   - allowlistが強制されることを検証
   - redactionがrunnerで動作することを検証
   - 機密envを持つコマンドを実行
   - 出力が適切にredactされることを検証

2. 異なるセキュリティコンテキストを持つ複数のコマンド
   - 制限的なallowlistを持つグループのコマンド1
   - 寛容なallowlistを持つグループのコマンド2
   - コマンド間の分離を検証

3. ランタイムセキュリティチェック
   - セキュリティチェックがランタイムで発生することを検証
   - エラーがセキュリティを意識していることを検証
   - ログが機密データを漏らさないことを検証
```

**ファイル場所**: `internal/runner/runner_security_test.go` (新規ファイル)

**見積時間**: 1-2時間

---

### 実装優先度

1. **TestSecurityIntegration_E2E** - 最高優先度
2. **TestSecurityAttackPrevention** - セキュリティ検証に重要
3. **TestRunner_SecurityIntegration** - フルスタック検証に重要

---

## クリティカル優先度: 環境変数優先順位E2Eテスト

### ギャップ分析

**削除されたテスト** (runner_test.go):
- `TestRunner_EnvironmentVariablePriority`
- `TestRunner_EnvironmentVariablePriority_CurrentImplementation`
- `TestRunner_EnvironmentVariablePriority_EdgeCases`
- `TestRunner_resolveEnvironmentVars`

**現在のカバレッジ**: ❌ **なし** E2Eレベルで

### なぜクリティカルか

環境変数の優先順位は正しい動作の**基本**:
- 優先順位: command env > group env > global env > system env
- 間違った優先順位 = 本番環境で間違った変数値
- ランタイム/E2Eレベルでテストする必要がある(ユニットレベルだけではない)
- エッジケースは微妙なバグを引き起こす可能性

### 推奨テスト

#### テスト1: `TestRunner_EnvironmentVariablePriority_Basic`

**目的**: 基本的な優先順位ルールを検証

**テストシナリオ**:
```go
// テストケース:
1. システムenvのみ
   System: HOME=/home/system
   Global: (なし)
   Group: (なし)
   Command: (なし)
   期待: HOME=/home/system

2. Globalがsystemを上書き
   System: HOME=/home/system
   Global: env=[["HOME", "/home/global"]]
   Group: (なし)
   Command: (なし)
   期待: HOME=/home/global

3. Groupがglobalを上書き
   System: HOME=/home/system
   Global: env=[["HOME", "/home/global"]]
   Group: env=[["HOME", "/home/group"]]
   Command: (なし)
   期待: HOME=/home/group

4. Commandがすべてを上書き
   System: HOME=/home/system
   Global: env=[["HOME", "/home/global"]]
   Group: env=[["HOME", "/home/group"]]
   Command: env=[["HOME", "/home/command"]]
   期待: HOME=/home/command

5. 混合優先順位
   System: A=sys_a, B=sys_b, C=sys_c
   Global: env=[["B", "global_b"]]
   Group: env=[["C", "group_c"]]
   Command: (なし)
   期待: A=sys_a, B=global_b, C=group_c
```

**ファイル場所**: `cmd/runner/integration_test.go` (E2Eテスト)

**見積時間**: 1.5時間

---

#### テスト2: `TestRunner_EnvironmentVariablePriority_WithVars`

**目的**: vars展開を伴う優先順位を検証

**テストシナリオ**:
```go
// テストケース:
1. 下位優先度envを参照するvars
   Global: vars=[["myvar", "%{USER}"]]
           env=[["HOME", "%{myvar}"]]
   System: USER=testuser
   期待: HOME=testuser

2. commandのvarsがgroupを上書き
   Global: vars=[["v", "global"]]
   Group: vars=[["v", "group"]]
   Command: vars=[["v", "command"]]
            env=[["RESULT", "%{v}"]]
   期待: RESULT=command

3. 優先順位を尊重する複雑なチェーン
   Global: from_env=[["gvar", "HOME"]]
           vars=[["gv2", "%{gvar}/global"]]
   Group: vars=[["gv3", "%{gv2}/group"]]
   Command: env=[["FINAL", "%{gv3}/cmd"]]
   System: HOME=/home/test
   期待: FINAL=/home/test/global/group/cmd
```

**ファイル場所**: `cmd/runner/integration_test.go`

**見積時間**: 1時間

---

#### テスト3: `TestRunner_EnvironmentVariablePriority_EdgeCases`

**目的**: エッジケースと異常なシナリオをテスト

**テストシナリオ**:
```go
// テストケース:
1. 異なるレベルでの空値
   Global: env=[["VAR", ""]]
   Group: (VARは設定されていない)
   期待: VAR="" (空、未設定ではない)

2. より高い優先度で未設定
   Global: env=[["VAR", "global_value"]]
   Group: (VARは設定されていない)
   期待: VAR=global_value (上書きされない)

3. 数値と特殊値
   Command: env=[["NUM", "123"], ["SPECIAL", "$pecial!@#"]]
   期待: 正確に保持される

4. 非常に長い値
   Command: env=[["LONG", "<4KB文字列>"]]
   期待: 正確に保持される

5. 多くの変数(パフォーマンス/正確性)
   各レベルで50以上の変数
   期待: すべて正しい優先順位で正しく解決

6. レベル間の循環参照試行
   Global: vars=[["A", "%{B}"]]
   Group: vars=[["B", "%{A}"]]
   期待: エラーが検出される
```

**ファイル場所**: `cmd/runner/integration_test.go`

**見積時間**: 1時間

---

#### テスト4: `TestRunner_ResolveEnvironmentVars_Integration`

**目的**: 変数解決の統合テスト

**ファイル場所**: `cmd/runner/integration_test.go`

**見積時間**: 30分

---

### 実装ガイド

**ファイル場所**: `cmd/runner/integration_test.go` (既存ファイルに追加)

**テストパターン** (E2Eスタイル):
```go
func TestRunner_EnvironmentVariablePriority_Basic(t *testing.T) {
    tests := []struct {
        name           string
        systemEnv      map[string]string
        globalEnv      [][]string
        groupEnv       [][]string
        commandEnv     [][]string
        expectedEnv    map[string]string
    }{
        {
            name: "commandがすべてを上書き",
            systemEnv: map[string]string{"HOME": "/home/system"},
            globalEnv: [][]string{{"HOME", "/home/global"}},
            groupEnv: [][]string{{"HOME", "/home/group"}},
            commandEnv: [][]string{{"HOME", "/home/command"}},
            expectedEnv: map[string]string{"HOME": "/home/command"},
        },
        // ... その他のケース
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // システムenvのセットアップ
            for k, v := range tt.systemEnv {
                t.Setenv(k, v)
            }

            // configを作成
            cfg := createTestConfig(tt.globalEnv, tt.groupEnv, tt.commandEnv)

            // configをロードして実行
            // (これはE2Eなので、実際のrunnerコードを使用)
            runner, err := NewRunner(cfg)
            require.NoError(t, err)

            result, err := runner.ExecuteCommand(ctx, "printenv")
            require.NoError(t, err)

            // 環境をパースして検証
            actualEnv := parseEnvOutput(result.Stdout)
            for k, expectedV := range tt.expectedEnv {
                assert.Equal(t, expectedV, actualEnv[k], "変数 %s", k)
            }
        })
    }
}
```

---

## 実装タイムライン

### 週1: クリティカルテスト

**1-2日目** (6-8時間):
- [ ] Allowlist強制テスト(全5テスト)
- [ ] 基本的なスモークテスト

**3-4日目** (8-10時間):
- [ ] セキュリティ統合テスト(全3テスト)
- [ ] 攻撃防止検証

**5日目** (6-8時間):
- [ ] 環境変数優先順位E2Eテスト(全4テスト)
- [ ] 統合検証

**週1合計**: 20-26時間

### 週2: 高優先度テスト(オプションだが推奨)

**1-2日目** (4-6時間):
- [ ] コマンドenv展開テスト
- [ ] Verify files展開テスト

**3日目** (2-3時間):
- [ ] 自己参照テスト(時間があれば)

**週2合計**: 6-9時間

---

## 成功基準

### 完了の定義

各テストについて:
- [ ] テストが実装されてパスしている
- [ ] すべての指定されたシナリオをカバーしている
- [ ] %{VAR}構文を使用している(${VAR}ではない)
- [ ] 明確なアサーションとエラーメッセージがある
- [ ] `-race`フラグで問題なく実行される
- [ ] 目的とシナリオが文書化されている
- [ ] 関連領域のコードカバレッジが増加している

### 検証チェックリスト

テスト実装後:
- [ ] 完全なテストスイートを実行: `go test ./...`
- [ ] レース検出器で実行: `go test -race ./...`
- [ ] カバレッジをチェック: `go test -cover ./...`
- [ ] 特定のパッケージテストを実行: `go test ./internal/runner/config/...`
- [ ] 既存テストにリグレッションがないことを検証
- [ ] クリティカルシナリオの手動テスト
- [ ] テストシナリオのセキュリティレビュー

---

## 参考資料

### 関連ファイル

**テストファイル**:
- `internal/runner/config/expansion_test.go` - 現在の展開テスト
- `internal/runner/runner_test.go` - 現在のrunnerテスト
- `cmd/runner/integration_test.go` - 統合/E2Eテスト

**実装ファイル**:
- `internal/runner/config/expansion.go` - 展開ロジック
- `internal/runner/config/loader.go` - Config読み込み
- `internal/runner/runner.go` - Runner実装

### Git参照

**キーコミット**: `db64c21` - Switch config variable system to vars + %{VAR} syntax
**前のコミット**: `db64c21~1` - 削除されたテストコードが含まれる

---

## 問い合わせ

これらの推奨事項に関する質問は:
1. テスト検証サマリーをレビュー
2. git履歴で削除されたテストコードを確認
3. vars-env分離設計ドキュメントを参照
4. 開発チームに連絡

---

**ドキュメントバージョン**: 1.0
**最終更新**: 2025-10-15
**ステータス**: 実装準備完了
