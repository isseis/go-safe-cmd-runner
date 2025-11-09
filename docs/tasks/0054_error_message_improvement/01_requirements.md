# エラーメッセージ改善 - 要件定義書

## 1. 概要

### 1.1 背景

現在、runner 実行時に発生するエラーメッセージは、複数レベルのエラーチェーンが連鎖した形で表示されるため、ユーザーが問題の本質を即座に理解することが困難である。

**現状のエラーメッセージの例**:
```
[ERROR] Command failed error=output path validation failed: security validation failed: directory validation failed: directory security validation failed for /tmp/scr-mattermost_backup-4288425963/data: invalid directory permissions: directory /tmp/scr-mattermost_backup-4288425963/data has group write permissions (0775) but group membership cannot be verified command=dump_db
```

この例では以下の問題が見られる:
- エラーが5段階に連鎖している
- "validation failed" が4回繰り返される
- 根本原因（グループメンバーシップを確認できない）が最後に埋もれている
- 技術的な詳細が多すぎてユーザーが何を修正すべきか分かりにくい

### 1.2 目的

エラーメッセージを改善し、以下を実現する:
- ユーザーが問題の本質を即座に理解できる
- 開発者/管理者がデバッグに必要な詳細情報を得られる
- ログの可読性が向上する

### 1.3 スコープ

- **対象**: runner 実行時の全エラーメッセージ
- **実装範囲**: エラーメッセージのフォーマット改善、ログレベル分離
- **注**: エラー型やエラーハンドリングのロジックそのものは変更しない（既存の `errors.Is()` による型チェックとの互換性を保持）

## 2. 現状の問題分析

### 2.1 問題点の詳細

#### 2.1.1 エラーチェーンの冗長性

複数レベルのラッピングにより、同じような表現が繰り返される。

**例**:
- "output path validation failed"
- "security validation failed"
- "directory validation failed"
- "directory security validation failed"

これらは全て本質的に同じ情報を異なる抽象度で表現しているだけである。

#### 2.1.2 根本原因の埋没

ユーザーが最も知りたい情報（何が問題で、どう修正すべきか）が長いメッセージの最後に埋もれている。

#### 2.1.3 ログレベルの未分化

技術的な詳細情報と、ユーザーが知るべき情報が同じレベルで表示される。

### 2.2 影響範囲

- ユーザーエクスペリエンスの低下
- トラブルシューティング時間の増加
- サポートコストの増加

## 3. 改善案の比較

### 3.1 案1: エラーメッセージの簡潔化

**概要**: エラーチェーンの中間層を削減し、ユーザーに必要な情報のみを表示

**改善後のメッセージ例**:
```
[ERROR] Command failed error=invalid directory permissions: /tmp/scr-mattermost_backup-4288425963/data has group write permissions (0775) but group membership cannot be verified command=dump_db
```

**長所**:
- 実装が単純（既存のエラーメッセージ文字列を修正するだけ）
- 既存のエラー型チェック（`errors.Is()`）への影響なし
- 即座に理解できる簡潔なメッセージ

**短所**:
- 詳細な技術情報が失われる
- トラブルシューティング時に追加情報が必要になる場合がある

### 3.2 案2: 構造化されたエラー表示

**概要**: エラーを階層的に表示して理解しやすくする

**改善後のメッセージ例**:
```
[ERROR] Command failed command=dump_db
  └─ Output path validation error
     └─ Directory permission error: /tmp/scr-mattermost_backup-4288425963/data
        - Permissions: 0775 (group writable)
        - Problem: Cannot verify group membership
        - Action: Remove group write permission or ensure group membership can be verified
```

**長所**:
- 視覚的に分かりやすい
- エラーの階層構造が明確
- アクションアイテムを明示できる

**短所**:
- 複数行にわたるため、ログ解析が複雑になる
- 実装が複雑（新しいエラーフォーマッターが必要）
- 既存のログ処理ツールとの互換性問題

