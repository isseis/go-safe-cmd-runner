# 実装計画書: 単一スレッド前提に照らして過剰な排他制御を棚卸しし、削除する

## Document Status

| Item | Value |
|---|---|
| Status | `approved` |
| Created | 2026-09-01 |
| Review date | 2026-09-01 |
| Reviewer | isseis |
| Comments | - |

## 関連文書

- [01_requirements.md](01_requirements.md)（AC 番号はすべてこちらを指す）
- [02_architecture.md](02_architecture.md)（設計の根拠。本書は設計を再掲せず参照する）
- [セキュリティアーキテクチャ（日本語版・原本）](../../dev/architecture_design/security-architecture.ja.md) と
  [英語版](../../dev/architecture_design/security-architecture.md)

本書に出てくる記号は `02_architecture.md` と共通である。**D1〜D11** は削除対象、**K1〜K7** は
維持対象、**census guard test** は棚卸し結果を機械で固定するテスト（`02_architecture.md` §4）、
**特権の隙** は `seteuid(0)` から復帰までの区間を指す。

**英語リテラルの綴りについて**: 本書が「この語句を書く」と指定する英語のリテラルは、すべて
米国綴り（`serialize`／`initialization` など）に統一する。§7 の静的検証はこれらのリテラルを
そのまま検索するため、綴りを変えると検証が落ちる。

---

## 1. 実装概要

### 1.1 目的

production コードにある 28 箇所の排他制御・アトミック変数を「実際に並行アクセスがあるか」で
仕分け、無い側（12 宣言）を削除し、ある側（15 宣言）と対象外の `pwentMutex`（1 宣言）には
なぜ必要かを doc コメントとして残す。外部から観測できる挙動は変えない。

判定の根拠、並行性の発生源の網羅、脅威モデルはすべて `02_architecture.md` にある。本書は
その設計を実行に移す手順だけを扱う。

### 1.2 実装原則

`02_architecture.md` §1.2 の6原則に従う。実装時に効いてくるのは次の3つである。

1. **1件1コミット**（AC-02）。D1〜D11 はそれぞれ独立したコミットにし、1件だけ revert できる
   粒度を保つ。
2. **契約と実装を同時に動かす**（`02_architecture.md` §3.2）。ロックを外すコミットには、その
   ロックの存在を前提にした記述（doc コメント・コード内コメント・インターフェース定義・設計文書・
   テストコード）の修正をすべて同梱する。記述だけを後回しにしたコミットは作らない。
3. **削除の順序を守る**（`02_architecture.md` §7.1）。排他制御を削除し、並行テストをまだ残した
   状態で `-race` を1回走らせてから、テストを整理する。この `-race` 実行の位置づけについては
   §4.1 に限界を明記した。

### 1.3 既存コード調査結果

`02_architecture.md` の記述を HEAD に対して照合した。設計の判断はすべて有効であり、加えて
**設計書に載っていない変更箇所が 8 件**と、**設計書が触れていない副作用が 2 件**見つかった。

#### 1.3.1 宣言の総数の照合

次のコマンドで production コードを走査した結果は 31 行であり、`02_architecture.md` §1.1 の
「テキスト検索では 31 行、論理宣言は 28」と一致した。

```
rg -n -g '*.go' -g '!*_test.go' \
  -e 'sync\.(Mutex|RWMutex|Once|OnceValue|OnceFunc|OnceValues|WaitGroup|Map|Cond|Locker)' \
  -e 'atomic\.(Bool|Int32|Int64|Uint32|Uint64|Value|Pointer)' internal/ cmd/
```

D1〜D11 の宣言位置、K1〜K7 の宣言位置、`pwentMutex` の位置はいずれも設計書の記述どおりである。

#### 1.3.2 設計書に無い変更箇所（新規に発見）

いずれもロックやアトミック変数を**テストコード側から直接触っている**もので、削除と同じコミットで
直さなければコンパイルが通らない。

| 削除 | 位置 | 現在の記述 | 対処 |
|---|---|---|---|
| D2 | `internal/groupmembership/manager_test.go:152,157` | `gm.cacheMutex.Lock()`／`Unlock()` でキャッシュを直接書き換える | 2行を削除する（間の書き換え処理は残す） |
| D3 | `internal/groupmembership/manager_test.go:1500` | `processSudoUIDAdoptionReporter.reported.Load()` | `processSudoUIDAdoptionReporter.reported` に変える |
| D5 | `internal/groupmembership/test_helpers.go:45` | `processNSSCompletenessReporter.reported.Store(false)` | `processNSSCompletenessReporter.reported = false` に変える |
| D5 | `internal/groupmembership/test_helpers.go:73` | `processNSSCompletenessReporter.reported.Store(true)` | `processNSSCompletenessReporter.reported = true` に変える |
| D5 | `internal/groupmembership/manager_test.go:1626` | `processNSSCompletenessReporter.reported.Load()` | `processNSSCompletenessReporter.reported` に変える |
| D7 | `internal/verification/path_resolver_test.go:185,187` | `resolver.mu.RLock()`／`RUnlock()` でキャッシュを直接読む | 2行を削除する（間の `resolver.cache[...]` の読み出しは残す） |
| D9 | `internal/runner/resource/normal_manager_test.go:391,393,403,405,414,416` | `f.Manager.mu` の Lock／RLock／Unlock／RUnlock（3対） | 6行を削除する（間の `tempDirs` へのアクセスは残す） |
| — | `docs/dev/architecture_design/security-architecture.ja.md` の6箇所 | 英語版と同じ6箇所が日本語版にもある | 英語版と同じ改訂を行う（§1.3.4） |

#### 1.3.3 削除に伴って不要になる import（コンパイルが通らなくなる箇所）

削除対象の宣言が、そのファイルにおける `sync`／`sync/atomic` の**唯一の利用**であるものが多い。
import を残すと `imported and not used` でコンパイルが落ち、そのコミットが緑にならない
（AC-20・AC-21 に反する）。次が全件である。

| ファイル | 不要になる import | 落とせるタイミング |
|---|---|---|
| `internal/runner/base/executor/executor.go` | `sync` | D1 のコミット |
| `internal/groupmembership/manager.go` | `sync` | **D2 と D4 の両方**が済んだ側のコミット |
| `internal/groupmembership/manager.go` | `sync/atomic` | D3 のコミット |
| `internal/groupmembership/manager_test.go` | `sync` | **D3 と D4 の両方**が済んだ側のコミット（`:1360` が D3、`:1432,1445` が D4） |
| `internal/groupmembership/nsswitch.go` | `sync/atomic` | D5 のコミット |
| `internal/groupmembership/policy.go` | `sync/atomic` | D6 のコミット |
| `internal/groupmembership/policy_test.go` | `sync` | D6 のコミット（`:87` が唯一の利用） |
| `internal/verification/path_resolver.go` | `sync` | D7 のコミット |
| `internal/verification/result_collector.go` | `sync` | D8 のコミット |
| `internal/verification/result_collector_test.go` | `sync` | D8 のコミット（`:118` が唯一の利用） |
| `internal/runner/resource/normal_manager.go` | `sync` | D9 のコミット |
| `internal/runner/resource/dryrun_manager.go` | `sync` | D9 のコミット |
| `internal/runner/base/risktypes/types.go` | `sync/atomic` | D10 のコミット |
| `internal/runner/base/risktypes/types_test.go` | `sync` | D10 のコミット（`:105` が唯一の利用） |
| `internal/runner/base/privilege/unix.go` | `sync` | D11 のコミット |

**この表は Phase 2 の順序に制約を与える。** `manager.go` の `sync` は D2 と D4 の後の方、
`manager_test.go` の `sync` は D3 と D4 の後の方でしか落とせない。したがって D2・D3・D4 は
「どの順でもよいが、後から来た方が import を落とす」という取り決めで進める（§2 Phase 2 の共通手順
に組み込む）。他の 8 件は相互に依存しない。

§3.2 の PR 構成では D2 が PR-2、D3・D4 が PR-3 に入る。PR-2 の時点では D4 の
`sudoUIDExistenceMemo.mu` が残っているので `manager.go` の `sync` はまだ使われており、落とさなくても
コンパイルは通る。実際に落とすのは PR-3 の D4 のコミットである。この依存は PR をまたぐが、PR は
順にマージされるため順序は保たれる。

#### 1.3.4 既存テストで再利用できるもの（新規テストを書かない根拠）

| AC・削除 | 既存テスト | 判断 |
|---|---|---|
| AC-04（D6） | `internal/groupmembership/policy_test.go::TestSetProcessPermissionCheckUIDPolicy` | `02_architecture.md` §7.2 の契約表のうち4行を既に検証している。**実装時の訂正**: 第2行（未設定への設定）のうち `SudoUIDAware` を格納する側だけは、削除する `TestSetProcessPermissionCheckUIDPolicy_Concurrent` が唯一の in-package の検証だった（`assert.Contains` で最終値が2値のいずれかであることを見ていた）。Step 2-6 でサブテスト `unset to SudoUIDAware succeeds` を追加してこれを引き継いだ |
| AC-05（D3） | `internal/groupmembership/manager_test.go::TestSudoUIDAdoptionReporter_ReportsOnlyOnce` | 逐次の「1回だけ」を既に検証している |
| AC-05（D5） | `internal/groupmembership/nsswitch_test.go::TestNSSCompletenessReporter_ReportsOnlyOnce` | 同上。`02_architecture.md` §3.6 は `nsswitch.go` の行に「新規に要るテスト AC-05」と書いているが、この既存テストが同じ性質を同じ粒度で検証しているため新規テストは書かない（CLAUDE.md「重複したテストを足す前に既存を確認する」） |
| AC-06（D4） | `manager_test.go::TestSudoUIDExistenceMemo_ReusesConfirmation`、同 `::TestSudoUIDExistenceMemo_DoesNotRememberFailures` | memo のヒットと失敗の非記憶を既に検証している |
| AC-07（D2） | `manager_test.go::TestCompletenessSurvivesCache`（AC-19 を満たすのはこれ）、同 `::TestGroupMembership`、同 `::TestGetGroupMembers_ErrorNotCached` | **本表の当初の記述は誤りだった**。`TestGroupMembership` の `cache behavior` 系サブテストと `TestGetGroupMembers_ErrorNotCached` は `GetCacheStats` の件数しか見ておらず、`getGroupEnumeration` のキャッシュ参照を外しても通る。AC-19 を実際に満たしているのは本表が挙げていなかった `TestCompletenessSurvivesCache`（同一 GID の2回参照で列挙が1回であることを検証する）である。加えて、別 GID の未ヒットと失効後の再列挙はどのテストも検証していないため、Step 2-2 で `TestGetGroupMembers_CacheHitSkipsEnumeration` を追加した（ヒットの検証は `TestCompletenessSurvivesCache` と重なるが、同じ準備で未ヒットと失効まで公開 API の `GetGroupMembers` 上で通して検証するため、分割せず1本にまとめている） |
| AC-07（D7） | `internal/verification/path_resolver_test.go::TestPathResolver_ValidateAndCacheCommand` | **本表の当初の記述は誤りだった**。この既存テストが検証しているのは `validateAndCacheCommand` の**キャッシュ格納**だけで、`ResolvePath` 冒頭の**キャッシュ参照**は外してもパッケージ全体が緑のままである（Step 1-0 の基準カバレッジでも、参照側の分岐にある `RUnlock` は未カバーだった）。Step 2-8 でサブテスト `answers from the cache once the command can no longer be resolved` を追加してこの分岐を引き受けた（格納側と参照側でキーが食い違う壊れ方も同時に捕まえる）。D2 側と同じ種類の見落としである |
| AC-08（D9） | `normal_manager_test.go::TestNormalResourceManager_CreateTempDir`、同 `::TestNormalResourceManager_CleanupTempDir`、`dryrun_manager_test.go::TestDryRunResourceManager_CreateTempDir`、同 `::TestDryRunResourceManager_CleanupTempDir` | 通常版・dry-run 版の登録／解放を既に検証している。**本表の当初の記述は誤りだった**: 全解放の欄に挙げていた `default_manager_test.go::TestDefaultResourceManager_CleanupAllTempDirs` は dry-run のサブテストしか持たず、しかも `tempDirs` が空のマネージャに対して呼ぶため、通常版の `NormalResourceManager.CleanupAllTempDirs` に一度も到達しない（基準カバレッジでも同関数は 0.0%）。通常版の全解放は Step 2-9 で追加した `normal_manager_test.go::TestNormalResourceManager_CleanupAllTempDirs` が引き受ける |
| D8 | `result_collector_test.go::TestResultCollector_RecordSuccess`（:34）、同 `::TestResultCollector_RecordFailure`（:47）、同 `::TestResultCollector_GetSummary`（:91）、同 `::TestResultCollector_MixedResults`（:335） | 削除する `TestResultCollector_Concurrency` が主張していた「成功・失敗の記録が集計へ正しく反映される」は、この4本が検証している。確認済みなので Step 2-7 では追加しない |
| AC-10（D10） | `internal/runner/base/risktypes/types_test.go::TestVerifiedFD_FdAndIdempotentClose`、同 `::TestVerifiedFD_NilReceiverClose` | 冪等性と nil レシーバは検証しているが、**「`syscall.Close` が1回だけ走る」ことは検証していない**。同ファイルの既存ヘルパ `fdIsOpen`（:46）を使って前者を拡張する（新規テスト関数は追加しない） |
| D11 | §4.4 の表を参照。`race_test.go` の4関数の削除は、カバレッジ比較では検証できない（§4.4 の注記） | Step 3-4 で関数単位にどのテストが検証しているかを議論する |
| AC-18（K2） | `internal/runner/base/output/capture_test.go::TestCapture_ConcurrentAccess` | K2 を検証する維持対象。削除しない |

**新規に書くテストは 4 つである。** AC-09（`outputWrapper` の stdout／stderr 識別）、
AC-07 の D2 側（上表のとおり実装時に追加）、AC-24（再入ガード）、AC-23（census guard test）。

#### 1.3.5 D2 が変える、公開 API の並行使用の契約（設計書に無い副作用）

`internal/groupmembership/manager.go:419-420` と `:434-435` の `cacheMutex` は、**公開メソッド**
`ClearCache` と `GetCacheStats` の唯一の同期手段である。D2 を削除すると、この2つの公開 API は
並行呼び出しに対する保護を失う。`02_architecture.md` は D7・D8 について同種の警告 doc コメントを
求めている（§8.3）が、D2 については触れていない。

同じ理由（設計原則4: 削除によって暗黙の契約が変わるなら、契約の側も同じコミットで書き換える）が
そのまま当てはまるため、**D2 のコミットで `GroupMembership` 型の doc コメントにも同じ警告を書く**
（Step 2-2）。これは設計の変更ではなく、設計原則4 の適用範囲を1件広げるものである。

#### 1.3.6 `security-architecture` の日本語版を対象に含める理由

`02_architecture.md` §6.3 は `security-architecture.md`（英語版）の6箇所だけを挙げているが、
同じ6箇所が日本語版 `security-architecture.ja.md` にも存在する。CLAUDE.md のバイリンガル
方針では日本語版が原本であり、日本語版を直さずに英語版だけを直すと原本に誤りが残る。設計原則4
の当然の適用として、**日本語版を先に編集し、英語版へは `/mktrans` で反映する**。

行番号は6箇所のうち4箇所（309・322-323・407・437）が両版で一致し、残る2箇所はずれる
（脅威モデルが ja:1192 / en:1197、Performance が ja:1256 / en:1261）。

> **承認済み設計書との差**: `02_architecture.md` §2.2 の「変更するファイルは合計 23」は、この
> 日本語版1ファイルを含まないため 24 が正しい。§3.6 の責務表にも `security-architecture.ja.md` の
> 行が要る。本計画はこの2点で設計書の数を更新するものとして扱う。

#### 1.3.7 ビルドと検証コマンドの実測

