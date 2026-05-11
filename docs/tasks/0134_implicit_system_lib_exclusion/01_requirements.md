# 暗黙的システムライブラリの再帰的解析除外 要件定義書

## 1. 概要

### 1.1 背景

タスク 0123（動的リンクライブラリの再帰的システムコール解析）により、`record`
コマンドは実行ファイルが動的リンクしているライブラリ自体も解析対象とし、libc
等を経由したシステムコール利用を検出できるようになった。

このタスクでは syscall ラッパライブラリ（libc, libpthread, libdl, librt,
libgcc_s, ld-linux, linux-vdso）を `IsSyscallWrapperLibrary` により解析対象から
除外している。これは「これらは OS ABI レイヤーであり、`syscall` 命令を直接ラップ
する」ことが理由であった。

しかし実運用において、syscall ラッパ以外にも、多くの実行ファイルが**暗黙的に**
リンクするライブラリが存在し、現行の解析ではそれらが false positive を生成する。

**具体例（`/usr/bin/cp` のレコード出力）:**

```json
{
  "name": "dlsym",
  "source_path": "/usr/lib/x86_64-linux-gnu/libselinux.so.1"
}
```

- `cp` 自体は `dlsym` を呼び出していない
- libselinux.so.1 が libc から `dlsym` を import している
- libselinux はライブラリレベル解析の対象となり、その import シンボル（libc 由来）が
  `DetectedSymbols` に記録される
- 結果として `cp` のレコードに「dlsym 経由の動的ロード」が記録される → **false positive**

このようなライブラリは以下の特性を持つ。

- 多くの実行ファイルが暗黙的にリンクする（直接利用していなくてもリンクされる）
- 主な役割は OS / カーネル機能との統合（SELinux, ACL, capabilities 等）
- ネットワーク機能を持たない、または持ったとしても kernel /sys インターフェース等の
  非ネットワーク経路に限られる

### 1.2 目的

- libselinux のような「暗黙的にリンクされるシステム統合ライブラリ」を、動的リンクライブラリの
  再帰的解析の対象から除外することで `DetectedSymbols` / `SyscallAnalysis` における
  false positive を削減する
- 同時に、ネットワーク API 呼び出しの検出能力（精度）を損なわない
- 除外対象はハードコードのリストで管理し、最小限度に留める

### 1.3 スコープ

#### 対象

- 暗黙的システムライブラリの除外リスト定義（`internal/security/binaryanalyzer` 配下）
- `internal/filevalidator/validator.go` の `analyzeLibraries` における除外判定への
  反映
- 除外対象ライブラリの選定基準のドキュメント化

#### 対象外

- 除外対象ライブラリのファイル完全性検証への影響（ライブラリ自身は引き続き
  `DynLibDeps` に記録され、ハッシュ検証は実施される）
- 設定ファイルによる除外対象のカスタマイズ（ハードコードのみ）
- libssl, libcurl, libxml2 等の「中間アプリケーションライブラリ」の除外
  （task 0123 §6.2.4 で扱う別課題）
- 関数レベル到達可能性解析による精度改善（task 0123 §8、本タスクのスコープ外）

---

## 2. 用語定義

| 用語 | 定義 |
|------|------|
| 暗黙的システムライブラリ | 多くの実行ファイルが意図せずリンクしているが、実行ファイルの直接的な機能要件としては利用されていないシステム統合ライブラリ |
| syscall ラッパライブラリ | task 0123 FR-3.1.1 が定義するライブラリ（libc, libpthread, libdl, librt, libgcc_s, ld-linux, linux-vdso）。`IsSyscallWrapperLibrary` で判定する |
| ライブラリレベル解析 | task 0123 で導入された、依存ライブラリの機械語・dynsym シンボルを再帰的に解析する処理（`Validator.analyzeLibraries`） |
| ネットワーク API | BSD socket API（socket/connect/bind/accept/send/recv 等）、DNS 解決 API（getaddrinfo/gethostbyname 等）、Unix ドメインソケット API |

---

## 3. 機能要件

