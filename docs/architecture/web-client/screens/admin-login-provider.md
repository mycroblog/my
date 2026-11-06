---
start: "/admin"
persona: "hana"
paths: ["app/web/src/routes/admin.login-providers_.$id.tsx", "app/web/src/features/admin/components/LoginProviderSettings.tsx", "app/web/src/features/admin/components/loginProviderParts.tsx", "app/web/src/features/admin/hooks/useLoginProviders.ts", "app/web/src/features/admin/api/loginProviders.ts"]
seed: [{"api": "hana", "method": "POST", "path": "/admin/login_providers", "body": {"kind": "github", "display_name": "GitHub", "client_id": "demo-client", "client_secret": "demo-secret"}}]
---

## プロバイダーのページ（`/admin/login-providers/{id}`）

`manage_login_providers` を持つ管理者が、1 つの外部ログインのプロバイダーのコールバック URL を確かめ、設定を変え、クライアントシークレットを更新し、有効と無効を切り替え、削除するページ（[ADR-0106](../../../adr/0106-external-login-providers.md)、[§14](../14-admin.md)）。
`GET /admin/login_providers/{id}` で読む。削除した ID は 404 でページの失敗になる。

### 1. 使う人と頻度

- 管理者。設定した直後（コールバック URL を IdP に登録する）、IdP でシークレットを作り直したとき、プロバイダーをやめるとき。年に数回。

### 2. 作業

| 作業 | 最短の手順の数 |
| --- | --- |
| GitHub のコールバック URL をコピーする | 3 |
| GitHub で新規登録を許可する | 4 |
| GitHub のログインプロバイダーを無効にする | 4 |
| GitHub のログインプロバイダーを削除する | 4 |

手順は、メニューで一覧を開く 1 つ、表示名を押す 1 つ、それから「コピー」を押す 1 つ。新規登録は「新規登録を許可する」を選ぶ 1 つと「保存」を押す 1 つ。無効と削除は、ボタンと確かめのボタンの 2 つ。

### 3. 情報の優先順位とまとまり

1. 見出し（`h1`「ログインプロバイダー」）と「ログインプロバイダーの一覧へ戻る」。
2. プロバイダー（区画の見出しは表示名、`dir="auto"`）: 種類・Issuer かサーバー・状態と変更日時・新規登録・ひも付けた利用者の数・設定日時の説明リストと、コールバック URL（`dir="ltr"`）を IdP に登録することの説明とコピーのボタン。
3. 設定: 種類は変えられないことの説明と、設定のフォーム（追加のページと同じ項目。シークレットの欄は持たない）。
4. シークレットと状態: 「シークレットを更新」（新しいシークレットを入れるダイアログ）、「無効にする」か「有効にする」、「削除」。

### 4. 主な操作と保存の仕方

- 主な操作は設定の「保存」。設定はまとめて保存（`FormActionBar`）。
- 無効にすることと削除は `ConfirmDialog` で確かめ、ひも付けた利用者の数と、その利用者がこのプロバイダーでログインできなくなることを出す。有効にすることは、ログインプロバイダーだけのログインがそれで始まるときだけ警告で確かめる。

### 5. 操作ごとの結果の出し方

| 操作 | 結果（その場・トースト・画面の遷移） |
| --- | --- |
| コピー | トースト |
| 保存 | その場。ボタンの隣に結果（`role="status"`）。拒否はその項目に出して入力を残し、ほかの失敗はボタンの隣（`role="alert"`） |
| シークレットを更新 | その場。ダイアログを閉じ、区画の `StatusMessage` に結果。拒否はダイアログの欄 |
| 無効にする・有効にする | その場。ボタンが切り替わり、状態の説明が変わる。区画の `StatusMessage`、失敗はダイアログの中か同じ場所（`role="alert"`） |
| 削除 | 画面の遷移とトースト。確かめの後、一覧へ移り、削除したことをトーストで知らせる。失敗はダイアログの中 |

### 6. 入口と出口

- 入口: 一覧の表示名、追加のページの「設定する」。
- 出口: 「ログインプロバイダーの一覧へ戻る」、削除の後の一覧。
