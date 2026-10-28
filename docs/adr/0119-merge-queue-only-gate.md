# ADR-0119: Merge Queue の検査を唯一の関門にし、手元の検査を必須にしない

- Status: Accepted
- Date: 2026-10-29
- Deciders: @rosylilly
- Issue: #1734

## Context（背景・課題）

[ADR-0079](0079-ci-single-gate-merge-queue.md) は、PR の `check` を preflight だけにし、コンパイル・型検査・テストを Merge Queue に寄せた。
そのとき、Merge Queue に壊れた PR を入れない役を「手元の `check:changed`」に持たせ、コミット・push・PR の前に必ず走らせることにした
（[workflow.md §5](../agents/workflow.md#5-verify)、AGENTS.md、CONTRIBUTING.md、PR テンプレートの「passes locally」項目、`agentkit pr open`）。

この規則は、実際には次の問題を起こした。

- 1 台のマシンで 5 エージェントを並行させた日、手元の `check:changed` が負荷でタイムアウトした（Go の sqlite アダプタのテストが 600 秒に達した）。
  検査のために資源を食い合い、並行の速さを自分で潰している。
- その日 Merge Queue が止めた 2 件は、手元の検査では見つからなかった。
  #1704 は L4 パーサとの PR をまたいだ相互作用で、#1678 は proxy.golang.org のネットワークエラーである。
  手元の検査が防げるのは自分の PR の中で閉じた失敗だけで、Merge Queue が見ているもの（統合した状態）とは別物である。
- Merge Queue で外れた PR は、直して入れ直すだけで、後続の PR は止まらずにマージされていく。
  手元の検査で「何度もやり直しになる」のを避けるという理由は成り立たない。
  PR 同士の衝突や順序の問題は、計画（[workflow.md §9](../agents/workflow.md#9-parallel-batches)）で避けるもので、各自のマシンの資源で避けるものではない。

## Decision Drivers（判断基準）

- 検査は、結果を決める状態（統合した状態）でだけ走らせる。同じ検査を 3 回通さない。
- 並行で動くエージェントのマシン資源を、意味のない検査に使わない。
- 手元の実行は、CI の失敗を再現するか、書いている途中で確かめるために使う。義務にしない。

## Considered Options

1. 手元の `check:changed` を必須のまま残す（ADR-0079 のまま）。
2. PR でも全検査を走らせる（`ci:full` を既定にする）。
3. 手元の必須も PR の全検査もやめ、Merge Queue の全検査だけを関門にする。

## Decision（決定）

決定者は、手元の `check:changed` を必須に残すか、PR で全検査を走らせるか、どちらもやめるかを問われ、どちらもやめると決めた。決定者の言葉を引く。

> C ですね。意味がないことをしない。「何度もやり直しになる」も意味不明です。単にデキューされて後続 PR がマージされていく話だし、影響範囲がどう作用するかの計画性の話を適当なマシンリソースを食い潰す理由にしかしていない。

それ以前の発言も引く。

> 手元の check:changed … CI で落ちたものを再現取るために実行するのはわかるんですけど、二度手間なだけでなんら価値を感じません。

選択肢 3 を採る。

1. Merge Queue の全検査が、`main` に入る前の唯一の関門である。
2. PR の `check`（preflight）は変えない。`ci:full` ラベルも今のまま、Merge Queue の失敗の調査や外部の貢献の早期確認に使う。
3. エージェントも人間も、`check` / `check:changed` をコミット・push・PR の前に走らせる義務を負わない。
   手元の実行は、CI の失敗の再現と、書いている途中の確認のためのものである。
4. Merge Queue から外れた PR は、持ち主が直して入れ直す。その間も列の残りは進む。
5. 並行する PR の順序や衝突は、計画の問題（workflow §9）として扱う。手元で検査を走らせる理由にしない。
6. フォーマット（`web:fmt`、`gofmt`）は、preflight が見るものなので、これまでどおり push の前にかける。安く、preflight の失敗を避けられる。
7. `agentkit check` は任意のコマンドとして残す。`agentkit pr open` は、手元の検査を通したことを PR の項目にしない。

[ADR-0079](0079-ci-single-gate-merge-queue.md) のうち、手元の検査を Merge Queue の前の関門にする部分（「守るものと防ぐ場所」の「手元」の行と、それに依る記述）をこの ADR が置き換える。
それ以外（preflight の中身、Merge Queue の構成、`ci:full`）は ADR-0079 のままである。

## Consequences（影響）

- 良い影響: 並行バッチで、各エージェントのマシンが検査で詰まらない。PR を早く出せる。
- 良い影響: 検査の義務が、結果を決める場所（Merge Queue）に一本化される。
- 悪い影響: 自分の PR の中で閉じた失敗も、Merge Queue で初めて見つかることが増える。外れた PR は持ち主が直して入れ直す。
- 悪い影響: Merge Queue の失敗が増える場合は、`ci:full` の使い方や PR の preflight の範囲を、新しい ADR で見直す。
