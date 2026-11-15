// SPDX-License-Identifier: AGPL-3.0-only

// The direct messages (FR-212, ADR-0154 §6, the pick conversation-list of #3090): the reader's conversations, newest
// first, each an entry of the people's faces and names, the unread mark, the time and more, over the latest post's
// first lines with its author before it (the reader's "You", another's name in a group). A press on the entry, or on
// its time, opens the conversation; more hides it, which takes it out of the list until a new post arrives in it, as a
// line at the top says (there is no undo: the server has none). New posts move a conversation to the top as the
// stream brings them (@my/app-data).
//
// A conversation: its head under the back button (the people, and more to hide it), then its direct posts, oldest
// first, one exchange closer together than a timeline, each as every screen shows a post, the reader's marked as
// theirs and none saying whom it replies to; then the reply in place, which the app places (`reply`). Opening it marks
// it read on every device, and so does a post arriving while it is open.

import { AccountName, FeedKeys, ListColumn, nameOf, PostContent } from "@my/app-components";
import {
  conversationQuery,
  conversationsQuery,
  useConversation,
  useConversationPosts,
  useConversations,
  useConversationWrites,
  type AppData,
  type Conversation,
  type OneConversation,
  type Status,
} from "@my/app-data";
import {
  Avatar,
  BackButton,
  ConversationAuthor,
  ConversationHead,
  ConversationLine,
  ConversationText,
  Entry,
  Feed,
  FeedItem,
  FeedNotice,
  MenuButton,
  MenuItem,
  ScreenStack,
  TextButton,
  useLocale,
} from "@my/design-system";
import { Fragment, useEffect, useId, useRef, useState, type ReactNode } from "react";
import { stringsIn } from "../locale";
import { usePostActions, usePostNotice, usePostOwn, usePostPicker, usePostVoting } from "../post-actions";
import { TranslatablePost } from "../post-translation";
import { formatParts } from "../slots";

type T = ReturnType<typeof stringsIn>["t"];

/** The app's address of the conversation of `id`. */
export const conversationPath = (id: string) => `/conversations/${encodeURIComponent(id)}`;

/** What the list's loader waits for: the first page of the conversations, from the cache unless marked stale. */
export async function loadConversations(data: AppData): Promise<void> {
  await data.queryClient.infiniteQuery({ ...conversationsQuery(data), staleTime: Infinity }).catch(() => undefined);
}

/** What a conversation's loader waits for: the conversation, from the list when it holds it. */
export async function loadConversation(data: AppData, id: string): Promise<void> {
  await data.queryClient.query({ ...conversationQuery(data, id), staleTime: Infinity }).catch(() => undefined);
}

/** An account's name as plain text isolated in its own direction (FSI … PDI), as a <bdi> isolates it on the page. */
const isolated = (account: Conversation["accounts"][number]) => `⁨${nameOf(account)}⁩`;

/** A conversation's other accounts as the reader's locale lists them, each name isolated, and as plain text. */
function namesOf(accounts: Conversation["accounts"], t: T, lang: string): { node: ReactNode; text: string } {
  if (accounts.length > 3) {
    const [first, second] = accounts;
    const count = accounts.length - 2;
    const values = { first: first ? isolated(first) : "", second: second ? isolated(second) : "", count };
    return {
      node: formatParts(
        t,
        "conversations.names_more",
        { count },
        {
          first: <bdi>{first && <AccountName account={first} />}</bdi>,
          second: <bdi>{second && <AccountName account={second} />}</bdi>,
        },
      ),
      text: t("conversations.names_more", values),
    };
  }
  // Each name stands in the list as a mark of its index, so that the locale may order them as it lists.
  const format = new Intl.ListFormat(lang, { type: "conjunction" });
  const marks = accounts.map((_, i) => `\u{E000}${String(i)}\u{E000}`);
  return {
    node: format
      .format(marks)
      .split(/\u{E000}(\d+)\u{E000}/u)
      .map((piece, i) => {
        if (i % 2 === 0) return <Fragment key={i}>{piece}</Fragment>;
        const account = accounts[Number(piece)];
        return <bdi key={i}>{account && <AccountName account={account} />}</bdi>;
      }),
    text: format.format(accounts.map(isolated)),
  };
}

