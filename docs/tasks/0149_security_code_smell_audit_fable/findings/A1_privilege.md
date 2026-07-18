# A1: internal/runner/base/privilege/ セキュリティ監査所見

- 監査日: 2026-07-18
- 対象: `internal/runner/base/privilege/` (unix.go, manager.go, errors.go, metrics.go, identity_linux.go, identity_other.go)
- 方法: ソースコードの静的分析（読み取り専用）。テストコードおよび主要呼び出し元（`internal/runner/base/executor/executor.go`, `internal/runner/resource/*.go`）も参照。

## サマリ

| 重大度 | 件数 |
|---|---|
| 🔴 High | 0 |
| 🟡 Medium | 2 |
| 🟠 Low | 5 |
| 🔵 Info | 4 |

---

## 所見

### 🟡 M-1: 特権昇格ウィンドウがプロセス全体に及び、コールバック実行中は親プロセス全体が euid=0 で動作する

- **該当箇所**: `internal/runner/base/privilege/unix.go:89-109` (`WithPrivileges`), `unix.go:294` (`syscall.Seteuid(0)`), 呼び出し元 `internal/runner/base/executor/executor.go:236-240`
- **問題**: `syscall.Seteuid` は（Go 1.16+ の Linux では全スレッドに適用されるため）プロセス全体の実効 UID を変更する。`m.mu` はマネージャ経由の操作を直列化するだけで、プロセス内の他の goroutine（ログ出力、出力キャプチャ、シグナルハンドラ等）は昇格ウィンドウ中 root 権限で動作する。特に `OperationUserGroupExecution` では `fn()` が子プロセスの実行完了まで（コマンドの全実行時間）を包含するため、昇格ウィンドウは一瞬ではなく長時間になる。子プロセス自体は `syscall.Credential` で降格されるが、親プロセス側で行うファイル操作（出力ファイルの作成等）は euid=0 で実行され、root 所有ファイルの作成や symlink 攻撃を受けた場合の被害が増幅される。
- **悪用シナリオ**: 昇格ウィンドウ中に親プロセス側の別 goroutine が攻撃者の管理可能なパス（例: 出力ディレクトリ）へ書き込みを行う場合、symlink 差し替えにより root 権限での任意ファイル上書きに繋がり得る（実際の可否は output/safefileio 側の防御に依存）。
- **推奨対応**: 設計上の制約（seteuid ベースの特権管理）として文書化されているかを確認しつつ、(1) 昇格ウィンドウを fork/exec 直後まで最小化する（子プロセスの完了待ちは降格後に行う）、(2) 昇格中の親プロセス側ファイル操作を禁止・監査する、のいずれかを検討。少なくとも呼び出し規約（fn 内で親側の書き込みをしない）をコメントで明示すべき。

### 🟡 M-2: `changeUserGroupInternal` の実降格パス（setegid/seteuid 実行部）が本番コードから到達不能なデッドコード

- **該当箇所**: `internal/runner/base/privilege/unix.go:154-166` (`prepareExecution` の operation 分岐), `unix.go:558-588` (実降格・ロールバック処理), `unix.go:179-189` (`performElevation` のロールバックブロック)
- **問題**: `needsUserGroupChange=true` になるのは `OperationUserGroupDryRun` のみで、その場合 `changeUserGroupInternal` は `dryRun=true` で呼ばれ line 566 で早期リターンする。したがって:
  - line 570-580 の実際の `Setegid`/`Seteuid` 呼び出しと EGID ロールバック、`emergencyShutdown("egid_rollback_failure_...")` は本番のどの操作からも到達不能。
  - `performElevation` line 182-186 のロールバックブロックも、`needsPrivilegeEscalation && needsUserGroupChange` が同時に true になる operation が存在しないため到達不能。
  - 実際のユーザー切替は executor 側の `syscall.Credential`（子プロセス起動時）で行われており、`WithUserGroup` という名前（unix.go:592）と実装（root 昇格のみ、RunAsUser/RunAsGroup はログ用途）が乖離している。
- **悪用シナリオ**: 直接の悪用はないが、特権 syscall を含む未使用コードは将来の変更で意図せず有効化された場合の検証が不十分になりやすく、監査コストを増大させる（命名と実態の乖離により、レビュー者が「fn は対象ユーザー権限で実行される」と誤解する危険がある）。
- **推奨対応**: 到達不能な実降格パスを削除するか、`OperationUserGroupExecution` の実フローが「root 昇格 + 子プロセス Credential 降格」であることを `WithUserGroup`/`WithPrivileges` の doc コメントに明記する。YAGNI 原則（CLAUDE.md）にも整合する。

