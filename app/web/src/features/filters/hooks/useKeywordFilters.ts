// SPDX-License-Identifier: AGPL-3.0-only
// The user's keyword filters on their settings page (web-client/screens/settings-filters.md, ADR-0157): the list, the
// form that adds one or changes one in its row, and deleting one after asking. A form saves all its fields at once;
// the server matches the posts, so a save marks the posts on screen stale and they are fetched again. The hooks
// return no JSX.
import { useNotice } from "@my/ui";
import { useMutation, useQuery, useQueryClient, useSuspenseQuery } from "@tanstack/react-query";
import { type SyntheticEvent, useState } from "react";
import { useIntl } from "react-intl";
import { useApi } from "../../../lib/auth/SessionProvider";
import { errorMessage } from "../../../lib/errors";
import { useT } from "../../../lib/i18n";
import {
  cacheDeletedFilter,
  cacheSavedFilter,
  createKeywordFilterMutation,
  deleteKeywordFilterMutation,
  filterContexts,
  filterFailure,
  filterTitleLimit,
  filterTitleProblem,
  filterWordLimit,
  filterWordTooLong,
  instanceQueryOptions,
  type KeywordFilter,
  type KeywordFilterAction,
  type KeywordFilterContext,
  type KeywordFilterRequest,
  keywordFiltersQueryOptions,
  maxFilters,
  maxFilterWords,
  maxWordsInAll,
  updateKeywordFilterMutation,
  wordMatchingManualUrl,
} from "../api/filters";

/** A filter's name inside a sentence of the UI's language, isolated from it (FSI … PDI; web-client/README.md Y10). */
export function isolate(title: string): string {
  return `⁨${title}⁩`;
}

/** The limits of filters (ADR-0157 §9) as the messages take them: numbers formatted in the UI's locale ("1,000"). */
export function useFilterLimits(): { filters: string; words: string; wordsInAll: string; title: string; word: string } {
  const intl = useIntl();
  return {
    filters: intl.formatNumber(maxFilters),
    words: intl.formatNumber(maxFilterWords),
    wordsInAll: intl.formatNumber(maxWordsInAll),
    title: intl.formatNumber(filterTitleLimit),
    word: intl.formatNumber(filterWordLimit),
  };
}

/** The user's keyword filters, oldest first, which the route has loaded. */
export function useKeywordFilterList(): readonly KeywordFilter[] {
  const api = useApi();
  return useSuspenseQuery(keywordFiltersQueryOptions(api)).data.items;
}

/** The page of the user manual about how the words of filters match, at the server's version and in the UI's locale. */
export function useWordMatchingManual(): string {
  const api = useApi();
  const intl = useIntl();
  const instance = useQuery(instanceQueryOptions(api));
  return wordMatchingManualUrl(instance.data?.version, intl.locale);
}

/** The localized names of the places a filter applies. */
export function useContextLabels(): Readonly<Record<KeywordFilterContext, string>> {
  const t = useT();
  return {
    home: t("client:filters.context.home"),
    notifications: t("client:filters.context.notifications"),
    public: t("client:filters.context.public"),
    thread: t("client:filters.context.thread"),
    account: t("client:filters.context.account"),
  };
}

/** The ends a filter can be given from now, in seconds: 30 minutes, 1, 6, and 12 hours, 1 day, 1 week. */
export const expiryChoices: readonly number[] = [1800, 3600, 21_600, 43_200, 86_400, 604_800];

/** When the filter of a form ends: as it is (`keep`, for a filter that has an end), never, or a number of seconds from now. */
export type Expiry = "keep" | "never" | number;

/** A word of the form, with a key of its own so that its row keeps its state as rows come and go. */
type WordDraft = { key: string; keyword: string; wholeWord: boolean };

/** The form of a filter: adding one, or changing one in its row. Every field is saved at once by `submit`. */
export type FilterForm = {
  title: string;
  setTitle: (title: string) => void;
  words: readonly WordDraft[];
  changeWord: (key: string, change: Partial<Omit<WordDraft, "key">>) => void;
  addWord: () => void;
  removeWord: (key: string) => void;
  /** Whether another word fits in the filter (at most 100). */
  canAddWord: boolean;
  contexts: ReadonlySet<KeywordFilterContext>;
  toggleContext: (context: KeywordFilterContext, on: boolean) => void;
  action: KeywordFilterAction;
  setAction: (action: KeywordFilterAction) => void;
  expiry: Expiry;
  setExpiry: (expiry: Expiry) => void;
  /** The end the filter has, for the choice that keeps it; null for a new filter or one without an end. */
  currentEnd: { expired: boolean; date: string } | null;
  /** Why a typed word is refused (empty, too long), for its field; null when it may be sent. */
  wordProblem: (keyword: string) => string | null;
  /** Why a typed name is refused, for its field; null when it may be sent. */
  titleProblem: (title: string) => string | null;
  /** Sends the filter once the browser has checked the fields; `onSaved` is called once the list shows it. */
  submit: (event: SyntheticEvent<HTMLFormElement>, onSaved: () => void) => void;
  isPending: boolean;
  /** Why the filter cannot be saved, localized, for the form's action bar; undefined while nothing failed. */
  error: string | undefined;
};

