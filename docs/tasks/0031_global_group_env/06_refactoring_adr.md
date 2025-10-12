# ADR: Command展開処理のアーキテクチャ統合

## ステータス

提案中（Proposed）

## コンテキスト

Task 0031（Global・Groupレベル環境変数設定機能）のPhase 4実装中に、以下の問題が発見された：

### 現状の問題点

1. **展開処理の重複**
   - `Global.Env`と`Group.Env`が2回展開されている
   - 1回目: `config.Loader.processConfig()`
   - 2回目: `bootstrap.LoadAndPrepareConfig()`（重複）

2. **autoEnv生成の重複**
   - 自動環境変数（`__RUNNER_DATETIME`、`__RUNNER_PID`）が2回生成される
   - 両方で同じ値が生成されるが、処理が無駄

3. **責務の不明確さ**
   - `config.Loader`: Global/Group.Env展開
   - `bootstrap`: Global/Group.Env展開（重複）+ Command.Env/Cmd/Args展開
   - 責務分離の境界が不明確

4. **コメントと実装の不一致**
   - コメント: "This separation maintains clean architectural boundaries"
   - 実態: 重複展開により境界が不明確

### 現在のアーキテクチャ（Option 0）

```
[config.Loader]
  └─ processConfig()
      ├─ autoEnv生成
      ├─ ExpandGlobalEnv()
      ├─ ExpandGroupEnv()
      └─ ExpandGroupVerifyFiles()

[bootstrap.LoadAndPrepareConfig]
  ├─ config.Loader.LoadConfig()       ← 上記を呼ぶ
  ├─ autoEnv生成（重複）
  ├─ ExpandGlobalEnv()（重複）
  ├─ ExpandGroupEnv()（重複）
  └─ ExpandCommand()                   ← Command.Env/Cmd/Args
      ├─ ExpandCommandEnv()
      ├─ Cmd展開
      └─ Args展開
```

## 検討した選択肢

### Option 1: Command.EnvをprocessConfigで展開

**アーキテクチャ:**
```
[config.Loader]
  └─ processConfig()
      ├─ ExpandGlobalEnv()
      ├─ ExpandGroupEnv()
      ├─ ExpandGroupVerifyFiles()
      └─ ExpandCommandEnv()            ← NEW

[bootstrap.LoadAndPrepareConfig]
  ├─ config.Loader.LoadConfig()
  └─ ExpandCommandStrings()            ← NEW: Cmd/Argsのみ
```

**責務分離:**
- **config.Loader**: すべての環境変数展開
- **bootstrap**: コマンド文字列展開（Cmd/Args）

**Pros:**
- ✅ 環境変数展開がconfig.Loaderで完結
- ✅ 重複展開が解消される
- ✅ 「環境変数」と「コマンド文字列」で責務が明確
- ✅ テストしやすい
- ✅ 実装変更の影響範囲が中程度

**Cons:**
- ❌ `ExpandCommand()`を分割する必要がある
- ⚠️ bootstrapの役割が縮小
- ⚠️ 実装変更の影響範囲が中程度

---

### Option 2: すべての展開をprocessConfigで実施（採用案）

**アーキテクチャ:**
```
[config.Loader]
  └─ processConfig()
      ├─ ExpandGlobalEnv()
      ├─ ExpandGroupEnv()
      ├─ ExpandGroupVerifyFiles()
      └─ ExpandCommand()               ← Command.Env/Cmd/Args全展開

[bootstrap.LoadAndPrepareConfig]
  └─ config.Loader.LoadConfig()       ← すべて展開済み
      (展開処理なし)
```

**責務分離:**
- **config.Loader**: TOMLパース + すべての変数展開
- **bootstrap**: ファイル検証 + 設定ロード

**Pros:**
- ✅ すべての展開がconfig.Loaderで完結
- ✅ 重複展開が完全に解消
- ✅ autoEnvが1回のみ生成
- ✅ 最もシンプルなアーキテクチャ
- ✅ テストが最もシンプル（loaderテストですべて検証可能）
- ✅ 展開処理が1か所に集約（保守性向上）

**Cons:**
- ❌ bootstrapの役割が「検証+ロードのみ」になる
- ❌ 既存のbootstrap側のテストが変更される
- ⚠️ 実装変更の影響範囲が大きい

---

