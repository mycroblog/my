### 4.2 account

ローカル・リモートの Actor です。識別（`accounts`）、部分型、プロフィール（版）、ライフサイクル（イベント）を分けます。

#### accounts【リソース】と部分型

| テーブル | 分類 | カラム | 制約 |
| --- | --- | --- | --- |
| `accounts` | リソース | `id`, `uri`, `domain`（`''` = ローカル）, `actor_type`（`enum(person, service, application, group, organization)`）, `created_at` | 一意 `(uri)`。インデックス `(domain, id)`。部分一意 `(domain, actor_type) WHERE domain = '' AND actor_type = 'application'`（インスタンス Actor は 1 つ） |
| `local_accounts` | リソース（部分型） | `account_id`（PK）, `user_id`, `username`, `username_normalized` | 一意 `(user_id)`, `(username_normalized)`。ローカルのユーザー名は変更しない（URI は ID ベースだが、ハンドルの再利用による混乱を避けるため） |
| `remote_accounts` | リソース（部分型） | `account_id`（PK）, `first_seen_at` | |
| `remote_account_snapshots` | イベント | `id`, `account_id`, `preferred_username`, `profile_url`（NULL 可）, `inbox_url`, `shared_inbox_url`（NULL 可）, `outbox_url` / `followers_url` / `following_url` / `featured_url`（NULL 可。持たない実装がある）, `followers_total` / `following_total` / `statuses_total`（`bigint`、NULL 可）, `document_digest`（`bytes`。文書の SHA-256）, `fetched_at` | インデックス `(account_id, id DESC)`。取得のたびに 1 行。内容（ダイジェスト）が前回と同じなら挿入しない |
| `remote_account_heads` | 投影 | `account_id`（PK）, `snapshot_id`, `inbox_url`, `shared_inbox_url`（NULL 可）, `last_fetched_at`, `consecutive_fetch_failures`（`int`） | インデックス `(last_fetched_at)`（定期リフレッシュ）、`(shared_inbox_url)`（アカウント削除の `Delete` を送る既知の shared inbox、§8）。取得のたびに最新のスナップショットを指し、`last_fetched_at` を取得の時刻、`consecutive_fetch_failures` を 0 にする。取得に失敗したら `consecutive_fetch_failures` を 1 増やし、`last_fetched_at` を失敗の時刻にする（次の定期リフレッシュを 24 時間後に送る） |
| `remote_account_handle_verifications` | イベント | `id`, `account_id`, `username`, `domain`, `outcome`（`enum(verified, mismatch)`）, `checked_at` | インデックス `(account_id, id DESC)`。WebFinger で `acct:{username}@{domain}` を確かめた記録（[federation.md §6.2](../../federation.md#62-actor-とハンドル)）。`verified` は `self` リンクが Actor の `uri` を指したこと、`mismatch` は答えがそれを確かめなかったこと。結果が最新の記録と同じなら挿入しない |
| `account_handles` | 投影 | `account_id`（PK）, `username`, `username_normalized`, `domain` | **一意 `(username_normalized, domain)`**。WebFinger・`@user@host` の解決（FR-305, FR-401）。ローカルは `local_accounts`、リモートは最新の `remote_account_handle_verifications` が `verified` のときだけ作る（確かめていない・`mismatch` のリモートのアカウントは行を持たない） |
| `account_search_terms` | 投影 | `account_id`, `kind`（`enum(username, display_name)`）, `term`, `is_local`（`bool`） | PK `(account_id, kind, term)`。インデックス `(term, account_id)`（PostgreSQL は `term` を `COLLATE "C"`、SQLite は既定の `BINARY` でバイト順）。アカウントの前方一致の検索（FR-119, FR-306, ADR-0092 §1）が照合する語 |

- インスタンス自身の Actor（`Application`）もローカルの `accounts` に 1 行持ちます（`local_accounts` と `account_handles` には入れません）。
  ローカルの `application` はこの 1 行だけなので、部分一意インデックスで二重作成を防ぎ、作成を冪等にします。
- ローカルアカウントの `uri` は `https://{host}/ap/actors/{id}`（ADR-0018）です。ハンドルの `domain` は `account_domain` ではなく `''` です（設定値に依存させない）。
- リモートのハンドルが変わった場合、同じハンドルを別の Actor が後から名乗ることがあります。後から `verified` を記録した Actor がハンドルを持ち、先に持っていた Actor の行は投影から消えます（再構築も記録の `id` 順に当てはめるので同じ結果になります）。
- `account_search_terms` は、ハンドル（`account_handles`）・プロフィールの現在の版（`account_profile_heads`）・ライフサイクル（`account_lifecycle_states`）と、`moderation` が凍結しているか（`AccountPolicy.SuspendedTx`）から作ります（ADR-0092 §1）。
  - 語は、`username_normalized` と、リモートのアカウントでは加えて `username@domain`（`kind = username`）、表示名から `:shortcode:` を除いて NFKC + 小文字化した全体と、空白で区切った各語（`kind = display_name`、20 語まで）です。CJK の表示名は語に分けないので、先頭からの前方一致になります。語は 100 文字（検索の `q` の上限）で切ります。
  - `active` でない・凍結中（自身の処分による `suspend` / `ban`）・ハンドルのない・`Application` のアカウントは行を持ちません。停止したドメインのアカウントは行を持ち、検索のときに除きます。
  - 同期投影です（ADR-0020 規約 7）。`SearchTermProjector` が、ローカルのアカウントの作成・プロフィールの版・ライフサイクルのイベント・ハンドルの確認（ハンドルを奪われたアカウントも）と、`moderation` の `EventAccountSuspensionChanged` のたびに、そのアカウントの行を作り直します。再構築（`my projection rebuild account_search_terms`）は、ハンドルを持つアカウントごとに同じ規則で作ります。
  - 検索は `term >= q AND term < q || U+10FFFF` の範囲を索引の順に最大 100 行読み、`graph` の `CandidateRelations`（ブロックの双方向・期限内のミュート・フォロー・フォロワー数）と `moderation` のドメインの停止で除いてから、完全一致 → フォロー中 → ローカル → フォロワーの多い順に並べます。`graph` のテーブルは JOIN しません。

#### プロフィール【イベント（版）】

| テーブル | 分類 | カラム | 制約 |
| --- | --- | --- | --- |
| `account_profile_revisions` | イベント | `id`, `account_id`, `seq`, `display_name`, `note_html`, `locked`, `bot`, `discoverable`, `indexable`, `ai_opt_out`（`bool`。ADR-0057）, `avatar_media_id` / `header_media_id`（NULL 可）, `revised_at` | 一意 `(account_id, seq)` |
| `account_profile_revision_sources` | イベント（部分型） | `revision_id`（PK）, `note_source` | ローカルのみ。入力されたままの自己紹介 |
| `account_profile_fields` | イベント（版の子） | `revision_id`, `position`（`int`）, `name`, `value_html` | PK `(revision_id, position)`（FR-006） |
| `account_profile_revision_emojis` | イベント（版の子） | `revision_id`, `emoji_id` | PK `(revision_id, emoji_id)`。インデックス `(emoji_id)`（リモートの絵文字の消去）。表示名・自己紹介・補足情報のカスタム絵文字（ADR-0051）。`emoji_id` は `content` の `custom_emojis` の行で、`content` の公開 API で得る |
| `account_profile_field_sources` | イベント（部分型） | `revision_id`, `position`, `value_source` | PK `(revision_id, position)`。ローカルのみ。入力されたままの補足情報の値 |
| `account_profile_field_verifications` | イベント | `revision_id`, `position`, `verified_at` | PK `(revision_id, position)`。`rel="me"` リンクの検証結果 |
| `remote_profile_revisions` | イベント（部分型） | `revision_id`（PK）, `snapshot_id`, `html_policy_version`（`int`） | リモートのみ。版を作った取得と、`note_html` / `value_html` をサニタイズしたポリシーの版（ADR-0019 §2） |
| `account_profile_heads` | 投影 | `account_id`（PK）, `revision_id` | `seq` が最大の版を指す |

- 鍵アカウント（`locked`）・bot などのフラグはプロフィールの一部として版を持ちます。いつから鍵アカウントだったかが分かります。
- 新しい版の `seq` は現在の版の `seq + 1` です。同時に版を作って `(account_id, seq)` が衝突したら、トランザクションを再実行して次の `seq` を取ります。
- ローカルアカウントは作成時に `seq = 1` の版を持ちます。
- ローカルの版は、入力されたままの自己紹介と補足情報の値を `account_profile_revision_sources` と `account_profile_field_sources` に持ちます。
  サーバーはそれを投稿の本文と同じ規則（メンション・ハッシュタグ・URL をリンクにし、ほかはエスケープ）で HTML にして `note_html` と `value_html` に保存します（ADR-0019 §2）。
  補足情報の `name` はプレーンテキストです。プロフィールの更新は、変えない項目を現在の版から写した新しい版を作ります。
- アバター・ヘッダーにできるのは、本人がアップロードして処理が成功した `media_files` だけです。
  表示する画像は、版が指すファイルの現在の処理結果（§4.4 メディア）の `full` 版です。ファイルの所有者がそのアカウントでないか、現在の処理結果が `ready` でなければ表示しません。
  アカウントの一覧・タイムライン・通知は、1 ページ分のアカウントの画像を `content` モジュールの公開 API（`MediaService.ImageURLs`）で 1 回の問い合わせにまとめて読みます。
- リモートのプロフィールは、取得した文書から版を作ります（内容が変わったときだけ）。`summary` と補足情報の値は取り込み時にサニタイズして `note_html` / `value_html` に保存し、表示名と補足情報の名前はプレーンテキストにします（[federation.md §5](../../federation.md#5-リモート-html-のサニタイズ)）。
  Actor の `tag` の `Emoji` は、投稿の絵文字と同じ規則（§4.4 のリモート投稿のコマンド）で `content` の公開 API（`Service.RecordRemoteEmojis`）が記録し、版の `account_profile_revision_emojis` に結びます。絵文字の組が変わったときも新しい版になります。
- ローカルのアカウントとの関係が無いリモートのアカウント（どちらの向きのフォローも無く、保存した投稿も無い）は、1 日ごとの消去ジョブ `account.prune_remote_profiles` が、プロフィールの版を現在の 1 つに縮めます（[ADR-0034](../../../adr/0034-remote-content-scope-and-retention.md)）。
  現在の版でない版と子テーブルの行（`account_profile_fields`、`account_profile_revision_emojis`、`remote_profile_revisions`）、`remote_account_heads` と現在の版のどちらも指さない `remote_account_snapshots` を消し、`accounts`・`remote_accounts`・鍵は残します。
  関係の判定は `graph` と `content` の公開 API で行い（`account` はこの 2 つを import しないので、組み立てる側が渡す）、トランザクションの前に読みます。1 トランザクションで調べるのは 100 アカウントまでで、続きはカーソル（`account_id`）を持った次のジョブが読みます。

#### 署名鍵

| テーブル | 分類 | カラム | 制約 |
| --- | --- | --- | --- |
| `account_keys` | リソース | `id`, `account_id`, `key_id`（`iri`）, `algorithm`（`enum(rsa_sha256, ed25519)`）, `public_key_pem`（PKIX の `PUBLIC KEY`）, `fingerprint`（`bytes`。DER の SHA-256）, `created_at` | 一意 `(key_id, fingerprint)`。インデックス `(key_id, id DESC)`, `(account_id)` |
| `local_account_private_keys` | リソース（部分型） | `account_key_id`（PK）, `private_key_enc`（`bytes`） | NFR-SEC-7。リストア後も変わらないこと（NFR-REL-4） |
| `account_key_revocations` | イベント | `account_key_id`（PK）, `revoked_at` | |

- ローカルの Actor（インスタンス Actor を含む）は、作成時に RSA 2048 ビット（`rsa_sha256`、`key_id` = `{actor}#main-key`）と Ed25519（`ed25519`、`key_id` = `{actor}#ed25519-key`）の 2 つの鍵を持ちます。
- リモートの Actor は、同じ `key_id` のまま公開鍵を替えることがあります（Mastodon の `#main-key`）。取得した鍵の `fingerprint` が既存の行と違えば新しい行を作り、同じトランザクションで古い行に `account_key_revocations` を記録します。文書が宣言しなくなった鍵も同じように失効させます。別のアカウントの鍵になっている `key_id` は書き換えません。署名の検証は、`key_id` の失効していない最新の行を使います（[ADR-0032](../../../adr/0032-http-signature-profile.md)）。
- `private_key_enc` は PKCS #8 の秘密鍵を `kernel.Sealer` で暗号化した値です（ADR-0019 §6）。AAD は `account_key_id` と列名 `local_account_private_keys.private_key_enc` で、別の行・列に写すと復号できません。

#### ライフサイクル

| テーブル | 分類 | カラム | 制約 |
| --- | --- | --- | --- |
| `account_lifecycle_events` | イベント | `id`, `account_id`, `kind`（`enum(deactivated, reactivated, deletion_scheduled, deletion_cancelled, deleted, remote_deleted)`）, `initiated_by`（`enum(self, moderator, system, remote)`）, `moderation_action_id`（NULL 可）, `deletion_due_at`（NULL 可。`deletion_scheduled` のみ）, `occurred_at` | インデックス `(account_id, id DESC)` |
| `account_lifecycle_states` | 投影 | `account_id`（PK）, `state`（`enum(active, deactivated, deletion_scheduled, deleted)`）, `since`, `deletion_due_at`（NULL 可） | インデックス `(state, deletion_due_at)`（削除の実行） |
| `account_erasures` | イベント | `account_id`（PK）, `erased_at` | 消去ジョブがアカウントのデータを消し終えた記録（§8）。1 アカウントにつき 1 回 |

- **ライフサイクルは本人とシステムの軸** です。モデレーションの処分は別の軸（§4.3）に置きます。
  - BAN のように処分が削除を伴う場合は、処分が `deletion_scheduled` イベントを `initiated_by = moderator` で発生させます（`moderation_action_id` で根拠を辿れる）。BAN への異議申し立てを覆すと、`deletion_cancelled`（`initiated_by = moderator`、同じ `moderation_action_id`）で取り消します（`account.Service.CancelDeletionByModeration`）。
    期日は処分から 30 日後（`moderation.BanDeletionDelay`）で、処分と同じトランザクションで記録します（`account.Service.ScheduleDeletionByModeration`）。
    削除済みのアカウントと、それより早い期日の削除がすでに予定されているアカウントには記録しません。
- 本人による削除（FR-008、`POST /api/v1/accounts/delete`）は、現在のパスワードかその利用者のパスキーで確かめてから、猶予を置かずに `deleted`（`initiated_by = self`）を記録します（`account.Service.DeleteBySelf`）。
  本人による削除の予約はログインできない（§4.1）ので、予約しても本人には取り消せないためです。データの消去は、ほかの削除と同じく `deleted` から 7 日後です（下記）。削除済みのアカウントには記録しません。
- 期日を過ぎた `deletion_scheduled` は、1 時間ごとのジョブ `account.execute_deletions` が `deleted`（`initiated_by = system`）にします（`account.Service.ExecuteDueDeletions`）。
  1 回で 100 件まで、アカウントごとに短いトランザクションで、状態を読み直してから記録するので、再実行しても二重に削除しません。
  `deleted` は既存の購読者を通して、アカウントと投稿を表示から外し、`Delete`（Actor）を送ります（[federation.md §7.1](../../federation.md)）。
- `deleted` から 7 日（`account.ErasureDelay`。`Delete`（Actor）の配送のリトライが終わるまで、鍵とフォローが要る）を過ぎたローカルのアカウントは、1 時間ごとのジョブ `account.erase_deleted` がデータを消します（§8）。
- 行が無いアカウントは `active` とみなします。作成時には行を作らず、最初のライフサイクルイベントで作ります（イベントからの再構築と一致させるため）。
- 投影はイベントを `id` 順に畳み込んで作ります: `deactivated` → `deactivated`、`reactivated` / `deletion_cancelled` → `active`、`deletion_scheduled` → `deletion_scheduled`（`deletion_due_at` を写す）、`deleted` / `remote_deleted` → `deleted`。`deleted` は最終状態で、以後のイベントでは変わりません。`since` は最後に状態を変えたイベントの `occurred_at` です。
- `deletion_due_at` は、イベントでは `kind = deletion_scheduled` のとき、投影では `state = deletion_scheduled` のときだけ値を持ちます（`CHECK` 制約）。

#### 移転（ADR-0089）

| テーブル | 分類 | カラム | 制約 |
| --- | --- | --- | --- |
| `account_moves` | イベント | `id`, `account_id`（移転元）, `target_account_id`, `initiated_by`（`enum(self, remote)`）, `activity_uri`（NULL 可。受けた `Move`）, `occurred_at` | 一意 `(account_id, target_account_id)`、インデックス `(account_id, occurred_at DESC, id DESC)`、`CHECK (account_id <> target_account_id)` |
| `account_move_cancellations` | イベント | `move_id`（PK）, `cancelled_at` | ローカルの移転だけ |
| `account_move_states` | 投影 | `account_id`（PK）, `move_id`, `target_account_id`, `since` | インデックス `(target_account_id)` |

- **移転はライフサイクルと直交する軸** です（ADR-0020 規約 2）。移転済みでも `account_lifecycle_states` は変わりません。
- リモートのアカウントの移転は、検証を通った `Move` を `account.Service.RecordRemoteMove` が `initiated_by = remote` で記録します（[federation.md §3.4](../../federation.md)）。同じ移転先への 2 度目の `Move` は一意制約で何も記録せず、別の移転先への `Move` は最新の行の `occurred_at` から 7 日（`account.RemoteMoveInterval`）以内なら記録しません（待ち時間は状態の列を持たず、読み取りのときに判定します）。
- ローカルのアカウントの移転は、`federation.LocalMoves` が移転先を確かめた後に `account.Service.RecordLocalMove` が `initiated_by = self` で記録します。最新の行の `occurred_at` から 30 日（`account.LocalMoveInterval`）以内は、その移転が取り消されていても記録せず、前に移った移転先へは一意制約のため移れません。移転元が制限中で移転先がローカルのとき、`account.moved` は監査ログ `account_moved` の ID を運び、`moderation` がその記録を書きます。
- 取り消し（`account.Service.CancelMove`）は `account_move_cancellations` を記録し、ドメインイベント `account.move_cancelled` で projector が `account_move_states` の行を消します。`federation` は `movedTo` の無い `Update(Actor)` を送ります。
- 記録は同じトランザクションでドメインイベント `account.moved` を出し、projector が `account_move_states` をその移転に向けます。`moderation` が移転元を `limit` しているかを運び（`SourceLimited`）、`notification` がモデレーターに知らせます。`graph` はこのイベントからジョブ `graph.migrate_follows` を積みます（§4.5）。
- 投影はアカウントごとの最新の移転（`occurred_at`、`id` の順）で、それが取り消されていれば行を作りません（`account.MoveStates`）。取り消しは行を消します。
- 移転先は client API のアカウントの表現の `moved`（アカウント自身のエンドポイント）に出します（`account.Service.MoveState`）。
