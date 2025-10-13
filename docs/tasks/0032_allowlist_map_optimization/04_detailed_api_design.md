# Allowlist データ構造最適化 - 詳細 API 設計

## 1. 概要

本ドキュメントでは、allowlist データ構造最適化における新しい API インターフェース、メソッドシグネチャ、および互換性維持戦略の詳細設計を定義する。

### 1.1 IsAllowed 実装の段階的移行

**重要**: 本ドキュメントでは、`IsAllowed`メソッドについて以下の2つの実装を説明している:

1. **Phase 1 (現状の実装)** - セクション4.2
   - `Mode`に基づいて`groupAllowlistSet`または`globalAllowlistSet`を直接検索
   - 現在のコードベースで使用されている実装
   - すでにO(1)の検索性能を持つ

2. **Phase 2 (将来の最適化)** - セクション4.1
   - `effectiveSet`を事前計算してから使用
   - `computeEffectiveSet()`でMode判定を一度だけ実行
   - さらなるパフォーマンス向上を目指す

各セクションでは、該当するPhaseを明示的に記載している。混同しないよう注意すること。

## 2. API 設計原則

### 2.1 設計方針

1. **段階的移行**
   - 既存コードを破壊しない段階的なアプローチ
   - Phase 1: Getter メソッド追加、既存参照の置換
   - Phase 2: 内部実装の最適化
   - Phase 3: 公開フィールドの非推奨化

2. **現行継承モードとの整合性**
   - `Inherit/Explicit/Reject` の3モードに準拠
   - `GlobalOnly/GroupOnly/Merge/Override` は将来の拡張として留保
   - **重要**: 本ドキュメント内のすべての実装例は、現行3モードのいずれかを使用

3. **零コピー指向**
   - 可能な限りデータのコピーを避ける
   - 参照による共有を活用

4. **後方互換性**
   - 既存のパブリック API を破壊しない
   - 段階的な非推奨化をサポート

## 3. 核心データ構造設計

### 3.1 AllowlistResolution の新設計

#### 現在の API（非推奨予定）
```go
type AllowlistResolution struct {
    Mode            InheritanceMode
    GroupAllowlist  []string            // ❌ 公開フィールド（非効率）
    GlobalAllowlist []string            // ❌ 公開フィールド（非効率）
    EffectiveList   []string            // ❌ 公開フィールド（非効率）
    GroupName       string

    groupAllowlistSet  map[string]struct{}  // 内部使用
    globalAllowlistSet map[string]struct{}  // 内部使用
}
```

#### 新しい API 設計（Phase 1）
```go
// AllowlistResolution represents an efficient allowlist resolution
// Phase 1: 既存フィールドを維持しつつ内部最適化とgetter追加
type AllowlistResolution struct {
    Mode            InheritanceMode   // 継承モード（Inherit/Explicit/Reject）
    GroupAllowlist  []string          // 公開 - 既存互換性維持
    GlobalAllowlist []string          // 公開 - 既存互換性維持
    EffectiveList   []string          // 公開 - 既存互換性維持
    GroupName       string            // グループ名

    // 内部データ（効率的な検索用）- 既存
    groupAllowlistSet  map[string]struct{}  // 非公開（検索用）
    globalAllowlistSet map[string]struct{}  // 非公開（検索用）

    // Phase 1 追加: getter用キャッシュ（遅延評価）
    effectiveListCache []string  // GetEffectiveList() 用キャッシュ
}
```

### 3.2 コンストラクタ設計

#### Phase 1 での既存コンストラクタ維持
```go
// 現行の ResolveAllowlistConfiguration は維持（後で最適化）
// Filter.determineInheritanceMode(allowlist) を使用してmodeを決定
func (f *Filter) ResolveAllowlistConfiguration(
    allowlist []string,
    groupName string,
) *AllowlistResolution {
    mode := f.determineInheritanceMode(allowlist)  // 現行のロジック

    resolution := &AllowlistResolution{
        Mode:           mode,
        GroupAllowlist: allowlist,    // 既存互換性
        GroupName:      groupName,
    }

    // 現行のロジックを維持
    globalList := make([]string, 0, len(f.globalAllowlist))
    for variable := range f.globalAllowlist {
        globalList = append(globalList, variable)
    }
    resolution.GlobalAllowlist = globalList

    // 内部 map の設定（既存）
    resolution.SetGroupAllowlistSet(buildAllowlistSet(allowlist))
    resolution.SetGlobalAllowlistSet(f.globalAllowlist)

    // EffectiveList の設定（既存ロジック）
    switch mode {
    case InheritanceModeInherit:
        resolution.EffectiveList = resolution.GlobalAllowlist
    case InheritanceModeExplicit:
        resolution.EffectiveList = resolution.GroupAllowlist
    case InheritanceModeReject:
        resolution.EffectiveList = []string{}
    }

    return resolution
}
```

