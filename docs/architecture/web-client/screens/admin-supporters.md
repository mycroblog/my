---
start: "/admin"
persona: "hana"
paths: ["app/web/src/routes/admin.supporters.tsx", "app/web/src/features/admin/components/SupporterProgram.tsx", "app/web/src/features/admin/hooks/useSupporters.ts", "app/web/src/features/admin/api/supporters.ts", "app/web/src/features/admin/components/supporterParts.tsx"]
---

## サポーター: 課金（`/admin/supporters`）

`manage_supporters` を持つ管理者が、サポーターの課金を有効にし、決済の事業者の状態を確かめ、サポーターの一覧を誰に見せるかを決めるページ（FR-910〜FR-912、ADR-0072、[§14](../14-admin.md)）。
サポーターの画面の最初のページで、`h1` の下の `LocalNav`（「課金」「プラン」「加入」）から[プラン](admin-supporters-plans.md)と[加入](admin-supporters-subscriptions.md)のページへ行く。

### 1. 使う人と頻度

- 管理者。課金を始めるときと、サポーターの一覧の公開を変えるときに。決済の事業者の状態は月に数回。

### 2. 作業

| 作業 | 最短の手順の数 |
| --- | --- |
| 決済の事業者の照合が成功しているかを知る | 1 |
| サポーターの一覧をログインした人に公開する | 3 |

どれもメニューで画面を開く 1 つから。公開は「ログインした人」を選ぶ 1 つと「保存する」の 1 つ（`manage_settings` のある人だけ）。

### 3. 情報の優先順位とまとまり

1. 課金: 有効か、事業者、続いている加入の数、警告、「課金の機能を有効にする」のチェックボックスと、その下の「サポーターの一覧」（`h3`。一覧を見られる人。`manage_settings` のある人には選択肢、ほかの人には今の値の文）。最後に「保存する」と `FormActionBar`。
2. 決済の事業者: 設定の事業者があるときだけ。状態・最後の照合と「今すぐ照合する」。

### 4. 主な操作と保存の仕方

- 主な操作は「保存する」の 1 つ。課金の有効・無効とサポーターの一覧の公開は 1 つのフォームでまとめて保存し、変えたものだけを送る。
- 「今すぐ照合する」は保存ではなく、決済の事業者に照合を頼む補助の操作。

### 5. 操作ごとの結果の出し方

| 操作 | 結果（その場・トースト・画面の遷移） |
| --- | --- |
| 課金とサポーターの一覧を保存する | その場。フォームの `FormActionBar`。拒否の理由も同じ場所 |
| 今すぐ照合する | その場。区画の中 |

### 6. 入口と出口

- 入口: 管理のモードのメニューの「サポーター」。
- 出口: `LocalNav` のプランと加入のページ、課金の警告から規約とポリシーの画面の特定商取引法に基づく表記などのページ、メニューのほかの画面。
