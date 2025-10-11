# 環境変数展開関数のリファクタリング計画

## 背景

現在、環境変数の展開は以下の3つの関数で行われている：

1. `ExpandGlobalEnv`: Global.Env の展開
2. `ExpandGroupEnv`: Group.Env の展開
3. `ExpandCommandEnv`: Command.Env の展開

これらの関数は類似したロジックを持つが、個別に実装されているため以下の課題がある：

- コードの重複
- allowlist 継承ロジックが分散
- 保守性の低下
- 将来の拡張が困難

## 目的

3つの環境変数展開関数を内部ヘルパー関数で統合し、以下を実現する：

1. **コードの共通化**: 重複を排除し、保守性を向上
2. **allowlist 継承ロジックの一元化**: 継承計算を1箇所に集約
3. **型安全性の向上**: group オブジェクトを直接渡すことで情報アクセスを確実に
4. **拡張性の確保**: 将来の機能追加に対応しやすい設計

## リファクタリング方針

### 基本設計

- **公開 API は維持**: 3つの関数は独立した公開 API として保持（後方互換性）
- **内部実装を統合**: 共通の内部ヘルパー関数 `expandEnvInternal` を作成
- **段階的実装**: リスクを最小化するため、Phase 単位で実装

### allowlist 継承ルール

```
Global level:  cfg.EnvAllowlist
Group level:   group.EnvAllowlist ?? global.EnvAllowlist
Command level: group.EnvAllowlist ?? global.EnvAllowlist
```

## 実装計画

### Phase 1: 内部ヘルパー関数の実装 ✅ **完了**

**目標**: 3つの関数の内部実装を共通化

**タスク**:

- [x] `expandEnvInternal` 関数を実装
  - [x] 関数シグネチャの定義
  - [x] allowlist 継承ロジックの実装
  - [x] `buildExpansionParams` との統合
  - [x] `expandEnvironment` の呼び出し
  - [x] 結果の書き込み処理
- [x] `ExpandGlobalEnv` を `expandEnvInternal` 使用に書き換え
- [x] `ExpandGroupEnv` を `expandEnvInternal` 使用に書き換え
- [x] `ExpandCommandEnv` を `expandEnvInternal` 使用に書き換え
- [x] 既存のテストがすべて通過することを確認
- [x] コードフォーマットとリント実行

**達成された効果**:
- ✅ コードの重複削除（約60行の重複コードを削除）
- ✅ 保守性の向上（allowlist 継承ロジックが一元化）
- ✅ 動作は完全に後方互換（すべての既存テストが通過）
- ✅ リントエラーゼロ

**リスク評価**: 低（既存の公開 API は不変）

---

### Phase 2: ExpandCommandEnv のシグネチャ改善 ✅ **完了**

**目標**: `ExpandCommandEnv` が group オブジェクトを受け取るように変更

**タスク**:

- [x] `ExpandCommandEnv` の引数変更
  - [x] `groupName string` → `group *runnertypes.CommandGroup`
- [x] `ExpansionContext` 構造体の更新
  - [x] `GroupName string` → `Group *runnertypes.CommandGroup`
- [x] `ExpandCommand` 関数の更新
  - [x] 新しい `ExpandCommandEnv` シグネチャに合わせる
  - [x] `group.Name` を使用するように変更
- [x] テストの更新
  - [x] `ExpandCommandEnv` 直接呼び出しのテストを更新
  - [x] `ExpandCommand` のテストを更新
- [x] コードフォーマットとリント実行

**達成された効果**:
- ✅ 型安全性の向上（`group *runnertypes.CommandGroup`による厳密な型チェック）
- ✅ `groupName` パラメータの削減（`group.Name`でアクセス可能）
- ✅ group オブジェクトへの直接アクセス（将来の拡張性向上）
- ✅ すべてのテストが通過（後方互換性を保持）
- ✅ リントエラーゼロ

**リスク評価**: 中（内部 API の破壊的変更）

---

### Phase 3: allowlist 計算の完全内部化 ✅ **完了**

**目標**: `ExpandCommandEnv` 内部で allowlist 継承を計算

**タスク**:

- [x] `ExpandCommandEnv` の引数変更
  - [x] `allowlist []string` → `globalAllowlist []string`
- [x] `expandEnvInternal` での allowlist 継承計算を活用
  - [x] `localAllowlist` として `group.EnvAllowlist` を渡す
  - [x] `globalAllowlist` として global allowlist を渡す
- [x] bootstrap/config.go の更新
  - [x] `DetermineEffectiveAllowlist` 呼び出しを削除
  - [x] `cfg.Global.EnvAllowlist` を直接渡す
- [x] `ExpandCommand` 関数の更新
  - [x] `ExpansionContext.EnvAllowlist` の意味を明確化（globalAllowlist）
