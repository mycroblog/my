### 4.1 identity

ログイン主体（`users`）を中心に、資格情報・メール・登録審査・権限・設定を、それぞれ独立した概念として持ちます。

#### users【リソース】

| カラム | 型 | 説明 |
| --- | --- | --- |
| `id` | `id` | PK |
| `created_at` | `timestamp` | |

ログイン主体の識別だけを持ちます。ローカルアカウントとの対応は `local_accounts.user_id` です。

#### メールアドレス

| テーブル | 分類 | カラム | 制約 |
| --- | --- | --- | --- |
| `user_email_addresses` | リソース | `id`, `user_id`, `address`（`string(320)`）, `address_normalized`, `added_at` | インデックス `(user_id, id)` |
| `user_email_verifications` | イベント | `email_address_id`（PK）, `verified_at` | 確認は 1 アドレスにつき 1 回 |
| `user_primary_emails` | 投影 | `user_id`（PK）, `email_address_id`, `address_normalized` | **一意 `(address_normalized)`**。ログイン時の照合とアドレスの重複防止 |

- メールアドレスの変更は、新しいアドレスの追加 → 確認 → 主アドレスの切り替え（投影の更新）で表します。古いアドレスは消去ジョブの対象です。

#### 資格情報

| テーブル | 分類 | カラム | 制約 |
| --- | --- | --- | --- |
| `password_credentials` | イベント（版） | `id`, `user_id`, `password_hash`（Argon2id の PHC 文字列）, `set_at` | インデックス `(user_id, id DESC)`。削除イベントの無い最新行が現在のパスワード。過去のハッシュは再利用防止に使え、保持期間後に消去する |
| `password_credential_removals` | イベント | `password_credential_id`（PK）, `removed_at` | 最新行に削除イベントがあれば、その利用者はパスワードを持たない。2 回目の記録は何もしない |
| `totp_enrollments` | イベント | `id`, `user_id`, `secret_enc`（`bytes`）, `enrolled_at` | インデックス `(user_id, id DESC)` |
| `totp_enrollment_revocations` | イベント | `enrollment_id`（PK）, `revoked_at` | 有効な登録 = 失効イベントの無い最新の登録 |
| `totp_recovery_codes` | リソース | `enrollment_id`, `code_hash` | PK `(enrollment_id, code_hash)` |
| `totp_recovery_code_uses` | イベント | `enrollment_id`, `code_hash`, `used_at` | PK `(enrollment_id, code_hash)`。挿入の成否で 1 回性を保証 |

- `secret_enc` は TOTP の 160 ビットの秘密を `kernel.Sealer` で暗号化した値です（ADR-0019 §6）。AAD は登録の `id` と列名 `totp_enrollments.secret_enc` です。
- `code_hash` は、正規化したリカバリーコード（80 ビットの乱数）の SHA-256 です。
- リカバリーコードを作り直すときは、同じ秘密を新しい登録に移し、古い登録に失効のイベントを記録します。管理者が二要素認証を外すときも失効のイベントを記録し、監査ログに `two_factor_disabled` を残します。
- パスワードのリセットと変更（FR-002）は、`password_credentials` に新しい行を足し、その利用者のセッションに `session_revocations`（`password_changed`）、トークンに `oauth_token_revocations`（`password_changed`）を同じトランザクションで記録します。設定の画面からの変更は、変更を要求したトークンの系列だけを残します。TOTP の登録とパスキーには何も記録しないので、どちらもそのまま使えます。
- パスキーを持つ利用者は、パスワードを削除できます（FR-004、`POST /api/v1/accounts/password/remove`）。最新の `password_credentials` の行に `password_credential_removals` を記録し、変更と同じくセッションとトークンを `password_changed` で失効させ（要求したトークンの系列は残す）、メールを送るサーバーではメールで知らせます。
  それからはパスキーだけでログインします。リセットのリンクか、パスキーで確かめた変更（`POST /api/v1/accounts/password`）が新しい行を足すと、パスワードはまた使えます。
- 利用者がログインの手段を失わないように、パスワードはパスキーが 1 つ以上あるときだけ削除でき、最後のパスキーはパスワードがあるときだけ削除できます。どちらの削除も、確かめてから書き終えるまで同じトランザクションで利用者のログインの手段をロックします（PostgreSQL はトランザクションの間のアドバイザリーロック、SQLite は書き込みのトランザクションが 1 つずつ走る）。同時に 2 つの削除が走っても両方は消えません。
- 使った TOTP のコードの時間ステップと、パスワードの後で 2 段目を待つサインインのチャレンジ（5 分）は、DB ではなく `KeyValueStore` の名前空間 `two_factor` に置きます。

| テーブル | 分類 | カラム | 制約 |
| --- | --- | --- | --- |
| `passkey_credentials` | リソース | `id`, `user_id`, `credential_id`（`bytes`）, `public_key`（`bytes`。COSE_Key）, `backup_eligible`（`bool`）, `name`, `created_at` | 一意 `(credential_id)`、インデックス `(user_id, id)` |
| `passkey_credential_revocations` | イベント | `passkey_id`（PK）, `revoked_at` | 有効なパスキー = 失効イベントの無いパスキー |

- パスキー（FR-004）は WebAuthn の発見可能な資格情報（discoverable credential）で、パスワードの代わりにログインに使います。ユーザーハンドルは利用者の `id` です。登録とログインはどちらも利用者の検証（端末の PIN や生体認証）を求め、アテステーションは求めません。
- パスキーはパスワードの代わりになるだけで、二要素認証の 2 段目は省きません。TOTP を有効にした利用者は、パスキーでのログインの後にも認証コードを入力します。
- 登録には現在のパスワードか、その利用者のパスキーでの確認が要ります（失敗は利用者ごとに 1 時間 5 回まで、`passkey.password`）。1 人の利用者が持てるパスキーは 20 個までです。削除したパスキーの行は残すので、同じ資格情報 ID を登録し直すことはできません。
- 本人を確かめる操作（パスキーの登録、パスワードの変更と削除、アカウントの削除）は、現在のパスワードの代わりにその利用者のパスキーで確かめられます。パスワードの削除だけはパスキーでしか確かめません。残るパスキーが使えることを、削除の前に確かめるためです。
  確認は `POST /api/v1/passkeys/confirmation` が始め、その状態は利用者に結び付けて `kernel.Sealer` で封をし、ログインと同じく 1 回だけ使えます。ほかの利用者のパスキーや削除したパスキーでは確かめられず、失敗は操作ごとの上限（パスワードの間違いと同じ）に数えます。
