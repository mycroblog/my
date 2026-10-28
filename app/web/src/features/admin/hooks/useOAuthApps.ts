// SPDX-License-Identifier: AGPL-3.0-only
// The OAuth apps of the admin screen (ADR-0090 §1, manage_oauth_apps): the clients under a registration route,
// disabling one and lifting that, and denying and allowing the host of Client ID Metadata Documents. What each
// change did is announced and kept in a StatusMessage; a refusal is said in the server's words next to the list.
import { useAnnouncer } from "@my/ui";
import { useMutation, useQueryClient, useSuspenseInfiniteQuery } from "@tanstack/react-query";
import { useState } from "react";
import { useApi } from "../../../lib/auth/SessionProvider";
import { errorMessage } from "../../../lib/errors";
import { useT } from "../../../lib/i18n";
import { type PagedPages, toPagedList } from "../../moderation";
import {
  allowHostMutation,
  denyHostMutation,
  type HostDenial,
  hostDenialsQueryOptions,
  type OAuthClient,
  type OAuthClientFilter,
  oauthClientsQueryOptions,
  refreshOAuthApps,
  setOAuthClientDisabledMutation,
} from "../api/oauthApps";

/** An app's name for a sentence: isolated left to right from the text around it (U+2066 … U+2069), as it is the app's own claim. */
export function appName(client: Pick<OAuthClient, "name">): string {
  return `⁦${client.name}⁩`;
}

/** The host of a client that registered by its metadata document, whose `client_id` is that document's URL. */
export function metadataHost(client: Pick<OAuthClient, "client_id" | "registration">): string | undefined {
  if (client.registration !== "metadata_document") return undefined;
  try {
    return new URL(client.client_id).hostname.toLowerCase();
  } catch {
    return undefined;
  }
}

/** The clients under `filter`, newest first. */
export function useOAuthClients(filter: OAuthClientFilter): PagedPages<OAuthClient> {
  const api = useApi();
  return toPagedList(useSuspenseInfiniteQuery(oauthClientsQueryOptions(api, filter)));
}

/** The hosts whose metadata documents are denied, newest denial first. */
export function useHostDenials(): PagedPages<HostDenial> {
  const api = useApi();
  return toPagedList(useSuspenseInfiniteQuery(hostDenialsQueryOptions(api)));
}

export type OAuthAppActions = {
  /** Disables a client (`true`) or lifts that (`false`). */
  setDisabled: (client: OAuthClient, disable: boolean) => void;
  /** Denies a host; resolves with whether it succeeded. */
  denyHost: (host: string) => Promise<boolean>;
  /** Lifts the denial of a host. */
  allowHost: (host: string) => void;
  /** The client or the host being changed. */
  pendingId: string | undefined;
  /** Why the last change failed, in the server's words, until the next change. */
  error: string | undefined;
  /** What the last change did, for the list's StatusMessage. */
  status: string | undefined;
};

/** Changing the clients and the denied hosts. */
export function useOAuthAppActions(): OAuthAppActions {
  const api = useApi();
  const t = useT();
  const queryClient = useQueryClient();
  const announce = useAnnouncer();
  const [status, setStatus] = useState<string | undefined>(undefined);
  const report = (message: string) => {
    setStatus(message);
    announce(message);
  };
  const disabling = useMutation(setOAuthClientDisabledMutation(api));
  const denying = useMutation(denyHostMutation(api));
  const allowing = useMutation(allowHostMutation(api));
  const resetAll = () => {
    disabling.reset();
    denying.reset();
    allowing.reset();
    setStatus(undefined);
  };
  const pendingId = disabling.isPending ? disabling.variables.id : allowing.isPending ? allowing.variables : undefined;
  const failed = disabling.isError
    ? disabling.error
    : denying.isError
      ? denying.error
      : allowing.isError
        ? allowing.error
        : undefined;
  return {
    setDisabled: (client, disable) => {
      resetAll();
      disabling.mutate(
        { id: client.id, disable },
        {
          onSuccess: () => {
            void refreshOAuthApps(queryClient).then(() => {
              report(
                t(disable ? "client:admin.oauth_apps.disabled_done" : "client:admin.oauth_apps.enabled_done", {
                  name: appName(client),
                }),
              );
            });
          },
        },
      );
    },
    denyHost: async (host) => {
      resetAll();
      try {
        await denying.mutateAsync(host);
      } catch {
        return false;
      }
      await refreshOAuthApps(queryClient);
      report(t("client:admin.oauth_apps.hosts.denied_done", { host }));
      return true;
    },
    allowHost: (host) => {
      resetAll();
      allowing.mutate(host, {
        onSuccess: () => {
          void refreshOAuthApps(queryClient).then(() => {
            report(t("client:admin.oauth_apps.hosts.allowed_done", { host }));
          });
        },
      });
    },
    pendingId,
    error: failed === undefined ? undefined : errorMessage(failed),
    status,
  };
}
