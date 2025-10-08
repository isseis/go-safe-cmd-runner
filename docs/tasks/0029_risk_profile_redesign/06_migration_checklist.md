# 移行チェックリスト: コマンドプロファイル定義の新形式への移行

## 概要

本ドキュメントは、既存の`commandGroupDefinitions`を新形式に移行するためのチェックリストを提供する。

## 移行ステータス

### Phase 2.2.1: 権限昇格コマンド

- [ ] sudo, su, doas の移行
- [ ] リスクレベル一致確認テスト
- [ ] IsPrivilege フラグ確認テスト

**期待される動作:**
- BaseRiskLevel: Critical
- IsPrivilege: true
- NetworkType: None

---

### Phase 2.2.2: ネットワークコマンド (常時)

- [ ] curl, wget の移行
- [ ] nc, netcat, telnet の移行
- [ ] ssh, scp の移行
- [ ] aws の移行
- [ ] リスクレベル一致確認テスト
- [ ] NetworkType=Always 確認テスト

**期待される動作:**
- BaseRiskLevel: Medium
- NetworkType: Always

---

### Phase 2.2.3: ネットワークコマンド (条件付き)

- [ ] git の移行
  - [ ] NetworkSubcommands 設定確認
  - [ ] サブコマンド検出テスト
- [ ] rsync の移行
  - [ ] 引数ベース検出テスト
- [ ] リスクレベル一致確認テスト

**期待される動作 (git):**
- BaseRiskLevel: Medium (旧: Low → 変更あり)
- NetworkType: Conditional
- NetworkSubcommands: ["clone", "fetch", "pull", "push", "remote"]

**期待される動作 (rsync):**
- BaseRiskLevel: Medium (旧: Low → 変更あり)
- NetworkType: Conditional
- NetworkSubcommands: [] (空 - 引数ベース判定)

---

### Phase 2.2.4: 破壊的操作コマンド

- [ ] rm の移行
  - [ ] BaseRiskLevel: High
- [ ] dd の移行
  - [ ] BaseRiskLevel: Critical (旧: High → 変更あり)
- [ ] リスクレベル一致確認テスト

**注意:** `rm`と`dd`は別々に定義する（リスクレベルが異なるため）

---

### Phase 2.2.5: AIサービスコマンド

- [ ] claude, gemini, chatgpt, gpt, openai, anthropic の移行
- [ ] 複数リスク要因 (NetworkRisk + DataExfilRisk) の設定確認
- [ ] リスクレベル一致確認テスト
- [ ] リスク理由の複数項目確認テスト

**期待される動作:**
- BaseRiskLevel: High
- NetworkRisk: High
- DataExfilRisk: High
- NetworkType: Always
- GetRiskReasons(): ["Always communicates with external AI API", "May send sensitive data to external service"]

---

### Phase 2.2.6: システム変更コマンド

- [ ] systemctl, service の移行
- [ ] リスクレベル一致確認テスト

**期待される動作:**
- BaseRiskLevel: High
- SystemModRisk: High

---

### Phase 2.2.7: 残りのコマンド

現在の`commandGroupDefinitions`には上記以外のコマンドは含まれていないため、このフェーズはスキップ可能。

将来的に追加されたコマンドがある場合、ここで対応する。

---

## リスクレベル変更の確認

以下のコマンドで意図的にリスクレベルを変更する：

| コマンド | 旧リスクレベル | 新リスクレベル | 確認項目 |
|---------|-------------|-------------|---------|
| dd      | High        | Critical    | - [ ] テストで Critical を確認 |
| git     | Low         | Medium      | - [ ] テストで Medium を確認<br>- [ ] 条件付きネットワーク動作を確認 |
| rsync   | Low         | Medium      | - [ ] テストで Medium を確認<br>- [ ] 条件付きネットワーク動作を確認 |

**注意:** `git`と`rsync`のリスクレベル変更は、ネットワーク操作時のリスクを明示的に反映するため。非ネットワーク操作時の動作には影響しない（実行時のリスク評価は別途行われる）。

---

## テスト項目

### 2.3.1 全プロファイルのバリデーション

- [ ] `TestAllProfilesAreValid` 実装
  - すべてのプロファイルが `Validate()` を通過することを確認

- [ ] `TestAllProfilesHaveReasons` 実装
  - リスクレベルが`Unknown`より高いプロファイルは理由を持つことを確認

### 2.3.2 リスクレベル一致確認

- [ ] `TestMigration_RiskLevelConsistency` 実装
  - 以下のコマンドでリスクレベルが一致することを確認：
    - sudo, su, doas: Critical
    - curl, wget, nc, netcat, telnet, ssh, scp, aws: Medium
    - systemctl, service: High
    - rm: High
    - claude, gemini, chatgpt, gpt, openai, anthropic: High
  - 以下のコマンドでリスクレベルが変更されることを確認：
    - dd: High → Critical
    - git: Low → Medium
    - rsync: Low → Medium

### 2.3.3 NetworkType一致確認

- [ ] `TestMigration_NetworkTypeConsistency` 実装
  - NetworkTypeAlways: curl, wget, nc, netcat, telnet, ssh, scp, aws, claude, gemini, chatgpt, gpt, openai, anthropic
  - NetworkTypeConditional: git, rsync
  - NetworkTypeNone: sudo, su, doas, systemctl, service, rm, dd

### 2.3.4 NetworkSubcommands確認

- [ ] `TestMigration_NetworkSubcommandsConsistency` 実装
  - git: ["clone", "fetch", "pull", "push", "remote"]
  - rsync: [] (空)
  - その他: NetworkTypeConditional以外は空

### 2.3.5 IsPrivilege確認

- [ ] `TestMigration_IsPrivilegeConsistency` 実装
  - IsPrivilege=true: sudo, su, doas
  - IsPrivilege=false: その他すべて

### 2.3.6 複数リスク要因確認

- [ ] `TestMigration_MultipleRiskFactors` 実装
  - AIサービスコマンド: NetworkRisk + DataExfilRisk
  - GetRiskReasons()で複数の理由が返されることを確認

---

## 移行完了基準

以下のすべてを満たすこと：

- [ ] すべてのコマンドが新形式に移行完了
- [ ] すべてのテストがパス
- [ ] リスクレベル一致確認完了（意図的な変更を除く）
- [ ] NetworkType一致確認完了
- [ ] NetworkSubcommands一致確認完了
- [ ] IsPrivilege一致確認完了
- [ ] 複数リスク要因の動作確認完了
- [ ] `make lint` でエラーなし
- [ ] `make test` でエラーなし
- [ ] ドキュメント更新完了

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
