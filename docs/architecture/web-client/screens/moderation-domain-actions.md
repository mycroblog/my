---
start: "/moderation"
persona: "hana"
paths: ["app/web/src/routes/moderation.domain-actions.tsx", "app/web/src/features/moderation/components/DomainActions.tsx", "app/web/src/features/moderation/hooks/useDomainActions.ts", "app/web/src/features/moderation/api/staff.ts"]
seed: [{"my": ["admin", "domain", "issue", "-kind", "limit", "-public-comment", "Unsolicited replies to many people here.", "noisy.example"]}]
---

## ドメインへの処分の一覧（`/moderation/domain-actions`）

`manage_domains` を持つ管理者が、リモートのサーバーへの処分（FR-702）を新しい順に読み、1 つの処分の詳細か新しい処分のフォームへ進む画面（[§14](../14-admin.md)）。
一覧は読むだけで、処分を出すのは[新しい処分](moderation-domain-action-new.md)、解除するのは[処分の詳細](moderation-domain-action.md)、Mastodon の CSV は [CSV のページ](moderation-domain-actions-import.md)で行う。

### 1. 使う人と頻度

- 管理者（`manage_domains`）。連合先のサーバーからスパムや嫌がらせが届いたとき、通報や連合先の詳細から来る。月に数回。
- いま効いている処分を見直す管理者。期限の近いものや古いものを確かめる。月に 1 回ほど。

### 2. 作業

| 作業 | 最短の手順の数 |
| --- | --- |
| モデレーションのメニューから来て、いま効いている処分を読む | 1 |
| 解除・期限切れも含めて、すべての処分を読む | 3 |
| 処分の公開コメントを読む | 2 |
| 新しい処分のフォームを開く | 2 |

手順は、メニューの「ドメインへの処分」を押す 1 つから。すべての処分は、ツールバーの絞り込みを開く 1 つと「すべて」を選ぶ 1 つを足す。
公開コメントは、行のドメインのリンクを押す 1 つを足す（詳細のページにある）。フォームは「新しい処分」を押す 1 つを足す。

### 3. 情報の優先順位とまとまり

`h1` の下に画面のページの `LocalNav`（「処分の一覧」・「Mastodon の CSV」）を置き、その下に「新しい処分」のボタン（`/moderation/domain-actions/new` へのリンク）、
`?domain=` のときはそのドメインだけを出していることと「すべてのドメインを表示」、ツールバーの絞り込み（「有効」が既定、「すべて」は URL の `filter=all`）、処分の表を並べる。区画の見出し（`h2`）は持たない。

1. 処分の表（`RecordTable`）: 列はドメイン（`DomainName` で U-label と A-label。詳細へのリンク）・種類・状態・終わる日時（なければ「解除するまで」）の 4 つ。公開コメントと非公開メモは一覧に出さない。
   40rem より狭いコンテナでは行ごとのカードになり、横にスクロールしない。

### 4. 主な操作と保存の仕方

- 主な操作は「新しい処分」（`primary` のリンク）。一覧には保存する操作がない。
- 絞り込みはその場で URL に置き、一覧は最初のページから出直す。

### 5. 操作ごとの結果の出し方

| 操作 | 結果（その場・トースト・画面の遷移） |
| --- | --- |
| 絞り込みを変える | その場。URL の `filter` が変わり、一覧が最初のページから出直す |
| 行のドメインを押す | 画面の遷移。[処分の詳細](moderation-domain-action.md)へ |
| 「新しい処分」を押す | 画面の遷移。[新しい処分](moderation-domain-action-new.md)へ（`?domain=` のときはそのドメインを入れて） |
| 「すべてのドメインを表示」を押す | その場。URL から `domain` が外れ、一覧が出直す |
| 次のページを読み込む | その場。前後のページへ移る。失敗は一覧の中のエラーと再試行 |

### 6. 入口と出口

- 入口: モデレーションのメニュー（`manage_domains`）、連合先の詳細の「モデレーション」の区画のリンク（`?domain=…`）、監査ログの `domain` の項目のリンク（`?filter=all&domain=…`）、詳細と新しい処分のページの「ドメインへの処分に戻る」。
- 出口: 処分の詳細、新しい処分、CSV のページ。権限がなければ `AdminError` が足りない権限を示す。
