# タイムアウト解決コンテキストの強化

## 1. 概要

dry-runモードにおいて、タイムアウト値の解決過程を可視化し、デバッグやトラブルシューティングを容易にする。具体的には、最終的に適用されるタイムアウト値だけでなく、その値がどのレベル（command, group, global, default）で設定されたかを表示する。

## 2. 背景

### 2.1. 現状分析

#### タイムアウト解決の実装状況

現在、タイムアウト値の解決は以下の2つの関数で実装されている：

1. **`ResolveTimeout`** ([internal/common/timeout_resolver.go:24](internal/common/timeout_resolver.go))
   - 戻り値: `(int, TimeoutResolutionContext)`
   - 解決されたタイムアウト値と、その値がどのレベルで設定されたかのコンテキスト情報を返す
   - **現状**: プロダクションコードで使用されていない（テストコードとドキュメントのみ）

2. **`ResolveEffectiveTimeout`** ([internal/common/timeout.go:111](internal/common/timeout.go))
   - 戻り値: `int`
   - 解決されたタイムアウト値のみを返す（コンテキスト情報なし）
   - **現状**: プロダクションコードで実際に使用されている

#### 使用箇所

`ResolveEffectiveTimeout`は[internal/runner/runnertypes/runtime.go:202](internal/runner/runnertypes/runtime.go#L202)の`NewRuntimeCommand`で使用されている：

```go
func NewRuntimeCommand(spec *CommandSpec, globalTimeout common.Timeout) (*RuntimeCommand, error) {
    // ...
    commandTimeout := common.NewFromIntPtr(spec.Timeout)
    effectiveTimeout := common.ResolveEffectiveTimeout(commandTimeout, globalTimeout)

    return &RuntimeCommand{
        // ...
        EffectiveTimeout: effectiveTimeout,
    }, nil
}
```

#### dry-runモードでの表示

[internal/runner/resource/dryrun_manager.go:171](internal/runner/resource/dryrun_manager.go#L171)で、タイムアウト値をParametersに含めているが、**誤った値**を使用している：

```go
Parameters: map[string]any{
    "command":           cmd.ExpandedCmd,
    "working_directory": cmd.EffectiveWorkDir,
    "timeout":           cmd.Timeout(),  // ← 問題：common.Timeout型（構造体）がそのまま表示される
},
```

**問題点**:
1. `cmd.Timeout()`は`common.Timeout`型を返すため、`String()`メソッドが実装されていないと内部構造（`{value:0x...}`）が表示される
2. 解決済みの最終値（`EffectiveTimeout`）ではなく、コマンドレベルの生の設定値のみを返す
3. どのレベルで値が設定されたかのコンテキスト情報が全くない

**正しい実装**:
```go
Parameters: map[string]any{
    "command":           cmd.ExpandedCmd,
    "working_directory": cmd.EffectiveWorkDir,
    "timeout":           cmd.EffectiveTimeout,  // ← 正解：解決済みのint値
},
```

### 2.2. タイムアウト解決の階層構造

現在のタイムアウト解決は以下の優先順位で行われる：

1. **コマンドレベル**: `[[groups.commands]]`の`timeout`（最優先）
2. **グループレベル**: `[[groups]]`の`timeout`（**現在未実装**）
3. **グローバルレベル**: `[global]`の`timeout`
4. **デフォルト**: `DefaultTimeout = 60秒`

### 2.3. 型システムの不整合

`ResolveTimeout`と現在の実装の間に型システムの不整合がある：

- **`ResolveTimeout`**: `*int`を期待（nilで未設定を表現）
- **現在の実装**: `common.Timeout`型を使用（構造化された型安全な表現）

この不整合により、`ResolveTimeout`を直接使用するには型変換が必要。

## 3. 要件

### 3.1. 機能要件

#### FR-1: タイムアウト解決コンテキストの保存
- `RuntimeCommand`は、タイムアウト値の解決過程のコンテキスト情報を保持する
- コンテキスト情報には以下を含む：
  - 解決されたタイムアウト値（秒単位）
  - 値が設定されたレベル（"command", "group", "global", "default"）
  - コマンド名
  - グループ名（該当する場合）

#### FR-2: dry-runモードでの表示
- dry-runモードでは、各コマンドのタイムアウト情報として以下を表示する：
  - `timeout`: 解決済みの最終タイムアウト値（秒単位、int型）
  - `timeout_level`: 値が設定されたレベル（string型）
- 表示は既存の`Parameters`マップに追加する形で実装

#### FR-3: コードの統一と簡素化
- `ResolveEffectiveTimeout`を削除し、`ResolveTimeout`に統一する
- タイムアウト解決ロジックを一元化し、保守性を向上させる
- `EffectiveTimeout`フィールドは引き続き保持（既存の利用箇所があるため）

### 3.2. 非機能要件

#### NFR-1: コードの簡潔性
- 重複したタイムアウト解決ロジックを削除
- 単一の解決関数（`ResolveTimeout`）に統一

#### NFR-2: 型安全性
- `common.Timeout`型を活用し、型安全な実装を維持
- nilポインタ参照のリスクを避ける

#### NFR-3: テスト容易性
- 解決ロジックは独立してテスト可能
- dry-run出力のテストが容易

## 4. 制約事項

### 4.1. グループレベルタイムアウトの未サポート
- 現在、グループレベルのタイムアウト設定は未実装
- `ResolveTimeout`はグループレベルのパラメータを持つが、常に`nil`が渡される
- 将来の拡張に備えた設計とする

### 4.2. 既存のResolveTimeout関数の活用
- `ResolveTimeout`は既に実装されているが、`*int`を期待する
- `common.Timeout`型との型変換が必要
- 将来的には`common.Timeout`型を受け取るオーバーロード版の実装を検討

## 5. 対象外

以下は本タスクの対象外とする：

1. **グループレベルタイムアウトの実装**: 別タスクとして扱う
2. **タイムアウト解決ロジックの変更**: 既存ロジックは変更しない
3. **通常実行モードでのコンテキスト表示**: dry-runモードのみを対象

## 6. 成果物

1. 更新された`RuntimeCommand`構造体（`TimeoutResolution`フィールド追加）
2. `NewRuntimeCommand`の更新（`ResolveTimeout`を使用）
3. `ResolveEffectiveTimeout`関数の削除とその呼び出し箇所の更新
4. dry-run出力の更新（`timeout_level`の表示）
5. 単体テストと統合テストの更新
6. アーキテクチャ設計書

## 7. 期待される効果

### 7.1. デバッグ容易性の向上
- タイムアウト設定の意図しない継承を即座に発見できる
- 設定ファイルの問題を特定しやすくなる

### 7.2. ドキュメント価値の向上
- dry-run出力自体が設定のドキュメントとして機能
- チーム内でのタイムアウト設定の理解が深まる

### 7.3. コードの簡素化
- 未使用の`ResolveTimeout`関数を活用し、重複した`ResolveEffectiveTimeout`を削除
- `TimeoutResolutionContext`の価値を実証
- 保守すべきコードパスを削減

## 8. リスクと対応

### 8.1. API変更の影響
**リスク**: `NewRuntimeCommand`のシグネチャ変更が呼び出し元に影響

**対応**:
- 段階的な実装（内部でのみ使用開始）
- もしくはグループ名をオプショナルパラメータとして追加

### 8.2. テストの不足
**リスク**: 既存の`ResolveTimeout`のテストが不十分

**対応**:
- 統合テストで実際の動作を確認
- dry-run出力のアサーションを追加

## 9. 参考情報

### 関連ファイル
- [internal/common/timeout_resolver.go](internal/common/timeout_resolver.go) - `ResolveTimeout`実装
- [internal/common/timeout.go](internal/common/timeout.go) - `Timeout`型と`ResolveEffectiveTimeout`
- [internal/runner/runnertypes/runtime.go](internal/runner/runnertypes/runtime.go) - `RuntimeCommand`定義
- [internal/runner/resource/dryrun_manager.go](internal/runner/resource/dryrun_manager.go) - dry-run実装

### 関連タスク
- Task 0043: Timeout Specification Refinement（タイムアウト仕様の整理）
