// SPDX-License-Identifier: AGPL-3.0-only

// Relationships, the entry of the settings at /settings/relationships (FR-303, FR-304): the four lists of what the
// reader keeps between them and others, a section each, in this order: the accounts they block, the accounts they
// mute, the servers they mute, and the hashtags they follow. A reader comes here to undo one thing and leave, so the
// entry shows every list at once with its count once the whole of it is read: a list of seven rows or fewer whole, a
// longer one by its first five and the door to the whole list (a row that leads on, with its chevron), on a screen
// under the entry (/settings/relationships/<list>) that pages through it, its section named by what it lists, since
// the screen's title names the list. An account leads to its profile, and a muted one says what the mute covers (its
// notifications too, until when) where the server says it.
//
// The entry reads the four lists itself, once, and gives each to its section: it says that it loads until every one
// answered, and that the settings could not be read when none could. From then on its sections stay, and a list that
// failed says so in its own section, asked for again only by the reader's Try again there; so does the next page of
// a whole list, under the rows read so far.
//
// Each row is taken out of its list as the profile's and a post's menus undo the same: at once, with no confirmation
// (Unblock, Unmute, the server's Unmute, the hashtag's Unfollow), through app-data's writes, which read the list again.
// The row then keeps its place until the reader leaves the screen, its button gone and its result said in words in
// its place (Unblocked: a muted account's line of what the mute covers gives way to it), so that nothing moves under
// the finger and no second press undoes another row: which rows a cut list shows, and its door, stay as they were
// too. The focus goes to that result, named by the sentence of what was done. The heading's count follows the server.
// A write that failed says so in the section's status, a polite live region there from the start, and leaves the row
// and the focus as they were; the sentence goes as the next press on that row begins. An empty list says what it
// would hold and how something comes to be in it.

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
  FeedNotice,
  SettingsAccountItem,
  SettingsEmpty,
  SettingsList,
  SettingsListRow,
  SettingsScreen,
  SettingsSection,
  ShowMore,
  Stack,
  TextLink,
  useAddedFocus,
  useLocale,
  type SettingsRowResult,
} from "@my/design-system";
import { Fragment, useEffect, useId, useRef, useState, type ReactNode, type RefCallback } from "react";
import { stringsIn } from "../locale";
import type { SettingsEntryProps } from "./entries";
import { ReadingNotice, useMoment } from "./parts";

type T = ReturnType<typeof stringsIn>["t"];

/** The lists of the entry, in the order of its sections. */
const relationshipLists = ["blocked", "muted", "servers", "hashtags"] as const;
type RelationshipList = (typeof relationshipLists)[number];

/** The address of the entry's screen. */
const entryPath = "/settings/relationships";

/** The address of the screen under the entry that holds the whole of `list`. */
const relationshipListPath = (list: RelationshipList): string => `${entryPath}/${list}`;

/** Whether `path`, the last part of an address under the entry, names one of its lists. */
const isRelationshipList = (path: string): path is RelationshipList => relationshipLists.some((list) => list === path);

/**
 * How the entry shows a list: whole while it has `most` rows at most, else its first `few` and the door to the whole
 * of it, which then hides three rows or more: a door is worth its press only past a row or two.
 */
const most = 7;
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

/**
 * What takes a row out of its list: the write; the row's words once it is done (`shown`: Unblocked) and the sentence
 * read in their place (`done`: You unblocked @kai.); and what the section's status says when it is not (`failed`).
 */
type Removal = {
  readonly write: () => Promise<void>;
  readonly shown: string;
  readonly done: string;
  readonly failed: string;
};

/** What a row is given: whether its removal is on its way, what came of it once done, and what runs it. */
type RowState = {
  readonly busy: boolean;
  readonly result: SettingsRowResult | undefined;
  readonly remove: (removal: Removal) => void;
  /** The ref of its row, for a list that grows under Show more. */
  readonly itemRef: RefCallback<HTMLElement> | undefined;
};

/**
 * The removals of a section's rows. `remove` runs one at a time for a row (`busy` tells which are on their way). A
 * row taken out keeps its place among the rows until the reader leaves the screen (`kept`: the item and where it
 * stood), its actions gone and its result said in their place, so that nothing moves under the finger, and its
 * result takes the focus, where its button was; its sentence is that place's name, read once. A removal that failed
 * says so in `status`, a polite live region there from the start, the row and the focus left as they were; what it
 * says of a row is gone as the next removal of that row begins, which then says what came of it.
 */
