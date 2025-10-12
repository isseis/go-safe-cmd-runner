# Allowlist データ構造最適化 - 包括的実装計画書

## 1. 概要

本実装計画書では、allowlist データ構造最適化を3つのPhaseに分けて段階的に実装する詳細計画を定義する。各Phaseは独立して実装・テスト・リリース可能であり、後方互換性を維持しながらパフォーマンス向上を実現する。

### 1.1 実装方針

1. **段階的アプローチ**: 既存システムを破壊せず、段階的に最適化
2. **互換性維持**: 各Phase完了時点で既存APIが動作すること
3. **パフォーマンス重視**: O(1)検索性能の実現とメモリ効率化
4. **テスト充実**: 各Phase完了時に包括的テストでの検証

### 1.2 成果物の構成

- **Phase 1**: Getter メソッド追加、既存参照の置換
- **Phase 2**: 内部実装最適化、新コンストラクタAPI
- **Phase 3**: 公開フィールド非推奨化、完全最適化

## 2. Phase 1: Getter メソッド追加と既存参照の置換

### 2.1 目標

**主要目標**
- 既存の公開フィールドを維持しつつ、getter メソッドを追加
- `expansion.go` での直接フィールド参照を getter 呼び出しに置換
- 構造体の破壊的変更を避ける
- パフォーマンス劣化なしで互換性を確保

**成功基準**
- 全ての既存テストがパス
- `expansion.go` での直接フィールド参照がゼロ
- パフォーマンステストで性能劣化がないこと
- 新しいgetterメソッドの動作が既存フィールドと同等

### 2.2 実装内容

#### 2.2.1 新しいGetter メソッドの追加

**対象ファイル**: `internal/runner/runnertypes/config.go`

**追加メソッド**:
```go
// GetEffectiveList returns the effective allowlist as a string slice.
// This method provides controlled access to the effective allowlist with
// potential for future optimization.
func (r *AllowlistResolution) GetEffectiveList() []string {
    if r == nil {
        return []string{}
    }
    return r.EffectiveList  // Phase 1では既存フィールドを返す
}

// GetEffectiveSize returns the number of effective allowlist entries.
// More efficient than len(GetEffectiveList()) for size-only queries.
func (r *AllowlistResolution) GetEffectiveSize() int {
    if r == nil {
        return 0
    }
    return len(r.EffectiveList)
}

// GetGroupAllowlist returns group allowlist (compatibility getter).
func (r *AllowlistResolution) GetGroupAllowlist() []string {
    if r == nil {
        return []string{}
    }
    return r.GroupAllowlist
}

// GetGlobalAllowlist returns global allowlist (compatibility getter).
func (r *AllowlistResolution) GetGlobalAllowlist() []string {
    if r == nil {
        return []string{}
    }
    return r.GlobalAllowlist
}

// GetMode returns the inheritance mode used for this resolution.
func (r *AllowlistResolution) GetMode() runnertypes.InheritanceMode {
    if r == nil {
        return runnertypes.InheritanceModeInherit  // safe default
    }
    return r.Mode
}

// GetGroupName returns the name of the group this resolution is for.
func (r *AllowlistResolution) GetGroupName() string {
    if r == nil {
        return ""
    }
    return r.GroupName
}
```

#### 2.2.2 expansion.go での参照の置換

**対象ファイル**: `internal/runner/config/expansion.go`

**置換箇所**:

1. **Line 125**: `resolution.EffectiveList` → `resolution.GetEffectiveList()`
   ```go
   // 変更前
   effectiveAllowlist := resolution.EffectiveList

   // 変更後
   effectiveAllowlist := resolution.GetEffectiveList()
   ```

2. **Line 372**: `resolution.EffectiveList` → `resolution.GetEffectiveList()`
   ```go
   // 変更前
   allowlist := resolution.EffectiveList

   // 変更後
   allowlist := resolution.GetEffectiveList()
   ```

3. **Line 562**: `resolution.EffectiveList` → `resolution.GetEffectiveList()`
   ```go
   // 変更前
   effectiveAllowlist := resolution.EffectiveList

   // 変更後
   effectiveAllowlist := resolution.GetEffectiveList()
   ```

4. **ログ出力での最適化**: `len(resolution.EffectiveList)` → `resolution.GetEffectiveSize()`

### 2.3 実装手順

#### Step 2.3.1: Getter メソッドの実装 (0.5日)

**作業内容**:
1. `config.go` にgetter メソッドを追加
2. 包括的な単体テストを追加
3. nil安全性の確認

**チェックリスト**:
- [ ] `GetEffectiveList()` メソッドの実装
- [ ] `GetEffectiveSize()` メソッドの実装
- [ ] `GetGroupAllowlist()` メソッドの実装
- [ ] `GetGlobalAllowlist()` メソッドの実装
- [ ] `GetMode()` メソッドの実装
- [ ] `GetGroupName()` メソッドの実装
- [ ] 全メソッドのnil安全性テスト
- [ ] 既存フィールドとの整合性テスト