| 前提 | 実測結果 |
|---|---|
| `make test` が `CGO_ENABLED=1`（`-race`）と `CGO_ENABLED=0` の両方を回すか | `Makefile:477-483` の `unit-test` が両方を回す。したがって AC-20 は `make test` 1回で足りる（macOS では `CGO_ENABLED=0` 側が skip される） |
| `make lint` が両構成を回すか | `Makefile:150-156` が両方を回す。AC-21 も `make lint` 1回で足りる |
| テストのビルドタグ | `Makefile:478` は `-tags test`、`Makefile:24` の golangci-lint は `--build-tags test`。`//go:build test` のファイルはテストと lint の双方でコンパイルされる |
| `make deadcode` | `Makefile:743-744` が `deadcode ./cmd/record ./cmd/runner ./cmd/verify` を実行する |
| `identitymutationguard.ProductionGoFiles` | `internal/testutil/identitymutationguard/helpers.go:151` に存在し、1ディレクトリを対象に `*_test.go` と「`test` タグを積極的に要求するファイル」を除いた `.go` の一覧を返す。再帰走査は含まない |
| `identitymutationguard` パッケージのビルドタグ | `helpers.go:1` が `//go:build test`。これを import する census guard test にも同じタグが要る |
| `//go:build test` の `_test.go` だけを含むディレクトリ | `go build ./...`／`go test ./...`（タグ無し）はそのディレクトリを**黙って飛ばす**ので壊れない。ただし `go test ./internal/testutil/synccensus/` のようにパスを明示するとタグ無しでは `build constraints exclude all Go files` で失敗する。`make test` は常に `-tags test` を渡すため CI は影響を受けない |

#### 1.3.8 census guard test の走査で落としてはならない宣言の形

`02_architecture.md` §4.5 は「修飾された型式」と「初期化子の呼び出し式」の2つを挙げる。実装に
あたって、走査対象の**構文位置**を2つ足す必要がある。

1. **関数本体の中のローカル変数宣言。** K3（`internal/runner/bootstrap/logger.go:457` の
   `var wg sync.WaitGroup`）は構造体フィールドでもパッケージレベル変数でもない。フィールドと
   トップレベル宣言だけを見る走査は K3 を取りこぼし、期待表との突合が初日から
   「期待表にあるが見つからない」で失敗する。
2. **短縮変数宣言（`:=`）。** `mu := sync.Mutex{}` は `*ast.AssignStmt` であり `*ast.ValueSpec` では
   ない。現在の production コードにこの形は存在しないが、拾わないと「今後増えたロックを捕まえる」
   という AC-23 の目的（`02_architecture.md` §4.6）を将来にわたって果たせない。

なお `internal/testutil/handlers.go:43` の `&sync.Mutex{}`（複合リテラル）は `*ast.CompositeLit`
であって上記のいずれでもないため、**そもそも走査に現れない**。重複を除く処理は要らない。

---

## 2. 実装ステップ

### Phase 1: 維持対象の根拠を書く（AC-14〜AC-17）と、誤解を招くテスト名の是正

**位置づけ**: `02_architecture.md` §9 の Phase 1。以降の削除コミットが「なぜこれは残るのか」を
既に書かれた doc コメントで説明できるようにするため、削除より先に行う。

> **設計書からの差分（1件）**: `02_architecture.md` §9 は Phase 1 に「§8.3 の3件の**改名・警告追加**」を
> 置いている。本計画は**改名の3件は Phase 1 に置き、警告の追加（`PathResolver`・`ResultCollector` の
> 「並行使用を想定しない」doc コメント）は Phase 2 の D7・D8 のコミットへ移す**。Phase 1 の時点では
> まだ `mu` があり、「並行使用を想定しない」は事実に反するためである。フェーズの順序そのものは
> §9 のとおりで変えていない。

#### Step 1-0: 着手前の基準値を記録する

基準値は PR-2 以降 PR-7 まで参照するため、複数の PR とブランチをまたいで残る場所へ置く。
`/tmp` は再起動で消え、別のマシンやコンテナへも引き継がれないので使わない。本計画では
`.git/0170-baseline/`（git の管理対象外で、ブランチ切り替えの影響を受けない）を使う。
ディレクトリごと失われた場合は、`base.sha` をチェックアウトした `git worktree` で同じコマンドを
流し直せば再生成できる。

カバレッジ基準は `CGO_ENABLED=1` に固定する。`internal/groupmembership` は CGO の有無でビルドされる
ファイル集合が変わり（`membership_cgo.go` と `membership_nocgo.go`）、構成をまたいで比較すると
実体の無いカバレッジ低下が出るためである。

- [x] `make test` と `make lint` が現状で通ることを確認する
- [x] 基準値の置き場を作る: `mkdir -p .git/0170-baseline`
- [x] **本タスクの起点コミットを固定する**: `git rev-parse HEAD > .git/0170-baseline/base.sha`。
      §7 と §8 が `<base>` と書くのはこの SHA である。PR を1本ずつ main へマージしていくと
      `git merge-base main HEAD` は直前の PR の先端へ動いてしまい、コミット数やコミットメッセージを
      数える検証が常に 0 を返すようになるため、起点は必ず SHA で固定する
- [x] `make deadcode > .git/0170-baseline/deadcode.txt` を実行する（AC-22 の比較基準）
- [x] 削除対象を含む 8 パッケージのカバレッジを関数単位で記録する（AC-13 の比較基準）。対象は
      `internal/groupmembership`、`internal/verification`、`internal/runner/resource`、
      `internal/runner/base/executor`、`internal/runner/base/risktypes`、
      `internal/runner/base/privilege`、`internal/runner/base/output`、`internal/logging`
- [x] 記録は次のループで取る（パッケージ名のスラッシュをファイル名で潰す）:

```sh
for p in groupmembership verification runner/resource runner/base/executor \
         runner/base/risktypes runner/base/privilege runner/base/output logging; do
  n=$(echo "$p" | tr / _)
  CGO_ENABLED=1 go test -tags test -coverprofile=".git/0170-baseline/cov-$n.out" "./internal/$p/"
  go tool cover -func=".git/0170-baseline/cov-$n.out" > ".git/0170-baseline/covfunc-$n.txt"
done
```

**完了条件**: `.git/0170-baseline/base.sha`、`deadcode.txt`、8個の `covfunc-*.txt` が存在する。

#### Step 1-1: K1（`internal/logging/slack_sender.go`）に根拠を書く（AC-15）

4件それぞれについて、既存の doc コメントに「どの goroutine とどの goroutine の間で並行なのか」を
補う。役割の説明ではなく相手の goroutine を名指しする。§7 の静的検証が下記のリテラルをそのまま
検索するため、**語句は一字一句このとおりに書く**。

- [x] `mu sync.RWMutex`（117 行）: `guards the fields below against the send worker started by go sd.run()`
      の一文を含める
- [x] `aggregateOnce sync.Once`（136 行）: `Flush and Close can both reach this from different goroutines`
      の一文を含める
- [x] `syncInFlight sync.WaitGroup`（143 行）: `terminate waits here for the goroutines running sendSync`
      の一文を含める
- [x] `slackCounters` 型（175 行付近）の doc コメント: `updated concurrently by the send worker and by callers`
      の一文を含める（5フィールドを個別に書き分けない）

**完了条件**: 次が 4 を返す。

```
rg -F -c -e 'guards the fields below against the send worker started by go sd.run()' \
        -e 'Flush and Close can both reach this from different goroutines' \
        -e 'terminate waits here for the goroutines running sendSync' \
        -e 'updated concurrently by the send worker and by callers' \
  internal/logging/slack_sender.go
```

#### Step 1-2: K2（`internal/runner/base/output/capture.go:21`）に根拠を書く（AC-14）

- [x] `mutex` の行末コメント `// Protects concurrent access to file and size` を、`Capture` 型の
      doc コメント側の記述へ移し、次の2つのリテラルを含める
      - `os/exec starts one goroutine per writer`（`Cmd.Stdout`／`Cmd.Stderr` に `*os.File` でない
        `io.Writer` を渡した場合の挙動）
      - `stdout and stderr wrappers share this Capture`

**完了条件**: `rg -F -c -e 'os/exec starts one goroutine per writer' -e 'stdout and stderr wrappers share this Capture' internal/runner/base/output/capture.go`
が 2 を返す。

#### Step 1-3: K3・K4・K5 に根拠を書く（AC-15）

- [x] `internal/runner/bootstrap/logger.go:457` の `var wg sync.WaitGroup`:
      `this WaitGroup is what makes the Slack flush concurrent` の一文を含める
- [x] `internal/logging/log_line_tracker.go:22` の `lineCounter atomic.Int64`: `output copy goroutine` の
      語句を含め、`output.Capture` が出力サイズ上限超過時に `Logger.Error` を呼ぶ経路で、slog ハンドラが
      `os/exec` の出力コピー goroutine 上を走りうることを書く。`DefaultLogLineTracker` 型の既存 doc
      コメント「provides a thread-safe implementation ... using atomic operations for concurrent
      access」は並行の相手を述べていないので、相手を名指しする記述に書き換える
- [x] `internal/redaction/error_collector.go:18` の `mu sync.RWMutex`: 同じく `output copy goroutine` の
      語句を含め、`RedactingHandler` が `RecordFailure` を呼ぶ経路で同じ goroutine 上を走りうることを
      書く。型 doc コメントの「Safe for concurrent use」だけの記述を、相手を名指しする記述に置き換える

**完了条件**:

- `rg -F -c 'this WaitGroup is what makes the Slack flush concurrent' internal/runner/bootstrap/logger.go` が 1 を返す
- `rg -F -c 'output copy goroutine' internal/logging/log_line_tracker.go internal/redaction/error_collector.go` が
  2ファイル合計で 2 以上を返し、どちらのファイルも 1 件以上を含む

#### Step 1-4: K6 の2箇所に根拠を書く（AC-16）

- [x] `internal/runner/base/executor/fdexec_linux.go:19` の `fdExecSupported`:
      `memoization, not mutual exclusion` の語句を含める
- [x] `internal/runner/base/risktypes/runas_ident.go:29` の `OriginalExecutionIdentity`: 同じ語句に加えて
      `must not be replaced with a hand-written lazy initialization` の語句を含め、「最初の権限変更より
      前に捕捉する」正しさの要請を担うことを書く（既存の 15 行目・24 行目の記述を活かす）

**完了条件**:

- `rg -F -c 'memoization, not mutual exclusion' internal/runner/base/executor/fdexec_linux.go internal/runner/base/risktypes/runas_ident.go` が 2 を返す
- `rg -F -c 'must not be replaced with a hand-written lazy initialization' internal/runner/base/risktypes/runas_ident.go` が 1 を返す

#### Step 1-5: `pwentMutex` の doc コメントを改訂する（AC-17）

- [x] `internal/groupmembership/membership_cgo.go:302` の `pwentMutex` の doc コメントに、次の3つの
      リテラルを含む記述を書く
      - `process-wide cursor`（`setpwent`／`getpwent`／`endpwent` が libc のプロセス全体のカーソルで
        あること）
      - `silently wrong enumeration`（外すと、将来の並行呼び出しでエラーではなく黙って誤った列挙結果に
        なること）
      - `deliberately kept by task 0170`（本タスクの棚卸しで意図的に維持したこと）
- [x] `membership_cgo.go:299-300` の「Lock ordering: `GroupMembership.cacheMutex` -> `pwentMutex`」の
      記述は**この Step では削除しない**。`cacheMutex` は D2 のコミットで消えるため、その削除は
      Step 2-2 に含める（削除と記述の追随を同じコミットに置く原則2）

**完了条件**: `rg -F -c -e 'process-wide cursor' -e 'silently wrong enumeration' -e 'deliberately kept by task 0170' internal/groupmembership/membership_cgo.go`
が 3 を返す。

#### Step 1-6: 実体に合わないテスト名を是正する（`02_architecture.md` §8.3）

- [x] `internal/runner/base/output/integration_test.go:299` の
      `TestOutputCaptureIntegration_ConcurrentWrites` を
      `TestOutputCaptureIntegration_SequentialWrites` に改名し、doc コメントを実体（逐次書き込み）に
      合わせる
- [x] `internal/runner/resource/error_scenarios_test.go:258` の `TestConcurrentExecutionConsistency` を
      `TestExecutionConsistencyAcrossModes` に改名する。goroutine ごとに別のマネージャを構築する構造は
      残し、doc コメントに「各 goroutine が自分のマネージャを持つため、マネージャは共有されない」ことを
      書く
- [x] `internal/runner/resource/error_scenarios_test.go:598` の `TestConcurrentExecution` を
      `TestDryRunExecutionAcrossIndependentManagers` に改名する。このテストは 10 goroutine を起こす
      **本当に並行なテスト**であり、共有していないのはマネージャのインスタンスだけである。「Repeated」
      のような逐次を示唆する名前にはしない。doc コメントには、各 goroutine が自分の
      `DryRunResourceManager` を構築するので同一インスタンスが共有されない、という事実だけを書く。
      「これが D9 を許す根拠である」まで書くのは Step 2-9（PR-5）で行う。この時点ではまだ `mu` が
      あり、Phase 1 冒頭の差分注記と同じ理由で、未マージの変更を前提にした記述は置かない
- [x] `internal/verification/shebang_chain_verifier_test.go::TestVerifyCommandDependencies_ConcurrentCallsAreRaceFree`
      は**変更しない**（`02_architecture.md` §8.3 の判断）

**完了条件**: `rg -n -e 'func TestConcurrentExecution' -e 'func TestOutputCaptureIntegration_ConcurrentWrites' internal/`
が0件を返す。

#### Phase 1 の完了ゲート

- [x] `make fmt` → `make test` → `make lint` が通る
- [x] コミットを分ける。Step 1-1〜1-5 を1コミット、Step 1-6 を別コミットにする

---

### PR-1 作成ポイント: retained-lock rationale and test renames

**対象ステップ**: 1-0 / 1-1 / 1-2 / 1-3 / 1-4 / 1-5 / 1-6

**推奨タイトル**: `refactor(0170): document retained synchronization and rename misleading tests`

**レビュー観点**: 指定された英語リテラルが §7 の静的検証と一字一句一致するか / 並行の相手 goroutine を名指しできているか（役割の説明で終わっていないか） / 改名した3件のテストの doc コメントが実体（逐次か、インスタンス非共有か）と合っているか / Step 1-0 の基準値が後続 PR から参照できる場所に置かれたか

**実装モデル要件**: standard

**判定理由**: rubric のどのトリガにも該当しない。未確立の設計判断も、パネルモード条件も、Conditional checks も無く、コード挙動を変えない doc コメントと改名に閉じる。

- [x] グリーンゲート（`_context.md` の "Green gate" 参照）がパスしていることを確認した
- [x] PR を作成した（https://github.com/isseis/go-safe-cmd-runner/pull/1082）
- [x] PR がマージされた
- [x] 次のブランチへ切り替えた（次ステップは新しいブランチで作業する）

---

### Phase 2: D1〜D10 の削除（1件1コミット）

**位置づけ**: `02_architecture.md` §9 の Phase 2。§1.3.3 の import 依存を除けば、10 件は相互に
依存しない。

**各件に共通する手順**（`02_architecture.md` §7.1。以下の Step では繰り返さない）:

1. 排他制御を削除し、同じコミットで記述（doc コメント・コード内コメント・テストコード・設計文書）を
   追随させる
2. **§1.3.3 の表を見て、そのコミットで不要になる `sync`／`sync/atomic` の import を落とす。**
   `manager.go` の `sync` は D2・D4 の後の方、`manager_test.go` の `sync` は D3・D4 の後の方でのみ
   落とす
3. **並行テストを残したまま** `make test` を1回走らせ、`-race` の報告の有無を記録する
   （この実行の限界は §4.1 に明記した。報告の有無をそのまま「削除してよい証拠」として読まない）
4. 並行テストを削除または改名する。削除するテストが主張していた性質は逐次テストが検証しており、その対応は §1.3.4 で確認済みである
5. カバレッジを再取得し、Step 1-0 の基準と関数単位で比較する（`CGO_ENABLED=1`）
6. `make fmt` → `make test` → `make lint` を通す
7. コミットする。件名は `refactor(0170): remove D<N> <識別子>` の形とし、本文に次を含める
   - `Rationale:` — 単一スレッドでしか到達しない根拠（AC-02）
   - `Race observation:` — 手順3の結果と、その結果が何を意味するか（§4.1）
   - `Coverage:` — 手順5の比較結果（AC-13）
   - `Falsification:` — 検証対象の挙動を壊してテストが失敗することを確認した内容。AC が複数の主張を
     含む場合は**主張ごとに1行**書く（AC-19、§4.5）

#### Step 2-1: D1 `outputWrapper.mu` の削除

