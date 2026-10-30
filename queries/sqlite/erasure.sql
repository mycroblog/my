-- SPDX-License-Identifier: AGPL-3.0-only

-- The erasure of deleted local accounts' data (data-model sections 4.2 and 8, #397). Each module
-- erases its own tables; the statements whose rows grow with the number of users or followers
-- take a bounded batch (@lim). A list argument is a JSON array of strings, read with json_each.

-- account ---------------------------------------------------------------------------------------

-- ListAccountsToErase returns, in account ID order after @after, up to @lim local accounts that
-- were deleted at or before @deleted_before and are not erased yet, with their users.
-- name: ListAccountsToErase :many
SELECT s.account_id, la.user_id FROM account_lifecycle_states s
JOIN local_accounts la ON la.account_id = s.account_id
WHERE s.state = 'deleted' AND s.since <= CAST(sqlc.arg(deleted_before) AS INTEGER)
  AND s.account_id > CAST(sqlc.arg(after) AS TEXT)
  AND NOT EXISTS (SELECT 1 FROM account_erasures e WHERE e.account_id = s.account_id)
ORDER BY s.account_id
LIMIT sqlc.arg(lim);

-- name: EraseProfileFieldSources :exec
DELETE FROM account_profile_field_sources WHERE revision_id IN (
  SELECT r.id FROM account_profile_revisions r WHERE r.account_id = sqlc.arg(account_id));

-- name: EraseProfileFields :exec
DELETE FROM account_profile_fields WHERE revision_id IN (
  SELECT r.id FROM account_profile_revisions r WHERE r.account_id = sqlc.arg(account_id));

-- name: EraseProfileRevisionEmojis :exec
DELETE FROM account_profile_revision_emojis WHERE revision_id IN (
  SELECT r.id FROM account_profile_revisions r WHERE r.account_id = sqlc.arg(account_id));

-- name: EraseProfileRevisionSources :exec
DELETE FROM account_profile_revision_sources WHERE revision_id IN (
  SELECT r.id FROM account_profile_revisions r WHERE r.account_id = sqlc.arg(account_id));

-- name: EraseProfileHead :exec
DELETE FROM account_profile_heads WHERE account_id = sqlc.arg(account_id);

-- name: EraseProfileRevisions :exec
DELETE FROM account_profile_revisions WHERE account_id = sqlc.arg(account_id);

-- name: ErasePrivateKeys :exec
DELETE FROM local_account_private_keys WHERE account_key_id IN (
  SELECT k.id FROM account_keys k WHERE k.account_id = sqlc.arg(account_id));

-- name: EraseKeyRevocations :exec
DELETE FROM account_key_revocations WHERE account_key_id IN (
  SELECT k.id FROM account_keys k WHERE k.account_id = sqlc.arg(account_id));

-- name: EraseKeys :exec
DELETE FROM account_keys WHERE account_id = sqlc.arg(account_id);

-- name: EraseAliases :exec
DELETE FROM account_aliases WHERE account_id = sqlc.arg(account_id);

-- name: InsertAccountErasure :exec
INSERT INTO account_erasures (account_id, erased_at) VALUES (sqlc.arg(account_id), sqlc.arg(erased_at))
ON CONFLICT (account_id) DO NOTHING;

-- name: AccountErased :one
SELECT EXISTS (SELECT 1 FROM account_erasures WHERE account_id = sqlc.arg(account_id));

-- identity: the user's rows, which do not grow with other users ---------------------------------

-- name: EraseEmailSuppressions :exec
DELETE FROM email_suppressions WHERE email_address_id IN (
  SELECT a.id FROM user_email_addresses a WHERE a.user_id = sqlc.arg(user_id));

-- name: EraseEmailSuppressionLifts :exec
DELETE FROM email_suppression_lifts WHERE email_address_id IN (
  SELECT a.id FROM user_email_addresses a WHERE a.user_id = sqlc.arg(user_id));

