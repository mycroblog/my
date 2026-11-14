// SPDX-License-Identifier: AGPL-3.0-only

// The policy of the new posts of a shown timeline (ADR-0013 §2–§8): with `timeline_auto_insert` on, a post that
// arrives goes in at once, unless the reader is in the feed (focus or a selection inside it, WCAG 2.2.2), when it is
// held until they leave it. At the top the reader sees it come in; away from it, the feed keeps their place and the
// button counts the posts that went in above since they left the top, until they come back to it. With the setting
// off, every post is held until the reader presses the button. After a break (a reconnection, or more held than join
// those loaded), the first page is read again at once at the top, else when the reader presses the button. The button
// tells how many are held or went in above; the feed takes the reader to the top and the focus to the heading.

import { useHomeArrivals, type Arrival } from "@my/app-data";
import { useState } from "react";

/** Where the reader is: the setting, whether the feed's top is in view, and whether they are in the feed. */
type Reading = { readonly autoInsert: boolean; readonly atTop: boolean; readonly inside: boolean };

/** Whether what arrived goes into the timeline now: a post unless it pauses, and a break only at the top. */
export function takesNow(arrival: Arrival, { autoInsert, atTop, inside }: Reading): boolean {
  if (!autoInsert || inside) return false;
  return arrival.kind === "statuses" || atTop;
}

/** What the button tells: how many posts it brings or went in above, and whether posts may have been missed. */
type NewPostsCount = { readonly count: number; readonly gap: boolean };

/**
 * The policy of the home timeline's new posts with the setting `autoInsert`: what its feed hears, what its button
 * tells (undefined while it has nothing to tell), and what a press does.
 */
export function useNewHomePosts(autoInsert: boolean): {
  onTopChange: (atTop: boolean) => void;
  onReaderChange: (inside: boolean) => void;
  told: NewPostsCount | undefined;
  onPress: () => void;
} {
  const [atTop, setAtTop] = useState(true);
  const [inside, setInside] = useState(false);
  // The posts that went in above since the reader left the top.
  const [above, setAbove] = useState(0);
  const goIn = (arrival: Arrival, reading: Reading) => {
    const now = takesNow(arrival, reading);
    if (now && arrival.kind === "statuses" && !reading.atTop) setAbove((n) => n + arrival.count);
    return now;
  };
  const arrivals = useHomeArrivals((arrival) => goIn(arrival, { autoInsert, atTop, inside }));
  const { statuses, gap } = arrivals.held;
  const count = above + statuses.length;
  return {
    onTopChange: (top) => {
      setAtTop(top);
      if (top) setAbove(0);
    },
    onReaderChange: (now) => {
      setInside(now);
      // Leaving the feed puts in what was held while the reader was in it.
      const held: Arrival = gap ? { kind: "gap" } : { kind: "statuses", count: statuses.length };
      if (!now && (gap || statuses.length > 0) && goIn(held, { autoInsert, atTop, inside: false })) {
        void arrivals.take();
      }
    },
    told: count > 0 || gap ? { count, gap } : undefined,
    onPress: () => {
      setAbove(0);
      void arrivals.take();
    },
  };
}