#### Phase 1 追加メソッド
```go
// Phase 1: 既存フィールド直接参照を置換するgetter
// GetEffectiveList returns effective allowlist with caching for performance
func (r *AllowlistResolution) GetEffectiveList() []string {
    if r == nil {
        return []string{}
    }
    // Phase 1: 既存 EffectiveList をそのまま返す（後で最適化）
    return r.EffectiveList
}

// GetEffectiveSize returns the number of effective allowlist entries
// より効率的な情報取得（len()の代替）
func (r *AllowlistResolution) GetEffectiveSize() int {
    if r == nil {
        return 0
    }
    return len(r.EffectiveList)
}

// GetGroupAllowlist returns group allowlist (compatibility getter)
func (r *AllowlistResolution) GetGroupAllowlist() []string {
    if r == nil {
        return []string{}
    }
    return r.GroupAllowlist
}

// GetGlobalAllowlist returns global allowlist (compatibility getter)
func (r *AllowlistResolution) GetGlobalAllowlist() []string {
    if r == nil {
        return []string{}
    }
    return r.GlobalAllowlist
}
```

## 4. 核心メソッド設計

### 4.1 高性能検索メソッド

#### IsAllowed（最重要 API）

**Phase 2 のターゲット実装(将来の最適化)**
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

**注意**:
- 現状の実装(Phase 1)については、セクション4.2を参照してください。
- `effectiveSet == nil`は不変条件違反であり、panicすることを推奨します。詳細はセクション8.1を参照。

#### Contains Variations（用途別最適化）
```go
// ContainsInGroup checks if a variable exists in group-specific allowlist only.
// Useful for debugging and policy validation.
func (r *AllowlistResolution) ContainsInGroup(variable string) bool {
    if r == nil || r.groupAllowlistSet == nil {
        return false
    }
    _, exists := r.groupAllowlistSet[variable]
    return exists
}

// ContainsInGlobal checks if a variable exists in global allowlist only.
// Useful for debugging and policy validation.
func (r *AllowlistResolution) ContainsInGlobal(variable string) bool {
    if r == nil || r.globalAllowlistSet == nil {
        return false
    }
    _, exists := r.globalAllowlistSet[variable]
    return exists
}
```

### 4.2 現状の実装

#### IsAllowed の現状の実装(Phase 1)

**現在の実装**: Mode に基づいて直接判定
```go
// IsAllowed checks if a variable is allowed in the effective allowlist.
// This is the most frequently called method and is optimized for O(1) performance.
//
// 現状の実装(Phase 1): Mode に基づいて直接判定
// - InheritanceModeReject: 常に false
// - InheritanceModeExplicit: groupAllowlistSet を検索
// - InheritanceModeInherit: globalAllowlistSet を検索
//
// この実装はすでに最適化されており、O(1) の検索性能を持つ。
// Phase 2 では、effectiveSet を事前計算することでさらに最適化する。
//
// Parameters:
//   - variable: environment variable name to check
// Returns: true if the variable is allowed, false otherwise
func (r *AllowlistResolution) IsAllowed(variable string) bool {
    if r == nil {
        return false
    }

    switch r.Mode {
    case InheritanceModeReject:
        return false
    case InheritanceModeExplicit:
        if r.groupAllowlistSet == nil {
            return false
        }
        _, ok := r.groupAllowlistSet[variable]
        return ok
    case InheritanceModeInherit:
        if r.globalAllowlistSet == nil {
            return false
        }
        _, ok := r.globalAllowlistSet[variable]
        return ok
    default:
        return false
    }
}
```

#### Phase 1 での Getter メソッド設計

```go
// Phase 1 では slice フィールドの完全除去は行わない
// getter メソッドは既存フィールドのプロキシとして機能
```

### 4.3 メタ情報アクセッサ

#### 基本情報の取得
```go
// GetMode returns the inheritance mode used for this resolution.
func (r *AllowlistResolution) GetMode() InheritanceMode {
    if r == nil {
        return InheritanceModeInherit  // safe default
    }
    return r.mode
}

// GetGroupName returns the name of the group this resolution is for.
func (r *AllowlistResolution) GetGroupName() string {
    if r == nil {
        return ""
    }
    return r.groupName
}

// GetGroupSize returns the number of variables in the group allowlist.
func (r *AllowlistResolution) GetGroupSize() int {
    if r == nil || r.groupAllowlistSet == nil {
        return 0
    }
    return len(r.groupAllowlistSet)
}

// GetGlobalSize returns the number of variables in the global allowlist.
func (r *AllowlistResolution) GetGlobalSize() int {
    if r == nil || r.globalAllowlistSet == nil {
        return 0
    }
    return len(r.globalAllowlistSet)
}

// GetEffectiveSize returns the number of variables in the effective allowlist.
func (r *AllowlistResolution) GetEffectiveSize() int {
    if r == nil || r.effectiveSet == nil {
        return 0
    }
    return len(r.effectiveSet)
}
```

## 5. Filter コンポーネント API 設計

### 5.1 ResolveAllowlistConfiguration の最適化

