# libSystem.dylib syscall ラッパー関数キャッシュ 要件定義書

## 1. 概要

### 1.1 背景

タスク 0079 では、ELF バイナリの libc（`libc.so.6`）のエクスポート関数を関数単位で静的解析
し、「エクスポート関数名 → 実際に呼び出される syscall 番号」のマッピングをキャッシュする
機能を実装した。これにより、動的リンクバイナリが libc 経由で呼び出すシステムコールを
インポートシンボル照合によって検出できるようになった。

macOS の正規バイナリは `libSystem.dylib`（およびサブコンポーネント `libsystem_kernel.dylib`）
経由でシステムコールを発行する。したがって、ELF 版 libc キャッシュと同等の検出を macOS で
実現するには、`libSystem.dylib` のエクスポート関数に対して同様の syscall 解析を適用する
必要がある。

しかし macOS 11 (Big Sur) 以降、`libSystem.dylib` を含むシステムライブラリはファイル
システム上に個別ファイルとして存在せず、dyld shared cache に統合されている。このため
ELF 版のように「ファイルを直接開いてエクスポート関数を逆アセンブル」することが
できないケースが大半である。

タスク 0097 では `svc #0x80`（直接 syscall 命令）の検出とキャッシュ統合を実装したが、
正規 macOS バイナリは `svc #0x80` を使用しないため、このシグナルで検出できるのは
マルウェア的挙動のバイナリに限定される。`libSystem.dylib` 経由のネットワーク syscall
（`socket`, `connect` 等）を検出するには、本タスクのキャッシュが必要となる。

### 1.2 目的

- 動的リンク Mach-O バイナリが `libSystem.dylib` 経由で呼び出す syscall を、インポート
  シンボル照合によって検出できるようにする
- `libSystem.dylib` の syscall ラッパー解析結果をキャッシュし、`record` 実行のたびに
  ライブラリを再解析するコストを避ける
- ハードコードされた対応表を持たず、ライブラリの実際の実装から syscall 番号を実測する
  ことで、macOS バージョン差異に対応する
- 既存の ELF 版 libc キャッシュ（タスク 0079）の設計を最大限再利用し、実装コストを
  最小化する

### 1.3 スコープ

- **対象**: 動的リンクされた Mach-O バイナリ（`DynLibDeps` に `libSystem.dylib` 系ライブラリ
  が記録されているもの）
- **対象ライブラリ**: `libSystem.dylib` および `libsystem_kernel.dylib`（syscall ラッパーが
  実装されている実体）
- **ラッパー関数の特定方法**: 対象ライブラリのエクスポート関数を関数単位で逆アセンブルし、
  `svc #0x80` 命令を含み、かつ関数サイズが閾値以下のものを対象とする
- **キャッシュ保存場所**: 記録ファイル保存ディレクトリ直下の `lib-cache/` サブディレクトリ
  （ELF と共通）
- **キャッシュトリガー**: `record` コマンド実行時に自動的に生成・参照する
- **対象外**: `libSystem.dylib` 以外のライブラリ（`libcurl.dylib` 等）
- **対象外**: 静的 Mach-O バイナリ（`SyscallAnalysis` ベースの既存フローを維持）
- **対象外**: スクリプトファイル、ELF バイナリ（タスク 0079 が対応済み）
- **対象外**: `verify` コマンドの検証対象への追加（ハッシュ値検証のみで十分）

### 1.4 段階的リリース方針

dyld shared cache からのライブラリ抽出は実装コストが高いため、段階的リリースとする。

**段階 1（本タスクのスコープ）**: ファイルシステム上に `libSystem.dylib` /
`libsystem_kernel.dylib` が存在する場合のみ関数単位解析を実行し、キャッシュを構築する。
ファイルが存在しない場合（dyld shared cache 内）は解析をスキップし、**シンボル名
単体一致にフォールバック**する。

**段階 2（将来タスク）**: `blacktop/ipsw` の `pkg/dyld` パッケージ等を用いて
dyld shared cache から対象ライブラリを抽出し、段階 1 と同じ解析を適用する。

### 1.5 前提条件

- タスク 0096（Mach-O `LC_LOAD_DYLIB` 整合性検証）の実装が完了しており、Mach-O バイナリの
  `DynLibDeps` が `record` 時に記録されていること
- タスク 0097（Mach-O svc #0x80 キャッシュ統合）の実装が完了しており、スキーマバージョンが
  v15 であること

## 2. 用語定義

