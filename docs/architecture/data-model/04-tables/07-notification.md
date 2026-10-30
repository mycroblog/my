### 4.7 notification

| テーブル | 分類 | カラム | 制約 |
| --- | --- | --- | --- |
| `notifications` | イベント | `id`, `recipient_account_id`, `type`（`enum(follow, follow_request, follow_accepted, mention, boost, favourite, reaction, poll_closed, moderation_notice, appeal_decided, report_resolved, relationships_severed, admin_report, admin_sign_up, admin_appeal, admin_webhook_disabled, admin_custom_emoji_request, custom_emoji_request_decided, supporter_payment_failed, supporter_subscription_ended, supporter_plan_retired, account_moved, admin_account_moved, service_closure, admin_service_closure_due, supporter_service_closure, voice_room_started, voice_room_invited, admin_error_group, status)`）, `source_account_id`（NULL 可。モデレーションの通知はインスタンス）, `source_domain`（NULL 可。差出人がリモートのアカウントのときのそのドメイン。ドメインのミュートの照合に使う）, `status_id`（NULL 可）, `moderation_action_id`（NULL 可）, `report_id`（NULL 可）, `appeal_id`（NULL 可）, `severance_id`（NULL 可）, `subject_user_id`（NULL 可。`admin_sign_up` の登録者）, `custom_emoji_request_id`（NULL 可。`admin_custom_emoji_request` と `custom_emoji_request_decided` の申請）, `supporter_subscription_id`（NULL 可。サポーターの通知の加入）, `reaction_emoji` / `reaction_custom_emoji_id`（NULL 可。`reaction` の絵文字で、Unicode 絵文字かカスタム絵文字のどちらか一方）, `account_move_id`（NULL 可。`account_moved` と `admin_account_moved` の移転）, `account_move_outcome`（NULL 可。`account_moved` のフォローの行方、`enum(followed, requested, blocking, blocked_by, domain_muted)`）, `service_closure_event_id`（NULL 可。サービスの終了の通知の `service_closure_events` の行）, `service_closure_change`（NULL 可。`service_closure` の伝える変化、`enum(announced, rescheduled, withdrawn)`）, `voice_room_id`（NULL 可。通話の通知のルーム）, `error_group_change`（NULL 可。`admin_error_group` の伝えること、`enum(created, regressed, others)`）, `error_group_id`（NULL 可。`created`・`regressed` のエラーのグループ）, `error_group_hour`（NULL 可。`others` がまとめる 1 時間の始まり）, `dedupe_key`, `created_at` | 一意 `(recipient_account_id, dedupe_key)`。インデックス `(recipient_account_id, id)`, `(recipient_account_id, type, id)`, `(status_id)`（保持期間の判定） |
| `error_group_notice_hours` | 現在値 | `hour_start`（PK。UTC の 1 時間の始まり）, `notices`（`bigint`） | `admin_error_group` の上限の数え（ADR-0103 §8）。`notices > 0` |
| `notification_dismissals` | イベント | `notification_id`（PK）, `dismissed_at` | |
| `notification_read_markers` | 現在値 | `account_id`（PK）, `last_read_id` | 未読数は `id > last_read_id` で数える |
| `notification_exclusions` | 投影 | `viewer_account_id`, `target_account_id`, `reason`（`enum(block, blocked_by, mute)`）, `expires_at`（NULL 可） | PK `(viewer_account_id, target_account_id, reason)`。`graph` のブロック・ミュートのイベントから作る |
| `notification_domain_exclusions` | 投影 | `viewer_account_id`, `domain` | PK `(viewer_account_id, domain)`。`graph` のドメインのミュートのイベントから作る。サブドメインにも効く（FR-304） |
| `notification_deleted_statuses` | 投影 | `status_id`（PK） | `content.status_deleted` から作る。その投稿についての通知があるときだけ（削除された投稿に後から通知は付かない） |
| `push_subscriptions` | 現在値 | `id`, `account_id`, `oauth_token_family_id`, `endpoint`, `key_p256dh`, `key_auth`, `created_at`, `updated_at` | 一意 `(endpoint)`, 一意 `(oauth_token_family_id)`。インデックス `(account_id)`（FR-206） |
| `push_subscription_alerts` | 現在値 | `subscription_id`, `notification_type` | PK `(subscription_id, notification_type)` |
| `post_subscriptions` | 現在値 | `account_id`（購読する人）, `target_account_id`（対象のアカウント）, `created_at` | PK `(account_id, target_account_id)`。CHECK `account_id <> target_account_id`。インデックス `(account_id, created_at, target_account_id)`（購読の一覧。新しい順のキーセット）、`(target_account_id, account_id)`（投稿の通知の購読者の走査）。1 人あたり 1,000 件まで（ADR-0123） |
| `post_subscription_requests` | 現在値 | `account_id`, `target_account_id`, `created_at` | PK `(account_id, target_account_id)`。インデックス `(target_account_id, account_id)`。承認待ちのリモートのフォローと同時に頼まれた購読 |
| `notification_settings` | 現在値 | `account_id`, `notification_group`（`enum(mention, follow, follow_request, follow_accepted, boost, favourite, reaction, quote, poll_closed, account_moved, voice, admin_report, admin_sign_up, admin_appeal, admin_webhook_disabled, admin_custom_emoji_request, admin_account_moved, admin_service_closure_due, admin_error_group, moderation, status)`）, `channel`（`enum(in_app, email)`）, `enabled`（`bool`）, `updated_at` | PK `(account_id, notification_group, channel)`。CHECK `moderation` は `email` だけ（FR-210、ADR-0109 §6） |