export function useRemovals<I>(id: string) {
  const [said, setSaid] = useState<{ readonly key: string; readonly text: string; readonly n: number } | undefined>(
    undefined,
  );
  const [busy, setBusy] = useState<ReadonlySet<string>>(new Set());
  const [kept, setKept] = useState<ReadonlyMap<string, { item: I; at: number; result: SettingsRowResult }>>(new Map());
  const count = useRef(0);
  const focus = useRef<string | undefined>(undefined);
  // The result takes the focus once it is drawn: a row whose list has not yet come from the cache without it (the
  // server's answer reaches the screen after the write resolves) says nothing yet, and the focus waits for it.
  useEffect(() => {
    if (focus.current === undefined) return;
    const place = document.getElementById(focus.current);
    if (place === null) return;
    place.focus();
    focus.current = undefined;
  });
  const resultId = (key: string) => `${id}-done-${key}`;
  const remove = (key: string, item: I, at: number, { write, shown, done, failed }: Removal) => {
    if (busy.has(key)) return;
    setBusy((now) => new Set(now).add(key));
    setSaid((was) => (was?.key === key ? undefined : was));
    write()
      .then(
        () => {
          const result = { id: resultId(key), shown, spoken: done };
          setKept((now) => new Map(now).set(key, { item, at, result }));
          focus.current = result.id;
        },
        () => {
          count.current += 1;
          setSaid({ key, text: failed, n: count.current });
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
    <Box as="p" role="status" text="caption" tone="muted">
      {said !== undefined && <span key={said.n}>{said.text}</span>}
    </Box>
  );
  return { busy, kept, remove, status };
}

/** What a section of a list shows of it, and how. */
type ListOf<I> = {
  readonly list: RelationshipList;
  readonly items: PagedList<I>;
  readonly keyOf: (item: I) => string;
  /** A row of the list. */
  readonly row: (item: I, state: RowState) => ReactNode;
};

/**
 * A list's section: its rows, those the reader took out where they stood, or what stands in their place (that it
 * loads, that it failed, that it is empty), the status of its removals, and at the end the door to the whole list
 * (`capped`, on the entry) or the next page, which says under the rows read so far that it could not be read, its
 * button then the way to ask again. On the entry its heading is the list's name with how many it holds, as the
 * server has it once the whole list is read; on the screen of the whole list, whose title is that name, the heading
 * is the count's sentence alone (9 servers). The door and the rows a cut list shows are decided over the server's
 * rows with those taken out put back, so that taking one out changes neither.
 */
function ListSection<I>({ list, items, keyOf, row, capped, t }: ListOf<I> & { capped: boolean; t: T }) {
  const { lang } = stringsIn(useLocale().locale);
  const id = useId();
  const { busy, kept, remove, status } = useRemovals<I>(id);
  // A next page that failed leaves the pages before it, under a status of `error`: the list is read all the same.
  const read = items.status === "success" || items.failedNextPage;
  const whole = read && !items.hasNextPage;
  const count = items.items.length;
  const heading = capped
    ? t("settings.relationships.list", { list })
    : read
      ? t("settings.relationships.count", { list, count, more: whole ? "no" : "yes" })
      : t("settings.relationships.list", { list });
  // The rows the server lists, with each the reader took out put back where it stood.
  const listed: I[] = [...items.items];
  const taken = [...kept].sort((a, b) => a[1].at - b[1].at);
  for (const entry of taken) {
    const [key, { item, at }] = entry;
    if (!listed.some((i) => keyOf(i) === key)) listed.splice(Math.min(at, listed.length), 0, item);
  }
  const door = capped && (listed.length > most || items.hasNextPage);
  const shown = door ? listed.slice(0, few) : listed;
  const added = useAddedFocus(shown.length);
  const retry = (onPress: () => void) => (
    <Button size="timeline" touch onPress={onPress}>
      {t("settings.retry")}
    </Button>
  );
  const rows =
    items.status === "pending" ? (
      <SettingsEmpty>{t("settings.relationships.loading")}</SettingsEmpty>
    ) : !read ? (
      <SettingsEmpty action={retry(items.refetch)}>{t("settings.relationships.failed_list")}</SettingsEmpty>
    ) : shown.length === 0 ? (
      <SettingsEmpty>{t("settings.relationships.empty", { list })}</SettingsEmpty>
    ) : (
      <SettingsList label={heading}>
        {shown.map((item, at) => {
          const key = keyOf(item);
          return (
            <Fragment key={key}>
              {row(item, {
                busy: busy.has(key),
                result: kept.get(key)?.result,
                itemRef: capped ? undefined : added.item(at),
                remove: (removal) => {
                  remove(key, item, at, removal);
                },
              })}
            </Fragment>
          );
        })}
      </SettingsList>
    );
  const all = t("settings.relationships.all", { list });
  const more = capped ? (
    door && (
      <SettingsList label={all}>
        <SettingsListRow title={all} href={relationshipListPath(list)} />
      </SettingsList>
    )
  ) : (
    <ShowMore
      more={items.hasNextPage}
      busy={items.isFetchingNextPage}
      failed={items.failedNextPage}
      onPress={added.press(items.fetchNextPage)}
    />
  );
  return (
    <SettingsSection
      heading={heading}
      count={capped && whole && count > 0 ? new Intl.NumberFormat(lang).format(count) : undefined}
    >
      <Stack>
        {rows}
        {status}
      </Stack>
      {more}
    </SettingsSection>
  );
}

/** The button that takes a row out of its list, its name `label`, with what it runs. */
function RemoveButton({ label, state, removal }: { label: string; state: RowState; removal: Removal }) {
  return (
    <Button
      size="timeline"
      touch
      busy={state.busy}
      onPress={() => {
        state.remove(removal);
      }}
    >
      {label}
    </Button>
  );
}

/** What the rows of one kind of removal say: its button, its words once done, its sentences. */
function removalOf(
  action: "unblock" | "unmute" | "unmute_server" | "unfollow",
  name: string,
  write: () => Promise<void>,
  t: T,
): Removal {
  return {
    write,
    shown: t("settings.relationships.result", { action }),
    done: t("settings.relationships.done", { action, name }),
    failed: t("settings.relationships.failed", { action, name }),
  };
}

/** What a section is given: its list as it was read, whether the entry cuts it short, and the strings. */
type SectionProps<I> = { items: PagedList<I>; capped: boolean; t: T };

/** The blocked accounts: each its row, leading to its profile, with Unblock. */
function Blocked({ items, capped, t }: SectionProps<Account>) {
  const change = useChangeRelationship();
  return (
    <ListSection<Account>
      list="blocked"
      items={items}
      keyOf={(account) => account.id}
      capped={capped}
      t={t}
      row={(account, state) => {
        const name = sentenceHandleOf(account);
        return (
          <SettingsAccountItem
            account={<AccountLine account={account} />}
            label={name}
            result={state.result}
            itemRef={state.itemRef}
            end={
              <RemoveButton
                label={t("settings.relationships.unblock")}
                state={state}
                removal={removalOf("unblock", name, () => change(account.id, "unblock"), t)}
              />
            }
          />
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
function Muted({ items, capped, t }: SectionProps<MutedAccount>) {
  const change = useChangeRelationship();
  const ends = useMoment();
  return (
    <ListSection<MutedAccount>
      list="muted"
      items={items}
      keyOf={(mute) => mute.account.id}
      capped={capped}
      t={t}
      row={(mute, state) => {
        const name = sentenceHandleOf(mute.account);
        return (
          <SettingsAccountItem
            account={<AccountLine account={mute.account} />}
            label={name}
            note={muteCovers(mute, ends, t)}
            result={state.result}
            itemRef={state.itemRef}
            end={
              <RemoveButton
                label={t("settings.relationships.unmute")}
                state={state}
                removal={removalOf("unmute", name, () => change(mute.account.id, "unmute"), t)}
              />
            }
          />
        );
      }}
    />
  );
}

/** The muted servers: each its domain, with Unmute. */
function Servers({ items, capped, t }: SectionProps<string>) {
  const mute = useMuteDomain();
  return (
    <ListSection<string>
      list="servers"
      items={items}
      keyOf={(domain) => domain}
      capped={capped}
      t={t}
      row={(domain, state) => (
        <SettingsListRow
          title={domain}
          result={state.result}
          itemRef={state.itemRef}
          end={
            <RemoveButton
              label={t("settings.relationships.unmute")}
              state={state}
              removal={removalOf("unmute_server", domain, () => mute(domain, false), t)}
            />
          }
        />
      )}
    />
  );
}

/** The followed hashtags: each its name, with Unfollow. */
function Hashtags({ items, capped, t }: SectionProps<FollowedHashtag>) {
  const follow = useFollowTag();
  return (
    <ListSection<FollowedHashtag>
      list="hashtags"
      items={items}
      keyOf={(tag) => tag.name.toLowerCase()}
      capped={capped}
      t={t}
      row={(tag, state) => (
        <SettingsListRow
          icon="hashtag"
          title={tag.name}
          result={state.result}
          itemRef={state.itemRef}
          end={
            <RemoveButton
              label={t("settings.relationships.unfollow")}
              state={state}
              removal={removalOf("unfollow", `#${tag.name}`, () => follow(tag.name, false), t)}
            />
          }
        />
      )}
    />
  );
}

/** The whole of each list, read by the screen that shows it alone. */
function AllBlocked({ t }: { t: T }) {
  return <Blocked items={useBlocks()} capped={false} t={t} />;
}
function AllMuted({ t }: { t: T }) {
  return <Muted items={useMutes()} capped={false} t={t} />;
}
function AllServers({ t }: { t: T }) {
  return <Servers items={useMutedDomains()} capped={false} t={t} />;
}
function AllHashtags({ t }: { t: T }) {
  return <Hashtags items={useFollowedHashtagList()} capped={false} t={t} />;
}

/** The section of the whole of `list`. */
function Whole({ list, t }: { list: RelationshipList; t: T }) {
  switch (list) {
    case "blocked":
      return <AllBlocked t={t} />;
    case "muted":
      return <AllMuted t={t} />;
    case "servers":
      return <AllServers t={t} />;
    case "hashtags":
      return <AllHashtags t={t} />;
  }
}

/**
 * The entry's screen: the four lists, a few rows of each. It reads them here, once, for its sections: until every one
 * answered it says that it loads, and when none could be read, that, with the retry of them all. Once its sections
 * show they stay, whatever a list does then (read again after a row is taken out, or at the reader's Try again):
 * each says in its own place that it loads or failed, and what the reader did in the others stands. A section that
 * read its list itself would ask for a failed one again as it is drawn, and the screen, back at loading, would draw
 * it again without end.
 */
export function RelationshipSettings({ back }: SettingsEntryProps) {
  const { t } = stringsIn(useLocale().locale);
  const blocked = useBlocks();
  const muted = useMutes();
  const servers = useMutedDomains();
  const hashtags = useFollowedHashtagList();
  const reads = [blocked, muted, servers, hashtags];
  const pending = reads.some((read) => read.status === "pending");
  const failed = reads.every((read) => read.status === "error");
  const [opened, setOpened] = useState(false);
  const open = opened || (!pending && !failed);
  if (open && !opened) setOpened(true);
  const title = t("settings.entry.relationships");
  if (!open)
    return (
      <SettingsScreen
        icon="members"
        title={title}
        back={back}
        notice={
          <ReadingNotice
            status={!pending ? "failed" : "loading"}
            retry={() => {
              for (const read of reads) read.refetch();
            }}
          />
        }
      />
    );
  return (
    <SettingsScreen icon="members" title={title} back={back}>
      <Blocked items={blocked} capped t={t} />
      <Muted items={muted} capped t={t} />
      <Servers items={servers} capped t={t} />
      <Hashtags items={hashtags} capped t={t} />
    </SettingsScreen>
  );
}

/**
 * The title of the screen at `list`, the last part of an address under the entry: the list's name, or the entry's for
 * a part that names no list, whose screen says so under it.
 */
export function useRelationshipListTitle(list: string): string {
  const { t } = stringsIn(useLocale().locale);
  return isRelationshipList(list) ? t("settings.relationships.list", { list }) : t("settings.entry.relationships");
}

/** The screen under the entry that holds the whole of `list`, a page at a time; `onBack` goes back to the entry. */
export function RelationshipListScreen({ list, onBack }: { list: string; onBack: () => void }) {
  const { t } = stringsIn(useLocale().locale);
  const title = useRelationshipListTitle(list);
  const back = <BackButton onPress={onBack}>{t("settings.entry.relationships")}</BackButton>;
  if (!isRelationshipList(list))
    return (
      <SettingsScreen
        sub
        title={title}
        back={back}
        notice={
          <FeedNotice alert lead action={<TextLink href={entryPath}>{t("settings.entry.relationships")}</TextLink>}>
            {t("settings.not_found")}
          </FeedNotice>
        }
      />
    );
  return (
    <SettingsScreen sub title={title} back={back}>
      <Whole list={list} t={t} />
    </SettingsScreen>
  );
}
