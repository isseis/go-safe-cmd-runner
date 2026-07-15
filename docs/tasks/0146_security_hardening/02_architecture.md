# セキュリティ堅牢化（セキュリティリスク評価レポート対応） — アーキテクチャ設計書

## Document Status

| Item | Value |
|---|---|
| Status | `approved` |
| Created | 2026-07-14 |
| Review date | 2026-07-14 |
| Reviewer | isseis |
| Comments | - |

## 0. 本書の位置づけ

本書は [`01_requirements.md`](01_requirements.md)（status: `approved`）で定義した振る舞い（WHAT）を、
実現機構（HOW）へ落とし込む設計文書である。対象はセキュリティリスク評価レポート
[`00_security_risk_report.md`](00_security_risk_report.md) の検出事項 F-1〜F-7（本書での対応要件 F-001〜F-007）である。

要件書が設計判断に委ねた主要論点は次の 2 点であり、本書で結論を確定する。

- 補助グループ／saved-set-uid のアトミック設定機構（要件 F-001 / F-002、前提・依存節）
  → §3.1 で `os/exec` の `SysProcAttr.Credential` 採用を確定する。
- ファイルサイズ上限の設定可能化／上限分離の可否（要件 F-006）
  → §3.6 で「本タスクでは実装せず、根拠をドキュメント化するにとどめる」を確定する。

用語は [`docs/translation_glossary.md`](../../translation_glossary.md) に従う（「フェイルクローズド」「Redaction」等）。

---

## 1. 設計の全体像

### 1.1 設計原則

1. **フェイルクローズド徹底**（要件 NF-002）: 権限設定・信頼の起点で不確実性が生じたら実行を止める。
   新たな fail-open を導入しない。
2. **後方互換の保持**（要件 NF-001）: 権限昇格を伴わない既存経路（native root 実行、`run_as` 未使用、dry-run 正常系）の
   挙動を変えない。変更は run-as を使う経路と、外部流出面（ログ／Slack）に限定する。
3. **既存資産の再利用（DRY）**: 補助グループ解決・機微情報 Redaction・ディレクトリ権限チェックには、
   コードベースに既存の実装が存在する。新規に作らず、それらを整理・接続する。
4. **カーネル側アトミック設定の優先**: ユーザー切替はプロセス内 `seteuid` の時間差ではなく、
   `execve` 時にカーネルが uid/gid/補助グループを一括設定する方式へ移す（§3.1）。

### 1.2 コンセプトモデル

本タスクが堅牢化するのは、信頼境界を跨ぐ 3 つの面である。いずれも「信頼境界の起点となるデータ →
本タスクで堅牢化するコンポーネント → 到達先データ」という一方向・一直線の流れであり、互いに独立している。
そのため下図では 3 つの面を同一方向（左→右）の並行レーンとして並べ、各面が同じ形（起点 → 堅牢化対象 →
到達先）で読めるようにする。

| 面 | 信頼境界の起点 | 堅牢化するコンポーネント | 到達先 | 対応要件 |
|---|---|---|---|---|
| 下方委譲面（特権分離） | 起動元 identity（root / setuid-root） | privilege → executor | run_as 対象ユーザー identity | F-001 / F-002 |
| 外部流出面（機微情報） | コマンド出力 / 引数 / 環境変数値 | redaction.ValueDetector → SlackHandler | 外部送信先 / ログ | F-003 |
| 信頼の起点（生成と検証） | cmd/record | ハッシュDB → verification（dry-run） | 後段 runner の検証 | F-004 / F-005 |

（F-006 / F-007 はコンポーネントの流れではなく可用性・環境上の前提のドキュメント化であり、本図には含めない。）

```mermaid
flowchart LR
    classDef process fill:#fff1e6,stroke:#ff7f0e,stroke-width:1px,color:#8a3e00;
    classDef enhanced fill:#e8f5e8,stroke:#2e8b57,stroke-width:2px,color:#006400;
    classDef data fill:#e6f7ff,stroke:#1f77b4,stroke-width:1px,color:#0b3d91;

    subgraph Down["下方委譲面（特権分離, F-001/F-002）"]
        direction LR
        ROOT[("起動元 identity<br>root / setuid-root")] --> P1["privilege<br>UnixPrivilegeManager"] --> E1["executor<br>DefaultExecutor"] --> TARGET[("run_as 対象ユーザー<br>identity")]
    end

    subgraph Out["外部流出面（機微情報, F-003）"]
        direction LR
        OUTSRC[("コマンド出力/引数/<br>環境変数値")] --> R1["redaction<br>ValueDetector"] --> S1["logging<br>SlackHandler"] --> SINK[("外部送信先/ログ")]
    end

    subgraph Trust["信頼の起点（生成と検証, F-004/F-005）"]
        direction LR
        REC["cmd/record"] --> HDB[("ハッシュDB<br>信頼の起点")] --> VER["verification<br>dry-run fallback"]
    end

    class P1,E1,R1,REC,VER enhanced
    class S1 process
    class ROOT,TARGET,OUTSRC,SINK,HDB data

    subgraph Legend["凡例"]
        direction LR
        L1["変更/追加コンポーネント"]:::enhanced
        L2["既存コンポーネント"]:::process
        L3[("データ")]:::data
    end
```

> 各レーンは 1 つの面を表し、実線矢印 A → B は左から右への一方向の流れ（データ・制御が A から B へ渡る）を表す。
> 3 レーン間に依存関係はない。円柱形はデータ、着色はコンポーネントの種別を示す（凡例参照）。
> 凡例のノードは色分けの意味のみを示し、相互関係は表さない。

---

## 2. システム構成

### 2.1 変更対象パッケージと責務

各要件の変更対象を、既存パッケージ上に配置する（新規パッケージは追加しない）。
F-003 の値ベース検出器のみ既存 `internal/redaction` 内に新規ファイルとして追加する。

```mermaid
flowchart LR
    classDef process fill:#fff1e6,stroke:#ff7f0e,stroke-width:1px,color:#8a3e00;
    classDef enhanced fill:#e8f5e8,stroke:#2e8b57,stroke-width:2px,color:#006400;
    classDef newpkg fill:#ffe8f5,stroke:#d946ef,stroke-width:2px,color:#701a75;
    classDef data fill:#e6f7ff,stroke:#1f77b4,stroke-width:1px,color:#0b3d91;

    subgraph priv["internal/runner/base/privilege"]
        UNIX["unix.go<br>UnixPrivilegeManager"]
    end
    subgraph exec["internal/runner/base/executor"]
        EXE["executor.go<br>DefaultExecutor"]
    end
    subgraph red["internal/redaction"]
        VD["value_detector.go<br>ValueDetector (新規)"]
        RC["redactor.go<br>Config.RedactText"]
    end
    subgraph verpkg["internal/verification"]
        MGR["manager.go / result_collector.go"]
    end
    subgraph rec["cmd/record"]
        RECM["main.go"]
    end
    subgraph secpkg["internal/security"]
        TOC["toctou.go (既存, 戻り値を利用)"]
    end
    subgraph docs["docs/user (ドキュメント)"]
        DOC["runner/record/security 各ドキュメント"]
    end

    EXE -->|"WithPrivileges で root 昇格を要求"| UNIX
    EXE -->|"子プロセス Credential 設定"| CRED[("SysProcAttr.Credential<br>Uid/Gid/Groups")]
    VD --> RC
    RC -->|"Layer1/Layer2 で適用"| SLK["logging/slack_handler.go"]
    RECM -->|"違反時 fail-closed"| TOC
    MGR --> RESULT[("dry-run 検証サマリ<br>UNVERIFIED 区別")]

    class UNIX,EXE,RC,MGR,RECM enhanced
    class VD newpkg
    class TOC,SLK,DOC process
    class CRED,RESULT data

    %% Legend
    subgraph Legend["凡例"]
        LA["変更コンポーネント"]:::enhanced
        LB["新規追加"]:::newpkg
        LC["既存（無変更/再利用）"]:::process
        LD[("データ")]:::data
    end
```

