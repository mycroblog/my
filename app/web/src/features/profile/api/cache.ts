// SPDX-License-Identifier: AGPL-3.0-only
// One account appears in many cached answers at once: its own queries (GET /accounts/lookup and
// /accounts/{id}), the author of each of its posts in every timeline, thread and post, the notifications it
// caused and the lists of accounts it is in. A change of its profile that the server has answered is known
// for certain, so it is written into every copy at once (web-client/README.md S1, S8): a page that a route shows
// from the cache (S7) shows the new profile from its first render, without fetching it again. The
// answers are matched by shape, not by key, as the posts' are (features/status/api/cache.ts).
import type { QueryClient } from "@tanstack/react-query";
import type { Account } from "./accounts";

/** The fields of an account that a change of its profile (PATCH /accounts/update_credentials) sets. */
const profileFields = [
  "display_name",
  "note",
  "fields",
  "emojis",
  "locked",
  "bot",
  "avatar",
  "header",
] as const satisfies readonly (keyof Account)[];

function isAccount(value: unknown): value is Account {
  if (typeof value !== "object" || value === null) return false;
  const { id, username, acct, display_name, fields } = value as Record<string, unknown>;
  // A post's mentions have an id, a username and an acct too, but no display name or fields.
  return (
    typeof id === "string" &&
    typeof username === "string" &&
    typeof acct === "string" &&
    typeof display_name === "string" &&
    Array.isArray(fields)
  );
}

/** `account` with the profile of `saved`; the fields it has beside those (the follow counts) stay as they are. */
function withProfile(account: Account, saved: Account): Account {
  // The metadata fields are a list of plain values, compared by their content.
  if (profileFields.every((field) => JSON.stringify(account[field]) === JSON.stringify(saved[field]))) return account;
  const kept = Object.entries(account).filter(([key]) => !(profileFields as readonly string[]).includes(key));
  // An image that the change removed is left out of the answer (api/openapi.yaml, Account.avatar).
  const set = profileFields.flatMap((field) => (saved[field] === undefined ? [] : [[field, saved[field]] as const]));
  return Object.fromEntries([...kept, ...set]) as Account;
}

/**
 * Returns `data` with the profile of `saved` in every copy of its account, or `data` itself when it holds
 * none (so that the queries without it do not render again).
 */
export function updateAccounts(data: unknown, saved: Account): unknown {
  if (Array.isArray(data)) {
    const next = data.map((item: unknown) => updateAccounts(item, saved));
    return next.every((item, i) => item === data[i]) ? data : next;
  }
  if (typeof data !== "object" || data === null) return data;
  let record = data as Record<string, unknown>;
  if (isAccount(record) && record.id === saved.id) record = withProfile(record, saved);
  let next: Record<string, unknown> | null = null;
  for (const [key, value] of Object.entries(record)) {
    if (typeof value !== "object" || value === null) continue;
    const updated = updateAccounts(value, saved);
    if (updated !== value) next = { ...(next ?? record), [key]: updated };
  }
  return next ?? record;
}

/**
 * Writes the profile of `saved`, the viewer's account as the server answered a change of it, into every
 * cached answer of the API. The viewer's own query (GET /accounts/verify_credentials) holds more than an
 * account and is left to the caller, which has its whole answer.
 */
export function updateCachedAccount(queryClient: QueryClient, saved: Account): void {
  // The API's keys start with the method (openapi-react-query's `[method, path, init]`); reads are "get".
  queryClient.setQueriesData(
    {
      predicate: (query) =>
        query.queryKey[0] === "get" &&
        query.queryKey[1] !== "/accounts/verify_credentials" &&
        query.state.data !== undefined,
    },
    (data: unknown) => {
      const next = updateAccounts(data, saved);
      // undefined leaves the query as it is.
      return next === data ? undefined : next;
    },
  );
}
