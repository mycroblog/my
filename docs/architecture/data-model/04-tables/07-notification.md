### 4.7 notification

| テーブル | 分類 | カラム | 制約 |
| --- | --- | --- | --- |
| `notifications` | イベント | `id`, `recipient_account_id`, `type`（`enum(follow, follow_request, follow_accepted, mention, boost, favourite, reaction, poll_closed, moderation_notice, appeal_decided, report_resolved, relationships_severed, admin_report, admin_sign_up, admin_appeal, admin_webhook_disabled, admin_custom_emoji_request, custom_emoji_request_decided, supporter_payment_failed, supporter_subscription_ended, supporter_plan_retired, account_moved, admin_account_moved)`）, `source_account_id`（NULL 可。モデレーションの通知はインスタンス）, `source_domain`（NULL 可。差出人がリモートのアカウントのときのそのドメイン。ドメインのミュートの照合に使う）, `status_id`（NULL 可）, `moderation_action_id`（NULL 可）, `report_id`（NULL 可）, `appeal_id`（NULL 可）, `severance_id`（NULL 可）, `subject_user_id`（NULL 可。`admin_sign_up` の登録者）, `custom_emoji_request_id`（NULL 可。`admin_custom_emoji_request` と `custom_emoji_request_decided` の申請）, `supporter_subscription_id`（NULL 可。サポーターの通知の加入）, `reaction_emoji` / `reaction_custom_emoji_id`（NULL 可。`reaction` の絵文字で、Unicode 絵文字かカスタム絵文字のどちらか一方）, `account_move_id`（NULL 可。`account_moved` と `admin_account_moved` の移転）, `account_move_outcome`（NULL 可。`account_moved` のフォローの行方、`enum(followed, requested, blocking, blocked_by, domain_muted)`）, `dedupe_key`, `created_at` | 一意 `(recipient_account_id, dedupe_key)`。インデックス `(recipient_account_id, id)`, `(recipient_account_id, type, id)`, `(status_id)`（保持期間の判定） |
| `notification_dismissals` | イベント | `notification_id`（PK）, `dismissed_at` | |
| `notification_read_markers` | 現在値 | `account_id`（PK）, `last_read_id` | 未読数は `id > last_read_id` で数える |
| `notification_exclusions` | 投影 | `viewer_account_id`, `target_account_id`, `reason`（`enum(block, blocked_by, mute)`）, `expires_at`（NULL 可） | PK `(viewer_account_id, target_account_id, reason)`。`graph` のブロック・ミュートのイベントから作る |
| `notification_domain_exclusions` | 投影 | `viewer_account_id`, `domain` | PK `(viewer_account_id, domain)`。`graph` のドメインのミュートのイベントから作る。サブドメインにも効く（FR-304） |
| `notification_deleted_statuses` | 投影 | `status_id`（PK） | `content.status_deleted` から作る。その投稿についての通知があるときだけ（削除された投稿に後から通知は付かない） |
| `push_subscriptions` | 現在値 | `id`, `account_id`, `oauth_token_family_id`, `endpoint`, `key_p256dh`, `key_auth`, `created_at`, `updated_at` | 一意 `(endpoint)`, 一意 `(oauth_token_family_id)`。インデックス `(account_id)`（FR-206） |
| `push_subscription_alerts` | 現在値 | `subscription_id`, `notification_type` | PK `(subscription_id, notification_type)` |

