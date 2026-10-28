// SPDX-License-Identifier: AGPL-3.0-only
// The form of a moderation action on an account (FR-703), as a report's page offers it on the report's target:
// the kinds the account can take (a remote account takes three), the category of the reason, the public reason
// and the private note, how long it lasts, whether the account is told, whether a ban also blocks sign-ups with
// the email address of the account's user (FR-719), and the posts of an action on posts. An
// action that cannot be undone (a ban, deleting posts) asks first in an alert dialog (NFR-MOD-7). With a report,
// the action resolves it as actioned (FR-710).
import { type SyntheticEvent, useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useApi } from "../../../lib/auth/SessionProvider";
import { errorMessage } from "../../../lib/errors";
import { useT } from "../../../lib/i18n";
import { cacheIssuedAccountAction, refreshActedReports } from "../api/accounts";
import { refreshSignupBlockLists } from "../api/signupBlocks";
import {
  type AccountAction,
  type AccountActionCategory,
  type AccountActionKind,
  createAccountActionMutation,
  type NewAccountAction,
} from "../api/reports";

/** The kinds of action, from the lightest to the heaviest, in the order of the form. */
const accountActionKinds = [
  "warn",
  "mark_statuses_sensitive",
  "delete_statuses",
  "force_sensitive",
  "limit",
  "restrict_posting",
  "restrict_interaction",
  "disable_login",
  "suspend",
  "ban",
] as const satisfies readonly AccountActionKind[];

/** The kinds a remote account takes; the others act on what only this server keeps (sign-in, posting). */
const remoteKinds: readonly AccountActionKind[] = ["force_sensitive", "limit", "suspend"];

/** The kinds that act on some of the account's posts, named by `status_ids`. */
const postKinds: readonly AccountActionKind[] = ["mark_statuses_sensitive", "delete_statuses"];

/** The kinds that cannot be undone, which ask first (NFR-MOD-7): a ban deletes the account, the posts go for good. */
const irreversibleKinds: readonly AccountActionKind[] = ["ban", "delete_statuses"];

/** Where a kind stands on the scale from the lightest to the heaviest (its step of `max`), and whether it can be undone. */
export function severityOf(kind: AccountActionKind): { level: number; max: number; irreversible: boolean } {
  return {
    level: accountActionKinds.indexOf(kind) + 1,
    max: accountActionKinds.length,
    irreversible: irreversibleKinds.includes(kind),
  };
}

/**
 * Whether a kind takes how long it lasts. A ban and the actions on posts take none (the API refuses
 * `expires_in` on them); a warning is a notice only, so it lasts nothing either.
 */
export function takesDuration(kind: AccountActionKind | null): boolean {
  return kind !== null && kind !== "ban" && kind !== "warn" && !postKinds.includes(kind);
}

/** Whether a kind acts on posts that the form chooses. */
export function isPostKind(kind: AccountActionKind | null): boolean {
  return kind !== null && postKinds.includes(kind);
}

/**
 * Whether an action of `kind` on `target` may also block sign-ups with the email address of the account's user:
 * only a ban of a local account whose user has one (the API refuses `block_email` on anything else).
 */
export function takesEmailBlock(kind: AccountActionKind | null, target: ActionTarget): boolean {
  return kind === "ban" && !target.remote && target.hasEmail !== false;
}

/** The account the form acts on, and what it starts from. */
export type ActionTarget = {
  accountId: string;
  /** Whether the account belongs to another server: it takes three kinds and is never told. */
  remote: boolean;
  /** The report that the action follows from, which it resolves. */
  reportId?: string | undefined;
  /**
   * The posts that an action on posts may choose, all chosen until the moderator leaves some out; none offers no
   * action on posts. They may grow while the posts load.
   */
  statusIds: readonly string[];
  /** The category of the reason at first: the report's own. */
  category: AccountActionCategory;
  /**
   * Whether the account's user has an email address that a ban can block sign-ups with (FR-719); false offers no
   * such block, and unknown offers it on every local account (the server refuses it when there is none).
   */
  hasEmail?: boolean | undefined;
};

/** The form, filled in by the moderator. */
type Draft = {
  kind: AccountActionKind | null;
  category: AccountActionCategory;
  publicReason: string;
  privateNote: string;
  /** The seconds the action lasts, or null until it is lifted. */
  expiresIn: number | null;
  notify: boolean;
  /** Whether a ban also blocks sign-ups with the email address of the account's user (FR-719). */
  blockEmail: boolean;
  /** The posts of the target that the moderator left out of an action on posts. */
  leftOut: readonly string[];
};

function emptyDraft(target: ActionTarget): Draft {
  return {
    kind: null,
    category: target.category,
    publicReason: "",
    privateNote: "",
    expiresIn: null,
    notify: true,
    blockEmail: false,
    leftOut: [],
  };
}

