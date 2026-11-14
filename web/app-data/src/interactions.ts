// SPDX-License-Identifier: AGPL-3.0-only
// The reader's own marks on a status (ADR-0028 §6, §7): favourite, boost, bookmark and an emoji reaction, each set or
// taken back. Every cached copy of the status shows the change at once, the server's answer replaces it, and a failed
// write puts back what the copies showed before. A user has one favourite or one reaction on a status, so setting one
// takes back the other. The write is a plain request (no outbox yet, #2892): a failure rejects with mykit's
// ProblemError or NetworkError.
import type { AppData } from "./app-data";
import { useAppData } from "./provider";
import { updateCachedStatuses, type Status } from "./statuses";

/** A mark to set (`on`) or take back: a reaction names its emoji (a Unicode emoji, or a custom one's shortcode). */
export type StatusMark =
  | { readonly type: "favourite" | "boost" | "bookmark"; readonly on: boolean }
  | { readonly type: "reaction"; readonly name: string; readonly on: boolean };

/** The fields of a status that a mark changes. */
type Marks = Pick<Status, "favourited" | "favourites_count" | "boosted" | "boosts_count" | "bookmarked" | "reactions">;

/** The marks of a status, one it does not have as false. */
const marksOf = (s: Status): Marks => ({
  favourited: s.favourited ?? false,
  favourites_count: s.favourites_count,
  boosted: s.boosted ?? false,
  boosts_count: s.boosts_count,
  bookmarked: s.bookmarked ?? false,
  reactions: s.reactions,
});

/** The reactions without the reader's own: its count one less, and gone at none. */
const withoutMine = (reactions: Status["reactions"]): Status["reactions"] =>
  reactions.flatMap((r) => (r.me !== true ? [r] : r.count > 1 ? [{ ...r, count: r.count - 1, me: false }] : []));

/** The reader's favourite taken back, if they had one. */
const unfavourited = (s: Status): Status =>
  s.favourited === true ? { ...s, favourited: false, favourites_count: Math.max(0, s.favourites_count - 1) } : s;

/** The status as it shows once the server took the mark. */
export function marked(s: Status, mark: StatusMark): Status {
  const step = (n: number) => Math.max(0, n + (mark.on ? 1 : -1));
  switch (mark.type) {
    case "favourite":
      if ((s.favourited ?? false) === mark.on) return s;
      return {
        ...s,
        favourited: mark.on,
        favourites_count: step(s.favourites_count),
        reactions: mark.on ? withoutMine(s.reactions) : s.reactions,
      };
    case "boost":
      return (s.boosted ?? false) === mark.on ? s : { ...s, boosted: mark.on, boosts_count: step(s.boosts_count) };
    case "bookmark":
      return (s.bookmarked ?? false) === mark.on ? s : { ...s, bookmarked: mark.on };
    case "reaction": {
      if (!mark.on) return s.reactions.some((r) => r.me === true) ? { ...s, reactions: withoutMine(s.reactions) } : s;
      if (s.reactions.some((r) => r.name === mark.name && r.me === true)) return s;
      const others = withoutMine(s.reactions);
      const reactions = others.some((r) => r.name === mark.name)
        ? others.map((r) => (r.name === mark.name ? { ...r, count: r.count + 1, me: true } : r))
        : [...others, { name: mark.name, count: 1, me: true }];
      return { ...unfavourited(s), reactions };
    }
  }
}

/** Sends the mark: the request of its type, set or taken back. */
async function send(data: AppData, id: string, mark: StatusMark): Promise<Status | undefined> {
  const params = { params: { path: { id } } };
  switch (mark.type) {
    case "favourite":
      return (
        mark.on
          ? await data.api.POST("/statuses/{id}/favourite", params)
          : await data.api.DELETE("/statuses/{id}/favourite", params)
      ).data;
    case "boost":
      return (
        mark.on
          ? await data.api.POST("/statuses/{id}/boost", params)
          : await data.api.DELETE("/statuses/{id}/boost", params)
      ).data;
    case "bookmark":
      return (
        mark.on
          ? await data.api.POST("/statuses/{id}/bookmark", params)
          : await data.api.DELETE("/statuses/{id}/bookmark", params)
      ).data;
    case "reaction":
      return (
        mark.on
          ? await data.api.PUT("/statuses/{id}/reaction", { ...params, body: { emoji: mark.name } })
          : await data.api.DELETE("/statuses/{id}/reaction", params)
      ).data;
  }
}

/** The status with the ID given as some cached query holds it, else undefined. */
export function cachedStatus(data: AppData, id: string): Status | undefined {
  let found: Status | undefined;
  updateCachedStatuses(data.queryClient, id, (s) => {
    found ??= s;
    return s;
  });
  return found;
}

/**
 * Sets or takes back the reader's mark on the status with the ID given: every cached copy shows it at once, then as
 * the server answered; a failure puts back what they showed and rejects.
 */
export async function markStatus(data: AppData, id: string, mark: StatusMark): Promise<void> {
  let before: Marks | undefined;
  updateCachedStatuses(data.queryClient, id, (s) => {
    before ??= marksOf(s);
    return marked(s, mark);
  });
  try {
    const answer = await send(data, id, mark);
    if (answer?.id === id) {
      const after = marksOf(answer);
      updateCachedStatuses(data.queryClient, id, (s) => ({ ...s, ...after }));
    }
  } catch (e) {
    const put = before;
    if (put !== undefined) updateCachedStatuses(data.queryClient, id, (s) => ({ ...s, ...put }));
    throw e;
  }
}

/** markStatus for the session of the nearest AppDataProvider, and the status a mark acts on as the cache holds it. */
export function useStatusMarks(): {
  readonly mark: (id: string, mark: StatusMark) => Promise<void>;
  readonly cached: (id: string) => Status | undefined;
} {
  const data = useAppData();
  return { mark: (id, mark) => markStatus(data, id, mark), cached: (id) => cachedStatus(data, id) };
}
