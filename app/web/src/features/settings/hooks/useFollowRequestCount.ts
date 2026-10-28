// SPDX-License-Identifier: AGPL-3.0-only
import { useQuery } from "@tanstack/react-query";
import { useApi, useSessionStatus } from "../../../lib/auth/SessionProvider";
import { shellQueryReader } from "../../../lib/query";
import { credentialsQueryOptions } from "../api/me";

/**
 * The number of follow requests that await the viewer's approval, from the viewer's own account (the badge of the
 * settings menu's Follow requests item): 0 while signed out, while the account loads, and when it cannot be loaded
 * (the badge then stays away rather than showing a guess). Accepting or rejecting a request fetches the account again
 * (cacheListChange).
 */
export function useFollowRequestCount(): number {
  const api = useApi();
  const signedIn = useSessionStatus() === "signed-in";
  const account = useQuery({ ...credentialsQueryOptions(api), ...shellQueryReader, enabled: signedIn });
  return signedIn ? (account.data?.follow_requests_count ?? 0) : 0;
}
