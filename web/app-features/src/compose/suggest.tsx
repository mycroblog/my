// SPDX-License-Identifier: AGPL-3.0-only

// The suggestions for the word being typed at the caret of a composer's field (ADR-0092 §1–§3): accounts for `@`,
// hashtags for `#`, emoji for `:`, each where the field reads it (§5: the text all three, the content warning `:`
// alone). A word opens them once it holds a character after its trigger (two after `:`), never while an input method
// composes; the accounts and the hashtags are asked of the server once typing has paused for 200 ms (the server puts
// the followed first, leaves the blocked and muted out, and ranks the hashtags by use), the hashtags posted lately on
// this device then first; the emoji come from the device, the custom ones and the Unicode ones of the picker (its own
// chunk, fetched as `:` is first typed), those used often first among matches as good. While they are asked for more
// than 300 ms a line says so, and another when none came; a failure closes them. Accepting one writes it over the word,
// with a space after it, as typing would; Escape closes them until the word changes.

import { AccountName, nameOf } from "@my/app-components";
import { useAccountSearch, useCustomEmojis, useTagSearch, type PickerCustomEmoji } from "@my/app-data";
import { SuggestionList, type PickerEmoji, type Suggestion } from "@my/design-system";
import type { EditorHandle, FieldCaret, FieldSuggest } from "@my/design-system/editor";
import { useEffect, useId, useState, type RefObject } from "react";
import { useFrequentEmojis } from "../emoji/device";
import { usePickerData, useUnicodeReady } from "../emoji/picker";
import { stringsIn } from "../locale";
import { recentTags } from "./recent-tags";
import { activeWord, type TokenTrigger } from "./text-tokens";

/** How many characters after its trigger a word needs before anything is suggested for it. */
const fewest: Readonly<Record<TokenTrigger, number>> = { "@": 1, "#": 1, ":": 2 };

/** How long typing pauses before the server is asked (ADR-0092 §1). */
const queryDelay = 200;

/** How long the suggestions may be on their way before a line says they are loading (ADR-0092 §3). */
const loadingDelay = 300;

/** How many suggestions show. */
const most = 8;

type Kind = "account" | "hashtag" | "emoji";
const kindOf: Readonly<Record<TokenTrigger, Kind>> = { "@": "account", "#": "hashtag", ":": "emoji" };

/** A suggestion: what its row shows, and what accepting it writes over the word. */
type Option = { shown: Suggestion; value: string };

/** A custom emoji as the picker offers it. */
const customOf = (e: PickerCustomEmoji): PickerEmoji => ({
  key: `:${e.shortcode}:`,
  name: `:${e.shortcode}:`,
  image: { url: e.url, staticUrl: e.static_url },
});

/** The custom emoji that match `query` while the Unicode ones are on their way: those it starts first. */
function customMatches(custom: readonly PickerCustomEmoji[], query: string): PickerEmoji[] {
  const q = query.toLowerCase();
  const words = (e: PickerCustomEmoji) => [e.shortcode, ...e.aliases].map((w) => w.toLowerCase());
  const starts = custom.filter((e) => words(e).some((w) => w.startsWith(q)));
  const holds = custom.filter((e) => !starts.includes(e) && words(e).some((w) => w.includes(q)));
  return [...starts, ...holds].map(customOf);
}

/** `tags` with the one named `q` first, then those posted lately on this device, the latest first, then the rest. */
function withRecentFirst<T extends { name: string }>(tags: readonly T[], q: string): T[] {
  const recent = recentTags().map((t) => t.toLowerCase());
  const rank = (t: T) => {
    const name = t.name.toLowerCase();
    if (name === q.toLowerCase()) return -1;
    const at = recent.indexOf(name);
    return at < 0 ? recent.length : at;
  };
  return tags.toSorted((a, b) => rank(a) - rank(b));
}

export type Suggestions = {
  /** What the field tells the suggestions. */
  caret: FieldCaret;
  /** The suggestions, while they are open. */
  suggest: FieldSuggest | undefined;
};

/**
 * The suggestions of the field `field`, whose value is `text`, for the words of `triggers` (ADR-0092 §5), in the
 * words of `locale`.
 */