- [x] **新規テストを先に書く**: `internal/runner/base/executor/output_wrapper_test.go`（新規、
      `package executor`）に `TestOutputWrapper_SeparatesStdoutAndStderr` を追加する。stdout 用と
      stderr 用の `outputWrapper` に**異なる内容**を書き、`GetBuffer` の返り値がそれぞれ対応することと、
      `OutputWriter` が受け取る `OutputStream` が対応することを検証する。総量が等しいだけでは
      取り違えを検出できないため、内容で照合する（`02_architecture.md` §8.4）。
      **計画からの差分**: 当初は `executor_test.go` に置くとしていたが、同ファイルは
      `package executor_test`（外部テストパッケージ）であり非公開型 `outputWrapper` に届かない。
      §4.2 が求める「パッケージ `executor` の内部テスト」を満たすため、同パッケージの新規ファイルに置いた
      （`shell_escape_test.go`・`stagefromfd_test.go` と同じ先例）
- [x] 同テストに、`writer.Write` が2回続けて別のエラーを返す場合に `GetWriteError` が**最初の**エラーを
      返すことの検証を含める（サブテスト `get_write_error_returns_first_error`）
- [x] `internal/runner/base/executor/executor.go:640` の `mu sync.Mutex` フィールドを削除する
- [x] 同ファイル 644-645・665-666・671-672 の `w.mu.Lock()`／`defer w.mu.Unlock()`（3対6行）を削除する
- [x] AC-19 の確認（2つの主張それぞれ）:
      (a) `w.writer.Write(w.stream, p)` を `w.writer.Write(StdoutStream, p)` に変えて識別のアサーションが
      失敗すること（テストが `outputWrapper` を直接構築するため、`stream` の入れ替えは production 側の
      タグ付けを壊す形で行った）。あわせて `02_architecture.md` §8.4 が挙げる本来の壊し方
      （`executor.go:332-333` の `StdoutStream` と `StderrStream` を入れ替える）も実行した。これは
      `TestOutputWrapper_SeparatesStdoutAndStderr` では捕まらず、`cmd/runner` の4本の統合テスト
      （`TestIntegration_TempDirHandling`・`_ErrorCleanup`・`_MultipleGroups`・`_CommandLevelWorkdir`）が
      失敗する。§7 の AC-09 の行にこの4本を明記した、
      (b) `executor.go:653` の `if w.writeErr == nil` のガードを外して `writeErr` を毎回上書きさせ、
      「最初のエラー」のアサーションが失敗すること

**完了条件**: `rg -n 'w\.mu\.' internal/runner/base/executor/executor.go` が0件。新規テストが通る。

---

#### Step 2-2: D2 `GroupMembership.cacheMutex` の削除

- [x] `internal/groupmembership/manager.go:90` の `cacheMutex sync.RWMutex` を削除する
- [x] 同ファイル 134-143 の RLock／RUnlock／Lock／Unlock を削除し、**二重確認（double-check）の
      再読み込みごと**不要になるので削る（`02_architecture.md` §3.2）
- [x] 同ファイル 419-420 の Lock／`defer Unlock` の対、434-435 の RLock／`defer RUnlock` の対
      （2対4行）を削除する
- [x] `manager.go:88` の doc コメント「cache for group membership data with thread safety」から
      並行性の言及を削る
- [x] `manager.go:145` のコメント「Double-check after acquiring write lock (another goroutine might
      have populated it)」を、二重確認の削除に合わせて削る
- [x] `manager.go:454` の「must be called with write lock held」を削る
- [x] **`GroupMembership` 型の doc コメントに `This type is not safe for concurrent use` の一文を書く**
      （§1.3.5）。公開メソッド `ClearCache`・`GetCacheStats` が同期を失うためである
- [x] `internal/groupmembership/membership_cgo.go:299-301` のコメント段落から、ロック順序を述べる部分
      （`Lock ordering: GroupMembership.cacheMutex -> pwentMutex. Reverse acquisition is forbidden.`）
      だけを削る。**続く `nsswitchVerdict takes no lock at all: the classification is settled at
      startup and only read afterwards.` は真であり、独立した文として残す**
- [x] `internal/groupmembership/manager_test.go:152,157` の `gm.cacheMutex.Lock()`／`Unlock()` の2行を
      削除する（間のキャッシュ書き換え処理は残す）
- [x] **未ヒットと失効の新規テストを追加する（計画からの差分）**: `manager_test.go` に
      `TestGetGroupMembers_CacheHitSkipsEnumeration` を追加し、同じ GID の2回目の
      `GetGroupMembers` が列挙関数を呼ばないこと・別 GID は呼ぶこと・失効後は再列挙することを
      列挙回数で検証する。§1.3.4 が挙げていた2本（`TestGroupMembership`・
      `TestGetGroupMembers_ErrorNotCached`）はキャッシュ参照を外しても通るため、その2本では
      AC-19 を満たせない。ヒットについては §1.3.4 が挙げていなかった `TestCompletenessSurvivesCache`
      が既に AC-19 を満たしており、追加分の固有の価値は未ヒットと失効にある
- [x] AC-19 の確認: `getGroupEnumeration` のキャッシュ参照を外し、
      `TestGetGroupMembers_CacheHitSkipsEnumeration` と `TestCompletenessSurvivesCache` の双方が
      列挙回数のアサーションで失敗すること

**完了条件**: `rg -n 'cacheMutex' internal/ cmd/` が0件。

> **`02_architecture.md` §1.2 の原則5（失われる検知を置き直す）の D2 への適用**: `cacheMutex` は
> 再入不可のロックであり、先行タスク 0169 が
> [`0169/02_architecture.md`](../0169_groupmembership_cgo_enumeration_completeness/02_architecture.md) で
> 論じたとおり（同 §1043 の決定記録）、`getGroupEnumeration` はロックを保持したまま列挙関数を呼ぶため、
> そこから slog ハンドラ経由で `safefileio` → `CanCurrentUserSafelyWriteFile` →
> `getGroupEnumeration` と再入すれば自己デッドロックする。0169 は完全性判定を起動時確定に改めて
> ロック下での `report` を無くしたが、ロック自体が持つ再入検知は残っていた。削除により、この再入は
> 検知されなくなる。**D11 と違って置き直しは行わない。**
> 守っている状態が純粋なキャッシュだからである。再入が起きた場合の結果は、内側の呼び出しが同じ
> GID を再列挙して同じ値を格納し、外側がそれを上書きすることだけであり、返り値も権限判定も変わらない。
> 特権の隙のような不可逆な状態を持たないので、fail-closed なガードを置く対象が無い。

### PR-2 作成ポイント: single-owner buffer and cache state (D1, D2)

**対象ステップ**: 2-1 / 2-2

**推奨タイトル**: `refactor(0170): remove outputWrapper and membership cache locks (D1-D2)`

**レビュー観点**: D1: stdout 用と stderr 用が別インスタンスであるという削除根拠が成り立っているか / D1: 新規テストが内容で照合しており総量比較になっていないか / D2: 公開 API（`ClearCache`・`GetCacheStats`）の並行契約が変わる点に警告 doc コメントが付いたか / D2: 二重確認の削除で `getGroupEnumeration` の分岐が変わっていないか

**実装モデル要件**: frontier-recommended

**判定理由**: rubric (c)「隔離された高リスクな変更」に D2 が該当する。公開 API 2本の並行使用の契約を変え、二重確認ロジックごと削除するため、機械的な置換では済まない。影響の重い D2 を PR の末尾に置いている。

- [x] グリーンゲート（`_context.md` の "Green gate" 参照）がパスしていることを確認した
- [x] PR を作成した（https://github.com/isseis/go-safe-cmd-runner/pull/1083）
- [x] PR がマージされた
- [x] 次のブランチへ切り替えた（次ステップは新しいブランチで作業する）

---

#### Step 2-3: D3 `sudoUIDAdoptionReporter.reported` の `bool` 化

- [x] `internal/groupmembership/manager.go:473` の `reported atomic.Bool` を `reported bool` に変える
- [x] 同 479 行の `if !r.reported.CompareAndSwap(false, true) { return }` を
      `if r.reported { return }` ＋ `r.reported = true` に置き換える
- [x] `internal/groupmembership/manager_test.go:1500` の `processSudoUIDAdoptionReporter.reported.Load()` を
      `processSudoUIDAdoptionReporter.reported` に変える
- [x] 並行テスト `manager_test.go::TestSudoUIDAdoptionReporter_ReportsOnlyOnceConcurrently` を削除する
- [x] AC-19 の確認: `reported` の判定を外し、`TestSudoUIDAdoptionReporter_ReportsOnlyOnce` が失敗すること

**完了条件**: `rg -n 'func TestSudoUIDAdoptionReporter_ReportsOnlyOnceConcurrently' internal/` が0件。

#### Step 2-4: D4 `sudoUIDExistenceMemo.mu` の削除

- [x] `internal/groupmembership/manager.go:504` の `mu sync.Mutex` を削除する
- [x] 同 512-513 の `m.mu.Lock()`／`defer m.mu.Unlock()` を削除する
- [x] `manager.go:509-510` の doc コメントから「The lock is held across lookup to single-flight
      concurrent queries」の single-flight の主張を削る
- [x] 並行テスト `manager_test.go::TestSudoUIDExistenceMemo_Concurrent` を削除する
- [x] AC-19 の確認（2つの主張それぞれ）:
      (a) memo の参照を外し、`TestSudoUIDExistenceMemo_ReusesConfirmation` が失敗すること、
      (b) 失敗した確認も memo に記録するように変え、`TestSudoUIDExistenceMemo_DoesNotRememberFailures` が
      失敗すること

**完了条件**: `rg -n 'single-flight' internal/groupmembership/manager.go` が0件。

#### Step 2-5: D5 `nssCompletenessReporter.reported` の `bool` 化

- [x] `internal/groupmembership/nsswitch.go:278` の `reported atomic.Bool` を `reported bool` に変える
- [x] 同 290 行の `CompareAndSwap(false, true)` を `if r.reported { return }` ＋ `r.reported = true` に
      置き換える
- [x] `internal/groupmembership/test_helpers.go:45` の `.Store(false)` を `= false` に変える
- [x] `internal/groupmembership/test_helpers.go:73` の `.Store(true)` を `= true` に変える
- [x] `internal/groupmembership/manager_test.go:1626` の `.reported.Load()` を `.reported` に変える
- [x] AC-19 の確認: `reported` の判定を外し、`TestNSSCompletenessReporter_ReportsOnlyOnce` が失敗すること

**完了条件**: `rg -n '\.reported\.(Load|Store|Swap|CompareAndSwap)' internal/` が0件。

#### Step 2-6: D6 `processPermissionCheckUIDPolicy` の削除

- [x] `internal/groupmembership/policy.go:68` の `var processPermissionCheckUIDPolicy atomic.Int32` を
      `var processPermissionCheckUIDPolicy PermissionCheckUIDPolicy` に変える
- [x] 同 83-95 の CAS ループを、判定と代入を続けて行う形に書き換える。`02_architecture.md` §7.2 の
      契約表の5行がそのまま成り立つ形にする
- [x] 同 94 行のコメント「Another goroutine changed the value concurrently; re-evaluate.」を削る
- [x] `policy.go:101` の `PermissionCheckUIDPolicy(processPermissionCheckUIDPolicy.Load())` を
      `processPermissionCheckUIDPolicy` に変える
- [x] `policy.go:11` の `type PermissionCheckUIDPolicy int32` を `type PermissionCheckUIDPolicy int` に
      戻す（`int32` は `atomic.Int32` の都合であって設計上の要請ではない）
- [x] `policy.go:49` の `fmt.Sprintf("unknown(%d)", int32(p))` を `fmt.Sprintf("unknown(%d)", int(p))` に
      変える
- [x] `internal/groupmembership/test_helpers_policy.go:40` の
      `previous := PermissionCheckUIDPolicy(processPermissionCheckUIDPolicy.Swap(int32(p)))` を
      `previous := processPermissionCheckUIDPolicy` ＋ `processPermissionCheckUIDPolicy = p` に置き換える
- [x] 同 42 行の `processPermissionCheckUIDPolicy.Store(int32(previous))` を
      `processPermissionCheckUIDPolicy = previous` に変える
- [x] 並行テスト `policy_test.go::TestSetProcessPermissionCheckUIDPolicy_Concurrent` を削除する
- [x] **計画からの差分**: `TestSetProcessPermissionCheckUIDPolicy` にサブテスト
      `unset to SudoUIDAware succeeds` を追加する。§7.2 の契約表の第2行は「未設定に
      `RealUIDOnly` **または** `SudoUIDAware`」であるが、既存の4サブテストは `RealUIDOnly` 側しか
      設定しておらず、`SudoUIDAware` を格納する側を in-package で見ていたのは削除する並行テスト
      だけだった（§1.3.4 の訂正）。格納値を `RealUIDOnly` に固定するとこのサブテストだけが失敗する
      ことを確認済みである
- [x] **計画からの差分（レビュー指摘への対応）**: 平坦化した3つのプロセス全体のグローバル
      （`processSudoUIDAdoptionReporter`・`processNSSCompletenessReporter`・
      `processPermissionCheckUIDPolicy`）の doc コメントに、アトミックの代わりに何が安全性を担って
      いるのか（それぞれ読み取り安全性検査の単一 goroutine、起動時の単一スレッド区間、各バイナリの
      `init`）を1文ずつ書く。維持対象に根拠を書く AC-14〜AC-17 と同じ理由が、素の型へ落とした側にも
      当てはまるためである。あわせて `manager_test.go::TestProcessSudoUIDAdoptionReporterIsProcessWide`
      から `t.Parallel()` と、既に成立しなくなった「まだ production の利用者がいない」旨の
      doc コメントを外す（`manager.go` の `getPermissionCheckUID` が現在この instance を使う）
- [x] AC-19 の確認（`02_architecture.md` §7.2 の契約表の各分岐）:
      (a) 衝突判定の分岐を外し、異値の設定が通るようにして `TestSetProcessPermissionCheckUIDPolicy` の
      衝突ケースが失敗すること、(b) 同値再設定の早期 `return nil` を外して no-op ケースが失敗すること、
      (c) `PolicyUnset`／不正値の弾きを外して不正値ケースが失敗すること

**完了条件**: `rg -n 'int32' internal/groupmembership/policy.go internal/groupmembership/test_helpers_policy.go` が0件。

---

### PR-3 作成ポイント: groupmembership reporter, memo and policy state (D3-D6)

**対象ステップ**: 2-3 / 2-4 / 2-5 / 2-6

**推奨タイトル**: `refactor(0170): remove groupmembership reporter and policy synchronization (D3-D6)`

**レビュー観点**: D6: CAS ループを直線化した後も `02_architecture.md` §7.2 の契約表5行が保たれているか / D6: 基底型を `int32` から `int` へ戻した影響が全参照点に及んでいるか / §1.3.3 の import 順序依存（`manager.go` の `sync/atomic` は D3、`manager_test.go` の `sync` は D3・D4 の後）が本 PR 内で解消されているか / D3・D5 の「1回だけ」がレポータのインスタンス共有で担保されている点が変わっていないか

**実装モデル要件**: frontier-recommended

**判定理由**: rubric (c)「隔離された高リスクな変更」に D6 が該当する。CAS 再試行ループを直線的な判定と代入へ書き換えつつ、返り値と格納値の5通りの組み合わせを不変に保つ必要がある。D3〜D5 は機械的な置換なので、判断を要する D6 を PR の末尾に置いている。

- [x] グリーンゲート（`_context.md` の "Green gate" 参照）がパスしていることを確認した
- [x] PR を作成した（https://github.com/isseis/go-safe-cmd-runner/pull/1084）
- [x] PR がマージされた
- [x] 次のブランチへ切り替えた（次ステップは新しいブランチで作業する）

---

#### Step 2-7: D8 `ResultCollector.mu` の削除

- [x] `internal/verification/result_collector.go:24` の `mu sync.Mutex` を削除する
- [x] 同 49-50、58-59、89-90、108-109、117-118 の Lock／Unlock（5対10行）を削除する
- [x] 同 122・126 の「Deep copy … to prevent data races」を、複製の目的を
      「呼び出し側への内部状態の漏出を防ぐ」に改める。**複製そのものは残す**
