// SPDX-License-Identifier: AGPL-3.0-only
// A fake server for the shell's tests: its instance, as GET /instance answers it with the fields the shell reads, and
// a client API that answers the instance and the health, up or down, through the app's watch of the server. No test
// reaches a live server.
import {
  createAppData,
  watchServer,
  webClientEpoch,
  type AppData,
  type Instance,
  type ServerWatch,
} from "@my/app-data";
import { createClient } from "@mycroblog/mykit";

const server = "https://example.com";
export const statusPage = "https://status.example.com";

/** The instance with every timeline open to everyone, the voice rooms on and the funding meter in the rail. */
export function instance(fields: Partial<Instance> = {}): Instance {
  return {
    domain: "example.com",
    name: "Example",
    status_page_url: statusPage,
    timelines: { local: "everyone", federated: "everyone" },
    voice_enabled: true,
    single_user: false,
    funding_meter: {
      visibility: "percent",
      label: "2026-11",
      starts_on: "2026-11-01",
      ends_on: "2026-11-30",
      percent: 72,
      covered: false,
      estimated: false,
      rail: true,
    },
    ...fields,
  } as Instance;
}

/** How the server answers: up; down, with its health `unavailable`; or out of the reader's reach. */
export type Mode = "up" | "outage" | "offline";

/**
 * The app's data on a server whose instance is `answered`, in the mode the test sets, which sends `epoch` as its
 * X-Web-Client-Epoch; through the app's watch of the server.
 */
export function app(
  answered: Instance = instance(),
  epoch = webClientEpoch,
): {
  s: { mode: Mode };
  watch: ServerWatch;
  data: AppData;
} {
  const s = { mode: "up" as Mode };
  const fetch = (request: Request): Promise<Response> => {
    if (s.mode === "offline") return Promise.reject(new TypeError("Failed to fetch"));
    const path = new URL(request.url).pathname;
    const answer =
      s.mode === "outage"
        ? Response.json(path.endsWith("/instance/health") ? { status: "unavailable" } : { status: 503 }, {
            status: 503,
          })
        : Response.json(
            path.endsWith("/instance/health") ? { status: "ok" } : path.endsWith("/instance") ? answered : {},
          );
    answer.headers.set("X-Web-Client-Epoch", String(epoch));
    return Promise.resolve(answer);
  };
  const watch = watchServer({ server, fetch, followDevice: false });
  const api = createClient({ server, fetch: watch.fetch, retry: { disabled: true } });
  return { s, watch, data: createAppData(api, undefined, watch) };
}
