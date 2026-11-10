---
start: "/moderation/domain-actions/subscriptions"
persona: "hana"
paths: ["app/web/src/routes/moderation.domain-actions_.subscriptions_.new.tsx", "app/web/src/features/moderation/components/DomainListSubscriptions.tsx", "app/web/src/features/moderation/components/domainListParts.tsx", "app/web/src/features/moderation/hooks/useDomainListSubscriptions.ts", "app/web/src/features/moderation/api/domainLists.ts"]
unavailable: {"https://lists.example/blocklist.csv を下書きのモードで購読する": "The demo server cannot serve a list over https, so the subscription's first fetch fails."}
---

## ブロックリストの購読の追加（`/moderation/domain-actions/subscriptions/new`）

`manage_domains` を持つ管理者が、ほかのサーバーや人が `https` の URL で公開するドメインのブロックリストを購読するページ（[ADR-0159](../../../adr/0159-federation-allowlist-and-blocklist-subscriptions.md) §4・§5、[§14](../14-admin.md)）。
サーバーは購読したらすぐ、そのあと毎日リストを取得し、行をドメインへの処分にする。下書きのモード（既定）では差分が管理者の受け入れを待ち、自動のモードでは取得のたびに反映する。

### 1. 使う人と頻度

- 管理者（`manage_domains`）。共有のブロックリストを使い始めるとき。年に数回。

### 2. 作業

| 作業 | 最短の手順の数 |
| --- | --- |
| https://lists.example/blocklist.csv を下書きのモードで購読する | 4 |

手順は、一覧の「購読を追加」を押す 1 つ、名前と URL の欄に入れる 2 つ（形式とモードは既定のまま）、「購読する」を押す 1 つ。

### 3. 情報の優先順位とまとまり

`h1` は「購読を追加」で、その下に「購読の一覧に戻る」のリンクを置く。フォームは 1 枚のカードで、区画の見出し（`h2`）は持たない。

1. リスト: 名前（必須、200 文字まで、`dir="auto"`）・URL（`type="url"`、必須、2,048 文字まで。`https` だけで、内部のアドレスに向く URL やリダイレクトは取得の失敗になること）・形式（`RadioGroup`、既定は Mastodon の CSV。Mastodon の CSV・Mastodon のサーバーの公開の一覧（JSON）・1 行に 1 つのドメインのテキスト）。
2. 反映のしかた（`h2` を持たない `Fieldset` の並び。購読のページの設定と同じ欄）: モード（`RadioGroup`、既定は「下書き」。どちらでも人が出した処分には触れないこと）・リストの `suspend` を（`Select`: 凍結・制限・何もしない）・リストの `silence` を（`Select`: 制限・何もしない）・メディアの拒否を反映・通報の拒否を反映・リストの公開コメントを写す（`Checkbox`）。
3. 「購読する」（`FormActionBar`）。

### 4. 主な操作と保存の仕方

- 主な操作は「購読する」（`primary`）。保存はすぐ保存で、確かめない（下書きのモードは何も変えずに待ち、購読は消せる）。ブラウザの制約検証が通ってから `POST /admin/domain_list_subscriptions` で送る。

### 5. 操作ごとの結果の出し方

| 操作 | 結果（その場・トースト・画面の遷移） |
| --- | --- |
| 購読する | 画面の遷移。新しい購読のページへ（見出しが結果を示す。G24）。一覧と監査ログは取り直す |
| サーバーが断る（`https` でない URL など） | その場。`FormActionBar`（`role="alert"`）に理由を出し、入力を残す |

### 6. 入口と出口

- 入口: [購読の一覧](moderation-domain-list-subscriptions.md)の「購読を追加」。
- 出口: 新しい購読のページ、「購読の一覧に戻る」。権限がなければ `AdminError` が足りない権限を示す。
