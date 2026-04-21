# 実装計画書: Mach-O 機能パリティ（サブタスク管理）

本書は `01_requirements.md` §8 で定義したサブタスク分割に基づき、各サブタスクの進捗を追跡する。
各サブタスクは独立した番号付きタスクとして `docs/tasks/` 以下に作成する。

## 進捗サマリー

| タスク | 概要 | 優先度 | 状態 |
|--------|------|--------|------|
| 0096 | Mach-O `LC_LOAD_DYLIB` 整合性検証 | 高 | 完了 |
| 0097 | Mach-O `svc #0x80` キャッシュ統合・CGO フォールバック | 中 | 完了 |
| 0098 | Mach-O `.dylib` ベース名による既知ネットワークライブラリ検出 | 中 | 未着手 |
| 0100 | libSystem.dylib syscall ラッパー関数キャッシュ・mprotect 検出 | 中 | 完了 |
| 0104 | Mach-O arm64 syscall 番号解析（0097 の偽陽性修正） | 高 | 未着手 |
| 0099 | Mach-O `mprotect(PROT_EXEC)` 直接 svc 検出（**0104 完了後に実施**） |  中 | 未着手 |
| 0101 | Mach-O 特権アクセス（execute-only バイナリ）対応 | 低 | 未着手 |

---

## フェーズ 1: 基盤・高優先度

### タスク 0096: Mach-O `LC_LOAD_DYLIB` 整合性検証（FR-4.3）

**概要**: `record` 実行時に Mach-O バイナリの依存ライブラリ（`LC_LOAD_DYLIB` / `LC_LOAD_WEAK_DYLIB`）のパスを解決してハッシュを記録し、`runner` 実行時に照合することで供給チェーン攻撃を検出する。ELF の `DT_NEEDED` 整合性検証（タスク 0074）の Mach-O 版。`@executable_path` / `@loader_path` / `@rpath` トークンの展開と `LC_RPATH` の走査を含む。dyld shared cache 内のシステムライブラリ（`libSystem.dylib` 等）はハッシュ検証をスキップしてコード署名検証に委譲する。

- [x] `docs/tasks/0096_macho_lc_load_dylib_integrity/01_requirements.md` を作成する
- [x] `docs/tasks/0096_macho_lc_load_dylib_integrity/02_architecture.md` を作成する
- [x] `docs/tasks/0096_macho_lc_load_dylib_integrity/03_detailed_specification.md` を作成する
- [x] `docs/tasks/0096_macho_lc_load_dylib_integrity/04_implementation_plan.md` を作成する
- [x] 実装・テストを行い PR をマージする

---

## フェーズ 2: 検出力強化

### タスク 0097: Mach-O svc #0x80 キャッシュ統合・キャッシュ優先判定（FR-4.4 / FR-4.5）

**概要**: `svc #0x80` スキャン結果を `fileanalysis.Record.SyscallAnalysis` に保存してキャッシュとして活用し live 再解析を最小化する（FR-4.4）。`SymbolAnalysis = NoNetworkSymbols` の Mach-O バイナリに対して `runner` が `SyscallAnalysis` キャッシュを優先参照し、SymbolAnalysis キャッシュヒット時に svc スキャンが迂回される問題を解消する（FR-4.5）。CGO フォールバックは本タスクのスコープ外。`svc #0x80` は正規 macOS バイナリでは現れないため syscall 番号解析は行わず、`svc #0x80` の存在自体を一律高リスクとして扱う現行方針を維持する。

> **[訂正 - タスク 0104]** 「`svc #0x80` は正規 macOS バイナリでは現れない」という前提は誤りであることが判明した。Go ランタイムは macOS arm64 においても `svc #0x80` を直接発行するため、本タスクの「一律高リスク」方針は正規 Go バイナリに対して偽陽性を生じさせる。タスク 0104 にて syscall 番号解析による方針変更を行う。

- [x] `docs/tasks/0097_macho_arm64_syscall_analysis/01_requirements.md` を作成する
- [x] `docs/tasks/0097_macho_arm64_syscall_analysis/02_architecture.md` を作成する
- [x] `docs/tasks/0097_macho_arm64_syscall_analysis/03_detailed_specification.md` を作成する
- [x] `docs/tasks/0097_macho_arm64_syscall_analysis/04_implementation_plan.md` を作成する
- [x] 実装・テストを行い PR をマージする

