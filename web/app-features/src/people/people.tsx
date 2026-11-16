// SPDX-License-Identifier: AGPL-3.0-only

// Finding people (FR-308): the accounts the server suggests the reader follow, each a row with its follow, on a screen
// of its own; the first few of them where the reader starts out (the home timeline while it is empty, the search before
// anything is written), with a link to all. A one-person server suggests no one (ADR-0152): there the few show nothing
// at all, and the screen says there is no one to suggest. Where the discovery's places go is `DiscoveryLayout`'s.

import { AccountLine } from "@my/app-components";
import {
  suggestionsQuery,
  useChangeRelationship,
  useInstance,
  useRelationship,
  useSuggestions,
  type Account,
  type AppData,
} from "@my/app-data";
import {
  Button,
  FeedNotice,
  Fold,
  GroupHeading,
  RowItem,
  RowList,
  ScreenStack,
  ScreenTitle,
  StateText,
  TextButton,
  TextLink,
  useLocale,
} from "@my/design-system";
import { createContext, use, useRef, type ReactNode } from "react";
import { stringsIn } from "../locale";

type T = ReturnType<typeof stringsIn>["t"];

/** Where the find-people screen opens from. */
const peoplePath = "/people";

/**
 * How the screens of finding show what they find, a choice of the decider's while it is open: the results of a search
 * in `sections` one under another or in `tabs`; and where finding people is reached, from the `search` before anything
 * is written, from a place of the `menu`, or `both`.
 */
export type DiscoveryLayout = { readonly results: "sections" | "tabs"; readonly people: "search" | "menu" | "both" };

const DiscoveryContext = createContext<DiscoveryLayout>({ results: "sections", people: "both" });

/** Sets how the screens of finding inside show what they find. */
export function DiscoveryLayoutProvider({ value, children }: { value: DiscoveryLayout; children: ReactNode }) {
  return <DiscoveryContext value={value}>{children}</DiscoveryContext>;
}

/** How the screens of finding show what they find. */
export function useDiscoveryLayout(): DiscoveryLayout {
  return use(DiscoveryContext);
}

/** What the find-people screen's loader waits for: the suggestions. A failure is left to the screen. */
export async function loadPeople(data: AppData): Promise<void> {
  await data.queryClient.query({ ...suggestionsQuery(data), staleTime: Infinity }).catch(() => undefined);
}

/**
 * The follow of an account's row: follow (back, or ask to) until the reader follows it or asked to, then what is so. A
 * press while the server answers does nothing; no button shows itself busy, which React Aria would announce by an id
 * the answer takes away.
 */
function FollowAction({ account, t }: { account: Account; t: T }) {
  const relationship = useRelationship(account.id);
  const change = useChangeRelationship();
  const sending = useRef(false);
  if (relationship === undefined || relationship.blocking === true) return null;
  if (relationship.following === true) return <StateText>{t("profile.following")}</StateText>;
  if (relationship.requested === true) return <StateText>{t("profile.requested")}</StateText>;
  return (
    <Button
      size="timeline"
      kind="primary"
      onPress={() => {
        if (sending.current) return;
        sending.current = true;
        change(account.id, "follow")
          .catch(() => undefined)
          .finally(() => {
            sending.current = false;
          });
      }}
    >
      {t(
        account.locked ? "profile.request_follow" : relationship.followed_by ? "profile.follow_back" : "profile.follow",
      )}
    </Button>
  );
}

/** Accounts as a list named by `label`, each a row with its follow. */
export function AccountRows({ label, accounts, t }: { label: string; accounts: readonly Account[]; t: T }) {
  return (
    <RowList label={label}>
      {accounts.map((a) => (
        <RowItem key={a.id}>
          <AccountLine account={a} end={<FollowAction account={a} t={t} />} />
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
      <AccountRows label={label} accounts={few} t={t} />
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
 * The first few suggestions under their heading, where the reader starts out (`intro` says why, on an empty home);
 * nothing on a one-person server.
 */
export function SuggestedPeople({ intro }: { intro?: string | undefined }) {
  const { t } = stringsIn(useLocale().locale);
  const instance = useInstance();
  if (instance === undefined || instance.single_user)
    return intro === undefined ? null : <FeedNotice>{intro}</FeedNotice>;
  return (
    <ScreenStack>
      {intro !== undefined && <FeedNotice start>{intro}</FeedNotice>}
      <GroupHeading>{t("people.suggested")}</GroupHeading>
      <Suggested label={t("people.suggested")} limit={fewSuggestions} t={t} />
    </ScreenStack>
  );
}
