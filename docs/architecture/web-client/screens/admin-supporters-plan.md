---
start: "/admin"
persona: "hana"
paths: ["app/web/src/routes/admin.supporters_.plans_.$id.tsx", "app/web/src/features/admin/components/SupporterPlanDetail.tsx", "app/web/src/features/admin/components/SupporterPlanForm.tsx", "app/web/src/features/admin/hooks/useSupporters.ts", "app/web/src/features/admin/api/supporters.ts", "app/web/src/features/admin/components/supporterParts.tsx"]
---

## サポーター: プラン（`/admin/supporters/plans/$id`）

`manage_supporters` を持つ管理者が、1 つのプランの事実を確かめ、条件を変え、廃止するページ（ADR-0072 §3、[§14](../14-admin.md)）。

### 1. 使う人と頻度

- 管理者。価格や特典を変えるときと、プランを廃止するときに。年に数回。

### 2. 作業

| 作業 | 最短の手順の数 |
| --- | --- |
| プランの月額を変える | 5 |

メニューで画面を開く 1 つ、`LocalNav` の「プラン」の 1 つ、プランの名前のリンクの 1 つ、月額を入れる 1 つ、「保存する」の 1 つ。

### 3. 情報の優先順位とまとまり

1. プラン（見出しはプランの名前、`dir="auto"`）: 廃止の結果の `StatusMessage`、廃止したプランならその旨、価格の準備中ならその旨、説明、状態・月額・年額と割引率・順位・公開・絵文字の上限・ロールのキーと位置・ロールの権限・加入中の数・作成・廃止の日時。
2. プランを編集する: 提供中のときだけ。名前・説明・月額・割引率・絵文字の上限・順位・ロールの位置・公開のフォームと、「保存する」の `FormActionBar`。
3. プランを廃止する: 提供中のときだけ。元に戻せないことの説明と「廃止」。

`h1` の下に「プランの一覧へ戻る」のリンクを置く。

### 4. 主な操作と保存の仕方

- 主な操作は「保存する」の 1 つ。フォームをまとめて保存し、変えた項目だけを送る。
- 廃止は確認のダイアログ（`ConfirmDialog`、`danger`）を経る。

### 5. 操作ごとの結果の出し方

| 操作 | 結果（その場・トースト・画面の遷移） |
| --- | --- |
| プランを保存する | その場。フォームの `FormActionBar`。拒否の理由はその欄 |
| プランを廃止する | その場。プランの区画の `StatusMessage`。編集と廃止の区画が消え、フォーカスはプランの見出しへ移る |

### 6. 入口と出口

- 入口: [プランのページ](admin-supporters-plans.md)の名前のリンク、作成のページの作成の後、加入のページのプランのリンク。
- 出口: 「プランの一覧へ戻る」、メニューのほかの画面。