- 署名カウンターは保存しません。同期されるパスキーは常に 0 を返すので、複製の検出に使えないからです。
- 登録・ログイン・確認の途中の状態（チャレンジ、5 分）は、`kernel.Sealer` で封をしてブラウザに持たせ、DB にも KV にも置きません。ログインと確認のチャレンジは 1 回だけ使えるように、使ったことを `KeyValueStore` の名前空間 `passkey` に有効期限まで記録します。ログインの失敗は、パスワードの失敗と同じクライアントのログイン失敗の上限（`login.ip`）に数えます。

#### 登録審査

| テーブル | 分類 | カラム | 制約 |
| --- | --- | --- | --- |
| `registration_applications` | リソース | `id`, `user_id`, `reason`（`text`、NULL 可）, `invite_id`（NULL 可）, `submitted_at` | 一意 `(user_id)` |
| `registration_decisions` | イベント | `id`, `application_id`, `decision`（`enum(approved, rejected)`）, `decided_by`（`enum(moderator, system)`）, `decided_by_user_id`（NULL 可 = `system` または CLI による決定）, `note`（NULL 可。1,000 文字まで）, `decided_at` | インデックス `(application_id, id DESC)`。`open` / `invite` モードでは `system` が即時に承認する |
| `user_registration_states` | 投影 | `user_id`（PK）, `state`（`enum(pending, approved, rejected)`）, `since` | インデックス `(state, since)`（承認待ち一覧）。行の無い利用者（CLI で作った管理者など）は承認済み |
| `registration_origins` | リソース | `user_id`（PK）, `ip`, `recorded_at` | インデックス `(ip)`, `(recorded_at)`。登録画面からの登録だけが作る。登録の拒否リスト（FR-719）と同じ IP のアカウントの検索に使い、保持期間（ADR-0038、既定 180 日）を過ぎた行を消去ジョブが消す |

- 登録画面からの登録は、登録モード（インスタンス設定 `registration_mode`、§4.9）に従います。
  `open` は `system` が即時に承認し、`approval` はモデレーターの決定まで `pending` のまま、`invite` は有効な招待があれば `system` が即時に承認し、`closed` は受け付けません。
  `approval` モードでだけ `reason` を受け取り、`invite` モードでだけ `invite_id` を記録します。
- メールを届けられない構成（`mail = none`）では、メール確認の代わりにモデレーターの承認を求めます（FR-002）。このとき `open` モードの登録も `pending` で始まり、確認済みでないことはアプリの利用を妨げません。
- アプリ（クライアント API と OAuth の認可）を使えるのは、`user_registration_states` が `approved`（または行が無い）で、メール確認が必要な構成では主アドレスが確認済みの利用者だけです。
  承認待ち・却下・未確認の利用者もサーバーが返す画面にはログインでき、何を待っているかが表示されます。
  ログインを奪う処分（制限 `login`、§5.2）を受けた利用者も同じで、サーバーが返す画面にはログインでき、効いている処分のうちログインを奪うもの（種類・理由のカテゴリと公開の理由・期限。`ban` は削除までの日数）が本人の UI 言語で表示され、その画面から異議を申し立てられます。アクセストークンは得られません。
  ライフサイクルが `active` でないアカウント（§4.2）はログインできません。例外は `ban` が効いているあいだの `deletion_scheduled` で、削除まで（`ban` への異議申し立ての期間、§4.3）は同じくサーバーが返す画面にだけログインできます。本人による削除の予約・無効化はログインできないままです。
- モデレーターの決定（CLI と管理の API）は、`pending` の行だけを更新する条件付きの更新で `user_registration_states` を変え、同じトランザクションで `registration_decisions` と監査ログの行を加えます。
  更新する行が無ければ（決定済み、または同時に決まった）何も記録しないので、登録の決定は 1 回だけです。
- CLI で作った管理者の主アドレスは、作成と同じトランザクションで確認済みにします（運営者が与えたアドレスなので）。

#### 招待・1 回限りのトークン

| テーブル | 分類 | カラム | 制約 |
| --- | --- | --- | --- |
| `invites` | リソース | `id`, `code`（80 ビットの乱数の base32、16 文字。照合は大文字・空白とハイフンを除いた形で行う）, `created_by_user_id`（NULL 可 = CLI による作成）, `max_uses`（NULL 可 = 無制限）, `expires_at`（NULL 可）, `created_at` | 一意 `(code)` |
| `invite_redemptions` | イベント | `invite_id`, `seq`（`int`、1 始まり）, `user_id`, `redeemed_at` | PK `(invite_id, seq)`。`seq = 既存件数 + 1` で挿入し、一意制約違反なら再試行する。`seq > max_uses` は拒否する |
| `invite_revocations` | イベント | `invite_id`（PK）, `revoked_by_user_id`（NULL 可 = CLI による取り消し）, `revoked_at` | 取り消しは 1 回だけ記録する（2 回目は何もしない） |
| `user_tokens` | リソース | `id`, `user_id`, `purpose`（`enum(email_verification, password_reset)`）, `email_address_id`（NULL 可）, `token_hash`（`bytes`。トークンの SHA-256）, `expires_at`, `issued_at` | 一意 `(token_hash)`。メール確認のトークンは接頭辞 `my_evt_`、有効期限 24 時間。パスワードリセットのトークンは接頭辞 `my_prt_`、有効期限 1 時間 |
| `user_token_redemptions` | イベント | `token_id`（PK）, `redeemed_at` | 挿入の成否で 1 回性を保証する |