/** The first three of the other accounts' faces, at a row's size. */
const facesOf = (accounts: Conversation["accounts"]) =>
  accounts
    .slice(0, 3)
    .map((a) => <Avatar key={a.id} name={nameOf(a)} src={a.avatar?.small.url} size="row" decorative />);

/** More of a conversation: hiding it. */
function More({ t, onHide }: { t: T; onHide: () => void }) {
  return (
    <MenuButton icon="more" label={t("conversations.more")}>
      <MenuItem id="hide" icon="hidden" onAction={onHide}>
        {t("conversations.hide")}
      </MenuItem>
    </MenuButton>
  );
}

/** Who wrote the latest post, before its excerpt: the reader, or in a group another; none in a conversation of two. */
function authorOf(c: Conversation, own: boolean, t: T): string | undefined {
  if (own) return t("conversations.you");
  if (c.accounts.length < 2) return undefined;
  return nameOf(c.last_status.account);
}

/** A conversation of the list. */
function Row({ c, t, lang, onHide }: { c: Conversation; t: T; lang: string; onHide: () => void }) {
  const id = useId();
  const ownOf = usePostOwn();
  const names = namesOf(c.accounts, t, lang);
  const status = c.last_status;
  const language = status.language === "und" ? undefined : status.language;
  const author = authorOf(c, ownOf(status), t);
  const text = (
    <ConversationText lang={language}>
      <PostContent html={status.content} emojis={status.emojis} lang={language} />
    </ConversationText>
  );
  return (
    <Entry labelledBy={id}>
      <ConversationLine
        faces={facesOf(c.accounts)}
        names={names.node}
        namesId={id}
        date={status.created_at}
        href={conversationPath(c.id)}
        unread={c.unread ? t("conversations.unread") : undefined}
        more={<More t={t} onHide={onHide} />}
      >
        {author === undefined
          ? text
          : formatParts(
              t,
              "conversations.excerpt",
              {},
              { author: <ConversationAuthor>{author}</ConversationAuthor>, text },
            )}
      </ConversationLine>
    </Entry>
  );
}

/**
 * The reader's conversations, newest first; `onOpen` opens one. A conversation hidden leaves the list, and a line at
 * its top says it comes back with a new post.
 */
export function ConversationList({ onOpen }: { onOpen: (path: string) => void }) {
  const { t, lang } = stringsIn(useLocale().locale);
  const { items, ...list } = useConversations();
  const { hide } = useConversationWrites();
  const failed = usePostNotice();
  const [hidden, setHidden] = useState<string | undefined>(undefined);
  const hideOne = (c: Conversation) => {
    const names = namesOf(c.accounts, t, lang).text;
    setHidden(undefined);
    hide(c.id).then(
      () => {
        setHidden(names);
      },
      (e: unknown) => {
        failed(e, t("conversations.hide_failed"), () => {
          hideOne(c);
        });
      },
    );
  };
  return (
    <ScreenStack>
      {hidden !== undefined && <FeedNotice start>{t("conversations.hidden", { names: hidden })}</FeedNotice>}
      <ListColumn
        {...list}
        label={t("conversations.label")}
        items={items}
        keyOf={(c) => c.id}
        words={{
          loading: t("conversations.loading"),
          loadingMore: t("conversations.loading_more"),
          failed: t("conversations.failed"),
          failedMore: t("conversations.failed_more"),
          empty: t("conversations.empty"),
        }}
        onEntryClick={(id) => {
          onOpen(conversationPath(id));
        }}
      >
        {(c) => (
          <Row
            c={c}
            t={t}
            lang={lang}
            onHide={() => {
              hideOne(c);
            }}
          />
        )}
      </ListColumn>
    </ScreenStack>
  );
}

/** What the app places at a conversation's end: the reply in place, to its latest post and its people. */
export type ConversationReplyTo = {
  /** The ID of the conversation's latest post. */
  readonly to: string;
  /** The handles (`acct`) of its other accounts. */
  readonly people: readonly string[];
  /** Their names as the head lists them, as plain text. */
  readonly names: string;
};

