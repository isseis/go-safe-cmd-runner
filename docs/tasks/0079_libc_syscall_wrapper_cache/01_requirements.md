# libc システムコールラッパー関数キャッシュ 要件定義書

## 1. 概要

### 1.1 背景

タスク 0070/0072 では、ELF バイナリ内の `syscall` 命令を静的解析してシステムコール呼び出しを検出する機能を実装した。しかし `/usr/bin/mkdir` のような動的リンクバイナリでは、`syscall` 命令はバイナリ本体ではなく依存ライブラリ（libc.so.6）の内部に存在する。このため、バイナリ本体の静的解析のみでは `detected_syscalls: []` となり、実際のシステムコール呼び出しが検出されない。

この問題を解決するため、libc のエクスポート関数を関数単位で静的解析し、「エクスポート関数名 → 実際に呼び出される syscall 番号」のマッピングをキャッシュする機能を導入する。解析には既存の `elfanalyzer` の syscall 解析ロジック（Pass 1: `syscall` 命令検出）を再利用する。

libc は頻繁に更新されるものではないため、ライブラリファイルのハッシュ値に基づくキャッシュを活用することで、解析コストを最小化できる。

### 1.2 目的

- 動的リンクバイナリが libc 経由で呼び出すシステムコールを、インポートシンボル解析によって検出できるようにする
- libc のシステムコールラッパー解析結果をキャッシュし、`record` 実行のたびに libc を再解析するコストを避ける
- ハードコードされた対応表を持たず、libc の実際の実装から syscall 番号を実測することで、glibc バージョン差異（例: `open` → `openat` への委譲）に対応する
- 既存の `syscall` 命令ベースの検出（`SyscallAnalysis`）との整合性を保ちつつ、情報源（`source` フィールド）で区別する

### 1.3 スコープ

- **対象**: 動的リンクされた ELF バイナリ（`dyn_lib_deps` に libc が記録されているもの）
- **対象ライブラリ**: libc のみ（`dyn_lib_deps` に記録された libc）
- **ラッパー関数の特定方法**: libc のエクスポート関数を関数単位で逆アセンブルし、`syscall` 命令を含み、かつ関数サイズが閾値以下のものを対象とする
- **キャッシュ保存場所**: 記録ファイル保存ディレクトリ直下の `lib-cache/` サブディレクトリ
- **キャッシュトリガー**: `record` コマンド実行時に自動的に生成・参照する
- **対象外**: libc 以外のライブラリ（`libselinux`, `libpcre2` 等）
- **対象外**: 静的 ELF バイナリ（`SyscallAnalysis` ベースの既存フローを維持）
- **対象外**: スクリプトファイル、Mach-O バイナリ
- **対象外**: `verify` コマンドの検証対象への追加（ハッシュ値検証のみで十分）

## 2. 用語定義

| 用語 | 定義 |
|------|------|
| システムコールラッパー関数 | libc のエクスポート関数のうち、関数本体内に `syscall` 命令を含み、かつ関数サイズが閾値以下のもの。`mkdir`, `socket`, `write` 等 |
| ライブラリキャッシュ | ライブラリファイルのパスをファイル名に、スキーマバージョンとハッシュ値を有効性の判定キーとして、システムコールラッパー関数の一覧（関数名 → 実測 syscall 番号）を保存したファイル |
| インポートシンボル | ELF バイナリの `.dynsym` に記録された未解決シンボル（`U` 型）。動的リンク時に共有ライブラリから提供される |
| 関数単位解析 | libc の `.dynsym` エクスポートシンボルから各関数のアドレス範囲を特定し、その範囲内の `syscall` 命令を検出する処理 |
| `source` フィールド | `SyscallInfo` に付与される、検出手法を示す文字列。`syscall` 命令由来は省略、libc シンボル照合由来は `"libc_symbol_import"`。ライブラリキャッシュファイルの `syscall_wrappers` には付与しない（フィールド名で自明なため） |

## 3. 機能要件

### 3.1 ライブラリキャッシュファイル

#### FR-3.1.1: キャッシュファイルの内容

ライブラリキャッシュファイルは以下の情報を JSON 形式で保持する。