- パスワードリセットのトークン（FR-002）は、期限内で使われておらず、発行がパスワードの最後の変化（`password_credentials` の最新行の `set_at`、削除されていればその `removed_at`）より前でないときだけ使えます。
  リセット・変更・管理者の発行など、どの経路でもパスワードが新しくなるか削除されれば、それより前に発行したリンクは使えなくなります。状態の列は持たず、これらのイベントから求めます。パスワードを削除した利用者も、リセットのリンクで新しいパスワードを設定できます。
- リセットのリンクは、メールを送るサーバー（`mail` が `none` でない）ではログインの画面から本人がメールで受け取り、どのサーバーでも管理者が管理画面（権限 `manage_login_recovery`）か CLI（`my admin account password-reset-link`）で発行できます。
  管理者の発行は監査ログに `password_reset_link_issued` を残し、その `source_event_id` はトークンの `id` です。

#### 権限

固定の `role` enum ではなく、ロールと権限を分けます。組み込みロールは冪等に作ります（`roles (key)` の一意制約で重複しない）。
`admin`（`position` 1000）はすべての権限、`moderator`（`position` 100）は最初 `manage_reports`・`manage_accounts`・`view_audit` を持ちます。
組み込みロールは `my serve` の起動時（どの `role` のプロセスでも）、`my admin create`、`my admin role` のたびに作り直すので、版を上げても手作業は要りません。作り直すときは、ない組み込みロールを足し、`admin` にすべての権限（新しい版で増えた権限を含む）を持たせ、`moderator` の権限と位置は管理者が変えたまま残します。
権限は区分（[ADR-0058](../../../adr/0058-moderation-admin-separation.md)、[ADR-0070](../../../adr/0070-permission-split-emoji-finance.md)）を持ちます。モデレーションは `manage_reports`・`manage_accounts`・`manage_domains`・`manage_invites`・`view_audit`、
利用者の機能は `view_finance`、サーバー管理はそれ以外のすべて（`manage_settings`・`manage_rules`・`manage_roles`・`manage_jobs` など）です。区分は列に持たず、`internal/domain/identity` の `Permission.Category` が返します
（Web クライアントのナビゲーションの出し分けと `my admin role list` の表示が使う）。

| テーブル | 分類 | カラム | 制約 |
| --- | --- | --- | --- |
| `roles` | リソース | `id`, `key`（`string(64)`）, `position`（`int`）, `created_at` | 一意 `(key)` |
| `role_permissions` | 現在値 | `role_id`, `permission`（`enum(manage_reports, manage_accounts, manage_domains, manage_rules, manage_settings, manage_roles, manage_invites, view_audit, manage_jobs, manage_login_recovery, manage_custom_emojis, manage_webhooks, manage_announcements, manage_finance, view_finance, manage_supporters, create_custom_emojis)`） | PK `(role_id, permission)` |
| `user_role_grants` | イベント | `id`, `user_id`, `role_id`, `granted_by_user_id`（NULL 可 = CLI による付与。最初の管理者など）, `granted_at` | |
| `user_role_revocations` | イベント | `grant_id`（PK）, `revoked_by_user_id`（NULL 可 = CLI による剥奪）, `revoked_at` | |
| `user_roles` | 投影 | `user_id`, `role_id`, `grant_id` | PK `(user_id, role_id)` |
| `role_badges` | 現在値 | `role_id`（PK）, `label`（`string(32)`）, `color`（`enum(neutral, red, orange, yellow, green, teal, blue, purple, pink)`）, `updated_at` | 行がある = バッジがある |
| `user_role_badge_displays` | 現在値 | `user_id`, `role_id`, `created_at` | PK `(user_id, role_id)`。行がある = プロフィールに出す |
| `managed_roles` | 現在値 | `role_id`（PK）, `manager`（`enum(supporters)`）, `created_at` | 行がある = 課金のプランが管理するロール（[ADR-0072](../../../adr/0072-supporter-plans-and-roles.md) §5） |

- 付与は `user_role_grants` に 1 行足して `user_roles` に加え、剥奪は `user_role_revocations` に 1 行足して `user_roles` から除きます。どちらも同じトランザクションで監査ログ（`role_granted`・`role_revoked`）を書きます。
  持っているロールの付与と持っていないロールの剥奪は何もしません。
- 利用者は自分のロールの最も高い `position` 以下のロールだけを付与・剥奪できます（FR-720）。CLI はどのロールも付与・剥奪できます。
- `admin` は最後の 1 人から剥奪できません。1 つのロールの変更は、トランザクションの最初にそのロールの行を読んで（PostgreSQL では `FOR UPDATE`）1 つずつ進めます。
- 権限は要求ごとに `user_roles` から読むので、変更は次の要求から効きます。モデレーター向けの通知の宛先も、通知を作るときの `user_roles` で決まります。
- ロールの作成・変更・削除（FR-720）は `roles` と `role_permissions` を書き、同じトランザクションで監査ログ（`role_created`・`role_updated`・`role_deleted`）を書きます。
  キーは 1〜64 文字の英小文字・数字・`_`・`-` で、変えられません。`admin` 以外のロールの `position` は 0〜999 で、`admin` がいつもいちばん上にあります。
  `admin` はバッジのほかは変えられず（すべての権限と `position` 1000。権限を取り戻す道のため）、`admin` と `moderator` は削除できません。`moderator` の権限と位置は変えられます。
  持っている人がいるロールは削除できない（先に剥奪する）ので、`user_roles` が消えたロールを指すことはありません。消えたロールを指すのは剥奪済みの `user_role_grants` だけです。
  利用者は自分のいちばん高いロール以下のロールだけを作り・変え・消せ（新しい位置も同じ）、自分のロールが持つ権限だけを付け外しできます。CLI は何でもできます。
