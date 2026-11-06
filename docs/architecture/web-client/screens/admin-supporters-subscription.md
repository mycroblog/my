---
start: "/admin"
persona: "hana"
paths: ["app/web/src/routes/admin.supporters_.subscriptions_.$id.tsx", "app/web/src/features/admin/components/SupporterSubscriptionDetail.tsx", "app/web/src/features/admin/hooks/useSupporters.ts", "app/web/src/features/admin/api/supporters.ts", "app/web/src/features/admin/components/supporterParts.tsx"]
---

## サポーター: 加入（`/admin/supporters/subscriptions/$id`）

`manage_supporters` を持つ管理者が、1 つの加入の事実を確かめ、すぐに解約するページ（ADR-0072 §10、[§14](../14-admin.md)）。

### 1. 使う人と頻度

- 管理者。問い合わせや支払いの問題に応えるときに。月に数回。

### 2. 作業

| 作業 | 最短の手順の数 |
| --- | --- |
| サポーターの加入の金額を確かめる | 3 |

メニューで画面を開く 1 つ、`LocalNav` の「加入」の 1 つ、アカウントのリンクの 1 つ。

### 3. 情報の優先順位とまとまり

1. 加入（見出しは利用者名）: すぐの解約の結果の `StatusMessage`、状態・プラン（[プランのページ](admin-supporters-plan.md)へのリンク）・金額と間隔・開始・期間の終わり・猶予の終わり・予定している間隔の変更。
2. 加入を終わらせる: 加入が続いているときだけ。返金しないことと元に戻せないことの説明と「すぐに解約」。

`h1` の下に「加入の一覧へ戻る」のリンクを置く。

### 4. 主な操作と保存の仕方

- 主な操作は「すぐに解約」の 1 つ。確認のダイアログ（`ConfirmDialog`、`danger`）を経る。保存するフォームはない。

### 5. 操作ごとの結果の出し方

| 操作 | 結果（その場・トースト・画面の遷移） |
| --- | --- |
| すぐに解約する | その場。加入の区画の `StatusMessage`。解約の区画が消え、フォーカスは加入の見出しへ移る |

### 6. 入口と出口

- 入口: [加入のページ](admin-supporters-subscriptions.md)のアカウントのリンク。
- 出口: 「加入の一覧へ戻る」、プランのリンク、メニューのほかの画面。
