// SPDX-License-Identifier: AGPL-3.0-only
// The reports as a holder of `manage_reports` handles them (FR-709–FR-712, FR-727, ADR-0058): the moderators' queue,
// filtered by its state, its category and who handles it, the newest or the longest waiting first, a page at a time by
// the server's cursor; a report as a moderator opens it (its notes, the posts it points at whatever their visibility and
// also when their author deleted them, the voice chat room or the event it points at, what the moderators had about
// its target); and what a moderator does to it: take it, leave it to nobody, write a note, resolve it with an outcome,
// reopen it, forward a remote one to its server once, and translate its comment and its posts. Every write answers the
// report whole, which the cache then holds, and reads the queue and the counts again, which it changes. The reader's
// own reports are reports.ts's.
import { type components } from "@mycroblog/mykit";
import { keepPreviousData, useQuery } from "@tanstack/react-query";
import type { AppData } from "./app-data";
import { accountByIdQuery, viewerQuery, type Account } from "./accounts";
import { moderationCountsQuery } from "./rights";
import { useAppData } from "./provider";
import { TranslationRefusal, translationRefusalOf, type StatusTranslation } from "./translation";

/** A report in the moderators' queue. */
export type QueuedReport = components["schemas"]["AdminReport"];
/** A report as a moderator opens it. */
export type ReportDetail = components["schemas"]["AdminReportDetail"];
/** A post a report points at, as a moderator reads it, with when it was deleted, if it was. */
export type ReportedStatus = components["schemas"]["AdminReportStatus"];
/** A moderator's internal note on a report. */
export type ReportNote = components["schemas"]["AdminReportNote"];
/** The voice chat room a report points at, as the moderators review it. */
export type ReportedVoiceRoom = components["schemas"]["AdminReportVoiceRoom"];
/** The event a report points at, as the moderators review it. */
export type ReportedEvent = components["schemas"]["AdminReportEvent"];
/** Whether a report waits for the moderators (`open`) or was handled (`resolved`). */
export type ReportState = components["schemas"]["AdminReportState"];
/** How a report was resolved: a moderator acted on its target (`actioned`), or nothing needed doing (`dismissed`). */
export type ReportOutcome = components["schemas"]["AdminReportOutcome"];
/** The comment of a report in another language, which the server does not keep. */
export type ReportCommentTranslation = components["schemas"]["ReportCommentTranslation"];

type Category = components["schemas"]["ReportCategory"];

/** The states of a report, open first. */
export const reportStates = ["open", "resolved"] as const satisfies readonly ReportState[];
/** The categories of a report, in the server's order. */
export const reportCategories = ["spam", "legal", "violation", "other"] as const satisfies readonly Category[];
/** The outcomes of a resolution. */
export const reportOutcomes = ["actioned", "dismissed"] as const satisfies readonly ReportOutcome[];

/** The most characters of a note, and of the text a forward sends to the target's server. */
export const reportNoteLimit = 2000;
export const reportForwardLimit = 1000;

/**
 * What the queue shows: the reports of one state, one category, and assigned to the reader (`me`) or to nobody
 * (`none`), each of every kind where left out; the newest first, or the longest waiting (`oldest`); and the page after
 * `cursor`, the first without it.
 */
export type ReportQueueQuery = {
  readonly state?: ReportState | undefined;
  readonly category?: Category | undefined;
  readonly assignee?: "me" | "none" | undefined;
  readonly order?: "newest" | "oldest" | undefined;
  readonly cursor?: string | undefined;
};

/** The query of the API that asks for `q`, without what it leaves out. */
function apiQuery(q: ReportQueueQuery) {
  return {
    ...(q.state === undefined ? {} : { state: q.state }),
    ...(q.category === undefined ? {} : { category: q.category }),
    ...(q.assignee === undefined ? {} : { assignee: q.assignee }),
    ...(q.order === undefined || q.order === "newest" ? {} : { order: q.order }),
    ...(q.cursor === undefined ? {} : { cursor: q.cursor }),
  };
}

/** A page of the queue. */
export function reportQueueQuery({ queries }: AppData, q: ReportQueueQuery) {
  return queries.queryOptions("get", "/admin/reports", { params: { query: apiQuery(q) } });
}

/** A report as a moderator opens it. */
export function reportQuery({ queries }: AppData, id: string) {
  return queries.queryOptions("get", "/admin/reports/{id}", { params: { path: { id } } });
}

