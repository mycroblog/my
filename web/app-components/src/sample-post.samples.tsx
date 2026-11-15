// SPDX-License-Identifier: AGPL-3.0-only

// One sample post of the stories, outside any timeline: it keeps its status and answers its actions as the server
// would, so that it behaves while nothing is fetched.

import { useState } from "react";
import { Post, type PostAction } from "./post";
import { statusOf, translationOf, type TimelinePost } from "./post.samples";
import type { PostStatus, PostTranslation } from "./status";

/** The status a toggle leaves: on or off, its count moved with it. */
const toggled = (on: boolean | undefined, count: number) => ({ on: !(on ?? false), count: count + (on ? -1 : 1) });

/** The status after the reader's reaction with `name`: taken back if it was theirs, else moved there. */
function reacted(status: PostStatus, name: string): PostStatus {
  const was = status.reactions.find((r) => r.me === true)?.name;
  const reactions = status.reactions
    .map((r) => {
      if (r.name === name) return { ...r, me: was !== name, count: r.count + (was === name ? -1 : 1) };
      if (r.name === was) return { ...r, me: false, count: r.count - 1 };
      return r;
    })
    .filter((r) => r.count > 0);
  return { ...status, reactions };
}

/** The status after the reader's vote for `choices` in its poll. */
function voted(status: PostStatus, choices: readonly number[]): PostStatus {
  const poll = status.poll;
  if (poll === undefined) return status;
  return {
    ...status,
    poll: {
      ...poll,
      voted: true,
      own_votes: choices,
      votes_count: poll.votes_count + choices.length,
      voters_count: (poll.voters_count ?? 0) + 1,
      options: poll.options.map((o, i) =>
        o.votes_count === undefined || !choices.includes(i) ? o : { ...o, votes_count: o.votes_count + 1 },
      ),
    },
  };
}

/** How long the sample server takes to answer a vote, so that the vote in flight shows. */
const voteDelay = 1200;

/** What the server would answer to an action: the status a favourite, a boost, a bookmark or a reaction leaves. */
function answered<S extends PostStatus>(status: S, action: PostAction): S {
  switch (action.type) {
    case "favourite": {
      const t = toggled(status.favourited, status.favourites_count);
      return { ...status, favourited: t.on, favourites_count: t.count };
    }
    case "boost": {
      const t = toggled(status.boosted, status.boosts_count);
      return { ...status, boosted: t.on, boosts_count: t.count };
    }
    case "bookmark":
      return { ...status, bookmarked: !(status.bookmarked ?? false) };
    case "reaction":
      return reacted(status, action.name) as S;
    default:
      return status;
  }
}

/**
 * One sample post, with its status and whether it shows translated kept as the server and the reader leave them;
 * `translationAs` changes the translation the server answers (a translation it knows less of).
 */
export function SamplePost({
  post,
  lang,
  translationAs,
  signIn,
  votingAt,
}: {
  post: TimelinePost;
  lang: "en" | "ja";
  translationAs?: ((t: PostTranslation) => PostTranslation) | undefined;
  /** A reader without an account, whom the actions that need one lead to sign in (Post's `signIn`). */
  signIn?: ((status: PostStatus) => void) | undefined;
  /** A vote for these choices in flight at first, which the server never answers, to show that state. */
  votingAt?: readonly number[] | undefined;
}) {
  const [status, setStatus] = useState(() => statusOf(post, post.id));
  const answer = translationOf(post, lang);
  const translation = answer && translationAs ? translationAs(answer) : answer;
  const [translated, setTranslated] = useState(post.translated === true);
  const [voting, setVoting] = useState<readonly number[] | undefined>(votingAt);
  const onAction = (action: PostAction) => {
    if (action.type === "translate" || action.type === "original") {
      setTranslated(action.type === "translate");
      return;
    }
    if (action.type === "vote") {
      setVoting(action.choices);
      setTimeout(() => {
        setVoting(undefined);
        setStatus((s) =>
          s.boost === undefined ? voted(s, action.choices) : { ...s, boost: voted(s.boost, action.choices) },
        );
      }, voteDelay);
      return;
    }
    setStatus((s) => (s.boost === undefined ? answered(s, action) : { ...s, boost: answered(s.boost, action) }));
  };
  return (
    <Post
      status={status}
      own={post.own}
      translatable={translation !== undefined}
      translation={translated ? translation : undefined}
      onAction={onAction}
      signIn={signIn}
      voting={voting}
    />
  );
}
