---
start: "/admin"
persona: "hana"
paths: ["app/web/src/routes/admin.webhooks_.$id.tsx", "app/web/src/features/admin/components/WebhookSettings.tsx", "app/web/src/features/admin/components/WebhookDeliveries.tsx", "app/web/src/features/admin/components/webhookParts.tsx", "app/web/src/features/admin/hooks/useWebhooks.ts", "app/web/src/features/admin/api/webhooks.ts"]
seed: [{"api": "hana", "method": "POST", "path": "/admin/webhooks", "body": {"url": "https://hooks.example/in", "description": "Moderation chat", "events": ["report.created"]}}]
notes: {"Moderation chat にテストのメッセージを送る": "the demo server reaches no receiver: the test delivery stays failing"}
---

## Webhook のページ（`/admin/webhooks/{id}`）

`manage_webhooks` を持つ管理者が、1 つの webhook の設定を変え、有効と無効を切り替え、テストを送り、秘密を作り直し、削除し、送信履歴を見てもう一度送るページ（[ADR-0060](../../../adr/0060-instance-webhooks.md)、FR-722、[§14](../14-admin.md)）。

### 1. 使う人と頻度

- 管理者。設定した直後（テストを送る）、受け取り側を変えたとき、`admin_webhook_disabled` の通知でサーバーが無効にした webhook を直すとき。月に数回以下。

### 2. 作業

| 作業 | 最短の手順の数 |
| --- | --- |
| Moderation chat にテストのメッセージを送る | 3 |
| Moderation chat の秘密を作り直す | 4 |
| Moderation chat を削除する | 4 |

手順は、メニューで一覧を開く 1 つ、名前を押す 1 つ、「テスト送信」を押す 1 つ。秘密の作り直しと削除は、ボタンと確かめのボタンの 2 つ。

### 3. 情報の優先順位とまとまり

1. 見出し（`h1`「Webhook」）と「webhook の一覧へ戻る」。
2. Webhook（区画の見出しは説明、なければ URL のホスト名）: URL（`dir="ltr"`）・イベント・状態（サーバーが無効にしたときはその理由）・最後の成功から失敗した回数と最初の失敗の日時・変更日時の説明リスト。
3. 設定: URL・説明・イベントのフォーム。
4. 状態・テスト・秘密: 「無効にする」か「有効にする」（同じ位置のボタン）、「テスト送信」（有効なときだけ）、「秘密を作り直す」、「削除」。作り直した秘密はこの区画に一度だけ出す。
5. 送信履歴: 過去 14 日の送信の新しい順の表（ページング）。イベントとメッセージ ID（`dir="ltr"`）・結果と試行回数と最後の試行（結果・状態コード・日時）・作成日時・「もう一度送る」。

### 4. 主な操作と保存の仕方

- 主な操作は設定の「保存する」。設定はまとめて保存（`FormActionBar`）。
- 秘密の作り直しは `ConfirmDialog`（受け取り側が 24 時間のうちに新しい秘密に変えること）、削除は元に戻せないので `ConfirmDialog` で確かめる。無効と有効、テスト、もう一度送るは確かめずにすぐ送る。

### 5. 操作ごとの結果の出し方

| 操作 | 結果（その場・トースト・画面の遷移） |
| --- | --- |
| 保存する | その場。ボタンの隣に結果（`role="status"`）。拒否はその項目に出して入力を残し、ほかの失敗はボタンの隣（`role="alert"`） |
| 無効にする・有効にする・テスト送信 | その場。区画の `StatusMessage`、失敗は同じ場所（`role="alert"`）。ボタンが切り替わってもフォーカスが残る。テストは送信履歴を読み直す |
| 秘密を作り直す | その場。確かめの後、区画に新しい秘密とコピーのボタン（次の操作で消える） |
| 削除 | 画面の遷移とトースト。確かめの後、一覧へ移り、削除したことをトーストで知らせる。失敗はダイアログの中 |
| もう一度送る | その場。履歴を読み直し、履歴の区画の `StatusMessage` に知らせる |

### 6. 入口と出口

- 入口: 一覧の名前、追加のページの「Webhook を開く」。
- 出口: 「webhook の一覧へ戻る」、削除の後の一覧。
