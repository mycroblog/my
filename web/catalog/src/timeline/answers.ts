// SPDX-License-Identifier: AGPL-3.0-only

// What the server would answer to a post's action, on the catalog, where no server runs: the status a favourite, a
// boost, a bookmark or an emoji reaction leaves. /app/timeline writes it into the cache of @my/app-data, and the single
// sample posts of the catalog (SamplePost.tsx) keep it.

import type { PostAction, PostStatus } from "@my/app-components";

/** The status a toggle leaves: on or off, its count moved with it. */
const toggled = (on: boolean | undefined, count: number) => ({ on: !(on ?? false), count: count + (on ? -1 : 1) });

/** The status after the reader's reaction with `name`: taken back if it was theirs, else moved there. */
function reacted(status: PostStatus, name: string): PostStatus {
  const was = status.reactions.find((r) => r.me === true)?.name;
  const reactions = status.reactions
    .map((r) => {
      if (r.name === name) return { ...r, me: was !== name, count: r.count + (was === name ? -1 : 1) };
      if (r.name === was) return { ...r, me: false, count: r.count - 1 };
      return r;
    })
    .filter((r) => r.count > 0);
  return { ...status, reactions };
}

/** What an action changes on the status it acts on; the others change nothing here. */
export function answered<S extends PostStatus>(status: S, action: PostAction): S {
  switch (action.type) {
    case "favourite": {
      const t = toggled(status.favourited, status.favourites_count);
      return { ...status, favourited: t.on, favourites_count: t.count };
    }
    case "boost": {
      const t = toggled(status.boosted, status.boosts_count);
      return { ...status, boosted: t.on, boosts_count: t.count };
    }
    case "bookmark":
      return { ...status, bookmarked: !(status.bookmarked ?? false) };
    case "reaction":
      return reacted(status, action.name) as S;
    default:
      return status;
  }
}