/** Sending the request of a form: adding a filter or changing one. */
export type FilterSave = {
  /** Sends `request`; `onSaved` is called once the list shows the filter. */
  save: (request: KeywordFilterRequest, onSaved: () => void) => void;
  isPending: boolean;
  /** Why the last send failed, localized; undefined while nothing failed. */
  error: string | undefined;
  /** Forgets the last failure, once a field changes. */
  reset: () => void;
};

let wordKeys = 0;

/** A new word row with a key unique in the page. */
function wordDraft(keyword = "", wholeWord = false): WordDraft {
  wordKeys += 1;
  return { key: `word-${String(wordKeys)}`, keyword, wholeWord };
}

/**
 * The fields of the form of `filter` (a new filter when undefined), their checks, and the submit that sends them
 * through `saver`. The form's component mounts with the panel or the row's form and unmounts when it closes, so the
 * fields start again from the filter each time it opens.
 */
export function useFilterForm(filter: KeywordFilter | undefined, saver: FilterSave): FilterForm {
  const t = useT();
  const intl = useIntl();
  const limits = useFilterLimits();
  const [title, setTitle] = useState(filter?.title ?? "");
  const [words, setWords] = useState<readonly WordDraft[]>(() =>
    filter === undefined
      ? [wordDraft()]
      : filter.keywords.map((keyword) => wordDraft(keyword.keyword, keyword.whole_word)),
  );
  // A new filter applies everywhere at first: the user narrows it.
  const [contexts, setContexts] = useState<ReadonlySet<KeywordFilterContext>>(
    () => new Set(filter?.contexts ?? filterContexts),
  );
  const [action, setAction] = useState<KeywordFilterAction>(filter?.action ?? "warn");
  const [expiry, setExpiry] = useState<Expiry>(filter?.expires_at === undefined ? "never" : "keep");
  const [refused, setRefused] = useState<string | undefined>(undefined);
  const changed = () => {
    setRefused(undefined);
    saver.reset();
  };
  const titleProblem = (value: string): string | null => {
    switch (filterTitleProblem(value.trim())) {
      case "empty":
        return t("client:filters.form.title.required");
      case "too-long":
        return t("client:filters.form.title.too_long", { max: limits.title });
      case "invalid":
        return t("client:filters.form.title.invalid");
      case undefined:
        return null;
    }
  };
  /** The request of the fields, or the localized reason it cannot be sent. */
  const request = (): KeywordFilterRequest | string => {
    const value = title.trim();
    const titleRefused = titleProblem(value);
    if (titleRefused !== null) return titleRefused;
    const keywords = words
      .map((word) => ({ keyword: word.keyword.trim(), whole_word: word.wholeWord }))
      .filter((word) => word.keyword !== "");
    if (keywords.length === 0) return t("client:filters.form.word.required");
    if (contexts.size === 0) return t("client:filters.form.contexts.required");
    const base = {
      title: value,
      action,
      contexts: filterContexts.filter((context) => contexts.has(context)),
      keywords,
    };
    if (expiry === "never") return base;
    if (expiry === "keep") return filter?.expires_at === undefined ? base : { ...base, expires_at: filter.expires_at };
    // The end counts from the moment the user sends the form: an event, not a render (web-client/README.md R1).
    return { ...base, expires_at: new Date(Date.now() + expiry * 1000).toISOString() };
  };
  return {
    title,
    setTitle: (next) => {
      setTitle(next);
      changed();
    },
    words,
    changeWord: (key, change) => {
      setWords(words.map((word) => (word.key === key ? { ...word, ...change } : word)));
      changed();
    },
    addWord: () => {
      if (words.length < maxFilterWords) setWords([...words, wordDraft()]);
    },
    removeWord: (key) => {
      setWords(words.filter((word) => word.key !== key));
      changed();
    },
    canAddWord: words.length < maxFilterWords,
    contexts,
    toggleContext: (context, on) => {
      const next = new Set(contexts);
      if (on) next.add(context);
      else next.delete(context);
      setContexts(next);
      changed();
    },
    action,
    setAction: (next) => {
      setAction(next);
      changed();
    },
    expiry,
    setExpiry: (next) => {
      setExpiry(next);
      changed();
    },
    currentEnd:
      filter?.expires_at === undefined
        ? null
        : {
            expired: filter.expired,
            date: intl.formatDate(filter.expires_at, { dateStyle: "medium", timeStyle: "short" }),
          },
    wordProblem: (keyword) => {
      const value = keyword.trim();
      if (value === "") return t("client:filters.form.word.required");
      if (filterWordTooLong(value)) return t("client:filters.form.word.too_long", { max: limits.word });
      return null;
    },
    titleProblem,
    submit: (event, onSaved) => {
      event.preventDefault();
      if (saver.isPending) return;
      const next = request();
      if (typeof next === "string") {
        setRefused(next);
        return;
      }
      setRefused(undefined);
      saver.save(next, onSaved);
    },
    isPending: saver.isPending,
    error: refused ?? saver.error,
  };
}

