## 2. 整合性モデル

### 2.1 トランザクションの規律

ADR-0003（短く再実行可能なトランザクション、内部で外部 I/O をしない）と ADR-0020 規約 7 に従います。

- 1 つのコマンドは、**1 つのモジュールのリソース / イベント** を書き込みます。加えて、そのイベントを購読する **同期 projector**（自分の投影を更新）と投入するジョブ（`jobs`、`Outbox.EnqueueTx`）を、同じトランザクションで書きます。
- projector は購読するイベントの内容だけで投影を更新し、他モジュールのテーブルを読みません。必要な値はイベントに載せます。
- 同期 projector は起動時に `uow.Projectors` へイベント種別ごとに登録します。コマンドがトランザクション関数の中で `tx.Emit(ctx, event)` を呼ぶと、登録順に同じトランザクションで実行され、projector の失敗はコマンドごとロールバックします。
- コミットフック（`uow.Projectors.OnCommit`）は、コミットしたトランザクションが `tx.Emit` で発行したイベントを、コミットの後にトランザクションの外で受け取ります。
  ロールバックした試行・再実行で捨てた試行のイベントは届きません。フックはプロセス内の通知（リアルタイム更新、[overview §4.4](../overview.md#44-リアルタイム更新sse)）だけに使い、失わせてはいけない処理は outbox のジョブにします。
  投影の再構築（§9）は `tx.Emit` を通らないので、フックを起こしません。
- 件数が利用者数・フォロワー数に比例する処理は、非同期 projector またはカーソル付きのジョブで、一定件数ずつ行います。
  - 該当するのは、配送先の展開・消去・アカウント単位の制限の全投稿への反映などです。
- 時刻と ID はトランザクション関数の外で一度だけ生成して渡します（再実行で値が変わらないように）。

| コマンド | 書き込むリソース / イベント | 同期で更新する投影 | outbox |
| --- | --- | --- | --- |
| 利用者登録 | `users`, `password_credentials`, `accounts`, `local_accounts`, `account_profile_revisions`, `account_keys`, `local_account_private_keys`（登録画面からは加えて `user_email_addresses`, `registration_applications`, `registration_decisions`, `invite_redemptions`, `sessions`） | `account_handles`, `account_profile_heads`（登録画面からは加えて `user_primary_emails`, `user_registration_states`, `session_activity`） | なし（登録画面からは確認メールのジョブ。ジョブが `user_tokens` にトークンを書いてから送る、[ADR-0086](../../adr/0086-email-delivery.md) §1） |
| 投稿 | `statuses`, `local_statuses`, `status_revisions` と子テーブル | `status_heads`, `status_mention_index`, `status_reply_links`, `status_counters`, `feed_entries`, `feed_tag_entries`, `feed_direct_recipients`, `account_post_counters`, `local_post_counters`, `notifications`（メンション） | 配送の展開 |
| 投稿の編集 | `status_revisions` と子テーブル | `status_heads`, `status_mention_index`, `feed_entries`（言語）, `feed_tag_entries`, `feed_direct_recipients`, `notifications`（増えたメンション） | `Update` の配送 |
| 投稿の削除 | `status_deletions` | `status_heads`, `status_mention_index`, `status_reply_links`, `status_counters`, `account_post_counters`, `local_post_counters`, `boost_edges`, `feed_entries`, `feed_tag_entries`, `feed_direct_recipients`, `notification_deleted_statuses` | `Delete` の配送 |
| ブースト | `boosts` | `boost_edges`, `feed_entries`, `status_counters`, `local_post_counters`, `notifications` | `Announce` の配送（`local` 投稿のブーストは連合しない） |
| ブーストの取り消し | `boost_retractions` | `boost_edges`, `feed_entries`, `status_counters`, `local_post_counters` | `Undo Announce` の配送（連合したブーストだけ） |
| お気に入り / 取り消し | `favourites`（現在値） | `status_counters`, `notifications`（お気に入りだけ） | `Like` / `Undo Like` の配送（リモートの投稿だけ） |
| 絵文字リアクション / 取り消し | `reactions`（リソース。取り消しで行を消す） | `status_reaction_counts`, `notifications`（リアクションだけ） | 絵文字付きの `Like` / `Undo Like` の配送（ローカルの人のリモートの投稿へのものだけ） |
| ブックマーク・ピン留め / 取り消し | `bookmarks` / `status_pins`（現在値） | なし | なし |
| フォロー | `follows`（＋自動承認なら `follow_acceptances`） | `follow_edges`, `feed_follow_edges`, `follow_counters`, `notifications` | `Follow` / `Accept` の配送 |
| ブロック | `blocks`（＋両方向のフォローの `follow_terminations`） | `block_edges`, `follow_edges`, `feed_follow_edges`, `feed_exclusions`, `follow_counters`, `notification_exclusions` | `Block` の配送 |
| ミュート / 解除 | `mutes`（現在値） | `feed_exclusions`, `notification_exclusions` | なし |
| ドメインのミュート / 解除 | `account_domain_mutes`（現在値） | `feed_domain_mutes`, `notification_domain_exclusions` | なし |
| 処分 | `moderation_actions`（＋`moderation_action_statuses`） | `account_restrictions`, `feed_author_restrictions` | 通知・投稿の削除やセンシティブ化（大量なら非同期） |
| Inbox 処理 | `received_activities` と対象のリソース / イベント | 対象に応じて | 対象に応じて |

表の「outbox」の列は、同じトランザクションで `Outbox.EnqueueTx` が `jobs` に書くジョブです。外部のキューを使うときもジョブは `jobs` に入ります（[ADR-0120](../../adr/0120-external-queue-wake-up-signals.md)）。

利用者登録は、ログイン主体（`identity`）とそれが名乗るアカウント（`account`）が片方だけ存在する状態を作らないよう、例外として 2 つのモジュールに書き込みます。
`identity` のコマンドが `account` のアプリケーション層の公開 API（`PrepareLocalAccount` / `WriteLocalAccount`）を同じトランザクションの中で呼び、`account` のテーブルは `account` のコードだけが書きます。
鍵の生成・暗号化とパスワードのハッシュ計算は、トランザクションの前に済ませます。

PostgreSQL の並行性の挙動（READ COMMITTED、`SELECT ... FOR UPDATE`、`FOR UPDATE SKIP LOCKED`、トランザクションスコープのアドバイザリロック、デッドロック（40P01）の再実行）は、
同じコントラクトテストが確かめます。毎 PR では pgmem（PGlite のフォークの PostgreSQL 18）の上で、遅い検査（リリースの関門・nightly・手動）では本物の PostgreSQL のコンテナの上で流します（[ADR-0041](../../adr/0041-postgres-tests-on-pgmem.md)、[ADR-0079](../../adr/0079-ci-single-gate-merge-queue.md)）。

### 2.2 冪等キーと一意制約

| 対象 | キー | 用途 |
| --- | --- | --- |
| 受信アクティビティ | `received_activities (activity_uri)` | Inbox の受信時の重複排除（[ADR-0033](../../adr/0033-inbox-processing-model.md)）。`id` を持たないものは `actor` と本文の SHA-256 から `urn:my:digest:<hex>` を合成する（[federation §3.1](../federation.md#31-同期の部分)） |
| 順序の入れ替わり | `remote_object_tombstones (uri, actor_uri)`、`remote_follow_tombstones (activity_uri)`、`status_deletions (uri)` | 先に届いた `Delete` / `Undo` の後に届いた `Create` / `Follow` などを捨てる |
| AP オブジェクト | `accounts (uri)`, `statuses (uri)`, `boosts (uri)`, `follows (uri)`, `follow_resends (uri)`, `blocks (uri)`, `reports (uri)`, `poll_votes (uri)`, `custom_emojis (uri)` | 受信・取得の重複排除 |
| 版 | `status_revisions (status_id, seq)`, `account_profile_revisions (account_id, seq)` | 同じ `Update` の再処理を no-op にする。リモートの `updated` が現在版以下なら版を作らない |
| 状態遷移イベント | `follow_acceptances (follow_id)`, `follow_rejections (follow_id)`, `follow_terminations (follow_id)`, `boost_retractions (boost_id)`, `block_retractions (block_id)`, `moderation_action_lifts (action_id)` | 同じ遷移の二重記録を防ぐ（1 リソースにつき 1 回だけ起きる遷移は PK にする） |
| 1 回限りの引き換え | `oauth_authorization_code_redemptions (code_hash)`, `user_token_redemptions (token_id)`, `invite_redemptions (invite_id, seq)` | 条件付き UPDATE の代わりに、引き換えイベントの一意制約で 1 回性を保証する |
| クライアントからの投稿 | `local_statuses (author_account_id, idempotency_key)` | API の `Idempotency-Key`。再送には同じ投稿を返す |
| 通知 | `notifications (recipient_account_id, dedupe_key)` | 同じイベントの再処理や繰り返した操作で通知を増やさない（§4.7） |
| 配送 | `deliveries (outbound_activity_id, inbox_url)` | 展開ジョブの再実行で重複しない |
| ジョブ | `jobs (idempotency_key)` | 未完了のジョブの重複投入を no-op にする（ADR-0004） |

### 2.3 集計値

- カウンタは【投影】です。イベントを集計するモジュールが所有します。
  - `account_post_counters`・`local_post_counters`・`status_counters` は `content`、`follow_counters` は `graph` が所有します。
- 加算・減算は、元のイベントが実際に挿入された（影響行数 = 1）ときだけ行います。
- ずれは定期ジョブ `projection.verify`（1 日ごと）が、イベントから数え直して補正します（§9）。
- リモートアカウントのフォロワー数などは、取得スナップショットの `totalItems` を表示に使います。ローカルのカウンタは「このインスタンスが知っている件数」として別に持ちます。
