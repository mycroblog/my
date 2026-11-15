// SPDX-License-Identifier: AGPL-3.0-only

// What a post's actions do, the same on every screen that draws posts (the home timeline, a thread, the
// notifications, a profile): Reply and Quote ask the app to open the composer, favourite, boost, bookmark and a
// reaction chip set the reader's mark, or take it back, and Vote sends the reader's vote in the post's poll, each
// through @my/app-data, which every cached copy of the post shows at once. While a vote is in flight every copy of the
// post holds its choices (usePostVoting). A mark or a vote that fails shows again as it was, and a notice at the
// bottom of the page says so, in the server's words when it refused, with Try again for a moment. It is shared by the
// features, not a feature of its own.

import type { PostAction } from "@my/app-components";
import { refusal, usePollVote, useStatusMarks, type PollVote, type Status, type StatusMark } from "@my/app-data";
import { UndoNotice, useLocale } from "@my/design-system";
import { createContext, use, useEffect, useState, type ReactNode } from "react";
import { stringsIn } from "./locale";

/** How long the notice of a failed mark offers Try again. */
const noticeMs = 6000;

/** What the composer opens for: a reply to the post, or a quote of it. */
export type ComposeRequest = { kind: "reply" | "quote"; id: string };

/** A write a post's action asks for: a mark, or a vote in its poll. */
type Write = { kind: "mark"; mark: StatusMark } | { kind: "vote"; vote: PollVote };

type Send = (id: string, write: Write) => void;

/** The choices of each status's vote in flight, by the status's ID. */
type Voting = ReadonlyMap<string, readonly number[]>;

const Context = createContext<{
  onCompose: ((to: ComposeRequest) => void) | undefined;
  send: Send;
  voting: Voting;
} | null>(null);

/**
 * The mark an action asks for, on the status as the cache holds it: the one it does not have, set, or the one it has,
 * taken back; none for an action that is no mark, or a status not in the cache.
 */
export function markOf(action: PostAction, status: Status | undefined): StatusMark | undefined {
  if (status === undefined) return undefined;
  switch (action.type) {
    case "favourite":
      return { type: "favourite", on: !(status.favourited ?? false) };
    case "boost":
      return { type: "boost", on: !(status.boosted ?? false) };
    case "bookmark":
      return { type: "bookmark", on: !(status.bookmarked ?? false) };
    case "reaction":
      return {
        type: "reaction",
        name: action.name,
        on: !(status.reactions.find((r) => r.name === action.name)?.me ?? false),
      };
    default:
      return undefined;
  }
}

/** Sends a write through @my/app-data. */
function useWrite(): (id: string, write: Write) => Promise<void> {
  const { mark } = useStatusMarks();
  const vote = usePollVote();
  return (id, write) => (write.kind === "mark" ? mark(id, write.mark) : vote(id, write.vote));
}

/** The handler of a post's actions, for `onAction` of a post or a column. */
export function usePostActions(): (action: PostAction) => void {
  const context = use(Context);
  const { cached } = useStatusMarks();
  const write = useWrite();
  const send: Send =
    context?.send ??
    ((id, w) => {
      void write(id, w).catch(() => undefined);
    });
  return (action) => {
    if (action.type === "reply" || action.type === "quote") {
      context?.onCompose?.({ kind: action.type, id: action.id });
      return;
    }
    if (action.type === "vote") {
      send(action.id, { kind: "vote", vote: { poll: action.poll, choices: action.choices } });
      return;
    }
    const m = markOf(action, cached(action.id));
    if (m !== undefined) send(action.id, { kind: "mark", mark: m });
  };
}

/**
 * The choices of the reader's vote in flight in the poll a status shows (a boost's, the boosted status's), for
 * `voting` of a post; undefined while none is.
 */
export function usePostVoting(): (status: {
  readonly id: string;
  readonly boost?: { readonly id: string } | undefined;
}) => readonly number[] | undefined {
  const voting = use(Context)?.voting;
  return (status) => voting?.get((status.boost ?? status).id);
}

/**
 * Where the posts of `children` answer their actions: `onCompose` opens the composer, a vote in flight shows on every
 * copy of its post, and a failed mark or vote is told.
 */
export function PostActions({
  onCompose,
  children,
}: {
  onCompose?: ((to: ComposeRequest) => void) | undefined;
  children: ReactNode;
}) {
  const { t } = stringsIn(useLocale().locale);
  const write = useWrite();
  const [voting, setVoting] = useState<Voting>(new Map());
  const [failed, setFailed] = useState<{ id: string; write: Write; reason: string | undefined } | undefined>(undefined);
  useEffect(() => {
    if (failed === undefined) return undefined;
    const timer = setTimeout(() => {
      setFailed(undefined);
    }, noticeMs);
    return () => {
      clearTimeout(timer);
    };
  }, [failed]);
  /** Sets the vote in flight on the status of `id`, or takes it off. */
  const hold = (id: string, choices: readonly number[] | undefined) => {
    setVoting((now) => {
      const next = new Map(now);
      if (choices === undefined) next.delete(id);
      else next.set(id, choices);
      return next;
    });
  };
  const send: Send = (id, w) => {
    if (w.kind === "vote") {
      if (voting.has(id)) return;
      hold(id, w.vote.choices);
    }
    setFailed(undefined);
    write(id, w)
      .catch((e: unknown) => {
        setFailed({ id, write: w, reason: refusal(e) });
      })
      .finally(() => {
        if (w.kind === "vote") hold(id, undefined);
      });
  };
  return (
    <Context value={{ onCompose, send, voting }}>
      {children}
      {failed && (
        <UndoNotice
          action={t("post_actions.retry")}
          onAction={() => {
            send(failed.id, failed.write);
          }}
        >
          {failed.reason ??
            t("post_actions.failed", { action: failed.write.kind === "vote" ? "vote" : failed.write.mark.type })}
        </UndoNotice>
      )}
    </Context>
  );
}
