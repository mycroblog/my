// SPDX-License-Identifier: AGPL-3.0-only

// Where the composer is open (ADR-0025 §4): over the screen the reader is on, by the address's search (`?compose=new`,
// `?compose=reply&to=<id>`, `?compose=quote&to=<id>`), so that the screen behind stays where it was, the browser's back
// (a phone's back gesture) leaves the composer, and the composer opens again on a reload. It is a screen of its own on
// a phone and a dialog from `tablet` (the app's choice by the viewport); opening it raises it (`modal`, transitions.ts).

import type { ComposeTarget } from "@my/app-features";
import { useNavigate, useRouter, useRouterState } from "@tanstack/react-router";

/** The composer's part of the address's search, which every route takes (__root.tsx). */
export type ComposeSearch = { compose?: "new" | "reply" | "quote" | undefined; to?: string | undefined };

/** Reads the composer's part of a search, dropping what it does not know. */
export function parseComposeSearch(search: Record<string, unknown>): ComposeSearch {
  const compose = search["compose"];
  const to = search["to"];
  if (compose === "new") return { compose };
  if ((compose === "reply" || compose === "quote") && (typeof to === "string" || typeof to === "number"))
    return { compose, to: String(to) };
  return {};
}

/** The target of a search, or none while the composer is closed. */
export function composeTarget(search: ComposeSearch): ComposeTarget | undefined {
  if (search.compose === "new") return { kind: "new" };
  if ((search.compose === "reply" || search.compose === "quote") && search.to !== undefined)
    return { kind: search.compose, id: search.to };
  return undefined;
}

declare module "@tanstack/react-router" {
  interface HistoryState {
    /** The composer was opened from the entry before this one: closing it goes back there. */
    composer?: boolean;
  }
}

/** The composer's target, and how to open and close it. */
export function useCompose() {
  const router = useRouter();
  const navigate = useNavigate();
  // The search of the screen shown, not of the location: the location changes when a navigation starts, before the
  // View Transition takes its snapshot of the old screen, and the composer would be in it already (rising twice); the
  // shown screen's match changes inside the transition.
  const target = useRouterState({
    select: (state) => composeTarget(parseComposeSearch(state.matches.at(-1)?.search ?? {})),
  });
  const open = (to: ComposeTarget) => {
    void navigate({
      to: ".",
      search: (prev) => ({
        ...prev,
        compose: to.kind,
        to: to.kind === "new" ? undefined : to.id,
      }),
      state: (prev) => ({ ...prev, composer: true }),
    });
  };
  const close = () => {
    if (router.state.location.state.composer === true) {
      router.history.back();
      return;
    }
    void navigate({ to: ".", search: (prev) => ({ ...prev, compose: undefined, to: undefined }), replace: true });
  };
  return { target, open, close };
}
