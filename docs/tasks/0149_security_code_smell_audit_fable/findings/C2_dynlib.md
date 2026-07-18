# C2: dynlib / libccache パッケージ セキュリティ監査

- 監査日: 2026-07-19
- 対象:
  - `internal/dynlib/` (resolve.go, errors.go)
  - `internal/dynlib/elfdynlib/` (analyzer.go, resolver.go, ldcache.go, verifier.go, default_paths.go, errors.go)
  - `internal/dynlib/machodylib/` (analyzer.go, resolver.go, libsystem_resolver.go, dyld_extractor_darwin.go, shared_cache_darwin.go, dyld_layout_darwin.go, errors.go)
  - `internal/libccache/` (cache.go, macho_cache.go, analyzer.go, macho_analyzer.go, matcher.go, adapters.go, schema.go, macos_syscall_table.go, macos_syscall_numbers.go, errors.go)
- 方法: 静的コードレビュー（読み取り専用）。動的ライブラリ解決の攻撃面（RUNPATH/$ORIGIN/@rpath 展開、シンボリックリンク、探索順）、record→verify の整合性、fail-open 方向のエラーハンドリング、バイナリパーサの境界条件、キャッシュ改ざん耐性を中心に確認。呼び出し元（`cmd/record/main.go`, `internal/verification/manager.go`）も参照した。

## サマリ

| 重大度 | 件数 |
|---|---|
| 🔴 High | 0 |
| 🟡 Medium | 3 |
| 🟠 Low | 3 |
| 🔵 Info | 5 |

このパッケージ群は「record 時に依存ライブラリツリーを解決してハッシュを記録し、verify 時に記録済みハッシュを照合する」二段構えの中核。パーサの境界チェック・上限値・fail-closed の方向付けは全体的に丁寧で、High 相当の欠陥は見つからなかった。主な残存リスクは (1) verify 時に解決を再実行しないことによる探索順シャドーイング、(2) record 時の解決結果が実際のローダ挙動と乖離し得る箇所（ld.so.cache のフラグ無視、権限エラーのフォールスルー）、(3) 子依存パース失敗の fail-soft による記録漏れ、の3系統。

---

## 所見

### F-1 🟡Medium: verify 時に依存解決を再実行しないため、探索順シャドーイングを検出できない

- 該当箇所: `internal/dynlib/elfdynlib/verifier.go:31-63` (`DynLibVerifier.Verify`)、呼び出し元 `internal/verification/manager.go:645-660`
- 問題: verify 時の検証は「record 時に解決されたパス群のハッシュ照合」のみで、ローダの探索アルゴリズムを再実行しない。record 時に存在しなかったファイルが、より優先順位の高い探索位置（DT_RUNPATH の `$ORIGIN` 相対ディレクトリ、Mach-O の `@rpath` 候補ディレクトリ等）に後から置かれた場合、記録済みライブラリには一切触れずに実際のロード対象を差し替えられる。
- 悪用/再現シナリオ: コマンドバイナリが `RUNPATH=$ORIGIN/../lib` を持ち、record 時には `../lib` が空で依存が `/usr/lib` から解決・記録されたとする。攻撃者が後から `../lib/libfoo.so` に悪性ライブラリを書き込める（バイナリ設置ディレクトリの隣接ディレクトリが書込可能な）場合、ld.so は RUNPATH 側を先にロードするが、Verify は記録済みの `/usr/lib/libfoo.so` を照合して成功する。バイナリ本体はハッシュ検証されるため RUNPATH 値自体は改ざんできないが、探索先ディレクトリの「中身の追加」は検知されない。
- 緩和要因: デフォルト探索パス（`/lib`, `/usr/lib` 等）は通常 root 所有。攻撃には RUNPATH/@rpath が指すディレクトリへの書込権限が必要。`docs/security/README.md` の脅威モデル記述（ld.so.cache 改ざんはスコープ外）と同系統の制約として整理は可能。
- 推奨対応: verify 時にも `Analyze` 相当の解決を再実行し「解決パス集合が record と一致すること」を確認する（差分があれば fail）。それが重い場合は、record 時に RUNPATH/@rpath 探索ディレクトリの所有者・書込権限を検査して警告する層を追加し、脅威モデル文書に本制約を明記する。

