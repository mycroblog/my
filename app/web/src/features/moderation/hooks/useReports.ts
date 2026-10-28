// SPDX-License-Identifier: AGPL-3.0-only
// The report queue (FR-701, FR-710): the reports of a tab, newest first; one report with what the moderators
// need to handle it on one page (NFR-MOD-4): the posts it points at, the target's earlier actions, the moderator
// who handles it, the notes, and resolving or reopening it. Each operation on the report answers the report after
// it, which replaces the cached one once the lists of reports it changes and the audit log are fetched again;
// failures are shown in a toast and leave the page as it was (web-client/README.md E2).
import { useToast } from "@my/ui";
import {
  useInfiniteQuery,
  useMutation,
  useQuery,
  useQueryClient,
  useSuspenseInfiniteQuery,
  useSuspenseQuery,
} from "@tanstack/react-query";
import { type SyntheticEvent, useState } from "react";
import { useApi } from "../../../lib/auth/SessionProvider";
import { errorMessage } from "../../../lib/errors";
import { type ClientMessageKey, type Translate, useT } from "../../../lib/i18n";
import type { Account } from "../../profile";
import type { Status } from "../../status";
import {
  type AccountAction,
  accountActionsQueryOptions,
  type AccountActionKind,
  type AccountActionState,
  accountByIdQueryOptions,
  addReportNoteMutation,
  type ChangeReport,
  changeReportMutation,
  instanceInfoQueryOptions,
  type Report,
  type ReportCategory,
  type ReportDetail,
  type ReportListQuery,
  type ReportOutcome,
  reportQueryOptions,
  reportsQueryOptions,
  storeReport,
  storeReportNote,
} from "../api/reports";
import { type PagedList, type PagedPages, toPagedList } from "./pagedList";

export const reportCategoryLabels: Readonly<Record<ReportCategory, ClientMessageKey>> = {
  spam: "client:admin.reports.category.spam",
  legal: "client:admin.reports.category.legal",
  violation: "client:admin.reports.category.violation",
  other: "client:admin.reports.category.other",
};

export const reportOutcomeLabels: Readonly<Record<ReportOutcome, ClientMessageKey>> = {
  actioned: "client:admin.reports.outcome.actioned",
  dismissed: "client:admin.reports.outcome.dismissed",
};

export const accountActionKindLabels: Readonly<Record<AccountActionKind, ClientMessageKey>> = {
  warn: "client:admin.account_actions.kind.warn",
  mark_statuses_sensitive: "client:admin.account_actions.kind.mark_statuses_sensitive",
  delete_statuses: "client:admin.account_actions.kind.delete_statuses",
  force_sensitive: "client:admin.account_actions.kind.force_sensitive",
  limit: "client:admin.account_actions.kind.limit",
  restrict_posting: "client:admin.account_actions.kind.restrict_posting",
  restrict_interaction: "client:admin.account_actions.kind.restrict_interaction",
  disable_login: "client:admin.account_actions.kind.disable_login",
  suspend: "client:admin.account_actions.kind.suspend",
  ban: "client:admin.account_actions.kind.ban",
};

export const accountActionStateLabels: Readonly<Record<AccountActionState, ClientMessageKey>> = {
  active: "client:admin.account_actions.state.active",
  expired: "client:admin.account_actions.state.expired",
  lifted: "client:admin.account_actions.state.lifted",
};

/** An account's handle as this server knows it: `@alice`, or `@bob@example.com` for a remote one. */
export function handleOf(account: Pick<Account, "acct">): string {
  return `@${account.acct}`;
}

/** The name of an account for a row or a sentence: its display name, or its handle when it has none. */
export function accountName(account: Pick<Account, "acct" | "display_name">): string {
  return account.display_name === "" ? handleOf(account) : account.display_name;
}

/** Text written by people inside a sentence, isolated from it in its own direction (U+2068 FIRST STRONG ISOLATE … U+2069). */
export function isolate(text: string): string {
  return `\u2068${text}\u2069`;
}