```json
{
  "schema_version": 1,
  "lib_path": "/usr/lib/x86_64-linux-gnu/libc.so.6",
  "lib_hash": "sha256:d8db87...",
  "analyzed_at": "2026-03-16T00:00:00Z",
  "syscall_wrappers": [
    { "name": "write",  "number": 1   },
    { "name": "socket", "number": 41  },
    { "name": "mkdir",  "number": 83  }
  ]
}
```

`syscall_wrappers` は `number` 昇順、同一 `number` 内では `name` 昇順の複合キーでソートして保存する。決定論的な出力とすることで diff が読みやすくなる。同一 `number` を複数エントリが持つ場合も `name` を第 2 キーとすることで入力順に依存しない完全決定論的な順序を保証する。

`syscall_wrappers` の各エントリが持つ `number` は、ハードコードされた対応表ではなく libc の実際の実装から実測した値である。例えば glibc の `open()` が内部で `openat` (257) を呼び出す実装になっている場合、`{ "name": "open", "number": 257 }` として記録される。

#### FR-3.1.2: キャッシュファイルの命名規則

キャッシュファイル名のエンコード入力には `DynLibDeps.Libs[].Path` の値をそのまま使用する。この値は `dynlibanalysis.LibraryResolver` が `filepath.EvalSymlinks + filepath.Clean` を適用して正規化した実体ファイルパスであり（`resolver.go:39`, `resolver.go:87`）、シンボリックリンクが解決済みの状態である。

複数バージョンの libc が共存する環境でも衝突しないよう、`internal/filevalidator/pathencoding` パッケージが提供する既存のファイル名エンコーディング方式（[hash-file-naming-adr.ja.md](../../dev/architecture_design/hash-file-naming-adr.ja.md) 参照）を使用してこのパスをエンコードする。

#### FR-3.1.3: キャッシュファイルの保存場所

キャッシュファイルは、`record` コマンドが使用するハッシュディレクトリ（`-hash-dir` フラグで指定、未指定時はビルド時に埋め込まれた `DefaultHashDirectory`）直下の `lib-cache/` サブディレクトリに保存する。

```
<hash-dir>/
  <encoded-mkdir>          ← /usr/bin/mkdir の記録ファイル
  <encoded-libc.so.6>      ← libc.so.6 の記録ファイル（record に libc を渡した場合）
  lib-cache/
    <encoded-libc.so.6>    ← libc.so.6 のキャッシュファイル
```

記録ファイルとキャッシュファイルを別ディレクトリに分離することで、`record /usr/lib/.../libc.so.6` を実行した際の記録ファイルとキャッシュファイルの衝突を防ぐ。

#### FR-3.1.4: キャッシュの有効性判定

キャッシュファイルが存在し、かつ以下の条件をすべて満たす場合にキャッシュを有効とみなす。

1. JSON パースが成功すること
2. `schema_version` がコード側の `LibcCacheSchemaVersion` 定数と一致すること
3. `lib_hash` が現在のライブラリファイルのハッシュ値と一致すること

`schema_version` が不一致の場合（スキーマ変更時）、または `lib_hash` が不一致の場合（ライブラリ更新時）は再解析を行い、キャッシュを上書きする。

#### FR-3.1.5: libc の特定ルール

`dyn_lib_deps` の `Libs[]` を走査し、`SOName` が `"libc.so."` で始まるエントリを libc とみなす（`strings.HasPrefix(soname, "libc.so.")`）。

この前方一致により `libc.so.6`（現行 glibc）および将来導入されうる `libc.so.7` 等に自動対応する。`"libc.so."` にドットを含めることで `libsomething.so.1` のような無関係なライブラリへの誤検出を防ぐ。

musl libc（`libc.musl-x86_64.so.1` 等）は初期実装の対象外とする。musl 対応は ARM64 拡張と同じタイミングで追加する。

複数エントリが条件に一致した場合（同一バイナリが複数バージョンの libc にリンクしている等、通常は起こらない）、それぞれについてキャッシュを参照・生成する。

### 3.2 libc の関数単位解析

#### FR-3.2.1: エクスポート関数の列挙