- ロールのバッジ（[ADR-0071](../../../adr/0071-role-badges-and-supporter-list.md)、FR-913）は `role_badges` の行で、名前（1〜32 文字の 1 行。管理者が書く文字列で、どのロケールでも同じ値）と色（`@my/ui` のトークンの 9 色の名前）だけを持ちます。
  組み込みのロールを含めどのロールにも付けられ、既定ではどのロールも持ちません。設定・変更・削除はロールの変更（`role_updated`）で、ロールを削除すると `role_badges` の行も消えます。
- 利用者は、持っていてバッジのあるロールごとに、プロフィールに出すかを選びます（`user_role_badge_displays`。既定は出さない）。
  剥奪（`user_role_revocations` を足す処理）とバッジの削除は、同じトランザクションでその選択の行を消します。付与も同じトランザクションで古い行を消すので、もう一度付与されれば既定の「出さない」から始まります。
  プランの変更でロールが入れ替わるときは、`identity.Service.ReplaceRole` が古いロールの剥奪と新しいロールの付与を 1 つのトランザクションで行い、古いロールの選択を新しいロールに移します。新しいロールにバッジが無ければ選択は消えます。
- プロフィールのバッジは `user_role_badge_displays`・`user_roles`・`role_badges`・`roles` の結合 1 回で、ロールの `position` の高い順に読みます。読むのはアカウント自身の取得だけで、タイムラインや一覧のアカウントでは読みません。
  バッジは ActivityPub・NodeInfo・WebFinger に載せず、リモートのバッジも取り込みません。アカウントの消去（§8）は本人の選択の行を消します。
- 課金のプランのロール（[ADR-0072](../../../adr/0072-supporter-plans-and-roles.md) §5、FR-912）は `managed_roles` の行で表します。プランの作成が、持っている人のいない、権限がすべて区分 `member` の、
  どのスタッフのロール（組み込みのロールか、区分 `moderation`・`server` の権限を 1 つでも持つロール）よりも `position` の低いロールを、同じトランザクションで `managed_roles` に入れます（`identity.Service.ManageRole`）。
  管理されたロールの付与・剥奪・入れ替えは `supporters` だけが、加入の遷移のトランザクションで行い（`GrantManagedRole`・`RevokeManagedRole`・`ReplaceManagedRole`。監査ログの操作者は `system`）、権限と位置はプランのフォームから変えます（`UpdateManagedRole`）。
  FR-720 の手での付与・剥奪と、権限・位置の直接の変更とロールの削除は `ErrRoleManagedByPlan`（409）で断り、バッジはふつうに変えられます。
  管理されていないロールの変更は、スタッフのロールを管理されたロールのいちばん高い `position` 以下に置くとき `ErrBreaksManagedRoles`（409）で断ります。
  廃止したプランのロールは、持つ人がいなくなったときに `managed_roles` から外れます（`UnmanageRole`）。

#### セッション・OAuth

