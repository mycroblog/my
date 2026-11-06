---
start: "/admin"
persona: "hana"
paths: ["app/web/src/routes/admin.supporters_.plans_.new.tsx", "app/web/src/features/admin/components/SupporterPlanForm.tsx", "app/web/src/features/admin/hooks/useSupporters.ts", "app/web/src/features/admin/api/supporters.ts", "app/web/src/features/admin/components/supporterParts.tsx"]
unavailable: {"プランを作成する": "a new plan needs a role that nobody holds, and the demo scenario has none"}
---

## サポーター: プランの作成（`/admin/supporters/plans/new`）

`manage_supporters` を持つ管理者が、サポーターのプランを作るページ（ADR-0072 §3、[§14](../14-admin.md)）。

### 1. 使う人と頻度

- 管理者。課金を始めるときと、プランを足すときに。年に数回。

### 2. 作業

| 作業 | 最短の手順の数 |
| --- | --- |
| プランを作成する | 7 |

メニューで画面を開く 1 つ、`LocalNav` の「プラン」の 1 つ、「プランを作成する」の 1 つ、ロールのキー・名前・月額を入れる 3 つ、「プランを作成する」を送る 1 つ（通貨・割引率・上限・順位・公開は既定のまま）。

### 3. 情報の優先順位とまとまり

1. 作成のフォーム: 説明、ロールのキー・名前・説明・月額と通貨・割引率・絵文字の上限・順位・公開、「プランを作成する」と「キャンセル」（プランの一覧へ戻る）の `FormActionBar`。

`h1` の下に「プランの一覧へ戻る」のリンクを置く。

### 4. 主な操作と保存の仕方

- 主な操作は送信の「プランを作成する」の 1 つ。フォームをまとめて送る。

### 5. 操作ごとの結果の出し方

| 操作 | 結果（その場・トースト・画面の遷移） |
| --- | --- |
| プランを作成する | 画面の遷移。作ったプランのページへ移り、結果をトーストで出す。拒否の理由はその欄に出し、入力を残す |

### 6. 入口と出口

- 入口: [プランのページ](admin-supporters-plans.md)の「プランを作成する」。
- 出口: 作ったプランのページ、「キャンセル」と「プランの一覧へ戻る」でプランの一覧。
