# 移行チェックリスト: コマンドプロファイル定義の新形式への移行

## 概要

本ドキュメントは、既存の`commandGroupDefinitions`を新形式に移行するためのチェックリストを提供する。

## 移行ステータス概要 (2024-10-08更新)

**✅ 移行完了: コア実装とテストはすべて完了**

- **実装状況**: すべてのコマンドが新形式 `CommandRiskProfileNew` + `ProfileBuilder` で定義済み
- **テスト状況**: マイグレーション検証テスト5件すべてがパス (総計24サブテスト)
- **品質基準**: `make lint` (0 issues) および `make test` (すべてパス) を満たす
- **残作業**: ドキュメント最終確認・更新

### 実装済み機能

1. **新型定義 (CommandRiskProfileNew)**: 5つのリスク要因を明示的に分離
   - PrivilegeRisk, NetworkRisk, DestructionRisk, DataExfilRisk, SystemModRisk
2. **ビルダーパターン (ProfileBuilder)**: 流暢なAPIでプロファイル作成
3. **バリデーション**: ビルド時にプロファイル整合性を検証
4. **後方互換性**: 既存の `CommandRiskProfile` への変換機能

### 移行済みコマンド (全23コマンド)

- 権限昇格: sudo, su, doas
- システム変更: systemctl, service
- 破壊的操作: rm, dd
- AIサービス: claude, gemini, chatgpt, gpt, openai, anthropic
- ネットワーク(常時): curl, wget, nc, netcat, telnet, ssh, scp, aws
- ネットワーク(条件付): git, rsync

### 実装ファイル

| ファイル | 説明 |
|---------|------|
| `internal/runner/security/command_risk_profile.go` | CommandRiskProfileNew 構造体定義 |
| `internal/runner/security/risk_factor.go` | RiskFactor 型定義 |
| `internal/runner/security/profile_builder.go` | ProfileBuilder (流暢API) |
| `internal/runner/security/command_profile_def.go` | CommandProfileDef (プロファイル定義) |
| `internal/runner/security/command_analysis.go` | commandProfileDefinitions (全コマンド定義: L93-145) |
| `internal/runner/security/command_analysis_test.go` | マイグレーションテスト (L2092-2277) |

## 移行ステータス

### Phase 2.2.1: 権限昇格コマンド

- [x] sudo, su, doas の移行
- [x] リスクレベル一致確認テスト
- [x] IsPrivilege フラグ確認テスト

**期待される動作:**
- BaseRiskLevel: Critical
- IsPrivilege: true
- NetworkType: None

**実装済み (command_analysis.go:95-97):**
```go
NewProfile("sudo", "su", "doas").
    PrivilegeRisk(runnertypes.RiskLevelCritical, "Allows execution with elevated privileges, can compromise entire system").
    Build(),
```

---

### Phase 2.2.2: ネットワークコマンド (常時)

- [x] curl, wget の移行
- [x] nc, netcat, telnet の移行
- [x] ssh, scp の移行
- [x] aws の移行
- [x] リスクレベル一致確認テスト
- [x] NetworkType=Always 確認テスト

**期待される動作:**
- BaseRiskLevel: Medium
- NetworkType: Always

**実装済み (command_analysis.go:120-135):**
```go
NewProfile("curl", "wget").
    NetworkRisk(runnertypes.RiskLevelMedium, "Always performs network operations").
    AlwaysNetwork().
    Build(),
NewProfile("nc", "netcat", "telnet").
    NetworkRisk(runnertypes.RiskLevelMedium, "Establishes network connections").
    AlwaysNetwork().
    Build(),
NewProfile("ssh", "scp").
    NetworkRisk(runnertypes.RiskLevelMedium, "Remote operations via network").
    AlwaysNetwork().
    Build(),
NewProfile("aws").
    NetworkRisk(runnertypes.RiskLevelMedium, "Cloud service operations via network").
    AlwaysNetwork().
    Build(),
```