libc の `.dynsym` セクションからエクスポートシンボル（定義済みシンボル、関数型）を列挙する。シンボルのアドレスとサイズを取得し、関数ごとの解析範囲を決定する。

#### FR-3.2.2: 関数サイズによるフィルタリング

関数サイズが **256 バイト超** のものは解析対象から除外する。

この閾値は実測に基づく。現在の glibc (Ubuntu x86_64) において、`syscall` 命令を含むエクスポート関数のサイズ分布を計測した結果：

- 典型的なラッパー関数（`mkdir`, `socket`, `bind` 等）: 16〜64 バイト
- キャンセルポイントを持つ関数（`read`, `write`, `connect` 等）: 160〜192 バイト
- 256 バイト以下の関数: 234 件 / 346 件（`syscall` 命令を含む関数全体）

`open64`, `openat64` 等は計測上限（256 バイト）に達しており実サイズは不明だが、これらは複雑な実装を持つことが多く、単純ラッパーとは異なる。256 バイトを閾値とすることで、セキュリティ上重要なネットワーク系関数（`sendto`, `recvfrom`: 192 バイト）や入出力関数（`read`, `write`: 160 バイト）を網羅しつつ、明らかに複雑な関数を除外できる。

#### FR-3.2.3: 関数単位の syscall 命令検出

サイズフィルタを通過した関数について、既存の `elfanalyzer` Pass 1 ロジック（`syscall` 命令の検出と直前の `mov $N, %eax` からの syscall 番号読み取り）を関数のアドレス範囲に適用する。

1つの関数が複数の `syscall` 命令を含む場合（例: スレッドキャンセル対応のパス分岐）、関数全体をスキャンしてすべての syscall 番号を収集する。収集した番号がすべて同一であればその番号を採用する。1つでも異なる syscall 番号が検出された場合は、その関数をキャッシュに含めない（単純ラッパーとみなせないため）。

#### FR-3.2.4: アーキテクチャの対応範囲

初期実装では x86_64 のみを対象とする（タスク 0070 の syscall 解析と同じ対応範囲）。ARM64 への拡張は将来のタスクで対応する。非対応アーキテクチャの場合は libc 解析をスキップし、キャッシュを生成しない。

### 3.3 `record` コマンドの拡張

#### FR-3.3.1: キャッシュの参照と生成

`record` コマンド実行時、`dyn_lib_deps` に libc が含まれている場合、以下を行う。

1. libc の実体ファイルパスとハッシュ値を `dyn_lib_deps` から取得する
2. 対応するキャッシュファイルが存在し、FR-3.1.4 の 3 条件（JSON パース成功・`schema_version` 一致・`lib_hash` 一致）をすべて満たす場合、キャッシュを読み込む
3. キャッシュが存在しない、または上記条件のいずれかを満たさない場合、libc を解析してキャッシュを生成する

**保存順序**

キャッシュファイル（`lib-cache/`）の書き込みが成功した後にのみ、記録ファイル（`hashes/`）を保存する。この順序により、記録ファイルが存在する場合は必ずキャッシュも存在することが保証される。逆順（記録ファイル先行）は禁止する。

キャッシュファイルが書き込まれた後に記録ファイルの保存が失敗した場合、キャッシュファイルはそのまま残る。キャッシュの有効性はライブラリのハッシュ値で判定されるため、次回 `record` 実行時に正しく再利用または再生成される。

**既存処理フローへの影響（実装上の必須変更）**

現在の `cmd/record/main.go` の処理順序は以下の通りである：

```
SaveRecord（記録ファイル保存）→ analyzeFile（syscall 解析 → SaveSyscallAnalysis）
```

本タスクが要求する保存順序（キャッシュ → 記録ファイル）を実現するため、すべての解析処理を `store.Update()` コールバック内に統合する：

```
store.Update() コールバック内:
  dynlibAnalyzer.Analyze() → libc キャッシュ参照・生成 → SyscallAnalyzer 実行 → record 設定
store.Save() → 記録ファイル保存（コールバック成功後のみ）
```

この変更に伴い以下の実装変更が必要となる：

