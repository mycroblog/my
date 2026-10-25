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

- 権限はモデレーション（`manage_reports`・`manage_accounts`・`manage_domains`・`manage_invites`・`view_audit`）とサーバー管理（`manage_settings`・`manage_rules`・`manage_roles`・`manage_jobs`・`manage_login_recovery`・`manage_custom_emojis`・`manage_webhooks`・`manage_announcements`・`manage_finance`・`manage_oauth_apps`・`manage_service_closure`）と利用者の機能（`view_finance`・`create_custom_emojis`）の区分を持ち、
  `list` は区分ごとの列（`MODERATION`・`SERVER`・`MEMBER`）に出します。Web クライアントは区分ごとに「モデレーション」と「管理」を出し、利用者の機能はどちらにも数えません（[ADR-0058](../adr/0058-moderation-admin-separation.md)、[ADR-0070](../adr/0070-permission-split-emoji-finance.md)、[モデレーション](moderation.md#web-クライアントのモデレーションとサーバー管理)）。
- 変更は利用者の次の要求から効きます。モデレーター向けの通知は、通知を作るときに権限を持つ人に届きます。
- 付与と剥奪は監査ログに `role_granted`・`role_revoked`（対象の種類 `role`）で残ります。CLI の操作者は `cli` です（`my admin audit list -target-kind role`）。
- 持っているロールの付与と持っていないロールの剥奪は何も変えず、記録もしません。
- `admin` を持つ最後の 1 人からは剥奪できません（終了コード 1）。先に別の利用者に `admin` を付与します。
- 管理 API は `GET /api/v1/admin/roles`、`GET /api/v1/admin/accounts/{id}/roles`、`PUT` / `DELETE /api/v1/admin/accounts/{id}/roles/{role}` で、権限 `manage_roles` を要します。
  API では、自分のロールの最も高い `position` より上のロールは付与・剥奪できません（403）。最後の `admin` の剥奪は 409 です。

## サービスの終了の予告

サーバーを閉じるときは、まず終了を予告します（FR-808、[ADR-0088](../adr/0088-service-shutdown-modes.md)）。
予告からデータの破棄までの手順の全体は [サービスの終了](service-closure.md) にあります。
予告の間もサーバーはいつもどおり動き、`GET /api/v1/instance` の `closure` と NodeInfo の `metadata.serviceClosure` が状態と日付を示します。

```sh
my admin closure announce -closes-at 2027-01-31T15:00:00Z -message "ご利用ありがとうございました。"
my admin closure announce -closes-at 2027-02-28 -export-until 2027-04-30   # 日付の変更（日付だけなら UTC の 0 時）
my admin closure status                                                   # 状態と、効いている予定
my admin closure withdraw                                                 # 予告の取り消し
```

- `-closes-at` に下限はありません。`-export-until`（データの持ち出しの期限）は `-closes-at` 以後で、省くと閉鎖日の 30 日後です。
- 予告から閉鎖日までが 30 日より短いと、標準エラーに警告を出します（予告はそのまま記録します）。
- 日付の変更は新しい予告として記録し、取り消しの後の予告は新しい予告です。サーバーを閉じた後は、予告も取り消しもできません（終了コード 1）。
- 予告・日付の変更・取り消しは監査ログに `service_closure_announced`・`service_closure_withdrawn`（対象の種類 `instance`）で残ります（`my admin audit list -target-kind instance`）。
- 管理 API は `GET /api/v1/admin/closure`、`PUT` / `DELETE /api/v1/admin/closure/announcement`、`GET /api/v1/admin/closure/check`、`POST /api/v1/admin/closure/close` で、権限 `manage_service_closure`（組み込みのロールでは `admin` だけ）を要します。
- 予告の間は、新しい利用者と新しい支払いを受け付けません（ADR-0088 §2）。
  - 登録モードの実効値は `closed` です（`registration_mode` の値は変わらず、取り消すと元に戻ります）。登録も招待を使った登録もできず、審査待ちの申請は承認できません（`409`）。
  - 予告の開始で、審査待ちの申請をすべて却下し（決めた者は `service_closure`）、続いているサポーターの加入すべてを期間の終わりで解約して、加入者に通知（`supporter_service_closure`）とメールで知らせます。新しい加入は `409` です。取り消しても、却下と解約は戻りません。
  - 予告・日付の変更・取り消しのたびに、ローカルの全利用者に通知 `service_closure` が届きます。どれもジョブで行うので、ワーカーを止めないでください。
  - メールの確認が済んだ全利用者には、予告・日付の変更・取り消し・閉鎖日の 7 日前と前日・クローズ・持ち出しの期限の 7 日前にメールが届きます。利用者は止められません（[メールの送信](email.md#what-is-sent-and-how-to-configure-it)）。
  - 閉鎖日を過ぎると、`manage_service_closure` を持つ人に通知 `admin_service_closure_due` が届きます。サーバーは自動ではクローズしません。

閉鎖日を過ぎたら、サーバーをクローズします（FR-809）。クローズは取り消せません。

```sh
my admin closure check                                      # クローズで終わるものの点検
my admin closure close                                      # 点検を示し、y とドメイン名の入力で確かめてクローズ
my admin closure close -confirm-domain social.example       # 自動化: 質問の代わりにドメイン名を渡す
```

- `check` と `close` は、ローカルアカウント数・宛先の inbox（既知の shared inbox）の数・送る `Delete` の数の見積もり（その積）・続いているサポーターの加入の数・終わっていない移転（`Move`、[ADR-0089](../adr/0089-account-migration-move.md)）の数・保全（`retention_holds`）のあるアカウントの数を示します。
  移転が残っているときと保全があるときは、標準エラーに警告を出します。保全のあるアカウントの記録は、データを破棄する前に `my admin account export-moderation` で書き出して保管してください。
- `close` は、予告がないときと閉鎖日の前には何も聞かずに断ります（終了コード 1）。`y` 以外の答えと、サーバーのドメイン（設定の `host`、大文字と小文字は区別しない）と違う入力でも、何も変えずに終了コード 1 で終わります。
- クローズは監査ログに `service_closure_closed` で残り、同じトランザクションで、続いているサポーターの加入をすべて終え（理由 `service_closed`）、決済事業者のサブスクリプションをすぐ解約するジョブ（`CancelNow`）を積みます。
- クローズの後、client API は、自分のアカウントと設定の読み取り・エクスポート（`/exports`、`/csv_exports`）・セッション（`/authorized_apps`）・`GET /instance` と、クローズの状態とジョブのキューの管理の操作だけを通し、ほかは `403`（`/api/v1/problems/service-closed`）で断ります。ストリームは止まり、Web Push は送りません。サインインとパスワードの再設定はできます。
- クローズの後、ワーカーが全ローカルアカウントの `Delete`（Actor）を 100 件ずつ連合先へ配送します（移転済みのアカウントは最後。[federation.md §15](../architecture/federation.md#15-サービスの終了)）。`federation_mode` が `off` / `inbound` でも送り、凍結したドメインには送りません。
  `my admin closure status`（と `GET /api/v1/admin/closure` の `progress`）は、`Delete` を記録したアカウント数・展開の残り・配送の状態ごとの件数（`pending`・`succeeded`・`permanent_failure`・`skipped`・`dead`）を示します。配送の間はワーカーを止めないでください。
  すべての配送が終わると、10 分ごとの確認が `delivery_completed` を記録し（監査ログの `service_closure_delivery_completed`）、ローカルの Actor も `410` になります。`dead` の配送は自動では再実行しません（`my job retry` で再実行できます）。

持ち出しの期限を過ぎたら、サーバーを止めた後にドメインが返す静的な閉鎖のページを書き出します（ADR-0088 §6）。DB を読むので、DB を破棄する前に実行します。

```sh
my admin closure export-html -out ./closure              # 全ロケール（インスタンスの既定のロケールが先頭）
my admin closure export-html -out ./closure -lang ja,en  # ロケールと index.html の順を決める
```

- 出力: ロケールごとの `<dir>/<locale>.html` と、全ロケールを順に並べた `<dir>/index.html`（`my maintenance export` と同じテンプレート。サーバー名・閉鎖日・予告の運営者のメッセージ・`status_page_url`）、
  前段の設定例 `nginx.conf`・`Caddyfile`・`cloudfront-function.js`（`/ap/*`・`/inbox`・WebFinger・host-meta・NodeInfo・`/@*` は `410`、ほかは閉鎖のページ）。
- サーバーがクローズしていないと断ります（終了コード 1）。`delivery_completed` の前は警告を出して書き出します。前段の切り替えは [サービスの終了 §6](service-closure.md#6-静的なページへの切り替え) です。

## Exporting the maintenance page

`my maintenance export` writes the page of the maintenance mode ([ADR-0084](../adr/0084-maintenance-mode.md)) as static HTML,
for a load balancer, CDN, or reverse proxy to serve while `my serve` itself cannot run (an image being replaced, a revision that does not start).
It uses the same template as the maintenance mode, so both show the same words and look.

```sh
my maintenance export -out ./maintenance -lang en,ja            # reads the database for at most 5 seconds
my maintenance export -out ./maintenance -lang en,ja -offline   # the configuration and the default theme only
```

- Output: `<dir>/<locale>.html` for each locale of `-lang`, for front ends that choose a page by `Accept-Language`,
  and `<dir>/index.html` with every locale one after another, for those that cannot.
  The pages have inline CSS, no script, and no external resource; their Content-Security-Policy is in a `<meta>` element.
- Words: the settings `maintenance_server_name`, `maintenance_message`, `maintenance_until`, and `status_page_url`
  ([configuration reference](../reference/configuration.md)).
- Without `-offline`, it also reads the instance's name (when `maintenance_server_name` is unset), its default locale (when `-lang` is not given),
  and the token overrides of its theme. The theme's custom CSS is never included. When the database cannot be read,
  it prints a warning, writes the pages from the configuration and the default theme, and exits 0.
- Run it again whenever those settings or the theme change, and before each maintenance, so that the pages in the front end stay current.