---

### Phase 2.2.3: ネットワークコマンド (条件付き)

- [x] git の移行
  - [x] NetworkSubcommands 設定確認
  - [x] サブコマンド検出テスト
- [x] rsync の移行
  - [x] 引数ベース検出テスト
- [x] リスクレベル一致確認テスト

**期待される動作 (git):**
- BaseRiskLevel: Medium (旧: Low → 変更あり)
- NetworkType: Conditional
- NetworkSubcommands: ["clone", "fetch", "pull", "push", "remote"]

**期待される動作 (rsync):**
- BaseRiskLevel: Medium (旧: Low → 変更あり)
- NetworkType: Conditional
- NetworkSubcommands: [] (空 - 引数ベース判定)

**実装済み (command_analysis.go:138-145):**
```go
NewProfile("git").
    NetworkRisk(runnertypes.RiskLevelMedium, "Network operations for clone/fetch/pull/push/remote").
    ConditionalNetwork("clone", "fetch", "pull", "push", "remote").
    Build(),
NewProfile("rsync").
    NetworkRisk(runnertypes.RiskLevelMedium, "Network operations when using remote sources/destinations").
    ConditionalNetwork().
    Build(),
```

---

### Phase 2.2.4: 破壊的操作コマンド

- [x] rm の移行
  - [x] BaseRiskLevel: High
- [x] dd の移行
  - [x] BaseRiskLevel: Critical (旧: High → 変更あり)
- [x] リスクレベル一致確認テスト

**注意:** `rm`と`dd`は別々に定義する（リスクレベルが異なるため）

**実装済み (command_analysis.go:105-110):**
```go
NewProfile("rm").
    DestructionRisk(runnertypes.RiskLevelHigh, "Can delete files and directories").
    Build(),
NewProfile("dd").
    DestructionRisk(runnertypes.RiskLevelCritical, "Can overwrite entire disks, potential data loss").
    Build(),
```

---

### Phase 2.2.5: AIサービスコマンド

- [x] claude, gemini, chatgpt, gpt, openai, anthropic の移行
- [x] 複数リスク要因 (NetworkRisk + DataExfilRisk) の設定確認
- [x] リスクレベル一致確認テスト
- [x] リスク理由の複数項目確認テスト

**期待される動作:**
- BaseRiskLevel: High
- NetworkRisk: High
- DataExfilRisk: High
- NetworkType: Always
- GetRiskReasons(): ["Always communicates with external AI API", "May send sensitive data to external service"]

**実装済み (command_analysis.go:113-118):**
```go
NewProfile("claude", "gemini", "chatgpt", "gpt", "openai", "anthropic").
    NetworkRisk(runnertypes.RiskLevelHigh, "Always communicates with external AI API").
    DataExfilRisk(runnertypes.RiskLevelHigh, "May send sensitive data to external service").
    AlwaysNetwork().
    Build(),
```

---

### Phase 2.2.6: システム変更コマンド

- [x] systemctl, service の移行
- [x] リスクレベル一致確認テスト

**期待される動作:**
- BaseRiskLevel: High
- SystemModRisk: High

**実装済み (command_analysis.go:100-102):**
```go
NewProfile("systemctl", "service").
    SystemModRisk(runnertypes.RiskLevelHigh, "Can modify system services and configuration").
    Build(),
```

---

### Phase 2.2.7: 残りのコマンド

現在の`commandGroupDefinitions`には上記以外のコマンドは含まれていないため、このフェーズはスキップ可能。

将来的に追加されたコマンドがある場合、ここで対応する。

---

## リスクレベル変更の確認

以下のコマンドで意図的にリスクレベルを変更する：

