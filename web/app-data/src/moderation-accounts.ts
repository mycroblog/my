// SPDX-License-Identifier: AGPL-3.0-only
// The accounts as a holder of `manage_accounts` moderates them (FR-703, FR-707, FR-708, FR-713, FR-714, ADR-0058): the
// search by name, domain, origin, state, email, email domain or IP; an account's details, its email and IPs shown on
// demand alone (§9), each showing recorded by the server; the actions on it, a page at a time, one action, imposing one
// and lifting one; the appeals of the actions, upheld or overturned; lifting an email suppression and refetching a
// remote account's posts; and the login recovery of `manage_login_recovery` (a password reset link, two-factor
// removed). A moderator of reports (`manage_reports`) also ends a voice chat room and cancels an event a report points
// at. Every write reads again what it changes (the lists, the things, the counts of what awaits the moderators).
import type { components } from "@mycroblog/mykit";
import { keepPreviousData, useQueries, useQuery } from "@tanstack/react-query";
import type { AppData } from "./app-data";
import { useAppData } from "./provider";
import { moderationCountsQuery } from "./rights";
import { oneOf, pageOf, staffChanged, type StaffOne, type StaffPage } from "./staff-reads";
import { accountByIdQuery, viewerQuery, type Account } from "./accounts";
import { reportQuery, type ReportDetail } from "./moderation-reports";

/** An account as the moderators find it. */
export type ModeratedAccount = components["schemas"]["AdminAccount"];
/** An account as the moderators open it. */
export type ModeratedAccountDetail = components["schemas"]["AdminAccountDetail"];
/** The email and IPs of a local account, shown on demand. */
export type AccountPrivateData = components["schemas"]["AdminAccountPrivateData"];
/** The state of an account, as the moderators see it. */
export type ModeratedAccountState = components["schemas"]["AdminAccountState"];
/** A capability an action takes away. */
export type AccountRestriction = components["schemas"]["AdminAccountRestriction"];
/** A moderation action on an account. */
export type AccountAction = components["schemas"]["AdminAccountAction"];
/** The kind of a moderation action. */
export type AccountActionKind = components["schemas"]["AdminAccountActionKind"];
/** The kind of reason of a moderation action. */
export type AccountActionCategory = components["schemas"]["AdminAccountActionCategory"];
/** A moderation action to take. */
export type NewAccountAction = components["schemas"]["CreateAccountActionRequest"];
/** A local user's appeal of an action on their account, with the action. */
export type ModeratedAppeal = components["schemas"]["AdminAppeal"];
/** Where an appeal stands. */
export type ModeratedAppealState = components["schemas"]["ModerationAppealState"];
/** A password reset link, shown once. */
export type PasswordResetLink = components["schemas"]["AdminPasswordResetLink"];

/** The states of an account, in the server's order. */
export const moderatedAccountStates = [
  "active",
  "pending",
  "rejected",
  "limited",
  "disabled",
  "deactivated",
  "suspended",
  "deleted",
] as const satisfies readonly ModeratedAccountState[];

/** The kinds of an action on a local account, the mildest first. */
export const localActionKinds = [
  "warn",
  "mark_statuses_sensitive",
  "delete_statuses",
  "force_sensitive",
  "limit",
  "restrict_interaction",
  "restrict_posting",
  "disable_login",
  "suspend",
  "ban",
] as const satisfies readonly AccountActionKind[];

/** The kinds of an action on a remote account. */
export const remoteActionKinds = [
  "force_sensitive",
  "limit",
  "suspend",
] as const satisfies readonly AccountActionKind[];

/** The kinds of an action on posts, which name the posts and take no expiry. */
export const postActionKinds: ReadonlySet<AccountActionKind> = new Set(["mark_statuses_sensitive", "delete_statuses"]);

/** The kinds of reason of an action, in the server's order. */
export const actionCategories = [
  "spam",
  "legal",
  "violation",
  "other",
] as const satisfies readonly AccountActionCategory[];

/** The states of an appeal, pending first. */
export const appealStates = ["pending", "upheld", "overturned"] as const satisfies readonly ModeratedAppealState[];

/** The most characters of an action's reason and note, of the reason a lift gives, and of an appeal's decision. */
export const actionTextLimit = 5000;
export const appealReasonLimit = 2000;
/** The most posts an action on posts names, and the longest an action lasts (ten years, in seconds). */
export const actionStatusLimit = 100;
export const actionLongest = 315_360_000;
/** The most characters of the reason a moderator ends a room or cancels an event with. */
export const placeReasonLimit = 500;

