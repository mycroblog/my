# アカウント・ドメインへの処分と通報の対応

運営者・モデレーター向けの手順です。要件は FR-701〜FR-703・FR-707・FR-714・FR-718（[機能要件](../requirements/01-functional.md)）、
テーブルは [データモデル §4.3](../architecture/data-model/04-tables/03-moderation.md#43-moderation)、連合への効き方は [federation.md §11](../architecture/federation.md#11-モデレーションとの連携) です。

アカウントの管理（管理 API だけ）を除き、どれも管理 API（`/api/v1/admin/...`、スコープ `admin:read` / `admin:write`）と CLI（`my admin account action` / `my admin domain` / `my admin report` / `my admin appeal`）で行えます。
管理 API はロールの権限を要します。アカウントの管理と処分と異議申し立ては `manage_accounts`、ドメイン単位の処分は `manage_domains`、通報は `manage_reports` です。
CLI はサーバーと同じ設定（`-config` か環境変数）で DB に直接つなぎ、操作者の利用者を記録しません（`issued_by_user_id` などは NULL、[監査ログ](#監査ログ)の操作者は `cli`）。

## Web クライアントのモデレーションとサーバー管理

Web クライアントは、スタッフの画面を 2 つに分けます（[ADR-0058](../adr/0058-moderation-admin-separation.md)）。
ナビゲーションには、ロールが各区分の権限のどれかを持つ人にだけ「モデレーション」と「管理」が出ます。どの画面も、権限のない人には理由だけを出し、サーバーも要求を断ります。

| 区分 | 権限 | 画面 |
| --- | --- | --- |
| モデレーション（`/moderation`） | `manage_reports`・`manage_accounts`・`manage_domains`・`manage_invites`・`view_audit` | 通報（`/moderation/reports`）・異議申し立て（`/moderation/appeals`）・アカウント（`/moderation/accounts`）・登録の審査（`/moderation/registrations`）・登録の拒否リスト（`/moderation/signup-blocks`）・招待（`/moderation/invites`）・ドメインへの処分（`/moderation/domain-actions`）・連合先（`/moderation/instances`）・監査ログ（`/moderation/audit-log`） |
| サーバー管理（`/admin`） | `manage_settings`・`manage_rules`・`manage_roles`・`manage_jobs`・`manage_login_recovery`・`manage_custom_emojis`・`manage_webhooks`・`manage_announcements`・`manage_finance`・`manage_oauth_apps` | サーバーの設定（`/admin/settings`）・ルール（`/admin/rules`）・規約とポリシー（`/admin/documents`）・ジョブのキューと失敗したジョブ（`/admin/jobs`）・カスタム絵文字（`/admin/custom-emojis`）・お知らせ（`/admin/announcements`）・Webhook（`/admin/webhooks`）・費用と利用料金の取り込み（`/admin/finance`、`manage_finance`）。`manage_login_recovery`（ログインの回復: 二要素認証の解除とパスワードリセットのリンクの発行）はアカウントのページ（`/moderation/accounts/{id}`、`manage_accounts` も要る）の区画に出ます |

- 「モデレーション」のバッジは、未解決の通報・承認待ちの登録・審査待ちの異議申し立ての合計です（[モデレーターへの通知と未対応の件数](#モデレーターへの通知と未対応の件数)）。「管理」には件数を出しません。
- 組み込みの `moderator` は `manage_reports`・`manage_accounts`・`view_audit` だけを持ち、「管理」は出ません。ドメインへの処分と連合先の画面（`manage_domains`）も出ないので、
  モデレーターに任せるときは、その権限を持つロールを作って与えます（[ロールの付与と剥奪](admin-cli.md#ロールの付与と剥奪)）。
- 管理 API の経路は区分によらず `/api/v1/admin/...` のままで、スコープも `admin:read` / `admin:write` の 1 組です。区分は API リファレンスのタグ（`admin-moderation`・`admin-server`）で分かれます。
- 権限には 3 つめの区分「利用者の機能」（`view_finance`）があり、スタッフの画面の外にあります（[ADR-0070](../adr/0070-permission-split-emoji-finance.md)）。これだけを持つ人には「モデレーション」も「管理」も出ず、`view_finance` を持つ人のナビゲーションには設定の後に「会計」（`/finance`）が出ます。
  API は利用者の名前空間（`GET /api/v1/finance/cost_totals`・`GET /api/v1/finance/income_totals`、スコープ `read:finance`）で、`admin:*` のスコープを要しません。見せる範囲は[インスタンス設定](instance-settings.md)の `finance_viewer_*` で決めます。

## アカウントへの処分

処分は記録したトランザクションで制限の投影（`account_restrictions`）に反映され、次の要求から効きます（[データモデル §5.2](../architecture/data-model/05-states-and-policies.md#52-処分の種類と制限能力の剥奪)）。

| 種類 | ローカルのアカウントへの効果 |
| --- | --- |
| `warn` | 制限はない（本人に知らせるだけ） |
| `force_sensitive` | 添付をすべてセンシティブとして表示する |
| `limit` | 投稿（過去の投稿を含む）をフォロワー限定として扱い、公開タイムラインに出さない |
| `restrict_posting` | 投稿・返信・ブースト・編集を禁止する |
| `restrict_interaction` | フォロー・お気に入り・ブースト・返信を禁止する |
| `disable_login` | アプリと API の利用を禁止し、有効なセッションとトークンを失効させる（ログイン画面では処分を確かめられる） |
| `suspend` | 上の制限をすべて課し、プロフィールと投稿を本人以外に見せない |
| `ban` | `suspend` と同じ制限を課し、30 日後にアカウントを削除する。解除できない（その 30 日のあいだの異議申し立てを覆したときだけ取り消される） |
| `delete_statuses` | 指定した投稿を削除する。本人にも見えなくなり、連合先には本人の削除と同じ `Delete` を送る |
| `mark_statuses_sensitive` | 指定した投稿の添付をセンシティブとして表示する |

リモートのアカウントには `limit`・`suspend`・`force_sensitive` だけを出せます（ほかの種類は管理 API では `422`、CLI では失敗）。
効果はこのサーバーの中だけで、相手のサーバーに送るのは `suspend` で切ったフォローの `Reject` / `Undo` だけです（[ADR-0036](../adr/0036-moderation-federation-propagation.md)）。

| 種類 | リモートのアカウントへの効果 |
| --- | --- |
| `force_sensitive` | 添付をすべてセンシティブとして表示する |
| `limit` | 投稿（過去の投稿を含む）をフォロワー限定として扱い、公開タイムラインに出さず、フォローしていない利用者への通知を作らない |
| `suspend` | そのアカウントからの受信を捨て、プロフィールと投稿を見せない。ローカルの利用者とのフォローを両方向とも（フォローリクエストも）切り、相手のサーバーにそのアカウントのフォローの `Reject` と利用者のフォローの `Undo` を送る。解除すると表示が戻り、また受信する（凍結中に捨てた受信と切ったフォローは戻らない） |

- `notify_target`（`-no-notify`）は、リモートのアカウントには意味がありません（通知は届かない）。
- `suspend` でフォローを失ったローカルの利用者には、失ったフォロワーとフォローの数を伝える通知（`relationships_severed`）が 1 件ずつ届きます（[切れたフォロー](#切れたフォロー)）。
- 発行と解除は、ローカルのアカウントへの処分と同じく[監査ログ](#監査ログ)に残ります。

```sh
my admin account action issue -kind suspend -category spam -public-reason "spam" -duration 720h @alice
my admin account action issue -kind limit -report <report id> @bob@remote.example
my admin account action issue -kind delete_statuses -status <status id> -status <status id> @alice
my admin account action issue -kind ban -category legal -public-reason "illegal content" @alice
my admin account action list -active @alice
my admin account action lift -reason "appealed" <action id>
```

管理 API では `POST /api/v1/admin/accounts/{id}/actions`（本文は `kind`・`reason_category`・`public_reason`・`private_note`・`report_id`・`notify_target`・`expires_in` 秒・`status_ids`）、
`GET /api/v1/admin/accounts/{id}/actions`（`active` で絞り込み）、`GET /api/v1/admin/account_actions/{id}`、`POST .../{id}/lift`（本文 `{"reason": "..."}` は省略可）です。

- アカウントは CLI では `@username`（ローカル）、`@username@domain`（リモート）かアカウント ID で、管理 API ではアカウント ID で指定します。
- 複数の処分は重なります（制限の和集合）。解除はその処分の制限だけを消し、期限付きの処分は期限で自然に終わります（一覧では `expired`）。
- `disable_login`・`suspend`・`ban` が失効させたセッションとトークンは、解除しても戻りません。本人はもう一度ログインします。
- ローカルのアカウントの `suspend` と `ban` は、フォロワーのサーバーと既知のサーバーに、プロフィールを空にして凍結を示す Actor の `Update` を送ります（Mastodon では「凍結」と表示される）。
  解除と期限切れでは元のプロフィールの `Update` を送ります。期限切れは 5 分ごとのジョブが送るので、期限から最大 5 分遅れます（[federation.md §11.1](../architecture/federation.md#111-処分の伝播)）。
- `delete_statuses` と `mark_statuses_sensitive` は、対象のアカウントの削除されていない投稿を 1〜100 件、`-status`（`status_ids`）で指定します。期限は付けられません。
  `delete_statuses` を解除しても消した投稿は戻りません。`mark_statuses_sensitive` を解除すると元の表示に戻ります。
- `ban` は期限を付けられず、解除もできません。取り消せるのは、削除までの 30 日のあいだに本人が申し立てた異議を覆したときだけです（[異議申し立て](#異議申し立て)）。迷うときは期限付きの `suspend` を使います。
  処分から 30 日後に、1 時間ごとのジョブがアカウントを削除します（アカウントと投稿が表示から消え、連合先に `Delete` を送ります）。
- `report_id`（`-report`）は、対象のアカウントの通報だけを指定できます。未解決の通報は、処分と同時に `actioned` で解決されます（監査ログにも解決が残ります）。解決済みの通報は変わりません。
- `public_reason` は本人に見せる理由、`private_note` はモデレーターだけのメモです。`notify_target`（既定は真、CLI では `-no-notify` で偽）は本人に知らせるかどうかです。

### 本人に見えるもの

- `notify_target` が真の処分は、本人に通知（`moderation_notice`）として届きます。通知には処分の種類・理由のカテゴリ・`public_reason`・期限・状態が入り、種類とカテゴリは本人の UI 言語の表示名でも返ります。
- 本人は `GET /api/v1/moderation/actions`（スコープ `read:accounts`、`active` で絞り込み）で自分への処分を新しい順に確かめられます。`notify_target` が偽の処分も、解除・期限切れの処分も並びます。
- `private_note`・通報・処分した人・解除の理由は、本人には一切見せません。
- 本人は自分への処分に、処分ごとに 1 回、異議を申し立てられます（[異議申し立て](#異議申し立て)）。処分の一覧と通知の処分には、申し立てられるか（`appealable`）・期限（`appeal_deadline`）・申し立てとその結果（`appeal`）が入ります。
- `disable_login`・`suspend`・`ban` を受けた本人は、アプリと API を使えませんが、ログイン画面（`/auth/sign_in`）にはログインでき、効いている `disable_login` / `suspend` / `ban` の種類・理由・期限（`ban` はアカウントが削除されるまでの日数）を確かめ、その画面から異議を申し立てられます（[異議申し立て](#異議申し立て)）。認可（`/oauth/authorize`）はこの画面を `403` で返し、トークンを発行しません。
  `ban` を受けた本人がログインできるのはアカウントが削除されるまでで、削除はそのセッションも失効させます。本人が自分で削除を予約したアカウントと無効にしたアカウントは、ログイン画面にもログインできません。

### 異議申し立て

ローカルの利用者は、自分への処分に異議を申し立てられます（FR-707、[ADR-0038](../adr/0038-moderation-personal-data-retention.md)）。
モデレーターはそれを審査し、維持（`uphold`）するか覆す（`overturn`）かを決めます。

```sh
my admin appeal list -state pending
my admin appeal uphold -reason "The posts were spam." <appeal id>
my admin appeal overturn -reason "Sorry, our mistake." <appeal id>
```

本人は `POST /api/v1/moderation/actions/{id}/appeal`（スコープ `write:accounts`、本文 `{"text": "..."}`）で申し立てます。
アクセストークンを得られない `disable_login` / `suspend` / `ban` の本人は、ログイン画面の処分ごとのフォーム（`POST /auth/appeal`）から申し立てます。
画面は、申し立てられる処分にはフォームと期限を、申し立て済みの処分にはその内容と結果を、期限切れの処分や申し立てられない処分にはその理由を示し、断った申し立てには理由を示します。
管理 API では `GET /api/v1/admin/appeals`（`state` で絞り込み）、`GET .../{id}`、`POST .../{id}/uphold`・`POST .../{id}/overturn`（本文 `{"public_reason": "..."}` は省略可）で、権限 `manage_accounts` を要します。

- 申し立ては処分 1 件につき 1 回、本文は 1〜2,000 文字です。期限は処分からインスタンス設定 `appeal_window` の日数（既定 180 日、20〜365 日、[インスタンス設定](instance-settings.md)）です。
  期限を過ぎた処分と 2 回目の申し立ては `422`（`/api/v1/problems/appeal-window-closed`・`/api/v1/problems/already-appealed`）です。
- `ban` には、アカウントが削除されるまで（処分から 30 日、`appeal_window` が短ければその期限まで）申し立てられます。削除済みのアカウントの `ban` は `422`（`/api/v1/problems/not-appealable`）です。解除済み・期限切れの処分にも、期限内なら申し立てられます。
- 申し立てると、`manage_accounts` を持つ人に通知（`admin_appeal`）が届きます。審査すると、本人に通知（`appeal_decided`）が届き、本人は結果と `public_reason` を見られます。審査した人は見えません。
- 覆すと、同じトランザクションで処分を解除します（`lift` と同じ効果。解除の理由は空）。`delete_statuses` を覆しても消した投稿は戻りません。審査までに解除済みの処分は、そのままです。
- `ban` を覆すと、予定されていたアカウントの削除も同じトランザクションで取り消し（監査ログは `account_deletion_cancelled`）、凍結が終わって連合先にも元のプロフィールの `Update` を送ります。失効したセッションとトークンは戻らないので、本人はもう一度ログインします。審査までにアカウントが削除されていれば、覆せません（`422`、`/api/v1/problems/not-appealable`。維持はできる）。
- 審査は 1 回です。同じ結果でもう一度審査しても何も変わらず、反対の結果は `409`（`/api/v1/problems/appeal-decided`）です。

## アカウントの管理

アカウントの検索・詳細・IP アドレスとメールアドレスの表示は管理 API で行い、権限 `manage_accounts` を要します（FR-714）。

- `GET /api/v1/admin/accounts` は、ローカルとリモートのアカウントを新しい順に返します（インスタンス Actor は除く）。
  ユーザー名の前方一致（`username`）・ドメイン（`domain`）・ローカルかリモートか（`origin`）・状態（`state`）と、ローカルのアカウントのメールアドレス（`email`）・メールのドメイン（`email_domain`）・登録かログインに使った IP アドレス（`ip`）で絞り込めます。
  `email`・`email_domain`・`ip` は 1 つだけ指定できます。一覧には IP アドレスとメールアドレスを含めません。
- 状態は次のうち最初に当てはまるものです: `deleted`（削除）、`suspended`（`suspend`・`ban`）、`disabled`（`disable_login`）、`rejected`・`pending`（登録審査）、`limited`（`limit`）、`active`。
  状態と制限（`restrictions`）はアカウント自身への処分だけから決まり、ドメイン単位の処分は含みません。
- `GET /api/v1/admin/accounts/{id}` は、状態・制限と、ローカルのアカウントでは登録の申請と審査・メールの確認・ロール、新しい順に 20 件までの処分と通報を返します。
- `POST /api/v1/admin/accounts/{id}/reveal` は、ローカルのアカウントのメールアドレス、登録とログイン（新しい順に 20 件）の IP アドレスと User-Agent、同じ IP アドレスを使ったほかのアカウント（IP アドレスごとに 20 件まで）を返します。
  呼ぶたびに監査ログに `revealed_private_data` を記録し、記録できたときだけ返します。IP アドレスは保持期間（既定 180 日）を過ぎると消えるので、それより前のログイン元は表示できません。
- `GET /api/v1/admin/accounts/{id}` の `two_factor_enabled` は、ローカルのアカウントの利用者が二要素認証（TOTP、FR-005）を使っているかを示します。
  認証アプリとリカバリーコードを両方なくした利用者は、`POST /api/v1/admin/accounts/{id}/two_factor/disable` で二要素認証を外すとパスワードだけでログインできます。
  外せるのは権限 `manage_login_recovery` を持つ利用者で、既定では組み込みの管理者のロールだけが持ちます（モデレーターは状態を見られますが、外せません。ADR-0058 のサーバー管理）。
  外すと監査ログに `two_factor_disabled` を記録します。本人であることは、メールなど別の経路で確かめてから外してください。
  ほかに管理者のいない管理者自身がなくしたときは、運営者が CLI の `my admin account disable-two-factor @alice` で外します（監査ログの操作者は CLI）。
- パスワードを忘れた利用者は、メールを送るサーバーではログインの画面の「パスワードを忘れたとき」から、新しいパスワードを設定するリンクをメールで受け取ります（FR-002）。
  メールを送らないサーバー（`mail = none`）や、メールが届かない利用者には、管理者がリンクを発行して渡します。
  管理画面ではアカウントのページの「パスワードのリセット」（`POST /api/v1/admin/accounts/{id}/password_reset_link`）で発行します。
  発行できるのは二要素認証の解除と同じ権限 `manage_login_recovery` を持つ利用者で、既定では組み込みの管理者のロールだけが持ちます（リンクを持つ人はアカウントに入れるので、モデレーターの `manage_accounts` では発行できません。ADR-0058 のサーバー管理）。
  発行したリンクはその場で一度だけ表示し、コピーできます。サーバーはリンクのハッシュしか保存しないので、あとから見ることはできません。必要ならもう一度発行します。
  管理者のいないときや管理画面を使えないときは、運営者が CLI の `my admin account password-reset-link @alice` で発行します（監査ログの操作者は CLI）。
  リンクは 1 時間・1 回だけ使え、その後にパスワードがほかの方法で変わると使えなくなります。どちらで発行しても監査ログに `password_reset_link_issued` を記録します。
  リンクを持つ人は誰でもパスワードを設定できるので、本人であることをメールなど別の経路で確かめてから、本人にだけ渡してください。
  新しいパスワードを設定すると、その利用者のすべてのセッションとアプリのトークンが失効します。パスキーと二要素認証はそのまま残ります。
- 検索は 1 ページを埋めるのに 400 アカウントまで読みます。条件に合うアカウントが少ないと、短いページや空のページにも `next_cursor` が付くので、続けて読みます。

## ドメイン単位の処分

| 種類 | 効果 |
| --- | --- |
| `suspend` | 配送・取得・受信をすべて止める（受信は署名の検証より前に `403`）。既存のアカウントと投稿はタイムラインやプロフィールから隠れる。ローカルの利用者とそのドメインのアカウントのフォローを両方向とも切る（相手には何も送らない） |
| `limit` | 投稿を公開タイムライン・検索に出さず、フォローしていない利用者への通知を作らない |
| `reject_media` | メディアを取得しない |
| `reject_reports` | そのドメインからの通報（`Flag`）を捨てる |
| `force_sensitive` | メディアをセンシティブとして表示する |

```sh
my admin domain issue -kind suspend -public-comment "spam" spam.example
my admin domain issue -kind limit -duration 720h noisy.example
my admin domain list -active
my admin domain lift <action id>
```

管理 API では `POST /api/v1/admin/domain_actions`（本文は `domain`・`kind`・`public_comment`・`private_note`・`obfuscate`・`expires_in` 秒）、
`GET /api/v1/admin/domain_actions`（`domain`・`active` で絞り込み）、`GET .../{id}`、`POST .../{id}/lift` です。

- ドメインは IDNA の A-label（小文字、末尾のドットなし）に正規化して記録します。`例え.jp` と `xn--r8jz45g.jp` は同じドメインです。
- 処分はサブドメインにも効きます（`spam.example` の処分は `a.spam.example` にも効く）。
- 処分は記録したトランザクションで制限の投影（`domain_restrictions`）とタイムラインの制限に反映され、次の配送・取得・受信から効きます。
- 期限（`-duration` / `expires_in`）を付けた処分は期限で自然に終わります。期限切れは「解除」ではないので、一覧では `expired` と表示します。
- 解除はその処分の制限だけを消します。同じドメインへの別の処分は残ります。解除済みの処分をもう一度解除しても何も変わりません。
- 一覧は新しい順で、`-active`（`active=true`）を付けると効いているものだけを出します。

### Mastodon の CSV でのインポートとエクスポート

ドメインへの処分の一覧を、Mastodon のドメインブロックの CSV（`#domain,#severity,#reject_media,#reject_reports,#public_comment,#obfuscate`）で受け渡しできます（FR-706）。
Mastodon がエクスポートした CSV をインポートでき、このサーバーがエクスポートした CSV は Mastodon でインポートできます。

```sh
my admin domain export > domain_blocks.csv
my admin domain import domain_blocks.csv                       # 何をするかを表示するだけ
my admin domain import -apply -only spam.example,bad.example domain_blocks.csv
```

管理 API では `GET /api/v1/admin/domain_actions/export`（`text/csv`）、`POST .../import/preview`（本文は `csv`）、`POST .../import`（本文は選んだ行の `blocks` と `private_note`）です。
Web クライアントでは、ドメインへの処分の画面（`/moderation/domain-actions`）の「CSV のインポート」と「CSV のエクスポート」で行います。

| CSV の行 | このサーバーの処分 |
| --- | --- |
| `#severity` が `suspend`（空のときも） | `suspend` |
| `#severity` が `silence` | `limit` |
| `#severity` が `noop` | なし |
| `#reject_media` / `#reject_reports` が真 | それぞれ `reject_media` / `reject_reports` を足す |

- インポートは 2 段階です。まずプレビューで行ごとに出す処分を確かめ、残す行を選んでからインポートします（CLI は `-apply` を付けるまで何も変えず、`-only` で行のドメインを選びます）。
- そのドメインか親ドメインに同じ種類の有効な処分がある行は、その種類を出しません（すべてあれば「適用済み」）。同じドメインの 2 行目・不正な行・何も出さない行（`noop` だけ）も飛ばします。同じ CSV をもう一度インポートしても処分は増えません。
- `#domain` の見出しのない CSV は、1 行に 1 つのドメインの一覧として、すべて `suspend` にします（Mastodon と同じ）。1 回に 5,000 行までです。
- インポートした処分は「解除するまで」で、行の公開コメントと伏せ字、指定した非公開のメモを持ちます。1 件ずつ出した処分と同じく監査ログに載ります（操作者は API ならその人、CLI なら `cli`）。
- エクスポートは効いている処分を、ドメインごとに 1 行にします。`suspend` があれば `suspend`、`limit` があれば `silence`、どちらもなければ `noop` です。Mastodon のドメインブロックにない `force_sensitive` だけのドメインは出しません。非公開のメモと期限は出しません。

### 切れたフォロー

ドメインの `suspend` とリモートのアカウントの `suspend` は、ローカルの利用者と凍結したアカウントのフォローを両方向とも切ります（FR-717）。

- ドメインの `suspend` は、記録した後のジョブ（`moderation.sever_domain`）がローカルの利用者 1 人ずつ切ります。ジョブが終わるまでは、フォローが一覧に残ります。サブドメインのアカウントも切ります。
- リモートのアカウントの `suspend` は、処分を記録したトランザクションで切ります。すでに凍結中のアカウントへの 2 つ目の `suspend` は何も切りません。
- 承認済みのフォローを失った利用者ごとに 1 件、失ったフォロワーとフォローの数を伝える通知（`relationships_severed`）が届きます。フォローリクエストだけを失った利用者には届きません。
- 処分を解除しても、切ったフォローは戻りません。利用者はフォローし直します。
- 同じ処分を処理し直しても（ジョブの再実行など）、切れた関係の記録と通知は増えません。

### 処分したドメインの公開

効いている `limit` と `suspend` の処分は、`GET /api/v1/instance/domain_actions` で公開できます（FR-718）。
誰に見せるかはインスタンス設定 `domain_actions_visibility`（`none` / `users` / `all`、既定 `none`）で、
公開コメントを誰に見せるかは `domain_actions_rationale_visibility`（同じ値、既定 `none`）で決めます（[インスタンス設定](instance-settings.md)）。

```sh
my instance set domain_actions_visibility users
my instance set domain_actions_rationale_visibility users
```

- `none` では `404`、`users` ではアクセストークンの無い要求に `401` を返します。
- 処分ごとに 1 項目で、新しい順です。ドメイン・ドメインの SHA-256（16 進）・種類と、見せてよい相手には公開コメントを返します。非公開のメモは返しません。
- `-obfuscate`（`obfuscate`）を付けた処分は、ドメインの中ほど半分の文字（ドット以外）を `*` に置き換えて公開します（`bad.example` → `bad.*****le`）。
  知っているドメインの SHA-256 と比べれば、伏せたドメインかどうかを確かめられます。
- `reject_media`・`reject_reports`・`force_sensitive` の処分と、解除・期限切れの処分は公開しません。
- Web クライアントのインスタンスの情報ページ（`https://<host>/about`）は、この一覧を同じ設定に従って出します。`none` では一覧の区画を出さず、`users` ではサインインしていない人に、ログインした利用者だけが見られることを出します。

## リモートの投稿の取り直し

保存したリモートの投稿を、相手のサーバーから取り直せます（FR-407、[federation.md §6.4](../architecture/federation.md#64-投稿の取り直し)）。
取り直しは、管理 API・CLI・Web クライアントから頼んだときだけ行い、定期的には行いません。

```sh
my admin refetch status <投稿の ID か URI>
my admin refetch account @bob@remote.example
my admin refetch domain remote.example
```

管理 API では `POST /api/v1/admin/statuses/{id}/refetch`・`POST /api/v1/admin/accounts/{id}/refetch_statuses`・`POST /api/v1/admin/instances/{domain}/refetch_statuses` で、権限 `manage_domains` を要します。
Web クライアントでは、アカウントの画面（`/moderation/accounts/{id}`、リモートのアカウント）と連合先の画面（`/moderation/instances/{domain}`）の「投稿を取り直す」で行います。

- 投稿は、受け取った `Update` と同じく扱います。新しい版があれば版を足し、投票の票数を更新し、変わっていなければ何もしません。相手が 410 Gone か `Tombstone` を返した投稿は削除します。取得できない投稿（404 を含む）はそのまま残します。
- アカウントは、削除されていない保存済みの投稿の新しいものから 1,000 件までを、100 件ずつ 1 分おきに取り直します。
- ドメインは、そのドメインのアカウントの新しいものから、アカウントごとに新しい投稿 20 件まで、合わせて 1,000 件までを、5 アカウントずつ 1 分おきに取り直します。
- ドメインへの `suspend` があるドメインと、配送の失敗で `unavailable` のドメインからは取得しません。同じ URI の取得の失敗は 1 時間覚えているので、その間は取り直しても取得しません。
- 頼んだことは監査ログに `status_refetch_queued`・`account_statuses_refetch_queued`・`domain_statuses_refetch_queued` として残ります。

## 通報

他のサーバーから受信した通報（`Flag`）は、未解決（`open`）として通報の一覧に入ります。

```sh
my admin report list -state open
my admin report show <report id>
my admin report assign <report id> <username>
my admin report unassign <report id>
my admin report resolve -outcome dismissed <report id>
my admin report note <report id> "以前の通報と同じ相手"
my admin report reopen <report id>
```

管理 API では `GET /api/v1/admin/reports`（`state` で絞り込み）、`GET .../{id}`、`POST .../{id}/assign_to_self`、`POST .../{id}/unassign`、
`POST .../{id}/resolve`（本文 `{"outcome": "actioned" | "dismissed"}`）、`POST .../{id}/notes`（本文 `{"body": "..."}`）、`POST .../{id}/reopen` です。

- 担当（`assign`）は、ロールが `manage_reports` を持つローカルの利用者にだけ割り当てられます。管理 API では自分を担当にします。
- 解決（`resolve`）の結果は、対象に処分をした `actioned` か、処分の必要がなかった `dismissed` です。解決済みの通報をもう一度解決しても結果は変わりません。
- 通報から処分を出す（処分の `report_id` / `-report`）と、その通報は同じ操作で `actioned` として解決されます。
- メモ（`note`）はモデレーターだけが見る内部メモで、2,000 文字までです。対象にも通報者にも見せません。編集・削除はできません。
- 再開（`reopen`）は、解決済みの通報を `open` に戻します（担当は残ります）。未解決の通報は再開できません（管理 API では `409`）。
- 通報の詳細（`show`・`GET .../{id}`）は、メモを古い順に、対象のほかの通報の数と対象への処分の数（解除・期限切れを含む）とともに返します。一覧はメモを返しません。
- 担当・解決・再開・メモはイベント（`report_assignments`・`report_resolutions`・`report_reopenings`・`report_notes`）として残り、現在の状態は `report_states` が持ちます。
- ローカルの通報者には、最初の解決で通知（`report_resolved`）が届きます。再開してもう一度解決しても、通知は増えません。
  通報者は自分の通報を `GET /api/v1/reports` で一覧でき、未解決か解決済みかだけが見えます。解決の結果・処分の内容・対象への理由・担当は見えません（FR-711）。

## モデレーターへの通知と未対応の件数

新しい通報と承認待ちの登録と異議申し立ては、権限を持つ利用者にアプリ内の通知で届きます（FR-712）。

| 通知 | 届くとき | 届く人 |
| --- | --- | --- |
| `admin_report` | 通報の対象に、ほかの未解決の通報が無いとき（同じアカウントへの 2 件目以降の通報では届かない） | ロールが `manage_reports` を持つ人。通報者と通報の対象は除く |
| `admin_sign_up` | 登録が承認待ちで始まったとき（承認制、またはメールを送れない構成の `open`） | ロールが `manage_accounts` を持つ人 |
| `admin_appeal` | 処分に異議が申し立てられたとき | ロールが `manage_accounts` を持つ人。申し立てた本人は除く |
| `admin_custom_emoji_request` | カスタム絵文字の作成か画像の差し替えが申請されたとき（[ADR-0070](../adr/0070-permission-split-emoji-finance.md)） | ロールが `manage_custom_emojis` を持つ人。申請者は除く |

- 届く人は、通知を作るときの権限で決まります。後から権限を失った人の一覧と未読数からは、その種類の通知が消えます。権限を戻せばまた見えます。
- 利用者設定 `email_moderator_notifications`（`PATCH /api/v1/preferences`、既定は無効）を有効にした人には、同じ通知をメールでも送ります。
  Web クライアントでは、設定の「表示と投稿」の「モデレーターへの通知」で切り替えます。この区画は通知が届く権限を持つ人にだけ出ます。
  メールは通知を記録したトランザクションでジョブを積み、コミット後にジョブが送ります。送る時点で権限か設定を失っていれば送りません。
  メールは通知の種類と通知の画面へのリンクだけを書き、通報やアカウントの中身は書きません。`mail = none` ではメールを送りません。
  そのとき `GET /api/v1/instance` の `sends_email` は `false` になり、Web クライアントは切り替えの代わりにその理由を出します。

`GET /api/v1/admin/counts` は、未解決の通報の数（`open_reports`、`manage_reports` が要る）と、承認待ちの登録の数（`pending_registrations`）と審査待ちの異議申し立ての数（`pending_appeals`）を返します（どちらも `manage_accounts` が要る）。
権限の無い項目は返しません。

## 監査ログ

モデレーターと管理者の操作は、操作と同じトランザクションで監査ログ（`audit_log_entries`）に 1 行ずつ記録されます（FR-704、[ADR-0037](../adr/0037-audit-log.md)）。
失敗した操作と、何も変えなかった操作（解除済みの処分の解除、解決済みの通報の解決、同じ担当への割り当て、廃止済みのルールの廃止、位置の変わらない移動、持っているロールの付与、持っていないロールの剥奪）は記録しません。

| 対象の種類 | 操作 |
| --- | --- |
| `account` | `account_action_issued`・`account_action_lifted`（アカウントへの処分の発行・解除）・`account_deletion_cancelled`（`ban` の異議申し立てを覆したことによる削除の取り消し）・`revealed_private_data`（IP アドレスとメールアドレスの表示）・`two_factor_disabled`（二要素認証の解除）・`password_reset_link_issued`（パスワードリセットのリンクの発行）・`account_statuses_refetch_queued`（リモートのアカウントの投稿の取り直し） |
| `status` | `status_refetch_queued`（リモートの投稿の取り直し。対象の名前は投稿の URI） |
| `domain` | `domain_action_issued`・`domain_action_lifted`（ドメイン単位の処分の発行・解除）・`domain_statuses_refetch_queued`（ドメインのアカウントの投稿の取り直し） |
| `report` | `report_assigned`・`report_unassigned`・`report_resolved`・`report_reopened`・`report_note_added`（通報の担当・担当の解除・解決・再開・メモ） |
| `user` | `registration_approved`・`registration_rejected`（登録審査） |
| `setting` | `setting_changed`（インスタンス設定）・`text_changed`（インスタンスの文章） |
| `rule` | `rule_added`・`rule_revised`・`rule_moved`・`rule_retired`（インスタンスのルール） |
| `role` | `role_granted`・`role_revoked`（利用者へのロールの付与・剥奪）・`role_created`・`role_updated`・`role_deleted`（ロールの作成・変更・削除） |
| `appeal` | `appeal_upheld`・`appeal_overturned`（異議申し立ての審査。覆したときは処分の `account_action_lifted`、`ban` なら `account_deletion_cancelled` も記録する） |
| `signup_block` | `signup_block_added`・`signup_block_lifted`（登録の拒否の追加・解除） |
| `custom_emoji` | `custom_emoji_created`・`custom_emoji_image_replaced`・`custom_emoji_copied`・`custom_emoji_updated`・`custom_emoji_enabled`・`custom_emoji_disabled`・`custom_emoji_deleted`（カスタム絵文字の登録・画像の差し替え・ほかのサーバーの絵文字の複製・変更・有効化・無効化・削除） |
| `webhook` | `webhook_created`・`webhook_updated`・`webhook_deleted`・`webhook_enabled`・`webhook_disabled`・`webhook_secret_rotated`（webhook の設定・変更・削除・有効化・無効化・秘密の作り直し） |
| `announcement` | `announcement_created`・`announcement_updated`・`announcement_published`・`announcement_withdrawn`（お知らせの作成・変更・公開・取り下げ） |
| `invite` | `invite_created`・`invite_revoked`（招待の作成・失効。対象の名前はコードの先頭 4 文字） |
| `billing_source` | `billing_import_queued`（利用料金の取り込みを手で始めた。対象の名前は事業者の名前） |
| `cost_item` | `cost_item_created`・`cost_item_relabeled`・`cost_item_price_revised`・`cost_item_end_changed`・`cost_item_canceled`（費用の登録・名前などの変更・金額の改定・終了日の変更・取り消し。対象の名前は項目の名前） |
| `supporter_program` | `supporter_program_enabled`・`supporter_program_disabled`（課金の有効化・無効化。対象の名前は決済の事業者の名前） |
| `supporter_plan` | `supporter_plan_created`・`supporter_plan_revised`・`supporter_plan_retired`（サポーターのプランの作成・変更・廃止。対象の名前はプランの名前） |
| `supporter_subscription` | `supporter_subscription_canceled`（管理者による加入の解約。対象の名前はプランの名前と利用者のハンドル） |
| `custom_emoji_request` | `custom_emoji_request_submitted`・`custom_emoji_request_withdrawn`・`custom_emoji_request_approved`・`custom_emoji_request_rejected`（カスタム絵文字の申請・取り下げ・承認・却下。対象の名前は `:shortcode:`） |
| `payment_provider` | `payment_reconcile_queued`（決済の事業者との照合を手で始めた。対象の名前は事業者の名前） |
| `oauth_client` | `oauth_client_disabled`・`oauth_client_enabled`（OAuth のクライアントの無効化と取り消し。無効化は全トークンを失効させる。対象の名前はクライアントの名前と `client_id`） |
| `oauth_client_host` | `oauth_client_host_denied`・`oauth_client_host_allowed`（Client ID Metadata Document のホストの拒否と取り消し。拒否はそのホストのクライアントのトークンを失効させる。対象の名前はホスト） |

一覧は、対象が今もあるかを示します。保持期間を過ぎて消えたアカウント・通報・異議申し立て・登録の拒否と、削除されたロール・カスタム絵文字・webhook は、記録時の名前だけを出し、画面へのリンクを出しません。

```sh
my admin audit list
my admin audit list -actor alice -action domain_action_issued
my admin audit list -target-kind domain -target-id spam.example
```

管理 API では `GET /api/v1/admin/audit_log`（`actor_account_id`・`action`・`target_kind`・`target_id` で絞り込み）で、権限 `view_audit` を要します。

- 1 行は操作者（`user` ならその利用者、CLI なら `cli`）・操作・対象の種類と ID・対象の表示名・元の記録の ID・日時です。
  対象の ID は、アカウント・通報・異議申し立て・利用者・ルール・ロール・登録の拒否の ID、ドメイン単位の処分ではドメイン、設定と文章ではキーです。
- 対象の表示名（`@alice`・`@bob@example.com`・ドメイン・設定のキー・ルールの文。文章はキー、ロールはキーと利用者のハンドル、登録の拒否は種類と値）は記録したときの写しなので、対象が消えた後も読めます。
  API の各行は対象が今もあるか（`target_exists`）も返し、消えたアカウント・通報・異議申し立て・登録の拒否では `false` です。Web の監査ログは、消えた対象をリンクにせず表示名だけを出します。
- 理由とメモの本文は写しません。元の記録（処分・担当・解決・再開・メモ・審査・設定やルールの版）の ID から引きます。
- 一覧は新しい順のキーセットページングです（CLI は `-before <entry id>`）。
- 監査ログを更新・削除する API と CLI はありません。古い行は[保持期間](#記録の保持期間と保全)を過ぎると消えます。

## 記録の保持期間と保全

モデレーションの記録は、種類ごとの保持期間を過ぎると、1 時間ごとのジョブ（`moderation.purge_records` と `identity.purge_origins`）が消します（[ADR-0038](../adr/0038-moderation-personal-data-retention.md)、[データモデル §8](../architecture/data-model/08-deletion-and-purge.md#8-削除と消去)）。
期間はインスタンス設定で変えられますが、下限より短くはできません（[インスタンス設定](instance-settings.md)）。変えた期間は次の回から効きます。

| 記録 | 設定 | 既定 | 下限 | 起点 |
| --- | --- | --- | --- | --- |
| ログイン元の IP アドレスと User-Agent | `retention_session_origins` | 180 日 | 90 日 | ログイン |
| 登録元の IP アドレス | `retention_registration_origins` | 180 日 | 90 日 | 登録 |
| 通報（コメント・対象の投稿・メモ・担当・解決・再開） | `retention_reports` | 365 日 | 180 日 | 最後の解決 |
| アカウントへの処分（理由・メモ・対象の投稿・解除）と異議申し立て | `retention_moderation_actions` | 365 日 | 180 日 | 処分の終わり |
| 監査ログ | `retention_audit_log` | 730 日 | 365 日 | 記録 |
| 登録の拒否 | なし（変えられない） | 30 日 | — | 解除と期限切れの早い方 |

- 未解決の通報は消えません。処分が参照する通報（処分の `report_id`）は、その処分が消えるまで残ります。
- 処分の終わりは、解除と期限切れの早い方です。異議が申し立てられた処分は、審査がそれより後ならその審査の時刻で、審査待ちの間は消えません。
  `warn` と `delete_statuses` は効果が続かないので、出した時刻が終わりです。期限のない処分（`ban`、期限なしの `suspend` など）は、解除されるまで消えません。
- 通報と処分が指す投稿は、作者が削除していても、その通報・処分が消えるまで消去ジョブが添付のメディアごと物理削除しません（表示からは消えます）。リモートの投稿は、保持期間（設定 `remote_status_max_retention` を含む）を過ぎても残ります。
- 解除も期限も無い登録の拒否は、効いている間ずっと残ります。
- 1 回のジョブが消すのは、種類ごとに上限のある件数です（処分・通報・登録の拒否は 100 件ずつ 20 回、監査ログは 1,000 行ずつ 20 回）。残りは次の回が消します。

### 保全

裁判所の消去禁止命令（情報流通プラットフォーム対処法 16 条）や、開示請求の対応中には、アカウント単位で消去を止められます。

```sh
my admin retention hold -reason "court order 2026-123" @alice
my admin retention list -account @alice
my admin retention release <hold id>
```

- 保全のあるアカウントの、処分と異議申し立て・そのアカウントの通報とそのアカウントが出した通報・それらの監査ログ・ログイン元と登録元の IP アドレス・投稿は、保持期間を過ぎても消えません。
  そのアカウントの BAN から作った登録の拒否も、解除・期限切れから 30 日を過ぎても消えません。
- 1 つのアカウントに複数の保全を置けます。すべてを解除すると、期間を過ぎた記録が次の回に消えます。解除済みの保全をもう一度解除しても何も変わりません。
- 保全とその解除は CLI だけで行い、記録の操作者は CLI です。
- 削除したアカウント（`ban` の期日を過ぎたものと、本人が削除したもの）のデータは、削除から 7 日後に 1 時間ごとのジョブ `account.erase_deleted` が消します（[データモデル §8](../architecture/data-model/08-deletion-and-purge.md#8-削除と消去)）。
  保全のあるアカウントは消しません。通報と処分が指す投稿は、その記録が消えるまで残ります。アカウントの URI・ユーザー名と処分の記録は残ります。

### 開示請求への書き出し

開示請求（GDPR 15 条・個人情報保護法 33 条）には、アカウントのモデレーションの記録をすべて JSON で書き出して応じます。

```sh
my admin account export-moderation @alice > alice-moderation.json
```

- 書き出すのは、そのアカウントへの処分（非公開のメモ・解除・異議申し立てと審査を含む）、そのアカウントの通報（コメント・対象の投稿・引用したルール・モデレーターのメモ・担当・解決・再開）、そのアカウントが出した通報（未解決か解決済みかだけ）、それらの監査ログです。
- 通報者の身元（通報したアカウントと、受信した通報の URI・転送元のサーバー）は書き出しません。通報のコメントに通報者を特定できることが書かれていないかは、渡す前に確かめてください。
- モデレーターは利用者 ID で書き出します。書き出しの対応中に記録が消えないよう、先に保全を置いてください。
