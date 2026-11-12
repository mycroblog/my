# ADR-0005: GitHub のデータをローカルにキャッシュし、ワークフローの手順をラップする

- Status: Accepted
- Date: 2026-10-22
- Deciders: @rosylilly
- Issue: なし

## Context（背景・課題）

本体の [/work-on-issue](../../../../.claude/skills/work-on-issue/SKILL.md) の手順（Issue の着手条件の確認、PR の作成、CI の待ち、衝突の解消、マージの監視）は、毎回 gh と make の出力を読みながら進めている。
着手条件（担当者なし、依存する ADR が Accepted、待つべき Issue が閉じている）の確認や PR の本文は機械的に決まる。CI の待ちは、ポーリングのたびに文脈全体を読み直すターンを消費する。
決定者の回答（原文、2 は「GitHub のデータをローカルにキャッシュすること」）:

> 1, 2 : OK

## Decision Drivers（判断基準）

- 1 回の手順の出力を、エージェントが次に取る行動に必要な行だけにする
- ユーザー自身の gh の認証を使い、トークンを別に持たない
- 規則の判断（着手してよいか）を、文書を読ませずに機械的に行う

## Considered Options（選択肢）

1. 手順のたびに gh と make を直接使う
2. 手順をラップするコマンドと、Issue・PR のローカルのキャッシュ

## Decision（決定）

選択肢 2。GitHub とのやり取りはすべて gh CLI を通す。

- キャッシュ: Issue（本文・ラベル・状態）と、マージ済みの PR（本文・ブランチ・変更したファイル）を [ADR-0002](0002-knowledge-graph.md) と同じデータベースに保存し、言及する ID と変更したファイルを辺にする。`gh sync` と、15 分より古いときの `ctx` が、前回から更新された分だけを取り直す
- `issue claim <n>`: 着手条件を確かめ、満たさなければ理由を返して止まる。満たせば自分を担当者にし、計画をコメントする
- `issue block <n>`: 待つ理由を、それを止める Issue として記録する
- `pr open`: push し、テンプレートの節を Issue（`Closes #N`、受け入れ基準）とコミットの件名から埋め、Issue に PR を記録する。「`make check` が通る」の項目は、`agentkit check` が同じ作業ツリーで通ったときだけ埋める。stacked PR は `Stacked on #M` を先頭に書く
- `pr wait <pr>`: CI が終わるまでポーリングし、通れば 1 行を、落ちれば失敗したジョブのログから失敗した行だけを返す（全体は `.tmp/agentkit/ci/` に保存）。衝突は終了コード 3。エージェントはバックグラウンドで動かす
- `pr watch`: オーケストレーター向けに、opened・merged・closed・conflicting・ci-failed・ci-green・dequeued の各イベントを 1 行ずつ出す。Monitor ツールの通知と 1 対 1 で対応する
- `check`: `make check-changed` のコンテキストの一覧・結果と、失敗した行だけを返す（全体はログに保存）
- `regen`: 変わった生成元から、必要な生成（generate・sqlc・i18n-gen・web-dist・config-docs・adr-index）を判断して実行する
- マージとキューへの投入はラップしない。本体の規則どおり、オーケストレーターか決定者が行う

## Consequences（影響）

- 初回の同期は数十秒かかる（マージ済みの PR を 50 件ずつ取得する）
- 着手条件の判断は、Issue の本文の表現（「マージされるまで着手しない」など）に依存する。言い回しを増やしたら `text.Dependencies` も直す
- 手順の正本は本体の /work-on-issue のままで、agentkit はその手順を実行する
