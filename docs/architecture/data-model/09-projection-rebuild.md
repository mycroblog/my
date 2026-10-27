## 9. 投影の再構築と検証

| 投影 | 元になるイベント / リソース | 更新 |
| --- | --- | --- |
| `account_restrictions` | `moderation_actions`, `moderation_action_lifts`, §5.2 の対応表 | 同期 |
| `domain_restrictions` | `domain_moderation_actions`, `domain_moderation_action_lifts` | 同期 |
| `report_states` | `reports`, `report_assignments`, `report_resolutions`, `report_reopenings` | 同期 |
| `appeal_states` | `moderation_appeals`, `moderation_appeal_decisions` | 同期 |
| `status_sensitivity_overrides` | `mark_statuses_sensitive` の `moderation_actions` と `moderation_action_statuses`、`moderation_action_lifts` | 同期 |
| `account_lifecycle_states` | `account_lifecycle_events` | 同期 |
| `user_registration_states` | `registration_applications`, `registration_decisions` | 同期 |
| `user_roles` | `user_role_grants`, `user_role_revocations` | 同期 |
| `user_primary_emails` | `user_email_addresses` | 同期 |
| `account_handles` / `remote_account_heads` / `account_profile_heads` | `local_accounts` と `remote_account_handle_verifications`、`remote_account_snapshots` と取得の結果、`account_profile_revisions` | 同期 |
| `account_search_terms` | `account_handles`・`account_profile_heads`・`account_lifecycle_states` と `moderation` の凍結（`account_restrictions`） | 同期（再構築は Go の `account.SearchTermProjector.Rebuild`。`moderation` の後に走る） |
| `status_heads` / `status_mention_index` / `status_reply_links` | `status_revisions` と子テーブル、`statuses`、`status_deletions` | 同期 |
| `status_link_cards` | `status_revisions` の本文・ソース・添付・投票、`status_deletions` | 同期（URL は Go の `content.LinkCardURL` が選ぶので、`my projection` の対象ではない） |
| `status_quote_states` | `status_quotes`, `status_quote_events` | 同期（再構築は Go の `content.RebuildQuoteStates`） |
| `follow_edges` / `block_edges` / `boost_edges` | `follows` と承認・拒否・終了、`blocks` と取り消し、`boosts` と取り消し | 同期 |
| `feed_*` / `tag_usage` | `content` と `graph` の `ReplayEvents` が現在の状態から再生するイベント（下記） | 同期 |
| `feed_author_restrictions` / `feed_domain_restrictions` | `moderation.ReplayEvents` と `account.ReplayDeletions` が再生するイベント（下記） | 同期 |
| `poll_tallies` | `poll_tally_observations`, `poll_tally_observation_counts` | 同期 |
| `notification_exclusions` / `notification_domain_exclusions` | `blocks` と取り消し（`block_edges`）、`mutes`、`account_domain_mutes` | 同期 |
| `notification_deleted_statuses` | `status_deletions` | 同期 |
| `instance_settings` / `instance_texts` / `instance_rule_heads` / `instance_theme` / `announcement_heads` / `service_closure_heads` | `instance_setting_revisions`、`instance_text_revisions`、`instance_rules` と `instance_rule_revisions`・`instance_rule_retirements`、`instance_theme_revisions`、`announcement_revisions`・`announcement_state_changes`、`service_closure_events`・`service_closure_schedules` | 同期 |
| `webhook_endpoint_heads` / `webhook_endpoint_failures` | `webhook_endpoint_revisions`・`webhook_endpoint_state_changes`・`webhook_secrets`、`webhook_delivery_attempts` | 同期 |
| `relay_states` | `relay_events` | 同期 |
| `cost_item_heads` | `cost_items`, `cost_item_label_revisions`, `cost_item_end_changes`, `cost_item_cancellations` | 同期 |
| `supporter_plan_heads` / `supporter_plan_price_heads` / `supporter_subscription_heads` / `supporter_live_subscriptions` / `supporter_subscription_deadlines` | `supporter_plan_revisions`・`supporter_plan_retirements`、`supporter_plan_prices`・`supporter_plan_price_archivals`、`supporter_subscriptions`・`supporter_subscription_transitions`・`supporter_grace_periods` | 同期 |
| `voice_room_heads` / `voice_room_participants` | `voice_rooms`・`voice_room_revisions`・`voice_room_host_changes`・`voice_room_endings`、`voice_room_participations`・`voice_room_participation_ends`・`voice_room_role_changes`・`voice_room_hand_raises`・`voice_room_cohost_changes` | 同期（`voice_room_host_graces` は現在値で、再構築の対象でない） |
| `error_group_states` | `error_groups`・`error_group_events` | 同期（`error_group_counts`・`error_group_samples` は現在値で、再構築の対象でない） |
| カウンタ類 | 元のイベントの件数 | 同期（加算）＋定期検証 |