### FR-1: 暗黙的システムライブラリの除外リスト

#### FR-1.1: 除外リストの定義

以下のライブラリを「暗黙的システムライブラリ」として、ライブラリレベル解析の対象から
除外する。

照合方式は task 0123 FR-3.1.1 と同じ「SOName プレフィックス + 区切り文字
（`.`, `-`, 数字）」を用いる。

| SOName プレフィックス | 対応ライブラリ | 除外理由 |
|---|---|---|
| `libselinux` | SELinux ユーザー空間ライブラリ | 多くの実行ファイルが暗黙的にリンクするが、内部で `dlsym`/`open` 等を呼ぶことが、リンク元実行ファイルの実動作と乖離する |

> 初期リストは libselinux のみとする。他の候補（libacl, libattr, libcap, libaudit
> 等）は実運用における false positive 発生頻度を観測した上で、将来タスクで追加する
> こととし、本タスクのスコープ外とする。

#### FR-1.2: 除外判定の統合方針

除外判定は既存の syscall ラッパ判定（`IsSyscallWrapperLibrary`）と同じ呼び出し箇所
（`validator.go::analyzeLibraries`）で行う。

API 設計（既存リストへの追記、既存関数への並列リスト追加、新規関数の導入 等）の
選択はアーキテクチャ設計で決定する。

### FR-2: ネットワーク API 検出経路の維持

除外対象ライブラリの追加によって、以下のネットワーク API 検出経路の精度が低下
しないこと。

| 検出経路 | 検出方法 | 該当タスク |
|---|---|---|
| libc wrapper 経由のネットワーク syscall | 実行ファイル本体の `.dynsym` UNDEF シンボル解析 | 0069, 0106 |
| 機械語 `syscall`/`svc` 命令による直接呼び出し | 実行ファイル本体・除外対象外ライブラリの機械語スキャン | 0070, 0072 |
| 動的ライブラリ読み込み（`dlopen`, `dlsym`, `dlvsym`） | `DynamicLoadSymbols` の検出（実行ファイル本体および除外対象外ライブラリの import） | 0069 |
| 動的コード書き込み・実行（`mprotect` + `PROT_EXEC`、`pkey_mprotect`） | 実行ファイル本体・除外対象外ライブラリの機械語解析 | 0078, 0081, 0125 |

### FR-3: 除外対象選定基準の明文化

除外対象ライブラリの選定基準を `syscall_wrapper_libs.go` 付近のコメントまたは関連
ドキュメントに明記する。

**選定基準（すべて満たすこと）:**

1. **広範な暗黙リンク**: 多くの実行ファイル（例: GNU coreutils）が直接利用していなくても
   リンクされる
2. **ネットワーク非利用**: ライブラリ自身が BSD socket API / DNS 解決 API / Unix
   ドメインソケットを利用しない、または利用したとしても kernel /sys インターフェース等の
   非ネットワーク経路のみ
3. **false positive 発生実績**: 実運用において当該ライブラリ由来のシンボル/syscall が
   「実行ファイルの実際の動作と乖離する」ことが確認されている

逆に、以下に該当するライブラリは除外しない。

- libssl, libcurl, libxml2, libglib-2.0 等の中間ライブラリ（実際にネットワークを
  使う可能性がある）
- 言語ランタイムライブラリ（libpython, libruby 等。スクリプト次第でネットワークを使う）
- libpam（認証バイナリでは実際にネットワーク NSS / LDAP を経由する可能性あり）

---

## 4. 非機能要件

### NFR-1: ファイル完全性検証への非影響

除外対象ライブラリは依然として `DynLibDeps` に記録され、ハッシュ検証は実施される。

ただし、除外対象ライブラリ自身が改ざんされている場合、`runner` 自身も同じシステム
ライブラリ群に依存するため検証結果の信頼性は限定的である。これは本機能の前提として
許容する設計判断とする。

### NFR-2: スキーマバージョン

`DetectedSymbols` / `SyscallAnalysis` のセマンティクスは変化する（libselinux 由来
シンボル/syscall が記録されなくなる）が、構造は変わらない。

