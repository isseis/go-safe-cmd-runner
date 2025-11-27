# グループ展開時のコマンド事前展開 - 要件定義書

## 1. 概要

### 1.1 背景

現在のアーキテクチャでは、コマンドの変数展開が以下の2つのタイミングで実行されている：

1. **検証時** (`collectVerificationFiles` 内)
   - タイミング: グループのファイル検証フェーズ
   - 展開範囲: `cmd` フィールドのみ
   - 利用可能な変数: グループレベルまでの変数
   - 目的: コマンドパスの解決とファイルハッシュ検証

2. **実行時** (`config.ExpandCommand` 内)
   - タイミング: 各コマンド実行直前
   - 展開範囲: `cmd`, `args`, `env`, `vars` 全て
   - 利用可能な変数: グループ + コマンドレベルの変数
   - 目的: 完全な `RuntimeCommand` の作成

この設計により、以下の問題が発生している：

#### 問題1: 参照可能な変数の不一致

```toml
[[groups.commands]]
name = "test_cmd"
vars = ["cmd_var=/opt/command"]  # コマンドレベル変数
cmd = "%{cmd_var}/binary"        # この変数を参照
```

- **検証時**: `cmd_var` が未定義 → 警告ログを出力してスキップ
- **実行時**: `cmd_var` が展開される → 正常に実行

ユーザーからすると、なぜ検証時にエラーが出ないのか（または出るのか）が理解しづらい。

#### 問題2: 重複した展開処理

同じコマンドパスを2回展開することで、以下のコストが発生：

- CPU: 文字列処理の重複実行
- メモリ: 一時的な文字列オブジェクトの重複生成
- コード: 類似処理の重複実装とメンテナンスコスト

#### 問題3: 設計の非一貫性

`RuntimeGroup` には既に `Commands []*RuntimeCommand` フィールドが定義されているが、現在は使用されていない。この未使用フィールドの存在は、設計意図の不明確さを示している。

### 1.2 目的

グループ展開時に全コマンドを事前展開することで、以下を達成する：

1. **一貫性**: 検証時も実行時も同じ展開済みコマンドを参照
2. **完全性**: コマンドレベル変数も検証時に利用可能
3. **効率性**: 展開処理を1回のみ実行
4. **シンプル性**: 重複コードの削除とメンテナンス性向上

### 1.3 スコープ

#### 対象範囲 (In Scope)

- `config.ExpandGroup` の拡張（コマンド展開の追加）
- `RuntimeGroup.Commands` フィールドの活用
- `collectVerificationFiles` の修正（展開済みコマンドを使用）
- `executeAllCommands` の修正（展開済みコマンドを使用）
- テストケースの更新

#### 対象外 (Out of Scope)

- コマンドの遅延展開（条件付き実行など）
- `RuntimeGroup` の他のフィールドの変更
- コマンドレベル以外の展開タイミング変更

## 2. 機能要件

### 2.1 グループ展開時のコマンド展開

#### F-001: `config.ExpandGroup` でのコマンド展開

**概要**: グループ展開時に全コマンドを展開し、`RuntimeGroup.Commands` に格納する。

**重要**: `__runner_workdir` 変数との関係

現在、`__runner_workdir` は `group_executor.go` でグループ展開後、作業ディレクトリ解決後に設定される（line 186-189）。このため、コマンド展開を `ExpandGroup` 内で行うと、`__runner_workdir` がまだ未定義の状態になる。

対応方針：
1. **`cmd` フィールドでの `__runner_workdir` 使用を禁止**
   - コマンドパスに作業ディレクトリを含めるユースケースは正当ではない
   - `cmd` で `%{__runner_workdir}` が使用された場合はエラーとする
   - `args` や `env` では引き続き使用可能（これらは実行時に展開される）

2. **展開タイミングの調整**
   - `ExpandGroup` では `__runner_workdir` なしでコマンド展開を実行
   - `ExecuteGroup` で作業ディレクトリ解決後に `__runner_workdir` を設定
   - コマンド展開は `ExpandGroup` の直後、`__runner_workdir` 設定の直後に移動

