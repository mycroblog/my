// SPDX-License-Identifier: AGPL-3.0-only
import { useQuery } from "@tanstack/react-query";
import { useEffect, useState } from "react";
import { useApi, useSessionStatus, useSessionUnreachable } from "../../../lib/auth/SessionProvider";
import { shellQueryRetry } from "../../../lib/query";
import { timelinesKey } from "../../../lib/storage";
import { instanceQueryOptions, type InstanceAudience } from "../api/timelines";

/**
 * Which public timelines to offer the viewer: an instance setting turns each off (it is then a 404) or keeps it for
 * signed-in users (a visitor then gets a 401) (FR-202).
 */
export type TimelineAvailability = {
  /** Whether to offer the local timeline (`/local`). */
  local: boolean;
  /** Whether to offer the federated timeline (`/public`). */
  federated: boolean;
};

/** Whether a timeline of `access` is offered to a viewer who is or is not signed in. */
function offered(access: InstanceAudience, signedOut: boolean): boolean {
  return access === "everyone" || (access === "signed_in" && !signedOut);
}

/**
 * Which public timelines the instance serves to the viewer, from GET /instance and whether the viewer is signed in.
 * The local timeline is part of every instance's navigation, so it is offered until the instance says who may read
 * it. The federated timeline is offered only once the instance says so, so that an instance that turned it off never
 * shows it, not even while GET /instance loads or after it failed. Offline, a session that could not be resumed is
 * most likely still there, so it counts as signed in (web-client/11-pwa.md §11), as the navigation does.
 *
 * The last answer is kept in localStorage (lib/storage's timelinesKey: an instance setting, the same for every user of the origin), so
 * that a page opened or reloaded offline, where GET /instance fails, offers the same timelines as before
 * (web-client/16-look-states-and-wording.md G17). It is read once when the navigation mounts and gives way to the server's answer.
 */
export function useTimelineAvailability(): TimelineAvailability {
  const api = useApi();
  const instance = useQuery({ ...instanceQueryOptions(api), retry: shellQueryRetry });
  const status = useSessionStatus();
  const unreachable = useSessionUnreachable();
  const [stored] = useState(() => timelinesKey.read());
  const local = instance.data?.timelines.local;
  const federated = instance.data?.timelines.federated;
  useEffect(() => {
    if (local !== undefined && federated !== undefined) timelinesKey.write({ local, federated });
  }, [local, federated]);
  const signedOut = status === "signed-out" && !unreachable;
  return {
    local: offered(local ?? stored?.local ?? "everyone", signedOut),
    federated: offered(federated ?? stored?.federated ?? "off", signedOut),
  };
}
