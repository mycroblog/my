// SPDX-License-Identifier: AGPL-3.0-only
// The post an account's moderation page starts from (`?status=`, a post's menu leads there with it), which its form
// of an action offers (screens/moderation-account.md). The page works without it: a failure only leaves the post out.
import { useQuery } from "@tanstack/react-query";
import { useApi } from "../../../lib/auth/SessionProvider";
import { errorMessage } from "../../../lib/errors";
import { type Status, statusQueryOptions } from "../../status";

export type StartingPost =
  /** The page starts from no post, or the address names a post of another account. */
  | { kind: "none" }
  | { kind: "loading" }
  /** The post cannot be read, or is gone now that an action deleted it; `message` is localized. */
  | { kind: "failed"; message: string }
  | { kind: "ready"; post: Status };

/** The post `statusId` of the account `accountId`. */
export function useStartingPost(accountId: string, statusId: string | undefined): StartingPost {
  const api = useApi();
  const query = useQuery({ ...statusQueryOptions(api, statusId ?? ""), enabled: statusId !== undefined });
  if (statusId === undefined) return { kind: "none" };
  if (query.isError) return { kind: "failed", message: errorMessage(query.error) };
  if (query.data === undefined) return { kind: "loading" };
  return query.data.account.id === accountId ? { kind: "ready", post: query.data } : { kind: "none" };
}
