// SPDX-License-Identifier: AGPL-3.0-only
import { type RefObject, useEffect, useRef, useState } from "react";
import { useQuery, useSuspenseInfiniteQuery } from "@tanstack/react-query";
import { useApi } from "../../../lib/auth/SessionProvider";
import { shellQueryReader } from "../../../lib/query";
import { preferencesQueryOptions } from "../../settings";
import { timelineQueryOptions } from "../api/timelines";
import { useNewHomeStatuses } from "./useNewHomeStatuses";
import { useReaderInFeed } from "./useReaderPause";

/** What the home timeline needs to show new posts: the button's count and press, and how the feed keeps the reader's place. */
export type HomeNewPosts = {
  /** The count the "N new posts" button shows; 0 hides it. */
  count: number;
  /** Whether the button stands at the top of the feed, where it would cover the first post without space kept for it. */
  buttonAtTop: boolean;
  /** Shows what waits and takes the reader to the top. */
  press: () => void;
  /** Props for the feed: only the auto-insert setting turns them on. */
  feed: { keepPosition: boolean; onTopChange: (atTop: boolean) => void };
};

function prefersReducedMotion(): boolean {
  return typeof window.matchMedia === "function" && window.matchMedia("(prefers-reduced-motion: reduce)").matches;
}

/**
 * New posts of the home timeline, by the reader's setting (ADR-0077). Off: they wait behind the button until it is
 * pressed (web-client/README.md Y7). On: they are inserted at once, and the feed keeps the post being read in place, unless
 * the reader is in the feed (focus or a selection: paused, they wait behind the button as when off) or they may not join
 * the loaded ones (a gap: the first page is loaded again at the top, otherwise on pressing the button). While the reader
 * is away from the top, the button counts what was inserted since they left it.
 */
export function useHomeNewPosts(feedRef: RefObject<HTMLElement | null>, headingId: string): HomeNewPosts {
  const api = useApi();
  const autoInsert =
    useQuery({ ...preferencesQueryOptions(api), ...shellQueryReader }).data?.timeline_auto_insert === true;
  const { count: waiting, gap, show } = useNewHomeStatuses();
  const paused = useReaderInFeed(feedRef);
  const [atTop, setAtTop] = useState(true);
  // The newest loaded post when the reader left the top: what is above it was inserted since (derived in render, R6).
  const [leftAt, setLeftAt] = useState<string | null>(null);
  const home = useSuspenseInfiniteQuery(timelineQueryOptions(api, { kind: "home" }));
  const loadedIds = home.data.pages.flatMap((page) => page.items.map((status) => status.id));
  const inserted = leftAt === null ? 0 : Math.max(0, loadedIds.indexOf(leftAt));

  // Waiting entries are inserted when nothing keeps them; a gap is closed at the top, where nothing is lost.
  const inserting = autoInsert && !paused && waiting > 0 && (!gap || atTop);
  const insertingRef = useRef(false);
  useEffect(() => {
    if (!inserting) {
      insertingRef.current = false;
      return;
    }
    if (insertingRef.current) return;
    insertingRef.current = true;
    show();
    // `show` is the same until the cache changes, and a second call before the entries left would repeat the first.
  }, [inserting, gap, atTop, waiting, show]);

  const away = autoInsert && !atTop;
  const held = autoInsert ? waiting > 0 && !inserting : waiting > 0;
  const count = (away ? inserted : 0) + (held ? waiting : 0);

  return {
    count,
    buttonAtTop: count > 0 && atTop,
    press: () => {
      show();
      setLeftAt(null);
      const heading = document.getElementById(headingId);
      if (!autoInsert) {
        heading?.focus();
        return;
      }
      window.scrollTo({ top: 0, behavior: prefersReducedMotion() ? "instant" : "smooth" });
      heading?.focus({ preventScroll: true });
    },
    feed: {
      keepPosition: autoInsert,
      onTopChange: (top) => {
        setAtTop(top);
        setLeftAt(top ? null : (loadedIds[0] ?? null));
      },
    },
  };
}
