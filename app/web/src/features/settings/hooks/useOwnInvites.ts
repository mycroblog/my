// SPDX-License-Identifier: AGPL-3.0-only
// The user's own invites (FR-011, ADR-0108 §2, §5): whether the settings offer the page, the invites with what the user
// may do, the form of a new invite whose choices stay within the caps of the instance settings, copying an invite's
// sign-up link, and revoking an invite. What each change did is announced and kept in a StatusMessage; a failure stays
// next to its cause.
import { useToast } from "@my/ui";
import {
  useInfiniteQuery,
  useMutation,
  useMutationState,
  useQueryClient,
  useSuspenseInfiniteQuery,
} from "@tanstack/react-query";
import { type SyntheticEvent, useState } from "react";
import { useApi, useSessionStatus } from "../../../lib/auth/SessionProvider";
import { errorMessage } from "../../../lib/errors";
import { useT } from "../../../lib/i18n";
import { shellQueryRetry } from "../../../lib/query";
import { type PagedList, toPagedList } from "../../moderation";
import {
  cacheRevokedOwnInvite,
  createOwnInviteMutation,
  type NewOwnInvite,
  type OwnInvite,
  type OwnInviteLimits,
  ownInvitesQueryOptions,
  refreshOwnInvites,
  revokeOwnInviteMutation,
} from "../api/invites";

/** An hour and a day in seconds, the units of how long an invite works. */
const hour = 3600;
const day = 86_400;

/** How many sign-ups the form offers, from the fewest; only those within the cap are offered, with the cap itself. */
const maxUsesChoices = [1, 2, 3, 5, 10, 25, 50, 100, 250, 500, 1000] as const;

/** How long the form offers an invite to work, in seconds; only those within the cap are offered, with the cap itself. */
const expiryChoices = [
  hour,
  6 * hour,
  12 * hour,
  day,
  3 * day,
  7 * day,
  14 * day,
  30 * day,
  90 * day,
  180 * day,
  365 * day,
] as const;

/** `choices` up to `cap`, with `cap` and `preset` among them, from the smallest. */
function withinCap(choices: readonly number[], cap: number, preset: number): number[] {
  return [...new Set([...choices.filter((choice) => choice <= cap), cap, Math.min(preset, cap)])].toSorted(
    (a, b) => a - b,
  );
}

/**
 * Whether the settings list the page of the user's invites: the user may create invites now (ADR-0108 §5). False
 * while signed out, while it loads, and when it fails; the page itself still opens from its address.
 */
export function useOwnInvitesEntry(): boolean {
  const api = useApi();
  const signedIn = useSessionStatus() === "signed-in";
  const query = useInfiniteQuery({ ...ownInvitesQueryOptions(api), enabled: signedIn, retry: shellQueryRetry });
  return signedIn && query.data?.pages[0]?.can_create === true;
}

/** The user's invites, newest first, with whether they may create one, how many usable ones they hold, and the caps. */
export function useOwnInvites(): PagedList<OwnInvite> & {
  canCreate: boolean;
  usable: number;
  limits: OwnInviteLimits;
} {
  const api = useApi();
  const query = useSuspenseInfiniteQuery(ownInvitesQueryOptions(api));
  const first = query.data.pages[0];
  return {
    ...toPagedList(query),
    canCreate: first?.can_create ?? false,
    usable: first?.usable ?? 0,
    limits: first?.limits ?? {
      max_active: 0,
      max_uses: 1,
      max_expires_in: hour,
      default_max_uses: 1,
      default_expires_in: hour,
    },
  };
}

/** The choices of the form of a new invite; undefined is the server's default. */
type Draft = { maxUses: number | undefined; expiresIn: number | undefined };

/** The form of a new invite within `limits`: its choices start at the server's defaults. */
export function useOwnInviteForm(limits: OwnInviteLimits) {
  const api = useApi();
  const t = useT();
  const queryClient = useQueryClient();
  const [draft, setDraft] = useState<Draft>({ maxUses: undefined, expiresIn: undefined });
  const [created, setCreated] = useState<OwnInvite | undefined>(undefined);
  const create = useMutation({
    ...createOwnInviteMutation(api),
    onMutate: () => {
      setCreated(undefined);
    },
    onSuccess: async (invite) => {
      // The invite is at the top of the list once it is fetched again (the button stays pending until then).
      await refreshOwnInvites(queryClient);
      setCreated(invite);
    },
  });
  const maxUses = Math.min(draft.maxUses ?? limits.default_max_uses, limits.max_uses);
  const expiresIn = Math.min(draft.expiresIn ?? limits.default_expires_in, limits.max_expires_in);
  return {
    maxUses,
    expiresIn,
    /** The numbers of sign-ups offered, within the cap. */
    maxUsesChoices: withinCap(maxUsesChoices, limits.max_uses, maxUses),
    /** The lengths of time offered, in seconds, within the cap. */
    expiryChoices: withinCap(expiryChoices, limits.max_expires_in, expiresIn),
    change: (change: Partial<Draft>) => {
      setDraft({ ...draft, ...change });
    },
    submit: (event: SyntheticEvent<HTMLFormElement>) => {
      event.preventDefault();
      if (create.isPending) return;
      const body: NewOwnInvite = { max_uses: maxUses, expires_in: expiresIn };
      create.mutate(body);
    },
    isPending: create.isPending,
    /** Why creating failed, localized; the form keeps its choices. */
    error: create.isError ? errorMessage(create.error) : undefined,
    /** The last invite created, to show its link, until the next one. */
    created,
    /** Says what the last invite is, for the form's StatusMessage. */
    status: created === undefined ? undefined : t("client:settings.invites.created", { code: created.code }),
  };
}

/** Copies an invite's sign-up link to the clipboard and says whether that worked in a toast. */
export function useCopyOwnInviteLink(): (invite: OwnInvite) => void {
  const t = useT();
  const toast = useToast();
  return (invite) => {
    navigator.clipboard.writeText(invite.url).then(
      () => {
        // The toast is announced too.
        toast.show({ message: t("client:settings.invites.copied", { code: invite.code }), tone: "success" });
      },
      () => {
        toast.show({ message: t("client:settings.invites.copy_failed"), tone: "danger" });
      },
    );
  };
}

export type RevokeOwnInvite = {
  /** Revokes the invite; `onRevoked` says so, and a failure is shown in a toast. */
  revoke: (invite: OwnInvite) => void;
  /** The IDs of the invites being revoked. */
  pendingIds: readonly string[];
};

/**
 * Revoking the user's invites. `onRevoked(invite)` is called with the revoked invite just before its row shows it
 * revoked (to keep focus once its buttons are gone). Several invites may be revoked at once, so everything after a
 * success is in the mutation's own callbacks.
 */
export function useRevokeOwnInvite(onRevoked: (invite: OwnInvite) => void): RevokeOwnInvite {
  const api = useApi();
  const t = useT();
  const queryClient = useQueryClient();
  const toast = useToast();
  const options = revokeOwnInviteMutation(api);
  const mutation = useMutation({
    ...options,
    onSuccess: async (revoked) => {
      onRevoked(revoked);
      await cacheRevokedOwnInvite(queryClient, revoked);
    },
    // The invite still works, and its row and focus stay (web-client/README.md E2).
    onError: (error) => {
      toast.show({
        message: t("client:settings.invites.revoke_failed", { reason: errorMessage(error) }),
        tone: "danger",
      });
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