`CurrentSchemaVersion` の更新要否は、旧キャッシュとの互換性が問題になるかをアーキテクチャ
設計で判断する。旧キャッシュに libselinux 由来のシンボルが残っていても、新 `runner` が
ネットワーク判定を行う際に false positive が増えるのみで安全側に倒れる場合は、
バージョン更新を不要とすることも検討する。

### NFR-3: 性能

除外リストへの追加は O(prefix list size) のプレフィックス比較を 1 回追加するのみであり、
性能への影響は無視できる。

---

## 5. 受け入れ基準

| ID | 基準 |
|----|------|
| AC-1 | `libselinux.so.1` を含む実行ファイル（例: `/usr/bin/cp` 相当のテストバイナリ）に対して `record` を実行したとき、`DetectedSymbols` に `source_path = .../libselinux.so.*` のエントリが含まれないこと |
| AC-2 | 同じ実行ファイルの `SyscallAnalysis.occurrences[].source_path` に libselinux のパスが含まれないこと |
| AC-3 | 実行ファイル本体が libc から `socket`/`connect`/`bind`/`getaddrinfo` 等のシンボルを import している場合、これらが `DetectedSymbols` に正しいカテゴリ（`socket` / `dns`）で記録されること（ネットワーク API 検出の維持） |
| AC-4 | 実行ファイル本体に `syscall` 命令で `socket`/`connect` 等が含まれる場合、`SyscallAnalysis` に検出されること（機械語経路の維持） |
| AC-5 | 実行ファイル本体が `dlopen` / `dlsym` を import している場合、`DynamicLoadSymbols` に `dynamic_load` カテゴリで記録されること（除外対象内部の dlsym と区別される） |
| AC-6 | 実行ファイル本体に `mprotect` + `PROT_EXEC`（または `pkey_mprotect`）が含まれる場合、検出が継続すること（task 0125, 0081 経路の維持） |
| AC-7 | `DynLibDeps` に libselinux のエントリ（`path` + `hash`）が引き続き記録され、`verify` 時にハッシュ検証が行われること |
| AC-8 | `libselinuxabc.so.1` のような前方一致するが区切り文字条件を満たさない SOName は除外されないこと（既存 `matchesKnownPrefix` の挙動踏襲） |
| AC-9 | `make test` / `make lint` がエラーなしで通過すること |

---

## 6. 制約事項

1. **ハードコード限定**: 除外対象リストはハードコードのみ。設定ファイルによる
   ユーザー側カスタマイズは対象外。
2. **最小限の除外**: ネットワーク API 検出能力を損なわない範囲に限り、初期リストは
   libselinux 1 件のみとする。
3. **関数レベル解析の非採用**: 関数レベル到達可能性解析による精度改善は task 0123 §8
   と同様に本タスクのスコープ外。

---

## 7. 先行タスクとの関係

| タスク | 関係 |
|---|---|
| 0069 ELF dynsym ネットワーク検出 | 実行ファイル本体の dynsym 解析。本タスク後も維持 |
| 0070 / 0072 ELF syscall 命令解析 | 機械語スキャン経路。除外対象ライブラリ内ではスキップされるが、実行ファイル本体・除外対象外ライブラリでは維持 |
| 0078 / 0081 / 0125 mprotect / PROT_EXEC 検出 | 動的コード実行検出。FR-2 で挙動維持を要求 |
| 0082 dynlib シンボル解析 | DynLibDeps 収集基盤。除外対象ライブラリも引き続き DynLibDeps に記録 |
| 0106 シンボル解析ライブラリフィルタ | libc/libSystem 限定の symbol 解析（基盤） |
| 0123 dynlib 再帰的 syscall 解析 | 本タスクが拡張する除外メカニズムの導入元。§6.2.4 の「false positive 軽減策としての除外リスト」を本タスクで具体化 |
| 0131 デバッグ source attribution | `source_path` フィールドによる false positive の可視化（本タスクの背景となる現象を観測可能にした） |