| コマンド | 旧リスクレベル | 新リスクレベル | 確認項目 |
|---------|-------------|-------------|---------|
| dd      | High        | Critical    | - [x] テストで Critical を確認 (TestMigration_RiskLevelConsistency) |
| git     | Low         | Medium      | - [x] テストで Medium を確認 (TestMigration_RiskLevelConsistency)<br>- [x] 条件付きネットワーク動作を確認 (TestMigration_NetworkTypeConsistency) |
| rsync   | Low         | Medium      | - [x] テストで Medium を確認 (TestMigration_RiskLevelConsistency)<br>- [x] 条件付きネットワーク動作を確認 (TestMigration_NetworkTypeConsistency) |

**注意:** `git`と`rsync`のリスクレベル変更は、ネットワーク操作時のリスクを明示的に反映するため。非ネットワーク操作時の動作には影響しない（実行時のリスク評価は別途行われる）。

---

## テスト項目

### 2.3.1 全プロファイルのバリデーション

- [x] `TestAllProfilesAreValid` 実装
  - すべてのプロファイルが `Validate()` を通過することを確認
  - **実装済み** (command_analysis_test.go:2092-2099)

- [x] `TestAllProfilesHaveReasons` 実装
  - リスクレベルが`Unknown`より高いプロファイルは理由を持つことを確認
  - **実装済み** (command_analysis_test.go:2101-2112)

### 2.3.2 リスクレベル一致確認

- [x] `TestMigration_RiskLevelConsistency` 実装
  - **実装済み** (command_analysis_test.go:2114-2165)
  - 以下のコマンドでリスクレベルが一致することを確認：
    - sudo, su, doas: Critical ✓
    - curl, wget, nc, netcat, telnet, ssh, scp, aws: Medium ✓
    - systemctl, service: High ✓
    - rm: High ✓
    - claude, gemini, chatgpt, gpt, openai, anthropic: High ✓
  - 以下のコマンドでリスクレベルが変更されることを確認：
    - dd: High → Critical ✓
    - git: Low → Medium ✓
    - rsync: Low → Medium ✓

### 2.3.3 NetworkType一致確認

- [x] `TestMigration_NetworkTypeConsistency` 実装
  - **実装済み** (command_analysis_test.go:2168-2200)
  - NetworkTypeAlways: curl, wget, nc, netcat, telnet, ssh, scp, aws, claude, gemini, chatgpt, gpt, openai, anthropic ✓
  - NetworkTypeConditional: git, rsync ✓
  - NetworkTypeNone: sudo, su, doas, systemctl, service, rm, dd ✓

### 2.3.4 NetworkSubcommands確認

- [x] `TestMigration_NetworkSubcommandsConsistency` 実装
  - **実装済み** (command_analysis_test.go:2202-2227)
  - git: ["clone", "fetch", "pull", "push", "remote"] ✓
  - rsync: [] (空) ✓
  - その他: NetworkTypeConditional以外は空 ✓

### 2.3.5 IsPrivilege確認

- [x] `TestMigration_IsPrivilegeConsistency` 実装
  - **実装済み** (command_analysis_test.go:2229-2249)
  - IsPrivilege=true: sudo, su, doas ✓
  - IsPrivilege=false: その他すべて ✓

### 2.3.6 複数リスク要因確認

- [x] `TestMigration_MultipleRiskFactors` 実装
  - **実装済み** (command_analysis_test.go:2251-2277)
  - AIサービスコマンド: NetworkRisk + DataExfilRisk ✓
  - GetRiskReasons()で複数の理由が返されることを確認 ✓

---

## 移行完了基準

以下のすべてを満たすこと：

- [x] すべてのコマンドが新形式に移行完了
  - **完了**: すべてのコマンドが `commandProfileDefinitions` に定義済み (command_analysis.go:93-145)
- [x] すべてのテストがパス
  - **完了**: マイグレーションテスト5件すべてがパス (2024-10-08確認)
- [x] リスクレベル一致確認完了（意図的な変更を除く）
  - **完了**: TestMigration_RiskLevelConsistency パス
