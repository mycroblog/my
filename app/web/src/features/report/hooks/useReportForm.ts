// SPDX-License-Identifier: AGPL-3.0-only
// The steps of reporting an account (FR-701, FR-709): the category, the rules it breaks (for a rule
// violation), the posts it is about, a comment for the moderators and, for an account of another server,
// whether to forward the report there. A report of a voice chat room (ADR-0100 §11) targets the room's host and
// points at the room instead of posts, so it has no posts step and is never forwarded. The draft lives here until the last step files it; nothing reaches
// the server before.
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { type SyntheticEvent, useState } from "react";
import { useApi } from "../../../lib/auth/SessionProvider";
import { errorMessage } from "../../../lib/errors";
import {
  type Account,
  cacheFiledReport,
  createReportMutation,
  type CreateReportRequest,
  type InstanceRule,
  instanceQueryOptions,
  type Report,
  type ReportCategory,
  reportablePostsQueryOptions,
  type Status,
} from "../api/reports";

/** The steps of the dialog: the four of the draft (`rules` only for a violation), then the confirmation. */
export type ReportStep = "category" | "rules" | "posts" | "comment" | "sent";

type ReportDraft = {
  category: ReportCategory | null;
  ruleIds: readonly string[];
  statusIds: readonly string[];
  comment: string;
  forward: boolean;
};

/** Data that a step loads: loading, failed with a localized reason and a retry, or there (S5). */
type Loadable<T> = { status: "loading" } | { status: "error"; message: string; retry: () => void; isRetrying: boolean } | { status: "ready"; value: T };

export type ReportForm = {
  step: ReportStep;
  /** The step's place among the steps of the draft, from 1, and how many there are for the chosen category. */
  position: { current: number; total: number };
  draft: ReportDraft;
  change: (change: Partial<ReportDraft>) => void;
  /** The rules in effect; a violation is offered only when there is one. */
  rules: Loadable<readonly InstanceRule[]>;
  /** The posts to choose from: the account's latest, with the post the report started from first. */
  posts: Loadable<readonly Status[]>;
  /** Whether the user went on from the rules without choosing one. */
  isRuleMissing: boolean;
  /** The domain of an account of another server, which a report can be forwarded to; null for a local one. */
  remoteDomain: string | null;
  /** Goes to the next step once the browser has checked the fields; the last step files the report. */
  next: (event: SyntheticEvent<HTMLFormElement>) => void;
  back: () => void;
  isPending: boolean;
  /** Why filing failed, localized; undefined otherwise. */
  error: string | undefined;
  /** The filed report, once the server has it. */
  report: Report | undefined;
};

/** What a step shows of a query: `value` is the part of its data that the step needs. */
function loadable<T>(query: { isError: boolean; error: unknown; refetch: () => unknown; isRefetching: boolean }, value: T | undefined): Loadable<T> {
  if (value !== undefined) return { status: "ready", value };
  if (query.isError)
    return {
      status: "error",
      message: errorMessage(query.error),
      retry: () => {
        void query.refetch();
      },
      isRetrying: query.isRefetching,
    };
  return { status: "loading" };
}

/** A voice chat room that a report points at, by its ID and its name as the user sees it now. */
export type ReportedVoiceRoom = { id: string; name: string };

/** The steps of the draft for `category`; a room's report has no posts step. */
function stepsOf(category: ReportCategory | null, isRoom: boolean): readonly ReportStep[] {
  const steps: readonly ReportStep[] = category === "violation" ? ["category", "rules", "posts", "comment"] : ["category", "posts", "comment"];
  return isRoom ? steps.filter((step) => step !== "posts") : steps;
}

/** The request of a draft whose category is chosen; what does not apply is left out. */
function requestOf(account: Account, draft: ReportDraft, category: ReportCategory, remoteDomain: string | null, room: ReportedVoiceRoom | undefined): CreateReportRequest {
  const comment = draft.comment.trim();
  if (room !== undefined) {
    // A room's report points at the room, not at posts, and stays on this server (its host is local).
    return {
      account_id: account.id,
      category,
      voice_room_id: room.id,
      ...(category === "violation" && { rule_ids: [...draft.ruleIds] }),
      ...(comment !== "" && { comment }),
      forward: false,
    };
  }
  return {
    account_id: account.id,
    category,
    ...(draft.statusIds.length > 0 && { status_ids: [...draft.statusIds] }),
    ...(category === "violation" && { rule_ids: [...draft.ruleIds] }),
    ...(comment !== "" && { comment }),
    // Forwarding means nothing for a local account.
    forward: remoteDomain !== null && draft.forward,
  };
}

/**
 * The report of `account`, started from its post `status` (preselected), from its profile, or from the page of
 * the voice chat room `room` that it hosts. The steps are kept here, so the dialog that shows them only renders
 * one at a time.
 */
export function useReportForm(account: Account, status: Status | undefined, room?: ReportedVoiceRoom): ReportForm {
  const api = useApi();
  const queryClient = useQueryClient();
  const [step, setStep] = useState<ReportStep>("category");
  const [draft, setDraft] = useState<ReportDraft>(() => ({
    category: null,
    ruleIds: [],
    statusIds: status === undefined ? [] : [status.id],
    comment: "",
    forward: false,
  }));
  const [isRuleMissing, setIsRuleMissing] = useState(false);
  const instance = useQuery(instanceQueryOptions(api));
  // A room's report cites no posts, so it does not load them.
  const latest = useQuery({ ...reportablePostsQueryOptions(api, account.id), enabled: room === undefined });
  const file = useMutation({ ...createReportMutation(api), onSuccess: () => cacheFiledReport(queryClient) });

  const rules = loadable(instance, instance.data?.rules);
  const latestPosts = latest.data?.items;
  const posts = loadable(
    latest,
    latestPosts && (status === undefined || latestPosts.some((post) => post.id === status.id) ? latestPosts : [status, ...latestPosts]),
  );
  const remoteDomain = account.acct.includes("@") ? account.domain : null;
  const steps = stepsOf(draft.category, room !== undefined);
  const index = steps.indexOf(step);

  return {
    step,
    position: { current: index + 1, total: steps.length },
    draft,
    change: (change) => {
      if (change.ruleIds !== undefined && change.ruleIds.length > 0) setIsRuleMissing(false);
      setDraft({ ...draft, ...change });
    },
    rules,
    posts,
    isRuleMissing,
    remoteDomain,
    next: (event) => {
      event.preventDefault();
      if (draft.category === null || file.isPending) return;
      if (step === "rules" && draft.ruleIds.length === 0) {
        setIsRuleMissing(true);
        return;
      }
      const following = steps[index + 1];
      if (following !== undefined) {
        setStep(following);
        return;
      }
      file.mutate(requestOf(account, draft, draft.category, remoteDomain, room), {
        onSuccess: () => {
          setStep("sent");
        },
      });
    },
    back: () => {
      const previous = steps[index - 1];
      if (previous === undefined) return;
      file.reset();
      setStep(previous);
    },
    isPending: file.isPending,
    error: file.isError ? errorMessage(file.error) : undefined,
    report: file.data,
  };
}
