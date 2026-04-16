# 実装計画書: Mach-O 機能パリティ（サブタスク管理）

本書は `01_requirements.md` §8 で定義したサブタスク分割に基づき、各サブタスクの進捗を追跡する。
各サブタスクは独立した番号付きタスクとして `docs/tasks/` 以下に作成する。

## 進捗サマリー

| タスク | 概要 | 優先度 | 状態 |
|--------|------|--------|------|
| 0096 | Mach-O `LC_LOAD_DYLIB` 整合性検証 | 高 | 未着手 |
| 0097 | Mach-O arm64 syscall 静的解析・キャッシュ統合・CGO フォールバック | 中 | 未着手 |
| 0098 | Mach-O `.dylib` ベース名による既知ネットワークライブラリ検出 | 中 | 未着手 |
| 0099 | Mach-O `mprotect(PROT_EXEC)` 静的検出 | 中 | 未着手 |
| 0100 | libSystem.dylib syscall ラッパー関数キャッシュ | 低 | 未着手 |
| 0101 | Mach-O 特権アクセス（execute-only バイナリ）対応 | 低 | 未着手 |

---

## フェーズ 1: 基盤・高優先度

### タスク 0096: Mach-O `LC_LOAD_DYLIB` 整合性検証（FR-4.3）

**概要**: `record` 実行時に Mach-O バイナリの依存ライブラリ（`LC_LOAD_DYLIB` / `LC_LOAD_WEAK_DYLIB`）のパスを解決してハッシュを記録し、`runner` 実行時に照合することで供給チェーン攻撃を検出する。ELF の `DT_NEEDED` 整合性検証（タスク 0074）の Mach-O 版。`@executable_path` / `@loader_path` / `@rpath` トークンの展開と `LC_RPATH` の走査を含む。dyld shared cache 内のシステムライブラリ（`libSystem.dylib` 等）はハッシュ検証をスキップしてコード署名検証に委譲する。

- [x] `docs/tasks/0096_macho_lc_load_dylib_integrity/01_requirements.md` を作成する
- [ ] `docs/tasks/0096_macho_lc_load_dylib_integrity/02_architecture.md` を作成する
- [ ] `docs/tasks/0096_macho_lc_load_dylib_integrity/03_detailed_specification.md` を作成する
- [ ] `docs/tasks/0096_macho_lc_load_dylib_integrity/04_implementation_plan.md` を作成する
- [ ] 実装・テストを行い PR をマージする

---

## フェーズ 2: 検出力強化

### タスク 0097: Mach-O arm64 syscall 静的解析・キャッシュ統合・CGO フォールバック（FR-4.2 / FR-4.4 / FR-4.5）

**概要**: Mach-O の `__TEXT,__text` セクションを逆アセンブルし、`svc #0x80` 直前の `x16` レジスタへの即値設定から BSD syscall 番号を特定することで、ネットワーク関連 syscall（`socket`=97, `connect`=98 等）を検出する（FR-4.2）。Darwin arm64 では `x16` に BSD クラスプレフィックス `0x2000000` が付加されるため解析時に考慮する。タスク 0072 の arm64 デコーダを再利用。Fat バイナリは全スライスを解析し最も深刻な結果を採用する。解析結果を `fileanalysis.Record.SyscallAnalysis` に保存してキャッシュとして活用し live 再解析を最小化する（FR-4.4）。インポートシンボル解析で `NoNetworkSymbols` となった CGO/動的バイナリにも同 syscall 解析をフォールバック適用する（FR-4.5）。

- [ ] `docs/tasks/0097_macho_arm64_syscall_analysis/01_requirements.md` を作成する
- [ ] `docs/tasks/0097_macho_arm64_syscall_analysis/02_architecture.md` を作成する
- [ ] `docs/tasks/0097_macho_arm64_syscall_analysis/03_detailed_specification.md` を作成する
- [ ] `docs/tasks/0097_macho_arm64_syscall_analysis/04_implementation_plan.md` を作成する
- [ ] 実装・テストを行い PR をマージする

### タスク 0098: Mach-O `.dylib` ベース名による既知ネットワークライブラリ検出（FR-4.8）

