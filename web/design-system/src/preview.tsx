// SPDX-License-Identifier: AGPL-3.0-only

// A state shown at rest, for a catalog of the components (ADR-0168): hover, press and keyboard focus follow the
// pointer and the keyboard, so a page that shows them side by side asks for them here. Only the subpath
// @my/design-system/preview exports it; screens never use it.

import { createContext, useContext, type ReactNode } from "react";

export type PreviewState = "hover" | "pressed" | "focus-visible";

const PreviewContext = createContext<PreviewState | undefined>(undefined);

/** The state a component shows, the real ones aside. */
export const usePreviewState = () => useContext(PreviewContext);

/** Shows its components in `state`. */
export function Preview({ state, children }: { state: PreviewState | undefined; children: ReactNode }) {
  return <PreviewContext value={state}>{children}</PreviewContext>;
}
