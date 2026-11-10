// SPDX-License-Identifier: AGPL-3.0-only
// The featured hashtags being edited in the profile form (ADR-0006 §17; web-client/screens/settings-profile.md): a row
// per hashtag in the order the profile shows them, moved one place at a time, and saving them with the profile. The
// form owns when it saves (its one save button); this hook holds the rows and the request.
import { useNotice } from "@my/ui";
import { useMutation, useQueryClient, useSuspenseQuery } from "@tanstack/react-query";
import { useState } from "react";
import { useApi } from "../../../lib/auth/SessionProvider";
import { errorMessage } from "../../../lib/errors";
import { useT } from "../../../lib/i18n";
import {
  featuredTagsQueryOptions,
  hashtagFailure,
  hashtagInput,
  hashtagKey,
  isHashtagName,
  maxFeaturedTags,
  setFeaturedTagsMutation,
  storeFeaturedTags,
} from "../api/tags";
import { hashtagInText } from "./text";

/** One featured hashtag being edited; `key` identifies its row while rows are added, moved and removed. */
export type FeaturedTagDraft = { key: number; name: string };

/** The featured hashtags of the profile form. */
export type FeaturedTagsDraft = {
  rows: FeaturedTagDraft[];
  canAdd: boolean;
  add: () => void;
  update: (key: number, name: string) => void;
  remove: (key: number) => void;
  /** Moves the row `key` one place up (-1) or down (1); the move is announced. */
  move: (key: number, step: -1 | 1) => void;
  /** Why a row's text is no hashtag, for the field's own check; null when it is one or empty. */
  validate: (name: string) => string | null;
  /** Whether the hashtags differ from the saved ones, in names or in order. */
  hasChanges: boolean;
  /** Saves the hashtags; rejects when the server refuses them, which `error` then says. */
  save: () => Promise<void>;
  /** Forgets the result of the last save, before the form saves again. */
  reset: () => void;
  isSaving: boolean;
  isSaved: boolean;
  /** Why the last save failed, localized; undefined when it did not. */
  error: string | undefined;
};

/**
 * The featured hashtags of the viewer `accountId`, as the route has loaded them, being edited. Mount it again (with
 * the form's `key`) to start over from newer saved hashtags.
 */
export function useFeaturedTagsDraft(accountId: string): FeaturedTagsDraft {
  const api = useApi();
  const t = useT();
  const notice = useNotice();
  const queryClient = useQueryClient();
  const { data } = useSuspenseQuery(featuredTagsQueryOptions(api));
  const [rows, setRows] = useState<FeaturedTagDraft[]>(() => data.items.map((tag, key) => ({ key, name: tag.name })));
  const [nextKey, setNextKey] = useState(data.items.length);
  const mutation = useMutation({
    ...setFeaturedTagsMutation(api),
    onSuccess: (saved) => {
      // The profile shows the new hashtags from its first render (web-client/README.md S1).
      storeFeaturedTags(queryClient, api, accountId, saved);
    },
  });
  // A row left empty is not a hashtag; the `#` the user may have typed is not part of its name.
  const names = rows.map((row) => hashtagInput(row.name)).filter((name) => name !== "");
  const saved = data.items.map((tag) => hashtagKey(tag.name));
  const hasChanges = names.length !== saved.length || names.some((name, i) => hashtagKey(name) !== saved[i]);
  let error: string | undefined;
  if (mutation.isError) {
    error =
      hashtagFailure(mutation.error) === "too-many"
        ? t("client:tag.featured.too_many", { max: String(maxFeaturedTags) })
        : t("client:tag.featured.save_failed", { reason: errorMessage(mutation.error) });
  }
  return {
    rows,
    canAdd: rows.length < maxFeaturedTags,
    add: () => {
      setRows([...rows, { key: nextKey, name: "" }]);
      setNextKey(nextKey + 1);
    },
    update: (key, name) => {
      setRows(rows.map((row) => (row.key === key ? { ...row, name } : row)));
    },
    remove: (key) => {
      setRows(rows.filter((row) => row.key !== key));
    },
    move: (key, step) => {
      const from = rows.findIndex((row) => row.key === key);
      const to = from + step;
      const row = rows[from];
      if (row === undefined || to < 0 || to >= rows.length) return;
      setRows(rows.toSpliced(from, 1).toSpliced(to, 0, row));
      const name = hashtagInput(row.name);
      notice.inPlace(
        name === ""
          ? t("client:tag.featured.moved_empty", { position: String(to + 1) })
          : t("client:tag.featured.moved", { tag: hashtagInText(name), position: String(to + 1) }),
      );
    },
    validate: (name) => {
      const value = hashtagInput(name);
      return value === "" || isHashtagName(value) ? null : t("client:tag.name.invalid");
    },
    hasChanges,
    save: async () => {
      await mutation.mutateAsync(names);
    },
    reset: () => {
      mutation.reset();
    },
    isSaving: mutation.isPending,
    isSaved: mutation.isSuccess,
    error,
  };
}
