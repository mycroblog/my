// SPDX-License-Identifier: AGPL-3.0-only
// What the settings' Security screen reads at once (FR-002, FR-004, FR-005, FR-010, FR-012): whether the account has
// a password, its passkeys, its two-factor authentication, its linked accounts and the browsers signed in to it. Each
// is a query of its own (password.ts, passkeys.ts, two-factor.ts, linked-identities.ts, browser-sessions.ts); here
// they are read together, for the screen's loader and for the one notice that says they load or failed. And what the
// account confirms an operation with, which the screens that ask for a confirmation outside Security read: with the
// account itself for those that move and delete it, with what deactivating would do for the one that deactivates it
// (deactivation.ts).
import { useQueries, useQuery } from "@tanstack/react-query";
import { viewerQuery } from "./accounts";
import type { AppData } from "./app-data";
import { browserSessionsQuery } from "./browser-sessions";
import { linkedIdentitiesQuery } from "./linked-identities";
import { passkeysQuery } from "./passkeys";
import { passwordStatusQuery } from "./password";
import type { CredentialAccount } from "./profile";
import { useAppData } from "./provider";
import { readOf, type ScreenRead } from "./reads";
import { twoFactorQuery } from "./two-factor";

/**
 * The reads of what the account confirms an operation with (ADR-0106 §3.7): whether it has a password, its passkeys
 * and its linked accounts. A screen that asks for a confirmation shows what it asks it for once they answered.
 */
export function confirmationQueries(data: AppData) {
  return [passwordStatusQuery(data), passkeysQuery(data), linkedIdentitiesQuery(data)] as const;
}

/** Reads what the account confirms with, from the cache where it holds it; a read that fails is the screen's to say. */
export async function loadConfirmation(data: AppData): Promise<void> {
  const { queryClient } = data;
  await Promise.all([
    queryClient.query({ ...passwordStatusQuery(data), staleTime: Infinity }),
    queryClient.query({ ...passkeysQuery(data), staleTime: Infinity }),
    queryClient.query({ ...linkedIdentitiesQuery(data), staleTime: Infinity }),
  ]).catch(() => undefined);
}

/**
 * Reads what a screen that moves or deletes the viewer's account shows, from the cache where it holds it: the
 * account and what it confirms with. A read that fails is the screen's to say.
 */
export async function loadOwnAccount(data: AppData): Promise<void> {
  await Promise.all([
    data.queryClient.query({ ...viewerQuery(data), staleTime: Infinity }).catch(() => undefined),
    loadConfirmation(data),
  ]);
}

/**
 * Where the reads of a screen that moves or deletes the viewer's account are, and the account once read, whose
 * `moved` is the account it moved to: `read` once the server answered the account and what it confirms with,
 * `failed` when it did not answer one, with `retry`, which reads again those that failed, and `loading` until then.
 */
export function useOwnAccountRead(): ScreenRead & { account: CredentialAccount | undefined } {
  const data = useAppData();
  const account = useQuery(viewerQuery(data));
  const ways = useQueries({ queries: [...confirmationQueries(data)] });
  return { ...readOf([account, ...ways]), account: account.data };
}

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
export function useSecurityRead(): ScreenRead {
  const data = useAppData();
  return readOf(
    useQueries({
      queries: [
        passwordStatusQuery(data),
        passkeysQuery(data),
        twoFactorQuery(data),
        linkedIdentitiesQuery(data),
        browserSessionsQuery(data),
      ],
    }),
  );
}