- 通知は `notification` の同期 projector が、`graph` と `content` のドメインイベントから同じトランザクションで作ります（§2.1）。

  | ドメインイベント | 種類 | 受け取る人 | `source_account_id` | `dedupe_key` |
  | --- | --- | --- | --- | --- |
  | `graph.followed` | `follow` | フォローされた人 | フォローした人 | `follow:<follow_id>` |
  | `graph.follow_requested` | `follow_request` | フォローされた人 | フォローした人 | `follow_request:<follow_id>` |
  | `graph.follow_accepted` | `follow_accepted` | フォローした人 | 承認した人 | `follow_accepted:<follow_id>` |
  | `content.status_created` / `content.status_revised` | `mention` | 版がメンションした人ごと | 投稿者 | `mention:<status_id>` |
  | `content.status_created`（ローカルの作成と配送された `Create`。ジョブ `notification.notify_post_subscribers` が作る） | `status` | 投稿者の購読者のうち投稿を見られる人 | 投稿者 | `status:<status_id>` |
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
  | サービスの終了の予告の開始による解約（`supporters.closure_notice_cancellations`。`pending` を除く続いている加入、ADR-0088 §2） | `supporter_service_closure` | 加入者のローカルのアカウント | `supporter_service_closure:<event_id>:<subscription_id>` |
  | サービスの終了の予告・日付の変更・取り消し（`instance.closure_recorded`、ADR-0088 §2） | `service_closure` | ローカルの全利用者（インスタンス Actor を除く） | `service_closure:<event_id>` |
  | 効いている予告の閉鎖日を過ぎた（ADR-0088 §1） | `admin_service_closure_due` | ロールが `manage_service_closure` を持つ人 | `admin_service_closure_due:<announcement_event_id>` |
  | 新しいエラーのグループ（ログインした報告が作った `diagnostics.error_group_created` か、`diagnostics.error_group_confirmed`。ADR-0103 §8） | `admin_error_group`（`created`） | ロールが `manage_errors` を持つ人 | `admin_error_group:created:<group_id>` |
  | 解決したグループの再発（ログインしていない報告を除く `diagnostics.error_group_regressed`） | `admin_error_group`（`regressed`） | ロールが `manage_errors` を持つ人 | `admin_error_group:regressed:<event_id>` |
  | 1 時間の 11 件め以降のグループ | `admin_error_group`（`others`） | ロールが `manage_errors` を持つ人 | `admin_error_group:others:<hour_start>` |

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
  - サービスの終了の通知（ADR-0088）の行は `service_closure_event_id` を必ず持ち、`source_account_id` を持ちません。ほかの種類は持ちません。`service_closure_change` は `service_closure` だけが持ちます。
    `service_closure` は、予告・日付の変更・取り消しを記録したトランザクションで積むジョブ（`notification.service_closure_fanout`、冪等キーはイベントの ID）が、ローカルのアカウントを 100 件ずつ通知します。
    `admin_service_closure_due` は、予告と日付の変更のたびに閉鎖日に動くよう積むジョブ（`notification.service_closure_due`）が、その予告がまだ効いていてクローズしていないときだけ作ります（サーバーは閉鎖日に自動ではクローズしないため、クローズの操作を促す）。メールはモデレーターの通知と同じです。
    `supporter_service_closure` は加入を `supporter_subscription_id` にも持ち、サポーターの通知と同じくメールでも送ります。クライアント API は `service_closure` に、伝える変化と予告の日付（閉鎖日・持ち出しの期限。取り消しにはない）を載せます。予告・日付の変更・取り消しのメールは別に送ります（ADR-0086）。
  - `admin_error_group`（ADR-0103 §8）は、エラーのグループのドメインイベントと同じトランザクションで、`error_group_notice_hours` のその 1 時間の行に 1 を足し（upsert。同時のトランザクションも 1 件ずつ足す）、足した後の数が 10 以下ならそのグループを、ちょうど 11 なら `others` の通知を 1 つ作ります。12 以降は数えるだけで、`others` の件数はクライアント API が読むときの `notices - 10` です。
    ログインしていない Web クライアントの報告だけが作ったグループは、ログインした報告が確かめたとき（`diagnostics.error_group_confirmed`）に知らせ、そのときミュートしていれば知らせません。ログインしていない報告による再発は知らせません。同じ規則で `error_group.*` の webhook も送ります。
    定期のジョブ `notification.prune_error_group_notices`（24 時間ごと）が、始まりが 90 日より前の時間の `error_group_notice_hours` を 1 文で消します（`error_group_counts` と同じ期間）。それより古い `others` の通知の件数は 0 になります。
    Web Push は `/admin/errors`（グループの通知は `/admin/errors/<group_id>`）を開き、メールはモデレーターの通知と同じです。
  - 通話の通知（ADR-0100 §8）は、`voice` が `notification` の公開 API（`VoiceNotifier`）を自分のトランザクションの中で呼んで作ります（`notification` は `voice` を import しない）。行は `voice_room_id` と `source_account_id` を必ず持ち、ほかの種類は `voice_room_id` を持ちません。
    `voice_room_started` は、ルームを開いたトランザクションで積むジョブ（`voice.notify_started`、冪等キーはルーム）が、ホストのローカルのフォロワーを 100 件ずつ、開いた時の公開範囲といまの公開範囲の両方で `CanSee` が真の人に作ります。差出人はホスト、`dedupe_key` は `voice_room_started:<room_id>` で、ルームと受け取る人ごとに 1 つです。終わったルームとホストが変わったルームでは、残りを作りません。
    `voice_room_invited` は招待のトランザクションで作ります。差出人は招待した人、`dedupe_key` は `voice_room_invited:<invitation_id>` です。どちらにもミュート・ブロック・`unsolicited_reach` の除外がほかの通知と同じに効きます。
    クライアント API は `voice_room_id` を載せ、ルームが開いていて閲覧者に見えるときだけ `voice_room` にルームを埋め込みます。Web Push はルームのページ（`/voice/<room_id>`）を開きます。
  - `admin_report` は、通報を受け付けたトランザクションで対象にほかの未解決の通報が無いときだけ作り（`moderation.report_filed` の `FirstOpen`）、通報者と通報の対象には作りません。
  - `admin_sign_up` は `identity.registration_pending`（登録が `pending` で始まった）から作ります。
  - 受け取る人がその種類をアプリ内でオフにしていれば作りません（下の通知の設定）。メールがオン（既定）の受け取る人には、新しく作った通知ごとに、同じトランザクションの outbox でメールのジョブ（`notification.moderator_email`、冪等キーは通知の ID）を積み、未読を待たずに送ります。
    ジョブは送る時点で権限と設定とあて先（確認済みで、抑止されていない主のアドレス）を確かめ、通知の種類と通知の画面へのリンクだけを書いたメールを `Mailer` で送ります。`mail = none` ではジョブを積みません。
