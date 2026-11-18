// SPDX-License-Identifier: AGPL-3.0-only

// The guard of a form saved as a whole (ADR-0110 §5), for the route of its screen: while the feature says it holds
// changes that are not saved (`onUnsaved`), leaving the screen is held back, whichever way the reader leaves (a link,
// the navigation, Back, closing the tab), until they say whether to: the feature asks (`leaving`), here the router
// waits. An address of the same path (the one that asks about signing out over the screen) is no leaving. Once the
// session is ending (sign-out.tsx: signed out here or in another tab, or ended by the server), nothing is held any
// more, the tab's unload neither: the app leaves by a load of the page, and a reader the browser asked to stay would
// stay on a form of a session that is gone.

import { useBlocker, useRouter } from "@tanstack/react-router";
import { useCallback, useRef } from "react";
import { sessionEnding } from "./sign-out";

/** What the route gives the feature's screen: who hears of its unsaved changes, and the reader held back, if they are. */
export type UnsavedGuard = {
  readonly onUnsaved: (unsaved: boolean) => void;
  readonly leaving: { readonly onStay: () => void; readonly onLeave: () => void } | undefined;
};

/** Holds the router while the screen says it has changes that are not saved, until the session is ending. */
export function useUnsavedGuard(): UnsavedGuard {
  const router = useRouter();
  // Read as the router asks, so that a way out right after a save is not held by what the save just changed.
  const unsaved = useRef(false);
  const held = useCallback(() => unsaved.current && !sessionEnding(router.options.context.data), [router]);
  const blocker = useBlocker({
    shouldBlockFn: ({ current, next }) => held() && current.pathname !== next.pathname,
    enableBeforeUnload: held,
    withResolver: true,
  });
  const onUnsaved = useCallback((now: boolean) => {
    unsaved.current = now;
  }, []);
  return {
    onUnsaved,
    leaving: blocker.status === "blocked" ? { onStay: blocker.reset, onLeave: blocker.proceed } : undefined,
  };
}
