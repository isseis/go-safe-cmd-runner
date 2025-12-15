# 変数展開アルゴリズム

このドキュメントでは、go-safe-cmd-runner における変数展開アルゴリズムの実装詳細を説明します。

## 概要

go-safe-cmd-runner は、設定ファイル内で `%{変数名}` という形式の変数参照を展開する機能を提供します。このアルゴリズムは以下の特徴を持ちます：

- **定義順序に依存しない展開**: 変数の定義順序に関わらず正しく展開
- **遅延評価**: 必要になったタイミングで初めて展開
- **メモ化**: 一度展開した結果をキャッシュして再利用
- **循環参照検出**: 無限ループを防ぐ循環参照の検出
- **型安全性**: 文字列変数と配列変数の混在を防止

## アーキテクチャ

### コア関数の階層構造

変数展開は2つの戦略に応じて、異なる階層で実装されています：

#### 即時展開（ExpandString経由）

```
ExpandString (パブリックAPI)
  ↓
resolveAndExpand (変数マップからresolverを生成)
  ↓
parseAndSubstitute (パース・置換のコアロジック)
```

| 関数名 | 役割 | 可視性 | 実装 |
|--------|------|--------|------|
| `ExpandString` | パブリックAPI（エントリーポイント） | public | [expansion.go](../../internal/runner/config/expansion.go) |
| `resolveAndExpand` | 変数マップからresolverを生成し再帰展開 | private | [expansion.go](../../internal/runner/config/expansion.go) |
| `parseAndSubstitute` | パース、エスケープ処理、変数置換のコアロジック | private | [expansion.go](../../internal/runner/config/expansion.go) |

#### 遅延展開（varExpander経由）

```
varExpander.expandString (エントリーポイント)
  ↓
varExpander.resolveVariable (変数解決・メモ化)
  ↓
parseAndSubstitute (パース・置換のコアロジック)
```

| 関数名 | 役割 | 可視性 | 実装 |
|--------|------|--------|------|
| `varExpander.expandString` | エントリーポイント（内部変数の展開） | private | [expansion.go](../../internal/runner/config/expansion.go) |
| `varExpander.resolveVariable` | 変数解決とメモ化による遅延評価 | private | [expansion.go](../../internal/runner/config/expansion.go) |
| `parseAndSubstitute` | パース、エスケープ処理、変数置換のコアロジック（両戦略で共有） | private | [expansion.go](../../internal/runner/config/expansion.go) |

### 2つの展開戦略

実装では、用途に応じて2つの異なる展開戦略を使い分けています：

#### 1. 即時展開（`ExpandString` 経由）

**用途**: 環境変数、コマンド引数など、すでに展開済みの変数を使った展開

**特徴**:
- 入力: `expandedVars map[string]string` （すべて展開済み）
- 処理: 変数参照を解決するだけ
- 状態: ステートレス（メモ化不要）
- 性能: 高速（マップ検索のみ）

**実装**: [expansion.go](../../internal/runner/config/expansion.go)

#### 2. 遅延展開（`varExpander` 経由）

**用途**: 変数定義の処理（`ProcessVars`）

**特徴**:
- 入力: `rawVars map[string]interface{}` （未展開）
- 処理: 必要時に動的に展開（順序独立）
- 状態: ステートフル（メモ化あり）
- 性能: 初回は展開コスト、2回目以降はキャッシュヒット

**実装**: [expansion.go](../../internal/runner/config/expansion.go)

## 遅延展開の詳細

### varExpander の内部構造

```go
type varExpander struct {
    expandedVars      map[string]string       // ① キャッシュ（展開済み）
    expandedArrayVars map[string][]string     // ② 配列変数キャッシュ
    rawVars           map[string]interface{}  // ③ ソース（未展開）
    level             string                  // エラーメッセージ用
}
```

3つのマップを使い分けることで、遅延評価を実現しています：

1. **expandedVars**: すでに展開済みの変数（メモ化キャッシュ）
2. **expandedArrayVars**: 展開済みの配列変数
3. **rawVars**: 未展開の変数定義（TOML から読み込んだ生データ）

### 解決アルゴリズム

`resolveVariable` メソッド（[expansion.go](../../internal/runner/config/expansion.go)）の処理フロー：

```
1. キャッシュチェック
   ├─ expandedVars[varName] が存在 → すぐ返す (O(1))
   └─ なし → 次へ

2. 配列変数チェック
   ├─ expandedArrayVars[varName] が存在 → エラー（文字列コンテキストで配列変数は使用不可）
   └─ なし → 次へ

3. 未展開変数の取得
   ├─ rawVars[varName] が存在 → 次へ
   └─ なし → エラー（未定義変数）

4. 動的展開
   ├─ visited[varName] = struct{}{} （循環参照検出用）
   ├─ parseAndSubstitute で再帰的に展開
   └─ delete(visited, varName) （展開完了後にマーク解除）

5. メモ化
   ├─ expandedVars[varName] = expanded （次回以降高速化）
   └─ return expanded
```