> 矢印 A → B は「A が B を利用する、または A が B（データ）を生成・供給する」ことを表し、各エッジの
> 具体的な関係はラベルで明示する（データノードは円柱形で区別する）。凡例のノードは色分けの意味のみを示す。

### 2.2 コンポーネント責務表

新規・変更するファイルと、更新が必要な既存テストを列挙する。

| ファイル | 対応要件 | 責務（変更点） | 更新が必要な主なテスト |
|---|---|---|---|
| `internal/runner/base/executor/executor.go` | F-001, F-002 | run-as コマンド実行を `SysProcAttr.Credential` によるカーネル側アトミック設定へ移行。子プロセスの uid/gid/補助グループを一括設定。fd-bound 実行・staging fallback（いずれも §3.1.3 で説明）と併用。 | `executor_test.go`, `executor_privilege_test.go`（存在すれば）, `executor_usergroup_test.go` |
| `internal/runner/base/privilege/unix.go` | F-001, F-002 | `OperationUserGroupExecution` の親プロセス処理を「root 昇格のみ・親のユーザー/グループは切替えない」に変更（子 Credential へ委譲）。`executionContext` に操作開始時の saved-set-uid/gid を捕捉し、復元後の不変条件チェックへ suid/sgid 検証を追加。 | `unix_test.go`, `unix_privilege_test.go`, `race_test.go`, `manager_test.go` |
| `internal/runner/base/risktypes/runas_ident.go`（想定）ほか run-as identity 解決 | F-001, F-002 | run-as identity（uid/gid/補助グループ）解決を単一の共有関数に集約し、`risk` と `executor`/`privilege` の双方から呼ぶ（新規の並行実装を作らない。§3.1.2）。 | `runas_identity_test.go`, 新規テスト |
| `internal/runner/base/privilege/identity_unix.go`（新規想定） | F-002 | 操作開始時の saved-set-uid/gid 読み取り（Linux: `x/sys/unix.Getresuid`/`Getresgid`）と復元後検証。非 Linux ではガードした no-op。 | 新規テスト |
| `internal/redaction/value_detector.go`（新規） | F-003 | キー名に依存しない値ベースの秘密検出・マスク（既知フォーマット）。 | 新規テスト |
| `internal/redaction/redactor.go` | F-003 | `Config` に `ValueDetector` を統合し、`RedactText` から値ベース検出を適用。 | `redactor_test.go` |
| `internal/redaction/sensitive_patterns.go` | F-003 | 既知フォーマット用パターンの定義追加（既存キー名パターンは維持）。 | `sensitive_patterns_test.go` |
| `internal/verification/manager.go` | F-004 | `readAndVerifyFileWithReadFallback` の2 つのフォールバック経路（検証器 nil／検証失敗）両方で、検証を通さず返した内容を「未検証（UNVERIFIED）」として検証サマリへ伝播。 | `manager_test.go` |
| `internal/verification/result_collector.go` / `types.go` | F-004 | 未検証内容採用のフラグ（`UsedUnverifiedContent`）と、その理由（`skipped_no_validator` / `verify_failed_<reason>`）を検証サマリに保持。 | `result_collector_test.go` |
| `internal/runner/resource/formatter.go` ほか dry-run 出力 | F-004 | UNVERIFIED を出力上で明示表示（text / json 両形式）。 | dry-run 出力系テスト |
| `internal/runner/resource/dryrun_manager.go` | F-004 | 未検証内容採用時の dry-run 終了コード判定（§3.4.3 の終了コード対応表）。 | `dryrun_manager_test.go` |
| `internal/runner/resource/types.go` | F-004 | `FailOnVerificationUnavailable` の対象拡張に伴う doc コメント・意味変更（§3.4.3）。終了コード定数の対応を明確化。 | `dryrun_manager_test.go` |
| `cmd/record/main.go` | F-005 | `RunTOCTOUPermissionCheck` の戻り値を評価し、違反があれば ERROR ログ出力のうえ無条件で非ゼロ終了（ハッシュ生成せず。バイパスフラグは設けない）。`hashDirPermissions` を `0o750` → `0o700`。 | `main_test.go` |
| `docs/user/runner_command.*.md`, `record_command.*.md`, `security-risk-assessment.*.md` ほか | F-003, F-004, F-005, F-006, F-007 | 運用前提・限界の明文化（各 AC 参照）。 | `static` 検証 |

---

## 3. コンポーネント設計

### 3.1 F-001 / F-002: run-as を `SysProcAttr.Credential` でカーネル側アトミック設定に移す

#### 3.1.1 なぜ既存方式（親プロセス seteuid）では不十分か

現状、run-as コマンド実行は `DefaultExecutor.executeWithUserGroup` が
`PrivilegeManager.WithPrivileges(OperationUserGroupExecution, …)` を呼び、その内部で
親プロセスを「root へ昇格 → `changeUserGroupInternal` で親の EUID/EGID を対象ユーザーへ切替 →
`execCmd.Run()` を同期実行 → 復元」する構造である。この方式には次の限界がある。

- 親プロセスの補助グループを一切操作しないため、setuid-root 配備時は起動元 root の補助グループ
  （`docker`, `wheel` 等）が子へそのまま継承される（要件 F-001 / AC-01）。
  `setgroups(2)` はコードベース全体で未使用。
- 切替が `Setegid`/`Seteuid` のみで、`euid=target, ruid=originalUser, suid=0` の区間が生じる
  （要件 F-002 / AC-05）。復元は `restoreUserGroupInternal` → `restorePrivileges` の二段構えで複雑。

これらは「親プロセスを対象ユーザーへ seteuid する」構造自体に起因するため、同構造を保ったまま
`setgroups` を足すより、子プロセスの identity をカーネルが `execve` 時に確定する方式へ移すほうが
根本的な解決になる。要件書・レポートの推奨（`SysProcAttr.Credential`）と一致する。

#### 3.1.2 採用する新方式

run-as コマンド実行に限り、次の流れに変更する（ファイル検証等の親プロセス内特権処理は現行
`WithPrivileges` を維持。要件 F-002 の許容範囲）。

```mermaid
flowchart TB
    classDef enhanced fill:#e8f5e8,stroke:#2e8b57,stroke-width:2px,color:#006400;
    classDef problem fill:#ffe6e6,stroke:#d62728,stroke-width:2px,color:#7b0000;

    subgraph Before["Before（親プロセスを対象ユーザーへ切替）"]
        direction LR
        B1["root へ昇格 (Seteuid 0)"]
        B2["親を対象ユーザーへ<br>Setegid/Seteuid"]
        B3["execCmd.Run()<br>親=対象ユーザーで実行"]
        B4["補助グループ=root のまま継承"]
        B1 --> B2 --> B3
        B3 -.->|"問題"| B4
        class B2,B4 problem
    end

    subgraph After["After（子プロセス Credential で確定）"]
        direction LR
        A1["root へ昇格 (Seteuid 0)<br>※親のユーザー/グループは変えない"]
        A2["対象ユーザーの identity 解決<br>uid/gid/補助グループ"]
        A3["execCmd.SysProcAttr.Credential =<br>Uid/Gid/Groups, NoSetGroups=false"]
        A4["execCmd.Run()<br>カーネルが execve 時に一括設定"]
        A5["親を操作開始時の状態へ復元<br>+ 不変条件 (euid/egid/suid/sgid) 検証"]
        A1 --> A2 --> A3 --> A4 --> A5
        class A1,A2,A3,A4,A5 enhanced
    end

    subgraph Legend["凡例"]
        direction LR
        LP["現状の欠陥箇所"]:::problem
        LE["新方式の処理"]:::enhanced
        LD["共通/標準の処理"]
    end

    Before ~~~ After ~~~ Legend
```

