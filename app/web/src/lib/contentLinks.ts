// SPDX-License-Identifier: AGPL-3.0-only
// The links in a post body or a profile that lead to this instance's own pages. The server renders them
// as absolute URLs under https://{host}/ (the hashtags and mentions of a post, ADR-0018), the same in
// the HTML it federates, so they cannot follow the scheme the app is served at: on the local plain-HTTP
// setup (web_origin http://localhost:8080) https://localhost:8080/ does not open. The app opens them at
// its own pages through the router instead, whatever the scheme, as it does every link between its pages.
import { useRouter } from "@tanstack/react-router";
import { useConfig } from "./config";

/** The paths of the app's pages that the server links to from rendered HTML: a profile, a post, a hashtag. */
const contentPagePath = /^\/(?:@[^/]+(?:\/status\/[^/]+)?|tags\/[^/]+)$/;

/**
 * The app's address of `href` when it is a link to a profile, a post or a hashtag page on this instance
 * (whose host is the host of `apiOrigin`, over http or https), or null for any other link.
 */
export function contentLinkPath(href: string, apiOrigin: string): string | null {
  let url: URL, origin: URL;
  try {
    url = new URL(href);
    origin = new URL(apiOrigin);
  } catch {
    return null;
  }
  if (
    (url.protocol !== "https:" && url.protocol !== "http:") ||
    url.host !== origin.host ||
    !contentPagePath.test(url.pathname)
  ) {
    return null;
  }
  return url.pathname + url.search + url.hash;
}

/**
 * The `onLinkClick` of a `PostContent`: a link to one of this instance's pages opens that page through
 * the router; any other link is left to the browser.
 */
export function useContentLinkClick(): (event: MouseEvent, link: HTMLAnchorElement) => void {
  const router = useRouter();
  const { apiOrigin } = useConfig();
  return (event, link) => {
    const path = contentLinkPath(link.href, apiOrigin);
    if (path === null) return;
    event.preventDefault();
    void router.navigate({ href: path });
  };
}