/**
 * What the search of accounts asks for: a username's start, a domain, local or remote accounts, a state, and one of an
 * email, an email domain and an IP; and the page after `cursor`.
 */
export type AccountSearch = {
  readonly username?: string | undefined;
  readonly domain?: string | undefined;
  readonly origin?: "local" | "remote" | undefined;
  readonly state?: ModeratedAccountState | undefined;
  readonly email?: string | undefined;
  readonly email_domain?: string | undefined;
  readonly ip?: string | undefined;
  readonly cursor?: string | undefined;
};

/** The members of `q` that are given, as the API's query. */
function given(q: AccountSearch) {
  const words = (key: "username" | "domain" | "email" | "email_domain" | "ip" | "cursor") => {
    const value = q[key];
    return value === undefined || value === "" ? {} : { [key]: value };
  };
  return {
    ...words("username"),
    ...words("domain"),
    ...(q.origin === undefined ? {} : { origin: q.origin }),
    ...(q.state === undefined ? {} : { state: q.state }),
    ...words("email"),
    ...words("email_domain"),
    ...words("ip"),
    ...words("cursor"),
  };
}

/** A page of the accounts `q` asks for. */
function moderatedAccountSearchQuery({ queries }: AppData, q: AccountSearch) {
  return queries.queryOptions("get", "/admin/accounts", { params: { query: given(q) } });
}

/** The page of the accounts `q` asks for; the page shown before stays while the next is on its way. */
export function useModeratedAccountSearch(q: AccountSearch): StaffPage<ModeratedAccount> {
  return pageOf(useQuery({ ...moderatedAccountSearchQuery(useAppData(), q), placeholderData: keepPreviousData }));
}

/** Reads the page of the accounts `q` asks for, for a route's loader. */
export async function loadModeratedAccountSearch(data: AppData, q: AccountSearch): Promise<void> {
  await data.queryClient.query(moderatedAccountSearchQuery(data, q)).catch(() => undefined);
}

/** An account as the moderators open it. */
function moderatedAccountQuery({ queries }: AppData, id: string) {
  return queries.queryOptions("get", "/admin/accounts/{id}", { params: { path: { id } } });
}

/** The account of `id` as the moderators open it. */
export function useModeratedAccount(id: string): StaffOne<ModeratedAccountDetail> {
  return oneOf(useQuery(moderatedAccountQuery(useAppData(), id)));
}

/** Reads the account of `id` as the moderators open it, for a route's loader. */
export async function loadModeratedAccount(data: AppData, id: string): Promise<void> {
  await data.queryClient.query(moderatedAccountQuery(data, id)).catch(() => undefined);
}

/** A page of the actions on an account, newest first, those in effect alone with `active`. */
function accountActionsQuery({ queries }: AppData, id: string, active: boolean, cursor: string | undefined) {
  return queries.queryOptions("get", "/admin/accounts/{id}/actions", {
    params: { path: { id }, query: { ...(active ? { active } : {}), ...(cursor === undefined ? {} : { cursor }) } },
  });
}

/** The page after `cursor` of the actions on the account of `id`, those in effect alone with `active`. */
export function useAccountActions(id: string, active: boolean, cursor: string | undefined): StaffPage<AccountAction> {
  return pageOf(
    useQuery({ ...accountActionsQuery(useAppData(), id, active, cursor), placeholderData: keepPreviousData }),
  );
}

/** Reads a page of the actions on an account, for a route's loader. */
export async function loadAccountActions(
  data: AppData,
  id: string,
  active: boolean,
  cursor: string | undefined,
): Promise<void> {
  await data.queryClient.query(accountActionsQuery(data, id, active, cursor)).catch(() => undefined);
}

/** One action on an account. */
function accountActionQuery({ queries }: AppData, id: string) {
  return queries.queryOptions("get", "/admin/account_actions/{id}", { params: { path: { id } } });
}

/** The action of `id`; nothing asked without one. */
export function useAccountAction(id: string | undefined): StaffOne<AccountAction> {
  return oneOf(useQuery({ ...accountActionQuery(useAppData(), id ?? ""), enabled: id !== undefined }));
}

/** Reads the action of `id`, for a route's loader. */
export async function loadAccountAction(data: AppData, id: string): Promise<void> {
  await data.queryClient.query(accountActionQuery(data, id)).catch(() => undefined);
}

/** What the accounts' writes change: the accounts, their actions, the appeals, the reports an action resolves. */
const accountsChanged = (data: AppData) => {
  staffChanged(data, ["/admin/accounts", "/admin/account_actions", "/admin/appeals", "/admin/reports"]);
};

/** The answer of a write, which must have one. */
function answerOf<T>(answer: T | undefined): T {
  if (answer === undefined) throw new Error("app-data: a write of the moderators answered without a body");
  return answer;
}

