// SPDX-License-Identifier: AGPL-3.0-only
// The members of one list, the region that "Manage members" opens under the list's row (web-client/screens/lists.md
// §3–§5): its title (`h3`) with the count over the most members, the search of the accounts the user follows (each
// result "Add", or the "Added" mark for a member; nothing can be added to a full list, which says why), and the members
// with "Remove" and "Show more". Adding keeps focus in the search field; removing moves it to the next member, else to
// the search field. Both are announced (web-client/16-look-states-and-wording.md G24).
import {
  AccountItem,
  ActionResult,
  Badge,
  Box,
  Button,
  CheckIcon,
  Cluster,
  Heading,
  Hint,
  IconLabel,
  RegionError,
  Spinner,
  Stack,
  TextField,
} from "@my/ui";
import { type ReactNode, type RefObject, useId, useRef, useState } from "react";
import { handleOf } from "../../../lib/handle";
import { useT } from "../../../lib/i18n";
import { imageSources } from "../../../lib/images";
import { profilePath } from "../../../lib/paths";
import { useFocusAfterRemoval } from "../../../lib/useFocusAfterRemoval";
import { type Account, type List, maxListMembers } from "../api/lists";
import { accountName, isolate, useLimits } from "../hooks/names";
import {
  type MemberChanges,
  useFollowingSearch,
  useListMembers,
  useMemberChanges,
  useMembership,
} from "../hooks/useListMembers";

export function ListMembersPanel({ list }: { list: List }) {
  const t = useT();
  const limits = useLimits();
  const headingId = useId();
  // The box of the search field, whose input takes focus after adding and after removing the last member.
  const searchRef = useRef<HTMLDivElement>(null);
  const changes = useMemberChanges(list);
  return (
    <Box as="section" aria-labelledby={headingId} border="lg" background="sunken" padding="4">
      <Stack gap="4">
        <Cluster gap="2" align="center">
          <Heading level={3} id={headingId}>
            {t("client:lists.members.title", { name: isolate(list.name) })}
          </Heading>
          <Badge
            tone="neutral"
            count={list.members_count}
            limit={maxListMembers}
            label={t("client:lists.members.count", { count: list.members_count, max: limits.members })}
          />
        </Cluster>
        <FollowingSearch list={list} changes={changes} searchRef={searchRef} />
        <Members list={list} changes={changes} searchRef={searchRef} />
      </Stack>
    </Box>
  );
}

/** The search field's input in its box. */
function searchInput(box: RefObject<HTMLDivElement | null>): HTMLInputElement | null | undefined {
  return box.current?.querySelector("input");
}

type PartProps = { list: List; changes: MemberChanges; searchRef: RefObject<HTMLDivElement | null> };

/** The search field over the accounts the user follows, and its results. */
function FollowingSearch({ list, changes, searchRef }: PartProps) {
  const t = useT();
  const limits = useLimits();
  const [text, setText] = useState("");
  const search = useFollowingSearch(text);
  const accounts = search.status === "done" ? search.accounts : [];
  const isMember = useMembership(list, accounts);
  const fullId = useId();
  return (
    <Stack gap="3">
      <Box width="sm" ref={searchRef}>
        <TextField
          type="search"
          label={t("client:lists.members.search")}
          description={t("client:lists.members.search.description")}
          value={text}
          onChange={setText}
          autoComplete="off"
          autoCorrect="off"
          spellCheck="false"
        />
      </Box>
      {changes.isFull && <Hint id={fullId}>{t("client:lists.members.limit", { max: limits.members })}</Hint>}
      <div aria-live="polite">
        {search.status === "loading" && <Spinner label={t("client:lists.members.searching")} />}
        {search.status === "error" && (
          <Hint>{t("client:lists.members.search_failed", { reason: search.message })}</Hint>
        )}
        {search.status === "done" && accounts.length === 0 && <Hint>{t("client:lists.members.no_match")}</Hint>}
      </div>
      {accounts.length > 0 && (
        <Stack as="ul" divided aria-label={t("client:lists.members.results")}>
          {accounts.map((account) => {
            const member = isMember(account);
            const error = changes.addError(account);
            return (
              <Box as="li" key={account.id} paddingY="3" container>
                <AccountRow
                  account={account}
                  trailing={
                    <>
                      {member === true ? (
                        <IconLabel icon={<CheckIcon />}>{t("client:lists.members.added_mark")}</IconLabel>
                      ) : (
                        <Button
                          size="sm"
                          isPending={changes.addingId === account.id || member === undefined}
                          // A full list keeps the button in the tab order and says why it does nothing (screens/lists.md §4).
                          {...(changes.isFull && { "aria-disabled": true, "aria-describedby": fullId })}
                          aria-label={t("client:lists.members.add_of", { name: isolate(accountName(account)) })}
                          onPress={() => {
                            if (changes.isFull) return;
                            changes.add(account, () => {
                              searchInput(searchRef)?.focus();
                            });
                          }}
                        >
                          {t("client:lists.members.add")}
                        </Button>
                      )}
                      {error !== undefined && <ActionResult result={{ tone: "danger", message: error }} />}
                    </>
                  }
                />
              </Box>
            );
          })}
        </Stack>
      )}
    </Stack>
  );
}

