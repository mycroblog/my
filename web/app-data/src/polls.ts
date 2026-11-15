// SPDX-License-Identifier: AGPL-3.0-only
// The reader's vote in the poll of a status (FR-112, ADR-0028 §7, §18), written through the shadow. While the vote is
// in flight the poll shows as it was (the post shows the vote in flight itself); once the server took it, every copy
// of the status shows the poll the server answered, and a fetch that raced the vote does not undo it. The counts are
// not guessed in flight: a poll that hides them until it ends, or one of another server, which counts the vote only
// once that server reports it again, would show a count the server never gives. The write is a plain request (no
// outbox yet, #2892): a failure rejects with mykit's ProblemError or NetworkError, the server's refusal (a poll that
// ended, or that an edit replaced) among them.
import type { AppData } from "./app-data";
import { useAppData } from "./provider";
import { changeStatus } from "./shadow";

/** A vote: the poll the reader saw, and the indexes of the options they chose. */
export type PollVote = { readonly poll: string; readonly choices: readonly number[] };

/**
 * Votes in the poll of the status with the ID given: once the server took it, every copy shows the poll as the
 * server answered; a failure rejects, and the copies show what they showed.
 */
export async function voteInPoll(data: AppData, id: string, vote: PollVote): Promise<void> {
  await changeStatus(data, id, {
    key: "poll",
    change: (s) => s,
    send: async () =>
      (
        await data.api.POST("/statuses/{id}/poll/votes", {
          params: { path: { id } },
          body: { poll_id: vote.poll, choices: [...vote.choices] },
        })
      ).data,
    answered: (answer) => {
      const poll = answer?.id === id ? answer.poll : undefined;
      return poll === undefined ? undefined : (s) => ({ ...s, poll });
    },
  });
}

/** voteInPoll for the session of the nearest AppDataProvider. */
export function usePollVote(): (id: string, vote: PollVote) => Promise<void> {
  const data = useAppData();
  return (id, vote) => voteInPoll(data, id, vote);
}