| 用語 | 定義 |
|------|------|
| `libSystem.dylib` | macOS のシステムコールラッパーを提供するフレームワーク。Linux の libc に相当する。実体は `libsystem_kernel.dylib` 等のサブコンポーネントに分かれている |
| `libsystem_kernel.dylib` | `libSystem.dylib` のサブコンポーネントで、BSD syscall ラッパー（`socket`, `connect`, `mprotect` 等）の実装を含む |
| dyld shared cache | macOS のシステム `.dylib` を単一の大きなキャッシュファイルに統合した機構。macOS 11 以降は個別の `.dylib` ファイルがファイルシステム上に存在しない場合がある |
| syscall ラッパー関数 | ライブラリのエクスポート関数のうち、関数本体内に `svc #0x80` 命令を含み、かつ関数サイズが閾値以下のもの。`socket`, `connect`, `write` 等 |
| ライブラリキャッシュ | ライブラリファイルのパスをファイル名に、スキーマバージョンとハッシュ値を有効性の判定キーとして、syscall ラッパー関数の一覧（関数名 → 実測 syscall 番号）を保存したファイル |
| インストール名 | `LC_LOAD_DYLIB` に記録されるライブラリのパス文字列（例: `/usr/lib/libSystem.B.dylib`）。ELF の `DT_NEEDED` SOName に相当する |
| `Source` フィールド | `SyscallInfo.Source` で検出手法を識別する文字列。libSystem シンボル照合由来は `"libsystem_symbol_import"` |
| BSD syscall 番号 | macOS arm64 における syscall 番号。`x16` レジスタにクラスプレフィックス `0x2000000`（BSD クラス）を含む値が設定される |

## 3. 機能要件

### 3.1 ライブラリキャッシュファイル

#### FR-3.1.1: キャッシュファイルの内容

ライブラリキャッシュファイルは ELF 版（タスク 0079）と同じ JSON スキーマで保持する。

```json
{
  "schema_version": 1,
  "lib_path": "/usr/lib/libSystem.B.dylib",
  "lib_hash": "sha256:a1b2c3...",
  "analyzed_at": "2026-04-19T00:00:00Z",
  "syscall_wrappers": [
    { "name": "connect", "number": 98  },
    { "name": "socket",  "number": 97  },
    { "name": "write",   "number": 4   }
  ]
}
```

`syscall_wrappers` は `number` 昇順、同一 `number` 内では `name` 昇順の複合キーで
ソートして保存する（タスク 0079 FR-3.1.1 と同一）。

`syscall_wrappers` の各エントリが持つ `number` は、ハードコードされた対応表ではなく
ライブラリの実際の実装から実測した値である。macOS arm64 の BSD syscall 番号は
`x16` レジスタに `0x2000000 | <番号>` として設定されるが、キャッシュには
クラスプレフィックスを除いた番号（例: `socket` = 97）を保存する。

#### FR-3.1.2: キャッシュファイルの命名規則

ELF 版（タスク 0079 FR-3.1.2）と同じ命名規則を使用する。`DynLibDeps` に記録された
ライブラリの解決済み実体ファイルパスを `internal/filevalidator/pathencoding` パッケージの
エンコーディング方式でエンコードする。

#### FR-3.1.3: キャッシュファイルの保存場所

ELF 版（タスク 0079 FR-3.1.3）と同じ `<hash-dir>/lib-cache/` サブディレクトリに
保存する。ELF の libc キャッシュと Mach-O の libSystem キャッシュは同一ディレクトリに
共存するが、ファイルパスのエンコーディングにより衝突は発生しない。

#### FR-3.1.4: キャッシュの有効性判定

ELF 版（タスク 0079 FR-3.1.4）と同じ 3 条件で判定する。

1. JSON パースが成功すること
2. `schema_version` がコード側の `LibcCacheSchemaVersion` 定数と一致すること
3. `lib_hash` が現在のライブラリファイルのハッシュ値と一致すること

#### FR-3.1.5: libSystem 系ライブラリの特定ルール

`DynLibDeps` の各エントリの `SOName`（インストール名）を走査し、以下の条件に一致する
エントリを libSystem 系ライブラリとみなす。

**優先順位**:
1. `SOName` が `/usr/lib/libSystem.B.dylib` に一致するエントリ
2. `SOName` のベース名が `libsystem_kernel.dylib` に一致するエントリ

`libSystem.B.dylib` は `libsystem_kernel.dylib` を含むアンブレラフレームワークで
あるため、`libSystem.B.dylib` が `DynLibDeps` に含まれている場合は
`libsystem_kernel.dylib` のキャッシュ参照のみ行う（`libSystem.B.dylib` 自体は
再エクスポートシンボルのみを含み、syscall ラッパーの実体コードは
`libsystem_kernel.dylib` に存在する）。

**`libsystem_kernel.dylib` の解決方法**:

1. `libsystem_kernel.dylib` が `DynLibDeps` に直接含まれている場合：そのエントリの
   `Path` と `Hash` をそのまま使用する