#### Phase 1 での ResolveAllowlistConfiguration
```go
// Phase 1: 既存実装を維持、後の Phase で最適化
func (f *Filter) ResolveAllowlistConfiguration(
    allowlist []string,     // 現行のシグネチャを維持
    groupName string,
) *AllowlistResolution {
    // 現行のロジックを保持
    mode := f.determineInheritanceMode(allowlist)

    resolution := &AllowlistResolution{
        Mode:           mode,
        GroupAllowlist: allowlist,
        GroupName:      groupName,
    }

    // 現在の非効率な変換（Phase 2で最適化予定）
    globalList := make([]string, 0, len(f.globalAllowlist))
    for variable := range f.globalAllowlist {
        globalList = append(globalList, variable)
    }
    resolution.GlobalAllowlist = globalList

    // 内部map設定（既存）
    resolution.SetGroupAllowlistSet(buildAllowlistSet(allowlist))
    resolution.SetGlobalAllowlistSet(f.globalAllowlist)

    // EffectiveList設定（既存）
    switch mode {
    case InheritanceModeInherit:
        resolution.EffectiveList = resolution.GlobalAllowlist
    case InheritanceModeExplicit:
        resolution.EffectiveList = resolution.GroupAllowlist
    case InheritanceModeReject:
        resolution.EffectiveList = []string{}
    }

    return resolution
}
```

### 5.2 高レベル検索 API

#### 便利メソッドの提供
```go
// IsVariableAccessAllowed provides a high-level interface for variable access checking.
// This method is optimized for frequent calls by minimizing object creation.
func (f *Filter) IsVariableAccessAllowed(
    groupName string,
    allowlist []string,
    variable string,
) bool {
    resolution := f.ResolveAllowlistConfiguration(groupName, allowlist)
    return resolution.IsAllowed(variable)
}

// BatchCheckVariableAccess checks multiple variables efficiently.
// Returns a map indicating which variables are allowed.
func (f *Filter) BatchCheckVariableAccess(
    groupName string,
    allowlist []string,
    variables []string,
) map[string]bool {
    resolution := f.ResolveAllowlistConfiguration(groupName, allowlist)
    result := make(map[string]bool, len(variables))

    for _, variable := range variables {
        result[variable] = resolution.IsAllowed(variable)
    }

    return result
}
```

## 6. ユーティリティ関数設計

### 6.1 Set 操作ユーティリティ

#### 効率的な Set 構築
```go
// buildAllowlistSet converts a string slice to an optimized set representation.
// Returns: map[string]struct{} for O(1) lookups
func buildAllowlistSet(allowlist []string) map[string]struct{} {
    if len(allowlist) == 0 {
        return make(map[string]struct{})
    }

    set := make(map[string]struct{}, len(allowlist))
    for _, variable := range allowlist {
        if variable != "" {  // skip empty strings
            set[variable] = struct{}{}
        }
    }

    return set
}

// cloneAllowlistSet creates a deep copy of an allowlist set.
// Use this when you need to modify a set without affecting the original.
func cloneAllowlistSet(original map[string]struct{}) map[string]struct{} {
    if original == nil {
        return make(map[string]struct{})
    }

    clone := make(map[string]struct{}, len(original))
    for k, v := range original {
        clone[k] = v
    }

    return clone
}
```

#### Set から Slice への変換
```go
// setToSortedSlice converts a set to a sorted string slice.
// The sorting ensures consistent output for testing and debugging.
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

### 6.2 Effective Set 計算

#### Phase 2 以降での継承モード最適化実装（将来）
```go
// Phase 2 以降で実装予定の最適化された継承モード処理
// 現行の3モード（Inherit/Explicit/Reject）での実装例

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
    // (上記のすべてのケースで設定されるはずだが、防御的にチェック)
    if r.effectiveSet == nil {
        panic("AllowlistResolution: internal error - effectiveSet is still nil after computeEffectiveSet()")
    }
}

// Phase 3以降: Merge/Override モードの追加検討
// 現在は inherit/explicit/reject の3モードのみサポート

// mergeAllowlistSets efficiently merges two allowlist sets.
func (r *AllowlistResolution) mergeAllowlistSets(
    global map[string]struct{},
    group map[string]struct{},
) map[string]struct{} {
    // Optimize for common cases
    if len(global) == 0 {
        return group
    }
    if len(group) == 0 {
        return global
    }

    // Create merged set with appropriate capacity
    merged := make(map[string]struct{}, len(global)+len(group))

    // Add global variables
    for variable := range global {
        merged[variable] = struct{}{}
    }

    // Add group variables (overwrites are no-op for struct{})
    for variable := range group {
        merged[variable] = struct{}{}
    }

    return merged
}
```

## 7. 互換性維持 API

### 7.1 Legacy Field Accessors

#### 既存コードをサポートするための getter
```go
// GroupAllowlist returns the group allowlist as a string slice.
// DEPRECATED: Use GetGroupAllowlist() instead for better performance tracking.
func (r *AllowlistResolution) GroupAllowlist() []string {
    // Log deprecation warning in development builds
    if isDebugBuild() {
        log.Printf("DEPRECATED: AllowlistResolution.GroupAllowlist field access. Use GetGroupAllowlist() method instead.")
    }
    return r.GetGroupAllowlist()
}