- `Validator.updateAnalysisRecord()` のコールバック内に libc キャッシュ処理・インポートシンボル照合・syscall 解析を統合する
- `cmd/record/main.go` の `processFiles` から独立した `analyzeFile()` 呼び出しを削除する

**意図的な挙動変更（Warning → Fatal）**

現行実装では `analyzeFile()` 失敗時（ELF パースエラー等）も記録ファイルは保存され、syscall 解析なしの記録が永続化される（警告のみ）。新設計では解析が `store.Update()` コールバック内に移動するため、解析失敗時はコールバックがエラーを返し `store.Save()` が呼ばれず、記録ファイルが保存されない。

これは意図的な破壊的変更である。syscall 解析に失敗したまま記録ファイルを保存すると `SyscallAnalysis` が空のまま永続化され、検証時に過小評価を引き起こす。解析失敗は「記録不能」として扱い、利用者に明示的なエラーを返す方が安全である（`dynlibAnalyzer` の既存の扱いと同じレベル）。

**失敗時の挙動**（原則 fatal: `record` をエラーで終了し、記録ファイルを保存しない。非対応アーキテクチャのみ例外として継続）:

| ケース | 挙動 |
|--------|------|
| キャッシュファイルが破損（JSON パース失敗等） | 再解析を試みる。再解析も失敗した場合はエラーで終了 |
| libc ファイルが読み取れない（権限不足、ファイル不存在） | エラーで終了 |
| libc のエクスポートシンボル取得失敗 | エラーで終了 |
| libc の解析中に予期しないエラー | エラーで終了 |
| キャッシュファイルの書き込み失敗 | エラーで終了 |
| 非対応アーキテクチャ（x86_64 以外） | libc 解析をスキップし、`SyscallAnalysis` に libc 由来エントリなしで継続（エラーなし） |

非対応アーキテクチャのみ継続を許容する。これは x86_64 以外での `record` 実行を妨げないための例外であり、セキュリティ姿勢の例外ではない。

#### FR-3.3.2: インポートシンボルとキャッシュの照合

対象バイナリの `.dynsym` からインポートシンボル一覧を取得し、キャッシュ内の `syscall_wrappers` と関数名で照合する。一致したシンボルを `SyscallInfo` として `SyscallAnalysis.DetectedSyscalls` に追加する。

**重複統合ルール**

集約キーを `Number` とする。同じ `Number` を持つエントリが複数存在する場合は、`Source == ""`（直接 syscall 命令由来）を優先して 1 件に絞る。`Source == ""` のエントリが存在しない場合は `Source == "libc_symbol_import"` のエントリを採用する。

この設計の根拠: 目的はネットワーク関連 syscall が呼び出されているかどうかの判定であり、同一 syscall 番号に対して信頼性の高い根拠が 1 つあれば十分である。直接検出（`Source == ""`）の `Name` は `GetSyscallName()` で解決されるが、テーブルに登録されていない番号では空文字になる。これは libc import 由来でも同様に起こり得るため（libc キャッシュの `WrapperEntry.Name` はテーブル外の関数名を持つ場合がある）、`mergeSyscallInfos` における direct 優先によって Name 解決の質が低下することはない。なお `Name` が空であっても `Number` と `IsNetwork` は正しく設定されるため、セキュリティ判定への影響はない。

#### FR-3.3.3: `SyscallAnalysis` の保存対象拡張（契約変更）

本タスクにより `SyscallAnalysis` の保存対象が変更される。

**変更前**: 静的 ELF バイナリのみ（`schema.go` の `// Only present for static ELF binaries that have been analyzed.` コメント）

**変更後**: 静的 ELF バイナリに加え、libc 経由でシステムコールを呼び出す動的 ELF バイナリも対象となる

この変更に伴い以下を実施する：

- `schema.go` の `SyscallAnalysis` フィールドのコメントを「静的 ELF バイナリのみ」から「静的 ELF バイナリ、および libc 経由の syscall が検出された動的 ELF バイナリ」に更新する
- `runner` 側の `lookupSyscallAnalysis` は `ErrNoSyscallAnalysis` をキャッシュミスとして扱いフォールバックする既存の動作を変更しない

**`runner` 側での libc import 由来 `SyscallInfo` の扱い**

