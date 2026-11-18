// SPDX-License-Identifier: AGPL-3.0-only

// Relationships, the entry of the settings at /settings/relationships (FR-303, FR-304): the four lists of what the
// reader keeps between them and others, a section each, in this order: the accounts they block, the accounts they
// mute, the servers they mute, and the hashtags they follow. A reader comes here to undo one thing and leave, so the
// entry shows every list at once, the first few of each, its count once the whole of it is read, and the door to the
// whole list on a screen under the entry (/settings/relationships/<list>), which pages through it, only where it has
// more. An account leads to its profile, and a muted one says what the mute covers (its notifications too, until
// when) where the server says it.
//
// Each row is taken out of its list as the profile's and a post's menus undo the same: at once, with no confirmation
// (Unblock, Unmute, the server's Unmute, the hashtag's Unfollow), through app-data's writes, which read the list again.
// What came of it is the section's status, a polite live region there from the start: once the row is gone the focus
// goes to it, where it says what was done; a write that failed says so there and leaves the row and the focus as they
// were. An empty list says what it would hold and how something comes to be in it.

import { AccountLine, sentenceHandleOf } from "@my/app-components";
import {
  blocksQuery,
  domainMutesQuery,
  followedHashtagsQuery,
  mutesQuery,
  useBlocks,
  useChangeRelationship,
  useFollowedHashtagList,
  useFollowTag,
  useMutedDomains,
  useMuteDomain,
  useMutes,
  type Account,
  type AppData,
  type FollowedHashtag,
  type MutedAccount,
  type PagedList,
} from "@my/app-data";
import {
  BackButton,
  Box,
  Button,
  dateFormat,
  FeedNotice,
  LinkButton,
  Row,
  SettingsEmpty,
  SettingsList,
  SettingsListItem,
  SettingsListRow,
  SettingsScreen,
  SettingsSection,
  Stack,
  TextButton,
  TextLink,
  useLocale,
  useTimeZone,
} from "@my/design-system";
import { Fragment, useEffect, useId, useRef, useState, type ReactNode } from "react";
import { stringsIn } from "../locale";
import { ReadingNotice } from "./controls";
import type { SettingsEntryProps } from "./entries";

type T = ReturnType<typeof stringsIn>["t"];

/** The lists of the entry, in its order. */
const relationshipLists = ["blocked", "muted", "servers", "hashtags"] as const;
type RelationshipList = (typeof relationshipLists)[number];

/** The address of the entry's screen. */
const entryPath = "/settings/relationships";

/** The address of the screen under the entry that holds the whole of `list`. */
const relationshipListPath = (list: RelationshipList): string => `${entryPath}/${list}`;

/** Whether `path`, the last part of an address under the entry, names one of its lists. */
const isRelationshipList = (path: string): path is RelationshipList => relationshipLists.some((list) => list === path);

/** How many of each list the entry shows before the door to the whole of it. */
const few = 5;

/** What the route's loader waits for: the first page of each list, from the cache when it has them. */
export async function loadRelationshipSettings(data: AppData): Promise<void> {
  await Promise.all([
    data.queryClient.infiniteQuery({ ...blocksQuery(data), staleTime: Infinity }),
    data.queryClient.infiniteQuery({ ...mutesQuery(data), staleTime: Infinity }),
    data.queryClient.query({ ...domainMutesQuery(data), staleTime: Infinity }),
    data.queryClient.query({ ...followedHashtagsQuery(data), staleTime: Infinity }),
  ]).catch(() => undefined);
}

/** What takes a row out of its list: the write, and what the section's status says of it, done or not. */
type Removal = { readonly write: () => Promise<void>; readonly done: string; readonly failed: string };

/**
 * The status of a section and its rows' removals: `remove` runs a removal once at a time for a row (`busy` tells
 * which are on their way), and `status`, the live region that says what came of the last one, takes the focus once
 * the row it took out is gone.
 */
