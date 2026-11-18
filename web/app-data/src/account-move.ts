// SPDX-License-Identifier: AGPL-3.0-only
// Moving the viewer's account to another server, and the aliases that let an account on another server move to it
//: the accounts the viewer says are also theirs; moving this account to one; and canceling the move. The
// confirmation of a move is the viewer's password, a passkey or the confirmation of an external login. A write puts what
// the server answered into the cache and reads the viewer's account again, which says whether it moved. What the
// server refuses of a move is told by its kind (`moveRefusalOf`).
import { ProblemError, type components } from "@mycroblog/mykit";
import { useQuery } from "@tanstack/react-query";
import { loadAccount, viewerQuery, type Account } from "./accounts";
import type { AppData } from "./app-data";
import { useAppData } from "./provider";

/** An account the viewer says is also theirs: its ActivityPub ID, its handle, and when they added it. */
export type AccountAlias = components["schemas"]["AccountAlias"];

/**
 * How the viewer proves it is them for an act that cannot be undone: their password, a passkey, or the token of an
 * external login; one of them.
 */
export type Reauthentication = {
  readonly password?: string;
  readonly passkey?: components["schemas"]["PasskeyAssertion"];
  readonly external_confirmation?: string;
};

/** The most aliases an account has. */
export const aliasLimit = 5;

/**
 * Why the server refuses an alias: its address leads to no account that can be one (`invalid`: none there, not an
 * account, the viewer's own), or the viewer has the most aliases (`full`).
 */
export type AliasRefusal = "invalid" | "full";

/** The kind of the server's refusal of an alias that `error` is; undefined for any other failure. */
export function aliasRefusalOf(error: unknown): AliasRefusal | undefined {
  if (!(error instanceof ProblemError)) return undefined;
  if (error.type === "/api/v1/problems/invalid-alias") return "invalid";
  if (error.type === "/api/v1/problems/too-many-aliases") return "full";
  return undefined;
}

/** The viewer's aliases, oldest first. */
export function accountAliasesQuery({ queries }: AppData) {
  return queries.queryOptions("get", "/accounts/aliases");
}

/** The viewer's aliases; empty until the server answered. */
export function useAccountAliases(): readonly AccountAlias[] {
  return useQuery(accountAliasesQuery(useAppData())).data?.items ?? [];
}

/** The viewer's aliases, empty until the server answered, and `retry`, which reads them again after a failure. */
export function useAccountAliasList(): {
  aliases: readonly AccountAlias[];
  status: "pending" | "error" | "success";
  retry: () => void;
} {
  const query = useQuery(accountAliasesQuery(useAppData()));
  return {
    aliases: query.data?.items ?? [],
    status: query.status,
    retry: () => {
      void query.refetch();
    },
  };
}

/**
 * Adds the account of `acct` (`user@domain`, or the URL of its actor or profile) as an alias, and resolves once the
 * cache holds the list as the server answered. It rejects with the server's problem (an account it cannot find, too
 * many aliases).
 */
export async function addAccountAlias(data: AppData, acct: string): Promise<void> {
  const { data: list } = await data.api.POST("/accounts/aliases", { body: { acct } });
  if (list !== undefined) data.queryClient.setQueryData(accountAliasesQuery(data).queryKey, list);
}

/** Removes the alias of `uri` (as the list gives it), and resolves once the cache holds the list as the server answered. */
export async function removeAccountAlias(data: AppData, uri: string): Promise<void> {
  const { data: list } = await data.api.DELETE("/accounts/aliases", { params: { query: { uri } } });
  if (list !== undefined) data.queryClient.setQueryData(accountAliasesQuery(data).queryKey, list);
}

/** Adds and removes aliases, from a screen (`addAccountAlias`, `removeAccountAlias`). */
export function useAccountAliasWrites(): {
  readonly add: (acct: string) => Promise<void>;
  readonly remove: (uri: string) => Promise<void>;
} {
  const data = useAppData();
  return { add: (acct) => addAccountAlias(data, acct), remove: (uri) => removeAccountAlias(data, uri) };
}

