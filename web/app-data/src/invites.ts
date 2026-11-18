// SPDX-License-Identifier: AGPL-3.0-only
// The invites the reader creates as a member (ADR-0106 §14–§19): their list, newest first, a page at a time, with what the
// reader may do now (`canCreate`, and why not: `cannotCreateReason`; how many usable invites they hold against the
// instance's caps), one of them by its ID, making one, and revoking one. A write reads the list again, since it
// changes the count of the usable ones, and so does a write the server refused, since a refusal says the list the
// screen holds is no longer the server's. Whether the settings offer the invites at all (`useMayInvite`) is the
// list's `can_create`: the instance says only that it takes no new account, and then nothing is asked. The staff's
// invites are not here (`/admin/invites`). On a one-person server every operation is a 404 of the type
// `/api/v1/problems/single-user-mode`.
import { ProblemError, type components } from "@mycroblog/mykit";
import { useQuery, type InfiniteData } from "@tanstack/react-query";
import { useEffect } from "react";
import type { AppData } from "./app-data";
import { instanceQuery, type Instance } from "./instance";
import { pagedQuery, usePagedListOf, type PagedList } from "./paged";
import { useAppData } from "./provider";

/** An invite of the reader's: its code, its address to send, its uses and its state now. */
export type OwnInvite = components["schemas"]["OwnInvite"];

/** The caps and defaults of the invites members create. */
export type MemberInviteLimits = components["schemas"]["MemberInviteLimits"];

/** What an invite allows now: `usable`, `revoked`, `expired`, `exhausted` or `creator_ineligible`. */
export type InviteState = components["schemas"]["AdminInviteState"];

/**
 * What an invite allows a person who would sign up with it now: its state (`InviteState`), or `registration_closed`
 * for one the server reports `usable` while it takes no new account, whose sign-up page then takes no invite
 * (ADR-0106 §17): the server judges an invite by its creator's standing alone, whatever the registration mode.
 */
export type InviteStateNow = InviteState | "registration_closed";

/** What `invite` allows now (`InviteStateNow`), on a server that takes no new account when `registrationClosed`. */
export function inviteStateNow(invite: OwnInvite, registrationClosed: boolean): InviteStateNow {
  return invite.state === "usable" && registrationClosed ? "registration_closed" : invite.state;
}

/**
 * Why the reader may not create invites now: the server takes no new account (`registration_closed`), it lets only
 * some accounts create invites (`not_permitted`), the reader's registration is not approved (`not_approved`), or a
 * moderation action is on their account (`restricted`).
 */
export type InvitesNotAllowedReason = components["schemas"]["InvitesNotAllowedReason"];

const notAllowedReasons: readonly string[] = [
  "registration_closed",
  "not_permitted",
  "not_approved",
  "restricted",
] satisfies readonly InvitesNotAllowedReason[];

/** A page of the reader's invites, with what the reader may do. */
type OwnInvitePage = components["schemas"]["OwnInvitePage"];

/** What a member sets on a new invite; what is left out takes the instance's default, within its caps. */
export type CreateOwnInvite = components["schemas"]["CreateOwnInviteRequest"];

/** The reader's invites, newest first, a page at a time. */
export function ownInvitesQuery({ api, queries }: AppData) {
  return pagedQuery<OwnInvite, OwnInvitePage>(
    queries.queryOptions("get", "/invites", { params: { query: {} } }).queryKey,
    async (cursor, signal) => {
      const { data } = await api.GET("/invites", {
        params: { query: cursor === undefined ? {} : { cursor } },
        signal,
      });
      return data;
    },
  );
}

/** Reads the list again, from its first page: a write changed an invite and the count of the usable ones. */
async function invitesWritten(data: AppData): Promise<void> {
  await data.queryClient.invalidateQueries({ queryKey: ownInvitesQuery(data).queryKey });
}

/** Whether `error` is the server's refusal of a new invite because the reader may not create invites now (the 403). */
export function invitesNotAllowed(error: unknown): boolean {
  return error instanceof ProblemError && error.type === "/api/v1/problems/invites-not-allowed";
}

/**
 * Why the reader may not create invites, as the refusal `error` says it (its `reason`); undefined for a refusal that
 * names no cause this client knows, and for any other failure.
 */