function useRemovals() {
  const id = useId();
  const [said, setSaid] = useState<{ readonly text: string; readonly n: number } | undefined>(undefined);
  const [busy, setBusy] = useState<ReadonlySet<string>>(new Set());
  const count = useRef(0);
  const focus = useRef(false);
  useEffect(() => {
    if (!focus.current) return;
    focus.current = false;
    document.getElementById(id)?.focus();
  });
  const remove = (key: string, { write, done, failed }: Removal) => {
    if (busy.has(key)) return;
    setBusy((now) => new Set(now).add(key));
    const say = (text: string) => {
      count.current += 1;
      setSaid({ text, n: count.current });
    };
    write()
      .then(
        () => {
          // The write resolves once the list is read again, without the row: the focus it held goes to what says so.
          focus.current = true;
          say(done);
        },
        () => {
          say(failed);
        },
      )
      .finally(() => {
        setBusy((now) => {
          const next = new Set(now);
          next.delete(key);
          return next;
        });
      });
  };
  const status = (
    <Box as="p" id={id} role="status" tabIndex={-1} text="caption" tone="muted">
      {said !== undefined && <span key={said.n}>{said.text}</span>}
    </Box>
  );
  return { busy, remove, status };
}

/** What a section of a list shows of it, and how. */
type ListOf<I> = {
  readonly list: RelationshipList;
  readonly items: PagedList<I>;
  readonly keyOf: (item: I) => string;
  /** A row of the list, `busy` while its removal is on its way, `remove` running it. */
  readonly row: (item: I, busy: boolean, remove: (removal: Removal) => void) => ReactNode;
};

/**
 * A list's section: its rows, or what stands in their place (that it loads, that it failed, that it is empty), the
 * status of its removals, and at the end the door to the whole list (`capped`, on the entry) or the next page.
 */
function ListSection<I>({ list, items, keyOf, row, capped, t }: ListOf<I> & { capped: boolean; t: T }) {
  const { lang } = stringsIn(useLocale().locale);
  const { busy, remove, status } = useRemovals();
  const heading = t("settings.relationships.list", { list });
  const whole = items.status === "success" && !items.hasNextPage;
  const shown = capped ? items.items.slice(0, few) : items.items;
  const failedMore = items.status === "success" && items.error !== null && items.hasNextPage;
  const rows =
    items.status === "pending" ? (
      <SettingsEmpty>{t("settings.relationships.loading")}</SettingsEmpty>
    ) : items.status === "error" ? (
      <SettingsEmpty action={<TextButton onPress={items.refetch}>{t("settings.retry")}</TextButton>}>
        {t("settings.relationships.failed_list")}
      </SettingsEmpty>
    ) : items.items.length === 0 ? (
      <SettingsEmpty>{t("settings.relationships.empty", { list })}</SettingsEmpty>
    ) : (
      <SettingsList label={heading}>
        {shown.map((item) => {
          const key = keyOf(item);
          return (
            <Fragment key={key}>
              {row(item, busy.has(key), (removal) => {
                remove(key, removal);
              })}
            </Fragment>
          );
        })}
      </SettingsList>
    );
  const more = capped ? (
    (items.items.length > few || items.hasNextPage) && (
      <Row justify="start">
        <LinkButton href={relationshipListPath(list)} size="timeline">
          {t("settings.relationships.all", { list })}
        </LinkButton>
      </Row>
    )
  ) : failedMore && !items.isFetchingNextPage ? (
    <SettingsEmpty action={<TextButton onPress={items.fetchNextPage}>{t("settings.retry")}</TextButton>}>
      {t("settings.relationships.failed_more")}
    </SettingsEmpty>
  ) : (
    items.hasNextPage && (
      <Row justify="start">
        <Button size="timeline" busy={items.isFetchingNextPage} onPress={items.fetchNextPage}>
          {t("settings.relationships.more")}
        </Button>
      </Row>
    )
  );
  return (
    <SettingsSection
      heading={heading}
      count={whole && items.items.length > 0 ? new Intl.NumberFormat(lang).format(items.items.length) : undefined}
    >
      <Stack>
        {rows}
        {status}
      </Stack>
      {more}
    </SettingsSection>
  );
}