**処理フロー**:
```
# ExpandGroup 内（変更なし）
ExpandGroup(groupSpec, globalRuntime)
  ↓
1. グループレベル変数展開（既存）
  ↓
2. cmd_allowed 展開（既存）
  ↓
3. RuntimeGroup を返す（コマンド展開は行わない）

# ExecuteGroup 内（変更）
ExecuteGroup(groupSpec, runtimeGlobal)
  ↓
1. config.ExpandGroup (既存)
  ↓
2. Working Directory 解決（既存）
  ↓
3. __runner_workdir 設定（既存）
  ↓
4. 全コマンドを展開（★新規★）
   for each command in groupSpec.Commands:
     runtimeCmd = ExpandCommand(command, runtimeGroup, runtimeGlobal, ...)
     workDir = resolveCommandWorkDir(runtimeCmd, runtimeGroup)  ← ★追加★
     runtimeCmd.EffectiveWorkDir = workDir  ← ★追加★
     runtimeGroup.Commands.append(runtimeCmd)
  ↓
5. verifyGroupFiles（既存、展開済みコマンドを使用）
  ↓
6. executeAllCommands（既存、展開済みコマンドを使用）
```

**変更ファイル**:
- `internal/runner/group_executor.go` の `ExecuteGroup` 関数
  - 作業ディレクトリ解決と `__runner_workdir` 設定の後に、コマンド展開を追加

**入力**:
- `groupSpec *runnertypes.GroupSpec` - グループ仕様
- `runtimeGroup *runnertypes.RuntimeGroup` - 展開済みグループ（`__runner_workdir` 設定済み）
- `runtimeGlobal *runnertypes.RuntimeGlobal` - グローバル実行時設定

**出力**:
- `runtimeGroup.Commands []*RuntimeCommand` - 全コマンドが完全に展開済み
  - `ExpandedCmd`, `ExpandedArgs`, `ExpandedEnv` が設定済み
  - `EffectiveTimeout` が解決済み
  - `EffectiveWorkDir` が解決済み ★NEW★

**エラー処理**:
- いずれかのコマンド展開が失敗した場合、`ExecuteGroup` 全体が失敗する
- いずれかのコマンドのworkdir解決が失敗した場合、`ExecuteGroup` 全体が失敗する ★NEW★
- エラーメッセージにはコマンド名とグループ名を含める
- `cmd` フィールドで `__runner_workdir` が使用された場合は、明確なエラーメッセージを返す

**例**:
```go
// Before (現在)
runtimeGroup, err := config.ExpandGroup(groupSpec, globalRuntime)
// runtimeGroup.Commands == nil (未使用)

// After (変更後)
runtimeGroup, err := config.ExpandGroup(groupSpec, globalRuntime)
// runtimeGroup.Commands[0].ExpandedCmd == "/home/user/bin/testcmd"
// runtimeGroup.Commands[0].ExpandedArgs == ["--verbose", "/tmp/output.txt"]
// runtimeGroup.Commands[0].EffectiveWorkDir == "/tmp/scr-xxx"  ← ★追加★
```

### 2.2 検証時の展開済みコマンド使用

#### F-002: `collectVerificationFiles` での展開済みコマンド参照

**概要**: ファイル検証時に、展開済みの `RuntimeCommand` からコマンドパスを取得する。

**変更内容**:
```go
// Before (現在)
for _, command := range groupSpec.Commands {
    expandedCmd, err := config.ExpandString(command.Cmd, runtimeGroup.ExpandedVars, ...)
    resolvedPath, err := m.pathResolver.ResolvePath(expandedCmd)
    fileSet[resolvedPath] = struct{}{}
}

// After (変更後)
for _, runtimeCmd := range runtimeGroup.Commands {
    resolvedPath, err := m.pathResolver.ResolvePath(runtimeCmd.ExpandedCmd)
    fileSet[resolvedPath] = struct{}{}
}
```