#### Step 2.3.2: expansion.go の参照置換 (0.5日)

**作業内容**:
1. 3箇所の`EffectiveList`参照をgetter呼び出しに置換
2. ログ出力の最適化
3. リグレッションテストの実行

**チェックリスト**:
- [ ] Line 125 の置換
- [ ] Line 372 の置換
- [ ] Line 562 の置換
- [ ] ログ出力の`len()`呼び出し最適化
- [ ] 全体のビルド確認
- [ ] 既存テストスイートのパス確認

#### Step 2.3.3: テストと検証 (0.5日)

**作業内容**:
1. 新しいgetterメソッドの単体テスト
2. 統合テストでの動作確認
3. パフォーマンステストでの劣化確認

**チェックリスト**:
- [ ] 新メソッドの単体テスト完了
- [ ] 既存テストスイート完全パス
- [ ] パフォーマンス劣化なし確認
- [ ] メモリリーク検査
- [ ] 並行性テスト

### 2.4 リスク管理

#### 2.4.1 識別されたリスク

1. **互換性破損**: 既存コードが動作しなくなる
   - **軽減策**: 既存フィールドを維持、段階的置換
   - **対応**: 包括的回帰テスト

2. **パフォーマンス劣化**: getter呼び出しによるオーバーヘッド
   - **軽減策**: インライン化、最適化フラグ
   - **対応**: ベンチマークテストでの監視

3. **テスト不備**: 新機能のバグ見逃し
   - **軽減策**: TDD、カバレッジ100%達成
   - **対応**: ピアレビュー、静的解析

#### 2.4.2 フォールバック戦略

- **問題発生時**: 即座に前のコミットにリバート
- **段階的リリース**: カナリア環境での先行テスト
- **監視強化**: パフォーマンスメトリクスの継続監視

### 2.5 成果物とマイルストーン

#### 2.5.1 成果物

1. **拡張された AllowlistResolution**: getter メソッド付き
2. **更新された expansion.go**: getter使用に変更
3. **包括的テストスイート**: 新機能のテスト
4. **パフォーマンスレポート**: 劣化なしの証明

#### 2.5.2 マイルストーン

- **Day 1**: Getter メソッド実装完了
- **Day 1.5**: expansion.go 置換完了
- **Day 2**: テスト・検証完了、Phase 1リリース準備

---

## 3. Phase 2: 内部実装の最適化

### 3.1 目標

**主要目標**
- map → slice → map の変換排除
- `effectiveSet` の事前計算導入
- メモリ使用量削減
- 新しいコンストラクタAPIの導入

**成功基準**
- `ResolveAllowlistConfiguration` の実行時間50%短縮
- メモリアロケーション40%削減
- 大規模 allowlist でのパフォーマンス向上
- すべての既存テストがパス

### 3.2 実装内容

#### 3.2.1 AllowlistResolution 構造体の拡張

**対象ファイル**: `internal/runner/runnertypes/config.go`

**構造体拡張**:
```go
type AllowlistResolution struct {
    Mode            InheritanceMode   // 継承モード（Inherit/Explicit/Reject）
    GroupAllowlist  []string          // 公開 - 既存互換性維持
    GlobalAllowlist []string          // 公開 - 既存互換性維持
    EffectiveList   []string          // 公開 - 既存互換性維持
    GroupName       string            // グループ名

    // 内部データ（効率的な検索用）- 既存
    groupAllowlistSet  map[string]struct{}  // 非公開（検索用）
    globalAllowlistSet map[string]struct{}  // 非公開（検索用）

    // Phase 2 追加: 事前計算されたeffective set
    effectiveSet       map[string]struct{}  // 非公開（最適化された検索用）

    // Phase 2 追加: getter用キャッシュ（遅延評価）
    groupAllowlistCache  []string  // GetGroupAllowlist() 用キャッシュ
    globalAllowlistCache []string  // GetGlobalAllowlist() 用キャッシュ
    effectiveListCache   []string  // GetEffectiveList() 用キャッシュ
}
```

#### 3.2.2 新しいコンストラクタの追加