> 実線矢印 A → B は処理順序（A の後に B）を表す。破線＋「問題」ラベルは現状の欠陥箇所を指す。
> 凡例のノードは色分けの意味のみを示し、相互関係は表さない。

新方式の要点:

- **親プロセスは root のまま**: 対象ユーザーへは切替えず、対象ユーザー identity は
  `execCmd.SysProcAttr.Credential{Uid, Gid, Groups, NoSetGroups: false}` に載せ、`execve` 時に
  カーネルが real/effective/saved の uid/gid と補助グループをアトミックに一括設定する。
  - `NoSetGroups: false` により補助グループが対象ユーザーのものへ明示的に再設定され、root の
    補助グループは継承されない（AC-01）。子プロセスの suid も対象 uid になるため、子側に
    saved-set-uid=0 は残らない（AC-05）。これは子の性質であり、親の性質ではない（後述の親不変条件と区別する）。
  - 親が「対象ユーザーへ seteuid される区間」は消滅する（AC-05 option (a)）。
- **run-as identity 解決の一元化**: 既存 `internal/runner/base/risk/runas_identity.go` の
  `resolveRunAsIdent(base, userName, groupName)` を唯一の解決関数として共有する。risk 側の
  パス信頼区分の判定と executor 側の Credential 生成が同一関数を呼ぶことで、両者が同じ集合を返すことをコードで保証する
  （並行実装を作らない。DRY）。そのため本タスクでこの関数を `risk` パッケージから、`risk` と
  `executor`/`privilege` の双方が依存できる中立な位置（`risktypes` 近傍を想定）へ移設する。
  `RunAsIdent` 型も既存の `risktypes.RunAsIdent`（`UID/GID uint32`, `Groups []uint32`）を再利用し、
  新しい identity 型は追加しない。
  - **3 形態すべてを定義**: `resolveRunAsIdent` の既存規則に基づき、`base` は「起動元プロセスの元 identity」
    （run_as を指定しないコマンドの既定であり、group-only 形態の土台）。
    - user 指定あり: そのユーザーの uid / 主 gid / 補助グループ（`base` は上書きされる）。
    - group 指定のみ: `base` の uid・補助グループを保ち gid のみ上書き。この形態では補助グループが
      `base`（＝起動元）のものになため、setuid-root 配備では root の補助グループが残る。したがって
      group-only 形態の `base` には「起動元プロセスの元 identity（`os.Getuid`/`Getgid`/`Getgroups`）」を用い、
      昇格中の root の現在の identity を読まない（`risk/runas_identity.go` が構築時に元 identity を捕捉するのと
      同じ規則）。これにより AC-01 の「起動元の補助グループを 1 つも引き継がない」を group-only でも満たす。
    - user・group 両指定: ユーザーの identity を土台に gid のみ group で上書き。
  - **補助グループ列挙のビルド依存**: cgo 有効時は `getgrouplist(3)`（nsswitch 準拠）、
    純 Go ビルド（`osusergo`）では `/etc/group` のみを参照する。取得不能時はエラーとして扱い、
    後述のフェイルクローズド規則（AC-02）に従って実行を中止する（root の補助グループを空へ倒すフェイル
    セーフは採らない）。cgo/純 Go 間の列挙結果の差異はビルド依存として許容するが、dry-run と実行時の
    一貫性のため §5.3 に制約として記す。
- **フェイルクローズド**: identity 解決（未知ユーザー/グループ、補助グループ列挙不能）または `Credential`
  設定に失敗した場合は、コマンドを実行せずエラーを返し非ゼロ終了する。root の補助グループを保持したまま
  実行を継続しない（AC-02）。
- **PrivilegeManager 側の役割変更**: `OperationUserGroupExecution` は「root 昇格のみ・親のユーザー/グループ
  切替なし」に変更する。親を対象ユーザーへ切替える `changeUserGroupInternal` 呼び出しは実行経路から外す。
  dry-run 経路（`OperationUserGroupDryRun`）は従来どおり検証・ログのみで identity 変更を行わない（AC-04）。
- **親プロセスの不変条件チェックの拡張**: 復元後の検証に saved-set-uid/gid（suid/sgid）を加える（AC-06）。
  - **重要**: 親の suid を `real UID` と比較してはならない。setuid-root 配備では親の suid は起動時から `0`
    であり、それは次操作の再昇格・ファイル検証のための正当な状態である（`real UID` は非 0 の起動ユーザー）。
    正しい不変条件は「操作開始時に捕捉した suid/sgid から変化していないこと」である。このため
    `executionContext` に操作開始時の suid/sgid を捕捉し（現状は `originalEUID`/`originalEGID` のみ捕捉）、
    復元後にそれと一致するかを検証する。setuid-root では期待値 `0`、native root では `0`。
  - **既存検証は維持**: EUID==UID / EGID==GID 検証は維持し（AC-07、`restorePrivileges` により euid は元へ戻る）、
    いずれかが操作開始時と異なれば既存の emergency shutdown（即時プロセス停止）を行う。
  - **suid/sgid 読み取りの実装**: `golang.org/x/sys/unix.Getresuid`/`Getresgid`（標準 `syscall` には無い、
    Linux/対応 Unix のみ）を用いる。`unix.go` は `//go:build !windows` で darwin も含むため、非 Linux では
    ガードした no-op（suid 検証を省略）とする（F-007 の「macOS は開発・限定用途」と整合）。

#### 3.1.3 fd-bound 実行・staging fallback との整合（要 PoC）

`DefaultExecutor` は検証済み inode を fd-bound（`/proc/self/fd/<n>` を argv[0] に与え、複製 fd を
`ExtraFiles` で渡す。Linux）または staging コピー（非 Linux）で実行する（検証〜実行間 TOCTOU 遮断）。
`Credential` は `execve` 時にカーネルが適用するが、**fd-bound 実行との相互作用は自明ではなく、実装前に
PoC（概念実証）で確認する**（要件 01 §4 / レポート 00 §4 の PoC 推奨に対応）。

- **既知のリスク（fd-bound + Credential setuid）**: 子プロセスが `execve` 前に `setgroups`/`setgid`/`setuid`
  で権限降格すると **dumpable フラグ**がクリアされ、カーネルは `/proc/<pid>` を `root:root` 所有へ戻し
  `/proc/<pid>/fd` の探索を制限し得る。この後で `/proc/self/fd/<n>` を文字列パスとして解決する
  `execve` は、降格済みの子から `EACCES` になる可能性がある（`fexecve` over `/proc` の既知問題と同型）。
  本番ターゲットは Linux + fd-bound + run_as（F-007）であり、ここが壊れると run_as 実行が本番で機能しない
  （AC-04 回帰・機能停止）。
- **PoC で不成立だった場合の代替**（優先順）: (1) `/proc` パス解決を避け、複製 fd に対し
  `execveat(fd, "", AT_EMPTY_PATH)` 相当で exec する、(2) `PR_SET_DUMPABLE` で dumpable を維持する、
  (3) staging fallback に倒す。採否は PoC 結果を本節にインラインで記録する。
