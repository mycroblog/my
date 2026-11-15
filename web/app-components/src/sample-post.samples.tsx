// SPDX-License-Identifier: AGPL-3.0-only

// One sample post of the stories, outside any timeline: it keeps its status and answers its actions as the server
// would, so that it behaves while nothing is fetched; its edited mark opens its edit history, and Delete… and Delete
// and redraft… their confirmation, which takes it away. The reader holds one favourite or one reaction per post
// (FR-116): a reaction takes the favourite back, and a favourite the reaction.

import { ModalPresence } from "@my/design-system";
import { sampleCustomEmoji } from "@my/design-system/samples";
import { useState, type ReactNode } from "react";
import { DeleteConfirm } from "./delete-confirm";
import { EditHistory } from "./edit-history";
import { Post, type PostAction, type PostPicker, type TranslationFailure } from "./post";
import { editsOf, statusOf, translationOf, type TimelinePost } from "./post.samples";
import type { PostReaction, PostStatus, PostTranslation } from "./status";

/** The status a toggle leaves: on or off, its count moved with it. */
const toggled = (on: boolean | undefined, count: number) => ({ on: !(on ?? false), count: count + (on ? -1 : 1) });

/** A reaction the post has none of yet: a Unicode emoji, or a custom one of the samples by its shortcode. */
function added(name: string): PostReaction {
  const url = sampleCustomEmoji[name];
  return url === undefined
    ? { name, count: 1, me: true }
    : { name, shortcode: name, emoji: { shortcode: name, url, static_url: url }, count: 1, me: true };
}

/** The reactions with the reader's taken back. */
const withoutMine = (reactions: readonly PostReaction[]) =>
  reactions.map((r) => (r.me === true ? { ...r, me: false, count: r.count - 1 } : r)).filter((r) => r.count > 0);

/** The status after the reader's reaction with `name`: taken back if it was theirs, else moved there. */
function reacted(status: PostStatus, name: string): PostStatus {
  const was = status.reactions.find((r) => r.me === true)?.name;
  const rest = withoutMine(status.reactions);
  if (was === name) return { ...status, reactions: rest };
  const reactions = rest.some((r) => r.name === name)
    ? rest.map((r) => (r.name === name ? { ...r, me: true, count: r.count + 1 } : r))
    : [...rest, added(name)];
  const favourited = status.favourited ?? false;
  return {
    ...status,
    reactions,
    favourited: false,
    favourites_count: status.favourites_count - (favourited ? 1 : 0),
  };
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
      return {
        ...status,
        favourited: t.on,
        favourites_count: t.count,
        ...(t.on ? { reactions: withoutMine(status.reactions) } : {}),
      };
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

/** How long the sample server takes to answer a translation, so that the translation on its way shows. */
const translateDelay = 1200;

/** What a sample post opens over the page: its edit history, or the confirmation of deleting it. */
export type SampleOpen = "history" | "delete" | "redraft";

/**
 * One sample post, with its status and whether it shows translated kept as the server and the reader leave them;
 * `translationAs` changes the translation the server answers (a translation it knows less of), and `translationFails`
 * makes it refuse instead, as it answers each Translate pressed.
 */
export function SamplePost({
  post,
  lang,
  translationAs,
  translationFails,
  translatingAt,
  signIn,
  votingAt,
  open: initial,
  picker,
  details,
}: {
  post: TimelinePost;
  lang: "en" | "ja";
  translationAs?: ((t: PostTranslation) => PostTranslation) | undefined;
  /** The server refuses to translate it, for this reason: shown at first, and again after each Translate. */
  translationFails?: TranslationFailure | undefined;
  /** Its translation on its way at first (Post's `translating`), which the server never answers, to show that state. */
  translatingAt?: boolean | number | undefined;
  /** A reader without an account, whom the actions that need one lead to sign in (Post's `signIn`). */
  signIn?: ((status: PostStatus) => void) | undefined;
  /** A vote for these choices in flight at first, which the server never answers, to show that state. */
  votingAt?: readonly number[] | undefined;
  /** What is open over the page at first. */
  open?: SampleOpen | undefined;
  /** The emoji picker the react action opens (Post's `picker`). */
  picker?: PostPicker | undefined;
  /** The focused post's own lines (Post's `details`), with the stamp left out of its author's line. */
  details?: ReactNode;
}) {
  const [status, setStatus] = useState(() => statusOf(post, post.id));
  const answer = translationOf(post, lang);
  const translation = answer && translationAs ? translationAs(answer) : answer;
  const [translated, setTranslated] = useState(post.translated === true);
  const [translating, setTranslating] = useState<boolean | number>(translatingAt ?? false);
  const [failure, setFailure] = useState(translationFails);
  const [voting, setVoting] = useState<readonly number[] | undefined>(votingAt);
  const [open, setOpen] = useState<SampleOpen | undefined>(initial);
  const [deleted, setDeleted] = useState(false);
  const close = () => {
    setOpen(undefined);
  };
  const onAction = (action: PostAction) => {
    if (action.type === "original") {
      setTranslated(false);
      return;
    }
    if (action.type === "translate") {
      setFailure(undefined);
      setTranslating(true);
      setTimeout(() => {
        setTranslating(false);
        if (translationFails === undefined) setTranslated(true);
        else setFailure(translationFails);
      }, translateDelay);
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
    if (action.type === "history" || action.type === "delete" || action.type === "redraft") {
      setOpen(action.type);
      return;
    }
    setStatus((s) => (s.boost === undefined ? answered(s, action) : { ...s, boost: answered(s.boost, action) }));
  };
  if (deleted) return null;
  return (
    <>
      <Post
        status={status}
        own={post.own}
        translatable={translation !== undefined || translationFails !== undefined}
        translation={translated ? translation : undefined}
        translating={translating}
        translationFailure={failure}
        onAction={onAction}
        signIn={signIn}
        voting={voting}
        picker={picker}
        details={details}
        stamp={details === undefined}
      />
      <ModalPresence>
        {open === "history" && <EditHistory key="history" edits={editsOf(post)} postLang={post.lang} onClose={close} />}
        {(open === "delete" || open === "redraft") && (
          <DeleteConfirm
            key={open}
            redraft={open === "redraft"}
            poll={(status.boost ?? status).poll !== undefined}
            onCancel={close}
            onConfirm={() => {
              setOpen(undefined);
              setDeleted(true);
            }}
          />
        )}
      </ModalPresence>
    </>
  );
}
