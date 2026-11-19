// SPDX-License-Identifier: AGPL-3.0-only
// What the staff's screens read of a list or of one thing, as one shape for every screen of accounts, appeals, sign-ups
// and automated reviews (FR-703–FR-721): a page of a list by the server's cursor, the page shown before staying while
// another is on its way; one thing, with whether the server has it or refuses it to the reader; and what a refused
// write of theirs means, by the problem's type, never its status alone.
import { ProblemError } from "@mycroblog/mykit";
import type { AppData } from "./app-data";

/** What a screen shows of a page of a list: its items, the next page's cursor, and where its read is. */
export type StaffPage<T> = {
  /** The page's items; none until it is read. */
  readonly items: readonly T[];
  /** The cursor of the next page; none on the last one. */
  readonly next: string | undefined;
  /** `pending` until the page is read, `error` when it failed, then `success`. */
  readonly status: "pending" | "error" | "success";
  /** The page shows what the reader asked before while the page they asked for now is on its way. */
  readonly stale: boolean;
  /** The server refused the reader the list (403): their roles no longer have its permission. */
  readonly forbidden: boolean;
  readonly refetch: () => void;
};

/** What a screen shows of one thing: the thing, once read, and where its read is. */
export type StaffOne<T> = {
  readonly value: T | undefined;
  /** `pending` until it is read, `error` when it failed, then `success`. */
  readonly status: "pending" | "error" | "success";
  /** The server has no such thing (404). */
  readonly missing: boolean;
  /** The server refused the reader the thing (403). */
  readonly forbidden: boolean;
  readonly refetch: () => void;
};

/** Whether `error` is the server's answer of `status`. */
const answered = (error: unknown, status: number) => error instanceof ProblemError && error.status === status;

/** What a query says of where its read is (TanStack Query's result, whatever its error's type). */
type Query<D> = {
  readonly data: D | undefined;
  readonly status: "pending" | "error" | "success";
  readonly isPlaceholderData: boolean;
  readonly error: unknown;
  readonly refetch: () => unknown;
};

/** The page a query of a list holds, as a screen shows it. */
export function pageOf<T>(query: Query<{ items: T[]; next_cursor?: string }>): StaffPage<T> {
  const cursor = query.data?.next_cursor;
  return {
    items: query.data?.items ?? [],
    next: cursor === undefined || cursor === "" ? undefined : cursor,
    status: query.status,
    stale: query.isPlaceholderData,
    forbidden: answered(query.error, 403),
    refetch: () => {
      void query.refetch();
    },
  };
}

/** The thing a query holds, as a screen shows it. */
export function oneOf<T>(query: Query<T>): StaffOne<T> {
  return {
    value: query.data,
    status: query.status,
    missing: answered(query.error, 404),
    forbidden: answered(query.error, 403),
    refetch: () => {
      void query.refetch();
    },
  };
}

/**
 * What a refused write of a staff screen means, by the problem's type: the target ranks as high as the reader
 * (`outranks`), the only administrator would be locked out (`last_admin`), an appeal or a decision was decided the other
 * way meanwhile (`decided`), a ban whose account is deleted (`not_appealable`), a decision with no effect to cancel
 * (`not_overridable`), an email that is not suppressed (`not_suppressed`), an account without two-factor
 * (`no_two_factor`), registrations closed (`registration_closed`) or the service closing (`closing`), an event
 * cancelled already (`event_closed`), the reader's permission gone (`forbidden`), the thing gone (`missing`), and any
 * other failure (`failed`). A 422 that names a member is `refusedMember`'s (compose.ts).
 */
export type StaffRefusal =
  | "outranks"
  | "last_admin"
  | "decided"
  | "not_appealable"
  | "not_overridable"
  | "not_suppressed"
  | "no_two_factor"
  | "registration_closed"
  | "closing"
  | "event_closed"
  | "forbidden"
  | "missing"
  | "failed";

const refusalTypes: Readonly<Record<string, StaffRefusal>> = {
  "/api/v1/problems/target-outranks-actor": "outranks",
  "/api/v1/problems/last-administrator": "last_admin",
  "/api/v1/problems/appeal-decided": "decided",
  "/api/v1/problems/not-appealable": "not_appealable",
  "/api/v1/problems/hook-decision-not-overridable": "not_overridable",
  "/api/v1/problems/email-not-suppressed": "not_suppressed",
  "/api/v1/problems/two-factor-not-enabled": "no_two_factor",
  "/api/v1/problems/registration-closed": "registration_closed",
  "/api/v1/problems/service-closing": "closing",
  "/api/v1/problems/event-closed": "event_closed",
};

/** What the refusal of a staff screen's write, `error`, means (`StaffRefusal`). */
export function staffRefusalOf(error: unknown): StaffRefusal {
  if (!(error instanceof ProblemError)) return "failed";
  const typed = refusalTypes[error.type];
  if (typed !== undefined) return typed;
  if (error.status === 403) return "forbidden";
  if (error.status === 404) return "missing";
  return "failed";
}

/** Reads again every query of the staff's under `paths` (a list and the things it lists), and what awaits them. */
export function staffChanged(data: AppData, paths: readonly string[]): void {
  void data.queryClient.invalidateQueries({
    predicate: ({ queryKey }) =>
      typeof queryKey[1] === "string" &&
      (queryKey[1] === "/admin/counts" || paths.some((p) => (queryKey[1] as string).startsWith(p))),
  });
}
