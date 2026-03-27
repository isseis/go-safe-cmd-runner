# Shebang インタープリタ追跡 要件定義書

## 1. 概要

### 1.1 背景

`record` コマンドは、指定されたファイルのハッシュ値および ELF 解析結果を記録し、`runner` 実行時に改ざん検出を行う。

現状、スクリプトファイル（`#!/bin/sh` 等の shebang を持つファイル）は、ファイル本体のハッシュのみが記録される。しかし、スクリプトを実際に実行するインタープリタバイナリ（`/bin/sh`、`/usr/local/bin/python3` 等）は record・runner 実行時検証の対象外であり、インタープリタが差し替えられた場合に検出できない。

### 1.2 採用アプローチ

- **record 時**: スクリプトファイルの shebang を解析し、インタープリタバイナリを自動的に `record` 対象として追加する。インタープリタは独立したエントリ（独立した JSON ファイル）として保存する。スクリプト自身の `Record` にはインタープリタのパスを記録する。`env` 形式（`#!/usr/bin/env python3` 等）の場合は `env` 自身のパスに加え、コマンドの解決済みパスも記録する。
- **runner 時**: スクリプト実行前に、`Record` に記録された `ShebangInterpreter` を参照してインタープリタ検証を行う。`env` 形式では `Record` の `command_name` を runner の実行環境で PATH 解決し、記録済みの `resolved_path` と一致することを確認する。direct 形式（`#!/bin/sh` 等）では PATH 再解決は不要で、インタープリタバイナリのハッシュ検証のみ行う。

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

shebang 行の `#!` プレフィックス（先頭 2 バイト）を除去した後、先頭の空白（スペース・タブ）をスキップし、残りをスペース・タブで分割して先頭トークンをインタープリタパスとして取得する。

例:
- `#!/bin/sh` → インタープリタパス: `/bin/sh`
- `#! /bin/sh` → インタープリタパス: `/bin/sh`（`#!` 直後の空白を許容）
- `#!/bin/bash -e` → インタープリタパス: `/bin/bash`（`-e` は無視）
- `#!/usr/bin/env python3` → インタープリタパス: `/usr/bin/env`、コマンド名: `python3`

インタープリタパスが空文字列になる場合（`#!\n` や `#!  \n` 等）はエラーとする。

#### FR-3.1.3: `env` 形式の判定

インタープリタパスのベース名が `env` である場合（例: `/usr/bin/env`）、第 2 トークンをコマンド名として取得し、record 実行時のプロセス環境の `PATH` 環境変数を用いて絶対パスに解決する。

第 2 トークンが存在しない場合はエラーとする。第 2 トークンが以下のいずれかに該当する場合もエラーとする:

- `-` で始まるフラグ（例: `-S`、`-u`）
- `=` を含む環境変数代入形式（例: `PYTHONPATH=.`）

#### FR-3.1.4: shebang 行の読み取り制限

shebang 解析は先頭の最大 1 行（改行文字まで）のみを読み取る。

Linux カーネルの `BINPRM_BUF_SIZE`（256 バイト）に準拠し、先頭 256 バイト以内に改行が見つからない場合はエラーとする。

改行文字は `\n` のみを認識する（Linux カーネル準拠）。`\r\n` 改行のスクリプトでは `\r` がインタープリタパスの末尾に含まれ、絶対パスとして不正な値になるためエラーとして検出される。

### 3.2 record 時の動作

#### FR-3.2.1: インタープリタの独立 record

shebang 解析後、以下のバイナリに対して `SaveRecord` を呼び出す（スクリプトファイルの record に加えて）:

- インタープリタパス（例: `/bin/sh`、`/usr/bin/env`）
- `env` 形式の場合は解決済みパス（例: `/usr/local/bin/python3`）

各パスは `filepath.EvalSymlinks` でシンボリックリンクを解決した実体パスを使用する（DynLibDeps の取り扱いと同一）。`interpreter_path` および `resolved_path` に記録するパスも、同様にシンボリックリンク解決済みの絶対パスとする。

インタープリタバイナリの `SaveRecord` は `force=true` で呼び出す。これにより、同一パスが既に record 済みであっても `ErrHashFileExists` を返さず上書き更新される。同一インタープリタの独立 Record は常に 1 つだけ存在する。

#### FR-3.2.2: スクリプト Record へのインタープリタ情報の保存

スクリプトファイルの `Record` に `ShebangInterpreter` フィールド（JSON キー: `shebang_interpreter`）を追加し、record 時点で解決したインタープリタ情報を保存する。

```
ShebangInterpreter (JSON: shebang_interpreter):
  interpreter_path  string  // JSON: "interpreter_path" — shebang のインタープリタパス (e.g. "/bin/sh", "/usr/bin/env")
  command_name      string  // JSON: "command_name"      — env 形式の場合のみ。env に渡すコマンド名 (e.g. "python3")
  resolved_path     string  // JSON: "resolved_path"     — env 形式の場合のみ。解決済みパス (e.g. "/usr/local/bin/python3")
```

`env` 形式でない場合、`command_name` および `resolved_path` は省略する。

shebang を持たないファイル（ELF バイナリ・テキストファイル等）の `Record` には `ShebangInterpreter` フィールドを設定しない（JSON 出力で `omitempty` により省略される）。

#### FR-3.2.3: スキーマバージョンの更新

`CurrentSchemaVersion` を 10 → 11 に更新する。

