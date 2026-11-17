// SPDX-License-Identifier: AGPL-3.0-only
// The Worker of the documentation site (scripts/docs-site/wrangler.jsonc). The static assets answer every request
// themselves, without this code; `assets.run_worker_first` lists the paths that come here instead, and each path has
// its handler below, which may build on the asset of the same path (env.ASSETS):
//   - the release notes page of each version (release-notes.ts).
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

/** Answers `request` with the handler of its path, or with the asset of the path. */
export async function handle(request: Request, env: Env, ctx: WorkerContext, runtime: Runtime): Promise<Response> {
  const url = new URL(request.url);
  if ((request.method === "GET" || request.method === "HEAD") && isReleaseNotesPath(url.pathname)) {
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
