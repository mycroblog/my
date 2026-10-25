// SPDX-License-Identifier: AGPL-3.0-only
// The moderators' account screens (FR-714, FR-703, FR-713): the search form and its results, one account, showing
// its email address and IP addresses only when the moderator asks (each time is recorded in the audit log, ADR-0038),
// removing its two-factor authentication, issuing a password reset link for its user (shown once), and lifting the
// moderation actions on it. Failures of what the moderator does are toasts or stay in the
// confirmation; a failure to load the results stays in their region (web-client/README.md E2, web-client/16-look-states-and-wording.md G16).
import { useToast } from "@my/ui";
import { useInfiniteQuery, useMutation, useQueryClient, useSuspenseQuery } from "@tanstack/react-query";
import { type SyntheticEvent, useState } from "react";
import { useApi } from "../../../lib/auth/SessionProvider";
import { ApiError, errorMessage } from "../../../lib/errors";
import { type ClientMessageKey, useT } from "../../../lib/i18n";
import { domainToAscii } from "../../../lib/idn";
import {
  type AdminAccount,
  type AdminAccountContactField,
  adminAccountContactFields,
  type AdminAccountDetail,
  type AdminAccountOrigin,
  adminAccountOrigins,
  type AdminAccountPrivateData,
  adminAccountQueryOptions,
  type AdminAccountRegistrationState,
  type AdminAccountRestriction,
  type AdminAccountSearch,
  adminAccountsQueryOptions,
  type AdminAccountState,
  adminAccountStates,
  cacheLiftedAccountAction,
  type AdminPasswordResetLink,
  disableAccountTwoFactorMutation,
  liftAccountEmailSuppressionMutation,
  refreshAccountEmailSuppression,
  issuePasswordResetLinkMutation,
  liftAccountActionMutation,
  refreshAccountTwoFactor,
  refreshLiftedAccountAction,
  revealAccountMutation,
} from "../api/accounts";
import { refreshAuditLog } from "../api/staff";
import type { AccountAction } from "../api/reports";
import { type PagedPages, toPagedList } from "./pagedList";

export const adminAccountStateLabels: Readonly<Record<AdminAccountState, ClientMessageKey>> = {
  active: "client:admin.accounts.state.active",
  pending: "client:admin.accounts.state.pending",
  rejected: "client:admin.accounts.state.rejected",
  limited: "client:admin.accounts.state.limited",
  disabled: "client:admin.accounts.state.disabled",
  suspended: "client:admin.accounts.state.suspended",
  deleted: "client:admin.accounts.state.deleted",
};

export const adminAccountRestrictionLabels: Readonly<Record<AdminAccountRestriction, ClientMessageKey>> = {
  login: "client:admin.accounts.restriction.login",
  post: "client:admin.accounts.restriction.post",
  interact: "client:admin.accounts.restriction.interact",
  visibility_ceiling_private: "client:admin.accounts.restriction.visibility_ceiling_private",
  public_discovery: "client:admin.accounts.restriction.public_discovery",
  unsolicited_reach: "client:admin.accounts.restriction.unsolicited_reach",
  media_sensitive: "client:admin.accounts.restriction.media_sensitive",
  profile_view: "client:admin.accounts.restriction.profile_view",
  federation: "client:admin.accounts.restriction.federation",
};

export const adminAccountRegistrationStateLabels: Readonly<Record<AdminAccountRegistrationState, ClientMessageKey>> = {
  pending: "client:admin.accounts.registration.pending",
  approved: "client:admin.accounts.registration.approved",
  rejected: "client:admin.accounts.registration.rejected",
};

export const adminAccountOriginLabels: Readonly<Record<AdminAccountOrigin, ClientMessageKey>> = {
  local: "client:admin.accounts.origin.local",
  remote: "client:admin.accounts.origin.remote",
};

