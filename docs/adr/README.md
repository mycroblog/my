# Architecture Decision Records

設計判断の記録です。運用ルールは [ADR-0000](0000-record-architecture-decisions.md) を参照してください。

- ADR は主題ごとに 1 ファイルです。主題を持つ ADR があれば、その ADR を直します。統合で廃番になった番号と移転先は `aliases.yaml` にあり、`scripts/agentkit.sh show` は古い番号も解決します。
- 新規作成: `scripts/task.sh docs:adr-new TITLE="短いタイトル"`。番号は、手元の `docs/adr`、廃番、開いている PR にある ADR の番号の最大に 1 を足して選びます
  （`gh` が使えないときは手元の作業ツリーだけで選び、その旨を出します）。それでも番号が重なると Merge Queue の `docs:adr-numbers` が失敗するので、
  後から並んだ PR が番号を付け直します。
- `Proposed` は推奨案であり未確定です。`Accepted` にできるのは人間の Decider のみです。

## 一覧

索引のファイルは置きません。一覧は次の方法で見ます。

- `scripts/agentkit.sh adr list`: 番号・Status・タイトルの一覧。`--status Proposed` で Status（`- Status:` の行の最初の語）で絞ります
- GitHub: このディレクトリのファイルの一覧と、コード検索 `path:docs/adr "Status: Proposed"`
