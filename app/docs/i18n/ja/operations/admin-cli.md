<!-- source: administrators-and-roles 1baeba6d67b4 -->
# 管理者とロール

最初の管理者の作り方、コマンドラインからのロールの管理、サービス終了の告知と実施、アップロードされた画像の再処理、検索インデックスの再構築、メンテナンスページの書き出しについて説明します。

<!-- source: prerequisites b7ae62128809 -->
## 前提条件

- 設定（`host`、`database`、`database_url`、`kek` など）が、[設定リファレンス](../reference/configuration.md)の説明どおりに設定されていること。`my config check` で確認できます。
- データベースにスキーマが適用されていること（`my schema apply`。[database.md](database.md) を参照してください）。
- `host` と `account_domain` が確定していること。データベースを使う最初のコマンドがこれらを記録し、以降のコマンドは設定が異なると実行を拒否します（[あとから変えられない設定](production-checklist.md#settings-you-cannot-change-later)を参照してください）。

<!-- source: creating-an-administrator e5fec2e3d287 -->
## 管理者を作る

```sh
my admin create -username alice -email alice@example.com
```

- 端末から実行すると、コマンドはパスワードを 2 回、画面に表示せずに尋ねます。
- 標準入力が端末でないときは、標準入力の 1 行目がパスワードになります（たとえば `my admin create ... < password.txt`）。
  パスワードはコマンドライン引数や環境変数では渡せません。これらはプロセス一覧やシェルの履歴に残るためです。
- [Cloud Run ジョブ](deploy-cloud-run.md#9-create-the-first-administrator)のように、単発のコマンドに端末も標準入力もない環境では、`-password-link` を付けます。このときコマンドはパスワードを読まず、誰も知らないランダムなパスワードを設定し、`created admin` の行のあとにリンクを出力します。このリンクは `my admin account password-reset-link` と同じように、1 時間のあいだ 1 回だけパスワードを設定できます（監査ログには `password_reset_link_issued` として記録されます）。
  コマンドの出力を読める人なら誰でもこのリンクを使えます。そうしたプラットフォームでは、リンクが使われるか期限が切れるまで、その出力がプラットフォームのログに残ります。リンクはすぐに開いてください。期限が切れたら、`my admin account password-reset-link @alice` でもう一度発行します。リンクの発行に失敗した場合、管理者はすでに作られています。コマンドはそのことを伝えて終了コード 1 で終了し、同じコマンドで新しいリンクを得られます。
- パスワードは 8 文字以上 128 文字以下です。出力、ログ、エラーメッセージには決して表示されません。
- 1 つのトランザクションで、ユーザー、パスワード、メインのメールアドレス、ローカルアカウント（鍵を含む）、そして `admin` ロールの付与（付与者なし。これは CLI を意味します）を作ります。
  組み込みのロールがなければ、先にそれらを作ります。
- 成功すると、コマンドは `created admin @alice@<account_domain> (user <id>, account <id>)` を出力し、終了コード 0 で終了します。
- [一人用のサーバー](instance-settings.md#one-person-server)では、コマンドはアカウントを作らず、終了コード 1 で終了します。
  [ログインをログインプロバイダーに限定している](login-providers.md#limit-sign-in-to-login-providers)あいだは、その限定が終わるまでパスワードではログインできないことも警告します。

<!-- source: running-it-again 197692f7f103 -->
## もう一度実行する

- ユーザー名またはメールアドレス（どちらも大文字と小文字を区別しません）がすでに使われているときは、コマンドは何も変えず、どちらが使われているかを出力して、終了コード 1 で終了します。
  既存のユーザーのパスワードやロールは決して変えません。
- 失敗した実行は何も書き込まないので、原因を直して同じコマンドをもう一度実行できます。
- 終了コード 2 は使い方の誤りを意味します。

<!-- source: granting-and-revoking-roles b87e1adb131e -->
## ロールの付与と取り消し

ロールは権限を持ちます。組み込みのロールは `admin`（`position` 1000、すべての権限）と
`moderator`（`position` 100、`manage_reports`、`manage_accounts`、`view_audit`）です。
`my serve` は起動するたびに（どの `role` のプロセスでも）組み込みのロールを確認し、`my admin role` も実行するたびに確認します。
どちらも足りないものを作り、`admin` に実行中のバージョンのすべての権限を与えます。
アップグレードで権限が増えても、手作業は必要ありません。`moderator` は、管理者が設定した権限と position を保ちます。

```sh
my admin role list                 # each role's position, permissions (grouped as moderation, server administration, and member features), and holders
my admin role list -user alice     # the roles of alice
my admin role grant alice moderator
my admin role revoke alice moderator
```

- 権限は 3 つのグループのいずれかに属します。
  - モデレーション: `manage_reports`、`manage_accounts`、`manage_domains`、`manage_invites`、`view_audit`。
  - サーバーの管理: `manage_settings`、`manage_rules`、`manage_roles`、`manage_jobs`、`manage_login_recovery`、`manage_custom_emojis`、`manage_webhooks`、`manage_announcements`、`manage_finance`、`manage_supporters`、`manage_oauth_apps`、`manage_service_closure`、`manage_errors`、`manage_login_providers`。
  - メンバーの機能: `view_finance`、`create_custom_emojis`、`start_voice_rooms`、`create_invites`、`keep_posts_longer`。

  `list` はグループごとに 1 列を出力します（`MODERATION`、`SERVER`、`MEMBER`）。Web クライアントは最初の 2 つのグループを「モデレーション」と「管理」として表示し、メンバーの機能はどちらにも数えません（[moderation.md](moderation.md) を参照してください）。
- 変更は、そのユーザーの次のリクエストから有効になります。モデレーター向けの通知は、通知が作られた時点でその権限を持つ人に届きます。
- 付与と取り消しは、監査ログに `role_granted` と `role_revoked`（対象の種類は `role`）として記録されます。CLI による操作の実行者は `cli` です（`my admin audit list -target-kind role`）。
- ユーザーがすでに持っているロールの付与や、持っていないロールの取り消しは、何も変えず、何も記録しません。
- `admin` の最後の保持者からは、そのロールを外せません（終了コード 1）。先にほかのユーザーに `admin` を付与してください。
- 管理 API は `GET /api/v1/admin/roles`、`GET /api/v1/admin/accounts/{id}/roles`、`PUT` / `DELETE /api/v1/admin/accounts/{id}/roles/{role}` で、権限 `manage_roles` が必要です。
  API では、自分のロールのうち最も高い `position` より上のロールは付与も取り消しもできません（403）。最後の `admin` を取り消すと 409 が返ります。
- スタッフは自分より下の階級にだけ操作できます。API では、モデレーションの措置とその解除、異議申し立ての判断、メール送信停止の解除、アカウントの非公開データの表示、パスワードリセットリンクの発行、二要素認証の解除には、自分のロールのうち最も高い `position` が、対象のすべてのロールのものより上である必要があります。同じかそれより高い階級（自分のアカウントを含む）に対しては 403 が返ります。モデレーターは管理者に対して操作できず、管理者どうしも互いに操作できません。これらのコマンド（`my admin account ...`）にはこの制限がないので、管理者どうしの意見が合わないときはこちらを使ってください。
- 唯一の管理者を締め出したり取り除いたりはできません。その管理者への `disable_login`、`suspend`、`ban` は、API（409）でもこれらのコマンド（終了コード 1）でも拒否され、その管理者は自分のアカウントを削除できません（409）。`admin` のほかの保持者は、ログインできるあいだだけ数えられます。アカウントを削除したあとや、凍結、BAN、`disable_login` の状態にあるあいだは数えられません。

<!-- source: announcing-the-end-of-the-service e1c40ec145f0 -->
## サービス終了の告知

サーバーを閉じるときは、まず終了を告知します。告知からデータの破棄までの手順全体は [service-closure.md](service-closure.md) で説明しています。
告知のあいだも、サーバーはふだんどおりに動きます。`GET /api/v1/instance` の `closure` と NodeInfo の `metadata.serviceClosure` が、状態と日付を示します。

```sh
my admin closure announce -closes-at 2027-01-31T15:00:00Z -message "Thank you for using this server."
my admin closure announce -closes-at 2027-02-28 -export-until 2027-04-30   # change the dates (a bare date means 00:00 UTC)
my admin closure status                                                   # the state and the schedule in effect
my admin closure withdraw                                                 # withdraw the announcement
```

- `-closes-at` に下限はありません。`-export-until`（データを持ち出せる期限）は `-closes-at` と同じかそれより後でなければならず、既定では終了日の 30 日後です。
- 終了日まで 30 日を切っているときは、コマンドは標準エラーに警告を出力します（告知は記録されます）。
- 日付の変更は新しい告知として記録され、取り下げのあとに行った告知も新しい告知になります。サーバーを閉じたあとは、告知も取り下げもできません（終了コード 1）。
- 告知、日付の変更、取り下げは、監査ログに `service_closure_announced` と `service_closure_withdrawn`（対象の種類は `instance`。`my admin audit list -target-kind instance` を参照してください）として記録されます。
- 管理 API は `GET /api/v1/admin/closure`、`PUT` / `DELETE /api/v1/admin/closure/announcement`、`GET /api/v1/admin/closure/check`、`POST /api/v1/admin/closure/close` です。権限 `manage_service_closure` が必要です（組み込みのロールでは `admin` だけが持っています）。
- 告知のあいだ、サーバーは新しいユーザーも新しい支払いも受け付けません。
  - 実際の登録モードは `closed` になります（`registration_mode` の値は変わらず、告知を取り下げると再び有効になります）。登録も招待による登録もできず、保留中の申請は承認できません（`409`）。
  - 告知すると、保留中の申請はすべて却下され（`service_closure` による判断として記録されます）、実行中のサポーターのサブスクリプションはすべて期間の終わりに解約されます。
    購読者には通知（`supporter_service_closure`）とメールが届きます。新しいサブスクリプションには `409` が返ります。告知を取り下げても、却下と解約は元に戻りません。
  - 告知、日付の変更、取り下げのたびに、すべてのローカルユーザーに通知 `service_closure` が送られます。これらはすべてジョブとして実行されるので、ワーカーを止めないでください。
  - 確認済みのメールアドレスを持つすべてのユーザーに、告知、日付の変更、取り下げ、終了日の 7 日前と 1 日前、終了、そして持ち出し期限の 7 日前にメールが届きます。ユーザーはこれを止められません（[email.md](email.md) を参照してください）。
  - 終了日を過ぎると、`manage_service_closure` を持つ人に通知 `admin_service_closure_due` が届きます。サーバーが自分で閉じることはありません。

終了日を過ぎたら、サーバーを閉じます。閉じたら元に戻せません。

```sh
my admin closure check                                      # review what the closure ends
my admin closure close                                      # show the review, then ask for y and the domain name
my admin closure close -confirm-domain social.example       # for automation: pass the domain name instead of answering
```

- `check` と `close` は、ローカルアカウントの数、配送先の inbox の数（既知の共有 inbox）、送る `Delete` アクティビティの見積もり（その積）、実行中のサポーターのサブスクリプションの数、
  完了していない移行（`Move`）の数、保存期間の延長（`retention_holds`）の対象になっているアカウントの数を表示します。
  移行が残っているときや保存期間の延長があるときは、コマンドは標準エラーに警告を出します。延長の対象になっているアカウントの記録は、データを破棄する前に `my admin account export-moderation` で書き出して保管してください。
- 告知がないときや終了日の前には、`close` は尋ねずに拒否します（終了コード 1）。`y` 以外の答えや、サーバーのドメイン（設定の `host`。大文字と小文字は区別しません）と異なる入力では、何も変えずに終了コード 1 で終了します。
- 終了は、監査ログに `service_closure_closed` として記録されます。同じトランザクションで、実行中のサポーターのサブスクリプションをすべて終了し（理由は `service_closed`）、支払いプロバイダーでのサブスクリプションをすぐに解約するジョブ（`CancelNow`）をキューに入れます。
- 終了後、クライアント API が通すのは次のリクエストだけです。自分のアカウントと設定の読み取り、エクスポート（`/exports`、`/csv_exports`）、セッション（`/sessions`、`/authorized_apps`）、`GET /instance`、そして終了とジョブキューの管理です。
  それ以外はすべて `403`（`/api/v1/problems/service-closed`）で拒否されます。ストリームは止まり、Web Push は何も送りません。ログインとパスワードのリセットはそのまま使えます。
- 終了後、ワーカーはすべてのローカルアカウントのアクターの `Delete` を、連合先に 100 件ずつ配送します（移行したアカウントは最後です）。`federation_mode` が `off` や `inbound` でも送り、停止中のドメインには決して送りません。
  `my admin closure status`（と `GET /api/v1/admin/closure` の `progress`）は、`Delete` が記録されたアカウントの数、まだ展開されていない配送の数、状態ごとの配送の数（`pending`、`succeeded`、`permanent_failure`、`skipped`、`dead`）を表示します。配送のあいだはワーカーを動かし続けてください。
  すべての配送が終わると、10 分ごとに実行される確認が `delivery_completed`（監査ログでは `service_closure_delivery_completed`）を記録し、以降ローカルのアクターは `410` を返します。
  サーバーは `dead` の配送を自分ではやり直しません。`my job retry` でやり直してください。

持ち出し期限を過ぎたら、サーバーを止めたあとにドメインで配信する静的な終了ページを書き出します。このコマンドはデータベースを読むので、データベースを破棄する前に実行してください。

```sh
my admin closure export-html -out ./closure              # every locale (the instance's default locale first)
my admin closure export-html -out ./closure -lang ja,en  # choose the locales and the order in index.html
```

- 出力: ロケールごとの `<dir>/<locale>.html` と、すべてのロケールを順に並べた `<dir>/index.html`。ページは `my maintenance export` と同じテンプレートを使い、サーバー名、終了日、告知での運営者のメッセージ、`status_page_url` を表示します。
  出力には、フロントエンドの設定例 `nginx.conf`、`Caddyfile`、`cloudfront-function.js` も含まれます。これらは `/ap/*`、`/inbox`、WebFinger、host-meta、NodeInfo、`/@*` に `410` を返し、それ以外には終了ページを配信します。
- サーバーが閉じていないと、コマンドは拒否します（終了コード 1）。`delivery_completed` の前には警告を出したうえでページを書き出します。フロントエンドを静的なページに切り替える方法は [service-closure.md](service-closure.md) で説明しています。

<!-- source: processing-uploaded-images-again 65a9e3da0bd8 -->
## アップロードされた画像の再処理

アップロードされた画像は、それぞれ 3 つのサイズを持ち、アニメーションは動くバージョンも持ちます。`my` の新しいリリースでこれらのサイズや形式が変わることがあり、既存の画像は再処理するまで今のファイルのままです。

```sh
my admin media reprocess -before 2027-01-31T09:00:00Z
```

- `-before` には、アップグレードした時刻など、変更の時刻を RFC 3339 で指定します。その時刻より前に最後に処理された画像だけが対象になり、まだ来ていない時刻は拒否されます。
- 処理はワーカーが行い、1 分に 100 枚ずつ、1 枚ごとに別のジョブで処理します（[ジョブのリファレンス](../reference/jobs.md)の `media.reprocess_walk` と `media.reprocess`）。ワーカーを動かしておいてください。管理画面のジョブのページ（`/admin/jobs`）でキューを確認できます。
- 同じ時刻でもう一度実行しても安全です。処理済みの画像はそのままで、最初の実行で届かなかった画像が処理されます。
- 元のファイルは保存されていないので、各画像は大きいサイズのバージョンから作り直されます。大きいサイズで失われた細部は戻らず、より後の時刻で実行するたびに画像はもう一度エンコードされ、少しずつ画質が落ちます。変更ごとに 1 回だけ実行してください。
- 動画と音声のファイルは再エンコードされず、そのままです。今の制限では受け付けられない画像は今のバージョンのままになり、ワーカーが警告をログに出します。

画像の新しいバージョンには新しい URL が付きます。古いファイルは `media_replaced_retention`（既定では 30 日、[設定リファレンス](../reference/configuration.md#media_replaced_retention)）のあいだ残るので、開いているページやキャッシュは動き続けます。その後、1 時間ごとのジョブ `media.purge_replaced` が古いファイルを削除します。ほかのサーバーには新しい URL は伝えられません。古い URL を持ったまま自分のコピーを持たないサーバーでは、その時間を過ぎると画像が表示されなくなります。古いファイルが消えるまでは、そのぶんのストレージを見込んでおいてください。

<!-- source: rebuilding-the-search-index 9061821656ac -->
## 検索インデックスの再構築

`my search` は、検索エンジンのあるサーバーで、投稿の全文検索のインデックスを扱います。`search = none` のときは、どのコマンドもそのことを伝え、何もせずに終了コード 0 で終了します。

```sh
my search rebuild              # empty the index, then have the worker add every post that qualifies
my search rebuild --keep       # add them without emptying the index
my search rebuild --rate 50    # at most 50 posts a second (20 by default, 1000 at most)
my search clear                # empty the index and end any rebuild
my search status               # the backend and the progress and failures of the latest rebuild
```

止まった再構築を再開するには、`my search rebuild` をもう一度実行します。最後に処理した投稿から続け、インデックスをもう一度空にすることはありません。再構築が終わっていないあいだは、もう一方のモードは終了コード 1 で拒否されます。それぞれをいつ使うかは[投稿の全文検索](search.md)で説明しています。

<!-- source: exporting-the-maintenance-page 1756fcf1e02e -->
## メンテナンスページの書き出し

`my maintenance export` は、メンテナンスモードのページを静的な HTML として書き出します。
`my serve` 自体が動けないあいだ（イメージの入れ替え中や、起動しないリビジョンなど）に、ロードバランサー、CDN、リバースプロキシが配信するためのものです。
メンテナンスモードと同じテンプレートを使うので、どちらも同じ文言と見た目になります。

```sh
my maintenance export -out ./maintenance -lang en,ja            # reads the database for at most 5 seconds
my maintenance export -out ./maintenance -lang en,ja -offline   # the configuration and the default theme only
```

- 出力: `-lang` のロケールごとの `<dir>/<locale>.html`（`Accept-Language` でページを選ぶフロントエンド向け）と、
  すべてのロケールを順に並べた `<dir>/index.html`（それができないフロントエンド向け）。
  ページはインラインの CSS を持ち、スクリプトも外部リソースも持ちません。Content-Security-Policy は `<meta>` 要素にあります。
- 文言: 設定 `maintenance_server_name`、`maintenance_message`、`maintenance_until`、`status_page_url`
  （[設定リファレンス](../reference/configuration.md)）。
- `-offline` を付けないと、インスタンスの名前（`maintenance_server_name` が未設定のとき）、既定のロケール（`-lang` を指定しないとき）、
  テーマのトークンの上書きも読みます。テーマのカスタム CSS は決して含まれません。データベースを読めないときは、
  警告を出し、設定と既定のテーマからページを書き出して、0 で終了します。
- フロントエンドのページを最新に保つため、これらの設定やテーマを変えたときと、メンテナンスの前には毎回もう一度実行してください。
