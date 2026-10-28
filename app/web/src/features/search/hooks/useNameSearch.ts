// SPDX-License-Identifier: AGPL-3.0-only
// The accounts whose names match the text typed in the search field (FR-307, web-client/screens/search.md): this
// server's accounts by part of their display name or handle, and the accounts the server knows by the start of their
// names (GET /accounts/search). The search waits until the typing pauses, and needs a session. The hook returns no JSX.
import { keepPreviousData, useQuery } from "@tanstack/react-query";
import { useEffect, useState } from "react";
import { useApi } from "../../../lib/auth/SessionProvider";
import { errorMessage } from "../../../lib/errors";
import type { Account } from "../../profile";

/** How long the typing pauses before the search is sent, in milliseconds. */
const nameSearchDelay = 300;

/** The longest text the name search takes (api/openapi.yaml, searchAccounts `q` after its `@`). */
const maxNameQuery = 100;

/** The most accounts the search shows (SearchLimit's maximum). */
const nameSearchLimit = 20;

export type NameSearch =
  /** Nothing to search: the field is empty, the text is not a name, or nobody is signed in. */
  | { status: "idle" }
  | { status: "loading" }
  | { status: "error"; message: string }
  | { status: "done"; accounts: readonly Account[] };

/** The text as the name search takes it, or null when it is not one (empty, too long, a URL). */
function nameQueryOf(text: string): string | null {
  const q = text.trim().replace(/^@/u, "");
  if (q === "" || Array.from(q).length > maxNameQuery || /^https?:\/\//iu.test(q)) return null;
  return q;
}

export function useNameSearch(text: string, enabled: boolean): NameSearch {
  const api = useApi();
  const [query, setQuery] = useState(() => nameQueryOf(text));
  const next = nameQueryOf(text);
  useEffect(() => {
    // The search follows the typed text once it pauses (a timer, an external system: react.dev "Synchronizing with Effects").
    const timer = setTimeout(() => {
      setQuery(next);
    }, nameSearchDelay);
    return () => {
      clearTimeout(timer);
    };
  }, [next]);
  const result = useQuery({
    ...api.query.queryOptions("get", "/accounts/search", {
      params: { query: { q: query ?? "", limit: nameSearchLimit } },
    }),
    enabled: enabled && query !== null,
    // The list stays while the next text is searched, rather than flashing empty at every key.
    placeholderData: keepPreviousData,
  });
  if (!enabled || next === null) return { status: "idle" };
  if (result.isError) return { status: "error", message: errorMessage(result.error) };
  if (result.data === undefined) return { status: "loading" };
  return { status: "done", accounts: result.data };
}