/** What a moderator does to an account and its actions; each rejects with the server's problem. */
export type AccountWrites = {
  /** Takes an action on the account of `id`, and resolves with it. */
  readonly impose: (id: string, action: NewAccountAction) => Promise<AccountAction>;
  /** Lifts the action of `id`, with `reason` (none when empty), and resolves with it. */
  readonly lift: (id: string, reason: string) => Promise<AccountAction>;
  /** The email and IPs of the local account of `id`, a showing the server records. */
  readonly reveal: (id: string) => Promise<AccountPrivateData>;
  /** Lets the server send email again to the local account of `id`. */
  readonly liftEmailSuppression: (id: string) => Promise<void>;
  /** Queues the refetch of the posts of the remote account of `id` (`manage_domains`). */
  readonly refetchStatuses: (id: string) => Promise<void>;
  /** Issues a password reset link for the local account of `id` (`manage_login_recovery`), shown once. */
  readonly passwordResetLink: (id: string) => Promise<PasswordResetLink>;
  /** Removes the two-factor authentication of the local account of `id` (`manage_login_recovery`). */
  readonly disableTwoFactor: (id: string) => Promise<void>;
};

/** What a moderator does to accounts, as `AccountWrites`. */
function accountWrites(data: AppData): AccountWrites {
  const path = (id: string) => ({ params: { path: { id } } });
  return {
    impose: async (id, action) => {
      const { data: answer } = await data.api.POST("/admin/accounts/{id}/actions", { ...path(id), body: action });
      const imposed = answerOf(answer);
      data.queryClient.setQueryData(accountActionQuery(data, imposed.id).queryKey, imposed);
      accountsChanged(data);
      return imposed;
    },
    lift: async (id, reason) => {
      const { data: answer } = await data.api.POST("/admin/account_actions/{id}/lift", {
        ...path(id),
        body: reason === "" ? {} : { reason },
      });
      const lifted = answerOf(answer);
      await data.queryClient.cancelQueries({ queryKey: accountActionQuery(data, id).queryKey });
      data.queryClient.setQueryData(accountActionQuery(data, id).queryKey, lifted);
      accountsChanged(data);
      return lifted;
    },
    reveal: async (id) => answerOf((await data.api.POST("/admin/accounts/{id}/reveal", path(id))).data),
    liftEmailSuppression: async (id) => {
      await data.api.POST("/admin/accounts/{id}/email_suppression/lift", path(id));
      staffChanged(data, ["/admin/accounts"]);
    },
    refetchStatuses: async (id) => {
      await data.api.POST("/admin/accounts/{id}/refetch_statuses", path(id));
    },
    passwordResetLink: async (id) =>
      answerOf((await data.api.POST("/admin/accounts/{id}/password_reset_link", path(id))).data),
    disableTwoFactor: async (id) => {
      await data.api.POST("/admin/accounts/{id}/two_factor/disable", path(id));
      staffChanged(data, ["/admin/accounts"]);
    },
  };
}

/** What a moderator does to accounts, for a screen (`accountWrites`). */
export function useAccountWrites(): AccountWrites {
  return accountWrites(useAppData());
}

/** A page of the appeals of one state (every state where left out), newest first. */
function moderatedAppealsQuery(
  { queries }: AppData,
  state: ModeratedAppealState | undefined,
  cursor: string | undefined,
) {
  return queries.queryOptions("get", "/admin/appeals", {
    params: { query: { ...(state === undefined ? {} : { state }), ...(cursor === undefined ? {} : { cursor }) } },
  });
}

/** The page after `cursor` of the appeals in `state`; the page shown before stays while the next is on its way. */
export function useModeratedAppeals(
  state: ModeratedAppealState | undefined,
  cursor: string | undefined,
): StaffPage<ModeratedAppeal> {
  return pageOf(useQuery({ ...moderatedAppealsQuery(useAppData(), state, cursor), placeholderData: keepPreviousData }));
}

/** Reads a page of the appeals, for a route's loader. */
export async function loadModeratedAppeals(
  data: AppData,
  state: ModeratedAppealState | undefined,
  cursor: string | undefined,
): Promise<void> {
  await data.queryClient.query(moderatedAppealsQuery(data, state, cursor)).catch(() => undefined);
}

/** One appeal. */
function moderatedAppealQuery({ queries }: AppData, id: string) {
  return queries.queryOptions("get", "/admin/appeals/{id}", { params: { path: { id } } });
}