export function useSuggestions({
  field,
  text,
  triggers,
  locale,
}: {
  field: RefObject<EditorHandle | null>;
  text: string;
  triggers: readonly TokenTrigger[];
  locale: string;
}): Suggestions {
  const { t } = stringsIn(locale);
  const listId = useId();
  const [caret, setCaret] = useState<number>();
  const [composing, setComposing] = useState(false);
  // The word the list was closed or accepted on, which opens it again once it changes.
  const [closed, setClosed] = useState<{ start: number; query: string }>();
  const found = caret === undefined || composing ? null : activeWord(text, caret);
  const word =
    found !== null &&
    triggers.includes(found.trigger) &&
    found.query.length >= fewest[found.trigger] &&
    !(closed?.start === found.start && closed.query === found.query)
      ? found
      : null;
  const trigger = word?.trigger;
  const wordKey = word === null ? undefined : `${word.trigger}${String(word.start)}`;
  const query = word?.query;

  // The server is asked for the word once typing has paused.
  const [asked, setAsked] = useState<{ key: string; query: string }>();
  useEffect(() => {
    if (trigger === undefined || trigger === ":" || wordKey === undefined || query === undefined) return;
    const timer = setTimeout(() => {
      setAsked({ key: wordKey, query });
    }, queryDelay);
    return () => {
      clearTimeout(timer);
    };
  }, [trigger, wordKey, query]);
  const askedQuery = asked !== undefined && asked.key === wordKey ? asked.query : undefined;
  const accounts = useAccountSearch(trigger === "@" ? askedQuery : undefined);
  const tags = useTagSearch(trigger === "#" ? askedQuery : undefined);

  // The emoji: the Unicode ones are fetched as a `:` word is first typed.
  const picker = usePickerData(locale);
  const ready = useUnicodeReady(locale);
  const custom = useCustomEmojis();
  const frequent = useFrequentEmojis();
  const wantsEmoji = trigger === ":";
  const { preload } = picker;
  useEffect(() => {
    if (wantsEmoji) preload();
  }, [wantsEmoji, preload]);

  let options: readonly Option[] | undefined;
  let failed = false;
  if (word !== null && trigger === "@") {
    failed = accounts.failed;
    options = accounts.items?.slice(0, most).map((a) => {
      const handle = `@${a.acct}`;
      return {
        shown: {
          kind: "account",
          name: <AccountName account={a} />,
          handle,
          avatar: a.avatar?.small.url,
          initialOf: nameOf(a),
          label: t("compose.suggest.account", { name: nameOf(a), handle }),
        },
        value: handle,
      };
    });
  } else if (word !== null && trigger === "#") {
    failed = tags.failed;
    options =
      tags.items === undefined
        ? undefined
        : withRecentFirst(tags.items, word.query)
            .slice(0, most)
            .map((tag) => ({
              shown: {
                kind: "hashtag",
                name: tag.name,
                uses: t("compose.suggest.uses", { count: tag.statuses_count }),
                label: t("compose.suggest.hashtag", { name: tag.name, count: tag.statuses_count }),
              },
              value: `#${tag.name}`,
            }));
  } else if (word !== null && trigger === ":") {
    const used = new Set(frequent);
    const emojis = ready ? picker.search(word.query, (e) => used.has(e.key)) : customMatches(custom, word.query);
    // Until the Unicode emoji arrive, the custom ones that match stand alone; with none, the list waits for them.
    options =
      !ready && emojis.length === 0
        ? undefined
        : emojis.slice(0, most).map((e) =>
            "image" in e
              ? { shown: { kind: "emoji", name: e.name, image: e.image.url }, value: e.key }
              : {
                  shown: { kind: "emoji", name: e.name, text: e.text },
                  value: (picker.tone > 0 ? e.tones?.[picker.tone - 1] : undefined) ?? e.text,
                },
          );
  }

  // A line says the suggestions are loading once they have been on their way for a moment.
  const pending = word !== null && options === undefined && !failed ? wordKey : undefined;
  const [slow, setSlow] = useState<string>();
  useEffect(() => {
    if (pending === undefined) return;
    const timer = setTimeout(() => {
      setSlow(pending);
    }, loadingDelay);
    return () => {
      clearTimeout(timer);
    };
  }, [pending]);
  const loading = pending !== undefined && slow === pending;

  // The first option is active as the list opens or its options change.
  const listKey = options === undefined ? undefined : `${wordKey ?? ""}:${options.map((o) => o.value).join(" ")}`;
  const [chosen, setChosen] = useState<{ key: string; index: number }>();
  const count = options?.length ?? 0;
  const active = chosen !== undefined && chosen.key === listKey ? Math.min(chosen.index, count - 1) : 0;

  const caretProps: FieldCaret = { onCaret: setCaret, onComposing: setComposing };
  if (word === null || trigger === undefined || failed || (options === undefined && !loading))
    return { caret: caretProps, suggest: undefined };
  const kind = kindOf[trigger];
  const accept = (index: number) => {
    const option = options?.[index];
    const editor = field.current;
    if (option === undefined || editor === null) return;
    // The word as accepted does not open the list again, where no space could follow it (before a line's end).
    setClosed({ start: word.start, query: option.value.slice(1) });
    // A space follows unless white space does; the caret goes after that space (not past a line's end).
    const next = text.charAt(word.end);
    const spaced = /\s/u.test(next);
    const insert = spaced ? option.value : `${option.value} `;
    const after = spaced && next !== "\n" && next !== "\r" ? 1 : 0;
    editor.write(word.start, word.end, insert, word.start + insert.length + after);
  };
  const status =
    options === undefined
      ? { kind: "loading" as const, text: t("compose.suggest.loading") }
      : count === 0
        ? { kind: "empty" as const, text: t("compose.suggest.none", { kind }) }
        : undefined;
  return {
    caret: caretProps,
    suggest: {
      at: word.end,
      list: (
        <SuggestionList
          id={listId}
          label={t("compose.suggest.list", { kind })}
          items={options?.map((o) => o.shown) ?? []}
          active={active}
          onChoose={accept}
          status={status}
        />
      ),
      listId: count > 0 ? listId : undefined,
      count,
      active,
      announce: status?.text ?? t("compose.suggest.count", { count }),
      onActive: (index) => {
        setChosen({ key: listKey ?? "", index });
      },
      onAccept: accept,
      onClose: () => {
        setClosed({ start: word.start, query: word.query });
      },
    },
  };
}
