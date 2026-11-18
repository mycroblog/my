// SPDX-License-Identifier: AGPL-3.0-only
// The invites the reader creates as a member (ADR-0106 §14–§19): their list, newest first, a page at a time, with what the
// reader may do now (`canCreate`, how many usable invites they hold against the instance's caps), making one, and
// revoking one. A write reads the list again, since it changes the count of the usable ones. The staff's invites are
// not here (`/admin/invites`). On a one-person server every operation is a 404 of the type
// `/api/v1/problems/single-user-mode`.
import type { components } from "@mycroblog/mykit";
import type { AppData } from "./app-data";
import { pagedQuery, usePagedListOf, type PagedList } from "./paged";
import { useAppData } from "./provider";

/** An invite of the reader's: its code, its address to send, its uses and its state now. */
export type OwnInvite = components["schemas"]["OwnInvite"];

/** The caps and defaults of the invites members create. */
export type MemberInviteLimits = components["schemas"]["MemberInviteLimits"];

/** What an invite allows now: `usable`, `revoked`, `expired`, `exhausted` or `creator_ineligible`. */
export type InviteState = components["schemas"]["AdminInviteState"];

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

/**
 * Makes an invite. It rejects with the server's problem: the reader may not create invites (403
 * `/api/v1/problems/invites-not-allowed`), holds as many usable ones as the cap (409 `too-many-invites`), a value over
 * a cap (422, `field` names `max_uses` or `expires_in`).
 */
export async function createOwnInvite(data: AppData, request: CreateOwnInvite = {}): Promise<OwnInvite> {
  const { data: made } = await data.api.POST("/invites", { body: request });
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
  /** How many usable invites the reader holds. */
  readonly usable: number | undefined;
  /** The caps and defaults of the instance. */
  readonly limits: MemberInviteLimits | undefined;
};

/** The reader's invites, newest first, and what they may do; nothing is asked while `enabled` is false. */
export function useOwnInvites(enabled = true): OwnInvites {
  const { firstPage, ...list } = usePagedListOf<OwnInvite, OwnInvitePage>(ownInvitesQuery(useAppData()), enabled);
  return { ...list, canCreate: firstPage?.can_create, usable: firstPage?.usable, limits: firstPage?.limits };
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
