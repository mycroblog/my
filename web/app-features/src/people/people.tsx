// SPDX-License-Identifier: AGPL-3.0-only

// Finding people (FR-308): the accounts the server suggests the reader follow, each a row with its follow, on a screen
// of its own; the first few of them where the reader starts out (the home timeline while it is empty, the search before
// anything is written, the decider's pick over a place of the menu), with a link to all. Finding people shows nowhere
// unless the server suggests at least one account: a one-person server suggests no one (ADR-0152), and any server may
// have none to suggest. Its screen, reached by its address, then says plainly there is no one to suggest.

import { AccountLine } from "@my/app-components";
import { suggestionsQuery, useInstance, useSuggestions, type Account, type AppData } from "@my/app-data";
import {
  FeedNotice,
  Fold,
  ResultGroup,
  RowItem,
  RowList,
  ScreenStack,
  ScreenTitle,
  TextButton,
  TextLink,
  useLocale,
} from "@my/design-system";
import { FollowOf } from "../account-follow";
import { stringsIn } from "../locale";

type T = ReturnType<typeof stringsIn>["t"];

/** Where the find-people screen opens from. */
const peoplePath = "/people";

/** What the find-people screen's loader waits for: the suggestions. A failure is left to the screen. */
export async function loadPeople(data: AppData): Promise<void> {
  await data.queryClient.query({ ...suggestionsQuery(data), staleTime: Infinity }).catch(() => undefined);
}

/** Accounts as a list named by `label`, each a row with its follow. */
export function AccountRows({ label, accounts }: { label: string; accounts: readonly Account[] }) {
  return (
    <RowList label={label}>
      {accounts.map((a) => (
        <RowItem key={a.id}>
          <AccountLine account={a} end={<FollowOf account={a} />} />
        </RowItem>
      ))}
    </RowList>
  );
}

/** The suggestions, or what stands in their place: that they load, that they failed with Try again, or none. */
function Suggested({ label, limit, t }: { label: string; limit?: number | undefined; t: T }) {
  const { items, status, refetch } = useSuggestions();
  if (status === "pending")
    return (
      <ScreenStack busy>
        <FeedNotice start>{t("people.loading")}</FeedNotice>
      </ScreenStack>
    );
  if (status === "error")
    return (
      <FeedNotice start alert action={<TextButton onPress={refetch}>{t("people.retry")}</TextButton>}>
        {t("people.failed")}
      </FeedNotice>
    );
  if (items.length === 0) return <FeedNotice start>{t("people.empty")}</FeedNotice>;
  const few = limit === undefined ? items : items.slice(0, limit);
  return (
    <>
      <AccountRows label={label} accounts={few} />
      {limit !== undefined && items.length > limit && (
        <Fold>
          <TextLink href={peoplePath}>{t("people.more")}</TextLink>
        </Fold>
      )}
    </>
  );
}

/** The find-people screen: its title, then the suggested accounts, each with its follow. */
export function FindPeople() {
  const { t } = stringsIn(useLocale().locale);
  const single = useInstance()?.single_user === true;
  return (
    <ScreenStack>
      <ScreenTitle glyph="follow">{t("people.title")}</ScreenTitle>
      {single ? <FeedNotice start>{t("people.empty")}</FeedNotice> : <Suggested label={t("people.title")} t={t} />}
    </ScreenStack>
  );
}

/** How many suggestions show where the reader starts out, before the link to all of them. */
const fewSuggestions = 5;

/**
 * Whether the server suggests anyone to follow: true once it answered with at least one account, false when it
 * suggests none (a one-person server, an empty answer, a failure), undefined while it is asked. Finding people shows
 * nowhere but at its address unless it is true.
 */
function useSuggestionsOffered(): boolean | undefined {
  const instance = useInstance();
  const { items, status } = useSuggestions(instance !== undefined && !instance.single_user);
  if (instance?.single_user === true) return false;
  if (instance === undefined || status === "pending") return undefined;
  return status === "success" && items.length > 0;
}

/**
 * The first few suggestions under their heading, where the reader starts out (`intro` says why, on an empty home);
 * nothing at all while the server suggests no one, busy while it is asked.
 */
export function SuggestedPeople({ intro }: { intro?: string | undefined }) {
  const { t } = stringsIn(useLocale().locale);
  const offered = useSuggestionsOffered();
  if (offered === undefined) return <ScreenStack busy>{null}</ScreenStack>;
  if (!offered) return null;
  return (
    <ScreenStack>
      {intro !== undefined && <FeedNotice start>{intro}</FeedNotice>}
      <ResultGroup title={t("people.suggested")}>
        <Suggested label={t("people.suggested")} limit={fewSuggestions} t={t} />
      </ResultGroup>
    </ScreenStack>
  );
}