export const adminAccountContactLabels: Readonly<Record<AdminAccountContactField, ClientMessageKey>> = {
  email: "client:admin.accounts.search.contact.email",
  email_domain: "client:admin.accounts.search.contact.email_domain",
  ip: "client:admin.accounts.search.contact.ip",
};

/** The option of a select of the search that keeps every account. */
export const anyOption = "any";

/** The search form, filled in by the moderator: each select is an option of the API or `any`. */
type SearchDraft = {
  username: string;
  domain: string;
  origin: AdminAccountOrigin | typeof anyOption;
  state: AdminAccountState | typeof anyOption;
  /** Which of the fields that only the moderators may see is searched, if any; its value is `contactValue`. */
  contact: AdminAccountContactField | typeof anyOption;
  contactValue: string;
};

function draftOf(search: AdminAccountSearch): SearchDraft {
  const contact = adminAccountContactFields.find((field) => search[field] !== undefined);
  return {
    username: search.username ?? "",
    domain: search.domain ?? "",
    origin: search.origin ?? anyOption,
    state: search.state ?? anyOption,
    contact: contact ?? anyOption,
    contactValue: contact === undefined ? "" : (search[contact] ?? ""),
  };
}

/** A text field of the search: trimmed, and omitted when empty. */
function field(value: string): string | undefined {
  const trimmed = value.trim();
  return trimmed === "" ? undefined : trimmed;
}

/** The search of a draft; the domain goes in its A-labels, as the server keeps domains. */
function searchOf(draft: SearchDraft): AdminAccountSearch {
  // A handle (`@bob@example.com`) is a username and a domain.
  const [username, handleDomain] = (field(draft.username)?.replace(/^@/, "") ?? "").split("@", 2);
  const domain = field(draft.domain) ?? field(handleDomain ?? "");
  const contactValue = field(draft.contactValue);
  return {
    username: field(username ?? ""),
    domain: domain === undefined ? undefined : domainToAscii(domain),
    origin: draft.origin === anyOption ? undefined : draft.origin,
    state: draft.state === anyOption ? undefined : draft.state,
    ...(draft.contact === anyOption || contactValue === undefined ? {} : { [draft.contact]: contactValue }),
  };
}

export function isOrigin(key: unknown): key is AdminAccountOrigin {
  return adminAccountOrigins.some((origin) => origin === key);
}

export function isAccountState(key: unknown): key is AdminAccountState {
  return adminAccountStates.some((state) => state === key);
}

export function isContactField(key: unknown): key is AdminAccountContactField {
  return adminAccountContactFields.some((item) => item === key);
}

export type AccountSearchForm = {
  draft: SearchDraft;
  change: (change: Partial<SearchDraft>) => void;
  /** Searches once the browser has checked the fields. */
  submit: (event: SyntheticEvent<HTMLFormElement>) => void;
};

/**
 * The search form, starting from `search` (the URL's). `onSearch(next)` runs the search; the route puts it into
 * the URL, and a new search there gives the form a new key (web-client/README.md R10).
 */
export function useAccountSearchForm(search: AdminAccountSearch, onSearch: (search: AdminAccountSearch) => void): AccountSearchForm {
  const [draft, setDraft] = useState(() => draftOf(search));
  return {
    draft,
    change: (change) => {
      setDraft({ ...draft, ...change });
    },
    submit: (event) => {
      event.preventDefault();
      onSearch(searchOf(draft));
    },
  };
}

/** The results of a search: loading the first page, failing to, or the accounts so far. */
export type AccountSearchResults =
  | { status: "loading" }
  | { status: "error"; reason: string; retry: () => void; isRetrying: boolean }
  | { status: "ready"; list: PagedPages<AdminAccount>; moreError: string | undefined };

/**
 * The accounts of `search`. The route's loader has just tried the first page; a failure (a search the server
 * refuses, say) stays in the results until the retry button instead of being tried again as they mount.
 */