/** The members, a page at a time, each with "Remove". */
function Members({ list, changes, searchRef }: PartProps) {
  const t = useT();
  const members = useListMembers(list);
  const listRef = useRef<HTMLUListElement>(null);
  const removed = useFocusAfterRemoval(
    listRef,
    members.status === "done" ? members.members.map((account) => account.id) : [],
    () => searchInput(searchRef),
  );
  const result =
    changes.removeError === undefined ? undefined : { tone: "danger" as const, message: changes.removeError };
  switch (members.status) {
    case "loading":
      return (
        <div aria-busy="true">
          <Spinner label={t("client:lists.members.loading")} />
        </div>
      );
    case "error":
      return (
        <RegionError
          headingLevel={4}
          heading={t("client:lists.members.failed")}
          reason={members.message}
          retryLabel={t("client:lists.retry")}
          onRetry={members.retry}
          isRetrying={members.isRetrying}
        />
      );
    case "done":
      return (
        <Stack gap="3">
          <div>
            <ActionResult result={result} />
          </div>
          {members.members.length === 0 ? (
            <Hint>{t("client:lists.members.empty")}</Hint>
          ) : (
            <Stack as="ul" ref={listRef} divided aria-label={t("client:lists.members.list")}>
              {members.members.map((account) => (
                <Box as="li" key={account.id} data-row-id={account.id} paddingY="3" container>
                  <AccountRow
                    account={account}
                    trailing={
                      <Button
                        size="sm"
                        isPending={changes.removingId === account.id}
                        aria-label={t("client:lists.members.remove_of", { name: isolate(accountName(account)) })}
                        onPress={() => {
                          changes.remove(account, () => {
                            removed(account.id);
                          });
                        }}
                      >
                        {t("client:lists.members.remove")}
                      </Button>
                    }
                  />
                </Box>
              ))}
            </Stack>
          )}
          {members.loadMoreError !== undefined && (
            <Hint>{t("client:lists.members.more_failed", { reason: members.loadMoreError })}</Hint>
          )}
          {members.hasMore && (
            <div>
              <Button onPress={members.loadMore} isPending={members.isLoadingMore}>
                {t("client:lists.members.more")}
              </Button>
            </div>
          )}
        </Stack>
      );
  }
}

/** An account: its avatar, display name and handle as a link to its profile, with its button after them. */
function AccountRow({ account, trailing }: { account: Account; trailing: ReactNode }) {
  return (
    <AccountItem
      name={accountName(account)}
      handle={handleOf(account.acct)}
      avatar={imageSources(account.avatar)}
      avatarSeed={account.id}
      href={profilePath(account.acct)}
      trailing={trailing}
      trailingPlacement="below"
    />
  );
}
