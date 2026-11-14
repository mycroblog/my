// SPDX-License-Identifier: AGPL-3.0-only

// The rough of the notifications (#3047), at /app/notifications: the reader's notifications of the kinds they meet
// (FR-205, and the server's: a poll's end, the moderators' notice, a report resolved, an announcement), newest first,
// one entry each, since no decision groups them. Over them, the filters (all, or the mentions alone: the API's
// `types`) with mark-all-read and the settings at the row's end, then the row of the requests the notification policy
// holds back (ADR-0158 §7). Each unread notification (newer than the read marker) carries its own mark, a dot in the
// text's color, on the line of its kind, which then reads in the text's color; a read one's line is muted. A
// notification about a post that speaks to the reader (a mention, a reply, a quote, a new post of an account they
// subscribe to) shows the post whole, the caption of its kind inside it (the post's `context`); the others are a line
// of their kind (its glyph, the account's avatar, the sentence of its Web Push notification, the time), with the
// reader's post as its first words, or the account and what the reader may do. The affirmative action of a
// notification that asks for a response (follow back, follow, accept, approve, read) is the primary button.

import {
  Avatar,
  Button,
  CaptionLine,
  Entry,
  Feed,
  FeedItem,
  Icon,
  IconButton,
  Stamp,
  useAction,
  type IconName,
} from "@my/design-system";
import { useId, useState, type ReactNode } from "react";
import { SamplePost } from "../timeline/SamplePost";
import { held, notices, type Notice } from "./samples";
import { Moves, ScreenPreview, useLang } from "./ScreenPreview";
import { AccountRow, Sentence, Tabs } from "./parts";
import type { Strings } from "./strings";

const instance = "my.example";

const glyphs: Record<Notice["type"], IconName> = {
  mention: "reply",
  favourite: "favourite",
  boost: "boost",
  reaction: "react",
  quote: "quote",
  follow: "follow",
  follow_request: "followers",
  follow_accepted: "follow",
  status: "notifications",
  quote_request: "quote",
  account_moved: "open",
  poll_closed: "poll",
  moderation_notice: "moderation",
  report_resolved: "moderation",
  announcement: "announcements",
};

const ago = (ms: number) => new Date(Date.now() - ms);

/** The mark of an unread notification: a dot in the text's color, by its shape alone, with no hue. */
function Unread({ s }: { s: Strings }) {
  return <span role="img" aria-label={s.unread} className="sc-unread" />;
}

/**
 * The line of a notification's kind: its glyph (a reaction's emoji), the account's avatar, the sentence, the unread
 * mark and the time.
 */
function KindLine({
  glyph,
  emoji,
  avatar,
  time,
  id,
  unread,
  s,
  children,
}: {
  glyph: IconName;
  emoji?: string | undefined;
  avatar?: ReactNode;
  time: number;
  id: string;
  unread: boolean;
  s: Strings;
  children: ReactNode;
}) {
  return (
    <div className="sc-kind" data-read={unread ? undefined : ""}>
      <span className="sc-kind-glyph">{emoji ?? <Icon name={glyph} place="action" />}</span>
      {avatar}
      <p id={id} className="sc-kind-text">
        {children}
      </p>
      {unread && <Unread s={s} />}
      <Stamp date={ago(time)} />
    </div>
  );
}

type RowProps<K extends Notice["kind"]> = { n: Extract<Notice, { kind: K }>; unread: boolean; s: Strings };

const primary = (label: string) => (
  <Button size="timeline" kind="primary">
    {label}
  </Button>
);

function Act({ n, unread, s }: RowProps<"act">) {
  const name = useId();
  const avatar = <Avatar name={n.who.name} src={n.who.avatar} size="inline" decorative />;
  return (
    <Entry labelledBy={name}>
      <KindLine glyph={glyphs[n.type]} emoji={n.emoji} avatar={avatar} time={n.ago} id={name} unread={unread} s={s}>
        <Sentence template={s.notified[n.type]} who={n.who} />
      </KindLine>
      {n.about !== undefined && <p className="sc-about">{n.about}</p>}
      {n.type === "follow" && <AccountRow who={n.who} end={primary(s.followBack)} />}
      {n.type === "follow_request" && (
        <AccountRow
          who={n.who}
          end={
            <>
              <Button size="timeline">{s.reject}</Button>
              {primary(s.accept)}
            </>
          }
        />
      )}
      {n.type === "quote_request" && (
        <div className="sc-pair">
          <Button size="timeline">{s.decline}</Button>
          {primary(s.approve)}
        </div>
      )}
      {n.type === "account_moved" && n.to !== undefined && <AccountRow who={n.to} end={primary(s.follow)} />}
    </Entry>
  );
}