**NewAllowlistResolution コンストラクタ**:
```go
// NewAllowlistResolution creates a new AllowlistResolution with pre-computed effective set.
// Phase 2 で導入される新しいコンストラクタ
//
// Parameters:
//   - mode: 継承モード
//   - groupName: グループ名
//   - groupSet: グループallowlist (map形式)
//   - globalSet: グローバルallowlist (map形式)
//
// Returns:
//   - *AllowlistResolution: effectiveSet が事前計算された新しいインスタンス
//
// Panics:
//   - groupSet が nil の場合
//   - globalSet が nil の場合
func NewAllowlistResolution(
    mode InheritanceMode,
    groupName string,
    groupSet map[string]struct{},
    globalSet map[string]struct{},
) *AllowlistResolution {
    // 入力検証
    if groupSet == nil {
        panic("NewAllowlistResolution: groupSet cannot be nil")
    }
    if globalSet == nil {
        panic("NewAllowlistResolution: globalSet cannot be nil")
    }

    r := &AllowlistResolution{
        Mode:               mode,
        GroupName:          groupName,
        groupAllowlistSet:  groupSet,
        globalAllowlistSet: globalSet,

        // Phase 2: slice フィールドは遅延生成に変更
        GroupAllowlist:  []string{},
        GlobalAllowlist: []string{},
        EffectiveList:   []string{},
    }

    // effectiveSet を事前計算
    r.computeEffectiveSet()

    return r
}
```

#### 3.2.3 computeEffectiveSet メソッドの実装

**効率的な継承モード処理**:
```go
// computeEffectiveSet calculates the effective allowlist based on inheritance mode.
//
// この関数は不変条件を確立する責任を持つ:
// - 呼び出し後、effectiveSet は必ず nil でない状態になる
// - groupAllowlistSet と globalAllowlistSet が nil の場合はpanic
//
// Panics:
//   - groupAllowlistSet が nil の場合
//   - globalAllowlistSet が nil の場合
func (r *AllowlistResolution) computeEffectiveSet() {
    // 不変条件の前提: groupAllowlistSet と globalAllowlistSet は nil であってはならない
    if r.groupAllowlistSet == nil {
        panic("AllowlistResolution: groupAllowlistSet is nil - cannot compute effective set")
    }
    if r.globalAllowlistSet == nil {
        panic("AllowlistResolution: globalAllowlistSet is nil - cannot compute effective set")
    }

    switch r.Mode {
    case InheritanceModeInherit:
        // グローバルallowlistを直接参照（zero-copy）
        r.effectiveSet = r.globalAllowlistSet

    case InheritanceModeExplicit:
        // グループallowlistを直接参照（zero-copy）
        r.effectiveSet = r.groupAllowlistSet

    case InheritanceModeReject:
        // 空のset(nil ではなく空のマップ)
        r.effectiveSet = make(map[string]struct{})

    default:
        // デフォルトは継承モード
        r.effectiveSet = r.globalAllowlistSet
    }

    // POST-CONDITION: effectiveSet は nil であってはならない
    if r.effectiveSet == nil {
        panic("AllowlistResolution: internal error - effectiveSet is still nil after computeEffectiveSet()")
    }
}
```

#### 3.2.4 IsAllowed メソッドの最適化

**Phase 2 のターゲット実装**:
```go
// IsAllowed checks if a variable is allowed in the effective allowlist.
// This is the most frequently called method and is optimized for O(1) performance.
//
// Phase 2 での実装: effectiveSet を事前計算して使用
// - computeEffectiveSet() で Mode に基づいて effectiveSet を計算
// - IsAllowed() では effectiveSet のみを参照(より高速)
//
// Parameters:
//   - variable: environment variable name to check
// Returns: true if the variable is allowed, false otherwise
//
// Panics:
//   - effectiveSet が nil の場合(不変条件違反)
func (r *AllowlistResolution) IsAllowed(variable string) bool {
    // nil receiver は呼び出し側のエラー - false を返す
    if r == nil {
        return false
    }

    // 空の変数名は入力検証エラー - false を返す
    if variable == "" {
        return false
    }

    // INVARIANT: effectiveSet は初期化時に設定されていなければならない
    // これが nil の場合は不変条件違反なので panic
    if r.effectiveSet == nil {
        panic("AllowlistResolution: effectiveSet is nil - object not properly initialized")
    }

    _, allowed := r.effectiveSet[variable]
    return allowed
}
```

#### 3.2.5 Getter メソッドの最適化

**遅延評価による効率化**:
```go
// GetEffectiveList returns effective allowlist with lazy evaluation for performance.
func (r *AllowlistResolution) GetEffectiveList() []string {
    if r == nil {
        return []string{}
    }

    // Phase 2: 遅延評価とキャッシュ
    if r.effectiveListCache == nil && r.effectiveSet != nil {
        r.effectiveListCache = r.setToSortedSlice(r.effectiveSet)
    }

    return r.effectiveListCache
}

// GetGroupAllowlist returns group allowlist with lazy evaluation.
func (r *AllowlistResolution) GetGroupAllowlist() []string {
    if r == nil {
        return []string{}
    }

    // Phase 2: 遅延評価とキャッシュ
    if r.groupAllowlistCache == nil && r.groupAllowlistSet != nil {
        r.groupAllowlistCache = r.setToSortedSlice(r.groupAllowlistSet)
    }

    return r.groupAllowlistCache
}

// GetGlobalAllowlist returns global allowlist with lazy evaluation.
func (r *AllowlistResolution) GetGlobalAllowlist() []string {
    if r == nil {
        return []string{}
    }

    // Phase 2: 遅延評価とキャッシュ
    if r.globalAllowlistCache == nil && r.globalAllowlistSet != nil {
        r.globalAllowlistCache = r.setToSortedSlice(r.globalAllowlistSet)
    }

    return r.globalAllowlistCache
}

// setToSortedSlice converts a set to a sorted string slice.
func (r *AllowlistResolution) setToSortedSlice(set map[string]struct{}) []string {
    if len(set) == 0 {
        return []string{}
    }

    slice := make([]string, 0, len(set))
    for variable := range set {
        slice = append(slice, variable)
    }

    sort.Strings(slice)
    return slice
}
```

