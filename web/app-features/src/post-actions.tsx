// SPDX-License-Identifier: AGPL-3.0-only

// What a post's actions do, the same on every screen that draws posts (the home timeline, a thread, the
// notifications, a profile): Reply and Quote ask the app to open the composer, favourite, boost, bookmark and a
// reaction chip set the reader's mark, or take it back, and Vote sends the reader's vote in the post's poll, each
// through @my/app-data, which every cached copy of the post shows at once. While a vote is in flight every copy of the
// post holds its choices (usePostVoting). A bookmark or a thread's mute, set or taken back, which the post shows no mark
// of, is told by a notice at the bottom of the page. A mark or a vote that fails shows again as it was, and a notice at the
// bottom of the page says so, in the server's words when it refused, with Try again for a moment, or, for one a
// restriction of the reader's account refused, which would be refused again, a link to Account status. The react action
// opens the emoji picker (emoji/picker.ts, usePostPicker) where the app decides, a sheet on a phone or a popover
// beside the button, and a reaction counts its emoji as used on this device.
//
// The more menu (FR-108, FR-109): Copy link copies the post's address, Open original opens a remote post's page on its
// own server, and Share hands the address to the system's share sheet, else copies it. On the reader's own post, Edit
// reads what they typed and opens the composer to edit it; Delete… and Delete and redraft… ask first, then delete it,
// gone at once from every list and view, and a redraft opens the composer with what the post held. The edited mark
// opens the post's edit history once it was read. A read or a deletion that fails is told with Try again (a redraft's
// again deletes with what its first attempt read, since a post deleted may not be read again); a copy the
// browser refused is told without, since it would be refused again. A screen's own write around its posts (following
// a hashtag) tells its failure in the same notice (usePostNotice). It is shared by the features, not a feature of its
// own.
//
// The author's actions of another's post (FR-303, FR-304, usePostAuthor): mute or block its author, block after a
// confirmation, and mute a remote author's server. Once done, the post shows in its place what was done, with Undo
// (Unblock for a block, since the follows it ended do not come back); the lists stay as they are until they open
// again, so that the line stays where the reader pressed.
//
// The posts' link cards show in the size the reader chose (FR-007, usePostCardSize).

import {
  accountStatusPath,
  DeleteConfirm,
  EditHistory,
  sentenceHandleOf,
  ServerDomain,
  statusPath,
  type PostAccount,
  type PostAction,
  type PostPicker,
  type PostStatus,
} from "@my/app-components";
import {
  RedraftError,
  refusal,
  restrictionRefused,
  useChangeRelationship,
  useInstance,
  useMuteDomain,
  useOwnPosts,
  usePollVote,
  usePreferences,
  useStatusHistory,
  useStatusMarks,
  useViewer,
  type LinkCardSize,
  type PollVote,
  type PostSource,
  type Status,
  type StatusMark,
} from "@my/app-data";
import { ConfirmDialog, ModalPresence, UndoNotice, useLocale } from "@my/design-system";
import { copyText, shareLink } from "@my/platform";
import { createContext, use, useEffect, useState, type ReactNode } from "react";
import { rememberReaction, usePickerData } from "./emoji/picker";
import { emojiStrings } from "./emoji/strings";
import { stringsIn } from "./locale";
import { ReportDialog } from "./report";

/** How long a notice stays. */
const noticeMs = 6000;

/**
 * What the composer opens for: a reply to the post, a quote of it, an edit of the reader's post, or a new post written
 * again from the reader's post just deleted (a redraft), the last two with what the post held; or a draft of the
 * reader's on the server (a notification of its failed schedule); or a new post on the board of an event, named by its
 * title. A reply to a post of a board, and a redraft of one, name the board's event (`board`): they go to the board.
 */
export type ComposeRequest =
  | { kind: "reply"; id: string; board?: string | undefined }
  | { kind: "quote" | "draft"; id: string }
  | { kind: "edit"; id: string; source: PostSource }
  | { kind: "redraft"; id: string; source: PostSource; board?: string | undefined }
  | { kind: "board"; event: string; title: string };

/** The event whose board `status` is posted on, as a request to the composer names it; none for any other post. */
const boardOn = (status: Status | undefined) =>
  status?.event_board === undefined ? {} : { board: status.event_board.event_id };

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

/** What the reader does from a post's menu to its author, or to its author's server. */
export type AuthorAction = "mute" | "block" | "mute-server";

/** What the reader did from a post's menu to its author or its server, which the post shows in its place. */
export type AuthorDone = { readonly action: AuthorAction; readonly account: PostAccount };

/** The author's actions of the posts under PostActions, and what they left in a post's place. */
export type PostAuthor = {
  /** Does `action` to the author of `status` (the post a boost shows), after asking first for a block. */
  readonly act: (action: AuthorAction, status: PostStatus) => void;
  /** What the reader did from the menu of the post of `id`, shown in its place; undefined while nothing is. */
  readonly done: (id: string) => AuthorDone | undefined;
  /** Undoes what the reader did from the menu of the post of `id`, which shows again. */
  readonly undo: (id: string) => void;
  /** Opens the report of the author of `status` (the post a boost shows), the post chosen (report.tsx). */
  readonly report: (status: PostStatus) => void;
};

