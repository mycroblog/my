// SPDX-License-Identifier: AGPL-3.0-only

// What the app decides by the viewport (ADR-0025 §4: only the app reads it; components and features follow their
// container): whether it is a phone's, narrower than `tablet`, where a post's emoji picker opens as a sheet from the
// bottom instead of a popover beside its button, and a feed is read anew by a pull from its top.

import { breakpoint } from "@my/design-tokens";
import { useSyncExternalStore } from "react";

const query = `(width < ${String(breakpoint.tablet)}rem)`;

const media = () => (typeof window.matchMedia === "function" ? window.matchMedia(query) : undefined);

const subscribe = (listener: () => void) => {
  const list = media();
  list?.addEventListener("change", listener);
  return () => {
    list?.removeEventListener("change", listener);
  };
};

const snapshot = () => media()?.matches ?? false;

/** Whether the viewport is a phone's, narrower than `tablet`; following it as it changes. */
export function useNarrowViewport(): boolean {
  return useSyncExternalStore(subscribe, snapshot, () => false);
}