/** Whether an account belongs to another server (its handle has a domain). */
export function isRemote(account: Pick<Account, "acct">): boolean {
  return account.acct.includes("@");
}

/** The reports that `list` selects, in its order. */
export function useReports(list: ReportListQuery): PagedPages<Report> {
  const api = useApi();
  return toPagedList(useSuspenseInfiniteQuery(reportsQueryOptions(api, list)));
}

/** One report, which the route has loaded. */
export function useReport(id: string): ReportDetail {
  const api = useApi();
  return useSuspenseQuery(reportQueryOptions(api, id)).data;
}

/** An account by its ID, for the moderator of a report or a note: loading, found, or not to be found. */
export type AccountById = { status: "loading" } | { status: "found"; account: Account } | { status: "missing" };

export function useAccountById(id: string): AccountById {
  const api = useApi();
  const account = useQuery(accountByIdQueryOptions(api, id));
  if (account.data !== undefined) return { status: "found", account: account.data };
  return account.isError ? { status: "missing" } : { status: "loading" };
}

/** A rule a report cites: its text from the instance, or null when the instance no longer has it (retired). */
export type CitedRule = { id: string; text: string } | { id: string; text: null };

/** The rules that a report cites, in its order; null while the instance loads. */
export function useCitedRules(ruleIds: readonly string[]): CitedRule[] | null {
  const api = useApi();
  const instance = useQuery({ ...instanceInfoQueryOptions(api), enabled: ruleIds.length > 0 });
  if (ruleIds.length === 0) return [];
  if (instance.data === undefined) return instance.isError ? ruleIds.map((id) => ({ id, text: null })) : null;
  const rules = instance.data.rules;
  return ruleIds.map((id) => {
    const rule = rules.find((item) => item.id === id);
    return rule === undefined ? { id, text: null } : { id, text: rule.text };
  });
}

/**
 * One post a report points at: shown, whatever its visibility, with when it was deleted for a post its author (or
 * someone else) deleted; or gone, when the server no longer has it.
 */
export type ReportedStatus =
  { id: string; status: "found"; value: Status; deletedAt: string | undefined } | { id: string; status: "gone" };

/** The posts that a report points at, in its order; the report carries them for the moderators (`statuses`). */
export function reportedStatuses(report: Pick<ReportDetail, "status_ids" | "statuses">): ReportedStatus[] {
  return report.status_ids.map((id) => {
    const found = report.statuses.find((item) => item.status.id === id);
    return found === undefined
      ? { id, status: "gone" }
      : { id, status: "found", value: found.status, deletedAt: found.deleted_at };
  });
}

/** The moderation actions on the target of a report, newest first; only for a viewer with manage_accounts. */
export function useTargetActions(
  accountId: string,
): PagedList<AccountAction> & { error: string | undefined; isLoading: boolean } {
  const api = useApi();
  const query = useInfiniteQuery(accountActionsQueryOptions(api, accountId));
  // An infinite query's data before its first page is undefined; the list shows a spinner or the reason meanwhile.
  const pages = query.data?.pages ?? [];
  return {
    items: pages.flatMap((page) => page.items),
    hasMore: query.hasNextPage,
    isLoadingMore: query.isFetchingNextPage,
    loadMore: () => {
      // A fetch in progress would be overwritten by the page fetched now (TanStack Query, "Infinite Queries").
      if (!query.isFetching) void query.fetchNextPage();
    },
    error: query.isError ? errorMessage(query.error) : undefined,
    isLoading: query.isPending,
  };
}

/** What a change of a report did, in words. */
function changedMessage(t: Translate, change: ChangeReport): string {
  switch (change.operation) {
    case "assign_to_self":
      return t("client:admin.report.assigned");
    case "unassign":
      return t("client:admin.report.unassigned");
    case "reopen":
      return t("client:admin.report.reopened");
    case "resolve":
      return t("client:admin.report.resolved", { outcome: t(reportOutcomeLabels[change.outcome]) });
  }
}