### F-2 🟡Medium: ld.so.cache の Lookup が Flags/HWCap/OSVersion を無視し、multilib 環境で誤ったライブラリを記録し得る

- 該当箇所: `internal/dynlib/elfdynlib/ldcache.go:145-182`（`newCacheEntry.Flags` 等を読み取るが未使用、`Lookup` は soname のみで照合、「First entry wins」）、`internal/dynlib/elfdynlib/resolver.go:60-66`
- 問題: glibc の ld.so は cache エントリ選択時に flags（アーキテクチャ/ABI）、hwcap、osversion を照合するが、本実装は soname の最初のエントリを無条件に採用する。biarch/multilib 環境（x86-64 + i386 など）では同一 soname が複数エントリ（例: `/lib/x86_64-linux-gnu/libc.so.6` と `/lib32/libc.so.6`）で存在し、ファイル内の並び順次第で解析対象バイナリのアーキテクチャと異なるライブラリのパスが返る。
- 悪用/再現シナリオ: multilib 環境で record すると、実際に ld.so がロードする 64-bit ライブラリではなく 32-bit 側のパスとハッシュが記録される可能性がある。以後 verify は「ロードされないファイル」を検証し続け、実ロード対象のライブラリは検証対象外になる（改ざんされても検知されない）。攻撃者の能動的操作がなくても環境依存で発生し得る。
- 緩和要因: cache 照合失敗時はアーキテクチャ別デフォルトパスにフォールスルーするため、多くの構成では正しいパスに到達する。ただし cache が「間違ったが存在するパス」を返した場合はフォールスルーせずそこで確定する。
- 推奨対応: `newCacheEntry.Flags` の ELF クラス/マシン種別ビットを `LibraryResolver` の `elf.Machine` と照合してから採用する。少なくとも解析対象バイナリの ELF クラス（32/64bit）とエントリの flags が矛盾するエントリはスキップする。

### F-3 🟡Medium: 子依存のパース失敗が fail-soft（Debug ログ + skip）で、遷移的依存の記録漏れを許す

- 該当箇所:
  - `internal/dynlib/elfdynlib/analyzer.go:207-218`（子 ELF パース失敗を `slog.Debug` で握って traversal をスキップ）
  - `internal/dynlib/elfdynlib/analyzer.go:115-127`（トップレベルでも `elf.NewFile` 失敗・`DynString` エラーを `nil, nil`＝「依存なし」に縮退。`//nolint:nilerr`）
  - `internal/dynlib/machodylib/analyzer.go:215-221`（`parseMachODeps` 失敗を `slog.Debug` で握る）
- 問題: 解決済みライブラリ（ハッシュは記録される）の子依存パースに失敗すると、そのサブツリー全体が記録から漏れ、以後 verify の対象にならない。Go の `debug/elf`/`debug/macho` は glibc/dyld より厳格でない部分と厳格な部分の両方があり、「ローダはロードできるが Go パーサは失敗する」よう細工されたライブラリを依存ツリーに混ぜると、その先の依存を検証対象から外せる。トップレベルの `DynString(DT_NEEDED)` エラーを「依存なし」と同一視する分岐も同様に fail-open 方向。
- 悪用/再現シナリオ: record 対象バイナリの依存ライブラリの1つ（攻撃者が事前に用意した正規配置のライブラリ）が、動的セクションの一部を Go パーサがエラーにする形式で保持しつつ ld.so では正常にロードされるとする。record は当該ライブラリのハッシュのみ記録し、その DT_NEEDED 先（攻撃者の実ペイロード）は未記録・未検証となる。
- 緩和要因: 記録漏れしたライブラリを後から差し替えても、親ライブラリ自身のハッシュは検証される。攻撃には record 時点で解決パス上に細工済みライブラリを置ける前提が必要で、その時点でかなり強い権限を持つ。ELF マジックを持つファイルのトップレベルパース失敗は Mach-O 側の `HasDynamicLibDeps` では `looksLikeMachO` により区別してエラー化しており（良い実装）、非対称になっている。
- 推奨対応: 子依存パース失敗を最低でも `slog.Warn` に格上げし、record の `AnalysisWarnings` 相当に記録して可視化する。トップレベルは「ELF マジックあり + パース失敗」をエラー（fail-closed）にする（Mach-O 側 `HasDynamicLibDeps` と同じ判定方式を ELF にも導入）。