export function useAccountSearchResults(search: AdminAccountSearch): AccountSearchResults {
  const api = useApi();
  const query = useInfiniteQuery({ ...adminAccountsQueryOptions(api, search), retryOnMount: false });
  if (query.data === undefined) {
    if (!query.isError) return { status: "loading" };
    return {
      status: "error",
      reason: errorMessage(query.error),
      retry: () => {
        void query.refetch();
      },
      isRetrying: query.isFetching,
    };
  }
  return {
    status: "ready",
    list: toPagedList({
      data: query.data,
      hasNextPage: query.hasNextPage,
      isFetchingNextPage: query.isFetchingNextPage,
      isFetching: query.isFetching,
      fetchNextPage: query.fetchNextPage,
    }),
    moreError: query.isFetchNextPageError ? errorMessage(query.error) : undefined,
  };
}

/** One account, which the route has loaded. */
export function useAdminAccount(id: string): AdminAccountDetail {
  const api = useApi();
  return useSuspenseQuery(adminAccountQueryOptions(api, id)).data;
}

/** The email address and IP addresses of a local account: hidden until `reveal()`, then what the server answered. */
export type PrivateData = {
  reveal: () => void;
  isPending: boolean;
  data: AdminAccountPrivateData | undefined;
};

/**
 * The email address and IP addresses of local account `id`. Nothing is asked of the server until `reveal()`, the
 * moderator's own action, and the answer is kept by this mutation only: it is not cached, so another visit to
 * the page shows them again only when asked again (and recorded again). They are shown, and `onRevealed()` runs,
 * once the audit log, which records the call, is fetched again (refreshAuditLog).
 */
export function usePrivateData(id: string, onRevealed: () => void): PrivateData {
  const api = useApi();
  const t = useT();
  const queryClient = useQueryClient();
  const toast = useToast();
  const mutation = useMutation({
    ...revealAccountMutation(api),
    onSuccess: async () => {
      await refreshAuditLog(queryClient);
      onRevealed();
    },
    onError: (error) => {
      toast.show({ message: t("client:admin.account.private.failed", { reason: errorMessage(error) }), tone: "danger" });
    },
  });
  return {
    reveal: () => {
      if (!mutation.isPending) mutation.mutate(id);
    },
    isPending: mutation.isPending,
    data: mutation.data,
  };
}

/** The lift being confirmed: of which action, and the reason typed so far. */
type LiftConfirmation = { action: AccountAction; reason: string };

/** The confirmation dialog: open, or closing and still showing the last confirmation while it fades out. */
type LiftDialog = { isOpen: boolean; confirmation: LiftConfirmation } | null;

export type LiftAccountAction = {
  /** Opens the confirmation of lifting `action`. */
  open: (action: AccountAction) => void;
  isOpen: boolean;
  /** The confirmation open now or last, which a closing dialog still shows; null before the first one. */
  confirmation: LiftConfirmation | null;
  changeReason: (reason: string) => void;
  /** Lifts the action; on success the confirmation closes and the action shows as lifted. */
  confirm: () => void;
  cancel: () => void;
  isPending: boolean;
  /** Why the lift failed, localized, for the confirmation (which stays open to try again). */
  error: string | undefined;
};

/**
 * Lifting the moderation actions on account `accountId`. `onLifted(action)` is called with the lifted action just
 * before its row loses its lift button (to keep focus), once the account's state and restrictions and the audit
 * log are read again.
 */
