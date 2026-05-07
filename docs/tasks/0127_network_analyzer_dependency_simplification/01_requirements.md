# 要件定義: NetworkAnalyzer 周辺の依存配線簡素化

## 背景

`internal/runner/base/security/NetworkAnalyzer` は以下の分析用インターフェースを利用している。

- `fileanalysis.NetworkSymbolStore`
- `fileanalysis.SyscallAnalysisStore`
- `fileanalysis.DynLibDepsStore`
- `dynamicanalysis.Store`
- `fileanalysis.ShebangInterpreterStore`

これらの実体は `internal/verification/Manager` の生成時に初期化されるが、
実際の利用地点である `NetworkAnalyzer` までの受け渡し経路が長い。
現在は以下のような配線になっている。

```text
verification.Manager
  -> runner.createNormalResourceManager
  -> resource.Config
  -> resource.newNormalManager
  -> risk.NewStandardEvaluator
  -> security.NewNetworkAnalyzer
```

この経路では、同じ依存集合を複数レイヤーで個別フィールドとして再定義し、
再梱包している。
その結果、以下の問題が生じている。

- 依存追加・削除のたびに複数シグネチャの変更が必要になる
- `resource` レイヤーが本来不要な分析ストアの知識を持っている
- `runner` 側で `PathResolver` への複数の型アサーションが必要になる
- `NetworkAnalyzer` の構成責務が見えにくく、テスト差し替えポイントも散らばる

## 問題

現在の配線は「分析依存の所有者」と「分析依存を利用するレイヤー」の境界が曖昧である。

- 分析ストアの生成責務は `verification` にある
- リスク判定責務は `risk` / `security` にある
- しかし中間の `resource` が分析ストアを直接知っている

この構造は、レイヤー境界を越えて詳細が漏れている状態であり、
責務分離と変更容易性の両方を悪化させている。

## 目標

分析用ストア群の受け渡しを簡素化し、以下を実現する。

- `NetworkAnalyzer` が必要とする依存群を 1 つの論理単位として扱える
- `resource` レイヤーから分析ストアの詳細知識を取り除ける
- `runner` の composition root で依存を一度だけ組み立てれば済む
- 今後ストアが追加・削除されても変更箇所が局所化される
- 既存のネットワーク判定挙動、フェイルクローズ挙動、dry-run 挙動を維持する

## スコープ

### 対象

- `NetworkAnalyzer` の依存注入方式
- `risk.StandardEvaluator` への依存の渡し方
- `resource.Config` における分析依存の扱い
- `runner.createNormalResourceManager` における依存組み立て
- `verification.Manager` から分析依存を取得する API

### 非対象

- `NetworkAnalyzer` のネットワーク検知ロジック自体の変更
- キャッシュの保存形式、ハッシュ仕様、スキーマ変更
- `record` 側の分析生成ロジックの機能変更
- dry-run 以外の実行モード追加

## 機能要件

#### F-127-1: 分析依存の集約

`NetworkAnalyzer` が利用する複数の分析依存は、呼び出し側から 1 つの論理単位として渡せること。
論理単位は bundle / provider / factory などの形を取り得るが、呼び出しシグネチャ上で個別の 5 引数を並べないこと。

**Acceptance Criteria**:
1. `security.NewNetworkAnalyzer` の公開シグネチャが分析依存 5 個を個別引数で受け取らない。
2. `risk.NewStandardEvaluator` の公開シグネチャが分析依存 5 個を個別引数で受け取らない。
3. 分析依存の欠如を表す `nil` の意味は維持され、個別分析の無効化条件が変わらない。
4. 既存の `NetworkAnalyzer` の判定ロジックは、依存の受け取り方以外で意味変更しない。

#### F-127-2: resource レイヤーの責務縮小

`resource` レイヤーは分析ストア群の詳細を直接保持せず、リスク評価に必要な上位抽象だけを扱うこと。

