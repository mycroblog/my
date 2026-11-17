// SPDX-License-Identifier: AGPL-3.0-only
// The data of one signed-in session: the mykit client of the server and the TanStack Query cache, the one cache of
// server data (ADR-0028 §6, S1).
import type { Api, paths, StreamOptions } from "@mycroblog/mykit";
import { QueryClient } from "@tanstack/react-query";
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

/**
 * The cache with the defaults of ADR-0028 §7: data is fresh for 30 seconds; a stale query is fetched again when a
 * screen opens, never on focus or reconnect while it is read (Y7). The client retries 429 and 503 itself (ADR-0093
 * §3), so a query is not retried again.
 */
export function createQueryClient(): QueryClient {
  return new QueryClient({
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
