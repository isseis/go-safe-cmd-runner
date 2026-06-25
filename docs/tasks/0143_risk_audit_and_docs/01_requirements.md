# 監査・文書整合 — 要件定義書

## Document Status

| Item | Value |
|---|---|
| Status | `draft` |
| Created | 2026-06-20 |
| Review date | - |
| Reviewer | - |
| Comments | - |

> 本書は 0140 を 3 分割した第 3 タスク（横断成果物＝監査・文書）の要件である。分割方針・根本原因の訂正は
> [0140/00_decomposition.md](../0140_risk_level_classification_review/00_decomposition.md)、原典の確定要件・根拠は
> [0140/01_requirements.md](../0140_risk_level_classification_review/01_requirements.md)（superseded）を参照する。
> 先行タスクの分担は次のとおり: コマンド名分類・ラッパ/特権（判断軸1）は 0141、宛先パス信頼区分（判断軸2）は
> 0142。0142 のオペランド抽出層は、宣言的フラグ仕様＋単一 getopt パーサへの集中リファクタ（挙動保存）を 0144 が、
> 各コマンドのフラグ集合の実 CLI 整合（安全側の挙動変更）を 0145 が担う。本書はこれら先行タスクが確定させた分類・
> データ構造を、監査ログ出力・理由コード・移行ノート・文書・sample config へ反映する最終タスクであり、**実装順序の
> 最後**に位置する（0141・0142・0144・0145 の実装完了を前提とする。§3）。

## 1. 背景と目的

0141（判断軸1）・0142（判断軸2）はリスクレベル分類のロジックとデータ構造（判断軸1 の名前集合・判断軸2 の
`LocationResult`／オペランド毎の判定 DTO・新規 `ReasonCode`）を確定させた。続く 0144 はオペランド抽出層を宣言的
フラグ仕様（`flag_spec.go` の `CommandFlagSpec` 群）＋単一 getopt パーサへ集約し（挙動保存）、0145 はその
フラグ集合を実 CLI に整合させて過剰認識を除去した（安全側の挙動変更＝`recognized=true→false` の fail-closed 方向）。
本タスクはこれら先行タスクが確定させた分類・データ構造を **運用者が観測・理解できる形** に仕上げる横断成果物を所有する。
具体的には:

- **監査**: 0142 が `RiskAssessment` へ格納したオペランド毎の判定 DTO を監査ログへ実際に出力し、引き上げ/変更された
  コマンドが deny されたとき対応する理由コードが記録されることを end-to-end で担保する（0142 は DTO の定義と格納まで、
  本タスクが logger 出力を担う＝[00_decomposition.md](../0140_risk_level_classification_review/00_decomposition.md) §3.4）。
- **理由コード**: 0141・0142 が各々追加した `ReasonCode` を統合し、監査ストリーム上での family 区別（システム変更／
  破壊／パス信頼区分由来／特権 等）の最終化と網羅性を確定する。
- **文書**: ユーザー/開発者文書・用語集・分類ガイドを実装の確定挙動に一致させ、破壊的変更（引き上げ・引き下げ
  双方）を移行ノートで周知し、sample/test config を新分類へ追従させる。

本タスクは新しい分類ロジックを **追加しない**。0141・0142（分類）と 0144・0145（抽出層の構造・確定フラグ集合）の
確定挙動を正とし、それを監査・文書へ忠実に射影する。
後方互換不要のため段階ロールアウト/フラグは設けず、破壊的変更は移行ノートでの周知に留める
（[00_decomposition.md](../0140_risk_level_classification_review/00_decomposition.md) §3.2）。

## 2. スコープ

- **In**:
  - **F-001**: 監査ログへのオペランド毎判定フィールドの出力（0142 が `RiskAssessment` へ格納した DTO の logger 射影）。
  - **F-002**: deny 時の理由コード記録の end-to-end での担保（AC-30）と、監査ストリーム上の理由コード family 区別の最終化。
  - **F-003**: 移行ノート（changelog）— 引き上げ（AC-32）・引き下げ（AC-33）双方の周知。
  - **F-004**: ユーザー/開発者文書・用語集の整合（AC-34）。
  - **F-005**: 引き上げ対象コマンドを使う既存 sample/test config の追従（AC-35）。
  - **F-006**: アーキテクト/SRE 向け分類ガイドの最終化（AC-36、日本語確定→英語 `/mktrans` の順序厳守）。
