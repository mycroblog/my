// SPDX-License-Identifier: AGPL-3.0-only

// A post's translation on the reader's request (FR-120, ADR-0148 §4, §12), the same on every screen that draws posts:
// the reader's view of one post in one place, so each drawn post keeps its own, in memory only, and another copy of
// the post elsewhere stays as it is. A post offers Translate when it is in another language than the reader's, is not
// the reader's own (they wrote it), and the instance has a way to translate it: a provider of the server that may receive its visibility (for a signed-in
// reader), or the browser's translator, where the instance allows it and the browser translates between the two
// languages. The server is asked first; when it refuses for a reason the browser may cure (its limit, the post's
// visibility, the language, its providers down), the browser translates at once where it needs no download, and
// otherwise the reader's next press goes to it (Translate in this browser), since a download needs that press. A post
// the reader may not read is never handed over. Without a way, the refusal is said, with Try again only where the
// server may answer next time. Show original shows the text again, and Translate the translation it already has.

import {
  Post,
  sentenceHandleOf,
  translateContent,
  translatePlain,
  type PostProps,
  type PostStatus,
  type PostTranslation,
} from "@my/app-components";
import type { TranslationFailure } from "@my/app-components";
import {
  serverTranslates,
  TranslationRefusal,
  useInstance,
  useTranslateStatus,
  type StatusTranslation,
} from "@my/app-data";
import { CaptionLine, Entry, MenuGroup, MenuItem, TextButton, useLocale } from "@my/design-system";
import {
  browserName,
  browserTranslatorAvailability,
  hasBrowserTranslator,
  needsActivation,
  openBrowserTranslator,
  type TranslatorAvailability,
} from "@my/platform";
import { useEffect, useRef, useState } from "react";
import { stringsIn } from "./locale";
import {
  usePostAuthor,
  usePostCardSize,
  usePostOwn,
  type AuthorAction,
  type AuthorDone,
  type PostAuthor,
} from "./post-actions";
import { ApprovalLine } from "./post-approval";

/** The language tag of the reader's locale that a translation goes into: its language, and its script if it names one. */
export function readerLanguage(locale: string): string {
  const l = new Intl.Locale(locale);
  return l.script === undefined ? l.language : `${l.language}-${l.script}`;
}

/**
 * Whether a post in `source` is in another language than the reader's `target`: another language, or the same in
 * another script (zh-Hant, zh-Hans). A post whose language is not known is not, since it may be the reader's.
 */
export function inAnotherLanguage(source: string, target: string): boolean {
  if (source === "" || source === "und") return false;
  let s: Intl.Locale;
  try {
    s = new Intl.Locale(source);
  } catch {
    return false;
  }
  const t = new Intl.Locale(target);
  if (s.language !== t.language) return true;
  return s.script !== undefined && t.script !== undefined && s.script !== t.script;
}

/** What the browser said of each pair of languages, asked once a session; a translation it made sets `available`. */
const availabilities = new Map<string, Promise<TranslatorAvailability>>();

const pairKey = (source: string, target: string) => `${source}>${target}`;

function availabilityOf(source: string, target: string): Promise<TranslatorAvailability> {
  const key = pairKey(source, target);
  let found = availabilities.get(key);
  if (found === undefined) {
    found = browserTranslatorAvailability(source, target);
    availabilities.set(key, found);
  }
  return found;
}

/** Whether the browser translates from `source` into `target`, while `ask`; undefined until it said. */
function useBrowserAvailability(source: string, target: string, ask: boolean): TranslatorAvailability | undefined {
  const [said, setSaid] = useState<{ key: string; availability: TranslatorAvailability } | undefined>(undefined);
  const key = pairKey(source, target);
  useEffect(() => {
    // An answer for another pair than the one asked for now is never read: `said` keeps the pair it is for.
    if (ask)
      void availabilityOf(source, target).then((availability) => {
        setSaid({ key, availability });
      });
  }, [ask, key, source, target]);
  return ask && said?.key === key ? said.availability : undefined;
}

/**
 * The post `shown` translated by the browser from `source` into `target`: its text, content warning, poll options and
 * alt texts, with the browser's name as the provider. While the browser downloads its model first, `onProgress` hears
 * the share that came. Rejects as the browser does.
 */
