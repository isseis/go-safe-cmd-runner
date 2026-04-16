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
| G4 | 機械語 syscall 静的解析（syscall 番号の特定） | ✅ 0070 (x86_64) / 0072 (arm64) | ⚠️ 0073（`svc #0x80` の存在確認のみ、番号解析なし） | **FR-4.2** |
| G5 | 動的リンクライブラリ整合性検証（依存ライブラリのハッシュ記録・照合） | ✅ 0074 | ❌ 未実装 | **FR-4.3** |
| G6 | ネットワークシンボル解析結果のキャッシュ（`record` → `runner`） | ✅ 0076 | ⚠️ `DetectedSymbols` / `DynamicLoadSymbols` は共通キャッシュ済みだが、Mach-O 固有の direct syscall/high risk 信号は live 解析依存 | **FR-4.4** |
| G7 | CGO/動的バイナリへの syscall 解析フォールバック | ✅ 0077 | ❌ 未実装 | **FR-4.5** |
| G8 | 動的コードロード（`mprotect(PROT_EXEC)`）静的検出 | ✅ 0078 | ❌ 未実装 | **FR-4.6** |
| G9 | `pkey_mprotect(PROT_EXEC)` 静的検出 | ✅ 0081 | N/A（Linux 固有） | 対象外 |
| G10 | libc syscall ラッパー関数キャッシュ（関数名 → syscall 番号） | ✅ 0079 | ❌ 未実装（libSystem.dylib 対応なし） | **FR-4.7** |
| G11 | 直接依存 `.dylib` のベース名による既知ネットワークライブラリ検出 | ✅ 0082 | ❌ 未実装 | **FR-4.8** |
| G12 | `dlopen`/`dlsym` シンボル検出（dynamic load） | ✅ 0074/0076 | ✅ `DynamicLoadSymbols` は `SymbolAnalysis` に保存され、runner 判定でも利用済み | 対応済み |
| G13 | 実行バイナリ本体の `.text` セクション走査 | ✅ 0070/0072 | ✅ 0073（`svc #0x80` のみ） | G4 に包含 |
| G14 | 特権アクセス（execute-only バイナリ）対応 | ✅ 0069 | ❌ 未実装 | **FR-4.9** |

### 3.2 macOS 固有の制約

| 項目 | 内容 | 影響 |
|------|------|------|
| dyld shared cache | `libSystem.dylib` 等のシステムライブラリはディスク上に個別ファイルとして存在しない場合がある（macOS 11+）。詳細は下記 | G5（依存ライブラリ整合性検証）および G10（libSystem syscall ラッパーキャッシュ）で独自対応が必要 |
| コード署名 | Apple の署名検証が OS レベルで実施される | 本システムのハッシュ検証と併存する。未署名バイナリを追加で拒否する要件は本タスクでは扱わない |
| `svc #0x80` の希少性 | 正規バイナリでは出現しない | ELF のように全 syscall 命令を解析する前提が成り立たない。検出時は即 high risk 扱い（タスク 0073 方針維持） |
| SIP (System Integrity Protection) | `/usr/bin` 等の書き換え不可領域が存在 | 供給チェーン攻撃の前提が ELF と異なり、優先度判断に影響 |

#### dyld shared cache の詳細

**格納場所の変遷:**

| macOS バージョン | 格納場所 |
|---|---|
| ~10.15 (Catalina) | `/private/var/db/dyld/dyld_shared_cache_<arch>` |
| 11 (Big Sur) | `/System/Library/dyld/dyld_shared_cache_<arch>` |
| 12 (Monterey) | 同上（サブキャッシュ `.1`, `.2`, ... 分割導入） |
| 13 (Ventura)〜15 | Cryptex 導入: `/System/Volumes/Preboot/Cryptexes/OS/System/Library/dyld/dyld_shared_cache_<arch>[.N]` |

- Apple Silicon マシンでは `arm64e` アーキテクチャのキャッシュが使用される
- macOS 12+ ではキャッシュが主キャッシュ + 複数サブキャッシュ (`.1`, `.2`, ...) + `.symbols` に分割されるため、抽出には全ファイルが必要

**ファイルフォーマット概要:**