// GlobalAllowlist returns the global allowlist as a string slice.
// DEPRECATED: Use GetGlobalAllowlist() instead for better performance tracking.
func (r *AllowlistResolution) GlobalAllowlist() []string {
    if isDebugBuild() {
        log.Printf("DEPRECATED: AllowlistResolution.GlobalAllowlist field access. Use GetGlobalAllowlist() method instead.")
    }
    return r.GetGlobalAllowlist()
}

// EffectiveList returns the effective allowlist as a string slice.
// DEPRECATED: Use GetEffectiveList() instead for better performance tracking.
func (r *AllowlistResolution) EffectiveList() []string {
    if isDebugBuild() {
        log.Printf("DEPRECATED: AllowlistResolution.EffectiveList field access. Use GetEffectiveList() method instead.")
    }
    return r.GetEffectiveList()
}
```

### 7.2 Migration Utilities

#### 移行支援ユーティリティ

```go
// ValidateAllowlistResolution checks the integrity of an AllowlistResolution.
// Useful for debugging and testing during migration.
//
// この関数は不変条件(invariants)を検証する。不変条件違反が検出された場合、
// エラーを返すが、これは開発/テスト時の診断用である。
// 実際の IsAllowed() では不変条件違反は panic すべき(セクション8.1参照)。
//
// Phase 2 での不変条件:
//   - groupAllowlistSet は nil であってはならない
//   - globalAllowlistSet は nil であってはならない
//   - effectiveSet は nil であってはならない(初期化後)
//
// Returns:
//   - nil: すべての不変条件が満たされている
//   - error: 不変条件違反が検出された
func ValidateAllowlistResolution(r *AllowlistResolution) error {
    if r == nil {
        return errors.New("AllowlistResolution is nil")
    }

    // Phase 2 での不変条件チェック
    if r.groupAllowlistSet == nil {
        return fmt.Errorf("invariant violation: groupAllowlistSet is nil")
    }

    if r.globalAllowlistSet == nil {
        return fmt.Errorf("invariant violation: globalAllowlistSet is nil")
    }

    if r.effectiveSet == nil {
        return fmt.Errorf("invariant violation: effectiveSet is nil - object not properly initialized (did you call computeEffectiveSet()?)")
    }

    // 整合性チェック: effective set のサイズが妥当か
    // (これはエラーではなく警告レベルかもしれない)
    effectiveSize := len(r.effectiveSet)
    groupSize := len(r.groupAllowlistSet)
    globalSize := len(r.globalAllowlistSet)

    // Mode に基づく期待されるサイズの検証
    switch r.Mode {
    case InheritanceModeReject:
        if effectiveSize != 0 {
            return fmt.Errorf("invariant violation: Reject mode should have empty effectiveSet, got %d entries", effectiveSize)
        }
    case InheritanceModeExplicit:
        if effectiveSize != groupSize {
            return fmt.Errorf("invariant violation: Explicit mode should have effectiveSet == groupSet, got %d vs %d", effectiveSize, groupSize)
        }
    case InheritanceModeInherit:
        if effectiveSize != globalSize {
            return fmt.Errorf("invariant violation: Inherit mode should have effectiveSet == globalSet, got %d vs %d", effectiveSize, globalSize)
        }
    }

    return nil
}