- **Out**:
  - **0141 が担当する項目**: コマンド名分類（判断軸1 の High/Medium 名前集合）、Critical 限定、env/timeout、ラッパ/
    特権、間接実行。本タスクはこれらの確定結果を文書へ反映するのみで、分類ロジックには触れない。
  - **0142 が担当する項目**: 宛先パス信頼区分の判定、オペランド抽出、`LocationResult`、オペランド毎判定 DTO の
    **定義と `RiskAssessment` への格納**（本タスクは格納済み DTO の logger 出力以降を担う）、max 合成。
  - **0144／0145 が担当する項目**: オペランド抽出層の宣言的フラグ仕様化・単一 getopt パーサ（0144）と、フラグ集合の
    実 CLI 整合・過剰認識除去（0145）。本タスクは抽出層の構造・フラグ知識には触れず、確定したフラグ集合・挙動変化を
    文書（検出限界の最終整合＝AC-06）・移行ノート（0145 の安全側挙動変化＝AC-04）へ反映するのみ。
  - `RiskLevel` の段数/意味づけ変更（新レベル追加しない。0140 §6 を継承）。
  - 段階ロールアウト/フラグ/shadow（後方互換不要。新分類は 0141・0142 で直接適用済み。0140/00 §3.2）。
  - 0139 のドキュメント本体（0139 AC-06 の乖離訂正は 0141 が行い、その文書反映のみ本タスク。0139 文書は触らない）。

## 3. 横断制約（0140/00_decomposition.md §3 を継承）

- **実装順序の最後・0141/0142/0144/0145 の実装完了を前提とする**: 本タスクの成果物（監査出力・移行ノート・ガイド・
  sample config）はいずれも 0141・0142（分類ロジック・DTO）と 0144・0145（抽出層の構造・確定フラグ集合）の
  **実装された確定挙動** を正とする。コードと文書の齟齬を防ぐため、これら 4 タスクがマージされ `make test` が緑に
  なっていることを着手の前提とする（[00_decomposition.md](../0140_risk_level_classification_review/00_decomposition.md)
  §2 実装順序・§3.4 build/完了ゲート）。
- **前提とする確定データ（タスク間契約。B-1）**: 本タスクは次を **読み取り専用** で参照し、carrier・分類・コードを
  新設しない（新設すると分類結果が 2 箇所で定義され齟齬を生む）:
  - **carrier**: オペランド毎判定 DTO は `RiskAssessment.OperandZones`（型 `[]risktypes.OperandZone`。
    [types.go](../../../internal/runner/base/risktypes/types.go) / [operand_zone.go](../../../internal/runner/base/risktypes/operand_zone.go)）に格納される。サブフィールドは
    Index/Raw/Resolved/Zone/Role/MatchedCritical/Trusted/UnresolvedErr。
  - **空・nil の意味**: carrier が空（`len()==0`／nil）は判断軸2 非適用を表し、判断軸2 が適用されたが解決不能な
    オペランドは `Zone==ZoneUnresolved` の要素として残る（両者は区別可能）。
  - **判断軸2 由来の `ReasonCode`**: [reason_codes.go](../../../internal/runner/base/risktypes/reason_codes.go) の
    "Destination-path trust-zoning codes (axis 2)" ブロック（`ReasonTrustBoundaryWrite`/`ReasonDestinationZone`/
    `ReasonPermissionGrant`/`ReasonDeviceIO`/`ReasonRecursiveOutsideSafeZone`/`ReasonSensitiveSourceCopy`/
    `ReasonUnresolvedDestination`）。
- **新分類は直接適用済み（後方互換不要。フラグ/shadow なし）**: 破壊的変更（raise/lower）の周知は本タスクの移行
  ノートに委ね、実行時の運用ガード（`RolloutMode` 等）は設けない（§3.2）。
- **バイリンガル文書の編集順序**: ユーザー/開発者文書・ガイドは **日本語版を先に編集・コミットし、英語版へは必ず
  `/mktrans` で反映する**（直接両方を編集しない。CLAUDE.md 翻訳ガイドライン・用語集 `docs/translation_glossary.md`
  に従う）。
- **English ソース**: Go の識別子・コメント・文字列リテラル（監査フィールドのキー名・`ReasonCode` 値を含む）は英語
  （テストソース含む）。

## 4. 機能要件と受け入れ基準

> 各 AC は 0140/01 の対応 AC を継承する（末尾「対応」）。本タスクは分類を追加しないため、AC は「0141/0142 の
> 確定挙動を監査・文書へ忠実に射影できていること」を検証対象とする。