#### 3.2.6 ResolveAllowlistConfiguration の最適化

**対象ファイル**: `internal/runner/environment/filter.go`

**最適化された実装**:
```go
// ResolveAllowlistConfiguration resolves the effective allowlist configuration for a group
// Phase 2: NewAllowlistResolution を使用した最適化実装
func (f *Filter) ResolveAllowlistConfiguration(allowlist []string, groupName string) *runnertypes.AllowlistResolution {
    mode := f.determineInheritanceMode(allowlist)

    // Phase 2: 効率的な map ベース構築
    groupSet := buildAllowlistSet(allowlist)
    globalSet := f.globalAllowlist  // 既に map 形式

    // Phase 2: 新しいコンストラクタを使用
    resolution := runnertypes.NewAllowlistResolution(mode, groupName, groupSet, globalSet)

    // Log the resolution for debugging
    slog.Debug("Resolved allowlist configuration",
        "group", groupName,
        "mode", mode.String(),
        "group_allowlist_size", len(allowlist),
        "global_allowlist_size", len(f.globalAllowlist),
        "effective_allowlist_size", resolution.GetEffectiveSize())

    return resolution
}
```

#### 3.2.7 Builder パターンの導入

**AllowlistResolutionBuilder**:
```go
// AllowlistResolutionBuilder provides a fluent interface for creating AllowlistResolution
// Available in Phase 2 and later.
type AllowlistResolutionBuilder struct {
    mode      InheritanceMode
    groupName string
    groupVars []string
    globalVars []string
}

// NewAllowlistResolutionBuilder creates a new builder
func NewAllowlistResolutionBuilder() *AllowlistResolutionBuilder {
    return &AllowlistResolutionBuilder{
        mode: InheritanceModeInherit, // default
    }
}

// WithMode sets the inheritance mode
func (b *AllowlistResolutionBuilder) WithMode(mode InheritanceMode) *AllowlistResolutionBuilder {
    b.mode = mode
    return b
}

// WithGroupName sets the group name
func (b *AllowlistResolutionBuilder) WithGroupName(name string) *AllowlistResolutionBuilder {
    b.groupName = name
    return b
}

// WithGroupVariables sets the group-specific variables
func (b *AllowlistResolutionBuilder) WithGroupVariables(vars []string) *AllowlistResolutionBuilder {
    b.groupVars = vars
    return b
}

// WithGlobalVariables sets the global variables
func (b *AllowlistResolutionBuilder) WithGlobalVariables(vars []string) *AllowlistResolutionBuilder {
    b.globalVars = vars
    return b
}

// Build creates the AllowlistResolution
func (b *AllowlistResolutionBuilder) Build() *AllowlistResolution {
    groupSet := buildAllowlistSet(b.groupVars)
    globalSet := buildAllowlistSet(b.globalVars)
    return NewAllowlistResolution(b.mode, b.groupName, groupSet, globalSet)
}
```

### 3.3 実装手順

#### Step 3.3.1: データ構造拡張とコンストラクタ実装 (1日)

**作業内容**:
1. `AllowlistResolution` 構造体に `effectiveSet` とキャッシュフィールドを追加
2. `NewAllowlistResolution` コンストラクタの実装
3. `computeEffectiveSet` メソッドの実装

**チェックリスト**:
- [ ] 構造体フィールド追加
- [ ] `NewAllowlistResolution` 実装
- [ ] `computeEffectiveSet` 実装
- [ ] 入力検証とエラーハンドリング
- [ ] 単体テスト作成

#### Step 3.3.2: IsAllowed メソッドの最適化 (0.5日)

**作業内容**:
1. `IsAllowed` メソッドを `effectiveSet` ベースに変更
2. パフォーマンステストでの検証
3. 既存テストとの整合性確認

**チェックリスト**:
- [ ] `IsAllowed` メソッド変更
- [ ] パフォーマンステスト実行
- [ ] 機能テスト完全パス
- [ ] エラーハンドリングテスト

#### Step 3.3.3: Getter メソッドの遅延評価実装 (1日)

**作業内容**:
1. getter メソッドを遅延評価に変更
2. キャッシュ機能の実装
3. `setToSortedSlice` ヘルパーメソッドの実装