export function invitesNotAllowedReason(error: unknown): InvitesNotAllowedReason | undefined {
  if (!(error instanceof ProblemError) || !invitesNotAllowed(error)) return undefined;
  const { reason } = error.problem;
  return typeof reason === "string" && notAllowedReasons.includes(reason)
    ? (reason as InvitesNotAllowedReason)
    : undefined;
}

/**
 * The value of a new invite the server refused as over what it allows now (the 422's `field`): its uses (`max_uses`) or
 * the time it works (`expires_in`); undefined for any other failure.
 */
export function inviteValueRefused(error: unknown): "max_uses" | "expires_in" | undefined {
  if (!(error instanceof ProblemError) || error.status !== 422) return undefined;
  const { field } = error.problem;
  return field === "max_uses" || field === "expires_in" ? field : undefined;
}

/**
 * What a refused write tells of the list: a reader who may not create invites reads so at once, with the cause, in
 * the list the cache holds, and the list is read again, since the server refused by what it holds now (the right, the
 * count of the usable ones, the caps).
 */
function inviteRefused(data: AppData, error: unknown): void {
  if (!(error instanceof ProblemError)) return;
  if (invitesNotAllowed(error)) {
    const reason = invitesNotAllowedReason(error);
    data.queryClient.setQueryData<InfiniteData<OwnInvitePage, string | undefined>>(
      ownInvitesQuery(data).queryKey,
      (now) => {
        const [first, ...rest] = now?.pages ?? [];
        if (now === undefined || first === undefined) return now;
        const { cannot_create_reason: _before, ...page } = first;
        const told = { ...page, can_create: false, ...(reason === undefined ? {} : { cannot_create_reason: reason }) };
        return { ...now, pages: [told, ...rest] };
      },
    );
  }
  void invitesWritten(data);
}

/**
 * Makes an invite. It rejects with the server's problem: the reader may not create invites (403
 * `/api/v1/problems/invites-not-allowed`, whose `reason` says why: `invitesNotAllowedReason`, which the list then
 * holds as `cannotCreateReason`), holds as many usable ones as the cap (409 `too-many-invites`), a value over a cap
 * (422, `field` names `max_uses` or `expires_in`). After a refusal the list is read again.
 */
export async function createOwnInvite(data: AppData, request: CreateOwnInvite = {}): Promise<OwnInvite> {
  const { data: made } = await data.api.POST("/invites", { body: request }).catch((error: unknown) => {
    inviteRefused(data, error);
    throw error;
  });
  if (made === undefined) throw new Error("app-data: /invites answered without the invite");
  await invitesWritten(data);
  return made;
}

/**
 * Revokes an invite, which stops working at once, and returns it as the server left it. Revoking a revoked one changes
 * nothing. It rejects with the server's problem (404: not an invite of the reader's).
 */
export async function revokeOwnInvite(data: AppData, id: string): Promise<OwnInvite> {
  const { data: revoked } = await data.api.POST("/invites/{id}/revoke", { params: { path: { id } } });
  if (revoked === undefined) throw new Error("app-data: /invites/{id}/revoke answered without the invite");
  await invitesWritten(data);
  return revoked;
}

/** The reader's invites and what they may do about them. */
export type OwnInvites = PagedList<OwnInvite> & {
  /**
   * Whether the reader may create an invite now; undefined until the first page arrives. It does not count the cap of
   * usable invites: compare `usable` with `limits.max_active`.
   */
  readonly canCreate: boolean | undefined;
  /**
   * Why the reader may not create an invite, while `canCreate` is false; undefined while they may, until the first
   * page arrives, and when the server names no cause.
   */
  readonly cannotCreateReason: InvitesNotAllowedReason | undefined;
  /** How many usable invites the reader holds. */
  readonly usable: number | undefined;
  /** The caps and defaults of the instance. */
  readonly limits: MemberInviteLimits | undefined;
  /**
   * Whether the server takes no new account now, while no invite can be used whatever its state says
   * (`inviteStateNow`); false until the instance is read.
   */
  readonly registrationClosed: boolean;
};

/**
 * The reader's invites, newest first, and what they may do; nothing is asked while `enabled` is false, nor once the
 * instance says it is one person's, which has no invites (ADR-0152 §6).
 */