### F-001: 監査ログへのオペランド毎判定フィールドの出力

- **AC-01**（オペランド毎判定 DTO の logger 出力）— 0140/00 §3.4・0142 AC-19 からの引き継ぎ:
  - **参照入力**: 本 AC は carrier フィールド `RiskAssessment.OperandZones`（型 `[]risktypes.OperandZone`。
    [types.go](../../../internal/runner/base/risktypes/types.go) / [operand_zone.go](../../../internal/runner/base/risktypes/operand_zone.go)）を**読み取り専用の入力**とする（carrier・分類は
    §3 のとおり新設しない）。carrier が空（`len()==0`／nil）は判断軸2 非適用、判断軸2 が適用されたが解決不能な
    オペランドは `Zone==ZoneUnresolved` の要素として残る（両者は区別可能）。
  - **規則**: 上記 DTO（サブフィールド: Index/Raw/Resolved/Zone/**Role**/MatchedCritical/Trusted/UnresolvedErr。
    `Role` は write/read を区別し、`ZoneUnresolved` の非対称 fail-closed〔write=High／read=Medium〕の根拠となるため
    監査出力に含める）を、`LogRiskProfile`（[audit/logger.go](../../../internal/runner/base/audit/logger.go)）が
    `command_risk_profile` 監査エントリへ構造化フィールド（`operand_zones`、各要素は上記サブフィールドを持つ JSON
    オブジェクト）として出力する。
  - **存在条件**: carrier が空（`len()==0`／nil）のコマンド（判断軸2 非適用）では `operand_zones` キーを**付けない**
    （既存の `reason_codes`/`risk_factors` が `len()>0` ガードで採る扱いを範例とする。`operand_zones: []`／`null` を
    書かない＝grep/相関を壊さない）。allow/deny いずれの経路でも出力される（`LogRiskProfile` は deny の
    error-return 経路でも常に書かれる）。
  - **秘匿マスキング**: オペランドの Raw/Resolved パス文字列、および `UnresolvedErr` のメッセージは、`Args` と同じ
    出力境界の redaction 機構を**経由するよう本タスクで配線する**（既存の `Args` マスクは別フィールド・別ループの
    ため operand DTO には自動適用されない。`Chain` の Path が未マスクである既存傾向と同根なので、明示配線が必要）。
  - **テスト**:
    - 代表コマンド（`cp evil /usr/bin/ls`・symlink 経由・複数オペランド）で `command_risk_profile` エントリを捕捉し、
      `operand_zones` 配列の各要素の Index/Raw/Resolved/Zone/Role/Trusted が carrier に格納された値どおりに出力される
      ことを表明。
    - carrier が空のコマンドでは `operand_zones` キーが**無い**こと、deny 経路でも出力されることを表明。
    - **漏えい否定テスト（S-1）**: Raw/Resolved/`UnresolvedErr` に秘匿パターン（資格情報を含むパス等）を持つオペランドを
      与え、出力で当該秘匿値が**マスクされている**ことを表明（存在テストでなく漏えいの否定）。
    （新規 AC＝[00_decomposition.md](../0140_risk_level_classification_review/00_decomposition.md) §3.4・§4 の新規 AC 注記
    「オペランド毎の監査フィールドの logger 出力」／0142 AC-19 からの引き継ぎ。deny 時の理由コード記録〔0140 AC-30〕は
    AC-02 が継承）

### F-002: deny 時の理由コード記録と監査ストリームの family 区別の最終化

- **AC-02**（deny 時の理由コード記録, end-to-end）— 0140 AC-30:
  - **規則**: 0141・0142 で引き上げ/変更されたコマンドが deny されたとき、`command_risk_profile` 監査エントリの
    `reason_codes`（および Critical/Blocking deny では `blocking_reason`）に、その deny を生んだ判定経路に対応する
    理由コードが記録される。
  - **テスト**: 代表ケースで deny エントリを捕捉し理由コードを表明する — 判断軸1 由来（例 `insmod`＝
    `system_modification`）、判断軸2 由来（例 trust-critical 書込＝`ReasonTrustBoundaryWrite`／`trust_boundary_write`）、
    危険引数パターン由来（`dd if=`＝`dangerous_arg_pattern`）。判断軸2 由来コードは
    [reason_codes.go](../../../internal/runner/base/risktypes/reason_codes.go) の "axis 2" ブロックの定数名
    （`ReasonTrustBoundaryWrite`/`ReasonPermissionGrant`/`ReasonDeviceIO`/`ReasonRecursiveOutsideSafeZone`/
    `ReasonSensitiveSourceCopy`/`ReasonUnresolvedDestination`/`ReasonDestinationZone`）を正確に引く（S-3）。理由コードは
    family が区別できる（AC-03 で機械的に担保）。