### Option 3: 現状維持（Option 0）

**Pros:**
- ✅ 既存コードへの影響なし

**Cons:**
- ❌ すべての問題点が未解決のまま
- ❌ 技術的負債の累積

## 決定

**Option 2を採用する**

### 採用理由

1. **アーキテクチャの明確性**
   - 「設定の読み込みと展開」をconfig.Loaderに集約
   - 「ファイル検証と初期化」をbootstrapに集約
   - 単一責任の原則（SRP）に準拠

2. **保守性の向上**
   - 展開処理が1か所に集約され、理解しやすい
   - 重複コードが完全に解消
   - 将来的な変更の影響範囲が明確

3. **テスト容易性**
   - config.Loaderのテストですべての展開処理を検証可能
   - bootstrapのテストは検証とロードのみに集中
   - テストの責務が明確

4. **パフォーマンス**
   - 重複展開が解消され、処理が効率化
   - autoEnvが1回のみ生成

5. **一貫性**
   - Global.Env/Group.Env/Command.Env/Cmd/Argsすべてが同じ場所で展開
   - 変数展開のロジックが統一

### トレードオフ

以下のトレードオフを受け入れる：

1. **bootstrap役割の縮小**
   - bootstrap.LoadAndPrepareConfigが「Prepare」をしなくなる
   - → **対策**: 関数名を適切に変更するか、bootstrapの役割を明確化

2. **実装変更の影響範囲**
   - bootstrap側のコードとテストの変更が必要
   - → **対策**: 段階的な実装とテストで安全に移行

3. **既存テストの更新**
   - bootstrap側のテストが変更される
   - → **対策**: テストケースの意図を維持しながら更新

## 影響範囲

### 変更が必要なファイル

1. **config/loader.go**
   - `processConfig()`にCommand.Env/Cmd/Args展開を追加

2. **bootstrap/config.go**
   - `LoadAndPrepareConfig()`から展開処理を削除
   - 関数名の変更を検討（`LoadConfig`または`LoadVerifiedConfig`）

3. **テストファイル**
   - `config/loader_test.go`: Command.ExpandedEnv/ExpandedCmd/ExpandedArgsの検証を追加
   - `bootstrap/config_test.go`（存在する場合）: 展開処理のテストを削除

### 影響を受けるコンポーネント

- ✅ **config.Loader**: 展開処理の追加（主な変更）
- ✅ **bootstrap**: 展開処理の削除（主な変更）
- ⚠️ **cmd/runner/main.go**: bootstrap呼び出し部分（関数名変更の可能性）
- ⚠️ **既存テスト**: bootstrap側のテスト更新

### 影響を受けないコンポーネント

- ✅ **config/expansion.go**: 展開ロジック自体は変更なし
- ✅ **environment/**: 環境変数処理は変更なし
- ✅ **runner.Runner**: 実行ロジックは変更なし
- ✅ **公開API**: 外部から見た動作は変更なし

## 実装戦略

### Phase 1: 準備
1. 現在の動作を保証する統合テストを追加
2. 影響範囲の詳細調査
3. 実装計画書の作成

### Phase 2: config.Loader側の実装
1. `processConfig()`にCommand.Env/Cmd/Args展開を追加
2. テストを追加して動作確認
3. この時点では重複展開が存在（意図的）

### Phase 3: bootstrap側の変更
1. bootstrap側の展開処理を削除
2. テストを更新
3. 統合テストで動作確認

### Phase 4: クリーンアップ
1. 関数名の変更（必要に応じて）
2. コメントの更新
3. ドキュメントの更新

## 検証方法

### 機能テスト
- すべての既存テストがPASS
- 新規追加テストがPASS
- サンプルTOMLファイルがすべて正常動作

### パフォーマンステスト
- 展開処理が1回のみ実行されることを確認
- 設定ロード時間が改善されることを確認

### リグレッションテスト
- 既存のすべてのサンプル設定ファイルで動作確認
- 実際のユースケースでの動作確認

## 参考情報

- Task 0031: Global・Groupレベル環境変数設定機能
- Phase 4実装時の調査結果
- `/tmp/architecture_comparison.md`: 詳細な比較分析

## メモ

- この決定は、Task 0031の完了後、別途サブプロジェクトとして実施する
- 実装計画書は`07_refactoring_implementation_plan.md`として別途作成