- **PoC 結果**: Linux 6.12.76-linuxkit (arm64) で検証。root (euid=0) プロセスが非 root ユーザー
  （nobody:uid=65534/gid=65534、daemon:uid=1/gid=1）へ `SysProcAttr.Credential` で降格しながら
  `/proc/self/fd/<n>` を argv[0] に指定した `execve` が **exit code 0・EACCES なしで成功**する
  ことを確認した。`Credential` は Go ランタイムが子プロセス内で `execve` 直前に
  `setgroups`/`setgid`/`setuid` を発行して適用するため、本 PoC は上記「既知のリスク」が指す
  「子が権限降格したうえで `/proc/self/fd/<n>` を解決する」経路そのものを実行している。それでも
  本カーネルでは解決が成功し、run_as 経路は機能した。
  - **観測できたこと／できていないこと**: 本 PoC が確認したのは「降格後の `/proc/self/fd/<n>` 解決が
    成功する」という観測結果のみである。子プロセスの dumpable フラグ値そのものは取得していない
    （権限降格に伴い dumpable は 0 へクリアされるが、`prctl(PR_GET_DUMPABLE)` による直接測定は
    行っていない）。dumpable が 0 でも本カーネルで EACCES にならない理由（exec するタスク自身の
    `/proc/self` へのアクセス扱い等）は未確定である。※実装者は Phase 1 でこの前提が成立する
    カーネル範囲を `prctl(PR_GET_DUMPABLE)` 等で確認する。
  - **選定方式**: 現行の fd-bound 実行を維持する。PoC 不成立時の代替（(1)〜(3)）は不要。
  - **環境**: `uname -a` = `Linux arm64-dev-box 6.12.76-linuxkit #1 SMP aarch64 GNU/Linux`、Ubuntu 26.04 LTS。
  - **手順**: root で `go run` し、対象バイナリを `os.Open` → `syscall.Dup`（複製 fd を `ExtraFiles`
    で子へ渡す）→ その fd を指す `/proc/self/fd/<n>` を argv[0] として `exec.Command` に与え、
    `SysProcAttr.Credential`（`Uid`, `Gid`, `NoSetGroups: false`）を設定して `Run()`。対象バイナリは
    `/bin/true`。※本番経路と同じく複製 fd は `ExtraFiles` で受け渡す。
  - **結果**: exit code 0。複数ターゲットユーザー（nobody、daemon）で同様に成功。
- **staging fallback の所有権**（非 Linux／fd-exec 無効時）: 現行は親が対象ユーザーの EUID でコピーを作り、
  コピーは対象ユーザー所有・`0o500` だった。新方式では親が root のままコピーを作るため所有は root になる。
  - **コピーを対象ユーザーへ chown してはならない**。所有者になった run_as ユーザーは `0o500` を
    `chmod +w` で覆して検証済みバイトを差し替えられ、検証〜exec 間の置換（staging が防ぐはずの TOCTOU）を
    再導入する。run_as ユーザーは本ツールの脅威モデルでは攻撃者になり得る。
  - **対策**: コピーは root 所有のまま、他者に実行のみ許す `0o555`（ディレクトリは `0o711`）とし、
    書込みは誰にも与えない。run_as ユーザーは exec できるが `chmod` 権限を持たない。
  - staging は開発・限定用途の経路であり、本番は fd-bound 実行である（F-007）。

#### 3.1.4 検証可能性（AC-03）

切替後の子プロセスの実効・実 uid/gid・補助グループが対象ユーザーと一致することを検証可能にするため、
identity 解決と `Credential` 生成、および操作開始時 suid/sgid の読み取りを注入可能なインタフェースへ切り出す。
既存の `syscallSeteuid`/`syscallSetegid`（privilege 側）・`identityChecker`（executor 側、`defaultIdentityChecker`）
と同方式で、ユニットテストから期待値を検証できる形にする。統合テストは環境依存が強いため方針を §7 に定める。

### 3.2 権限管理インタフェースの拡張

型・シグネチャは実ソースに整合させる。新しい identity 型は追加せず、既存の `risktypes.RunAsIdent` と
`risk` の既存解決関数を再利用・移設する。以下は高レベル定義のみを示す。

```go
// 既存 risktypes.RunAsIdent を再利用する（新規型は作らない）。
//   type RunAsIdent struct { UID, GID uint32; Groups []uint32 }

// 既存 resolveRunAsIdent を共有関数として移設し、risk と executor/privilege が同一関数を呼ぶ。
// base は起動元プロセスの元 identity（group-only 形態の土台）。
// 解決失敗（未知ユーザー/グループ、補助グループ列挙不能）はエラーを返し、呼び出し側はフェイルクローズドする（AC-02）。
func resolveRunAsIdent(base risktypes.RunAsIdent, userName, groupName string) (risktypes.RunAsIdent, error)

// 親プロセスの復元後不変条件検証。EUID/EGID は操作開始時の値へ復元されたか、
// suid/sgid は操作開始時の値から変化していないかを検証する（AC-06/AC-07）。
// executor 側の既存注入フィールド名は identityChecker（defaultIdentityChecker）。
type identityChecker func() error
```

### 3.3 F-003: 値ベースの機微情報検出・マスク

#### 3.3.1 なぜキー名パターンだけでは不十分か

現状の `redaction` は主にキー名/変数名の正規表現（`password|token|secret|key|api_key` 等）に依存し、
値検出 `IsSensitiveValue` も同じキー向けパターンを流用している。よって、キー名を伴わずに値だけが
コマンド引数・stdout/stderr・環境変数値へ現れた場合（`AKIA…`、JWT、PEM ブロック等）を取りこぼす。
これらはログや Slack 通知へ平文で載り得る（要件 F-003 / AC-08）。

#### 3.3.2 追加する値ベース検出器

`internal/redaction` に値そのものを対象とする検出器 `ValueDetector` を追加し、既知フォーマットを
マスクする。少なくとも次を対象とする（AC-08）。

- クラウド資格情報プレフィックス: AWS（`AKIA`/`ASIA` 等）、GitHub（`ghp_`/`gho_`/`ghs_` 等）、
  Slack（`xox` 系）、GCP サービスアカウント。
- PEM / private key ブロック（`-----BEGIN … PRIVATE KEY-----` 等）。
- `Bearer <token>`。
- URL 埋め込み credential（`scheme://user:pass@host`）。

**実装時の注記（weakreview 指摘、2026-07-15 追記）**: 上記のうち GCP サービスアカウントのみ、
値そのものにプレフィックス等の識別可能なフォーマットを持たない（サービスアカウントの鍵IDは
単なる16進フィンガープリントであり、他の任意のハッシュ値と値だけでは区別できない）。そのため
実装（`internal/redaction/value_detector.go` の `gcpSAKey`）は JSON フィールド名
`"private_key_id"` に隣接する場合のみ検出する方式を採用しており、他の6パターンと異なり
厳密には「キー名に依存しない値ベース検出」ではない。実際の GCP 資格情報本体（`private_key` の
PEM ブロック）は本節の PEM パターンによりキー名非依存で検出されるため、実害は限定的である。
この制約は `docs/user/security-risk-assessment.md`/`.ja.md` の Limitations/限界節に明記する。

高エントロピー文字列ヒューリスティックは、誤検出リスクを踏まえ本タスクでは実装しない**
（要件スコープ外の任意事項）。将来の拡張点として §9 に記す。

#### 3.3.3 既存 Redaction 経路への接続（漏れの防止）

要件 AC-09 は「値ベース検出をコマンド引数・stdout/stderr・環境変数値へ適用し、少なくとも Slack へ載る
内容には必ず適用する」ことを求める。現状の二層防御（Layer 1: `SanitizeOutputForLogging`、
Layer 2: `RedactingHandler`）は共通して `redaction.Config.RedactText` を経由する。したがって
`ValueDetector` を `Config` に統合し `RedactText`（および `RedactLogAttribute`/`redactLogAttributeWithContext`
の値判定）から呼ぶことで、両層・全出力先（file / syslog / Slack）へ一括適用され、経路ごとの
実装漏れを避けられる（DRY）。