/** The appeal of `id`, with its action. */
export function useModeratedAppeal(id: string): StaffOne<ModeratedAppeal> {
  return oneOf(useQuery(moderatedAppealQuery(useAppData(), id)));
}

/** Reads the appeal of `id`, for a route's loader. */
export async function loadModeratedAppeal(data: AppData, id: string): Promise<void> {
  await data.queryClient.query(moderatedAppealQuery(data, id)).catch(() => undefined);
}

/**
 * Decides the appeal of `id`: `uphold` keeps the action, `overturn` lifts it (and cancels a ban's deletion), with
 * `reason` told to the user (none when empty); resolves with the appeal as decided.
 */
async function decideAppeal(
  data: AppData,
  id: string,
  decision: "uphold" | "overturn",
  reason: string,
): Promise<ModeratedAppeal> {
  const init = { params: { path: { id } }, body: reason === "" ? {} : { public_reason: reason } };
  const { data: answer } =
    decision === "uphold"
      ? await data.api.POST("/admin/appeals/{id}/uphold", init)
      : await data.api.POST("/admin/appeals/{id}/overturn", init);
  const decided = answerOf(answer);
  await data.queryClient.cancelQueries({ queryKey: moderatedAppealQuery(data, id).queryKey });
  data.queryClient.setQueryData(moderatedAppealQuery(data, id).queryKey, decided);
  accountsChanged(data);
  return decided;
}

/** Decides an appeal, for a screen (`decideAppeal`). */
export function useDecideAppeal(): (
  id: string,
  decision: "uphold" | "overturn",
  reason: string,
) => Promise<ModeratedAppeal> {
  const data = useAppData();
  return (id, decision, reason) => decideAppeal(data, id, decision, reason);
}

/**
 * How many registrations wait for approval and appeals for a decision, of the counts of what awaits the moderators;
 * each undefined until the server said, for a reader without `manage_accounts`, and with nothing asked while `enabled`
 * is false (a reader without a session).
 */
export function useAccountCounts(enabled = true): {
  readonly registrations: number | undefined;
  readonly appeals: number | undefined;
} {
  const data = useAppData();
  const viewer = useQuery({ ...viewerQuery(data), enabled }).data;
  const may = enabled && viewer?.permissions.includes("manage_accounts") === true;
  const counts = useQuery({ ...moderationCountsQuery(data), enabled: may }).data;
  return { registrations: counts?.pending_registrations, appeals: counts?.pending_appeals };
}

/** What a moderator of reports does to the place a report points at; each rejects with the server's problem. */
export type PlaceWrites = {
  /** Ends the open voice chat room of `id` for everyone, with `reason` for the moderators (none when empty). */
  readonly endRoom: (id: string, reason: string) => Promise<void>;
  /** Cancels the event of `id`, with `reason` the event shows (none when empty). */
  readonly cancelEvent: (id: string, reason: string) => Promise<void>;
};

/** What a moderator of reports does to the place a report points at, for a screen; the reports are read again. */
export function usePlaceWrites(): PlaceWrites {
  const data = useAppData();
  const body = (reason: string) => (reason === "" ? {} : { reason });
  return {
    endRoom: async (id, reason) => {
      await data.api.POST("/admin/voice_rooms/{id}/end", { params: { path: { id } }, body: body(reason) });
      staffChanged(data, ["/admin/reports", "/voice_rooms"]);
    },
    cancelEvent: async (id, reason) => {
      await data.api.POST("/admin/events/{id}/cancel", { params: { path: { id } }, body: body(reason) });
      staffChanged(data, ["/admin/reports", "/events"]);
    },
  };
}

/**
 * The accounts of `ids` that a list of the moderators names by ID alone (who appealed, who decided), each read once
 * and kept by the cache; an account not read yet, or gone, is not in the map.
 */
export function useStaffAccounts(ids: readonly (string | undefined)[]): ReadonlyMap<string, Account> {
  const data = useAppData();
  const unique = [...new Set(ids.filter((id): id is string => id !== undefined))];
  const results = useQueries({ queries: unique.map((id) => accountByIdQuery(data, id)) });
  return new Map(unique.flatMap((id, i) => (results[i]?.data === undefined ? [] : [[id, results[i].data] as const])));
}

/**
 * The report of `id` an action follows from (the account's screen opened from a report), whose posts an action on posts
 * names; undefined without one, until it is read, or for a reader without `manage_reports`.
 */
export function useActionReport(id: string | undefined): ReportDetail | undefined {
  const data = useAppData();
  const may = useQuery(viewerQuery(data)).data?.permissions.includes("manage_reports") === true;
  return useQuery({ ...reportQuery(data, id ?? ""), enabled: id !== undefined && id !== "" && may }).data;
}