export function useOwnInvites(enabled = true): OwnInvites {
  const data = useAppData();
  const instance = useQuery(instanceQuery(data)).data;
  const alone = instance?.single_user === true;
  const { firstPage, ...list } = usePagedListOf<OwnInvite, OwnInvitePage>(ownInvitesQuery(data), enabled && !alone);
  return {
    ...list,
    canCreate: firstPage?.can_create,
    cannotCreateReason: firstPage?.can_create === false ? firstPage.cannot_create_reason : undefined,
    usable: firstPage?.usable,
    limits: firstPage?.limits,
    registrationClosed: instance?.registrations.mode === "closed",
  };
}

/**
 * Reads the first page of the reader's invites, from the cache when it holds it, for a screen's loader; a read that
 * fails is the screen's to say.
 */
export async function loadOwnInvites(data: AppData): Promise<void> {
  await data.queryClient.infiniteQuery({ ...ownInvitesQuery(data), staleTime: Infinity }).catch(() => undefined);
}

/** Where the read of one invite of the reader's is, and the invite once a page that holds it was read. */
export type OwnInviteRead = {
  readonly invite: OwnInvite | undefined;
  /**
   * `read` once a page of the list holds the invite, `loading` until then, `failed` when the list could not be read,
   * with `retry`, and `missing` once every page was read and none holds it.
   */
  readonly status: "loading" | "failed" | "missing" | "read";
  readonly retry: () => void;
};

/**
 * The reader's invite of `id`. The API reads no invite alone: it is found in the list, whose next pages are read, one
 * after another, until one holds it or there is none left.
 */
export function useOwnInviteRead(id: string): OwnInviteRead {
  const { items, status, hasNextPage, isFetchingNextPage, fetchNextPage, refetch } = useOwnInvites();
  const invite = items.find((item) => item.id === id);
  const further = invite === undefined && status === "success" && hasNextPage && !isFetchingNextPage;
  useEffect(() => {
    // It asks for nothing while a page is on its way.
    if (further) fetchNextPage();
  });
  return {
    invite,
    status:
      invite !== undefined
        ? "read"
        : status === "error"
          ? "failed"
          : status === "success" && !hasNextPage
            ? "missing"
            : "loading",
    retry: refetch,
  };
}

/** Whether a server whose instance is `instance` takes invites: not one person's, and one that takes new accounts. */
const takesInvites = (instance: Instance | undefined): boolean =>
  instance !== undefined && !instance.single_user && instance.registrations.mode !== "closed";

/**
 * Reads whether the reader may create invites, for the settings to wait for before their menu paints: the instance,
 * from the cache, and, unless it takes no invites, the first page of the reader's invites, which the Invites screen
 * shows too.
 */
export async function loadInviteRight(data: AppData): Promise<void> {
  const instance = await data.queryClient.query({ ...instanceQuery(data), staleTime: Infinity }).catch(() => undefined);
  if (takesInvites(instance)) await loadOwnInvites(data);
}

/**
 * Whether the reader may create invites now (ADR-0106 §14), which the settings' menu offers the Invites entry by;
 * false until the server said so. The instance does not say who may (the setting `invite_creators` is the
 * administrators'), so this is `can_create` of the first page of the reader's invites; on a server that takes no
 * invites (one person's, or one that takes no new account) nothing is asked, and it is false whatever the cache holds.
 */
export function useMayInvite(): boolean {
  const offered = takesInvites(useQuery(instanceQuery(useAppData())).data);
  return useOwnInvites(offered).canCreate === true && offered;
}

/**
 * Whether the server has answered whether the reader may create invites (`useMayInvite`): the instance was read, and,
 * unless it takes no invites, the first page of the reader's invites was read or could not be. The settings' menu
 * paints its entries once it has, so that it never gains one a moment after it painted, on the app's first load too,
 * whose loader does not wait.
 */
export function useInviteRightKnown(): boolean {
  const instance = useQuery(instanceQuery(useAppData()));
  const offered = takesInvites(instance.data);
  const list = useOwnInvites(offered);
  if (instance.status === "pending") return false;
  return !offered || list.status !== "pending";
}

export type OwnInviteWrites = {
  readonly create: (request?: CreateOwnInvite) => Promise<OwnInvite>;
  readonly revoke: (id: string) => Promise<OwnInvite>;
};

/** Making and revoking invites, from a screen. */
export function useOwnInviteWrites(): OwnInviteWrites {
  const data = useAppData();
  return { create: (request) => createOwnInvite(data, request), revoke: (id) => revokeOwnInvite(data, id) };
}