const Context = createContext<{
  onCompose: ((to: ComposeRequest) => void) | undefined;
  send: Send;
  voting: Voting;
  menu: (action: MenuAction) => void;
  /** The ID of the reader's account; undefined without one, or until it is read. */
  viewer: string | undefined;
  picker: PostPicker;
  failed: Failed;
  author: PostAuthor;
  /** How the posts' link cards show: the reader's preference. */
  cardSize: LinkCardSize;
} | null>(null);

/** Tells that `what` failed, in the server's words when it refused (`error`), with Try again doing `again` if given. */
type Failed = (error: unknown, what: string, again?: () => void) => void;

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
    case "mute-thread":
      return { type: "mute", on: !(status.muted ?? false) };
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

/** Opens the composer, as the app does; none for a reader without an account, or outside PostActions. */
export function useCompose(): ((to: ComposeRequest) => void) | undefined {
  return use(Context)?.onCompose;
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
    if (action.type === "reply") {
      context?.onCompose?.({ kind: "reply", id: action.id, ...boardOn(cached(action.id)) });
      return;
    }
    if (action.type === "quote") {
      context?.onCompose?.({ kind: "quote", id: action.id });
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

/**
 * Tells a failure of a screen's own write (following a hashtag) in the notice where a post's actions tell theirs: in
 * the server's words when it refused, else in `what`, with Try again doing `again` where the same may succeed. Outside
 * PostActions there is no notice, and it tells nothing.
 */
export function usePostNotice(): Failed {
  return use(Context)?.failed ?? (() => undefined);
}

/**
 * The author's actions of the posts, and what they left in a post's place; undefined outside PostActions, where a post
 * offers none.
 */
export function usePostAuthor(): PostAuthor | undefined {
  return use(Context)?.author;
}

/**
 * How the posts' link cards show: the size the reader chose (`link_card_size` of their preferences); large, the
 * server's default, until that is read, for a reader without an account, and outside PostActions.
 */
export function usePostCardSize(): LinkCardSize {
  return use(Context)?.cardSize ?? "large";
}

/** The address of a post to hand on: its own page where it was posted, else its page here. */
const linkOf = (status: Status) => status.url ?? new URL(statusPath(status), location.href).href;

/** A notice at the bottom of the page, with Try again when the same may succeed, or a link where it cannot. */
type Notice = {
  readonly text: string;
  readonly retry?: (() => void) | undefined;
  /** Where the reader can do something about it: Account status, for a write a restriction of their account refused. */
  readonly link?: string | undefined;
};

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
 * the more menu's actions are answered, what failed is told, the emoji picker opens in a sheet when `sheet` (the
 * app's choice by the viewport), else in a popover, and a hashtag of this server opens the app's hashtag screen.
 */
export function PostActions({
  onCompose,
  onEditArticle,
  sheet = false,
  children,
}: {
  onCompose?: ((to: ComposeRequest) => void) | undefined;
  /** Opens the article editor for the reader's article of `id`, which Edit of an article does. */
  onEditArticle?: ((id: string) => void) | undefined;
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
  const cardSize = usePreferences(onCompose !== undefined)?.link_card_size ?? "large";
  const domain = useInstance()?.domain;
  const [voting, setVoting] = useState<Voting>(new Map());
  const [notice, setNotice] = useState<Notice | undefined>(undefined);
  const [confirm, setConfirm] = useState<Confirm | undefined>(undefined);
  const [history, setHistory] = useState<{ id: string; lang: string | undefined } | undefined>(undefined);
  const change = useChangeRelationship();
  const muteServer = useMuteDomain();
  /** What the reader did to each post's author or server from its menu, by the post's ID. */
  const [done, setDone] = useState<ReadonlyMap<string, AuthorDone>>(new Map());
  /** The confirmation of blocking the author of a post, open over the page. */
  const [blocking, setBlocking] = useState<{ status: PostStatus } | undefined>(undefined);
  /**
   * The report of a post's author, open over the page, and what the reader did to the author from it, which the post
   * shows in its place once the report closes, so that the focus goes back to the menu it was opened from.
   */
  const [reporting, setReporting] = useState<{ status: PostStatus; did?: AuthorAction } | undefined>(undefined);
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
  /**
   * Tells that `what` failed, in the server's words when it refused, with Try again doing `again`; a write a
   * restriction of the reader's account refused would be refused again, and leads to Account status instead.
   */
  const failed: Failed = (e, what, again) => {
    const text = refusal(e) ?? what;
    setNotice(restrictionRefused(e) ? { text, link: accountStatusPath } : { text, retry: again });
  };
  const send: Send = (id, w) => {
    if (w.kind === "vote") {
      if (voting.has(id)) return;
      hold(id, w.vote.choices);
    }
    setNotice(undefined);
    write(id, w)
      .then(() => {
        // A bookmark and a thread's mute show no mark on the post, only in its menu: the notice says what they did.
        if (w.kind !== "mark") return;
        if (w.mark.type === "bookmark") {
          setNotice({ text: t(w.mark.on ? "post_actions.bookmarked" : "post_actions.unbookmarked") });
        }
        if (w.mark.type === "mute") {
          setNotice({ text: t(w.mark.on ? "post_actions.thread_muted" : "post_actions.thread_unmuted") });
        }
      })
      .catch((e: unknown) => {
        failed(e, t("post_actions.failed", { action: w.kind === "vote" ? "vote" : w.mark.type }), () => {
          send(id, w);
        });
      })
      .finally(() => {
        if (w.kind === "vote") hold(id, undefined);
      });
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
  /**
   * Deletes the post to write it again; `kept` is the source an attempt read before its deletion failed, and `board`
   * the board a post of one is written again on, read before the deletion takes the post from the cache.
   */
  const redraft = (id: string, kept?: PostSource, read?: ReturnType<typeof boardOn>) => {
    const board = read ?? boardOn(own.cached(id));
    setConfirm((c) => c && { ...c, busy: true });
    setNotice(undefined);
    own.redraft(id, kept).then(
      (source) => {
        setConfirm(undefined);
        onCompose?.({ kind: "redraft", id, source, ...board });
      },
      (e: unknown) => {
        setConfirm(undefined);
        const source = e instanceof RedraftError ? e.source : kept;
        failed(e instanceof RedraftError ? e.cause : e, t("post_actions.delete_failed"), () => {
          redraft(id, source, board);
        });
      },
    );
  };
  /** Shows what the reader did in the place of the post of `id`, or the post again. */
  const leave = (id: string, what: AuthorDone | undefined) => {
    setDone((now) => {
      const next = new Map(now);
      if (what === undefined) next.delete(id);
      else next.set(id, what);
      return next;
    });
  };
  /** The failure's name of an action on an author or a server, done (`on`) or undone. */
  const failure = (action: AuthorAction, on: boolean) =>
    t("profile.action_failed", {
      action: action === "mute-server" ? (on ? "mute_server" : "unmute_server") : on ? action : `un${action}`,
    });
  /**
   * Does or undoes `action` on `account`, leaving the shown lists as they are, so that the post stays where it was, in
   * its new state; their marks are read when they open again.
   */
  const writeAuthor = (action: AuthorAction, account: PostAccount, on: boolean): Promise<void> => {
    if (action === "mute-server") return muteServer(account.domain, on, "later");
    return change(account.id, on ? action : action === "mute" ? "unmute" : "unblock", "later");
  };
  const doAuthor = (action: AuthorAction, status: PostStatus) => {
    setNotice(undefined);
    writeAuthor(action, status.account, true).then(
      () => {
        leave(status.id, { action, account: status.account });
      },
      (e: unknown) => {
        failed(e, failure(action, true), () => {
          doAuthor(action, status);
        });
      },
    );
  };
  const author: PostAuthor = {
    act: (action, status) => {
      if (action === "block") setBlocking({ status });
      else doAuthor(action, status);
    },
    done: (id) => done.get(id),
    report: (status) => {
      setReporting({ status });
    },
    undo: (id) => {
      const what = done.get(id);
      if (what === undefined) return;
      setNotice(undefined);
      writeAuthor(what.action, what.account, false).then(
        () => {
          leave(id, undefined);
        },
        (e: unknown) => {
          failed(e, failure(what.action, false), () => {
            author.undo(id);
          });
        },
      );
    },
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
        // An article is edited in the article editor, which reads what its author wrote of it.
        if (status?.article !== undefined) {
          onEditArticle?.(id);
          return;
        }
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
    <Context value={{ onCompose, send, voting, menu, viewer, picker, failed, author, cardSize }}>
      <ServerDomain value={domain}>{children}</ServerDomain>
      <ModalPresence>
        {blocking && (
          <ConfirmDialog
            key={`block:${blocking.status.id}`}
            title={t("profile.block_confirm_title", { handle: sentenceHandleOf(blocking.status.account) })}
            cancel={t("profile.cancel")}
            confirm={t("profile.block_confirm")}
            onCancel={() => {
              setBlocking(undefined);
            }}
            onConfirm={() => {
              // It closes at once, as a deletion's does: the post answers in its place.
              setBlocking(undefined);
              doAuthor("block", blocking.status);
            }}
          >
            {t("profile.block_confirm_text")}
          </ConfirmDialog>
        )}
        {reporting && (
          <ReportDialog
            key={`report:${reporting.status.id}`}
            subject={{ kind: "account", account: reporting.status.account, post: reporting.status }}
            act={(action) =>
              writeAuthor(action, reporting.status.account, true).then(() => {
                // A block says more of the author than a mute does, whichever came first.
                setReporting((now) => now && { ...now, did: now.did === "block" ? "block" : action });
              })
            }
            onClose={() => {
              const { status, did } = reporting;
              setReporting(undefined);
              if (did !== undefined) leave(status.id, { action: did, account: status.account });
            }}
          />
        )}
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
        <UndoNotice
          action={notice.link === undefined ? notice.retry && t("post_actions.retry") : t("settings.account.status")}
          onAction={notice.retry}
          href={notice.link}
        >
          {notice.text}
        </UndoNotice>
      )}
    </Context>
  );
}