- [x] テストの更新
  - [x] allowlist 継承のテストケースを追加
  - [x] 既存のテストを新しいシグネチャに合わせる
- [x] コードフォーマットとリント実行

**達成された効果**:
- ✅ allowlist 計算の完全な一元化（3つの展開関数すべてで統一）
- ✅ 呼び出し側のコードが簡潔に（外部での allowlist 計算が不要）
- ✅ 型安全性の向上（group オブジェクトを直接渡すことでアクセス確実）
- ✅ allowlist 継承の正確な実装（nil 継承、空配列制限、明示的上書きすべて対応）
- ✅ すべてのテストが通過（既存機能への影響なし）
- ✅ 包括的なテストケースの追加（allowlist 継承の各パターンを検証）

**リスク評価**: 中（内部 API の破壊的変更）

---

### Phase 4: 最終検証と最適化 🔄

**目標**: 統合の完成度を高め、パフォーマンスを確認

**タスク**:

- [ ] パフォーマンステストの実施
  - [ ] ベンチマークテストの作成
  - [ ] リファクタリング前後の比較
- [ ] エッジケースのテスト追加
  - [ ] allowlist が nil の場合
  - [ ] allowlist が空配列の場合
  - [ ] 継承の各パターン
- [ ] ドキュメントの更新
  - [ ] 関数のコメント更新
  - [ ] アーキテクチャドキュメントの更新
- [ ] コードレビューと最終調整

**期待される効果**:
- 堅牢性の向上
- ドキュメントの充実
- 保守性の確認

**リスク評価**: 低（検証フェーズ）

---

## 成功基準

1. ✅ すべての既存テストが通過
2. ✅ リントエラーがゼロ
3. ✅ 後方互換性が保たれている
4. ✅ コードの重複が削減されている
5. ✅ allowlist 継承ロジックが一元化されている

## ロールバック計画

Phase 1 は非破壊的変更のため、ロールバックは容易：
- コミット前の状態に戻すだけ

Phase 2 以降で問題が発生した場合：
- Phase 1 の状態で一旦コミット
- Phase 2 は別ブランチで実施

## 備考

- このリファクタリングは機能追加ではなく、コード品質向上が目的
- 段階的に実装することで、各フェーズでの動作確認が可能
- Phase 1 完了後に一旦コミットし、Phase 2 は別途検討可能

## 作業メモ

### 完全統合版: すべての展開ロジックを1つの内部関数に集約
```go
// 内部ヘルパー関数（非公開）
// すべての環境変数展開ロジックを統合
func expandEnvInternal(
    envList []string,                    // 展開対象の環境変数リスト
    contextName string,                  // エラーメッセージ用のコンテキスト名
    localAllowlist []string,             // ローカルレベルの allowlist (Global/Group/Command)
    globalAllowlist []string,            // グローバル allowlist（継承計算用、Global level では nil）
    globalEnv map[string]string,         // 参照する Global.ExpandedEnv（Global level では nil）
    groupEnv map[string]string,          // 参照する Group.ExpandedEnv（Global/Group level では nil）
    autoEnv map[string]string,           // 自動環境変数
    expander *environment.VariableExpander,
    failureErr error,                    // エラー時のセンチネルエラー
    outputTarget *map[string]string,     // 結果の書き込み先
) error {
    // allowlist の継承計算
    effectiveAllowlist := localAllowlist
    if effectiveAllowlist == nil && globalAllowlist != nil {
        effectiveAllowlist = globalAllowlist
    }

    params := buildExpansionParams(
        envList,
        contextName,
        effectiveAllowlist,
        globalEnv,
        groupEnv,
        autoEnv,
        expander,
        failureErr,
    )

    expandedEnv, err := expandEnvironment(params)
    if err != nil {
        return err
    }

    *outputTarget = expandedEnv
    return nil
}

// 公開 API 1: Global.Env の展開
func ExpandGlobalEnv(
    cfg *runnertypes.GlobalConfig,
    expander *environment.VariableExpander,
    autoEnv map[string]string,
) error {
    if cfg == nil {
        return ErrNilConfig
    }
    if expander == nil {
        return ErrNilExpander
    }

    return expandEnvInternal(
        cfg.Env,              // envList
        "global.env",         // contextName
        cfg.EnvAllowlist,     // localAllowlist
        nil,                  // globalAllowlist (継承元がない)
        nil,                  // globalEnv (自己展開中)
        nil,                  // groupEnv (存在しない)
        autoEnv,              // autoEnv
        expander,             // expander
        ErrGlobalEnvExpansionFailed, // failureErr
        &cfg.ExpandedEnv,     // outputTarget
    )
}

// 公開 API 2: Group.Env の展開
func ExpandGroupEnv(
    group *runnertypes.CommandGroup,
    globalEnv map[string]string,
    globalAllowlist []string,
    expander *environment.VariableExpander,
    autoEnv map[string]string,
) error {
    if group == nil {
        return ErrNilGroup
    }
    if expander == nil {
        return ErrNilExpander
    }

    return expandEnvInternal(
        group.Env,                   // envList
        fmt.Sprintf("group.env:%s", group.Name), // contextName
        group.EnvAllowlist,          // localAllowlist
        globalAllowlist,             // globalAllowlist (継承用)
        globalEnv,                   // globalEnv (Global.ExpandedEnv)
        nil,                         // groupEnv (自己展開中)
        autoEnv,                     // autoEnv
        expander,                    // expander
        ErrGroupEnvExpansionFailed,  // failureErr
        &group.ExpandedEnv,          // outputTarget
    )
}

// 公開 API 3: Command.Env の展開
func ExpandCommandEnv(
    cmd *runnertypes.Command,
    group *runnertypes.CommandGroup,
    globalAllowlist []string,
    expander *environment.VariableExpander,
    globalEnv map[string]string,
    groupEnv map[string]string,
    autoEnv map[string]string,
) error {
    if cmd == nil {
        return ErrNilCommand
    }
    if group == nil {
        return ErrNilGroup
    }
    if expander == nil {
        return ErrNilExpander
    }

    return expandEnvInternal(
        cmd.Env,                     // envList
        fmt.Sprintf("command.env:%s (group:%s)", cmd.Name, group.Name), // contextName
        group.EnvAllowlist,          // localAllowlist (command は group の allowlist を使用)
        globalAllowlist,             // globalAllowlist (継承用)
        globalEnv,                   // globalEnv (Global.ExpandedEnv)
        groupEnv,                    // groupEnv (Group.ExpandedEnv)
        autoEnv,                     // autoEnv
        expander,                    // expander
        ErrCommandEnvExpansionFailed, // failureErr
        &cmd.ExpandedEnv,            // outputTarget
    )
}
```