- 通知は `notification` の同期 projector が、`graph` と `content` のドメインイベントから同じトランザクションで作ります（§2.1）。

  | ドメインイベント | 種類 | 受け取る人 | `source_account_id` | `dedupe_key` |
  | --- | --- | --- | --- | --- |
  | `graph.followed` | `follow` | フォローされた人 | フォローした人 | `follow:<follow_id>` |
  | `graph.follow_requested` | `follow_request` | フォローされた人 | フォローした人 | `follow_request:<follow_id>` |
  | `graph.follow_accepted` | `follow_accepted` | フォローした人 | 承認した人 | `follow_accepted:<follow_id>` |
  | `content.status_created` / `content.status_revised` | `mention` | 版がメンションした人ごと | 投稿者 | `mention:<status_id>` |
  | `content.boosted` | `boost` | 元投稿の作者 | ブーストした人 | `boost:<status_id>:<account_id>` |
  | `content.favourited` | `favourite` | 投稿の作者 | お気に入りした人 | `favourite:<status_id>:<account_id>` |
  | `content.reacted` | `reaction` | 投稿の作者 | リアクションした人 | `reaction:<status_id>:<account_id>:<emoji_key>` |

  - `dedupe_key` は受け取る人ごとに一意で、挿入は `ON CONFLICT DO NOTHING` です。同じイベントを再処理しても、同時に処理しても、通知は 1 つです。
    フォローし直すと新しいフォローなので通知し直し、ブースト・お気に入りの取り消しと繰り返しでは通知し直しません。リアクションは、同じ絵文字の付け直しでは通知し直さず、別の絵文字なら新しく通知します。編集では、新しくメンションされた人だけに届きます。
  - 受け取るのはローカルのアカウントだけです（リモートの人には相手のサーバーが通知します）。自分の操作は通知しません（CHECK `source_account_id <> recipient_account_id`）。
  - どちらかがもう一方をブロックしているとき、または受け取る人が相手を `hide_notifications` 付きで（期限切れでなく）ミュートしているときは作りません。
  - 受け取る人が差出人のドメイン（またはその親のドメイン）をミュートしているときも作りません（`notification_domain_exclusions`、FR-304）。差出人のドメインは `account` の `DomainOf` で調べ、通知の `source_domain` に記録します。
  - 差出人に `unsolicited_reach`（本人かドメインへの `limit`、§5.2）があり、受け取る人が差出人を承認済みでフォローしていないときも作りません。承認待ちのフォローはフォローに数えません。
  - 受け取る人がローカルかどうか（`account` の `IsLocal`）、ブロック・ミュート（`graph` の `HidesNotifications`）、受け取る人が差出人をフォローしているか（`graph` の `Follows`）は、他モジュールのテーブルを直接読まず、そのモジュールのアプリケーション層の公開 API をイベントのトランザクションの中で呼んで調べます。
    差出人の制限は `notification` のポート `ReachPolicy`（`moderation.AccountPolicy` の `UnsolicitedReachRestricted`、§5.3）で調べます。
    通知のイベントの時点で有効なミュートとブロックの有無はそのイベントに載らない事実なので、§2.1 の「イベントの内容だけで更新する」の例外です。
- 後からのブロック・ミュートは、作成済みの通知を読み取り時に隠します。`notification_exclusions` は `graph` のイベントから同期 projector が作る投影です。
  - `graph.blocked` は、ブロックした人から相手への `block` と、相手からブロックした人への `blocked_by` の 2 行を作り、`graph.unblocked` はその 2 行を消します。両方向にブロックしていれば、片方を解除してももう一方の行が残ります。
  - `graph.muted` は、`hide_notifications` 付きなら `mute` の行をミュートの期限で作り（あれば期限を置き換え）、付いていなければ消します。`graph.unmuted` は `mute` の行を消します。
  - 一覧と未読数は、`source_account_id` を `target_account_id` とする受け取る人の行のうち、読み取り時点で有効なもの（`expires_at` が NULL か現在時刻より後）がある通知を除きます。解除や期限切れで行が無効になれば、その通知はまた見えます。差出人の無い通知（`moderation_notice`）は除きません。
  - 作成時の判定は残します。ミュート中に起きたことは、解除しても通知になりません。
  - `graph.domain_muted` は `notification_domain_exclusions` に行を作り、`graph.domain_unmuted` は消します。一覧と未読数は、`source_domain` がその行のドメインかそのサブドメインである通知を除きます。
- 投稿が削除されたら、その投稿についての通知（メンション・ブースト・お気に入り）は読み取り時に隠します。
  - 通知は【イベント】なので、projector は通知の行を消しません。`content.status_deleted` を購読する同期 projector が `notification_deleted_statuses` に投稿の `status_id` を記録し、一覧と未読数は `status_id` がそこにある通知を除きます（同じ削除を再処理しても 1 行です）。
  - 行を物理削除するのは消去ジョブだけです（§1.4、§8）。表示から消えるのは削除のトランザクションで即時、物理削除は非同期です。
