// SPDX-License-Identifier: AGPL-3.0-only
// The viewer's actions on a post (FR-106 to FR-109, FR-113, FR-116): boost, favourite, bookmark and the emoji
// reaction (which shares the Like slot with the favourite, useLikeSlot) are turned on and off optimistically in every cached copy of the post and restored when the server refuses
// (web-client/README.md S8); delete waits for the server, then drops the post from every list, and a pin (FR-403)
// waits for it, then marks the post and reads the viewer's pinned posts again. Each result is
// announced (Y7) and a failure is also shown as a toast, because the restored button alone tells a
// sighted user nothing (E2).
import { useNotice, useToast } from "@my/ui";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useApi } from "../../../lib/auth/SessionProvider";
import { errorMessage } from "../../../lib/errors";
import { useFederation } from "../../../lib/federation";
import { isRemoteAcct } from "../../../lib/handle";
import { type Translate, useT } from "../../../lib/i18n";
import { moderationAccountPath } from "../../../lib/paths";
import {
  type InteractionKind,
  type InteractionPolicy,
  type InteractionRule,
  useQuote,
  useReply,
  withChange,
} from "../../compose";
import { useViewer } from "../../session";
import { hasRelation, relationCount } from "../api/cache";
import { useLikeSlot } from "./useLikeSlot";
import {
  cacheDeletedStatus,
  cachePinAnswer,
  cacheInteractionPolicyAnswer,
  cacheRevokedQuote,
  cacheRelationAnswer,
  cacheRelationPending,
  deleteStatus,
  type ReactionEmoji,
  type Relation,
  restoreRelation,
  revokeQuote,
  setPin,
  setInteractionPolicy,
  setRelation,
  type Status,
  statusQuotesPath,
  statusShareUrl,
} from "../api/status";

/** One toggle of the action bar. */
export type RelationToggle = {
  isSelected: boolean;
  /** The public count, or null for bookmarks. */
  count: number | null;
  /**
   * Whether the server would refuse it: a boost that the post's policy or visibility refuses the viewer
   * (`Status.permissions`). A boost the viewer has can always be taken back.
   */
  isDisabled: boolean;
  /**
   * For the boost, what became of the viewer's boost that waits for the author's approval (`Status.boost_approval`,
   * ADR-0155 §7): `pending` until they answer, `rejected` once they refused it; null for none.
   */
  approval?: "pending" | "rejected" | null;
  onChange: (on: boolean) => void;
};

export type StatusActions = {
  /**
   * Opens the form as a reply to the post; null when its policy refuses the viewer's reply (`Status.permissions`,
   * ADR-0155 §10), which hides the button.
   */
  reply: (() => void) | null;
  boost: RelationToggle;
  favourite: RelationToggle;
  bookmark: RelationToggle;
  /**
   * Reacts to the post with `emoji`, in place of the viewer's favourite or other reaction (ADR-0051 §6); reacting
   * with the emoji the viewer reacted with takes it back.
   */
  react: (emoji: ReactionEmoji) => void;
  /** Copies the post's address to the clipboard. */
  copyLink: () => void;
  /**
   * Deletes the post, for the viewer's own posts; null otherwise. Resolves to whether the post is gone; a
   * failure is already shown.
   */
  delete: (() => Promise<boolean>) | null;
  /** Whether the viewer can edit the post: their own posts without a poll only (FR-109, FR-112: its votes belong to the poll as posted). */
  canEdit: boolean;
  /** Whether the post is being deleted. */
  isDeleting: boolean;
  /** Whether the viewer can report the post's author: anyone's posts but the viewer's own (FR-701). */
  canReport: boolean;
  /**
   * Pins the post to the viewer's profile or unpins it (FR-403), for the viewer's own posts that are not
   * direct (the server's rule); null otherwise. The result or the failure (the limit of pins, say) is shown.
   */
  pin: { isPinned: boolean; toggle: () => void } | null;
  /**
   * Opens the form quoting the post (FR-114). `isDisabled` while the viewer may not quote it (`Status.permissions`:
   * its policy, or a followers-only or direct post of someone else). Null for a post of another server while the
   * instance sends nothing out (its federation mode is `off` or `inbound`, ADR-0006): the quote request could not
   * reach it, and the server refuses the quote.
   */
  quote: { onPress: () => void; isDisabled: boolean } | null;
  /**
   * Whether the reply, boost, favourite and reaction stay on this server: the post is of another server and the
   * instance sends nothing out (`off` or `inbound`, ADR-0006). They are still recorded here; the bar says so.
   */
  staysLocal: boolean;
  /**
   * Who may reply to, boost, and quote the viewer's own post and its change (ADR-0155 §10), with the address of the
   * posts that quote it; null for others' posts.
   */
  interactionPolicy: {
    value: InteractionPolicy;
    set: (kind: InteractionKind, rule: InteractionRule) => void;
    quotesPath: string;
  } | null;
  /**
   * Revokes the approval of this post's quote of the viewer's post (ADR-0069 §3): only for an approved quote of a
   * post of the viewer; null otherwise. The result or the failure is shown.
   */
  revokeQuote: (() => void) | null;
  /**
   * The moderation page of the post's author, and the same page starting from this post, for a viewer with the
   * permission that page asks for (manage_accounts); null otherwise, and on the viewer's own posts.
   */
  moderation: { accountHref: string; postHref: string } | null;
};

