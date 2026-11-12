// SPDX-License-Identifier: AGPL-3.0-only
// The health of remote servers (ADR-0006 §3): the list under a state filter, and one server's health
// with the history of its availability and marking it available again.
import { useNotice, useToast } from "@my/ui";
import { useMutation, useQueryClient, useSuspenseInfiniteQuery, useSuspenseQuery } from "@tanstack/react-query";
import { useApi } from "../../../lib/auth/SessionProvider";
import { errorMessage } from "../../../lib/errors";
import { useT } from "../../../lib/i18n";
import {
  type AvailabilityEvent,
  cacheMarkedAvailable,
  availabilityEventsQueryOptions,
  type InstanceFilter,
  type InstanceHealth,
  instanceQueryOptions,
  instancesQueryOptions,
  markAvailableMutation,
} from "../api/staff";
import { type PagedPages, toPagedList } from "./pagedList";

/** The address of a remote server's page. */
export function instancePath(domain: string): string {
  return `/moderation/instances/${encodeURIComponent(domain)}`;
}

/** Whether a text starts with a URL scheme (`https://`). */
const schemePattern = /^[a-z][a-z\d+.-]*:\/\//iu;

/**
 * The server a moderator typed or pasted, as the API names servers (IDNA A-labels in lower case without a trailing
 * dot): from a domain, the domain of a handle (`@alice@mastodon.example`), or a URL on the server. `undefined` when
 * it names no host. The server, not this code, decides what a host name is.
 */
export function instanceDomainOf(input: string): string | undefined {
  let text = input.trim();
  if (text === "") return undefined;
  if (!schemePattern.test(text)) {
    const slash = text.indexOf("/");
    const at = text.lastIndexOf("@", slash === -1 ? text.length : slash);
    text = `https://${text.slice(at + 1)}`;
  }
  let url: URL;
  try {
    url = new URL(text);
  } catch {
    return undefined;
  }
  if (url.protocol !== "https:" && url.protocol !== "http:") return undefined;
  const host = url.hostname.replace(/\.$/u, "");
  return host === "" ? undefined : host;
}

/**
 * The server a remote account lives on, as its page names it: the host of the account's actor, which deliveries to
 * the account go to, since the domain of its handle may be another (WebFinger on a separate domain).
 */
export function accountServer(account: { uri: string; domain: string }): string {
  return instanceDomainOf(account.uri) ?? account.domain;
}

/** The remote servers under `filter`, in domain order. */
export function useInstances(filter: InstanceFilter): PagedPages<InstanceHealth> {
  const api = useApi();
  return toPagedList(useSuspenseInfiniteQuery(instancesQueryOptions(api, filter)));
}

/** One remote server: its health, why and when its availability changed, and marking it available. */
export type InstanceView = {
  health: InstanceHealth;
  events: PagedPages<AvailabilityEvent>;
  /** Clears the server's failures so that deliveries to it resume; the result is announced once the lists have it. */
  markAvailable: () => void;
  isMarking: boolean;
  /** Whether there is anything for markAvailable to clear. */
  canMarkAvailable: boolean;
};

export function useInstance(domain: string): InstanceView {
  const api = useApi();
  const t = useT();
  const queryClient = useQueryClient();
  const notice = useNotice();
  const toast = useToast();
  const health = useSuspenseQuery(instanceQueryOptions(api, domain));
  const events = useSuspenseInfiniteQuery(availabilityEventsQueryOptions(api, domain));
  const mark = useMutation({
    ...markAvailableMutation(api),
    onSuccess: async (next) => {
      // The lists show the server's state: it is marked available once they are fetched again (the button stays
      // pending until then), so that going back to a list shows the server as it is now (cacheMarkedAvailable).
      await cacheMarkedAvailable(api, queryClient, domain, next);
      notice.inPlace(t("client:admin.instance.marked_available", { domain }));
    },
    // The server keeps its state; the failure is shown and the button stays (web-client/README.md E2).
    onError: (error) => {
      toast.show({ message: t("client:admin.instance.mark_failed", { reason: errorMessage(error) }), tone: "danger" });
    },
  });
  return {
    health: health.data,
    events: toPagedList(events),
    markAvailable: () => {
      mark.mutate(domain);
    },
    isMarking: mark.isPending,
    canMarkAvailable: health.data.state === "unavailable" || health.data.failure_days > 0,
  };
}