- 新しい投稿の購読（ADR-0123）は、ローカルの利用者が対象のアカウントごとに `post_subscriptions` に持ちます。購読は相手に知らせず、API・ActivityPub のどれにも出しません。
  - ローカルの相手はフォローを問わず購読でき、リモートの相手は承認済みでフォローしている間（`graph` の `Follows` が真）だけ購読できます。自分自身は CHECK と `notification.ErrSelfSubscription`、どちらかがブロックしている相手（`graph` の `BlockedEitherWay`）とフォローしていないリモートの相手は `notification.ErrNotSubscribable` で断ります。上限を超える購読は、挿入した後の件数で `notification.ErrSubscriptionLimit` にしてトランザクションを戻します。
  - クライアント API の `notify`（`FollowAccountRequest`・`Relationship`）と CSV の `Notify on new posts` は購読を読み書きします。承認待ちのリモートのフォローと同時の `notify: true` は `post_subscription_requests` に記録し、`graph.follow_accepted` の projector が購読に変えます（上限に達していれば作らない）。承認待ちの間、`Relationship.notify` は偽です。
  - `graph.follow_terminated`（理由を問わない）は同じ組の依頼を消し、相手がリモートなら購読も消します。ローカルの相手の購読は、フォローを解除しても残ります。`graph.follow_rejected` は依頼を消し、`graph.blocked` は両方向の購読と依頼を消します。
  - 通知 `status` は、ローカルの作成と配送された `Create`（`Update` を `Create` として取り込んだものを含む）の取り込みで発行される `content.status_created` の projector が、投稿者に購読者がいるとき、同じトランザクションでジョブ（`notification.notify_post_subscribers`、冪等キーは投稿）を積んで作ります。取りに行った投稿（版の `RemoteRevisionSource.ActivityURI` が空）と編集（`content.status_revised`）では積みません。ブーストは投稿ではないので対象外です。
  - ジョブは、他人への返信（返信先の投稿者が投稿者と違うか、返信先がこのサーバーにない）なら何もしません。そうでなければ購読者を 100 件ずつ、1 回ごとに短いトランザクションで、投稿がメンションした人を除き、単一の公開範囲のポリシー関数で投稿を見られる人に作ります。差出人は投稿者、`dedupe_key` は `status:<status_id>` で、投稿と受け取る人ごとに 1 つです。ブロック・ミュート・ドメインのミュート・`unsolicited_reach`・アプリ内の設定の除外は、ほかの通知と同じに効きます。
  - `status` の設定のグループは普段の通知の 1 行で、アプリ内は既定オン、メールの経路を持ちません（`Rule.Absent`。行を持てず、メールのジョブを積まない）。Web Push は購読の `alerts` に `status` があるときに送り、投稿の画面を開きます。
  - アカウントの消去は、その人の購読と依頼、その人への購読と依頼も消します（§8）。
