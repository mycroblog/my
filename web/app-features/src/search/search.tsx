// SPDX-License-Identifier: AGPL-3.0-only

// Search (FR-305–FR-307, FR-309, ADR-0092 §1, ADR-0149 §9): a field that finds as the reader writes, what it finds
// under it. People (accounts by the start of a handle or a word of their name, and this server's by any part of
// them), each a row with its follow; a remote account named by `@user@host` or by its address, looked up on its
// server and put first, the field saying while it is fetched and when none is there; hashtags by their start, each a
// row that opens its timeline with how many posts carry it; and posts by their words, only where the server has a
// search engine, nothing of them shown elsewhere. The kinds stand in sections one under another or in tabs
// (`DiscoveryLayout`). Before anything is written, the field says what it finds, over the suggested people where
// finding people is reached from here. Each kind says that it loads, that it failed with Try again, or that it found
// none; one line says when nothing at all was found.

import { Column } from "@my/app-components";
import { useSearch, usePostSearchOffered, type SearchResults } from "@my/app-data";
import {
  FeedNotice,
  OpenRow,
  ResultGroup,
  RowItem,
  RowList,
  ScreenStack,
  TabRow,
  TextButton,
  TextField,
  useLocale,
} from "@my/design-system";
import { useEffect, useId, useState, type ReactNode } from "react";
import { stringsIn } from "../locale";
import { AccountRows, SuggestedPeople, useDiscoveryLayout } from "../people/people";
import { usePostActions, usePostOwn, usePostPicker, usePostVoting } from "../post-actions";
import { TranslatablePost } from "../post-translation";

type T = ReturnType<typeof stringsIn>["t"];

/** How long the words must stand before they are searched (ADR-0092 §3: 200 ms of a pause). */
const pause = 300;

/** The words, once they stood still for a moment. */
function useSettled(words: string): string {
  const [settled, setSettled] = useState(words);
  useEffect(() => {
    const timer = setTimeout(() => {
      setSettled(words);
    }, pause);
    return () => {
      clearTimeout(timer);
    };
  }, [words]);
  return settled;
}

type Kind = "people" | "hashtags" | "posts";

/** What a kind says in place of its results while they load, when they failed, or when there are none. */
function Notice({
  status,
  none,
  refetch,
  t,
}: {
  status: "pending" | "error" | "success";
  none: string;
  refetch: () => void;
  t: T;
}): ReactNode {
  if (status === "pending") return <FeedNotice start>{t("search.searching")}</FeedNotice>;
  if (status === "error")
    return (
      <FeedNotice start alert action={<TextButton onPress={refetch}>{t("search.retry")}</TextButton>}>
        {t("search.failed")}
      </FeedNotice>
    );
  return <FeedNotice start>{none}</FeedNotice>;
}

/** The people found, the remote one named by the words first, or what stands in their place. */
function People({ results, t }: { results: SearchResults; t: T }) {
  const { accounts, remote } = results;
  if (accounts.items.length > 0) return <AccountRows label={t("search.people")} accounts={accounts.items} t={t} />;
  if (remote?.status === "pending")
    return <FeedNotice start>{t("search.looking_up", { handle: remote.acct })}</FeedNotice>;
  if (remote !== undefined && remote.status === "success" && !remote.found && accounts.status !== "error")
    return <FeedNotice start>{t("search.remote_none", { handle: remote.acct })}</FeedNotice>;
  return <Notice status={accounts.status} none={t("search.people_none")} refetch={accounts.refetch} t={t} />;
}

/** The hashtags found, each opening its timeline, or what stands in their place. */
function Hashtags({ results, t }: { results: SearchResults; t: T }) {
  const { tags } = results;
  if (tags.items.length === 0)
    return <Notice status={tags.status} none={t("search.hashtags_none")} refetch={tags.refetch} t={t} />;
  return (
    <RowList label={t("search.hashtags")}>
      {tags.items.map((tag) => (
        <RowItem key={tag.name}>
          <OpenRow
            href={`/tags/${encodeURIComponent(tag.name)}`}
            icon="hashtag"
            title={`#${tag.name}`}
            hint={t("search.tag_posts", { count: tag.statuses_count })}
          />
        </RowItem>
      ))}
    </RowList>
  );
}