**概要**: `LC_LOAD_DYLIB` のインストール名（例: `/usr/local/opt/ruby/lib/libruby.3.2.dylib`）からベース名を抽出し、ELF 版の `known_network_libs.go` が持つ既知ネットワークライブラリ・言語ランタイムのプレフィックスリストと照合する。Mach-O 側で追加するのはインストール名 → ベース名の正規化のみ。タスク 0082（方策 C）の Mach-O 版。

- [ ] `docs/tasks/0098_macho_known_network_libs/01_requirements.md` を作成する
- [ ] `docs/tasks/0098_macho_known_network_libs/02_architecture.md` を作成する
- [ ] `docs/tasks/0098_macho_known_network_libs/03_detailed_specification.md` を作成する
- [ ] `docs/tasks/0098_macho_known_network_libs/04_implementation_plan.md` を作成する
- [ ] 実装・テストを行い PR をマージする

### タスク 0099: Mach-O `mprotect(PROT_EXEC)` 静的検出（FR-4.6）

**概要**: Mach-O の `__TEXT,__text` セクションで `svc #0x80`（`mprotect` = BSD syscall 74）を検出し、`x2` レジスタ（第3引数 `prot`）に `PROT_EXEC`（`0x4`）が設定されているかを後方スキャンで確認する。タスク 0097 の arm64 デコーダを共用。結果は `fileanalysis.Record.SyscallAnalysis.ArgEvalResults` に保存する。タスク 0078（ELF 版）の Mach-O 対応。

- [ ] `docs/tasks/0099_macho_mprotect_exec_detection/01_requirements.md` を作成する
- [ ] `docs/tasks/0099_macho_mprotect_exec_detection/02_architecture.md` を作成する
- [ ] `docs/tasks/0099_macho_mprotect_exec_detection/03_detailed_specification.md` を作成する
- [ ] `docs/tasks/0099_macho_mprotect_exec_detection/04_implementation_plan.md` を作成する
- [ ] 実装・テストを行い PR をマージする

---

## フェーズ 3: 補完

### タスク 0100: libSystem.dylib syscall ラッパー関数キャッシュ（FR-4.7）

**概要**: タスク 0079（ELF 版 libc syscall ラッパーキャッシュ）の macOS 版。`libSystem.dylib` の各エクスポート関数に FR-4.2 の syscall 解析を適用し、関数名 → syscall 番号のキャッシュを構築する。ファイルシステム上に存在しない場合は dyld shared cache（`/System/Library/dyld/dyld_shared_cache_arm64e` 等）から対象ライブラリを抽出して解析する。段階的リリースを推奨（段階 1: ファイル存在時のみ解析、段階 2: dyld shared cache 対応）。

- [ ] `docs/tasks/0100_macho_libsystem_syscall_cache/01_requirements.md` を作成する
- [ ] `docs/tasks/0100_macho_libsystem_syscall_cache/02_architecture.md` を作成する
- [ ] `docs/tasks/0100_macho_libsystem_syscall_cache/03_detailed_specification.md` を作成する
- [ ] `docs/tasks/0100_macho_libsystem_syscall_cache/04_implementation_plan.md` を作成する
- [ ] 実装・テストを行い PR をマージする

### タスク 0101: Mach-O 特権アクセス（execute-only バイナリ）対応（FR-4.9）

**概要**: `PrivilegeManager` 経由の特権読み取りを `StandardMachOAnalyzer` でも対応し、execute-only パーミッション（読み取り不可・実行のみ）が設定された Mach-O バイナリを解析可能にする。macOS では execute-only バイナリは稀なため優先度は低い。ELF 版タスク 0069 での対応を Mach-O に適用する。

- [ ] `docs/tasks/0101_macho_privileged_access/01_requirements.md` を作成する
- [ ] `docs/tasks/0101_macho_privileged_access/02_architecture.md` を作成する
- [ ] `docs/tasks/0101_macho_privileged_access/03_detailed_specification.md` を作成する
- [ ] `docs/tasks/0101_macho_privileged_access/04_implementation_plan.md` を作成する
- [ ] 実装・テストを行い PR をマージする