`standard_analyzer.go` の動的バイナリ解析フロー（`AnalyzeNetworkSymbols`）では、`.dynsym` にネットワークシンボルが存在しない場合（`NoNetworkSymbols`）、CGO バイナリへのフォールバックとして `lookupSyscallAnalysis` が呼び出される（`standard_analyzer.go:236-243`）。

本タスクにより動的バイナリの `SyscallAnalysis` に libc import 由来のネットワーク syscall（`socket`, `connect` 等）が記録されている場合、このフォールバックパスで `convertSyscallResult` が `HasNetworkSyscalls: true` を検出し `NetworkDetected` を返す。

これは**意図した動作**である。libc import 由来の `socket` 等が `SyscallAnalysis` に記録されているバイナリはネットワークを使用する可能性があり、`NetworkDetected` を返すことは正しいセキュリティ判定となる。`runner` 側の変更は不要。

#### FR-3.3.4: `source` フィールドの設定

libc シンボル照合によって検出されたシステムコール情報の `SyscallInfo` には、`source` フィールド（値: `"libc_symbol_import"`）を設定する。`syscall` 命令から検出されたものは `source` なし（既存の動作を維持）。

`Location` フィールド: libc シンボル照合由来の `SyscallInfo` では対象バイナリ内のアドレスが特定できないため `0` とする。

#### FR-3.3.5: 静的バイナリの既存フロー維持

静的 ELF バイナリは `SyscallAnalysis` ベースの既存フローを維持する。変更なし。

#### FR-3.3.6: `record --force` との整合性

`--force` フラグは libc キャッシュの有効性判定に影響しない。`--force` 実行時も通常フローと同じく `lib_hash` の一致・不一致でキャッシュのヒット・再生成を判定する。対象バイナリの `SyscallAnalysis` は `--force` の場合も通常通り新しい値で上書きする。

### 3.4 `SyscallInfo` の拡張

#### FR-3.4.1: `source` フィールドの追加

`common.SyscallInfo` に `Source string` フィールド（JSON: `"source,omitempty"`）を追加する。

```go
type SyscallInfo struct {
    Number              int    `json:"number"`
    Name                string `json:"name,omitempty"`
    IsNetwork           bool   `json:"is_network"`
    Location            uint64 `json:"location"`
    DeterminationMethod string `json:"determination_method"`
    Source              string `json:"source,omitempty"` // 追加
}
```

`Source` の値:
- `""` (空文字列、省略): `syscall` 命令から検出（既存の動作を維持）
- `"libc_symbol_import"`: libc のインポートシンボル照合によって検出

## 4. 非機能要件

### 4.1 パフォーマンス

#### NFR-4.1.1: キャッシュによる解析コスト削減

libc の解析は初回のみ（またはライブラリ更新時のみ）実行する。通常の `record` 実行では、キャッシュファイルの読み込みとハッシュ照合のみで完結する。

#### NFR-4.1.2: サイズフィルタによる解析対象の削減

256 バイト超の関数を除外することで、libc 全体のエクスポート関数（`syscall` 命令を含むもの 346 件）のうち約 1/3（112 件）を解析対象から除外できる。

### 4.2 セキュリティ

#### NFR-4.2.1: キャッシュの信頼性

キャッシュファイル自体のハッシュ検証は行わない。キャッシュは `record` 実行環境（信頼できる環境）で生成されるものとし、`dyn_lib_deps` に記録されたライブラリのハッシュ値との一致のみを有効性の根拠とする。

#### NFR-4.2.2: `verify` コマンドへの非影響

`verify` コマンドは `SyscallAnalysis` のハッシュ値検証のみを行う。キャッシュファイル自体は `verify` の検証対象に含めない。

### 4.3 スキーマバージョン

`CurrentSchemaVersion` の変更は不要。根拠は以下の通り。

**読み込み方向（旧記録 → 新コード）**: `Source` フィールドは `omitempty` 付きで追加されるため、`Source` を持たない既存の記録ファイルは正常に読み込める。Go の `encoding/json` は未知フィールドをデフォルトで無視し、存在しないフィールドはゼロ値（空文字列）になる。`runner` および `verify` は `Source` フィールドを参照しないため、動作に影響しない。

