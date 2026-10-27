// SPDX-License-Identifier: AGPL-3.0-only

package sqlite

import (
	"context"
	"time"

	"github.com/mycroblog/my/internal/adapter/db/sqlite/sqlitegen"
	appaccount "github.com/mycroblog/my/internal/app/account"
	appcontent "github.com/mycroblog/my/internal/app/content"
	appgraph "github.com/mycroblog/my/internal/app/graph"
	appidentity "github.com/mycroblog/my/internal/app/identity"
	appnotification "github.com/mycroblog/my/internal/app/notification"
	apptimeline "github.com/mycroblog/my/internal/app/timeline"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/graph"
)

// The erasure of deleted local accounts' data (data-model §8): each module's repository
// implements its part.
var (
	_ appaccount.ErasureRepository      = (*AccountRepository)(nil)
	_ appidentity.ErasureRepository     = (*IdentityRepository)(nil)
	_ appcontent.ErasureRepository      = (*StatusRepository)(nil)
	_ appgraph.ErasureRepository        = (*GraphRepository)(nil)
	_ apptimeline.ErasureRepository     = (*TimelineRepository)(nil)
	_ appnotification.ErasureRepository = (*NotificationRepository)(nil)
)

// ListAccountsToErase implements account.ErasureRepository.
func (r *AccountRepository) ListAccountsToErase(ctx context.Context, deletedBefore time.Time, after string, limit int) ([]appaccount.ErasureTarget, error) {
	rows, err := r.db.reader().ListAccountsToErase(ctx, sqlitegen.ListAccountsToEraseParams{
		DeletedBefore: toMillis(deletedBefore), After: after, Lim: int64(limit),
	})
	if err != nil {
		return nil, wrap(err, "list accounts to erase")
	}
	out := make([]appaccount.ErasureTarget, len(rows))
	for i, row := range rows {
		out[i] = appaccount.ErasureTarget{AccountID: row.AccountID, UserID: row.UserID}
	}
	return out, nil
}

// EraseProfilesAndKeys implements account.ErasureRepository.
func (r *AccountRepository) EraseProfilesAndKeys(ctx context.Context, tx uow.Tx, accountID string) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return runSteps(ctx, accountID, "erase profiles and keys",
		q.EraseProfileFieldSources, q.EraseProfileFields, q.EraseProfileRevisionEmojis, q.EraseProfileRevisionSources, q.EraseProfileHead,
		q.EraseProfileRevisions, q.ErasePrivateKeys, q.EraseKeyRevocations, q.EraseKeys, q.EraseAliases)
}

// InsertErasure implements account.ErasureRepository.
func (r *AccountRepository) InsertErasure(ctx context.Context, tx uow.Tx, accountID string, at time.Time) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return wrap(q.InsertAccountErasure(ctx, sqlitegen.InsertAccountErasureParams{AccountID: accountID, ErasedAt: toMillis(at)}), "insert account erasure")
}

// AccountErased implements account.ErasureRepository.
func (r *AccountRepository) AccountErased(ctx context.Context, accountID string) (bool, error) {
	ok, err := r.db.reader().AccountErased(ctx, accountID)
	return ok, wrap(err, "find account erasure")
}

// EraseUserData implements identity.ErasureRepository.
func (r *IdentityRepository) EraseUserData(ctx context.Context, tx uow.Tx, userID string) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	// Children before their parents.
	return runSteps(ctx, userID, "erase user data",
		q.EraseEmailSuppressions, q.EraseEmailSuppressionLifts, q.EraseEmailDeliveryFailures,
		q.EraseEmailVerifications, q.EraseUserPrimaryEmail, q.EraseEmailAddresses, q.ErasePasswordCredentialRemovals, q.ErasePasswordCredentials,
		q.EraseTOTPRecoveryCodeUses, q.EraseTOTPRecoveryCodes, q.EraseTOTPEnrollmentRevocations, q.EraseTOTPEnrollments,
		q.ErasePasskeyCredentialRevocations, q.ErasePasskeyCredentials,
		q.EraseUserTokenRedemptions, q.EraseUserTokens, q.EraseSessionRevocations, q.EraseSessionActivity,
		q.EraseSessions, q.EraseAuthorizationCodeRedemptions, q.EraseAuthorizationCodes,
		q.EraseAccessTokenRevocations, q.EraseAccessTokens, q.EraseUserPreferences, q.EraseUserRoleBadgeDisplays, q.EraseUserTimelineLanguages, q.EraseUserTimeZone,
		q.EraseRegistrationDecisions, q.EraseRegistrationApplications, q.EraseRegistrationState, q.EraseServiceClosureEmails)
}

