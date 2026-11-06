---
start: "/admin"
persona: "hana"
paths: ["app/web/src/routes/admin.finance_.costs.tsx", "app/web/src/features/admin/components/CostItems.tsx", "app/web/src/features/admin/components/FinanceParts.tsx", "app/web/src/features/admin/hooks/useFinance.ts", "app/web/src/features/admin/api/finance.ts"]
---

## 登録した費用（`/admin/finance/costs`）

`manage_finance` を持つ管理者が、登録した費用の項目を一覧し、新しい費用の登録（[admin-finance-costs-new.md](admin-finance-costs-new.md)）と項目のページ（[admin-finance-costs-item.md](admin-finance-costs-item.md)）へ進む、費用の画面のページ（FR-901・FR-902、ADR-0067、[§14](../14-admin.md)）。

### 1. 使う人と頻度

- 管理者。月に 1 回から数回、請求が来たときや契約を変えたときに。

### 2. 作業

| 作業 | 最短の手順の数 |
| --- | --- |
| 登録した費用の今の金額を知る | 2 |

メニューで「費用」を開く 1 つと、`LocalNav` の「登録した費用」を押す 1 つ。今の金額は一覧の列にある。

### 3. 情報の優先順位とまとまり

ページのナビゲーション（`LocalNav`。「登録した費用」が今のページ）の下に、次の区画を置く。

1. 登録した費用: 「費用を登録する」（`/admin/finance/costs/new` へのボタンの形のリンク）と、ページングした `RecordTable`（新しい順）。
   列は 4 つ: 名前（行見出し。項目のページへのリンク、`dir="auto"`）・周期・今の金額・最初の支払日。そのほかの事実（事業者・区分・終了日・金額の改定・メモ）は項目のページに置く。狭い幅では行ごとのカード。
   空のときは、空の状態（登録へ導く 1 文と「費用を登録する」）。

### 4. 主な操作と保存の仕方

- 主な操作は「費用を登録する」。ページに 1 か所だけ置く（一覧があれば一覧の上、空なら空の状態）。このページでは保存しない。

### 5. 操作ごとの結果の出し方

| 操作 | 結果（その場・トースト・画面の遷移） |
| --- | --- |
| 「費用を登録する」を押す | 新しい費用のページへ遷移する |
| 項目の名前を押す | 項目のページへ遷移する |
| 次・前のページへ | その場。一覧の表を入れ替える |

### 6. 入口と出口

- 入口: 費用の画面の `LocalNav` の「登録した費用」、新しい費用と項目のページの「登録した費用に戻る」、項目を取り消した後。
- 出口: 新しい費用のページ、項目のページ、`LocalNav` のほかのページ。