-- name: EraseEmailDeliveryFailures :exec
DELETE FROM email_delivery_failures WHERE email_address_id IN (
  SELECT a.id FROM user_email_addresses a WHERE a.user_id = sqlc.arg(user_id));

-- name: EraseEmailVerifications :exec
DELETE FROM user_email_verifications WHERE email_address_id IN (
  SELECT a.id FROM user_email_addresses a WHERE a.user_id = sqlc.arg(user_id));

-- name: EraseUserPrimaryEmail :exec
DELETE FROM user_primary_emails WHERE user_id = sqlc.arg(user_id);

-- name: EraseEmailAddresses :exec
DELETE FROM user_email_addresses WHERE user_id = sqlc.arg(user_id);

-- name: ErasePasswordCredentialRemovals :exec
DELETE FROM password_credential_removals WHERE password_credential_id IN (
  SELECT c.id FROM password_credentials c WHERE c.user_id = sqlc.arg(user_id));

-- name: ErasePasswordCredentials :exec
DELETE FROM password_credentials WHERE user_id = sqlc.arg(user_id);

-- name: EraseTOTPRecoveryCodeUses :exec
DELETE FROM totp_recovery_code_uses WHERE enrollment_id IN (
  SELECT e.id FROM totp_enrollments e WHERE e.user_id = sqlc.arg(user_id));

-- name: EraseTOTPRecoveryCodes :exec
DELETE FROM totp_recovery_codes WHERE enrollment_id IN (
  SELECT e.id FROM totp_enrollments e WHERE e.user_id = sqlc.arg(user_id));

-- name: EraseTOTPEnrollmentRevocations :exec
DELETE FROM totp_enrollment_revocations WHERE enrollment_id IN (
  SELECT e.id FROM totp_enrollments e WHERE e.user_id = sqlc.arg(user_id));

-- name: EraseTOTPEnrollments :exec
DELETE FROM totp_enrollments WHERE user_id = sqlc.arg(user_id);

-- name: ErasePasskeyCredentialRevocations :exec
DELETE FROM passkey_credential_revocations WHERE passkey_id IN (
  SELECT c.id FROM passkey_credentials c WHERE c.user_id = sqlc.arg(user_id));

-- name: ErasePasskeyCredentials :exec
DELETE FROM passkey_credentials WHERE user_id = sqlc.arg(user_id);

-- name: EraseUserExternalIdentities :exec
DELETE FROM user_external_identities WHERE user_id = sqlc.arg(user_id);

-- name: EraseExternalIdentityUnlinks :exec
DELETE FROM external_identity_unlinks WHERE link_id IN (
  SELECT l.id FROM external_identity_links l WHERE l.user_id = sqlc.arg(user_id));

-- EraseExternalIdentities deletes the external identities that only the user linked.
-- name: EraseExternalIdentities :exec
DELETE FROM external_identities WHERE id IN (
  SELECT l.external_identity_id FROM external_identity_links l WHERE l.user_id = sqlc.arg(user_id))
  AND NOT EXISTS (
    SELECT 1 FROM external_identity_links o
    WHERE o.external_identity_id = external_identities.id AND o.user_id <> sqlc.arg(user_id));

-- name: EraseExternalIdentityLinks :exec
DELETE FROM external_identity_links WHERE user_id = sqlc.arg(user_id);

-- name: EraseUserTokenRedemptions :exec
DELETE FROM user_token_redemptions WHERE token_id IN (
  SELECT t.id FROM user_tokens t WHERE t.user_id = sqlc.arg(user_id));

-- name: EraseUserTokens :exec
DELETE FROM user_tokens WHERE user_id = sqlc.arg(user_id);

-- name: EraseSessionRevocations :exec
DELETE FROM session_revocations WHERE session_id IN (
  SELECT s.id FROM sessions s WHERE s.user_id = sqlc.arg(user_id));

-- name: EraseSessionActivity :exec
DELETE FROM session_activity WHERE session_id IN (
  SELECT s.id FROM sessions s WHERE s.user_id = sqlc.arg(user_id));

