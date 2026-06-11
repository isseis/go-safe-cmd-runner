# メモ：Ubuntu 26.04 Rust coreutils 対応

## 発端（課題提起の原文）

Ubuntu 26.04 から基本コマンドの多くが Rust で書き直され、`/usr/lib/cargo/bin/coreutils` 以下に単一バイナリ（へのハードリンク）として実装されるようになった。

これに伴い、次の問題が生じている。

1. バイナリファイルだけを解析すると `dlsym`, `mprotect` などが解析されることになるため、基本的なコマンド `mkdir` なども HighRisk 扱いされてしまう。
2. パス名なしで TOML ファイルに記述した場合に安全なディレクトリにないと判断される。

coreutils はシステム標準コマンドのため `/usr/bin` ディレクトリなどと同じく原則として安全なディレクトリとして扱いたい。

また安全なコマンドに関しては、バイナリ解析結果によらずリスク判定を LowRisk, MiddleRisk に下げたい。

## 方針に関する決定事項（確認済み）

- **適用範囲**：coreutils ディレクトリ（`/usr/lib/cargo/bin/coreutils`）配下に解決されたコマンドのみを対象とする。`/usr/bin` 等の挙動は変更しない。
- **リスク水準**：コマンド別に Low/Medium を割り当てる（既知の安全コマンドは Low、その他の coreutils コマンドは Medium）。

## 調査メモ（アーキテクチャ検討の参考。設計は mkarch で詳細化）

- coreutils の各コマンド（例 `/usr/bin/mkdir`）はシンボリックリンク解決後 `/usr/lib/cargo/bin/coreutils/<cmd>` を指し、その実体は全サブコマンド共通の単一バイナリ（へのハードリンク）。
  - したがって**単一バイナリのバイナリ解析は、特定サブコマンドのリスク判定材料として原理的に無意味**（全サブコマンドが同じシンボル `dlsym`/`mprotect`/socket 系を共有するため区別不能）。
- リスク評価には独立した 2 経路がある。
  - 実行時のブロック判定：`risk.EvaluateRisk`（`internal/runner/base/risk/evaluator.go`）。privilege escalation → destructive → network（バイナリ解析）→ system modification → 既定 Low の順。**ディレクトリ別の既定リスクは参照していない**。
  - dry-run の表示：`security.AnalyzeCommandSecurity`（`internal/runner/base/security/command_analysis.go`）。こちらはディレクトリ別既定リスクを参照する。
  - → 2 経路でリスク判定結果が一致するようにする必要がある。
- すでに加えられている暫定変更：
  - `internal/common/secure_path.go` に coreutils ディレクトリを追加済み。これにより安全ディレクトリ判定（問題 2）はほぼ解消（安全ディレクトリパターンと PATH 探索の双方を満たす）。
  - `internal/runner/base/security/directory_risk.go` に coreutils=Medium を追加済みだが、これは dry-run 経路にしか効かず実行時ブロック（問題 1）には無効。
- 潜在的不整合（本タスクのスコープ外だが記録）：
  - `IsDestructiveFileOperation` / `IsSystemModification` はコマンド名を basename 化せず照合する一方、実行時の `ExpandedCmd` は解決済みフルパス。フルパスに対して破壊的/システム変更判定がマッチしない可能性がある。
  - そのため「破壊的コマンドは破壊的検出に任せる」前提は coreutils 配下では不確実であり、破壊的コマンドの High 維持はリスク判定側で自己完結的に担保する必要がある。