/** For how many days after a move the account cannot move again, also when that move was canceled. */
export const moveCooldownDays = 30;

/**
 * Why the server refuses a move: the target does not list the viewer's account among its aliases (`alias_missing`),
 * moved itself (`target_moved`), or cannot be moved to (`target_unavailable`: the viewer's own account, one the
 * server cannot fetch, one suspended here); or the account moved less than `moveCooldownDays` days ago or moved to
 * that target before (`too_soon`).
 */
export type MoveRefusal = "alias_missing" | "target_moved" | "target_unavailable" | "too_soon";

const moveRefusals: Readonly<Record<string, MoveRefusal>> = {
  "/api/v1/problems/move-alias-missing": "alias_missing",
  "/api/v1/problems/move-target-moved": "target_moved",
  "/api/v1/problems/move-target-unavailable": "target_unavailable",
  "/api/v1/problems/move-too-soon": "too_soon",
};

/** The kind of the server's refusal of a move that `error` is; undefined for any other failure. */
export function moveRefusalOf(error: unknown): MoveRefusal | undefined {
  return error instanceof ProblemError ? moveRefusals[error.type] : undefined;
}

/** Whether the server found no account at the address the viewer named (a 404, or its 410 for one that is gone). */
export function accountNotFound(error: unknown): boolean {
  return error instanceof ProblemError && (error.status === 404 || error.status === 410);
}

/** Marks the viewer's account, which says whether it moved, and what shows it or follows from it. */
async function readMoved(data: AppData): Promise<void> {
  // A moved account cannot be deactivated: what deactivating would do is read again where a screen shows it.
  void data.queryClient.invalidateQueries({
    queryKey: data.queries.queryOptions("get", "/accounts/deactivation").queryKey,
  });
  await data.queryClient.invalidateQueries({ queryKey: viewerQuery(data).queryKey });
}

/**
 * Moves the viewer's account to the account of `targetAccountId` (which must have the viewer's account as an alias),
 * and resolves once the viewer's account is read again. It rejects with the server's problem (a wrong confirmation,
 * a refusal that `moveRefusalOf` tells).
 */
export async function moveOwnAccount(data: AppData, targetAccountId: string, proof: Reauthentication): Promise<void> {
  await data.api.POST("/accounts/move", { body: { target_account_id: targetAccountId, ...proof } });
  await readMoved(data);
}

/**
 * Cancels the move of the viewer's account, and resolves once the viewer's account is read again. An account the
 * server says has not moved (a 409 of the type `/api/v1/problems/account-not-moved`: the move was canceled
 * elsewhere) is canceled already: it resolves all the same, the account read again.
 */
export async function cancelOwnAccountMove(data: AppData): Promise<void> {
  try {
    await data.api.DELETE("/accounts/move");
  } catch (error) {
    if (!(error instanceof ProblemError) || error.type !== "/api/v1/problems/account-not-moved") throw error;
  }
  await readMoved(data);
}

/**
 * Finds the account to move to, moves the account and cancels the move, from a screen: `find` resolves with the
 * account at a handle or an address, which the server fetches from its server where it does not know it yet
 * (`loadAccount`), and rejects with the server's problem (`accountNotFound` tells the one of no account there).
 */
export function useAccountMove(): {
  readonly find: (acct: string) => Promise<Account>;
  readonly move: (targetAccountId: string, proof: Reauthentication) => Promise<void>;
  readonly cancel: () => Promise<void>;
} {
  const data = useAppData();
  return {
    find: (acct) => loadAccount(data, acct),
    move: (targetAccountId, proof) => moveOwnAccount(data, targetAccountId, proof),
    cancel: () => cancelOwnAccountMove(data),
  };
}