-- name: EraseSessions :exec
DELETE FROM sessions WHERE user_id = sqlc.arg(user_id);

-- name: EraseAuthorizationCodeRedemptions :exec
DELETE FROM oauth_authorization_code_redemptions WHERE code_hash IN (
  SELECT c.code_hash FROM oauth_authorization_codes c WHERE c.user_id = sqlc.arg(user_id));

-- name: EraseAuthorizationCodes :exec
DELETE FROM oauth_authorization_codes WHERE user_id = sqlc.arg(user_id);

-- name: EraseAccessTokenRevocations :exec
DELETE FROM oauth_token_revocations WHERE token_id IN (
  SELECT t.id FROM oauth_access_tokens t WHERE t.user_id = sqlc.arg(user_id));

-- name: EraseAccessTokens :exec
DELETE FROM oauth_access_tokens WHERE user_id = sqlc.arg(user_id);

-- name: EraseUserPreferences :exec
DELETE FROM user_preferences WHERE user_id = sqlc.arg(user_id);

-- name: EraseUserRoleBadgeDisplays :exec
DELETE FROM user_role_badge_displays WHERE user_id = sqlc.arg(user_id);

-- name: EraseUserTimelineLanguages :exec
DELETE FROM user_timeline_languages WHERE user_id = sqlc.arg(user_id);

-- name: EraseUserTimeZone :exec
DELETE FROM user_time_zones WHERE user_id = sqlc.arg(user_id);

-- name: EraseRegistrationDecisions :exec
DELETE FROM registration_decisions WHERE application_id IN (
  SELECT a.id FROM registration_applications a WHERE a.user_id = sqlc.arg(user_id));

-- name: EraseRegistrationApplications :exec
DELETE FROM registration_applications WHERE user_id = sqlc.arg(user_id);

-- name: EraseRegistrationState :exec
DELETE FROM user_registration_states WHERE user_id = sqlc.arg(user_id);

-- name: EraseServiceClosureEmails :exec
DELETE FROM service_closure_emails WHERE user_id = sqlc.arg(user_id);

-- content: the account's statuses and its interactions --------------------------------------------

-- ListAuthorStatusesToErase returns, in ID order after @after, up to @lim statuses of the author
-- with whether each is deleted already.
-- name: ListAuthorStatusesToErase :many
SELECT s.id, s.uri, s.url, s.author_account_id, s.is_local, s.visibility, s.in_reply_to_uri,
  s.conversation_uri, s.published_at,
  CAST(EXISTS (SELECT 1 FROM status_deletions d WHERE d.status_id = s.id) AS INTEGER) AS deleted
FROM statuses s
WHERE s.author_account_id = sqlc.arg(author_account_id) AND s.id > CAST(sqlc.arg(after) AS TEXT)
ORDER BY s.id
LIMIT sqlc.arg(lim);

-- The EraseOfStatuses* statements delete what refers to the statuses @ids beyond what
-- PurgeStatuses deletes: their local rows, every boost of them, and the favourites, reactions
-- (with their counts), bookmarks, and pins of them.

-- name: EraseLocalStatusesOfStatuses :exec
DELETE FROM local_statuses WHERE status_id IN (SELECT value FROM json_each(CAST(sqlc.arg(ids) AS TEXT)));

-- name: EraseBoostEdgesOfStatuses :exec
DELETE FROM boost_edges WHERE status_id IN (SELECT value FROM json_each(CAST(sqlc.arg(ids) AS TEXT)));

-- name: EraseBoostRetractionsOfStatuses :exec
DELETE FROM boost_retractions WHERE boost_id IN (
  SELECT b.id FROM boosts b WHERE b.status_id IN (SELECT value FROM json_each(CAST(sqlc.arg(ids) AS TEXT))));

-- name: EraseBoostsOfStatuses :exec
DELETE FROM boosts WHERE status_id IN (SELECT value FROM json_each(CAST(sqlc.arg(ids) AS TEXT)));