/** Why a change of a report failed, in words; `reason` is the server's. */
function failedMessage(t: Translate, change: ChangeReport, reason: string): string {
  switch (change.operation) {
    case "assign_to_self":
      return t("client:admin.report.assign_failed", { reason });
    case "unassign":
      return t("client:admin.report.unassign_failed", { reason });
    case "reopen":
      return t("client:admin.report.reopen_failed", { reason });
    case "resolve":
      return t("client:admin.report.resolve_failed", { reason });
  }
}

export type ReportChanges = {
  assignToSelf: () => void;
  unassign: () => void;
  resolve: (outcome: ReportOutcome) => void;
  reopen: () => void;
  /** The change being sent, if any; its button waits and the others are disabled. */
  pending: ChangeReport | undefined;
};

/**
 * Taking, leaving, resolving and reopening one report. `onChanged(change, message)` is called once the cache has
 * the report after it and the lists of reports and the audit log are fetched again (the change stays pending until
 * then), with what the change did in words: the page shows it next to the pressed button, and keeps focus where
 * that button went away.
 */
export function useReportChanges(
  id: string,
  onChanged: (change: ChangeReport, message: string) => void,
): ReportChanges {
  const api = useApi();
  const t = useT();
  const queryClient = useQueryClient();
  const toast = useToast();
  const mutation = useMutation({
    ...changeReportMutation(api),
    onSuccess: async (report, change) => {
      await storeReport(queryClient, api, report);
      onChanged(change, changedMessage(t, change));
    },
    onError: (error, change) => {
      toast.show({ message: failedMessage(t, change, errorMessage(error)), tone: "danger" });
    },
  });
  const send = (change: ChangeReport) => {
    if (!mutation.isPending) mutation.mutate(change);
  };
  return {
    assignToSelf: () => {
      send({ id, operation: "assign_to_self" });
    },
    unassign: () => {
      send({ id, operation: "unassign" });
    },
    resolve: (outcome) => {
      send({ id, operation: "resolve", outcome });
    },
    reopen: () => {
      send({ id, operation: "reopen" });
    },
    pending: mutation.isPending ? mutation.variables : undefined,
  };
}

export type ReportNoteForm = {
  body: string;
  change: (body: string) => void;
  /** Adds the note once the browser has checked the field; a failure is shown under it and keeps the text. */
  submit: (event: SyntheticEvent<HTMLFormElement>) => void;
  isPending: boolean;
  /** Why the note was not added, localized: a blank note, or the server's reason. */
  error: string | undefined;
  /** That the last note was added, for the form's StatusMessage. */
  status: string | undefined;
};

/**
 * The form of a new note on a report. A success empties it and the report's notes show the new one, once the
 * audit log is fetched again (storeReportNote).
 */
export function useReportNoteForm(id: string): ReportNoteForm {
  const api = useApi();
  const t = useT();
  const queryClient = useQueryClient();
  const [body, setBody] = useState("");
  const [status, setStatus] = useState<string | undefined>(undefined);
  const [isBlank, setIsBlank] = useState(false);
  const add = useMutation({
    ...addReportNoteMutation(api),
    onSuccess: async (report) => {
      await storeReportNote(queryClient, api, report);
      setBody("");
      setStatus(t("client:admin.report.notes.added"));
    },
  });
  return {
    body,
    change: (next) => {
      setBody(next);
      setIsBlank(false);
    },
    submit: (event) => {
      event.preventDefault();
      if (add.isPending) return;
      add.reset();
      // A note of white space only is blank to the server too.
      if (body.trim() === "") {
        setIsBlank(true);
        return;
      }
      add.mutate({ id, body });
    },
    isPending: add.isPending,
    error: isBlank
      ? t("client:admin.report.notes.blank")
      : add.isError
        ? t("client:admin.report.notes.failed", { reason: errorMessage(add.error) })
        : undefined,
    status,
  };
}
