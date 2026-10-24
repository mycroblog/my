### 4.3 moderation

#### 処分【イベント】

| テーブル | 分類 | カラム | 制約 |
| --- | --- | --- | --- |
| `moderation_actions` | イベント | `id`, `target_account_id`, `kind`（§5.2 の enum）, `reason_category`（`enum(spam, legal, violation, other)`）, `public_reason`（`text`、NULL 可。5,000 文字まで。本人に見せる理由）, `private_note`（`text`、NULL 可。5,000 文字まで）, `report_id`（NULL 可）, `issuer`（`enum(moderator, system, remote)`）, `issued_by_user_id`（NULL 可）, `notify_target`（`bool`）, `expires_at`（NULL 可 = 無期限）, `issued_at` | インデックス `(target_account_id, id DESC)`, `(report_id)` |
| `moderation_action_statuses` | イベント（子） | `action_id`, `status_id` | PK `(action_id, status_id)`。投稿単位の処分（`delete_statuses` / `mark_statuses_sensitive`）の対象。処分 1 件につき 1〜100 件で、処分の対象のアカウントの削除されていない投稿だけ（処分のユースケースが検査する）。ほかの種類の処分は持たない。インデックス `(status_id)`（投稿の消去で証拠を残す判定） |
| `moderation_action_lifts` | イベント | `action_id`（PK）, `lifted_by_user_id`（NULL 可 = システム）, `reason`（`text`、NULL 可。5,000 文字まで）, `lifted_at` | 期限切れは「解除」ではないので記録しない（投影の読み取り時に期限で判定） |
| `moderation_appeals` | リソース | `id`, `action_id`, `account_id`（申し立てた本人 = 処分の対象）, `body`（1〜2,000 文字。空白だけは不可）, `submitted_at` | 一意 `(action_id)`。ローカルのアカウントへの処分だけが対象。期間（ADR-0038、インスタンス設定 `appeal_window`、既定は処分から 180 日。`ban` はアカウントの削除の期日 = 処分から 30 日も過ぎない）を過ぎた処分と、アカウントが削除された `ban` には申し立てられない |
| `moderation_appeal_decisions` | イベント | `appeal_id`（PK）, `decision`（`enum(upheld, overturned)`）, `public_reason`（`text`、NULL 可。2,000 文字まで。本人に見せる）, `decided_by_user_id`（NULL 可 = CLI）, `decided_at` | 覆った場合は同じトランザクションで `moderation_action_lifts` も記録する。`delete_statuses` を覆しても投稿は戻らない |
| `appeal_states` | 投影 | `appeal_id`（PK）, `state`（`enum(pending, upheld, overturned)`）, `since` | インデックス `(state, appeal_id)`（審査待ち一覧） |

- 異議申し立ては `moderation.Service.FileAppeal` が記録し、同じトランザクションで `appeal_states` に `pending` を書いて `moderation.appeal_filed` を発行します（`admin_appeal` の通知、§4.7）。
  期限は処分の `issued_at` に、申し立ての時点の `appeal_window` を足した時刻です（期限ちょうどまで申し立てられる）。`ban` は `appeal_window` と `moderation.BanDeletionDelay`（30 日）の短い方で、アカウントが削除済み（`account_lifecycle_states.state = deleted`）なら申し立てられません（決定者の判断「30 日のあいだは、BAN への異議申し立てを受け付ける」）。解除済み・期限切れの処分にも、期限内なら申し立てられます。
- 審査（`moderation.Service.DecideAppeal`）は、`moderation_appeal_decisions` と `appeal_states` と監査ログ（`appeal_upheld` / `appeal_overturned`）を 1 つのトランザクションで書き、`moderation.appeal_decided` を発行します（`appeal_decided` の通知）。
  覆す（`overturned`）と、同じトランザクションで処分の解除（`moderation_action_lifts`、`lifted_by_user_id` は審査した人、`reason` は NULL）とその効果と監査ログ（`account_action_lifted`）を記録します。すでに解除されていた処分には何もしません。
  `ban` を覆すときは、その前に `account.Service.CancelDeletionByModeration` が `deletion_cancelled`（`initiated_by = moderator`、`moderation_action_id` は BAN）を記録し（監査ログ `account_deletion_cancelled`、`source_event_id` はその行）、解除で凍結が終わって `moderation.account_suspension_changed` が出ます。削除済みのアカウントの BAN は覆せません。`ban` を解除するのはこの経路だけで、`LiftAccountAction` は拒否します。
  審査は 1 回で、同じ結果の再審査は何もせず、反対の結果は拒否します（`moderation_appeal_decisions` の PK で、同時の審査も 1 つだけが記録される）。