### 具体例: 定義順序に依存しない展開

#### TOML設定（逆順で定義）

```toml
[vars]
config_path = "%{base_dir}/config.toml"  # base_dirより先に定義
log_path = "%{base_dir}/logs"
base_dir = "/opt/myapp"                   # 後で定義
```

#### 初期状態

```go
rawVars = {
    "config_path": "%{base_dir}/config.toml",  // 未展開
    "log_path": "%{base_dir}/logs",            // 未展開
    "base_dir": "/opt/myapp"                   // 未展開
}
expandedVars = {}  // 空
```

#### 展開プロセス: `config_path` を展開

```
1. expandString("config_path", ...)
   ↓
2. resolveVariable("config_path")
   ├─ expandedVars["config_path"] → なし
   ├─ rawVars["config_path"] → "%{base_dir}/config.toml"
   └─ parseAndSubstitute("%{base_dir}/config.toml", ...)
      ↓ %{base_dir} を発見
3. resolveVariable("base_dir")  ← 再帰呼び出し
   ├─ expandedVars["base_dir"] → なし
   ├─ rawVars["base_dir"] → "/opt/myapp"
   └─ parseAndSubstitute("/opt/myapp", ...)
      ↓ 変数参照なし
      ✓ 展開完了: "/opt/myapp"
   ├─ expandedVars["base_dir"] = "/opt/myapp"  ← メモ化
   └─ return "/opt/myapp"
   ↓
4. 戻ってconfig_pathの展開続行
   "/opt/myapp/config.toml"
   ├─ expandedVars["config_path"] = "/opt/myapp/config.toml"  ← メモ化
   └─ return "/opt/myapp/config.toml"
```

#### 展開後の状態

```go
expandedVars = {
    "base_dir": "/opt/myapp",                    // メモ化済み
    "config_path": "/opt/myapp/config.toml"      // メモ化済み
}
```

#### 次の展開: `log_path`（高速）

```
1. resolveVariable("log_path")
   └─ parseAndSubstitute("%{base_dir}/logs", ...)
      ↓ %{base_dir} を発見
2. resolveVariable("base_dir")
   ├─ expandedVars["base_dir"] → "/opt/myapp"  ← キャッシュヒット！
   └─ return "/opt/myapp"  ← 再展開不要
   ↓
3. "/opt/myapp/logs"
```

## セキュリティ機能

### 1. 循環参照検出

循環参照を検出して無限ループを防ぎます。

**検出方法**: `visited` マップで現在展開中の変数を追跡

**実装**: [expansion.go](../../internal/runner/config/expansion.go)

```go
visited[varName] = struct{}{}  // 展開開始時にマーク
// ... 展開処理 ...
delete(visited, varName)       // 展開完了後に削除
```

**循環参照の例**:

```toml
[vars]
A = "%{B}"
B = "%{C}"
C = "%{A}"  # 循環！
```

**検出プロセス**:

```
resolveVariable("A")
  visited = {"A"}
  ↓ resolveVariable("B")
    visited = {"A", "B"}
    ↓ resolveVariable("C")
      visited = {"A", "B", "C"}
      ↓ resolveVariable("A")
        visited に "A" が既に存在 → ErrCircularReferenceDetail
```

### 2. 再帰深度制限

スタックオーバーフローを防ぐため、再帰深度を制限しています。

**制限値**: `MaxRecursionDepth = 100`

**実装**: [expansion.go](../../internal/runner/config/expansion.go)

```go
if depth >= MaxRecursionDepth {
    return "", &ErrMaxRecursionDepthExceededDetail{...}
}
```

### 3. 変数数の制限

DoS攻撃を防ぐため、各レベルでの変数数を制限しています。

**制限値**: `MaxVarsPerLevel = 1000`

**実装**: [expansion.go](../../internal/runner/config/expansion.go)

### 4. サイズ制限

メモリ枯渇を防ぐため、文字列長と配列サイズを制限しています。

**制限値**:
- `MaxStringValueLen = 10KB`
- `MaxArrayElements = 1000`

**実装**: [expansion.go](../../internal/runner/config/expansion.go), [expansion.go](../../internal/runner/config/expansion.go)

### 5. 変数名の検証

インジェクション攻撃を防ぐため、変数名を検証しています。

**実装**: `security.ValidateVariableName()` を使用 ([expansion.go](../../internal/runner/config/expansion.go))

### 6. 型安全性

文字列変数と配列変数の混在を防止します。

**ルール**:
- 文字列変数を配列変数で上書き → エラー
- 配列変数を文字列変数で上書き → エラー
- 文字列コンテキストで配列変数を参照 → エラー

