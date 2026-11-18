// SPDX-License-Identifier: AGPL-3.0-only
// What the settings' Apps screen reads at once (ADR-0090 §1, §5): the apps the account authorized and the apps it
// registered. Each is a query of its own (authorized-apps.ts, oauth-apps.ts); here they are read together, for the
// screen's loader and for the one notice that says they load or failed.
import { useQueries } from "@tanstack/react-query";
import type { AppData } from "./app-data";
import { authorizedAppsQuery } from "./authorized-apps";
import { oAuthAppsQuery } from "./oauth-apps";
import { useAppData } from "./provider";
import { readOf, type ScreenRead } from "./reads";

/** Reads what the Apps screen shows, from the cache where it holds it; a read that fails is the screen's to say. */
export async function loadApps(data: AppData): Promise<void> {
  const { queryClient } = data;
  await Promise.all([
    queryClient.query({ ...authorizedAppsQuery(data), staleTime: Infinity }),
    queryClient.query({ ...oAuthAppsQuery(data), staleTime: Infinity }),
  ]).catch(() => undefined);
}

/**
 * Where the reads of the Apps screen are: `read` once the server answered both, `failed` when it did not answer one,
 * with `retry`, which reads again those that failed, and `loading` until then.
 */
export function useAppsRead(): ScreenRead {
  const data = useAppData();
  return readOf(useQueries({ queries: [authorizedAppsQuery(data), oAuthAppsQuery(data)] }));
}