// ListAuthorStatusesToErase implements content.ErasureRepository.
func (r *StatusRepository) ListAuthorStatusesToErase(ctx context.Context, tx uow.Tx, authorAccountID, after string, limit int) ([]appcontent.ErasableStatus, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListAuthorStatusesToErase(ctx, sqlitegen.ListAuthorStatusesToEraseParams{
		AuthorAccountID: authorAccountID, After: after, Lim: int64(limit),
	})
	if err != nil {
		return nil, wrap(err, "list statuses to erase")
	}
	out := make([]appcontent.ErasableStatus, len(rows))
	for i, row := range rows {
		out[i] = appcontent.ErasableStatus{
			Status: statusFrom(sqlitegen.Status{
				ID: row.ID, Uri: row.Uri, Url: row.Url, AuthorAccountID: row.AuthorAccountID, IsLocal: row.IsLocal,
				Visibility: row.Visibility, InReplyToUri: row.InReplyToUri, ConversationUri: row.ConversationUri,
				PublishedAt: row.PublishedAt,
			}),
			Deleted: row.Deleted != 0,
		}
	}
	return out, nil
}

// EraseStatuses implements content.ErasureRepository.
func (r *StatusRepository) EraseStatuses(ctx context.Context, tx uow.Tx, statusIDs []string) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	ids, err := jsonList(statusIDs)
	if err != nil {
		return err
	}
	if err := runSteps(ctx, ids, "erase statuses",
		q.EraseLocalStatusesOfStatuses, q.EraseCountersOfStatuses, q.EraseLinkCardsOfStatuses, q.EraseBoostEdgesOfStatuses,
		q.EraseBoostRetractionsOfStatuses, q.EraseBoostsOfStatuses, q.EraseFavouritesOfStatuses,
		q.EraseReactionsOfStatuses, q.EraseReactionCountsOfStatuses, q.EraseBookmarksOfStatuses, q.ErasePinsOfStatuses); err != nil {
		return err
	}
	return r.PurgeStatuses(ctx, tx, statusIDs)
}

// EraseInteractionsBy implements content.ErasureRepository.
func (r *StatusRepository) EraseInteractionsBy(ctx context.Context, tx uow.Tx, accountID string, limit int) (int64, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return 0, err
	}
	lim := int64(limit)
	var most int64
	keep := func(n int64, err error) error {
		most = max(most, n)
		return err
	}
	steps := []func() error{
		func() error {
			return q.UncountFavouritesBy(ctx, sqlitegen.UncountFavouritesByParams{AccountID: accountID, Lim: lim})
		},
		func() error {
			return keep(q.EraseFavouritesBy(ctx, sqlitegen.EraseFavouritesByParams{AccountID: accountID, Lim: lim}))
		},
		func() error {
			return q.DeleteSingleReactionCountsBy(ctx, sqlitegen.DeleteSingleReactionCountsByParams{AccountID: accountID, Lim: lim})
		},
		func() error {
			return q.UncountReactionsBy(ctx, sqlitegen.UncountReactionsByParams{AccountID: accountID, Lim: lim})
		},
		func() error {
			return keep(q.EraseReactionsBy(ctx, sqlitegen.EraseReactionsByParams{AccountID: accountID, Lim: lim}))
		},
		func() error {
			return keep(q.EraseBookmarksBy(ctx, sqlitegen.EraseBookmarksByParams{AccountID: accountID, Lim: lim}))
		},
		func() error {
			return keep(q.ErasePollVotesBy(ctx, sqlitegen.ErasePollVotesByParams{AccountID: accountID, Lim: lim}))
		},
		func() error { return keep(q.ErasePinsBy(ctx, accountID)) },
		// The automatic deletion policy (FR-118) is a setting of the account.
		func() error { return q.EraseAutoDeleteKeptVisibilitiesBy(ctx, accountID) },
		func() error { return q.EraseAutoDeletePolicyBy(ctx, accountID) },
		func() error {
			return q.UncountBoostsBy(ctx, sqlitegen.UncountBoostsByParams{AccountID: accountID, Lim: lim})
		},
		func() error {
			// The boosts in force count as the account's local posts too (local_post_counters).
			n, err := q.EraseBoostEdgesBy(ctx, sqlitegen.EraseBoostEdgesByParams{AccountID: accountID, Lim: lim})
			if err := keep(n, err); err != nil {
				return err
			}
			return r.db.addLocalPosts(ctx, tx, accountID, -n)
		},
		func() error {
			return q.EraseBoostRetractionsBy(ctx, sqlitegen.EraseBoostRetractionsByParams{AccountID: accountID, Lim: lim})
		},
		func() error {
			return keep(q.EraseBoostsBy(ctx, sqlitegen.EraseBoostsByParams{AccountID: accountID, Lim: lim}))
		},
	}
	for _, step := range steps {
		if err := step(); err != nil {
			return 0, wrap(err, "erase interactions")
		}
	}
	return most, nil
}