### タスク 0098: Mach-O `.dylib` ベース名による既知ネットワークライブラリ検出（FR-4.8）

**概要**: `LC_LOAD_DYLIB` のインストール名（例: `/usr/local/opt/ruby/lib/libruby.3.2.dylib`）からベース名を抽出し、ELF 版の `known_network_libs.go` が持つ既知ネットワークライブラリ・言語ランタイムのプレフィックスリストと照合する。Mach-O 側で追加するのはインストール名 → ベース名の正規化のみ。タスク 0082（方策 C）の Mach-O 版。

- [x] `docs/tasks/0098_macho_known_network_libs/01_requirements.md` を作成する
- [-] `docs/tasks/0098_macho_known_network_libs/02_architecture.md` を作成する（変更最小のため省略）
- [-] `docs/tasks/0098_macho_known_network_libs/03_detailed_specification.md` を作成する（変更最小のため省略）
- [x] `docs/tasks/0098_macho_known_network_libs/04_implementation_plan.md` を作成する
- [ ] 実装・テストを行い PR をマージする

---

## フェーズ 3: 補完

### タスク 0100: libSystem.dylib syscall ラッパー関数キャッシュ（FR-4.7）

**概要**: タスク 0079（ELF 版 libc syscall ラッパーキャッシュ）の macOS 版。`libSystem.dylib`（実体は `libsystem_kernel.dylib`）の各エクスポート関数に syscall 解析を適用し、関数名 → syscall 番号のキャッシュを構築する。ファイルシステム上に存在しない場合は `blacktop/ipsw` の `pkg/dyld` を用いて dyld shared cache から抽出して解析する（dyld shared cache 対応は本タスクのスコープに含める。段階的リリースは採用しない）。mprotect 検出（FR-4.6）は本タスクのスコープ外とするが、syscall テーブルに `mprotect`（番号 74）を含めることで libSystem 経由の呼び出しは自然に検出される（詳細は `01_requirements.md` セクション 8 参照）。

**0099 より先に実装する理由**: macOS では通常バイナリは `libSystem.dylib` 経由で `mprotect` を呼ぶため、`svc #0x80` の直接スキャン（タスク 0099）より libSystem.dylib シンボル経由の検出が実用的な攻撃ベクタに直接対応する。また、タスク 0097 で「`svc #0x80` の存在自体を一律ハイリスク」と確定済みであり、直接 svc スキャンによる mprotect 引数判定（0099）はリスク判定を変えない。

> **[訂正 - タスク 0104]** 後半の理由（「タスク 0097 で一律ハイリスク確定済みのため 0099 はリスク判定を変えない」）は、タスク 0104 の完了により無効となる。タスク 0104 以降は `mprotect` (syscall 74) の直接 `svc #0x80` はネットワーク syscall ではないため高リスク扱いされなくなる。タスク 0099 は 0104 完了後に実施しなければ検出能力の後退を招く（詳細は下記タスク 0099 参照）。

- [x] `docs/tasks/0100_macho_libsystem_syscall_cache/01_requirements.md` を作成する
- [x] `docs/tasks/0100_macho_libsystem_syscall_cache/02_architecture.md` を作成する
- [x] `docs/tasks/0100_macho_libsystem_syscall_cache/03_detailed_specification.md` を作成する
- [x] `docs/tasks/0100_macho_libsystem_syscall_cache/04_implementation_plan.md` を作成する
- [x] 実装・テストを行い PR をマージする

### タスク 0104: Mach-O arm64 syscall 番号解析（FR-4.4 / FR-4.5 の方針修正）

