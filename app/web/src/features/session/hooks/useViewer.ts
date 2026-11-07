// SPDX-License-Identifier: AGPL-3.0-only
import { useQuery } from "@tanstack/react-query";
import { useApi, useSessionStatus } from "../../../lib/auth/SessionProvider";
import { errorMessage } from "../../../lib/errors";
import { useLoadFailure } from "../../../lib/loadFailure";
import { shellQueryReader } from "../../../lib/query";
import { type CredentialAccount, viewerQueryOptions } from "../api/viewer";

/** Who is using the app. */
export type Viewer =
  /** The session is resuming, or the account is loading. */
  | { status: "loading" }
  | { status: "signed-out" }
  | { status: "signed-in"; account: CredentialAccount }
  /** Signed in, but the account could not be loaded; `message` is localized. `isRetrying` while `retry` loads it again. */
  | { status: "error"; message: string; retry: () => void; isRetrying: boolean };

/**
 * Returns the viewer: the session's status and, once signed in, the account from the API. The app shell reads it
 * on every page, so an account loaded before stays the viewer while a refetch fails (a 429 of the rate limit, say).
 */
export function useViewer(): Viewer {
  const api = useApi();
  const status = useSessionStatus();
  // The root route's loader loads it first, with the same retry; a failure it met is not sent again on mount.
  const account = useQuery({ ...viewerQueryOptions(api), enabled: status === "signed-in", ...shellQueryReader });
  const failure = useLoadFailure(account);
  if (status === "signed-out") return { status: "signed-out" };
  if (status === "unknown") return { status: "loading" };
  if (account.data !== undefined) return { status: "signed-in", account: account.data };
  if (failure.error !== null) {
    return {
      status: "error",
      message: errorMessage(failure.error),
      retry: () => {
        void account.refetch();
      },
      isRetrying: failure.isRetrying,
    };
  }
  return { status: "loading" };
}