// ListEdgesToErase implements graph.ErasureRepository.
func (r *GraphRepository) ListEdgesToErase(ctx context.Context, tx uow.Tx, accountID string, limit int) ([]graph.Edge, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListFollowEdgesToErase(ctx, sqlitegen.ListFollowEdgesToEraseParams{AccountID: accountID, Lim: int64(limit)})
	if err != nil {
		return nil, wrap(err, "list follow edges to erase")
	}
	return edgesFrom(rows), nil
}

// EraseFollowsOf implements graph.ErasureRepository.
func (r *GraphRepository) EraseFollowsOf(ctx context.Context, tx uow.Tx, accountID string, limit int) (int64, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return 0, err
	}
	list, err := q.ListFollowIDsToErase(ctx, sqlitegen.ListFollowIDsToEraseParams{AccountID: accountID, Lim: int64(limit)})
	if err != nil || len(list) == 0 {
		return 0, wrap(err, "list follows to erase")
	}
	ids, err := jsonList(list)
	if err != nil {
		return 0, err
	}
	err = runSteps(ctx, ids, "erase follows",
		q.EraseFollowAcceptances, q.EraseFollowRejections, q.EraseFollowTerminations, q.EraseFollowResends,
		q.EraseFollowPreferences, q.EraseFollowEdgesOfFollows, q.EraseFollows)
	return int64(len(list)), err
}

// EraseBlocksOf implements graph.ErasureRepository.
func (r *GraphRepository) EraseBlocksOf(ctx context.Context, tx uow.Tx, accountID string, limit int) (int64, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return 0, err
	}
	list, err := q.ListBlockIDsToErase(ctx, sqlitegen.ListBlockIDsToEraseParams{AccountID: accountID, Lim: int64(limit)})
	if err != nil || len(list) == 0 {
		return 0, wrap(err, "list blocks to erase")
	}
	ids, err := jsonList(list)
	if err != nil {
		return 0, err
	}
	err = runSteps(ctx, ids, "erase blocks", q.EraseBlockRetractions, q.EraseBlockEdgesOfBlocks, q.EraseBlocks)
	return int64(len(list)), err
}

// EraseMutesOf implements graph.ErasureRepository.
func (r *GraphRepository) EraseMutesOf(ctx context.Context, tx uow.Tx, accountID string, limit int) (int64, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return 0, err
	}
	by, err := q.EraseMutesBy(ctx, sqlitegen.EraseMutesByParams{AccountID: accountID, Lim: int64(limit)})
	if err != nil {
		return 0, wrap(err, "erase mutes")
	}
	of, err := q.EraseMutesOf(ctx, sqlitegen.EraseMutesOfParams{AccountID: accountID, Lim: int64(limit)})
	if err != nil {
		return 0, wrap(err, "erase mutes")
	}
	domains, err := q.EraseDomainMutesBy(ctx, sqlitegen.EraseDomainMutesByParams{AccountID: accountID, Lim: int64(limit)})
	return max(by, of, domains), wrap(err, "erase domain mutes")
}

// EraseCountersOf implements graph.ErasureRepository.
func (r *GraphRepository) EraseCountersOf(ctx context.Context, tx uow.Tx, accountID string) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return wrap(q.EraseFollowCounters(ctx, accountID), "erase follow counters")
}