- **ヘッダ** (`dyld_cache_header`): magic `"dyld_v1 "` + アーキ文字列、mapping/images オフセット、UUID
- **mapping 配列** (`dyld_cache_mapping_info`): 仮想アドレス → ファイルオフセット（TEXT/DATA/LINKEDIT）
- **images 配列** (`dyld_cache_image_info`): 各 dylib のロードアドレス、パス文字列オフセット
- **slide info**: DATA 領域の再配置テーブル
- 仕様ソース: [apple-oss-distributions/dyld](https://github.com/apple-oss-distributions/dyld) の `cache-builder/dyld_cache_format.h`

### 3.3 FR 実装完了後の到達状態

全 FR（優先度低を含む）を実装した場合に達成される ELF との機能パリティを示す。

凡例: ✅ ELF と同等 / ⚠️ 機能的に近いが差異あり / N/A 対象外（プラットフォーム固有）

| # | 機能 | ELF | Mach-O（FR 完了後） | 残存する差異・制約 |
|---|------|-----|--------------------|-------------------|
| G1 | バイナリ形式判定 | ✅ | ✅ | — |
| G2 | インポートシンボル解析 | ✅ | ✅ | — |
| G3 | Fat バイナリの全スライス解析 | N/A | ✅ | — |
| G4 | 機械語 syscall 静的解析（syscall 番号の特定） | ✅ | ✅ | `svc #0x80` は正規バイナリでは稀なため適用頻度は低い。検出ロジック自体は ELF arm64 と同等 |
| G5 | 動的リンクライブラリ整合性検証 | ✅ | ⚠️ | dyld shared cache 内のシステムライブラリ（`libSystem.dylib` 等）はハッシュ検証不可。Apple のコード署名に委譲（macOS プラットフォーム制約） |
| G6 | ネットワークシンボル解析結果のキャッシュ | ✅ | ✅ | FR-4.4 で Mach-O 固有の direct syscall 信号も既存スキーマへ統合 |
| G7 | CGO/動的バイナリへの syscall 解析フォールバック | ✅ | ✅ | macOS の正規 Go バイナリは `libSystem.dylib` 経由で syscall を発行するため、主に難読化・マルウェア的バイナリの検出が対象になる点で用途が異なる |
| G8 | 動的コードロード（`mprotect(PROT_EXEC)`）静的検出 | ✅ | ⚠️ | バイナリ本体 `.text` 内の直接 `svc #0x80` 経由呼び出しのみ検出。通常の動的リンクバイナリが `libSystem.dylib` 経由で呼ぶケースは FR-4.7（libSystem キャッシュ）との組み合わせで補完 |
| G9 | `pkey_mprotect(PROT_EXEC)` 静的検出 | ✅ | N/A | Linux 固有 syscall。macOS には存在しない |
| G10 | libc syscall ラッパー関数キャッシュ | ✅ | ⚠️ | 初期リリース（FR-4.7 段階 1）は dyld shared cache 内ライブラリをシンボル名一致にフォールバック。`blacktop/ipsw` を用いた将来実装（段階 2）で ELF 同等のキャッシュに到達 |
| G11 | 直接依存ライブラリの既知ネットワークライブラリ検出 | ✅ | ✅ | ELF 版の `known_network_libs.go` を共用。Mach-O 側はインストール名正規化のみ追加 |
| G12 | `dlopen`/`dlsym` シンボル検出 | ✅ | ✅ | 既に対応済み |
| G13 | 実行バイナリ本体の `.text` セクション走査 | ✅ | ✅ | G4 に包含 |
| G14 | 特権アクセス（execute-only バイナリ）対応 | ✅ | ✅ | macOS では execute-only バイナリは稀 |

**⚠️ 項目のまとめ:** 残存する差異はいずれも実装上の問題ではなく、macOS プラットフォームの構造的制約（dyld shared cache によるシステムライブラリの非公開、`svc #0x80` の希少性）に起因する。ELF と完全同一ではないが、これらの制約を踏まえた「機能的同等」を達成できる。

## 4. 機能要件

### FR-4.1: 共通要件

本タスクで定義される個別機能（FR-4.2〜FR-4.9）は、いずれも以下を満たすこと：

- `safefileio` パッケージを経由したファイルアクセス（シンボリックリンク・TOCTOU 保護）
- 不正な Mach-O ファイルに対するパニック非発生
- 解析失敗時は安全側（ネットワーク操作ありと見做す）に倒す
- 原則として Go 標準ライブラリ `debug/macho` および準公式 `golang.org/x/arch/arm64/arm64asm` を使用する。追加依存を導入する場合は、FR ごとに必要性・ライセンス・配布影響を明記する

### FR-4.2: Mach-O 機械語 syscall 静的解析（G4）

タスク 0072 の arm64 syscall 解析と同等の仕組みを Mach-O の `__TEXT,__text` セクションに
適用すること。ただし macOS 固有の事情に配慮する：

- **syscall 命令**: `svc #0x80`（arm64）
- **syscall 番号レジスタ**: `x16`（macOS ABI）
- **後方スキャン**: タスク 0072 の `arm64_decoder.go` のロジックを再利用し、`x16` への即値設定を特定する
- **検出対象 syscall**: macOS のシステムコール番号テーブル（BSD クラス: `socket`=97, `connect`=98 等）を新規に定義する。なお、Darwin arm64 では `x16` レジスタにクラスプレフィックス（BSD の場合は `0x2000000`）が含まれるため、解析時にこれを考慮すること
- **フォールバック動作**: 現行の「`svc #0x80` 存在 → 即 high risk」挙動は維持しつつ、syscall 番号が特定できた場合はネットワーク関連 syscall のみを `NetworkDetected` として扱い、番号不明の `svc #0x80` は引き続き high risk とする
- **Fat バイナリ**: 全スライスを解析（タスク 0073 方針維持）

**実装優先度**: 中（`svc #0x80` は稀なため、主に Go 純正静的バイナリの将来対応用）

### FR-4.3: Mach-O 動的リンクライブラリ整合性検証（G5）

タスク 0074 相当の機能を Mach-O に適用する：

- **依存ライブラリ抽出**: `macho.File.ImportedLibraries()` により `LC_LOAD_DYLIB` / `LC_LOAD_WEAK_DYLIB` の一覧を取得
- **ライブラリパス解決**:
  1. `@executable_path` / `@loader_path` / `@rpath` の展開
  2. `LC_RPATH` エントリの走査
  3. デフォルト検索パス（`/usr/lib`, `/usr/local/lib`）
  4. `DYLD_LIBRARY_PATH` / `DYLD_FALLBACK_LIBRARY_PATH` は **`record` 時は使用しない**。`runner` は常にこれらをクリアして子プロセスを起動する
- **dyld shared cache への対応**: `libSystem.dylib` 等がファイルシステム上に存在しない場合は、ハッシュ検証をスキップし、**コード署名検証に委譲する旨をログに記録**する。あるいは macOS バージョンとアーキテクチャに基づく whitelist で代替する（詳細は設計フェーズ）
- **パス正規化**: `filepath.EvalSymlinks` + `filepath.Clean`（ELF と同様）
- **`fileanalysis.Record` への保存**: Mach-O でも既存の `DynLibDeps` フィールドを使用する。別フィールドは追加しない
- **`LC_RPATH` 非対応ケース**: 未サポートのパストークンを検出した場合は `ErrDyldTokenNotSupported` として `record` を中断

**実装優先度**: 高（供給チェーン攻撃対策として基幹）

### FR-4.4: Mach-O 固有 high risk 信号のキャッシュ統合（G6）

`DetectedSymbols` / `DynamicLoadSymbols` のキャッシュ自体は既に共通実装で扱えているため、
本 FR では Mach-O 固有の未キャッシュ信号を既存スキーマへ統合する：

- **`record` 時**: FR-4.2 / FR-4.6 で得た Mach-O の syscall 解析結果を既存の `fileanalysis.Record.SyscallAnalysis` に保存する
- **`record` 時の direct syscall**: 現行の「`svc #0x80` 検出 → `AnalysisError`」だけでは runner が毎回 live 解析に依存するため、記録可能な表現へ整理する
- **`runner` 実行時**: 保存済みの `SyscallAnalysis` と `SymbolAnalysis` を参照し、Mach-O でも ELF 同様に live 再解析を最小化する
- **Fat バイナリ**: 記録時点で全スライスの結合結果を保存し、再解析時の非決定性を避ける
- **スキーマ方針**: 新規の `NetworkSymbolAnalysis` / `HasDirectSyscall` フィールドは導入せず、既存の `SymbolAnalysis` / `SyscallAnalysis` / `AnalysisWarnings` に集約する

**実装優先度**: 中（既存キャッシュ基盤はあるため、Mach-O 固有信号の保存統合が中心）

### FR-4.5: CGO/動的 Mach-O バイナリへの syscall 解析フォールバック（G7）

タスク 0077 相当のフォールバックを Mach-O にも適用する：

- インポートシンボル解析で `NoNetworkSymbols` となった Mach-O バイナリに対して、FR-4.2 の syscall 解析を適用
- **注記**: macOS の正規 Go バイナリは `libSystem.dylib` 経由で syscall を発行するため、本フォールバックが検出するのは主に「難読化・マルウェア的挙動のバイナリ」である
- 実装は FR-4.2 の機械語 syscall 解析に依存

**実装優先度**: 低（FR-4.2 の副次的効果として実現）

### FR-4.6: Mach-O における `mprotect(PROT_EXEC)` 静的検出（G8）

タスク 0078 相当の検出を Mach-O に適用する：

- **syscall 番号**: macOS arm64 BSD syscall `mprotect` = 74
- **引数レジスタ**: `x2`（第3引数 `prot`）
- **PROT_EXEC フラグ**: `0x4`（POSIX 共通）
- **後方スキャン**: FR-4.2 と共通の arm64 デコーダを再利用
- **libSystem.dylib 経由の呼び出し**: 本検出はバイナリ本体の `.text` にしか適用されないため、通常の動的リンクバイナリでは `mprotect` ラッパーは libSystem 側にある。FR-4.7 との併用で検出範囲を広げる
- **結果の保存**: `SyscallArgEvalResult` を Mach-O 版でも既存の `fileanalysis.Record.SyscallAnalysis.ArgEvalResults` に保存する

**実装優先度**: 中（動的コードロードシグナルの補強）

### FR-4.7: libSystem.dylib システムコールラッパー関数キャッシュ（G10）

タスク 0079 相当のキャッシュを macOS でも実装する：

- **対象ライブラリ**: `libSystem.dylib`（およびサブコンポーネント `libsystem_kernel.dylib`）
- **ラッパー関数の特定**: FR-4.2 の syscall 解析を libSystem の各エクスポート関数に適用
- **dyld shared cache への対応**:
  1. `libSystem.dylib` がファイルシステム上に存在する場合は従来通り解析
  2. 存在しない場合は dyld shared cache から該当 `.dylib` を抽出して解析（格納場所は §3.2 参照）
  3. 実装複雑度が高いため、初期リリースでは「dyld shared cache 利用時は解析をスキップし、シンボル名単体一致にフォールバック」とすることを許容
- **キャッシュ保存場所**: `<hash-dir>/lib-cache/` サブディレクトリ（ELF と共通）
- **キャッシュキー**: ライブラリファイルのハッシュ値。dyld shared cache 由来の場合は shared cache 自体のハッシュ + エントリ名

**dyld shared cache からの抽出手段:**

| 手段 | 概要 | 長所 | 短所 |
|------|------|------|------|
| **blacktop/ipsw `pkg/dyld`** | Pure Go の dyld shared cache パーサ/抽出ライブラリ | MIT ライセンス、クロスプラットフォーム、活発にメンテナンス（★3300+）、サブキャッシュ対応済み、Go import で単体バイナリに統合可能 | 依存ライブラリが増加、ipsw 全体のサイズが大きいため pkg/dyld のみ import する場合でも依存整理が必要 |
| **keith/dyld-shared-cache-extractor** | `dsc_extractor.bundle` を呼び出すラッパー | `brew install` で導入可能、サブキャッシュ対応 | macOS 限定、Xcode 依存（`dsc_extractor.bundle`）、外部コマンド呼び出し |
| **`dsc_extractor.bundle` (Xcode 付属)** | Apple 提供の抽出用 dylib | Apple 公式 | Xcode インストール必須、再配布不可、cgo (`dlopen`) 必要 |
| **Apple `dyld_shared_cache_util`** | Apple 公式 CLI | フル機能 | 自前ビルド必要、APSL 2.0 ライセンス、再配布の整理が必要 |
| **ランタイム `dlopen` + メモリ解析** | 自プロセスに dylib をロードしてメモリ上の Mach-O を解析 | SIP 下でも動作、外部依存なし | rebase/bind 済みのため元バイナリと異なる、cgo 必要 |
| **`.tbd` スタブ** | Xcode SDK 内のテキストベース dylib 定義 | シンボル名一覧のみの用途なら十分 | **マシンコードを含まない** — syscall ラッパーの命令列解析には使用不可 |

**推奨方針（段階的アプローチ）:**

1. **初期リリース**: dyld shared cache 内のライブラリに対しては syscall ラッパー解析をスキップし、シンボル名単体一致にフォールバック
2. **将来実装**: `blacktop/ipsw` の `pkg/dyld` パッケージを Go ライブラリとして import し、キャッシュからの `.dylib` 抽出を実装する
   - **根拠**: Pure Go（cgo 不要）、MIT ライセンス、サブキャッシュ対応済み、単体バイナリに統合可能
   - **リスク**: 依存ライブラリのサイズ・transitive dependency の評価を設計フェーズで実施すること
3. **不採用**: `.tbd` ファイルは命令列を含まないため syscall 解析には使用不可。`dsc_extractor.bundle` / `dyld_shared_cache_util` は外部依存・ライセンスの観点から非推奨

**実装優先度**: 低（dyld shared cache 対応の実装コストが大きい）。段階的リリースを推奨

### FR-4.8: 直接依存 `.dylib` のベース名ベース既知ネットワークライブラリ検出（G11）

タスク 0082（方策 C）相当の仕組みを Mach-O に適用する：

- `LC_LOAD_DYLIB` のインストール名（例: `/usr/local/opt/ruby/lib/libruby.3.2.dylib`）からベース名を抽出し、既知ネットワークライブラリリスト（`libruby`, `libpython`, `libperl`, `libcurl`, `libssl` 等のプレフィックス照合）と突き合わせる
- **プレフィックスリスト**: ELF 版の `known_network_libs.go` をそのまま再利用し、Mach-O 側では「インストール名 → ベース名」正規化のみ追加する
- **FR-4.3 との統合**: `DynLibDeps` に格納された Mach-O 依存ライブラリ名から `KnownNetworkLibDeps` を導出する

**実装優先度**: 中（Ruby/Python 等のランタイムでの誤検出回避に有用）

### FR-4.9: 特権アクセス対応（G14）

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
| 0070 (ELF x86_64 syscall) | FR-4.2 | x86_64 Mach-O は対象外 |
| 0072 (ELF arm64 syscall) | FR-4.2 | arm64 デコーダを再利用 |
| 0073 (Mach-O ネットワーク検出) | G1-G3, G13 | 基盤。本タスクはこの上に機能追加 |
| 0074 (ELF `DT_NEEDED` 整合性) | FR-4.3 | `LC_LOAD_DYLIB` に置換 |
| 0076 (ネットワークシンボルキャッシュ) | FR-4.4 | 共通キャッシュ基盤は既存実装を流用し、Mach-O 固有信号の保存だけを追加 |
| 0077 (CGO 動的バイナリフォールバック) | FR-4.5 | FR-4.2 に依存 |
| 0078 (mprotect PROT_EXEC) | FR-4.6 | arm64 デコーダを共用 |
| 0079 (libc syscall ラッパーキャッシュ) | FR-4.7 | dyld shared cache 対応が追加必要 |
| 0081 (pkey_mprotect) | 対象外（Linux 固有） | — |
| 0082 (SOName ベース検出) | FR-4.8 | 既知ライブラリリストを共用 |

## 8. 実装サブタスク分割案

本書で定義した FR を個別タスクに分割して段階的に実装することを推奨する。優先度に応じた
実装順序案：

### フェーズ 1（基盤・高優先度）

1. **FR-4.3**: `LC_LOAD_DYLIB` 整合性検証
   - 供給チェーン攻撃対策として最も効果的。dyld shared cache 対応は初期では簡易版でよい

### フェーズ 2（検出力強化）

2. **FR-4.2**: Mach-O 機械語 syscall 静的解析（`x16` レジスタ + macOS syscall テーブル）
3. **FR-4.4**: Mach-O 固有 high risk 信号のキャッシュ統合
4. **FR-4.8**: `.dylib` ベース名ベース既知ネットワークライブラリ検出
5. **FR-4.6**: Mach-O `mprotect(PROT_EXEC)` 静的検出

### フェーズ 3（補完）

6. **FR-4.5**: CGO Mach-O フォールバック（FR-4.2 の副次的効果）
7. **FR-4.7**: libSystem.dylib syscall ラッパーキャッシュ（dyld shared cache 対応）
8. **FR-4.9**: 特権アクセス対応

## 9. 設計上の決定事項

| 項目 | 決定内容 |
|------|----------|
| dyld shared cache からの `.dylib` 抽出 | Go での実装は可能（§3.2 に格納場所・フォーマット、FR-4.7 に抽出手段の比較・推奨方針を記載） |
| コード署名検証との関係 | 本システムのハッシュ検証と Apple のコード署名検証は**相互補完**として併存する。本システムは供給チェーン攻撃（record 時と実行時のバイナリ差し替え）を検出し、Apple のコード署名は改竄検出を担う |
| Fat バイナリのキャッシュ粒度 | 当面は arm64 のみ対応のため、スライス分割は不要 |
| `fileanalysis.Record` スキーマ | ELF 用 `DynLibDeps` と Mach-O 用データは同一フィールドで扱う。別フィールドの必要性が出た時点で再検討する |
| macOS バージョン差異 | 原則として最新版 macOS を対象とする |
| CI 環境 | 本タスクでは検討対象外とする |