// CompareAllowlistResolutions compares two AllowlistResolution instances.
// Useful for testing and validation during migration.
func CompareAllowlistResolutions(a, b *AllowlistResolution) bool {
    if (a == nil) != (b == nil) {
        return false
    }
    if a == nil {
        return true
    }

    return a.GetMode() == b.GetMode() &&
           a.GetGroupName() == b.GetGroupName() &&
           slicesEqual(a.GetGroupAllowlist(), b.GetGroupAllowlist()) &&
           slicesEqual(a.GetGlobalAllowlist(), b.GetGlobalAllowlist()) &&
           slicesEqual(a.GetEffectiveList(), b.GetEffectiveList())
}
```

## 8. エラーハンドリング設計

### 8.1 Graceful Degradation

#### 防御的プログラミングの実装

#### 不変条件の違反処理

**重要な設計決定**: `effectiveSet == nil`の扱い

`effectiveSet`がnilである状態は、不変条件違反(invariant violation)であり、
オブジェクトが正しく初期化されなかったことを示す。この状態は以下の理由でpanicすべき:

1. **プログラミングエラーの検出**: 初期化忘れや不正な構築を即座に検出
2. **バグの隠蔽防止**: falseを返すだけでは根本原因が隠蔽される
3. **セキュリティ**: allowlist検証の失敗は重大なセキュリティリスク

**Phase 2 以降での実装例**: effectiveSet を使用する場合
```go
// IsAllowed with comprehensive error handling (Phase 2 implementation)
// Phase 2 では effectiveSet を使用するため、このエラーハンドリングが必要
func (r *AllowlistResolution) IsAllowed(variable string) bool {
    // Handle nil receiver - これは呼び出し側のエラーなので false を返す
    if r == nil {
        return false
    }

    // Handle empty variable name - これは入力検証なので false を返す
    if variable == "" {
        return false
    }

    // INVARIANT: effectiveSet must not be nil after proper initialization
    // これは不変条件違反なので panic する
    if r.effectiveSet == nil {
        panic("AllowlistResolution: effectiveSet is nil - object not properly initialized")
    }

    _, allowed := r.effectiveSet[variable]
    return allowed
}
```

**代替案: 開発時のみpanic**
```go
// より穏健なアプローチ: 開発時はpanic、本番環境ではログ + false
func (r *AllowlistResolution) IsAllowed(variable string) bool {
    if r == nil {
        return false
    }

    if variable == "" {
        return false
    }

    // INVARIANT: effectiveSet must not be nil
    if r.effectiveSet == nil {
        const errMsg = "AllowlistResolution: effectiveSet is nil - object not properly initialized"

        // 開発/テスト環境では即座にpanic
        if isDebugBuild() || isTesting() {
            panic(errMsg)
        }

        // 本番環境ではエラーログ + メトリクス + false
        log.Errorf(errMsg)
        metrics.IncrementInvariantViolations()
        return false  // fail-safe: 不正な状態ではアクセスを拒否
    }

    _, allowed := r.effectiveSet[variable]
    return allowed
}
```

**推奨**: 本プロジェクトではセキュリティが重要なため、不変条件違反は常にpanicすることを推奨。
テストで確実に検出され、本番環境では適切な初期化が保証される。

**Phase 1 での実装**: Mode に基づく判定では、effectiveSetを使用しないため、
この問題は発生しない。セクション4.2の現状の実装では、`groupAllowlistSet`や
`globalAllowlistSet`がnilの場合は単にfalseを返す(これは正常な動作)。

### 8.2 診断とデバッグ支援

#### デバッグ情報の提供
```go
// GetDiagnosticInfo returns detailed information about the AllowlistResolution state.
// This is intended for debugging and should not be used in production code paths.
func (r *AllowlistResolution) GetDiagnosticInfo() map[string]interface{} {
    if r == nil {
        return map[string]interface{}{
            "error": "AllowlistResolution is nil",
        }
    }

    return map[string]interface{}{
        "mode":             r.GetMode().String(),
        "groupName":        r.GetGroupName(),
        "groupSize":        r.GetGroupSize(),
        "globalSize":       r.GetGlobalSize(),
        "effectiveSize":    r.GetEffectiveSize(),
        "cacheStatus": map[string]bool{
            "groupCache":     r.groupAllowlistCache != nil,
            "globalCache":    r.globalAllowlistCache != nil,
            "effectiveCache": r.effectiveListCache != nil,
        },
    }
}

// String provides a human-readable representation for debugging.
func (r *AllowlistResolution) String() string {
    if r == nil {
        return "AllowlistResolution(nil)"
    }

    return fmt.Sprintf(
        "AllowlistResolution(mode=%s, group=%s, sizes=[group:%d, global:%d, effective:%d])",
        r.GetMode().String(),
        r.GetGroupName(),
        r.GetGroupSize(),
        r.GetGlobalSize(),
        r.GetEffectiveSize(),
    )
}
```

## 9. パフォーマンス監視 API

### 9.1 メトリクス収集

#### パフォーマンス統計の追跡
```go
// PerformanceMetrics tracks usage patterns and performance characteristics
type AllowlistResolutionMetrics struct {
    CreationCount     int64  // Number of AllowlistResolution instances created
    IsAllowedCalls    int64  // Number of IsAllowed() calls
    GetterCalls       int64  // Number of getter method calls
    CacheHits         int64  // Number of cache hits
    AverageGroupSize  float64 // Average group allowlist size
    AverageGlobalSize float64 // Average global allowlist size
}

// GetMetrics returns current performance metrics.
// This can be used for monitoring and optimization.
func GetAllowlistResolutionMetrics() AllowlistResolutionMetrics {
    metricsLock.RLock()
    defer metricsLock.RUnlock()
    return globalMetrics
}

// ResetMetrics clears all performance metrics.
// Useful for benchmarking specific code paths.
func ResetAllowlistResolutionMetrics() {
    metricsLock.Lock()
    defer metricsLock.Unlock()
    globalMetrics = AllowlistResolutionMetrics{}
}
```

### 9.2 プロファイリング支援

#### メモリとCPU使用量の監視
```go
// EnableProfiling enables detailed performance profiling.
// This should only be used during development and testing.
func (r *AllowlistResolution) EnableProfiling() {
    r.profilingEnabled = true
    r.creationTime = time.Now()
}

