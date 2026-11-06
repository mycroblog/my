---
start: "/moderation/domain-actions"
persona: "hana"
paths: ["app/web/src/routes/moderation.domain-actions_.import.tsx", "app/web/src/features/moderation/components/DomainBlockCsv.tsx", "app/web/src/features/moderation/hooks/useDomainBlockCsv.ts", "app/web/src/features/moderation/api/domainBlocks.ts"]
unavailable: {"ファイルを選んでインポートをプレビューする": "the agent has no file to choose: the browser tools give it none"}
---

## ドメインへの処分の CSV（`/moderation/domain-actions/import`）

`manage_domains` を持つ管理者が、いま効いている処分を Mastodon のドメインブロックの CSV（FR-706）で書き出し、ほかのサーバーの CSV を読んで、選んだ行を処分として取り込むページ（[§14](../14-admin.md)）。
ドメインへの処分の画面の 2 つ目のページで、`h1` の下の `LocalNav` の「Mastodon の CSV」が今いるページになる。

### 1. 使う人と頻度

管理者（`manage_domains`）。サーバーを始めるとき、信頼するサーバーの公開リストを取り込むとき、ほかのサーバーへ自分の一覧を渡すとき。年に数回。

### 2. 作業

| 作業 | 最短の手順の数 |
| --- | --- |
| 一覧から来て、いま効いている処分を CSV で書き出す | 2 |
| CSV を貼り付けてプレビューし、すべての行を取り込む | 5 |
| ファイルを選んでインポートをプレビューする | 4 |

手順は、一覧の `LocalNav` の「Mastodon の CSV」を押す 1 つから。書き出しは「CSV をエクスポート」を押す 1 つを足す。
取り込みは、CSV の欄に貼り付ける 1 つ、「プレビュー」を押す 1 つ、「{件数} 行をインポート」を押す 1 つ、凍結を含むなら確かめのダイアログの 1 つ。

### 3. 情報の優先順位とまとまり

1. Mastodon の CSV（`h2`）: CSV の働きの説明、「CSV をエクスポート」と結果の `StatusMessage`、インポート（`h3`）の CSV の欄（`TextArea`）とファイルのボタン（`FileTrigger`）と「プレビュー」、
   プレビューの行（取り込む行はドメインで名付けたチェックボックス、取り込まない行はその理由）、非公開メモと「{件数} 行をインポート」、結果の `StatusMessage`。

### 4. 主な操作と保存の仕方

- 主な操作は「{件数} 行をインポート」。プレビューで選んだ行をまとめて保存する。凍結を含むときは `ConfirmDialog`（NFR-MOD-7）で確かめてから送る。
- 書き出しはファイルを保存するだけで、サーバーの状態を変えない。

### 5. 操作ごとの結果の出し方

| 操作 | 結果（その場・トースト・画面の遷移） |
| --- | --- |
| CSV をエクスポートする | その場。ファイルを保存し、`StatusMessage` が言う。失敗は `InlineError` |
| プレビューする | その場。行の一覧が出る。失敗はフォームの `InlineError` |
| インポートする | その場。結果がプレビューに代わり、ボタンがなくなるのでフォーカスはインポートの見出しへ移る。新しい処分は一覧にある |

### 6. 入口と出口

- 入口: ドメインへの処分の `LocalNav` の「Mastodon の CSV」。
- 出口: `LocalNav` の「処分の一覧」で一覧へ。権限がなければ `AdminError` が足りない権限を示す。