### F-4 🟠Low: ResolveRealPath のエラー種別を呼び出し元が区別せず、permission denied 等も「not found」として次の探索候補へフォールスルーする

- 該当箇所: `internal/dynlib/resolve.go:18-26`、呼び出し元 `internal/dynlib/elfdynlib/resolver.go:50-77`、`internal/dynlib/machodylib/resolver.go:47-140`
- 問題: `ResolveRealPath` の doc コメントは「os.Lstat で not-found と permission denied 等を区別する」と述べるが、実際には両 resolver とも `err == nil` 以外を一律「候補になし」として次の探索位置に進む。EACCES・ELOOP・EIO 等が発生した候補が本来のロード対象だった場合、record は別の（優先順位の低い）パスのライブラリを記録し、実ローダの挙動と乖離する。
- 悪用/再現シナリオ: RUNPATH ディレクトリの実行権限を落とせる攻撃者（または偶発的な権限設定ミス）により、record 時のみ第1候補が Lstat 失敗 → デフォルトパス側が記録される。実行時に権限が戻っていれば ld.so は RUNPATH 側をロードし、verify をすり抜ける（F-1 と複合）。
- 推奨対応: `os.IsNotExist` 以外のエラーは探索を継続せずエラーとして伝播する（record は fail-closed が原則）。doc コメントと実装の乖離を解消する。

### F-5 🟠Low: HasDynamicLibDeps のシーク失敗・短読みが (false, nil) に縮退する fail-open

- 該当箇所: `internal/dynlib/machodylib/analyzer.go:617-632`（`Seek` 失敗 → `return false, nil`、`io.ReadFull` 失敗 → `return false, nil`）
- 問題: この関数は「DynLibDeps が記録されているべきなのに欠けている Mach-O」を runner 側で検出するためのゲートだが、シーク失敗や読み取り失敗を「依存なし」と同じ戻り値に落とす。I/O エラーを誘発できる状況では、DynLibDeps 未記録バイナリの検出（`ErrDynLibDepsRequired` 相当の強制）を黙ってスキップさせられる。
- 緩和要因: `SafeOpenFile` 成功後の Seek/Read 失敗は通常起こりにくい（FUSE・NFS 等の特殊環境が主）。Fat スライスのパース失敗や「Mach-O マジックありのパース失敗」はエラー化されており、主要経路は fail-closed。
- 推奨対応: Seek/ReadFull 失敗はエラーとして返す（`false, nil` ではなく `false, err`）。

### F-6 🟠Low: libccache のキャッシュ読み込みが自己申告値のみで妥当性判定され、safefileio を経由しない

- 該当箇所: `internal/libccache/cache.go:60-68`（`os.ReadFile` + `SchemaVersion`/`LibHash` 一致で即採用）、`internal/libccache/macho_cache.go:52-60`（同様）、`cacheDirPerm = 0o755` / `cacheFilePerm = 0o644`（cache.go:17-19）
- 問題: キャッシュファイルの正当性は「JSON 内の SchemaVersion と LibHash がメモリ上の期待値と一致するか」だけで判定される。LibHash は対象 libc の公開ハッシュであり攻撃者も計算できるため、キャッシュディレクトリに書込できる者は `SyscallWrappers` を空にした/network syscall を除いた偽キャッシュを事前配置でき、record 時の syscall 検出（ネットワーク/exec リスク評価）を抑制できる。読み込みは `os.ReadFile` でシンボリックリンク追従・所有者検査なし。
- 緩和要因: `cacheDir` は `hashDir/lib-cache` で、`cmd/record/main.go:133-137` が record 実行前に `RunTOCTOUPermissionCheck` で hashDir とその祖先の権限・所有者を fail-closed 検査している。ディレクトリ/ファイルとも group/world 書込不可の権限で作成される。したがって前提が破れるのは hashDir 自体が奪取された場合で、その時点で hash 記録全体が改ざん可能。
- 推奨対応: 多層防御として、キャッシュ読み込みも `safefileio` 経由（symlink 拒否・regular file 検査）にする。`lib-cache` サブディレクトリ自体を TOCTOU 検査対象に含める（現在の検査対象は hashDir で、`MkdirAll` で後から作られる `lib-cache` の既存ディレクトリ所有者は未検査）。