**書き込み方向（新コード → 新記録）**: `Source` が空文字列のエントリは `omitempty` により JSON 出力から省略されるため、既存エントリのフォーマットは変わらない。

**概念的変更（静的バイナリのみ → 動的バイナリも含む）について**: `SyscallAnalysis` の保存対象が拡大されるが、`runner` 側はこのフィールドの有無を `nil` チェックのみで判断しており（`ErrNoSyscallAnalysis`）、保存対象の種別を区別しない。動的バイナリに `SyscallAnalysis` が存在しても既存の読み込みロジックは正しく動作する。

### 4.4 保守性

#### NFR-4.4.1: 関数サイズ閾値の定数化

256 バイトの閾値はコード内の名前付き定数として定義し、変更容易な構造とする。

#### NFR-4.4.2: 将来の拡張性

ARM64 や他のアーキテクチャへの拡張を想定し、関数単位解析はアーキテクチャ別のデコーダーを使用する既存構造に沿って実装する。

## 5. 受け入れ条件

### AC-1: `SyscallInfo` の拡張

- [ ] `common.SyscallInfo` に `Source string` フィールド（`json:"source,omitempty"`）が追加されていること
- [ ] 既存の `SyscallInfo` を使用するテストが引き続きパスすること

### AC-2: ライブラリキャッシュの生成

- [ ] `record` 実行時、対象バイナリの `dyn_lib_deps` に libc が含まれている場合にキャッシュファイルが `lib-cache/` 以下に生成されること
- [ ] キャッシュファイルに `schema_version`, `lib_path`, `lib_hash`, `analyzed_at`, `syscall_wrappers` が含まれること
- [ ] `syscall_wrappers` の各エントリに `name`, `number` が含まれること
- [ ] `syscall_wrappers` が `number` 昇順・同一 `number` 内で `name` 昇順でソートされていること
- [ ] 256 バイト超の関数がキャッシュに含まれないこと
- [ ] 複数の異なる syscall 番号を含む関数がキャッシュに含まれないこと

### AC-3: キャッシュの有効性判定と失敗時の挙動

- [ ] キャッシュファイルが存在し `schema_version` および `lib_hash` が一致する場合、libc の再解析が行われないこと
- [ ] `schema_version` が不一致の場合（スキーマ変更時）、キャッシュが再生成されること
- [ ] `lib_hash` が不一致の場合（libc 更新時）、キャッシュが再生成されること
- [ ] キャッシュファイルが破損している場合、再解析が行われること
- [ ] キャッシュファイルの書き込み成功後にのみ記録ファイルが保存されること（逆順にならないこと）
- [ ] libc ファイルが読み取れない場合、`record` がエラーで終了し記録ファイルが保存されないこと
- [ ] libc のエクスポートシンボル取得に失敗した場合、`record` がエラーで終了すること
- [ ] キャッシュファイルの書き込みに失敗した場合、`record` がエラーで終了すること
- [ ] 非対応アーキテクチャ（x86_64 以外）の場合、libc 解析をスキップして `record` が継続すること

### AC-4: インポートシンボルとキャッシュの照合

テスト対象バイナリ: GCC でコンパイルした専用の動的リンクバイナリ（`mkdir` syscall を呼ぶ最小 C プログラム）。GCC が利用可能な x86_64 環境でのみ実行する（GCC 非利用環境では `t.Skip()`）。

- [ ] GCC でビルドした動的リンクバイナリを `record` した際、`SyscallAnalysis.DetectedSyscalls` に `mkdir`（syscall 番号 83）が含まれること
- [ ] 照合によって検出された `SyscallInfo` の `Source` が `"libc_symbol_import"` であること
- [ ] 照合によって検出された `SyscallInfo` の `Location` が `0` であること
- [ ] 同一 `Number` のエントリが `DetectedSyscalls` に重複して含まれないこと
- [ ] `syscall` 命令由来（`Source` 空）と libc import 由来（`Source: "libc_symbol_import"`）の両方が検出された場合、`Source` 空のエントリが採用されること

### AC-5: 既存機能への非影響

