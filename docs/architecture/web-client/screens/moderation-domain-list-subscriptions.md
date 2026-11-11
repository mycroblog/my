---
start: "/moderation/domain-actions"
persona: "hana"
paths: ["app/web/src/routes/moderation.domain-actions_.subscriptions.tsx", "app/web/src/features/moderation/components/DomainListSubscriptions.tsx", "app/web/src/features/moderation/hooks/useDomainListSubscriptions.ts", "app/web/src/features/moderation/api/domainLists.ts"]
unavailable: {"購読の最後の取得を確かめる": "The demo server's subscriptions would fetch a list from a host the scenario does not serve."}
budget: {"controls": 3, "helpers": 0, "words": 33}
---

## ブロックリストの購読の一覧（`/moderation/domain-actions/subscriptions`）

`manage_domains` を持つ管理者が、ほかのサーバーや人が公開するドメインのブロックリストの購読（[ADR-0159](../../../adr/0159-federation-allowlist-and-blocklist-subscriptions.md) §5）を新しい順に読み、
購読のページか追加のページへ進む画面（[§14](../14-admin.md)）。購読の追加は[追加のページ](moderation-domain-list-subscription-new.md)、取得の履歴・下書き・設定・削除は[購読のページ](moderation-domain-list-subscription.md)で行う。

### 1. 使う人と頻度

- 管理者（`manage_domains`）。共有のブロックリストを使い始めるときと、取得の失敗を知らせる通知（`admin_domain_list_failing`）を受けたとき。月に数回以下。

### 2. 作業

| 作業 | 最短の手順の数 |
| --- | --- |
| どのリストを購読しているかを知る | 2 |
| 購読の最後の取得を確かめる | 2 |
| 購読を追加するページを開く | 3 |

手順は、メニューの「ドメインへの処分」を押す 1 つ、ページの間のリンクの「ブロックリストの購読」を押す 1 つから（表の最後の取得の列でわかる）。追加は「購読を追加」を押す 1 つを足す。

### 3. 情報の優先順位とまとまり

`h1` は「ドメインへの処分」で、その下に[許可したドメイン](moderation-domain-allows.md)と同じページの間の `LocalNav` を置く。その下に「購読を追加」のボタン（`/moderation/domain-actions/subscriptions/new` へのリンク）と、区画の見出しを持たない表を並べる。

1. 購読の表（`RecordTable`、カーソルで 40 件ずつ）: 列は購読（名前、`dir="auto"`。購読のページへのリンク）・モード（「下書き」か「自動」）・最後の取得（結果と日時。続けて失敗しているときはその回数を `danger` の文字で、まだなければ「まだ取得していません」）の 3 つ。
   40rem より狭いコンテナでは行ごとのカードになり、横にスクロールしない。

### 4. 主な操作と保存の仕方

- 主な操作は「購読を追加」（`primary` のリンク）。一覧には保存する操作がない。

### 5. 操作ごとの結果の出し方

| 操作 | 結果（その場・トースト・画面の遷移） |
| --- | --- |
| 購読の名前を押す | 画面の遷移。[購読のページ](moderation-domain-list-subscription.md)へ |
| 「購読を追加」を押す | 画面の遷移。[追加のページ](moderation-domain-list-subscription-new.md)へ |
| 次のページを読み込む | その場。前後のページへ移る。失敗は一覧の中のエラーと再試行 |

### 6. 入口と出口

- 入口: ドメインへの処分のページの間のリンク、取得の失敗の通知（`admin_domain_list_failing`）のリンク、購読のページの「購読の一覧に戻る」と削除。
- 出口: 購読のページ、追加のページ、ページの間のリンクのほかのページ。権限がなければ `AdminError` が足りない権限を示す。