2. `libSystem.B.dylib` が `DynLibDeps` に含まれているが、`libsystem_kernel.dylib` が
   直接含まれていない場合：
   - `libSystem.B.dylib` がファイルシステム上に存在する場合：`LC_REEXPORT_DYLIB` を
     走査して `libsystem_kernel.dylib` のインストール名を特定し、タスク 0096 で実装した
     ライブラリパス解決ロジックを再利用して実体パスを取得する
   - `libSystem.B.dylib` がファイルシステム上に存在しない場合（dyld shared cache 内）：
     ウェルノウンパス `/usr/lib/system/libsystem_kernel.dylib` を試行する。
     ファイルが存在すればそのパスで解析を進める。存在しなければ段階 1 フォールバック
     （FR-3.4）を適用する
3. libSystem 系ライブラリが `DynLibDeps` に含まれていない場合：段階 1 フォールバック
   （FR-3.4）を適用する

**dyld shared cache 内のケース**: 上記いずれの方法でも `libsystem_kernel.dylib` が
ファイルシステム上に存在しない場合（macOS 11+ の通常環境）は、キャッシュ生成を
スキップし、段階 1 フォールバック（FR-3.4）を適用する。

### 3.2 libSystem 系ライブラリの関数単位解析

#### FR-3.2.1: エクスポート関数の列挙

`libsystem_kernel.dylib` の Mach-O シンボルテーブル（`LC_SYMTAB`）から
エクスポートシンボル（外部定義済み・関数型）を列挙する。Go 標準ライブラリ
`debug/macho` の `File.Symtab` を使用する。

シンボルのアドレスは `Symtab` から取得し、関数のサイズは隣接するシンボルの
アドレス差から推定する（Mach-O のシンボルテーブルには ELF の `st_size` に相当する
フィールドがないため）。最後のシンボル（隣接シンボルが存在しない場合）は
`__TEXT,__text` セクション終端までをサイズとする。非関数シンボルの扱い等の
エッジケースは設計・詳細仕様フェーズで対処する。

#### FR-3.2.2: 関数サイズによるフィルタリング

ELF 版（タスク 0079 FR-3.2.2）と同じ閾値（**256 バイト超**で除外）を初期値として
適用する。macOS arm64 の `libsystem_kernel.dylib` における syscall ラッパーの
サイズ分布が ELF の libc と大きく異なる場合は、実測に基づいて調整する。

#### FR-3.2.3: 関数単位の syscall 命令検出

サイズフィルタを通過した関数について、`__TEXT,__text` セクション内の該当アドレス
範囲を走査し、`svc #0x80`（エンコード `0xD4001001`）を検出する。

**syscall 番号の特定**: `svc #0x80` 命令の直前で `x16` レジスタに設定されている
即値を後方スキャンで特定する。macOS arm64 の BSD syscall 番号はクラスプレフィックス
`0x2000000` を含むため（例: `socket` = `0x2000061`）、解析時にプレフィックスを
除去して番号のみ（97）を抽出する。

**後方スキャン**: `svc #0x80` から最大 N 命令を遡り、`x16` への即値設定命令
（`mov x16, #imm` または `movz + movk` シーケンス）を検出する。

**複数 syscall 番号の扱い**: 1 つの関数が複数の `svc #0x80` 命令を含む場合、
検出したすべての syscall 番号がすべて同一であればその番号を採用する。1 つでも
異なる syscall 番号が検出された場合は、その関数をキャッシュに含めない
（ELF 版 FR-3.2.3 と同一方針）。

#### FR-3.2.4: アーキテクチャの対応範囲

arm64 のみを対象とする（macOS arm64 のみがランタイム対象であるため）。
x86_64 Mach-O 向けの解析は本タスクのスコープ外とする。

### 3.3 `record` コマンドの拡張

#### FR-3.3.1: キャッシュの参照と生成

`record` コマンド実行時、対象 Mach-O バイナリの `DynLibDeps` に libSystem 系ライブラリ
が含まれている場合、以下を行う。

1. `libsystem_kernel.dylib` の実体ファイルパスとハッシュ値を特定する（FR-3.1.5）
2. ファイルシステム上にライブラリが存在するか確認する
3. 存在する場合：対応するキャッシュファイルが存在し FR-3.1.4 の条件を満たす場合は
   キャッシュを読み込む。条件を満たさない場合はライブラリを解析してキャッシュを生成する
4. 存在しない場合（dyld shared cache 内）：段階 1 フォールバック（FR-3.4）を適用する

**保存順序**: ELF 版（タスク 0079 FR-3.3.1）と同じく、キャッシュファイルの書き込みが
成功した後にのみ記録ファイルを保存する。

**失敗時の挙動**:

