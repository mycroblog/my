// SPDX-License-Identifier: AGPL-3.0-only

// The focus of a page that opens on what the server refused (ADR-0025 §4: the page's focus is the app's): an auth
// screen loads anew after a refused post, and its alert, right before the button that sent the form, takes the focus,
// so that it is read first and the button is the next stop.

import { focusOnLoadAttribute } from "@my/design-system";

/** Moves the focus to the element of `root` that asks for it when the page opens, if there is one. */
export function focusOnLoad(root: Pick<ParentNode, "querySelector">): void {
  const target = root.querySelector<HTMLElement>(`[${focusOnLoadAttribute}]`);
  target?.focus();
}