**Acceptance Criteria**:
1. `resource.Config` が `NetworkSymbolStore`、`SyscallAnalysisStore`、`DynLibDepsStore`、`dynamicanalysis.Store`、`ShebangInterpreterStore` を個別フィールドとして保持しない。
2. `resource.newNormalManager` は分析ストア群を直接 `risk.NewStandardEvaluator` へ渡さない。
3. `NormalResourceManager` の振る舞いは変更されず、コマンド実行前のリスク評価が引き続き行われる。

#### F-127-3: composition root での組み立て一元化

分析依存の具体生成場所は維持しつつ、`runner` 側での配線は 1 回の組み立てで完結すること。

**Acceptance Criteria**:
1. `runner.createNormalResourceManager` において、分析依存の個別 getter 呼び出しや個別型アサーションが 5 系統並ばない。
2. `verification.Manager` 由来の分析依存取得 API は、呼び出し側に個別ストアの列挙を強制しない。
3. 分析依存の生成責務は引き続き `verification` 側に残り、`security` や `risk` に具体生成処理を移さない。

#### F-127-4: 既存挙動の互換維持

このリファクタリングは配線と依存境界の整理を目的とし、外部仕様を変えないこと。

**Acceptance Criteria**:
1. 既存のネットワーク判定、high risk 判定、shebang 追跡の振る舞いが変わらない。
2. 分析ストアが利用不能な場合の fail-open / fail-closed ポリシーは既存通り維持される。
3. dry-run モードと通常実行モードの初期化フローに後方互換性がある。
4. 既存テストで検証されているネットワーク判定シナリオが引き続き成立する。

#### F-127-5: テスト容易性の向上

新しい依存注入構造は、`NetworkAnalyzer` と `StandardEvaluator` のテストダブル差し替えを現状より悪化させないこと。

**Acceptance Criteria**:
1. 単体テストから、集約された依存単位または `risk.Evaluator` を差し替え可能である。
2. 新構造により、テストのためだけに `resource.Config` へ 5 種類のストアを個別設定する必要がない。
3. モック化の責務境界が文書上で明示されている。

## 制約条件

- 依存の具体生成は composition root もしくは `verification` に残す
- `security` パッケージに hash directory や store directory の具体構築責務を持ち込まない
- 既存の公開 API 変更は、変更理由と移行先が設計書で説明されること
- 実装は YAGNI と DRY を優先し、余分な汎化を避ける

## 受け入れ基準

| # | 基準 |
|---|------|
| AC-1 | `NetworkAnalyzer` と `StandardEvaluator` のコンストラクタが分析依存 5 個を個別引数で受け取らない |
| AC-2 | `resource.Config` から分析ストア群の個別フィールドが除去される、または同等に非公開な 1 単位へ置き換わる |
| AC-3 | `runner.createNormalResourceManager` での分析依存配線が単一の組み立て操作に簡約される |
| AC-4 | 分析依存の具体生成責務は `verification` に残り、`security` は具体ストア構築を行わない |
| AC-5 | 既存のネットワーク判定系テストと runner 初期化系テストが通過する |
| AC-6 | `make test`、`make lint`、`make build` が成功する |

## 設計方針

### 基本方針

- **依存を束ねて渡す**: 分析用ストア群を 1 つの依存単位として扱う
- **上位抽象を渡す**: `resource` は分析ストアではなく `risk.Evaluator` または同等の高位依存を扱う
- **生成と利用を分離する**: 具体生成は `verification`、利用は `risk` / `security` に留める
- **境界で完結させる**: 型アサーションや変換は composition root で閉じる
- **挙動不変のリファクタリング**: ロジック変更ではなく配線変更として実施する

### 推奨アプローチ

段階的には以下を推奨する。

1. 分析依存 bundle を導入し、5 個のストア引数を 1 単位に集約する
2. `resource.Config` は bundle ではなく `risk.Evaluator` を直接受け取る形へ整理する
3. `verification.Manager` は個別 getter 群ではなく、集約依存を提供する

この順序により、公開 API の変更範囲を制御しつつ、最終的に `resource` から分析詳細を除去できる。
