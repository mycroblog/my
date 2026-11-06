---
start: "/admin"
persona: "hana"
paths: ["app/web/src/routes/admin.supporters_.plans.tsx", "app/web/src/features/admin/components/SupporterPlans.tsx", "app/web/src/features/admin/hooks/useSupporters.ts", "app/web/src/features/admin/api/supporters.ts", "app/web/src/features/admin/components/supporterParts.tsx"]
---

## サポーター: プラン（`/admin/supporters/plans`）

`manage_supporters` を持つ管理者が、サポーターのプランを一覧で見て、作成のページとプランごとのページへ行くページ（ADR-0072 §3、[§14](../14-admin.md)）。サポーターの画面の `LocalNav` の「プラン」。

### 1. 使う人と頻度

- 管理者。プランを作るときと変えるときに。年に数回。

### 2. 作業

| 作業 | 最短の手順の数 |
| --- | --- |
| 廃止したプランだけを見る | 4 |

メニューで画面を開く 1 つ、`LocalNav` の「プラン」の 1 つ、状態の `Select` を開く 1 つと「廃止」を選ぶ 1 つ。

### 3. 情報の優先順位とまとまり

1. 絞り込みと作成: 状態の `Select`（すべて・提供中・廃止。URL の `?state=`）と、[作成のページ](admin-supporters-plan-new.md)へ行く「プランを作成する」。
2. プランの表: 提供中のプランを順位の順に、そのあとに廃止したプラン。列は名前（[プランのページ](admin-supporters-plan.md)へのリンク、`dir="auto"`）・状態（提供中・提供中 (非公開)・価格を準備中・廃止）・作成の 3 つ。ページングする。
   空のときは空の状態（作成へ導く 1 文）。

### 4. 主な操作と保存の仕方

- 主な操作は「プランを作成する」の 1 つ（作成のページへのリンク）。このページでは何も保存しない。

### 5. 操作ごとの結果の出し方

| 操作 | 結果（その場・トースト・画面の遷移） |
| --- | --- |
| 状態で絞り込む | その場。一覧が最初のページから出直す（URL が変わる） |
| プランを作成する・プランを開く | 画面の遷移 |

### 6. 入口と出口

- 入口: サポーターの画面の `LocalNav` の「プラン」、プランのページと作成のページの「プランの一覧へ戻る」。
- 出口: 作成のページ、プランのページ、`LocalNav` のほかのページ。
