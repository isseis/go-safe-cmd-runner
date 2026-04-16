# Mach-O バイナリ解析機能の ELF 同等性（フィーチャーパリティ）要件定義書

## 1. 概要

### 1.1 背景

タスク 0069 以降、Linux/ELF バイナリを対象としたバイナリ静的解析によるネットワーク操作・
動的コードロード等のリスク判定機能を段階的に実装してきた。タスク 0073 で macOS/Mach-O
バイナリのネットワーク検出を導入したが、ELF 側のみに存在する機能が多数ある。

現状、macOS プラットフォームでの runner 実行では以下のケースを検出できない：

1. 動的リンクライブラリ（`.dylib`）の差し替え・供給チェーン攻撃
2. `libSystem.dylib` を経由するネットワーク syscall の網羅的把握（現状はシンボル名単体の一致のみ）
3. `record` 時と `runner` 実行時のシンボル解析結果のキャッシュ（毎回再解析）
4. 動的コードロード（`mprotect(PROT_EXEC)` 相当）の静的検出
5. 言語ランタイム用 `.dylib`（例: `libruby`, `libpython`）を介したネットワーク利用

本タスクは、これらのギャップを体系的に整理し、Mach-O でも ELF と同等のリスク判定が
行える水準を目指すための包括的要件定義を行う。

### 1.2 目的

- ELF 側に実装済みでありながら Mach-O 側に欠落している解析機能を列挙し、macOS プラットフォームにおける検出力を ELF と同等水準まで引き上げる。
- 個別サブタスク（本タスク配下または独立タスク）での実装計画策定の土台となる要件を提供する。
- macOS 固有の制約（dyld shared cache、コード署名、`svc #0x80` の非推奨）を踏まえ、
  ELF と完全同一ではなく「機能的に等価な検出」を定義する。

### 1.3 スコープ

- **対象プラットフォーム**: macOS (Darwin) arm64
- **対象バイナリ形式**: Mach-O（単一アーキテクチャおよび Fat バイナリ）
- **対象機能**: 本書のギャップ分析（§3）で特定する全項目
- **対象外**: x86_64 macOS（タスク 0073 のスコープに準拠）
- **対象外**: iOS/iPadOS/tvOS/watchOS 向けバイナリ
- **対象外**: スクリプトファイル・非 Mach-O ファイル
- **対象外**: `pkey_mprotect`（タスク 0081、Linux 固有）

## 2. 用語定義

| 用語 | 定義 |
|------|------|
| `LC_LOAD_DYLIB` | Mach-O Load Command の一種。バイナリが実行時にリンクする共有ライブラリ（`.dylib`）を指定する。ELF の `DT_NEEDED` に相当 |
| `LC_LOAD_WEAK_DYLIB` | `LC_LOAD_DYLIB` の弱リンク版。ライブラリが存在しない場合でもバイナリはロード可能 |
| `LC_RPATH` | Mach-O におけるランタイム検索パス。ELF の `DT_RUNPATH` に相当 |
| `@executable_path` | `LC_RPATH` 中のトークン。バイナリ本体のディレクトリを指す。ELF の `$ORIGIN` に相当 |
| `@loader_path` | `LC_RPATH` 中のトークン。`.dylib` をロードした主体（バイナリまたは別の `.dylib`）のディレクトリを指す |
| `@rpath` | `LC_LOAD_DYLIB` に記録されるパス中で、`LC_RPATH` の各エントリを順に展開して解決されるプレースホルダ |
| dyld shared cache | macOS のシステム `.dylib`（`libSystem.dylib` 等）を単一の大きなキャッシュファイルに統合した機構。macOS 11 以降は個別の `.dylib` ファイルがファイルシステム上に存在しない場合がある |
| コード署名 | Apple 独自のバイナリ完全性検証機構（`LC_CODE_SIGNATURE`）。本書のハッシュ検証とは独立に運用される |
| `svc #0x80` | arm64 macOS における直接 syscall 命令（エンコード `0xD4001001`）。正規バイナリは `libSystem.dylib` 経由で呼ぶため通常出現しない |
| ImportedLibraries | `debug/macho` パッケージの `File.ImportedLibraries()`。`LC_LOAD_DYLIB` / `LC_LOAD_WEAK_DYLIB` のライブラリ名一覧を返す |

## 3. ギャップ分析

### 3.1 機能対応表