- 一覧は、消していない、除外されていない、削除された投稿についてでない通知を `id` のキーセットで新しい順に引きます。種類の絞り込み（複数可）は `(recipient_account_id, type, id)` を使います。
- 既読位置は前にだけ進みます（`last_read_id` より古い通知を指定しても変わりません）。同時に既読位置を送る複数の端末が位置を戻さないためです。指定できるのは自分の通知だけです。
- 未読数は、一覧に出る通知のうち `id > last_read_id` のものの件数です。既読位置が無ければすべてが未読です。
- モデレーションの通知は `moderation` と `identity` のドメインイベントから、カスタム絵文字の申請の通知は `content` のドメインイベントから、サポーターの通知は `supporters` のドメインイベントから作ります。

  | ドメインイベント | 種類 | 受け取る人 | `dedupe_key` |
  | --- | --- | --- | --- |
  | 処分の記録（`moderation.account_action_issued` のうち `notify_target` が真のもの） | `moderation_notice` | 対象のローカルのアカウント | `moderation_notice:<action_id>` |
  | 異議申し立ての審査 | `appeal_decided` | 申し立てた本人 | `appeal_decided:<appeal_id>` |
  | 通報の解決（最初の解決だけ） | `report_resolved` | ローカルの通報者 | `report_resolved:<report_id>` |
  | 関係の切断 | `relationships_severed` | 関係を失ったローカルのアカウント | `relationships_severed:<severance_id>` |
  | 通報の受け付け（同じ対象に未解決の通報が無いときだけ） | `admin_report` | ロールが `manage_reports` を持つ人 | `admin_report:<report_id>` |
  | 承認待ちの登録 | `admin_sign_up` | ロールが `manage_accounts` を持つ人 | `admin_sign_up:<user_id>` |
  | 異議申し立て | `admin_appeal` | ロールが `manage_accounts` を持つ人 | `admin_appeal:<appeal_id>` |
  | サーバーによる webhook の無効化（`webhook.endpoint_disabled`、ADR-0060） | `admin_webhook_disabled` | ロールが `manage_webhooks` を持つ人 | `admin_webhook_disabled:<state_change_id>` |
  | `limit` の処分中のアカウントの移転（`account.moved` の `SourceLimited`、ADR-0089） | `admin_account_moved` | ロールが `manage_accounts` を持つ人 | `admin_account_moved:<move_id>` |
  | カスタム絵文字の申請（`content.custom_emoji_request_recorded` の申請、ADR-0070 §9） | `admin_custom_emoji_request` | ロールが `manage_custom_emojis` を持つ人（申請者を除く） | `admin_custom_emoji_request:<request_id>` |
  | カスタム絵文字の申請の承認と却下（同じイベントの承認・却下。取り下げと `lapsed` は通知しない） | `custom_emoji_request_decided` | 申請者 | `custom_emoji_request_decided:<request_id>` |
  | 加入の遷移のうち `past_due` に入ったもの（`supporters.subscription_transitioned`、ADR-0072 §16） | `supporter_payment_failed` | 加入者のローカルのアカウント | `supporter_payment_failed:<transition_id>` |
  | 加入の遷移のうちロールのある状態（`active`・`canceling`・`past_due`）から `lapsed`・`expired`・`revoked` に入ったもの（アカウントの削除による終わりは除く） | `supporter_subscription_ended` | 加入者のローカルのアカウント | `supporter_subscription_ended:<transition_id>` |
  | プランの廃止（`supporters.plan_retired`。`pending` を除く続いている加入） | `supporter_plan_retired` | 加入者のローカルのアカウント | `supporter_plan_retired:<plan_id>:<subscription_id>` |

  - 本人は `moderation_notice` から処分の種類・公開理由・対象の投稿・期限を見られ、異議申し立てに進めます（FR-715）。
    クライアント API は通知に処分を本人に見せる形（種類と理由のカテゴリは本人の UI 言語の表示名付き）で埋め込み、`GET /api/v1/moderation/actions` は本人への処分を通知の有無に関わらずすべて新しい順に返します。
    どちらも非公開のメモ・通報・処分した人を含めません（ADR-0038）。`moderation_notice` の行は `moderation_action_id` を必ず持ち、ほかの種類は持ちません。
  - `appeal_decided` と `admin_appeal` の行は `appeal_id` を必ず持ち、ほかの種類は持ちません。クライアント API は `appeal_decided` に申し立てた処分を本人に見せる形（申し立てと審査の結果を含む）で埋め込み、`admin_appeal` には `appeal_id` だけを載せます。
    `admin_appeal` は申し立てた本人には作りません。
  - `report_resolved` は解決したことだけを伝え、処分の内容は含めません（FR-711）。
  - `admin_*` の受け取る人は、通知を作るトランザクションでロールの権限から求めます（`identity` の公開 API。ユーザーからローカルのアカウントへは `account` の公開 API で引く）。
    後から権限を失った人の通知は、一覧と未読数から読み取り時に除きます（閲覧者のロールが持たない権限の種類を除く）。権限が戻れば、また見えます。
  - `admin_*` の `source_account_id` は NULL で、ブロック・ミュートでは隠しません。`admin_report` は `report_id` を、`admin_sign_up` は `subject_user_id` を持ちます。
    `admin_webhook_disabled` は対象の列を持たず、管理画面の webhook の一覧（無効にした理由が出る）に導きます。
  - `admin_custom_emoji_request` と `custom_emoji_request_decided` の行は `custom_emoji_request_id` を必ず持ち、ほかの種類は持ちません。クライアント API はどちらにも申請をいまの状態（却下の理由を含む）で埋め込みます。
  - フォローしていたアカウントの移転は `graph.follow_migrated`（§4.5、ADR-0089）から `account_moved` を作ります。受け取る人はローカルのフォロワー、`source_account_id` は移転元、`account_move_id` は移転、`account_move_outcome` は移転先をフォローしたか（`followed` / `requested`）、しなかった理由（`blocking` / `blocked_by` / `domain_muted`）で、`dedupe_key` は `account_moved:<move_id>` です。
    フォロワーはもう移転元をフォローしていないことがあるので、移転元の `unsolicited_reach` では止めません。
  - `account_moved` と `admin_account_moved` の行は `account_move_id` を必ず持ち、ほかの種類は持ちません。`admin_account_moved` は `source_account_id` を持たず、`account_move_outcome` は `account_moved` だけが持ちます。クライアント API はどちらにも移転先を `account_move` に、`admin_account_moved` には移転元を `account` に載せます。
  - サポーターの通知の行は `supporter_subscription_id` を必ず持ち、`source_account_id` を持ちません。ほかの種類は `supporter_subscription_id` を持ちません。
    クライアント API は加入をいまの状態（猶予の終わり、終わった理由を含む）で本人にだけ埋め込みます。`past_due` のまま期間の終わりなどが変わった遷移と、`lapsed` から `expired` への遷移は、もう一度は通知しません。
    新しく作った通知ごとに、同じトランザクションの outbox でメールのジョブ（`notification.supporter_email`、冪等キーは通知の ID）を積み、ジョブは主アドレスへ受け取る人の言語で送ります（課金の有効化はメールを送る構成を要するので、いつも送れます）。
    Web Push と通知の文面は支払いの画面へ導かず、Web Push は通知の画面を開きます。Web クライアントだけが `/settings/supporter` へのリンクを出します（Tauri のアプリは出さない。ADR-0072 §14）。
  - `admin_report` は、通報を受け付けたトランザクションで対象にほかの未解決の通報が無いときだけ作り（`moderation.report_filed` の `FirstOpen`）、通報者と通報の対象には作りません。
  - `admin_sign_up` は `identity.registration_pending`（登録が `pending` で始まった）から作ります。
  - `user_preferences.email_moderator_notifications` が真の受け取る人には、新しく作った通知ごとに、同じトランザクションの outbox でメールのジョブ（`notification.moderator_email`、冪等キーは通知の ID）を積みます。
    ジョブは送る時点で権限と設定を確かめ、通知の種類と通知の画面へのリンクだけを書いたメールを `Mailer` で送ります。`mail = none` ではジョブを積みません。