- [x] `ResultCollector` 型の doc コメントに、D7 と同じ §8.3 の警告を書く。リテラルは
      `This type is not safe for concurrent use; it must not be reached from verification.Manager's
      concurrent paths.` とする（同じ理由で本コミットに置く）
- [x] 並行テスト `result_collector_test.go::TestResultCollector_Concurrency` を削除する。削除する
      テストが主張していた「成功・失敗の記録が集計へ正しく反映される」は
      `TestResultCollector_RecordSuccess`・`TestResultCollector_RecordFailure`・
      `TestResultCollector_GetSummary`・`TestResultCollector_MixedResults` が検証している（§1.3.4 で確認済み）
- [x] AC-19 の確認（削除した並行テストが主張していた2つの主張それぞれ）:
      (a) `RecordFailure` の `rc.totalFiles++` を落とし、`TestResultCollector_RecordFailure`・
      `_GetSummary`・`_MixedResults` が失敗すること、(b) `RecordSuccess` の `rc.verifiedFiles++` を
      落とし、`TestResultCollector_RecordSuccess`・`_GetSummary`・`_MixedResults` が失敗すること

**完了条件**: `rg -n 'rc\.mu\.' internal/verification/result_collector.go` が0件。

---

#### Step 2-8: D7 `PathResolver.mu` の削除

- [x] `internal/verification/path_resolver.go:17` の `mu sync.RWMutex` を削除する
- [x] 同 60-62、86-91 の Lock／Unlock／RLock／RUnlock を削除する
- [x] `PathResolver` 型の doc コメントに `02_architecture.md` §8.3 の警告を書く。リテラルは
      `This type is not safe for concurrent use; it must not be reached from verification.Manager's
      concurrent paths.` とする。**この警告は D7 を削除する本コミットに置く**（Phase 1 の時点では
      まだ `mu` があり、警告は事実に反するため。§2 Phase 1 冒頭の差分注記を参照）
- [x] `docs/dev/architecture_design/security-architecture.ja.md:437` の `PathResolver` のコード例から
      `mu      sync.RWMutex` の行を削る
- [x] **計画からの差分**: `/mktrans` を使わず、英語版 `security-architecture.md:437` に同じ1行削除を
      直接適用した。削除対象は Go のコード例の1行であり、日英で綴りが完全に同一で訳出の余地が無い
      ためである（散文を編集する箇所では従来どおり `/mktrans` を使う。Step 3-5 の5箇所は散文を含む
      ので `/mktrans` の対象である）
- [x] `internal/verification/path_resolver_test.go:185,187` の `resolver.mu.RLock()`／`RUnlock()` の2行を
      削除する（間の `resolver.cache["chained_cmd"]` の読み出しは残す）
- [x] **警告を誤りが起きる場所にも置く（計画からの差分・レビュー指摘への対応）**: `02_architecture.md`
      §8.3 は型の doc コメントを「唯一の防御」と位置づけるが、その警告は
      `path_resolver.go`／`result_collector.go` の型宣言にあり、実際に競合を生む編集が行われるのは
      `manager.go` の側である。加えて `VerifyCommandDependencies` の doc コメント（`manager.go:601-608`）は
      「concurrent callers」を無条件に支持すると読める。そこで `Manager` の `pathResolver`・
      `resultCollector` の両フィールドに行末コメントで並行安全でない旨を書き、
      `VerifyCommandDependencies` の doc コメントに「並行呼び出しを支持するのはこのメソッド単体であって
      `Manager` 全体ではない。ここから到達する経路がこの2フィールドに触れてはならない」の一文を足した
- [x] **キャッシュの往復のサブテストを追加する（計画からの差分）**: `TestPathResolver_ResolvePath` に
      サブテスト `answers from the cache once the command can no longer be resolved` を追加した。
      §1.3.4 は AC-07 の D7 側は `TestPathResolver_ValidateAndCacheCommand` で足りるとしていたが、
      それが検証するのは **キャッシュへの格納**だけであり、`ResolvePath` 冒頭の**キャッシュ参照**を
      外してもパッケージ全体が緑のままだった。Step 1-0 の基準カバレッジでも、参照側の分岐にある
      `RUnlock` は未カバーのステートメントとして現れている。サブテストは非公開の `cache` を直接
      触らず、公開 API だけで往復させる。すなわち `ResolvePath` で1回解決したのち実行ファイルを
      削除し、キャッシュが冷えた別のリゾルバが `ErrCommandNotFound` になることを確認したうえで、
      元のリゾルバが同じ結果を返すことを見る。この形にしたのは、格納側と参照側が**別のキーを
      使う**という壊れ方（`validateAndCacheCommand(found, found)`）も捕まえる必要があるためである。
      キーが食い違うとキャッシュは永久に冷えたままになり、`ResolvePath` の doc コメントが
      TOCTOU 緩和として説明する「シンボリックリンクを最初に1回だけ解決する」性質が黙って失われる
- [x] AC-19 の確認（3つの壊し方それぞれ）:
      (a) `validateAndCacheCommand` の `pr.cache[cacheKey] = resolvedPath` を落とすと、
      `TestPathResolver_ValidateAndCacheCommand/validates_and_caches_fully_resolved_path` と
      新規サブテストの双方が失敗すること、(b) `ResolvePath` 冒頭のキャッシュ参照を落とすと
      新規サブテストが失敗すること、(c) `pr.validateAndCacheCommand(found, command)` を
      `(found, found)` に変えて格納キーと参照キーを食い違わせると新規サブテストが失敗すること

**完了条件**: `rg -n 'pr\.mu\.|mu +sync' internal/verification/path_resolver.go` が0件、
`rg -n 'sync\.RWMutex' docs/dev/architecture_design/security-architecture.ja.md docs/dev/architecture_design/security-architecture.md` が0件。

### PR-4 作成ポイント: verification package synchronization removal (D8, D7)

**対象ステップ**: 2-7 / 2-8

**推奨タイトル**: `refactor(0170): remove verification package synchronization (D7-D8)`

**レビュー観点**: `verification.Manager` は並行使用の契約を持ちテストもあるのに、その内部フィールドから同期を外す点（`02_architecture.md` §8.3）を doc コメントの警告だけで守れているか / 警告のリテラルが2型で一致しているか / `security-architecture` 行 437 の改訂が日本語版・英語版の両方に入っているか / D8 の複製が残り、目的の記述だけが書き換わっているか

**実装モデル要件**: frontier-recommended

**判定理由**: rubric (c)「隔離された高リスクな変更」に D7 が該当する。`TestVerifyCommandDependencies_ConcurrentCallsAreRaceFree` が単一の `verification.Manager` を8 goroutine で駆動しており、その `PathResolver` フィールドから同期を外すという並行経路の判断を伴う（`02_architecture.md` §8.3 が「最も重要」とした項目）。機械的な D8 を先に置き、D7 を PR の末尾に置いている。

- [x] グリーンゲート（`_context.md` の "Green gate" 参照）がパスしていることを確認した
- [x] PR を作成した（https://github.com/isseis/go-safe-cmd-runner/pull/1085）
- [x] PR がマージされた
- [x] 次のブランチへ切り替えた（次ステップは新しいブランチで作業する）

---

#### Step 2-9: D9 `NormalResourceManager.mu`／`DryRunResourceManager.mu` の削除

D9 は2ファイルにまたがるが、同じ `tempDirs` 管理の1つの判断なので1コミットにまとめる。

- [x] `internal/runner/resource/normal_manager.go:42` の `mu sync.RWMutex` を削除する
- [x] 同 328-330、343-350、357-360 の Lock／Unlock／RLock／RUnlock を削除する
- [x] `internal/runner/resource/dryrun_manager.go:91` の `mu sync.RWMutex` を削除する
- [x] 同ファイルの `d.mu.` の Lock／Unlock／RLock／RUnlock をすべて削除する
      （`rg -n 'd\.mu\.' internal/runner/resource/dryrun_manager.go` が返す全行）
- [x] `dryrun_manager.go:540` の `previewExitCodeLocked` を `previewExitCode` に改名し、
      呼び出し側（同 534・909 行）を追随させる
- [x] `dryrun_manager.go:895` の `refreshDryRunResultLocked` を `refreshDryRunResult` に改名し、
      呼び出し側（同 870・885 行）を追随させる
- [x] 改名した2関数の名前を含むコメント（`dryrun_manager.go:84`・`537`・`569`・`866`・`877`・`892`）を
      新しい名前に合わせて直す
- [x] `dryrun_manager.go:564-570` の「Concurrent calls are serialized with mu.」を含む並行性の主張と、
      `:864-866` の「it must hold the exclusive lock: a read lock would let concurrent callers race」の
      記述を削る
- [x] `internal/runner/resource/normal_manager_test.go:391,393,403,405,414,416` の `f.Manager.mu` の
      Lock／RLock／Unlock／RUnlock（3対6行）を削除する（間の `tempDirs` へのアクセスは残す）
- [x] `TestDryRunExecutionAcrossIndependentManagers` の doc コメントに、各 goroutine が自分の
      マネージャを構築して同一インスタンスを共有しないことが D9 を許す根拠である旨を書き足す
      （Step 1-6 から持ち越した分）
- [x] **計画に無かった記述の追随（実装時に発見）**: `dryrun_manager.go:75` の
      「Preview decision tracking (guarded by mu)」と `:486` の `recordPreviewDecision` の
      「It locks once.」も `mu` の存在を前提にしているため、同じコミットで削った。§3.2 の表は
      `:84`／`:564-570`／`:864-866` の3箇所しか挙げていないが、原則2（記述を同じコミットで追随
      させる）の当然の適用である
- [x] **計画からの差分**: `NormalResourceManager.CleanupAllTempDirs` の `tempDirs` の複製は
      **残す**。この複製は並行性のためではなく、`CleanupTempDir` が反復中に `n.tempDirs` を
      書き換えるためである。ロックを外すと複製の理由が読めなくなるので、その旨を1行の
      コメントに書いた（D8 の「複製の目的を書き換える」と同じ扱い）。あわせて複製と削除を
      `slices.Clone`／`slices.Index`＋`slices.Delete` に置き換えた（CLAUDE.md の `slices` 優先）
- [x] **`Manager` インターフェース側の並行性の主張を削る（計画に無し・レビュー指摘）**:
      `internal/runner/resource/manager.go:52` の `CommandToken` の「Used to safely update debug
      information even in parallel execution scenarios」と、同 `:75` の `FinalizeDryRunResults` の
      「atomically returns」を削る。前者は `UpdateCommandDebugInfo` が無同期で `tokenToIndex` と
      `resourceAnalyses` を書き換えるようになった以上、成り立たない。後者は実装側の doc を本コミットで
      「in a single operation」に直したのにインターフェース側だけが同期の語を残していた。D11 が
      `runnertypes/config.go` に対して行うのと同じ、原則4（契約側を同じコミットで動かす）の適用である
- [x] **`CleanupAllTempDirs` の新規テストを追加する（計画からの差分・レビュー指摘）**:
      `normal_manager_test.go` に `TestNormalResourceManager_CleanupAllTempDirs` を追加した。
      §1.3.4 と §7 の AC-08 の行は全解放を `default_manager_test.go::TestDefaultResourceManager_
      CleanupAllTempDirs` が検証しているとしていたが、同テストは dry-run のサブテストしか持たず、
      しかも `tempDirs` が空のマネージャに対して呼ぶため、通常版の実装に一度も到達しない
      （基準カバレッジでも `NormalResourceManager.CleanupAllTempDirs` は 0.0% である）。
      本コミットが理由を書き直した複製そのものが未検証のままになるので、3件を登録した状態で
      全解放を通し、全件が `RemoveAll` され `tempDirs` が空になることを検証する
- [x] AC-19 の確認（通常版・dry-run 版の両方）:
      (a) `NormalResourceManager.CreateTempDir` の `tempDirs` への登録を落とし、
      `TestNormalResourceManager_CreateTempDir` が失敗すること、
      (b) `NormalResourceManager.CleanupTempDir` の登録解除を落とし、
      `TestNormalResourceManager_CleanupTempDir` が失敗すること、
      (c) `DryRunResourceManager` 側で同じく登録を落とし、`TestDryRunResourceManager_CreateTempDir` が
      失敗すること、(d) `CleanupAllTempDirs` の `slices.Clone` を `n.tempDirs` の直接参照に変えると
      （反復中の削除で1件飛ばしになり）`TestNormalResourceManager_CleanupAllTempDirs` が失敗すること

**完了条件**: `rg -n 'Locked\b' internal/runner/resource/dryrun_manager.go` が0件、
`rg -n '\.mu\.' internal/runner/resource/` が0件。

#### Step 2-10: D10 `VerifiedFD.closed` の `bool` 化と契約の取り下げ

- [x] `internal/runner/base/risktypes/types.go:33` の `closed atomic.Bool` を `closed bool` に変える
- [x] 同 54 行の `if f.closed.Swap(true) { return nil }` を
      `if f.closed { return nil }` ＋ `f.closed = true` に置き換える
- [x] 型 doc コメントの `Contract` 節（22-24 行）の
      `Close is idempotent and safe for concurrent use: it guarantees the underlying descriptor is
      closed exactly once even if several callers race, and a nil receiver returns nil.` を
      `Close is idempotent: a second call from the same goroutine is a no-op, and a nil receiver
      returns nil. It provides no protection against concurrent calls; only the owning goroutine
      may call it.` に置き換える
- [x] `Close` の doc コメント（47-49 行）の
      `Close closes the underlying descriptor. It is idempotent, safe for concurrent use, and safe to
      call on a nil receiver. The atomic swap ensures syscall.Close runs for exactly one caller,
      avoiding a double-close race (CWE-1341).` を
      `Close closes the underlying descriptor. It is idempotent -- a second call from the same
      goroutine runs no syscall -- and safe to call on a nil receiver. There is no protection
      against concurrent calls: only the goroutine that owns this VerifiedFD may call Close.` に
      置き換える
- [x] 並行テスト `types_test.go::TestVerifiedFD_ConcurrentClose` を削除する
- [x] `types_test.go::TestVerifiedFD_FdAndIdempotentClose` を拡張し、既存ヘルパ `fdIsOpen`（:46）を用いて
      「2回目の `Close` が `syscall.Close` を走らせない」ことを検証する。手順は次のとおり厳密に定める
      1. 1回目の `Close` の後、`fdIsOpen(fd)` が偽であることを確認する
      2. `syscall.Open(os.DevNull, syscall.O_RDONLY, 0)` で新しい fd を取り、**その場で**
         `t.Cleanup(func() { _ = syscall.Close(newFD) })` を登録する（以降のアサーションが落ちても
         リークしない）
      3. `require.Equal(t, fd, newFD, "this test requires the kernel to reuse the freed fd number")` で
         番号の再利用を**必須条件として明示的に要求する**。再利用されなかった場合は skip でも
         暗黙の成功でもなく、テストの失敗とする（そうしないとアサーションが同語反復になる）
      4. 2回目の `vfd.Close()` を呼び、`fdIsOpen(newFD)` が真のままであることを確認する
- [x] **拡張したテストを2つのサブテストに分ける（計画からの差分・レビュー指摘）**: 上の手順1〜4は
      サブテスト `second close does not close a descriptor reusing the number` としてそのまま実装した。
      加えて、再利用に依存しない検出器を失わないためにサブテスト
      `second close returns nil and runs no syscall` を置く。ガードを外した2回目の `Close` は
      閉じ済みの fd に対する `close(2)` として EBADF を返すため、再オープンを挟まない側では
      `assert.NoError` がそのまま検出器になる。fd 番号の再利用はプロセス全体の状態に依存する
      前提なので、それだけに寄りかからない
- [x] **開いた fd はすべて取得箇所で `t.Cleanup` に登録する**。`VerifiedFD` が所有する fd の後始末は
      `vfd.Close()` 経由で行い、cleanup 自身が二重 close を起こさないようにする
- [x] AC-19 の確認: `closed` の判定を外すと、2つのサブテストが**別々の検出器**で失敗すること
      （再オープン無しの側は EBADF、再利用の側は「他人の fd を閉じた」ことの検出）
