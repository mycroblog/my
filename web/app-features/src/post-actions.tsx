// SPDX-License-Identifier: AGPL-3.0-only

// What a post's actions do, the same on every screen that draws posts (the home timeline, a thread, the
// notifications, a profile): Reply and Quote ask the app to open the composer, favourite, boost, bookmark and a
// reaction chip set the reader's mark, or take it back, and Vote sends the reader's vote in the post's poll, each
// through @my/app-data, which every cached copy of the post shows at once. While a vote is in flight every copy of the
// post holds its choices (usePostVoting). A mark or a vote that fails shows again as it was, and a notice at the
// bottom of the page says so, in the server's words when it refused, with Try again for a moment. The react action
// opens the emoji picker (emoji/picker.ts, usePostPicker) where the app decides, a sheet on a phone or a popover
// beside the button, and a reaction counts its emoji as used on this device.
//
// The more menu (FR-108, FR-109): Copy link copies the post's address, Open original opens a remote post's page on its
// own server, and Share hands the address to the system's share sheet, else copies it. On the reader's own post, Edit
// reads what they typed and opens the composer to edit it; Delete… and Delete and redraft… ask first, then delete it,
// gone at once from every list and view, and a redraft opens the composer with what the post held. The edited mark
// opens the post's edit history once it was read. A read or a deletion that fails is told with Try again (a redraft's
// again deletes with what its first attempt read, since a post deleted may not be read again); a copy the
// browser refused is told without, since it would be refused again. It is shared by the features, not a feature of
// its own.

import { DeleteConfirm, EditHistory, statusPath, type PostAction, type PostPicker } from "@my/app-components";
import {
  RedraftError,
  refusal,
  useOwnPosts,
  usePollVote,
  useStatusHistory,
  useStatusMarks,
  useViewer,
  type PollVote,
  type PostSource,
  type Status,
  type StatusMark,
} from "@my/app-data";
import { ModalPresence, UndoNotice, useLocale } from "@my/design-system";
import { copyText, shareLink } from "@my/platform";
import { createContext, use, useEffect, useState, type ReactNode } from "react";
import { rememberReaction, usePickerData } from "./emoji/picker";
import { emojiStrings } from "./emoji/strings";
import { stringsIn } from "./locale";

/** How long a notice stays. */
const noticeMs = 6000;

/**
 * What the composer opens for: a reply to the post, a quote of it, an edit of the reader's post, or a new post written
 * again from the reader's post just deleted (a redraft); the last two with what the post held.
 */
export type ComposeRequest =
  { kind: "reply" | "quote"; id: string } | { kind: "edit" | "redraft"; id: string; source: PostSource };

/** A write a post's action asks for: a mark, or a vote in its poll. */
type Write = { kind: "mark"; mark: StatusMark } | { kind: "vote"; vote: PollVote };

type Send = (id: string, write: Write) => void;

/** The choices of each status's vote in flight, by the status's ID. */
type Voting = ReadonlyMap<string, readonly number[]>;

/** The actions of the more menu and the edited mark, which PostActions answers. */
type MenuAction = {
  type: "copy-link" | "open-original" | "share" | "edit" | "delete" | "redraft" | "history";
  id: string;
};

const menuActions: ReadonlySet<PostAction["type"]> = new Set<MenuAction["type"]>([
  "copy-link",
  "open-original",
  "share",
  "edit",
  "delete",
  "redraft",
  "history",
]);
const isMenuAction = (a: PostAction): a is MenuAction & PostAction => menuActions.has(a.type);

const Context = createContext<{
  onCompose: ((to: ComposeRequest) => void) | undefined;
  send: Send;
  voting: Voting;
  menu: (action: MenuAction) => void;
  /** The ID of the reader's account; undefined without one, or until it is read. */
  viewer: string | undefined;
  picker: PostPicker;
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
    if (isMenuAction(action)) {
      context?.menu(action);
      return;
    }
    const m = markOf(action, cached(action.id));
    if (m === undefined) return;
    if (m.type === "reaction" && m.on) rememberReaction(m.name);
    send(action.id, { kind: "mark", mark: m });
  };
}

