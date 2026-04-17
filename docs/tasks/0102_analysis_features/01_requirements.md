# 要件定義書: 解析フィーチャーフラグ（AnalysisFeatures）の導入

## 1. 概要

### 1.1 背景

現行の `fileanalysis.CurrentSchemaVersion` は、レコード JSON の構造変更（フィールド追加・削除・型変更）と「どの解析が実施されたか」の両方を単一の整数で表している。このため、以下の 2 つの問題が生じている。

#### 問題 1: 不要な全バイナリ再 `record` の強制

特定のバイナリフォーマット固有の解析機能を追加するたびに `CurrentSchemaVersion` を上げると、その機能と無関係なバイナリも含む**すべての管理対象バイナリで `record` の再実行が必要**になる。例：

- Mach-O 専用機能（`LC_LOAD_DYLIB` 整合性検証）を追加してスキーマを 13 → 14 に上げると、Linux 環境の ELF バイナリを管理しているユーザーも再 `record` を強いられる
- 将来 arm64 専用のシステムコール抽出機能を追加した場合、arm64 を使用しない環境のユーザーにも影響が波及する

#### 問題 2: 不完全レコードを警告のみでスキップする挙動

`verifyDynLibDeps` は `SchemaVersionMismatchError`（`Actual < Expected`）を検出した場合、現状では**警告ログを出力して dynlib 検証をスキップし実行を許可する**。これはセキュリティ上の問題である。不完全な（古い）レコードに対して `runner` が実行を許可すべきではない。

### 1.2 目的

- 「レコードで実施された解析の種類」を `AnalysisFeatures` フラグで明示的に記録し、グローバルなスキーマバージョン bump なしに機能追加を行えるようにする
- `runner` が不完全なレコード（必要な解析が実施されていない）に対してエラーで終了するよう修正する

### 1.3 スコープ

- **対象**: `fileanalysis.Record` への `AnalysisFeatures` フィールド追加
- **対象**: `filevalidator.Validator.SaveRecord` での `AnalysisFeatures` フラグ設定
- **対象**: `verification.Manager.verifyDynLibDeps` でのフラグチェックへの移行
- **対象**: `SchemaVersionMismatchError`（`Actual < Expected`）のスキップ挙動をエラーに修正
- **対象外**: `CurrentSchemaVersion` 自体の廃止（JSON 構造変更時のフェイルセーフとして継続使用）
- **対象外**: `AnalysisFeatures` 導入以前に追加された既存解析（syscall 解析、シンボル解析等）のフラグ化（必要に応じて後続タスクで対応）

## 2. 用語定義

| 用語 | 定義 |
|------|------|
| `AnalysisFeatures` | `record` 実行時にどの解析が実施されたかを示すフラグの集合。`fileanalysis.Record` に追加する |
| フィーチャーフラグ | `AnalysisFeatures` 内の個別フラグ。各フラグは特定の解析が実施済みであることを示す |
| 旧レコード | `AnalysisFeatures` フィールドを持たないレコード（本タスク以前の `record` コマンドで作成） |
| グローバルスキーマバージョン | `CurrentSchemaVersion` で管理するバージョン番号。JSON 構造の互換性を保証するためのフェイルセーフ |

## 3. 機能要件

### FR-1: `AnalysisFeatures` 構造体の追加

`fileanalysis.Record` に以下のフィールドを追加する：

```go
// AnalysisFeatures records which optional analyses were performed during record.
// Nil indicates an old record that predates feature tracking; runner treats this as an error.
AnalysisFeatures *AnalysisFeatures `json:"analysis_features,omitempty"`
```

```go
// AnalysisFeatures is a set of flags indicating which analyses were performed
// during record.
// If a flag is true, the corresponding analysis was performed and reflected in
// the record.
// If a flag is false, the analysis was either not performed, not applicable to
// the binary, or not implemented yet.
type AnalysisFeatures struct {
    // ELFDynLibDeps indicates that DT_NEEDED dependency analysis for ELF
    // binaries (task 0074) was performed. It is false for non-ELF binaries.
    ELFDynLibDeps bool `json:"elf_dynlib_deps,omitempty"`

    // MachODynLibDeps indicates that LC_LOAD_DYLIB dependency analysis for
    // Mach-O binaries (task 0096) was performed. It is false for non-Mach-O
    // binaries.
    MachODynLibDeps bool `json:"macho_dynlib_deps,omitempty"`
}
```

`AnalysisFeatures` 導入に伴うグローバルスキーマバージョンの bump は行わない。追加フィールドは `omitempty` であり JSON 構造は後方互換である。