### 3.3 runner 実行時の検証

#### FR-3.3.1: インタープリタ検証の実施タイミング

runner はコマンド実行前に対象ファイルの `Record` を参照し、`ShebangInterpreter` フィールドの有無でインタープリタ検証を実施するかを決定する:

- `ShebangInterpreter` フィールドが存在する場合: FR-3.3.2〜FR-3.3.4 のインタープリタ検証を実施する。
- `ShebangInterpreter` フィールドが存在しない場合: 当該ファイルはスクリプトでないと判断し、インタープリタ検証をスキップする。

スキーマ v10 以前の `Record` については、インタープリタ検証に到達する前にスキーマバージョン不一致（`SchemaVersionMismatchError`）として実行が拒否される。

#### FR-3.3.2: インタープリタ Record の存在確認

`ShebangInterpreter.interpreter_path` に対応する独立 Record が存在しない場合、エラーとして実行を拒否する。

`env` 形式の場合は `ShebangInterpreter.resolved_path` の独立 Record も確認する。

#### FR-3.3.3: インタープリタのハッシュ検証

`ShebangInterpreter.interpreter_path` のバイナリを通常の runner 実行時検証と同様にハッシュ検証する。

`env` 形式の場合は `ShebangInterpreter.resolved_path` のバイナリも検証する。

#### FR-3.3.4: `env` 形式のパス再解決と一致確認

`ShebangInterpreter.command_name` が設定されている場合（`env` 形式）、runner は `Record` から取得した `command_name` を用いて PATH 解決を行い、解決されたパスが `ShebangInterpreter.resolved_path` と一致することを確認する。

ここでの PATH は、runner が当該コマンド実行に実際に使用する最終環境（設定適用後の環境変数）を指す。

一致しない場合はエラーとして実行を拒否する（PATH 操作による別バイナリへの誘導の検出）。

`env` 形式でない場合（例: `#!/bin/sh`）、インタープリタパスは絶対パスで固定されるため PATH 再解決は不要である。バイナリの差し替えは FR-3.3.3 のハッシュ検証で検出する。

#### FR-3.3.5: `skip_standard_paths` との関係

`skip_standard_paths` オプションはインタープリタ検証（FR-3.3.2〜FR-3.3.4）には適用しない。インタープリタが `/bin/sh` 等の標準パスに存在する場合でも、Record の存在確認・ハッシュ検証・パス再解決を省略しない。

FR-3.2.1 でインタープリタは常に `record` 対象となるため、標準パスのインタープリタであっても独立 Record が必ず存在し、`skip_standard_paths` を有効にした環境でも Record 欠落によるエラーは発生しない。

### 3.4 エラー処理

| 状況 | 動作 |
|------|------|
| インタープリタパスが空（`#!\n` 等） | record コマンド実行時にエラー |
| インタープリタパスが絶対パスでない | record コマンド実行時にエラー |
| `env` 形式で第 2 トークンが存在しない | record コマンド実行時にエラー |
| `env` 形式で第 2 トークンが `-` で始まるフラグ（例: `-S`） | record コマンド実行時にエラー |
| `env` 形式で第 2 トークンが `=` を含む環境変数代入（例: `PYTHONPATH=.`） | record コマンド実行時にエラー |
| `env` のコマンド名が PATH 解決できない | record コマンド実行時にエラー |
| インタープリタ自身が shebang スクリプトである | record コマンド実行時にエラー |
| shebang 行が先頭 256 バイト以内に改行を含まない | record コマンド実行時にエラー |
| インタープリタの独立 Record が存在しない | runner 実行時にエラー、実行拒否 |
| インタープリタのハッシュ不一致 | runner 実行時にエラー、実行拒否 |
| `env` 形式で PATH 再解決結果が `resolved_path` と異なる | runner 実行時にエラー、実行拒否 |
| スキーマバージョン不一致（v10 以前の Record） | runner 実行時にエラー（`SchemaVersionMismatchError`）、実行拒否 |

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
| AC-5 | `#!/usr/bin/env python3` の場合、`shebang_interpreter.interpreter_path` が `/usr/bin/env`、`shebang_interpreter.command_name` が `python3`、`shebang_interpreter.resolved_path` が解決済みパスになる |
| AC-6 | `#!/bin/sh` の場合、`shebang_interpreter.interpreter_path` が `/bin/sh`、`command_name` および `resolved_path` は省略される |
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
| AC-18 | スキーマ v10 以前で record されたスクリプトを runner 実行すると `SchemaVersionMismatchError` で実行が拒否される |
| AC-19 | `env` 形式の再解決は、runner が実際に使用する最終環境（設定適用後）の PATH で実施される |
| AC-20 | `env` 形式の再解決には `Record` の `command_name` を使用し、スクリプトの shebang を runner 実行時に再解析することは行わない |
| AC-21 | `#! /bin/sh`（`#!` 直後にスペース）を含むスクリプトを record すると、`/bin/sh` の独立 Record が作成される |
| AC-22 | `#!\n` のように空のインタープリタパスを持つスクリプトを record するとエラーになる |
| AC-23 | インタープリタパスがシンボリックリンクである場合、`interpreter_path` にはシンボリックリンク解決済みの実体パスが記録される |
| AC-24 | `#!/usr/bin/env PYTHONPATH=. python3` のような環境変数代入形式を record するとエラーになる |
