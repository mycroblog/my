// SPDX-License-Identifier: AGPL-3.0-only
// The viewer's relationship with an account, and the changes to it (FR-301 to FR-303).
// The relationship is server data read from the query cache (web-client/README.md S2); a change shows the
// pending action until the server answers (S8) and then stores the relationship the server returns.
import { useNotice, useToast } from "@my/ui";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useApi, useSessionStatus } from "../../../lib/auth/SessionProvider";
import { errorMessage } from "../../../lib/errors";
import { useFederation } from "../../../lib/federation";
import { isolate, isRemoteAcct, nameOf } from "../../../lib/handle";
import { type Translate, useT } from "../../../lib/i18n";
import { profilePath, signInPath } from "../../../lib/paths";
import { useViewer } from "../../session";
import type { components } from "@mycroblog/mykit";
import {
  changeRelationship,
  type Relationship,
  type RelationshipAction,
  relationshipQueryOptions,
  storeRelationship,
} from "../api/relationship";

type Account = components["schemas"]["Account"];

/** Where the viewer's follow of the account stands; a block replaces the follow, which it ends. */
type FollowState = "none" | "requested" | "following" | "blocking";

/**
 * Whether the viewer can turn on the notifications of the account's new posts (ADR-0086 §14): `available` (an account
 * of this server, or one of another server that the viewer follows), `follow-first` (an account of another server that
 * the viewer does not follow yet: the button is shown turned off, with the reason), or `hidden` (a blocked account, or
 * one of another server that the instance cannot follow now).
 */
type SubscribeState = "available" | "follow-first" | "hidden";

export type RelationshipState =
  /** Nobody is signed in; `signInHref` signs in and comes back to the profile. */
  | { kind: "signed-out"; signInHref: string }
  /** The session is resuming, or the viewer or the relationship is loading. */
  | { kind: "loading" }
  /** The profile is the viewer's own. */
  | { kind: "self" }
  /** The relationship failed to load; `message` is localized, and `retry` reads it again. */
  | { kind: "error"; message: string; retry: () => void }
  /** The server left the relationship out: the account is gone or blocks the viewer. */
  | { kind: "unavailable" }
  | {
      kind: "other";
      relationship: Relationship;
      follow: FollowState;
      /**
       * Whether to offer a new follow (`follow` is `none`): not of an account of another server while the instance
       * does not follow other servers (its federation mode is `off` or `outbound`, ADR-0006), which the server
       * refuses. A follow or a request that already exists can still be undone.
       */
      canFollow: boolean;
      /** Whether the notifications of the account's new posts can be turned on; `relationship.notify` is whether they are. */
      subscribe: SubscribeState;
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
    case "subscribe":
      return t("client:profile.done.subscribed", values);
    case "unsubscribe":
      return t("client:profile.done.unsubscribed", values);
  }
}

/** Whether the viewer can subscribe to the account's new posts: one of another server only while it is followed. */
function subscribeStateOf(follow: FollowState, remote: boolean, canFollow: boolean): SubscribeState {
  if (follow === "blocking") return "hidden";
  if (!remote || follow === "following") return "available";
  return canFollow || follow === "requested" ? "follow-first" : "hidden";
}

export type UseRelationshipOptions = {
  /**
   * Whether to read the relationship; true by default. A menu that shows it passes whether it is open, so that a list of
   * posts reads nothing until then. What the cache holds is still returned, and turning it back on reads a failed or stale answer again.
   */
  enabled?: boolean;
  /**
   * Where the result of a change shows: `in-place` (the default) for a button that switches next to the viewer, `toast`
   * for a menu, which closes and leaves the result no place of its own (web-client/16-look-states-and-wording.md G24).
   */
  result?: "in-place" | "toast";
};

/** Returns the viewer's relationship with `account` and the way to change it. */
export function useRelationship(
  account: Account,
  { enabled = true, result = "in-place" }: UseRelationshipOptions = {},
): RelationshipState {
  const api = useApi();
  const t = useT();
  const queryClient = useQueryClient();
  const notice = useNotice();
  const toast = useToast();
  const session = useSessionStatus();
  const viewer = useViewer();
  const { followsRemote } = useFederation();
  const isOther = viewer.status === "signed-in" && viewer.account.id !== account.id;
  const query = useQuery({ ...relationshipQueryOptions(api, account.id), enabled: isOther && enabled });
  const mutation = useMutation({
    mutationFn: (action: RelationshipAction) => changeRelationship(api, account.id, action),
    onSuccess: async (relationship, action) => {
      // The change is done once what it changes is fetched again (the action stays pending until then), so that
      // a page opened next shows it at once (web-client/README.md S1).
      if (viewer.status === "signed-in")
        await storeRelationship(queryClient, api, relationship, action, viewer.account.id);
      const message = doneMessage(t, action, relationship, nameOf(account));
      if (result === "toast") notice.toast({ message, tone: "success" });
      else notice.inPlace(message);
    },
    onError: (error) => {
      toast.show({ message: t("client:profile.change_failed", { reason: errorMessage(error) }), tone: "danger" });
    },
  });

  if (session === "signed-out") {
    return {
      kind: "signed-out",
      signInHref: signInPath(profilePath(account.acct)),
    };
  }
  if (viewer.status === "error") return { kind: "error", message: viewer.message, retry: viewer.retry };
  if (viewer.status !== "signed-in") return { kind: "loading" };
  if (!isOther) return { kind: "self" };
  if (query.isError) {
    return {
      kind: "error",
      message: errorMessage(query.error),
      retry: () => {
        void query.refetch();
      },
    };
  }
  if (query.data === undefined) return { kind: "loading" };
  // The server leaves out an account that blocks the viewer, whose profile is then not found at all.
  if (query.data === null) return { kind: "unavailable" };
  const follow = followStateOf(query.data);
  const remote = isRemoteAcct(account.acct);
  const canFollow = followsRemote || !remote;
  return {
    kind: "other",
    relationship: query.data,
    follow,
    canFollow,
    subscribe: subscribeStateOf(follow, remote, canFollow),
    pending: mutation.isPending ? mutation.variables : null,
    change: (action) => {
      if (!mutation.isPending) mutation.mutate(action);
    },
  };
}
