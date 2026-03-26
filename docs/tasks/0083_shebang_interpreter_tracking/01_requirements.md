# Shebang インタープリタ追跡 要件定義書

## 1. 概要

### 1.1 背景

`record` コマンドは、指定されたファイルのハッシュ値および ELF 解析結果を記録し、`runner` 実行時に改ざん検出を行う。

現状、スクリプトファイル（`#!/bin/sh` 等の shebang を持つファイル）は、ファイル本体のハッシュのみが記録される。しかし、スクリプトを実際に実行するインタープリタバイナリ（`/bin/sh`、`/usr/local/bin/python3` 等）は record・verify の対象外であり、インタープリタが差し替えられた場合に検出できない。

### 1.2 採用アプローチ

- **record 時**: スクリプトファイルの shebang を解析し、インタープリタバイナリを自動的に `record` 対象として追加する。インタープリタは独立したエントリ（独立した JSON ファイル）として保存する。スクリプト自身の `Record` にはインタープリタの解決済みパスを記録する。
- **runner 時**: スクリプト実行前に、`Record` に記録されたインタープリタパスを現在の環境で再解決し、パスが一致することを確認する。また、インタープリタバイナリのハッシュ検証も行う。

### 1.3 スコープ

**対象:**
- `#!` で始まるスクリプトファイルの shebang 解析
- `#!/bin/sh` 等の直接指定形式
- `#!/usr/bin/env <cmd>` 形式（`env` 自体と解決後の `<cmd>` の両方を record）
- runner 実行時のインタープリタ存在確認・パス一致確認・ハッシュ検証

**対象外:**
- `#!/usr/bin/env -S <cmd>` 等の `env` フラグ付き形式（検出時はエラーとする）
- インタープリタ自身が shebang スクリプトである場合の再帰的解析（検出時はエラーとする）

---

## 2. 用語定義

| 用語 | 定義 |
|------|------|
| shebang 行 | スクリプトファイルの先頭行で `#!` から始まる行（例: `#!/usr/bin/env python3`） |
| インタープリタパス | shebang 行の `#!` 直後に記述された実行ファイルパス（例: `/usr/bin/env`、`/bin/sh`） |
| コマンド名 | `env` 経由の場合に `env` に渡されるコマンド名（例: `python3`） |
| 解決済みパス | コマンド名を PATH 解決した絶対パス（例: `/usr/local/bin/python3`） |
| 独立エントリ | インタープリタバイナリを対象とした、スクリプトとは別の `Record` JSON ファイル |

---

## 3. 機能要件

### 3.1 shebang 解析

#### FR-3.1.1: shebang 検出

`record` 対象ファイルの先頭 2 バイトが `#!` である場合、shebang 行の解析を行う。

#### FR-3.1.2: インタープリタパスの抽出

shebang 行をスペース・タブで分割し、先頭トークンをインタープリタパスとして取得する。

例:
- `#!/bin/sh` → インタープリタパス: `/bin/sh`
- `#!/bin/bash -e` → インタープリタパス: `/bin/bash`（`-e` は無視）
- `#!/usr/bin/env python3` → インタープリタパス: `/usr/bin/env`、コマンド名: `python3`

#### FR-3.1.3: `env` 形式の判定

インタープリタパスのベース名が `env` である場合（例: `/usr/bin/env`）、第 2 トークンをコマンド名として取得し、`PATH` 環境変数を用いて絶対パスに解決する。

第 2 トークンが存在しない場合はエラーとする。第 2 トークンが `-` で始まるフラグである場合（例: `-S`、`-u`）もエラーとする。

#### FR-3.1.4: shebang 行の読み取り制限

shebang 解析は先頭の最大 1 行（改行文字まで）のみを読み取る。

Linux カーネルの `BINPRM_BUF_SIZE`（256 バイト）に準拠し、先頭 256 バイト以内に改行が見つからない場合はエラーとする。

### 3.2 record 時の動作

#### FR-3.2.1: インタープリタの独立 record

shebang 解析後、以下のバイナリに対して `SaveRecord` を呼び出す（スクリプトファイルの record に加えて）:

- インタープリタパス（例: `/bin/sh`、`/usr/bin/env`）
- `env` 形式の場合は解決済みパス（例: `/usr/local/bin/python3`）

#### FR-3.2.2: スクリプト Record へのインタープリタ情報の保存

スクリプトファイルの `Record` に `ShebangInterpreter` フィールド（JSON キー: `shebang_interpreter`）を追加し、record 時点で解決したインタープリタ情報を保存する。

```
ShebangInterpreter (JSON: shebang_interpreter):
  interpreter_path  string  // JSON: "interpreter_path" — shebang のインタープリタパス (e.g. "/bin/sh", "/usr/bin/env")
  resolved_path     string  // JSON: "resolved_path"    — env 形式の場合のみ。解決済みパス (e.g. "/usr/local/bin/python3")
```

`env` 形式でない場合、`resolved_path` は省略する。

#### FR-3.2.3: スキーマバージョンの更新

`CurrentSchemaVersion` を 10 → 11 に更新する。

### 3.3 runner 実行時の検証

#### FR-3.3.1: インタープリタ検証の実施タイミング

スクリプトファイルの実行前に、`Record` の `ShebangInterpreter` フィールドを参照してインタープリタ検証を行う。