/** What a screen shows of a page of the queue: its reports, the next page's cursor, and where its read is. */
export type ReportQueuePage = {
  /** The page's reports; none until it is read. */
  readonly reports: readonly QueuedReport[];
  /** The cursor of the next page; none on the last one. */
  readonly next: string | undefined;
  /** `pending` until the page is read, `error` when it failed, then `success`. */
  readonly status: "pending" | "error" | "success";
  /** The page shows what the reader asked before while the page they asked for now is on its way. */
  readonly stale: boolean;
  /** The server refused the reader the queue (403): their roles no longer have `manage_reports`. */
  readonly forbidden: boolean;
  readonly refetch: () => void;
};

/** Whether `error` is the server's refusal of what needs a permission the reader lacks. */
const refusedRight = (error: unknown) =>
  typeof error === "object" && error !== null && "status" in error && error.status === 403;

/**
 * The page of the queue `q` asks for; while another page is on its way, the one shown before stays (`stale`), so that
 * a change of the filters or a press on Next does not empty the table.
 */
export function useReportQueue(q: ReportQueueQuery): ReportQueuePage {
  const query = useQuery({ ...reportQueueQuery(useAppData(), q), placeholderData: keepPreviousData });
  const cursor = query.data?.next_cursor;
  return {
    reports: query.data?.items ?? [],
    next: cursor === undefined || cursor === "" ? undefined : cursor,
    status: query.status,
    stale: query.isPlaceholderData,
    forbidden: refusedRight(query.error),
    refetch: () => {
      void query.refetch();
    },
  };
}

/** Reads the page of the queue `q` asks for, for a route's loader. */
export async function loadReportQueue(data: AppData, q: ReportQueueQuery): Promise<void> {
  await data.queryClient.query(reportQueueQuery(data, q)).catch(() => undefined);
}

/** What a screen shows of a report: the report, once read, and where its read is. */
export type ReportRead = {
  readonly report: ReportDetail | undefined;
  /** `pending` until it is read, `error` when it failed, then `success`. */
  readonly status: "pending" | "error" | "success";
  /** The server has no such report (404). */
  readonly missing: boolean;
  /** The server refused the reader the report (403). */
  readonly forbidden: boolean;
  readonly refetch: () => void;
};

/** The report of `id` as a moderator opens it. */
export function useReport(id: string): ReportRead {
  const query = useQuery(reportQuery(useAppData(), id));
  const status = (query.error as { status?: unknown } | null)?.status;
  return {
    report: query.data,
    status: query.status,
    missing: status === 404,
    forbidden: status === 403,
    refetch: () => {
      void query.refetch();
    },
  };
}

/** Reads the report of `id`, for a route's loader. */
export async function loadReport(data: AppData, id: string): Promise<void> {
  await data.queryClient.query(reportQuery(data, id)).catch(() => undefined);
}

/**
 * How many reports are open, of the counts of what awaits the moderators; undefined until the server said, for a
 * reader without `manage_reports`, and with nothing asked while `enabled` is false (a reader without a session).
 */
export function useOpenReportCount(enabled = true): number | undefined {
  const data = useAppData();
  const viewer = useQuery({ ...viewerQuery(data), enabled }).data;
  const may = enabled && viewer?.permissions.includes("manage_reports") === true;
  return useQuery({ ...moderationCountsQuery(data), enabled: may }).data?.open_reports;
}

/**
 * Puts the report the server answered a write with into the cache, and reads again the queue, whose rows and filters
 * it changes, and the counts of what awaits the moderators.
 */
async function reportWritten(data: AppData, report: ReportDetail | undefined): Promise<ReportDetail> {
  if (report === undefined) throw new Error("app-data: a write of a report answered without the report");
  const { queryClient } = data;
  await queryClient.cancelQueries({ queryKey: reportQuery(data, report.id).queryKey });
  queryClient.setQueryData(reportQuery(data, report.id).queryKey, report);
  void queryClient.invalidateQueries({ predicate: ({ queryKey }) => queryKey[1] === "/admin/reports" });
  void queryClient.invalidateQueries({ queryKey: moderationCountsQuery(data).queryKey });
  return report;
}

/** What a moderator does to a report; each resolves with the report as the server answered it. */
export type ReportActions = {
  /** Makes the reader the moderator who handles it. */
  readonly assignToSelf: () => Promise<ReportDetail>;
  /** Leaves it to nobody, whoever handled it. */
  readonly unassign: () => Promise<ReportDetail>;
  /** Adds an internal note, which only the moderators read and nobody edits; a 422 names `body`. */
  readonly addNote: (body: string) => Promise<ReportDetail>;
  /** Resolves an open report with `outcome`; a resolved one keeps its own. */
  readonly resolve: (outcome: ReportOutcome) => Promise<ReportDetail>;
  /** Puts a resolved report back in the queue, open, keeping who handles it; an open one is a 409. */
  readonly reopen: () => Promise<ReportDetail>;
  /**
   * Sends a remote target's report to its server, with `comment` as its only text (none when empty): a 409
   * `/api/v1/problems/report-not-forwardable` when it cannot be, a 422 naming `comment` when the text is too long.
   */
  readonly forward: (comment: string) => Promise<ReportDetail>;
};