| ケース | 挙動 |
|--------|------|
| キャッシュファイルが破損（JSON パース失敗等） | 再解析を試みる。再解析も失敗した場合はエラーで終了 |
| ライブラリファイルが読み取れない（権限不足） | エラーで終了 |
| ライブラリのエクスポートシンボル取得失敗 | エラーで終了 |
| ライブラリの解析中に予期しないエラー | エラーで終了 |
| キャッシュファイルの書き込み失敗 | エラーで終了 |
| ライブラリがファイルシステム上に存在しない（dyld shared cache 内） | 解析をスキップし、段階 1 フォールバック（FR-3.4）を適用。エラーなし |
| ライブラリが非 arm64 アーキテクチャ | 解析をスキップし、`SyscallAnalysis` に libSystem 由来エントリなしで継続 |

#### FR-3.3.2: インポートシンボルとキャッシュの照合

対象 Mach-O バイナリのインポートシンボル一覧（`debug/macho` `File.ImportedSymbols()`）を
取得し、キャッシュ内の `syscall_wrappers` と関数名で照合する。Mach-O のインポート
シンボルはアンダースコアプレフィックス（`_socket` 等）を持つため、照合前にプレフィックスを
除去する（タスク 0073 の `normalizeSymbolName` を再利用）。

一致したシンボルを `SyscallInfo` として `SyscallAnalysis.DetectedSyscalls` に追加する。

**macOS syscall 番号テーブル**: BSD syscall の番号 → ネットワーク関連かどうか（`IsNetwork`）
の判定には、macOS 固有の syscall テーブルを定義する。ELF 版の Linux syscall テーブルとは
番号が異なるため（例: Linux `socket` = 41, macOS `socket` = 97）、別テーブルとする。

**重複統合ルール**: ELF 版（タスク 0079 FR-3.3.2）と同一方針。集約キーを `Number` とする。
同じ `Number` を持つエントリが複数存在する場合は、`Source == "direct_svc_0x80"`
（直接 syscall 命令由来）を優先して 1 件に絞る。Mach-O コンテキストでは ELF の
`Source == ""`（`syscall` 命令由来）は出現しない。

#### FR-3.3.3: `SyscallAnalysis` の保存対象拡張

本タスクにより `SyscallAnalysis` の保存対象に「libSystem 経由の syscall が検出された
動的 Mach-O バイナリ」が追加される。

**タスク 0097 との統合**: タスク 0097 で追加された `svc #0x80` 直接検出の結果と、
本タスクの libSystem シンボル照合の結果は、同一の `SyscallAnalysis` に統合する。
Mach-O バイナリに対して以下の順序で解析し、結果をマージする。

1. `svc #0x80` 直接スキャン（タスク 0097 実装済み）
2. libSystem シンボル照合（本タスク）

マージ後の `DetectedSyscalls` は `Number` 昇順でソートする。

**現行コードとの互換性に関する注意**: 現行の `validator.go` では Mach-O svc スキャンが
`analyzeSyscalls()` の後に実行され、非 ELF バイナリの `SyscallAnalysis = nil` を
**上書き**する設計となっている。本タスクの実装では、libSystem キャッシュ処理と
svc スキャンの両結果を**マージ**するパターンに変更する必要がある。具体的には：
- libSystem キャッシュ照合 → libSystem 由来の `DetectedSyscalls` を生成
- svc スキャン → `direct_svc_0x80` の `DetectedSyscalls` を生成
- 両者をマージして `record.SyscallAnalysis` に設定

**`runner` 側での判定への影響**: タスク 0097 で実装済みの `runner` 側
`isNetworkViaBinaryAnalysis` ロジックは、`SyscallAnalysis` を参照して判定を行う。
libSystem 由来のネットワーク syscall（`socket` 等）が `DetectedSyscalls` に記録
されていれば、`runner` 側の判定ロジックがこれを検出し `NetworkDetected` を返す。
ただし、現行の `runner` は `SyscallAnalysis` から `svc #0x80` シグナルの有無のみを
確認しており（`syscallAnalysisHasSVCSignal`）、`IsNetwork` フラグに基づく判定は
行っていない。本タスクでは `runner` に `IsNetwork` エントリの確認ステップを追加する
必要がある（FR-3.6.2 参照）。

#### FR-3.3.4: `source` フィールドと `DeterminationMethod` の設定

libSystem シンボル照合によって検出された `SyscallInfo` には、`Source` フィールド
（値: `"libsystem_symbol_import"`）を設定する。これにより ELF 版の
`"libc_symbol_import"` と区別できる。

`DeterminationMethod` フィールド:
- **キャッシュヒット時**（`libsystem_kernel.dylib` が存在し関数単位解析キャッシュから
  照合した場合）: `"lib_cache_match"`