async function translateInBrowser(
  shown: PostStatus,
  source: string,
  target: string,
  onProgress: (share: number) => void,
): Promise<PostTranslation> {
  const ready = (await availabilityOf(source, target)) === "available";
  const translator = await openBrowserTranslator(source, target, ready ? undefined : onProgress);
  availabilities.set(pairKey(source, target), Promise.resolve("available"));
  const plain = (text: string) => translatePlain(text, translator.translate);
  const parts = async () => {
    const content = await translateContent(shown.content, translator.translate);
    const spoiler = shown.spoiler_text === "" ? "" : await plain(shown.spoiler_text);
    const options: string[] = [];
    for (const o of shown.poll?.options ?? []) options.push(await plain(o.title));
    const media: { id: string; description: string }[] = [];
    for (const m of shown.media_attachments)
      if (m.description !== undefined && m.description !== "")
        media.push({ id: m.id, description: await plain(m.description) });
    return { content, spoiler, options, media };
  };
  const done = await parts().then(
    (p) => {
      translator.destroy();
      return p;
    },
    (e: unknown) => {
      translator.destroy();
      throw e;
    },
  );
  const name = browserName();
  return {
    language: target,
    source_language: source,
    content: done.content,
    spoiler_text: done.spoiler,
    media_attachments: done.media,
    poll_options: done.options,
    ...(name === undefined ? {} : { provider: { name } }),
  };
}

/** A translation's state on one drawn post. */
type State =
  | { kind: "none" }
  | { kind: "translating"; progress: boolean | number }
  | { kind: "failed"; failure: TranslationFailure }
  | { kind: "shown" | "hidden"; translation: PostTranslation };

/** What the server's refusal says to the reader, by why it refused. */
const refusalReason = {
  limit: "limit",
  visibility: "visibility",
  language: "language",
  failed: "failed",
  denied: "failed",
} as const satisfies Record<TranslationRefusal["reason"], TranslationFailure["reason"]>;

/**
 * Where the server translates a post for a screen that asks it otherwise than the reader's own posts do (a post a report
 * points at, which the moderators ask through the report): whether the server may send it at all, and the request.
 */
export type ServerTranslation = {
  readonly allowed: boolean;
  readonly translate: (id: string, language: string, version?: string) => Promise<StatusTranslation>;
};

/** The props of a post's translation, and the handler of its Translate and Show original. */
export type PostTranslationState = {
  readonly props: Pick<PostProps, "translatable" | "translation" | "translating" | "translationFailure">;
  /** Handles `type` when it is translate or original, and says whether it did. */
  readonly act: (type: string) => boolean;
};

/**
 * The translation of the post that `status` shows (a boost's boosted status), for a reader signed in, or a visitor
 * (`visitor`), whom only the browser translates for; `through`, where the server is asked otherwise (a report's post),
 * whether it may be and how, the browser's translator offered as for any post.
 */
export function usePostTranslation(
  status: PostStatus,
  visitor = false,
  through?: ServerTranslation,
): PostTranslationState {
  const shown = status.boost ?? status;
  const target = readerLanguage(useLocale().locale);
  const source = shown.language;
  const version = shown.edited_at ?? shown.created_at;
  const offer = useInstance()?.translation;
  const ownServer = useTranslateStatus();
  const translateOnServer = through?.translate ?? ownServer;
  // The reader wrote their own post: they are never offered to translate it, whatever language it is marked in.
  const foreign = !usePostOwn()(status) && inAnotherLanguage(source, target);
  const server = !visitor && foreign && (through?.allowed ?? true) && serverTranslates(offer, shown.visibility);
  const browserAllowed = foreign && offer?.browser === true && hasBrowserTranslator();
  const availability = useBrowserAvailability(source, target, browserAllowed);
  const browser = browserAllowed && availability !== undefined && availability !== "unavailable";
  const [state, setState] = useState<{ version: string; state: State }>({ version, state: { kind: "none" } });
  // A post the server refused, whose next press goes to the browser (§12).
  const [refused, setRefused] = useState<{ version: string; reason: TranslationFailure["reason"] } | undefined>();
  const alive = useRef(true);
  useEffect(() => {
    alive.current = true;
    return () => {
      alive.current = false;
    };
  }, []);
  const now = state.version === version ? state.state : { kind: "none" as const };
  const set = (next: State) => {
    if (alive.current) setState({ version, state: next });
  };

  const inBrowser = async (afterServer: TranslationFailure["reason"] | undefined) => {
    set({ kind: "translating", progress: true });
    try {
      const translation = await translateInBrowser(shown, source, target, (share) => {
        set({ kind: "translating", progress: share });
      });
      set({ kind: "shown", translation });
    } catch (e) {
      // The download needs the reader's press: the next press starts it.
      if (needsActivation(e)) set({ kind: "failed", failure: { reason: afterServer ?? "browser", retry: "browser" } });
      else set({ kind: "failed", failure: { reason: "browser", retry: false } });
    }
  };

  const onServer = async () => {
    set({ kind: "translating", progress: true });
    try {
      set({ kind: "shown", translation: await translateOnServer(shown.id, target, version) });
    } catch (e) {
      const why = e instanceof TranslationRefusal ? e.reason : "failed";
      const reason = refusalReason[why];
      const handOver = why !== "denied" && browser;
      if (handOver && (await availabilityOf(source, target)) === "available") {
        if (alive.current) setRefused({ version, reason });
        await inBrowser(reason);
        return;
      }
      if (handOver) {
        if (alive.current) setRefused({ version, reason });
        set({ kind: "failed", failure: { reason, retry: "browser" } });
        return;
      }
      set({ kind: "failed", failure: { reason, retry: why === "failed" ? "server" : false } });
    }
  };

  const act = (type: string): boolean => {
    if (type === "original") {
      if (now.kind === "shown") set({ kind: "hidden", translation: now.translation });
      return true;
    }
    if (type !== "translate") return false;
    if (now.kind === "translating") return true;
    if (now.kind === "hidden") {
      set({ kind: "shown", translation: now.translation });
      return true;
    }
    const after = refused?.version === version ? refused.reason : undefined;
    if (server && after === undefined) void onServer();
    else if (browser) void inBrowser(after);
    return true;
  };

  return {
    props: {
      translatable: foreign && (server || browser),
      translation: now.kind === "shown" ? now.translation : undefined,
      translating: now.kind === "translating" ? now.progress : false,
      translationFailure: now.kind === "failed" ? now.failure : undefined,
    },
    act,
  };
}

