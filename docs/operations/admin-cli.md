# 管理者とロール

運営者向けの手順です。要件は FR-801 と FR-720（[機能要件](../requirements/01-functional.md)）、パスワードの扱いは [ADR-0019 §3](../adr/0019-security-baseline.md) です。

## 前提

- 設定（`host`・`db`・`database_url`・`kek` など）が [設定リファレンス](../reference/configuration.md) どおりに与えられている。`my config check` で確かめられる。
- DB にスキーマが適用済み（Atlas で `schema/<dialect>/` を適用する。[DB の初期化と作り直し](database.md)）。

## 作る

```sh
my admin create -username alice -email alice@example.com
```

- 端末から実行すると、パスワードをエコーなしで 2 回尋ねる。
- 標準入力が端末でなければ、標準入力の 1 行目をパスワードとして読む（例: `my admin create ... < password.txt`）。
  コマンドライン引数や環境変数では渡せない（プロセス一覧や履歴に残るため）。
- パスワードは 8〜128 文字。表示・ログ・エラーメッセージには出ない。
- 1 回のトランザクションで、ユーザー・パスワード・主メールアドレス・ローカルアカウント（鍵を含む）・`admin` ロールの付与（付与者なし = CLI）を作る。
  組み込みロールが無ければ先に作る。
- 成功すると `created admin @alice@<account_domain> (user <id>, account <id>)` を表示して終了コード 0 で終わる。

## 再実行したとき

- ユーザー名（大文字小文字を区別しない）かメールアドレス（大文字小文字を区別しない）が既に使われていれば、
  何も変更せずに終了コード 1 で終わり、どちらが使われているかを表示する。既存のユーザーのパスワードやロールは変えない。
- 失敗した実行は何も書き込まないので、原因を直してそのまま再実行してよい。
- 終了コード 2 は引数の誤り。

## ロールの付与と剥奪

権限はロールが持ちます（[data-model §4.1](../architecture/data-model/README.md)）。組み込みのロールは `admin`（`position` 1000、すべての権限）と
`moderator`（`position` 100、`manage_reports`・`manage_accounts`・`view_audit`）です（FR-720）。
`my serve` は起動のたびに（どの `role` のプロセスでも）、`my admin role` は実行のたびに組み込みロールを確かめ、無いものを作って `admin` にその版のすべての権限を持たせます。
版を上げて権限が増えても手作業は要りません。`moderator` の権限と位置は管理者が変えたまま残ります。

```sh
my admin role list                 # ロールごとの position・権限（モデレーション・サーバー管理・利用者の機能に分けて）・持っている利用者
my admin role list -user alice     # alice のロール
my admin role grant alice moderator
my admin role revoke alice moderator
```

- 権限はモデレーション（`manage_reports`・`manage_accounts`・`manage_domains`・`manage_invites`・`view_audit`）とサーバー管理（`manage_settings`・`manage_rules`・`manage_roles`・`manage_jobs`・`manage_login_recovery`・`manage_custom_emojis`・`manage_webhooks`・`manage_announcements`・`manage_finance`・`manage_oauth_apps`）と利用者の機能（`view_finance`・`create_custom_emojis`）の区分を持ち、
  `list` は区分ごとの列（`MODERATION`・`SERVER`・`MEMBER`）に出します。Web クライアントは区分ごとに「モデレーション」と「管理」を出し、利用者の機能はどちらにも数えません（[ADR-0058](../adr/0058-moderation-admin-separation.md)、[ADR-0070](../adr/0070-permission-split-emoji-finance.md)、[モデレーション](moderation.md#web-クライアントのモデレーションとサーバー管理)）。
- 変更は利用者の次の要求から効きます。モデレーター向けの通知は、通知を作るときに権限を持つ人に届きます。
- 付与と剥奪は監査ログに `role_granted`・`role_revoked`（対象の種類 `role`）で残ります。CLI の操作者は `cli` です（`my admin audit list -target-kind role`）。
- 持っているロールの付与と持っていないロールの剥奪は何も変えず、記録もしません。
- `admin` を持つ最後の 1 人からは剥奪できません（終了コード 1）。先に別の利用者に `admin` を付与します。
- 管理 API は `GET /api/v1/admin/roles`、`GET /api/v1/admin/accounts/{id}/roles`、`PUT` / `DELETE /api/v1/admin/accounts/{id}/roles/{role}` で、権限 `manage_roles` を要します。
  API では、自分のロールの最も高い `position` より上のロールは付与・剥奪できません（403）。最後の `admin` の剥奪は 409 です。