**概要**: タスク 0097 の「`svc #0x80` = 一律ハイリスク」方針を廃止し、X16 レジスタ後方スキャンによる syscall 番号解析に置き換える。Go ランタイムが macOS arm64 でも `svc #0x80` を直接発行するため、正規の Go バイナリ（本システムの `record` コマンドを含む）が誤ってハイリスク判定される偽陽性を解消する。ELF 版（タスク 0070/0072）と同様の 2 パス構成（Pass 1: X16 後方スキャン、Pass 2: Go ラッパー呼び出しサイト解析）を採用し、ネットワーク関連 syscall のみを高リスクとする。スキーマバージョンを v15 → v16 に変更する。

**実装前提**: タスク 0097（スキーマ基盤・キャッシュ統合）が完了済みであること。

**タスク 0099 との関係**: 本タスク完了後、`mprotect` (syscall 74) の `svc #0x80` はネットワーク syscall ではないため高リスク扱いされなくなる。タスク 0099 は本タスク完了直後に実施し、`mprotect(PROT_EXEC)` の検出能力を回復する必要がある（詳細は下記タスク 0099 参照）。

- [x] `docs/tasks/0104_macho_syscall_number_analysis/01_requirements.md` を作成する
- [ ] `docs/tasks/0104_macho_syscall_number_analysis/02_architecture.md` を作成する
- [ ] `docs/tasks/0104_macho_syscall_number_analysis/03_detailed_specification.md` を作成する
- [ ] `docs/tasks/0104_macho_syscall_number_analysis/04_implementation_plan.md` を作成する
- [ ] 実装・テストを行い PR をマージする

---

### タスク 0099: Mach-O `mprotect(PROT_EXEC)` 直接 svc 検出（FR-4.6 後半）

**概要**: Mach-O の `__TEXT,__text` セクションで `svc #0x80`（`mprotect` = BSD syscall 74）を検出し、`x2` レジスタ（第3引数 `prot`）に `PROT_EXEC`（`0x4`）が設定されているかを後方スキャンで確認する。タスク 0104 の X16 後方スキャン基盤を共用して `mprotect` を特定し、x2 後方スキャンで引数を評価する。結果は `fileanalysis.Record.SyscallAnalysis.ArgEvalResults` に保存する。タスク 0078（ELF 版）の Mach-O 対応。

**実装前提**: タスク 0104（syscall 番号解析）が完了していること。タスク 0104 の X16 解析によって `mprotect` (syscall 74) を特定できて初めて x2 引数スキャンが成立する。

**優先度の変更**: タスク 0097 の「`svc #0x80` = 一律ハイリスク」が廃止（タスク 0104）されると、`mprotect(PROT_EXEC)` の直接 `svc #0x80` 経由呼び出しはハイリスク扱いされなくなる。タスク 0099 はその検出能力を回復するために、**タスク 0104 完了直後に実施しなければならない**。かつて「保留」としていた理由（「タスク 0097 で既にハイリスク扱い済みのため追加価値なし」）はタスク 0104 の完了により消滅する。

- [ ] `docs/tasks/0099_macho_mprotect_exec_detection/01_requirements.md` を作成する
- [ ] `docs/tasks/0099_macho_mprotect_exec_detection/02_architecture.md` を作成する
- [ ] `docs/tasks/0099_macho_mprotect_exec_detection/03_detailed_specification.md` を作成する
- [ ] `docs/tasks/0099_macho_mprotect_exec_detection/04_implementation_plan.md` を作成する
- [ ] 実装・テストを行い PR をマージする

### タスク 0101: Mach-O 特権アクセス（execute-only バイナリ）対応（FR-4.9）

**概要**: `PrivilegeManager` 経由の特権読み取りを `StandardMachOAnalyzer` でも対応し、execute-only パーミッション（読み取り不可・実行のみ）が設定された Mach-O バイナリを解析可能にする。macOS では execute-only バイナリは稀なため優先度は低い。ELF 版タスク 0069 での対応を Mach-O に適用する。

- [ ] `docs/tasks/0101_macho_privileged_access/01_requirements.md` を作成する
- [ ] `docs/tasks/0101_macho_privileged_access/02_architecture.md` を作成する
- [ ] `docs/tasks/0101_macho_privileged_access/03_detailed_specification.md` を作成する
- [ ] `docs/tasks/0101_macho_privileged_access/04_implementation_plan.md` を作成する
- [ ] 実装・テストを行い PR をマージする
