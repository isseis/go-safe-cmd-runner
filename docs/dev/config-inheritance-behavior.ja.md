# 設定の継承・マージ動作

このドキュメントでは、go-safe-cmd-runner における設定項目の階層間での継承・マージ動作を詳細に説明します。

## 概要

runner の設定は以下の4階層に分かれています：

1. **ランタイム** - runner 呼び出し時の環境変数など
2. **Global セクション** - TOML の `[global]` セクション
3. **Groups セクション** - TOML の `[[groups]]` セクション
4. **Commands セクション** - TOML の `[[groups.commands]]` セクション

設定項目によって、これらの階層間での継承・マージ動作が異なります。

## 設定項目の継承・マージ動作比較表

設定項目を**単一値項目**と**複数値項目**に分けて整理します。単一値項目は複数レイヤーで設定された場合に Override（上書き）しか選択肢がない一方、複数値項目は Union（結合）または Override（上書き）などの選択があります。

### 単一値項目

単一値を取る項目は、複数レイヤーで設定されている場合に必ず Override 動作となります。

| 設定項目 | Global | Group | Command | 継承・マージ動作 | 備考 |
|---------|--------|-------|---------|-----------------|------|
| **timeout** | ✓ | - | ✓ | **Override**: Command.Timeout > 0 の場合はそれを使用、それ以外は Global.Timeout を使用 | Command レベルで上書き可能<br>実装: [runner.go:582-586](../../internal/runner/runner.go#L582-L586) |
| **workdir** | ✓ | ✓ | ✓ | **Override**: Command.Dir が空文字列の場合のみ Global.WorkDir を設定 | Group.WorkDir は temp_dir 用途<br>Command.Dir は実行時に使用<br>実装: [runner.go:526-528](../../internal/runner/runner.go#L526-L528) |
| **log_level** | ✓ | - | - | **Global only**: Global.LogLevel のみ定義可能 | Command や Group レベルでは未対応 |
| **max_output_size** | ✓ | - | - | **Global only**: Global.MaxOutputSize のみ定義可能 | Command や Group レベルでは未対応 |
| **skip_standard_paths** | ✓ | - | - | **Global only**: Global.SkipStandardPaths のみ定義可能 | Command や Group レベルでは未対応 |
| **max_risk_level** | - | - | ✓ | **Command only**: Command.MaxRiskLevel のみ定義可能 | Global や Group レベルでは未対応 |
| **run_as_user** | - | - | ✓ | **Command only**: Command.RunAsUser のみ定義可能 | Global や Group レベルでは未対応 |
| **run_as_group** | - | - | ✓ | **Command only**: Command.RunAsGroup のみ定義可能 | Global や Group レベルでは未対応 |
| **output** | - | - | ✓ | **Command only**: Command.Output のみ定義可能 | Global や Group レベルでは未対応 |

### 複数値項目

複数値を取る項目は、Union（結合）または Override（上書き）の選択があります。現在の実装では、項目によって動作が異なります。

| 設定項目 | Global | Group | Command | 継承・マージ動作 | 備考 |
|---------|--------|-------|---------|-----------------|------|
| **env** | - | - | ✓ | **Independent (層間マージなし)**: Command.Env で定義された環境変数のみ使用。複数 Command 間では独立 | 各 Command が独自の env を持つ<br>Union ではなく Independent 動作 |
| **env_allowlist** | ✓ | ✓ | - | **Inherit/Override/Prohibit**: <br>• Group.EnvAllowlist が `nil` → Inherit (Global を継承)<br>• Group.EnvAllowlist が `[]` → Prohibit (すべて拒否)<br>• Group.EnvAllowlist が `["VAR1", ...]` → Override (Group の値のみ使用) | 3つの継承モード<br>**Union ではなく Override** を採用<br>実装: [filter.go:141-153](../../internal/runner/environment/filter.go#L141-L153)<br>型定義: [config.go:121-135](../../internal/runner/runnertypes/config.go#L121-L135) |
| **verify_files** | ✓ | ✓ | - | **Effective Union**: Global と Group で個別に管理されるが、実行時には両方の検証成功が必要。Global 失敗→プログラム終了、Group 失敗→グループスキップ | ユーザー観点では実質的に Union 動作<br>両方の検証成功が Group 実行の前提条件<br>実装: [main.go:129-133](../../cmd/runner/main.go#L129-L133), [runner.go:406-417](../../internal/runner/runner.go#L406-L417) |

#### 複数値項目の設計方針

複数値項目において **Union（結合）を採用しなかった理由**:

1. **env_allowlist**: セキュリティ上の理由から、明示的な制御が必要。Union では意図しない環境変数が継承される可能性がある。Override により、Group レベルで厳格な制御が可能。
2. **env**: 各 Command が独自の環境変数セットを持つことで、Command 間の独立性を確保。
3. **verify_files**: Global と Group で検証対象が異なるため、独立した管理が適切。

## 特記事項

### 1. ランタイム環境変数の扱い

- Runner 呼び出し時の OS 環境変数は `env_allowlist` でフィルタリングされ、Command 実行時に利用可能
- `Filter.ResolveGroupEnvironmentVars()` が Group レベルの `env_allowlist` に基づいてシステム環境変数をフィルタリング
- 実装: [filter.go:114-139](../../internal/runner/environment/filter.go#L114-L139)

### 2. 自動環境変数 (Auto Env)

- `__RUNNER_DATETIME`, `__RUNNER_PID` などの自動生成環境変数は Command.Env より優先
- Command.Env から自動環境変数を参照可能だが、上書きは不可
- 実装: [expansion.go:81-94](../../internal/runner/config/expansion.go#L81-L94)

### 3. env_allowlist の継承モード詳細

[config.go:120-136](../../internal/runner/runnertypes/config.go#L120-L136) で定義されている3つの継承モード：

#### InheritanceModeInherit (継承モード)

- **条件**: Group に `env_allowlist` フィールドが未定義 (TOML に記載なし)
- **動作**: Global の allowlist を継承
- **使用例**:
  ```toml
  [global]
  env_allowlist = ["PATH", "HOME"]

  [[groups]]
  name = "example"
  # env_allowlist を記載しない → Global の ["PATH", "HOME"] を継承
  ```

#### InheritanceModeExplicit (明示モード)

- **条件**: Group に `env_allowlist = ["VAR1", "VAR2"]` のように値を指定
- **動作**: Group の allowlist のみ使用 (Global は無視)
- **使用例**:
  ```toml
  [global]
  env_allowlist = ["PATH", "HOME"]

  [[groups]]
  name = "example"
  env_allowlist = ["USER", "LANG"]  # Global を無視し、この値のみ使用
  ```

#### InheritanceModeReject (拒否モード)

- **条件**: Group に `env_allowlist = []` のように空配列を指定
- **動作**: すべての環境変数アクセスを拒否
- **使用例**:
  ```toml
  [global]
  env_allowlist = ["PATH", "HOME"]

  [[groups]]
  name = "example"
  env_allowlist = []  # すべての環境変数アクセスを拒否
  ```

### 4. verify_files の実行時検証動作

#### 4.1 実行フロー

verify_files の検証は以下の順序で実行されます：

1. **Global 検証** ([main.go:137-145](../../cmd/runner/main.go#L137-L145))
   - プログラム開始時に Global.VerifyFiles の全ファイルを検証
   - **検証失敗 → プログラム全体が終了**

2. **Group 検証** ([runner.go:406-417](../../internal/runner/runner.go#L406-L417))
   - 各グループ実行前に Group.VerifyFiles の全ファイルを検証
   - **検証失敗 → 該当グループをスキップ、他グループは継続実行**

#### 4.2 ユーザー観点での動作

Global と Group の両方に verify_files が設定されている場合：

- **両方の検証に成功** → グループ内コマンドが実行される
- **Global が失敗** → プログラム全体が終了（Group の検証すら行われない）
- **Global が成功、Group が失敗** → 該当グループのコマンドは実行されない

これにより、実質的に **Union** のような動作となります。

#### 4.3 変数展開

verify_files 内のパスに環境変数が含まれる場合、展開に使用する allowlist が階層によって異なります。

#### Global レベル

- **使用する allowlist**: `Global.EnvAllowlist`
- **実装**: [expansion.go:194-216](../../internal/runner/config/expansion.go#L194-L216)
- **例**:
  ```toml
  [global]
  env_allowlist = ["HOME"]
  verify_files = ["${HOME}/.config/app.conf"]  # HOME を使用可能
  ```

#### Group レベル

- **使用する allowlist**: Group の `env_allowlist` 継承ルール (`InheritanceMode`) に従って決定
- **実装**: [expansion.go:218-247](../../internal/runner/config/expansion.go#L218-L247)
- **例**:
  ```toml
  [global]
  env_allowlist = ["HOME", "USER"]

  [[groups]]
  name = "example1"
  # env_allowlist 未指定 → Global の ["HOME", "USER"] を継承
  verify_files = ["${HOME}/.local/bin/app"]  # HOME, USER を使用可能

  [[groups]]
  name = "example2"
  env_allowlist = ["USER"]  # Global を無視
  verify_files = ["${USER}.conf"]  # USER のみ使用可能

  [[groups]]
  name = "example3"
  env_allowlist = []  # すべて拒否
  verify_files = ["app.conf"]  # 変数展開不可 (エラー)
  ```

### 5. timeout の優先順位

- Command.Timeout が 0 より大きい → Command.Timeout を使用
- Command.Timeout が 0 以下 → Global.Timeout を使用
- 実装: [runner.go:582-586](../../internal/runner/runner.go#L582-L586)

```go
timeout := time.Duration(r.config.Global.Timeout) * time.Second
if cmd.Timeout > 0 {
    timeout = time.Duration(cmd.Timeout) * time.Second
}
```

### 6. workdir の優先順位

- Command.Dir が空文字列でない → Command.Dir を使用
- Command.Dir が空文字列 → Global.WorkDir を設定
- 実装: [runner.go:526-528](../../internal/runner/runner.go#L526-L528)

```go
if cmd.Dir == "" {
    cmd.Dir = r.config.Global.WorkDir
}
```

注意: Group.WorkDir は temp_dir 機能で使用されますが、Command の実行ディレクトリには直接影響しません。

## まとめ

### 値の種類による分類

設定項目は**単一値項目**と**複数値項目**に大別されます：

- **単一値項目** (timeout, workdir, log_level など): 複数レイヤーで設定された場合、必ず Override（上書き）動作
- **複数値項目** (env, env_allowlist, verify_files): Union（結合）または Override（上書き）の選択が可能だが、現在の実装ではすべて **Override または Independent** を採用

### 継承・マージ動作のパターン

設定項目の継承・マージ動作は一様ではなく、項目ごとに以下のパターンがあります：

1. **Override パターン** (timeout, workdir): 下位レベルが上位レベルを上書き
2. **Independent パターン** (env): 各レベルで独立して管理（層間でのマージなし）
3. **Effective Union パターン** (verify_files): 設定は独立管理だが、実行時には両方の検証成功が必要
4. **Inherit/Override/Prohibit パターン** (env_allowlist): 3つの継承モードで柔軟に制御
4. **Single-level パターン** (log_level, max_output_size など): 特定のレベルでのみ定義可能

### 設計思想

**複数値項目で Union（結合）を採用しなかった理由**:

1. **セキュリティの明示性**: env_allowlist で Union を使用すると、上位レベルの設定が意図せず継承され、セキュリティリスクが発生する可能性がある
2. **Command の独立性**: env で各 Command が独自の環境変数を持つことで、Command 間の意図しない影響を防止
3. **検証対象の明確性**: verify_files で Global と Group の検証対象を独立管理することで、それぞれのスコープを明確化。ただし、実行時には **Effective Union** として機能し、両方の検証成功がコマンド実行の前提条件となる

この設計により、セキュリティ要件と柔軟性のバランスを実現しています。特に verify_files については、設定管理は独立しつつも、実行時のセキュリティ保証は厳格（Union的動作）にすることで、堅牢性を確保しています。
