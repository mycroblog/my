// SPDX-License-Identifier: AGPL-3.0-only

// A list's members (ADR-0013 §11, §14, §15), a screen of their own under a back button named by the list: its title
// with how many, the field that finds an account the reader follows (the only ones a list may hold), what it found on
// the raised surface (Add, or Added once in the list), then the members as a list, each with Remove, named by whom it
// takes out. Adding puts the account at
// the top of the members and in the count at once; the server's refusal at 1,000 members, or any failure, is said
// under the field, Add dimmed at the limit. The members load a page at a time, with a fold that shows more.

import { AccountLine, nameOf } from "@my/app-components";
import {
  listLimitOf,
  listMembersQuery,
  listQuery,
  useFollowedSearch,
  useList,
  useListMembers,
  useListWrites,
  type Account,
  type AppData,
} from "@my/app-data";
import {
  BackButton,
  Button,
  FeedNotice,
  Fold,
  FoundRows,
  RowItem,
  RowList,
  ScreenStack,
  ScreenTitle,
  SearchField,
  StateText,
  TextButton,
  useLocale,
} from "@my/design-system";
import { useId, useRef, useState } from "react";
import { stringsIn } from "../locale";
import { listWriteError } from "./lists";

/** What the members' loader waits for: the list and the first page of its members. */
export async function loadListMembers(data: AppData, id: string): Promise<void> {
  await Promise.all([
    data.queryClient.query({ ...listQuery(data, id), staleTime: Infinity }),
    data.queryClient.infiniteQuery({ ...listMembersQuery(data, id), staleTime: Infinity }),
  ]).catch(() => undefined);
}

/** A refused or failed write, said under the field; `limit` when the list holds as many as it may. */
type Refusal = { readonly text: string; readonly limit: boolean };

/** The members of the reader's list of `id`; `onBack` goes back to the list's timeline. */
export function ListMembers({ id, onBack }: { id: string; onBack: () => void }) {
  const { t } = stringsIn(useLocale().locale);
  const { list, status, refetch } = useList(id);
  const members = useListMembers(id);
  const writes = useListWrites();
  const fieldId = useId();
  const [q, setQ] = useState("");
  const [refused, setRefused] = useState<Refusal | undefined>(undefined);
  const sending = useRef(false);
  const found = useFollowedSearch(q);
  // Named by the list it goes back to, once it is known.
  const back = <BackButton onPress={onBack}>{list?.name ?? t("timeline.list_back")}</BackButton>;
  // A press while the server answers another does nothing. No button shows itself busy: React Aria announces a pending
  // button by its id, which Added (or the row gone) takes away, and the announcement would be left naming nothing.
  const change = (account: Account, add: boolean) => {
    if (sending.current) return;
    sending.current = true;
    writes
      .member(id, account, add)
      .then(
        () => {
          setRefused(undefined);
        },
        (e: unknown) => {
          setRefused({ text: listWriteError(e, t), limit: listLimitOf(e) === "members" });
        },
      )
      .finally(() => {
        sending.current = false;
      });
  };
  const retry = (onPress: () => void) => <TextButton onPress={onPress}>{t("timeline.retry")}</TextButton>;
  if (list === undefined && status !== "pending")
    return (
      <ScreenStack>
        {back}
        {status === "gone" ? (
          <FeedNotice start>{t("timeline.list_gone")}</FeedNotice>
        ) : (
          <FeedNotice start alert action={retry(refetch)}>
            {t("timeline.list_failed")}
          </FeedNotice>
        )}
      </ScreenStack>
    );
  const inList = new Set(members.items.map((a) => a.id));
  const failedMore = members.status === "success" && members.error !== null && !members.isFetchingNextPage;
  const end =
    members.status === "pending" || members.isFetchingNextPage ? (
      <FeedNotice start>{t("timeline.list_loading")}</FeedNotice>
    ) : members.status === "error" ? (
      <FeedNotice start alert action={retry(members.refetch)}>
        {t("timeline.list_failed")}
      </FeedNotice>
    ) : failedMore && members.hasNextPage ? (
      <FeedNotice start alert action={retry(members.fetchNextPage)}>
        {t("timeline.list_failed")}
      </FeedNotice>
    ) : members.items.length === 0 ? (
      <FeedNotice start>{t("timeline.list_members_empty")}</FeedNotice>
    ) : members.hasNextPage ? (
      <Fold>
        <TextButton onPress={members.fetchNextPage}>{t("profile.follows_more")}</TextButton>
      </Fold>
    ) : undefined;
  return (
    <ScreenStack busy={members.status === "pending"}>
      <ScreenTitle
        back={back}
        glyph="members"
        count={list === undefined ? undefined : t("timeline.list_members", { count: list.members_count })}
      >
        {t("timeline.list_members_open")}
      </ScreenTitle>
      <SearchField
        id={fieldId}
        label={t("timeline.list_add")}
        error={refused?.text}
        clearLabel={t("search.clear")}
        onChange={(value) => {
          setQ(value.trim());
        }}
      />
      {found.items !== undefined && found.items.length > 0 && (
        <FoundRows label={t("timeline.list_add")}>
          {found.items.map((a) => (
            <AccountLine
              key={a.id}
              account={a}
              end={
                inList.has(a.id) ? (
                  <StateText>{t("timeline.list_added")}</StateText>
                ) : (
                  <Button
                    size="timeline"
                    isDisabled={refused?.limit === true}
                    onPress={() => {
                      change(a, true);
                    }}
                  >
                    {t("timeline.list_add_button")}
                  </Button>
                )
              }
            />
          ))}
        </FoundRows>
      )}
      {members.items.length > 0 && (
        <RowList label={t("timeline.list_members_open")}>
          {members.items.map((a) => (
            <RowItem key={a.id}>
              <AccountLine
                account={a}
                end={
                  // Named by whom it takes out: every row has one.
                  <Button
                    size="timeline"
                    kind="quiet"
                    aria-label={t("timeline.list_remove_member", { name: nameOf(a) })}
                    onPress={() => {
                      change(a, false);
                    }}
                  >
                    {t("timeline.list_remove")}
                  </Button>
                }
              />
            </RowItem>
          ))}
        </RowList>
      )}
      {end}
    </ScreenStack>
  );
}