**チェックリスト**:
- [ ] `GetEffectiveList` 遅延評価実装
- [ ] `GetGroupAllowlist` 遅延評価実装
- [ ] `GetGlobalAllowlist` 遅延評価実装
- [ ] キャッシュ機能テスト
- [ ] メモリリーク検査

#### Step 3.3.4: ResolveAllowlistConfiguration 最適化 (0.5日)

**作業内容**:
1. `ResolveAllowlistConfiguration` を新コンストラクタ使用に変更
2. パフォーマンス向上の測定
3. 既存機能との互換性確認

**チェックリスト**:
- [ ] メソッド最適化実装
- [ ] パフォーマンス測定
- [ ] 機能互換性テスト
- [ ] 統合テスト実行

#### Step 3.3.5: Builder パターンとテスト支援実装 (1日)

**作業内容**:
1. `AllowlistResolutionBuilder` の実装
2. テスト用ファクトリメソッドの実装
3. 包括的テストスイートの作成

**チェックリスト**:
- [ ] Builder パターン実装
- [ ] テストファクトリ実装
- [ ] 包括的テストスイート
- [ ] ドキュメント更新

#### Step 3.3.6: パフォーマンステストと最終検証 (0.5日)

**作業内容**:
1. 大規模データでのパフォーマンステスト
2. メモリ使用量の測定
3. 最終的な統合テスト

**チェックリスト**:
- [ ] 大規模パフォーマンステスト
- [ ] メモリ使用量測定
- [ ] 50%性能向上確認
- [ ] 40%メモリ削減確認
- [ ] 全テストスイート完全パス

### 3.4 リスク管理

#### 3.4.1 識別されたリスク

1. **複雑性増加**: 新しい内部構造による複雑性
   - **軽減策**: 段階的実装、包括的ドキュメント
   - **対応**: コードレビュー強化、設計文書充実

2. **メモリ使用量増加**: キャッシュによるメモリ使用量増加
   - **軽減策**: 遅延評価、適切なキャッシュサイズ
   - **対応**: メモリプロファイリング、制限設定

3. **並行安全性**: キャッシュの並行アクセス
   - **軽減策**: 読み取り専用設計、適切な同期
   - **対応**: 並行テスト、race detector使用

### 3.5 成果物とマイルストーン

#### 3.5.1 成果物

1. **最適化された AllowlistResolution**: effectiveSet 付き
2. **新しいコンストラクタAPI**: NewAllowlistResolution
3. **Builder パターン**: 柔軟な構築インターフェース
4. **パフォーマンスレポート**: 50%性能向上、40%メモリ削減証明

#### 3.5.2 マイルストーン

- **Day 1**: データ構造拡張完了
- **Day 2**: 最適化メソッド実装完了
- **Day 3**: 遅延評価実装完了
- **Day 4**: Builder パターン完了
- **Day 4.5**: パフォーマンステスト完了、Phase 2リリース準備

---

## 4. Phase 3: 公開フィールド非推奨化と完全最適化

### 4.1 目標

**主要目標**
- 公開フィールドの段階的廃止
- 完全な map ベース内部実装への移行
- レガシーコードの完全サポート
- 長期的な保守性向上

**成功基準**
- すべての公開フィールドアクセスが非推奨警告
- 新規コードでの公開フィールド使用がゼロ
- 既存コードの段階的移行計画完成
- パフォーマンスの更なる向上

### 4.2 実装内容

#### 4.2.1 公開フィールドの非推奨化

**段階的非推奨マーク**:
```go
type AllowlistResolution struct {
    Mode            InheritanceMode   // 継承モード

    // DEPRECATED: Use GetGroupAllowlist() instead. Will be removed in v3.0.
    GroupAllowlist  []string          // 非推奨予定

    // DEPRECATED: Use GetGlobalAllowlist() instead. Will be removed in v3.0.
    GlobalAllowlist []string          // 非推奨予定

    // DEPRECATED: Use GetEffectiveList() instead. Will be removed in v3.0.
    EffectiveList   []string          // 非推奨予定

    GroupName       string            // グループ名

    // 内部データ（効率的な検索用）
    groupAllowlistSet  map[string]struct{}  // 非公開
    globalAllowlistSet map[string]struct{}  // 非公開
    effectiveSet       map[string]struct{}  // 非公開

    // キャッシュ（遅延評価）
    groupAllowlistCache  []string
    globalAllowlistCache []string
    effectiveListCache   []string
}
```

#### 4.2.2 非推奨警告システム

