// SPDX-License-Identifier: AGPL-3.0-only

// The lists of whom an account follows and who follows it (FR-311), which its profile's counts open: one screen under a
// button back to the profile, a tab each named with its count, the tab the reader came by chosen. Each account a row,
// newest follow first, with its follow at the end (the primary button until the reader follows it or asked to, then
// what is so); then a line that the list loads, that it failed (with a retry), that it is empty, that its owner keeps
// it from the reader, or a fold that shows more while more follow. The owner always reads their own lists.

import { AccountLine, nameOf } from "@my/app-components";
import {
  accountFollowsQuery,
  loadAccount,
  useAccount,
  useAccountFollows,
  useChangeRelationship,
  useRelationship,
  useViewer,
  viewerQuery,
  type Account,
  type AppData,
  type FollowsList,
} from "@my/app-data";
import {
  BackButton,
  Button,
  FeedNotice,
  Fold,
  ScreenStack,
  StateText,
  TabRow,
  TextButton,
  useLocale,
} from "@my/design-system";
import { useState } from "react";
import { stringsIn } from "../locale";

type T = ReturnType<typeof stringsIn>["t"];

/**
 * What the route of an account's list waits for: the account of the handle, then the list's first page with the
 * reader's relationships with its accounts, and the reader's own account. A failure is left to the screen.
 */
export async function loadFollows(data: AppData, acct: string, list: FollowsList): Promise<void> {
  await Promise.all([
    loadAccount(data, acct).then((account) =>
      data.queryClient.infiniteQuery({ ...accountFollowsQuery(data, account.id, list), staleTime: Infinity }),
    ),
    data.queryClient.query({ ...viewerQuery(data), staleTime: Infinity }),
  ]).catch(() => undefined);
}

/** The follow of a row: follow (back, or ask to) until the reader follows the account or asked to, then what is so. */
function RowFollow({ account, t }: { account: Account; t: T }) {
  const relationship = useRelationship(account.id);
  const change = useChangeRelationship();
  const [busy, setBusy] = useState(false);
  if (relationship === undefined || relationship.blocking === true) return null;
  if (relationship.following === true) return <StateText>{t("profile.following")}</StateText>;
  if (relationship.requested === true) return <StateText>{t("profile.requested")}</StateText>;
  return (
    <Button
      size="timeline"
      kind="primary"
      busy={busy}
      onPress={() => {
        setBusy(true);
        change(account.id, "follow")
          .catch(() => undefined)
          .finally(() => {
            setBusy(false);
          });
      }}
    >
      {t(
        account.locked ? "profile.request_follow" : relationship.followed_by ? "profile.follow_back" : "profile.follow",
      )}
    </Button>
  );
}

/** The rows of the list, then its line: loading, failed, empty, kept from the reader, or the fold of more. */
function Rows({ account, list, t }: { account: Account; list: FollowsList; t: T }) {
  const follows = useAccountFollows(account.id, list);
  const viewer = useViewer()?.id;
  const retry = (onPress: () => void) => <TextButton onPress={onPress}>{t("profile.retry")}</TextButton>;
  const failed = list === "followers" ? "profile.followers_failed" : "profile.following_failed";
  const loading = list === "followers" ? "profile.followers_loading" : "profile.following_loading";
  const failedMore = follows.status === "success" && follows.error !== null && !follows.isFetchingNextPage;
  const end = follows.hidden ? (
    // The server does not say whether the owner shows the list to their followers or to nobody: one line, true of
    // both for a reader it keeps the list from.
    <FeedNotice start>{t("profile.follows_hidden_private", { name: nameOf(account) })}</FeedNotice>
  ) : follows.status === "pending" || follows.isFetchingNextPage ? (
    <FeedNotice start>{t(loading)}</FeedNotice>
  ) : follows.status === "error" ? (
    <FeedNotice start alert action={retry(follows.refetch)}>
      {t(failed)}
    </FeedNotice>
  ) : failedMore && follows.hasNextPage ? (
    <FeedNotice start alert action={retry(follows.fetchNextPage)}>
      {t(failed)}
    </FeedNotice>
  ) : follows.items.length === 0 ? (
    <FeedNotice start>{t(list === "followers" ? "profile.followers_empty" : "profile.following_empty")}</FeedNotice>
  ) : follows.hasNextPage ? (
    <Fold>
      <TextButton onPress={follows.fetchNextPage}>{t("profile.follows_more")}</TextButton>
    </Fold>
  ) : undefined;
  return (
    <ScreenStack busy={follows.status === "pending"}>
      {follows.items.map((a) => (
        <AccountLine key={a.id} account={a} end={a.id === viewer ? undefined : <RowFollow account={a} t={t} />} />
      ))}
      {end}
    </ScreenStack>
  );
}

/**
 * The lists of the account of a handle, `list` the tab shown: `onList` opens the other, `onBack` goes back to the
 * profile.
 */
export function Follows({
  acct,
  list,
  onList,
  onBack,
}: {
  acct: string;
  list: FollowsList;
  onList: (list: FollowsList) => void;
  onBack: () => void;
}) {
  const { t, lang } = stringsIn(useLocale().locale);
  const { account, error } = useAccount(acct);
  const back = <BackButton onPress={onBack}>{t("profile.follows_back")}</BackButton>;
  if (account === undefined)
    return (
      <ScreenStack busy={error === null}>
        {back}
        {error === null ? (
          <FeedNotice start>{t("profile.loading")}</FeedNotice>
        ) : (
          <FeedNotice start alert>
            {t("profile.failed")}
          </FeedNotice>
        )}
      </ScreenStack>
    );
  const n = (v: number | undefined) => (v === undefined ? "" : ` ${new Intl.NumberFormat(lang).format(v)}`);
  return (
    <ScreenStack>
      {back}
      <TabRow
        label={t("profile.follows_tabs")}
        tabs={[
          { id: "following", name: `${t("profile.following_count")}${n(account.following_count)}` },
          { id: "followers", name: `${t("profile.followers_count")}${n(account.followers_count)}` },
        ]}
        current={list}
        onChange={onList}
      >
        <Rows account={account} list={list} t={t} />
      </TabRow>
    </ScreenStack>
  );
}