| テーブル | 分類 | カラム | 制約 |
| --- | --- | --- | --- |
| `sessions` | リソース | `id`, `user_id`, `token_hash`（`bytes`。Cookie の値の SHA-256）, `created_at`, `expires_at` | 一意 `(token_hash)`。インデックス `(user_id)`（処分による失効） |
| `session_origins` | リソース（子） | `session_id`（PK）, `user_id`, `ip`（NULL 可）, `user_agent`（`string(512)`、NULL 可）, `created_at` | インデックス `(ip)`（同じ IP のアカウントの検索、FR-714）, `(user_id, created_at DESC)`（アカウントの最近のログイン元、FR-714）, `(created_at)`（保持期間による消去）。保持期間（[ADR-0038](../../../adr/0038-moderation-personal-data-retention.md)、既定 180 日）を過ぎた行を消去ジョブが消す |
| `session_revocations` | イベント | `session_id`（PK）, `reason`（`enum(logout, replaced, password_changed, moderation, expired_cleanup, account_deleted)`）, `revoked_at` | 有効なセッション = 失効イベントが無く `expires_at` を過ぎていないもの。`replaced` は同じブラウザでのログインし直し |
| `session_activity` | 現在値 | `session_id`（PK）, `last_seen_at` | ログイン時に作り、以後の更新は 1 時間に 1 回に間引く |
| `oauth_clients` | リソース | `id`, `client_id`, `client_secret_hash`（NULL 可 = 公開クライアント）, `name`, `website`（NULL 可）, `scope`, `registration`（`enum(first_party, user, metadata_document, dynamic)`）, `owner_user_id`（NULL 可）, `created_at` | 一意 `(client_id)`。インデックス `(owner_user_id)`。`registration` は登録の経路で、変わらない。`name`・`website`・`scope`・`client_secret_hash` は登録したときの値 |
| `oauth_client_disablements` | イベント | `id`, `client_id`, `disabled_by_user_id`（NULL 可 = CLI）, `disabled_at` | インデックス `(client_id)`。取り消しのない行があればクライアントは無効 |
| `oauth_client_disablement_lifts` | イベント | `disablement_id`（PK）, `lifted_by_user_id`（NULL 可 = CLI）, `lifted_at` | 無効化の取り消し |
| `oauth_client_deletions` | イベント | `client_id`（PK）, `reason`（`enum(owner, unused)`）, `deleted_by_user_id`（NULL 可 = 使われないクライアントの自動の削除）, `deleted_at` | 削除したクライアントは二度と使えない |
| `oauth_client_revisions` | イベント | `id`, `client_id`, `name`, `website`（NULL 可）, `scope`, `revised_at` | インデックス `(client_id, id)`。登録者による変更。いちばん新しい行がいまの名前・サイト・スコープ |
| `oauth_client_secret_rotations` | イベント | `id`, `client_id`, `secret_hash`, `rotated_at` | インデックス `(client_id, id)`。シークレットの再発行。いちばん新しい行のシークレットだけが使える |
| `oauth_client_redirect_uris` | 現在値 | `client_id`, `redirect_uri` | PK `(client_id, redirect_uri)`。完全一致で照合する |
| `oauth_client_metadata_documents` | 現在値（キャッシュ） | `client_id`（PK）, `client_name`, `client_uri`（無ければ空）, `logo_uri`（無ければ空）, `scope`, `fetched_at`, `expires_at` | 経路が `metadata_document` のクライアントの、最後に取得した Client ID Metadata Document。取得のたびに UPSERT し、`oauth_client_redirect_uris` も文書の `redirect_uris` に置き換える |
| `oauth_client_host_denials` | イベント | `id`, `host`, `denied_by_user_id`（NULL 可 = CLI）, `denied_at` | インデックス `(host)`。取り消しのない行があれば、そのホストの `client_id` の URL は取得も利用もしない |
| `oauth_client_host_denial_lifts` | イベント | `denial_id`（PK）, `lifted_by_user_id`（NULL 可 = CLI）, `lifted_at` | ホストの拒否の取り消し |
| `oauth_authorization_codes` | リソース | `code_hash`（PK）, `client_id`, `user_id`, `redirect_uri`, `scope`, `code_challenge`, `session_id`（NULL 可 = セッションの外で発行）, `issued_at`, `expires_at` | `session_id` は認可した画面のセッション |
| `oauth_authorization_code_redemptions` | イベント | `code_hash`（PK）, `token_id`, `redeemed_at` | 一意 `(token_id)`。`token_id` は交換で発行した系列の最初のトークン |
| `oauth_access_tokens` | リソース | `id`, `family_id`, `client_id`, `user_id`, `access_token_hash`, `refresh_token_hash`（NULL 可）, `scope`, `rotated_from_token_id`（NULL 可）, `issued_at`, `expires_at`, `refresh_expires_at`（NULL 可） | 一意 `(access_token_hash)`, `(refresh_token_hash)`。インデックス `(family_id)`, `(user_id)`（処分による失効）, `(client_id)`（クライアントの無効化・削除による失効）。`refresh_token_hash` と `refresh_expires_at` は同時に NULL |
| `oauth_token_revocations` | イベント | `token_id`（PK）, `reason`（`enum(user, rotation, refresh_reuse, code_reuse, password_changed, moderation, account_deleted, client_disabled, client_deleted)`）, `revoked_at` | |
| `oauth_token_family_last_used` | 現在値 | `family_id`（PK）, `last_used_at` | アクセストークンの認証で、系列ごとに 1 時間に 1 回まで更新する（`KeyValueStore` の名前空間 `oauth_last_used` の `IncrBy` で間引く）。行は系列が使われてから作る |

- 認可したアプリの一覧（ADR-0090 §5、FR-003）は、そのためのテーブルを持たず、有効なトークン（失効のイベントが無く、アクセストークンか更新トークンが期限内）を利用者とクライアントで束ねて導きます。
  付与したスコープは束ねたトークンのスコープの和集合、最初に認可した日時は系列の最初のトークンの `issued_at` の最小、最後に使った日時は `oauth_token_family_last_used` の最大です。
  取り消しは、その利用者とクライアントのトークンすべてに `oauth_token_revocations`（`user`）を記録します。
- セッションは、サーバーが返す画面（ログイン・登録・認可同意）の Cookie `__Host-my_session` です（ADR-0019 §4、ADR-0030）。
  ログインのたびに新しいトークンを発行し、ブラウザが持っていた古いセッションは `replaced` で失効させます。
- ローカルアカウントの `deleted`（§4.2）は、同じトランザクションでその利用者の有効なセッションとトークンを `account_deleted` で失効させます（`identity.RegisterLifecycleProjectors`）。
- ログイン元（`session_origins`）と登録元（`registration_origins`）は、セッション・利用者と同じトランザクションで記録します。
  - IP アドレスは正規形（IPv4 射影の IPv6 は IPv4、ゾーンは外す）で持ち、同じアドレスが同じ文字列になるようにします。IP アドレスとして読めない値は記録しません。
  - IP アドレスも User-Agent も分からないログインは `session_origins` の行を作らず、IP アドレスの分からない登録は `registration_origins` の行を作りません。どちらもログインと登録を妨げません。
  - 1 時間ごとの消去ジョブ `identity.purge_origins` が、保持期間（インスタンス設定 `retention_session_origins`・`retention_registration_origins`、既定 180 日、§4.9）を過ぎた行を、1 文あたり 1,000 行・1 テーブルあたり 20 文まで、古い順に消します。
    保全（§4.3）のあるローカルのアカウントの利用者の行は消しません。期間と保全のある利用者は回ごとに読み直します。セッションと利用者は残ります。
  - IP アドレスとメールアドレスを返すのは、モデレーターのアカウントの管理（FR-714）の「表示」（`identity.AccountAdmin.Reveal`）だけで、呼ぶたびにそのためのトランザクションで監査ログ（`revealed_private_data`）を書き、書けたときだけ返します。
- アカウントの管理（FR-714）の検索は、モジュールをまたぐ JOIN をしません。条件ごとに 1 つのモジュールが自分のテーブルから ID を新しい順のキーセットで返し（メールアドレス・メールのドメイン・IP アドレス・登録の状態は `identity` が利用者の ID で、処分の制限は `moderation` が `account_restrictions` から、削除は `account` が `account_lifecycle_states` から、それ以外は `account` が `accounts` と `account_handles` から）、`identity.AccountAdmin` が残りの条件をアカウントごとに確かめます。
  1 ページを埋めるのに読むのは 400 アカウントまでで、埋まらなければ読んだ位置をカーソルにして返します（短い、または空のページにも続きがある）。