- [x] **§7.1 の「並行テストを残したまま `-race`」の扱い（レビュー指摘）**: D10 ではこの実測は
      構成上の意味を持たない。削除する `TestVerifiedFD_ConcurrentClose` は取り下げようとしている
      契約そのものを符号化しているため、素の `bool` に対して必ず競合を報告する。実際に測ると
      `types.go:54`（read）と `:57`（write）で `WARNING: DATA RACE` が出る。これは
      「そのテストにとって atomic が効いていた」ことの証拠であって、production が descriptor を
      共有することの証拠ではない。コミットメッセージの `Race observation:` には両方の実測
      （テストを残した場合の報告と、削除後の無報告）と、そのどちらも到達可能性を語らないことを書いた

**完了条件**: `rg -F -n -e 'safe for concurrent use' -e 'CWE-1341' internal/runner/base/risktypes/types.go` が0件。

#### Phase 2 の完了ゲート

- [x] `git log --oneline` に `refactor(0170): remove D1` 〜 `remove D10` の 10 コミットが並ぶ
- [x] `make fmt` → `make test` → `make lint` が通る

---

### PR-5 作成ポイント: resource and descriptor lifecycle state (D9, D10)

**対象ステップ**: 2-9 / 2-10

**推奨タイトル**: `refactor(0170): remove resource and descriptor lifecycle synchronization (D9-D10)`

**レビュー観点**: D10 の契約取り下げが型レベルと `Close` の2箇所で揃っているか / fd 番号の再利用テストが `require.Equal` で必須条件になっており、非再利用時に暗黙成功しないか / 新たに開く fd が取得箇所で `t.Cleanup` に登録されているか / `Locked` 接尾辞の改名が全呼び出し側と6箇所のコメントに反映されているか

**実装モデル要件**: frontier-recommended

**判定理由**: rubric (b)「Conditional checks に2件以上該当」。resource cleanup at acquisition（Step 2-10 で新規に開く fd を取得箇所で `t.Cleanup` に登録する）と real cleanup/close API existence（`VerifiedFD.Close` の契約取り下げ）の2件に該当する。影響の重い D10 を PR の末尾に置いている。

- [x] グリーンゲート（`_context.md` の "Green gate" 参照）がパスしていることを確認した
- [x] PR を作成した（https://github.com/isseis/go-safe-cmd-runner/pull/1086）
- [x] PR がマージされた
- [x] 次のブランチへ切り替えた（次ステップは新しいブランチで作業する）

---

### Phase 3: D11 の削除と再入ガードの導入

**位置づけ**: `02_architecture.md` §9 の Phase 3。影響範囲が最も広く、既存文書と脅威モデルの改訂を
伴うため単独で扱う。D11 の削除と再入ガードは分離できない（ガード無しに `mu` を外すと、
`02_architecture.md` §3.4 の言うとおり再入が静かな特権喪失になる）ので、1コミットにまとめる。

> **Step 3-1〜3-5 は1コミットである。** Step 3-1 の「テストを先に書く」は「先にコミットする」では
> ない。`ErrReentrantPrivilegeCall` は Step 3-2 で作るので、3-1 だけを単独でコミットするとコンパイルが
> 通らない。同じ理由で `race_test.go` の削除（Step 3-4）も同じコミットに入れる。

> **Phase 3 では §7.1 の「並行テストを残したまま `-race`」を行わない。** `race_test.go` の
> `TestUnixPrivilegeManager_LockSerialization` には skip が無く、10 goroutine から `WithPrivileges` を
> 呼ぶ。`mu` を外して素の `inPrivilegedWindow bool` を置いた時点で `-race` は**必ず**この変数への競合を
> 報告するが、それはテスト自身が起こした goroutine によるものであり、production の並行性については
> 何も語らない（§4.1）。したがって実装・記述の追随・`race_test.go` の削除を1コミットで行い、
> `Race observation:` には「`race_test.go` は同一コミットで削除するため実測なし。根拠は到達可能性の
> 議論のみ」と書く。

#### Step 3-1: 再入ガードのテストを先に書く（AC-24）

**このテストは、一般ユーザー権限の CI 環境で `skip` されずに実行されなければならない。**
削除対象の `race_test.go` の4関数はいずれも `IsPrivilegedExecutionSupported()` が偽のとき skip する
ため、同じ形を真似ると AC-24 の唯一の実行可能検証が空振りになる。

- [x] 既存の `internal/runner/base/privilege/unix_privilege_test.go::TestWithPrivileges_UserGroupExecutionDoesNotChangeIdentity`
      （:103-133）が使っている構成をそのまま流用する。すなわち `&UnixPrivilegeManager{...}` を手で
      構築し、次の4つを与える。**`fn` に到達できるのはこの構成のためであり、操作種別のためではない**
      （`OperationUserGroupExecution` は `unix.go:151-153` で `needsPrivilegeEscalation` を**真**にする）
      - `privilegeSupported: true` — `escalatePrivileges` 冒頭の「not supported」エラーを回避する
      - `originalUID: 0` — `escalatePrivileges` が「native root execution」の早期 return を取り、
        `syscall.Seteuid(0)` を呼ばない。一般ユーザーで `fn` まで到達できるのはこれが理由である
      - `identityVerifier: func() error { return nil }` — 復帰後の EUID／EGID 一致検証が
        `emergencyShutdown` を呼ばないようにする
      - `readSavedIDs: func() (int, int, error) { return -1, -1, ErrSavedSetNotSupported }` —
        saved-set 不変条件の検査を飛ばす
- [x] `osExit` には `func(_ int) { t.Fatal("emergencyShutdown called unexpectedly") }` を与え、
      緊急停止経路に入った場合にテストが黙って通らないようにする
- [x] `ElevationContext.Operation` には `runnertypes.OperationUserGroupExecution` を指定する
      （`prepareExecution` の `switch` が受け付ける2つの操作種別のうちの1つ。`default` は
      `ErrUnsupportedOperationType` を返すため、他の種別では `fn` の手前で戻ってしまう）
- [x] `TestWithPrivileges_ReentrantCallIsRejected` を追加し、2つのサブテスト
      （`reentrant call is rejected and the inner fn never runs` と
      `consecutive non-reentrant calls both run fn`）に分けて、次の3点を検証する
      - 外側の `fn()` の中から同一マネージャの `WithPrivileges` を呼ぶと `errors.Is(err,
        ErrReentrantPrivilegeCall)` が真になる
      - 内側の `fn()` が**1度も呼ばれない**（呼び出し回数カウンタで確認する）
      - 外側の `fn()` は最後まで走り、外側の `WithPrivileges` は内側とは独立に自分の結果を返す
- [x] 再入しない通常呼び出しでガードが発火しないことを確認するケースを同テストに含める
      （`WithPrivileges` を続けて2回呼び、2回目も `fn()` が呼ばれる）
- [x] **計画に無い追加（レビュー指摘）**: ガードは sticky fail-closed であり、`inPrivilegedWindow` を
      倒す `defer` が走らなくなると以降のすべての特権実行が拒否される。倒し漏れが起きうる2経路を
      サブテストで固定する。(a) `fn` が panic する経路（`handleCleanup` の recover と再 panic を
      通り抜けて倒れること）、(b) `prepareExecution` が操作種別を弾いて早期に返る経路。どちらも
      「1回目の後の2回目が `fn` を実行する」ことで検証する。`defer` を外すと両サブテストが失敗する
      ことを確認済みである

**完了条件**: `go test -tags test -run TestWithPrivileges_ReentrantCallIsRejected -v ./internal/runner/base/privilege/`
を一般ユーザーで実行して `--- PASS` になる（`--- SKIP` ではない）。

#### Step 3-2: `mu` の削除と再入ガードの実装

- [x] `internal/runner/base/privilege/errors.go` に
      `ErrReentrantPrivilegeCall = errors.New("reentrant WithPrivileges call")` を追加し、doc コメントに
      `ErrReentrantPrivilegeCall is returned when WithPrivileges is called from within a privilege
      window on the same manager.` と書く。このファイルの既存のセンチネルは `fmt.Errorf` で作られて
      いるため、`errors` の import を追加する。書式引数を取らないので `errors.New` を使うのが正しく、
      周囲に合わせて `fmt.Errorf` に直す必要はない
- [x] `internal/runner/base/privilege/unix.go:36` の `mu sync.Mutex` を削除し、
      非公開の `inPrivilegedWindow bool` フィールドを追加する
- [x] **計画に無い追加（原則2の適用）**: `inPrivilegedWindow` フィールドに doc コメントを付け、
      再入の拒否だけが目的であること・単一の goroutine が読み書きすること・並行呼び出しに対する
      保護を与えないことを書く。維持対象に根拠を書く AC-14〜AC-17 と同じ理由が、同期機構を使わない
      置き換え側にも当てはまる（D3・D5・D6 で平坦化したグローバルに doc を付けたのと同じ扱い）。
      **レビュー指摘を受けた加筆**: フィールドの doc には次の3点も書く。(a) 素の `bool` の無同期な
      読み書きは、2つの goroutine から呼ばれればデータ競合であり、ガード自体が fail-open 方向に
      成立しなくなること（「保護しない」では弱い）、(b) 捕まえるのは同一インスタンスへの再入だけで
      あり、euid はプロセス単位なので別インスタンス経由の入れ子は捕まらないこと、(c) フラグが立つ
      区間は特権の隙より広く、再入かつ操作種別が不正な呼び出しでは `ErrUnsupportedOperationType`
      より `ErrReentrantPrivilegeCall` が優先されること
- [x] 同 100-101 の `m.mu.Lock()`／`defer m.mu.Unlock()` を、入口のガードに置き換える。すなわち
      `inPrivilegedWindow` が立っていれば `fn()` を呼ばずに `ErrReentrantPrivilegeCall` を返し、
      立っていなければ立てて `defer` で倒す

#### Step 3-3: `privilege` パッケージとインターフェースの記述を追随させる（AC-11・AC-12・AC-24）

- [x] `unix.go:92-98` の再入不可の注意書き（`WithPrivileges is not reentrant: it holds m's mutex ...`
      から `... legitimate wait for the lock.` までの7行）を削除し、`WithPrivileges` の doc コメントに
      次の3つのリテラルを含む記述を書く（AC-11）
      - (a) `This method does not serialize privilege windows.`
      - (b) `While the window is open the process-wide euid is raised, so goroutines that never call
        WithPrivileges -- including the copy goroutines os/exec starts for non-*os.File writers --
        also run with that euid.`
        **実装時の差分**: 英文として読点が要るため `While the window is open, the process-wide euid
        is raised, ...` と1文字だけ変えた。§7 の AC-11 の静的検証が検索するのは
        `the process-wide euid is raised` であり、この語句は1行に収めてあるので検証は 3 を返す
      - (c) `This is an unresolved design issue: introducing parallel execution requires a separate
        design, not a lock inside this method.`
- [x] あわせて、再入は `ErrReentrantPrivilegeCall` で拒否されることを doc コメントに書く
- [x] `unix.go:248` と `unix.go:287` の
      `// Note: This method assumes the caller (WithPrivileges) has already acquired the mutex lock`
      の2行を削除する
- [x] `internal/runner/base/runnertypes/config.go:195-197` の
      `// WithPrivileges is not reentrant: fn must not call WithPrivileges again on` /
      `// the same manager, directly or indirectly, or the call deadlocks. Avoiding` /
      `// reentrant calls is the caller's responsibility.` を
      `// WithPrivileges is not reentrant: fn must not call WithPrivileges again on` /
      `// the same manager, directly or indirectly. Implementations must not deadlock` /
      `// on such a call; the Unix implementation returns ErrReentrantPrivilegeCall` /
      `// without running fn. Avoiding reentrant calls is the caller's responsibility.` に置き換える。
      **インターフェース側の記述は「実装は再入を必ずエラーで拒否する」と断定しない。** 他の実装
      （`internal/runner/base/privilege/testutil/mocks.go:33`、
      `internal/runner/resource/normal_manager_test.go:75`）は再入を検知しないため、断定すると
      本タスクが直そうとしている「実装より強い契約」を新たに作ってしまう

#### Step 3-4: 旧挙動を主張するテストを削除する

`internal/runner/base/privilege/race_test.go` の4関数を削除する。ファイル全体がこの4関数だけで
構成されるため、結果としてファイルごと削除になる。

- [x] `TestUnixPrivilegeManager_ConcurrentAccess` を削除する
- [x] `TestUnixPrivilegeManager_NoDeadlock` を削除する
- [x] `TestUnixPrivilegeManager_RaceConditionProtection` を削除する
- [x] `TestUnixPrivilegeManager_LockSerialization` を削除する
- [x] `race_test.go` を削除する
- [x] **カバレッジ比較ではなく、関数単位にどのテストが検証しているかを議論して AC-13 を満たす。** 4関数のうち3つ
      （`ConcurrentAccess`・`NoDeadlock`・`RaceConditionProtection`）は冒頭で
      `if !manager.IsPrivilegedExecutionSupported() { t.Skip(...) }` を行うため、setuid されて
      いない測定環境では**カバレッジに1行も寄与しない**。残る `LockSerialization` には skip が無く
      実行されるが、`Operation` に `OperationFileAccess` を渡しており、これは `prepareExecution` の
      `switch` の `default` に落ちて `ErrUnsupportedOperationType` で戻る。したがって寄与するのは
      `WithPrivileges` の入口と `prepareExecution` の一部だけで、`performElevation`・`handleCleanup`・
      `fn` には到達しない。いずれにせよ「カバレッジが落ちていない」は削除の妥当性をほとんど語らない。
      代わりに次をコミットメッセージの `Coverage:` に書く
      - 削除する4関数が触れていた production 関数（`WithPrivileges`、`prepareExecution`、
        `performElevation`、`handleCleanup`）を、`unix_privilege_test.go` と `manager_test.go` と
        `identity_mutation_guard_test.go` のどのテストが検証しているかを関数ごとに示す。とくに
        `LockSerialization` が実際に寄与していた `WithPrivileges` 入口と `prepareExecution` の
        `default` 分岐は、`ErrUnsupportedOperationType` を検証する既存テストが引き継ぐことを確認する
      - `TestUnixPrivilegeManager_NoDeadlock` と `TestUnixPrivilegeManager_LockSerialization` が
        主張していた「デッドロックしない」「ロックが直列化する」は、**本タスクが意図的に取り下げる
        性質**であり、置き換えるテストを作らないことを明記する
      - 参考値としてカバレッジ比較も併記してよいが、上記の議論の代わりにはしない

#### Step 3-5: 設計文書を追随させる（`02_architecture.md` §6.3）

日本語版を先に編集し、英語版へは `/mktrans` で反映する（§1.3.6）。6箇所のうち行 437 は Step 2-8 で
済んでいるので、ここで扱うのは残り5箇所である。

- [x] `security-architecture.ja.md:309` の構造体定義のコード例から `mu sync.Mutex  // 競合状態を防止`
      の行を削り、`inPrivilegedWindow bool` の行を加える
- [x] `security-architecture.ja.md:322-323` の `WithPrivileges` のコード例から
      `m.mu.Lock()  // スレッドセーフティのためのグローバルロック` と `defer m.mu.Unlock()` の2行を削り、
      再入ガードの2行に置き換える
- [x] `security-architecture.ja.md:407` の Security Guarantees の
      `- グローバルmutexによるスレッドセーフな特権操作` の行を削除する
- [x] `security-architecture.ja.md:1192` の脅威モデルで、脅威「特権処理における競合状態」への対策欄の
      `- スレッドセーフ操作` を削除し、**残存リスク**として
      「特権の隙が開いている間、参加しない goroutine は保護されない。これは未解決の設計課題である」を
      記す。対策を消すだけで脅威を対策なしのまま残さない（`02_architecture.md` §6.3(1)）
- [x] `security-architecture.ja.md:1256` の Performance の
      `- グローバルmutexが競合状態を防ぐが特権操作を直列化` の行を削除する
- [x] `/mktrans` で `security-architecture.md` の対応する5箇所（309・322-323・407・1197・1261）へ反映する
- [x] **設計書の表に無い2箇所（PR-4 のレビューで発見）**:
      `docs/dev/developer_guide/design-implementation-overview.ja.md:91` の
      「グローバルミューテックスを使用したスレッドセーフな特権操作」と同 `:477` の
      「グローバルミューテックスによる特権操作の直列化」を削除する。どちらも D11 が取り下げる性質を
      述べており、`02_architecture.md` §3.2・§6.3 のどの表にも載っていなかった。原則2（記述を同じ
      コミットで追随させる）の当然の適用として D11 のコミットに含める