### FR-2: `record` コマンドでのフラグ設定

`filevalidator.Validator.SaveRecord` は `Store.Update` コールバック内でレコードを構築する際、`record.AnalysisFeatures` を必ずセットする（旧レコードを区別できるよう、非 nil であることを保証する）：

- ELF バイナリで elfDynlibAnalyzer.Analyze を呼び出した場合（解析が正常に試行された場合）: AnalysisFeatures.ELFDynLibDeps = true
  - 注意: 現在の `DynLibAnalyzer.Analyze()` は全ファイルに対して呼び出され、non-ELF と static ELF の双方で `(nil, nil)` を返す。フラグを正しく設定するには、ファイルが ELF であるかどうかを `Analyze()` の戻り値とは独立に判定する仕組みが必要（例: `Analyze()` の戻り値にフォーマット情報を含める、または `updateAnalysisRecord` 内で ELF マジックバイトを別途チェックする）。詳細はアーキテクチャ設計で定める
- Mach-O バイナリで `machoDynlibAnalyzer.Analyze` を呼び出した場合: `AnalysisFeatures.MachODynLibDeps = true`（タスク 0096 実装後に有効化される）
- アナライザーが nil（未注入）の場合は対応するフラグを `false` のまま維持する
- `AnalysisFeatures` は `record` ごとに毎回構築しなおす（stale な値を引き継がない）

### FR-3: `runner` でのフラグチェック

`verification.Manager.verifyDynLibDeps` を以下のロジックに修正する：

#### FR-3.1: 旧レコードの拒否

`record.AnalysisFeatures == nil`（`AnalysisFeatures` フィールドが存在しない旧レコード）の場合、`runner` はエラーを返し実行をブロックする。エラーメッセージは `record` の再実行を促す内容とする。

スキーマバージョン不一致でレコードのロードに失敗した場合（`SchemaVersionMismatchError`）も同様にエラーとする（FR-4 参照）。

#### FR-3.2: バイナリフォーマット別のフラグチェック

ロードしたレコードに `AnalysisFeatures` が存在する場合（非 nil）、対象バイナリのフォーマットに応じて必要なフラグをチェックする：

| 対象バイナリ | 必要なフラグ | フラグが false の場合の挙動 |
|------------|-------------|--------------------------|
| 動的リンク ELF バイナリ（`DT_NEEDED` あり） | `ELFDynLibDeps == true` | エラー（`record` 再実行を要求） |
| static ELF バイナリ（`DT_NEEDED` なし） | なし | フラグチェックをスキップ |
| Mach-O バイナリ | `MachODynLibDeps == true` | エラー（`record` 再実行を要求）（タスク 0096 実装後に有効化） |
| スクリプト・その他 | なし | フラグチェックをスキップ |

注意: `ELFDynLibDeps` は「動的リンク ELF に対する dynlib 依存解析が実施された」ことを示すフラグとする。したがって、static ELF（`DT_NEEDED` なし）には `ELFDynLibDeps == true` を要求しない。これは現行の `verifyDynLibDeps` の挙動（動的リンク ELF のみをブロックし、static ELF は許可する）と整合させるためである。
注意: Mach-O フォーマットの検出ロジックはタスク 0096 のスコープであり、本タスクでは ELF のフラグチェックのみを実装する。Mach-O 行は将来拡張のための設計指針として記載する。

フラグが `true` の場合、既存の `DynLibDeps` ハッシュ検証ロジックをそのまま適用する（フラグは「解析が行われた」事実を示すのみで、検証ロジックは変更しない）。

#### FR-3.3: `verifyDynLibDeps` 内のスキーマバージョンベース分岐の削除

`verifyDynLibDeps` 内の `SchemaVersionMismatchError.Actual < Expected` による警告＋スキップ分岐を削除し、FR-3.1 のフラグチェックに一本化する。`Store.Load` のスキーマバージョン検証自体は維持する（JSON 構造の互換性チェックとして継続使用）。

### FR-4: `SchemaVersionMismatchError`（`Actual < Expected`）のエラー化

`verifyDynLibDeps` 内の以下の処理を修正する：

```go
// 修正前: 警告 + スキップ（実行許可）
if schemaErr.Actual < schemaErr.Expected {
    slog.Warn(...)
    return nil  // ← 実行を許可してしまっている
}

// 修正後: エラー（実行ブロック）
// Actual < Expected / Actual > Expected を区別せず、どちらもエラーとして返す
return fmt.Errorf("failed to load record for dynlib verification: %w", err)
```

