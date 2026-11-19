// SPDX-License-Identifier: AGPL-3.0-only
// The relays the instance joins (FR-724, ADR-0006), as a holder of `manage_settings` keeps them: each relay's protocol,
// its URL, whether it receives, where its membership stands and what it brought in during the last 24 hours;
// registering one (the server sends its Follow at once), joining one again, leaving one, and deleting one it left or
// that rejected it. Every write reads the list again.
import { ProblemError, type components } from "@mycroblog/mykit";
import { useQuery } from "@tanstack/react-query";
import type { AppData } from "./app-data";
import { useAppData } from "./provider";
import { readOf, type ScreenRead } from "./reads";
import { removeCached } from "./written";

/** A relay the instance joins or joined. */
export type AdminRelay = components["schemas"]["AdminRelay"];
/** How a relay is joined. */
export type RelayProtocol = components["schemas"]["RelayProtocol"];
/** Where the membership of a relay stands. */
export type RelayState = components["schemas"]["RelayState"];
/** A relay to register and join. */
export type CreateRelay = components["schemas"]["CreateRelayRequest"];
/** Why the server refused a relay operation. */
export type RelayRefusal = components["schemas"]["RelayProblemReason"];

/** The protocols of a relay, in the server's order. */
export const relayProtocols = ["mastodon", "litepub"] as const satisfies readonly RelayProtocol[];

/** The relays, in registration order. */
export function adminRelaysQuery({ queries }: AppData) {
  return queries.queryOptions("get", "/admin/relays");
}

/** Reads the relays, for a route's loader. */
export async function loadAdminRelays(data: AppData): Promise<void> {
  await data.queryClient.query({ ...adminRelaysQuery(data), staleTime: Infinity }).catch(() => undefined);
}

/** The relays and where their read is. */
export function useAdminRelays(): ScreenRead & { readonly relays: readonly AdminRelay[] | undefined } {
  const query = useQuery(adminRelaysQuery(useAppData()));
  return { ...readOf([query]), relays: query.data?.items };
}

/** The relay of `id`, found in the list; `missing` once the list is read without it. */
export function useAdminRelay(id: string): ScreenRead & { readonly relay: AdminRelay | undefined; missing: boolean } {
  const query = useQuery(adminRelaysQuery(useAppData()));
  const relay = query.data?.items.find((r) => r.id === id);
  return { ...readOf([query]), relay, missing: query.data !== undefined && relay === undefined };
}

/** Why the server refused a relay operation (`reason` of `/api/v1/problems/invalid-relay`); undefined otherwise. */
export function relayRefusalOf(error: unknown): RelayRefusal | undefined {
  if (!(error instanceof ProblemError) || error.type !== "/api/v1/problems/invalid-relay") return undefined;
  const reason = error.problem["reason"];
  return typeof reason === "string" ? (reason as RelayRefusal) : undefined;
}

/** The writes of the relays; each rejects with the server's problem (`relayRefusalOf`). */
export type AdminRelayWrites = {
  /** Registers a relay and sends its Follow; resolves with it, `pending`. */
  readonly create: (request: CreateRelay) => Promise<AdminRelay>;
  /** Sends a new Follow to a relay that rejected the instance or that it left. */
  readonly join: (id: string) => Promise<AdminRelay>;
  /** Sends the Undo of the Follow of a pending or accepted relay. */
  readonly leave: (id: string) => Promise<AdminRelay>;
  /** Deletes a relay the instance left or that rejected it. */
  readonly remove: (id: string) => Promise<void>;
};

/** The writes of the relays, for a screen. */
export function useAdminRelayWrites(): AdminRelayWrites {
  const data = useAppData();
  const key = adminRelaysQuery(data).queryKey;
  const written = async (relay: AdminRelay | undefined): Promise<AdminRelay> => {
    if (relay === undefined) throw new Error("app-data: a write of a relay answered without the relay");
    await data.queryClient.invalidateQueries({ queryKey: key });
    return relay;
  };
  const path = (id: string) => ({ params: { path: { id } } });
  return {
    create: async (request) => written((await data.api.POST("/admin/relays", { body: request })).data),
    join: async (id) => written((await data.api.POST("/admin/relays/{id}/follow", path(id))).data),
    leave: async (id) => written((await data.api.DELETE("/admin/relays/{id}/follow", path(id))).data),
    remove: async (id) => {
      await removeCached<components["schemas"]["AdminRelayList"]>(
        data,
        key,
        data.api.DELETE("/admin/relays/{id}", path(id)),
        (now) => ({ items: now.items.filter((r) => r.id !== id) }),
      );
    },
  };
}
