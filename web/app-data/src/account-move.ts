// SPDX-License-Identifier: AGPL-3.0-only
// Moving the viewer's account to another server, and the aliases that let an account on another server move to it
//: the accounts the viewer says are also theirs; moving this account to one; and canceling the move. The
// confirmation of a move is the viewer's password, a passkey or the confirmation of an external login. A write puts what
// the server answered into the cache and reads the viewer's account again, which says whether it moved.
import type { components } from "@mycroblog/mykit";
import { useQuery } from "@tanstack/react-query";
import { viewerQuery } from "./accounts";
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

/** The viewer's aliases, oldest first. */
export function accountAliasesQuery({ queries }: AppData) {
  return queries.queryOptions("get", "/accounts/aliases");
}

/** The viewer's aliases; empty until the server answered. */
export function useAccountAliases(): readonly AccountAlias[] {
  return useQuery(accountAliasesQuery(useAppData())).data?.items ?? [];
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

/** Marks the viewer's account, which says whether it moved, and what shows it. */
async function readMoved(data: AppData): Promise<void> {
  await data.queryClient.invalidateQueries({ queryKey: viewerQuery(data).queryKey });
}

/**
 * Moves the viewer's account to the account of `targetAccountId` (which must have the viewer's account as an alias),
 * and resolves once the viewer's account is read again. It rejects with the server's problem (a wrong confirmation,
 * a target without the alias).
 */
export async function moveOwnAccount(data: AppData, targetAccountId: string, proof: Reauthentication): Promise<void> {
  await data.api.POST("/accounts/move", { body: { target_account_id: targetAccountId, ...proof } });
  await readMoved(data);
}

/** Cancels the move of the viewer's account, and resolves once the viewer's account is read again. */
export async function cancelOwnAccountMove(data: AppData): Promise<void> {
  await data.api.DELETE("/accounts/move");
  await readMoved(data);
}

/** Moves the account and cancels the move, from a screen (`moveOwnAccount`, `cancelOwnAccountMove`). */
export function useAccountMove(): {
  readonly move: (targetAccountId: string, proof: Reauthentication) => Promise<void>;
  readonly cancel: () => Promise<void>;
} {
  const data = useAppData();
  return {
    move: (targetAccountId, proof) => moveOwnAccount(data, targetAccountId, proof),
    cancel: () => cancelOwnAccountMove(data),
  };
}
