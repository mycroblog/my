// SPDX-License-Identifier: AGPL-3.0-only
// The poll of a post (FR-112), as plain values for the component (web-client/README.md A4): the options to choose from
// while the viewer may vote, and the results once they voted, the poll has ended, or nobody is signed in. A vote
// waits for the server, then puts its answer into every copy of the post (web-client/README.md S1); a refused vote is
// shown as a toast and the post is read again, since its poll may have ended or changed (E2).
import { type CustomEmojiImage, useToast } from "@my/ui";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { useIntl } from "react-intl";
import { useApi } from "../../../lib/auth/SessionProvider";
import { errorMessage } from "../../../lib/errors";
import { hostOf } from "../../../lib/host";
import { isRemoteAcct } from "../../../lib/handle";
import { useT } from "../../../lib/i18n";
import { type DeadlineView, useDeadlineView } from "../../../lib/time";
import { useViewer } from "../../session";
import {
  cacheVoteAnswer,
  type Poll,
  refreshAfterRefusedVote,
  type Status,
  statusShareUrl,
  votePoll,
} from "../api/status";

/** One option of a poll, with its share of the votes. */
type PollOptionView = {
  /** The option's index in the poll, which a vote names. */
  index: number;
  title: string;
  /** The votes, or null while the poll hides them until it ends. */
  votes: number | null;
  /** The share of the votes, from 0 to 1, and as the locale writes a percentage; null while the votes are hidden. */
  ratio: number;
  share: string | null;
  /** Whether the viewer voted for it. */
  ownChoice: boolean;
};

/** The form that votes in a poll. */
type PollForm = {
  /** The indexes of the options chosen so far. */
  selected: readonly number[];
  /** Chooses the option at `index` alone (a single-choice poll). */
  choose: (index: number) => void;
  /** Adds or removes the option at `index` (a multiple-choice poll). */
  toggle: (index: number, on: boolean) => void;
  /** Sends the vote; it does nothing until an option is chosen. */
  submit: () => void;
  canSubmit: boolean;
  isSubmitting: boolean;
};

export type PollView = {
  multiple: boolean;
  options: PollOptionView[];
  /** The custom emoji of the options' titles. */
  emojis: readonly CustomEmojiImage[];
  /** The language of the post, for the options' titles. */
  lang: string | undefined;
  /** How many voted, in words ("12 votes", or "8 people" when the poll counts its voters). */
  total: string;
  /** When the poll ends, or null when it has no end. */
  ends: DeadlineView | null;
  /** Whether the poll hides the votes of each option until it ends (its author chose so). */
  hidden: boolean;
  /** Whether the poll has ended. */
  closed: boolean;
  /** For a poll of another server, its host: the counts are those it last reported. */
  remoteHost: string | null;
  /** The form while the viewer may vote; null shows the results. */
  form: PollForm | null;
};

/**
 * Returns the poll of `status` (the boosted post for a boost), or null without one. `canVote` is false where the
 * card offers no actions (a moderator's page); the poll then shows its results only.
 */
export function usePoll(status: Status, canVote: boolean): PollView | null {
  const post = status.boost ?? status;
  const poll = post.poll;
  const api = useApi();
  const t = useT();
  const intl = useIntl();
  const queryClient = useQueryClient();
  const toast = useToast();
  const viewer = useViewer();
  const ends = useDeadlineView(poll?.expires_at);
  // The options chosen so far, for this poll only: an edit that replaces the poll starts over.
  const [choice, setChoice] = useState<{ pollId: string; selected: readonly number[] }>({ pollId: "", selected: [] });
  const vote = useMutation({
    mutationFn: ({ pollId, choices }: { pollId: string; choices: readonly number[] }) =>
      votePoll(api, post.id, pollId, choices),
    // One vote at a time per post.
    scope: { id: `status:${post.id}:poll` },
    onSuccess: (answer) => {
      cacheVoteAnswer(queryClient, post.id, answer);
      // The toast is announced too.
      toast.show({ message: t("client:status.poll.voted"), tone: "success" });
    },
    onError: async (error) => {
      toast.show({ message: t("client:status.poll.vote_failed", { reason: errorMessage(error) }), tone: "danger" });
      await refreshAfterRefusedVote(api, queryClient, post.id);
    },
  });
  if (poll === undefined) return null;

  const closed = poll.expired || ends?.passed === true;
  const own = new Set(poll.own_votes ?? []);
  // A multiple-choice poll shares out its voters when it knows them, as the other implementations show it.
  const base = poll.multiple && poll.voters_count !== undefined ? poll.voters_count : poll.votes_count;
  const options = poll.options.map((option, index): PollOptionView => {
    const votes = option.votes_count ?? null;
    const ratio = votes !== null && base > 0 ? Math.min(1, votes / base) : 0;
    return {
      index,
      title: option.title,
      votes,
      ratio,
      share: votes === null ? null : intl.formatNumber(ratio, { style: "percent", maximumFractionDigits: 0 }),
      ownChoice: own.has(index),
    };
  });
  const selected = choice.pollId === poll.id ? choice.selected : [];
  const mayVote = canVote && viewer.status === "signed-in" && poll.voted !== true && !closed;
  return {
    multiple: poll.multiple,
    options,
    emojis: post.emojis,
    lang: post.language === "und" ? undefined : post.language,
    total:
      poll.voters_count === undefined
        ? t("client:status.poll.votes", { count: poll.votes_count })
        : t("client:status.poll.voters", { count: poll.voters_count }),
    ends,
    // The server leaves out the votes of the options while the poll hides them (PollOption.votes_count).
    hidden: options.some((option) => option.votes === null),
    closed,
    remoteHost: isRemoteAcct(post.account.acct) ? hostOf(statusShareUrl(post), post.account.domain) : null,
    form: mayVote ? pollForm(poll, selected, setChoice, vote.mutate, vote.isPending) : null,
  };
}

/** The form of `poll` with the options `selected`, which `setChoice` changes and `send` votes for. */
function pollForm(
  poll: Poll,
  selected: readonly number[],
  setChoice: (choice: { pollId: string; selected: readonly number[] }) => void,
  send: (vote: { pollId: string; choices: readonly number[] }) => void,
  isSubmitting: boolean,
): PollForm {
  return {
    selected,
    choose: (index) => {
      setChoice({ pollId: poll.id, selected: [index] });
    },
    toggle: (index, on) => {
      const rest = selected.filter((i) => i !== index);
      setChoice({ pollId: poll.id, selected: on ? [...rest, index].toSorted((a, b) => a - b) : rest });
    },
    submit: () => {
      if (selected.length > 0) send({ pollId: poll.id, choices: selected });
    },
    canSubmit: selected.length > 0,
    isSubmitting,
  };
}