- **フォールバック時**（FR-3.4.2 のシンボル名単体一致）: `"symbol_name_match"`

`Location` フィールド: libSystem シンボル照合由来の `SyscallInfo` では対象バイナリ内の
アドレスが特定できないため `0` とする（ELF 版と同一方針）。

#### FR-3.3.5: `record --force` との整合性

ELF 版（タスク 0079 FR-3.3.6）と同一。`--force` フラグは libSystem キャッシュの
有効性判定に影響しない。

### 3.4 段階 1 フォールバック: シンボル名単体一致

#### FR-3.4.1: フォールバック条件

以下のいずれかの場合に、関数単位の syscall 解析を行わず、シンボル名による直接一致に
フォールバックする。

1. `libsystem_kernel.dylib` がファイルシステム上に存在しない（dyld shared cache 内）
2. libSystem 系ライブラリが `DynLibDeps` に含まれていない

#### FR-3.4.2: フォールバックの動作

macOS 固有の syscall テーブルに含まれるネットワーク関連 syscall のラッパー関数名
（`socket`, `connect`, `bind`, `listen`, `accept`, `sendto`, `recvfrom`,
`sendmsg`, `recvmsg` 等）のリストを定義する。`sendmmsg` / `recvmmsg` は Linux 固有の
syscall であり macOS には存在しないため含めない。

対象 Mach-O バイナリのインポートシンボルにこのリスト内の関数名が含まれている場合、
`SyscallInfo` を生成して `DetectedSyscalls` に追加する。

**フォールバック時の `SyscallInfo` フィールド**:
- `Number`: macOS syscall テーブルから取得した番号（例: `socket` = 97）
- `Name`: 関数名
- `IsNetwork`: syscall テーブルに基づく
- `Location`: `0`
- `DeterminationMethod`: `"symbol_name_match"`
- `Source`: `"libsystem_symbol_import"`

#### FR-3.4.3: フォールバック時のログ出力

フォールバックが適用された場合、`slog.Info` レベルでその旨を出力する。
ログメッセージには以下を含める。

- フォールバック理由（dyld shared cache 内 / DynLibDeps に libSystem なし）
- フォールバックにより検出された syscall 数

### 3.5 macOS syscall テーブル

#### FR-3.5.1: BSD syscall テーブルの定義

macOS arm64 の BSD syscall 番号テーブルを定義する。テーブルには少なくとも以下の
ネットワーク関連 syscall を含める。

| 関数名 | BSD syscall 番号 | `IsNetwork` |
|--------|-----------------|-------------|
| `socket` | 97 | true |
| `connect` | 98 | true |
| `accept` | 30 | true |
| `bind` | 104 | true |
| `listen` | 106 | true |
| `sendto` | 133 | true |
| `recvfrom` | 29 | true |
| `sendmsg` | 28 | true |
| `recvmsg` | 27 | true |
| `socketpair` | 135 | true |
| `shutdown` | 134 | true |
| `setsockopt` | 105 | true |
| `getsockopt` | 118 | true |
| `getpeername` | 31 | true |
| `getsockname` | 32 | true |

加えて、セキュリティ上重要な非ネットワーク syscall も含める。

| 関数名 | BSD syscall 番号 | `IsNetwork` |
|--------|-----------------|-------------|
| `mprotect` | 74 | false |
| `write` | 4 | false |
| `read` | 3 | false |
| `open` | 5 | false |
| `close` | 6 | false |

テーブルは `scripts/generate_syscall_table.py` を拡張して macOS 用テーブルも
自動生成できるようにするか、あるいは macOS 固有の Go ソースファイルに手動定義する。
いずれの方式かは設計フェーズで決定する。

**x16 レジスタのクラスプレフィックス**: macOS arm64 では `x16` に
`0x2000000 | <番号>` が設定される。テーブルには番号のみ（プレフィックスなし）を
格納し、関数単位解析（FR-3.2.3）での後方スキャン時にプレフィックスを除去して照合する。

#### FR-3.5.2: テーブルの使用箇所

macOS syscall テーブルは以下の 2 箇所で使用する。

1. **関数単位解析時の syscall 番号判定**: `libsystem_kernel.dylib` のラッパー関数から
   検出した syscall 番号が何であるかを特定し、`IsNetwork` フラグを設定する
2. **インポートシンボル照合時**: キャッシュ内の `syscall_wrappers` と
   対象バイナリのインポートシンボルを照合した後、`SyscallInfo` の `IsNetwork` を設定する

### 3.6 `runner` 側の影響

#### FR-3.6.1: 既存判定ロジックの再利用

タスク 0097 で実装した `runner` の `isNetworkViaBinaryAnalysis` は、`SyscallAnalysis`
を参照して `svc #0x80` シグナルの有無を確認し、さらに `SymbolAnalysis` の結果を
組み合わせて最終判定を行う。