**実行時警告機能**:
```go
// deprecationWarning tracks deprecated field access
type deprecationWarning struct {
    fieldName    string
    accessCount  int64
    firstAccess  time.Time
    lastAccess   time.Time
}

var (
    deprecationWarnings = make(map[string]*deprecationWarning)
    deprecationMutex    sync.RWMutex
    warningEnabled      = true  // 設定で制御可能
)

// warnDeprecatedFieldAccess logs deprecation warnings
func warnDeprecatedFieldAccess(fieldName string) {
    if !warningEnabled {
        return
    }

    deprecationMutex.Lock()
    defer deprecationMutex.Unlock()

    warning, exists := deprecationWarnings[fieldName]
    if !exists {
        warning = &deprecationWarning{
            fieldName:   fieldName,
            firstAccess: time.Now(),
        }
        deprecationWarnings[fieldName] = warning
    }

    warning.accessCount++
    warning.lastAccess = time.Now()

    // 初回とその後の定期的な警告
    if warning.accessCount == 1 || warning.accessCount%100 == 0 {
        log.Printf("DEPRECATED: Direct access to AllowlistResolution.%s (count: %d). Use Get%s() method instead. This field will be removed in v3.0.",
            fieldName, warning.accessCount, fieldName)
    }
}
```

#### 4.2.3 レガシーアクセサ

**互換性維持のためのアクセサ**:
```go
// GroupAllowlist returns the group allowlist with deprecation warning.
// DEPRECATED: Use GetGroupAllowlist() instead.
func (r *AllowlistResolution) GroupAllowlist() []string {
    warnDeprecatedFieldAccess("GroupAllowlist")
    return r.GetGroupAllowlist()
}

// GlobalAllowlist returns the global allowlist with deprecation warning.
// DEPRECATED: Use GetGlobalAllowlist() instead.
func (r *AllowlistResolution) GlobalAllowlist() []string {
    warnDeprecatedFieldAccess("GlobalAllowlist")
    return r.GetGlobalAllowlist()
}

// EffectiveList returns the effective allowlist with deprecation warning.
// DEPRECATED: Use GetEffectiveList() instead.
func (r *AllowlistResolution) EffectiveList() []string {
    warnDeprecatedFieldAccess("EffectiveList")
    return r.GetEffectiveList()
}
```

#### 4.2.4 移行支援ツール

**自動移行検出ツール**:
```go
// MigrationAnalyzer analyzes code for deprecated field usage
type MigrationAnalyzer struct {
    deprecatedUsages map[string][]string  // file -> []usage
    fixSuggestions   map[string]string    // old -> new
}

// NewMigrationAnalyzer creates a new migration analyzer
func NewMigrationAnalyzer() *MigrationAnalyzer {
    return &MigrationAnalyzer{
        deprecatedUsages: make(map[string][]string),
        fixSuggestions: map[string]string{
            "resolution.GroupAllowlist":  "resolution.GetGroupAllowlist()",
            "resolution.GlobalAllowlist": "resolution.GetGlobalAllowlist()",
            "resolution.EffectiveList":   "resolution.GetEffectiveList()",
            "len(resolution.EffectiveList)": "resolution.GetEffectiveSize()",
        },
    }
}

// AnalyzeFile analyzes a Go file for deprecated usage
func (ma *MigrationAnalyzer) AnalyzeFile(filename string) error {
    // AST解析による非推奨使用箇所の検出
    // 自動修正提案の生成
    return nil
}

// GenerateMigrationReport generates a migration report
func (ma *MigrationAnalyzer) GenerateMigrationReport() string {
    // 移行レポートの生成
    return ""
}
```

#### 4.2.5 設定とモニタリング

**非推奨化制御設定**:
```go
// DeprecationConfig controls deprecation behavior
type DeprecationConfig struct {
    EnableWarnings    bool   // 警告の有効/無効
    MaxWarningsPerField int  // フィールドごとの最大警告数
    WarningInterval   int    // 警告間隔(アクセス回数)
    LogLevel         string  // ログレベル
}

// DefaultDeprecationConfig returns default deprecation configuration
func DefaultDeprecationConfig() DeprecationConfig {
    return DeprecationConfig{
        EnableWarnings:     true,
        MaxWarningsPerField: 1000,
        WarningInterval:    100,
        LogLevel:          "WARN",
    }
}

// SetDeprecationConfig updates global deprecation configuration
func SetDeprecationConfig(config DeprecationConfig) {
    configLock.Lock()
    defer configLock.Unlock()
    globalDeprecationConfig = config
    warningEnabled = config.EnableWarnings
}

// GetDeprecationStats returns deprecation usage statistics
func GetDeprecationStats() map[string]*deprecationWarning {
    deprecationMutex.RLock()
    defer deprecationMutex.RUnlock()

    stats := make(map[string]*deprecationWarning)
    for k, v := range deprecationWarnings {
        stats[k] = &deprecationWarning{
            fieldName:   v.fieldName,
            accessCount: v.accessCount,
            firstAccess: v.firstAccess,
            lastAccess:  v.lastAccess,
        }
    }
    return stats
}
```

#### 4.2.6 最終的な構造最適化