凡例: ✅ 実装済み / ❌ 未実装 / ⚠️ 部分実装 / N/A 対象外

| # | 機能 | ELF (Linux) | Mach-O (macOS) | 本タスクでの扱い |
|---|------|-------------|----------------|-----------------|
| G1 | バイナリ形式判定 | ✅ 0069 | ✅ 0073 | 対応済み |
| G2 | インポートシンボル解析 | ✅ 0069 | ✅ 0073 | 対応済み |
| G3 | Fat バイナリの全スライス解析 | N/A | ✅ 0073 | 対応済み |
| G4 | 機械語 syscall 静的解析（syscall 番号の特定） | ✅ 0070 (x86_64) / 0072 (arm64) | ⚠️ 0073（`svc #0x80` の存在確認のみ、番号解析なし） | **FR-3.2** |
| G5 | 動的リンクライブラリ整合性検証（依存ライブラリのハッシュ記録・照合） | ✅ 0074 | ❌ 未実装 | **FR-3.3** |
| G6 | ネットワークシンボル解析結果のキャッシュ（`record` → `runner`） | ✅ 0076 | ❌ 未実装（毎回 live 解析） | **FR-3.4** |
| G7 | CGO/動的バイナリへの syscall 解析フォールバック | ✅ 0077 | ❌ 未実装 | **FR-3.5** |
| G8 | 動的コードロード（`mprotect(PROT_EXEC)`）静的検出 | ✅ 0078 | ❌ 未実装 | **FR-3.6** |
| G9 | `pkey_mprotect(PROT_EXEC)` 静的検出 | ✅ 0081 | N/A（Linux 固有） | 対象外 |
| G10 | libc syscall ラッパー関数キャッシュ（関数名 → syscall 番号） | ✅ 0079 | ❌ 未実装（libSystem.dylib 対応なし） | **FR-3.7** |
| G11 | 直接依存ライブラリの SOName による既知ネットワークライブラリ検出 | ✅ 0082 | ❌ 未実装 | **FR-3.8** |
| G12 | `dlopen`/`dlsym` シンボル検出（`HasDynamicLoad`） | ✅ 0074/0076 | ⚠️ `DynamicLoadSymbols` フィールドは収集するが `record` 側でのキャッシュ・整合性利用が未配線 | **FR-3.4** に包含 |
| G13 | 実行バイナリ本体の `.text` セクション走査 | ✅ 0070/0072 | ✅ 0073（`svc #0x80` のみ） | G4 に包含 |
| G14 | 特権アクセス（execute-only バイナリ）対応 | ✅ 0069 | ❌ 未実装 | **FR-3.9** |

### 3.2 macOS 固有の制約

| 項目 | 内容 | 影響 |
|------|------|------|
| dyld shared cache | `libSystem.dylib` 等のシステムライブラリはディスク上に個別ファイルとして存在しない場合がある（macOS 11+） | G5（依存ライブラリ整合性検証）および G10（libSystem syscall ラッパーキャッシュ）で独自対応が必要 |
| コード署名 | Apple の署名検証が OS レベルで実施される | 本システムのハッシュ検証と併存可能だが、未署名バイナリの扱いを要定義 |
| `svc #0x80` の希少性 | 正規バイナリでは出現しない | ELF のように全 syscall 命令を解析する前提が成り立たない。検出時は即 high risk 扱い（タスク 0073 方針維持） |
| SIP (System Integrity Protection) | `/usr/bin` 等の書き換え不可領域が存在 | 供給チェーン攻撃の前提が ELF と異なり、優先度判断に影響 |

## 4. 機能要件

### FR-3.1: 共通要件

本タスクで定義される個別機能（FR-3.2〜FR-3.9）は、いずれも以下を満たすこと：

- `safefileio` パッケージを経由したファイルアクセス（シンボリックリンク・TOCTOU 保護）
- 不正な Mach-O ファイルに対するパニック非発生
- 解析失敗時は安全側（ネットワーク操作ありと見做す）に倒す
- Go 標準ライブラリ `debug/macho` および準公式 `golang.org/x/arch/arm64/arm64asm` のみを使用

### FR-3.2: Mach-O 機械語 syscall 静的解析（G4）

タスク 0072 の arm64 syscall 解析と同等の仕組みを Mach-O の `__TEXT,__text` セクションに
適用すること。ただし macOS 固有の事情に配慮する：