旧スキーマのレコードは `record` の再実行なしに `runner` が実行を許可してはならない。

## 4. 非機能要件

### NFR-1: 後方互換性（レコード構造）

`AnalysisFeatures` フィールドは `omitempty` であり、旧バージョンの `record` が生成したレコードは `AnalysisFeatures == nil` として読み込まれる。旧レコードを拒否する（FR-3.1）のはセキュリティ上の意図的な設計であり、後方互換は維持しない。

### NFR-2: 再 `record` の影響範囲

本タスクリリース後の初回 `record` で `AnalysisFeatures` が付与されるため、以後のレコードは有効となる。管理対象バイナリの全再 `record` が必要であるが、これは一回限りである。

以後は：
- ELF 専用機能を追加しても、Mach-O バイナリのレコードは再 `record` 不要（`MachODynLibDeps` フラグは維持される）
- Mach-O 専用機能を追加しても、ELF バイナリのレコードは再 `record` 不要（`ELFDynLibDeps` フラグは維持される）

ただし新機能のフラグが既存レコードに存在しない場合のエラー動作については、その機能追加タスクで定義する。

## 5. 受け入れ基準

### AC-1: `AnalysisFeatures` の追加

- `fileanalysis.Record` に `AnalysisFeatures *AnalysisFeatures` フィールドが追加されていること
- `AnalysisFeatures` が `omitempty` であり、フィールドを持たない既存レコードが正常にデシリアライズされること（`AnalysisFeatures == nil`）
- グローバル `CurrentSchemaVersion` が変更されていないこと

### AC-2: `record` コマンドでのフラグ設定

- ELF バイナリに対して `record` を実行すると、レコードに `analysis_features.elf_dynlib_deps: true` が設定されること
- スクリプトファイルに対して `record` を実行すると、`analysis_features` フィールドが存在するが `elf_dynlib_deps` / `macho_dynlib_deps` はいずれも `false`（省略）であること
- `record --force` を実行しても `AnalysisFeatures` が正しく設定されること
- Mach-O バイナリに対して `record` を実行すると、`analysis_features` フィールドが存在するが `macho_dynlib_deps` は `false`（省略）であること（タスク 0096 実装後に `true` が設定される。本タスクでは Mach-O アナライザーが未注入のため `false`）

### AC-3: `runner` での旧レコード拒否

- `AnalysisFeatures` なし（旧レコード）で `runner` を実行すると、エラーで終了し `record` 再実行を促すメッセージが出力されること
- `SchemaVersionMismatchError`（`Actual < Expected`）で `runner` を実行すると、警告のみでなくエラーで終了すること
- `SchemaVersionMismatchError`（`Actual > Expected`）の既存エラー動作は変更されないこと

### AC-4: `runner` でのフラグチェック

- ELF バイナリのレコードに `elf_dynlib_deps: false`（または未設定）の `AnalysisFeatures` が存在する場合、`runner` がエラーで終了すること
- ELF バイナリのレコードに `elf_dynlib_deps: true` の `AnalysisFeatures` が存在する場合、既存の dynlib 検証ロジックが適用されること
- スクリプト・非 ELF・非 Mach-O バイナリのレコードに `AnalysisFeatures` が存在しても、dynlib 関連フラグチェックをスキップして通常の ContentHash 検証のみが行われること
- Mach-O バイナリのフラグチェックはタスク 0096 のスコープとし、本タスクでは Mach-O バイナリを「その他」として扱う（dynlib フラグチェックをスキップ）

### AC-5: 既存テストへの非影響

- ELF `DynLibDeps` 検証の既存テストが `AnalysisFeatures` 追加後も全パスすること
- Mach-O `DynLibDeps` 検証の既存テストが `AnalysisFeatures` 追加後も全パスすること
- `make test` / `make lint` がエラーなしで通過すること

## 6. 先行タスクとの関係

| タスク | 関係 |
|-------|------|
| 0074 ELF DynLibDeps | `ELFDynLibDeps` フラグの導入元。`record` が ELF 解析を実施済みであることを明示するためにフラグを付与する |
| 0096 Mach-O LC_LOAD_DYLIB | `MachODynLibDeps` フラグの導入元。**未実装**。本タスクでは `AnalysisFeatures` 構造体にフィールドを定義するが、フラグの設定・チェックはタスク 0096 で実装する |
| 0102 本タスク | 0074 / 0096 で暗黙的にスキーマバージョンに依存していた「解析実施の証跡」を `AnalysisFeatures` に移管する |