- CLI: `my projection list` は投影と所有モジュールと表を示します。投影の名前は所有モジュールの再構築関数の単位です
  （`instance`、`account`、`identity`、`moderation`、`graph`、`content`、`status_sensitivity_overrides`、`boost_edges`、`feeds`、`feed_restrictions`、
  `notification_exclusions`、`notification_deleted_statuses`、`relay_states`、`webhook`、`finance`、`supporters`、`voice`、`diagnostics`）。
  `<name>` には投影の名前か、その表の名前を渡します。省略するとすべての投影です。
  - `my projection verify [<name>]` は、1 つのトランザクションの中で現在の表を読み、所有モジュールの再構築関数を実行し、結果の表を読んでロールバックします。
    差分の行（現在の表にあって作り直した結果にない行、その逆）を表示し、差分があれば 1 で終了します。何も変えません。
  - `my projection rebuild [<name>]` は同じ比較のあと、差分だけを 1 トランザクション 500 行までに分けて書きます（ADR-0003 の行数上限）。
    余分な行を消してから欠けた行を入れるので、読み手が空の投影を見ることはなく、正しい行は書き直しません。
    消すのは全列が一致する行だけ、入れるのはキーが重なる行のない行だけなので、比較と書き込みの間に変わった行は新しい値のまま残り、次の検証が残ったずれを見つけます。
  - 定期ジョブ `projection.verify`（1 日ごと）は、すべての投影に `rebuild` と同じことをし、ずれていた投影をログに残します。
  - 表を列に依存せず読み書きするのはアダプタの `projection.Store` です（`testkit.ProjectionStoreContract`）。
  - `remote_account_heads` の取得の状態（`last_fetched_at`、`consecutive_fetch_failures`）はイベントに残らないので、再構築は既存の行の値を残し、
    行のない Actor には最新のスナップショットの取得時刻と 0 を入れます。`snapshot_id` と配送先は最新のスナップショットから作ります。
  - `notification_deleted_statuses` は、通知の対象の投稿のうち `content` の公開 API が削除済みと答えるものから作ります。
  - `account_restrictions` は、解除されていない処分の行のうち、期限の来ていない行と、期限の来た行で表にまだあるものから作ります。
    期限の来た行は何も制限せず、`moderation.expire_restrictions` がそれを消すときに凍結の終わりを伝えます（§4.3）。消したことはイベントに残らないので、
    再構築は表にある期限の来た行を残し、ジョブが消した行を戻しません。`domain_restrictions` は期限の来た行を解除まで持つので、解除されていない処分の行をすべて作ります。
  - `report_states` は、受け付けで `open`、解決（`open` の通報だけ）で `resolved`、再開（`resolved` の通報だけ）で `open` という規則から作ります。
    解決が再開より多ければ `resolved` で、`outcome` と `since` は最新の解決のもの、そうでなければ `open` で、`since` は最新の再開か受け付けの時刻です。担当は最新の割り当てのものです。
    `appeal_states` は、審査があればその結果と審査の時刻、なければ `pending` と申し立ての時刻です。
  - `status_sensitivity_overrides` は、`content` が `moderation` の表を読まないので、`moderation.ReplaySensitivityMarks` が返す、解除されていない `mark_statuses_sensitive` の処分の投稿から作ります。
  - `user_registration_states` は、登録の申請に最新の決定があればその結果と決定の時刻、なければ `pending` と申請の時刻です。
    `user_primary_emails` は利用者の最初のメールアドレスから作ります。アドレスは登録と管理者の作成で 1 つだけ加わるので、それが主アドレスです。
    主アドレスを切り替える操作を足すときは、切り替えをイベントとして記録し、再構築をそれに合わせます。
  - `poll_tallies` は投票ごとに最新の観測（ID の最も大きい `poll_tally_observations`）の票数です。
  - `webhook_endpoint_heads` は宛先ごとに最新（ID の最も大きい）の版・状態の変化・秘密から作ります。
    `webhook_endpoint_failures` は、宛先の最新の成功した試行より後の失敗した試行から、その最初の試行と件数で作ります。
    試行は 14 日で消えるので、最初の失敗が消えた連続の行は作り直せません。再構築はその行を、残っている成功が連続を終えていないかぎり、表にあるまま残します。
  - `relay_states` はリレーごとに最新（ID の最も大きい）の `relay_events` の種類から状態を、その行から `Follow`・主体・時刻を作ります。
  - `cost_item_heads` は取り消しのない費用の項目ごとに、最新（ID の最も大きい）のラベルの版と、最新の終了の変更の `ends_on`（変更がなければ無期限）から作ります。
  - `instance_health` は投影ではなく【現在値】で、再構築・検証の対象ではありません（§4.8）。失敗の日数を 0 に戻す署名付きの受信と probe の成功は、
    `unavailable` からの復帰でないかぎり記録せず（間引き）、`delivery_attempts` は 30 日で消えるので、イベントから作り直せません。
- コントラクトテスト: 各 projector に「イベント列を与えて投影を作る」テストと、「インクリメンタル更新の結果 = 一括再構築の結果」の性質テストを書きます。
- 他のモジュールのイベントから作る投影（`feed_*`）は、元のモジュールのテーブルを読まずに再構築します。
  `content.ReplayEvents` は削除されていない投稿ごとに現在版つきの `content.status_created` を、有効なブーストごとに `content.boosted` を、
  `graph.ReplayEvents` は有効なフォローごとに `graph.followed`（承認待ちなら `graph.follow_requested`）を、有効なブロックごとに `graph.blocked` を、ミュートごとに `graph.muted` を、ドメインのミュートごとに `graph.domain_muted` を、
  再構築のトランザクションの中で発行します。`timeline.RebuildProjections` は `feed_*`（制限の 2 表を除く）と `tag_usage` を空にして、これらに自分の projector を適用します。
- 制限の 2 表は `timeline.RebuildRestrictions` が作ります。`moderation.ReplayEvents` は解除されていない処分のあるアカウントごとに `moderation.account_restrictions_changed` を、
  ドメインごとに `moderation.domain_restrictions_changed` を、その時点で効いている制限を載せて発行し、`account.ReplayDeletions` はアカウントを削除したライフサイクルのイベントごとに
  `account.lifecycle_event_recorded` を発行します。`timeline` の projector は処分と解除のたびにしか行を置き換えないので、期限の来た行が次の処分か解除まで残ります。
  その行は読み取りの条件で何も制限しないので、再構築は表にある期限の来た行を、同じ制限の行を作らなかったときに残します。