-- name: EraseFavouritesOfStatuses :exec
DELETE FROM favourites WHERE status_id IN (SELECT value FROM json_each(CAST(sqlc.arg(ids) AS TEXT)));

-- name: EraseReactionsOfStatuses :exec
DELETE FROM reactions WHERE status_id IN (SELECT value FROM json_each(CAST(sqlc.arg(ids) AS TEXT)));

-- name: EraseReactionCountsOfStatuses :exec
DELETE FROM status_reaction_counts WHERE status_id IN (SELECT value FROM json_each(CAST(sqlc.arg(ids) AS TEXT)));

-- name: EraseBookmarksOfStatuses :exec
DELETE FROM bookmarks WHERE status_id IN (SELECT value FROM json_each(CAST(sqlc.arg(ids) AS TEXT)));

-- name: ErasePinsOfStatuses :exec
DELETE FROM status_pins WHERE status_id IN (SELECT value FROM json_each(CAST(sqlc.arg(ids) AS TEXT)));

-- name: EraseCountersOfStatuses :exec
DELETE FROM status_counters WHERE status_id IN (SELECT value FROM json_each(CAST(sqlc.arg(ids) AS TEXT)));

-- The account's favourites: the counts of the statuses go down first, for the same batch.

-- name: UncountFavouritesBy :exec
UPDATE status_counters SET favourites_count = favourites_count - 1
WHERE favourites_count > 0 AND status_id IN (
  SELECT f.status_id FROM favourites f WHERE f.account_id = sqlc.arg(account_id)
  ORDER BY f.status_id LIMIT sqlc.arg(lim));

-- name: EraseFavouritesBy :execrows
DELETE FROM favourites WHERE favourites.account_id = sqlc.arg(account_id) AND favourites.status_id IN (
  SELECT f.status_id FROM favourites f WHERE f.account_id = sqlc.arg(account_id)
  ORDER BY f.status_id LIMIT sqlc.arg(lim));

-- The account's reactions, the oldest lim of them: the counts of their emoji go down first (a
-- count of one goes away), for the same batch.

-- name: DeleteSingleReactionCountsBy :exec
DELETE FROM status_reaction_counts WHERE count = 1 AND EXISTS (
  SELECT 1 FROM (SELECT r.status_id, r.emoji_key FROM reactions r WHERE r.account_id = sqlc.arg(account_id)
    ORDER BY r.id LIMIT sqlc.arg(lim)) x
  WHERE x.status_id = status_reaction_counts.status_id AND x.emoji_key = status_reaction_counts.emoji_key);

-- name: UncountReactionsBy :exec
UPDATE status_reaction_counts SET count = count - 1 WHERE EXISTS (
  SELECT 1 FROM (SELECT r.status_id, r.emoji_key FROM reactions r WHERE r.account_id = sqlc.arg(account_id)
    ORDER BY r.id LIMIT sqlc.arg(lim)) x
  WHERE x.status_id = status_reaction_counts.status_id AND x.emoji_key = status_reaction_counts.emoji_key);

-- name: EraseReactionsBy :execrows
DELETE FROM reactions WHERE reactions.id IN (
  SELECT r.id FROM reactions r WHERE r.account_id = sqlc.arg(account_id) ORDER BY r.id LIMIT sqlc.arg(lim));

-- name: EraseBookmarksBy :execrows
DELETE FROM bookmarks WHERE bookmarks.account_id = sqlc.arg(account_id) AND bookmarks.status_id IN (
  SELECT b.status_id FROM bookmarks b WHERE b.account_id = sqlc.arg(account_id)
  ORDER BY b.status_id LIMIT sqlc.arg(lim));

-- The account's votes in polls, the oldest lim of them.

-- name: ErasePollVotesBy :execrows
DELETE FROM poll_votes WHERE poll_votes.id IN (
  SELECT v.id FROM poll_votes v WHERE v.account_id = sqlc.arg(account_id) ORDER BY v.id LIMIT sqlc.arg(lim));