**変更ファイル**:
- `internal/verification/manager.go` の `collectVerificationFiles` 関数

**メリット**:
- 変数展開処理の削除（既に展開済み）
- コマンドレベル変数も参照可能
- エラーハンドリングのシンプル化

### 2.3 実行時の展開済みコマンド使用

#### F-003: `executeAllCommands` での展開済みコマンド参照

**概要**: コマンド実行時に、展開済みの `RuntimeCommand` を直接使用する。

**変更内容**:
```go
// Before (現在)
for i := range groupSpec.Commands {
    cmdSpec := &groupSpec.Commands[i]
    runtimeCmd, err := config.ExpandCommand(cmdSpec, runtimeGroup, globalRuntime, ...)
    // ... 実行処理 ...
}

// After (変更後)
for i, runtimeCmd := range runtimeGroup.Commands {
    // 既に展開済み - config.ExpandCommand の呼び出し不要
    // ... 実行処理 ...
}
```

**変更ファイル**:
- `internal/runner/group_executor.go` の `executeAllCommands` 関数

**メリット**:
- `config.ExpandCommand` 呼び出しの削除
- ループ内での変数展開エラー処理が不要
- コードの簡素化

## 3. 非機能要件

### 3.1 性能 (Performance)

#### NF-001: メモリ使用量の増加制限

**要件**: 全コマンドを事前展開することによるメモリ使用量の増加を許容範囲内に抑える。

**測定基準**:
- 100コマンドのグループで、メモリ増加量が 1MB 未満であること
- 既存のベンチマークテストでパフォーマンス劣化がないこと

**理由**:
- 通常のユースケースでは、1グループあたり数個〜数十個のコマンド
- `RuntimeCommand` のメモリフットプリントは小さい（主に文字列とマップ）

#### NF-002: 展開処理時間の短縮

**要件**: グループ展開時に全コマンドを展開するが、総実行時間は現在と同等かそれ以下であること。

**期待値**:
- 検証時の展開処理削除により、総実行時間は短縮される
- グループ展開時間の増加は、検証・実行での削減で相殺される

#### NF-002a: Dry-runモードとの互換性

**要件**: Dry-runモードでも事前展開されたコマンドが正しく動作すること。

**現在のDry-run実装**:
```go
// internal/runner/resource/dryrun_manager.go:150-180
func (d *DryRunResourceManager) ExecuteCommand(ctx context.Context, cmd *runnertypes.RuntimeCommand, ...) {
    analysis, err := d.analyzeCommand(ctx, cmd, group, env)
    // ...
}

// internal/runner/resource/dryrun_manager.go:200-268
func (d *DryRunResourceManager) analyzeCommand(_ context.Context, cmd *runnertypes.RuntimeCommand, ...) {
    analysis := ResourceAnalysis{
        Target:    cmd.ExpandedCmd,           // ← 展開済みフィールドを使用
        Parameters: map[string]ParameterValue{
            "command":           NewStringValue(cmd.ExpandedCmd),
            "working_directory": NewStringValue(cmd.EffectiveWorkDir),
            // ...
        },
    }
    // セキュリティ分析でも展開済みフィールドを使用
    d.analyzeCommandSecurity(cmd, &analysis)
}

// internal/runner/resource/dryrun_manager.go:272-294
func (d *DryRunResourceManager) analyzeCommandSecurity(cmd *runnertypes.RuntimeCommand, ...) {
    resolvedPath, err := d.pathResolver.ResolvePath(cmd.ExpandedCmd)  // ← 展開済みコマンド使用
    // ...
    security.AnalyzeCommandSecurity(resolvedPath, cmd.ExpandedArgs, opts)  // ← 展開済み引数使用
}
```

**影響分析**:

