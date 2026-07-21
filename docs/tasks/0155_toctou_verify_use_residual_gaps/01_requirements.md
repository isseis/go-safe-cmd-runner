# 要件定義書: 検証(verify)と使用(open/exec)間の TOCTOU 残存窓を閉じる

## Document Status

| Item | Value |
|---|---|
| Status | `approved` |
| Created | 2026-07-21 |
| Review date | 2026-07-21 |
| Reviewer | isseis |
| Comments | - |

## 関連 Issue

- [#862 [Security][P3] 検証(verify)と使用(open/exec)間のTOCTOU残存窓を閉じる](https://github.com/isseis/go-safe-cmd-runner/issues/862)
- 詳細所見:
  - [findings/A5_risk.md](../0149_security_code_smell_audit_fable/findings/A5_risk.md) Medium-1
  - [findings/B1_safefileio.md](../0149_security_code_smell_audit_fable/findings/B1_safefileio.md) F-1
  - [findings/B2_filevalidator.md](../0149_security_code_smell_audit_fable/findings/B2_filevalidator.md) B2-1, B2-3
  - [findings/C2_dynlib.md](../0149_security_code_smell_audit_fable/findings/C2_dynlib.md) F-1
  - [findings/B3_verification.md](../0149_security_code_smell_audit_fable/findings/B3_verification.md) L3, L4
  - 集約サマリ: [99_summary.md](../0149_security_code_smell_audit_fable/99_summary.md)（横断パターン P3）

## 背景

本コードベースは「検証(verify)は開いた fd の内容そのものを検査し、使用(exec/read)はその同じ fd から行う」という原則（fd 束縛実行、`SafeOpenFile` の openat2 symlink 排除、`VerifyAndRead` 等）を主要経路で徹底しており、Issue #862 が参照する集約監査（0149）でも High 相当の欠陥は見つかっていない。しかし、この原則が及んでいない残存箇所が 5 系統ある。いずれも「検証済みパスへの書き込み権限」を前提条件とするため、深層防御が部分的に欠けた状態に留まり、単独では成立しにくいと評価されているが、多層防御の一角として埋めることが望ましい。

1. **A5 Medium-1**: risk 評価の `openVerifiedIdentity` がグループ検証時点のハッシュと open 時点のファイル内容を突き合わせない。グループ内の先行コマンドが長時間実行される場合、この窓は数分〜数時間に及び得る。
2. **B1 F-1**: `AtomicMoveFile` は fd で検証したソースを、最終的な移動では `os.Rename` によりパス名で行う。検証対象と移動対象の同一性を保証しない。
3. **B2 B2-1, B2-3**: record 時のハッシュ計算と各種解析（shebang/dynlib/ELF・Mach-O syscall）が別々の `open` で行われ、ハッシュと解析結果の内容一致を保証しない。
4. **C2 F-1**: verify 時に依存解決（RUNPATH/$ORIGIN、@rpath 探索）を再実行しないため、record 後に探索優先度の高い位置へライブラリが追加された場合の「探索順シャドーイング」を検出できない。
5. **B3 L3, L4**: `PathResolver.validateAndCacheCommand` の `Stat`→`EvalSymlinks` の順序、および shebang インタプリタの symlink 検査とカーネルの exec 時再解決の間に TOCTOU 窓が残る。

これらはいずれも「検証と使用は同一 fd/同一読み取りから行う」という単一の設計原則の適用漏れであり、まとめて対処することで原則の一貫性を回復する。

## 目的

- `openVerifiedIdentity` が open した fd の内容をグループ検証時のハッシュと再照合し、不一致を fail-closed（Blocking）で扱うようにする。
- `AtomicMoveFile` が検証済みのソースと実際に移動するファイルの同一性を保証するようにする。
- record 時のハッシュ計算と内容解析（shebang/dynlib/syscall）が同一の読み取りに基づくようにする。
- verify 時の依存ライブラリ検証が、record 時と同じ探索結果が得られることを確認するようにする。
- `PathResolver` の実行可否チェックと解決済みパスのキャッシュが同一のパス解決結果に基づくようにする。
- 構造的に排除できない残存窓（shebang インタプリタの exec 時再解決）は、前提条件と残余リスクを設計/セキュリティ文書に明記する。
- 各修正が正常系（改ざんなし）の record/verify/実行フローの成功結果・出力内容に副作用を与えないことをテストで保証する。

## スコープ

### 対象（本タスクで対応する）

1. **A5 Medium-1**: `openVerifiedIdentity`（`internal/runner/base/risk/evaluator.go:570-580`）が `syscall.Open` した fd の内容ハッシュを `cmd.ExpandedCmdContentHash` と照合しない問題。付随して `O_NONBLOCK` なし（FIFO 差し替えによる無期限ブロック）、open 後の `fstat` による通常ファイル確認なしの問題。
2. **B1 F-1**: `atomicMoveFileCore`（`internal/safefileio/safe_file.go:140-`）が fd で検証したソースを `os.Rename(absSrc, absDst)` とパス名で移動する問題。
3. **B2 B2-1, B2-3**: `SaveRecord`/`saveRecordCore`（`internal/filevalidator/validator.go:365-411`）のハッシュ計算・shebang 解析・各種解析（`analyzeRecordTarget`, `validator.go:532-562`）が独立した `open` で行われる問題、および `analyzeOneLibrary`（`validator.go:713-776`）のサイズ検査（Stat）と実解析が別 open で、解析結果がハッシュキー（`lib.Hash`）と対応する保証がない問題。
4. **C2 F-1**: `DynLibVerifier.Verify`（`internal/dynlib/elfdynlib/verifier.go:30-`）が record 済みパス群のハッシュ照合のみを行い、ローダの探索アルゴリズムを再実行しない問題。
5. **B3 L3**: `PathResolver.validateAndCacheCommand`（`internal/verification/path_resolver.go:31-53`）の `os.Stat` と `filepath.EvalSymlinks` が別システムコールで、両者の間にリンク先が差し替えられ得る問題。
6. **B3 L4**: `verifyInterpreterSymlinkTarget`（`internal/verification/manager.go:923-`）の symlink 検査と、実際のスクリプト実行時にカーネルが shebang パスを再解決するタイミングの間に残る TOCTOU 窓。完全な排除は困難なため、残余リスクとしての文書化を対応方針とする。

### 対象外（別 Issue・別タスクとする、または本タスクでは対応しない）

- A5 の Low/Info 所見（Low-1〜4, Info-1〜3）: Issue #862 の「該当箇所」リストに含まれない、TOCTOU とは異なる系統の所見（廃止済み設定項目、audit 情報の非一貫性、switch の default 欠如等）。
- B1 の F-2〜F-9（F-1 を除く）: 非 Linux フォールバック経路の symlink TOCTOU（F-2）、fd リーク（F-3）、ロールバック欠如（F-4）、`Remove` の安全性契約（F-5）等は Issue #862 の「該当箇所」リストに含まれない別系統の所見。
- B2 の B2-2, B2-4〜B2-13: 解析器無効時の温存ロジック（B2-2）、エラーハンドリング（B2-4, B2-5）、Mach-O 解析縮退（B2-6）等は Issue #862 の「該当箇所」リストに含まれない。
- C2 の F-2〜F-6, Info 所見: ld.so.cache の Flags/HWCap 無視（F-2）、子依存パース失敗の fail-soft（F-3）、`ResolveRealPath` のエラー種別非区別（F-4）等は Issue #862 の「該当箇所」リストに含まれない別系統の所見。
- B3 の M1, M2, L1, L2, Info 所見: `collectVerificationFiles` の fail-open（M1）、`isDeferredHashDirUnavailable` のゲート漏れ（M2）等は Issue #862 の「該当箇所」リストに含まれない別系統の所見（TOCTOU ではなく fail-open/fail-closed 設計の一貫性の問題）。
- B3 L4 のコード側の完全排除（`execveat` 等によるインタプリタの fd 束縛起動）: 完全な排除は困難と評価されているため、本タスクでは残余リスクの文書化のみを対応範囲とする。将来的にコード側で対応する場合は別タスクとする。

## 現状の問題点（詳細）

### 1. A5 Medium-1: ハッシュ検証時点と open 時点の間の窓

実行バイナリのハッシュ検証（`VerifyGroupFiles`）はグループ開始時に一括で行われ、`cmd.ExpandedCmdContentHash` に伝播される。一方、fd 束縛実行用のディスクリプタは各コマンドのリスク評価時に `openVerifiedIdentity`（`evaluator.go:570-580`）でパス指定により再 open される。open 後の窓は fd 束縛実行（`fdexec_linux.go`）で閉じられているが、「ハッシュ検証時点」と「open 時点」の間の窓は残る。グループ内に長時間走るコマンドがあれば、この窓は数分〜数時間になり得る。

### 2. B1 F-1: AtomicMoveFile が fd で検証したソースをパスで rename する

ソースファイルは `SafeOpenFile`（openat2 `RESOLVE_NO_SYMLINKS`）で開き、fchmod・所有権/権限検証もその fd に対して行う。しかし最終的な移動は `os.Rename(absSrc, absDst)` とパス名で実行するため、検証対象と移動対象が同一である保証がない。

### 3. B2 B2-1, B2-3: record 時のハッシュ計算と解析が別々の読み取りで行われる

`SaveRecord` のフローでは、(1) shebang 解析、(2) `calculateHash` によるハッシュ計算、(3) dynlib/シンボル/syscall の各解析が、いずれも同一パスを独立に open して読む。単一の fd やメモリ上の内容を共有していないため、読み取りの合間にファイルが差し替えられると、`ContentHash` が指す内容と解析結果が食い違ったレコードが永続化され得る。`analyzeOneLibrary` も同様に、サイズ検査（Stat）用の open と実解析用の再 open の間に TOCTOU があり、解析結果がハッシュキー（`lib.Hash`）に対応する内容の解析結果である保証がない。

### 4. C2 F-1: verify 時に依存解決を再実行しない

verify 時の検証は「record 時に解決されたパス群のハッシュ照合」のみで、ローダの探索アルゴリズムを再実行しない。record 時に存在しなかったファイルが、より優先順位の高い探索位置（`DT_RUNPATH` の `$ORIGIN` 相対ディレクトリ、Mach-O の `@rpath` 候補ディレクトリ等）に後から置かれた場合、記録済みライブラリには一切触れずに実際のロード対象を差し替えられる。

### 5. B3 L3: PathResolver の Stat→EvalSymlinks 間の TOCTOU

`validateAndCacheCommand` は `os.Stat`（symlink 追従）で存在・regular・実行ビットを確認した後、別システムコールの `filepath.EvalSymlinks` で正規化する。2 呼び出しの間にリンク先を差し替えられると、実行可否チェックとキャッシュされる解決済みパスが別ファイルを指し得る。

### 6. B3 L4: shebang symlink 検査と exec の間の残存窓

`/bin/sh` 等のインタプリタパスを `filepath.EvalSymlinks` で検査し record 時の解決先と比較するが、実際のスクリプト実行時にはカーネルが exec 時点で shebang パスを再解決する。検査合格後〜exec の間にシンボリックリンクを差し替えられると、検証済みでないインタプリタが起動し得る。この窓はカーネル側の exec 時解決に起因するため、アプリケーション層のみでの完全排除は困難と評価されている。

## Acceptance Criteria

#### F-001: openVerifiedIdentity のハッシュ再検証

- **AC-01**: `openVerifiedIdentity` は、open した fd の内容から実測ハッシュを計算し、`cmd.ExpandedCmdContentHash` と比較する。不一致の場合、fd を close した上でエラーを返し、呼び出し元のリスク評価は fail-closed（Blocking）判定になる。
- **AC-02**: open 後の `fstat` により通常ファイルであることを確認する（主要な保証）。`O_NONBLOCK` はパスが FIFO に差し替えられた場合の無期限ブロックを防ぐ安全網として open 時に付与し、`fstat` による通常ファイル確認後に解除する。通常ファイルに対する `O_NONBLOCK` は no-op であり、保証の本質は `fstat` にある点に注意すること。
- **AC-03**: 改ざんがない正常系（open 時点の内容が検証済みハッシュと一致する）では、従来どおり fd 束縛実行に必要な `VerifiedIdentity` が返る（既存の成功経路に回帰がない）。
- **AC-04**: ハッシュ不一致・FIFO 検出等による拒否は、他の identity gate 失敗（`ReasonIdentityUnbound` 等）と区別可能な reason code で audit ログに記録される。

#### F-002: AtomicMoveFile のソース同一性保証

- **AC-05**: `AtomicMoveFile`（`atomicMoveFileCore`）は、検証済みソース fd と実際に rename されるファイルが同一の inode であることを、rename 直前に取得した `(dev, ino)` の突き合わせ、または同等の原子性を持つ方式で保証する。なお、`os.Rename(path, path)` に先立つ stat 系呼び出しによる `(dev, ino)` 照合のみでは、rename がパス名で解決される以上、検査時点と rename 時点の間に別 inode への差し替えを許す TOCTOU 窓が残る。この窓を閉じるには fd アンカー方式が必要であり、採用する具体的な方式は architecture 文書で確定する。本セキュリティ保証は、移動先ファイルの親ディレクトリが信頼できる所有者（root または厳格な権限（例：0o710）を持つ信頼できるユーザー）によって保護されていることを前提としている。
- **AC-06**: 同一性が確認できない場合（ソースパスが検証後に差し替えられた場合）、rename を行わずエラーを返す（fail-closed）。改ざんがない正常系の移動は従来どおり成功する。

#### F-003: record 時のハッシュ計算と解析の一貫性

- **AC-07**: `SaveRecord`/`saveRecordCore` における対象ファイルのハッシュ計算と内容解析（shebang/dynlib/ELF・Mach-O syscall 解析）は、同一の読み取り（共有 fd または読み込み済みバイト列）に基づいて行われる。読み取りの合間にファイルが差し替えられても、記録される `ContentHash` と解析結果は常に同一内容に対応する。
- **AC-08**: `analyzeOneLibrary` は、解析対象の実測ハッシュを解析用の読み取りから計算し、呼び出し元が保持する `lib.Hash`（ハッシュキー）と比較する。不一致の場合、その解析結果を記録せず、検証を fail-closed（エラー）として扱う。
- **AC-09**: 改ざんがない正常系（record 対象ファイルが操作中に変化しない）では、変更前と同一内容のレコードが生成される（既存の record 出力に回帰がない）。

#### F-004: verify 時の依存解決再実行

- **AC-10**: `VerifyCommandDynLibDeps` は、verify 時に依存解決（RUNPATH/`$ORIGIN`、`@rpath` 候補ディレクトリの探索）を再実行する際に、コマンドの実行環境（`envVars`）を受け入れ、使用することで、RUNPATH の `$ORIGIN` や環境変数展開など、実行時の動的な置換に基づく正確な依存解決を行う。得られた解決パス集合が record 時の記録済み集合と一致することを確認する。record 時より優先順位の高い探索位置に新たなライブラリが出現した場合、verify は失敗する。
- **AC-11**: 環境が record 時から変化していない正常系では、verify は従来どおり成功する。

#### F-005: PathResolver の Stat/EvalSymlinks 順序

- **AC-12**: `PathResolver.validateAndCacheCommand` は、symlink 解決後のパスに対して存在・regular file・実行ビットの検証を行う（`EvalSymlinks` を検証より先に実行する、または fd ベースの検証に置き換える）ことで、検証対象パスとキャッシュされる解決済みパスが常に同一のパス解決結果を指すことを保証する。
- **AC-13**: 改ざんがない正常系（対象ファイル・symlink 構成が操作中に変化しない）では、従来どおり解決済みパスが返り、キャッシュされる。

#### F-006: shebang インタプリタ symlink 検査の残余リスク文書化

- **AC-14**: `verifyInterpreterSymlinkTarget` の symlink 検査と実際の exec 時カーネル再解決の間に残る TOCTOU 窓について、(a) 構造的に完全排除が困難である理由、(b) 悪用の前提条件（インタプリタパスの symlink 差し替え権限）、(c) 残余リスクとして許容する判断が、設計文書またはセキュリティ文書に明記される。配置先は architecture 文書で確定する。本 AC は文書化のみの成果物であり暗黙に脱落しやすいため、配置先を architecture 文書で確定し、実装計画でも完了チェック項目として追跡すること。

## Success Criteria（要件レベル）

- AC-01〜AC-14 のすべてに対し、実装計画（`03_implementation_plan.md`）で具体的なテストまたは静的検証手段が対応付けられている。
- 各修正について、改ざんがない正常系のテスト（回帰防止）と、TOCTOU 窓を突く異常系のテスト（fail-closed の確認）の双方が存在する。
- `make test && make lint` が通過する。
