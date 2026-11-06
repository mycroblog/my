---
start: "/admin"
persona: "hana"
paths: ["app/web/src/routes/admin.finance_.publication.tsx", "app/web/src/features/admin/components/FinancePublication.tsx", "app/web/src/features/admin/components/PublicationSettings.tsx", "app/web/src/features/admin/hooks/usePublicationSettings.ts", "app/web/src/features/admin/components/FinanceParts.tsx"]
---

## 費用の公開範囲（`/admin/finance/publication`）

`manage_finance` を持つ管理者が、費用と収入をどこまで公開し、`view_finance` を持つ人に会計のページで何を見せるかを確かめて決める、費用の画面のページ（ADR-0067 §8・§17、ADR-0070 §5、[§14](../14-admin.md)）。
設定はサーバーの設定なので、`manage_settings` も持つ人にだけフォームを出し、ほかの人には今の値を文で書く（監査の A8: 公開するものの隣で決める）。

### 1. 使う人と頻度

- 管理者。費用を初めて登録したときと、公開の方針を変えるときに。年に数回まで。

### 2. 作業

| 作業 | 最短の手順の数 |
| --- | --- |
| 費用の内訳を公開する | 4 |

メニューで「費用」を開く 1 つ、`LocalNav` の「公開範囲」を押す 1 つ、「合計と内訳」を選ぶ 1 つと「公開範囲を保存」の 1 つ。

### 3. 情報の優先順位とまとまり

ページのナビゲーション（`LocalNav`。「公開範囲」が今のページ）の下に、次の区画を置く。

1. 公開範囲: `manage_settings` のある人には、運営の費用（公開するもの・会計のページで見せる項目の名前とメモと収入）と収入の公開のフォームと「公開範囲を保存」の送信の帯（`FormActionBar`）。ほかの人には、`GET /instance` と `GET /finance/cost_totals` から読んだ今の値の文。

### 4. 主な操作と保存の仕方

- 主な操作は「公開範囲を保存」。フォームはまとめて保存し、変えた設定だけを送る。

### 5. 操作ごとの結果の出し方

| 操作 | 結果（その場・トースト・画面の遷移） |
| --- | --- |
| 公開範囲を保存する | その場。フォームの `FormActionBar` |

### 6. 入口と出口

- 入口: 費用の画面の `LocalNav` の「公開範囲」。
- 出口: `LocalNav` のほかのページ。