**完全 map ベース内部実装**:
```go
// Phase 3: 最終的に公開フィールドを削除する場合の構造
type AllowlistResolution struct {
    // 公開フィールド（メタデータのみ）
    Mode      InheritanceMode
    GroupName string

    // 内部データ（完全 map ベース）
    groupAllowlistSet  map[string]struct{}
    globalAllowlistSet map[string]struct{}
    effectiveSet       map[string]struct{}

    // 遅延評価キャッシュ
    groupAllowlistCache  []string
    globalAllowlistCache []string
    effectiveListCache   []string

    // キャッシュ状態管理
    cacheValid struct {
        group     bool
        global    bool
        effective bool
    }
}
```

### 4.3 実装手順

#### Step 4.3.1: 非推奨警告システム実装 (1日)

**作業内容**:
1. 非推奨警告システムの実装
2. レガシーアクセサメソッドの追加
3. 設定システムの実装

**チェックリスト**:
- [ ] 警告システム実装
- [ ] アクセサメソッド実装
- [ ] 設定システム実装
- [ ] ログ出力テスト
- [ ] パフォーマンス影響測定

#### Step 4.3.2: 移行支援ツール開発 (1.5日)

**作業内容**:
1. 移行分析ツールの実装
2. 自動修正提案システム
3. 移行レポート生成機能

**チェックリスト**:
- [ ] AST解析ツール実装
- [ ] 使用箇所検出機能
- [ ] 自動修正提案生成
- [ ] 移行レポート機能
- [ ] CLI ツール化

#### Step 4.3.3: ドキュメントと移行ガイド作成 (1日)

**作業内容**:
1. 移行ガイドの作成
2. API ドキュメントの更新
3. ベストプラクティスガイド

**チェックリスト**:
- [ ] 移行ガイド作成
- [ ] API ドキュメント更新
- [ ] ベストプラクティス文書
- [ ] FAQ 作成
- [ ] サンプルコード更新

#### Step 4.3.4: 段階的移行計画実行 (2日)

**作業内容**:
1. 内部コードの段階的移行
2. テストコードの更新
3. ドキュメント例の更新

**チェックリスト**:
- [ ] 内部コード移行
- [ ] テストコード更新
- [ ] ドキュメント例更新
- [ ] CI/CD パイプライン更新
- [ ] 移行検証テスト

#### Step 4.3.5: 最終検証とリリース準備 (0.5日)

**作業内容**:
1. 包括的テストの実行
2. パフォーマンス最終測定
3. リリースノート作成

**チェックリスト**:
- [ ] 全テストスイート実行
- [ ] パフォーマンス測定
- [ ] セキュリティ監査
- [ ] リリースノート作成
- [ ] Phase 3 リリース準備

### 4.4 リスク管理

#### 4.4.1 識別されたリスク

1. **レガシーコード破損**: 古いコードが動作しなくなる
   - **軽減策**: 段階的非推奨化、長期サポート
   - **対応**: 移行ツール提供、詳細ガイド

2. **パフォーマンス影響**: 警告システムによるオーバーヘッド
   - **軽減策**: 効率的な警告実装、設定制御
   - **対応**: 本番環境での警告無効化オプション

3. **移行負担**: 開発者の移行負担が大きい
   - **軽減策**: 自動化ツール、明確なガイド
   - **対応**: 段階的移行期間の設定

### 4.5 成果物とマイルストーン

#### 4.5.1 成果物

1. **非推奨警告システム**: 段階的移行支援
2. **移行分析ツール**: 自動検出・修正提案
3. **包括的移行ガイド**: ステップバイステップ指示
4. **最適化された実装**: 完全 map ベース

#### 4.5.2 マイルストーン

- **Day 1**: 非推奨警告システム完了
- **Day 2.5**: 移行支援ツール完了
- **Day 3.5**: ドキュメント完了
- **Day 5.5**: 段階的移行完了
- **Day 6**: 最終検証完了、Phase 3リリース準備

---

## 5. 統合的実装戦略

### 5.1 Phase間の依存関係

#### 5.1.1 依存関係マップ

```
Phase 1 (Getter追加)
    ↓
Phase 2 (内部最適化) ← Phase 1の完了が前提
    ↓
Phase 3 (非推奨化) ← Phase 2の完了が前提
```

**Phase間の制約**:
- Phase 1 → Phase 2: Getter メソッドの存在が前提
- Phase 2 → Phase 3: 新しい内部実装の安定性が前提
- 各Phaseは独立してリリース可能

#### 5.1.2 共通基盤

**全Phase共通**:
1. **テスト基盤**: 包括的テストスイート
2. **パフォーマンス監視**: ベンチマークとメトリクス
3. **ドキュメント**: API仕様とベストプラクティス
4. **CI/CD**: 自動化されたビルド・テスト・デプロイ

### 5.2 品質保証戦略

#### 5.2.1 テスト戦略