- **AC-03**（理由コード family 区別の最終化）— [00_decomposition.md](../0140_risk_level_classification_review/00_decomposition.md)
  §4 NF-001 の「監査ストリームの family 区別の最終化」:
  - **規則**: 0141・0142 が各々追加した `ReasonCode`（[reason_codes.go](../../../internal/runner/base/risktypes/reason_codes.go)）を
    統合し、監査ストリーム上で各コードが属する family（profile 由来／runtime・argument 由来／パス信頼区分由来／特権／
    binary-analysis／uncertain 等）を **機械的に区別できる根拠を定義する**。現状 `reason_codes.go` の family は
    コメント上のグルーピングに留まり強制されていないため、(a) family を返す明示的マッピング（コード→family テーブル、
    または規約化したプレフィクス）を定義し、(b) **全 `ReasonCode` がいずれかの family に漏れなく分類される**ことを
    テストで担保する（exhaustive/distinct だけでは family 区別を保証しない＝S-2）。新規 family を導入する場合のみ
    マッピングを拡張し、既存コードの意味・値は変えない。
  - **根拠**: 0141・0142 は各々自タスク分の網羅性テストを緑に保つ（NF-001, per task）が、複数タスクが追加したコードを
    跨いだ family 区別の最終確認は横断成果物として本タスクが所有する。family 区別はインシデント相関で「判断軸1（名前/
    profile）由来か判断軸2（パス信頼区分）由来か」を判別する基盤となる（[00_decomposition.md](../0140_risk_level_classification_review/00_decomposition.md) §4）。

### F-003: 移行ノート（changelog）— 破壊的変更の周知

- **AC-04**（移行周知・引き上げ）— 0140 AC-32: 本一連の変更で **Low/Medium → High** に引き上がるコマンド群
  （0141 F-001〜F-002、0142 の trust-critical ケース）により、従来許可していた config がブロックされ得ることを移行
  ノートとして文書化する。安全運用は allowlist + ハッシュ固定 + 明示的な `risk_level` 設定が前提であることを併記する。
  - **0145 由来の fail-closed 化（過剰認識除去）の周知**: 0145 は実 CLI に存在しないフラグの受理（過剰認識）を除去し、
    当該フラグを与えた入力を `recognized=true→false`（＝High fail-closed）へ倒した（例 `sponge -r`／`mkdir -a`／
    `touch -p`／`unlink -r`／`rmdir -r`／`mv -s` 等。0145 §1.1・AC-02）。これは分類の raise ではなく精度是正だが、
    従来これらの形を **recognized のまま** 通していた config は新たに High で deny され得る（fail-loud）。移行ノートに
    「対象コマンド×無効フラグ形が fail-closed に変わる」旨を引き上げ側の周知として明記する（緩和方向ではないため
    AC-05 の独立警告ブロックには含めない）。
- **AC-05**（移行周知・引き下げ）— 0140 AC-33: 本一連の変更で **High → Medium/Low** に引き下がるコマンド
  （`rm`/`rmdir`/`shred`/`unlink`/`dd` の safe-zone/ordinary ケース＝0142 の D7 引き下げ）を **セキュリティ緩和方向の
  変更（security relaxation）** として移行ノートに明示する（baseline は直近リリースの挙動）。
  - **独立提示（B-2）**: 引き下げは引き上げリストへ並記して埋没させず、**独立した見出し／警告ブロック**で提示する。
    対象コマンドと緩和条件（safe-zone/ordinary）・baseline を明記する。**根拠（fail-loud/fail-silent の非対称）**: 引き上げ
    （ブロック増）は実行時に config が deny されて即気づく（fail-loud）が、引き下げ（許可増）は静かに通る
    （fail-silent）ため、移行ノートが唯一の事前検知手段になる。緩和が無監査で本番投入されるのを防ぐため視認性を要件化する。
  - **完了条件**: 引き上げ（AC-04）と引き下げ（AC-05）が同一移行ノート内に存在し、かつ引き下げセクションが独立見出し／
    警告ブロックとして視認上埋没しないこと。