本タスクにより `SyscallAnalysis.DetectedSyscalls` に libSystem 由来のネットワーク syscall
が追加されるが、`runner` の ELF 版 `convertSyscallResult` は `DetectedSyscalls` 内の
`IsNetwork == true` エントリの有無でネットワーク検出を判定しているため、Source の値に
依存しない。

**Mach-O 用の `convertSyscallResult` 追加または拡張が必要**: ELF 版は
`Number == -1`（不明 syscall）を高リスクとして扱うが、Mach-O 版では
`DeterminationMethod == "direct_svc_0x80"` のエントリを高リスクとして扱う
（タスク 0097 で定義済み）。この両方を処理できるよう `convertSyscallResult` を
汎用化するか、Mach-O 専用の変換ロジックを用意する。

#### FR-3.6.2: Mach-O `runner` で SyscallAnalysis キャッシュ内ネットワーク syscall を判定

`runner` が Mach-O バイナリの `SyscallAnalysis` を参照した際、以下の優先順位で
判定すること。

1. `DeterminationMethod == "direct_svc_0x80"` のエントリが存在 → `true, true`
   （高リスク確定、タスク 0097 と同一）
2. `IsNetwork == true` のエントリが存在 → `true, false`（ネットワーク検出）
3. 上記いずれも該当しない → `SymbolAnalysis` の結果に基づいて判定を続ける

## 4. 非機能要件

### NFR-4.1: パフォーマンス

#### NFR-4.1.1: キャッシュによる解析コスト削減

`libsystem_kernel.dylib` の解析は初回のみ（またはライブラリ更新時のみ）実行する。
通常の `record` 実行では、キャッシュファイルの読み込みとハッシュ照合のみで完結する。

#### NFR-4.1.2: フォールバック時のオーバーヘッド

段階 1 フォールバック（シンボル名単体一致）は追加のファイル I/O を伴わないため、
オーバーヘッドは無視できる水準（1ms 未満）に留める。

### NFR-4.2: セキュリティ

#### NFR-4.2.1: キャッシュの信頼性

ELF 版（タスク 0079 NFR-4.2.1）と同一。キャッシュファイル自体のハッシュ検証は
行わない。キャッシュは `record` 実行環境（信頼できる環境）で生成されるものとし、
ライブラリのハッシュ値との一致のみを有効性の根拠とする。

#### NFR-4.2.2: `verify` コマンドへの非影響

ELF 版（タスク 0079 NFR-4.2.2）と同一。キャッシュファイル自体は `verify` の
検証対象に含めない。

### NFR-4.3: スキーマバージョン

`CurrentSchemaVersion` の変更は不要。

**根拠**:

- **読み込み方向（旧記録 → 新コード）**: `Source` フィールドは既に `omitempty` 付きで
  存在する（タスク 0079 で追加済み）。本タスクで新しい `Source` 値
  (`"libsystem_symbol_import"`) を使用するが、フィールド自体は既存のため互換性に影響しない
- **書き込み方向（新コード → 新記録）**: libSystem 由来の `SyscallInfo` が
  `DetectedSyscalls` に追加されるが、`runner` はこれを `IsNetwork` フラグで判定するため
  `Source` 値を直接参照しない
- **概念的変更**: `SyscallAnalysis` の保存対象が「Mach-O の `svc #0x80` 直接検出」
  に加えて「libSystem シンボル照合」を含むようになる。これにより Mach-O バイナリの
  `SyscallAnalysis == nil` の意味が「svc スキャン実施済み・未検出」から「svc 未検出
  **かつ** libSystem インポート照合でもネットワーク syscall 未検出」に拡張される。
  ただし `runner` 側は `DetectedSyscalls` の内容で判定するため、nil セマンティクスの
  変化は判定結果に影響しない

### NFR-4.4: 保守性

#### NFR-4.4.1: ELF 版との共通化

libccache パッケージの既存コード（キャッシュ読み書き、スキーマ定義、ソート処理）を
最大限再利用する。Mach-O 固有の処理（`svc #0x80` デコード、`x16` 後方スキャン、
macOS syscall テーブル、シンボル名正規化）のみを追加する。

#### NFR-4.4.2: 関数サイズ閾値の定数化

256 バイトの閾値はコード内の名前付き定数として定義し、変更容易な構造とする。

## 5. 受け入れ条件

### AC-1: macOS syscall テーブル

- [ ] macOS arm64 BSD syscall のテーブルが定義されていること
- [ ] テーブルに少なくとも FR-3.5.1 に列挙されたネットワーク関連 syscall が含まれていること
- [ ] `socket`（97）、`connect`（98）等の番号が macOS のヘッダファイルと一致すること

