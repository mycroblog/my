// SPDX-License-Identifier: AGPL-3.0-only
// What the settings' Security screen reads at once (FR-002, FR-004, FR-005, FR-010, FR-012): whether the account has
// a password, its passkeys, its two-factor authentication, its linked accounts and the browsers signed in to it. Each
// is a query of its own (password.ts, passkeys.ts, two-factor.ts, linked-identities.ts, browser-sessions.ts); here
// they are read together, for the screen's loader and for the one notice that says they load or failed.
import { useQueries } from "@tanstack/react-query";
import type { AppData } from "./app-data";
import { browserSessionsQuery } from "./browser-sessions";
import { linkedIdentitiesQuery } from "./linked-identities";
import { passkeysQuery } from "./passkeys";
import { passwordStatusQuery } from "./password";
import { useAppData } from "./provider";
import { twoFactorQuery } from "./two-factor";

/** Reads what the Security screen shows, from the cache where it holds it; a read that fails is the screen's to say. */
export async function loadSecurity(data: AppData): Promise<void> {
  const { queryClient } = data;
  await Promise.all([
    queryClient.query({ ...passwordStatusQuery(data), staleTime: Infinity }),
    queryClient.query({ ...passkeysQuery(data), staleTime: Infinity }),
    queryClient.query({ ...twoFactorQuery(data), staleTime: Infinity }),
    queryClient.query({ ...linkedIdentitiesQuery(data), staleTime: Infinity }),
    queryClient.query({ ...browserSessionsQuery(data), staleTime: Infinity }),
  ]).catch(() => undefined);
}

/**
 * Where the reads of the Security screen are: `read` once the server answered them all, `failed` when it did not
 * answer one, with `retry`, which reads again those that failed, and `loading` until then.
 */
export function useSecurityRead(): { status: "loading" | "failed" | "read"; retry: () => void } {
  const data = useAppData();
  const reads = useQueries({
    queries: [
      passwordStatusQuery(data),
      passkeysQuery(data),
      twoFactorQuery(data),
      linkedIdentitiesQuery(data),
      browserSessionsQuery(data),
    ],
  });
  // What was read stays read: a later read of it that fails leaves the screen what it had.
  return {
    status: reads.every((read) => read.data !== undefined)
      ? "read"
      : reads.some((read) => read.status === "error" && !read.isFetching)
        ? "failed"
        : "loading",
    retry: () => {
      for (const read of reads) if (read.status === "error") void read.refetch();
    },
  };
}