### F-004: ユーザー/開発者文書・用語集の整合

- **AC-06**（文書整合）— 0140 AC-34: 次を本一連の確定分類（判断軸1 High/Medium 名前集合・判断軸2 のパス信頼区分・
  Critical 限定・max 合成）に一致するよう更新する。バイリンガル編集順序（§3）に従い日本語版を先に編集・コミットし、
  英語版へは `/mktrans` で反映する:
  - ユーザー向け: [docs/user/risk_assessment.ja.md](../../../docs/user/risk_assessment.ja.md) → [risk_assessment.md](../../../docs/user/risk_assessment.md)
  - 開発者向け: [command-risk-evaluation.ja.md](../../../docs/dev/architecture_design/command-risk-evaluation.ja.md) →
    [command-risk-evaluation.md](../../../docs/dev/architecture_design/command-risk-evaluation.md)
  - 用語集: [docs/translation_glossary.md](../../../docs/translation_glossary.md) に本一連で用いた用語（パス信頼区分／
    trust-critical／safe-zone／オペランド毎判定 等）を追加し、訳語を統一する。**changelog の和語は「移行ノート」を
    canonical とし（0141/0142/0143 で統一済み。N-4）、用語集に登録する**。
  - 0141 が開発者文書へ暫定追記した検出限界（0141 AC-20）の最終整合・日英反映を本タスクが所有する。**0144/0145 を
    反映した記述更新**: 検出限界の記述は「コマンドごとに独自実装した引数パーサ」を前提としていたが、0144 が宣言的
    フラグ仕様（`flag_spec.go`）＋単一 getopt パーサへ集約し、0145 がフラグ集合を実 CLI に整合させた。よって開発者文書の
    フラグ解析・検出限界の節は、(i) 現行アーキテクチャ（宣言的仕様＋単一パーサ＋完全性メタテスト）を反映し、(ii) 0145 で
    解消した過剰認識（実 CLI に無いフラグの受理）を旧制約として残さないよう更新する。fail-closed `Recognized` contract が
    安全保証を担う点（0142 §3.2）は不変として明記する。
  - **完了条件（observable 化, B-3）**: レビュアの主観でなく、次のチェックリストの充足をもって完了とする
    （0140 で起きた「文書↔実装の乖離」の再生産を防ぐ）:
    - (a) **追加の網羅**: 実装で High/Medium/Low に確定した全コマンド分類が、各文書の分類表に漏れなく反映されている。
    - (b) **除去の確認（削除の検証）**: 旧分類の残存記述を明示的に除去確認する。最低限の除去確認項目: 旧
      「fdisk/mkfs=Medium」（0141 AC-21 が High へ訂正）・旧「`rm`/`dd` の無条件 High」（0142 D7 で safe-zone/ordinary は
      引き下げ）が、いずれの文書にも残っていないこと。
    - (c) **0139 との上書き関係（N-1）**: 0139 文書本体は触らない（§2 Out）が、0139 と本一連で記述が衝突する箇所
      （例 0139 AC-06）は、本一連の分類が上書きする関係を移行ノート（AC-04/AC-05）側で明示する（0140 AC-27 を継承）。

### F-005: 既存 sample/test config の追従

- **AC-07**（sample/test config 追従）— 0140 AC-35: 本一連の変更で分類が引き上がるコマンドを使用する既存の
  sample（[sample/](../../../sample/)）／テスト用 config が、新しいレベルの下でも整合する（必要な `risk_level` 設定が
  付与されている）よう更新・検証する（0139 AC-14 と同型）。
  - **網羅の担保（B-3）**: 「`make test` が緑」だけでは、テストに載っていない sample config の追従漏れ（fail-silent）を
    検知できない。よって完了条件に**対象 config の網羅的な洗い出し**を含める: 引き上げ対象コマンド名（判断軸1 の
    High/Medium 化対象・判断軸2 の trust-critical 書込形）に加え、**0145 で過剰認識除去により fail-closed 化した
    無効フラグ形**（`sponge -r`／`mkdir -a`／`unlink -r`／`mv -s` 等）を含む sample/test config を grep で列挙し、各々に
    対し `risk_level` 付与済み、または新分類下で意図した結果（deny/allow）になることを確認する。
  - **検証**: 上記で列挙した config が `make test` 内でロード・評価でき、テスト用 config が新分類で**意図せず** deny
    されないこと（意図的に deny を検証する config はその旨を明示）。

### F-006: リスクレベル分類ガイドの最終化