- Web Push の購読（FR-206）は、OAuth のグラント（トークン系列。リフレッシュで変わらない）ごとに 1 つです。ブラウザの `PushSubscription` のエンドポイントと鍵（`key_p256dh`・`key_auth` は RFC 8291 の鍵をパディングなしの base64url で）と、送る通知の種類（`push_subscription_alerts`）を持ちます。
  - 登録は、同じグラントの購読と同じエンドポイントの購読を置き換えます（どちらも一意）。同じグラントが同じエンドポイントで種類だけを変えたときは、ID と作成時刻を保ちます。
    行は通知を受け取るアカウント（`account_id`）に結び付けるので、通知の作成のトランザクションの中で、受け取る人のアカウントと種類から購読を引けます。
  - 通知を作った projector は、同じトランザクションの outbox で、受け取る人の購読のうちその種類を受け取るものごとに送信のジョブ（`notification.push`、冪等キーは通知と購読の ID）を積みます。
    ジョブは購読が残っていて、その種類を受け取り、グラントが有効（失効していないトークンが系列にあり、リフレッシュトークンの期限内）なときだけ送ります。グラントが終わっていれば（ログアウト・処分・パスワードの変更）購読を消します。
    プッシュサービスが 404 / 410 を返した購読も消します。再送された同じ通知は、同じ `Topic`（通知の ID）でプッシュサービスの未配達の通知を置き換え、Service Worker が同じ `tag` で表示を置き換えます。
  - アカウントの消去は、そのアカウントの購読と種類も消します（§8）。
