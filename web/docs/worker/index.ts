// SPDX-License-Identifier: AGPL-3.0-only
// The Worker of the documentation site (scripts/docs-site/wrangler.jsonc). The static assets answer every request
// themselves, without this code; `assets.run_worker_first` lists the paths that come here instead, and each path has
// its handler below, which may build on the asset of the same path (env.ASSETS):
//   - the release notes page of each version (release-notes.ts);
//   - on a pull request's Preview, the root and /latest/, which have no asset there and open /next/ (previewRedirect):
//     the Preview serves /next/ alone, and `wrangler preview` refuses a _redirects file among the assets
//     (cloudflare/workers-sdk#15791), so scripts/docs-site.sh stage-preview routes these paths here instead. A path of
//     them that has its asset, such as /latest/releases/ of the site, is answered as any other.
import { isReleaseNotesPath, withReleaseNotes } from "./release-notes";

export interface Env {
  ASSETS: WorkerFetcher;
  /** A GitHub token that reads the public releases, a secret of the Worker; GitHub is asked unauthenticated without it. */
  GITHUB_TOKEN?: string;
}

export interface Runtime {
  cache: WorkerCache;
  fetch: (request: Request) => Promise<Response>;
  now: () => number;
}

/**
 * The /next/ path that a Preview redirects `pathname` to: / and /latest to /next/, and /latest/<rest> to /next/<rest>;
 * null for any other path.
 */
export function previewRedirect(pathname: string): string | null {
  if (pathname === "/" || pathname === "/latest") return "/next/";
  if (pathname.startsWith("/latest/")) return `/next/${pathname.slice("/latest/".length)}`;
  return null;
}

/** Answers `request` with the handler of its path, or with the asset of the path. */
export async function handle(request: Request, env: Env, ctx: WorkerContext, runtime: Runtime): Promise<Response> {
  const url = new URL(request.url);
  const read = request.method === "GET" || request.method === "HEAD";
  const target = read ? previewRedirect(url.pathname) : null;
  if (target !== null) {
    const asset = await env.ASSETS.fetch(request);
    if (asset.status === 404) return Response.redirect(new URL(target + url.search, url).toString(), 302);
    if (!isReleaseNotesPath(url.pathname)) return asset;
  }
  if (read && isReleaseNotesPath(url.pathname)) {
    // The page the Worker serves differs from the asset, so the browser's validators of the asset do not apply to it.
    const headers = new Headers(request.headers);
    headers.delete("if-none-match");
    headers.delete("if-modified-since");
    const asset = await env.ASSETS.fetch(new Request(request.url, { method: request.method, headers }));
    return withReleaseNotes(asset, { ...runtime, ctx, token: env.GITHUB_TOKEN });
  }
  return env.ASSETS.fetch(request);
}

export default {
  fetch(request: Request, env: Env, ctx: WorkerContext): Promise<Response> {
    return handle(request, env, ctx, { cache: caches.default, fetch: (r) => fetch(r), now: Date.now });
  },
};