- `scope` は RFC 6749 の scope 文字列（空白区切り・ソート済みの正規形）です。照合は文字列全体で行い、個々のスコープの検索には使わないため、1 カラムで持ちます。
- `oauth_clients` 以外のテーブルの `client_id` は `oauth_clients.id` を指します。クライアントが送る公開の識別子は `oauth_clients.client_id` です。
- 登録の経路（[ADR-0090](../../../adr/0090-third-party-oauth-apps.md) §1）は `first_party`（サーバー自身のクライアント。同意の画面を出さない）・`user`（利用者による登録、`owner_user_id` が登録者）・
  `metadata_document`（Client ID Metadata Document）・`dynamic`（動的クライアント登録、RFC 7591）です。一次のクライアントかどうかはこの経路だけで決めます（`identity.OAuthClient.FirstParty`）。
- 利用者による登録（ADR-0090 §1・§2、client API の `/api/v1/oauth_apps`）は、`oauth_clients` に `registration` が `user`・`owner_user_id` が登録者の行を作ります。
  - 名前・サイト・リダイレクト URI・スコープの上限を入れ、種類（public / confidential）は登録のときに決まって変わりません。confidential client の `client_secret`（`my_cs_` で始まる）は登録と再発行の応答に 1 度だけ出し、DB には SHA-256 のハッシュだけを保存します。
  - 変更は `oauth_client_revisions` の行を足し、リダイレクト URI は `oauth_client_redirect_uris` を置き換えます。再発行は `oauth_client_secret_rotations` の行を足し、古いシークレットはその時点で使えなくなります（猶予期間は持たない）。どちらもクライアントのトークンは失効させません。
    クライアントを読むときは、いちばん新しい変更と再発行を `oauth_clients` の値に重ねます。
  - 1 利用者あたり、削除していないクライアントは 25 件まで（登録のトランザクションで数える）。インスタンス設定 `user_app_registration` が無効なら登録を断ります（登録済みのものはそのまま使える）。
  - 登録者は、自分のアカウントへのトークンを redirect なしに発行できます。認可コードの交換を経ない通常のアクセストークンとリフレッシュトークンの組で、更新・失効・再利用の検出はほかのトークンと同じです。
  - 登録者の削除は `oauth_client_deletions` に理由 `owner` で記録します。登録者がアプリを使えない間（処分でログインできない、アカウントが削除された・削除の予定）は、そのクライアントを無効なクライアントと同じに断ります。
    ログインを制限する処分とアカウントの削除のトランザクションで、登録者のクライアントの全トークンを `client_disabled` で失効させます。
- クライアントの無効化・無効化の取り消し・削除は、`oauth_clients` に状態のカラムを足さず、イベントとして記録します（ADR-0090 §1）。
  - 無効なクライアント = 取り消しのない `oauth_client_disablements` の行があるもの、削除済みのクライアント = `oauth_client_deletions` の行があるものです。どちらも認可要求・トークンの交換と更新・トークンの失効の要求を未知のクライアントと同じに断り、API の認証ではそのクライアントのアクセストークンを受けません。
  - 無効化・削除は、同じトランザクションでそのクライアントの全トークン系列を `client_disabled` / `client_deleted` で失効させます。無効化を取り消しても、失効したトークンは戻りません（利用者が認可しなおす）。
  - 無効化は無効でないときだけ行を足し、取り消しは取り消しのない無効化すべてに行を足し、削除は 1 度だけ記録するので、どれも繰り返して安全です。一次のクライアントは無効化・削除しません。
- Client ID Metadata Document のクライアント（[ADR-0090](../../../adr/0090-third-party-oauth-apps.md) §1、経路 `metadata_document`）は、`client_id` が `https` の URL の認可要求で作ります。
  - `oauth_clients` の行は最初の取得のときに 1 度だけ作り（`client_id` = URL、`name` = URL のホスト名、`scope` = 一次専用を除くすべてのスコープ、シークレットなし）、以後は変えません。
    名前・サイトの URL・ロゴ・要求できるスコープの上限は自己申告で変わり得るので、`oauth_client_metadata_documents` に持ち、クライアントを読むときにこちらを使います。
  - 取得は認可要求のときだけ、`expires_at` を過ぎていれば行います（トークンの交換と更新は取得しません）。取得はトランザクションの外で行い、成功したときだけ同じトランザクションで `oauth_clients`（無ければ）・`oauth_client_metadata_documents`・`oauth_client_redirect_uris` を書きます。失敗はキャッシュしません。
  - `expires_at` は応答の HTTP のキャッシュ指示から決めます: `no-store`・`no-cache` は `fetched_at`（次の認可要求で取り直す）、`s-maxage`・`max-age`・`Expires` があればそれ、無ければ 1 時間。上限は 24 時間です。
  - 行は取得し直せば作り直せるキャッシュで、消しても次の認可要求で戻ります。
  - 管理者の無効化は、URL ごとにはクライアント（`oauth_client_disablements`）に、ホストごとには `oauth_client_host_denials` に記録し、キャッシュには持ちません。
    拒否したホストの `client_id` は取得の前に断り、認可要求・トークンの交換と更新・失効の要求を未知のクライアントと同じに断ります。ホストは小文字のホスト名（ポートなし）で照合します。
