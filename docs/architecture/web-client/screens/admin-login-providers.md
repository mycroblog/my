---
start: "/admin"
persona: "hana"
paths: ["app/web/src/routes/admin.login-providers.tsx", "app/web/src/features/admin/components/LoginProviderSettings.tsx", "app/web/src/features/admin/hooks/useLoginProviders.ts", "app/web/src/features/admin/api/loginProviders.ts"]
seed: [{"api": "hana", "method": "POST", "path": "/admin/login_providers", "body": {"kind": "github", "display_name": "GitHub", "client_id": "demo-client", "client_secret": "demo-secret"}}]
---

## ログインプロバイダー（`/admin/login-providers`）

`manage_login_providers` を持つ管理者が、このサーバーの外部ログインのプロバイダーを一覧で見る画面（[ADR-0106](../../../adr/0106-external-login-providers.md)、[§14](../14-admin.md)）。
設定のフォームは[追加のページ](admin-login-providers-new.md)、1 つのプロバイダーの編集・シークレットの更新・有効と無効・削除は[プロバイダーのページ](admin-login-provider.md)に分ける。

### 1. 使う人と頻度

- 管理者。外部ログインを始めるときと、プロバイダーの状態を確かめるとき。年に数回。

### 2. 作業

| 作業 | 最短の手順の数 |
| --- | --- |
| どのログインプロバイダーが有効で、新規登録を許すかを知る | 1 |

手順は、メニューで画面を開く 1 つ（状態の列に有効か無効かと、新規登録を許すかが出る）。

### 3. 情報の優先順位とまとまり

1. 「プロバイダーを追加」（[追加のページ](admin-login-providers-new.md)へのリンクのボタン）。
2. このサーバーのログインプロバイダー: 新しい順の表（40 行ずつのページ）。プロバイダー（表示名、`dir="auto"`、プロバイダーのページへのリンク）・種類・状態（有効か無効かと、新規登録を許すか）・設定日時。

### 4. 主な操作と保存の仕方

- 主な操作は「プロバイダーを追加」（ページの遷移）。一覧では何も保存しない。

### 5. 操作ごとの結果の出し方

| 操作 | 結果（その場・トースト・画面の遷移） |
| --- | --- |
| 「プロバイダーを追加」 | 画面の遷移。追加のページ |
| 表示名を押す | 画面の遷移。プロバイダーのページ |

### 6. 入口と出口

- 入口: 管理のモードのメニューの「接続」のまとまり、監査ログの対象のリンク、プロバイダーのページの「ログインプロバイダーの一覧へ戻る」と削除。
- 出口: 追加のページ、プロバイダーのページ、メニューのほかの画面。
