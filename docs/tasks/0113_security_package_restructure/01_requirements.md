# 要件定義書: セキュリティパッケージの再構成

## 1. プロジェクト概要

### 1.1 目的

`internal/runner/security` パッケージおよびそのサブパッケージを、
`runner` 固有の責務と汎用的な責務に分離し、
`cmd/record`・`cmd/verify`・`internal/filevalidator`・`internal/libccache` の
`internal/runner/` 以下への依存を解消または最小化する。

### 1.2 背景

`cmd/record`・`cmd/verify` は `runner` とは独立した実行プログラムであるにも関わらず、
`internal/runner/security` およびそのサブパッケージ
（`binaryanalyzer/`・`elfanalyzer/`・`machoanalyzer/`）をインポートしている。
同様に、汎用ライブラリである `internal/filevalidator` と `internal/libccache` も
`internal/runner/security` 以下のサブパッケージに依存している。

これらの依存関係はアーキテクチャ上の不整合であり、以下の問題を引き起こす。

- `record`・`verify` のビルドが `runner` 内部の変更に影響される
- パッケージの責務が不明確になり、依存グラフが把握しにくくなる
- 汎用ライブラリが特定実行プログラムの内部パッケージに依存する

### 1.3 スコープ

**対象範囲:**

- `internal/security/` 新規パッケージの作成
  - バイナリ解析サブパッケージ（`binaryanalyzer/`・`elfanalyzer/`・`machoanalyzer/`）の移動
  - ディレクトリ権限チェック機能の抽出
  - TOCTOU チェックユーティリティの移動
- `internal/runner/security/` の修正
  - 移動済みコードの削除・`internal/security/` への委譲
- `cmd/record`・`cmd/verify`・`internal/filevalidator`・`internal/libccache` のインポートパス更新

**対象外:**

- `cmd/runner` の `internal/runner/security` 依存（runner 固有のため維持）
- `internal/verification` の `internal/runner/security` 依存（runner 固有のため維持）
- セキュリティロジックの変更（インポートパスの再配置のみ）
- 外部 API や設定ファイル形式の変更

---

## 2. 現状分析

### 2.1 問題のある依存関係

| インポート元 | インポート先 | 問題 |
|---|---|---|
| `cmd/record` | `internal/runner/security` | runner 固有 NS への依存 |
| `cmd/record` | `internal/runner/security/elfanalyzer` | runner 固有 NS への依存 |
| `cmd/verify` | `internal/runner/security` | runner 固有 NS への依存 |
| `internal/filevalidator` | `internal/runner/security/binaryanalyzer` | 汎用 pkg が runner 固有 NS に依存 |
| `internal/filevalidator` | `internal/runner/security/machoanalyzer` | 汎用 pkg が runner 固有 NS に依存 |
| `internal/libccache` | `internal/runner/security/elfanalyzer` | 汎用 pkg が runner 固有 NS に依存 |

### 2.2 `internal/runner/security` の責務分析

現在の `internal/runner/security` は以下の 2 種類の責務を混在させている。

**汎用セキュリティ（runner 非依存）:**

| ファイル／パッケージ | 機能 |
|---|---|
| `toctou_check.go` | TOCTOU チェックユーティリティ関数群 |
| `file_validation.go` | ディレクトリ・ファイル権限検証ロジック |
| `binary_analyzer.go` | OS 別バイナリ解析ファクトリ |
| `binaryanalyzer/` | バイナリ解析インタフェース定義 |
| `elfanalyzer/` | ELF バイナリ解析 |
| `machoanalyzer/` | Mach-O バイナリ解析 |

**runner 固有セキュリティ:**

| ファイル | 機能 |
|---|---|
| `validator.go` | `Validator` 型（コマンド検証・環境変数サニタイズ統合） |
| `command_analysis.go` | コマンドパス検証・危険コマンド判定 |
| `environment_validation.go` | 環境変数サニタイズ・検証 |
| `network_analyzer.go` | ネットワークシステムコール解析 |
| `logging_security.go` | ログ出力のセキュリティサニタイズ |
| `command_profile_def.go` 等 | コマンドリスクプロファイル |

### 2.3 各コマンドが利用する機能

**`cmd/record`:**

- `security.NewValidatorForTOCTOU()` — TOCTOU チェック用 Validator 生成
- `security.CollectTOCTOUCheckDirs()` — チェック対象ディレクトリ収集
- `security.RunTOCTOUPermissionCheck()` — 権限チェック実行
- `security.NewBinaryAnalyzer()` — バイナリ解析器生成
- `elfanalyzer.NewSyscallAnalyzer()` — システムコール解析器生成

**`cmd/verify`:**

- `security.NewValidatorForTOCTOU()` — TOCTOU チェック用 Validator 生成
- `security.CollectTOCTOUCheckDirs()` — チェック対象ディレクトリ収集
- `security.RunTOCTOUPermissionCheck()` — 権限チェック実行