/**
 * The groups of another's post's menu that act on its author (mute, block…, report…, the post chosen in the report)
 * and on a remote author's server (mute server), headed by whom they act on; for a signed-in reader only.
 */
function AuthorMenu({ status, author }: { status: PostStatus; author: PostAuthor }) {
  const { t } = stringsIn(useLocale().locale);
  const on = (action: AuthorAction) => () => {
    author.act(action, status);
  };
  const { account } = status;
  return (
    <>
      <MenuGroup title={sentenceHandleOf(account)}>
        <MenuItem id="mute-author" icon="mute" onAction={on("mute")}>
          {t("profile.mute")}
        </MenuItem>
        <MenuItem id="block-author" icon="block" onAction={on("block")}>
          {t("profile.block")}
        </MenuItem>
        <MenuItem
          id="report-author"
          icon="report"
          onAction={() => {
            author.report(status);
          }}
        >
          {t("report.menu")}
        </MenuItem>
      </MenuGroup>
      {account.acct.includes("@") && (
        <MenuGroup title={account.domain}>
          <MenuItem id="mute-server" icon="mute" onAction={on("mute-server")}>
            {t("profile.mute_server")}
          </MenuItem>
        </MenuGroup>
      )}
    </>
  );
}

/** What the reader did from a post's menu, in the post's place, with its undoing. */
function AuthorDoneLine({ id, done, author }: { id: string; done: AuthorDone; author: PostAuthor }) {
  const { t } = stringsIn(useLocale().locale);
  const undo = (
    <TextButton
      onPress={() => {
        author.undo(id);
      }}
    >
      {t(done.action === "block" ? "profile.unblock" : "profile.undo")}
    </TextButton>
  );
  const handle = sentenceHandleOf(done.account);
  // An entry still, in the post's place in its feed.
  return (
    <Entry>
      <CaptionLine icon={done.action === "block" ? "block" : "mute"} action={undo}>
        {done.action === "mute"
          ? t("profile.muted_now", { handle })
          : done.action === "block"
            ? t("profile.blocked_now", { handle })
            : t("profile.server_muted_now", { domain: done.account.domain })}
      </CaptionLine>
    </Entry>
  );
}

/**
 * A post that translates on the reader's request and shows its original again, its translation its own: a `Post`
 * whose translate and original actions it answers itself, every other action going to `onAction`. As the first line of the
 * post itself, after the screen's own line (a thread's heading of its replies), it says when the reader's reply or boost of it waits for an approval (post-approval.tsx).
 * Another's post offers its author's actions in its menu (post-actions.tsx), and once one is done shows what was done
 * in its place. Its link card shows in the size the reader chose, unless the screen names one (`cardSize`).
 */
export function TranslatablePost(props: PostProps) {
  const { props: translation, act } = usePostTranslation(props.status, props.signIn !== undefined);
  const { onAction } = props;
  const approval = <ApprovalLine status={props.status} />;
  const author = usePostAuthor();
  const cardSize = usePostCardSize();
  const shown = props.status.boost ?? props.status;
  const done = author?.done(shown.id);
  if (author !== undefined && done !== undefined) return <AuthorDoneLine id={shown.id} done={done} author={author} />;
  return (
    <Post
      {...props}
      cardSize={props.cardSize ?? cardSize}
      // A visitor, who has no account to mute or block with, is offered none of it.
      authorMenu={
        author === undefined || props.signIn !== undefined ? undefined : <AuthorMenu status={shown} author={author} />
      }
      context={
        props.context === undefined ? (
          approval
        ) : (
          <>
            {props.context}
            {approval}
          </>
        )
      }
      {...translation}
      onAction={(action) => {
        if (!act(action.type)) onAction?.(action);
      }}
    />
  );
}