// GetProfilingInfo returns detailed profiling information.
func (r *AllowlistResolution) GetProfilingInfo() map[string]interface{} {
    if !r.profilingEnabled {
        return map[string]interface{}{
            "error": "profiling not enabled",
        }
    }

    return map[string]interface{}{
        "creationTime":    r.creationTime,
        "ageMilliseconds": time.Since(r.creationTime).Milliseconds(),
        "memoryUsage":     r.estimateMemoryUsage(),
        "cacheEfficiency": r.calculateCacheEfficiency(),
    }
}
```

## 10. 型安全性強化

### 10.1 型エイリアスの活用

#### より明確な型定義
```go
// VariableName represents an environment variable name
type VariableName string

// AllowlistSet represents an efficient set of allowed variables
type AllowlistSet map[VariableName]struct{}

// Enhanced type-safe methods
func (r *AllowlistResolution) IsVariableAllowed(variable VariableName) bool {
    return r.IsAllowed(string(variable))
}

func NewAllowlistSet(variables []VariableName) AllowlistSet {
    set := make(AllowlistSet, len(variables))
    for _, variable := range variables {
        set[variable] = struct{}{}
    }
    return set
}
```

### 10.2 Builder パターン (Phase 2 以降)

**重要**: このBuilderパターンは**Phase 2以降**で導入されます。

**理由**:
- Phase 1では`ResolveAllowlistConfiguration`のみ使用
- `NewAllowlistResolution`コンストラクタはPhase 2で導入
- Builderは`NewAllowlistResolution`に依存するため、Phase 2以降が適切

#### Phase 2 以降での複雑な設定を簡単にする Builder

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
// Available in Phase 2 and later.
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
// Requires Phase 2's NewAllowlistResolution constructor
func (b *AllowlistResolutionBuilder) Build() *AllowlistResolution {
    groupSet := buildAllowlistSet(b.groupVars)
    globalSet := buildAllowlistSet(b.globalVars)
    // Phase 2 コンストラクタを使用
    return NewAllowlistResolution(b.mode, b.groupName, groupSet, globalSet)
}
```

**使用例** (Phase 2以降):
```go
resolution := NewAllowlistResolutionBuilder().
    WithMode(InheritanceModeInherit).
    WithGroupName("test-group").
    WithGlobalVariables([]string{"PATH", "HOME"}).
    WithGroupVariables([]string{"APP_ENV"}).
    Build()
```

## 11. テスト支援 API

### 11.1 テストユーティリティ

#### テスト用のファクトリメソッド
```go
// TestAllowlistResolutionFactory creates AllowlistResolution for testing
type TestAllowlistResolutionFactory struct{}

// CreateSimple creates a simple AllowlistResolution for basic testing
// Uses InheritanceModeInherit (current implementation supports Inherit/Explicit/Reject)
func (f TestAllowlistResolutionFactory) CreateSimple(
    globalVars []string,
    groupVars []string,
) *AllowlistResolution {
    return NewAllowlistResolutionBuilder().
        WithMode(InheritanceModeInherit).  // 現行の3モードのいずれかを使用
        WithGroupName("test-group").
        WithGlobalVariables(globalVars).
        WithGroupVariables(groupVars).
        Build()
}

// CreateWithMode creates AllowlistResolution with specific inheritance mode
// Supports the current 3 modes: Inherit, Explicit, Reject
func (f TestAllowlistResolutionFactory) CreateWithMode(
    mode InheritanceMode,
    globalVars []string,
    groupVars []string,
) *AllowlistResolution {
    return NewAllowlistResolutionBuilder().
        WithMode(mode).
        WithGroupName("test-group").
        WithGlobalVariables(globalVars).
        WithGroupVariables(groupVars).
        Build()
}
```

**使用例: 現行3モードのテスト**
```go
// Inherit mode: グローバルallowlistを使用
inheritResolution := NewTestAllowlistResolutionWithMode(
    InheritanceModeInherit,
    []string{"PATH", "HOME"},  // global
    []string{"APP_ENV"},       // group (無視される)
)

// Explicit mode: グループallowlistのみ使用
explicitResolution := NewTestAllowlistResolutionWithMode(
    InheritanceModeExplicit,
    []string{"PATH", "HOME"},  // global (無視される)
    []string{"APP_ENV"},       // group
)

// Reject mode: すべて拒否
rejectResolution := NewTestAllowlistResolutionWithMode(
    InheritanceModeReject,
    []string{"PATH", "HOME"},  // global (無視される)
    []string{"APP_ENV"},       // group (無視される)
)
```

### 11.2 アサーションヘルパー

#### テスト用の比較ユーティリティ
```go
// AssertAllowlistResolutionEqual compares two AllowlistResolution instances for testing
func AssertAllowlistResolutionEqual(t *testing.T, expected, actual *AllowlistResolution) {
    t.Helper()

    if !CompareAllowlistResolutions(expected, actual) {
        t.Errorf("AllowlistResolution mismatch:\nExpected: %s\nActual: %s",
                 expected, actual)
    }
}

// AssertVariableAllowed checks if a variable is allowed and reports detailed error
func AssertVariableAllowed(t *testing.T, resolution *AllowlistResolution, variable string) {
    t.Helper()

    if !resolution.IsAllowed(variable) {
        t.Errorf("Variable %q should be allowed. Resolution: %s",
                 variable, resolution.String())
    }
}

// AssertVariableDenied checks if a variable is denied and reports detailed error
func AssertVariableDenied(t *testing.T, resolution *AllowlistResolution, variable string) {
    t.Helper()

    if resolution.IsAllowed(variable) {
        t.Errorf("Variable %q should be denied. Resolution: %s",
                 variable, resolution.String())
    }
}
```