### 3.3 案3: エラーレベルの分離

**概要**: 重要度に応じてエラー情報を分離

**改善後のメッセージ例**:
```
[ERROR] Command failed: Permission denied for output directory command=dump_db path=/tmp/scr-mattermost_backup-4288425963/data
[DEBUG] Permission details: directory has group write (0775) but group membership verification unavailable
```

**長所**:
- ユーザーは ERROR レベルだけを見れば十分
- 詳細情報は DEBUG レベルで利用可能
- 既存のログレベルフィルタリングと親和性が高い

**短所**:
- 情報が分散するため、全体像を把握しにくい場合がある
- DEBUG ログが無効の場合、詳細情報が失われる

### 3.4 案4: ユーザー向けメッセージとログの分離

**概要**: ユーザーに表示するメッセージと、ログに記録する詳細を分ける

**ユーザー表示**:
```
[ERROR] Output directory has insecure permissions
  Directory: /tmp/scr-mattermost_backup-4288425963/data
  Permissions: 0775
  Required: Remove group write permission (chmod 755) or verify group membership
  Command: dump_db
```

**ログ記録（JSON形式など）**:
```json
{
  "level": "error",
  "command": "dump_db",
  "error_type": "directory_permission",
  "path": "/tmp/scr-mattermost_backup-4288425963/data",
  "permissions": "0775",
  "issue": "group_write_unverified",
  "error_chain": ["output_path_validation", "security_validation", ...]
}
```

**長所**:
- ユーザー体験が最適化される
- ログ解析が容易（構造化データ）
- 詳細情報は保持される

**短所**:
- 実装コストが高い（二重の出力システムが必要）
- 設定ファイルでフォーマットを切り替える仕組みが必要

## 4. 推奨案の選定

### 4.1 選定結果

**推奨案: 案1（簡潔化）+ 案3（レベル分離）の組み合わせ**

### 4.2 選定理由

#### 4.2.1 実装コストと効果のバランス

- 案1は最小限の変更で大きな改善が得られる
- 案3は既存のロギングフラストラクチャを活用できる
- 案2や案4は実装コストが高い割に、得られる追加メリットが限定的

#### 4.2.2 既存システムとの互換性

- エラー型（`errors.Is()` によるチェック）は変更しないため、既存のエラーハンドリングロジックへの影響なし
- 既存のログレベルフィルタリング機能をそのまま活用できる

#### 4.2.3 ユーザー体験の向上

- ERROR レベル: 簡潔で理解しやすいメッセージ
- DEBUG レベル: トラブルシューティングに必要な技術的詳細
- ユーザーは必要に応じてログレベルを調整可能

#### 4.2.4 段階的な改善が可能

- まず簡潔化（案1）を実装
- 次にレベル分離（案3）を追加
- 将来的に案2や案4への拡張も可能

### 4.3 案2と案4を採用しなかった理由

#### 案2（構造化表示）について

- 複数行のエラーメッセージはログ解析ツールとの互換性に問題がある
- 多くのログ管理システムは1行単位でログを処理する
- 実装コストが高い（専用のフォーマッターが必要）

#### 案4（ユーザー/ログ分離）について

- 二重の出力システムが必要となり、実装・保守コストが高い
- 現時点では過剰設計と判断
- 将来的なニーズが明確になってから検討すべき

## 5. 機能要件

### 5.1 エラーメッセージの簡潔化

#### 5.1.1 冗長な中間層の削除

エラーチェーンの中間層から重複する表現を削除する。

**修正前**:
```
output path validation failed: security validation failed: directory validation failed: directory security validation failed for /tmp/data: invalid directory permissions: ...
```

**修正後**:
```
invalid directory permissions: /tmp/data has group write permissions (0775) but group membership cannot be verified
```

#### 5.1.2 根本原因の優先表示

最も重要な情報（根本原因と具体的な問題）をメッセージの先頭に配置する。

### 5.2 ログレベルの分離

#### 5.2.1 ERROR レベル