**レベル別テスト**:
1. **単体テスト**: 各メソッド・関数の個別テスト
2. **統合テスト**: コンポーネント間の連携テスト
3. **システムテスト**: エンドツーエンドの動作テスト
4. **パフォーマンステスト**: 性能・スケーラビリティテスト

**カバレッジ目標**:
- **コードカバレッジ**: 95%以上
- **ブランチカバレッジ**: 90%以上
- **パフォーマンステスト**: 各Phase完了時

#### 5.2.2 継続的品質監視

**監視項目**:
- **機能正確性**: 既存機能の破損なし
- **パフォーマンス**: レスポンス時間・スループット
- **メモリ使用量**: メモリリーク・使用量増加
- **セキュリティ**: セキュリティホールの導入なし

### 5.3 リリース戦略

#### 5.3.1 段階的リリース

**リリースタイムライン**:
```
Week 1-2: Phase 1 実装・テスト
Week 3:   Phase 1 リリース (v2.1.0)
Week 4-6: Phase 2 実装・テスト
Week 7:   Phase 2 リリース (v2.2.0)
Week 8-10: Phase 3 実装・テスト
Week 11:  Phase 3 リリース (v2.3.0)
```

**リリース判定基準**:
- 全テストスイートの完全パス
- パフォーマンス目標の達成
- セキュリティ監査の完了
- ドキュメントの完成

#### 5.3.2 フィードバック収集

**フィードバックチャネル**:
1. **内部レビュー**: 開発チーム内でのコードレビュー
2. **ユーザーテスト**: 実際の使用環境でのテスト
3. **パフォーマンス監視**: 本番環境でのメトリクス収集
4. **コミュニティフィードバック**: オープンソースコミュニティからの意見

### 5.4 成功指標とKPI

#### 5.4.1 技術指標

**パフォーマンス指標**:
- `ResolveAllowlistConfiguration` 実行時間: 50%短縮
- メモリアロケーション: 40%削減
- `IsAllowed` 呼び出し時間: O(1)維持

**品質指標**:
- テストカバレッジ: 95%以上維持
- 既存機能の互換性: 100%維持
- セキュリティ脆弱性: ゼロ

#### 5.4.2 運用指標

**開発効率指標**:
- 新機能開発時間: getter使用による効率化
- バグ修正時間: 内部構造最適化による短縮
- コードレビュー時間: 明確なAPI による短縮

**保守性指標**:
- 技術的負債: レガシーコードの段階的削減
- API一貫性: getter pattern による統一
- ドキュメント品質: 移行ガイドの提供

### 5.5 リスク管理と対応策

#### 5.5.1 技術リスク

**高リスク項目**:
1. **既存コード互換性破損**: 段階的移行、包括的テスト
2. **パフォーマンス劣化**: 継続的ベンチマーク、最適化
3. **セキュリティ問題**: セキュリティレビュー、監査

**中リスク項目**:
1. **複雑性増加**: 適切な抽象化、ドキュメント充実
2. **メモリ使用量増加**: プロファイリング、制限設定
3. **テスト不備**: TDD、カバレッジ監視

#### 5.5.2 運用リスク

**リスク軽減策**:
- **段階的ロールアウト**: カナリアリリース
- **監視強化**: メトリクス・アラート設定
- **緊急対応**: ロールバック手順の準備
- **コミュニケーション**: ステークホルダーへの適切な情報共有

### 5.6 長期的展望

#### 5.6.1 将来の拡張計画

**Phase 4以降の検討項目**:
1. **高度な継承モード**: Merge/Override モードの追加
2. **プラグインシステム**: カスタム解決ストラテジー
3. **分散キャッシュ**: 大規模環境での性能向上
4. **機械学習**: 使用パターンに基づく最適化

#### 5.6.2 技術的進化への対応

**継続的改善**:
- **Go言語進化**: 新機能・最適化への対応
- **エコシステム**: 関連ツール・ライブラリとの連携
- **標準化**: 業界標準・ベストプラクティスへの準拠
- **パフォーマンス**: ハードウェア進歩に合わせた最適化

---

## 6. まとめ

### 6.1 実装計画の特徴

**段階的アプローチ**:
- 各Phase独立実装・リリース可能
- リスク最小化と継続的価値提供
- 後方互換性完全維持

**パフォーマンス重視**:
- O(1)検索性能実現
- メモリ効率大幅改善
- 大規模環境対応

**開発者体験向上**:
- 明確なAPI設計
- 包括的ドキュメント
- 充実した移行支援

### 6.2 期待される成果

**技術的成果**:
- 実行時間50%短縮
- メモリ使用量40%削減
- O(1)検索性能実現

**運用的成果**:
- 保守性大幅向上
- 新機能開発効率化
- 技術的負債削減

**組織的成果**:
- 開発チーム生産性向上
- システム信頼性向上
- 長期的競争力強化

この包括的実装計画により、allowlist データ構造最適化を安全かつ効率的に実現し、長期的なシステムの発展基盤を構築できる。
