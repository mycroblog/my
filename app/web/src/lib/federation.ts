// SPDX-License-Identifier: AGPL-3.0-only
// The direction the instance federates in (FR-413, ADR-0076), from GET /instance (`federation.mode`), and what it
// lets the app offer: following and finding accounts of other servers needs the instance to take their posts in
// (`inbound`, `both`), and what the viewer does to posts of other servers reaches them only while the instance sends
// out (`outbound`, `both`). The server decides every request; the app only leaves out what the server would refuse
// and says what stays on this server.
import { type QueryClient, useQuery } from "@tanstack/react-query";
import type { Api } from "./api/client";
import type { components } from "@mycroblog/mykit";
import { useApi } from "./auth/SessionProvider";
import { ApiError } from "./errors";
import { shellQueryReader } from "./query";

/** The instance setting `federation_mode`. */
export type FederationMode = components["schemas"]["InstanceFederation"]["mode"];

/** The modes, from the narrowest to the widest. */
export const federationModes = ["off", "inbound", "outbound", "both"] as const satisfies readonly FederationMode[];

/** The problem type of the 403 for an operation that the federation mode does not let through. */
const federationRestrictedType = "/api/v1/problems/federation-restricted";

/** Whether `error` is the server's refusal of an operation that the federation mode does not let through. */
export function isFederationRestricted(error: unknown): boolean {
  return error instanceof ApiError && error.type === federationRestrictedType;
}

/** What the instance's federation mode lets the app offer. */
export type FederationReach = {
  mode: FederationMode;
  /**
   * Whether the viewer can follow and look up accounts of other servers (the flows `ingest` and `follow_out`):
   * not while the mode is `off` or `outbound`, when the server refuses them with `federation-restricted`.
   */
  followsRemote: boolean;
  /**
   * Whether what the viewer posts and does reaches other servers (the flow `publish`): not while the mode is
   * `off` or `inbound`, when replies, boosts, favourites and reactions to their posts stay on this server and
   * quoting their posts is refused.
   */
  reachesRemote: boolean;
};

/** What `mode` lets through (ADR-0076, "流れの分類"). */
export function federationReachOf(mode: FederationMode): FederationReach {
  return {
    mode,
    followsRemote: mode === "inbound" || mode === "both",
    reachesRemote: mode === "outbound" || mode === "both",
  };
}

/**
 * The instance's federation mode from GET /instance. Until it loads, and when it cannot, the app offers everything
 * (`both`, the default of the setting): the server still refuses what the mode does not let through, and the
 * refusal says why.
 */
export function useFederation(): FederationReach {
  const api = useApi();
  // The shell reads GET /instance on every page and handles its failures; this only reads its answer.
  const instance = useQuery({ ...api.query.queryOptions("get", "/instance"), ...shellQueryReader });
  return federationReachOf(instance.data?.federation.mode ?? "both");
}

/**
 * Fetches GET /instance again whenever the server refuses a request with `federation-restricted`: the operators
 * narrowed the mode after the page loaded it, and the app should stop offering what the server now refuses. The
 * refusal itself is shown where it happened (its localized title says why).
 */
export function refetchInstanceOnFederationRestricted(queryClient: QueryClient, api: Api): () => void {
  const refetch = () => {
    void queryClient.invalidateQueries({ queryKey: api.query.queryOptions("get", "/instance").queryKey });
  };
  const unsubscribeQueries = queryClient.getQueryCache().subscribe((event) => {
    if (event.type === "updated" && event.action.type === "error" && isFederationRestricted(event.action.error)) refetch();
  });
  const unsubscribeMutations = queryClient.getMutationCache().subscribe((event) => {
    if (event.type === "updated" && event.action.type === "error" && isFederationRestricted(event.action.error)) refetch();
  });
  return () => {
    unsubscribeQueries();
    unsubscribeMutations();
  };
}