```mermaid
flowchart LR
    classDef process fill:#fff1e6,stroke:#ff7f0e,stroke-width:1px,color:#8a3e00;
    classDef enhanced fill:#e8f5e8,stroke:#2e8b57,stroke-width:2px,color:#006400;
    classDef newpkg fill:#ffe8f5,stroke:#d946ef,stroke-width:2px,color:#701a75;
    classDef data fill:#e6f7ff,stroke:#1f77b4,stroke-width:1px,color:#0b3d91;

    OUT[("コマンド出力/引数/環境変数値")] --> L1["Layer1<br>SanitizeOutputForLogging"]
    OUT --> L2["Layer2<br>RedactingHandler"]
    L1 --> RT["Config.RedactText"]
    L2 --> RT
    RT --> KV["キー名/キー=値パターン<br>(既存)"]
    RT --> VD["ValueDetector<br>値ベース検出 (新規)"]
    KV --> SINK[("file / syslog / Slack")]
    VD --> SINK

    class L1,L2,RT enhanced
    class VD newpkg
    class KV process
    class OUT,SINK data

    %% Legend
    subgraph Legend["凡例"]
        LA["変更/接続点"]:::enhanced
        LB["新規検出器"]:::newpkg
        LC["既存パターン"]:::process
        LD[("データ")]:::data
    end
```

> 矢印 A → B は「A の出力が B へ渡る」処理の流れを表す。凡例のノードは色分けの意味のみを示す。

`ValueDetector` は組み立て済みのフィールド値（コマンド結果を格納した `slog.Attr`。Slack formatter が
生成する `cmd_N` 等のネストグループを含む）に対して適用する。`RedactingHandler` は
`redactLogAttributeWithContext` で `KindGroup` を再帰処理するため、`RedactText` へ統合すれば
ネストグループ内の値も再帰的に対象になる。未組み立ての生ストリームチャンク（`outputWrapper.Write` が
受け取る途中バイト列）ではなく、組み立て後の値に対して適用することを実装上の前提とする（チャンク境界を
跨ぐ秘密値の取りこぼしを避けるため。§5.3 に残余限界を記す）。

#### 3.3.4 Slack ペイロード方針（AC-10）

Slack 送信は `internal/logging/slack_handler.go` が担い、`RedactingHandler` に包まれるため §3.3.3 の
値ベースマスキングが適用される（`SlackHandler.Handle` はマスク済み `slog.Attr` からペイロードを組み立てる）。
加えて AC-10 の要求（自由記述本文に未マスクの生コマンド出力を載せない）に対し、「送信前に値ベースマスキングを必ず通す」方式を採用する（`RedactingHandler` 経由の保証を維持しつつ、
コマンド出力は Layer 1 で生成時点でマスク済み）。フィールド・ホワイトリスト化は本タスクでは追加要件とせず、
既存の長さ制限（stdout 1000 / stderr 500 文字）とマスキングの二重化で担保する。

#### 3.3.5 `--show-sensitive` と既定（AC-11）

`--show-sensitive` を明示指定した場合のみ平文化する既存挙動を維持し、既定はマスクとする。
`ValueDetector` の追加は既定マスク側の網羅性を上げるものであり、`--show-sensitive` の意味は変えない。

### 3.4 F-004: dry-run の未検証内容の明示区別と hard fail

#### 3.4.1 現状の危うさと 2 つのフォールバック経路

`Manager.readAndVerifyFileWithReadFallback` は、検証を通さずに `os.ReadFile` で内容を返す経路を 2 つ持つ。
本設計は両方を対象にする（片方だけを塞ぐと CI の主用途を取りこぼす）。

1. **検証器 nil 経路**（`m.fileValidator == nil`）: 検証をスキップして即 `os.ReadFile`。dry-run では、
   ハッシュディレクトリが書込み不可（`os.ErrPermission`、例: sudo なしの CI）だと `fileValidator` が
   **実行全体で nil** になる（`newManagerInternal`）。この場合、設定/テンプレートの読み込みはすべて
   この経路を通り、現状は `ResultCollector` に一切記録されない。F-004 が想定する CI の主用途はここである。
2. **検証失敗経路**（dry-run で `VerifyAndRead` 失敗）: 失敗を `ResultCollector` に記録したうえで
   `os.ReadFile` で読み直す。ハッシュ不一致（`ReasonHashMismatch`）等の改ざん兆候を含む。

対象範囲の正確化（AC-13 の「全経路」の実体）: 上記は `VerifyAndReadConfigFile`（設定）と
`VerifyAndReadTemplateFile`（テンプレート）が通る。env ファイルは別経路であり、
`VerifyEnvironmentFile` は内容を読まず dry-run では nil を返し、env 内容の読み込みは別の場所
（`config` ローダの `SafeReadFile` 等）で行われる。したがって「設定・テンプレート」は本節の
`readAndVerifyFileWithReadFallback` で、env 経路は当該読み込み箇所での UNVERIFIED 記録として扱う。
実装時に env 読み込み箇所が同種のフォールバックを持つかを確認し、持つ場合は同じ `UsedUnverifiedContent`
記録を適用する。

#### 3.4.2 UNVERIFIED の明示（AC-13）

未検証内容を採用した事実を検証サマリに保持する。`ResultCollector` に「未検証内容を採用したか」を表す
フラグ `UsedUnverifiedContent` と、その理由区分を追加する。

- 理由区分: `skipped_no_validator`（経路 1）／`verify_failed_<reason>`（経路 2。`reason` は既存の
  `ReasonHashMismatch`/`ReasonHashFileNotFound` 等）。
- 経路 1 は現状 `ResultCollector` に記録されないため、`readAndVerifyFileWithReadFallback` の nil 経路にも
  記録呼び出しを追加する。
- dry-run 出力（text / json 両形式）で該当ファイル内容を UNVERIFIED として検証済みと区別表示し、
  理由区分（検証スキップ／検証失敗）も併記する。

#### 3.4.3 hard fail の提供・終了コード・既定（AC-14）

既存の `--dry-run-fail-unverified`（`FailOnVerificationUnavailable`）は、現状この環境で検証できなかった
コマンド identity（例: 開発機で未記録）」の deny を、専用の終了コード
`DryRunExitVerificationUnavailable`（= 3）で hard fail にするフラグである。F-004 の未検証ファイル内容は
概念が異なり、とくにハッシュ不一致は改ざんの兆候（policy deny 相当）であって「環境で検証不能」ではない。

本設計は、フラグ増殖を避けて `--dry-run-fail-unverified` を「dry-run で未検証成果物を採用したら hard fail」の
単一スイッチとして共有する一方、終了コードは概念別に分ける（改ざん兆候を「環境で検証不能」に埋没させない）。

| 未検証の事象 | 意味 | フラグ指定時の終了コード |
|---|---|---|
| コマンド identity が環境で検証不能（既存） | 環境起因 | `DryRunExitVerificationUnavailable`（3、既存） |
| 設定/テンプレートを検証スキップで採用（経路 1） | 環境起因（検証器なし） | `DryRunExitVerificationUnavailable`（3） |
| 設定/テンプレートの検証失敗（ハッシュ不一致・not-found、経路 2） | 改ざん兆候（policy deny 相当） | policy-deny 終了コード（既存の deny コード。3 とは別） |

- **既定（フラグなし）**: dry-run は継続。未検証内容は UNVERIFIED として明示するが終了コードは 0
  （正常系 dry-run を回帰させない。AC-15）。
  - ただしハッシュ不一致は `security_risk: high` として出力上で強調表示し、既定でも見落とされないようにする
    （既定 exit 0 は「dry-run はプレビューであり実行しない」という前提に基づく。実行経路は非 dry-run で
    従来どおりフェイルクローズドする）。
- **`--dry-run-fail-unverified` 指定時**: 未検証内容が 1 件でもあれば非ゼロ終了。終了コードは上表に従い、
  改ざん兆候（検証失敗）と環境起因（検証不能・スキップ）を区別する。

