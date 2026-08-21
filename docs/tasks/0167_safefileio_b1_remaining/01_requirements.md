# 要件定義書: safefileio の残所見（資源リーク・失敗時契約・書き込みのアトミック化）

## Document Status

| Item | Value |
|---|---|
| Status | `approved` |
| Created | 2026-08-20 |
| Review date | 2026-08-20 |
| Reviewer | isseis |
| Comments | 2026-08-21: Success Criteria の挙動の変化を 6 つから 7 つへ改めた。Phase 3 でディレクトリ fd 起点へ変えるのに伴い、非 Linux の移動に同一性確認が加わって `ErrSourceIdentityMismatch` が新たに返りうるため（`03_implementation_plan.md` § 2 Phase 3）。2026-08-20: `02_architecture.md` § 10 の指摘を受け、AC-07a・AC-18・AC-19・AC-21・F-7 の性能見積もり・Success Criteria を実装可能な記述へ修正（挙動の方針は変更なし）。同日、スコープ対象 6 にディレクトリ fd 起点への変更を追加（`02_architecture.md` § 10.1） |

## 関連 Issue

- [#978 [Security][B1 F-2〜F-9] safefileio: AtomicMoveFile以外のTOCTOU・fdリーク・ロールバック欠如](https://github.com/isseis/go-safe-cmd-runner/issues/978)
- 詳細所見: [docs/tasks/0149_security_code_smell_audit_fable/findings/B1_safefileio.md](../0149_security_code_smell_audit_fable/findings/B1_safefileio.md) の F-2〜F-9
- 残件一覧: [docs/tasks/0149_security_code_smell_audit_fable/98_remaining_issues.md](../0149_security_code_smell_audit_fable/98_remaining_issues.md) §2 B1
- 先行タスク: [0155 verify〜use 間の残存 TOCTOU ギャップ](../0155_toctou_verify_use_residual_gaps/01_requirements.md)（B1 F-1 を対応し、本タスクの前提を作った）
- 関連 ADR: [resolved-path-symlink-enforcement-adr.ja.md](../../dev/architecture_design/resolved-path-symlink-enforcement-adr.ja.md)（リーフ symlink を検知して拒否するという設計前提）

## 背景

0149 監査の所見 B1（`internal/safefileio`）は 9 件からなる。0155 はこのうち F-1（`AtomicMoveFile` が fd で検証したソースをパス名で `rename` する TOCTOU）のみを対象とし、`moveFileAnchored` による fd アンカー方式を導入して解消した。残る F-2〜F-9 は、Task 0150〜0166 のいずれの対象にもならなかった。

2026-08-20 時点で現行コードと照合したところ、**8 件すべてが現存する**。以下に各件の現状と、所見の推奨をそのまま採れない箇所を示す。

### F-2: フォールバック経路の TOCTOU は、利用者向け文書では既に明示されている

`safeOpenFileFallback`（[safe_file.go:475-499](../../../internal/safefileio/safe_file.go#L475-L499)）の「親ディレクトリ確認 → `O_NOFOLLOW` で open → 再確認」という二段階方式は現行のままで、競合の隙は残っている。

所見は「非 Linux でも `openat` でルートからコンポーネント単位に降りる方式（dirfd ウォーク）に置き換えれば原理的に排除できる」と述べるが、本タスクはこれを実装しない。理由は 2 つある。

1. この経路が使われるのは openat2 が無い環境、すなわち macOS 等の非 Linux と Linux 5.5 以下に限られる（このほかに `FileSystemConfig.DisableOpenat2` による明示的な無効化があるが、これは両経路を試験するためのものであり本番の構成ではない）。[security-risk-assessment.ja.md](../../user/security-risk-assessment.ja.md) は本番ターゲットを Linux 5.6+ と定め、それ以外は開発・限定用途に限る運用を推奨すると既に明記している。開発環境でしか効かない対策に、パス解決の再実装という規模の変更を投じるのは CLAUDE.md の YAGNI 原則に反する。
2. dirfd ウォークは `ensureParentDirsNoSymlinks` が持つ OS 管理 symlink の allowlist（macOS の `/tmp` → `/private/tmp` 等）を別方式で作り直すことになり、現在 1 か所に集約されている判断が 2 か所に分かれる。

一方で、**コード側の公開 API には限界が書かれていない**。`security-risk-assessment.ja.md` を読まずに `SafeOpenFile`・`SafeReadFile`・`SafeWriteFileOverwrite` の doc コメントだけを見た利用者は、どの経路でも同じ保証が得られると受け取る。package コメント（[safe_file.go:1-6](../../../internal/safefileio/safe_file.go#L1-L6)）も、フォールバックの存在は書くが保証の差は書いていない。本タスクはこの差を埋める。

### F-3: fd リークに加えて、作成済みファイルの残置もある

`safeOpenFileFallback` は `os.OpenFile` 成功後に 2 回目の `ensureParentDirsNoSymlinks` を呼び、これが失敗すると `file` を Close せずに `nil, err` を返す（[safe_file.go:494-497](../../../internal/safefileio/safe_file.go#L494-L497)）。fd がそのまま漏れる。

所見は「必要に応じて作成したファイルの削除も」と述べており、これは `O_CREATE` 付きで呼ばれた場合に該当する。ただし削除には注意が要る。2 回目のチェックが失敗したということは、その時点でパスが指す先が信頼できないということであり、そこへパス名で `os.Remove` を実行すると、攻撃者が差し替えた先の無関係なファイルを消しうる。**削除するとしても、開いた fd の inode と削除対象が同一であることを確かめてからでなければならない。** 具体的な手段の選択は `02_architecture.md` で決める。

### F-4: 2 つある指摘のうち、後者は 0155 の決定と衝突する

`atomicMoveFileCore` の指摘は 2 点ある。

1. **chmod が検証より先**。`srcFile.Chmod(requiredPerm)`（[safe_file.go:162-164](../../../internal/safefileio/safe_file.go#L162-L164)）は `canSafelyAccessFile` による検証（同 :167-169）より前にある。検証が失敗すると、ソースの権限だけが変更されて残る。`requiredPerm` が元より広い場合、操作は失敗したのに緩い権限が残る。fchmod は開いた fd に対する操作なので、順序を変えても対象がずれることはない。

   ただし順序の入れ替えは**単なる並べ替えではなく、挙動を変える**。検証は `CanCurrentUserSafelyReadFile(gid, mode)` を通る。この `mode` を chmod の前後どちらで読むかによって結果が変わるためである。現在の順序では、ソースの元の権限が読み取り検査に通らないものであっても、先に `requiredPerm` へ狭められてから検査を受けるので通過する。順序を入れ替えると、元の権限のまま検査され、読み取り検査が拒否するもの（other 書き込み可、実行者が属さないグループからの書き込み可、`MaxAllowedReadPerms` 超過）は拒否される。つまりこの変更は、**安全でない権限のソースを「直してから受け入れる」のをやめ、拒否するようにする**。CLAUDE.md の「正規化するな、拒否せよ」に沿った方向であり、意図した変更として扱う。

   本番への影響は無い。`AtomicMoveFile` の本番呼び出し元は [output/file.go:73](../../../internal/runner/base/output/file.go#L73) の `MoveToFinal` 1 か所で、そのソースは同ファイルの `CreateTempFile` が `0600` で作った一時ファイルである。この権限は元のまま検査を通る。
2. **移動後の宛先検証が失敗しても宛先はそのまま**。所見は「宛先ファイルを削除（ロールバック）するか、少なくとも契約として明記する」と述べる。しかし 0155 は `moveFileAnchored` の doc コメントで「rename が成功した後の失敗は、rename を巻き戻さず宛先を残す」ことを意図的な設計として既に宣言している。加えて、`AtomicMoveFile` は既存ファイルの置換にも使えるため、「宛先を削除する」ロールバックは**移動前にそこにあった内容を復元しない**。失敗時に元ファイルも消える方が、宛先に検証失敗ファイルが残るより悪い。したがって本タスクは削除を実装せず、契約の明記で close する。

明記すべき場所は `moveFileAnchored` ではなく、公開 API である `AtomicMoveFile` の doc コメント（[safe_file.go:100-105](../../../internal/safefileio/safe_file.go#L100-L105)）である。現在ここにはパス解決方針しか書かれておらず、失敗時に宛先がどうなるかの記載がない。

### F-5: `Remove` は本番から一度も呼ばれていない

`FileSystem` インターフェースの `Remove`（[safe_file.go:57-58](../../../internal/safefileio/safe_file.go#L57-L58)）は `os.Remove` の素通しである（同 :96-98）。所見は「`ensureParentDirsNoSymlinks` を適用するか、安全性検査なしと明記する」ことを推奨した。

今回、**このメソッドには本番の呼び出し元が 1 つも存在しない**ことを確認した。参照はインターフェース定義、`osFS` の実装、テストのモック（`safe_file_cleanup_test.go`）だけである。紛らわしいことに `internal/runner/base/output/file.go:137` は `Remove` を呼んでいるが、これは `common.FileSystem` のもので、safefileio のものではない。`safe_file_cleanup_test.go` の該当箇所は「`Remove` が呼ばれ **ない** こと」を検証しており、呼び出し元の不在を裏付けている。

読み手のいない API の安全性契約を整えても得るものはない。本タスクは `Remove` を削除する。0157 が「本番呼び出し元を持たない `WithUserGroup`／`IsUserGroupSupported` をインターフェースごと削除する」と判断し、0166 が同じ基準で `privilege.Metrics` を削除したのと同じ扱いである。

ただし後述の F-7 が一時ファイルの後始末を必要とするため、その手段として `Remove` を残す選択肢もありうる。この判断は F-7 の設計と一体であり、`02_architecture.md` で決める。

### F-6: 現存する。実害はまだ無いが、静かに壊れる形をしている

Linux 経路は `openHow.mode` に `uint64(perm)` をそのまま渡す（[safe_file_linux.go:278](../../../internal/safefileio/safe_file_linux.go#L278)）。問題は 2 つある。

- Go の `os.FileMode` は setuid・setgid・sticky を独自のビット位置（`1<<23` 等）で表す。`os.OpenFile` は内部の `syscallMode` でこれを POSIX のビットへ変換してから渡すが、この経路には変換がない。これらのビットを含む `perm` はカーネルへ誤った mode として届く。
- `openat2(2)` は `O_CREAT`／`O_TMPFILE` の無い呼び出しで mode が 0 でないと `EINVAL` を返す。読み取り open に非ゼロの `perm` を渡すと、Linux 経路は失敗し、フォールバック経路（`os.OpenFile` は mode を無視する）は成功する。同じ引数で経路により結果が変わる。

現在の呼び出し元はすべて `0o777` 以下のビットしか渡さず、読み取り時は `perm=0` を渡すため実害は出ていない。将来の呼び出しで静かに壊れる罠として残っている。

### F-7: 唯一の本番呼び出し元は、まさに完全性が重要な書き込みである

`safeWriteFileCommon`（[safe_file.go:204-247](../../../internal/safefileio/safe_file.go#L204-L247)）は `O_WRONLY|O_CREATE` で開き、検証してから `Truncate(0)` し、`Write` する。書き込み中にクラッシュ・ディスク満杯が起きると、切り詰められたファイルや途中までの内容が残る。

公開 API は `SafeWriteFileOverwrite` のみで、その本番呼び出し元は [file_analysis_store.go:162](../../../internal/fileanalysis/file_analysis_store.go#L162) の解析レコード（ハッシュマニフェスト）保存 1 か所だけである。所見が「完全性が重要なファイル」と呼んだものそのものであり、対応の動機は明確である。破損したレコードは後段の検証で改ざんと区別できない。

同じパッケージに一時ファイル＋`AtomicMoveFile` の道具が既にあるのに使われていない、という所見の指摘も現行のままである。ただし経路を差し替えると、次の 2 点が新しい判断を要する。

- **リーフ symlink の扱いが変わる。** 現在は `SafeOpenFile` が `openat2(RESOLVE_NO_SYMLINKS)`（フォールバックでは `O_NOFOLLOW`）で開くため、宛先がすでに symlink なら `ErrIsSymlink` で拒否される。一時ファイルを `rename` で被せる方式では、`rename` は symlink 自体を置き換えるので、リンク先が書き換わる危険はないが、**拒否されるはずのものが黙って置き換えられる**。ADR [resolved-path-symlink-enforcement-adr.ja.md](../../dev/architecture_design/resolved-path-symlink-enforcement-adr.ja.md) は「リーフのシンボリックリンクを検知して拒否する」ことを `safefileio` の設計前提として宣言しており、CLAUDE.md の「正規化するな、拒否せよ」にも照らして、拒否は維持しなければならない。
- **一時ファイルの置き場と後始末。** `rename` がアトミックであるためには一時ファイルは宛先と同じディレクトリになければならない。書き込み失敗時にそれを残さない手段が要る（前述の F-5 との関わり）。

追加コストは、1 回の書き込みあたり create・write・fsync・link・rename が各 1 回発生することである。このうち無視できないのは `fsync` で、耐久性のあるストレージでは一般に 0.5〜10 ms を要する。マニフェスト書き込みは `record` 実行時に対象ファイル 1 件につき 1 回発生するため、対象が数百件なら合計で 0.1〜数秒の増加になりうる。その内容を作る ELF／Mach-O 解析が 1 件あたりミリ秒〜数十ミリ秒であることを踏まえると、これは同じ桁の増加であり、測定に現れないとは言えない。それでも受け入れる。破損したレコードは後段の検証で改ざんと区別できず、`record` は 1 回のコマンド実行に収まる操作だからである。CLAUDE.md の性能方針に従い、実装時に `record` 実行の wall time を変更の前後で実測し、絶対値を `03_implementation_plan.md` に記録する。

### F-8: 読み取りと書き込みで検査が非対称なのは意図的である

書き込みの検査は `(uid, gid, mode)` を見るのに対し、読み取りの検査は `CanCurrentUserSafelyReadFile(stat.Gid, fileInfo.Mode())` と `(gid, mode)` しか見ない（[safe_file.go:451-469](../../../internal/safefileio/safe_file.go#L451-L469)）。所見はこれを「D1 で読み取りポリシーの意図を確認し、コメントに設計意図を明記する」ものと位置づけた Info であり、挙動の変更は求めていない。本タスクはコメントの追記で close する。

この非対称は許容されているだけでなく、対称化してはならない。理由は 3 つある。

1. **厳密な対称化は本番構成を壊す。** 書き込みポリシーは owner-writable なファイルに所有者の一致を要求する（[manager.go:254-258](../../../internal/groupmembership/manager.go#L254-L258)）。これを読み取りに適用すると `0640` のハッシュファイルは所有者の一致を求められるが、[record_command.ja.md](../../user/record_command.ja.md) の分離運用は「所有者を runner 実行者にすると、その実行者が自分でハッシュを書き換えられるようになり、分離した意味が失われます」として、**所有者を付け替えないことを要求している**。読み手と所有者が異なることは事故ではなく、分離運用が成立するための条件である。root 所有のバイナリ・設定ファイルを非 root の runner が読む基本構成も同様に成り立たなくなる。
2. **緩い形にしても既存の検査と重複する。** 所見が心配するのは「悪意ある一般ユーザーが所有するファイル」なので、対称化ではなく「所有者は root か実行者本人に限る」という条件も考えられる。しかしこれは [dir_permissions_unix.go:182](../../../internal/security/dir_permissions_unix.go#L182) がディレクトリに対して既に課している。しかもそちらの方が効く。ファイルの所有者が示すのは誰がその中身を書き換えられるかだけだが、ディレクトリの所有者は誰がそのエントリを別のファイルに差し替えられるかを決める。差し替えが可能な状況では、ファイル側の所有者検査は素通りする。
3. **比較すべき UID が決まらない。** 読み取り検査は `getPermissionCheckUID()`（`SUDO_UID` を考慮する）を使い、書き込み検査は `getProcessRealUID()`（実 UID のみ）を使う。0160・0161 が意図的に分けたものである。読み取りに所有者比較を入れるとどちらと比べるかを決める必要があり、`SudoUIDAware` の下で実 UID が root のときにファイル所有者を `SUDO_UID` と突き合わせることになって、筋の通らない性質が生まれる。

### F-9: 現存する

`openat2` は `syscall.Syscall6` の直呼びで、`errno != 0` をそのまま返す（[safe_file_linux.go:74-96](../../../internal/safefileio/safe_file_linux.go#L74-L96)）。リトライループはない。Go ランタイムの非同期プリエンプション（SIGURG）等でシグナルが届くと `EINTR` が呼び出し元へ抜ける。失敗する方向なので安全側だが、まれな偽陰性エラーの原因になる。

## 目的

- `safefileio` が失敗したときに、fd・作成済みファイル・一時ファイルのいずれも残さないようにする。エラーを返したのに副作用だけが残る状態を無くす。
- エラーを返しても残る副作用が原理的に避けられない箇所（移動後の宛先）については、それを公開 API の契約として明記し、上位層が「エラー＝何も起きていない」と誤読しないようにする。
- ハッシュマニフェストの書き込みをアトミックにし、クラッシュや容量不足で破損したレコードが残らないようにする。その際、リーフ symlink を拒否するという既存の設計前提を崩さない。
- 環境によって保証の強さが違うこと（openat2 の有無）を、利用者向け文書だけでなくコードの公開 API からも読めるようにする。
- 読み手のいない `FileSystem.Remove` を無くし、「safefileio のインターフェースを使っているから安全」という誤解の余地そのものを消す。

## スコープ

### 対象

1. `safeOpenFileFallback` の失敗経路での fd Close と、`O_CREATE` で作成したファイルの後始末（F-3）。
2. `atomicMoveFileCore` の chmod と検証の順序入れ替え、およびそれに伴う「安全でない権限のソースを拒否する」挙動の変更（F-4-1）。
3. `AtomicMoveFile` の doc コメントへの、失敗時に宛先が残りうることの明記（F-4-2）。
4. `FileSystem.Remove` の削除、またはそれに代わる契約の確定（F-5）。
5. openat2 に渡す `mode` の正規化と、不正な `perm` の拒否（F-6）。
6. `safeWriteFileCommon` の一時ファイル＋アトミックな差し替えへの変更（F-7）。併せて、移動と書き込みの経路をディレクトリ fd 起点に変える（`moveFileAnchored` の書き換えを含む）。パスの二度目の解決を無くし、確認したディレクトリと書き込むディレクトリが食い違う余地を仕組みとして消すためである。2026-08-20 承認。
7. openat2 の `EINTR` リトライ（F-9）。
8. 公開 API と package コメントへの、フォールバック経路が best-effort であることの明記（F-2）。
9. `canSafelyReadFromFile` への、読み取り検査が UID を見ない理由の明記（F-8）。
10. 0149 の残件一覧（`98_remaining_issues.md` §2 B1）と findings（`B1_safefileio.md`）への対応状況の反映。

### 対象外

- **非 Linux 経路の dirfd ウォーク実装**（F-2 の主推奨）。上記「背景」のとおり、開発環境でしか効かない対策に見合わない規模であり、OS 管理 symlink の判断が二重化する。本タスクは契約の明記で F-2 を close する。
- **移動後の宛先削除によるロールバック**（F-4-2 の第 1 案）。上記「背景」のとおり、0155 の意図的な設計と衝突し、かつ上書き時に元の内容を復元しないため、より悪い結果を招く。
- **読み取り権限検査に UID を加えること**（F-8 の挙動変更）。groupmembership 側のポリシー設計であり、変更するなら D1 の所見として扱うべき別件である。
- **`safeOpenFileFallback` の二段階検証そのものの再設計**。F-3 は同関数の失敗経路の後始末だけを対象とし、検証方式は変更しない。
- **`common.FileSystem.Remove`（`internal/common/filesystem.go:79`）**。名前が似ているが別インターフェースであり、`output` 層に本番の呼び出し元を持つ。本タスクは safefileio 側のみを扱う。
- **`SafeReadFile` のサイズ上限や読み取り経路**。B1 で良好な防御層として評価されており、所見の対象外である。

## 決定事項

本タスクは、変更範囲が `internal/safefileio` に閉じている点では 0166 と同じだが、`02_architecture.md` を**省略せず作成する**。理由は次のとおり。

- F-7 は書き込み経路の形そのものを変える（その場で open して切り詰める方式から、一時ファイルを作って差し替える方式へ）。新しい経路は `atomicMoveFileCore` を通るが、そこは F-4 で同時に変更する関数である。2 つの変更が同じ地点で合流する。
- F-7 の一時ファイルの後始末をどう実現するかが F-5（`Remove` を消すか残すか）の結論を左右する。両者を別々に決めることができない。
- リーフ symlink の拒否をどこで行うか（差し替えの前か後か、どの検査で）が、ADR の設計前提を保てるかどうかを決める。失敗時にどこまで巻き戻るかの境界と併せて、図で示すべき内容がある。

以下は要件の段階で確定した事項であり、`02_architecture.md` はこれを前提とする。

- **F-2 と F-8 は挙動を変えず、doc の明記のみで close する。** 所見の主推奨（dirfd ウォーク・UID 検査の追加）はいずれも採らない。判断の根拠は本書と `98_remaining_issues.md` に残す。
- **F-4-2 はロールバックを実装せず、契約の明記で close する。**
- **F-4-1 の順序入れ替えに伴う挙動の変更は受け入れる。** 安全でない権限のソースは、権限を狭めてから受け入れるのではなく拒否する。本番の呼び出し元はいずれもこの変更の影響を受けない。
- **F-7 の適用範囲は `safeWriteFileCommon` に限る。** `SafeReadFile` 系および `AtomicMoveFile` の呼び出し規約は変更しない。
- **`SafeWriteFileOverwrite` の外部から見える挙動は、成功時と symlink 拒否時については変わらない。** 変わるのは失敗時に宛先に何が残るかだけである。

## Acceptance Criteria

#### F-001: フォールバック経路の失敗時に資源を残さない（B1 F-3）

エラーを返す経路で、fd も作成済みファイルも残さない。

**Acceptance Criteria**:
- **AC-01**: `safeOpenFileFallback` が `os.OpenFile` 成功後の 2 回目の `ensureParentDirsNoSymlinks` の失敗によりエラーを返すとき、開いた fd が Close されている。
- **AC-02**: AC-01 の経路で、その呼び出しが `O_CREATE` によりファイルを新規作成していた場合、作成されたファイルが残らない。既存ファイルを開いただけの場合は削除しない。
- **AC-03**: AC-02 の削除は、開いた fd が指す inode と削除対象が同一であることを確認したうえで行われる。同一性を確認できない場合は削除せず、その旨を警告として記録し、元のエラーを返す。パス名だけを頼りに削除することはない。
- **AC-04**: AC-01〜AC-03 の各分岐が、意図的に 2 回目のチェックを失敗させるテストで踏まれる。テストは fd が Close されたこと、および作成されたファイルの有無を検証する。
- **AC-05**: AC-04 のテストは、対象の後始末処理を取り除くと失敗する。取り除いた方法と失敗を確認した旨をコミットメッセージに記す。

#### F-002: `AtomicMoveFile` の副作用の順序と契約（B1 F-4）

**Acceptance Criteria**:
- **AC-06**: `atomicMoveFileCore` において、ソースファイルへの `Chmod` が `canSafelyAccessFile` によるソース検証の**後**に実行される。ソース検証が失敗した場合、ソースファイルの権限は呼び出し前から変化していない。
- **AC-07**: AC-06 の順序が、ソース検証を失敗させたうえでソースの権限が変化していないことを確認するテストで検証される。このテストは順序を元に戻すと失敗する。
- **AC-07a**: 読み取り検査（`CanCurrentUserSafelyReadFile`）が拒否する権限を持つソースを `AtomicMoveFile` に渡した場合、`requiredPerm` がより狭い権限であっても、ソース検証で拒否される。権限を狭めてから受け入れることはない。対象は、other から書き込み可能なソース、実行者が属さないグループから書き込み可能なソース、および `MaxAllowedReadPerms`（`0o6775`）を超える権限のソースである。実行者が属するグループから書き込み可能なソースは、読み取り検査の方針どおり従来と同じく受け入れる。
- **AC-07b**: `0600` のソースを `requiredPerm` に `0600` 以外の安全な権限を指定して移動した場合、本タスクの前と同じく成功し、宛先の権限が `requiredPerm` になる。`internal/runner/base/output` の一時ファイル移動が本タスクの前後で同じ結果になる。
- **AC-08**: `AtomicMoveFile` の doc コメントに、宛先への移動が成功した後の検証が失敗した場合、エラーを返しつつ宛先にはファイルが残ることが記載されている。移動前に宛先に別のファイルがあった場合、それは復元されないことも併せて記載されている。
- **AC-09**: AC-08 の doc に、宛先を削除するロールバックを行わない理由（上書き時に元の内容を復元できないため）が記載されている。

#### F-003: `FileSystem.Remove` の解消（B1 F-5）

安全性検査のない `Remove` が、検査を行う他メソッドと同じインターフェースに同居している状態を解消する。

**Acceptance Criteria**:
- **AC-10**: `safefileio.FileSystem` インターフェースに、安全性検査を伴わない `Remove` が公開メソッドとして存在しない。
- **AC-11**: AC-10 の変更により、テスト用モックからも対応する実装が取り除かれ、「`Remove` が呼ばれないこと」を検証していた既存テストが、検証対象の消滅に伴って整理されている。整理により失われるカバレッジがないことを `go tool cover -func` の比較で確認し、その旨をコミットメッセージに記す。
- **AC-12**: `internal/common` の `FileSystem.Remove` と、その本番呼び出し元（`internal/runner/base/output/file.go`）の挙動が変わっていない。
- **AC-13**: `make deadcode` が、本タスクの削除に起因する新たな未使用シンボルを報告しない。

#### F-004: openat2 に渡す mode の正規化（B1 F-6）

**Acceptance Criteria**:
- **AC-14**: `perm` に setuid・setgid・sticky のいずれかのビットが含まれる状態で `SafeOpenFile` が呼ばれた場合、Linux 経路・フォールバック経路のいずれでも同じ sentinel エラー（`errors.Is` で判定できる固定のエラー値）で拒否される。
- **AC-15**: `O_CREATE` を伴わない `SafeOpenFile` の呼び出しにおいて、Linux 経路がカーネルへ渡す mode が 0 である。同じ引数での成否が Linux 経路とフォールバック経路で一致する。
- **AC-16**: `O_CREATE` を伴う呼び出しにおいて、作成されるファイルの権限が本タスクの前後で変わらない。
- **AC-17**: AC-14・AC-15 が、Linux 経路とフォールバック経路の双方（`FileSystemConfig.DisableOpenat2` で切り替え）を通るテストで検証される。

#### F-005: 完全性が重要な書き込みのアトミック化（B1 F-7）

**Acceptance Criteria**:
- **AC-18**: `SafeWriteFileOverwrite` が宛先の差し替え（`rename`）に到達する前に失敗した場合、宛先のファイルは書き込み前の内容のままである。切り詰められた状態や、途中までの内容が残ることはない。差し替えに到達した後の失敗では宛先は新しい内容になり、その場合エラーは `ErrDestinationCommitted` を含んで `errors.Is` で判別できる。
- **AC-19**: `SafeWriteFileOverwrite` が成功した場合、宛先に書き込まれた内容が本タスクの前と同一である。宛先の権限は `perm` と一致する。宛先が新規作成でありかつ `perm` に umask が落とすビットが含まれる場合、および宛先が既存でありその権限が `perm` と異なる場合は、本タスクの前と結果が変わる。これは意図した変更である。
- **AC-20**: 宛先のパスの末尾がすでにシンボリックリンクである場合、`SafeWriteFileOverwrite` は `ErrIsSymlink` で拒否する。リンク自体を置き換えることも、リンク先へ書き込むこともない。この挙動は本タスクの前後で変わらない。
- **AC-21**: 書き込みが宛先の差し替えに到達する前に失敗した場合、宛先のディレクトリに一時ファイルが残らない。差し替えに到達した後の失敗では一時ファイルが残りうるため、その旨が警告として記録される。
- **AC-22**: `filePath.IsParentOnly()` を要求する既存の契約（ADR `resolved-path-symlink-enforcement-adr`）が維持され、`NewResolvedPath` 由来のパスは引き続き `ErrInvalidFilePath` で拒否される。
- **AC-23**: AC-18・AC-21 が、宛先の差し替えに到達する前に失敗を起こすテストで検証される。テストは失敗後の宛先の内容と、宛先ディレクトリの残存エントリの両方を検証する。
- **AC-24**: AC-20 のテストは、リンク先のファイルが書き換わっていないことと、シンボリックリンク自体が置き換えられていないことの両方を検証する。「エラーが返ること」だけを見るのではなく、どちらの誤った挙動も起きていないことを確かめる。
- **AC-25**: `internal/fileanalysis` の解析レコードの読み書きが、本タスクの前後で同じレコードを往復できる。既存のレコードファイルは変更なしに読める。

#### F-006: openat2 の EINTR リトライ（B1 F-9）

**Acceptance Criteria**:
- **AC-26**: `openat2` の呼び出しが `EINTR` を返した場合、呼び出しが再試行され、`EINTR` が呼び出し元へ伝播しない。
- **AC-27**: `EINTR` 以外の errno は、本タスクの前と同じ形で呼び出し元へ伝播する。`ELOOP`・`EEXIST`・`ENOENT` の既存のマッピング（`ErrIsSymlink`・`ErrFileExists`・`os.ErrNotExist`）が変わらない。
- **AC-28**: AC-26 が、`EINTR` を返す状況を作るテストで検証される。テストはリトライを取り除くと失敗する。

#### F-007: 挙動を変えずに契約を明記する所見（B1 F-2・F-8）

**Acceptance Criteria**:
- **AC-29**: `safefileio` の package コメントに、openat2 が利用できる環境とフォールバック経路とで保証の強さが異なること、後者は競合の隙を狭めるが排除はしない best-effort であることが記載されている。
- **AC-30**: `SafeOpenFile`・`SafeReadFile`・`SafeWriteFileOverwrite`・`AtomicMoveFile` の各 doc コメントから、AC-29 の限界が適用されることが読み取れる。package コメントへの参照でよい。
- **AC-31**: AC-29 の記載と `docs/user/security-risk-assessment.ja.md`「前提と限界」節の記述に食い違いがない。本番ターゲットが Linux 5.6+ であること、非 Linux は開発・限定用途に限ることの 2 点が、両者で同じ内容になっている。
- **AC-32**: `canSafelyReadFromFile` の doc コメントに、読み取り検査が所有者 UID を見ず `(gid, mode)` のみで判定すること、およびそれが意図的であることが記載されている。理由として次の 2 点が読み取れる。
  - 所有者の妥当性はディレクトリ権限監査が担っており（所有者は root か実行者本人に限られる）、ファイル単位の読み取り検査はそれを重複させない。ハッシュファイル自身のように、ハッシュ検証を受けない読み取り対象もこの監査で守られる。
  - 読み手と所有者が異なることは分離運用が成立するための条件であり、所有者の一致を要求すると本番構成が動かなくなる。
- **AC-33**: AC-29〜AC-32 の doc はすべて英語で記述される。

#### F-008: 文書への反映

**Acceptance Criteria**:
- **AC-34**: `98_remaining_issues.md` §2 の「B1（safefileio）」から、本タスクで解消した所見が残件として除かれ、解消したことと本タスク・#978 への参照が、同文書が既に D1 M-3・B3 M2・A1 に用いている引用ブロック（`> **… について**`）の形式で記載されている。
- **AC-35**: AC-34 の記載から、F-2・F-4-2・F-8 を所見の主推奨とは異なる形で close したこと、およびその根拠（本番ターゲットの限定、0155 の既存の設計決定、読み取り側のポリシー所在）が読み取れる。
- **AC-36**: `findings/B1_safefileio.md` の F-2〜F-9 に、本タスクでの対応結果が追記されている。所見の原文は書き換えず、監査時点の記述として残す。
- **AC-37**: `98_remaining_issues.md` の B1 以外の残件（D1・D2・E1 ほか）の記述が、本タスクの書き換えによって増減していない。
- **AC-38**: `docs/user/security-risk-assessment.ja.md` に引用されている `safeOpenFileFallback` および `safeOpenFileInternal` のコード片が、F-4・F-6 による変更後の実装と一致している。日本語版を先に更新し、英語版は `/mktrans` で反映する。

## Success Criteria（要件レベル）

- 上記すべての Acceptance Criteria が実装され、対応するテストが `make test` で成功する。
- `make lint` が警告なく通過する。
- 本タスクの前後で、`safefileio` の公開 API が成功したときに書き込まれる内容と移動先のファイルが変わらない。挙動の変化は次の 7 つに限られ、いずれも意図したものである。
  - 失敗したときに何が残るか（fd・作成済みファイル・一時ファイルを残さなくなる）。
  - 安全でない権限のソースを `AtomicMoveFile` に渡したときの結果（権限を狭めて受け入れるのをやめ、拒否する。AC-07a）。
  - `SafeWriteFileOverwrite` の宛先の権限が `perm` と一致するようになる（AC-19）。
  - `SafeWriteFileOverwrite` の宛先が既存かつ読み取り不可の場合に失敗する。宛先の存在確認が読み取りで開くようになるため。
  - `requiredPerm` が移動後の宛先検査を通らない場合、`AtomicMoveFile` が宛先を置き換える前に失敗する。副作用が減る方向の変化である。
  - フォールバック経路で `O_CREATE` を `O_EXCL` なしに使う `SafeOpenFile` が、対象が同時に削除されると `ENOENT` で失敗しうる。
  - 非 Linux で `AtomicMoveFile` が `ErrSourceIdentityMismatch` を返しうる。`moveFileAnchored` が `renameat` の直前に `fstatat` で同一性を確認するようになるため（`02_architecture.md` § 3.4.5・§ 5.3 の R4）。現在の非 Linux 実装は `os.Rename` 一発で確認を持たない。隙が狭まる方向の変化である。2026-08-21 承認。
- リーフ symlink を検知して拒否するという ADR の設計前提が、すべての公開 API について維持されている。
- #978 が挙げる 8 件それぞれについて、解消したのか、所見の推奨とは異なる形で close したのかが、コードと監査文書の双方から追える。
