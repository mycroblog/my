// SPDX-License-Identifier: AGPL-3.0-only
// The instance as the shell reads it (`GET /instance`, ADR-0110 §11, ADR-0088, ADR-0152, ADR-0059 §8): which
// timelines it offers, whether it is one person's, its closure, its funding meter, its status page and its logo; and
// the state of the server that the watch reads from every answer (server.ts). The status page's address is kept on
// the device from the last answer, since the shell needs it most when the server does not answer (ADR-0088 §20).
import type { components } from "@mycroblog/mykit";
import { readDeviceValue, writeDeviceValue } from "@my/platform";
import { useIsMutating, useQuery } from "@tanstack/react-query";
import { useEffect, useSyncExternalStore } from "react";
import type { AppData } from "./app-data";
import { useAppData } from "./provider";
import { unwatchedServer, type ServerState } from "./server";

/** The instance's public information. */
export type Instance = components["schemas"]["Instance"];

/** The closure of the service, while one is announced or after the server closed (ADR-0088 §2). */
export type Closure = components["schemas"]["InstanceClosure"];

/** The funding meter of this month (ADR-0067 §22). */
export type FundingMeter = components["schemas"]["FundingMeter"];

/** The instance's public information. */
export function instanceQuery({ queries }: AppData) {
  return queries.queryOptions("get", "/instance");
}

/** The device's key of the status page's address of a server. */
const statusPageKey = (server: string) => `my:status-page:${server}`;

/** The server's origin of `data`, which keys what the device keeps of it. */
const serverOf = (data: AppData) => data.connection?.server ?? data.watch?.server ?? "";

/** The instance's public information, undefined until the server answered. */
export function useInstance(): Instance | undefined {
  const data = useAppData();
  const instance = useQuery(instanceQuery(data)).data;
  const url = instance === undefined ? undefined : (instance.status_page_url ?? null);
  const key = statusPageKey(serverOf(data));
  useEffect(() => {
    if (url !== undefined) writeDeviceValue(key, url);
  }, [key, url]);
  return instance;
}

/**
 * The address of the instance's status page: the last answer's, which the device keeps while the server does not
 * answer; undefined when it has none.
 */
export function useStatusPageUrl(): string | undefined {
  const data = useAppData();
  const instance = useInstance();
  const maintenance = useServerState().maintenance;
  if (maintenance?.statusPageUrl !== undefined) return maintenance.statusPageUrl;
  if (instance !== undefined) return instance.status_page_url;
  return readDeviceValue(statusPageKey(serverOf(data))) ?? undefined;
}

/** The state of the server: its maintenance, whether it answers, and whether the app should load again. */
export function useServerState(): ServerState {
  const { watch } = useAppData();
  return useSyncExternalStore(watch?.subscribe ?? noSubscription, watch?.state ?? unwatched, watch?.state ?? unwatched);
}

/** What the shell does to the server's state: close the toast of a newer build, ask the server again. */
export function useServerActions(): { dismissUpdate: () => void; check: () => Promise<void> } {
  const { watch } = useAppData();
  return {
    dismissUpdate: () => {
      watch?.dismissUpdate();
    },
    check: () => watch?.check() ?? Promise.resolve(),
  };
}

/** Whether a write of the app is on its way to the server (ADR-0028 §13: no reload meanwhile). */
export function useWritesInFlight(): boolean {
  return useIsMutating() > 0;
}

const noSubscription = () => () => undefined;
const unwatched = () => unwatchedServer;