> **既存ポリシーの例外と影響**:
> `FailOnVerificationUnavailable` は Task 0136（`docs/tasks/0136_runtime_risk_evaluation_enforcement/02_architecture.md`
> §5.3 / AC-58）で「検証不能 deny（環境起因）」を通常 deny と区別する運用フラグとして導入された。
> 本設計はフラグの適用対象を「未検証成果物全般」へ拡張する意図的な変更である。
> - 元ポリシーの所在: Task 0136 の上記アーキテクチャ文書、および実装
>   `internal/runner/resource/types.go`（`FailOnVerificationUnavailable`・`DryRunExitVerificationUnavailable` の
>   doc コメント）、`internal/runner/resource/dryrun_manager.go` の該当分岐。
> - 例外理由: ハッシュ不一致による設定ファイル未検証は改ざんの兆候であり、CI で hard fail できることが
>   要件（AC-14）。別フラグ新設は運用者に 2 つの近接概念を強いるため避ける。ただし終了コードは概念別に
>   分離し、改ざん兆候を環境起因コードに埋没させない。
> - 旧挙動を前提とするテスト: `internal/runner/resource/dryrun_manager_test.go` の
>   `FailOnVerificationUnavailable` 関連ケース、および `cmd/runner` の該当フラグ挙動テスト。
>   これらは対象拡張・終了コード分離に合わせて期待値を更新する。

### 3.5 F-005: record のディレクトリ権限違反をフェイルクローズドに

`cmd/record/main.go` は `security.RunTOCTOUPermissionCheck` を実行するが、戻り値（`[]TOCTOUViolation`）を
破棄し警告ログのみで続行している。信頼の起点であるハッシュDB 生成を止めないため、書込み可能な祖先や
world/group-writable なハッシュディレクトリを放置したまま生成が進む（要件 F-005）。

```mermaid
flowchart TD
    classDef enhanced fill:#e8f5e8,stroke:#2e8b57,stroke-width:2px,color:#006400;

    Start(["record 実行"]) --> Chk["RunTOCTOUPermissionCheck<br>(戻り値 violations を評価)"]
    Chk --> Has{"違反あり?"}
    Has -->|"No"| Gen["ハッシュDB 生成"]
    Has -->|"Yes"| Fail["ERROR ログ (違反パス/理由/是正方法)<br>→ 非ゼロ終了・ハッシュ生成せず"]
    Gen --> End(["完了"])
    Fail --> EndF(["異常終了"])

    class Chk,Has,Fail enhanced

    subgraph Legend["凡例"]
        LE["変更/追加処理"]:::enhanced
        LD["既存/標準の処理"]
    end
```

> 実線矢印 A → B は処理順序・分岐を表す。菱形は判定を表す。凡例のノードは色分けの意味のみを示す。

変更点:

- `RunTOCTOUPermissionCheck` の戻り値（`[]TOCTOUViolation`）を評価し、違反が 1 件以上あれば
  違反ディレクトリと具体的な権限問題（是正方法を含む）を ERROR ログに出力のうえ、ハッシュレコードを
  生成・保存せず無条件で非ゼロ終了する（AC-16）。
  - **権限違反を継続するバイパス手段は設けない**。ハッシュDB は本システムの信頼の起点であり、TOCTOU
    チェックが弾く違反（非 sticky の world-writable 祖先、非信頼グループが書ける祖先）は、まさに攻撃者が
    ハッシュレコードを差し替え得る状況そのものである。継続用のエスケープハッチはこの脆弱性を警告付きで
    再導入するだけで、`record` を「信頼できる管理者権限・クリーンな環境で実行する」前提（AC-19）とも
    矛盾する。想定される違反はいずれも ERROR が指すディレクトリの `chmod` で是正可能であり、CI を含め
    継続を要する具体的用途も存在しないため、バイパスフラグ（`--allow-insecure-perms` 等）は導入しない（AC-18）。
  - **既存 `--force` に権限違反バイパスを兼ねさせない**。`--force` は「既存ハッシュファイルの上書き」という
    日常的で無害な指定であり、ここに権限違反バイパスを載せると、再記録のため常用する運用者が信頼の起点の
    fail-closed を気付かず無効化してしまう。`--force` の意味は上書き専用のまま維持する（AC-18）。
- ハッシュディレクトリ作成権限 `hashDirPermissions` を `0o750` → `0o700` に変更し、
  world/group-writable を許さない（AC-17）。
- **処理順序（実装上の注意）**: 現状 `mkdirAll` はディレクトリ作成を `parseArgs` 内で行い、TOCTOU チェックは
  その後に走る。ディレクトリ生成が違反判定より前になるため、**fail-closed の early return は
  `processFiles`（ハッシュ生成）より前**で行い、書込み可能な祖先の下に作られたディレクトリでレコードを
  生成しないようにする。TOCTOU チェックは祖先も対象にする（`CollectTOCTOUCheckDirs`）ため、作成後でも
  既存の world/group-writable な祖先を検出できる。
- **アップグレード時の挙動（移行）**: `os.MkdirAll` 既存ディレクトリの mode を変更しないため、`0o700` は
  新規作成時のみ適用される。既存 `0o750` 配備のディレクトリは自動的には是正されない（`0o750` は
  group-writable ではないため書込み中心の TOCTOU チェックでは違反にならない）。既存配備の是正は運用手順
  （手動 `chmod 0700`）としてドキュメント化し、自動 chmod は本タスクのスコープ外とする。
  - `0o700` により group 読み取りも失われる。record と runner が同一の管理者権限で運用される前提（AC-19）を
    満たす配備では問題にならないが、ハッシュDB を非 root で読む監視・付随ツールがある配備ではこの前提を
    ドキュメントで明示する。既存ハッシュDB とのフォーマット互換は保持され（NF-001）、
    変わるのはディレクトリのパーミッションのみ。

### 3.6 F-006 / F-007: 前提・限界のドキュメント化

いずれもドキュメント専用の対応であり、`static` 検証（記述の存在確認）で足りる。

- **F-006（AC-20 / AC-21）**: `SafeReadFile` / 各アナライザの 128MB 上限（`safefileio.MaxFileSize`）と
  その根拠（メモリ枯渇対策）、および大容量バイナリを検証・解析できない可用性上の制約をユーザー向け
  ドキュメントに明記する。
  - **設定可能化／上限分離の可否検討（AC-21、要件が本書に委ねた論点）**: ハッシュ計算は既に
    ストリーミング化されているが、解析（`filevalidator` の `maxFileSize = 1GB`、`safefileio` の 128MB）は
    メモリ上限に依存する。本タスクでは実装を行わないと結論する。理由は (1) 要件スコープが「前提の明文化と
    可否検討まで」であること、(2) 現時点で具体的な大容量検証の要求がなく YAGNI に反すること、(3) 上限分離は
    アナライザ全体のメモリモデル見直しを要し、堅牢化タスクの範囲を超えること。将来必要になった場合の
    拡張余地として §9 に記録する。
- **F-007（AC-22）**: 本番ターゲットは Linux + openat2 前提であること、non-Linux（openat2 非対応）環境には
  `safeOpenFileFallback` の二段階チェックでも原理的に残る極短の TOCTOU 競合ウィンドウが存在すること、
  macOS 等は開発・限定用途に限る運用ガイドをドキュメントに明記する。

---

## 4. エラーハンドリング設計

### 4.1 エラー方針

- **権限系（F-001/F-002）**: identity 解決失敗・`Credential` 設定失敗・復元後不変条件違反は、いずれも
  フェイルクローズド。解決/設定失敗はコマンド未実行のエラー返却（非ゼロ終了）、復元後の identity 逸脱は
  既存 emergency shutdown（即時 `os.Exit(1)`、ERROR ログ付き）。既存の `Error`/`ErrIdentityLeak` を踏襲し、
  suid/sgid 検証違反は `ErrIdentityLeak` でラップして識別可能にする。
- **record（F-005）**: 権限違反検出時は、違反パスと権限問題を ERROR ログに出したうえで、生成前段の
  early return（`processFiles` の失敗集計とは別）で非ゼロ終了する。
- **dry-run（F-004）**: フォールバック読み込みは従来どおりエラーを致命化しないが、`--dry-run-fail-unverified`
  指定時はプレビュー終了コードを §3.4.3 の対応表に従って非ゼロにする（改ざん兆候と環境起因を別コードに分離）。