`ShebangInterpreter` フィールドが存在しない（スキーマ v10 以前で record されたスクリプト）場合はエラーとして実行を拒否する。v11 以降で record されたスクリプトには必ず本フィールドが存在する。

#### FR-3.3.2: インタープリタ Record の存在確認

`ShebangInterpreter.interpreter_path` に対応する独立 Record が存在しない場合、エラーとして実行を拒否する。

`env` 形式の場合は `ShebangInterpreter.resolved_path` の独立 Record も確認する。

#### FR-3.3.3: インタープリタのハッシュ検証

`ShebangInterpreter.interpreter_path` のバイナリを通常の verify と同様にハッシュ検証する。

`env` 形式の場合は `ShebangInterpreter.resolved_path` のバイナリも検証する。

#### FR-3.3.4: `env` 形式のパス再解決と一致確認

`ShebangInterpreter.resolved_path` が設定されている場合、runner の実行環境で `env` コマンドと同じ PATH 解決を行い、解決されたパスが `ShebangInterpreter.resolved_path` と一致することを確認する。

一致しない場合はエラーとして実行を拒否する（PATH 操作による別バイナリへの誘導の検出）。

`env` 形式でない場合（例: `#!/bin/sh`）、インタープリタパスは絶対パスで固定されるため PATH 再解決は不要である。バイナリの差し替えは FR-3.3.3 のハッシュ検証で検出する。

### 3.4 エラー処理

| 状況 | 動作 |
|------|------|
| インタープリタパスが絶対パスでない | record/verify 時にエラー |
| `env` 形式で第 2 トークンが存在しない | record 時にエラー |
| `env` 形式で第 2 トークンが `-` で始まるフラグ（例: `-S`） | record 時にエラー |
| `env` のコマンド名が PATH 解決できない | record 時にエラー |
| インタープリタの独立 Record が存在しない | runner 実行時にエラー、実行拒否 |
| インタープリタのハッシュ不一致 | runner 実行時にエラー、実行拒否 |
| `env` 形式で解決パスが記録値と異なる | runner 実行時にエラー、実行拒否 |
| インタープリタ自身が shebang スクリプトである | record 時にエラー |
| shebang 行が先頭 256 バイト以内に改行を含まない | record 時にエラー |
| `ShebangInterpreter` フィールドが存在しない（v10 以前の Record） | runner 実行時にエラー、実行拒否 |

---

## 4. 将来の検討事項

### 4.1 `env` フラグ付き shebang

`#!/usr/bin/env -S python3 -v` や `#!/usr/bin/env PYTHONPATH=. python3` のような `env` フラグ付き形式は本タスクの対象外とし、record 時にエラーとする。将来対応する場合は `env` の `-S` フラグ仕様に基づいたパーサーが必要になる。

---

## 5. 受け入れ基準

| ID | 基準 |
|----|------|
| AC-1 | `#!/bin/sh` を含むスクリプトを record すると、`/bin/sh` の独立 Record が作成される |
| AC-2 | `#!/bin/bash -e` を含むスクリプトを record すると、`/bin/bash` の独立 Record が作成される（`-e` は無視） |
| AC-3 | `#!/usr/bin/env python3` を含むスクリプトを record すると、`/usr/bin/env` と解決済みの `python3` バイナリの独立 Record が作成される |
| AC-4 | スクリプトの `Record` に `shebang_interpreter` フィールドが保存される |
| AC-5 | `#!/usr/bin/env python3` の場合、`shebang_interpreter.interpreter_path` が `/usr/bin/env`、`shebang_interpreter.resolved_path` が解決済みパスになる |
| AC-6 | `#!/bin/sh` の場合、`shebang_interpreter.interpreter_path` が `/bin/sh`、`resolved_path` は省略される |
| AC-7 | インタープリタが未 record の状態でスクリプトを runner 実行するとエラーになり実行が拒否される |
| AC-8 | インタープリタのハッシュが変化した場合、runner 実行時に検証エラーになり実行が拒否される |
| AC-9 | `env` 形式で runner 実行時に PATH が変化し別パスに解決される場合、エラーになり実行が拒否される |
| AC-10 | shebang を持たないバイナリ（ELF 等）を record しても動作が変わらない |
| AC-11 | shebang を持たないテキストファイルを record しても動作が変わらない |
| AC-12 | `#!/usr/bin/env -S python3` のような `env` フラグ付き形式を record するとエラーになる |
| AC-13 | インタープリタ自身が shebang スクリプトである場合（例: `/usr/bin/python3` が `#!/bin/sh` を先頭に持つ）、record するとエラーになる |
| AC-14 | インタープリタパスが絶対パスでない（例: `#!python3`）スクリプトを record するとエラーになる |
| AC-15 | `#!/usr/bin/env` のみで後続トークンがない場合、record するとエラーになる |
| AC-16 | `#!/usr/bin/env nonexistent_cmd` のように PATH 解決できないコマンド名の場合、record するとエラーになる |
| AC-17 | shebang 行が先頭 256 バイト以内に改行を含まないスクリプトを record するとエラーになる |
| AC-18 | スキーマ v10 以前で record されたスクリプト（`shebang_interpreter` フィールドなし）を runner 実行するとエラーになり実行が拒否される |
