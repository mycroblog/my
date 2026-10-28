// SPDX-License-Identifier: AGPL-3.0-only
// The viewer's relationship with the account of a profile, and the changes to it (FR-301 to FR-303).
// The relationship is server data read from the query cache (web-client/README.md S2); a change shows the
// pending action until the server answers (S8) and then stores the relationship the server returns.
import { useAnnouncer, useToast } from "@my/ui";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useApi, useSessionStatus } from "../../../lib/auth/SessionProvider";
import { errorMessage } from "../../../lib/errors";
import { useFederation } from "../../../lib/federation";
import { isRemoteAcct } from "../../../lib/handle";
import { type Translate, useT } from "../../../lib/i18n";
import { profilePath } from "../../../lib/paths";
import { useViewer } from "../../session";
import {
  type Account,
  changeRelationship,
  type Relationship,
  type RelationshipAction,
  relationshipQueryOptions,
  storeRelationship,
} from "../api/accounts";
import { isolate, nameOf } from "./useProfile";

/** Where the viewer's follow of the account stands; a block replaces the follow, which it ends. */
type FollowState = "none" | "requested" | "following" | "blocking";

export type RelationshipState =
  /** Nobody is signed in; `signInHref` signs in and comes back to the profile. */
  | { kind: "signed-out"; signInHref: string }
  /** The session is resuming, or the viewer or the relationship is loading. */
  | { kind: "loading" }
  /** The profile is the viewer's own. */
  | { kind: "self" }
  /** The relationship failed to load; `message` is localized. */
  | { kind: "error"; message: string }
  | {
      kind: "other";
      relationship: Relationship;
      follow: FollowState;
      /**
       * Whether to offer a new follow (`follow` is `none`): not of an account of another server while the instance
       * does not follow other servers (its federation mode is `off` or `outbound`, ADR-0076), which the server
       * refuses. A follow or a request that already exists can still be undone.
       */
      canFollow: boolean;
      /** The change on its way to the server, or null. */
      pending: RelationshipAction | null;
      /** Sends a change. Its result is announced; a failure is shown in a toast. */
      change: (action: RelationshipAction) => void;
    };

function followStateOf(relationship: Relationship): FollowState {
  if (relationship.blocking) return "blocking";
  if (relationship.following) return "following";
  if (relationship.requested) return "requested";
  return "none";
}

/** What a screen reader hears once a change is done. The name is isolated from the sentence (Y10). */
function doneMessage(t: Translate, action: RelationshipAction, relationship: Relationship, name: string): string {
  const values = { name: isolate(name) };
  switch (action) {
    case "follow":
      return relationship.requested
        ? t("client:profile.done.requested", values)
        : t("client:profile.done.followed", values);
    case "unfollow":
      return t("client:profile.done.unfollowed", values);
    case "block":
      return t("client:profile.done.blocked", values);
    case "unblock":
      return t("client:profile.done.unblocked", values);
    case "mute":
      return t("client:profile.done.muted", values);
    case "unmute":
      return t("client:profile.done.unmuted", values);
  }
}

/** Returns the viewer's relationship with `account` and the way to change it. */
export function useRelationship(account: Account): RelationshipState {
  const api = useApi();
  const t = useT();
  const queryClient = useQueryClient();
  const announce = useAnnouncer();
  const toast = useToast();
  const session = useSessionStatus();
  const viewer = useViewer();
  const { followsRemote } = useFederation();
  const isOther = viewer.status === "signed-in" && viewer.account.id !== account.id;
  const query = useQuery({ ...relationshipQueryOptions(api, account.id), enabled: isOther });
  const mutation = useMutation({
    mutationFn: (action: RelationshipAction) => changeRelationship(api, account.id, action),
    onSuccess: async (relationship, action) => {
      // The change is done once what it changes is fetched again (the action stays pending until then), so that
      // a page opened next shows it at once (web-client/README.md S1).
      if (viewer.status === "signed-in")
        await storeRelationship(queryClient, api, relationship, action, viewer.account.id);
      announce(doneMessage(t, action, relationship, nameOf(account)));
    },
    onError: (error) => {
      toast.show({ message: t("client:profile.change_failed", { reason: errorMessage(error) }), tone: "danger" });
    },
  });

  if (session === "signed-out") {
    return {
      kind: "signed-out",
      signInHref: `/login?${new URLSearchParams({ return_to: profilePath(account.acct) }).toString()}`,
    };
  }
  if (viewer.status === "error") return { kind: "error", message: viewer.message };
  if (viewer.status !== "signed-in") return { kind: "loading" };
  if (!isOther) return { kind: "self" };
  if (query.isError) return { kind: "error", message: errorMessage(query.error) };
  // The server leaves out an account that blocks the viewer, whose profile is then not found at all.
  if (query.data === undefined || query.data === null) return { kind: "loading" };
  return {
    kind: "other",
    relationship: query.data,
    follow: followStateOf(query.data),
    canFollow: followsRemote || !isRemoteAcct(account.acct),
    pending: mutation.isPending ? mutation.variables : null,
    change: (action) => {
      if (!mutation.isPending) mutation.mutate(action);
    },
  };
}