### AC-2: ライブラリキャッシュの生成

- [ ] ファイルシステム上に `libsystem_kernel.dylib` が存在する環境で `record` を実行した際、
  `lib-cache/` 以下にキャッシュファイルが生成されること
- [ ] キャッシュファイルに `schema_version`, `lib_path`, `lib_hash`, `analyzed_at`,
  `syscall_wrappers` が含まれること
- [ ] `syscall_wrappers` の各エントリに `name`, `number` が含まれること
- [ ] `syscall_wrappers` が `number` 昇順・同一 `number` 内で `name` 昇順でソートされて
  いること
- [ ] 256 バイト超の関数がキャッシュに含まれないこと
- [ ] 複数の異なる syscall 番号を含む関数がキャッシュに含まれないこと
- [ ] `x16` レジスタのクラスプレフィックス（`0x2000000`）が除去された番号が記録されて
  いること

### AC-3: キャッシュの有効性判定と失敗時の挙動

- [ ] キャッシュファイルが存在し `schema_version` および `lib_hash` が一致する場合、
  ライブラリの再解析が行われないこと
- [ ] `schema_version` が不一致の場合、キャッシュが再生成されること
- [ ] `lib_hash` が不一致の場合、キャッシュが再生成されること
- [ ] キャッシュファイルが破損している場合、再解析が行われること
- [ ] キャッシュファイルの書き込み成功後にのみ記録ファイルが保存されること
- [ ] ライブラリファイルが読み取れない場合、`record` がエラーで終了し記録ファイルが
  保存されないこと
- [ ] ライブラリのエクスポートシンボル取得に失敗した場合、`record` がエラーで終了すること
- [ ] キャッシュファイルの書き込みに失敗した場合、`record` がエラーで終了すること

### AC-4: 段階 1 フォールバック

- [ ] `libsystem_kernel.dylib` がファイルシステム上に存在しない場合、シンボル名単体一致に
  フォールバックすること
- [ ] フォールバック時、対象バイナリのインポートシンボルに `socket` 等のネットワーク syscall
  ラッパー名が含まれている場合、`SyscallAnalysis.DetectedSyscalls` に該当エントリが
  追加されること
- [ ] フォールバック時の `DeterminationMethod` が `"symbol_name_match"` であること
- [ ] フォールバック時に `slog.Info` レベルのログが出力されること
- [ ] フォールバック時に `record` がエラーなく完了すること

### AC-5: インポートシンボルとキャッシュの照合

- [ ] Mach-O バイナリのインポートシンボル（`_socket` → `socket`）がキャッシュ内の
  `syscall_wrappers` と照合されること
- [ ] 照合によって検出された `SyscallInfo` の `Source` が `"libsystem_symbol_import"` で
  あること
- [ ] 照合によって検出された `SyscallInfo` の `Location` が `0` であること
- [ ] 同一 `Number` のエントリが `DetectedSyscalls` に重複して含まれないこと
- [ ] `svc #0x80` 直接検出（`Source: "direct_svc_0x80"`）と libSystem import 由来
  （`Source: "libsystem_symbol_import"`）の両方が検出された場合、直接検出が優先される
  こと

### AC-6: `runner` 側の判定

- [ ] `SyscallAnalysis` に libSystem 由来の `IsNetwork == true` エントリがある場合、
  `runner` が `true, false`（ネットワーク検出）を返すこと
- [ ] `SyscallAnalysis` に `direct_svc_0x80` エントリと libSystem 由来のネットワーク
  エントリが共存する場合、`runner` が `true, true`（高リスク確定）を返すこと
- [ ] libSystem 由来のエントリのみで全エントリが `IsNetwork == false` かつ
  `SymbolAnalysis = NoNetworkSymbols` の場合、`runner` が `false, false` を返すこと

### AC-7: 既存機能への非影響

- [ ] ELF バイナリの libc キャッシュフローが変更されないこと
- [ ] タスク 0097 の `svc #0x80` 検出フローが変更されないこと
- [ ] `make test` がすべてパスすること

## 6. テスト方針

### 6.1 関数単位解析のユニットテスト

| テストケース | 検証内容 |
|-------------|---------|
| `svc #0x80` を含み `x16` 後方スキャンで番号特定可能な関数が検出されること | 正常系 |
| 256 バイト超の関数が除外されること | サイズフィルタ |
| 複数の異なる syscall 番号を持つ関数が除外されること | 複数 syscall フィルタ |
| 同一 syscall 番号の `svc #0x80` を複数持つ関数は採用されること | 分岐パスの許容 |
| `svc #0x80` を含まない関数が除外されること | 非ラッパー除外 |
| `x16` の即値にクラスプレフィックス `0x2000000` が含まれる場合に正しく除去されること | macOS 固有 |