1. **現在の動作**:
   - Dry-runモードでは`group_executor.go:233`で`config.ExpandCommand`を呼び出し
   - 展開された`RuntimeCommand`を`DryRunResourceManager.ExecuteCommand`に渡す
   - `analyzeCommand`は既に展開済みの`cmd.ExpandedCmd`、`cmd.ExpandedArgs`を使用

2. **事前展開後の動作**:
   - `ExecuteGroup`で全コマンドを事前展開（dry-runモード含む）
   - `executeAllCommands`で展開済みコマンドを`DryRunResourceManager.ExecuteCommand`に渡す
   - `analyzeCommand`は引き続き展開済みフィールドを使用

3. **互換性評価**:
   - ✅ **完全互換**: Dry-runモードは既に展開済み`RuntimeCommand`を前提に設計されている
   - ✅ **動作変更なし**: 展開タイミングが早まるだけで、dry-run処理には影響なし
   - ✅ **追加メリット**: コマンド展開エラーがdry-run開始前に検出される

**Verification Managerへの影響**:

検証フェーズでもdry-runモードは同様に動作：
```go
// 現在: verification/manager.go:298-324
for _, command := range groupSpec.Commands {
    expandedCmd, err := config.ExpandString(command.Cmd, runtimeGroup.ExpandedVars, ...)
    if err != nil {
        slog.Warn("Failed to expand command path", ...)
        continue  // ← dry-runモードでも警告のみ
    }
    resolvedPath, err := m.pathResolver.ResolvePath(expandedCmd)
    // ...
}

// 事前展開後
for _, runtimeCmd := range runtimeGroup.Commands {
    resolvedPath, err := m.pathResolver.ResolvePath(runtimeCmd.ExpandedCmd)
    // ← 既に展開済み、エラーハンドリング不要
}
```

**Dry-runモードでの変更点**:
- ✅ 検証フェーズでの展開処理が不要になる（dry-run、通常モード共通）
- ✅ 展開エラーはExecuteGroupの早期段階で発生（dry-runでも同様）
- ✅ より早い段階でのエラー検出が可能

**確認項目**:
- Dry-runモードの全テストケースが通ること
- セキュリティ分析が正しく動作すること
- エラー発生時のメッセージが適切であること
- Verification Managerのdry-run動作が変わらないこと

### 3.2 互換性 (Compatibility)

#### NF-003: 既存設定ファイルとの互換性

**要件**: 既存の TOML 設定ファイルが正常に動作すること。

**重要な動作変更**:

現在の実装では、検証フェーズでのコマンドパス展開エラーは**警告として処理**され、実行は継続される：
```go
// 現在の動作（manager.go:305-310）
if err != nil {
    slog.Warn("Failed to expand command path", ...)
    continue  // ← 警告を出して次のコマンドへ
}
```

変更後は、コマンド展開エラーで**グループ全体が失敗**する：
```go
// 変更後の動作
if err != nil {
    return fmt.Errorf("failed to expand command[%s]: %w", ...)  // ← エラーで即座に終了
}
```

**影響**:
- **より厳格なエラーチェック**: コマンド展開エラーを早期に検出（Fail Fast）
- **動作の一貫性**: 検証エラーと展開エラーが同じレベルで扱われる
- **既存設定への影響**: 現在警告で済んでいる設定がエラーになる可能性

**確認項目**:
- 全ての既存テストケースが変更なしで通ること
- サンプル設定ファイルが正常に動作すること
- **検証フェーズで警告が出ている設定がないことを確認**
  - 警告ログを全て調査
  - 該当する設定があれば修正が必要

**互換性の判断**:
この動作変更は**意図的**であり、以下の理由から正当化される：
1. **セキュリティ向上**: 不正な設定を早期に検出
2. **デバッグ容易性**: エラーが明確になる
3. **一貫性**: 警告で済ませる合理的な理由がない

#### NF-004: API 互換性の維持

**要件**: 公開 API のシグネチャを変更しないこと。