/** When a mute ends, as the reader's locale and zone write it. */
function useEnds(): (at: string) => string {
  const { locale } = useLocale();
  const zone = useTimeZone();
  const format = dateFormat(locale, zone, { dateStyle: "medium", timeStyle: "short" });
  return (at) => format.format(new Date(at));
}

/** The blocked accounts: each its row, leading to its profile, with Unblock. */
function Blocked({ capped, t }: { capped: boolean; t: T }) {
  const change = useChangeRelationship();
  return (
    <ListSection<Account>
      list="blocked"
      items={useBlocks()}
      keyOf={(account) => account.id}
      capped={capped}
      t={t}
      row={(account, busy, remove) => {
        const name = sentenceHandleOf(account);
        return (
          <SettingsListItem>
            <AccountLine
              account={account}
              end={
                <Button
                  size="timeline"
                  busy={busy}
                  onPress={() => {
                    remove({
                      write: () => change(account.id, "unblock"),
                      done: t("settings.relationships.done", { action: "unblock", name }),
                      failed: t("settings.relationships.failed", { action: "unblock", name }),
                    });
                  }}
                >
                  {t("settings.relationships.unblock")}
                </Button>
              }
            />
          </SettingsListItem>
        );
      }}
    />
  );
}

/**
 * What a mute covers beyond the account's posts, a line each, where the server says it: its notifications, and when
 * it ends; undefined for a mute of the posts alone that does not end.
 */
function muteCovers(mute: MutedAccount, ends: (at: string) => string, t: T): ReactNode {
  const lines = [
    ...(mute.hide_notifications ? [t("settings.relationships.muted.notifications")] : []),
    ...(mute.expires_at === undefined
      ? []
      : [t("settings.relationships.muted.until", { date: ends(mute.expires_at) })]),
  ];
  if (lines.length === 0) return undefined;
  return (
    <Stack as="span">
      {lines.map((line) => (
        <Box as="span" key={line}>
          {line}
        </Box>
      ))}
    </Stack>
  );
}

/** The muted accounts: each its row, leading to its profile, what the mute covers under it, with Unmute. */
function Muted({ capped, t }: { capped: boolean; t: T }) {
  const change = useChangeRelationship();
  const ends = useEnds();
  return (
    <ListSection<MutedAccount>
      list="muted"
      items={useMutes()}
      keyOf={(mute) => mute.account.id}
      capped={capped}
      t={t}
      row={(mute, busy, remove) => {
        const name = sentenceHandleOf(mute.account);
        return (
          <SettingsListItem>
            <AccountLine
              account={mute.account}
              note={muteCovers(mute, ends, t)}
              end={
                <Button
                  size="timeline"
                  busy={busy}
                  onPress={() => {
                    remove({
                      write: () => change(mute.account.id, "unmute"),
                      done: t("settings.relationships.done", { action: "unmute", name }),
                      failed: t("settings.relationships.failed", { action: "unmute", name }),
                    });
                  }}
                >
                  {t("settings.relationships.unmute")}
                </Button>
              }
            />
          </SettingsListItem>
        );
      }}
    />
  );
}

/** The muted servers: each its domain, with Unmute. */
function Servers({ capped, t }: { capped: boolean; t: T }) {
  const mute = useMuteDomain();
  return (
    <ListSection<string>
      list="servers"
      items={useMutedDomains()}
      keyOf={(domain) => domain}
      capped={capped}
      t={t}
      row={(domain, busy, remove) => (
        <SettingsListRow
          title={domain}
          end={
            <Button
              size="timeline"
              busy={busy}
              onPress={() => {
                remove({
                  write: () => mute(domain, false),
                  done: t("settings.relationships.done", { action: "unmute_server", name: domain }),
                  failed: t("settings.relationships.failed", { action: "unmute_server", name: domain }),
                });
              }}
            >
              {t("settings.relationships.unmute")}
            </Button>
          }
        />
      )}
    />
  );
}