-- name: ErasePinsBy :execrows
DELETE FROM status_pins WHERE account_id = sqlc.arg(account_id);

-- The account's automatic deletion policy (FR-118) goes with its kept visibilities.

-- name: EraseAutoDeleteKeptVisibilitiesBy :exec
DELETE FROM status_auto_delete_kept_visibilities WHERE account_id = sqlc.arg(account_id);

-- name: EraseAutoDeletePolicyBy :exec
DELETE FROM status_auto_delete_policies WHERE account_id = sqlc.arg(account_id);

-- The account's boosts in force: the counts go down with their edges, then every boost of the
-- account goes with its retraction.

-- name: UncountBoostsBy :exec
UPDATE status_counters SET boosts_count = boosts_count - 1
WHERE boosts_count > 0 AND status_id IN (
  SELECT e.status_id FROM boost_edges e WHERE e.booster_account_id = sqlc.arg(account_id)
  ORDER BY e.status_id LIMIT sqlc.arg(lim));

-- name: EraseBoostEdgesBy :execrows
DELETE FROM boost_edges WHERE boost_edges.booster_account_id = sqlc.arg(account_id) AND boost_edges.status_id IN (
  SELECT e.status_id FROM boost_edges e WHERE e.booster_account_id = sqlc.arg(account_id)
  ORDER BY e.status_id LIMIT sqlc.arg(lim));

-- name: EraseBoostRetractionsBy :exec
DELETE FROM boost_retractions WHERE boost_id IN (
  SELECT b.id FROM boosts b WHERE b.booster_account_id = sqlc.arg(account_id)
  ORDER BY b.id LIMIT sqlc.arg(lim));

-- name: EraseBoostsBy :execrows
DELETE FROM boosts WHERE id IN (
  SELECT b.id FROM boosts b WHERE b.booster_account_id = sqlc.arg(account_id)
  ORDER BY b.id LIMIT sqlc.arg(lim));

-- graph -------------------------------------------------------------------------------------------

-- ListFollowEdgesToErase returns up to @lim live follows into or out of the account.
-- name: ListFollowEdgesToErase :many
SELECT * FROM follow_edges
WHERE follower_account_id = sqlc.arg(account_id) OR followee_account_id = sqlc.arg(account_id)
ORDER BY follow_id
LIMIT sqlc.arg(lim);

-- name: ListFollowIDsToErase :many
SELECT id FROM follows
WHERE follower_account_id = sqlc.arg(account_id) OR followee_account_id = sqlc.arg(account_id)
ORDER BY id
LIMIT sqlc.arg(lim);

-- name: EraseFollowAcceptances :exec
DELETE FROM follow_acceptances WHERE follow_id IN (SELECT value FROM json_each(CAST(sqlc.arg(ids) AS TEXT)));

-- name: EraseFollowRejections :exec
DELETE FROM follow_rejections WHERE follow_id IN (SELECT value FROM json_each(CAST(sqlc.arg(ids) AS TEXT)));

-- name: EraseFollowTerminations :exec
DELETE FROM follow_terminations WHERE follow_id IN (SELECT value FROM json_each(CAST(sqlc.arg(ids) AS TEXT)));

-- name: EraseFollowResends :exec
DELETE FROM follow_resends WHERE follow_id IN (SELECT value FROM json_each(CAST(sqlc.arg(ids) AS TEXT)));

-- name: EraseFollowPreferences :exec
DELETE FROM follow_preferences WHERE follow_id IN (SELECT value FROM json_each(CAST(sqlc.arg(ids) AS TEXT)));

-- name: EraseFollowEdgesOfFollows :exec
DELETE FROM follow_edges WHERE follow_id IN (SELECT value FROM json_each(CAST(sqlc.arg(ids) AS TEXT)));

-- name: EraseFollows :exec
DELETE FROM follows WHERE id IN (SELECT value FROM json_each(CAST(sqlc.arg(ids) AS TEXT)));

