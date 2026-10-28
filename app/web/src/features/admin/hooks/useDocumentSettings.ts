// SPDX-License-Identifier: AGPL-3.0-only
// The terms and policies screen (#489): the terms of service and the privacy policy, each written once in Markdown, in
// any language, and saved on its own (FR-705); every language shows the same text. The server renders and sanitizes
// them on their public pages, keeps every revision, and records each change in the audit log.
import { useMutation, useQuery, useQueryClient, useSuspenseQuery } from "@tanstack/react-query";
import { type SyntheticEvent, useState } from "react";
import { useApi } from "../../../lib/auth/SessionProvider";
import { errorMessage } from "../../../lib/errors";
import { useT } from "../../../lib/i18n";
import { shellQueryReader } from "../../../lib/query";
import {
  type DocumentKey,
  instanceQueryOptions,
  type InstanceText,
  instanceTextsQueryOptions,
  cacheSavedDocument,
  setDocumentMutation,
} from "../api/documents";

/** The drafts that differ from what is saved, by document. */
type Drafts = Readonly<Partial<Record<DocumentKey, string>>>;

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
};

export type DocumentSettings = {
  terms: DocumentEditor;
  privacyPolicy: DocumentEditor;
  /** The commerce disclosures of the supporter plans (ADR-0072 §13), published at /about/commerce. */
  commerceDisclosure: DocumentEditor;
  /** Whether a document has a draft that is not saved. */
  hasChanges: boolean;
};

type EditorContext = {
  texts: readonly InstanceText[];
  drafts: Drafts;
  setDraft: (key: DocumentKey, value: string | undefined) => void;
};

/** One document's form; the screen has three, so this is called three times (never in a loop, web-client/README.md R3). */
function useDocumentEditor(
  key: DocumentKey,
  { texts, drafts, setDraft }: EditorContext,
  pageUrl: string | undefined,
): DocumentEditor {
  const api = useApi();
  const t = useT();
  const queryClient = useQueryClient();
  const [confirming, setConfirming] = useState(false);
  const [status, setStatus] = useState<string | undefined>(undefined);
  const saved = texts.find((text) => text.key === key);
  const savedBody = saved?.body ?? "";
  const value = drafts[key] ?? savedBody;
  const save = useMutation({
    ...setDocumentMutation(api),
    onSuccess: async (text) => {
      // The texts, the links to the pages, and the audit log change: the text is saved once those shown now are
      // fetched again (the form stays pending until then); the others load again when opened (cacheSavedDocument).
      await cacheSavedDocument(queryClient);
      setConfirming(false);
      setDraft(key, undefined);
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
      setDraft(key, next === savedBody ? undefined : next);
    },
    savedAt: saved?.revised_at,
    pageUrl,
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
  };
}

/** The documents. Drafts are kept per document; leaving the screen with any asks first (useUnsavedChangesGuard). */
export function useDocumentSettings(): DocumentSettings {
  const api = useApi();
  const texts = useSuspenseQuery(instanceTextsQueryOptions(api)).data.items;
  const instance = useQuery({ ...instanceQueryOptions(api), ...shellQueryReader }).data;
  const [drafts, setDrafts] = useState<Drafts>({});
  const context: EditorContext = {
    texts,
    drafts,
    setDraft: (key, value) => {
      setDrafts((current) => {
        const others = Object.fromEntries(Object.entries(current).filter(([other]) => other !== key));
        return value === undefined ? others : { ...others, [key]: value };
      });
    },
  };
  return {
    terms: useDocumentEditor("terms", context, instance?.terms_url),
    privacyPolicy: useDocumentEditor("privacy_policy", context, instance?.privacy_policy_url),
    commerceDisclosure: useDocumentEditor("commerce_disclosure", context, instance?.commerce_disclosure_url),
    hasChanges: Object.keys(drafts).length > 0,
  };
}