- [x] `/mktrans` で `design-implementation-overview.md` の対応する2箇所（91・477）へ反映する
- [x] **設計書の表にも §8 の横断検索にも無い4箇所（レビュー指摘）**: §8 の横断検索は
      `グローバルミューテックス`／`global mutex` という語句だけを、しかも `docs/dev/` に限って
      探すため、次の4箇所が 0 件のまま残っていた。原則2の適用として同じコミットで直す
      - `design-implementation-overview.ja.md:99-100` と英語版の同位置: 直上の箇条書きだけを消して
        いたが、同じ節の `WithPrivileges` のコード例が `m.mu.Lock()`／`defer m.mu.Unlock()` のまま
        だった。`security-architecture` と同じ再入ガードの4行に置き換える
      - `docs/user/security-risk-assessment.ja.md:71,76-77` と英語版の同位置: 利用者向けの
        セキュリティ評価が「**排他制御**: mutexによる競合状態防止」を実装の優秀な点として挙げ、
        同じ古いコード例を載せていた。箇条書きを削り、コード例を差し替え、脅威モデルと同じく
        「残存リスク」の節を追加する
- [x] **今後の横断検索は語句ではなく機構名で行う（レビュー指摘）**: §8 の語句限定の検索の代わりに
      `rg -n --glob '!docs/tasks/**' -e 'm\.mu\.' -e 'sync\.Mutex' -e 'mutex' -e 'ミューテックス' docs/`
      を使う。残るのは K1（Slack 送信ワーカー）の記述だけであることを確認済みである
- [x] **`02_architecture.md` §10.1 への加筆（レビュー指摘）**: 再入ガードが素の `bool` である以上、
      `race_test.go` の削除と census guard test の走査対象（同期の型）の組み合わせにより、
      単一 goroutine という前提を機械的に守るものが無くなる。並列実行を導入する者が「何も失敗しない
      まま前提が壊れる」ことを知れるよう、その旨を将来の拡張性の節に記した

#### Phase 3 の完了ゲート

- [x] `make fmt` → `make test` → `make lint` が通る
- [x] コミット件名を `refactor(0170): remove D11 UnixPrivilegeManager.mu and add reentrancy guard` とし、
      本文に Phase 2 と同じ `Rationale:`／`Race observation:`／`Coverage:`／`Falsification:` を含める。
      `Falsification:` には「`inPrivilegedWindow` の判定を外すと
      `TestWithPrivileges_ReentrantCallIsRejected` が失敗する」ことを書く

---

### PR-6 作成ポイント: privilege manager lock removal and reentrancy guard

**対象ステップ**: 3-1 / 3-2 / 3-3 / 3-4 / 3-5

**推奨タイトル**: `feat(0170): reject reentrant WithPrivileges and remove the privilege manager mutex (D11)`

**レビュー観点**: ガード無しでは再入が静かな特権喪失になるため、削除・ガード追加・`race_test.go` 削除が同一コミットに収まっているか / AC-24 のテストが一般ユーザーで skip されず `--- PASS` するか（`originalUID: 0` と `privilegeSupported: true` の構成） / インターフェース側の記述が「全実装が再入をエラーで拒否する」と断定していないか（mock は検知しない） / 脅威モデル（ja:1192 / en:1197）で対策を消すだけにせず残存リスクを記したか / `race_test.go` 削除の根拠が、カバレッジ差分ではなく関数単位の議論になっているか

**実装モデル要件**: frontier-required

**判定理由**: `mkplan.md` step 8 のパネルモード条件のうち security-gate に該当する。既存ガードの撤去（挙動を下げる）と fail-closed な再入ガードの追加（挙動を上げる）を同時に行い、あわせて脅威モデルを改訂する。

- [x] グリーンゲート（`_context.md` の "Green gate" 参照）がパスしていることを確認した
- [x] PR を作成した（https://github.com/isseis/go-safe-cmd-runner/pull/1087）
- [x] PR がマージされた
- [x] 次のブランチへ切り替えた（次ステップは新しいブランチで作業する）

---

### Phase 4: census guard test の追加（AC-23）

**位置づけ**: `02_architecture.md` §9 の Phase 4。すべての削除が終わらないと期待表を確定できないため、
Phase 3 の後に置く。

#### Step 4-1: テストファイルを作る

- [x] `internal/testutil/synccensus/census_guard_test.go` を新規に作る。ファイル冒頭に `//go:build test`
      を置く（import する `identitymutationguard` が `//go:build test` のため。§1.3.7）
- [x] パッケージ名は `synccensus` とする。同じ親ディレクトリの既存パッケージ
      `internal/testutil/identitymutationguard` が `package identitymutationguard` を使っており、本件は
      その先例に揃えるものである（`test_organization.md` の `package <domain>testutil` は、他パッケージへ
      公開するヘルパを置く `testutil/` サブディレクトリの規則であり、本件は公開ヘルパを持たない）
- [x] テスト関数名は `TestSyncCensusMatchesExpectation` とする（§7 が参照する名前）
- [x] ファイル冒頭のコメントに、タグ無しで `go test ./internal/testutil/synccensus/` とパスを明示すると
      `build constraints exclude all Go files` になること、`make test` を使うことを書く

#### Step 4-2: 走査を実装する

- [x] リポジトリルートからの相対パス `../../../internal` と `../../../cmd` を再帰的に走査する
- [x] ディレクトリ名が `testdata` のものは走査から除く
- [x] 各ディレクトリについて `identitymutationguard.ProductionGoFiles(t, dir)` を呼び、production
      ファイルの一覧を得る（`ProductionGoFiles` は1ディレクトリ単位なので再帰は本テストが行う）
- [x] ファイルの読み取りに `os.ReadFile` を使う場合は、既存の
      `identitymutationguard/helpers.go` と同じく `// #nosec G304 -- path is built from an os.ReadDir
      result filtered to *.go, not from external/attacker-controlled input.` を**その行に限って**付ける。
      `parser.ParseFile` にパスだけを渡して読ませる場合はこの抑制は要らない
- [x] 得たファイルを `go/parser` で構文解析し、`ast.Inspect` で全ノードを辿って次の3種を拾う（§1.3.8）
      - `*ast.Field` — 構造体フィールド
      - `*ast.ValueSpec` — トップレベル宣言と**関数内のローカル変数宣言の双方**（K3 がこれ）
      - `*ast.AssignStmt` — `:=` による短縮変数宣言（現在は該当なしだが、将来の追加を捕まえるため）
- [x] 型式を持つ宣言では、`*`／`[]`／`map[...]` を剥がしたうえで、`sync` パッケージの並行
      プリミティブ全体（`Mutex`／`RWMutex`／`Once`／`OnceValue`／`OnceFunc`／`OnceValues`／
      `WaitGroup`／`Map`／`Cond`／`Locker`）と `atomic` パッケージの全型を検出する
- [x] 型式を持たない宣言では、初期化子が `sync.OnceValue`／`sync.OnceFunc`／`sync.OnceValues` の
      呼び出しである場合を検出する。K6 の2件はこの形でしか拾えない（`02_architecture.md` §4.5）

#### Step 4-3: 期待表と双方向に突き合わせる

- [x] 期待表を、ファイル・識別子・維持する短い理由の3列で持つ。詳細な根拠は production 側の
      doc コメントに置き、期待表には失敗メッセージに出る一文だけを持たせる
      （`02_architecture.md` §4.6・付録 A.4）
- [x] 期待表の行数は 16 になる。内訳は K1 が8、K2・K3・K4・K5 が各1、K6 が2、K7 が1、
      `pwentMutex` が1である
- [x] 「走査で見つかったが期待表に無い」と「期待表にあるが走査で見つからない」を**別々の見出し**で
      報告する。各行に「ファイル・識別子・（期待表にある場合は）理由」を出す

#### Step 4-4: テストが主張する理由で失敗できることを確認する（`02_architecture.md` §8.6）

3つの確認は、それぞれ別の失敗の向き・別の走査位置を突く。同じ経路を2回試すことにならないよう、
確認2で足すロックの**形**を指定する。

- [x] 確認1（期待表側の欠落）: 期待表から1行削り、「走査で見つかったが期待表に無い」で失敗すること
- [x] 確認2（走査側の余剰・構文位置の網羅）: production ファイルへロックを1つ**関数内のローカル
      `var` として**足し、同じく失敗すること。トップレベルやフィールドとして足すと確認1と同じ経路しか
      通らないため、構文位置を変える。確認後は必ず元に戻す
- [x] 確認3（型式を持たない宣言）: **K6 の行**（`sync.OnceValue`）を期待表から削り、失敗すること。
      型式だけを見る実装ではここが通ってしまうため省略できない。
      **計画からの差分**: 当初は失敗の向きを「期待表にあるが走査で見つからない」と書いていたが、
      これは誤りだった。期待表から行を削ると、その行は期待表側に存在しなくなるので、この向きでは
      そもそも報告されえない。走査が型式を持たない宣言を実際に拾えている場合の失敗の向きは
      「走査で見つかったが期待表に無い」である（型式だけを見る実装ならどちらの向きでも報告されず
      テストが通ってしまい、それが確認3の弁別したい状態である）。実測はこの向きで失敗した。
      あわせて「期待表にあるが走査で見つからない」の向きも報告経路として生きていることを、
      存在しない行（`manager.go` の `cacheMutex`、D2 で削除済み）を一時的に足して確認した
- [x] 3つの確認結果をコミットメッセージに記す

#### Phase 4 の完了ゲート

- [x] `make fmt` → `make test` → `make lint` が通る。とくに `//go:build test` を付けた新規ファイルが
      `make test`（`-tags test`）と `make lint`（`--build-tags test`）の双方でコンパイルされることを
      確認する
- [x] コミット件名を `test(0170): add sync census guard test` とする

---

### Phase 5: 全体の健全性の確認（AC-20〜AC-22）

**位置づけ**: `02_architecture.md` §9 の Phase 5。最終確認のみで、コード変更は伴わない。

- [ ] `make deadcode > .git/0170-baseline/deadcode-after.txt` を実行し、
      `diff .git/0170-baseline/deadcode.txt .git/0170-baseline/deadcode-after.txt` で新たな到達不能コードの
      報告が増えていないことを確認する（AC-22）
- [ ] `make test` を実行し、`CGO_ENABLED=1`（`-race`）と `CGO_ENABLED=0` の両方が通り、`-race` の警告が
      1件も出ないことを確認する（AC-20）
- [ ] `make lint` を実行し、両構成で通ることを確認する（AC-21）
- [ ] §7 の受け入れ基準検証表の静的検証コマンドをすべて実行し、期待結果と一致することを確認する
- [ ] §8 の横断検索チェックリストを実行する
- [ ] `git log` を確認し、D1〜D11 が 11 コミットに分かれていることを確認する（AC-02）

---

### PR-7 作成ポイント: sync census guard test and final verification

**対象ステップ**: 4-1 / 4-2 / 4-3 / 4-4（あわせて Phase 5 の最終確認を担う。Phase 5 は差分を生まないため対象ステップには挙げない）

**推奨タイトル**: `test(0170): add sync census guard test`

**レビュー観点**: 走査が `*ast.Field`・`*ast.ValueSpec`（関数内ローカルを含む）・`*ast.AssignStmt` の3位置を拾い、K3 と K6 を取りこぼさないか / `//go:build test` の新規ファイルが `make test` と `make lint` の双方でコンパイルされるか / Step 4-4 の確認2で production ファイルへ足したロックが確実に元へ戻っているか / §7 の `<base>` が §7 冒頭で固定した SHA を指しており、`git merge-base` になっていないか

**実装モデル要件**: frontier-recommended

**判定理由**: rubric (b)「Conditional checks に2件以上該当」。build-tag compilation（`//go:build test` の新規ファイルを最終的なタグと同じ条件でコンパイルするゲート）と rerun isolation（Step 4-4 の確認2が production ファイルを一時的に書き換えるため、戻し忘れると以後の実行が汚染される）の2件に該当する。

- [ ] グリーンゲート（`_context.md` の "Green gate" 参照）がパスしていることを確認した
- [ ] PR を作成した
- [ ] PR がマージされた
- [ ] 次のブランチへ切り替えた（次ステップは新しいブランチで作業する）

---

## 3. 実装順序とマイルストーン

### 3.1 マイルストーン

| マイルストーン | 対応フェーズ | 成果物 | 完了判定 |
|---|---|---|---|
| M1: 維持側の根拠が揃う | Phase 1 | K1〜K6・`pwentMutex` の doc コメント、3件のテスト改名 | AC-14〜AC-17 の静的検証が通る |
| M2: 通常削除の完了 | Phase 2 | D1〜D10 の 10 コミット、`security-architecture` 2言語の1箇所（行 437） | 10 コミットが個別に revert 可能で、各時点で緑 |
| M3: 特権まわりの完了 | Phase 3 | D11 の削除、再入ガード、`security-architecture` 2言語の残り5箇所 | AC-11・AC-12・AC-24 の検証が通る |
| M4: 棚卸しの固定 | Phase 4 | census guard test | AC-23 のテストが通り、3通りの壊し方で失敗する |
| M5: 全体の緑 | Phase 5 | 検証記録 | AC-20〜AC-22 が満たされる |

フェーズの順序は `02_architecture.md` §9 のとおりで変えていない。Phase 1 の内容のうち §8.3 の
「警告追加」だけを Phase 2 の D7・D8 のコミットへ移した（理由は Phase 1 冒頭の差分注記）。
Phase 2 の 10 件の順序制約は §1.3.3 の import 依存のみである。

### 3.2 PR 構成

Phase 2 の中で Step 2-7 と Step 2-8 だけを入れ替えた（旧 2-7=D7 / 旧 2-8=D8 → 新 2-7=D8 /
新 2-8=D7）。理由は、影響の重い D7 を PR-4 の末尾に置くためである。それ以外のステップは並べ替えて
おらず、各 PR は連続したステップだけで構成されている。

| PR | 対象ステップ | 主な変更内容 | 実装モデル要件 |
|---|---|---|---|
| PR-1 | 1-0 / 1-1 / 1-2 / 1-3 / 1-4 / 1-5 / 1-6 | 維持対象 K1〜K6 と `pwentMutex` への根拠 doc コメント、実体に合わないテスト3件の改名、起点 SHA と基準値の固定 | standard |
| PR-2 | 2-1 / 2-2 | D1・D2 の削除。stdout／stderr の識別を検証する新規テストと、公開 API の並行契約の警告を含む | frontier-recommended |
| PR-3 | 2-3 / 2-4 / 2-5 / 2-6 | D3〜D6 の削除。レポータと memo の平坦化、`PermissionCheckUIDPolicy` の基底型の復帰を含む | frontier-recommended |
| PR-4 | 2-7 / 2-8 | D8・D7 の削除（`verification` パッケージ）。`verification.Manager` の並行経路に対する警告と `security-architecture` 行 437 の改訂を含む | frontier-recommended |
| PR-5 | 2-9 / 2-10 | D9・D10 の削除。`Locked` 接尾辞の改名と、`VerifiedFD.Close` の並行安全性の契約取り下げを含む | frontier-recommended |
| PR-6 | 3-1 / 3-2 / 3-3 / 3-4 / 3-5 | D11 の削除、再入ガードの導入、`security-architecture` 残り5箇所と脅威モデルの改訂 | frontier-required |
| PR-7 | 4-1 / 4-2 / 4-3 / 4-4 | census guard test の追加。あわせて Phase 5 の最終確認（AC-20〜AC-22）と、`<base>..HEAD` を見る検証を担う | frontier-recommended |

**リスクの重い項目は各 PR の末尾に置く。** PR-2 の D2、PR-3 の D6、PR-4 の D7、PR-5 の D10 が
これにあたる（`02_architecture.md` §3.1 の「誤判定時の最悪の結果」が重い項目）。先に並ぶ機械的な
ステップのレビューを、重い項目が塞がないようにするためである。

各 PR は単独で緑ゲート（`make test && make lint`）を通す。Phase 2 の完了ゲートが挙げる
「D1〜D10 の 10 コミット」は PR-2〜PR-5 がすべて main へマージされた時点で揃う、**リポジトリ履歴に
対する**条件であり、PR-5 単体の差分に対する条件ではない。

---

## 4. テスト戦略

カバレッジの目標値は設けない。本タスクは挙動を変えないリファクタリングであり、目標は
「Step 1-0 の基準に対して関数単位で落ちないこと」（AC-13）だからである。後方互換性の検証も同じ理由で
独立した項目を立てず、「外部から観測できる挙動を変えない」を既存テスト全体（`make test`）が
担保する形とする。

