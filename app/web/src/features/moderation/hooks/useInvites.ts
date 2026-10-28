// SPDX-License-Identifier: AGPL-3.0-only
// The invites (FR-001, manage_invites): creating a code that lets people sign up in every registration mode but
// `closed` (ADR-0108 §4), the list of codes with how they were used, copying a code's sign-up link, and revoking a code.
import { useToast } from "@my/ui";
import {
  useMutation,
  useMutationState,
  useQueryClient,
  useSuspenseInfiniteQuery,
  useSuspenseQuery,
} from "@tanstack/react-query";
import { type SyntheticEvent, useState } from "react";
import { useApi } from "../../../lib/auth/SessionProvider";
import { errorMessage } from "../../../lib/errors";
import { useT } from "../../../lib/i18n";
import {
  cacheRevokedInvite,
  createInviteMutation,
  type Invite,
  invitesQueryOptions,
  type NewInvite,
  refreshInvites,
  revokeInviteMutation,
} from "../api/invites";
import { instanceInfoQueryOptions } from "../api/reports";
import { type PagedPages, toPagedList } from "./pagedList";

/** An hour and a day in seconds, the units of how long an invite works. */
const hour = 3600;
const day = 86_400;

/** How long a new invite may work, in seconds, from the shortest; the form also offers "never expires". */
export const inviteExpiries = [hour / 2, hour, 6 * hour, 12 * hour, day, 7 * day, 30 * day] as const;

/** How many sign-ups a new invite may allow; the form also offers "no limit". */
export const inviteMaxUses = [1, 5, 10, 25, 50, 100] as const;

/** The invites, newest first. */
export function useInvites(): PagedPages<Invite> {
  const api = useApi();
  return toPagedList(useSuspenseInfiniteQuery(invitesQueryOptions(api)));
}

/** Whether the server lets people sign up with an invite now: its registration mode is not `closed` (an invite works in `open`, `approval`, and `invite`). */
export function useInvitesWork(): boolean {
  const api = useApi();
  return useSuspenseQuery(instanceInfoQueryOptions(api)).data.registrations.mode !== "closed";
}

/** The form of a new invite: how many sign-ups it allows (null: no limit) and how long it works (null: forever). */
type Draft = { maxUses: number | null; expiresIn: number | null };

/** The CLI's defaults (`my invite create`): one sign-up within a week. */
const initialDraft: Draft = { maxUses: 1, expiresIn: 7 * day };

export function useInviteForm() {
  const api = useApi();
  const t = useT();
  const queryClient = useQueryClient();
  const [draft, setDraft] = useState(initialDraft);
  const [created, setCreated] = useState<Invite | undefined>(undefined);
  const create = useMutation({
    ...createInviteMutation(api),
    onMutate: () => {
      setCreated(undefined);
    },
    onSuccess: async (invite) => {
      // The invite is at the top of the list once it is fetched again (the button stays pending until then).
      await refreshInvites(queryClient);
      setCreated(invite);
    },
  });
  return {
    draft,
    change: (change: Partial<Draft>) => {
      setDraft({ ...draft, ...change });
    },
    submit: (event: SyntheticEvent<HTMLFormElement>) => {
      event.preventDefault();
      if (create.isPending) return;
      const body: NewInvite = {
        ...(draft.maxUses === null ? {} : { max_uses: draft.maxUses }),
        ...(draft.expiresIn === null ? {} : { expires_in: draft.expiresIn }),
      };
      create.mutate(body);
    },
    isPending: create.isPending,
    /** Why creating failed, localized; the form keeps its choices. */
    error: create.isError ? errorMessage(create.error) : undefined,
    /** The last invite created, to show its code and link, until the next one. */
    created,
    /** Says what the last invite is, for the form's StatusMessage. */
    status: created === undefined ? undefined : t("client:admin.invites.created", { code: created.code }),
  };
}

/** Copies an invite's sign-up link to the clipboard and says whether that worked in a toast. */
export function useCopyInviteLink(): (invite: Invite) => void {
  const t = useT();
  const toast = useToast();
  return (invite) => {
    navigator.clipboard.writeText(invite.url).then(
      () => {
        // The toast is announced too.
        toast.show({ message: t("client:admin.invites.copied", { code: invite.code }), tone: "success" });
      },
      () => {
        toast.show({ message: t("client:admin.invites.copy_failed"), tone: "danger" });
      },
    );
  };
}

export type RevokeInvite = {
  /** Revokes the invite; `onRevoked` says so, and a failure is shown in a toast. */
  revoke: (invite: Invite) => void;
  /** The IDs of the invites being revoked. */
  pendingIds: readonly string[];
};

/**
 * Revoking invites. `onRevoked(invite)` is called with the revoked invite just before its row shows it revoked
 * (to keep focus once its buttons are gone). Several invites may be revoked at once, so everything after a
 * success is in the mutation's own callbacks.
 */
export function useRevokeInvite(onRevoked: (invite: Invite) => void): RevokeInvite {
  const api = useApi();
  const t = useT();
  const queryClient = useQueryClient();
  const toast = useToast();
  const options = revokeInviteMutation(api);
  const mutation = useMutation({
    ...options,
    onSuccess: (revoked) => {
      onRevoked(revoked);
      cacheRevokedInvite(queryClient, revoked);
    },
    // The invite still works, and its row and focus stay (web-client/README.md E2).
    onError: (error) => {
      toast.show({ message: t("client:admin.invites.revoke_failed", { reason: errorMessage(error) }), tone: "danger" });
    },
  });
  const pendingIds = useMutationState({
    filters: { mutationKey: options.mutationKey, status: "pending" },
    select: (pending) => pending.state.variables as string,
  });
  return {
    revoke: (invite) => {
      mutation.mutate(invite.id);
    },
    pendingIds,
  };
}
