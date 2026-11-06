// SPDX-License-Identifier: AGPL-3.0-only
// The terms and policies screen (#489): the terms of service, the privacy policy and the commerce disclosures, each
// written once in Markdown, in any language, on a page of its own with one save (FR-705); every language shows the
// same text. The server renders and sanitizes them on their public pages, keeps every revision, and records each change
// in the audit log.
import { useMutation, useQuery, useQueryClient, useSuspenseQuery } from "@tanstack/react-query";
import { type SyntheticEvent, useState } from "react";
import { useApi } from "../../../lib/auth/SessionProvider";
import { errorMessage } from "../../../lib/errors";
import { useT } from "../../../lib/i18n";
import { shellQueryReader } from "../../../lib/query";
import {
  type DocumentKey,
  instanceQueryOptions,
  instanceTextsQueryOptions,
  cacheSavedDocument,
  setDocumentMutation,
} from "../api/documents";

/** One document's form. */
export type DocumentEditor = {
  key: DocumentKey;
  /** The text in the field: the draft, or else what is saved. */
  value: string;
  change: (value: string) => void;
  /** When the text was saved; undefined when there is none, which hides its page and links. */
  savedAt: string | undefined;
  /** The public page of the document; undefined when there is none. */
  pageUrl: string | undefined;
  /** Saves the text once the browser has checked the field; removing a saved text asks first. */
  submit: (event: SyntheticEvent<HTMLFormElement>) => void;
  isPending: boolean;
  /** The question before removing the text, or null while it is not asked. */
  confirmation: { confirm: () => void; cancel: () => void } | null;
  /** Why saving failed, localized: in the question while it is open, else under the form. */
  error: string | undefined;
  /** What the last save did, for the form's StatusMessage. */
  status: string | undefined;
  /** Whether the field holds a draft that is not saved; leaving the page with one asks first (useUnsavedChangesGuard). */
  hasChanges: boolean;
};

/** The public page of each document, from GET /instance. */
const pageUrls = {
  terms: "terms_url",
  privacy_policy: "privacy_policy_url",
  commerce_disclosure: "commerce_disclosure_url",
} as const satisfies Record<DocumentKey, string>;

/** The form of one document, the page of the screen it is on. */
export function useDocumentEditor(key: DocumentKey): DocumentEditor {
  const api = useApi();
  const t = useT();
  const queryClient = useQueryClient();
  const texts = useSuspenseQuery(instanceTextsQueryOptions(api)).data.items;
  const instance = useQuery({ ...instanceQueryOptions(api), ...shellQueryReader }).data;
  const [draft, setDraft] = useState<string | undefined>(undefined);
  const [confirming, setConfirming] = useState(false);
  const [status, setStatus] = useState<string | undefined>(undefined);
  const saved = texts.find((text) => text.key === key);
  const savedBody = saved?.body ?? "";
  const value = draft ?? savedBody;
  const save = useMutation({
    ...setDocumentMutation(api),
    onSuccess: async (text) => {
      // The texts, the links to the pages, and the audit log change: the text is saved once those shown now are
      // fetched again (the form stays pending until then); the others load again when opened (cacheSavedDocument).
      await cacheSavedDocument(queryClient);
      setConfirming(false);
      setDraft(undefined);
      setStatus(text.body === "" ? t("client:admin.documents.removed") : t("client:admin.documents.saved"));
    },
  });
  const run = () => {
    save.mutate({ key, body: value });
  };
  return {
    key,
    value,
    change: (next) => {
      setDraft(next === savedBody ? undefined : next);
    },
    savedAt: saved?.revised_at,
    pageUrl: instance?.[pageUrls[key]],
    submit: (event) => {
      event.preventDefault();
      if (save.isPending) return;
      save.reset();
      if (value === "" && saved !== undefined) setConfirming(true);
      else run();
    },
    isPending: save.isPending,
    confirmation: confirming
      ? {
          confirm: run,
          cancel: () => {
            setConfirming(false);
            save.reset();
          },
        }
      : null,
    error: save.isError ? errorMessage(save.error) : undefined,
    status,
    hasChanges: draft !== undefined,
  };
}