### 🟠 L-1: `changeUserGroupInternal` 内で `user.Lookup(userName)` を二重呼び出し（TOCTOU / DRY 違反）

- **該当箇所**: `internal/runner/base/privilege/unix.go:492` と `unix.go:527`
- **問題**: ユーザー名解決とプライマリグループ解決で同一ユーザーを 2 回 lookup している。NSS バックエンド（LDAP 等）ではその間にエントリが変わり、UID とプライマリ GID が別のアカウント状態から取得される可能性がある（現状 dry-run 専用パスのため実害は限定的だが、ログに不整合な情報が出得る）。
- **推奨対応**: 最初の `user.Lookup` の結果を保持して再利用する。

### 🟠 L-2: `escalatePrivileges`/`restorePrivileges` が注入可能な `m.syscallSeteuid` ではなく `syscall.Seteuid` を直接呼んでいる

- **該当箇所**: `internal/runner/base/privilege/unix.go:294`, `unix.go:324`（対比: `unix.go:570,574,576` は注入フィールドを使用）
- **問題**: テスト注入用フィールド (`syscallSeteuid`/`syscallSetegid`) が定義されているにもかかわらず、最も重要な昇格・復元パスは直接 syscall を呼ぶため、この経路（特に復元失敗→`emergencyShutdown`）を単体テストで網羅するには root/setuid 環境が必要になる。失敗時分岐のテストカバレッジ不足に繋がる。
- **推奨対応**: 昇格・復元でも注入フィールドを使用し、復元失敗時の emergencyShutdown 経路をモックでテストする。

### 🟠 L-3: `restorePrivilegesAndMetrics` の metrics 条件に恒偽の項が含まれ、記録セマンティクスが不明瞭

- **該当箇所**: `internal/runner/base/privilege/unix.go:232`
- **問題**: `} else if panicValue == nil && (execCtx.needsPrivilegeEscalation || execCtx.needsUserGroupChange) {` — この else 分岐に入る時点で `needsPrivilegeEscalation` は必ず false なので、条件の第一項は恒偽（実質 `needsUserGroupChange` のみ）。また:
  - `prepareExecution` の失敗（operation 不正・saved-set 読み取り失敗）も `RecordElevationFailure` として計上され（unix.go:95）、実際に昇格を試行していないのに「昇格失敗」となる。
  - `duration = time.Since(execCtx.start)` は `fn()`（コマンド全実行）を含むため、`AverageElevationTime` 等の指標名と実態（操作全体の時間）が乖離している。
- **推奨対応**: 恒偽項の削除、metrics の対象範囲（昇格 syscall のみか操作全体か）の明確化と命名修正。

### 🟠 L-4: `fn()` 実行中も `m.mu` を保持し続けるため、再入で自己デッドロックする

- **該当箇所**: `internal/runner/base/privilege/unix.go:90-91, 106`
- **問題**: `WithPrivileges` はコールバック実行中もミューテックスを保持する。fn 内（またはそこから呼ばれるコード）が同一マネージャの `WithPrivileges`/`WithUserGroup` を呼ぶと即デッドロックする。特権直列化のためには保持が必要な設計だが、再入禁止がコメント・doc に明示されていない。
- **推奨対応**: doc コメントに再入禁止を明記する。必要なら再入検出（保持中フラグ）でエラー返却にする。

### 🟠 L-5: `Metrics` 構造体が `sync.RWMutex` を含んだまま値として返却される（copylocks フットガン）

- **該当箇所**: `internal/runner/base/privilege/metrics.go:10-21`, `metrics.go:63-79` (`GetSnapshot`), `unix.go:455-457` (`GetMetrics`)
- **問題**: `GetSnapshot`/`GetMetrics` は `Metrics` を値で返す。実装はフィールドを個別コピーしているため mu はゼロ値になり動作上は安全だが、ロックを含む型を値渡しする API は `go vet` の copylocks 警告対象であり、将来 `snapshot := *m` のような変更が入ると競合検知不能なバグになる。スナップショット用に mutex を含まない別型（例: `MetricsSnapshot`）を分けるのが安全。
- **推奨対応**: mutex を持つ内部型と、返却用の POD スナップショット型を分離する。

