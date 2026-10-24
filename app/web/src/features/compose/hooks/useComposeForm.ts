// SPDX-License-Identifier: AGPL-3.0-only
import { useAnnouncer, useToast } from "@my/ui";
import { useMutation, useMutationState, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { useApi } from "../../../lib/auth/SessionProvider";
import { useConfig } from "../../../lib/config";
import { errorMessage } from "../../../lib/errors";
import { useFederation } from "../../../lib/federation";
import { isRemoteAcct } from "../../../lib/handle";
import { useLocale, useT } from "../../../lib/i18n";
import { shellQueryReader } from "../../../lib/query";
import {
  cachePostedStatus,
  createStatus,
  type CreateStatusRequest,
  credentialsQueryOptions,
  instanceQueryOptions,
  type PostingLimits,
  preferencesQueryOptions,
  type QuotePolicy,
  type Status,
  statusQueryOptions,
  type Visibility,
} from "../api/compose";
import { type Draft, isEmptyDraft } from "./draftStore";
import { newPoll, type PollDraft, pollProblem, pollRequest, withOption, withoutOption } from "./poll";
import { type LanguageOption, languageOptions } from "./languages";
import { countCharacters, defaultVisibility, domainMentions, postedVisibility, visibilities } from "./policy";
import { rememberTags } from "./recentTags";
import { type Attachments, descriptionFieldId, useAttachments } from "./useAttachments";
import { type ComposeController, composeTextId, useComposeState } from "./useCompose";
import { useUnreachedMentions } from "./useUnreachedMentions";
import { visibilityLabel } from "./visibilityMessages";

/** The key of every post mutation, so that the outbox can read their state (useMutationState). */
const createStatusMutationKey = ["compose", "createStatus"] as const;

type SendVariables = { idempotencyKey: string; body: CreateStatusRequest };

/** The post being replied to, or quoted. */
export type ReplyTarget = { status: "none" } | { status: "loading" } | { status: "error" } | { status: "ready"; post: Status };

/** A post of the outbox as the form shows it. `message` is localized. */
export type OutboxView = {
  idempotencyKey: string;
  text: string;
  language: string | undefined;
} & ({ status: "sending" } | { status: "failed"; message: string } | { status: "unsent" });

export type ComposeForm =
  | { status: "loading" }
  /** The limits or preferences could not be loaded; `message` is localized. */
  | { status: "error"; message: string; retry: () => void }
  | ({ status: "ready" } & ReadyComposeForm);

export type ReadyComposeForm = {
  draft: Draft;
  /** Why the last attempt failed (localized), or null. */
  error: string | null;
  edit: (change: Partial<Omit<Draft, "idempotencyKey" | "attachments">>) => void;
  /** The characters the text may have: the instance's limit less the content warning. */
  textLimit: number;
  /** The visibility the post gets: the chosen one, or the ceiling of the account's posting limits. */
  visibility: Visibility;
  /** What the account's restrictions let it post (data-model/05-states-and-policies.md §5.4), or null while they keep every visibility. */
  limits: PostingLimits | null;
  /**
   * The accounts of other servers that the text mentions and that will not receive the post because it is
   * `local` and they do not follow the viewer (ADR-0017); empty for the other visibilities.
   */
  unreachedMentions: string[];
  /**
   * Whether the post replies to or mentions an account of another server while the instance sends nothing out
   * (its federation mode is `off` or `inbound`, ADR-0076): the post is posted here but does not reach that server.
   */
  staysLocal: boolean;
  language: string;
  languages: LanguageOption[];
  replyTo: ReplyTarget;
  cancelReply: () => void;
  /** The post quoted (FR-114). */
  quoteOf: ReplyTarget;
  cancelQuote: () => void;
  /** Who may quote the post: the chosen policy, or the preference. */
  quotePolicy: QuotePolicy;
  attachments: Attachments;
  /** Whether files can be attached: not to a post with a poll (FR-112), and up to the limit. */
  canAttach: boolean;
  poll: PollForm;
  /** Whether the post must describe its images before it is sent without asking (the preference). */
  requireDescriptions: boolean;
  /**
   * Whether the server requires a description of every image (`limits.require_media_description`, FR-105): the form
   * is not sent while an image has none, and the preference's question never comes up.
   */
  isDescriptionRequired: boolean;
  canSubmit: boolean;
  /** Sends the draft, or asks first when images lack a description that the preferences ask for. */
  submit: () => void;
  /** Whether the question about images without a description is open, and how many lack one. */
  confirmation: { isOpen: boolean; missing: number };
  /** Answers the question: send anyway, or go back to the first image without a description. */
  confirm: (answer: "send" | "describe") => void;
  outbox: OutboxView[];
  retry: (idempotencyKey: string) => void;
  /** Puts an outbox post back into the form; only while the form is empty (`canEditOutbox`). */
  editOutbox: (idempotencyKey: string) => void;
  canEditOutbox: boolean;
  discard: (idempotencyKey: string) => void;
};

/** The poll of the draft and its editing (FR-112). */
export type PollForm = {
  /** The poll, or null when the post has none. */
  draft: PollDraft | null;
  /** Whether a poll can be added: a post with attachments has none. */
  canAdd: boolean;
  /** What keeps the poll from being sent (an empty or a repeated option), or null. */
  problem: "empty" | "duplicate" | null;
  /** Adds an empty poll, or removes the poll. */
  toggle: (on: boolean) => void;
  edit: (change: Partial<Omit<PollDraft, "options">>) => void;
  editOption: (key: string, title: string) => void;
  addOption: () => void;
  removeOption: (key: string) => void;
};

/** The request a draft becomes. */
function requestBody(draft: Draft, visibility: Visibility, language: string): CreateStatusRequest {
  const media = draft.attachments.flatMap((a) =>
    a.mediaId === null ? [] : [{ id: a.mediaId, ...(a.description.trim() === "" ? {} : { description: a.description.trim() }) }],
  );
  return {
    text: draft.text,
    visibility,
    language,
    ...(draft.hasWarning && draft.spoilerText.trim() !== "" ? { spoiler_text: draft.spoilerText.trim() } : {}),
    ...(media.length > 0 ? { media, sensitive: draft.sensitive } : {}),
    ...(draft.poll === null ? {} : { poll: pollRequest(draft.poll) }),
    ...(draft.replyToId === null ? {} : { in_reply_to_id: draft.replyToId }),
    ...(draft.quoteId === null ? {} : { quoted_status_id: draft.quoteId }),
    ...(draft.quotePolicy === null ? {} : { quote_policy: draft.quotePolicy }),
  };
}

/** The state of a post that the draft names, replied to or quoted. */
function targetOf(id: string | null, post: Status | undefined, isError: boolean): ReplyTarget {
  if (id === null) return { status: "none" };
  if (post !== undefined) return { status: "ready", post };
  return isError ? { status: "error" } : { status: "loading" };
}

/**
 * Whether the server's answers that the form needs (the instance's limits, the viewer's preferences) are
 * still on their way. The sheet waits for them before it opens (Compose.tsx).
 */
export function useComposeFormPending(): boolean {
  const api = useApi();
  const instance = useQuery({ ...instanceQueryOptions(api), ...shellQueryReader });
  const preferences = useQuery({ ...preferencesQueryOptions(api), ...shellQueryReader });
  return instance.isPending || preferences.isPending;
}

/**
 * The logic of the new-post form (web-client/README.md A4): the draft from the store, the limits and defaults
 * from the server, the attachments, and sending.
 *
 * Sending moves the draft into the outbox and empties the form; the outbox shows the post from the
 * mutation's variables while it is on its way (S8). A failure puts the draft back into the form when the
 * user has not started another one, and otherwise leaves it in the outbox to send again: a draft is
 * never lost (and survives a reload, draftStore.ts). Every attempt for one draft sends the same
 * Idempotency-Key, so a retry after a lost answer cannot post twice.
 */
export function useComposeForm(controller: ComposeController): ComposeForm {
  const { store } = controller;
  const api = useApi();
  const queryClient = useQueryClient();
  const t = useT();
  const locale = useLocale();
  const toast = useToast();
  const announce = useAnnouncer();
  const state = useComposeState(store);
  const { draft } = state;
  const instance = useQuery({ ...instanceQueryOptions(api), ...shellQueryReader });
  const preferences = useQuery({ ...preferencesQueryOptions(api), ...shellQueryReader });
  const replyQuery = useQuery({ ...statusQueryOptions(api, draft.replyToId ?? ""), enabled: draft.replyToId !== null });
  const quoteQuery = useQuery({ ...statusQueryOptions(api, draft.quoteId ?? ""), enabled: draft.quoteId !== null });
  // The form exists only for a signed-in viewer, so the account is in the cache already.
  const limits = useQuery({ ...credentialsQueryOptions(api), ...shellQueryReader }).data?.posting;
  const replyPost = draft.replyToId === null ? undefined : replyQuery.data;
  const preferredVisibility = preferences.data?.default_visibility;
  const visibility =
    preferredVisibility === undefined ? undefined : postedVisibility(draft.visibility ?? defaultVisibility(preferredVisibility, replyPost), limits);
  const unreachedMentions = useUnreachedMentions(draft.text, visibility === "local");
  const { reachesRemote } = useFederation();
  const { accountDomain } = useConfig();
  const attachments = useAttachments(store, draft.attachments);
  const [isConfirming, setIsConfirming] = useState(false);

  const send = useMutation({
    mutationKey: createStatusMutationKey,
    mutationFn: ({ idempotencyKey, body }: SendVariables) => createStatus(api, body, idempotencyKey),
    onSuccess: async (status, { idempotencyKey, body }) => {
      // The post is done once the lists it appears in are fetched again (it stays in the outbox as sending
      // until then), so that a list opened next shows it at once (web-client/README.md S1). A failed refetch does
      // not fail the post: that list fetches again when it is shown.
      await cachePostedStatus(api, queryClient, status);
      store.remove(idempotencyKey);
      // The server narrows the visibility of a restricted account (data-model/05-states-and-policies.md §5.4); say so.
      const narrowed = status.visibility !== body.visibility;
      toast.show({
        message: narrowed ? t("client:compose.visibility.limited", { visibility: t(visibilityLabel[status.visibility]) }) : t("client:compose.posted"),
        tone: narrowed ? "neutral" : "success",
        ...(narrowed ? { timeout: null } : {}),
      });
      if (store.getSnapshot().outbox.length === 0 && isEmptyDraft(store.getSnapshot().draft)) {
        controller.setSheetOpen(false);
      }
    },
    onError: (error, { idempotencyKey }) => {
      const message = t("client:compose.failed", { reason: errorMessage(error) });
      store.restore(idempotencyKey, message);
      announce(message, "assertive");
    },
  });
  const attempts = useMutationState({
    filters: { mutationKey: createStatusMutationKey },
    select: (mutation) => ({
      idempotencyKey: (mutation.state.variables as SendVariables | undefined)?.idempotencyKey,
      status: mutation.state.status,
      error: mutation.state.error,
    }),
  });

  if (instance.isError || preferences.isError) {
    return {
      status: "error",
      message: errorMessage(instance.error ?? preferences.error),
      retry: () => {
        void instance.refetch();
        void preferences.refetch();
      },
    };
  }
  if (instance.data === undefined || preferences.data === undefined || visibility === undefined) return { status: "loading" };

  const { max_post_characters: maxCharacters, require_media_description: isDescriptionRequired } = instance.data.limits;
  const { default_post_language: preferredLanguage, require_media_description: requireDescriptions } = preferences.data;
  const replyTo = targetOf(draft.replyToId, replyPost, replyQuery.isError);
  const quoteOf = targetOf(draft.quoteId, draft.quoteId === null ? undefined : quoteQuery.data, quoteQuery.isError);
  const quotePolicy = draft.quotePolicy ?? preferences.data.default_quote_policy;
  const ownDomain = accountDomain.toLowerCase();
  const staysLocal =
    !reachesRemote &&
    ((replyTo.status === "ready" && isRemoteAcct(replyTo.post.account.acct)) ||
      domainMentions(draft.text).some((handle) => handle.slice(handle.indexOf("@") + 1) !== ownDomain));
  const language = draft.language ?? preferredLanguage;
  const warningLength = draft.hasWarning ? countCharacters(draft.spoilerText) : 0;
  const textLimit = maxCharacters - warningLength;
  const missing = attachments.items.filter((item) => item.description.trim() === "").length;
  const problem = draft.poll === null ? null : pollProblem(draft.poll);
  const canSubmit =
    draft.text.trim() !== "" &&
    countCharacters(draft.text) <= textLimit &&
    !attachments.isBusy &&
    !attachments.hasFailed &&
    problem === null &&
    replyTo.status !== "loading" &&
    quoteOf.status !== "loading";
  const editPoll = (change: (poll: PollDraft) => PollDraft) => {
    if (draft.poll !== null) store.edit({ poll: change(draft.poll) });
  };
  const poll: PollForm = {
    draft: draft.poll,
    canAdd: draft.attachments.length === 0,
    problem,
    toggle: (on) => {
      store.edit({ poll: on ? newPoll() : null });
    },
    edit: (change) => {
      editPoll((p) => ({ ...p, ...change }));
    },
    editOption: (key, title) => {
      editPoll((p) => ({ ...p, options: p.options.map((o) => (o.key === key ? { ...o, title } : o)) }));
    },
    addOption: () => {
      editPoll(withOption);
    },
    removeOption: (key) => {
      editPoll((p) => withoutOption(p, key));
    },
  };

  const sendDraft = () => {
    const idempotencyKey = draft.idempotencyKey ?? crypto.randomUUID();
    const body = requestBody(draft, visibility, language);
    store.send(idempotencyKey, body);
    send.mutate({ idempotencyKey, body });
    rememberTags(draft.text);
    document.getElementById(composeTextId)?.focus();
  };

  const outbox: OutboxView[] = state.outbox.map((item) => {
    const attempt = attempts.findLast((a) => a.idempotencyKey === item.idempotencyKey);
    const base = { idempotencyKey: item.idempotencyKey, text: item.body.text, language: item.body.language };
    if (attempt?.status === "pending") return { ...base, status: "sending" };
    if (attempt?.status === "error") return { ...base, status: "failed", message: errorMessage(attempt.error) };
    // Sent before a reload, or never answered: sending it again is safe with its Idempotency-Key.
    return { ...base, status: "unsent" };
  });

  return {
    status: "ready",
    draft,
    error: state.error,
    edit: (change) => {
      store.edit(change);
    },
    textLimit,
    visibility,
    limits: limits === undefined || limits.visibilities.length === visibilities.length ? null : limits,
    unreachedMentions,
    staysLocal,
    language,
    languages: languageOptions(locale, [preferredLanguage, language], t("client:compose.language.und")),
    replyTo,
    cancelReply: () => {
      store.edit({ replyToId: null });
    },
    quoteOf,
    cancelQuote: () => {
      store.edit({ quoteId: null });
    },
    quotePolicy,
    attachments,
    canAttach: attachments.canAdd && draft.poll === null,
    poll,
    requireDescriptions,
    isDescriptionRequired,
    canSubmit,
    submit: () => {
      if (!canSubmit) return;
      // The browser's validation of the required description fields keeps the form from being sent first; the server
      // would refuse the post anyway.
      if (isDescriptionRequired && missing > 0) return;
      if (requireDescriptions && missing > 0) {
        setIsConfirming(true);
        return;
      }
      sendDraft();
    },
    confirmation: { isOpen: isConfirming, missing },
    confirm: (answer) => {
      setIsConfirming(false);
      if (answer === "send") {
        sendDraft();
        return;
      }
      const first = attachments.items.find((item) => item.description.trim() === "");
      if (first !== undefined) {
        // After the dialog has closed and returned focus, move it to the field (react-aria restores focus first).
        requestAnimationFrame(() => document.getElementById(descriptionFieldId(first.key))?.focus());
      }
    },
    outbox,
    retry: (idempotencyKey) => {
      const item = state.outbox.find((o) => o.idempotencyKey === idempotencyKey);
      if (item !== undefined) send.mutate({ idempotencyKey, body: item.body });
    },
    editOutbox: (idempotencyKey) => {
      store.restore(idempotencyKey, null);
    },
    canEditOutbox: isEmptyDraft(draft),
    discard: (idempotencyKey) => {
      store.remove(idempotencyKey);
    },
  };
}
