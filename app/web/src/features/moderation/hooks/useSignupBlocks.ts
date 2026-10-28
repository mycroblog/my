// SPDX-License-Identifier: AGPL-3.0-only
// The sign-up blocks (FR-719): the list under a filter, adding a block, and lifting one. The server keeps an
// email domain in IDNA A-labels; the UI shows it with the U-labels it stands for (lib/idn), like the domain
// actions.
import { useToast } from "@my/ui";
import { useMutation, useMutationState, useQueryClient, useSuspenseInfiniteQuery } from "@tanstack/react-query";
import { type SyntheticEvent, useState } from "react";
import { useApi } from "../../../lib/auth/SessionProvider";
import { errorMessage } from "../../../lib/errors";
import { type ClientMessageKey, type Translate, useT } from "../../../lib/i18n";
import { refreshAuditLog } from "../api/staff";
import {
  cacheLiftedSignupBlock,
  createSignupBlockMutation,
  liftSignupBlockMutation,
  type NewSignupBlock,
  refreshSignupBlockLists,
  type SignupBlock,
  type SignupBlockFilter,
  type SignupBlockKind,
  type SignupBlockSeverity,
  signupBlocksQueryOptions,
} from "../api/signupBlocks";
import { domainInText } from "./useDomainActions";
import { type PagedPages, toPagedList } from "./pagedList";

export const signupBlockKindLabels: Readonly<Record<SignupBlockKind, ClientMessageKey>> = {
  email_domain: "client:admin.signup_blocks.kind.email_domain",
  email_hash: "client:admin.signup_blocks.kind.email_hash",
  ip_range: "client:admin.signup_blocks.kind.ip_range",
};

export const signupBlockSeverityLabels: Readonly<Record<SignupBlockSeverity, ClientMessageKey>> = {
  reject: "client:admin.signup_blocks.severity.reject",
  require_approval: "client:admin.signup_blocks.severity.require_approval",
};

/**
 * What a block matches, for a sentence: a domain in both of its forms (domainInText), else the value as the
 * server keeps it (a hash, a CIDR prefix), isolated left to right from the text around it.
 */
export function signupBlockValueInText(t: Translate, block: Pick<SignupBlock, "kind" | "value">): string {
  // U+2066 LEFT-TO-RIGHT ISOLATE … U+2069 POP DIRECTIONAL ISOLATE.
  return block.kind === "email_domain" ? domainInText(t, block.value) : `⁦${block.value}⁩`;
}

/** The sign-up blocks under `filter`, newest first. */
export function useSignupBlocks(filter: SignupBlockFilter): PagedPages<SignupBlock> {
  const api = useApi();
  return toPagedList(useSuspenseInfiniteQuery(signupBlocksQueryOptions(api, filter)));
}

export type LiftSignupBlock = {
  /** Lifts the block; `onLifted` says so, and a failure is shown in a toast. */
  lift: (block: SignupBlock) => void;
  /** The IDs of the blocks being lifted. */
  pendingIds: readonly string[];
};

/**
 * Lifting sign-up blocks. `onLifted(block)` is called with the lifted block, once the audit log is fetched
 * again, just before the lists change: the lists of blocks in effect drop it (to keep focus,
 * lib/useFocusAfterRemoval), the others show it lifted.
 * Several blocks may be lifted at once, so everything after a success is in the mutation's own callbacks.
 */
export function useLiftSignupBlock(onLifted: (block: SignupBlock) => void): LiftSignupBlock {
  const api = useApi();
  const t = useT();
  const queryClient = useQueryClient();
  const toast = useToast();
  const options = liftSignupBlockMutation(api);
  const mutation = useMutation({
    ...options,
    onSuccess: async (lifted) => {
      // The audit log records the lift (refreshAuditLog); the lists change from the answer, which shows them right.
      await refreshAuditLog(queryClient);
      onLifted(lifted);
      cacheLiftedSignupBlock(queryClient, lifted);
    },
    // The block stays in effect, and its row and focus stay (web-client/README.md E2).
    onError: (error) => {
      toast.show({
        message: t("client:admin.signup_blocks.lift_failed", { reason: errorMessage(error) }),
        tone: "danger",
      });
    },
  });
  const pendingIds = useMutationState({
    filters: { mutationKey: options.mutationKey, status: "pending" },
    select: (pending) => pending.state.variables as string,
  });
  return {
    lift: (block) => {
      mutation.mutate(block.id);
    },
    pendingIds,
  };
}

/** The form of a new sign-up block, filled in by the moderator. */
type Draft = {
  kind: SignupBlockKind;
  value: string;
  severity: SignupBlockSeverity | null;
  comment: string;
  /** The seconds the block lasts, or null until it is lifted. */
  expiresIn: number | null;
};

function emptyDraft(kind: SignupBlockKind): Draft {
  return { kind, value: "", severity: null, comment: "", expiresIn: null };
}

/** The request of a draft whose severity is chosen; an empty comment is left out. */
function requestOf(draft: Draft, severity: SignupBlockSeverity): NewSignupBlock {
  return {
    kind: draft.kind,
    value: draft.value.trim(),
    severity,
    ...(draft.comment === "" ? {} : { comment: draft.comment }),
    ...(draft.expiresIn === null ? {} : { expires_in: draft.expiresIn }),
  };
}

/** What the form of a new sign-up block needs. */
export type SignupBlockForm = {
  draft: Draft;
  change: (change: Partial<Draft>) => void;
  /** Adds the block once the browser has checked the fields. */
  submit: (event: SyntheticEvent<HTMLFormElement>) => void;
  isPending: boolean;
  /** Why adding failed, localized, for under the form; the draft stays. */
  error: string | undefined;
  /** What the last block added does, for the form's StatusMessage; undefined before the first one. */
  status: string | undefined;
};

/**
 * The form of a new sign-up block. A success empties the form (keeping the kind, which the next block often
 * shares) and says what took effect once every cached list and the audit log are fetched again.
 */
export function useSignupBlockForm(): SignupBlockForm {
  const api = useApi();
  const t = useT();
  const queryClient = useQueryClient();
  const [draft, setDraft] = useState(() => emptyDraft("email_domain"));
  const [status, setStatus] = useState<string | undefined>(undefined);
  const create = useMutation({
    ...createSignupBlockMutation(api),
    onSuccess: async (block) => {
      // The block is in the lists of both tabs and in the audit log: it is added once they are fetched again
      // (the form stays pending until then), so that the other tab or the log shows it at once.
      await Promise.all([refreshSignupBlockLists(queryClient), refreshAuditLog(queryClient)]);
      setDraft(emptyDraft(draft.kind));
      setStatus(
        t("client:admin.signup_blocks.added", {
          severity: t(signupBlockSeverityLabels[block.severity]),
          kind: t(signupBlockKindLabels[block.kind]),
          value: signupBlockValueInText(t, block),
        }),
      );
    },
  });
  return {
    draft,
    change: (change) => {
      setDraft({ ...draft, ...change });
    },
    submit: (event) => {
      event.preventDefault();
      if (create.isPending || draft.severity === null) return;
      create.reset();
      create.mutate(requestOf(draft, draft.severity));
    },
    isPending: create.isPending,
    error: create.isError ? errorMessage(create.error) : undefined,
    status,
  };
}
