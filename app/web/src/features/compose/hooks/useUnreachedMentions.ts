// SPDX-License-Identifier: AGPL-3.0-only
import { useQueries, useQuery } from "@tanstack/react-query";
import { useEffect, useState } from "react";
import { useApi } from "../../../lib/auth/SessionProvider";
import { useFederation } from "../../../lib/federation";
import { lookupAccountQueryOptions, maxRelationshipIds, relationshipsQueryOptions } from "../api/compose";
import { domainMentions } from "./policy";

/** How long the text must stay unchanged before its mentions are looked up, in ms. */
const mentionLookupDelay = 400;

/** `value` once it has stayed the same for `delay` ms; the timer is the external system (R8). */
function useSettled(value: string, delay: number): string {
  const [settled, setSettled] = useState(value);
  useEffect(() => {
    const timer = setTimeout(() => {
      setSettled(value);
    }, delay);
    return () => {
      clearTimeout(timer);
    };
  }, [value, delay]);
  return settled;
}

/**
 * The handles (`acct`) of the accounts of other servers that `text` mentions and that do not follow the
 * viewer, while `isLocal` (the post's visibility is `local`): a `local` post reaches only the author's
 * followers outside this server, so they will not receive it, and its copy there does not name them as
 * mentioned (ADR-0017 asks the client to warn, ADR-0035).
 *
 * Each mention with a domain is resolved with GET /accounts/lookup (`resolve`, as the server resolves
 * the mentions of a post, unless the instance takes nothing in from other servers, ADR-0076) once the text
 * settles; the accounts it finds outside this server (an `acct` with a domain) are checked with
 * GET /accounts/relationships. Handles that do not resolve are left out: the server leaves them as text.
 * Nothing is reported until the answers are in.
 */
export function useUnreachedMentions(text: string, isLocal: boolean): string[] {
  const api = useApi();
  const { followsRemote } = useFederation();
  const handles = useSettled(isLocal ? domainMentions(text).join(" ") : "", mentionLookupDelay);
  const lookups = useQueries({
    queries: (handles === "" ? [] : handles.split(" ")).map((acct) =>
      lookupAccountQueryOptions(api, acct, followsRemote),
    ),
  });
  const remote = lookups.flatMap((lookup) =>
    lookup.data !== undefined && lookup.data.acct.includes("@") ? [lookup.data] : [],
  );
  const ids = [...new Set(remote.map((account) => account.id))].slice(0, maxRelationshipIds);
  const relationships = useQuery({ ...relationshipsQueryOptions(api, ids), enabled: ids.length > 0 });
  if (!isLocal || relationships.data === undefined) return [];
  const followers = new Set(relationships.data.filter((r) => r.followed_by).map((r) => r.id));
  const checked = new Set(ids);
  return [
    ...new Set(
      remote.filter((account) => checked.has(account.id) && !followers.has(account.id)).map((account) => account.acct),
    ),
  ];
}
