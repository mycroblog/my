// SPDX-License-Identifier: AGPL-3.0-only
// The relays of the admin screens (ADR-0061, manage_settings): the list, registering and joining one, leaving it,
// joining it again and deleting its registration. What each change did is announced and kept in a StatusMessage;
// a refusal is said in the client's words for its reason, next to the field or the list.
import { useNotice } from "@my/ui";
import { useMutation, useQueryClient, useSuspenseQuery } from "@tanstack/react-query";
import { type SyntheticEvent, useState } from "react";
import { useApi } from "../../../lib/auth/SessionProvider";
import { errorMessage } from "../../../lib/errors";
import { type ClientMessageKey, useT } from "../../../lib/i18n";
import {
  createRelayMutation,
  deleteRelayMutation,
  refreshRelays,
  type Relay,
  type RelayProblemReason,
  relayProblemReason,
  type RelayProtocol,
  relaysQueryOptions,
  setRelayMembershipMutation,
} from "../api/relays";

const problemLabels: Readonly<Record<RelayProblemReason, ClientMessageKey>> = {
  url: "client:admin.relays.problem.url",
  exists: "client:admin.relays.problem.exists",
  domain_suspended: "client:admin.relays.problem.domain_suspended",
  not_actor: "client:admin.relays.problem.not_actor",
  state: "client:admin.relays.problem.state",
};

/** The host name of a relay's URL, or the URL itself when it does not parse. */
function relayHost(url: string): string {
  try {
    return new URL(url).host;
  } catch {
    return url;
  }
}

/** A relay for a sentence: the host name of its URL, isolated left to right (U+2066 … U+2069). */
export function relayName(relay: Pick<Relay, "url">): string {
  return `⁦${relayHost(relay.url)}⁩`;
}

/** The host name of a relay for its heading, without the isolates. */
export function relayHeading(relay: Pick<Relay, "url">): string {
  return relayHost(relay.url);
}

/** Why the server refused a relay operation, localized: the words for its `reason`, else the server's own. */
function useRefusal(): (error: unknown) => { reason: RelayProblemReason | undefined; message: string } {
  const t = useT();
  return (error) => {
    const reason = relayProblemReason(error);
    return { reason, message: reason === undefined ? errorMessage(error) : t(problemLabels[reason]) };
  };
}

/** The relays, in registration order. */
export function useRelays(): Relay[] {
  const api = useApi();
  return useSuspenseQuery(relaysQueryOptions(api)).data.items;
}

/** How the form joins a relay: detected by the server from its URL, or one of the protocols. */
export type RelayProtocolChoice = "auto" | RelayProtocol;

/** A relay as the form edits it. */
export type RelayDraft = { url: string; protocol: RelayProtocolChoice; receive: boolean };

const emptyDraft: RelayDraft = { url: "", protocol: "auto", receive: true };

/** Why registering a relay failed: on the URL when its reason is about the URL, else under the form. */
export type RelayFormError = { field: "url" | "form"; message: string };

/** The form of a new relay. A success empties it and says which relay was registered. */
export function useRelayForm() {
  const api = useApi();
  const t = useT();
  const queryClient = useQueryClient();
  const refusal = useRefusal();
  const [draft, setDraft] = useState<RelayDraft>(emptyDraft);
  const [created, setCreated] = useState<Relay | undefined>(undefined);
  const mutation = useMutation({
    ...createRelayMutation(api),
    onMutate: () => {
      setCreated(undefined);
    },
    onSuccess: async (relay) => {
      await refreshRelays(queryClient);
      setDraft(emptyDraft);
      setCreated(relay);
    },
  });
  const error = (): RelayFormError | undefined => {
    if (!mutation.isError) return undefined;
    const { reason, message } = refusal(mutation.error);
    return { field: reason === undefined || reason === "state" ? "form" : "url", message };
  };
  return {
    draft,
    change: (change: Partial<RelayDraft>) => {
      setDraft((previous) => ({ ...previous, ...change }));
    },
    submit: (event: SyntheticEvent<HTMLFormElement>) => {
      event.preventDefault();
      if (mutation.isPending) return;
      mutation.mutate({
        url: draft.url.trim(),
        receive: draft.receive,
        ...(draft.protocol === "auto" ? {} : { protocol: draft.protocol }),
      });
    },
    isPending: mutation.isPending,
    /** Why registering failed; the draft stays. */
    error: error(),
    /** What the last submit did, for the form's StatusMessage. */
    status: created === undefined ? undefined : t("client:admin.relays.created", { name: relayName(created) }),
  };
}

export type RelayActions = {
  /** Joins a relay again (`true`) or leaves it (`false`). */
  setJoined: (relay: Relay, join: boolean) => void;
  /** Deletes the registration of a relay; resolves with whether it succeeded. */
  remove: (relay: Relay) => Promise<boolean>;
  /** The relay being changed. */
  pendingId: string | undefined;
  /** Why the last change failed, localized, until the next change. */
  error: string | undefined;
  /** What the last change did, for the list's StatusMessage. */
  status: string | undefined;
};

/**
 * Changing the relays of the list. `onDeleted(id)` is called once a deleted relay is about to leave the list (to
 * keep focus, lib/useFocusAfterRemoval); a relay that is left or joined again keeps its row, which shows the answer.
 */
export function useRelayActions(onDeleted: (id: string) => void): RelayActions {
  const api = useApi();
  const t = useT();
  const queryClient = useQueryClient();
  const notice = useNotice();
  const refusal = useRefusal();
  const [status, setStatus] = useState<string | undefined>(undefined);
  const report = (message: string) => {
    setStatus(message);
    notice.inPlace(message);
  };
  const membership = useMutation(setRelayMembershipMutation(api));
  const remove = useMutation(deleteRelayMutation(api));
  const resetAll = () => {
    membership.reset();
    remove.reset();
  };
  const pendingId = membership.isPending ? membership.variables.id : remove.isPending ? remove.variables : undefined;
  const failed = membership.isError ? membership.error : remove.isError ? remove.error : undefined;
  return {
    setJoined: (relay, join) => {
      resetAll();
      membership.mutate(
        { id: relay.id, join },
        {
          onSuccess: (saved) => {
            void refreshRelays(queryClient).then(() => {
              report(
                t(join ? "client:admin.relays.followed" : "client:admin.relays.unfollowed", { name: relayName(saved) }),
              );
            });
          },
        },
      );
    },
    remove: async (relay) => {
      resetAll();
      try {
        await remove.mutateAsync(relay.id);
      } catch {
        return false;
      }
      onDeleted(relay.id);
      await refreshRelays(queryClient);
      report(t("client:admin.relays.deleted", { name: relayName(relay) }));
      return true;
    },
    pendingId,
    error: failed === undefined ? undefined : refusal(failed).message,
    status,
  };
}