---

## 3. 要件定義

### 3.1 機能要件

#### FR-1: `internal/security` パッケージの新設

汎用セキュリティ機能を収容する `internal/security` パッケージを新設する。

以下のコンポーネントを含む：

- `DirectoryPermChecker` インタフェース（`ValidateDirectoryPermissions(path string) error`）
- `NewDirectoryPermChecker()` ファクトリ関数（ OS 標準ファイルシステム使用）
- `TOCTOUViolation` 型
- `CollectTOCTOUCheckDirs()` 関数
- `ResolveAbsPathForTOCTOU()` 関数
- `RunTOCTOUPermissionCheck(checker DirectoryPermChecker, dirs []string, logger *slog.Logger)` 関数
- 共有エラー変数（`ErrInvalidDirPermissions`・`ErrInsecurePathComponent`・`ErrInvalidPath` 等）

**受け入れ基準:**

1. `internal/security` パッケージは `internal/runner/` 以下のパッケージを一切インポートしない
2. `go build ./internal/security/...` がエラーなく成功する

#### FR-2: バイナリ解析サブパッケージの移動

`internal/runner/security/binaryanalyzer/`・`machoanalyzer/` を
`internal/security/` 以下に移動する。
`elfanalyzer/` はコア層（`NewSyscallAnalyzer()`、命令デコーダ、解析共通処理）を
`internal/security/elfanalyzer/` へ移動し、`StandardELFAnalyzer` は
`internal/runner/security/elfanalyzer/` に残留する。

**受け入れ基準:**

1. `internal/security/binaryanalyzer/`・`internal/security/machoanalyzer/`・
  `internal/security/elfanalyzer/`（コア層）が `internal/runner/` 以下を一切インポートしない
2. 既存のすべてのテストが引き続き合格する

#### FR-3: `cmd/record`・`cmd/verify` の依存再編

`cmd/verify` は `internal/runner/security` 依存を完全に解消する。
`cmd/record` は `internal/runner/security/elfanalyzer` 依存を解消し、
`NewSyscallAnalyzer()` を `internal/security/elfanalyzer` から利用する。
一方で `NewBinaryAnalyzer()` のための `internal/runner/security` 依存は当面維持する。

**受け入れ基準:**

1. `cmd/verify` のインポートグラフに `internal/runner/security` が含まれない
2. `cmd/record` のインポートグラフに `internal/runner/security/elfanalyzer` が含まれない
3. `cmd/record` は `internal/security/elfanalyzer` から `NewSyscallAnalyzer()` を利用する
4. `cmd/record` の `internal/runner/security` 依存は `NewBinaryAnalyzer()` 用のみに限定される
5. `go build ./cmd/record/` および `go build ./cmd/verify/` がエラーなく成功する
6. `record`・`verify` コマンドの既存テストがすべて合格する

#### FR-4: `internal/filevalidator`・`internal/libccache` の依存解消

`internal/filevalidator` が `internal/runner/security/binaryanalyzer`・`machoanalyzer` を
インポートしなくなる。
`internal/libccache` が `internal/runner/security/elfanalyzer` をインポートしなくなる。
代わりに `internal/security/` 以下のパッケージを使用する。

**受け入れ基準:**

1. `internal/filevalidator` のインポートグラフに `internal/runner/security` が含まれない
2. `internal/libccache` のインポートグラフに `internal/runner/security` が含まれない
3. 各パッケージの既存テストがすべて合格する

#### FR-5: `internal/runner/security` の更新

`internal/runner/security` が移動済みコードを `internal/security` 経由で利用するよう更新する。

**受け入れ基準:**

1. `internal/runner/security` の `Validator` は `internal/security.DirectoryPermChecker`
   インタフェースを満たす
2. `cmd/runner` および `internal/verification` の既存テストがすべて合格する
3. `NewValidatorForTOCTOU()` は `internal/runner/security` に残留し、
   `cmd/runner` が引き続き使用できる

#### FR-6: 全テスト・ビルドの合格

リファクタリング後、すべての既存テストおよびビルドが合格する。

**受け入れ基準:**

1. `make test` がエラーなく成功する
2. `make build` がエラーなく成功する
3. `make lint` がエラーなく成功する

---

## 4. 非機能要件

### 4.1 後方互換性

- 外部公開 API（コマンドラインインタフェース・設定ファイル形式）を変更しない
- 内部パッケージのインタフェースは必要最小限の変更に留める

### 4.2 テスト戦略

- 既存テストコードはインポートパスの更新のみで再利用する
- 新設する `internal/security` の主要コンポーネントに単体テストを追加する

### 4.3 セキュリティ

- ディレクトリ権限チェックのロジックは変更しない
- TOCTOU 防止のアルゴリズムは変更しない