- 通知の設定（FR-210、ADR-0109 §2・§3・§6）は、利用者が選んだ（種類のグループ、経路）だけを `notification_settings` に持ち、行が無ければグループの既定に従います。グループと既定、固定の経路はドメインの `notification.Groups` が決めます。
  - グループは、普段の通知が種類ごと（通話の `voice_room_started`・`voice_room_invited` は `voice` にまとめる）、モデレーター向けが種類ごと、インスタンスからの知らせが `moderation`（`moderation_notice`・`report_resolved`・`appeal_decided`・`relationships_severed`・`custom_emoji_request_decided`）・`service_closure`・`supporter`（`supporter_*`）です。
  - 既定は、普段の通知がアプリ内オン・メールオフ、モデレーター向けがどちらもオン、`moderation` がアプリ内固定・メールオフ、`service_closure` と `supporter` がどちらも固定です。固定の経路は行を持ちません（CHECK と `Setting.Validate` で拒否）。
    クライアント API は、権限の無いモデレーター向けの行と、サポーターの機能が無効のときの `supporter` の行を返しません。
  - 通知を作る projector は、同じトランザクションで受け取る人の設定を読み、その種類のアプリ内がオフなら作りません（Web Push・メールも届かない）。オフにする前の通知は消しません。管理画面の未対応の件数は通報や登録の状態から数えるので変わりません。
  - 普段の通知と `moderation` は、メールがオンなら作った通知ごとに同じトランザクションでジョブ（`notification.email`、冪等キーは通知の ID、実行は 15 分後）を積みます。
    ジョブは送る時点で、設定のメールがまだオン、通知が未読（`notification_read_markers` より新しく、消していない）、まだ見える（Web Push と同じ `Listed` と投稿の可視性）、あて先がある（確認済みで抑止されていない主のアドレス）ときだけ、Web Push と同じ文面を件名にしたメールを 1 通送ります。
  - 配信停止のリンク（ADR-0086 §4）の区分はグループで、使うとそのグループのメールの行を `enabled = false` で書きます。
- Web Push の購読（FR-206）は、OAuth のグラント（トークン系列。リフレッシュで変わらない）ごとに 1 つです。ブラウザの `PushSubscription` のエンドポイントと鍵（`key_p256dh`・`key_auth` は RFC 8291 の鍵をパディングなしの base64url で）と、送る通知の種類（`push_subscription_alerts`）を持ちます。
  - 登録は、同じグラントの購読と同じエンドポイントの購読を置き換えます（どちらも一意）。同じグラントが同じエンドポイントで種類だけを変えたときは、ID と作成時刻を保ちます。
    行は通知を受け取るアカウント（`account_id`）に結び付けるので、通知の作成のトランザクションの中で、受け取る人のアカウントと種類から購読を引けます。
  - 通知を作った projector は、同じトランザクションの outbox で、受け取る人の購読のうちその種類を受け取るものごとに送信のジョブ（`notification.push`、冪等キーは通知と購読の ID）を積みます。
    ジョブは購読が残っていて、その種類を受け取り、グラントが有効（失効していないトークンが系列にあり、リフレッシュトークンの期限内）なときだけ送ります。グラントが終わっていれば（ログアウト・処分・パスワードの変更）購読を消します。
    プッシュサービスが 404 / 410 を返した購読も消します。再送された同じ通知は、同じ `Topic`（通知の ID）でプッシュサービスの未配達の通知を置き換え、Service Worker が同じ `tag` で表示を置き換えます。
  - アカウントの消去は、そのアカウントの購読と種類も消します（§8）。