## 12. ドキュメント生成支援

### 12.1 自動ドキュメント生成

#### API ドキュメント用のメタデータ
```go
// APIMetadata provides metadata for documentation generation
type APIMetadata struct {
    Version     string
    Deprecated  bool
    Since       string
    Performance string
    Usage       string
}

// GetAPIMetadata returns metadata for each public method
func GetAllowlistResolutionAPIMetadata() map[string]APIMetadata {
    return map[string]APIMetadata{
        "IsAllowed": {
            Version:     "1.0",
            Performance: "O(1) - Optimized for frequent calls",
            Usage:       "Primary method for checking variable access",
        },
        "GetGroupAllowlist": {
            Version:     "2.0",
            Performance: "O(n) - Use sparingly, result is cached",
            Usage:       "For debugging and compatibility",
        },
        "GroupAllowlist": {
            Version:     "1.0",
            Deprecated:  true,
            Since:       "2.0",
            Performance: "O(n) - Triggers deprecation warning",
            Usage:       "Legacy compatibility - use GetGroupAllowlist()",
        },
    }
}
```

## 13. 設定と調整

### 13.1 パフォーマンス調整パラメータ

#### 調整可能な設定
```go
// AllowlistResolutionConfig holds configuration for performance tuning
type AllowlistResolutionConfig struct {
    EnableCaching          bool  // Enable getter result caching
    EnableDeprecationWarns bool  // Enable deprecation warnings
    EnableMetrics          bool  // Enable performance metrics collection
    MaxCacheSize          int   // Maximum number of cached slice conversions
    PreallocateCapacity   int   // Preallocate capacity for sets
}

// DefaultAllowlistResolutionConfig returns the default configuration
func DefaultAllowlistResolutionConfig() AllowlistResolutionConfig {
    return AllowlistResolutionConfig{
        EnableCaching:          true,
        EnableDeprecationWarns: true,
        EnableMetrics:          false,
        MaxCacheSize:          100,
        PreallocateCapacity:   16,
    }
}

// SetAllowlistResolutionConfig updates the global configuration
func SetAllowlistResolutionConfig(config AllowlistResolutionConfig) {
    configLock.Lock()
    defer configLock.Unlock()
    globalConfig = config
}
```

## 14. 今後の拡張性

### 14.1 プラグイン インターフェース

#### 将来の拡張に備えたインターフェース
```go
// AllowlistResolver defines the interface for allowlist resolution strategies
type AllowlistResolver interface {
    // ResolveAllowlist computes the effective allowlist based on inheritance rules
    ResolveAllowlist(global, group map[string]struct{}) map[string]struct{}

    // GetResolutionMode returns the resolution mode name
    GetResolutionMode() string
}

// RegisterAllowlistResolver registers a custom resolution strategy
func RegisterAllowlistResolver(mode string, resolver AllowlistResolver) error {
    if _, exists := customResolvers[mode]; exists {
        return fmt.Errorf("resolver for mode %q already registered", mode)
    }

    customResolvers[mode] = resolver
    return nil
}
```

### 14.2 バージョニング

#### API バージョン管理
```go
// APIVersion represents the API version
type APIVersion struct {
    Major int
    Minor int
    Patch int
}

// GetAPIVersion returns the current API version
func GetAPIVersion() APIVersion {
    return APIVersion{
        Major: 2,
        Minor: 0,
        Patch: 0,
    }
}

// IsCompatible checks if the client version is compatible
func IsCompatible(clientVersion APIVersion) bool {
    current := GetAPIVersion()
    return clientVersion.Major == current.Major &&
           clientVersion.Minor <= current.Minor
}
```

## 15. まとめ

### 15.1 API 設計の特徴

1. **パフォーマンス最適化**
   - O(1) 検索操作（`IsAllowed`）
   - 遅延評価によるメモリ効率
   - Zero-copy データ共有

2. **段階的移行**
   - Phase 1: 現状の実装を維持しつつgetter追加
   - Phase 2: effectiveSet による最適化
   - 各Phaseで明確な目標設定と実装の区別

3. **後方互換性**
   - 段階的な非推奨化
   - レガシー API の完全サポート
   - 移行支援ユーティリティ

4. **開発者体験**
   - 明確な責任分離
   - 包括的なエラーハンドリング
   - 豊富なデバッグ支援

5. **将来性**
   - プラグイン対応設計
   - 設定可能なパフォーマンス調整
   - API バージョニング

### 15.2 実装優先順位