/** The request of a draft whose kind is chosen; empty texts and what the kind does not take are left out. */
function requestOf(
  draft: Draft,
  kind: AccountActionKind,
  target: ActionTarget,
  statusIds: readonly string[],
): NewAccountAction {
  return {
    kind,
    reason_category: draft.category,
    ...(draft.publicReason === "" ? {} : { public_reason: draft.publicReason }),
    ...(draft.privateNote === "" ? {} : { private_note: draft.privateNote }),
    ...(target.reportId === undefined ? {} : { report_id: target.reportId }),
    // A remote account is never told; the server ignores the flag for it.
    notify_target: !target.remote && draft.notify,
    ...(takesDuration(kind) && draft.expiresIn !== null ? { expires_in: draft.expiresIn } : {}),
    ...(isPostKind(kind) ? { status_ids: [...statusIds] } : {}),
    ...(takesEmailBlock(kind, target) && draft.blockEmail ? { block_email: true } : {}),
  };
}

export type AccountActionForm = {
  draft: Draft;
  change: (change: Partial<Omit<Draft, "leftOut">>) => void;
  /** The posts chosen for an action on posts, in the target's order. */
  chosenStatusIds: readonly string[];
  /** Chooses a post for an action on posts, or leaves it out. */
  choosePost: (id: string, isChosen: boolean) => void;
  /** The kinds the account takes, in the order of the form. */
  kinds: readonly AccountActionKind[];
  /** Takes the action once the browser has checked the fields; one that cannot be undone asks first. */
  submit: (event: SyntheticEvent<HTMLFormElement>) => void;
  isPending: boolean;
  /** Whether an action on posts has no post chosen: it is not sent, and the posts say why (role="alert"). */
  isPostMissing: boolean;
  /** The confirmation of an action that cannot be undone: its kind, or null while it is closed. */
  confirmation: { kind: AccountActionKind; confirm: () => void; cancel: () => void } | null;
  /** Why the action failed, localized: in the confirmation while it is open, else under the form. */
  error: string | undefined;
};

/**
 * The form of a moderation action on `target`. `onIssued(action)` is called once the server has taken it and the
 * account, its actions, the searches and the audit log (and, for an action that resolves a report, the lists of
 * reports, and for an action that changes what everyone sees of the account, its profile and the lists of posts)
 * are fetched again, after the form is emptied; the reports, the posts acted on and, for a ban that blocks the
 * email address, the lists of sign-up blocks are fetched again after it.
 */
export function useAccountActionForm(
  target: ActionTarget,
  onIssued: (action: AccountAction) => void,
): AccountActionForm {
  const api = useApi();
  const t = useT();
  const queryClient = useQueryClient();
  const [draft, setDraft] = useState(() => emptyDraft(target));
  const [confirming, setConfirming] = useState(false);
  const [isPostMissing, setIsPostMissing] = useState(false);
  const kinds = accountActionKinds.filter(
    (kind) =>
      (target.remote ? remoteKinds.includes(kind) : true) && (target.statusIds.length > 0 || !postKinds.includes(kind)),
  );
  const create = useMutation({
    ...createAccountActionMutation(api),
    onSuccess: async (action, { accountId, body }) => {
      // The action is done once what it changes is fetched again (the form stays pending until then), so that going
      // back to a search, the queue, the log or a timeline shows it as it is now (cacheIssuedAccountAction).
      await cacheIssuedAccountAction(queryClient, api, accountId, action, body.report_id);
      setConfirming(false);
      setDraft(emptyDraft(target));
      // Before the report's page reads the report again: the page moves focus once it shows the report resolved
      // (useFocusAfterChange), which must come after this call.
      onIssued(action);
      refreshActedReports(queryClient, action);
      // A ban that blocks the email address adds a sign-up block, which every cached list of them misses.
      if (body.block_email === true) void refreshSignupBlockLists(queryClient);
    },
  });
  const chosenStatusIds = target.statusIds.filter((id) => !draft.leftOut.includes(id));
  const run = () => {
    if (draft.kind !== null)
      create.mutate({ accountId: target.accountId, body: requestOf(draft, draft.kind, target, chosenStatusIds) });
  };
  return {
    draft,
    change: (change) => {
      setDraft({ ...draft, ...change });
      if (change.kind !== undefined) setIsPostMissing(false);
    },
    chosenStatusIds,
    choosePost: (id, isChosen) => {
      setDraft({ ...draft, leftOut: isChosen ? draft.leftOut.filter((item) => item !== id) : [...draft.leftOut, id] });
      setIsPostMissing(false);
    },
    kinds,
    submit: (event) => {
      event.preventDefault();
      if (create.isPending || draft.kind === null) return;
      create.reset();
      if (isPostKind(draft.kind) && chosenStatusIds.length === 0) {
        setIsPostMissing(true);
        return;
      }
      if (irreversibleKinds.includes(draft.kind)) setConfirming(true);
      else run();
    },
    isPending: create.isPending,
    isPostMissing,
    confirmation:
      confirming && draft.kind !== null
        ? {
            kind: draft.kind,
            confirm: run,
            cancel: () => {
              setConfirming(false);
              create.reset();
            },
          }
        : null,
    error: create.isError
      ? t("client:admin.account_actions.issue_failed", { reason: errorMessage(create.error) })
      : undefined,
  };
}