// EraseFeedsOf implements timeline.ErasureRepository.
func (r *TimelineRepository) EraseFeedsOf(ctx context.Context, tx uow.Tx, accountID string, limit int) (int64, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return 0, err
	}
	lim := int64(limit)
	entries, err := q.ListFeedEntryIDsToErase(ctx, sqlitegen.ListFeedEntryIDsToEraseParams{AccountID: accountID, Lim: lim})
	if err != nil {
		return 0, wrap(err, "list feed entries to erase")
	}
	most := int64(len(entries))
	if len(entries) > 0 {
		ids, err := jsonList(entries)
		if err != nil {
			return 0, err
		}
		used, err := q.CountUsedFeedTagsOfEntries(ctx, ids)
		if err != nil {
			return 0, wrap(err, "count the counted tags of feed entries")
		}
		if err := runSteps(ctx, ids, "erase feed entries",
			q.EraseFeedDirectRecipientsOfEntries, q.EraseFeedTagEntriesOfEntries, q.EraseFeedEntries); err != nil {
			return 0, err
		}
		deltas := make(map[string]int64, len(used))
		for _, u := range used {
			deltas[u.TagID] = -u.N
		}
		if err := applyTagUsage(ctx, q, deltas); err != nil {
			return 0, err
		}
	}
	for _, step := range []func(context.Context, string, int64) (int64, error){
		func(ctx context.Context, a string, n int64) (int64, error) {
			return q.EraseFeedDirectRecipientsOf(ctx, sqlitegen.EraseFeedDirectRecipientsOfParams{AccountID: a, Lim: n})
		},
		func(ctx context.Context, a string, n int64) (int64, error) {
			return q.EraseFeedFollowEdgesAsFollower(ctx, sqlitegen.EraseFeedFollowEdgesAsFollowerParams{AccountID: a, Lim: n})
		},
		func(ctx context.Context, a string, n int64) (int64, error) {
			return q.EraseFeedFollowEdgesAsFollowee(ctx, sqlitegen.EraseFeedFollowEdgesAsFolloweeParams{AccountID: a, Lim: n})
		},
		func(ctx context.Context, a string, n int64) (int64, error) {
			return q.EraseFeedExclusionsAsViewer(ctx, sqlitegen.EraseFeedExclusionsAsViewerParams{AccountID: a, Lim: n})
		},
		func(ctx context.Context, a string, n int64) (int64, error) {
			return q.EraseFeedExclusionsAsTarget(ctx, sqlitegen.EraseFeedExclusionsAsTargetParams{AccountID: a, Lim: n})
		},
		func(ctx context.Context, a string, n int64) (int64, error) {
			return q.EraseFeedDomainMutesOf(ctx, sqlitegen.EraseFeedDomainMutesOfParams{AccountID: a, Lim: n})
		},
	} {
		n, err := step(ctx, accountID, lim)
		if err != nil {
			return 0, wrap(err, "erase feeds")
		}
		most = max(most, n)
	}
	return most, nil
}

// EraseNotificationsOf implements notification.ErasureRepository.
func (r *NotificationRepository) EraseNotificationsOf(ctx context.Context, tx uow.Tx, accountID, userID string, limit int) (int64, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return 0, err
	}
	lim := int64(limit)
	list, err := q.ListNotificationIDsToErase(ctx, sqlitegen.ListNotificationIDsToEraseParams{
		AccountID: accountID, UserID: toNullString(userID), Lim: lim,
	})
	if err != nil {
		return 0, wrap(err, "list notifications to erase")
	}
	most := int64(len(list))
	if len(list) > 0 {
		ids, err := jsonList(list)
		if err != nil {
			return 0, err
		}
		if err := runSteps(ctx, ids, "erase notifications", q.EraseNotificationDismissals, q.EraseNotifications); err != nil {
			return 0, err
		}
	}
	if err := q.EraseNotificationReadMarker(ctx, accountID); err != nil {
		return 0, wrap(err, "erase notification read marker")
	}
	if err := runSteps(ctx, accountID, "erase push subscriptions", q.ErasePushSubscriptionAlertsOf, q.ErasePushSubscriptionsOf); err != nil {
		return 0, err
	}
	viewer, err := q.EraseNotificationExclusionsAsViewer(ctx, sqlitegen.EraseNotificationExclusionsAsViewerParams{AccountID: accountID, Lim: lim})
	if err != nil {
		return 0, wrap(err, "erase notification exclusions")
	}
	target, err := q.EraseNotificationExclusionsAsTarget(ctx, sqlitegen.EraseNotificationExclusionsAsTargetParams{AccountID: accountID, Lim: lim})
	if err != nil {
		return 0, wrap(err, "erase notification exclusions")
	}
	domains, err := q.EraseNotificationDomainExclusionsOf(ctx, sqlitegen.EraseNotificationDomainExclusionsOfParams{AccountID: accountID, Lim: lim})
	if err != nil {
		return 0, wrap(err, "erase notification domain exclusions")
	}
	return max(most, viewer, target, domains), nil
}

// EraseStatusNotifications implements notification.ErasureRepository.
func (r *NotificationRepository) EraseStatusNotifications(ctx context.Context, tx uow.Tx, statusIDs []string) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	ids, err := jsonList(statusIDs)
	if err != nil {
		return err
	}
	return runSteps(ctx, ids, "erase notifications of statuses",
		q.EraseNotificationDismissalsOfStatuses, q.EraseNotificationsOfStatuses, q.EraseDeletedStatusesOfStatuses)
}

// runSteps runs the statements on the same argument in order and stops at the first error.
func runSteps(ctx context.Context, arg string, what string, steps ...func(context.Context, string) error) error {
	for _, step := range steps {
		if err := step(ctx, arg); err != nil {
			return wrap(err, what)
		}
	}
	return nil
}