### 主要な変更点
1. ExpandCommandEnv のシグネチャ変更
```go
// 変更前
func ExpandCommandEnv(
    cmd *runnertypes.Command,
    groupName string,              // ← string
    allowlist []string,            // ← 外部計算済み
    expander *environment.VariableExpander,
    globalEnv map[string]string,
    groupEnv map[string]string,
    autoEnv map[string]string,
) error

// 変更後
func ExpandCommandEnv(
    cmd *runnertypes.Command,
    group *runnertypes.CommandGroup, // ← *CommandGroup オブジェクト
    globalAllowlist []string,        // ← 内部で継承計算
    expander *environment.VariableExpander,
    globalEnv map[string]string,
    groupEnv map[string]string,
    autoEnv map[string]string,
) error
```

メリット:
group.Name を取得可能（groupName 引数が不要）
group.EnvAllowlist にアクセス可能（内部で継承計算）
group オブジェクト全体にアクセス可能（将来の拡張性）
2. allowlist の継承計算を内部化
```go
// expandEnvInternal 内で統一的に処理
effectiveAllowlist := localAllowlist
if effectiveAllowlist == nil && globalAllowlist != nil {
    effectiveAllowlist = globalAllowlist
}
```
これにより、3つの関数すべてで同じロジックが適用されます。

### 呼び出し側の変更
bootstrap/config.go の変更
```go
// 変更前
effectiveAllowlist := config.DetermineEffectiveAllowlist(group, &cfg.Global)

expandedCmd, expandedArgs, expandedEnv, err := config.ExpandCommand(&config.ExpansionContext{
    Command:      cmd,
    Expander:     expander,
    AutoEnv:      autoEnv,
    GlobalEnv:    cfg.Global.ExpandedEnv,
    GroupEnv:     group.ExpandedEnv,
    EnvAllowlist: effectiveAllowlist,
    GroupName:    group.Name,
})

// 変更後（effectiveAllowlist 計算が不要に）
expandedCmd, expandedArgs, expandedEnv, err := config.ExpandCommand(&config.ExpansionContext{
    Command:      cmd,
    Group:        group,  // group オブジェクトを渡す
    Expander:     expander,
    AutoEnv:      autoEnv,
    GlobalEnv:    cfg.Global.ExpandedEnv,
    GroupEnv:     group.ExpandedEnv,
    EnvAllowlist: cfg.Global.EnvAllowlist,  // global allowlist を渡す（継承は内部で計算）
})
```
ExpansionContext の変更
```go
type ExpansionContext struct {
    Command *runnertypes.Command
    Group   *runnertypes.CommandGroup  // 追加（以前は GroupName だけだった）

    Expander *environment.VariableExpander
    AutoEnv  map[string]string
    GlobalEnv map[string]string
    GroupEnv  map[string]string

    EnvAllowlist []string  // これは GlobalAllowlist として解釈される

    // GroupName は削除（Group.Name から取得可能）
}
```