### 6.2 キャッシュ生成・読み込みのユニットテスト

ELF 版と同じテストパターン（FR-3.1.4 の 3 条件に対するヒット/ミス/破損テスト）。

### 6.3 インポートシンボル照合のユニットテスト

| テストケース | 検証内容 |
|-------------|---------|
| インポートシンボルがキャッシュに存在する場合に `SyscallInfo` が生成されること | 正常系 |
| `_socket` → `socket` のアンダースコア除去が正しく行われること | Mach-O 固有 |
| インポートシンボルがキャッシュに存在しない場合は無視されること | 非ラッパー除外 |
| 生成された `SyscallInfo` の `Source` が `"libsystem_symbol_import"` であること | source 確認 |
| 生成された `SyscallInfo` の `Location` が `0` であること | アドレス未確定 |

### 6.4 段階 1 フォールバックのテスト

| テストケース | 検証内容 |
|-------------|---------|
| ライブラリ非存在時にフォールバックが発動すること | 条件確認 |
| ネットワーク syscall ラッパー名がインポートシンボルに含まれる場合に検出されること | 正常系 |
| `DeterminationMethod` が `"symbol_name_match"` であること | フィールド確認 |
| ネットワーク syscall ラッパー名がインポートシンボルに含まれない場合は検出されないこと | 偽陽性なし |

### 6.5 統合テスト

統合テストは macOS arm64 環境でのみ実行する。Linux 環境では `t.Skip()` でスキップする。

| テストケース | 検証内容 | 前提条件 |
|-------------|---------|---------|
| macOS で `record` した動的 Mach-O バイナリの `SyscallAnalysis` にネットワーク syscall が検出されること | エンドツーエンド | macOS arm64 |
| フォールバック時も `SyscallAnalysis` にネットワーク syscall が検出されること | フォールバック確認 | macOS arm64 |
| `make test` がすべてパスすること | 既存機能への非影響確認 | なし |

## 7. 先行タスクとの関係

| 先行タスク | 本タスクとの関係 | 備考 |
|----------|----------------|------|
| 0073 (Mach-O ネットワーク検出) | シンボル解析基盤を再利用 | `normalizeSymbolName` 等 |
| 0079 (ELF libc syscall ラッパーキャッシュ) | キャッシュスキーマ・管理ロジックを再利用 | `libccache` パッケージ |
| 0095 (Mach-O フィーチャーパリティ) | 親タスク（FR-4.7） | — |
| 0096 (Mach-O LC_LOAD_DYLIB 整合性検証) | `DynLibDeps` が Mach-O で記録されることが前提 | ライブラリパス解決ロジックも再利用 |
| 0097 (Mach-O svc #0x80 キャッシュ統合) | `SyscallAnalysis` への Mach-O 信号保存が前提 | `runner` の SyscallAnalysis 参照ロジックも基盤 |

## 8. 設計上の決定事項

| 項目 | 決定内容 | 根拠 |
|------|----------|------|
| 解析対象ライブラリ | `libsystem_kernel.dylib`（`libSystem.B.dylib` ではなく） | syscall ラッパーの実体コードは `libsystem_kernel.dylib` に存在する。`libSystem.B.dylib` は再エクスポートシンボルのみ |
| キャッシュスキーマ | ELF 版と同一の `LibcCacheFile` 構造体を再利用 | 共通基盤の最大化 |
| `Source` フィールド値 | `"libsystem_symbol_import"` | ELF 版 `"libc_symbol_import"` と区別するため |
| 段階 1 フォールバック | シンボル名単体一致 | dyld shared cache 対応なしでも実用的な検出を提供 |
| macOS syscall テーブル | Linux テーブルとは別に定義 | BSD syscall 番号は Linux と異なる |
| スキーマバージョン変更 | 不要 | フィールド追加なし、既存フィールドの値追加のみ |
| dyld shared cache 対応 | 段階 2 に延期 | 実装コストが大きく、段階 1 フォールバックで実用上十分な検出が可能 |
| FR-4.6（mprotect 検出）のスコープ | 本タスクには含めない | 0095 実装計画ではタスク 0100 に「FR-4.6 前半」を含む記述があるが、`mprotect` の libSystem 経由検出は本タスクの syscall テーブルに `mprotect`（番号 74）を含めることで自然に対応される。引数レベルの `PROT_EXEC` 検出はタスク 0099 のスコープとする |
| フォールバックとキャッシュヒットの Source 値 | 両方とも `"libsystem_symbol_import"` | `DeterminationMethod`（`"lib_cache_match"` / `"symbol_name_match"`）で区別可能。Source はプラットフォーム（ELF/Mach-O）の識別に使用し、検出手法の詳細は DeterminationMethod で表現する |