export function useLiftAccountAction(accountId: string, onLifted: (action: AccountAction) => void): LiftAccountAction {
  const api = useApi();
  const queryClient = useQueryClient();
  const [dialog, setDialog] = useState<LiftDialog>(null);
  const mutation = useMutation({
    ...liftAccountActionMutation(api),
    onSuccess: async (lifted) => {
      // The action is lifted once what it changes is fetched again (the confirmation stays pending until then), so
      // that going back to a search, the log or a timeline shows it as it is now (refreshLiftedAccountAction).
      // The row changes from the answer, after onLifted.
      await refreshLiftedAccountAction(queryClient, api, accountId, lifted);
      setDialog((current) => current && { ...current, isOpen: false });
      onLifted(lifted);
      cacheLiftedAccountAction(api, queryClient, accountId, lifted);
    },
  });
  const isOpen = dialog?.isOpen ?? false;
  return {
    open: (action) => {
      mutation.reset();
      setDialog({ isOpen: true, confirmation: { action, reason: "" } });
    },
    isOpen,
    confirmation: dialog?.confirmation ?? null,
    changeReason: (reason) => {
      if (dialog !== null) setDialog({ ...dialog, confirmation: { ...dialog.confirmation, reason } });
    },
    confirm: () => {
      if (dialog === null || !dialog.isOpen || mutation.isPending) return;
      mutation.mutate({ id: dialog.confirmation.action.id, reason: dialog.confirmation.reason.trim() });
    },
    cancel: () => {
      if (dialog !== null) setDialog({ ...dialog, isOpen: false });
      mutation.reset();
    },
    isPending: mutation.isPending,
    error: isOpen && mutation.isError ? errorMessage(mutation.error) : undefined,
  };
}

/** Removing the two-factor authentication of a local account, which asks first in a confirmation. */
export type RemoveTwoFactor = {
  open: () => void;
  isOpen: boolean;
  /** Removes it; on success the confirmation closes and the account shows it without. */
  confirm: () => void;
  cancel: () => void;
  isPending: boolean;
  /** Why the removal failed, localized, for the confirmation (which stays open). */
  error: string | undefined;
  /** What the removal did, for the region's StatusMessage; undefined before it. */
  status: string | undefined;
};

/**
 * Removing the two-factor authentication of local account `accountId`, whose handle (isolated) is `handle`, for a
 * user who lost their authenticator app and recovery codes. It is done once the account's page and the audit log,
 * which records it, are fetched again (refreshAccountTwoFactor); `onRemoved()` is called then, just before the
 * button that removes it goes away.
 */
export function useRemoveTwoFactor(accountId: string, handle: string, onRemoved: () => void): RemoveTwoFactor {
  const api = useApi();
  const t = useT();
  const queryClient = useQueryClient();
  const [isOpen, setIsOpen] = useState(false);
  const [status, setStatus] = useState<string | undefined>(undefined);
  const mutation = useMutation({
    ...disableAccountTwoFactorMutation(api),
    onSuccess: async () => {
      await refreshAccountTwoFactor(queryClient, api, accountId);
      setIsOpen(false);
      onRemoved();
      setStatus(t("client:admin.account.two_factor.removed", { handle }));
    },
    onError: (error) => {
      // Removed elsewhere already: the page shows the account as it is now.
      if (error instanceof ApiError && error.status === 409) void refreshAccountTwoFactor(queryClient, api, accountId);
    },
  });
  return {
    open: () => {
      mutation.reset();
      setIsOpen(true);
    },
    isOpen,
    confirm: () => {
      if (!isOpen || mutation.isPending) return;
      mutation.mutate(accountId);
    },
    cancel: () => {
      setIsOpen(false);
      mutation.reset();
    },
    isPending: mutation.isPending,
    error: isOpen && mutation.isError ? errorMessage(mutation.error) : undefined,
    status,
  };
}

/** Lifting the email suppression of a local account, which asks first in a confirmation. */
export type LiftEmailSuppression = RemoveTwoFactor;

/**
 * Lifting the suppression of the primary email address of local account `accountId`, whose handle (isolated) is
 * `handle`. It is done once the account's page and the audit log, which records it, are fetched again;
 * `onLifted()` is called then, just before the button that lifts it goes away. A 409 means it was lifted elsewhere
 * already: the page is fetched again and shows the account as it is now.
 */