### 🔵 I-1: `GetCurrentUID` は実際には effective UID を返す（命名と実装の乖離）

- **該当箇所**: `internal/runner/base/privilege/unix.go:361-364`
- **問題**: メソッド名は "CurrentUID" だが `syscall.Geteuid()` を返す。real UID と区別が必要な文脈（本パッケージはまさにそれ）で誤読を招く。`GetCurrentEUID` 等への改名、または doc コメントでの明示が望ましい。

### 🔵 I-2: 非 Linux (darwin 等) では saved-set-uid/gid 検証が構造的にスキップされる

- **該当箇所**: `internal/runner/base/privilege/identity_other.go:12-14`, `unix.go:140-143, 258`
- **問題**: darwin では `getresuid` 相当が使えないため saved-set 不変条件チェックが無効化され、EUID==UID/EGID==GID チェックのみになる。エラー型でゲートする実装（フェイルクローズドではなく明示的スキップ）は適切だが、本番デプロイ対象が Linux であることを前提とした防御差であることをセキュリティ文書に記載しておくべき。

### 🔵 I-3: `isRootOwnedSetuidBinary` の `os.Executable` + `os.Stat` は起動時 1 回の判定であり軽微な TOCTOU があるが fail-safe

- **該当箇所**: `internal/runner/base/privilege/unix.go:388-447`
- **問題**: 実行ファイルパスの stat はプロセス起動後の状態を見るため、理論上はバイナリ差し替えと競合し得る。ただし判定を誤って true にしても、実際の `Seteuid(0)` は setuid で起動されていなければ EPERM で失敗する（fail-closed）ため実害はない。誤って false になった場合も機能縮退（特権実行不可）であり安全側。現状のままで許容範囲。

### 🔵 I-4: `errors.go` の sentinel エラー群（`ErrPrivilegeElevationFailed` 等）が本番コードで未使用

- **該当箇所**: `internal/runner/base/privilege/errors.go:12-17`
- **問題**: `ErrPrivilegeElevationFailed`/`ErrPrivilegeRestorationFailed`/`ErrInvalidUID`/`ErrPrivilegedExecutionNotSupported` はテストコードでしか参照されていない。実際のエラーは `&Error{...}` やインラインの `fmt.Errorf` で生成されており、呼び出し側が `errors.Is` で分類できるエラーモデルと乖離している。未使用エラーの整理、または昇格・復元失敗時にこれらでラップする統一を推奨。

---

## 観察された良好な防御層

1. **失敗時フェイルクローズド（emergencyShutdown）**: 特権復元失敗・identity 検証失敗・saved-set 検証失敗のいずれでも `os.Exit(1)` で即時プロセス終了し、昇格状態のまま実行を継続しない（unix.go:335-354）。構造化ログと stderr の二重出力も適切。
2. **復元後の独立した identity 検証**: 復元ロジック自身とは独立に `EUID==UID` / `EGID==GID` を検証する defense-in-depth（unix.go:78-86, 241-245）。
3. **Linux での saved-set-uid/gid 不変条件検証**: `getresuid`/`getresgid` により、EUID 復元後でも saved-set が汚染されていないことまで検証する強い不変条件（identity_linux.go, unix.go:246-270）。負値検証によるフェイルクローズドも実装済み。
4. **panic 安全性**: `recover` → 特権復元 → 再 `panic` の順で、コールバックが panic しても特権リークしない（unix.go:195-218）。
5. **mutex による特権操作の直列化**: 同一マネージャ経由の昇格操作は並行実行されない（unix.go:90-91）。race_test.go による並行テストも存在。
6. **EGID ロールバック**: Setegid 成功後に Seteuid が失敗した場合の EGID 巻き戻しと、巻き戻し失敗時の emergencyShutdown（unix.go:574-580。ただし M-2 のとおり現状は到達不能パス）。
7. **setuid バイナリ判定の厳格性**: setuid ビット + root 所有 + 非 root 実 UID の 3 条件を要求し、実行時 UID/EUID だけに依存しない検出（unix.go:388-447）。
8. **dry-run の分離**: dry-run は identity を一切変更せず、検証スキップも構造的（operation 種別ゲート）に行われる。
9. **呼び出し元 (executor) のフェイルクローズド**: run-as identity 解決失敗や supplementary groups が nil の場合に実行を拒否し、`syscall.Credential` で `NoSetGroups: false` を明示して補助グループを必ずリセットする（executor.go:190-222）。