- **syscall 命令**: `svc #0x80`（arm64）
- **syscall 番号レジスタ**: `x16`（macOS ABI）
- **後方スキャン**: タスク 0072 の `arm64_decoder.go` のロジックを再利用し、`x16` への即値設定を特定する
- **検出対象 syscall**: macOS syscall 番号テーブル（BSD クラス: `socket`=97, `connect`=98, etc.）を新規に定義する
- **フォールバック動作**: 現行の「`svc #0x80` 存在 → 即 high risk」挙動は維持しつつ、syscall 番号が特定できた場合はネットワーク関連 syscall のみを `NetworkDetected` として扱い、番号不明の `svc #0x80` は引き続き high risk とする
- **Fat バイナリ**: 全スライスを解析（タスク 0073 方針維持）

**実装優先度**: 中（`svc #0x80` は稀なため、主に Go 純正静的バイナリの将来対応用）

### FR-3.3: Mach-O 動的リンクライブラリ整合性検証（G5）

タスク 0074 相当の機能を Mach-O に適用する：

- **依存ライブラリ抽出**: `macho.File.ImportedLibraries()` により `LC_LOAD_DYLIB` / `LC_LOAD_WEAK_DYLIB` の一覧を取得
- **ライブラリパス解決**:
  1. `@executable_path` / `@loader_path` / `@rpath` の展開
  2. `LC_RPATH` エントリの走査
  3. デフォルト検索パス（`/usr/lib`, `/usr/local/lib`）
  4. `DYLD_LIBRARY_PATH` / `DYLD_FALLBACK_LIBRARY_PATH` は **`record` 時は使用しない**。`runner` は常にこれらをクリアして子プロセスを起動する
- **dyld shared cache への対応**: `libSystem.dylib` 等がファイルシステム上に存在しない場合は、ハッシュ検証をスキップし、**コード署名検証に委譲する旨をログに記録**する。あるいは macOS バージョンとアーキテクチャに基づく whitelist で代替する（詳細は設計フェーズ）
- **パス正規化**: `filepath.EvalSymlinks` + `filepath.Clean`（ELF と同様）
- **`fileanalysis.Record` への保存**: `DynLibDeps` フィールドを Mach-O でも使用する。スキーマバージョン更新を要する場合は新規 `DYLIB_DEPS` フィールドとして区別するか、既存フィールドを共用するかは設計で決定
- **`LC_RPATH` 非対応ケース**: 未サポートのパストークンを検出した場合は `ErrDyldTokenNotSupported` として `record` を中断

**実装優先度**: 高（供給チェーン攻撃対策として基幹）

### FR-3.4: Mach-O ネットワークシンボル解析結果のキャッシュ（G6、G12 包含）

タスク 0076 相当の仕組みを Mach-O に適用する：

- **`record` 時**: `StandardMachOAnalyzer.AnalyzeNetworkSymbols` の結果（`HasNetworkSymbols`, `DetectedSymbols`, `DynamicLoadSymbols`, `HasDirectSyscall`）を `fileanalysis.Record.NetworkSymbolAnalysis` に保存
- **`runner` 実行時**: 保存済み結果を参照し、live 解析をスキップ
- **`HasDynamicLoad` の活用**: `DynamicLoadSymbols`（`dlopen`, `dlsym`, `dlvsym`）が検出された場合のリスク加点ロジックを、ELF 同等に配線
- **Fat バイナリ**: 記録時点で全スライスの結合結果を保存（再解析時の非決定性を避けるため）
- **`svc #0x80` 検出フラグ**: 記録時に `HasDirectSyscall` として保存し、runner 実行時の high risk 判定に使用

**実装優先度**: 高（毎回の live 解析コスト削減、およびスキーマの統一性確保）

### FR-3.5: CGO/動的 Mach-O バイナリへの syscall 解析フォールバック（G7）

タスク 0077 相当のフォールバックを Mach-O にも適用する：

- インポートシンボル解析で `NoNetworkSymbols` となった Mach-O バイナリに対して、FR-3.2 の syscall 解析を適用
- **注記**: macOS の正規 Go バイナリは `libSystem.dylib` 経由で syscall を発行するため、本フォールバックが検出するのは主に「難読化・マルウェア的挙動のバイナリ」である
- 実装は FR-3.2 の機械語 syscall 解析に依存