**実装**: [expansion.go](../../internal/runner/config/expansion.go), [expansion.go](../../internal/runner/config/expansion.go)

## エスケープシーケンス

変数展開構文をリテラルとして使用する場合のエスケープをサポートしています。

**サポートするエスケープシーケンス**:
- `\%` → `%` （パーセント記号）
- `\\` → `\` （バックスラッシュ）

**実装**: [expansion.go](../../internal/runner/config/expansion.go)

**例**:

```toml
[vars]
literal_percent = "100\\% complete"    # → "100% complete"
escaped_backslash = "path\\\\to\\\\file"  # → "path\to\file"
```

## パフォーマンス最適化

### 1. メモ化

一度展開した変数はキャッシュされ、次回以降の参照は高速化されます。

**実装**: [expansion.go](../../internal/runner/config/expansion.go)

```go
e.expandedVars[varName] = expanded
```

**効果**: 同じ変数を複数回参照しても、展開は1回だけ

### 2. 遅延評価

変数は参照されるまで展開されません。

**効果**: 使用されない変数の展開コストを削減

### 3. 順序独立な展開

変数定義の順序に依存しないため、トポロジカルソートなどの前処理が不要です。

**効果**: 設定ファイル読み込み時のオーバーヘッドを削減

## エラーハンドリング

変数展開で発生する可能性のあるエラーと、それぞれの検出タイミング：

| エラー型 | 説明 | 検出タイミング | 実装 |
|---------|------|--------------|------|
| `ErrUndefinedVariableDetail` | 未定義の変数を参照 | 変数解決時 | [expansion.go](../../internal/runner/config/expansion.go) |
| `ErrCircularReferenceDetail` | 循環参照 | 変数解決時 | [expansion.go](../../internal/runner/config/expansion.go) |
| `ErrMaxRecursionDepthExceededDetail` | 再帰深度超過 | パース時 | [expansion.go](../../internal/runner/config/expansion.go) |
| `ErrInvalidVariableNameDetail` | 不正な変数名 | パース時 | [expansion.go](../../internal/runner/config/expansion.go) |
| `ErrUnclosedVariableReferenceDetail` | 閉じられていない `%{` | パース時 | [expansion.go](../../internal/runner/config/expansion.go) |
| `ErrInvalidEscapeSequenceDetail` | 不正なエスケープシーケンス | パース時 | [expansion.go](../../internal/runner/config/expansion.go) |
| `ErrTypeMismatchDetail` | 型の不一致（文字列⇔配列） | 変数検証時 | [expansion.go](../../internal/runner/config/expansion.go) |
| `ErrArrayVariableInStringContextDetail` | 文字列コンテキストでの配列変数参照 | 変数解決時 | [expansion.go](../../internal/runner/config/expansion.go) |
| `ErrTooManyVariablesDetail` | 変数数超過 | 変数検証時 | [expansion.go](../../internal/runner/config/expansion.go) |
| `ErrValueTooLongDetail` | 文字列長超過 | 変数検証時 | [expansion.go](../../internal/runner/config/expansion.go) |
| `ErrArrayTooLargeDetail` | 配列サイズ超過 | 変数検証時 | [expansion.go](../../internal/runner/config/expansion.go) |

すべてのエラーには詳細な文脈情報（level, field, 変数名など）が含まれ、デバッグが容易です。

## 使用例

### 基本的な変数参照

```toml
[vars]
app_name = "myapp"
base_dir = "/opt/%{app_name}"
config_file = "%{base_dir}/config.toml"
```

展開結果:
```
base_dir = "/opt/myapp"
config_file = "/opt/myapp/config.toml"
```

### 配列変数

```toml
[vars]
base_dir = "/opt/myapp"
paths = [
    "%{base_dir}/bin",
    "%{base_dir}/lib",
    "%{base_dir}/share"
]
```

展開結果:
```
paths = ["/opt/myapp/bin", "/opt/myapp/lib", "/opt/myapp/share"]
```

### 環境変数のインポート

```toml
[global]
env_allowed = ["HOME", "USER"]
env_import = ["home_dir=HOME", "current_user=USER"]

[vars]
user_config = "%{home_dir}/.config/myapp"
```

### ネストした参照

```toml
[vars]
env = "production"
region = "us-west"
cluster = "%{env}-%{region}"
endpoint = "https://%{cluster}.example.com/api"
```

展開結果:
```
cluster = "production-us-west"
endpoint = "https://production-us-west.example.com/api"
```

## まとめ

go-safe-cmd-runner の変数展開アルゴリズムは、以下の特徴を持ちます：

1. **柔軟性**: 定義順序に依存しない遅延評価
2. **効率性**: メモ化による高速化
3. **安全性**: 循環参照検出、サイズ制限、型安全性
4. **明確性**: 詳細なエラーメッセージ

これらの設計により、複雑な設定でも安全かつ効率的に変数を展開できます。