-- name: ListBlockIDsToErase :many
SELECT id FROM blocks
WHERE blocker_account_id = sqlc.arg(account_id) OR blocked_account_id = sqlc.arg(account_id)
ORDER BY id
LIMIT sqlc.arg(lim);

-- name: EraseBlockRetractions :exec
DELETE FROM block_retractions WHERE block_id IN (SELECT value FROM json_each(CAST(sqlc.arg(ids) AS TEXT)));

-- name: EraseBlockEdgesOfBlocks :exec
DELETE FROM block_edges WHERE block_id IN (SELECT value FROM json_each(CAST(sqlc.arg(ids) AS TEXT)));

-- name: EraseBlocks :exec
DELETE FROM blocks WHERE id IN (SELECT value FROM json_each(CAST(sqlc.arg(ids) AS TEXT)));

-- name: EraseMutesBy :execrows
DELETE FROM mutes WHERE mutes.account_id = sqlc.arg(account_id) AND mutes.target_account_id IN (
  SELECT m.target_account_id FROM mutes m WHERE m.account_id = sqlc.arg(account_id)
  ORDER BY m.target_account_id LIMIT sqlc.arg(lim));

-- name: EraseMutesOf :execrows
DELETE FROM mutes WHERE mutes.target_account_id = sqlc.arg(account_id) AND mutes.account_id IN (
  SELECT m.account_id FROM mutes m WHERE m.target_account_id = sqlc.arg(account_id)
  ORDER BY m.account_id LIMIT sqlc.arg(lim));

-- name: EraseDomainMutesBy :execrows
DELETE FROM account_domain_mutes WHERE account_domain_mutes.account_id = sqlc.arg(account_id) AND account_domain_mutes.domain IN (
  SELECT m.domain FROM account_domain_mutes m WHERE m.account_id = sqlc.arg(account_id)
  ORDER BY m.domain LIMIT sqlc.arg(lim));

-- name: EraseFollowCounters :exec
DELETE FROM follow_counters WHERE account_id = sqlc.arg(account_id);

-- timeline ----------------------------------------------------------------------------------------

-- name: ListFeedEntryIDsToErase :many
SELECT id FROM feed_entries
WHERE author_account_id = sqlc.arg(account_id) OR original_author_account_id = sqlc.arg(account_id)
ORDER BY id
LIMIT sqlc.arg(lim);

-- name: EraseFeedDirectRecipientsOfEntries :exec
DELETE FROM feed_direct_recipients WHERE entry_id IN (SELECT value FROM json_each(CAST(sqlc.arg(ids) AS TEXT)));

-- CountUsedFeedTagsOfEntries returns how many of the entries among ids that tag_usage counts
-- carry each tag.
-- name: CountUsedFeedTagsOfEntries :many
SELECT tag_id, count(*) AS n FROM feed_tag_entries
WHERE entry_id IN (SELECT value FROM json_each(CAST(sqlc.arg(ids) AS TEXT))) AND visibility IN ('public', 'local', 'unlisted')
GROUP BY tag_id ORDER BY tag_id;

-- name: EraseFeedTagEntriesOfEntries :exec
DELETE FROM feed_tag_entries WHERE entry_id IN (SELECT value FROM json_each(CAST(sqlc.arg(ids) AS TEXT)));

-- name: EraseFeedEntries :exec
DELETE FROM feed_entries WHERE id IN (SELECT value FROM json_each(CAST(sqlc.arg(ids) AS TEXT)));

-- name: EraseFeedDirectRecipientsOf :execrows
DELETE FROM feed_direct_recipients WHERE feed_direct_recipients.recipient_account_id = sqlc.arg(account_id) AND feed_direct_recipients.entry_id IN (
  SELECT r.entry_id FROM feed_direct_recipients r WHERE r.recipient_account_id = sqlc.arg(account_id)
  ORDER BY r.entry_id LIMIT sqlc.arg(lim));