**実装優先度**: 低（FR-3.2 の副次的効果として実現）

### FR-3.6: Mach-O における `mprotect(PROT_EXEC)` 静的検出（G8）

タスク 0078 相当の検出を Mach-O に適用する：

- **syscall 番号**: macOS arm64 BSD syscall `mprotect` = 74
- **引数レジスタ**: `x2`（第3引数 `prot`）
- **PROT_EXEC フラグ**: `0x4`（POSIX 共通）
- **後方スキャン**: FR-3.2 と共通の arm64 デコーダを再利用
- **libSystem.dylib 経由の呼び出し**: 本検出はバイナリ本体の `.text` にしか適用されないため、通常の動的リンクバイナリでは `mprotect` ラッパーは libSystem 側にある。FR-3.7 との併用で検出範囲を広げる
- **結果の保存**: `SyscallArgEvalResult` を Mach-O 版でも保存（FR-3.4 のスキーマに組み込む）

**実装優先度**: 中（動的コードロードシグナルの補強）

### FR-3.7: libSystem.dylib システムコールラッパー関数キャッシュ（G10）

タスク 0079 相当のキャッシュを macOS でも実装する：

- **対象ライブラリ**: `libSystem.dylib`（およびサブコンポーネント `libsystem_kernel.dylib`）
- **ラッパー関数の特定**: FR-3.2 の syscall 解析を libSystem の各エクスポート関数に適用
- **dyld shared cache への対応**:
  1. `libSystem.dylib` がファイルシステム上に存在する場合は従来通り解析
  2. 存在しない場合は dyld shared cache（`/System/Volumes/Preboot/Cryptexes/OS/System/Library/dyld/dyld_shared_cache_*`）から該当 `.dylib` を抽出して解析。抽出には専用ツール（`dyld_shared_cache_util` または自前実装）が必要
  3. 実装複雑度が高いため、初期リリースでは「dyld shared cache 利用時は解析をスキップし、シンボル名単体一致にフォールバック」とすることを許容
- **キャッシュ保存場所**: `<hash-dir>/lib-cache/` サブディレクトリ（ELF と共通）
- **キャッシュキー**: ライブラリファイルのハッシュ値。dyld shared cache 由来の場合は shared cache 自体のハッシュ + エントリ名

**実装優先度**: 低（dyld shared cache 対応の実装コストが大きい）。段階的リリースを推奨

### FR-3.8: 直接依存 `.dylib` の SOName ベース既知ネットワークライブラリ検出（G11）

タスク 0082（方策 C）相当の仕組みを Mach-O に適用する：

- `LC_LOAD_DYLIB` のインストール名（例: `/usr/local/opt/ruby/lib/libruby.3.2.dylib`）からベース名を抽出し、既知ネットワークライブラリリスト（`libruby`, `libpython`, `libperl`, `libcurl`, `libssl` 等のプレフィックス照合）と突き合わせる
- **プレフィックスリスト**: ELF 版（`known_network_libs.go`）と共通のデータ構造を使用し、Mach-O のインストール名正規化レイヤーを追加
- **FR-3.3 との統合**: `DynLibDeps` 相当の記録に SOName ベース判定結果を併記

**実装優先度**: 中（Ruby/Python 等のランタイムでの誤検出回避に有用）

### FR-3.9: 特権アクセス対応（G14）

- `PrivilegeManager` 経由の execute-only バイナリ読み取りを `StandardMachOAnalyzer` でも対応
- **実装優先度**: 低（macOS では execute-only パーミッションは稀）

## 5. 非機能要件

### NFR-5.1: パフォーマンス

- 各機能追加によって `record` / `runner` の実行時間が既存比で **2 倍を超えない**こと
- キャッシュヒット時の runner 実行時の追加オーバーヘッドは 10ms 未満

### NFR-5.2: 互換性

- ELF 側の既存実装・テストに影響を与えないこと
- `fileanalysis.Record` のスキーマ変更時は、既存記録の再実行が必要な旨を明記

### NFR-5.3: 保守性

- シンボルリスト・syscall 番号テーブル等のデータは ELF 側と共通化し、プラットフォーム差分のみを局所化する
- Mach-O 専用のユーティリティ（Load Command 走査、`@rpath` 展開等）は `machoanalyzer` パッケージに集約する

### NFR-5.4: テスタビリティ

