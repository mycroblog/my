---
start: "/admin"
persona: "hana"
paths: ["app/web/src/routes/admin.webhooks_.new.tsx", "app/web/src/features/admin/components/WebhookSettings.tsx", "app/web/src/features/admin/components/webhookParts.tsx", "app/web/src/features/admin/hooks/useWebhooks.ts", "app/web/src/features/admin/api/webhooks.ts"]
---

## Webhook を追加（`/admin/webhooks/new`）

`manage_webhooks` を持つ管理者が、webhook を設定し、その秘密を一度だけ受け取るページ（[ADR-0060](../../../adr/0060-instance-webhooks.md)、FR-722、[§14](../14-admin.md)）。
秘密は二度と表示しないので、設定してもページを移らず、フォームの代わりに秘密を出す。

### 1. 使う人と頻度

- 管理者。外部のサービスへの連携を始めるとき。年に数回。

### 2. 作業

| 作業 | 最短の手順の数 |
| --- | --- |
| 通報が作られたら `https://chat.example/hook` に知らせる webhook を追加し、秘密を受け取る | 5 |

手順は、メニューで一覧を開く 1 つ、「Webhook を追加」を押す 1 つ、URL を入れる 1 つ、イベントの「通報が届いた」を選ぶ 1 つ、「設定する」を押す 1 つ（説明は任意。秘密は押した後に出る）。

### 3. 情報の優先順位とまとまり

1. 見出し（`h1`「Webhook を追加」）と「webhook の一覧へ戻る」。
2. フォーム: URL（必須、2048 バイトまで）・説明（200 文字まで）・イベント（`role="group"` の中の `Checkbox`、イベントの種類ごとに 1 つ。権限の足りないイベントは選べない）。
3. 設定した後: 結果の `StatusMessage`、秘密（`whsec_…`、`dir="ltr"`）と二度と表示しない旨とコピーのボタン、「Webhook を開く」（webhook のページへのリンクのボタン）。

### 4. 主な操作と保存の仕方

- 主な操作は「設定する」。フォームはまとめて保存（`FormActionBar`）。設定した後の主な操作は「Webhook を開く」。

### 5. 操作ごとの結果の出し方

| 操作 | 結果（その場・トースト・画面の遷移） |
| --- | --- |
| 設定する | その場。フォームの代わりに、結果の `StatusMessage` と秘密。秘密はキャッシュにも保存にも置かず、ページを離れると消える |
| 秘密をコピーする | トースト |
| サーバーが拒否（422 `invalid-webhook`） | その場。`reason` の項目（URL・説明・イベント）にサーバーの `detail` を出し、`missing_permission` では足りない権限も並べて、入力を残す |

### 6. 入口と出口

- 入口: 一覧の「Webhook を追加」。
- 出口: 「Webhook を開く」で webhook のページ、「webhook の一覧へ戻る」。
