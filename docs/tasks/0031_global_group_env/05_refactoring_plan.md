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

### Phase 2: ExpandCommandEnv のシグネチャ改善 🔄

**目標**: `ExpandCommandEnv` が group オブジェクトを受け取るように変更

**タスク**:

- [ ] `ExpandCommandEnv` の引数変更
  - [ ] `groupName string` → `group *runnertypes.CommandGroup`
- [ ] `ExpansionContext` 構造体の更新
  - [ ] `GroupName string` → `Group *runnertypes.CommandGroup`
- [ ] `ExpandCommand` 関数の更新
  - [ ] 新しい `ExpandCommandEnv` シグネチャに合わせる
  - [ ] `group.Name` を使用するように変更
- [ ] テストの更新
  - [ ] `ExpandCommandEnv` 直接呼び出しのテストを更新
  - [ ] `ExpandCommand` のテストを更新
- [ ] コードフォーマットとリント実行

**期待される効果**:
- 型安全性の向上
- `groupName` パラメータの削減
- group オブジェクトへの直接アクセス

**リスク評価**: 中（内部 API の破壊的変更）

---

### Phase 3: allowlist 計算の完全内部化 🔄

**目標**: `ExpandCommandEnv` 内部で allowlist 継承を計算

**タスク**:

- [ ] `ExpandCommandEnv` の引数変更
  - [ ] `allowlist []string` → `globalAllowlist []string`
- [ ] `expandEnvInternal` での allowlist 継承計算を活用
  - [ ] `localAllowlist` として `group.EnvAllowlist` を渡す
  - [ ] `globalAllowlist` として global allowlist を渡す
- [ ] bootstrap/config.go の更新
  - [ ] `DetermineEffectiveAllowlist` 呼び出しを削除
  - [ ] `cfg.Global.EnvAllowlist` を直接渡す
- [ ] `ExpandCommand` 関数の更新
  - [ ] `ExpansionContext.EnvAllowlist` の意味を明確化（globalAllowlist）
- [ ] テストの更新
  - [ ] allowlist 継承のテストケースを追加
  - [ ] 既存のテストを新しいシグネチャに合わせる
- [ ] コードフォーマットとリント実行

**期待される効果**:
- allowlist 計算の完全な一元化
- 呼び出し側のコードが簡潔に
- 3つの関数すべてで統一されたallowlist 継承パターン

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
