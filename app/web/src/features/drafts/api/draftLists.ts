// SPDX-License-Identifier: AGPL-3.0-only
import type { Api } from "../../../lib/api/client";
import { draftsQueryOptions } from "../../compose";

/** The two lists the drafts page shows, for its route's loader: the scheduled drafts, then the others. */
export function draftListsQueryOptions(api: Api) {
  return [draftsQueryOptions(api, true), draftsQueryOptions(api, false)] as const;
}