/** What is announced when the viewer turns `relation` on or off. */
function relationMessage(t: Translate, relation: Relation, on: boolean): string {
  switch (relation) {
    case "boost":
      return on ? t("client:status.boost.on") : t("client:status.boost.off");
    case "favourite":
      return on ? t("client:status.favourite.on") : t("client:status.favourite.off");
    case "bookmark":
      return on ? t("client:status.bookmark.on") : t("client:status.bookmark.off");
  }
}

/**
 * Whether the server lets the viewer boost `post` (the client API's 422 otherwise): public, local and unlisted posts
 * by anyone, a followers-only post by its author only, a direct post by no one, and only while its policy admits the
 * viewer (`Status.permissions`; a boost that waits for the author's approval counts). It only decides whether to offer the button; the server decides the
 * boost.
 */
function canOfferBoost(post: Status, isAuthor: boolean): boolean {
  const visible = post.visibility === "private" ? isAuthor : post.visibility !== "direct";
  return visible && post.permissions?.announce !== "denied";
}

/** The mutation of one relation of post `post`, serialized per post and relation so that the last press wins. */
function useRelation(post: Status, relation: Relation, viewerId: string): RelationToggle {
  const api = useApi();
  const t = useT();
  const queryClient = useQueryClient();
  const notice = useNotice();
  const toast = useToast();
  const mutationKey = ["status", post.id, relation] as const;
  const { mutate } = useMutation({
    mutationKey,
    // One request at a time per post and relation, in the order pressed (TanStack Query, "Mutation Scopes").
    scope: { id: `status:${post.id}:${relation}` },
    mutationFn: ({ on }: { on: boolean }) => setRelation(api, relation, post.id, on),
    onMutate: ({ on }) => cacheRelationPending(api, queryClient, post.id, relation, on),
    onSuccess: (answer, { on }) => {
      // The last press is the only one still running.
      const last = queryClient.isMutating({ mutationKey, exact: true }) <= 1;
      cacheRelationAnswer(queryClient, post.id, relation, answer, last, viewerId);
      // A boost of a post whose author approves boosts waits for them (ADR-0155 §7): the answer says so.
      const waits = relation === "boost" && on && (answer.boost ?? answer).boost_approval === "pending";
      notice.inPlace(waits ? t("client:status.boost.pending") : relationMessage(t, relation, on));
    },
    onError: (error, { on }) => {
      restoreRelation(queryClient, post.id, relation, on);
      toast.show({ message: t("client:status.action.failed", { reason: errorMessage(error) }), tone: "danger" });
    },
  });
  return {
    isSelected: hasRelation(post, relation),
    count: relationCount(post, relation),
    isDisabled: false,
    onChange: (on) => {
      mutate({ on });
    },
  };
}

/**
 * The actions on `status`, or null while nobody is signed in. For a boost, the actions are on the boosted
 * post. Read `status` from the query cache on every render; the hook never copies it (S2).
 */