export function useLiftEmailSuppression(accountId: string, handle: string, onLifted: () => void): LiftEmailSuppression {
  const api = useApi();
  const t = useT();
  const queryClient = useQueryClient();
  const [isOpen, setIsOpen] = useState(false);
  const [status, setStatus] = useState<string | undefined>(undefined);
  const mutation = useMutation({
    ...liftAccountEmailSuppressionMutation(api),
    onSuccess: async () => {
      await refreshAccountEmailSuppression(queryClient, api, accountId);
      setIsOpen(false);
      onLifted();
      setStatus(t("client:admin.account.email_suppression.lifted", { handle }));
    },
    onError: (error) => {
      if (error instanceof ApiError && error.status === 409) void refreshAccountEmailSuppression(queryClient, api, accountId);
    },
  });
  return {
    open: () => {
      mutation.reset();
      setIsOpen(true);
    },
    isOpen,
    confirm: () => {
      if (!isOpen || mutation.isPending) return;
      mutation.mutate(accountId);
    },
    cancel: () => {
      setIsOpen(false);
      mutation.reset();
    },
    isPending: mutation.isPending,
    error: isOpen && mutation.isError ? errorMessage(mutation.error) : undefined,
    status,
  };
}

/** Issuing a password reset link for the user of a local account, which asks first in a confirmation. */
export type IssuePasswordResetLink = {
  open: () => void;
  isOpen: boolean;
  /** Issues it; on success the confirmation closes and the link is shown. */
  confirm: () => void;
  cancel: () => void;
  isPending: boolean;
  /** Why issuing failed, localized, for the confirmation (which stays open). */
  error: string | undefined;
  /** The link just issued, to show once: kept only in this state, until the page closes or another is issued. */
  link: AdminPasswordResetLink | undefined;
  /** What issuing did, for the region's StatusMessage; undefined before it. */
  status: string | undefined;
};

/**
 * Issuing a password reset link for the user of local account `accountId`, whose handle (isolated) is `handle`, for
 * an administrator to hand to the user (FR-002). The server stores only the link's hash, so the answer is the only
 * place it is shown: it is kept in component state, never in the query cache, and the mutation drops it as soon as
 * the page closes. The audit log, which records the issuance, is marked stale.
 */
export function useIssuePasswordResetLink(accountId: string, handle: string): IssuePasswordResetLink {
  const api = useApi();
  const t = useT();
  const queryClient = useQueryClient();
  const [isOpen, setIsOpen] = useState(false);
  const [link, setLink] = useState<AdminPasswordResetLink | undefined>(undefined);
  const [status, setStatus] = useState<string | undefined>(undefined);
  const mutation = useMutation({
    ...issuePasswordResetLinkMutation(api),
    gcTime: 0,
    onSuccess: (issued) => {
      void refreshAuditLog(queryClient);
      setIsOpen(false);
      setLink(issued);
      setStatus(t("client:admin.account.password_reset.issued", { handle }));
    },
  });
  return {
    open: () => {
      mutation.reset();
      setIsOpen(true);
    },
    isOpen,
    confirm: () => {
      if (!isOpen || mutation.isPending) return;
      mutation.mutate(accountId);
    },
    cancel: () => {
      setIsOpen(false);
      mutation.reset();
    },
    isPending: mutation.isPending,
    error: isOpen && mutation.isError ? errorMessage(mutation.error) : undefined,
    link,
    status,
  };
}

/** Copies a password reset link to the clipboard and says whether that worked in a toast. */
export function useCopyPasswordResetLink(): (url: string) => void {
  const t = useT();
  const toast = useToast();
  return (url) => {
    navigator.clipboard.writeText(url).then(
      () => {
        // The toast is announced too.
        toast.show({ message: t("client:admin.account.password_reset.copied"), tone: "success" });
      },
      () => {
        toast.show({ message: t("client:admin.account.password_reset.copy_failed"), tone: "danger" });
      },
    );
  };
}