/** The posts found, a page at a time in the column every timeline has. */
function Posts({ results, t }: { results: SearchResults; t: T }) {
  const onAction = usePostActions();
  const votingOf = usePostVoting();
  const ownOf = usePostOwn();
  const picker = usePostPicker();
  if (results.posts === undefined) return null;
  const { items, ...posts } = results.posts;
  return (
    <Column
      label={t("search.posts")}
      statuses={items}
      {...posts}
      onAction={onAction}
      postProps={(s) => ({ voting: votingOf(s), own: ownOf(s), picker })}
      postAs={TranslatablePost}
      empty={<FeedNotice start>{t("search.posts_none")}</FeedNotice>}
    />
  );
}

/** Whether the words found nothing of any kind, every kind having answered. */
function foundNothing(results: SearchResults): boolean {
  const { accounts, tags, posts, remote } = results;
  const tagsDone = remote !== undefined || (tags.status === "success" && tags.items.length === 0);
  const postsDone = posts === undefined || (posts.status === "success" && posts.items.length === 0);
  const remoteDone = remote === undefined || remote.status !== "pending";
  return accounts.status === "success" && accounts.items.length === 0 && tagsDone && postsDone && remoteDone;
}

/** What the words find, in sections one under another or in tabs. */
function Results({ q, t }: { q: string; t: T }) {
  const { results: layout } = useDiscoveryLayout();
  const results = useSearch(q);
  const [tab, setTab] = useState<Kind>("people");
  // A handle or an address names one account: only people are searched for it.
  const kinds: readonly Kind[] =
    results.remote !== undefined
      ? ["people"]
      : results.posts === undefined
        ? ["people", "hashtags"]
        : ["people", "hashtags", "posts"];
  const name = { people: t("search.people"), hashtags: t("search.hashtags"), posts: t("search.posts") };
  const body = (kind: Kind) =>
    kind === "people" ? (
      <People results={results} t={t} />
    ) : kind === "hashtags" ? (
      <Hashtags results={results} t={t} />
    ) : (
      <Posts results={results} t={t} />
    );
  // Busy while any kind is still asked for, so that assistive technology (and a story) waits for all of them.
  const busy =
    results.accounts.status === "pending" ||
    (results.remote === undefined && results.tags.status === "pending") ||
    results.remote?.status === "pending" ||
    results.posts?.status === "pending";
  if (foundNothing(results)) return <FeedNotice start>{t("search.none", { q })}</FeedNotice>;
  if (layout === "tabs" && kinds.length > 1)
    return (
      <ScreenStack busy={busy}>
        <TabRow
          label={t("search.kinds")}
          tabs={kinds.map((id) => ({ id, name: name[id] }))}
          current={kinds.includes(tab) ? tab : "people"}
          onChange={setTab}
        >
          {body(kinds.includes(tab) ? tab : "people")}
        </TabRow>
      </ScreenStack>
    );
  return (
    <ScreenStack busy={busy}>
      {kinds.map((kind) => (
        <ResultGroup key={kind} title={name[kind]}>
          {body(kind)}
        </ResultGroup>
      ))}
    </ScreenStack>
  );
}

/**
 * The search screen: the field, written with `q` (the words of the address), then what they find; `onQuery` puts
 * the words the reader settled on into the address, so that back returns to them.
 */
export function Search({ q, onQuery }: { q: string; onQuery: (q: string) => void }) {
  const { t } = stringsIn(useLocale().locale);
  const id = useId();
  const [words, setWords] = useState(q);
  const settled = useSettled(words.trim());
  const posts = usePostSearchOffered() === true;
  const { people } = useDiscoveryLayout();
  useEffect(() => {
    if (settled !== q.trim()) onQuery(settled);
  }, [settled, q, onQuery]);
  return (
    <ScreenStack>
      <TextField
        id={id}
        name="q"
        type="search"
        label={t("search.field")}
        hint={t("search.hint", { posts: posts ? "yes" : "no" })}
        defaultValue={q}
        autoComplete="off"
        autoFocus={q === ""}
        onChange={setWords}
      />
      {settled === "" ? people !== "menu" && <SuggestedPeople /> : <Results q={settled} t={t} />}
    </ScreenStack>
  );
}
