---
start: "/admin"
persona: "hana"
paths: ["app/web/src/routes/admin.finance.tsx", "app/web/src/features/admin/components/FinanceTotals.tsx", "app/web/src/features/admin/components/FinanceParts.tsx", "app/web/src/features/admin/hooks/useFinance.ts", "app/web/src/features/admin/api/finance.ts", "app/web/src/features/costs/**"]
---

## 費用の概要（`/admin/finance`）

`manage_finance` を持つ管理者が、サーバーの費用と収入の集計を読む、費用の画面の最初のページ（FR-901・FR-917、ADR-0067・ADR-0067 §16、[§14](../14-admin.md)）。
費用の画面は、概要・支払いと返金（[admin-finance-income.md](admin-finance-income.md)）・登録した費用（[admin-finance-costs.md](admin-finance-costs.md)）・利用料金の取り込み（[admin-finance-imports.md](admin-finance-imports.md)）・公開範囲（[admin-finance-publication.md](admin-finance-publication.md)）のページに分かれ、`h1` の下の `LocalNav` でページの間を行き来する。
数は「費用の概要」（`StatTile`・`TrendChart`・`BreakdownBar`。ADR-0110）で先に出す。期間・計上のしかたは URL に置く。

### 1. 使う人と頻度

- 管理者。月に 1 回から数回、請求が来たときや支払いの後に。

### 2. 作業

| 作業 | 最短の手順の数 |
| --- | --- |
| 今月の費用が収入でまかなえているかを知る | 1 |
| 今月なにに最もかかっているかを知る | 1 |
| 費用と収入の推移を表で読む | 2 |

どれもメニューで画面を開く 1 つから。知る作業は画面を開いただけで済む（今月の収支のタイルと、事業者ごとの内訳の `BreakdownBar`）。表で読むのは、グラフの「表」を選ぶ 1 つ。

### 3. 情報の優先順位とまとまり

ページのナビゲーション（`LocalNav`。「概要」が今のページ）の下に、画面の説明と次の区画を置く。

1. 集計: 計上のしかたと週・月・年のタブ。その中に、今の期間の費用・収入・収支の `StatTile`（前の期間からの変化。収支が負なら「まかなえていない」）、期間ごとの費用と収入の棒の `TrendChart`（表に切り替えられる）、今の期間の事業者ごとの `BreakdownBar`、期間の区切りの規則、収入の総額・手数料の表とプランごとの加入者の数と数え方の規則。通貨が 2 つ以上なら、グラフの代わりに期間ごとの表。

### 4. 主な操作と保存の仕方

- 読むページで、保存する操作はない。主な操作は期間と計上のしかたを選ぶこと。

### 5. 操作ごとの結果の出し方

| 操作 | 結果（その場・トースト・画面の遷移） |
| --- | --- |
| 期間・計上のしかたを変える | その場。集計を読み直す（URL が変わる） |
| グラフと表を切り替える | その場。選んだ形はこのブラウザのすべてのグラフに残る |
| ほかのページへのリンクを押す | そのページへ遷移する。概要の期間・計上のしかたはそのページの URL に持ち越さない |

### 6. 入口と出口

- 入口: 管理のモードのメニューの「費用」。
- 出口: `LocalNav` の費用の画面のほかのページ、メニューのほかの画面。
