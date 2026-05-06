# 要件定義書：shebang スクリプトのネットワークリスク解析

## 1. 概要

**目的:** `record` コマンドが shebang スクリプト（`#!/bin/bash`, `#!/usr/bin/env python3` 等）を解析する際、インタープリタバイナリの解析結果を用いてネットワーク系 API・動的ロード API・mprotect(PROT_EXEC) のリスク判定を行えるようにする。

**背景:**
最近の実装（task 0069〜0125）により、`record` はネイティブバイナリ（ELF/Mach-O）に対して以下を検出できるようになった。

- バイナリが直接インポートするネットワーク系シンボル（`socket`, `getaddrinfo` 等）
- 動的ロード API（`dlopen`, `dlsym`, `dlvsym`）
- mprotect(PROT_EXEC)（`SyscallAnalysis` 経由）
- 共有ライブラリ経由の推移的なネットワーク・動的ロード・mprotect リスク

shebang スクリプトについては、`record` 実行時に `ShebangInterpreter` フィールド（インタープリタパス等）を記録しており、インタープリタバイナリ自体の record も自動保存される。しかしながら、runner 側のリスク判定（`NetworkAnalyzer.analyzeBinarySignals`）はスクリプト自身の JSON のみを参照するため、インタープリタの JSON に保存されたリスク情報が利用されていない。

---

## 2. 目的とスコープ

### 2.1. 目的

- runner がスクリプトを処理する際、スクリプト自身の JSON に加えてインタープリタバイナリの JSON も参照してリスク判定を行う
- インタープリタバイナリの `record` 結果（`SymbolAnalysis`・`SyscallAnalysis`・`DynLibDeps`）を活用し、ネイティブバイナリと同等のリスク検出精度をスクリプトにも提供する

### 2.2. スコープ（In Scope）

- `runner/base/security/NetworkAnalyzer.analyzeBinarySignals()` の拡張
  - shebang スクリプトのインタープリタパスとハッシュを取得するための新ストアインターフェース追加
  - インタープリタの既存解析結果（`SymbolAnalysis`・`SyscallAnalysis`・`DynLibDeps`）を用いたリスク判定
- `fileanalysis` パッケージへの新ストアインターフェース実装追加
- `internal/runner/base/security` パッケージのテスト追加
- 本機能に関するドキュメント作成

### 2.3. スコープ外（Out of Scope）

- `record` 側コードの変更（インタープリタバイナリは既に自動記録されている）
- スクリプトのソースコード静的解析（`import socket` 等の内容解析）
- Windows PE バイナリの解析
- schema バージョンの更新（既存フィールドを活用）

---

## 3. 機能要件

### F-001: shebang インタープリタ解析パスの取得

**概要:** スクリプトの JSON レコードから、解析対象となるインタープリタバイナリのパスとコンテンツハッシュを取得する。

**詳細:**
- 新ストアインターフェース `ShebangInterpreterStore` を `fileanalysis` パッケージに追加
- メソッド: `LoadInterpreterAnalysisPath(scriptPath, scriptContentHash string) (interpPath, interpContentHash string, err error)`
- 処理内容:
  1. スクリプトのレコードをロードし、コンテンツハッシュを検証
  2. `ShebangInterpreter` フィールドからインタープリタパスを取得
     - `ResolvedPath` が非空（env 形式）→ `ResolvedPath` を使用
     - `ResolvedPath` が空（direct 形式）→ `InterpreterPath` を使用
  3. インタープリタのレコードをロードし、`ContentHash` を返す

### F-002: インタープリタの解析結果を用いたリスク判定

**概要:** `NetworkAnalyzer.analyzeBinarySignals()` がインタープリタのシグナルも評価する。

**詳細:**
- `analyzeBinarySignals(interpPath, interpHash)` を再帰呼び出しし、以下すべてを評価:
  - インタープリタの `SymbolAnalysis`（ネットワークシンボル・dynload シンボル）
  - インタープリタの `SyscallAnalysis`（svc #0x80・ネットワーク syscall）
  - インタープリタの `DynLibDeps`（推移的ライブラリのリスク）
- 結果は既存のリスクシグナルと OR 結合する

---

## 4. 非機能要件

### 4.1. 性能

- インタープリタの解析は既存 JSON レコードを読み込むだけであり、再解析は行わない
- `analyzeBinarySignals` の再帰呼び出しは最大 1 段（インタープリタは常にネイティブバイナリであり再帰は発生しない）

### 4.2. セキュリティ（フェイルセーフ）

- スクリプトのコンテンツハッシュ不一致: `ErrHashMismatch` として high risk 扱い
- インタープリタのレコードが存在しない: スキップ（インタープリタのリスク判定をスキップ）
  - verify 側の `VerifyCommandShebangInterpreter` がインタープリタ未記録を検出する
- インタープリタのレコードロードエラー: high risk 扱い（fail-closed）
- インタープリタのコンテンツハッシュが空（未記録）: インタープリタのリスク判定をスキップ

### 4.3. 互換性

- 既存スクリプトの JSON レコードは変更なし
- `record` 側コードは無修正
- schema バージョンの更新不要

### 4.4. 保守性

- 変更は `runner/base/security/network_analyzer.go` と新規ストア実装に集中
- 新ストアインターフェースは明確な責務を持つ

---

## 5. 受け入れ基準（Acceptance Criteria）

| ID | 条件 | 期待結果 |
|----|------|---------|
| AC-01 | `#!/bin/bash` スクリプトを runner が処理 | bash が network シンボル（`socket` 等）を持つ場合 `IsNetworkOperation()` が `true` を返す |
| AC-02 | `#!/usr/bin/env python3` スクリプトを runner が処理 | python3 バイナリの解析結果が利用される |
| AC-03 | インタープリタの共有ライブラリが mprotect(PROT_EXEC) リスクを持つ | `IsNetworkOperation()` の `isHighRisk` が `true` になる |
| AC-04 | インタープリタの共有ライブラリが dynload シンボルを持つ | `isHighRisk` が `true` になる |
| AC-05 | インタープリタレコードがストアに存在しない | リスク判定をスキップ（エラーなし） |
| AC-06 | インタープリタレコードのロードが失敗する | high risk 扱い（fail-closed） |
| AC-07 | 非スクリプトファイル（ELF バイナリ等）を runner が処理 | 既存の動作が変わらない |
| AC-08 | インタープリタのコンテンツハッシュが取得できない | インタープリタのリスク判定をスキップ（エラーなし） |

---

## 6. 用語集

| 用語 | 説明 |
|------|------|
| shebang | スクリプトファイルの先頭行 `#!` から始まるインタープリタ指定 |
| インタープリタバイナリ | shebang で指定されたバイナリ（`/bin/bash`, `/usr/bin/python3` 等）|
| SymbolAnalysis | バイナリがインポートするネットワーク系・dynload シンボルの解析結果 |
| SyscallAnalysis | バイナリのシステムコール解析結果（mprotect PROT_EXEC 含む）|
| DynLibDeps | バイナリが依存する共有ライブラリのリスト |
| ShebangInterpreterStore | スクリプトのレコードからインタープリタパス・ハッシュを取得するストアインターフェース |
| env 形式 | `#!/usr/bin/env python3` のように `env` を経由してインタープリタを指定する shebang |
| direct 形式 | `#!/bin/bash` のようにインタープリタパスを直接指定する shebang |
