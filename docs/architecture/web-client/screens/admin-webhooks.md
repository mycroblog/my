---
start: "/admin"
persona: "hana"
paths: ["app/web/src/routes/admin.webhooks.tsx", "app/web/src/features/admin/components/WebhookSettings.tsx", "app/web/src/features/admin/hooks/useWebhooks.ts", "app/web/src/features/admin/api/webhooks.ts"]
seed: [{"api": "hana", "method": "POST", "path": "/admin/webhooks", "body": {"url": "https://hooks.example/in", "description": "Moderation chat", "events": ["report.created"]}}]
---

## Webhook（`/admin/webhooks`）

`manage_webhooks` を持つ管理者が、このサーバーの webhook を一覧で見る画面（[ADR-0060](../../../adr/0060-instance-webhooks.md)、FR-722、[§14](../14-admin.md)）。
設定のフォームは[追加のページ](admin-webhooks-new.md)、1 つの webhook の編集・操作・送信履歴は [webhook のページ](admin-webhook.md)に分ける。

### 1. 使う人と頻度

- 管理者。外部のサービスへの連携を始めるとき、`admin_webhook_disabled` の通知でサーバーが無効にした webhook を探すとき。月に数回以下。

### 2. 作業

| 作業 | 最短の手順の数 |
| --- | --- |
| どの webhook が有効で、どれが失敗しているかを知る | 1 |

手順は、メニューで画面を開く 1 つ（状態の列に有効か無効かと、サーバーが無効にした理由、続けて失敗した回数が出る）。

### 3. 情報の優先順位とまとまり

1. 「Webhook を追加」（[追加のページ](admin-webhooks-new.md)へのリンクのボタン）。
2. このサーバーの webhook: 新しい順の表（40 行ずつのページ）。Webhook（説明、なければ URL のホスト名。webhook のページへのリンク）・状態（有効か無効か、サーバーが無効にした理由、続けて失敗した回数）・変更日時。

### 4. 主な操作と保存の仕方

- 主な操作は「Webhook を追加」（ページの遷移）。一覧では何も保存しない。

### 5. 操作ごとの結果の出し方

| 操作 | 結果（その場・トースト・画面の遷移） |
| --- | --- |
| 「Webhook を追加」 | 画面の遷移。追加のページ |
| 名前を押す | 画面の遷移。webhook のページ |

### 6. 入口と出口

- 入口: 管理のモードのメニューの「接続」のまとまり、`admin_webhook_disabled` の通知、監査ログの対象のリンク、webhook のページの「webhook の一覧へ戻る」と削除。
- 出口: 追加のページ、webhook のページ、メニューのほかの画面。