- [ ] 静的 ELF バイナリの `SyscallAnalysis` ベースのフローが変更されないこと
- [ ] `syscall` 命令から検出された `SyscallInfo` の `Source` が空文字列（省略）であること
- [ ] `make test` がすべてパスすること

## 6. テスト方針

### 6.1 関数単位解析のユニットテスト

| テストケース | 検証内容 |
|-------------|---------|
| `syscall` 命令を含む小さな関数（≤256B）が検出されること | 正常系 |
| 256 バイト超の関数が除外されること | サイズフィルタ |
| 複数の異なる syscall 番号を持つ関数が除外されること | 複数 syscall フィルタ |
| 同一 syscall 番号の `syscall` 命令を複数持つ関数は採用されること | 分岐パスの許容 |
| `syscall` 命令を含まない関数が除外されること | 非ラッパー除外 |

### 6.2 キャッシュ生成・読み込みのユニットテスト

| テストケース | 検証内容 |
|-------------|---------|
| キャッシュ未存在時に解析・生成されること | 初回実行 |
| ハッシュ一致時にキャッシュが再利用されること | キャッシュヒット |
| ハッシュ不一致時にキャッシュが再生成されること | libc 更新時 |
| キャッシュファイルが破損している場合に再解析されること | エラー耐性 |
| `syscall_wrappers` が `number` 昇順・同一 `number` 内で `name` 昇順でソートされていること | 決定論的出力 |

### 6.3 インポートシンボル照合のユニットテスト

| テストケース | 検証内容 |
|-------------|---------|
| インポートシンボルがキャッシュに存在する場合に `SyscallInfo` が生成されること | 正常系 |
| インポートシンボルがキャッシュに存在しない場合は無視されること | 非ラッパー除外 |
| 生成された `SyscallInfo` の `Source` が `"libc_symbol_import"` であること | `source` フィールド確認 |
| 生成された `SyscallInfo` の `Location` が `0` であること | アドレス未確定 |

### 6.4 統合テスト

統合テストは `//go:build integration` タグで分離し、既存パターン（`syscall_analyzer_integration_test.go`）に倣う。テスト用バイナリは `/usr/bin/mkdir` 等のシステムバイナリに依存せず、テスト実行時に GCC でオンデマンド生成する。GCC が利用できない環境では `t.Skip()` でスキップする。

```c
// テスト用動的リンクバイナリの例（mkdir syscall を呼ぶ最小 C プログラム）
#include <sys/stat.h>
int main() { mkdir("/tmp/test", 0755); return 0; }
// gcc -o test_mkdir.elf test_mkdir.c  （動的リンク、デフォルト）
```

| テストケース | 検証内容 | 前提条件 |
|-------------|---------|---------|
| GCC でビルドした動的リンクバイナリを `record` した際に `mkdir` syscall が検出されること | エンドツーエンド | GCC が利用可能、x86_64 |
| `source: "libc_symbol_import"` の `SyscallInfo` が `SyscallAnalysis.DetectedSyscalls` に含まれること | 記録内容の確認 | 同上 |
| `make test` がすべてパスすること | 既存機能への非影響確認 | なし |

## 7. 先行タスクとの関係

| 項目 | タスク 0070/0072 | タスク 0074 | タスク 0076 | 本タスク（0079）|
|------|-----------------|------------|------------|----------------|
| 解析対象 | バイナリ本体の `syscall` 命令 | `DT_NEEDED` 依存ライブラリ | `.dynsym` ネットワークシンボル | libc のエクスポート関数（関数単位） |
| キャッシュ先 | `SyscallAnalysis` フィールド | `DynLibDeps` フィールド | `SymbolAnalysis` フィールド | `lib-cache/` ディレクトリ（独立ファイル） |
| 実行タイミング | `record` 時保存・`runner` 時読み込み | `record` 時保存・`runner` 時検証 | `record` 時保存・`runner` 時読み込み | `record` 時保存・参照 |
| 目的 | 静的バイナリの syscall 検出 | 依存ライブラリ整合性保証 | 動的バイナリのネットワーク検出キャッシュ化 | 動的バイナリの syscall 検出補完 |
