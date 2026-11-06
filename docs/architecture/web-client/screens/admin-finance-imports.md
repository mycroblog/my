---
start: "/admin"
persona: "hana"
paths: ["app/web/src/routes/admin.finance_.imports.tsx", "app/web/src/features/admin/components/BillingImports.tsx", "app/web/src/features/admin/components/FinanceParts.tsx", "app/web/src/features/admin/hooks/useFinance.ts", "app/web/src/features/admin/api/finance.ts"]
unavailable: {"利用料金をすぐに取り込む": "the demo server configures no billing source, so the page lists none"}
---

## 利用料金の取り込み（`/admin/finance/imports`）

`manage_finance` を持つ管理者が、クラウドの利用料金の取り込み元ごとの状態を確かめ、すぐに取り込む、費用の画面のページ（FR-904、ADR-0068、[§14](../14-admin.md)）。
取り込み元は設定した数だけの短い一覧なので、ページングしない。

### 1. 使う人と頻度

- 管理者。取り込みの失敗の通知（`MyBillingImportFailing`）が来たときや、月の初めに。

### 2. 作業

| 作業 | 最短の手順の数 |
| --- | --- |
| 最後の取り込みが成功したかを知る | 2 |
| 利用料金をすぐに取り込む | 3 |

どれもメニューで「費用」を開く 1 つと、`LocalNav` の「利用料金の取り込み」を押す 1 つから。すぐに取り込むのは、取り込み元の「今すぐ取り込む」を押す 1 つ。

### 3. 情報の優先順位とまとまり

ページのナビゲーション（`LocalNav`。「利用料金の取り込み」が今のページ）の下に、次の区画を置く。

1. 利用料金の取り込み: 取り込みと失敗の結果、取り込み元ごとの事業者（`h3`）、タイムゾーン・最後の成功・最後の実行と失敗の種類・今日と今月の要求の数と上限・BigQuery の今月の課金のバイト数の説明リスト、取り込んだ月ごとの種類（見込み・確定・訂正）と合計、「今すぐ取り込む」。
   取り込み元がないときは、空の状態。

### 4. 主な操作と保存の仕方

- 主な操作は取り込み元ごとの「今すぐ取り込む」。保存するフォームはない。

### 5. 操作ごとの結果の出し方

| 操作 | 結果（その場・トースト・画面の遷移） |
| --- | --- |
| 今すぐ取り込む | その場。区画の `StatusMessage`。1 時間以内の 2 回目（429）は `role="alert"` で次の 1 時間に試すよう案内する |

### 6. 入口と出口

- 入口: 費用の画面の `LocalNav` の「利用料金の取り込み」、監査ログの取り込み元の行。
- 出口: `LocalNav` のほかのページ。
