---
start: "/admin"
persona: "hana"
paths: ["app/web/src/routes/admin.supporters_.subscriptions.tsx", "app/web/src/features/admin/components/SupporterSubscriptions.tsx", "app/web/src/features/admin/hooks/useSupporters.ts", "app/web/src/features/admin/api/supporters.ts", "app/web/src/features/admin/components/supporterParts.tsx"]
---

## サポーター: 加入（`/admin/supporters/subscriptions`）

`manage_supporters` を持つ管理者が、サポーターの加入を絞り込んで探し、加入ごとのページへ行くページ（ADR-0072 §10、[§14](../14-admin.md)）。サポーターの画面の `LocalNav` の「加入」。

### 1. 使う人と頻度

- 管理者。支払いの失敗や問い合わせを確かめるときに。月に数回。

### 2. 作業

| 作業 | 最短の手順の数 |
| --- | --- |
| 加入を状態で絞り込む | 4 |

メニューで画面を開く 1 つ、`LocalNav` の「加入」の 1 つ、状態の `Select` を開く 1 つと選ぶ 1 つ。

### 3. 情報の優先順位とまとまり

1. 絞り込み: プランと状態の `Select`（URL の `?plan=`・`?state=`）。
2. 加入の表: 新しい順。列はアカウント（[加入のページ](admin-supporters-subscription.md)へのリンク。消去したアカウントは「削除されたアカウント」）・プラン（`dir="auto"`）・状態・期間の終わりの 4 つ。ページングする。

### 4. 主な操作と保存の仕方

- 主な操作は絞り込みの 1 つ。このページでは何も保存しない。すぐの解約は加入のページに置く。

### 5. 操作ごとの結果の出し方

| 操作 | 結果（その場・トースト・画面の遷移） |
| --- | --- |
| 加入を絞り込む | その場。一覧を読み直す（URL が変わる） |
| 加入を開く | 画面の遷移 |

### 6. 入口と出口

- 入口: サポーターの画面の `LocalNav` の「加入」、加入のページの「加入の一覧へ戻る」。
- 出口: 加入のページ、`LocalNav` のほかのページ。