- [x] NetworkType一致確認完了
  - **完了**: TestMigration_NetworkTypeConsistency パス
- [x] NetworkSubcommands一致確認完了
  - **完了**: TestMigration_NetworkSubcommandsConsistency パス
- [x] IsPrivilege一致確認完了
  - **完了**: TestMigration_IsPrivilegeConsistency パス
- [x] 複数リスク要因の動作確認完了
  - **完了**: TestMigration_MultipleRiskFactors パス
- [x] `make lint` でエラーなし
  - **完了**: 0 issues (2024-10-08確認)
- [x] `make test` でエラーなし
  - **完了**: すべてのテストがパス (2024-10-08確認)
- [ ] ドキュメント更新完了
  - 進行中: このドキュメント含め、関連ドキュメントの更新が必要

---

## トラブルシューティング

### 問題: バリデーションエラーが発生する

**原因:** NetworkTypeAlwaysでNetworkRisk < Mediumなど、バリデーションルール違反

**対処:**
1. エラーメッセージを確認し、違反しているルールを特定
2. 移行マッピング表を参照し、正しいリスクレベルを設定
3. 必要に応じて、バリデーションルールの妥当性を再確認

### 問題: リスクレベルが一致しない

**原因:** 意図しないリスクレベル変更、または複数リスク要因の最大値計算ミス

**対処:**
1. 移行マッピング表で意図的な変更かを確認
2. `BaseRiskLevel()`メソッドが全リスク要因の最大値を返すことを確認
3. 必要に応じて、個別のリスク要因レベルを調整

### 問題: NetworkSubcommandsが機能しない

**原因:** NetworkTypeがConditionalでない、または空のリスト

**対処:**
1. `NetworkType`が`NetworkTypeConditional`であることを確認
2. `ConditionalNetwork()`メソッドにサブコマンドを渡していることを確認
3. 実行時の引数パース処理を確認（`findFirstSubcommand`関数）

---

## 次のステップ

### 短期 (すぐに実施可能)

1. **ドキュメントレビュー**
   - [ ] `docs/tasks/0029_risk_profile_redesign/` 配下のすべてのドキュメントを確認
   - [ ] 実装状況との齟齬がないか検証
   - [ ] 必要に応じて更新

2. **リスクレベル変更の再評価**
   - [ ] dd: High → Critical の妥当性を確認
   - [ ] git, rsync: Low → Medium の影響範囲を確認
   - [ ] 必要に応じてユーザーへの通知方法を検討

### 中期 (Phase 3への準備)

1. **後方互換性の削除準備**
   - 旧形式 `CommandRiskProfile` の削除タイミングを検討
   - `convertNewProfileToOld()` の廃止計画を策定

2. **新規コマンド追加の手順書作成**
   - `ProfileBuilder` を使った新規コマンド追加方法を文書化
   - 典型的なパターン (権限昇格、ネットワーク、破壊的操作等) のテンプレート作成

3. **統合テストの拡充**
   - 実際のコマンド実行でのリスク評価動作を確認
   - ネットワーク検出 (サブコマンドベース・引数ベース) の動作検証

### 長期 (将来の改善)

1. **動的リスク評価の強化**
   - 実行時引数に基づくリスクレベル動的調整
   - コンテキスト情報 (実行環境、ユーザー権限等) の考慮

2. **リスクプロファイル外部化**
   - ユーザー定義プロファイルのサポート
   - TOML/YAML での設定ファイル対応

---

## 参照ドキュメント

- `00_problem_statement.md`: 問題定義と現状の課題
- `01_goals_and_non_goals.md`: プロジェクトの目標と非目標
- `02_proposed_solution.md`: 提案ソリューションの詳細
- `03_design_decisions.md`: 設計判断の記録
- `04_implementation_plan.md`: 実装計画 (TDDアプローチ)
- `05_migration_mapping.md`: 旧形式から新形式へのマッピング表