ユーザーが問題を理解し、対処するために必要な最小限の情報を含む。

**含むべき情報**:
- 問題の種類（例: "invalid directory permissions"）
- 影響を受けるリソース（例: パス）
- 問題の具体的内容（例: "has group write permissions (0775)"）
- 根本原因（例: "but group membership cannot be verified"）

#### 5.2.2 DEBUG レベル

技術的な詳細情報を含む。

**含むべき情報**:
- エラーチェーンの詳細
- 内部的な検証ステップ
- システムコールの結果
- その他のトラブルシューティング情報

### 5.3 一貫性の確保

#### 5.3.1 メッセージフォーマットの統一

全てのエラーメッセージで一貫したフォーマットを使用する。

**推奨フォーマット**:
```
[ERROR] <問題の種類>: <具体的な問題内容> <追加コンテキスト>
```

#### 5.3.2 用語の統一

同じ概念に対して常に同じ用語を使用する。

## 6. 非機能要件

### 6.1 互換性

- 既存のエラー型（`errors.Is()` による型チェック）を維持
- 既存のテストコードへの影響を最小化
- ログ解析スクリプトとの互換性を考慮

### 6.2 保守性

- エラーメッセージの変更が容易
- 新しいエラー種別の追加が容易
- テストケースの更新が最小限

### 6.3 拡張性

- 将来的により高度なエラーフォーマット（案2、案4）への移行が可能
- 国際化（i18n）への対応が可能

## 7. 制約事項

### 7.1 技術的制約

- Go 1.23.10 以上
- 既存のエラー型システムを変更しない
- 既存の `errors.Is()` による型チェックとの互換性を保持

### 7.2 互換性制約

- エラーメッセージ文字列に依存したテストコードは更新が必要
- ログ解析スクリプトがエラーメッセージ文字列に依存している場合は更新が必要

## 8. 成功基準

### 8.1 ユーザー体験の向上

- [ ] エラーメッセージを見たユーザーが問題の原因を5秒以内に理解できる
- [ ] エラーメッセージの長さが現状の50%以下になる
- [ ] "validation failed" などの冗長な表現が削除される

### 8.2 開発者体験の向上

- [ ] DEBUG レベルで必要な技術的詳細が利用可能
- [ ] トラブルシューティング時間が短縮される

### 8.3 コード品質

- [ ] 全ての既存テストが（必要な更新後に）パスする
- [ ] 新しいテストケースでエラーメッセージの品質を検証

## 9. 想定される影響範囲

### 9.1 修正が必要なコンポーネント

- `internal/runner/security/file_validation.go`: ディレクトリ検証エラーメッセージ
- `internal/runner/group_executor.go`: コマンド実行エラーメッセージ
- その他のエラー生成箇所

### 9.2 修正が必要なテストコード

- エラーメッセージ文字列を検証しているテストケース
  - 推奨: `errors.Is()` による型チェックに移行
  - 文字列チェックが必要な場合のみ、新しいメッセージに更新

### 9.3 ドキュメントの更新

- ユーザーガイド: エラーメッセージの読み方
- トラブルシューティングガイド: 一般的なエラーと対処法

## 10. 実装の優先順位

### 10.1 フェーズ1: 重要度の高いエラーメッセージの簡潔化

- ディレクトリパーミッションエラー（本タスクで発見されたもの）
- ファイルパーミッションエラー
- コマンド実行エラー

### 10.2 フェーズ2: ログレベル分離の実装

- DEBUG レベルへの詳細情報の移動
- ERROR レベルメッセージの最適化

### 10.3 フェーズ3: 全体的な一貫性の確保

- 全エラーメッセージの監査
- フォーマットと用語の統一

## 11. 参照

- タスク 0016: Logging System Redesign（ログシステムの基盤）
- タスク 0019: Security Validation Unification（セキュリティ検証の統合）
- タスク 0022: Hash Directory Security Enhancement（ディレクトリセキュリティ）
