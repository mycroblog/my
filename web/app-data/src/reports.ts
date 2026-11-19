// SPDX-License-Identifier: AGPL-3.0-only
// The reader's reports to the moderators (FR-701, FR-709): an account, with some of its posts the reader sees, a voice
// chat room of this server (its host the account) or an event of this server (its organizer the account), with why
// (the category, and the instance's rules it breaks for a violation), a comment, and for a remote account whether to
// forward it to its server. Nothing the reported account reads names the reporter; what the reader is told afterwards
// is only that the moderators resolved it (the `report_resolved` notification), never how.
import { ProblemError, type components } from "@mycroblog/mykit";
import type { AppData } from "./app-data";
import { useAppData } from "./provider";

/** Why an account is reported: `spam`, `legal` (illegal content), `violation` (of the cited rules), `other`. */
export type ReportCategory = components["schemas"]["ReportCategory"];

/** A report the reader filed, as the server tells it to them. */
export type OwnReport = components["schemas"]["Report"];

/** A rule of the instance, which a report of a violation cites. */
export type InstanceRule = components["schemas"]["InstanceRule"];

/** What a report sends. */
export type CreateReport = components["schemas"]["CreateReportRequest"];

/**
 * Files a report, and marks the list of the reader's reports, and for a moderator the queue of reports and the count
 * of what awaits them. It resolves with the report as the server filed it, and
 * rejects with the server's problem: 422 for a target or a post the reader may not report, a rule not in effect or a
 * comment too long, 429 past the hourly limit.
 */
export async function fileReport(data: AppData, report: CreateReport): Promise<OwnReport> {
  const { data: filed } = await data.api.POST("/reports", { body: report });
  if (filed === undefined) throw new Error("app-data: the report answered without a report");
  // A reader who moderates sees it join the queue and its count too.
  await data.queryClient.invalidateQueries({
    predicate: ({ queryKey }) =>
      queryKey[1] === "/reports" || queryKey[1] === "/admin/counts" || queryKey[1] === "/admin/reports",
  });
  return filed;
}

/** Files a report, from a screen (`fileReport`). */
export function useFileReport(): (report: CreateReport) => Promise<OwnReport> {
  const data = useAppData();
  return (report) => fileReport(data, report);
}

/**
 * The member of the request a refused report is about, when the server named one (a 422's `field`: `comment`,
 * `status_ids`, `rule_ids`, `account_id`); null for none, or for another failure.
 */
export function reportFaultField(error: unknown): string | null {
  if (!(error instanceof ProblemError) || error.status !== 422) return null;
  return error.problem.field ?? null;
}