/** The followed hashtags: each its name, with Unfollow. */
function Hashtags({ capped, t }: { capped: boolean; t: T }) {
  const follow = useFollowTag();
  return (
    <ListSection<FollowedHashtag>
      list="hashtags"
      items={useFollowedHashtagList()}
      keyOf={(tag) => tag.name.toLowerCase()}
      capped={capped}
      t={t}
      row={(tag, busy, remove) => {
        const name = `#${tag.name}`;
        return (
          <SettingsListRow
            icon="hashtag"
            title={tag.name}
            end={
              <Button
                size="timeline"
                busy={busy}
                onPress={() => {
                  remove({
                    write: () => follow(tag.name, false),
                    done: t("settings.relationships.done", { action: "unfollow", name }),
                    failed: t("settings.relationships.failed", { action: "unfollow", name }),
                  });
                }}
              >
                {t("settings.relationships.unfollow")}
              </Button>
            }
          />
        );
      }}
    />
  );
}

/** The section of `list`. */
function Section({ list, capped, t }: { list: RelationshipList; capped: boolean; t: T }) {
  switch (list) {
    case "blocked":
      return <Blocked capped={capped} t={t} />;
    case "muted":
      return <Muted capped={capped} t={t} />;
    case "servers":
      return <Servers capped={capped} t={t} />;
    case "hashtags":
      return <Hashtags capped={capped} t={t} />;
  }
}

/** Whether the read of every list is on its way, or every one failed, and the retry of them all. */
function useReads(): { pending: boolean; failed: boolean; retry: () => void } {
  const reads = [useBlocks(), useMutes(), useMutedDomains(), useFollowedHashtagList()];
  return {
    pending: reads.some((read) => read.status === "pending"),
    failed: reads.every((read) => read.status === "error"),
    retry: () => {
      for (const read of reads) read.refetch();
    },
  };
}

/** The entry's screen: the four lists, a few rows of each, once they are read. */
export function RelationshipSettings({ back }: SettingsEntryProps) {
  const { t } = stringsIn(useLocale().locale);
  const { pending, failed, retry } = useReads();
  const title = t("settings.entry.relationships");
  if (pending || failed)
    return (
      <SettingsScreen
        icon="members"
        title={title}
        back={back}
        notice={<ReadingNotice failed={!pending && failed} onRetry={retry} />}
      />
    );
  return (
    <SettingsScreen icon="members" title={title} back={back}>
      {relationshipLists.map((list) => (
        <Section key={list} list={list} capped t={t} />
      ))}
    </SettingsScreen>
  );
}

/** The title of the screen of the whole of `list`; undefined for a part of the address that names no list. */
export function useRelationshipListTitle(list: string): string | undefined {
  const { t } = stringsIn(useLocale().locale);
  return isRelationshipList(list) ? t("settings.relationships.list", { list }) : undefined;
}

/** The screen under the entry that holds the whole of `list`, a page at a time; `onBack` goes back to the entry. */
export function RelationshipListScreen({ list, onBack }: { list: string; onBack: () => void }) {
  const { t } = stringsIn(useLocale().locale);
  const back = <BackButton onPress={onBack}>{t("settings.entry.relationships")}</BackButton>;
  if (!isRelationshipList(list))
    return (
      <SettingsScreen
        sub
        title={t("settings.entry.relationships")}
        back={back}
        notice={
          <FeedNotice alert lead action={<TextLink href={entryPath}>{t("settings.entry.relationships")}</TextLink>}>
            {t("settings.not_found")}
          </FeedNotice>
        }
      />
    );
  return (
    <SettingsScreen sub title={t("settings.relationships.list", { list })} back={back}>
      <Section list={list} capped={false} t={t} />
    </SettingsScreen>
  );
}
