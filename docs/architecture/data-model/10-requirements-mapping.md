## 10. 機能要件とテーブルの対応

| FR | 優先度 | 主なテーブル |
| --- | --- | --- |
| FR-001 登録モード | M | `instance_settings`（`registration_mode`）, `registration_applications`, `registration_decisions`, `invites`, `invite_redemptions` |
| FR-002 メール + パスワード | M | `user_email_addresses`, `user_email_verifications`, `user_primary_emails`, `password_credentials`, `user_tokens` |
| FR-003 OAuth 2.0 + PKCE | M | `oauth_clients`, `oauth_client_disablements`, `oauth_client_disablement_lifts`, `oauth_client_deletions`, `oauth_client_redirect_uris`, `oauth_client_metadata_documents`, `oauth_client_host_denials`, `oauth_client_host_denial_lifts`, `oauth_authorization_codes`, `oauth_access_tokens`, `oauth_token_revocations`, `oauth_token_family_last_used`, `sessions` |
| FR-004 パスキー | C | `passkey_credentials`, `passkey_credential_revocations`, `password_credential_removals` |
| FR-005 TOTP | S | `totp_enrollments`, `totp_recovery_codes`, `totp_recovery_code_uses` |
| FR-006 プロフィール | M | `account_profile_revisions`, `account_profile_fields`, `account_profile_heads` |
| FR-007 言語・タイムゾーン | M | `user_preferences`, `user_time_zones` |
| FR-008 アカウント削除・エクスポート・インポート | S | `account_lifecycle_events`, `data_exports`, `data_export_results`, `data_imports`, `data_import_items`, `data_import_item_results` |
| FR-101〜104 投稿・公開範囲・メンション・タグ・CW | M | `statuses`, `status_revisions` と子テーブル, `status_mention_index`, `feed_tag_entries` |
| FR-105 画像添付・代替テキスト | M | `media_files`, `uploaded_media`, `media_processing_results`, `media_variants`, `media_focus`, `media_descriptions`, `status_revision_attachments` |
| FR-106 返信・スレッド | M | `statuses.in_reply_to_uri` / `conversation_uri`, `status_reply_links` |
| FR-107 ブースト・お気に入り | M | `boosts`, `boost_retractions`, `boost_edges`, `favourites`, `status_counters` |
| FR-108 削除 | M | `status_deletions`, `deliveries` |
| FR-109 編集と履歴 | S | `status_revisions`, `status_heads` |
| FR-110 言語タグ | M | `status_revisions.language`, `status_revision_bodies` |
| FR-111 カスタム絵文字 | S | `custom_emojis`, `custom_emoji_images`, `custom_emoji_settings`, `custom_emoji_aliases`, `status_revision_emojis`, `account_profile_revision_emojis`, `audit_log_entries` |
| FR-112 投票 | C | `polls`, `poll_options`, `poll_votes`, `poll_tally_observations`, `poll_tallies` |
| FR-113 ブックマーク | S | `bookmarks` |
| FR-114 引用投稿 | S | `status_quotes`, `status_quote_events`, `status_quote_states`, `status_quote_policies`, `quote_authorizations`, `quote_authorization_revocations`, `user_preferences`（`default_quote_policy`）, `notifications`（`quote`） |
| FR-116 絵文字リアクション | M | `reactions`, `status_reaction_counts`, `custom_emojis`, `notifications`（`reaction`） |
| FR-117 リンクのカード | S | `link_cards`, `link_card_media`, `status_link_cards`, `instance_settings`（`link_cards_enabled`） |
| FR-201〜204 タイムライン | M | `feed_entries`, `feed_direct_recipients`, `feed_tag_entries`, `feed_follow_edges`, `feed_exclusions`, `feed_author_restrictions`, `feed_domain_restrictions` |
| FR-205 通知 | M | `notifications`, `notification_dismissals`, `notification_read_markers` |
| FR-206 Web Push | S | `push_subscriptions`, `push_subscription_alerts` |
| FR-208 言語フィルタ | S | `user_timeline_languages`, `feed_entries.language` |
| FR-301〜302 フォロー・リクエスト | M | `follows`, `follow_acceptances`, `follow_rejections`, `follow_terminations`, `follow_edges` |
| FR-303 ブロック・ミュート | M | `blocks`, `block_retractions`, `block_edges`, `mutes` |
| FR-304 ドメインミュート | C | `account_domain_mutes`, `feed_domain_mutes`, `notification_domain_exclusions` |
| FR-305 リモートアカウント検索 | M | `account_handles`, `accounts.uri` |
| FR-306 アカウントとハッシュタグの前方一致の検索（FR-119 の `@` と `#` の補完） | S | `account_search_terms`, `tags`, `tag_usage` |
| FR-401〜404 WebFinger・NodeInfo・Actor・署名 | M | `account_handles`, `account_keys`, `local_account_private_keys`, `follow_edges`, `feed_entries` |
| FR-405 受信アクティビティ | M | `received_activities`, `received_activity_documents`, `remote_object_tombstones` |
| FR-406 配送・リトライ・一時停止 | M | `outbound_activities`, `outbound_activity_documents`, `deliveries`, `delivery_attempts`, `instance_availability_events`, `instance_health`, `jobs` |
| FR-407 リモート取得 | M | `remote_account_snapshots`, `remote_account_heads`, `remote_account_handle_verifications`, `remote_profile_revisions`, `remote_status_revisions`, `account_keys`, `kv_entries` |
| FR-409 リモートメディア方針 | M | `remote_media`, `remote_media_cache`, `instance_settings` |
| FR-411 リレーへの送信と受信 | S | `relay_states`, `deliveries`, `relayed_statuses`, `kv_entries`（時間あたりの件数） |
| FR-501 クライアント API | M | `oauth_*`, `local_statuses.idempotency_key` |
| FR-602 管理画面 | M | `user_roles`, `role_permissions`、モデレーション系 |
| FR-701 通報 | M | `reports`, `report_statuses`, `report_rule_citations`, `report_states` |
| FR-709〜711 通報の転送・対応・結果の通知 | S / M | `reports.forwarded`, `report_assignments`, `report_resolutions`, `report_reopenings`, `report_notes`, `notifications`（`report_resolved`） |
| FR-712 モデレーターへの通知 | M | `notifications`（`admin_report`, `admin_sign_up`, `admin_appeal`, `admin_webhook_disabled`） |
| FR-714 アカウントの管理 | M | `session_origins`, `registration_origins`, `user_primary_emails`, `moderation_actions`, `reports` |
| FR-716〜717 連合への伝播・切れた関係 | M / S | `outbox_events`, `follow_terminations`, `relationship_severances` |
| FR-718 処分したドメインの公開 | S | `domain_moderation_actions`（`public_comment`, `obfuscate`）, `instance_settings`（`domain_actions_visibility`） |
| FR-719 登録の拒否リスト | S | `signup_blocks`, `signup_block_lifts`, `registration_origins` |
| FR-720 ロール | S | `user_role_grants`, `user_role_revocations`, `user_roles` |
| FR-722 サーバー単位の webhook | S | §4.11 の `webhook_*`, `audit_log_entries`, `notifications`（`admin_webhook_disabled`） |
| FR-724 リレーの参加と脱退の管理 | S | `relays`, `relay_events`, `relay_states`, `outbound_activities`（`relay`）, `audit_log_entries` |
| FR-901 費用の記録 | S | §4.12 の `cost_items`, `cost_item_label_revisions`, `cost_item_price_revisions`, `cost_item_end_changes`, `cost_item_cancellations`, `cost_item_heads` |
| FR-902 期間の集計 | S | `cost_item_heads` と §4.12 の項目・改定（読み取りのときに計算し、投影を持たない） |
| FR-903 費用の公開範囲 | S | `instance_settings`（`finance_visibility`）と FR-902 の集計（読み取りのときに計算する） |
| FR-904 利用料金の自動取り込み | S | §4.12 の `billing_import_runs`, `imported_cost_revisions`, `imported_cost_heads`, `audit_log_entries`（手動の取り込み） |
| FR-910・FR-911 サポーターの決済と状態 | S | §4.13 の `payment_webhook_receipts`, `payment_reconcile_runs`, `supporter_*` の表, `notifications`（`supporter_*` の種類） |
| FR-912 課金のロールの安全柵 | S | `managed_roles`, `role_permissions`, `roles` |
| FR-913 ロールのバッジ | S | `role_badges`, `user_role_badge_displays`, `user_roles` |
| FR-914 参加者の一覧 | S | `supporter_listings`, `supporter_subscription_heads`, `supporter_plan_heads`, `instance_settings`（`supporter_list_visibility`） |
| FR-915 絵文字の作成の権限と承認制 | S | `role_permissions`（`create_custom_emojis`）, `custom_emojis`（`created_by_user_id`）, `custom_emoji_requests`, `custom_emoji_request_outcomes`, `custom_emoji_pending_requests`, `audit_log_entries`, `notifications`（`admin_custom_emoji_request`, `custom_emoji_request_decided`） |
| FR-916 会計の閲覧の権限 | S | `role_permissions`（`view_finance`）, `instance_settings`（`finance_viewer_item_names`, `finance_viewer_item_memos`, `finance_viewer_income`）と FR-902 の集計（読み取りのときに計算する） |
| FR-917 収入の記録と公開 | S | §4.12 の `income_movements` と、`instance_settings`（`income_visibility`）。集計は読み取りのときに計算する |
| FR-702 ドメインブロック | M | `domain_moderation_actions`, `domain_restrictions` |
| FR-703 処分（サイレンス・投稿禁止・ログイン停止・凍結・BAN ほか） | M | `moderation_actions`, `moderation_action_statuses`, `moderation_action_lifts`, `account_restrictions`, `status_sensitivity_overrides`, `status_deletions`, `account_lifecycle_events` |
| FR-707 異議申し立て | S | `moderation_appeals`, `moderation_appeal_decisions`, `appeal_states` |
| FR-708 交流の禁止 | S | `moderation_actions`（`restrict_interaction`）, `account_restrictions`（`interact`） |
| FR-704 監査ログ | M | `audit_log_entries` |
| FR-705 インスタンス設定 | M | `instance_setting_revisions`, `instance_text_revisions`, `instance_rules` とその版 |
| FR-801 CLI | M | `dead_jobs`, §9 の再構築、管理者作成（`users`, `user_primary_emails`, `user_role_grants`） |
| FR-803 定期クリーンアップ | M | `received_activities`, `received_activity_documents`, `remote_object_tombstones`, `remote_follow_tombstones`, `outbound_activity_documents`, 保持期間を過ぎたリモートの投稿, `delivery_attempts`, `remote_media_cache`, 参照されない `link_cards` と `link_card_media`, 未添付の `media_files`, `kv_entries`, `webhook_messages` とその配送・試行, 期限切れの制限 |