### 4.2 エラー型（高レベル）

```go
// run-as identity 解決失敗を表す（フェイルクローズド）。
var ErrRunAsIdentityResolution = errors.New("failed to resolve run-as identity (uid/gid/supplementary groups)")

// 親プロセスの復元後、euid/egid が操作開始時の値へ戻っていない、または
// suid/sgid が操作開始時の値から変化したことを表す（既存 ErrIdentityLeak を拡張利用）。
// suid は「操作開始時に捕捉した値」と比較する（real UID とは比較しない。§3.1.2 参照）。

// record: ディレクトリ権限違反により信頼の起点生成を中止したことを表す。
var ErrHashDirPermissionViolation = errors.New("hash directory permission violation detected; refusing to record")
```

---

## 5. セキュリティ考慮事項

### 5.1 セキュリティ設計原則の適用

| 原則 | 本タスクでの具体化 |
|---|---|
| フェイルクローズド | 補助グループ解決・Credential 設定・record 権限違反で安全側（実行/生成中止）に倒す |
| 最小権限 | 子プロセスは対象ユーザーの uid/gid/補助グループのみを持ち、起動元 root の補助グループを継承しない |
| 多層防御 | 値ベース Redaction を Layer1/Layer2 共通の `RedactText` に接続し全出力先へ適用 |
| 信頼の起点保護 | record 側でも権限違反を検知したら生成を止め、`0o700` で world/group-writable を排除 |

### 5.2 脅威モデル

```mermaid
flowchart TD
    classDef problem fill:#ffe6e6,stroke:#d62728,stroke-width:2px,color:#7b0000;
    classDef enhanced fill:#e8f5e8,stroke:#2e8b57,stroke-width:2px,color:#006400;
    classDef data fill:#e6f7ff,stroke:#1f77b4,stroke-width:1px,color:#0b3d91;

    T1["脅威: 補助グループ継承で<br>run_as ユーザーが root 相当権限"]:::problem
    T2["脅威: キー名なし秘密値が<br>Slack/ログへ平文流出"]:::problem
    T3["脅威: 未検証設定を dry-run が<br>検証済みと誤認"]:::problem
    T4["脅威: 権限脆弱なハッシュDB を<br>差し替え、改ざんを検証済みに偽装"]:::problem

    C1["対策: SysProcAttr.Credential で<br>補助グループを対象ユーザーへ再設定"]:::enhanced
    C2["対策: ValueDetector を<br>Layer1/Layer2 共通経路へ接続"]:::enhanced
    C3["対策: UNVERIFIED 明示 +<br>--dry-run-fail-unverified で hard fail"]:::enhanced
    C4["対策: record で権限違反を fail-closed +<br>ハッシュディレクトリ 0o700"]:::enhanced

    T1 --> C1
    T2 --> C2
    T3 --> C3
    T4 --> C4

    HDB[("ハッシュDB<br>信頼の起点")]:::data
    C4 --> HDB

    %% Legend
    subgraph Legend["凡例"]
        LP["脅威"]:::problem
        LE["対策"]:::enhanced
        LD[("データ")]:::data
    end
```

> 矢印 脅威 → 対策 は「その脅威を当該対策で緩和する」対応関係を表す。凡例は色分けの意味のみを示す。

### 5.3 残余リスク

- non-Linux 環境の TOCTOU 残余ウィンドウ（F-007）はドキュメント化で明確化するが、原理的な解消は本タスク範囲外。
- 値ベース Redaction は既知フォーマットに限る。未知形式・高エントロピー文字列は取りこぼし得る（F-003 のスコープ）。
  加えて、秘密値が複数のログフィールドや（ストリーム処理時の）チャンク境界を跨いで分割されると、値パターンに
  一致せず取りこぼし得る。§3.3.3 のとおり組み立て済みフィールドに適用してチャンク境界の分割は緩和するが、
  フィールド跨ぎの分割は残余リスクとして受容する。
- GCP サービスアカウントの値ベース検出（§3.3.2）は JSON フィールド名 `"private_key_id"` への隣接に
  依存しており、他のプレフィックス方式パターンと異なり厳密には「キー名に依存しない値ベース検出」では
  ない（weakreview 指摘、2026-07-15 追記）。実際の秘密（`private_key` PEM ブロック）は PEM パターンで
  キー名非依存にカバーされるため実害は限定的だが、既知の制約として受容する。
- 値ベース正規表現は file / syslog 出力先では全出力に対して適用されるため、巨大・高エントロピーな出力では
  CPU コストが増える。パターン数・入力長を過大にしない前提で運用する（Slack は既存の長さ制限で緩和済み）。
- group-only 形態の run-as 補助グループは `base`（起動元プロセスの元 identity）に依存する。純 Go ビルド
  （`osusergo`）では `getgrouplist(3)`/nsswitch を経由しないため、LDAP/SSSD 等で管理される補助グループは
  dry-run のリスク評価と実行時とで差が出得る（環境・ビルド依存の非決定性）。この制約はドキュメントに記す。
- group-only 形態で `base` が起動元の元 identity を正しく捕捉できない場合、起動元の補助グループが残る恐れが
  あるため、捕捉は昇格前（`os.Getgroups` が元 identity を返す時点）に行うことを実装上の不変条件とする。

---

## 6. 処理フロー詳細

### 6.1 run-as コマンド実行（新方式）シーケンス

```mermaid
sequenceDiagram
    participant EX as DefaultExecutor
    participant ID as resolveRunAsIdent
    participant PM as UnixPrivilegeManager
    participant K as Kernel(execve)

    EX->>ID: base(起動元の元 identity) + run_as_user/group を RunAsIdent へ解決
    alt 解決失敗
        ID-->>EX: error
        EX-->>EX: 実行中止・非ゼロ終了 (フェイルクローズド)
    else 解決成功
        ID-->>EX: RunAsIdent{UID,GID,Groups}
        EX->>PM: WithPrivileges(UserGroupExecution) 開始
        PM->>PM: 操作開始時の suid/sgid を捕捉。root へ昇格 (Seteuid 0)。親のユーザー/グループは変えない
        EX->>K: execCmd.Run() with SysProcAttr.Credential{Uid,Gid,Groups,NoSetGroups:false}
        K-->>EX: 子プロセスを対象 identity で実行 (子の real/eff/saved + 補助グループを一括設定)
        PM->>PM: 親を操作開始時の状態へ復元 (euid/egid を元へ)
        PM->>PM: 不変条件検証 (euid==元euid, egid==元egid, suid==開始時suid, sgid==開始時sgid)
        alt 不変条件違反
            PM-->>PM: emergency shutdown (ERROR ログ + os.Exit 1)
        end
    end
```

> 実線矢印 `->>` は同期メッセージ送信、破線矢印 `-->>` は応答メッセージを表す。`suid==開始時suid` は
> 「操作開始時に捕捉した親の saved-set-uid と一致すること」を意味する（real UID との比較ではない）。
> setuid-root 配備では開始時 suid は `0` であり、それが期待値となる（§3.1.2 参照）。

### 6.2 dry-run 未検証内容の扱い

```mermaid
flowchart TD
    classDef enhanced fill:#e8f5e8,stroke:#2e8b57,stroke-width:2px,color:#006400;

    V["設定/テンプレートを読み込み"] --> Path{"検証状態?"}
    Path -->|"検証成功"| Use["検証済みとして処理"]
    Path -->|"検証器なし (経路1)"| FB1["os.ReadFile<br>UsedUnverifiedContent=skipped_no_validator"]
    Path -->|"検証失敗 (経路2, dry-run)"| FB2["os.ReadFile<br>UsedUnverifiedContent=verify_failed_reason"]
    FB1 --> Mark["dry-run 出力に UNVERIFIED 表示 (理由併記)"]
    FB2 --> Mark
    Mark --> Flag{"--dry-run-fail-unverified?"}
    Flag -->|"No"| Zero["継続・終了コード 0<br>(改ざん兆候は high として強調)"]
    Flag -->|"Yes (環境起因)"| E3["終了コード 3<br>VerificationUnavailable"]
    Flag -->|"Yes (改ざん兆候)"| ED["policy-deny 終了コード"]

    class FB1,FB2,Mark,Flag enhanced

    subgraph Legend["凡例"]
        LE["変更/追加処理"]:::enhanced
        LD["既存/標準の処理"]
    end
```