/** What a moderator does to the report of `id`, as `ReportActions`. */
export function reportActions(data: AppData, id: string): ReportActions {
  const path = { params: { path: { id } } };
  return {
    assignToSelf: async () =>
      reportWritten(data, (await data.api.POST("/admin/reports/{id}/assign_to_self", path)).data),
    unassign: async () => reportWritten(data, (await data.api.POST("/admin/reports/{id}/unassign", path)).data),
    addNote: async (body) =>
      reportWritten(data, (await data.api.POST("/admin/reports/{id}/notes", { ...path, body: { body } })).data),
    resolve: async (outcome) =>
      reportWritten(data, (await data.api.POST("/admin/reports/{id}/resolve", { ...path, body: { outcome } })).data),
    reopen: async () => reportWritten(data, (await data.api.POST("/admin/reports/{id}/reopen", path)).data),
    forward: async (comment) =>
      reportWritten(
        data,
        (
          await data.api.POST("/admin/reports/{id}/forward", {
            ...path,
            body: comment === "" ? {} : { comment },
          })
        ).data,
      ),
  };
}

/** What a moderator does to the report of `id`, for a screen (`reportActions`). */
export function useReportActions(id: string): ReportActions {
  return reportActions(useAppData(), id);
}

/** Whether `error` is the server's refusal to forward a report that cannot be forwarded (any longer). */
export function reportNotForwardable(error: unknown): boolean {
  return (
    typeof error === "object" &&
    error !== null &&
    "type" in error &&
    error.type === "/api/v1/problems/report-not-forwardable"
  );
}

/**
 * The comment of the report of `id` in `language`, from the cache of this session when it was asked for before: the
 * server sends it to a provider only while the instance lets it send a report's text (`reports_to_server`), and as a
 * private post. Rejects with a TranslationRefusal.
 */
export async function translateReportComment(
  data: AppData,
  id: string,
  language: string,
): Promise<ReportCommentTranslation> {
  try {
    return await data.queryClient.query({
      queryKey: ["report-comment-translation", id, language],
      queryFn: async () => {
        const { data: answer } = await data.api.POST("/admin/reports/{id}/comment/translation", {
          params: { path: { id } },
          body: { target_language: language },
        });
        if (answer === undefined) throw new Error("app-data: the translation answered without a body");
        return answer;
      },
      staleTime: Number.POSITIVE_INFINITY,
    });
  } catch (e) {
    throw new TranslationRefusal(translationRefusalOf(e), e);
  }
}

/**
 * The post of `statusId` that the report of `id` points at, in `language`, whatever its visibility and also when its
 * author deleted it, from the cache of this session when it was asked for before. Rejects with a TranslationRefusal.
 */
export async function translateReportedStatus(
  data: AppData,
  id: string,
  statusId: string,
  language: string,
): Promise<StatusTranslation> {
  try {
    return await data.queryClient.query({
      queryKey: ["reported-status-translation", id, statusId, language],
      queryFn: async () => {
        const { data: answer } = await data.api.POST("/admin/reports/{id}/statuses/{status_id}/translation", {
          params: { path: { id, status_id: statusId } },
          body: { target_language: language },
        });
        if (answer === undefined) throw new Error("app-data: the translation answered without a body");
        return answer;
      },
      staleTime: Number.POSITIVE_INFINITY,
    });
  } catch (e) {
    throw new TranslationRefusal(translationRefusalOf(e), e);
  }
}

/** The translations of a report's text, for a screen. */
export function useReportTranslations(id: string): {
  readonly comment: (language: string) => Promise<ReportCommentTranslation>;
  readonly status: (statusId: string, language: string) => Promise<StatusTranslation>;
} {
  const data = useAppData();
  return {
    comment: (language) => translateReportComment(data, id, language),
    status: (statusId, language) => translateReportedStatus(data, id, statusId, language),
  };
}

/**
 * The account of `id` that a report names beside its target and its reporter (who handles it, who wrote a note, who
 * took part in its room); undefined until it is read, or for none.
 */
export function useReportAccount(id: string | undefined): Account | undefined {
  return useQuery({ ...accountByIdQuery(useAppData(), id ?? ""), enabled: id !== undefined }).data;
}