-- name: EraseFeedFollowEdgesAsFollower :execrows
DELETE FROM feed_follow_edges WHERE feed_follow_edges.follower_account_id = sqlc.arg(account_id) AND feed_follow_edges.followee_account_id IN (
  SELECT e.followee_account_id FROM feed_follow_edges e WHERE e.follower_account_id = sqlc.arg(account_id)
  ORDER BY e.followee_account_id LIMIT sqlc.arg(lim));

-- name: EraseFeedFollowEdgesAsFollowee :execrows
DELETE FROM feed_follow_edges WHERE feed_follow_edges.followee_account_id = sqlc.arg(account_id) AND feed_follow_edges.follower_account_id IN (
  SELECT e.follower_account_id FROM feed_follow_edges e WHERE e.followee_account_id = sqlc.arg(account_id)
  ORDER BY e.follower_account_id LIMIT sqlc.arg(lim));

-- name: EraseFeedExclusionsAsViewer :execrows
DELETE FROM feed_exclusions WHERE feed_exclusions.viewer_account_id = sqlc.arg(account_id) AND feed_exclusions.target_account_id IN (
  SELECT x.target_account_id FROM feed_exclusions x WHERE x.viewer_account_id = sqlc.arg(account_id)
  ORDER BY x.target_account_id LIMIT sqlc.arg(lim));

-- name: EraseFeedExclusionsAsTarget :execrows
DELETE FROM feed_exclusions WHERE feed_exclusions.target_account_id = sqlc.arg(account_id) AND feed_exclusions.viewer_account_id IN (
  SELECT x.viewer_account_id FROM feed_exclusions x WHERE x.target_account_id = sqlc.arg(account_id)
  ORDER BY x.viewer_account_id LIMIT sqlc.arg(lim));

-- name: EraseFeedDomainMutesOf :execrows
DELETE FROM feed_domain_mutes WHERE feed_domain_mutes.viewer_account_id = sqlc.arg(account_id) AND feed_domain_mutes.domain IN (
  SELECT m.domain FROM feed_domain_mutes m WHERE m.viewer_account_id = sqlc.arg(account_id)
  ORDER BY m.domain LIMIT sqlc.arg(lim));

-- The lists of an account and its membership in the lists of others (data-model 4.6). A list's
-- name goes before the list, and the steps pick the same first lists by ID.

-- name: EraseListMembersAsOwner :execrows
DELETE FROM list_members WHERE list_members.owner_account_id = sqlc.arg(account_id) AND list_members.list_id IN (
  SELECT m.list_id FROM list_members m WHERE m.owner_account_id = sqlc.arg(account_id)
  ORDER BY m.list_id LIMIT sqlc.arg(lim));

-- name: EraseListMembersAsMember :execrows
DELETE FROM list_members WHERE list_members.member_account_id = sqlc.arg(account_id) AND list_members.list_id IN (
  SELECT m.list_id FROM list_members m WHERE m.member_account_id = sqlc.arg(account_id)
  ORDER BY m.list_id LIMIT sqlc.arg(lim));

-- name: EraseListNamesOf :execrows
DELETE FROM list_names WHERE list_names.list_id IN (
  SELECT l.id FROM lists l WHERE l.owner_account_id = sqlc.arg(account_id)
  ORDER BY l.id LIMIT sqlc.arg(lim));

-- name: EraseListsOf :execrows
DELETE FROM lists WHERE lists.owner_account_id = sqlc.arg(account_id) AND lists.id IN (
  SELECT l.id FROM lists l WHERE l.owner_account_id = sqlc.arg(account_id)
  ORDER BY l.id LIMIT sqlc.arg(lim));

-- notification ------------------------------------------------------------------------------------

-- name: EraseNotificationDismissalsOfStatuses :exec
DELETE FROM notification_dismissals WHERE notification_id IN (
  SELECT n.id FROM notifications n WHERE n.status_id IN (SELECT value FROM json_each(CAST(sqlc.arg(ids) AS TEXT))));