**対象**:
- `config.ExpandGroup` のシグネチャは変更なし（コマンド展開は `ExecuteGroup` で実行）
- `VerifyGroupFiles` のシグネチャは変更なし
- `ExecuteGroup` のシグネチャは変更なし

**注意**:
- 当初は `ExpandGroup` でコマンド展開を行う予定だったが、`__runner_workdir` 変数の設定タイミングとの関係により、`ExecuteGroup` 内で展開することに変更

### 3.3 保守性 (Maintainability)

#### NF-005: コードの簡素化

**要件**: 重複コードを削減し、保守性を向上させること。

**測定基準**:
- 検証時の変数展開コードの削除（約20行）
- 実行時の `config.ExpandCommand` 呼び出し削除（約10行）
- 類似処理の統一によるバグ修正箇所の削減

#### NF-006: テストカバレッジの維持

**要件**: 変更後もテストカバレッジを維持すること。

**確認項目**:
- 既存のテストがすべて通ること
- カバレッジが低下しないこと
- 新規追加コードに対する適切なテストの追加

### 3.4 セキュリティ (Security)

#### NF-007: セキュリティレベルの維持

**要件**: 変更によってセキュリティレベルが低下しないこと。

**確認項目**:
- コマンドパスの検証が正しく行われること
- 変数展開のセキュリティチェックが維持されること
- `cmd_allowed` の検証が正常に動作すること

## 4. 技術的制約

### 4.1 実装上の制約

#### C-001: `RuntimeGroup.Commands` フィールドの使用

**制約**: 既存の `RuntimeGroup.Commands []*RuntimeCommand` フィールドを活用すること。

**理由**:
- フィールドは既に定義されているが未使用
- 新規フィールド追加は不要
- 設計意図を明確化

#### C-002: `__runner_workdir` 変数の扱い

**制約**: コマンド展開は `ExecuteGroup` 内で、`__runner_workdir` 設定後に行うこと。

**理由**:
- `__runner_workdir` は作業ディレクトリ解決後に設定される（`group_executor.go:186-189`）
- グループ展開時点では `__runner_workdir` がまだ存在しない
- コマンド展開時には `__runner_workdir` が利用可能である必要がある

**追加制約**:
- `cmd` フィールドで `__runner_workdir` を使用した場合はエラーとする
  - コマンドパスに作業ディレクトリを含める正当なユースケースは存在しない
  - エラーメッセージで明確に禁止事項を伝える

#### C-003: エラーハンドリングの一貫性

**制約**: コマンド展開エラーは、`ExecuteGroup` の早期段階で発生させること。

**理由**:
- Fail Fast 原則（早期にエラーを検出）
- エラー発生箇所の明確化
- デバッグの容易性

### 4.2 既存コードへの影響

#### C-004: 段階的な移行

**制約**: 変更は段階的に行い、各ステップでテストが通ること。

**移行ステップ**:
1. `ExecuteGroup` にコマンド展開を追加（`__runner_workdir` 設定後）
2. `collectVerificationFiles` を展開済みコマンド使用に変更
3. `executeAllCommands` を展開済みコマンド使用に変更
4. 古いコードの削除とリファクタリング

## 5. テスト要件

### 5.1 単体テスト

#### T-001: `ExecuteGroup` のコマンド展開テスト

**テスト項目**:
- 正常系: 複数コマンドが正しく展開されること
- 異常系: コマンド展開エラー時に適切なエラーが返されること
- 変数: グループ・コマンドレベル変数が正しく展開されること
- `__runner_workdir`: 展開時に `__runner_workdir` が利用可能であること
- 制約: `cmd` で `__runner_workdir` を使用した場合にエラーになること

#### T-002: `collectVerificationFiles` の変更テスト

**テスト項目**:
- 展開済みコマンドからパスが正しく収集されること
- コマンドレベル変数を使用したコマンドが検証できること
- エラー時の挙動が適切であること

#### T-003: `executeAllCommands` の変更テスト