- ファーストパーティの Web クライアント（`client_id` は `my-web`、経路は `first_party`）は起動時に冪等に登録します。リダイレクト URI は設定 `web_origin` から導くので、`oauth_client_redirect_uris` は起動のたびに設定値へ置き換える【現在値】です。
- 認可コード・アクセストークン・リフレッシュトークンは、DB には SHA-256 ハッシュだけを保存します（ADR-0019 §4）。認可コードの 1 回性は `oauth_authorization_code_redemptions` の PK で保証します。
- `oauth_access_tokens` の 1 行は 1 回の発行（アクセストークンと、あればリフレッシュトークン）です。`expires_at` はアクセストークンの、`refresh_expires_at` はリフレッシュトークンの期限です。
  - リフレッシュのたびに同じ系列（`family_id` = 系列の最初の行の `id`）に新しい行を作り、古い行を `rotation` で失効させます。失効イベントは行のアクセストークンとリフレッシュトークンの両方を無効にします。
  - 失効済みの行のリフレッシュトークンが再び使われたら、系列の全行を `refresh_reuse` で失効させます（RFC 9700 §4.14.2）。使用済みの認可コードが再び使われたら、そのコードで始まった系列を `code_reuse` で失効させます（RFC 6749 §4.1.2）。
  - 有効なアクセストークン = 失効イベントが無く `expires_at` を過ぎていない行です。トークンの失効（RFC 7009）は系列全体を `user` で失効させます。
  - Web クライアントによる失効はログアウトなので、同じトランザクションで、系列の最初の行（`family_id`）→ `oauth_authorization_code_redemptions.token_id`
    → `oauth_authorization_codes.session_id` と辿ったセッションも `logout` で失効させます（すでに失効していれば何もしない）。
    サードパーティのクライアントの失効では、画面のセッションは終わりません。

#### 利用者設定【現在値】

| テーブル | カラム | 制約 |
| --- | --- | --- |
| `user_preferences` | `user_id`（PK）, `locale`（`lang`）, `default_visibility`（`enum(public, local, unlisted, private, direct)`）, `default_quote_policy`（`enum(public, followers, nobody)`。新しい投稿の引用の許可、ADR-0069 §1）, `default_post_language`（`lang`）, `require_media_description`（`bool`）, `email_moderator_notifications`（`bool`。モデレーターへの通知をメールでも受け取るか、FR-712）, `timeline_auto_insert`（`bool`。タイムラインの新着を確認なしで挿入するか、ADR-0077）, `updated_at` | |
| `user_timeline_languages` | `user_id`, `language`（`lang`） | PK `(user_id, language)`。行が無ければすべての言語を表示（FR-208） |
| `user_time_zones` | `user_id`（PK）, `time_zone`（IANA 名） | 行が無ければタイムゾーンを選んでいない（日時は UTC。Web クライアントは端末のゾーンを保存する） |

- 登録時に既定値の行を作ります。`locale` は登録時に指定されたロケール、無ければインスタンスの既定（`default_locale`）、それも無ければ `en` です。
  サーバーで描く登録のページは、要求の `Accept-Language` にいちばん合う、カタログのあるロケール（疑似ロケールは除く。`i18n.Resolver.AcceptedLocale`）を指定します。
  合うものが無ければ指定せず、インスタンスの既定になります。
  `user_time_zones` の行は作らない（タイムゾーンは選んでいない状態で、`UTC` として扱う）。`default_visibility` は `public`、`default_post_language` は `und`（クライアントに任せる）、`require_media_description` と `email_moderator_notifications` と `timeline_auto_insert` は偽です。
- `lang` の値は BCP 47 として検証し、正規形（`ja-JP`、`zh-Hant-TW`、`iw` → `he`）で保存します。未知のサブタグ、言語を明示しないタグ（`und-JP`、私用の `x-…`）は拒否します。
  `locale` は `und` も拒否します。
- タイムゾーンを選んだ事実は `user_time_zones` の行の有無で表します（ADR-0020 規約 4。`user_preferences` に NULL 許容の列や「選んだか」の列を置かない）。
  `time_zone` を指定した設定の変更が行を UPSERT します（`UTC` を選んでも行ができる）。API は `time_zone`（行が無ければ `UTC`）と `time_zone_chosen` を返します。
- `time_zone` は IANA タイムゾーンデータベースにある名前だけを受け付けます（`Local` や `+09:00` は不可）。サーバーのバイナリはデータベースを埋め込む（Go の `time/tzdata`）ので、検証結果はホストに依存しません。
- `user_timeline_languages` は 1 回の設定で全体を置き換えます（重複を除き、上限 64 件）。
- 一部の設定だけの変更（`PATCH /api/v1/preferences`）は、今の値の読み出しと書き込みを 1 つのトランザクションで行い、読み出しで `user_preferences` の行をロックします（PostgreSQL は `SELECT ... FOR UPDATE`、SQLite は書き込みトランザクションが直列）。
  同時に別々の設定を変えても（Web クライアントが端末のタイムゾーンを保存する間にユーザーがロケールを変える、など）、片方の変更が古い値で上書きされません。

#### データエクスポート

| テーブル | 分類 | カラム |
| --- | --- | --- |
| `data_exports` | リソース | `id`, `user_id`, `requested_at`。インデックス `(user_id, id DESC)` |
| `data_export_results` | イベント | `export_id`（PK）, `outcome`（`enum(ready, failed)`）, `blob_key`（NULL 可。`ready` のときだけ値を持つ）, `file_size`（NULL 可。同じ）, `expires_at`, `completed_at`。インデックス `(expires_at)` |

- 本人のデータのアーカイブ（FR-008）です。要求（`POST /api/v1/exports`）が `data_exports` の行とジョブ `identity.build_export` を同じトランザクションで積みます。作成中（結果のない行）の要求があるあいだは、次の要求を受け付けません（`409`）。
- ジョブは ZIP のアーカイブを Blob の非公開のキー `exports/{user_id}/{export_id}.zip` に書き、`ready` の結果を記録します。中身は `actor.json`（Actor）、`outbox.json`（本人の削除されていない投稿すべてを、公開範囲に関わらず `Create` の `OrderedCollection` にしたもの。ブーストは含めない）、`moderation.json`（本人への処分と異議申し立て、本人が出した通報。非公開のメモと通報者は含めない。ADR-0038）と、下の「CSV のエクスポートとインポート」の 4 つの CSV です。
  結果のある行は作り直さないので、再実行してよい。5 回失敗すると `failed` を記録します。
