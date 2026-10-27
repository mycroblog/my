// SPDX-License-Identifier: MIT
import createFetchClient, { type Client, type Middleware } from "openapi-fetch";
import { acceptsIdempotencyKey, idempotencyKeyHeader } from "./idempotency.js";
import { problemFromResponse } from "./problem.js";
import type { paths } from "./schema.gen.js";
import type { TokenSource } from "./token.js";
import { createTransport, type RetryPolicy, type Sleep } from "./transport.js";

/** The path of the client API under the server's origin (`servers[0].url` of api/openapi.yaml). */
export const apiBasePath = "/api/v1";

/** The openapi-fetch client of the client API, with the cross-cutting behaviour of the library. */
export type Api = Client<paths>;

/** Configures a client of one server. */
export type ClientOptions = {
  /** The origin of the server, such as "https://example.com". The client API is under apiBasePath. */
  server: string;
  /** Gives the access token of the requests. Absent sends them without one (anonymous). */
  tokenSource?: TokenSource;
  /**
   * Sent as Accept-Language, so that the server localizes the title and detail of its problems. A function is read
   * for every request, for an app whose locale changes while it runs. Absent leaves it to the server (or the browser).
   */
  acceptLanguage?: string | (() => string | null | undefined);
  /** The policy for 429 and 503 answers; it retries with the defaults when absent. */
  retry?: RetryPolicy;
  /** Sends the requests; the global `fetch` by default. */
  fetch?: (input: Request) => Promise<Response>;
  /** Waits between retries; for tests. */
  sleep?: Sleep;
};

/**
 * The client of one server's client API: openapi-fetch typed by the spec, every operation as `GET`, `POST`, ... of
 * its path. An answer with a status of 400 or more rejects with a ProblemError, and a request that got no answer
 * with a NetworkError, so `data` is set whenever the call resolves. Middleware added with `use` sees the answer
 * after the token refresh and the retries, and before the ProblemError. A request given its own `fetch` bypasses
 * the token, refresh, and retries.
 */
export function createClient(options: ClientOptions): Api {
  const origin = parseOrigin(options.server);
  const client = createFetchClient<paths>({
    baseUrl: origin + apiBasePath,
    fetch: createTransport({
      fetch: options.fetch ?? ((request) => globalThis.fetch(request)),
      basePath: apiBasePath,
      tokenSource: options.tokenSource,
      retry: options.retry,
      ...(options.sleep === undefined ? {} : { sleep: options.sleep }),
    }),
  });
  client.use(middleware(options.acceptLanguage));
  return client;
}

/**
 * Adds Accept-Language and the Idempotency-Key, and turns an error answer into a ProblemError. It is the first
 * middleware, so its onResponse runs last.
 */
function middleware(acceptLanguage: ClientOptions["acceptLanguage"]): Middleware {
  return {
    onRequest({ request, schemaPath }) {
      const language = typeof acceptLanguage === "function" ? acceptLanguage() : acceptLanguage;
      if (!request.headers.has("Accept-Language") && language !== undefined && language !== null && language !== "") {
        request.headers.set("Accept-Language", language);
      }
      if (!request.headers.has(idempotencyKeyHeader) && acceptsIdempotencyKey(request.method, schemaPath)) {
        // One key for every attempt, so that the server applies the request once.
        request.headers.set(idempotencyKeyHeader, crypto.randomUUID());
      }
      return request;
    },
    async onResponse({ response }) {
      if (response.status >= 400) throw await problemFromResponse(response);
      return undefined;
    },
  };
}

/** Reads the server's origin: an absolute http or https URL without a path, query, or fragment. */
export function parseOrigin(server: string): string {
  if (server === "") throw new Error("mykit: server is empty");
  let u: URL;
  try {
    u = new URL(server);
  } catch {
    throw new Error(`mykit: server ${JSON.stringify(server)} is not a URL`);
  }
  if ((u.protocol !== "http:" && u.protocol !== "https:") || (u.pathname !== "/" && u.pathname !== "") || u.search !== "" || u.hash !== "") {
    throw new Error(`mykit: server ${JSON.stringify(server)} is not an origin such as https://example.com`);
  }
  return u.origin;
}