- 本人には、自分の処分（§4.7 の `GET /api/v1/moderation/actions` と `moderation_notice`）に申し立ての期限・今申し立てられるか・申し立ての本文と状態・審査の `public_reason` を見せます。審査した人は見せません。

#### 制限【投影】

| テーブル | カラム | 制約 |
| --- | --- | --- |
| `account_restrictions` | `account_id`, `restriction`（§5.2 の能力の enum）, `source_action_id`, `effective_until`（NULL 可 = 無期限） | PK `(account_id, restriction, source_action_id)`。インデックス `(effective_until)`（期限切れの行の削除）。読み取りは `WHERE account_id = ? AND (effective_until IS NULL OR effective_until > :now)` |

- 1 つの処分は複数の制限を生みます（例: サイレンス → 3 つの制限、§5.2）。
- 複数の処分が重なった場合は、制限の和集合になります。解除すると、その処分由来の行だけが消えます。期限切れの行は、読み取り時の条件で無効になり、定期ジョブで掃除します。
  `account_restrictions` の期限切れの行は、5 分ごとのジョブ `moderation.expire_restrictions` が、1 トランザクションあたり件数を区切って消します。
- アカウントの凍結（`federation` の制限）が効き始めたとき（効いている `federation` の行が無かったアカウントに行ができた）と、終わったとき（`federation` の行が消え、効いている行が残らない）に、`moderation` は同じトランザクションで `moderation.account_suspension_changed` を発行します。
  終わりは、解除か期限切れの行の削除の、先に行を消した方が 1 回だけ伝えます。ローカルのアカウントなら `federation` がこれを受けて `Update`（Actor）を積みます（[federation.md §11.1](../../federation.md#111-処分の伝播)）。

#### ドメイン単位の処分

| テーブル | 分類 | カラム | 制約 |
| --- | --- | --- | --- |
| `domain_moderation_actions` | イベント | `id`, `domain`, `kind`（`enum(limit, suspend, reject_media, reject_reports, force_sensitive)`）, `public_comment` / `private_note`（NULL 可。5,000 文字まで）, `obfuscate`（`bool`）, `issued_by_user_id`（NULL 可 = CLI）, `expires_at`（NULL 可）, `issued_at` | インデックス `(domain, id DESC)` |
| `domain_moderation_action_lifts` | イベント | `action_id`（PK）, `lifted_by_user_id`（NULL 可 = CLI）, `lifted_at` | |
| `domain_restrictions` | 投影 | `domain`, `restriction`, `source_action_id`, `effective_until`（NULL 可） | PK `(domain, restriction, source_action_id)` |

- `domain` は正規化したホスト（IDNA の A-label、[federation.md §1.2](../../federation.md#ホスト名の正規化idn)）です。
- サブドメインにも効きます。判定は、対象ドメインとその親ドメイン群を列挙して `domain IN (…)` で引きます。
- 処分と同じトランザクションで、その処分が生む制限の行を `account_restrictions` / `domain_restrictions` に書きます。解除は同じトランザクションでその処分の行（`source_action_id`）を消します。解除の再実行は何もしません。
- ドメイン単位の制限を各アカウントの行に展開することはしません（件数が膨らむため）。判定時に `account_restrictions` と合成します（§5.3）。
- 処分したドメインの公開（FR-718）は、効いている `limit` / `suspend` の処分を `domain_moderation_actions` から新しい順に、処分ごとに 1 項目で返します。
  返すのはドメイン（`obfuscate` なら伏せ字）・ドメインの SHA-256・種類・`public_comment` で、`private_note` は返しません。
  誰に見せるかはインスタンス設定 `domain_actions_visibility` と `domain_actions_rationale_visibility`（§4.9）で決めます。

#### 通報

| テーブル | 分類 | カラム | 制約 |
| --- | --- | --- | --- |
| `reports` | リソース | `id`, `uri`（NULL 可。受信した `Flag` の `id`、`id` の無い `Flag` は受信の記録と同じ `urn:my:digest:…`）, `reporter_account_id`, `target_account_id`, `category`（`enum(spam, legal, violation, other)`）, `comment`（5,000 文字まで）, `forwarded`（`bool`）, `filed_at` | 一意 `(uri)`。インデックス `(target_account_id, id)`, `(reporter_account_id, id)`（通報者の一覧） |
| `report_statuses` | リソース（子） | `report_id`, `status_id` | PK `(report_id, status_id)`。インデックス `(status_id)`（投稿の消去で証拠を残す判定） |
| `report_rule_citations` | リソース（子） | `report_id`, `rule_id` | PK `(report_id, rule_id)`。ルールの文言が改訂されても `rule_id` は変わらない（§4.9） |
| `report_assignments` | イベント | `id`, `report_id`, `assignee_user_id`（NULL 可 = 担当解除）, `assigned_by_user_id`（NULL 可 = CLI）, `assigned_at` | インデックス `(report_id, id DESC)`。担当が変わらない割り当ては記録しない |
| `report_resolutions` | イベント | `id`, `report_id`, `outcome`（`enum(actioned, dismissed)`）, `resolved_by_user_id`（NULL 可 = CLI）, `resolved_at` | インデックス `(report_id, id DESC)`。処分との関連は `moderation_actions.report_id` で辿る。解決済みの通報の解決は記録しない |
| `report_reopenings` | イベント | `id`, `report_id`, `reopened_by_user_id`（NULL 可 = CLI）, `reopened_at` | インデックス `(report_id, id DESC)`。未解決の通報の再開は記録しない |
| `report_notes` | イベント | `id`, `report_id`, `author_user_id`（NULL 可 = CLI）, `body`（2,000 文字まで）, `written_at` | インデックス `(report_id, id)`。モデレーターだけが見る内部メモ。対象にも通報者にも見せない |
| `report_states` | 投影 | `report_id`（PK）, `state`（`enum(open, resolved)`）, `outcome`（`enum(actioned, dismissed)`、NULL 可。最後の解決の結果）, `assignee_user_id`（NULL 可）, `since`（受け付けた時刻か、解決した時刻か、再開した時刻） | インデックス `(state, report_id)`（未解決一覧）。`outcome` は `state = resolved` のときだけ値を持つ（`CHECK` 制約） |

- 通報者（`reporter_account_id`）は、ローカルの通報ならローカルのアカウント、受信した `Flag` なら送ってきた Actor（多くはリモートのインスタンス Actor）です。通報者は対象に見せません（ADR-0038）。
- ローカルの通報者は、自分の通報を `(reporter_account_id, id)` のキーセットで新しい順に引けます。見せるのは通報の内容と状態（`open` / `resolved`）だけで、`outcome`・担当・処分・メモは見せません（FR-711）。
- 解決はすべて、通報と解決を載せたドメインイベント（`moderation.report_resolved`）を同じトランザクションで発行します。`notification` はそこから `report_resolved` を作り、`dedupe_key` が通報ごとなので、再開のあとの解決では通知し直しません（§4.7）。
- `forwarded` は、ローカルの通報者がリモートのアカウントの通報で相手のサーバーへの転送を求めたかどうかです（受け付けた時点で決まる事実）。対象がローカルのアカウントなら、求めても `false` です。転送は受け付けのトランザクションで outbox に積みます（[ADR-0036](../../../adr/0036-moderation-federation-propagation.md)）。受信した `Flag` は常に `false` です。
- ローカルの通報のコメントは 1,000 文字、受信した `Flag` のコメントは 5,000 文字までです。`report_rule_citations` は `category = violation` のときだけ持てます。
- ローカルの通報が指せる投稿は、対象のアカウントの投稿のうち通報者が見られるもの（単一の閲覧判定 `CanView` を通す）を 20 件まで、引用できるルールは効いているもの（引退していないもの）を 20 件までです。自分自身は通報できません。
- 通報の状態の投影は、受け付けで `open`、解決で `resolved`（`outcome` を写す）、再開で `open` に戻します（担当は残します）。再開できるのは解決済みの通報だけです。
  処分（`moderation_actions.report_id` を持つもの）を記録すると、同じトランザクションでその通報を `actioned` で解決し、監査ログに `report_resolved` を残します。解決済みの通報を指す処分は、通報を変えません。
- モデレーターが開く通報の詳細は、メモを古い順に、対象のほかの通報の数（`reports` の `(target_account_id, id)`）と対象への処分の数（`moderation_actions`。解除・期限切れを含む）とともに示します。
- 通報の詳細は、通報が指す投稿（`report_statuses`）を公開範囲によらず示します。作者などが削除した投稿も、最後の版の内容を削除の時刻とともに示し、物理削除された投稿は省きます。
  これはモデレーターに閲覧判定 `CanView` を通さずに投稿を見せる経路で、`manage_reports` を持つ人に、その通報が指す投稿だけを返します（ほかの画面では、モデレーターにも公開範囲が効きます）。
  通報の対応の一部なので、監査ログには記録しません（ADR-0037 が記録する閲覧は、IP アドレスとメールアドレスの表示だけです）。
- 通報と処分が参照する投稿（`report_statuses`・`moderation_action_statuses`）は、作者が削除しても、参照する記録の保持期間まで消去ジョブの物理削除から外します（通報の詳細のほかの表示からは消えます。§8）。

#### 登録の拒否リスト（S）

| テーブル | 分類 | カラム | 制約 |
| --- | --- | --- | --- |
| `signup_blocks` | イベント | `id`, `kind`（`enum(email_domain, email_hash, ip_range)`）, `value`, `severity`（`enum(reject, require_approval)`）, `comment`（NULL 可。1,000 文字まで）, `moderation_action_id`（NULL 可。BAN から作られた場合）, `created_by_user_id`（NULL 可 = CLI）, `expires_at`（NULL 可）, `created_at` | インデックス `(kind, value)` |
| `signup_block_lifts` | イベント | `block_id`（PK）, `lifted_by_user_id`（NULL 可 = CLI）, `lifted_at` | |

- `email_domain` の `value` は IDNA の A-label のドメインで、サブドメインにも効きます。`email_hash` はメールアドレスの正規形（ドメインを A-label にしたうえで小文字にし、ローカル部の最初の `+` 以降を除く）の SHA-256（16 進の小文字）で、BAN したアカウントのアドレスを平文で残さずに再登録を防ぎます。`ip_range` はマスク済みの CIDR（IPv4 射影アドレスは IPv4 に直す）で、単一のアドレスはその長さのプレフィックスです。
- 管理 API・CLI で作る拒否は即時に効き、追加と解除をそれぞれ監査ログに記録します。解除済みの拒否の解除は何も変えず、記録もしません。
- 一覧からのまとめての追加（使い捨てメールのドメインの一覧など。1 回に `moderation.MaxSignupBlockImport` = 10,000 件まで）も、値ごとに 1 行の `signup_blocks` と 1 行の監査ログを書きます。読み込みの単位の行は持ちません。
  正しくない値・一覧の中で重なる値・同じ `(kind, value)` で同じかより強い `severity` の拒否が効いている値は足さずに行番号と理由を返し、500 件ずつの短いトランザクションで書きます（効いている拒否の確認も同じトランザクションの中で、インデックス `(kind, value)` を引きます）。
- 拒否は、解除か期限切れの早い方から 30 日（`moderation.SignupBlockRetention`。インスタンス設定ではない）を過ぎると、消去ジョブ `moderation.purge_records` が解除の行とともに消します（ADR-0038、[記録の保持期間](#記録の保持期間)）。解除も期限も無い拒否は、効いている間ずっと残ります。
- BAN は、モデレーターが求めたとき（`block_email`）に、対象の利用者の主メールアドレスの `email_hash`・`reject` の拒否を同じトランザクションで作り（`moderation_action_id` は BAN）、その追加も監査ログに記録します。ローカルアカウントの BAN 以外と、メールアドレスのない利用者には作れません。
- 登録画面からの登録は、効いている拒否（解除も期限切れもしていないもの）に当たれば、`reject` なら受け付けず、`require_approval` なら登録モードに関わらず `pending` で始めます。登録できなかった理由は利用者に詳しく示しません。

#### 監査ログ【イベント】

[ADR-0037](../../../adr/0037-audit-log.md) の記録です。

| テーブル | 分類 | カラム | 制約 |
| --- | --- | --- | --- |
| `audit_log_entries` | イベント | `id`, `actor_kind`（`enum(user, cli, system, remote)`）, `actor_user_id`（NULL 可）, `action`（操作の enum）, `target_kind`（`enum(account, status, domain, report, appeal, user, role, setting, rule, signup_block, custom_emoji, webhook, announcement, invite, relay, billing_source, cost_item, supporter_program, supporter_plan, supporter_subscription, custom_emoji_request, payment_provider, oauth_client, oauth_client_host)`）, `target_id`, `target_label`（`string(512)`。記録時の対象の表示名の写し）, `source_event_id`（NULL 可。元のイベントの行の ID）, `occurred_at` | 新しい順の一覧は PK を逆順に読む。インデックス `(target_kind, target_id, id DESC)`, `(actor_user_id, id DESC)`。CHECK `(actor_kind = 'user') = (actor_user_id IS NOT NULL)` |

- 各モジュールの管理のユースケースが元のイベントと同じトランザクションで発行するドメインイベント（`moderation.audit_recorded`）を、`moderation` の同期の購読者が書きます。理由とメモの本文は写さず、`source_event_id` で元のイベントから引きます。
  記録の ID はトランザクションの前に作り、再試行しても同じ行を書きます。何も変えなかった操作（解除済みの処分の解除など）は記録しません。
- `action` と対象の種類の対応、`target_id` と `target_label` の値は次のとおりです。CLI の操作は `actor_kind = cli` で、`actor_user_id` は NULL です。

  | `action` | `target_kind` | `target_id` | `target_label` | `source_event_id` |
  | --- | --- | --- | --- | --- |
  | `account_action_issued`, `account_action_lifted` | `account` | `accounts.id` | アカウントのハンドル | `moderation_actions.id`（解除の行もこの ID が PK） |
  | `domain_action_issued`, `domain_action_lifted` | `domain` | ドメイン | ドメイン | `domain_moderation_actions.id`（解除の行もこの ID が PK） |
  | `report_assigned`, `report_unassigned`, `report_resolved`, `report_reopened`, `report_note_added` | `report` | `reports.id` | 通報されたアカウントのハンドル | `report_assignments.id` / `report_resolutions.id` / `report_reopenings.id` / `report_notes.id` |
  | `registration_approved`, `registration_rejected` | `user` | `users.id` | ローカルアカウントのハンドル（`@alice`） | `registration_decisions.id` |
  | `setting_changed` | `setting` | 設定のキー | 設定のキー | `instance_setting_revisions.id` |
  | `text_changed` | `setting` | 文章のキー | 文章のキー（`terms`） | `instance_text_revisions.id` |
  | `rule_added`, `rule_revised`, `rule_moved` | `rule` | `instance_rules.id` | ルールの文 | `instance_rule_revisions.id`（移動は動かしたルールの版。動かしたルールの位置が変わらないときは最初に記録した版） |
  | `rule_retired` | `rule` | `instance_rules.id` | ルールの文 | `instance_rule_retirements.rule_id` |
  | `role_granted`, `role_revoked` | `role` | `roles.id` | ロールのキーと利用者のハンドル（`moderator @alice`） | `user_role_grants.id`（剥奪の行もこの ID が PK） |
  | `role_created`, `role_updated`, `role_deleted` | `role` | `roles.id` | ロールのキー | なし（元の記録を持たない操作） |
  | `oauth_client_disabled` | `oauth_client` | `oauth_clients.id` | クライアントの名前と `client_id` | `oauth_client_disablements.id` |
  | `oauth_client_enabled` | `oauth_client` | `oauth_clients.id` | クライアントの名前と `client_id` | なし（取り消しの行は無効化の行の ID が主キー） |
  | `oauth_client_host_denied` | `oauth_client_host` | ホスト | ホスト | `oauth_client_host_denials.id` |
  | `oauth_client_host_allowed` | `oauth_client_host` | ホスト | ホスト | なし |
  | `appeal_upheld`, `appeal_overturned` | `appeal` | `moderation_appeals.id` | 申し立てたアカウントのハンドル | `moderation_appeal_decisions.appeal_id`（= `moderation_appeals.id`） |
  | `revealed_private_data` | `account` | `accounts.id` | ローカルアカウントのハンドル（`@alice`） | なし（元の記録を持たない操作） |
  | `two_factor_disabled` | `account` | `accounts.id` | ローカルアカウントのハンドル（`@alice`） | `totp_enrollment_revocations.enrollment_id` |
  | `password_reset_link_issued` | `account` | `accounts.id` | ローカルアカウントのハンドル（`@alice`） | `user_tokens.id` |
  | `signup_block_added`, `signup_block_lifted` | `signup_block` | `signup_blocks.id` | 種類と値（`email_domain spam.example`） | `signup_blocks.id`（解除の行もこの ID が PK） |
  | `custom_emoji_created`, `custom_emoji_image_replaced`, `custom_emoji_copied`, `custom_emoji_updated`, `custom_emoji_enabled`, `custom_emoji_disabled`, `custom_emoji_deleted` | `custom_emoji` | `custom_emojis.id` | `:shortcode:`（リモートは `:shortcode:@domain`） | `custom_emoji_images.id`（画像を記録した操作。`custom_emoji_copied` は複製した絵文字の画像）。ほかはなし |
  | `webhook_created`, `webhook_updated`, `webhook_deleted`, `webhook_enabled`, `webhook_disabled`, `webhook_secret_rotated` | `webhook` | `webhook_endpoints.id` | 説明と URL のホスト名（`Moderation chat (hooks.example.com)`）。URL は写さない | `webhook_endpoint_revisions.id` / `webhook_endpoint_state_changes.id` / `webhook_secrets.id` |
  | `announcement_created`, `announcement_updated`, `announcement_published`, `announcement_withdrawn` | `announcement` | `announcements.id` | 本文の最初の行（80 文字まで） | `announcement_revisions.id` / `announcement_state_changes.id` |
  | `invite_created`, `invite_revoked` | `invite` | `invites.id` | コードの先頭 4 文字と `…`（`ABCD…`）。使える間は登録に使えるコード全体は写さない | `invites.id` / `invite_revocations.invite_id`（= `invites.id`） |
  | `relay_added`, `relay_followed`, `relay_unfollowed`, `relay_removed`, `relay_accepted`, `relay_rejected` | `relay` | `relays.id` | 登録した URL | `relay_events.id`（登録と削除はなし） |
  | `status_refetch_queued` | `status` | `statuses.id` | 投稿の URI | なし（元の記録を持たない操作） |
  | `account_statuses_refetch_queued` | `account` | `accounts.id` | リモートのアカウントのハンドル | なし（元の記録を持たない操作） |
  | `domain_statuses_refetch_queued` | `domain` | ドメイン | ドメイン | なし（元の記録を持たない操作） |
  | `billing_import_queued` | `billing_source` | 取り込み元（`aws`・`gcp`） | 事業者の名前（`AWS`） | なし（元の記録を持たない操作） |
  | `cost_item_created`, `cost_item_relabeled`, `cost_item_price_revised`, `cost_item_end_changed`, `cost_item_canceled` | `cost_item` | `cost_items.id` | 項目の名前（変更は変更後の名前） | `cost_items.id` / `cost_item_label_revisions.id` / `cost_item_price_revisions.id` / `cost_item_end_changes.id` / `cost_item_cancellations.id` |
  | `supporter_program_enabled`, `supporter_program_disabled` | `supporter_program` | `supporter_program_changes.id` | 決済の事業者の名前（`stripe`） | `supporter_program_changes.id` |
  | `supporter_plan_created`, `supporter_plan_revised`, `supporter_plan_retired` | `supporter_plan` | `supporter_plans.id` | プランの名前（変更は変更後の名前） | `supporter_plan_revisions.id` / `supporter_plan_retirements.plan_id` |
  | `supporter_subscription_canceled` | `supporter_subscription` | `supporter_subscriptions.id` | プランの名前と利用者のハンドル（`Supporter @alice`） | `supporter_subscriptions.id` |
  | `custom_emoji_request_submitted`, `custom_emoji_request_withdrawn`, `custom_emoji_request_approved`, `custom_emoji_request_rejected` | `custom_emoji_request` | `custom_emoji_requests.id` | `:shortcode:` | 申請はなし、ほかは `custom_emoji_request_outcomes.id` |
  | `payment_reconcile_queued` | `payment_provider` | 決済の事業者の名前（`stripe`） | 決済の事業者の名前 | なし（元の記録を持たない操作） |

  申請と取り下げは申請者（`create_custom_emojis` を持つ利用者）を操作者とし、承認は申請への `custom_emoji_request_approved` と、作った絵文字への `custom_emoji_created`（画像の申請は `custom_emoji_image_replaced`）の 2 行を承認した人を操作者として残します（ADR-0070 §8）。作成者による変更と削除は `custom_emoji_updated`・`custom_emoji_deleted` を作成者を操作者として残します。
  サーバーが 410 Gone や失敗の継続で宛先を無効にした `webhook_disabled` は `actor_kind = system` です（ADR-0060）。リレーが送った `Accept` / `Reject` の `relay_accepted` / `relay_rejected` は `actor_kind = remote` です（ADR-0061）。

- ハンドルは、ローカルのアカウントは `@username`、リモートは `@username@domain`、ハンドルの無いアカウントは URI、消えたアカウントは ID です。表示名は 512 文字で切ります。
- 一覧（`moderation.Service.GoneAuditTargets`）は、ページの行の対象が今もあるかを、対象の種類ごとに 1 回の問い合わせでまとめて確かめます。
  アカウントは `account` の公開 API（`account.Service.ExistingIDs`）に、通報・異議申し立て・登録の拒否は `moderation` 自身の `reports`・`moderation_appeals`・`signup_blocks` に問い合わせます。
  ロール・カスタム絵文字・その申請・webhook・リレーは、それを持つモジュールが監査ログを発行する側で `moderation` から呼べないため、ポート `moderation.AuditTargets` を通して `identity.Service.ExistingRoleIDs`（`roles`）・`content.CustomEmojiService.ExistingIDs`（`custom_emojis`）・`content.CustomEmojiService.ExistingRequestIDs`（`custom_emoji_requests`。行を消さないので常にある）・`webhook.Service.ExistingIDs`（`webhook_endpoint_heads` の `deleted` でないもの）・`federation.Relays.ExistingIDs`（`relays`）に問い合わせます。
  ほかの種類の対象（ドメイン・利用者・設定・ルール・お知らせ・招待）は消えません。
- 新しい管理操作を足すときは、`internal/domain/moderation` の `AuditAction` と対象の種類、スキーマの `CHECK`、API の enum（`AdminAuditAction`）を一緒に増やし、ユースケースから発行します（API の enum と一致することをテストが確かめます）。
- 更新・削除の API と CLI はありません。保持期間（ADR-0038、既定 730 日）を過ぎた行だけを消去ジョブが消します。

#### 切れた関係【イベント】

| テーブル | 分類 | カラム | 制約 |
| --- | --- | --- | --- |
| `relationship_severances` | イベント | `id`, `domain_action_id`（NULL 可）, `action_id`（NULL 可。リモートのアカウントの凍結）, `account_id`（関係を失ったローカルのアカウント）, `followers_lost`（`int`）, `following_lost`（`int`）, `occurred_at` | インデックス `(account_id, id DESC)`。一意 `(domain_action_id, account_id)`, `(action_id, account_id)`。`domain_action_id` と `action_id` はどちらか一方だけが値を持つ（`CHECK`） |

- ドメインの凍結とリモートのアカウントの凍結は、両方向のフォローを（承認待ちも含めて）`follow_terminations`（`reason = moderation`）で切り、承認済みのフォローを失ったローカルのアカウントごとに 1 行を記録して通知します（FR-717、ADR-0036）。`followers_lost` / `following_lost` は承認済みのフォローの数で、承認待ちだけを失ったアカウントには行を作りません。解除しても関係は戻しません。
- リモートのアカウントの凍結は、凍結を始めた処分のトランザクションで切ります（相手は 1 アカウントなので、ローカルのアカウントの数で済む）。
- ドメインの凍結は件数に上限がないので、処分のトランザクションで outbox に積んだジョブ `moderation.sever_domain` が、ローカルのアカウント 1 つごとに 1 トランザクションで、そのアカウントとドメイン・サブドメインのアカウントの間のフォローを切ります。処分がジョブの前に解除されても切ります。
- 一意制約により、同じ処分を処理し直しても行と通知は増えません。リモートのアカウントがすでに凍結中なら 2 つ目の凍結は何も切りません（`account_suspension_changed` が出ないため）。

#### 保全

| テーブル | 分類 | カラム | 制約 |
| --- | --- | --- | --- |
| `retention_holds` | イベント | `id`, `account_id`, `reason`（`text`。1,000 文字まで）, `created_by_user_id`（NULL 可 = CLI）, `created_at` | インデックス `(account_id)` |
| `retention_hold_releases` | イベント | `hold_id`（PK）, `released_by_user_id`（NULL 可 = CLI）, `released_at` | |

- 保全は解放（`retention_hold_releases`）が無い間だけ効きます。運営者が CLI（`my admin retention hold|release|list`）で置き・解放し、1 つのアカウントに複数を置けます。
- 効いている保全のあるアカウントのモデレーション記録・ログイン元と登録元の IP・投稿は、保持期間を過ぎても消去ジョブが消しません（ADR-0038）。
  モデレーション記録は、そのアカウントへの処分と異議申し立て、そのアカウントが対象か通報者の通報、対象がそのアカウントかその通報・異議申し立ての監査ログです。
  登録の拒否はアカウントに属さないので保全の対象外ですが、BAN から作られた拒否（`moderation_action_id`）は、その BAN の対象のアカウントに効いている保全があれば残ります。

#### 記録の保持期間

1 時間ごとの消去ジョブ `moderation.purge_records` が、インスタンス設定（§4.9）の保持期間を過ぎたモデレーション記録を、処分 → 通報 → 登録の拒否 → 監査ログの順に消します（ADR-0038、§8）。
期間は回ごとに読み直します。処分・通報・登録の拒否は 1 トランザクションあたり 100 件、監査ログは 1 文あたり 1,000 行で、それぞれ 20 回まで、古い順（`id` 順）に消し、続きは次の回が消します。

| 記録 | 設定（既定・下限） | 起点 | 消すもの |
| --- | --- | --- | --- |
| 処分 | `retention_moderation_actions`（365 日・180 日） | 処分の終わり: `warn`・`delete_statuses` は `issued_at`、ほかは解除と期限の早い方（どちらも無ければ消さない）。異議申し立てがあれば審査の `decided_at` より前には消さず、審査待ちの間は消さない | `moderation_actions` と `moderation_action_statuses`・`moderation_action_lifts`・`moderation_appeals`・`moderation_appeal_decisions`・`appeal_states`・`account_restrictions` の行 |
| 通報 | `retention_reports`（365 日・180 日） | `report_states` が `resolved` になった `since`（最後の解決）。未解決の通報と、処分が参照する（`moderation_actions.report_id`）通報は消さない | `reports` と `report_statuses`・`report_rule_citations`・`report_assignments`・`report_resolutions`・`report_reopenings`・`report_notes`・`report_states` |
| 監査ログ | `retention_audit_log`（730 日・365 日） | `occurred_at` | `audit_log_entries` の行 |
| 登録の拒否 | なし（30 日に固定） | 拒否の終わり: 解除（`signup_block_lifts.lifted_at`）と期限（`expires_at`）の早い方。どちらも無ければ消さない | `signup_blocks` と `signup_block_lifts` の行 |

- 処分が参照する通報は、処分が消えた同じ回に、通報自身の期限を過ぎていれば消えます（処分を先に消すため）。
- `relationship_severances` と、記録を指す通知（`moderation_action_id`・`report_id`・`appeal_id`）は消しません。指す先の消えた通知は、その記録を埋め込まずに返します。
- 開示請求には、運営者が CLI（`my admin account export-moderation`）でアカウントの記録をすべて JSON で書き出します。
  処分（非公開のメモを含む）と異議申し立て、そのアカウントの通報（メモ・担当・解決・再開を含む）、そのアカウントが出した通報（状態だけ）、それらの監査ログを入れ、通報者の身元（`reporter_account_id`・`uri`・`forwarded`）だけを伏せます（ADR-0038）。
