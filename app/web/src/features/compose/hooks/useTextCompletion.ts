// SPDX-License-Identifier: AGPL-3.0-only
// The completion of a text field (ADR-0092 §1–§3, §5): `@` accounts from GET /accounts/search, `#` hashtags from
// GET /tags/search with the ones used lately on this browser first, and `:` emoji from the data the client has. The
// word at the caret is found by lib/textTokens `activeWord`, under the server's rules. The server is asked once typing
// pauses for 200 ms, the emoji at once; a newer word cancels the request of the one before, and a failure (a 429 too)
// just closes the list (@my/ui TextAreaAutocomplete).
import type { TextAutocompleteSuggestion, TextAutocompleteWord } from "@my/ui";
import { useQueryClient } from "@tanstack/react-query";
import { useApi } from "../../../lib/auth/SessionProvider";
import { useFederation } from "../../../lib/federation";
import { useT } from "../../../lib/i18n";
import { recentTagsKey, useStorage } from "../../../lib/storage";
import { activeWord } from "../../../lib/textTokens";
import { type EmojiSuggestion, useEmojiSuggestions } from "../../emoji";
import { type Account, lookUpAccount, searchAccounts, searchTags } from "../api/completion";
import { rankTags, type TagChoice } from "./recentTags";

/** What a field completes (ADR-0092 §5): mentions, hashtags and emoji, or emoji only (the content warning, the display name). */
export type CompletionKinds = "all" | "emoji";

/** A suggestion of the completion: an account, a hashtag or an emoji. */
export type CompletionSuggestion = TextAutocompleteSuggestion &
  ({ kind: "account"; account: Account } | { kind: "tag"; tag: TagChoice } | { kind: "emoji"; emoji: EmojiSuggestion });

/** How many suggestions a list shows (the server's default `limit`, ADR-0092 §1). */
const maxSuggestions = 8;

/** The longest query the searches take (`q`, ADR-0092 §1). */
const maxQueryLength = 100;

/** How long typing must pause before the server is asked (ADR-0092 §3). */
const serverQueryDelay = 200;

/** The fewest characters after each trigger before suggestions are asked for (ADR-0092 §2, §3). */
const allTriggers: Readonly<Record<string, number>> = { "@": 1, "#": 1, ":": 2 };
const emojiTriggers: Readonly<Record<string, number>> = { ":": 2 };

/** A full handle of another server, `user@domain` with a dot in the domain, which the lookup may resolve. */
const fullHandle = /^[A-Za-z0-9_]+@[A-Za-z0-9-]+(?:\.[A-Za-z0-9-]+)+$/u;

function accountSuggestion(account: Account): CompletionSuggestion {
  const handle = `@${account.acct}`;
  return { kind: "account", id: `account:${account.id}`, value: handle, textValue: `${account.display_name || account.username} ${handle}`, account };
}

/** The props of @my/ui TextAreaAutocomplete / TextFieldAutocomplete for a field that completes `kinds`, but the rendering of an option. */
export function useTextCompletion(kinds: CompletionKinds) {
  const api = useApi();
  const queryClient = useQueryClient();
  const t = useT();
  const { followsRemote } = useFederation();
  const recentTags = useStorage(recentTagsKey);
  const emojiSuggestions = useEmojiSuggestions();

  const loadAccounts = async (query: string, signal: AbortSignal): Promise<CompletionSuggestion[]> => {
    if (query.length > maxQueryLength) return [];
    const found = await searchAccounts(api, queryClient, query, signal);
    let accounts = found;
    // ADR-0092 §1: the search resolves nothing, so a full handle it does not know is looked up once, and resolved
    // from its server unless the instance takes nothing in from other servers (ADR-0076).
    if (fullHandle.test(query) && !found.some((account) => account.acct.toLowerCase() === query.toLowerCase())) {
      const looked = await lookUpAccount(api, queryClient, query, followsRemote, signal);
      if (looked !== undefined) accounts = [looked, ...found.filter((account) => account.id !== looked.id)];
    }
    return accounts.slice(0, maxSuggestions).map(accountSuggestion);
  };

  const loadTags = async (query: string, signal: AbortSignal): Promise<CompletionSuggestion[]> => {
    if (query.length > maxQueryLength) return [];
    const found = await searchTags(api, queryClient, query, signal);
    return rankTags(query, found, recentTags, maxSuggestions).map((tag) => ({
      kind: "tag",
      id: `tag:${tag.name}`,
      value: `#${tag.name}`,
      textValue: `#${tag.name}`,
      tag,
    }));
  };

  const loadEmoji = async (query: string): Promise<CompletionSuggestion[]> =>
    (await emojiSuggestions(query)).map((emoji) => ({
      kind: "emoji",
      id: `emoji:${emoji.key}`,
      value: emoji.value,
      textValue: emoji.kind === "custom" ? emoji.value : emoji.emoji.name,
      emoji,
    }));

  return {
    triggers: kinds === "all" ? allTriggers : emojiTriggers,
    findWord: activeWord,
    queryDelay: (word: TextAutocompleteWord) => (word.trigger === ":" ? 0 : serverQueryDelay),
    loadSuggestions: (word: TextAutocompleteWord, signal: AbortSignal): Promise<CompletionSuggestion[]> => {
      switch (word.trigger) {
        case "@":
          return loadAccounts(word.query, signal);
        case "#":
          return loadTags(word.query, signal);
        default:
          return loadEmoji(word.query);
      }
    },
    suggestionsLabel: t("client:compose.completion.suggestions"),
    formatSuggestionCount: (count: number) => t("client:compose.completion.count", { count }),
    loadingLabel: t("client:compose.completion.loading"),
    emptyLabel: t("client:compose.completion.empty"),
  };
}