### F-7 🔵Info: machodylib.Analyze は binaryPath を正規化せず、ELF 側（EvalSymlinks で正規化）と非対称

- 該当箇所: `internal/dynlib/machodylib/analyzer.go:73-93`（`executableDir := filepath.Dir(binaryPath)` を未正規化のまま `@executable_path` 展開に使用）、対比: `internal/dynlib/elfdynlib/analyzer.go:100-105`
- 問題: シンボリックリンク経由のパスで record すると `@executable_path`/`@loader_path` の展開基準がリンク位置になり、実体パスで record した場合と解決結果が変わり得る。dyld の実挙動（実行時に指定されたパス基準）にはむしろ近いが、ELF 側と設計が非対称で、同一バイナリに対する記録の再現性が呼び出しパスに依存する。
- 推奨対応: どちらの正規化ポリシーを正とするかを決めてコメントで明文化し、両実装を揃える。

### F-8 🔵Info: rpathName の pathOffset に下限チェックがない

- 該当箇所: `internal/dynlib/machodylib/analyzer.go:442-461`（`pathOffset >= len(raw)` のみ検査。`dylibName` は `>= dylibCmdHeaderSize` の下限検査あり）
- 問題: 細工された LC_RPATH で `path_offset < 12` を指定すると、ロードコマンドヘッダのバイト列が rpath 文字列として解釈される（例: offset=0 なら cmd バイト `\x1c` が rpath になる）。panic や領域外読みには至らず、ゴミ rpath が探索候補に増えるだけだが、`dylibName` と対称な下限チェック（`>= 12`）を入れるのが望ましい。

### F-9 🔵Info: dyld 共有キャッシュ抽出はハードコードされたヘッダオフセットと SIP 保護前提に依存

- 該当箇所: `internal/dynlib/machodylib/dyld_extractor_darwin.go:45-52`（オフセット 392/396 等は macOS 13+ レイアウト前提）、`:127-136`/`:401-410`/`:474`/`:571`/`:601`（`os.Open` 直使用。SIP 保護を根拠とするコメントあり）、`:757`（`oldSectOff - uint32(oldTextFileOff)` は理論上アンダーフロー可能）
- 評価: 入力は SIP 保護されたシステムファイルに限定され、`errImplausibleCount`/`errImplausibleSizeCmds`/64MB 読み取り上限/13万シンボル上限など過大確保対策も揃っている。オフセットのドリフトは cgo テスト（`dyld_layout_darwin.go` + `dyld_offsets_test.go`）で SDK ヘッダと突き合わせて検出される。将来の macOS でレイアウトが変わった場合も抽出失敗 → `nil, nil` → シンボル名マッチングへのフォールバックであり、fail-open にはなるが panic はしない設計。フォールバック時の検出精度低下（`fallbackNameMatch` はネットワーク系のみ）は仕様として文書化されているか確認しておくとよい。

### F-10 🔵Info: matcher.Match と MatchWithMethod がほぼ完全な重複（DRY 違反）

- 該当箇所: `internal/libccache/matcher.go:34-127`
- 問題: 両関数は `DeterminationMethod`/`Source` の定数以外同一ロジック。将来どちらか一方だけ修正されると、ELF 経路と Mach-O 経路で dedup 規則や優先順位が乖離するリスクがある。`Match` を `MatchWithMethod` への委譲（+ Source をパラメータ化）に統合することを推奨。