/**
 * The emoji picker of a post's react action, for `picker` of a post: the server's custom emoji and Unicode's, opening
 * in a sheet or a popover as the app said; none outside PostActions. A post leads a reader without an account to sign
 * in instead, and opens none.
 */
export function usePostPicker(): PostPicker | undefined {
  return use(Context)?.picker;
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

/** Whether the reader wrote the post a status shows (a boost's, the boosted status), for `own` of a post. */
export function usePostOwn(): (status: {
  readonly account: { readonly id: string };
  readonly boost?: { readonly account: { readonly id: string } } | undefined;
}) => boolean {
  const viewer = use(Context)?.viewer;
  return (status) => viewer !== undefined && (status.boost ?? status).account.id === viewer;
}

/** The address of a post to hand on: its own page where it was posted, else its page here. */
const linkOf = (status: Status) => status.url ?? new URL(statusPath(status), location.href).href;

/** A notice at the bottom of the page, with Try again when the same may succeed. */
type Notice = { readonly text: string; readonly retry?: (() => void) | undefined };

/** The confirmation open over the page: of deleting the post of `id`, or of deleting it to write it again. */
type Confirm = { readonly id: string; readonly redraft: boolean; readonly poll: boolean; readonly busy: boolean };

/** The edit history of the post of `id`, open over the page once read. */
function History({ id, lang, onClose }: { id: string; lang: string | undefined; onClose: () => void }) {
  const edits = useStatusHistory(id);
  if (edits === undefined) return null;
  return <EditHistory edits={edits} postLang={lang} onClose={onClose} />;
}

/**
 * Where the posts of `children` answer their actions: `onCompose` opens the composer (none for a reader without an
 * account, whose posts' actions that need one lead to the sign-in), a vote in flight shows on every copy of its post,
 * the more menu's actions are answered, what failed is told, and the emoji picker opens in a sheet when `sheet` (the
 * app's choice by the viewport), else in a popover.
 */
export function PostActions({
  onCompose,
  sheet = false,
  children,
}: {
  onCompose?: ((to: ComposeRequest) => void) | undefined;
  sheet?: boolean | undefined;
  children: ReactNode;
}) {
  const { locale } = useLocale();
  const { t } = stringsIn(locale);
  const words = emojiStrings(locale, []).sheet;
  const { ahead, ...data } = usePickerData(locale);
  const picker: PostPicker = { ...data, sheet, title: words.title, closeLabel: words.close };
  const write = useWrite();
  const own = useOwnPosts();
  // Only a reader with an account (whom the composer opens for) has posts of their own.
  const viewer = useViewer(onCompose !== undefined)?.id;
  const [voting, setVoting] = useState<Voting>(new Map());
  const [notice, setNotice] = useState<Notice | undefined>(undefined);
  const [confirm, setConfirm] = useState<Confirm | undefined>(undefined);
  const [history, setHistory] = useState<{ id: string; lang: string | undefined } | undefined>(undefined);
  // The posts' react action is at hand: the picker's code comes once the page is idle, and its emoji too for a reader
  // who opened a picker here before; else its emoji come on the reach. A reader without an account (no composer)
  // reacts after signing in, and gets nothing ahead.
  const reacts = onCompose !== undefined;
  useEffect(() => (reacts ? ahead() : undefined), [reacts, ahead]);
  useEffect(() => {
    if (notice === undefined) return undefined;
    const timer = setTimeout(() => {
      setNotice(undefined);
    }, noticeMs);
    return () => {
      clearTimeout(timer);
    };
  }, [notice]);
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
    setNotice(undefined);
    write(id, w)
      .catch((e: unknown) => {
        setNotice({
          text: refusal(e) ?? t("post_actions.failed", { action: w.kind === "vote" ? "vote" : w.mark.type }),
          retry: () => {
            send(id, w);
          },
        });
      })
      .finally(() => {
        if (w.kind === "vote") hold(id, undefined);
      });
  };
  /** Tells that `what` failed, in the server's words when it refused, with Try again doing `again`. */
  const failed = (e: unknown, what: string, again: () => void) => {
    setNotice({ text: refusal(e) ?? what, retry: again });
  };
  /** Copies the post's address, and tells whether it was. */
  const copy = (url: string) => {
    void copyText(url).then((done) => {
      setNotice({ text: t(done ? "post_actions.link_copied" : "post_actions.copy_failed") });
    });
  };
  const remove = (id: string) => {
    setConfirm(undefined);
    setNotice(undefined);
    own.remove(id).then(
      () => {
        setNotice({ text: t("post_actions.deleted") });
      },
      (e: unknown) => {
        failed(e, t("post_actions.delete_failed"), () => {
          remove(id);
        });
      },
    );
  };
  /** Deletes the post to write it again; `kept` is the source an attempt read before its deletion failed. */
  const redraft = (id: string, kept?: PostSource) => {
    setConfirm((c) => c && { ...c, busy: true });
    setNotice(undefined);
    own.redraft(id, kept).then(
      (source) => {
        setConfirm(undefined);
        onCompose?.({ kind: "redraft", id, source });
      },
      (e: unknown) => {
        setConfirm(undefined);
        const source = e instanceof RedraftError ? e.source : kept;
        failed(e instanceof RedraftError ? e.cause : e, t("post_actions.delete_failed"), () => {
          redraft(id, source);
        });
      },
    );
  };
  const menu = (action: MenuAction) => {
    const { id } = action;
    const status = own.cached(id);
    switch (action.type) {
      case "copy-link":
        if (status !== undefined) copy(linkOf(status));
        return;
      case "share":
        if (status === undefined) return;
        void shareLink(linkOf(status)).then((outcome) => {
          if (outcome === "copied") setNotice({ text: t("post_actions.link_copied") });
          if (outcome === "failed") setNotice({ text: t("post_actions.copy_failed") });
        });
        return;
      case "open-original":
        if (status?.url !== undefined) window.open(status.url, "_blank", "noopener,noreferrer");
        return;
      case "edit":
        setNotice(undefined);
        own.source(id).then(
          (source) => {
            onCompose?.({ kind: "edit", id, source });
          },
          (e: unknown) => {
            failed(e, t("post_actions.source_failed"), () => {
              menu(action);
            });
          },
        );
        return;
      case "delete":
      case "redraft":
        setConfirm({ id, redraft: action.type === "redraft", poll: status?.poll !== undefined, busy: false });
        return;
      case "history":
        setNotice(undefined);
        own.history(id).then(
          () => {
            const lang = status?.language;
            setHistory({ id, lang: lang === "und" || lang === "" ? undefined : lang });
          },
          (e: unknown) => {
            failed(e, t("post_actions.history_failed"), () => {
              menu(action);
            });
          },
        );
        return;
    }
  };
  return (
    <Context value={{ onCompose, send, voting, menu, viewer, picker }}>
      {children}
      <ModalPresence>
        {confirm && (
          <DeleteConfirm
            key={`${confirm.redraft ? "redraft" : "delete"}:${confirm.id}`}
            redraft={confirm.redraft}
            poll={confirm.poll}
            busy={confirm.busy}
            onCancel={() => {
              if (!confirm.busy) setConfirm(undefined);
            }}
            onConfirm={() => {
              if (confirm.busy) return;
              if (confirm.redraft) redraft(confirm.id);
              else remove(confirm.id);
            }}
          />
        )}
        {history && (
          <History
            key={`history:${history.id}`}
            id={history.id}
            lang={history.lang}
            onClose={() => {
              setHistory(undefined);
            }}
          />
        )}
      </ModalPresence>
      {notice && (
        <UndoNotice action={notice.retry && t("post_actions.retry")} onAction={notice.retry}>
          {notice.text}
        </UndoNotice>
      )}
    </Context>
  );
}
