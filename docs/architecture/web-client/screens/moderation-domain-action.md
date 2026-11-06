---
start: "/moderation/domain-actions"
persona: "hana"
paths: ["app/web/src/routes/moderation.domain-actions_.$id.tsx", "app/web/src/features/moderation/components/DomainActionDetail.tsx", "app/web/src/features/moderation/hooks/useDomainActions.ts", "app/web/src/features/moderation/api/staff.ts"]
seed: [{"my": ["admin", "domain", "issue", "-kind", "limit", "-public-comment", "Unsolicited replies to many people here.", "noisy.example"]}]
---

## ドメインへの処分の詳細（`/moderation/domain-actions/{id}`）

`manage_domains` を持つ管理者が、1 つの処分（FR-702、`GET /admin/domain_actions/{id}`）のコメントと日時を読み、解除するページ（[§14](../14-admin.md)）。

### 1. 使う人と頻度

管理者（`manage_domains`）。なぜその処分を出したかを確かめるとき、相手のサーバーが改善して解除するとき。月に数回より少ない。

### 2. 作業

| 作業 | 最短の手順の数 |
| --- | --- |
| 一覧から処分を開き、公開コメントと非公開メモを読む | 1 |
| 処分を解除する | 2 |
| 同じドメインへのすべての処分を読む | 2 |

手順は、一覧の行のドメインのリンクを押す 1 つから。解除は「解除」を押す 1 つを足す（確かめない）。同じドメインの処分は「このドメインへのすべての処分」を押す 1 つを足す。

### 3. 情報の優先順位とまとまり

`h1` は処分の種類とドメイン（「noisy.example の「制限」」）で、その下に「ドメインへの処分に戻る」のリンクを置く。

1. 状態（`h2`）: 状態・処分した日時・終わる日時（解除したものは解除した日時）と、有効な処分には「解除」と、解除で何が残るかの説明、結果の `StatusMessage`。
2. 詳細（`h2`）: ドメイン（`DomainName`）と「このドメインへのすべての処分」（`/moderation/domain-actions?filter=all&domain=…`）・種類・公開の一覧での隠し方・公開コメント・非公開メモ（どちらも平文で `dir="auto"`、なければ「なし」）。

### 4. 主な操作と保存の仕方

- 主な操作は「解除」（有効な処分だけ）。すぐ保存で、確かめない（解除はまた処分を出せば戻せる）。
- 解除した処分は、ページの値とすべての一覧のキャッシュに応答を書き、有効な処分の一覧からは外す。

### 5. 操作ごとの結果の出し方

| 操作 | 結果（その場・トースト・画面の遷移） |
| --- | --- |
| 解除する | その場。状態が「解除済み」になり、ボタンがなくなるので、フォーカスは状態の見出しへ移り、`StatusMessage`（`role="status"`）が何を解除したかを言う。失敗はトースト（`danger`）で、ボタンとフォーカスはそのまま |
| 「このドメインへのすべての処分」を押す | 画面の遷移。そのドメインの一覧（すべて）へ |
| 「ドメインへの処分に戻る」を押す | 画面の遷移。[一覧](moderation-domain-actions.md)へ |

### 6. 入口と出口

- 入口: [一覧](moderation-domain-actions.md)の行のドメインのリンク。
- 出口: 一覧。知らない ID は `RouteError`（404）、権限がなければ `AdminError` が足りない権限を示す。