-- name: EraseNotificationsOfStatuses :exec
DELETE FROM notifications WHERE status_id IN (SELECT value FROM json_each(CAST(sqlc.arg(ids) AS TEXT)));

-- name: EraseDeletedStatusesOfStatuses :exec
DELETE FROM notification_deleted_statuses WHERE status_id IN (SELECT value FROM json_each(CAST(sqlc.arg(ids) AS TEXT)));

-- ListNotificationIDsToErase returns up to @lim notifications to, from, or about the account and
-- its user.
-- name: ListNotificationIDsToErase :many
SELECT id FROM notifications
WHERE recipient_account_id = sqlc.arg(account_id) OR source_account_id = sqlc.arg(account_id)
  OR subject_user_id = sqlc.arg(user_id)
ORDER BY id
LIMIT sqlc.arg(lim);

-- name: EraseNotificationDismissals :exec
DELETE FROM notification_dismissals WHERE notification_id IN (SELECT value FROM json_each(CAST(sqlc.arg(ids) AS TEXT)));

-- name: EraseNotifications :exec
DELETE FROM notifications WHERE id IN (SELECT value FROM json_each(CAST(sqlc.arg(ids) AS TEXT)));

-- name: EraseNotificationReadMarker :exec
DELETE FROM notification_read_markers WHERE account_id = sqlc.arg(account_id);

-- name: EraseNotificationSettingsOf :exec
DELETE FROM notification_settings WHERE account_id = sqlc.arg(account_id);

-- name: EraseNotificationExclusionsAsViewer :execrows
DELETE FROM notification_exclusions WHERE notification_exclusions.viewer_account_id = sqlc.arg(account_id) AND notification_exclusions.target_account_id IN (
  SELECT x.target_account_id FROM notification_exclusions x WHERE x.viewer_account_id = sqlc.arg(account_id)
  ORDER BY x.target_account_id LIMIT sqlc.arg(lim));

-- name: EraseNotificationExclusionsAsTarget :execrows
DELETE FROM notification_exclusions WHERE notification_exclusions.target_account_id = sqlc.arg(account_id) AND notification_exclusions.viewer_account_id IN (
  SELECT x.viewer_account_id FROM notification_exclusions x WHERE x.target_account_id = sqlc.arg(account_id)
  ORDER BY x.viewer_account_id LIMIT sqlc.arg(lim));

-- name: EraseNotificationDomainExclusionsOf :execrows
DELETE FROM notification_domain_exclusions WHERE notification_domain_exclusions.viewer_account_id = sqlc.arg(account_id) AND notification_domain_exclusions.domain IN (
  SELECT x.domain FROM notification_domain_exclusions x WHERE x.viewer_account_id = sqlc.arg(account_id)
  ORDER BY x.domain LIMIT sqlc.arg(lim));

-- name: ErasePostSubscriptionsOf :exec
DELETE FROM post_subscriptions WHERE account_id = sqlc.arg(account_id);

-- name: ErasePostSubscriptionRequestsOf :exec
DELETE FROM post_subscription_requests WHERE account_id = sqlc.arg(account_id);

-- name: ErasePostSubscriptionsAsTarget :execrows
DELETE FROM post_subscriptions WHERE post_subscriptions.target_account_id = sqlc.arg(account_id) AND post_subscriptions.account_id IN (
  SELECT x.account_id FROM post_subscriptions x WHERE x.target_account_id = sqlc.arg(account_id)
  ORDER BY x.account_id LIMIT sqlc.arg(lim));

-- name: ErasePostSubscriptionRequestsAsTarget :execrows
DELETE FROM post_subscription_requests WHERE post_subscription_requests.target_account_id = sqlc.arg(account_id) AND post_subscription_requests.account_id IN (
  SELECT x.account_id FROM post_subscription_requests x WHERE x.target_account_id = sqlc.arg(account_id)
  ORDER BY x.account_id LIMIT sqlc.arg(lim));
