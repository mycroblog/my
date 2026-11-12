# ADR-0001: agentkit をリポジトリ内の独立した Go モジュールとして置く

- Status: Accepted
- Date: 2026-10-22
- Deciders: @rosylilly
- Issue: なし（agentkit の作業は Issue を作らずに進める）

## Context（背景・課題）

ADR・設計文書・運用文書、GitHub の Issue、サーバー・クライアント・スキーマの実装と、1 つのタスクのために読む前提が膨大になり、コーディングエージェントのトークン負荷が無視できなくなった。
このリポジトリの Claude Code のセッション記録（460 本、約 5 万ターン）を集計すると、次のことが分かった。

- 費用の約 7 割はキャッシュ読み込み（10.8B トークン）で、1 ターンあたりの文脈は平均約 22 万トークン。費用は「文脈の長さ × ターン数」で決まる
- ツールの出力の 39% は `grep` / `rg` / `find`、22% は `cat` / `sed` / `head` で、Read ツールは 1 割に満たない
- 探索の対象は文書（約 1 割）よりコード（api/spec、internal/app、web/）のほうが大きい
- CI の待ち（`gh pr checks --watch` など）は 1 回の出力は小さいが、1 回ごとに文脈全体を読み直すターンを消費する

この問題を解く道具（ナレッジグラフ、検索、ワークフローのラッパー）を作る。
決定者の指示（原文）:

> 「my」の話ではないので docs/adr にこの話は入れたくないです。あえてやるなら tools/agentkit 内に ADR を置いてそこで管理、Issue もこのためには作らず作り上げたいです。
>
> 1, 2 : OK

（1 は「`tools/agentkit` を別の Go モジュールとして置くこと」、2 は「GitHub のデータをローカルにキャッシュすること」への回答。）

## Decision Drivers（判断基準）

- サーバーの依存方向・ビルド・ライセンスの規則（本体の ADR-0002・ADR-0011）を崩さない
- Claude Code 以外のエージェント（Codex など）からも使える
- 本体の決定記録と混ざらない

## Considered Options（選択肢）

1. `tools/agentkit` の独立した Go モジュール（`go.mod` を持つ）
2. 本体モジュールの `cmd/` の下
3. 別リポジトリ

## Decision（決定）

選択肢 1。

- 置き場所は `tools/agentkit`（`module github.com/mycroblog/my/tools/agentkit`）。本体の `go vet ./...`・`golangci-lint`・depguard の対象外になり、本体の「バイナリは `cmd/my` だけ」という規則にも触れない
- 依存は、本体の `go.sum` にすでにある版のモジュールだけに限る（`scripts/agentkit-deps-check.sh`）。ライセンスは本体の `make go-licenses` が審査済みになる。`CGO_ENABLED=0` でビルドできること
- agentkit の決定は `tools/agentkit/docs/adr/` に置き、本体の `docs/adr/`・ADR の索引・要件の表には入れない。agentkit の作業には Issue を作らない
- 本体の規則のうち、SPDX ヘッダ、Conventional Commits、文書は現在の状態だけを書くこと（本体の ADR-0021）は agentkit にも適用する
- チェックは `make agentkit-check`（`make check` の 1 コンテキスト、CI の `ctx-agentkit.yml`）

## Consequences（影響）

- 本体の変更と同じ PR で、agentkit と context map（[ADR-0003](0003-context-map.md)）を直せる
- 依存の追加は、本体がそのモジュールを使うまでできない。必要になったら、この ADR を改める
- `make check` に agentkit のビルドとテストの時間（キャッシュ済みで数十秒）が加わる
