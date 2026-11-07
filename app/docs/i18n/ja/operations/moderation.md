<!-- source: account-and-domain-actions-and-handling-reports 59d39b2e3f37 -->
# アカウントとドメインへの処分、通報の対応

運営者とモデレーターが、アカウントやドメインに処分を行い、異議申し立てを審査し、通報に対応し、監査ログを読む方法を説明します。
アカウントの管理（管理 API のみ）を除き、ここで説明することはすべて、管理 API（`/api/v1/admin/...`、スコープ `admin:read` / `admin:write`）と CLI（`my admin account action` / `my admin domain` / `my admin report` / `my admin appeal`）で行えます。
管理 API にはロールの権限が必要です。アカウントの管理、処分、異議申し立てには `manage_accounts`、ドメインへの処分には `manage_domains`、通報には `manage_reports` が必要です。
CLI はサーバーと同じ設定（`-config` または環境変数）で DB に直接接続し、どのユーザーが操作したかは記録しません（`issued_by_user_id` などのフィールドは NULL になり、[監査ログ](#audit-log)の操作者は `cli` になります）。

<!-- source: from-a-report-to-an-action-and-an-appeal dabd0eabc7c7 -->
## 通報から処分、異議申し立てまで

![通報の流れ。通報が届き、モデレーターが確認し、処分と利用者へのお知らせが続くことがあり、利用者は異議を申し立てることができます。右の列は、各段階でほかのサーバーへ送るものを示しています。下の一覧が各段階を説明しています。](images/moderation-lifecycle.svg)

図の上から順に説明します。

1. **通報が届く。** ローカルの利用者が通報するか、ほかのサーバーが `Flag` を送ってくるか（そのドメインに
   `reject_reports` があると破棄されます。[ドメインへの処分](#domain-actions)を参照してください）、[判定フック](decision-hooks.md)が `flag` と答えます
   （[外部の審査が出した通報](#reports-that-an-external-review-filed)）。ほかのサーバーのアカウントを通報する
   ローカルの利用者は、通報を転送するかどうかを選べます。転送すると、そのサーバーにすぐ `Flag` が送られます。
2. **確認。** 通報は `open` になり、モデレーターに `admin_report` の通知が届きます。解決すると、結果が `actioned` または `dismissed` の
   `resolved` になり、ローカルの通報者に `report_resolved` の通知が届きます。
   再開すると `open` に戻ります。ほかのサーバーのアカウントについての通報を転送すると、そのサーバーに `Flag` が送られます
   （[通報](#reports)）。
3. **処分。** モデレーターが通報から[アカウントへの処分](#account-actions)を行うと、その通報は
   `actioned` として解決されます。処分は通報なしで行うこともできます。処分には種類、理由、終了日時があり、
   解除されるか期限が切れると終わります。`notify_target` を付けると、利用者に `moderation_notice` の通知が届きます
   （[利用者に見えるもの](#what-the-user-sees)）。ほかのサーバーには、`delete_statuses` では投稿の `Delete`、
   ローカルのアカウントの `suspend` や `ban` が始まるときと終わるときには Actor の `Update`、リモートのアカウントの `suspend` が断ち切ったフォローについては
   `Reject` / `Undo` が届きます。
4. **異議申し立て。** ローカルの利用者が処分に異議を申し立てます。異議申し立ては `pending` になり、モデレーターに `admin_appeal` の
   通知が届きます。審査の結果は `upheld` または `overturned` で、利用者に `appeal_decided` の通知が届きます。
   処分を取り消すと、その処分は解除され、`ban` による削除も取り消されます。そのため、ローカルの
   アカウントの `suspend` や `ban` を取り消すと、Actor の `Update` が送られます（[異議申し立て](#appeals)）。

担当の割り当て、メモ、お知らせ、異議申し立てはこのサーバーにとどまります。サーバーの外に出るのは右の列のアクティビティだけです。

<!-- source: moderation-and-server-administration-in-the-web-client 3beb1c9760a0 -->
## Web クライアントでのモデレーションとサーバー管理

Web クライアントでは、スタッフ向けの画面が 2 つに分かれています。
ナビゲーションの「モデレーション」と「管理」は、ロールがそのグループの権限を 1 つ以上持っている人にだけ表示されます。どの画面も、権限のない人には理由だけを表示し、サーバーもその人のリクエストを拒否します。

| グループ | 権限 | 画面 |
| --- | --- | --- |
| モデレーション（`/moderation`） | `manage_reports`、`manage_accounts`、`manage_domains`、`manage_invites`、`view_audit` | 通報（`/moderation/reports`）、異議申し立て（`/moderation/appeals`）、アカウント（`/moderation/accounts`）、登録の審査（`/moderation/registrations`）、登録の拒否リスト（`/moderation/signup-blocks`）、招待（`/moderation/invites`）、ドメインへの処分（`/moderation/domain-actions`）、連合先のサーバー（`/moderation/instances`）、監査ログ（`/moderation/audit-log`） |
| サーバー管理（`/admin`） | `manage_settings`、`manage_rules`、`manage_roles`、`manage_jobs`、`manage_login_recovery`、`manage_custom_emojis`、`manage_webhooks`、`manage_announcements`、`manage_finance`、`manage_oauth_apps`、`manage_service_closure`、`manage_errors`、`manage_login_providers` | サーバーの設定（`/admin/settings`）、ルール（`/admin/rules`）、規約とポリシー（`/admin/documents`）、ジョブキューと失敗したジョブ（`/admin/jobs`）、カスタム絵文字（`/admin/custom-emojis`）、お知らせ（`/admin/announcements`）、Webhook（`/admin/webhooks`）、費用と請求の取り込み（`/admin/finance`、`manage_finance`）。`manage_login_recovery`（ログインの復旧: 二要素認証の解除とパスワードのリセット用リンクの発行）と `manage_roles`（アカウントへのロールの付与と取り消し）は、アカウントの「管理」ページ（`/moderation/accounts/{id}/manage`。`manage_accounts` も必要です）のセクションとして表示されます |

- 「モデレーション」のバッジは、未解決の通報、承認待ちの登録、審査待ちの異議申し立ての合計です（[モデレーター向けの通知と未処理の件数](#notifications-for-moderators-and-the-pending-counts)）。「管理」には件数を表示しません。
- どちらのグループも概要から始まり、各画面はその横のメニューにあります。モデレーションの概要には、あなたのロールが数えるキュー（未解決の通報、承認待ちの登録、審査待ちの異議申し立て）ごとに、そのキューへのリンクになったタイルと、最も古い未解決の通報の待ち時間を示すタイル（古い順の通報の一覧へのリンク）があり、キュー全体から先に対応すべき未解決の通報を一覧にします。違法なコンテンツが先で、次に待ち時間の長いものが並び、それぞれの待ち時間と担当者が表示されます。
  通報の一覧は、状態、カテゴリー、担当者（自分または未割り当て）で絞り込め、新しい順か古い順に並べられます。絞り込みはページのアドレスに残ります。
  管理の概要にはサーバーの健康状態が表示されます。失敗したジョブ、ジョブキューで最も古いジョブが待っている時間、未解決のエラーで、各タイルはその画面へのリンクになり、対応が必要なときは目印が付きます。
- 組み込みの `moderator` が持つのは `manage_reports`、`manage_accounts`、`view_audit` だけで、「管理」は表示されません。「ドメインへの処分」と「連合先のサーバー」の画面（`manage_domains`）も表示されないため、
  これらを任せるには、その権限を持つロールを作って付与してください（[ロールの付与と取り消し](admin-cli.md#granting-and-revoking-roles)）。
- 管理 API のパスはグループにかかわらず `/api/v1/admin/...` で、スコープも同じ `admin:read` / `admin:write` の組です。API リファレンスでは、グループはタグ（`admin-moderation`、`admin-server`）で分かれています。
- 権限には 3 つ目のグループ「利用者の機能」（`view_finance`、`create_custom_emojis`、`start_voice_rooms`、`create_invites`、`keep_posts_longer`）があり、スタッフ向けの画面の外にあります。これらの権限だけを持つ人には「モデレーション」も「管理」も表示されず、
  `view_finance` を持つ人のナビゲーションには、設定のあとに「会計」（`/finance`）が表示されます。
  その API は利用者向けの名前空間にあり（`GET /api/v1/finance/cost_totals` と `GET /api/v1/finance/income_totals`、スコープ `read:finance`）、`admin:*` のスコープは必要ありません。表示する内容は[インスタンス設定](instance-settings.md)の `finance_viewer_*` で決まります。

<!-- source: account-actions a61dac7c5921 -->
## アカウントへの処分

処分は、それを記録するトランザクションの中で制限のプロジェクション（`account_restrictions`）に反映され、次のリクエストから効果を持ちます。

| 種類 | ローカルのアカウントへの効果 |
| --- | --- |
| `warn` | 制限なし（利用者に知らせるだけ） |
| `force_sensitive` | すべての添付をセンシティブとして表示する |
| `limit` | 投稿（過去のものも含む）をフォロワー限定として扱い、公開のタイムラインに出さない |
| `restrict_posting` | 投稿、返信、ブースト、編集を禁止する |
| `restrict_interaction` | フォロー、お気に入り、ブースト、返信を禁止する |
| `disable_login` | アプリと API の利用を禁止し、有効なセッションとトークンを無効にする（サインインページでは処分を確認できます） |
| `suspend` | 上のすべての制限を適用し、プロフィールと投稿を本人以外の全員から隠す |
| `ban` | `suspend` と同じ制限を適用し、30 日後にアカウントを削除する。解除はできない（その 30 日の間に行われた異議申し立てが認められて取り消されたときだけ取り消される） |
| `delete_statuses` | 指定した投稿を削除する。本人にも見えなくなり、本人が削除したときと同じ `Delete` をほかのサーバーへ送る |
| `mark_statuses_sensitive` | 指定した投稿の添付をセンシティブとして表示する |

リモートのアカウントに行えるのは `limit`、`suspend`、`force_sensitive` だけです（ほかの種類は、管理 API では `422` になり、CLI では失敗します）。
効果はこのサーバーの中だけです。相手のサーバーに送られるのは、`suspend` が断ち切ったフォローの `Reject` / `Undo` だけです。

| 種類 | リモートのアカウントへの効果 |
| --- | --- |
| `force_sensitive` | すべての添付をセンシティブとして表示する |
| `limit` | 投稿（過去のものも含む）をフォロワー限定として扱い、公開のタイムラインに出さず、そのアカウントをフォローしていない利用者への通知を作らない |
| `suspend` | そのアカウントから受け取るものをすべて破棄し、プロフィールと投稿を隠す。そのアカウントとローカルの利用者の間のフォローを双方向とも断ち切り（フォローリクエストも含む）、相手のサーバーに、そのアカウントのフォローの `Reject` と利用者のフォローの `Undo` を送る。解除すると表示が戻り、そのアカウントからの受信も再開する（凍結中に破棄されたものと断ち切られたフォローは戻らない） |

- `notify_target`（`-no-notify`）は、リモートのアカウントでは意味を持ちません（通知が届かないため）。
- `suspend` によってフォローを失ったローカルの利用者には、失ったフォロワーとフォローの数を伝える通知（`relationships_severed`）が 1 件届きます（[断ち切られたフォロー](#severed-follows)）。
- 処分と解除は、ローカルのアカウントへの処分と同じく[監査ログ](#audit-log)に記録されます。

```sh
my admin account action issue -kind suspend -category spam -public-reason "spam" -duration 720h @alice
my admin account action issue -kind limit -report <report id> @bob@remote.example
my admin account action issue -kind delete_statuses -status <status id> -status <status id> @alice
my admin account action issue -kind ban -category legal -public-reason "illegal content" @alice
my admin account action list -active @alice
my admin account action lift -reason "appealed" <action id>
```

管理 API では、`POST /api/v1/admin/accounts/{id}/actions`（本文には `kind`、`reason_category`、`public_reason`、`private_note`、`report_id`、`notify_target`、秒単位の `expires_in`、`status_ids` を指定します）、
`GET /api/v1/admin/accounts/{id}/actions`（`active` で絞り込み）、`GET /api/v1/admin/account_actions/{id}`、`POST .../{id}/lift`（本文 `{"reason": "..."}` は省略できます）を使います。

- CLI ではアカウントを `@username`（ローカル）、`@username@domain`（リモート）、またはアカウント ID で指定し、管理 API ではアカウント ID で指定します。
- 複数の処分は重なります（制限はそれらの和集合になります）。解除で外れるのはその処分の制限だけで、終了日時のある処分はその時刻に自動で終わります（一覧では `expired` と表示されます）。終了日時は最大で 10 年先までです（`expires_in` は 315360000 まで）。それより長いと `422` で拒否されます。
- `disable_login`、`suspend`、`ban` が無効にしたセッションとトークンは、処分を解除しても戻りません。利用者はもう一度サインインします。
- ローカルのアカウントへの `suspend` や `ban` では、凍結を示すために、プロフィールを空にした Actor の `Update` をフォロワーのサーバーと既知のサーバーへ送ります（Mastodon では「suspended」と表示されます）。
  解除と期限切れでは、元のプロフィールの `Update` を送ります。期限切れは 5 分ごとに実行されるジョブが送るため、最大で 5 分遅れることがあります。
- `delete_statuses` と `mark_statuses_sensitive` には、対象のアカウントの削除されていない投稿を 1〜100 件、`-status`（`status_ids`）で指定します。終了日時は付けられません。
  `delete_statuses` を解除しても、削除した投稿は戻りません。`mark_statuses_sensitive` を解除すると、元の表示に戻ります。
- `ban` には終了日時を付けられず、解除もできません。取り消されるのは、削除までの 30 日の間に利用者が行った異議申し立てが認められたときだけです（[異議申し立て](#appeals)）。迷うときは、終了日時のある `suspend` を使ってください。
  処分から 30 日後、1 時間ごとに実行されるジョブがアカウントを削除します（アカウントと投稿は見えなくなり、サーバーはほかのサーバーへ `Delete` を送ります）。
- `report_id`（`-report`）に指定できるのは、対象のアカウントについての通報だけです。未解決の通報は、処分とともに `actioned` として解決されます（解決は監査ログにも記録されます）。解決済みの通報は変わりません。
- `public_reason` は利用者に表示される理由で、`private_note` はモデレーターだけが見るメモです。`notify_target`（既定は true。CLI では `-no-notify` で false）で、利用者に知らせるかどうかを決めます。

<!-- source: what-the-user-sees 0ca06c59f8e1 -->
### 利用者に見えるもの

- `notify_target` が true の処分は、通知（`moderation_notice`）として利用者に届きます。通知には、種類、理由のカテゴリー、`public_reason`、終了日時、状態が含まれます。種類とカテゴリーは、利用者の UI の言語での表示名としても返されます。
- 利用者は、ファーストパーティの `GET /api/v1/moderation/actions`（`active` で絞り込み）で、自分への処分を新しい順に確認できます。`notify_target` が false の処分や、解除された処分、期限が切れた処分も一覧に含まれます。
- `private_note`、通報、処分を行った人、解除の理由は、利用者には決して表示されません。
- 利用者は処分ごとに 1 回、異議を申し立てられます（[異議申し立て](#appeals)）。処分の一覧と通知の中の処分には、異議を申し立てられるかどうか（`appealable`）、期限（`appeal_deadline`）、異議申し立てとその結果（`appeal`）が含まれます。
- `disable_login`、`suspend`、`ban` を受けた利用者は、アプリと API を使えませんが、サインインページ（`/auth/sign_in`）ではサインインできます。そこでは、効力のある処分の種類、理由、終了日時
  （`ban` ではアカウントが削除されるまでの日数）が表示され、そのページから異議を申し立てられます（[異議申し立て](#appeals)）。認可エンドポイント（`/oauth/authorize`）は `403` を返し、トークンを発行しません。
  `ban` を受けた利用者はアカウントが削除されるまでサインインでき、削除によってそのセッションも無効になります。自分で削除を予約した利用者と、無効にされたアカウントは、サインインページでもサインインできません。

<!-- source: accounts-that-their-owners-deactivated ed010f49b40a -->
### 持ち主が停止したアカウント

利用者は、パスワードか二要素認証のコードをもう一度入力するか、パスキーで確認すると、アカウントの設定から自分のアカウントを停止できます
（これらのどれも持たない利用者は、連携しているログインプロバイダーでもう一度サインインします）。これはモデレーションの処分ではなく、モデレーターが誰かのアカウントを停止することはできません。

- プロフィールと投稿はモデレーター以外の全員から隠され、ほかのサーバーには `suspended` の印を付けた Actor の
  `Update` が届くため、そのアカウントが凍結されていると表示されることがあります。利用者のセッションとトークンはすべて終わります。
  メールを送るサーバーでは、削除の日付を知らせるメールが 1 通届きます。それより前にほかのメールは送られません。
- アカウントには何も届きません。メンションとダイレクトメッセージは破棄され、通知、メール、Web Push は作られず、
  新しいフォロワーは拒否されます。お気に入りとブーストは数えられます。既存のフォローは残ります。
- 利用者はサーバーのページにサインインでき、そのページには削除の日付が表示され、アカウントを再開するか削除するかを選べます。
  再開するとすべてが元に戻ります。次に停止できるのは 24 時間後です。
- 停止から 30 日後、1 時間ごとのジョブがアカウントを削除します。その日付を過ぎたあとの再開は、ジョブがまだ実行されていなくても拒否されます。
  ユーザー名は解放されません。

モデレーターには、アカウントの一覧で `deactivated` の状態として表示され（`state=deactivated`）、そのアカウントについての通報を読んだり
処分を行ったりすることは引き続きできます。モデレーションの処分は削除の日付を変えませんが、`ban` だけは
自分の日付で置き換え、それ以降、利用者は再開できなくなります。アカウントが停止している間、凍結はほかのサーバーに何も送らず、
利用者が再開したあとも効力が続きます。モデレーションによってサインインできない間、削除が予約されている間、移行したあと、登録が審査待ちの間、
有料の申し込みが続いている間（期間の終わりに終了するよう設定したものも含みます）、またはサインインできる唯一の管理者であるときは、利用者はアカウントを停止できません。
停止した管理者は、サインインできる管理者として数えられません。ほかにサインインできる管理者がいないまま、停止した管理者の日付が過ぎると、
1 時間ごとのジョブはそのアカウントを削除せず、警告をログに出します。その管理者ももうサインインできないため、
`my admin create` で別の管理者を作ってください。次の実行でそのアカウントが削除されます。

<!-- source: appeals 8a6f1b4c288d -->
## 異議申し立て

ローカルの利用者は、自分への処分に異議を申し立てられます。
モデレーターは異議申し立てを審査し、処分を維持する（`uphold`）か、処分を取り消す（`overturn`）かを決めます。

```sh
my admin appeal list -state pending
my admin appeal uphold -reason "The posts were spam." <appeal id>
my admin appeal overturn -reason "Sorry, our mistake." <appeal id>
```

利用者は `POST /api/v1/moderation/actions/{id}/appeal`（スコープ `write:accounts`、本文 `{"text": "..."}`）で異議を申し立てます。
`disable_login`、`suspend`、`ban` を受けた利用者はアクセストークンを得られないため、サインインページにある処分ごとのフォーム（`POST /auth/appeal`）から異議を申し立てます。
[サインインがログインプロバイダーに限られている](login-providers.md#limit-sign-in-to-login-providers)間は、そうした利用者は連携しているプロバイダーを通してだけフォームにたどり着けます。
そのページには、異議を申し立てられる処分ではフォームと期限が、すでに異議を申し立てた処分では内容と結果が、期限を過ぎた処分や異議を申し立てられない処分では理由が表示されます。認められなかった異議申し立てでは、その理由が表示されます。
管理 API では、`GET /api/v1/admin/appeals`（`state` で絞り込み）、`GET .../{id}`、`POST .../{id}/uphold`、`POST .../{id}/overturn`（本文 `{"public_reason": "..."}` は省略できます）を使います。これらには `manage_accounts` の権限が必要です。

- 1 つの処分に異議を申し立てられるのは 1 回で、本文は 1〜2,000 文字です。期限は、処分からインスタンス設定 `appeal_window_days` の日数です（既定は 180 日、20〜365。[インスタンス設定](instance-settings.md)を参照してください）。
  期限を過ぎた処分と 2 回目の異議申し立ては `422` になります（`/api/v1/problems/appeal-window-closed` と `/api/v1/problems/already-appealed`）。
- `ban` には、アカウントが削除されるまで（処分から 30 日後。`appeal_window_days` の期限のほうが短ければその期限まで）異議を申し立てられます。削除されたアカウントの `ban` は `422`（`/api/v1/problems/not-appealable`）になります。解除された処分や期限が切れた処分にも、期限内であれば異議を申し立てられます。
- 利用者が異議を申し立てると、`manage_accounts` を持つ人に通知（`admin_appeal`）が届きます。審査されると、利用者に通知（`appeal_decided`）が届き、結果と `public_reason` を確認できます。審査した人は表示されません。
- 処分を取り消すと、同じトランザクションの中でその処分が解除されます（解除の理由が空の `lift` と同じ効果です）。`delete_statuses` を取り消しても、削除した投稿は戻りません。審査の前にすでに解除されていた処分はそのままです。
- `ban` を取り消すと、同じトランザクションの中で予約されていたアカウントの削除も取り消され（監査ログには `account_deletion_canceled` が記録されます）、凍結が終わり、元のプロフィールの `Update` がほかのサーバーへ送られます。無効になったセッションとトークンは戻らないため、利用者はもう一度サインインします。
  審査の前にアカウントが削除されていた場合は、取り消せません（`422`、`/api/v1/problems/not-appealable`。維持することはできます）。
- 審査は 1 回だけです。同じ結果でもう一度審査しても何も変わらず、反対の結果では `409`（`/api/v1/problems/appeal-decided`）になります。

<!-- source: account-management 99149899a198 -->
## アカウントの管理

アカウントの検索、詳細の表示、IP アドレスとメールアドレスの開示は管理 API で行い、`manage_accounts` の権限が必要です。

Web クライアントでは、1 つのアカウントに 3 つのページがあり、ページのタイトルの下に互いへのリンクがあります。

- **審査**（`/moderation/accounts/{id}`）: アカウントの状態と制限、新しい処分のフォーム、そのアカウントへの最新の処分とその理由、最新 20 件の通報。それより古い通報は通報の一覧（`/moderation/reports`）にだけあり、そこではアカウントで絞り込めません。
- **処分の履歴**（`/moderation/accounts/{id}/actions`）: そのアカウントへのすべての処分を 1 ページずつ表示し、効力のある処分には「解除」があります。各処分は、すべての詳細を載せた専用のページ（`/moderation/accounts/{id}/actions/{action id}`）で開きます。
- **管理**（`/moderation/accounts/{id}/manage`、ローカルのアカウントのみ）: アカウントのロール（`manage_roles`）、メールの送信停止、二要素認証の解除とパスワードのリセット用リンクの発行（`manage_login_recovery`）、メールアドレスと IP アドレス。各セクションは、その権限を持つ人にだけ表示されます。

スタッフが操作できるのは、自分より下の順位の相手だけです。処分を行うことと解除すること、異議申し立ての審査、メールの送信停止の解除、非公開のデータの開示、パスワードのリセット用リンクの発行、二要素認証の解除には、あなたのロールの中で最も高い `position` が、対象のすべてのロールのそれより上である必要があります。
順位が上か同じ相手（あなた自身のアカウントも含みます）には、`403` が問題の種類 `/api/v1/problems/target-outranks-actor` とともに返ります。そのため、モデレーターが管理者を BAN することも、管理者がほかの管理者を BAN することもできません。`my admin` のコマンドにはこの制限はありません。
サインインできる唯一の管理者がサインインできなくなる処分（`disable_login`、`suspend`、`ban`）は、誰が行っても拒否され、その管理者は自分のアカウントを削除できません（`409`、`/api/v1/problems/last-administrator`）。ロールについては [admin-cli.md](admin-cli.md) を参照してください。

- `GET /api/v1/admin/accounts` は、ローカルとリモートのアカウントを新しい順に返します（インスタンスの Actor は除きます）。
  ユーザー名の前方一致（`username`）、ドメイン（`domain`）、ローカルかリモートか（`origin`）、状態（`state`）で絞り込め、ローカルのアカウントではさらに、メールアドレス（`email`）、メールアドレスのドメイン（`email_domain`）、登録やサインインに使った IP アドレス（`ip`）で絞り込めます。
  `email`、`email_domain`、`ip` は 1 つだけ指定できます。一覧には IP アドレスとメールアドレスは含まれません。
- 状態は、次の一覧のうち最初に当てはまるものです: `deleted`、`suspended`（`suspend`、`ban`）、`disabled`（`disable_login`）、`rejected` と `pending`（登録の審査）、`limited`（`limit`）、`active`。
  状態と制限（`restrictions`）は、そのアカウント自身への処分だけから決まり、ドメインへの処分は含みません。
- `GET /api/v1/admin/accounts/{id}` は、状態と制限を返し、ローカルのアカウントではさらに、登録の申請と審査、メールアドレスの確認、ロール、最大 20 件の処分と通報を新しい順に返します。
- `POST /api/v1/admin/accounts/{id}/reveal` は、ローカルのアカウントのメールアドレス、登録とサインインの IP アドレスと User-Agent（新しい順に 20 件）、同じ IP アドレスを使ったほかのアカウント（IP アドレスごとに最大 20 件）を返します。
  呼び出すたびに監査ログに `revealed_private_data` が記録され、その記録を書き込めたときだけデータが返されます。IP アドレスは保存期間（既定は 180 日）を過ぎると削除されるため、それより古いサインイン元は表示できません。
- `two_factor_enabled`（`GET /api/v1/admin/accounts/{id}`）は、ローカルのアカウントの利用者が二要素認証（TOTP）を使っているかどうかを示します。
  認証アプリとリカバリーコードの両方を失った利用者は、`POST /api/v1/admin/accounts/{id}/two_factor/disable` で二要素認証を解除すると、パスワードだけでサインインできるようになります。
  これができるのは `manage_login_recovery` の権限を持つ人だけで、既定では組み込みの管理者のロールだけが持っています（サーバー管理に属する権限で、Web クライアントはそのセクションをその人たちにだけ表示します）。
  解除すると、監査ログに `two_factor_disabled` が記録されます。解除する前に、メールなど別の経路で、その人がアカウントの持ち主であることを確かめてください。
  ほかに管理者のいない管理者が失ったときは、運営者が CLI のコマンド `my admin account disable-two-factor @alice` で解除します（監査ログの操作者は CLI になります）。
- メールを送るサーバーでは、パスワードを忘れた利用者は、サインインページの「パスワードを忘れたとき」から、新しいパスワードを設定するためのリンクをメールで受け取ります。
  メールを送らないサーバー（`mail = none`）や、メールが届かない利用者には、管理者がリンクを発行して渡します。
  管理画面では、アカウントの「管理」ページ（`/moderation/accounts/{id}/manage`、`POST /api/v1/admin/accounts/{id}/password_reset_link`）の「パスワードのリセット」で発行します。
  発行できるのは、二要素認証の解除と同じ `manage_login_recovery` の権限を持つ人だけで、既定では組み込みの管理者のロールだけが持っています（リンクを持つ人は誰でもアカウントに入れるため、モデレーターの `manage_accounts` では足りません）。
  発行したリンクはその場で 1 回だけ表示され、コピーできます。サーバーはリンクのハッシュだけを保存するため、あとから確認することはできません。必要ならもう一度発行してください。
  管理者がいないときや管理画面が使えないときは、運営者が CLI のコマンド `my admin account password-reset-link @alice` で発行します（監査ログの操作者は CLI になります）。
  [サインインがログインプロバイダーに限られている](login-providers.md#limit-sign-in-to-login-providers)間も、どちらの方法でもリンクは発行され、制限が終わるまでリンクは使えないことが表示されます。
  リンクは 1 時間有効で、1 回だけ使えます。そのあと別の経路でパスワードが変わると使えなくなります。どちらの方法で発行しても、監査ログに `password_reset_link_issued` が記録されます。
  リンクを持つ人は誰でもパスワードを設定できるため、メールなど別の経路で本人を確かめ、リンクは本人にだけ渡してください。
  新しいパスワードを設定すると、その利用者のすべてのセッションとアプリのトークンが無効になります。パスキーと二要素認証はそのまま残ります。
- `email_suppression`（`GET /api/v1/admin/accounts/{id}`）は、ローカルのアカウントの主なメールアドレスへの送信が止まっていることを示します。
  `hard_bounce`（宛先が存在しない）では確認メール以外のすべてのメールが止まり、`complaint`（迷惑メールとして報告された）では通知とお知らせのメールだけが止まります。パスワードのリセットのメールも止まるため、必要なら上のリンクを発行して渡してください。
  利用者が確認メールを求めてメールアドレスをもう一度確認すると、ハードバウンスによる送信停止は解除されます。
  利用者がメールボックスを直したなど、送っても問題ないことを確かめたときは、`POST /api/v1/admin/accounts/{id}/email_suppression/lift`（権限 `manage_accounts`）で解除します。監査ログには `email_suppression_lifted` が記録されます。
- 検索は 1 ページを埋めるために最大 400 件のアカウントを読みます。一致するアカウントが少ないときは、短いページや空のページでも `next_cursor` が付くことがあるため、続けて読んでください。

<!-- source: domain-actions 13dd3d4d3a0f -->
## ドメインへの処分

| 種類 | 効果 |
| --- | --- |
| `suspend` | 配送、取得、受信をすべて止める（受信は署名を検証する前に `403` になる）。既存のアカウントと投稿をタイムラインとプロフィールから隠す。ローカルの利用者とそのドメインのアカウントの間のフォローを双方向とも断ち切る（相手側には何も送らない） |
| `limit` | 投稿を公開のタイムラインと検索に出さず、そのアカウントをフォローしていない利用者への通知を作らない |
| `reject_media` | メディアを取得しない |
| `reject_reports` | そのドメインからの通報（`Flag`）を破棄する |
| `force_sensitive` | メディアをセンシティブとして表示する |

```sh
my admin domain issue -kind suspend -public-comment "spam" spam.example
my admin domain issue -kind limit -duration 720h noisy.example
my admin domain list -active
my admin domain lift <action id>
```

管理 API では、`POST /api/v1/admin/domain_actions`（本文には `domain`、`kind`、`public_comment`、`private_note`、`obfuscate`、秒単位の `expires_in` を指定します）、
`GET /api/v1/admin/domain_actions`（`domain` と `active` で絞り込み）、`GET .../{id}`、`POST .../{id}/lift` を使います。

Web クライアントでは、「モデレーション」→「ドメインへの処分」（`/moderation/domain-actions`）を開きます。一覧には、効力のあるもの、またはすべての処分について、ドメイン、種類、状態、終了が表示されます。
「新しい処分」でフォーム（`/moderation/domain-actions/new`）が開き、ドメインを選ぶと、コメントと日時を載せた処分のページが開きます。そこで「解除」を選ぶと処分が終わります。
連合先のサーバーのページにある「ドメインへの処分」のリンクは、そのドメインの処分だけを一覧にし、そこでの「新しい処分」はドメインを入力した状態でフォームを開きます。

- ドメインは IDNA の A-label に正規化して記録されます（小文字、末尾のドットなし）。`例え.jp` と `xn--r8jz45g.jp` は同じドメインです。
- 処分はサブドメインにも適用されます（`spam.example` への処分は `a.spam.example` にも適用されます）。
- 処分は、それを記録するトランザクションの中で制限のプロジェクション（`domain_restrictions`）とタイムラインの制限に反映され、次の配送、取得、受信から効果を持ちます。
- 終了日時のある処分（`-duration` / `expires_in`、最大 10 年）は、その時刻に自動で終わります。期限切れは「解除」ではないため、一覧では `expired` と表示されます。
- 解除で外れるのはその処分の制限だけです。同じドメインへのほかの処分は残ります。すでに解除された処分を解除しても何も変わりません。
- 一覧は新しい順で、`-active`（`active=true`）を付けると効力のある処分だけを表示します。

<!-- source: importing-and-exporting-mastodon-csv ba9ed39bd32e -->
### Mastodon の CSV の取り込みと書き出し

ドメインへの処分の一覧は、Mastodon のドメインブロックの CSV 形式（`#domain,#severity,#reject_media,#reject_reports,#public_comment,#obfuscate`）で Mastodon とやり取りできます。
Mastodon が書き出した CSV を取り込むことも、このサーバーが書き出した CSV を Mastodon に取り込ませることもできます。

```sh
my admin domain export > domain_blocks.csv
my admin domain import domain_blocks.csv                       # only shows what it would do
my admin domain import -apply -only spam.example,bad.example domain_blocks.csv
```

管理 API では、`GET /api/v1/admin/domain_actions/export`（`text/csv`）、`POST .../import/preview`（本文には `csv` を指定します）、`POST .../import`（本文には、選んだ行の `blocks` と `private_note` を指定します）を使います。
このサーバー自身のドメインの行は飛ばされ（`own_domain`）、そのドメインへの処分は拒否されます。
Web クライアントでは、「モデレーション」→「ドメインへの処分」を開き、「Mastodon の CSV」（`/moderation/domain-actions/import`）を選びます。「CSV をエクスポート」でファイルを保存でき、取り込みでは、貼り付けたファイルか選んだファイルをプレビューしてから、残した行を取り込みます。

| CSV の行 | このサーバーでの処分 |
| --- | --- |
| `#severity` が `suspend`（または空） | `suspend` |
| `#severity` が `silence` | `limit` |
| `#severity` が `noop` | なし |
| `#reject_media` / `#reject_reports` が true | それぞれ `reject_media` / `reject_reports` を加える |

- 取り込みは 2 段階です。まずプレビューで各行がどの処分を行うかを確かめ、残す行を選んでから取り込みます（CLI は `-apply` を付けるまで何も変えず、`-only` でドメインごとに行を選びます）。
- そのドメインか親ドメインに同じ種類の処分がすでに効力を持っている行では、その種類は行いません（すべての種類がすでにあれば「適用済み」になります）。同じドメインの 2 行目、不正な行、何も行わない行（`noop` だけ）も飛ばされます。同じ CSV をもう一度取り込んでも、処分は増えません。
- `#domain` のヘッダーがない CSV は、1 行に 1 つのドメインを書いた一覧として読み、すべてのドメインに `suspend` を行います（Mastodon と同じです）。1 回の取り込みで扱えるのは 5,000 行までです。
- 取り込んだ処分は「解除されるまで」続き、行の公開コメントと伏せ字の指定、そしてあなたが指定した非公開のメモを持ちます。1 件ずつ行った処分と同じく監査ログに記録されます（操作者は、API ではその人、CLI では `cli` です）。
- 書き出しでは、効力のある処分について、ドメインごとに 1 行を書きます。`suspend` のあるドメインは `suspend`、`limit` のあるドメインは `silence`、どちらもないドメインは `noop` になります。Mastodon のドメインブロックにない `force_sensitive` だけを持つドメインは書き出しません。非公開のメモと終了日時は書き出しません。

<!-- source: severed-follows 51be435d8635 -->
### 断ち切られたフォロー

ドメインへの `suspend` とリモートのアカウントへの `suspend` は、ローカルの利用者と凍結されたアカウントの間のフォローを双方向とも断ち切ります。

- ドメインへの `suspend` では、処分を記録したあとに実行されるジョブ（`moderation.sever_domain`）が、ローカルの利用者 1 人ずつ断ち切ります。ジョブが終わるまでは、フォローはまだ一覧に表示されます。サブドメインのアカウントも断ち切られます。
- リモートのアカウントへの `suspend` では、処分を記録するトランザクションの中でフォローが断ち切られます。すでに凍結されているアカウントへの 2 回目の `suspend` は何も断ち切りません。
- 承認済みのフォローを失った利用者には、失ったフォロワーとフォローの数を伝える通知（`relationships_severed`）が 1 件ずつ届きます。フォローリクエストだけを失った利用者には届きません。
- 処分を解除しても、断ち切られたフォローは戻りません。利用者はもう一度フォローします。
- 同じ処分をもう一度処理しても（ジョブが再実行されたときなど）、断ち切られた関係の記録や通知は増えません。

<!-- source: publishing-the-domains-you-act-on b647d2c8c84c -->
### 処分したドメインの公開

効力のある `limit` と `suspend` の処分は、`GET /api/v1/instance/domain_actions` で公開できます。
インスタンス設定 `domain_actions_visibility`（`off` / `signed_in` / `everyone`、既定は `off`）で一覧を見られる人を、
`domain_actions_rationale_visibility`（値は同じ、既定は `off`）で公開コメントを見られる人を決めます（[インスタンス設定](instance-settings.md)）。

```sh
my instance set domain_actions_visibility signed_in
my instance set domain_actions_rationale_visibility signed_in
```

- `off` では API は `404` を返し、`signed_in` ではアクセストークンのないリクエストに `401` を返します。
- 処分ごとに 1 件で、新しい順に並びます。各件には、ドメイン、ドメインの SHA-256（16 進数）、種類があり、見ることを許された人には公開コメントも含まれます。非公開のメモは決して返されません。
- `-obfuscate`（`obfuscate`）を付けた処分では、ドメインの文字（ドットを除く）の真ん中の半分を `*` に置き換えます（`bad.example` は `bad.*****le` になります）。
  知っているドメインの SHA-256 を比べれば、誰でもそれが伏せ字のドメインかどうかを確かめられます。
- `reject_media`、`reject_reports`、`force_sensitive` の処分と、解除された処分や期限が切れた処分は公開しません。
- Web クライアントのインスタンスの情報ページ（`https://<host>/about`）は、同じ設定のもとでこの一覧を表示します。`off` ではそのセクションを表示せず、`users` では、サインインしていない人に、サインインした利用者だけが見られるという注記を表示します。

<!-- source: refetching-remote-posts 883740b40c9a -->
## リモートの投稿の取り直し

保存しているリモートの投稿を、相手のサーバーからもう一度取得できます。
取り直しは、管理 API、CLI、Web クライアントで求めたときだけ行い、定期的には行いません。

```sh
my admin refetch status <post ID or URI>
my admin refetch account @bob@remote.example
my admin refetch domain remote.example
```

管理 API では、`POST /api/v1/admin/statuses/{id}/refetch`、`POST /api/v1/admin/accounts/{id}/refetch_statuses`、`POST /api/v1/admin/instances/{domain}/refetch_statuses` を使います。これらには `manage_domains` の権限が必要です。
Web クライアントでは、アカウントの「審査」ページ（`/moderation/accounts/{id}`、リモートのアカウント）と連合先のサーバーの画面（`/moderation/instances/{domain}`）にある「投稿を取り直す」を使います。

- 投稿は、受け取った `Update` と同じように扱われます。新しい版があれば版を追加し、投票の票数を更新し、何も変わっていなければ何もしません。相手のサーバーが 410 Gone か `Tombstone` を返した投稿は削除されます。取得できない投稿（404 を含みます）はそのまま残ります。
- アカウントでは、保存している削除されていない投稿のうち新しいものから最大 1,000 件を、100 件ずつ 1 分おきに取り直します。
- ドメインでは、そのドメインの新しいアカウントから順に、アカウントごとに新しい投稿を最大 20 件、合計で最大 1,000 件を、5 アカウントずつ 1 分おきに取り直します。
- `suspend` のあるドメインや、配送の失敗によって `unavailable` になっているドメインからは取得しません。同じ URI の取得の失敗は 1 時間記憶され、その間の取り直しでは取得しません。
- あなたの依頼は、監査ログに `status_refetch_queued`、`account_statuses_refetch_queued`、`domain_statuses_refetch_queued` として記録されます。

<!-- source: reports e8a3f112b323 -->
## 通報

ほかのサーバーから受け取った通報（`Flag`）は、未解決（`open`）として通報の一覧に入ります。

```sh
my admin report list -state open
my admin report show <report id>
my admin report assign <report id> <username>
my admin report unassign <report id>
my admin report resolve -outcome dismissed <report id>
my admin report note <report id> "Same person as an earlier report"
my admin report reopen <report id>
my admin report forward -comment "Spam account, see the linked posts" <report id>
```

管理 API では、`GET /api/v1/admin/reports`（`state`、`category`、`assignee` で絞り込み。担当者は `me`、`none`、またはローカルのアカウントの ID。`order=oldest` で、新しい順ではなく最も長く待っている通報から並べます）、`GET .../{id}`、`POST .../{id}/assign_to_self`、`POST .../{id}/unassign`、
`POST .../{id}/resolve`（本文 `{"outcome": "actioned" | "dismissed"}`）、`POST .../{id}/notes`（本文 `{"body": "..."}`）、`POST .../{id}/reopen`、`POST .../{id}/forward`（本文 `{"comment": "..."}`、省略可）を使います。

- 担当の割り当て（`assign`）ができるのは、ロールが `manage_reports` を持つローカルの利用者だけです。管理 API では自分を割り当てます。
- 解決（`resolve`）の結果は、`actioned`（対象に処分を行った）か `dismissed`（処分は必要なかった）です。解決済みの通報をもう一度解決しても、結果は変わりません。
- 通報から処分を行うと（処分の `report_id` / `-report`）、同じ操作の中でその通報は `actioned` として解決されます。
- メモ（`note`）はモデレーターだけが見る内部のメモで、最大 2,000 文字です。対象の人にも通報者にも表示されません。編集も削除もできません。
- 再開（`reopen`）は、解決済みの通報を `open` に戻します（担当の割り当ては残ります）。未解決の通報は再開できません（管理 API では `409`）。
- 転送（`forward`、通報の詳細の **相手のサーバーに転送する** ボタン）は、ほかのサーバーのアカウントについての通報を、あなたのインスタンスの Actor による `Flag` として、そのアカウントとまだ残っている通報された投稿とともに、そのサーバーへ送ります。含まれる文章は、転送するときにそのサーバーのモデレーターに向けてあなたが書いたものだけです（`-comment`、管理 API の `comment`、または通報のページの **相手のモデレーターへのメッセージ**。最大 1,000 文字で、既定では何もありません）。通報者のコメントと[外部の審査](#reports-that-an-external-review-filed)のメモは、この方法では決してあなたのサーバーの外に出ず、通報者のことは何も送られません。通報者が通報するときに転送を選んだ通報は、通報者のコメントとともにすぐ送られています。転送できるのは、このサーバーで人または[外部の審査](#reports-that-an-external-review-filed)が出した通報で、未解決か解決済みかにかかわらず、まだ転送していないものです。ほかのサーバーから受け取った通報と、通話のルームについての通報は転送できません（管理 API では `409` `report-not-forwardable` になり、通報の詳細にはボタンが表示されません）。通報が送られるのは 1 回だけです。どんな文章でもう一度転送しても、通報者が通報時に転送していた通報を転送しても、何も変わりません。管理 API の `forwarded` は送られたかどうかを、`forwardable` はいま送れるかどうかを示します。
- 通報の詳細（`show`、`GET .../{id}`）は、メモを古い順に返し、対象についてのほかの通報の数と、対象への処分の数（解除されたものと期限が切れたものを含みます）も返します。一覧はメモを返しません。
- 割り当て、解決、再開、メモ、転送はイベント（`report_assignments`、`report_resolutions`、`report_reopenings`、`report_notes`、`report_forwards`）として保存され、`report_states` が現在の状態を持ちます。
- モデレーターは、通報のページで、または `POST .../{id}/statuses/{status_id}/translation` と `POST .../{id}/comment/translation`（本文 `{"target_language": "en"}`）で、通報された投稿と通報者のコメントを翻訳できます。
  文章が翻訳プロバイダーに送られるのは、[インスタンス設定](instance-settings.md)の `translation_reports_to_server` がオンの間だけで、送り先もその投稿の公開範囲を許可されたプロバイダーだけです。モデレーターの権限で範囲が広がることはないため、ダイレクトの投稿は `direct` を許可されたプロバイダーにだけ送られ、コメントは `private` として扱われます。
  コメントの翻訳も、削除された投稿の翻訳も保存されず、文字数はサーバーの月間の上限にだけ数えられます。`translation_browser` がオンのときのブラウザー組み込みの翻訳は、`translation_reports_to_server` にかかわらず通報で使えます。
- 通報者に見える対象は、ふだんそのアカウントについて見えるものだけです。対象が凍結されているか通報者をブロックしている間は、通報は一覧に残りますが、アカウントは表示されません。モデレーターには常に対象の全体が見えます。
- ローカルの通報者には、最初の解決のときに通知（`report_resolved`）が届きます。再開してもう一度解決しても、通知は増えません。
  通報者は `GET /api/v1/reports` で自分の通報を一覧にでき、それぞれが未解決か解決済みかだけが見えます。結果、行われた処分、対象に伝えた理由、担当者は表示されません。

<!-- source: reports-that-an-external-review-filed 8563536b46e4 -->
### 外部の審査が出した通報

`flag` と答える[判定フック](decision-hooks.md)は、あなたのインスタンスの Actor を通報者として通報を出します。通報には
そのことが示されます。管理 API と `report.created` の Webhook では `hook_origin`、通報の詳細では 1 行の説明、
`my admin report show` では `automated` です。コメントはプラグインのメモで、カテゴリーは理由から決まります。出されたときには転送されず、
解決しても誰にも通知されません。対象がほかのサーバーにいるときは、自分で
転送（`forward`）できます。プラグインのメモはあなたのサーバーに残り、外に出るのはあなたが書いた文章だけです。判定を取り消しても、
通報はそのまま残ります。

<!-- source: voice-chat-rooms 7fd142752b07 -->
### 通話のルーム

- ローカルの利用者は、自分に見えている開いた通話のルームを通報できます（`voice_room_id` を指定し、`account_id` をルームのホストにした `POST /api/v1/reports`）。
  その通報はその時点のホストを対象にし、その時点のルームの設定（名前と説明）を指します。投稿は指さず、ほかのサーバーへ転送されることはありません。
- 通報の詳細（`GET /api/v1/admin/reports/{id}`）にはルーム（`voice_room`）が表示されます。設定の変更のすべて、参加した人と退出した時刻、ルームから外された人、ルームの終わり方です。
  参加した人の記録は、ルームが終わってから 90 日間（`voice_participants_retention_days`）と、通報がそのルームを指している間、保存されます（[記録の保存](#record-retention-and-holds)）。
  録音もテキストチャットもないため、これらの記録が唯一の証拠です。
- `manage_reports` を持つ人は、`POST /api/v1/admin/voice_rooms/{id}/end`（本文 `{"reason": "..."}`、省略可、最大 500 文字）で、開いているルームを参加者全員について終了できます。
  理由はモデレーターのためだけのもので、参加者にはルームが終わったことだけが見えます。通話がオフの間も使え、監査ログには `voice_room_ended` が記録されます。

<!-- source: notifications-for-moderators-and-the-pending-counts 7b83fa4097b2 -->
## モデレーター向けの通知と未処理の件数

新しい通報、承認待ちの登録、異議申し立ては、権限を持つ人にアプリ内の通知として届きます。

| 通知 | 届くとき | 届く人 |
| --- | --- | --- |
| `admin_report` | 通報の対象にほかの未解決の通報がないとき（同じアカウントについての 2 件目以降の通報では通知しない） | ロールが `manage_reports` を持つ人。通報者と通報の対象を除く |
| `admin_sign_up` | 登録が承認待ちとして始まったとき（承認制、またはメールを送れない構成での `open`） | ロールが `manage_accounts` を持つ人 |
| `admin_appeal` | 処分に異議が申し立てられたとき | ロールが `manage_accounts` を持つ人。異議を申し立てた人を除く |
| `admin_custom_emoji_request` | カスタム絵文字の作成や画像の差し替えが申請されたとき | ロールが `manage_custom_emojis` を持つ人。申請した人を除く |
| `admin_error_group` | サーバーか Web クライアントが新しい種類のエラーを報告したとき、または解決済みのエラーグループがサーバーの別のバージョンで再び起きたとき | ロールが `manage_errors` を持つ人 |

- `admin_error_group` が届くのは、サーバー全体で 1 時間に最大 10 回です。その時間の残りは、ほかにいくつのエラーグループがあったかを伝える 1 件の通知に
  まとめられます。ミュートしたグループは通知しません。サインインしていない訪問者について Web クライアントが報告したエラーは、
  サインインした利用者のブラウザーも報告したときにだけ通知し、その再発は誰にも通知しません。
  エラーグループは、`GET /api/v1/admin/error_groups` と `POST /api/v1/admin/error_groups/{id}/resolve`（`/mute`、`/unmute`）で読んだり、解決、ミュート、ミュートの解除をしたりします。これらには `manage_errors` が必要です。
- 誰が通知を受け取るかは、通知が作られた時点の権限で決まります。あとで権限を失った人には、その種類の通知が一覧と未読数に表示されなくなります。権限が戻ると、再び表示されます。
- 上の通知はどの種類もメールでも届き、既定でオンです。各自が、設定の **通知** ページ（Web クライアントでは `/settings/notifications`）で、種類ごとにアプリとメールのそれぞれをオンまたはオフにします。
  そのページには、自分のロールが受け取る種類だけが表示されます。アプリでオフにした種類はその人について作られないため、プッシュもメールもされません。
  `GET /api/v1/admin/counts` の件数は変わりません。件数は通報、登録、異議申し立てそのものを数えるためです。
  未解決の通報とともに、件数には最も古い未解決の通報が出された時刻（`oldest_open_report_filed_at`）も返されます。
  サーバーは通知を記録するトランザクションの中でメールのジョブをキューに入れ、ジョブはコミットの直後に、通知が読まれるのを待たずにメールを送ります。
  送る時点でその人が権限を失っているか、その種類のメールをオフにしているか、主なメールアドレスが確認されていないかそのメールが止まっているときは、何も送りません。
  メールには通知の種類と通知の画面へのリンクだけが含まれ、通報やアカウントの内容は含まれません。配信停止のリンクは、その種類のメールだけをオフにします。
  `mail = none` ではメールは送られず、`GET /api/v1/instance` の `sends_email` は `false` です。

`GET /api/v1/admin/counts` は、未解決の通報の数（`open_reports`、`manage_reports` が必要）、承認待ちの登録の数（`pending_registrations`）、審査待ちの異議申し立ての数（`pending_appeals`）（どちらも `manage_accounts` が必要）を返します。
権限のない項目は返されません。

<!-- source: audit-log 86955f4182c0 -->
## 監査ログ

モデレーターと管理者の操作は、操作と同じトランザクションの中で、1 件ずつ 1 行として監査ログ（`audit_log_entries`）に記録されます。
失敗した操作と、何も変えなかった操作は記録されません。すでに解除された処分の解除、解決済みの通報の解決、同じ担当者への割り当て、すでに廃止されたルールの廃止、同じ位置への移動、すでに持っているロールの付与、持っていないロールの取り消しなどです。

| 対象の種類 | 操作 |
| --- | --- |
| `account` | `account_action_issued` と `account_action_lifted`（アカウントへの処分とその解除）、`account_deletion_canceled`（`ban` への異議申し立てが認められて削除が取り消された）、`revealed_private_data`（IP アドレスとメールアドレスの開示）、`two_factor_disabled`（二要素認証の解除）、`password_reset_link_issued`（パスワードのリセット用リンクの発行）、`email_suppression_lifted`（メールの送信停止の解除）、`account_moved`（制限を受けたローカルのアカウントがローカルのアカウントへ移行した。制限は移行先に引き継がれないため、移行先にも適用するかどうかはあなたが決めます）、`account_statuses_refetch_queued`（リモートのアカウントの投稿の取り直し） |
| `status` | `status_refetch_queued`（リモートの投稿の取り直し。対象の名前は投稿の URI） |
| `domain` | `domain_action_issued` と `domain_action_lifted`（ドメインへの処分とその解除）、`domain_statuses_refetch_queued`（そのドメインのアカウントの投稿の取り直し）、`instance_marked_available`（サーバーの失敗の記録の消去） |
| `report` | `report_assigned`、`report_unassigned`、`report_resolved`、`report_reopened`、`report_note_added`、`report_forwarded`（通報の担当の割り当て、割り当ての解除、解決、再開、メモの追加、転送） |
| `user` | `registration_approved`、`registration_rejected`（登録の審査） |
| `setting` | `setting_changed`（インスタンス設定）、`text_changed`（インスタンスの文章） |
| `rule` | `rule_added`、`rule_revised`、`rule_moved`、`rule_retired`（インスタンスのルール） |
| `role` | `role_granted`、`role_revoked`（利用者へのロールの付与と取り消し）、`role_created`、`role_updated`、`role_deleted`（ロールの作成、変更、削除） |
| `appeal` | `appeal_upheld`、`appeal_overturned`（異議申し立ての審査。取り消すと、その処分について `account_action_lifted` も記録され、`ban` では `account_deletion_canceled` も記録されます） |
| `signup_block` | `signup_block_added`、`signup_block_lifted`（登録の拒否の追加と解除） |
| `hook_decision` | `hook_decision_applied`（サーバーが外部の審査の判定に従って処理し、状態を残した。操作者はシステム）、`hook_decision_overridden`（モデレーターが判定を取り消した。異議申し立てを認めて取り消した場合も含む）、`hook_appeal_upheld`（モデレーターが拒否された投稿の異議申し立てを認めず、判定はそのまま） |
| `custom_emoji` | `custom_emoji_created`、`custom_emoji_image_replaced`、`custom_emoji_copied`、`custom_emoji_updated`、`custom_emoji_enabled`、`custom_emoji_disabled`、`custom_emoji_deleted`（カスタム絵文字の登録、画像の差し替え、ほかのサーバーからの絵文字のコピー、変更、有効化、無効化、削除） |
| `webhook` | `webhook_created`、`webhook_updated`、`webhook_deleted`、`webhook_enabled`、`webhook_disabled`、`webhook_secret_rotated`（Webhook の作成、変更、削除、有効化、無効化、シークレットの再生成） |
| `announcement` | `announcement_created`、`announcement_updated`、`announcement_published`、`announcement_withdrawn`（お知らせの作成、変更、公開、取り下げ） |
| `invite` | `invite_created`、`invite_revoked`（招待の作成と取り消し。対象の名前はコードの最初の 4 文字） |
| `billing_source` | `billing_import_queued`（請求の取り込みを手動で始めた。対象の名前はプロバイダーの名前） |
| `cost_item` | `cost_item_created`、`cost_item_relabeled`、`cost_item_price_revised`、`cost_item_end_changed`、`cost_item_deleted`（費用の登録、名前などの変更、金額の改定、終了日の変更、削除。対象の名前は項目の名前） |
| `supporter_program` | `supporter_program_enabled`、`supporter_program_disabled`（課金の有効化と無効化。対象の名前は決済サービスの名前） |
| `supporter_plan` | `supporter_plan_created`、`supporter_plan_revised`、`supporter_plan_retired`（サポーターのプランの作成、変更、廃止。対象の名前はプランの名前） |
| `supporter_subscription` | `supporter_subscription_ended`（管理者が申し込みをすぐに終了した。対象の名前はプランの名前と利用者のハンドル） |
| `custom_emoji_request` | `custom_emoji_request_submitted`、`custom_emoji_request_withdrawn`、`custom_emoji_request_approved`、`custom_emoji_request_rejected`（カスタム絵文字の申請の提出、取り下げ、承認、却下。対象の名前は `:shortcode:`） |
| `payment_provider` | `payment_reconcile_queued`（決済サービスとの照合を手動で始めた。対象の名前はプロバイダーの名前） |
| `oauth_client` | `oauth_client_disabled`、`oauth_client_enabled`（OAuth クライアントの無効化とその取り消し。無効化するとそのすべてのトークンが無効になる。対象の名前はクライアントの名前と `client_id`） |
| `oauth_client_host` | `oauth_client_host_denied`、`oauth_client_host_allowed`（Client ID Metadata Document のホストの拒否とその取り消し。拒否するとそのホストのクライアントのトークンが無効になる。対象の名前はホスト） |
| `instance` | `service_closure_announced`、`service_closure_withdrawn`、`service_closure_closed`、`service_closure_delivery_completed`（サービスの終了の告知または日付の変更、告知の取り下げ、終了、終了の配送の完了。対象の名前は `service_closure`） |
| `voice_room` | `voice_room_ended`（モデレーターが開いている通話のルームを終了した。名前は書いたアカウントが消去されるとなくなるため、対象の名前はルームの ID。通報の詳細には名前が表示される） |
| `link_card_domain` | `link_card_domain_fetch_stopped`、`link_card_domain_images_stopped`、`link_card_domain_rule_removed`（ドメインへのリンクカードのクローラーのルールの設定と削除。[リンクカードのドメイン](instance-settings.md#link-card-domains)を参照） |
| `login_provider` | `login_provider_created`、`login_provider_updated`、`login_provider_deleted`、`login_provider_enabled`、`login_provider_disabled`、`login_provider_secret_rotated`（外部のログインプロバイダーの追加、変更、削除、有効化、無効化、クライアントシークレットの差し替え。対象の名前はプロバイダーの表示名） |

一覧には、対象がまだ存在するかどうかが表示されます。保存期間を過ぎてなくなったアカウント、通報、異議申し立て、登録の拒否と、削除されたロール、カスタム絵文字、Webhook は、記録した時点の名前だけが表示され、画面へのリンクはありません。

```sh
my admin audit list
my admin audit list -actor alice -action domain_action_issued
my admin audit list -target-kind domain -target-id spam.example
```

管理 API では、`GET /api/v1/admin/audit_log`（`actor_account_id`、`action`、`target_kind`、`target_id` で絞り込み）を使います。`view_audit` の権限が必要です。

- 1 行は、操作者（利用者なら `user`、CLI なら `cli`）、操作、対象の種類と ID、対象の表示名、元の記録の ID、時刻です。
  対象の ID は、アカウント、通報、異議申し立て、利用者、ルール、ロール、登録の拒否の ID です。ドメインへの処分ではドメイン、設定と文章ではキーです。
- 対象の表示名（`@alice`、`@bob@example.com`、ドメイン、設定のキー、ルールの本文。文章ではそのキー、ロールではそのキーと利用者のハンドル、登録の拒否ではその種類と値）は記録した時点のコピーなので、対象がなくなったあとも読めます。
  API の各行は、対象がまだ存在するかどうか（`target_exists`）も返し、なくなったアカウント、通報、異議申し立て、登録の拒否では `false` になります。Web の監査ログでは、なくなった対象は表示名だけがリンクなしで表示されます。
- 理由とメモの本文はコピーされません。元の記録（処分、割り当て、解決、再開、メモ、審査、または設定やルールの版）の ID で調べてください。
- 一覧は新しい順で、キーセットによるページ送りを使います（CLI では `-before <entry id>`）。
- 監査ログを更新したり削除したりする API や CLI はありません。古い行は[保存期間](#record-retention-and-holds)を過ぎると削除されます。

<!-- source: record-retention-and-holds bd806b3bbda7 -->
## 記録の保存と保全

記録の種類ごとの保存期間を過ぎると、1 時間ごとのジョブ（`moderation.purge_records`、`identity.purge_origins`、`voice.purge_records`）がモデレーションの記録を削除します。
期間はインスタンス設定で変えられますが、下限より短くはできません（[インスタンス設定](instance-settings.md)）。変えた期間は次の実行から反映されます。解除された保全は、その理由とともに、解除からモデレーションの処分の保存期間が過ぎると削除されます。

| 記録 | 設定 | 既定 | 下限 | 起点 |
| --- | --- | --- | --- | --- |
| サインインの IP アドレスと User-Agent | `session_origins_retention_days` | 180 日 | 90 日 | サインイン |
| 登録の IP アドレス | `registration_origins_retention_days` | 180 日 | 90 日 | 登録 |
| 通報（コメント、対象の投稿、メモ、割り当て、解決、再開） | `reports_retention_days` | 365 日 | 180 日 | 最後の解決 |
| アカウントへの処分（理由、メモ、対象の投稿、解除）と異議申し立て | `moderation_actions_retention_days` | 365 日 | 180 日 | 処分の終わり |
| 監査ログ | `audit_log_retention_days` | 730 日 | 365 日 | 記録 |
| 登録の拒否 | なし（変更できません） | 30 日 | なし | 解除と期限切れのうち早いほう |
| 通話のルームに参加した人（参加、話す人と聞く人、挙手、招待、外された人） | `voice_participants_retention_days` | 90 日 | 30 日 | ルームの終わり |
| [判定フック](decision-hooks.md#the-decisions)の判定 | `hook_decisions_retention_days` | 90 日 | 30 日 | 判定 |

- 未解決の通報は削除されません。処分が参照している通報（処分の `report_id`）は、その処分が削除されるまで残ります。
- 処分の終わりは、解除と期限切れのうち早いほうです。異議申し立てのあった処分では、審査がそれより遅ければ審査の時刻が終わりになり、異議申し立てが審査を待っている間は処分は保存されます。
  `warn` と `delete_statuses` には続く効果がないため、行った時刻が終わりになります。終わりのない処分（`ban`、終了日時のない `suspend` など）は、解除されるまで削除されません。
- 通報や処分が指している投稿は、作成者が削除しても（表示からは消えます）、その通報や処分が削除されるまで、添付のメディアとともに削除ジョブによって物理的に削除されることはありません。リモートの投稿も保存期間（インスタンス設定 `remote_status_max_retention_days` を含みます）を過ぎても残ります。
- 解除も終了日時もない登録の拒否は、効力を持つ限り残ります。
- 通報が指している通話のルームに参加した人の記録は、その通報が削除されるまで残ります。ルームそのもの（名前、説明、ホスト、終わり方）は削除されません。
- 1 回のジョブの実行で削除するのは、種類ごとに上限のある行数だけです（処分、通報、登録の拒否は 100 件ずつ 20 回、監査ログは 1,000 行ずつ 20 回）。残りは次の実行で削除します。

<!-- source: holds f3bf57c2c258 -->
### 保全

裁判所の命令で消去が禁じられている間（日本の情報流通プラットフォーム対処法第 16 条）や、開示請求に対応している間は、アカウントごとに消去を止められます。

```sh
my admin retention hold -reason "court order 2026-123" @alice
my admin retention list -account @alice
my admin retention release <hold id>
```

- 保全中のアカウントについては、そのアカウントへの処分と異議申し立て、そのアカウントについての通報とそのアカウントが出した通報、それらの監査ログ、サインインと登録の IP アドレス、投稿、そのアカウントが参加したり何かを行ったりした通話のルームの記録が、保存期間を過ぎても削除されません。
  そのアカウントの BAN から作られた登録の拒否も、解除や期限切れから 30 日を過ぎても残ります。
- 1 つのアカウントに複数の保全をかけられます。すべてを解除すると、期間を過ぎた記録は次の実行で削除されます。すでに解除された保全を解除しても何も変わりません。
- 保全とその解除は CLI でだけ行い、記録の操作者は CLI になります。
- 削除されたアカウント（`ban` が日付に達したもの、利用者が削除したもの、そのサーバーが削除したリモートのアカウント）のデータは、削除から 7 日後に 1 時間ごとのジョブ `account.erase_deleted` が消去します。リモートのアカウントの投稿も、ローカルの利用者がブックマークしたものや返信したものを含めて消去されます。
  そのアカウント自身の署名付きの `Delete` がなく、取得が `410 Gone` を返しただけで削除済みとされたリモートのアカウントは、先にもう一度取得します。2 回目も `410` なら消去します。そのサーバーが再びアカウントを返すようになれば、アカウントは投稿とともに戻り、何も消去されません。それ以外の応答（`404`、または応答なし）では次の実行を待ち、削除から 30 日後にはいずれにしてもアカウントを消去します。
  保全中のアカウントは消去されません。通報や処分が指している投稿は、その記録が削除されるまで残ります。アカウントの URI とユーザー名、処分の記録は残ります。
  そのアカウントが通話のルームに付けた名前と説明は空になりますが、ルームは残ります。

<!-- source: exporting-for-disclosure-requests 89f1074dd950 -->
### 開示請求のためのエクスポート

開示請求（GDPR 第 15 条、日本の個人情報保護法第 33 条）には、アカウントのモデレーションの記録をすべて JSON としてエクスポートして対応します。

```sh
my admin account export-moderation @alice > alice-moderation.json
```

- エクスポートには、そのアカウントへの処分（非公開のメモ、解除、異議申し立てと審査を含みます）、そのアカウントについての通報（コメント、対象の投稿、挙げられたルール、モデレーターのメモ、割り当て、解決、再開）、そのアカウントが出した通報（それぞれが未解決か解決済みかだけ）、それらの監査ログが含まれます。
- 通報者の身元（通報したアカウント、受け取った通報の URI と送ってきたサーバー）はエクスポートしません。エクスポートを渡す前に、通報のコメントに通報者を特定できる内容がないことを確かめてください。
- モデレーターはユーザー ID でエクスポートされます。対応している間に記録が削除されないように、先に保全をかけてください。