- macOS CI 環境（GitHub Actions `macos-latest` 等）でのテスト実行を前提とする
- Linux 上でのクロスコンパイルによる Mach-O フィクスチャ生成の可否を設計フェーズで評価する

## 6. 受け入れ条件

本タスクは「ギャップ分析 + 要件定義」の位置づけであり、実装は各 FR ごとに個別サブタスクに
委ねる。本書単体での受け入れ条件は以下：

- [ ] ELF 側に実装済みの全機能が §3.1 の対応表に列挙されていること
- [ ] 各ギャップに対する FR が §4 で具体的に定義されていること
- [ ] macOS 固有の制約（dyld shared cache、コード署名等）が §3.2 で整理されていること
- [ ] 各 FR に実装優先度が付与されていること
- [ ] 実装サブタスクへの分割案（§8）が提示されていること

## 7. 先行タスクとの関係

| 先行タスク | 対応する本タスクの FR | 備考 |
|----------|---------------------|------|
| 0069 (ELF `.dynsym`) | G1, G2（0073 で対応済み） | — |
| 0070 (ELF x86_64 syscall) | FR-3.2 | x86_64 Mach-O は対象外 |
| 0072 (ELF arm64 syscall) | FR-3.2 | arm64 デコーダを再利用 |
| 0073 (Mach-O ネットワーク検出) | G1-G3, G13 | 基盤。本タスクはこの上に機能追加 |
| 0074 (ELF `DT_NEEDED` 整合性) | FR-3.3 | `LC_LOAD_DYLIB` に置換 |
| 0076 (ネットワークシンボルキャッシュ) | FR-3.4 | 既存のスキーマ拡張 |
| 0077 (CGO 動的バイナリフォールバック) | FR-3.5 | FR-3.2 に依存 |
| 0078 (mprotect PROT_EXEC) | FR-3.6 | arm64 デコーダを共用 |
| 0079 (libc syscall ラッパーキャッシュ) | FR-3.7 | dyld shared cache 対応が追加必要 |
| 0081 (pkey_mprotect) | 対象外（Linux 固有） | — |
| 0082 (SOName ベース検出) | FR-3.8 | 既知ライブラリリストを共用 |

## 8. 実装サブタスク分割案

本書で定義した FR を個別タスクに分割して段階的に実装することを推奨する。優先度に応じた
実装順序案：

### フェーズ 1（基盤・高優先度）

1. **FR-3.4**: Mach-O ネットワークシンボル解析結果のキャッシュ
   - 毎回 live 解析のコストを削減し、後続機能の記録先スキーマを確立する
2. **FR-3.3**: `LC_LOAD_DYLIB` 整合性検証
   - 供給チェーン攻撃対策として最も効果的。dyld shared cache 対応は初期では簡易版でよい

### フェーズ 2（検出力強化）

3. **FR-3.2**: Mach-O 機械語 syscall 静的解析（`x16` レジスタ + macOS syscall テーブル）
4. **FR-3.8**: SOName ベース既知ネットワークライブラリ検出
5. **FR-3.6**: Mach-O `mprotect(PROT_EXEC)` 静的検出

### フェーズ 3（補完）

6. **FR-3.5**: CGO Mach-O フォールバック（FR-3.2 の副次的効果）
7. **FR-3.7**: libSystem.dylib syscall ラッパーキャッシュ（dyld shared cache 対応）
8. **FR-3.9**: 特権アクセス対応

## 9. 未解決課題・調査事項

以下は設計フェーズで調査・決定すべき事項：

1. ~~**dyld shared cache からの `.dylib` 抽出**~~ → §10 で調査完了
2. **コード署名検証との関係**: 本システムのハッシュ検証と Apple のコード署名検証をどう併存させるか（相互補完 / 重複排除）
3. **Fat バイナリのキャッシュ粒度**: 全スライスを単一のエントリとして保存するか、スライスごとに分けるか
4. **`fileanalysis.Record` スキーマ**: ELF 用 `DynLibDeps` と Mach-O 用データを同一フィールドで扱うか、別フィールドで分離するか
5. **macOS バージョン差異**: macOS 11（Big Sur）以降の dyld shared cache、macOS 13 の Cryptex 導入等、対応範囲を明確化する
6. **CI 環境**: GitHub Actions `macos-latest` で arm64 テストが実行可能か（現状は x86_64 ランナーが多い）