> 実線矢印は処理順序・分岐を表す。菱形は判定を表す。凡例のノードは色分けの意味のみを示す。
> env ファイルは別経路（§3.4.1）であり、本図の対象は設定・テンプレートである。

---

## 7. テスト戦略

要件 NF-003 に従い、各 AC に少なくとも 1 つの `test` または `static` 検証を対応付ける
（対応は `03_implementation_plan.md` のトレーサビリティ節で確定）。

### 7.1 単体テスト

- **権限系（F-001/F-002）**: 環境依存を避けるため、identity 解決・`Credential` 生成・操作開始時 suid/sgid
  読み取り・復元後検証を注入可能インタフェース（既存 `syscallSeteuid`/`syscallSetegid` および executor 側の
  `identityChecker` と同方式）で分離する。検証項目:
  - user-only / group-only / both の 3 形態それぞれで期待する `{UID,GID,Groups}` が生成されること。とくに
    group-only で `base`（起動元の元 identity）から補助グループが継承されず、起動元固有グループを含まないこと（AC-01）。
  - **setuid-root 想定（`originalUID != 0`、開始時 suid=0）で、復元後に suid=0 のまま不変条件検証が
    成功すること**（`suid==uid` としないことの回帰防止）。逆に suid が開始時と変化したケースで
    emergency shutdown が呼ばれること。
  - 解決失敗・`Credential` 設定失敗時のフェイルクローズド（コマンド未実行・非ゼロ）。
- **F-003**: `ValueDetector` に既知フォーマット（AWS/GitHub/Slack/GCP/PEM/Bearer/URL 埋め込み credential）の
  正例・負例を与え、`RedactText` 経由で両層がマスクすること、ネストグループ内の値も再帰的にマスクされることを
  検証する。`--show-sensitive` 非指定時の既定マスクも検証。
- **F-004**: 2 経路（検証器 nil／検証失敗）それぞれで `UsedUnverifiedContent` と理由区分が立ち、UNVERIFIED
  表示されること、`--dry-run-fail-unverified` 指定時に事象別の終了コード（環境起因=3／改ざん兆候=policy-deny）が
  返ること、既定で 0 を返すこと。正常系 dry-run 回帰なしを検証（AC-15）。
- **F-005**: `mkdirAll` と TOCTOU チェッカを注入し、違反ありで ERROR ログ＋非ゼロ終了かつ未生成（バイパス
  手段なし）、違反なしで生成、`--force`（上書き）が権限バイパスに作用しないこと、新規作成時の権限 `0o700` を検証。

### 7.2 統合テスト

- run-as の実 uid/gid/補助グループ検証は root/setuid 環境が必要なため、CI では skip 可能なタグ付き
  統合テストとし、対象ユーザーで `id -G` 相当を実行して補助グループが対象ユーザーのものと一致し、
  root の補助グループ（例: `docker`）を含まないことを確認する方針とする（環境がある場合のみ実行）。

### 7.3 ドキュメント検証

- AC-12（一部）・AC-19・AC-20・AC-22 は該当ドキュメント記述の存在確認（`static`）で足りる。

---

## 8. 実装の優先順位

要件 §4（推奨対応順序）に従う。

- **Phase 0 — PoC（Phase 1 の前提）**: Linux + setuid-root + run_as + fd-bound 実行に `SysProcAttr.Credential`
  を組み合わせた際の `/proc/self/fd` exec 成否を確認（§3.1.3）。不成立なら代替（`execveat`/`PR_SET_DUMPABLE`/
  staging）を選定し結果を §3.1.3 にインライン記録する。
- **Phase 1 — F-001 / F-002（最優先）**: `SysProcAttr.Credential` 移行。executor と privilege の役割変更、
  run-as identity 解決の共有関数化（`risk` から移設し双方が呼ぶ）、操作開始時 suid/sgid 捕捉と検証。
  staging fallback は root 所有のまま `0o555`（chown しない）。
- **Phase 2 — F-003**: `ValueDetector` 追加と `RedactText` 統合、Slack 経路の適用確認。
- **Phase 3 — F-005 / F-004**: record の fail-closed 化（バイパス手段なし）・`0o700`、
  dry-run UNVERIFIED 明示（2 経路）と hard fail・終了コード分離。
- **Phase 4 — F-006 / F-007**: ドキュメント整備（前提・限界の明文化、可否検討結果の記録）。

---

## 9. 将来の拡張性

- **高エントロピー秘密値検出**: 長さ + base64/hex 比率 + Shannon エントロピー閾値のヒューリスティックを、
  誤検出を許容できる外部送信経路（Slack）限定で将来追加できる余地を残す（本タスクでは実装しない）。
- **ファイルサイズ上限の分離/設定可能化**: ハッシュ計算（ストリーミング）と静的解析（メモリ上限）で
  上限を分離する設計余地を §3.6 の結論とともに記録する。具体要求が生じた時点で別タスク化する。
- **`internal/groupmembership` との関係**: グループ所属確認は既存 `internal/groupmembership` が担う。本タスクの
  run-as identity 解決は所属確認ではなく「対象ユーザーの初期補助グループ集合の列挙」であり、既存
  `risk/runas_identity.go` の解決規則と一致させるのが最も安全なため、そちらを共有関数化して再利用する
  （新規の並行実装を作らない）。将来、両者の getgrouplist 参照を単一の getgrouplist ベース経路へ寄せる余地がある。

---

## 付録: 決定履歴（要点）

- 補助グループ／saved-set-uid の実現機構として、親プロセス seteuid + `setgroups` の追加ではなく
  `os/exec` `SysProcAttr.Credential` によるカーネル側アトミック設定を採用（§3.1）。理由は F-001 と F-002 を
  同時に解決し、親を対象ユーザーへ切替える複雑な復元ロジックを排除できるため。fd-bound 実行との相互作用は
  実装前に PoC で確認する（§3.1.3、Phase 0）。
- run-as identity 解決は新規型・並行実装を作らず、既存 `risktypes.RunAsIdent` と `risk` の既存解決関数を
  共有関数として移設し、`risk` と `executor`/`privilege` の双方から呼ぶ（§3.1.2、DRY）。
- 親プロセスの saved-set-uid 不変条件は「操作開始時に捕捉した suid との一致」で定義する（`real UID` とは
  比較しない）。setuid-root では期待値 `0`。`suid==uid` は setuid-root で誤検出・誤停止となるため採らない（§3.1.2）。
- staging fallback のコピーは root 所有・`0o555` を維持し、対象ユーザーへ chown しない（chown は書込み権を
  与え TOCTOU を再導入するため。§3.1.3）。
- record の権限違反は無条件 fail-closed とし、継続用のバイパスフラグは設けない（想定違反は `chmod` で是正可能、
  CI を含め継続を要する具体的用途がなく、エスケープハッチは信頼の起点の脆弱性を再導入するため）。既存 `--force`
  はハッシュ上書き専用のまま維持し、権限違反バイパスを兼ねさせない（§3.5）。
- F-006 の上限分離／設定可能化は「本タスクでは実装せず、根拠のドキュメント化と可否結論の記録にとどめる」と決定（§3.6）。
- F-004 の hard fail は新フラグを設けず既存 `--dry-run-fail-unverified` の対象を「未検証成果物全般」へ拡張する一方、
  終了コードは「改ざん兆候（検証失敗）」と「環境起因（検証不能・スキップ）」で分離する（§3.4.3）。
