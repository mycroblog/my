// SPDX-License-Identifier: AGPL-3.0-only
import { createContext, use } from "react";

/**
 * Whether the layout shows the third column (from xl, signed in), for a page that leaves out what the column shows
 * (the home page's announcements, web-client/09-app-shell.md §9). Only the layout reads the viewport
 * (web-client/README.md D2); pages ask it here.
 */
export const AsideContext = createContext(false);

/** Whether the layout shows the third column now. */
export function useHasAside(): boolean {
  return use(AsideContext);
}