/** What a conversation's screen says until the conversation is read: that it loads, failed, or is none. */
function ConversationNotice({ status, refetch, t }: { status: OneConversation["status"]; refetch: () => void; t: T }) {
  if (status === "gone") return <FeedNotice>{t("conversations.gone")}</FeedNotice>;
  if (status === "error")
    return (
      <FeedNotice alert action={<TextButton onPress={refetch}>{t("conversations.retry")}</TextButton>}>
        {t("conversations.conversation_failed")}
      </FeedNotice>
    );
  return <FeedNotice>{t("conversations.conversation_loading")}</FeedNotice>;
}

/** The posts of a conversation, oldest first, one exchange. */
function Posts({ c, label, t }: { c: Conversation; label: string; t: T }) {
  const { posts, status, refetch } = useConversationPosts(c);
  const onAction = usePostActions();
  const votingOf = usePostVoting();
  const ownOf = usePostOwn();
  const picker = usePostPicker();
  const you = t("conversations.you");
  const item = (s: Status, i: number) => (
    <FeedItem key={s.id} id={s.id} position={i + 1}>
      <TranslatablePost
        status={s}
        onAction={onAction}
        voting={votingOf(s)}
        own={ownOf(s)}
        picker={picker}
        self={you}
        replyingTo={false}
      />
    </FeedItem>
  );
  return (
    <Feed
      label={label}
      count={posts.length}
      more={false}
      busy={status === "pending"}
      tight
      end={
        status === "error" ? (
          <FeedNotice alert action={<TextButton onPress={refetch}>{t("conversations.retry")}</TextButton>}>
            {t("conversations.posts_failed")}
          </FeedNotice>
        ) : status === "pending" && posts.length === 0 ? (
          <FeedNotice>{t("conversations.conversation_loading")}</FeedNotice>
        ) : undefined
      }
    >
      <FeedKeys />
      {posts.map(item)}
    </Feed>
  );
}

/**
 * The reader's conversation of `id`: `onBack` goes back to the list, `onHidden` leaves it once hidden, and `reply`
 * draws the reply in place at its end.
 */
export function ConversationView({
  id,
  onBack,
  onHidden,
  reply,
}: {
  id: string;
  onBack: () => void;
  onHidden: () => void;
  reply: (to: ConversationReplyTo) => ReactNode;
}) {
  const { t, lang } = stringsIn(useLocale().locale);
  const { conversation: c, status, refetch } = useConversation(id);
  const { read, hide } = useConversationWrites();
  const failed = usePostNotice();
  const unread = c?.unread === true;
  const last = c?.last_status.id;
  // Opened, or a post arrived while it is open: the reader reads it, on every device. Once for each latest post: a read
  // that failed shows it unread again, and is tried again only with a newer post, or when the reader opens it again.
  const tried = useRef<string | undefined>(undefined);
  useEffect(() => {
    if (!unread || last === undefined || tried.current === last) return;
    tried.current = last;
    read(id).catch(() => undefined);
  }, [unread, last, id, read]);
  const back = <BackButton onPress={onBack}>{t("conversations.back")}</BackButton>;
  if (c === undefined)
    return (
      <ScreenStack>
        {back}
        <ConversationNotice status={status} refetch={refetch} t={t} />
      </ScreenStack>
    );
  const names = namesOf(c.accounts, t, lang);
  const hideIt = () => {
    hide(c.id).then(onHidden, (e: unknown) => {
      failed(e, t("conversations.hide_failed"), hideIt);
    });
  };
  return (
    <ScreenStack>
      <ConversationHead back={back} faces={facesOf(c.accounts)} more={<More t={t} onHide={hideIt} />}>
        {names.node}
      </ConversationHead>
      <Posts c={c} label={t("conversations.with", { names: names.text })} t={t} />
      {reply({ to: c.last_status.id, people: c.accounts.map((a) => a.acct), names: names.text })}
    </ScreenStack>
  );
}
