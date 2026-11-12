// SPDX-License-Identifier: AGPL-3.0-only
// The part of the Cloudflare Workers runtime the site's Worker uses (https://developers.cloudflare.com/workers/runtime-apis/),
// declared here rather than through @cloudflare/workers-types, whose globals would replace the ones of the site's
// scripts. Bun has the same HTMLRewriter, so `bun test` runs the Worker's code as it is.

interface WorkerElement {
  readonly tagName: string;
  readonly attributes: IterableIterator<[string, string]>;
  removeAttribute(name: string): WorkerElement;
  remove(): WorkerElement;
  removeAndKeepContent(): WorkerElement;
  setInnerContent(content: string, options?: { html: boolean }): WorkerElement;
}

declare class HTMLRewriter {
  on(selector: string, handlers: { element?(element: WorkerElement): void }): HTMLRewriter;
  transform(response: Response): Response;
}

/** The `ctx` of a Worker's fetch handler. */
interface WorkerContext {
  waitUntil(promise: Promise<unknown>): void;
}

/** A binding with a fetch method: the static assets (env.ASSETS). */
interface WorkerFetcher {
  fetch(request: Request): Promise<Response>;
}

/** A cache of the Cache API (caches.default), local to the data center. */
interface WorkerCache {
  match(request: Request): Promise<Response | undefined>;
  put(request: Request, response: Response): Promise<void>;
}

/** The Workers runtime's `caches` has the cache of the data center as `default`. */
interface CacheStorage {
  readonly default: WorkerCache;
}