export function useStatusActions(status: Status): StatusActions | null {
  const post = status.boost ?? status;
  const api = useApi();
  const t = useT();
  const queryClient = useQueryClient();
  const toast = useToast();
  const viewer = useViewer();
  const reply = useReply();
  const quote = useQuote();
  const { reachesRemote } = useFederation();
  const viewerId = viewer.status === "signed-in" ? viewer.account.id : "";
  const boost = useRelation(post, "boost", viewerId);
  const likeSlot = useLikeSlot(post);
  const bookmark = useRelation(post, "bookmark", viewerId);
  const remove = useMutation({
    mutationFn: () => deleteStatus(api, post.id),
    onSuccess: () => {
      cacheDeletedStatus(api, queryClient, post);
      toast.show({ message: t("client:status.delete.done"), tone: "success" });
    },
    onError: (error) => {
      toast.show({ message: t("client:status.delete.failed", { reason: errorMessage(error) }), tone: "danger" });
    },
  });

  const pin = useMutation({
    mutationFn: ({ on }: { on: boolean }) => setPin(api, post.id, on),
    // One request at a time per post, in the order chosen.
    scope: { id: `status:${post.id}:pin` },
    onSuccess: async (answer, { on }) => {
      await cachePinAnswer(api, queryClient, post.id, answer, viewerId);
      // The toast is announced too.
      toast.show({ message: on ? t("client:status.pin.done") : t("client:status.unpin.done"), tone: "success" });
    },
    onError: (error, { on }) => {
      const reason = errorMessage(error);
      toast.show({
        message: on ? t("client:status.pin.failed", { reason }) : t("client:status.unpin.failed", { reason }),
        tone: "danger",
      });
    },
  });

  const policy = useMutation({
    mutationFn: ({ kind, rule }: { kind: InteractionKind; rule: InteractionRule }) =>
      setInteractionPolicy(api, post.id, { [kind]: rule }),
    // One request at a time per post, in the order chosen.
    scope: { id: `status:${post.id}:interaction_policy` },
    onSuccess: (answer) => {
      cacheInteractionPolicyAnswer(queryClient, answer);
      toast.show({ message: t("client:status.interaction_policy.done"), tone: "success" });
    },
    onError: (error) => {
      toast.show({
        message: t("client:status.interaction_policy.failed", { reason: errorMessage(error) }),
        tone: "danger",
      });
    },
  });
  const revoke = useMutation({
    mutationFn: ({ quotedId }: { quotedId: string }) => revokeQuote(api, quotedId, post.id),
    onSuccess: async (_, { quotedId }) => {
      await cacheRevokedQuote(queryClient, quotedId, post.id);
      toast.show({ message: t("client:status.quote.revoke.done"), tone: "success" });
    },
    onError: (error) => {
      toast.show({ message: t("client:status.quote.revoke.failed", { reason: errorMessage(error) }), tone: "danger" });
    },
  });

  if (viewer.status !== "signed-in" || reply === null || quote === null) return null;
  const isAuthor = post.account.id === viewer.account.id;
  const isPinned = post.pinned === true;
  const quoted = post.quote?.quoted_status;
  const ownQuoted =
    quoted !== undefined && quoted.account.id === viewer.account.id && post.quote?.state === "accepted" && !isAuthor;
  const staysLocal = !reachesRemote && isRemoteAcct(post.account.acct);
  return {
    reply:
      post.permissions?.reply === "denied"
        ? null
        : () => {
            reply(post);
          },
    boost: {
      ...boost,
      isDisabled: !boost.isSelected && (!canOfferBoost(post, isAuthor) || post.boost_approval !== undefined),
      approval: post.boost_approval ?? null,
    },
    favourite: {
      isSelected: likeSlot.slot.kind === "favourite",
      count: post.favourites_count,
      isDisabled: false,
      onChange: (on) => {
        likeSlot.set(on ? { kind: "favourite" } : { kind: "none" });
      },
    },
    bookmark,
    react: (emoji) => {
      const { slot } = likeSlot;
      likeSlot.set(
        slot.kind === "reaction" && slot.emoji.name === emoji.name ? { kind: "none" } : { kind: "reaction", emoji },
      );
    },
    copyLink: () => {
      navigator.clipboard.writeText(statusShareUrl(post)).then(
        () => {
          // The toast is announced too.
          toast.show({ message: t("client:status.link.copied"), tone: "success" });
        },
        () => {
          toast.show({ message: t("client:status.link.failed"), tone: "danger" });
        },
      );
    },
    delete: isAuthor
      ? () =>
          remove.mutateAsync().then(
            () => true,
            () => false,
          )
      : null,
    canEdit: isAuthor && post.poll === undefined,
    isDeleting: remove.isPending,
    canReport: !isAuthor,
    pin:
      isAuthor && post.visibility !== "direct"
        ? {
            isPinned,
            toggle: () => {
              pin.mutate({ on: !isPinned });
            },
          }
        : null,
    quote: staysLocal
      ? null
      : {
          onPress: () => {
            quote(post);
          },
          isDisabled: post.permissions === undefined || post.permissions.quote === "denied",
        },
    staysLocal,
    interactionPolicy:
      isAuthor && post.interaction_policy !== undefined
        ? {
            value: policy.isPending
              ? withChange(post.interaction_policy, { [policy.variables.kind]: policy.variables.rule })
              : post.interaction_policy,
            set: (kind, rule) => {
              policy.mutate({ kind, rule });
            },
            quotesPath: statusQuotesPath(post),
          }
        : null,
    revokeQuote: ownQuoted
      ? () => {
          revoke.mutate({ quotedId: quoted.id });
        }
      : null,
    moderation:
      !isAuthor && viewer.account.permissions.includes("manage_accounts")
        ? {
            accountHref: moderationAccountPath(post.account.id),
            postHref: moderationAccountPath(post.account.id, post.id),
          }
        : null,
  };
}
