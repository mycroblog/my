// SPDX-License-Identifier: AGPL-3.0-only
// Route focus management, once for every page (web-client/README.md Y5). After the router renders a new page
// it names the document after the page's h1 and, when the path changed, moves focus to that h1 so that a
// screen reader reads the new page and keyboard users continue from its start. The first page of a
// visit keeps the browser's own focus; only its title is set. A new name of the instance renames the
// document too, without moving focus.
import { useRouter } from "@tanstack/react-router";
import { type RefObject, useEffect, useEffectEvent } from "react";
import { useT } from "../../lib/i18n";
import { useInstanceName } from "../../lib/instanceName";

/** Keeps `document.title` and focus in step with the page rendered inside `main`. */
export function useViewChange(main: RefObject<HTMLElement | null>): void {
  const router = useRouter();
  const t = useT();
  const instanceName = useInstanceName();

  const onViewChange = useEffectEvent((moveFocus: boolean) => {
    const heading = main.current?.querySelector("h1") ?? null;
    const page = heading?.textContent.trim() ?? "";
    // A page whose heading is the server's name (the landing page) is named once, not "My Demo – My Demo".
    document.title =
      page === "" || page === instanceName
        ? instanceName
        : t("client:shell.document_title", { page, instance: instanceName });
    if (moveFocus && heading !== null) {
      // Focusable by script only; PageHeading already sets it, other headings get it here.
      if (!heading.hasAttribute("tabindex")) heading.tabIndex = -1;
      // The router's scroll restoration places the page, and a page may scroll to what it is about
      // (a post below its thread): moving focus leaves the scroll position alone.
      heading.focus({ preventScroll: true });
    }
  });

  // The router is outside React: subscribe to its render events, and unsubscribe on unmount. A new instance
  // name subscribes again, which names the document after it at once.
  useEffect(() => {
    onViewChange(false);
    return router.subscribe("onRendered", (event) => {
      // A change of only the search or the hash (a tab, a filter) stays on the same page and keeps focus.
      onViewChange(event.fromLocation !== undefined && event.pathChanged);
    });
  }, [router, instanceName]);
}