## 10. 調査結果: dyld shared cache からの `.dylib` 抽出

### 10.1 dyld shared cache の格納場所

| macOS バージョン | 格納場所 |
|---|---|
| ~10.15 (Catalina) | `/private/var/db/dyld/dyld_shared_cache_<arch>` |
| 11 (Big Sur) | `/System/Library/dyld/dyld_shared_cache_<arch>` |
| 12 (Monterey) | 同上（サブキャッシュ `.1`, `.2`, ... 分割導入） |
| 13 (Ventura)〜15 | Cryptex 導入: `/System/Volumes/Preboot/Cryptexes/OS/System/Library/dyld/dyld_shared_cache_<arch>[.N]` |

- Apple Silicon マシンでは `arm64e` アーキテクチャのキャッシュが使用される
- macOS 12+ ではキャッシュが主キャッシュ + 複数サブキャッシュ (`.1`, `.2`, ...) + `.symbols` に分割されるため、抽出には全ファイルが必要

### 10.2 キャッシュファイルフォーマット概要

- **ヘッダ** (`dyld_cache_header`): magic `"dyld_v1 "` + アーキ文字列、mapping/images オフセット、UUID
- **mapping 配列** (`dyld_cache_mapping_info`): 仮想アドレス → ファイルオフセット（TEXT/DATA/LINKEDIT）
- **images 配列** (`dyld_cache_image_info`): 各 dylib のロードアドレス、パス文字列オフセット
- **slide info**: DATA 領域の再配置テーブル
- 仕様ソース: [apple-oss-distributions/dyld](https://github.com/apple-oss-distributions/dyld) の `cache-builder/dyld_cache_format.h`

### 10.3 抽出手段の比較

| 手段 | 概要 | 長所 | 短所 |
|------|------|------|------|
| **blacktop/ipsw `pkg/dyld`** | Pure Go の dyld shared cache パーサ/抽出ライブラリ | MIT ライセンス、クロスプラットフォーム、活発にメンテナンス（★3300+）、サブキャッシュ対応済み、Go import で単体バイナリに統合可能 | 依存ライブラリが増加、ipsw 全体のサイズが大きいため pkg/dyld のみ import する場合でも依存整理が必要 |
| **keith/dyld-shared-cache-extractor** | `dsc_extractor.bundle` を呼び出すラッパー | `brew install` で導入可能、サブキャッシュ対応 | macOS 限定、Xcode 依存（`dsc_extractor.bundle`）、外部コマンド呼び出し |
| **`dsc_extractor.bundle` (Xcode 付属)** | Apple 提供の抽出用 dylib | Apple 公式 | Xcode インストール必須、再配布不可、cgo (`dlopen`) 必要 |
| **Apple `dyld_shared_cache_util`** | Apple 公式 CLI | フル機能 | 自前ビルド必要、APSL 2.0 ライセンス、再配布の整理が必要 |
| **ランタイム `dlopen` + メモリ解析** | 自プロセスに dylib をロードしてメモリ上の Mach-O を解析 | SIP 下でも動作、外部依存なし | rebase/bind 済みのため元バイナリと異なる、cgo 必要 |
| **`.tbd` スタブ** | Xcode SDK 内のテキストベース dylib 定義 | シンボル名一覧のみの用途なら十分 | **マシンコードを含まない** — syscall ラッパーの命令列解析には使用不可 |

### 10.4 推奨方針

**段階的アプローチ:**

1. **初期リリース（FR-3.7 段階的リリースの第 1 段階）**: dyld shared cache 内のライブラリに対しては syscall ラッパー解析をスキップし、**シンボル名単体一致にフォールバック**する。これは §4 FR-3.7 で既に許容済み
2. **将来実装**: `blacktop/ipsw` の `pkg/dyld` パッケージを Go ライブラリとして import し、キャッシュからの `.dylib` 抽出を実装する
   - **根拠**: Pure Go（cgo 不要）、MIT ライセンス、サブキャッシュ対応済み、単体バイナリに統合可能
   - **リスク**: 依存ライブラリのサイズ・transitive dependency の評価を設計フェーズで実施すること
3. **不採用**: `.tbd` ファイルは命令列を含まないため syscall 解析には使用不可。`dsc_extractor.bundle` / `dyld_shared_cache_util` は外部依存・ライセンスの観点から非推奨