- **AC-08**（分類ガイドの最終化）— 0140 AC-36: アーキテクト/SRE 向け概念ガイド
  [risk-level-classification-guide.ja.md](../../../docs/dev/architecture_design/risk-level-classification-guide.ja.md)
  を最終化する。**作業順序は厳守**する:
  1. **0141・0142・0144・0145 のコード実装完了が先**（§3 の前提）。
  2. 実装完了後、現在 draft の日本語版を実装の確定挙動（判断軸1/判断軸2・max 合成・Critical 限定・safe-zone の
     安全要件等）に合わせて改訂・確定する（実装と齟齬しないこと）。
  3. 日本語版の確定後に **最後に英語版 `risk-level-classification-guide.md` を `/mktrans` で作成** する（CLAUDE.md の
     バイリンガル方針・翻訳ガイドライン・用語集に従う）。英語版は実装・日本語確定前には作成しない。
  - **完了条件（observable 化, B-3）**: 「最終化」を主観で閉じない。(a) ガイドの分類記述が AC-06 の分類表と突き合わせて
    齟齬が無いこと、(b) ガイド冒頭の Status が `draft` から確定状態へ遷移していること、(c) 英語版が存在し日本語版と
    構造一致（CLAUDE.md 翻訳ガイドライン）すること、の 3 点をもって完了とする。

## 5. 非機能要件

- **NF-001**（理由コード網羅性の最終化）: 全タスク統合後、`ReasonCode` の網羅性（exhaustive）・一意性（distinct）
  テスト（[reason_codes.go](../../../internal/runner/base/risktypes/reason_codes.go) の検証）が緑であること。0141・0142 が
  追加したコードを含む最終状態の family 区別を本タスクで確定する（AC-03。0140 NF-001／[00_decomposition.md](../0140_risk_level_classification_review/00_decomposition.md) §4）。
- **NF-002**: `make test`・`make lint`・`make fmt` がすべて成功する。本タスクは監査出力（`internal/runner/base/audit`）と
  config（`sample/`・テスト用 config）に触れるため、完了の判定基準にはこれらを含む `make test` 全体の成功を含める。
  （0140 NF-002）
- **NF-003**（横断 NF: AC-28 runtime==dry-run を含む）: 監査出力は決定的で、同一 `RiskAssessment` に対し常に同一
  フィールドを書く。本タスクは新たな FS 副作用や非決定な入力（live identity 等）を監査経路へ持ち込まない（パス解決・
  identity の決定性は 0142 が担保済み）。**AC-28 は全タスク横断 NF だが、本タスクは runtime のパス/identity 評価を
  追加せず監査出力と文書のみを扱うため、本タスクのスコープでは自明に充足（N/A）**。（0140 NF-003／AC-28）
- **NF-004**（ログレベル方針, S-4）: `command_risk_profile` のログレベル選択（`LogRiskProfile` の既存
  [riskLogLevel](../../../internal/runner/base/audit/logger.go)。allow かつ Low は Debug）は本タスクのスコープ外で
  既存挙動を踏襲する。ただし**引き下げ（緩和）対象コマンド（AC-05）の allow-Low は Debug に落ちると本番ログ設定で
  `operand_zones` が失われ事後追跡できなくなる**ため、この既知の制約を移行ノート（AC-05）に運用注記として記載する
  （緩和コマンドの監査証跡を残したい場合のログレベル設定を運用者へ周知）。本タスクで `riskLogLevel` の挙動自体は変えない。

## 6. スコープ外の根拠

- **分類ロジックは 0141/0142**: コマンド名分類・宛先パス信頼区分の判定・オペランド抽出・max 合成・DTO 定義と
  `RiskAssessment` への格納は 0141/0142 の所掌。本タスクは確定済みの分類結果・DTO を監査・文書へ射影するのみで、
  分類の値そのものを決めない（決めると同一挙動が 2 箇所で定義され齟齬を生むため）。
- **段階ロールアウト/フラグ/shadow は無し**: 後方互換不要のため（0140/00 §3.2）。破壊的変更は移行ノート（AC-04/
  AC-05）での周知に留める。
- **`RiskLevel` 段数/新レベル**: 変更しない（0140 §6 を継承）。
- **完全な情報漏えい（read）モデルの文書化**: 機密ファイル下限（0142）の文書反映は行うが、完全な read 系分類は
  将来課題であり本タスクでも導入しない（0140/02 §9 を継承）。
