// SPDX-License-Identifier: AGPL-3.0-only

// What a post's actions do, the same on every screen that draws posts (the home timeline, a thread, the
// notifications, a profile): Reply and Quote ask the app to open the composer, and favourite, boost, bookmark and a
// reaction chip set the reader's mark, or take it back, through @my/app-data, which every cached copy of the post shows
// at once. A mark that fails shows again as it was, and a notice at the bottom of the page says so, in the server's
// words when it refused, with Try again for a moment. It is shared by the features, not a feature of its own.

import type { PostAction } from "@my/app-components";
import { refusal, useStatusMarks, type Status, type StatusMark } from "@my/app-data";
import { UndoNotice, useLocale } from "@my/design-system";
import { createContext, use, useEffect, useState, type ReactNode } from "react";
import { stringsIn } from "./locale";

/** How long the notice of a failed mark offers Try again. */
const noticeMs = 6000;

/** What the composer opens for: a reply to the post, or a quote of it. */
export type ComposeRequest = { kind: "reply" | "quote"; id: string };

type Send = (id: string, mark: StatusMark) => void;

const Context = createContext<{ onCompose: ((to: ComposeRequest) => void) | undefined; send: Send } | null>(null);

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

/** The handler of a post's actions, for `onAction` of a post or a column. */
export function usePostActions(): (action: PostAction) => void {
  const context = use(Context);
  const { mark, cached } = useStatusMarks();
  const send: Send =
    context?.send ??
    ((id, m) => {
      void mark(id, m).catch(() => undefined);
    });
  return (action) => {
    if (action.type === "reply" || action.type === "quote") {
      context?.onCompose?.({ kind: action.type, id: action.id });
      return;
    }
    const m = markOf(action, cached(action.id));
    if (m !== undefined) send(action.id, m);
  };
}

/** Where the posts of `children` answer their actions: `onCompose` opens the composer, and a failed mark is told. */
export function PostActions({
  onCompose,
  children,
}: {
  onCompose?: ((to: ComposeRequest) => void) | undefined;
  children: ReactNode;
}) {
  const { t } = stringsIn(useLocale().locale);
  const { mark } = useStatusMarks();
  const [failed, setFailed] = useState<{ id: string; mark: StatusMark; reason: string | undefined } | undefined>(
    undefined,
  );
  useEffect(() => {
    if (failed === undefined) return undefined;
    const timer = setTimeout(() => {
      setFailed(undefined);
    }, noticeMs);
    return () => {
      clearTimeout(timer);
    };
  }, [failed]);
  const send: Send = (id, m) => {
    setFailed(undefined);
    mark(id, m).catch((e: unknown) => {
      setFailed({ id, mark: m, reason: refusal(e) });
    });
  };
  return (
    <Context value={{ onCompose, send }}>
      {children}
      {failed && (
        <UndoNotice
          action={t("post_actions.retry")}
          onAction={() => {
            send(failed.id, failed.mark);
          }}
        >
          {failed.reason ?? t("post_actions.failed", { action: failed.mark.type })}
        </UndoNotice>
      )}
    </Context>
  );
}