function Server({ n, unread, s }: RowProps<"server">) {
  const name = useId();
  const text =
    n.type === "poll_closed"
      ? s.pollClosed
      : n.type === "moderation_notice"
        ? s.moderationNotice(instance)
        : n.type === "report_resolved"
          ? s.reportResolved(instance)
          : s.announcement(instance, n.title ?? "");
  return (
    <Entry labelledBy={name}>
      <KindLine glyph={glyphs[n.type]} time={n.ago} id={name} unread={unread} s={s}>
        {text}
      </KindLine>
      {n.body !== undefined && <p className="sc-about">{n.body}</p>}
      {(n.type === "moderation_notice" || n.type === "announcement") && (
        <div className="sc-pair">{primary(s.read)}</div>
      )}
    </Entry>
  );
}

/** A notification about a post: the post, its kind in a caption line inside it, the unread mark at that line's end. */
function OnPost({ n, unread, s, lang }: RowProps<"post"> & { lang: "en" | "ja" }) {
  const kind = (
    <div className="sc-kind-caption" data-read={unread ? undefined : ""}>
      <CaptionLine icon={glyphs[n.type]}>
        <Sentence template={s.notified[n.type]} who={n.who} />
      </CaptionLine>
      {unread && <Unread s={s} />}
    </div>
  );
  return <SamplePost post={n.post} lang={lang} context={kind} />;
}

function NotificationRow({ n, unread, s, lang }: { n: Notice; unread: boolean; s: Strings; lang: "en" | "ja" }) {
  if (n.kind === "act") return <Act n={n} unread={unread} s={s} />;
  if (n.kind === "server") return <Server n={n} unread={unread} s={s} />;
  return <OnPost n={n} unread={unread} s={s} lang={lang} />;
}

function Requests({ s }: { s: Strings }) {
  return (
    <a className="sc-requests" href="#requests">
      <Icon name="followers" place="action" />
      <span>
        <span className="sc-requests-title">{s.requests(held.people, held.notifications)}</span>
        <span className="sc-requests-hint">{s.requestsHint}</span>
      </span>
      <span className="sc-chevron" aria-hidden="true">
        ›
      </span>
    </a>
  );
}

/** Marks every notification read: a screen action, which the keymap declares with no key yet. */
function ReadAll({ s, onPress }: { s: Strings; onPress: () => void }) {
  const keys = useAction("notifications.read_all", onPress, { scope: "screen", label: s.readAll });
  return <IconButton icon="read" label={s.readAll} onPress={onPress} {...keys} />;
}

function List({ s, lang }: { s: Strings; lang: "en" | "ja" }) {
  const [filter, setFilter] = useState<"all" | "mentions">("all");
  const [allRead, setAllRead] = useState(false);
  const shown = notices.filter((n) => filter === "all" || n.type === "mention" || n.type === "quote");
  return (
    <div className="sc-screen">
      <Tabs
        label={s.filters}
        tabs={[
          { id: "all", name: s.all },
          { id: "mentions", name: s.mentions },
        ]}
        current={filter}
        onChange={setFilter}
        end={
          <>
            <ReadAll
              s={s}
              onPress={() => {
                setAllRead(true);
              }}
            />
            <IconButton icon="settings" label={s.settings} />
          </>
        }
      />
      {filter === "all" && <Requests s={s} />}
      <Feed label={s.notifications} count={shown.length} more busy={false}>
        <Moves />
        {shown.map((n, i) => (
          <FeedItem key={n.id} id={n.id} position={i + 1}>
            <NotificationRow n={n} unread={n.unread && !allRead} s={s} lang={lang} />
          </FeedItem>
        ))}
      </Feed>
    </div>
  );
}

export function Notifications() {
  const { lang, s } = useLang();
  return (
    <ScreenPreview place="/notifications" title={s.notifications}>
      <List s={s} lang={lang} />
    </ScreenPreview>
  );
}
