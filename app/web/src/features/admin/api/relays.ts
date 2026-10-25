// SPDX-License-Identifier: AGPL-3.0-only
// The queries and mutations of the relays screen (web-client/README.md S1, ADR-0061, manage_settings): the relays the
// server joins or joined (GET /admin/relays), in registration order; registering and joining one, leaving it,
// joining it again and deleting its registration. An operation that breaks a rule is a 422 `invalid-relay` problem
// whose `reason` says which. Every change is recorded in the audit log.
import type { QueryClient } from "@tanstack/react-query";
import type { Api } from "../../../lib/api/client";
import type { components } from "@mycroblog/mykit";
import { ApiError } from "../../../lib/errors";
import { refreshAuditLog } from "../../moderation";

type Schemas = components["schemas"];

export type Relay = Schemas["AdminRelay"];
export type RelayProtocol = Schemas["RelayProtocol"];
export type RelayState = Schemas["RelayState"];
export type NewRelay = Schemas["CreateRelayRequest"];
export type RelayProblemReason = Schemas["RelayProblemReason"];

/** The problem type of a relay operation that breaks a rule. */
const invalidRelayProblem = "/api/v1/problems/invalid-relay";

/** The longest URL of a relay (CreateRelayRequest). */
export const relayUrlLimit = 2048;

/** The ways of joining a relay, in the order of the form after detecting it automatically (RelayProtocol). */
export const relayProtocols = ["mastodon", "litepub"] as const satisfies readonly RelayProtocol[];

const relayProblemReasons: readonly string[] = ["url", "exists", "domain_suspended", "not_actor", "state"] satisfies readonly RelayProblemReason[];

/** The relays, in registration order. */
export function relaysQueryOptions(api: Api) {
  return api.query.queryOptions("get", "/admin/relays");
}

/** The reason of an `invalid-relay` problem, or undefined for another error or a problem without one. */
export function relayProblemReason(error: unknown): RelayProblemReason | undefined {
  if (!(error instanceof ApiError) || error.type !== invalidRelayProblem) return undefined;
  const reason = error.document["reason"];
  return typeof reason === "string" && relayProblemReasons.includes(reason) ? (reason as RelayProblemReason) : undefined;
}

/** Registers a relay and joins it (POST /admin/relays), as the options of useMutation. */
export function createRelayMutation(api: Api) {
  return {
    mutationKey: ["post", "/admin/relays"] as const,
    mutationFn: async (body: NewRelay): Promise<Relay> => {
      const { data, error } = await api.fetch.POST("/admin/relays", { body });
      // The middleware throws an ApiError for every error status; this narrows the type.
      if (data === undefined) throw new ApiError(error);
      return data;
    },
  };
}

/** Joins a relay again (`follow`) or leaves it (`unfollow`), as the options of useMutation. */
export function setRelayMembershipMutation(api: Api) {
  return {
    mutationKey: ["post", "/admin/relays/{id}/follow"] as const,
    mutationFn: async ({ id, join }: { id: string; join: boolean }): Promise<Relay> => {
      const { data, error } = join
        ? await api.fetch.POST("/admin/relays/{id}/follow", { params: { path: { id } } })
        : await api.fetch.POST("/admin/relays/{id}/unfollow", { params: { path: { id } } });
      if (data === undefined) throw new ApiError(error);
      return data;
    },
  };
}

/** Deletes the registration of a relay that was left or rejected, as the options of useMutation. */
export function deleteRelayMutation(api: Api) {
  return {
    mutationKey: ["delete", "/admin/relays/{id}"] as const,
    mutationFn: async (id: string): Promise<void> => {
      // The middleware throws for every error status; a 204 has no body to read.
      await api.fetch.DELETE("/admin/relays/{id}", { params: { path: { id } } });
    },
  };
}

/**
 * Marks what a change of a relay changes (ADR-0045): the list, whose state the answer shows, and the audit log;
 * the queries shown now are fetched again and awaited.
 */
export async function refreshRelays(queryClient: QueryClient): Promise<void> {
  await Promise.all([queryClient.invalidateQueries({ queryKey: ["get", "/admin/relays"] }), refreshAuditLog(queryClient)]);
}