### F-11 🔵Info: verify 通過後〜実行までの本質的 TOCTOU ウィンドウ（設計上の既知制約）

- 該当箇所: `internal/dynlib/elfdynlib/verifier.go`（Verify）と runner の exec の間
- 評価: Verify がハッシュ照合した後、実際に ld.so/dyld がライブラリを mmap するまでの間にファイルを差し替えられれば検証は無意味になる。これはハッシュ事前検証方式の本質的限界であり、root 所有ディレクトリ前提で許容されている既知の設計制約と思われる。脅威モデル文書に「検証対象ライブラリの設置ディレクトリは信頼境界内（root 所有・非書込可能）であること」を運用要件として明記されているか確認を推奨（F-1 の推奨対応と同根）。

---

## 観察された良好な防御層

- **DT_RPATH の fail-closed 拒否**: バイナリ本体・遷移的依存のどちらに DT_RPATH があっても `ErrDTRPATHNotSupported` で record を中断（`elfdynlib/analyzer.go:134-140, 292-298`）。挙動が複雑な legacy 探索順を仕様ごと排除しており堅実。
- **LD_LIBRARY_PATH / DYLD_* を探索から除外**: resolver は環境変数由来の探索パスを一切参照しない（record は無視、runner は環境変数をクリアする前提がコメントで明記。`elfdynlib/resolver.go:36-41`）。環境変数経由のライブラリ差し替え攻撃面を構造的に遮断。
- **ld.so.cache パーサの境界防御**: uint64 で計算してから境界検査する整数オーバーフロー対策（32-bit 対応込み）、`errDataTruncated` 等の明示的エラー、`extractCString` の範囲検査（`ldcache.go:120-176, 185-194`）。cache 不在・不正形式はデフォルトパスへの明示的フォールバックで、panic 経路がない。
- **dyld キャッシュ抽出のサニティ上限**: 画像数 8192、サブキャッシュ 256、sizeofcmds 1MB、読み取り 64MB、シンボル 131072 の各上限で過大メモリ確保・無限ループを防止（`dyld_extractor_darwin.go`）。cgo による SDK ヘッダとのレイアウト一致テストも整備。
- **再帰深度制限と多段 dedup**: `MaxRecursionDepth=20`、ELF は解決済みパス集合、Mach-O は (installName, loaderPath) キーの BFS dedup + (installName, resolvedPath) の出力 dedup で、循環・爆発を防ぎつつ @rpath の文脈依存解決を正しく保持（`machodylib/analyzer.go:96-139`）。
- **fail-closed の方向付けが要所で正しい**: 強い依存（LC_LOAD_DYLIB）の解決失敗は record 中断、dyld 共有キャッシュ skip は「system prefix + ディスク不在」の 2 条件（permission エラーでは skip しない。`machodylib/analyzer.go:168-179`）、Fat ヘッダあり・スライスパース失敗はエラー化（`analyzer.go:584-588`）。
- **ハッシュ計算の symlink/TOCTOU 対策**: すべてのハッシュ計算・ファイル読みが `safefileio.SafeOpenFile` 経由。Fat バイナリはオープン済み fd を `SectionReader` で再利用し二重オープンの TOCTOU を排除（`machodylib/analyzer.go:299-314`）。
- **キャッシュファイルのアトミック書き込み**: temp file + rename による部分読み防止、失敗時の temp 掃除も `errors.Join` で漏れなく処理（`libccache/cache.go:113-140`）。並行 record プロセス間の競合でも壊れたキャッシュが観測されない。
- **決定的な出力**: 依存リストの (SOName, Path) ソート、wrapper リストの (Number, Name) ソートで record 出力の再現性を確保し、diff レビューを容易にしている。
- **libc キャッシュの妥当性キー**: `SchemaVersion` + 対象ライブラリの `LibHash` の二重チェックで、libc 更新時・スキーマ変更時に自動的に再解析される（`schema.go` にバージョン履歴も記載）。