/** The localized reason of a failed save of a filter. */
function useSaveError() {
  const t = useT();
  const limits = useFilterLimits();
  return (error: unknown): string => {
    switch (filterFailure(error)) {
      case "too-many-filters":
        return t("client:filters.limit", { max: limits.filters });
      case "too-many-words":
        return t("client:filters.form.too_many_words", { max: limits.wordsInAll });
      case "other":
        return t("client:filters.form.failed", { reason: errorMessage(error) });
    }
  };
}

/** Adding a filter, and what the last addition did, shown above the rows. */
export type CreateFilter = FilterSave & {
  /** Whether the user has the most filters, so that the add button gives way to a note. */
  isFull: boolean;
  /** What the last addition did; undefined before the first one. */
  status: string | undefined;
};

export function useCreateFilter(): CreateFilter {
  const api = useApi();
  const t = useT();
  const queryClient = useQueryClient();
  const filters = useKeywordFilterList();
  const failed = useSaveError();
  const [status, setStatus] = useState<string | undefined>(undefined);
  const create = useMutation({
    ...createKeywordFilterMutation(api),
    onSuccess: (filter) => cacheSavedFilter(api, queryClient, filter),
  });
  return {
    isFull: filters.length >= maxFilters,
    status,
    save: (request, onSaved) => {
      create.reset();
      setStatus(undefined);
      create.mutate(request, {
        onSuccess: (filter) => {
          setStatus(t("client:filters.added", { title: isolate(filter.title) }));
          onSaved();
        },
      });
    },
    isPending: create.isPending,
    error: create.isError ? failed(create.error) : undefined,
    reset: () => {
      if (create.isError) create.reset();
    },
  };
}

/** Changing `filter` from its row; what it did is announced. */
export function useUpdateFilter(filter: KeywordFilter): FilterSave {
  const api = useApi();
  const t = useT();
  const queryClient = useQueryClient();
  const notice = useNotice();
  const failed = useSaveError();
  const update = useMutation({
    ...updateKeywordFilterMutation(api),
    onSuccess: (saved) => cacheSavedFilter(api, queryClient, saved),
  });
  return {
    save: (request, onSaved) => {
      update.reset();
      update.mutate(
        { id: filter.id, body: request },
        {
          onSuccess: (saved) => {
            onSaved();
            notice.inPlace(t("client:filters.saved", { title: isolate(saved.title) }));
          },
        },
      );
    },
    isPending: update.isPending,
    error: update.isError ? failed(update.error) : undefined,
    reset: () => {
      if (update.isError) update.reset();
    },
  };
}

/** Deleting a filter, after the confirmation. */
export type DeleteFilter = {
  /** Deletes `filter`; `onDone(true)` is called once it is off the list, `onDone(false)` when it failed (a toast says why). */
  run: (filter: KeywordFilter, onDone: (deleted: boolean) => void) => void;
  /** The ID of the filter being deleted. */
  pendingId: string | undefined;
};

export function useDeleteFilter(): DeleteFilter {
  const api = useApi();
  const t = useT();
  const queryClient = useQueryClient();
  const notice = useNotice();
  const remove = useMutation({
    ...deleteKeywordFilterMutation(api),
    onSuccess: (filter) => cacheDeletedFilter(api, queryClient, filter),
  });
  return {
    pendingId: remove.isPending ? remove.variables.id : undefined,
    run: (filter, onDone) => {
      remove.mutate(filter, {
        onSuccess: () => {
          onDone(true);
          notice.inPlace(t("client:filters.deleted", { title: isolate(filter.title) }));
        },
        onError: (error) => {
          onDone(false);
          notice.toast({
            tone: "danger",
            message: t("client:filters.delete_failed", { title: isolate(filter.title), reason: errorMessage(error) }),
          });
        },
      });
    },
  };
}
