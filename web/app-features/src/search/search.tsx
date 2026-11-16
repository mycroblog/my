// SPDX-License-Identifier: AGPL-3.0-only

// Search (FR-305–FR-307, FR-309, ADR-0092 §1, ADR-0149 §9): a field that finds as the reader writes (the design
// system's SearchField), what it finds under it. People (accounts by the start of a handle or a word of their name,
// and this server's by any part of them), each a row with its follow; a remote account named by `@user@host` or by its
// address, looked up on its server and put first, the field busy while it is fetched; hashtags by their start, each a
// row that opens its timeline with how many posts carry it; and posts by their words, only where the server has a
// search engine, nothing of them shown elsewhere. The kinds that found something stand in sections one under another,
// each under its caption. One line says that the search runs, that it failed (Try again asks again what failed), that
// the account named is not there, or that nothing was found. Before anything is written, the suggested people show
// under the field, finding people's one place, where the server suggests anyone.

import { Column } from "@my/app-components";
import { useSearch, usePostSearchOffered, type SearchResults } from "@my/app-data";
import {
  FeedNotice,
  OpenRow,
  ResultGroup,
  RowItem,
  RowList,
  ScreenStack,
  SearchField,
  TextButton,
  useLocale,
} from "@my/design-system";
import { useEffect, useId, useState } from "react";
import { stringsIn } from "../locale";
import { AccountRows, SuggestedPeople } from "../people/people";
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

/** The hashtags found, each opening its timeline with how many posts carry it. */
function Hashtags({ results, t }: { results: SearchResults; t: T }) {
  return (
    <RowList label={t("search.hashtags")}>
      {results.tags.items.map((tag) => (
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
    />
  );
}

/** One line that stands for the whole search: that it runs, failed, found no account at the address, or nothing. */
function SearchLine({ q, results, t }: { q: string; results: SearchResults; t: T }) {
  const { status, remote } = results;
  if (status === "pending")
    return (
      <ScreenStack busy>
        <FeedNotice start>
          {remote === undefined ? t("search.searching") : t("search.looking_up", { handle: written(remote.acct) })}
        </FeedNotice>
      </ScreenStack>
    );
  if (status === "error")
    return (
      <FeedNotice start alert action={<TextButton onPress={results.retry}>{t("search.retry")}</TextButton>}>
        {t("search.failed")}
      </FeedNotice>
    );
  if (remote !== undefined && !remote.found && results.accounts.items.length === 0)
    return <FeedNotice start>{t("search.remote_none", { handle: written(remote.acct) })}</FeedNotice>;
  return <FeedNotice start>{t("search.none", { q })}</FeedNotice>;
}

/** A remote account as people write it: `@user@host` for a handle, an address as it is. */
function written(acct: string): string {
  return /^https?:\/\//iu.test(acct) ? acct : `@${acct}`;
}

/** What the words found: the kinds that found something, in sections one under another; else one line. */
function Results({ q, results, t }: { q: string; results: SearchResults; t: T }) {
  const counts: Record<Kind, number> = {
    people: results.accounts.items.length,
    hashtags: results.tags.items.length,
    posts: results.posts?.items.length ?? 0,
  };
  const kinds = (["people", "hashtags", "posts"] as const).filter((kind) => counts[kind] > 0);
  if (results.status !== "success" || kinds.length === 0) return <SearchLine q={q} results={results} t={t} />;
  const name = { people: t("search.people"), hashtags: t("search.hashtags"), posts: t("search.posts") };
  const body = (kind: Kind) =>
    kind === "people" ? (
      <AccountRows label={name.people} accounts={results.accounts.items} />
    ) : kind === "hashtags" ? (
      <Hashtags results={results} t={t} />
    ) : (
      <Posts results={results} t={t} />
    );
  return (
    <>
      {kinds.map((kind) => (
        <ResultGroup key={kind} title={name[kind]}>
          {body(kind)}
        </ResultGroup>
      ))}
    </>
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
  const results = useSearch(settled);
  useEffect(() => {
    if (settled !== q.trim()) onQuery(settled);
  }, [settled, q, onQuery]);
  return (
    <ScreenStack>
      <SearchField
        id={id}
        label={t("search.field")}
        hint={t("search.hint", { posts: posts ? "yes" : "no" })}
        defaultValue={q}
        autoFocus={q === ""}
        onChange={setWords}
        busy={results.remote?.status === "pending"}
        clearLabel={t("search.clear")}
      />
      {settled === "" ? <SuggestedPeople /> : <Results q={settled} results={results} t={t} />}
    </ScreenStack>
  );
}