### 4.1 `-race` 実行の位置づけと限界

`02_architecture.md` §7.1 は、削除直後・テスト削除前の `-race` 実行を手順に組み込んでいる。本計画は
この手順を守るが、**そこから得られるものを過大に読まない**。

削除対象に紐づく並行テスト（`TestSudoUIDAdoptionReporter_ReportsOnlyOnceConcurrently`、
`TestSudoUIDExistenceMemo_Concurrent`、`TestSetProcessPermissionCheckUIDPolicy_Concurrent`、
`TestResultCollector_Concurrency`、`TestVerifiedFD_ConcurrentClose`）は、**テスト自身が goroutine を
起こしている**。ロックを外せば `-race` は必ず報告する。その報告は「テストが並行に呼んだ」という
事実を述べるだけで、「production が並行に到達する」ことは何も示さない。判断の根拠は
`02_architecture.md` §1.3〜§1.5 の到達可能性の議論であり、`-race` ではない。

それでも実行するのは、**削除の範囲を誤って production の別経路にまで及ぼしていないか**を安く
検出できるためである。コミットメッセージの `Race observation:` には、報告の有無に加えて
「この報告（あるいは無報告）が到達可能性について何も語らない」ことを1行で添える。

例外は D9 である。`error_scenarios_test.go` の2つのテストは goroutine ごとに別のマネージャを
構築するので、D9 の削除後も `-race` は発火しない見込みである（`02_architecture.md` §7.1 末尾）。
発火した場合は「別インスタンスのはずが共有されている」ことを意味し、これは情報量のある観測である。

**D2 には対応する並行テストが無い**（`02_architecture.md` §8.2 の訂正）。D2 の
`Race observation:` には「該当する並行テストが無いため実測なし。根拠は到達可能性の議論のみ」と書く。

D11 はさらに別扱いである。`race_test.go` の `TestUnixPrivilegeManager_LockSerialization` は skip を
持たず 10 goroutine から `WithPrivileges` を呼ぶため、`mu` を素の `inPrivilegedWindow bool` へ置き換えた
時点で `-race` は必ず競合を報告する。報告が保証されている以上そこに情報は無いので、Phase 3 では
この実測を行わず、実装とテスト削除を1コミットにまとめる（Phase 3 冒頭の注記）。

### 4.2 新規に書くテスト

| テスト | 位置 | 検証内容 | 対応 AC |
|---|---|---|---|
| `TestOutputWrapper_SeparatesStdoutAndStderr` | `internal/runner/base/executor/output_wrapper_test.go`（新規・`package executor`） | stdout と stderr に異なる内容を書き、`GetBuffer` と `OutputStream` が対応すること。`GetWriteError` が最初のエラーを返すこと | AC-09 |
| `TestGetGroupMembers_CacheHitSkipsEnumeration` | `internal/groupmembership/manager_test.go` | 同じ GID の2回目の `GetGroupMembers` が列挙関数を呼ばないこと。別 GID は呼ぶこと。失効後は再列挙すること（§1.3.4 の実装時の訂正による追加） | AC-07（D2） |
| `TestWithPrivileges_ReentrantCallIsRejected` | `internal/runner/base/privilege/unix_privilege_test.go` | 再入が `ErrReentrantPrivilegeCall` を返し `fn()` を呼ばないこと。再入しない連続呼び出しでは発火しないこと。一般ユーザーで skip されずに走ること | AC-24 |
| `TestSyncCensusMatchesExpectation` | `internal/testutil/synccensus/census_guard_test.go` | 走査結果と期待表 16 行の双方向一致 | AC-23 |

`outputWrapper` は非公開型なので、AC-09 のテストはパッケージ `executor` の内部テストとして書く。

### 4.3 拡張する既存テスト

| テスト | 追加する検証 | 対応 AC |
|---|---|---|
| `internal/verification/path_resolver_test.go::TestPathResolver_ResolvePath` | サブテスト `answers from the cache once the command can no longer be resolved` を追加し、格納と参照が同じキーで往復することを公開 API だけで検証する。1回解決したのち実行ファイルを削除し、冷えたリゾルバが失敗する一方で元のリゾルバが同じ結果を返すことを見る（Step 2-8） | AC-07（D7） |
| `internal/runner/base/risktypes/types_test.go::TestVerifiedFD_FdAndIdempotentClose` | 既存ヘルパ `fdIsOpen` を使い、2回目の `Close` が `syscall.Close` を走らせないこと。fd 番号の再利用は `require.Equal` で必須条件として要求し、新たに開く fd は取得箇所で `t.Cleanup` に登録する（Step 2-10） | AC-10 |

### 4.4 削除・改名するテスト

| 削除・改名 | 属する削除 | 逐次側で検証するもの |
|---|---|---|
| `TestSudoUIDAdoptionReporter_ReportsOnlyOnceConcurrently` を削除 | D3 | `TestSudoUIDAdoptionReporter_ReportsOnlyOnce` |
| `TestSudoUIDExistenceMemo_Concurrent` を削除 | D4 | `TestSudoUIDExistenceMemo_ReusesConfirmation`、`TestSudoUIDExistenceMemo_DoesNotRememberFailures` |
| `TestSetProcessPermissionCheckUIDPolicy_Concurrent` を削除 | D6 | `TestSetProcessPermissionCheckUIDPolicy` |
| `TestResultCollector_Concurrency` を削除 | D8 | `TestResultCollector_RecordSuccess`、`_RecordFailure`、`_GetSummary`、`_MixedResults` |
| `TestVerifiedFD_ConcurrentClose` を削除 | D10 | 拡張後の `TestVerifiedFD_FdAndIdempotentClose` |
| `race_test.go` の4関数を削除 | D11 | **カバレッジ比較ではほとんど検証できない**（3関数は非 setuid 環境で skip され、残る `LockSerialization` も `ErrUnsupportedOperationType` で早期に戻る）。Step 3-4 の関数単位の議論で扱う。うち「デッドロックしない」「ロックが直列化する」の2件は本タスクが意図的に取り下げる性質であり、置き換えを作らない |
| `TestOutputCaptureIntegration_ConcurrentWrites` → `TestOutputCaptureIntegration_SequentialWrites` | — | 実体は逐次。名前を実体に合わせるだけで内容は変えない |
| `TestConcurrentExecutionConsistency` → `TestExecutionConsistencyAcrossModes` | D9 | 実体は goroutine ごとに別インスタンス。内容は変えない |
| `TestConcurrentExecution` → `TestDryRunExecutionAcrossIndependentManagers` | D9 | 同上。並行であること自体は事実なので名前から並行性を消さない |

### 4.5 テストが主張する理由で失敗できること（AC-19）

`02_architecture.md` §8.4 の表は AC ごとに壊し方を1つ挙げているが、**AC が複数の主張を含む場合は
主張ごとに壊す**。1つの壊し方で AC 全体を検証したことにはならないためである。対象は次のとおりで、
それぞれ該当する Step の「AC-19 の確認」に列挙してある。

| AC | 主張の数 | 壊す箇所 |
|---|---|---|
| AC-04 | 3（衝突・同値 no-op・不正値） | Step 2-6 |
| AC-05 | 1 | Step 2-3、Step 2-5 |
| AC-06 | 2（ヒットの再利用・失敗の非記憶） | Step 2-4 |
| AC-07 | 4（D2 のキャッシュ参照・D7 のキャッシュ格納・D7 のキャッシュ参照・D7 の格納キーと参照キーの一致） | Step 2-2、Step 2-8 |
| AC-08 | 3（通常版の登録・通常版の解放・dry-run 版の登録） | Step 2-9 |
| AC-09 | 2（stdout／stderr の識別・最初のエラー） | Step 2-1 |
| AC-10 | 1 | Step 2-10 |
| AC-24 | 1 | Phase 3 の完了ゲート |

### 4.6 維持するテスト

- `internal/runner/base/output/capture_test.go::TestCapture_ConcurrentAccess`（AC-18）。K2 を検証するものなので
  削除しない。各コミットで `-race` つきで通ることを確認する
- `internal/verification/shebang_chain_verifier_test.go::TestVerifyCommandDependencies_ConcurrentCallsAreRaceFree`
  （`02_architecture.md` §8.3）。維持し、D7・D8 の doc コメントに警告を書くことで対処する

### 4.7 テストヘルパの方針

**新規のテストヘルパファイルは作らない。** 必要な走査ヘルパ（`ProductionGoFiles`）は既存の
`internal/testutil/identitymutationguard` にあり、そのまま使える。既存の
`internal/groupmembership/test_helpers.go` と `test_helpers_policy.go`（いずれも `//go:build test`）は
D5・D6 への追随として**内容だけを変更**し、ファイルの新設・分割は行わない。

---

## 5. リスク管理

| リスク | 影響 | 対処 |
|---|---|---|
| 到達可能性の判定が誤っており、実際には並行だった | 権限判定の誤り（D2・D6）、解決パスの取り違え（D7）、二重 close による fd 再利用（D10）、特権の隙の重なり（D11） | 1件1コミット（AC-02）で revert 可能にする。`02_architecture.md` §3.1 の「誤判定時の最悪の結果」列が重い項目ほどレビューを厚くする。`-race` は §4.1 のとおり限定的な役割しか担わない |
| import の削除漏れでコミットが緑にならない | AC-20・AC-21 に反する | §1.3.3 の表を Phase 2 の共通手順の一部として毎回参照する。`manager.go`・`manager_test.go` の2件は順序依存があるため、後から来たコミットで落とす |
| D11 の再入ガードが既存の呼び出し経路を壊す | 特権コマンドが実行できなくなる | Step 3-1 でテストを先に書き、再入しない連続呼び出しでガードが発火しないことを検証する。テストが skip されずに走ることを完了条件に入れる |
| AC-24 のテストが CI で skip され、検証が空振りする | AC-24 の唯一の実行可能検証が無効になる | Step 3-1 で `OperationUserGroupExecution` を使う既存構成を流用し、完了条件を `--- PASS`（`--- SKIP` でない）とする |
| 設計文書の日本語版と英語版が食い違う | 原本に誤りが残る | 日本語版を先に編集し、英語版へは `/mktrans` で反映する（§1.3.6）。§8 の横断検索で両版を同時に検査する |
| census guard test が K3・K6 を取りこぼし、初日から失敗する | Phase 4 が進まない | ローカル変数宣言・短縮変数宣言・型式を持たない宣言をすべて拾う（§1.3.8）。Step 4-4 の確認2と確認3がこれを直接突く |
| カバレッジの比較基準を取り忘れる／構成が揃わない | AC-13 が検証できない、あるいは実体の無い低下が出る | Step 1-0 を Phase 1 の最初に置き、基準・比較とも `CGO_ENABLED=1` に固定する |

---

## 6. 実装チェックリスト

### 6.1 PR 単位の進捗

- [x] PR-1 マージ済み（対象ステップ: 1-0 / 1-1 / 1-2 / 1-3 / 1-4 / 1-5 / 1-6）
- [x] PR-2 マージ済み（対象ステップ: 2-1 / 2-2）
- [x] PR-3 マージ済み（対象ステップ: 2-3 / 2-4 / 2-5 / 2-6）
- [x] PR-4 マージ済み（対象ステップ: 2-7 / 2-8）
- [x] PR-5 マージ済み（対象ステップ: 2-9 / 2-10）
- [x] PR-6 マージ済み（対象ステップ: 3-1 / 3-2 / 3-3 / 3-4 / 3-5）
- [x] PR-7 マージ済み（対象ステップ: 4-1 / 4-2 / 4-3 / 4-4、および Phase 5 の最終確認）

### 6.2 PR ごとのステップ

#### PR-1: 維持対象の根拠と改名（AC-14〜AC-17）

- [x] Step 1-0: 起点 SHA と基準値（deadcode・カバレッジ）を `.git/0170-baseline/` に固定した
- [x] Step 1-1: K1 の4件に指定のリテラルを書いた
- [x] Step 1-2: K2 に指定のリテラルを書いた
- [x] Step 1-3: K3・K4・K5 に指定のリテラルを書いた
- [x] Step 1-4: K6 の2件に指定のリテラルを書いた
- [x] Step 1-5: `pwentMutex` の doc コメントを改訂した
- [x] Step 1-6: 3件のテストを実体に合わせて改名した
- [x] Phase 1 の完了ゲート: Step 1-1〜1-5 を1コミット、Step 1-6 を別コミットに分けた

#### PR-2: D1・D2 の削除

- [x] Step 2-1: D1 `outputWrapper.mu`
- [x] Step 2-2: D2 `GroupMembership.cacheMutex`（公開 API の警告と、キャッシュヒットの新規テストを含む）
- [x] §1.3.3 の import を落とした（`executor.go` の `sync`。`manager.go` の `sync` は D4 が未了なので**まだ落とさない**）

#### PR-3: D3〜D6 の削除（`groupmembership`）

- [x] Step 2-3: D3 `sudoUIDAdoptionReporter.reported`
- [x] Step 2-4: D4 `sudoUIDExistenceMemo.mu`
- [x] Step 2-5: D5 `nssCompletenessReporter.reported`
- [x] Step 2-6: D6 `processPermissionCheckUIDPolicy`
- [x] §1.3.3 の import を落とした（`manager.go` の `sync/atomic` は D3 のコミット、`sync` は D4 のコミット、`manager_test.go` の `sync` は D3・D4 の後のコミット、`policy.go` と `policy_test.go` は D6 のコミット）

#### PR-4: D8・D7 の削除（`verification`）

- [x] Step 2-7: D8 `ResultCollector.mu`
- [x] Step 2-8: D7 `PathResolver.mu`（`security-architecture` 行 437 を含む。キャッシュ参照のサブテストを追加）
- [x] §1.3.3 の import を落とした（`result_collector.go`・`result_collector_test.go`・`path_resolver.go`）

#### PR-5: D9・D10 の削除

- [x] Step 2-9: D9 `NormalResourceManager.mu`／`DryRunResourceManager.mu`
- [x] Step 2-10: D10 `VerifiedFD.closed`
- [x] §1.3.3 の import を落とした（`normal_manager.go`・`dryrun_manager.go`・`types.go`・`types_test.go`）
- [x] Phase 2 の完了ゲート: PR-2〜PR-5 の 10 コミットが main に並ぶ（PR-5 マージ後にリポジトリ履歴で確認する）

#### PR-6: D11 と再入ガード

- [x] Step 3-1: 再入ガードのテストを書き、一般ユーザーで skip されずに通ることを確認した
- [x] Step 3-2: `mu` を削除し再入ガードを実装した
- [x] Step 3-3: `privilege` パッケージと `runnertypes` の記述を追随させた
- [x] Step 3-4: `race_test.go` の4関数を削除し、関数単位にどのテストが検証しているかを記録した
- [x] Step 3-5: 設計文書の5箇所を日英両版で改訂した
- [x] Phase 3 の完了ゲート: Step 3-1〜3-5 を1コミットにまとめ、コミットメッセージの4行を書いた

#### PR-7: census guard test と全体の健全性

- [x] Step 4-1: テストファイルを作った
- [x] Step 4-2: 走査を実装した（フィールド・`var`・`:=` の3位置）
- [x] Step 4-3: 期待表（16 行）と双方向に突き合わせた
- [x] Step 4-4: 3通りの壊し方で失敗することを確認し、確認2で足したロックを元に戻した
- [x] Phase 4 の完了ゲート: `//go:build test` の新規ファイルが `make test`（`-tags test`）と `make lint`（`--build-tags test`）の双方でコンパイルされることを確認した
- [ ] `make deadcode` を基準と比較した（AC-22）
- [ ] `make test` が両構成で通り `-race` の警告が0件（AC-20）
- [ ] `make lint` が両構成で通る（AC-21）
- [ ] §7 のうち `<base>..HEAD` を見る行（AC-02・AC-03・AC-13 の2行目・AC-18 の2行目・AC-19）を、`.git/0170-baseline/base.sha` を `<base>` として実行した
- [ ] §7 の残りの静的検証コマンドをすべて実行した
- [ ] §8 の横断検索チェックリストを実行した

---

## 7. 受け入れ基準の検証

種別は `test`（実行可能で、挙動を壊すと失敗する）、`static`（`rg`／`git`／`make` による静的検証）、
`manual`（PR 上での観察）である。