#### Phase 1 (High Priority)
1. Getter メソッドの追加
   - `GetEffectiveList()`
   - `GetEffectiveSize()`
   - `GetGroupAllowlist()`
   - `GetGlobalAllowlist()`
2. `expansion.go` での直接参照の置換
3. 後方互換性の検証

#### Phase 2 (High Priority)
1. `NewAllowlistResolution` コンストラクタの導入
2. `AllowlistResolution` の内部構造変更
   - `effectiveSet` フィールドの追加
   - `computeEffectiveSet()` の実装
3. `IsAllowed` メソッドの最適化(effectiveSetベース)
4. `ResolveAllowlistConfiguration` の最適化
5. Builder パターンの導入

#### Phase 2/3 (Medium Priority)
1. キャッシュ機能の強化
2. レガシー互換性 API の追加
3. エラーハンドリングの強化
4. 不変条件の検証強化

#### Phase 3以降 (Low Priority)
1. パフォーマンス監視機能
2. 高度なテスト支援 API
3. プラグイン対応
4. 将来の拡張性対応(Merge/Overrideモード等)

## 16. 段階的移行戦略

### 16.1 Phase 1: Getter メソッド追加と既存参照の置換

#### 目標
- 既存の公開フィールドを残しつつ、getter メソッドを追加
- `expansion.go` の直接参照を getter 呼び出しに置換
- 構造体の破壊的変更を避ける

#### 実装内容
1. `GetEffectiveList()` メソッドの追加
   ```go
   func (r *AllowlistResolution) GetEffectiveList() []string {
       return r.EffectiveList  // Phase 1では既存フィールドを返す
   }
   ```

2. `GetEffectiveSize()` メソッドの追加（log用）
   ```go
   func (r *AllowlistResolution) GetEffectiveSize() int {
       return len(r.EffectiveList)
   }
   ```

3. 参照箇所の置換
   - `expansion.go:125` `resolution.EffectiveList` → `resolution.GetEffectiveList()`
   - `expansion.go:372` `resolution.EffectiveList` → `resolution.GetEffectiveList()`
   - `expansion.go:562` `resolution.EffectiveList` → `resolution.GetEffectiveList()`
   - log の `len(resolution.EffectiveList)` → `resolution.GetEffectiveSize()`

#### 成功基準
- 全ての既存テストがパス
- `expansion.go` での直接フィールド参照がゼロ
- パフォーマンスの劣化がない

### 16.2 Phase 2: 内部実装の最適化

#### 目標
- map → slice → map の変換排除
- `effectiveSet` の事前計算導入
- メモリ使用量削減
- 新しいコンストラクタAPIの導入

#### 実装内容

**1. NewAllowlistResolution コンストラクタの追加**
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
    r := &AllowlistResolution{
        Mode:               mode,
        GroupName:          groupName,
        groupAllowlistSet:  groupSet,
        globalAllowlistSet: globalSet,
    }

    // effectiveSet を事前計算
    r.computeEffectiveSet()

    // Phase 2 では slice フィールドは遅延生成に変更
    // (後方互換性のため空のsliceで初期化)
    r.GroupAllowlist = []string{}
    r.GlobalAllowlist = []string{}
    r.EffectiveList = []string{}

    return r
}
```

**2. ResolveAllowlistConfiguration の最適化**
- Phase 1の実装から Phase 2の最適化実装へ移行
- `NewAllowlistResolution` を内部で使用

**3. 内部データ構造の変更**
- `EffectiveList` の遅延生成への切り替え
- `effectiveSet` フィールドの追加

**4. Builder パターンの導入**
- `AllowlistResolutionBuilder` の追加(セクション10.2参照)
- テストコード用のファクトリメソッド

**5. パフォーマンステストによる検証**

#### Phase 2 で利用可能になるAPI
- `NewAllowlistResolution()` - 新しいコンストラクタ
- `AllowlistResolutionBuilder` - Builderパターン
- `IsAllowed()` - effectiveSetベースの実装(セクション4.1)
- `computeEffectiveSet()` - 内部メソッド(セクション6.2)

#### 成功基準
- `ResolveAllowlistConfiguration` の実行時間50%短縮
- メモリアロケーション40%削減
- 大規模 allowlist でのパフォーマンス向上
- すべての既存テストがパス

### 16.3 Phase 3: 公開フィールドの非推奨化（将来）

#### 目標
- 公開フィールドの段階的廃止
- 完全な map ベース内部実装への移行

#### 実装内容
1. 公開フィールドの非推奨マーク
2. 移行ガイドの提供
3. 段階的な削除検討

### 16.4 リスク軽減策

#### テスト戦略
- 包括的な単体テスト
- 段階的リリースとロールバック準備
- パフォーマンス監視とアラート

#### 互換性保証
- 既存 API の動作保証
- 段階的な非推奨化プロセス
- 緊急時のフォールバック機能

この詳細 API 設計により、現行の `Inherit/Explicit/Reject` 継承モードと `Filter.determineInheritanceMode` の実装に準拠しつつ、段階的な最適化を実現できる allowlist 管理システムを構築することができる。