**テスト項目**:
- 展開済みコマンドが正しく実行されること
- 既存のテストケースがすべて通ること

### 5.2 統合テスト

#### T-004: エンドツーエンドテスト

**テスト項目**:
- 設定ファイル読み込み → グループ展開 → 検証 → 実行の全フロー
- コマンドレベル変数を使用した設定での検証・実行
- 複数グループ、複数コマンドでの動作

### 5.3 性能テスト

#### T-005: ベンチマークテスト

**テスト項目**:
- 既存ベンチマークでパフォーマンス劣化がないこと
- メモリ使用量の測定

## 6. 実装計画

### 6.1 実装順序

1. **Phase 1**: `ExecuteGroup` でのコマンド展開追加
   - `__runner_workdir` 設定後にコマンド展開ロジックを追加
   - `cmd` フィールドで `__runner_workdir` 使用時のエラーハンドリング追加
   - 単体テストの追加

2. **Phase 2**: `collectVerificationFiles` の修正
   - 展開済みコマンド使用への変更
   - 単体テストの更新

3. **Phase 3**: `executeAllCommands` の修正
   - 展開済みコマンド使用への変更
   - `config.ExpandCommand` 呼び出しの削除
   - 統合テストの更新

4. **Phase 4**: リファクタリングとクリーンアップ
   - 検証時の変数展開コードの削除
   - 不要なエラーハンドリングの削除
   - ドキュメントの更新

### 6.2 リスクと対策

#### Risk-001: メモリ使用量の増加

**リスク**: 全コマンドを事前展開することでメモリが増加する。

**対策**:
- ベンチマークテストで測定
- 実際のユースケースでは影響が小さいことを確認
- 必要に応じて遅延展開も検討（将来の拡張）

#### Risk-002: `__runner_workdir` タイミング問題

**リスク**: `cmd` フィールドで `__runner_workdir` を使用している既存設定がエラーになる可能性。

**対策**:
- 既存の設定ファイル（サンプル、テスト）を全て確認
- `cmd` で `__runner_workdir` を使用している例がないことを確認
- エラーメッセージで代替方法を明示

**確認結果**:
- `__runner_workdir` は主に `args` や `workdir` で使用される
- `cmd` での使用例は見つかっていない（正当なユースケースがない）

#### Risk-003: 既存動作への影響

**リスク**: 変更により既存の動作が変わる可能性。

**対策**:
- 全テストケースを実行して互換性確認
- 段階的な移行でリスクを最小化
- 詳細なコードレビュー

## 7. 用語集

- **変数展開**: `%{variable}` 形式の変数参照を実際の値に置き換える処理
- **グループ展開**: `GroupSpec` から `RuntimeGroup` を作成する処理（変数展開を含む）
- **コマンド展開**: `CommandSpec` から `RuntimeCommand` を作成する処理（変数展開を含む）
- **事前展開**: 実行前（グループ展開時）に全コマンドを展開すること
- **RuntimeGroup**: 実行時に使用される、変数展開済みのグループ情報
- **RuntimeCommand**: 実行時に使用される、変数展開済みのコマンド情報

## 8. 参考資料

### 8.1 関連ファイル

- `internal/runner/config/expansion.go` - 変数展開ロジック
- `internal/runner/runnertypes/runtime.go` - Runtime 型定義
- `internal/verification/manager.go` - ファイル検証ロジック
- `internal/runner/group_executor.go` - グループ実行ロジック

### 8.2 関連タスク

- Task 0060: グループレベル cmd_allowed の実装
- Task 0030: ファイル変数展開の検証

### 8.3 設計上の議論

この要件定義書は、以下の議論に基づいて作成された：

1. **問題の発見**: 検証時にコマンドレベル変数が参照できない
2. **根本原因の分析**: 展開タイミングの違いによる不一致
3. **解決策の検討**: 事前展開 vs 遅延展開 vs 二重展開の継続
4. **設計の選択**: 事前展開（既存フィールド活用、一貫性・効率性重視）
