// SPDX-License-Identifier: AGPL-3.0-only
// The data of one signed-in session: the mykit client of the server and the TanStack Query cache, the one cache of
// server data (ADR-0028 §6, S1). The data ends with its session: closed, it keeps nothing and asks nothing more.
import type { Api, paths, StreamOptions } from "@mycroblog/mykit";
import {
  QueryCache,
  QueryClient,
  type DefaultedQueryObserverOptions,
  type DefaultError,
  type QueryClientConfig,
  type QueryKey,
  type QueryObserverOptions,
} from "@tanstack/react-query";
import createQueryHooks, { type OpenapiQueryClient } from "openapi-react-query";
import type { ServerWatch } from "./server";

/** How long a query's data counts as fresh before opening a screen fetches it again in the background (ADR-0028 §7). */
const defaultStaleTime = 30_000;

/** What the stream of a session connects with: the server, the access token, and how requests are sent. */
type StreamConnection = Pick<StreamOptions, "server" | "tokenSource" | "acceptLanguage" | "fetch">;

/**
 * What @my/app-data reads and writes through: the API, its TanStack Query connection, and the cache; for the
 * browser's session, what its stream connects with; and the watch of the server that the API's requests go through.
 */
export type AppData = {
  readonly api: Api;
  readonly queries: OpenapiQueryClient<paths>;
  readonly queryClient: QueryClient;
  readonly connection?: StreamConnection;
  readonly watch?: ServerWatch;
};

/**
 * Connects the mykit client of one server to a cache. The cache is new unless one is given; a change of the viewer
 * starts a new one (ADR-0028 §7). With the `watch` that the client's requests go through, what failed while the server
 * did not answer is read again once it does.
 */
export function createAppData(api: Api, queryClient: QueryClient = createQueryClient(), watch?: ServerWatch): AppData {
  // Data whose session ended asks nothing more of the server (`closeData`): a request refused here is never sent.
  api.use({
    onRequest: () => {
      if (isClosed(queryClient)) throw new Error("app-data: the session of this data ended, and it asks nothing more");
      return undefined;
    },
  });
  if (watch !== undefined) {
    let answering = answers(watch);
    watch.subscribe(() => {
      const now = answers(watch);
      if (now && !answering) void queryClient.refetchQueries({ predicate: (query) => query.state.status === "error" });
      answering = now;
    });
  }
  return { api, queries: createQueryHooks(api), queryClient, ...(watch === undefined ? {} : { watch }) };
}

/** Whether the server of `watch` answers: reached, and not under maintenance. */
function answers(watch: ServerWatch): boolean {
  const { connection, maintenance } = watch.state();
  return connection === "online" && maintenance === null;
}

/** The store of a session's cache, which keeps nothing more once the session ended: what is added then is dropped. */
class SessionCache extends QueryCache {
  closed = false;

  override add(query: Parameters<QueryCache["add"]>[0]): void {
    if (!this.closed) super.add(query);
  }
}

/**
 * The cache of one session. Once the session ended (`close`), it is empty and stays so, and the screens still drawn
 * on it read nothing: what was on its way is cancelled, what it held is removed, a query that a screen asks for again
 * as it draws is not kept, and no such query is read from the server, whatever the screen says of it (`enabled`).
 * The screens are not taken down with the session, since what ended it may still stand over them (the confirmation of
 * signing out, until the page loads anew), and a screen that draws again would otherwise read what was removed.
 */
class SessionQueryClient extends QueryClient {
  readonly #cache: SessionCache;

  constructor(config: QueryClientConfig) {
    const queryCache = new SessionCache();
    super({ ...config, queryCache });
    this.#cache = queryCache;
  }

  get closed(): boolean {
    return this.#cache.closed;
  }

  close(): void {
    this.#cache.closed = true;
    void this.cancelQueries();
    this.clear();
  }

  override defaultQueryOptions<
    TQueryFnData = unknown,
    TError = DefaultError,
    TData = TQueryFnData,
    TQueryData = TQueryFnData,
    TQueryKey extends QueryKey = QueryKey,
    TPageParam = never,
  >(
    options:
      | QueryObserverOptions<TQueryFnData, TError, TData, TQueryData, TQueryKey, TPageParam>
      | DefaultedQueryObserverOptions<TQueryFnData, TError, TData, TQueryData, TQueryKey>,
  ): DefaultedQueryObserverOptions<TQueryFnData, TError, TData, TQueryData, TQueryKey> {
    const defaulted = super.defaultQueryOptions(options);
    return this.closed ? { ...defaulted, enabled: false } : defaulted;
  }
}

/**
 * The cache with the defaults of ADR-0028 §7: data is fresh for 30 seconds; a stale query is fetched again when a
 * screen opens, never on focus or reconnect while it is read (Y7). The client retries 429 and 503 itself (ADR-0093
 * §3), so a query is not retried again. It can be closed with its session (`closeData`).
 */
export function createQueryClient(): QueryClient {
  return new SessionQueryClient({
    defaultOptions: {
      queries: {
        staleTime: defaultStaleTime,
        refetchOnWindowFocus: false,
        refetchOnReconnect: false,
        retry: false,
      },
    },
  });
}

/** Whether the session of the data that reads through `queryClient` ended (`closeData`). */
function isClosed(queryClient: QueryClient): boolean {
  return queryClient instanceof SessionQueryClient && queryClient.closed;
}

/**
 * What `asked` gives, while the session of `data` lasts: once the data is closed (`closeData`), before it is asked or
 * before it settles, it never settles, so that nothing comes of an answer that arrives after the session ended (a file
 * saved from a tab that another signed out). The data's API refuses only a request sent after the close.
 */
export function whileOpen<T>(data: AppData, asked: () => Promise<T>): Promise<T> {
  const never = new Promise<never>(() => undefined);
  if (isClosed(data.queryClient)) return never;
  return asked().then(
    (value) => (isClosed(data.queryClient) ? never : value),
    (error: unknown) => {
      if (isClosed(data.queryClient)) return never;
      throw error;
    },
  );
}

/**
 * Ends what `data` reads and keeps, with its session: its cache of server data is emptied and stays empty, the reads
 * on their way are cancelled, the screens still drawn on it read nothing more, and no request of its API is sent
 * again (each rejects). Data on a cache that is not this module's has its cache emptied, and no more.
 */
export function closeData({ queryClient }: AppData): void {
  if (queryClient instanceof SessionQueryClient) queryClient.close();
  else queryClient.clear();
}