- 状態は結果から求めます: 結果がなければ作成中、`expires_at` を過ぎていれば期限切れ、それ以外は `outcome` のとおり。`expires_at` は結果の記録から 7 日です（`identity.ExportTTL`）。
- 1 時間ごとのジョブ `identity.expire_exports` が、期限を過ぎたアーカイブ（`ready` は Blob を先に消す）と行を消します。アカウントの消去（§8）も、本人の要求と Blob をすべて消します。

#### CSV のエクスポートとインポート

| テーブル | 分類 | カラム |
| --- | --- | --- |
| `data_imports` | リソース | `id`, `user_id`, `account_id`, `kind`（`enum(following, blocks, mutes, domain_mutes, bookmarks)`）, `mode`（`enum(merge, overwrite)`）, `created_at`。インデックス `(user_id, id DESC)`、`(created_at)` |
| `data_import_items` | リソース（インポートの子） | `import_id`, `position`（PK は `(import_id, position)`）, `action`（`enum(add, remove)`）, `target`, `show_boosts`, `notify`, `hide_notifications` |
| `data_import_item_results` | イベント（項目の子） | `import_id`, `position`（PK）, `outcome`（`enum(done, failed)`）, `failure`（NULL 可。`failed` のときだけ `enum(unresolvable, unavailable, blocked, restricted, self, limit)`）, `processed_at` |

- 本人のフォロー・ブロック・ミュート・ミュートしたドメイン・ブックマークを、Mastodon の「インポートとエクスポート」と同じ CSV で読み書きします（FR-008）。Mastodon が書き出した CSV をこちらで、こちらが書き出した CSV を Mastodon で読み込めます。
  - フォローは `following_accounts.csv`（`Account address,Show boosts,Notify on new posts,Languages`。承認済みのフォローだけ）、ブロックは `blocked_accounts.csv`（1 行に 1 アドレス、見出しなし）、ミュートは `muted_accounts.csv`（`Account address,Hide notifications`。期限の過ぎていないもの）、ミュートしたドメインは `blocked_domains.csv`（1 行に 1 ドメイン、見出しなし。読み込みでは Mastodon の見出し `#domain` も受け付ける。Mastodon の利用者単位のドメインブロックはこちらのドメインのミュートになる、FR-304）、ブックマークは `bookmarks.csv`（1 行に投稿の URI 1 つ、見出しなし。今も見られる投稿だけ）です。
  - アドレスは `username@domain` で、このサーバーのアカウントはアカウントドメインを付けます。handle を確かめていないリモートのアカウントは書き出しません。
  - エクスポートは要求のたびにその時点の内容から作り（`GET /api/v1/csv_exports/{kind}`）、テーブルを持ちません。データのアーカイブにも同じ 5 つのファイルを入れます。
- 読み込みは Mastodon の読み方に合わせます（`identity.ParseImportCSV`）。見出しの行は任意で、最初の欄が既知の見出し（`Account address` など）なら見出し、そうでなければ種類の既定の列とします。アドレスの先頭の `@` は落とし、空行は無視し、省いた設定は Mastodon と同じ既定（ブーストを表示する・新しい投稿を通知しない・ミュートした相手の通知を隠す）にします。フォローの `Languages` は使いません。
  アドレス（ブックマークは `http(s)` の URL、ミュートしたドメインはホスト名）でない行は `invalid`、前の行と同じ相手（ユーザー名とドメインの大文字小文字を区別しない）は `duplicate` として飛ばします。1 ファイルは 10,000 件まで（`identity.MaxImportEntries`）です。
- 開始（`POST /api/v1/imports`）は、ファイルの各項目を `add` の項目に、`overwrite` では今あるもののうちファイルにないもの（アドレスを同じ規則で、ドメインは IDNA の A ラベルの小文字にして比べる）を `remove` の項目にし、`data_imports` の行・項目・最初のジョブ `identity.process_import` を 1 つのトランザクションで書きます。
  `remove` の項目の `target` は相手のアカウントか投稿の ID かドメイン、`add` の項目の `target` はファイルに書かれたアドレスか URL です。プレビュー（`POST /api/v1/imports/preview`）は同じ比較をして件数を返し、何も書きません。
  結果のない項目が残るインポートがあるあいだは、次のインポートを受け付けません（`409`）。
- ジョブは結果のない項目を 20 件ずつ順に行い、項目ごとに結果を記録してから次の 20 件のジョブを積みます（冪等キーは次の位置を含み、順序キーでインポートごとに 1 本ずつ流す）。
  リモートのアカウントは WebFinger と Actor の取得で解決し（検索と同じ `federation.RemoteActors.ResolveHandle`）、投稿は URL で解決して（`federation.Resolver`）、フォロー・ブロック・ミュート・ブックマークの各ユースケースを呼びます。リモートへの `Follow` の配送は、フォローのイベントから連合のモジュールが積むジョブが行います。
  同じ項目をもう一度行っても変わらない操作なので、ジョブの再実行は安全です。見つからない・処分された・ブロックで妨げられた・自分自身・ミュートできないドメイン（このサーバーのドメインは `unresolvable`、上限を超えたものは `limit`）などの項目は `failed` を記録して先へ進み、それ以外のエラーはジョブの再試行に任せます。5 回目の実行でも失敗する項目は `failed`（`unavailable`）にして、インポートが止まらないようにします。
- 状態は件数から求めます: 項目の数とすべての結果の数が等しければ `finished`、そうでなければ `in_progress`。
- 1 時間ごとのジョブ `identity.expire_imports` が、作成から 7 日（`identity.ImportTTL`）を過ぎたインポートを項目と結果ごと消します。アカウントの消去（§8）も、本人のインポートをすべて消します。
- 利用者ごとのドメインの非表示（Mastodon の `blocked_domains.csv`）は、その機能（#638）がまだないので扱いません。