**`<base>` の定義**: Step 1-0 で固定した起点コミット、すなわち `$(cat .git/0170-baseline/base.sha)` を
指す。`git merge-base main HEAD` は使わない——PR を1本ずつマージしていくと、その値は直前の PR の
先端へ動く。すると、コミットを数える検証（AC-02・AC-13 の2行目・AC-19）は常に 0 を返し、差分を見る
検証（AC-03・AC-18 の2行目）は最後の PR の差分しか見なくなって、主張する理由で失敗できなくなる。

**検証を行う PR**: `<base>..HEAD` を見る行は、すべての削除がマージされた後でなければ成立しないため
**PR-7 で実行する**。それ以外の行は、該当する変更を含む PR で実行する。
リテラルを検索する `rg` はすべて `-F`（固定文字列）で実行する。

| AC | 種別 | 検証方法 | 期待結果 |
|---|---|---|---|
| AC-01 | test | `internal/testutil/synccensus/census_guard_test.go::TestSyncCensusMatchesExpectation` | 期待表 16 行と走査結果が一致し、D1〜D11 の宣言が1つも現れない |
| AC-01 | static | `rg -n -e 'processPermissionCheckUIDPolicy\.(Load\|Store\|Swap\|CompareAndSwap)' -e '\.reported\.(Load\|Store\|Swap\|CompareAndSwap)' -e '\.closed\.(Load\|Store\|Swap\|CompareAndSwap)' -e 'cacheMutex' internal/ cmd/` | 0 件 |
| AC-01 | static | `rg -n '\.mu\.(RLock\|RUnlock\|Lock\|Unlock)' internal/runner/base/executor/executor.go internal/groupmembership/manager.go internal/verification/path_resolver.go internal/verification/result_collector.go internal/runner/resource/normal_manager.go internal/runner/resource/dryrun_manager.go internal/runner/base/privilege/unix.go` | 0 件 |
| AC-02 | static | `git log --oneline <base>..HEAD \| rg -c 'refactor\(0170\): remove D'` | 11 |
| AC-02 | static | `git log <base>..HEAD --format='%s%n%b' \| rg -c '^Rationale: '` | 11 以上 |
| AC-03 | static | `git diff --name-only <base>..HEAD \| rg -c 'internal/testutil/handlers.go'` | 0 件 |
| AC-03 | static | `git diff --name-only <base>..HEAD \| rg -v '^(docs/\|internal/\|cmd/)'` | 0 件（変更が `docs/`・`internal/`・`cmd/` に収まる） |
| AC-04 | test | `internal/groupmembership/policy_test.go::TestSetProcessPermissionCheckUIDPolicy` | `02_architecture.md` §7.2 の契約表の全行が本タスクの前後で変わらない |
| AC-05 | test | `internal/groupmembership/manager_test.go::TestSudoUIDAdoptionReporter_ReportsOnlyOnce`、`internal/groupmembership/nsswitch_test.go::TestNSSCompletenessReporter_ReportsOnlyOnce` | 3回呼んでも記録は1件 |
| AC-06 | test | `internal/groupmembership/manager_test.go::TestSudoUIDExistenceMemo_ReusesConfirmation`、同 `::TestSudoUIDExistenceMemo_DoesNotRememberFailures` | 確認済み UID は再問い合わせされず、失敗は毎回再問い合わせされる |
| AC-07 | test | `internal/groupmembership/manager_test.go::TestGetGroupMembers_CacheHitSkipsEnumeration`（未ヒット・失効）、同 `::TestCompletenessSurvivesCache`（ヒット）、同 `::TestGetGroupMembers_ErrorNotCached`（エラーを記録しない）、`internal/verification/path_resolver_test.go::TestPathResolver_ValidateAndCacheCommand`（格納）、同 `::TestPathResolver_ResolvePath` のサブテスト `answers from the cache once the command can no longer be resolved`（参照とキーの一致） | キャッシュヒット時・未ヒット時の返り値が変わらない。`TestGroupMembership` のキャッシュ系サブテストは `GetCacheStats` の件数のみを見るため、AC-19 の根拠には数えない（§1.3.4） |
| AC-08 | test | `internal/runner/resource/normal_manager_test.go::TestNormalResourceManager_CreateTempDir`、同 `::TestNormalResourceManager_CleanupTempDir`、同 `::TestNormalResourceManager_CleanupAllTempDirs`（Step 2-9 で追加。通常版の全解放を検証する唯一のテスト）、`internal/runner/resource/dryrun_manager_test.go::TestDryRunResourceManager_CreateTempDir`、同 `::TestDryRunResourceManager_CleanupTempDir`、`internal/runner/resource/default_manager_test.go::TestDefaultResourceManager_CleanupAllTempDirs`（dry-run の委譲のみ） | 登録・解放・全解放の挙動が通常版・dry-run 版とも変わらない |
| AC-09 | test | `internal/runner/base/executor/output_wrapper_test.go::TestOutputWrapper_SeparatesStdoutAndStderr`（`outputWrapper` 単体の識別と最初のエラー）、`cmd/runner` の `TestIntegration_TempDirHandling`・`TestIntegration_ErrorCleanup`・`TestIntegration_MultipleGroups`・`TestIntegration_CommandLevelWorkdir`（`executor.go:332-333` の配線の識別。§8.4 の「stdout 用と stderr 用を入れ替える」壊し方を捕まえるのはこちらである） | stdout と stderr の内容が取り違えられず、`GetWriteError` が最初のエラーを返す |
| AC-10 | test | `internal/runner/base/risktypes/types_test.go::TestVerifiedFD_FdAndIdempotentClose`、同 `::TestVerifiedFD_NilReceiverClose` | 二重 `Close` で `syscall.Close` は1回だけ走り（fd 番号の再利用で確認）、nil レシーバは `nil` を返す |
| AC-10 | static | `rg -F -n -e 'safe for concurrent use' -e 'CWE-1341' internal/runner/base/risktypes/types.go` | 0 件 |
| AC-11 | static | `rg -F -c -e 'This method does not serialize privilege windows.' -e 'the process-wide euid is raised' -e 'This is an unresolved design issue' internal/runner/base/privilege/unix.go` | 3 |
| AC-12 | static | `rg -ni -e 'mutex' -e 'thread.safe' -e 'safe for concurrent' -e 'protected from concurrent' -e 'acquired the .*lock' -g '!*_test.go' internal/runner/base/privilege/` | 0 件（HEAD では `unix.go:92-98,248,287` が該当するため、Step 3-3 を飛ばすと失敗する） |
| AC-13 | static | Step 1-0 の `.git/0170-baseline/covfunc-*.txt` と削除後の `CGO_ENABLED=1 go tool cover -func` 出力を関数単位で `diff` する | カバレッジが落ちた関数が0件（ただし `privilege` パッケージについては Step 3-4 の議論で代替する） |
| AC-13 | static | `git log <base>..HEAD --format='%s%n%b' \| rg -c '^Coverage: '` | 11 以上 |
| AC-14 | static | `rg -F -c -e 'os/exec starts one goroutine per writer' -e 'stdout and stderr wrappers share this Capture' internal/runner/base/output/capture.go` | 2 |
| AC-15 | static | `rg -F -c -e 'guards the fields below against the send worker started by go sd.run()' -e 'Flush and Close can both reach this from different goroutines' -e 'terminate waits here for the goroutines running sendSync' -e 'updated concurrently by the send worker and by callers' internal/logging/slack_sender.go` | 4（K1a〜K1d） |
| AC-15 | static | `rg -F -c 'this WaitGroup is what makes the Slack flush concurrent' internal/runner/bootstrap/logger.go` | 1（K3） |
| AC-15 | static | `rg -F -c 'output copy goroutine' internal/logging/log_line_tracker.go internal/redaction/error_collector.go` | 各ファイル 1 件以上（K4・K5） |
| AC-16 | static | `rg -F -c 'memoization, not mutual exclusion' internal/runner/base/executor/fdexec_linux.go internal/runner/base/risktypes/runas_ident.go` | 2 |
| AC-16 | static | `rg -F -c 'must not be replaced with a hand-written lazy initialization' internal/runner/base/risktypes/runas_ident.go` | 1 |
| AC-17 | static | `rg -F -c -e 'process-wide cursor' -e 'silently wrong enumeration' -e 'deliberately kept by task 0170' internal/groupmembership/membership_cgo.go` | 3 |
| AC-18 | test | `internal/runner/base/output/capture_test.go::TestCapture_ConcurrentAccess` | 削除されず、`-race` つきで通過する |
| AC-18 | static | `git diff <base>..HEAD -- internal/runner/base/output/capture_test.go` | 差分なし |
| AC-19 | static | `git log <base>..HEAD --format='%s%n%b' \| rg -c '^Falsification: '` | 14 以上（§4.5 の主張の総数） |
| AC-19 | manual | 各コミットの `Falsification:` 行が、壊し方と失敗したテスト名を具体的に述べていることを PR で確認する | §4.5 の表の主張が1つも欠けていない |
| AC-20 | static | `make test` | `CGO_ENABLED=1`（`-race`）と `CGO_ENABLED=0` の双方が通り、`-race` の警告が0件 |
| AC-21 | static | `make lint` | 両構成で通る |
| AC-22 | static | `make deadcode > .git/0170-baseline/deadcode-after.txt && diff .git/0170-baseline/deadcode.txt .git/0170-baseline/deadcode-after.txt` | 新たな報告行が増えていない |
| AC-23 | test | `internal/testutil/synccensus/census_guard_test.go::TestSyncCensusMatchesExpectation` | 走査結果と期待表 16 行が双方向に一致する |
| AC-23 | manual | Step 4-4 の3通りの壊し方でテストが失敗することを確認し、コミットメッセージに記す | 3通りすべてで失敗する |
| AC-24 | test | `internal/runner/base/privilege/unix_privilege_test.go::TestWithPrivileges_ReentrantCallIsRejected` | 再入時に `ErrReentrantPrivilegeCall` が返り `fn()` が呼ばれない。再入しない連続呼び出しでは発火しない。一般ユーザーで `--- PASS`（`--- SKIP` ではない） |
| AC-24 | static | `rg -F -n 'or the call deadlocks' internal/runner/base/runnertypes/config.go` | 0 件 |

**公開 API の警告（§1.3.5・`02_architecture.md` §8.3）の検証**（AC に直接は紐づかないが、設計原則4 の
適用として §8 ではなくここに置く）:

| 対象 | 種別 | 検証方法 | 期待結果 |
|---|---|---|---|
| D2・D7・D8 の型 | static | `rg -F -c 'not safe for concurrent use' internal/groupmembership/manager.go internal/verification/path_resolver.go internal/verification/result_collector.go` | 各ファイル 1 件 |

---

## 8. 横断検索チェックリスト

`make lint` と `make test` が検出できないものだけを挙げる。§7 に既にあるコマンドは繰り返さない。
検索範囲から `docs/tasks/` を除くのは、完了済みタスクの文書が当時の記述をそのまま保持しており、
本タスクで編集してはならないためである。

- [ ] **削除・改名した識別子の残存参照（現行のコードと文書）**:
      `rg -n -e 'previewExitCodeLocked' -e 'refreshDryRunResultLocked' -e 'cacheMutex' internal/ cmd/ docs/dev/`
      → 0 件
- [ ] **`pwentMutex` の参照先**: `rg -n 'pwentMutex' internal/ cmd/ docs/dev/`
      → `internal/groupmembership/membership_cgo.go` と
      `internal/testutil/synccensus/census_guard_test.go`（期待表の1行）のみ
- [ ] **設計文書の日英差分（コード例）**:
      `rg -n -e 'sync\.Mutex' -e 'sync\.RWMutex' -e 'm\.mu\.' docs/dev/architecture_design/security-architecture.ja.md docs/dev/architecture_design/security-architecture.md`
      → 両版とも 0 件（行 309・322-323・437 を検査する）
- [ ] **設計文書の日英差分（散文）**:
      `rg -n -e 'グローバルmutex' -e 'global mutex' -e 'スレッドセーフ' -e 'Thread-safe' docs/dev/architecture_design/security-architecture.ja.md docs/dev/architecture_design/security-architecture.md`
      → 両版とも 0 件（行 407・1192/1197・1256/1261 を検査する）
- [ ] **開発者ガイドに残る特権まわりの mutex の記述**:
      `rg -n -e 'グローバルミューテックス' -e 'global mutex' docs/dev/developer_guide/`
      → 0 件（Step 3-5 で `design-implementation-overview` の日英2箇所ずつを消す）
- [ ] **脅威モデルの対策欄が空にならないこと**:
      `security-architecture.ja.md` の脅威「特権処理における競合状態」の行に、対策の削除だけでなく
      残存リスクの記述が入っていることを目視で確認する（`rg` では対策欄と残存リスク欄を区別できない）
- [ ] **旧テスト名の残存**:
      `rg -n -e 'TestConcurrentExecution' -e 'TestOutputCaptureIntegration_ConcurrentWrites' -e 'TestUnixPrivilegeManager_' -e 'TestVerifiedFD_ConcurrentClose' -e 'TestResultCollector_Concurrency' -e 'TestSetProcessPermissionCheckUIDPolicy_Concurrent' -e 'TestSudoUIDExistenceMemo_Concurrent' -e 'TestSudoUIDAdoptionReporter_ReportsOnlyOnceConcurrently' internal/ docs/dev/`
      → 0 件
- [ ] **`synccensus` という識別子の衝突**: `rg -n 'synccensus' internal/ cmd/`
      → 新規ディレクトリとその中のファイルのみ
- [ ] **`ErrReentrantPrivilegeCall` の判定が `errors.Is` で行われていること**:
      `rg -n 'ErrReentrantPrivilegeCall' internal/` → 定義1件、`unix.go` の doc コメントと `return` 、
      テストでの `errors.Is` 経由の参照（合計4件以上）。文字列比較が1件も無いこと
- [ ] **本タスクで追加した Go の行に日本語が混入していないこと**:
      `git diff <base>..HEAD -- '*.go' | rg -c -P '^\+.*[\p{Hiragana}\p{Katakana}\p{Han}]'`
      → 0 件（既存の `error_scenarios_test.go:145` の `"echo 'こんにちは世界'"` のような既存行は
      追加行ではないので一致しない）

---

## 9. 成功基準

- `01_requirements.md` の AC-01〜AC-24 がすべて満たされ、§7 の検証がすべて期待結果と一致する
- 外部から観測できる挙動が本タスクの前後で変わらない。CLI の出力、エラーの種類、ログの内容、
  権限判定の結果のいずれも変化しない。唯一の例外は、再入という既存のバグが実在した場合に
  `ErrReentrantPrivilegeCall` が観測されるようになることであり、これは静かな特権喪失より望ましい
  （`02_architecture.md` §3.4）
- D1〜D11 の各件が独立して revert できる
- production コードに残る排他制御を読んだ読み手が、それぞれについて「どの goroutine と
  どの goroutine の間で並行なのか」を doc コメントから答えられる
- 特権まわりを次に触る者が、`WithPrivileges` が並行安全ではないこと、およびそれが未解決の設計課題で
  あることを doc コメントから知る
- 本タスク後に、census guard test が走査する形（構造体フィールド、`var` 宣言、`:=` 宣言、
  `sync.OnceX` の初期化子）で新しいロックを足すと、テストが失敗して追加の是非がレビューに
  可視化される。走査しない形（素の `int64` に対する `atomic.AddInt64` など、同期プリミティブの
  **型**を伴わない同期）は捕まえない。これは `02_architecture.md` §4.6 が述べる一方向性に加わる
  もう1つの限界であり、必要になった時点で走査を広げる

---

## 10. 次のステップ

- `02_architecture.md` §2.2 のファイル数（23 → 24）と §3.6 の責務表に
  `security-architecture.ja.md` の行を反映する（§1.3.6 の注記）
- PR-1 から順に実装を進め、各ステップのチェックボックスを実時間で更新する。1つの PR がマージ
  されるまで次の PR の作業を始めない（各 PR 作成ポイントの最後のチェックボックス）
- Phase 5 の完了後、`02_architecture.md` §10 に挙げた将来課題（特権操作の設計のやり直し、
  `pwentMutex` の扱い、census の保護漏れ検出への拡張）は本タスクの範囲外として、必要になった時点で
  別タスクを起こす
