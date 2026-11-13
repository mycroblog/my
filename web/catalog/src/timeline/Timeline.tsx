// SPDX-License-Identifier: AGPL-3.0-only

// The home timeline screen, composed roughly from the built parts and the rough ones (#3005), so that the decider
// judges the parts together as one screen: /timeline on the catalog's dev server. The shell (timeline.css) holds the
// compose entry and the navigation (home, notifications, search, profile, settings), which shows where the reader is,
// so the page has no visible header; the column holds the sample posts; on wide screens a right column follows. The
// locale and the direction are the page's (main.tsx), from ?lang=, which the overlay's locale switch sets; its own
// words are in ja for ja and in en for any other locale (ar-XB shows it right-to-left). The scheme is the page's.

import "./timeline.css";
// The wordmark, not the app icon: a mark, so that the compose entry under it is the only coral button.
import wordmarkDark from "../../../brand/Wordmark-DarkBg.svg";
import wordmarkLight from "../../../brand/Wordmark-Primary-Color.svg";
import { Avatar, Button, Icon, RelativeTime, useLocale, type IconName } from "@my/design-system";
import { text } from "@my/design-tokens";
import type { Lang } from "../Posts";
import { ago } from "../Times";
import { announcements, emoji, me, notifications, openRoom, posts, server, type NotificationKind } from "./fixtures";
import { Column } from "./Column";
import { strings } from "./strings";

const places = ["home", "notifications", "search", "profile", "settings"] as const satisfies IconName[];

/** The places, each a glyph and its label; home, where the reader is, is on. */
function NavList({ lang }: { lang: Lang }) {
  const s = strings[lang];
  return (
    <ul className="tl-nav-list">
      {places.map((place) => {
        const current = place === "home";
        return (
          <li key={place}>
            <a
              className="tl-nav-item"
              href={`#${place}`}
              aria-current={current ? "page" : undefined}
              aria-label={s[place]}
              title={s[place]}
            >
              <Icon name={place} place="action" on={current} />
              <span className="tl-label" aria-hidden="true">
                {s[place]}
              </span>
            </a>
          </li>
        );
      })}
    </ul>
  );
}

/** The compose entry: the primary action, a glyph and, where there is room, its label. */
const Compose = ({ lang }: { lang: Lang }) => (
  <Button kind="primary" aria-label={strings[lang].compose}>
    <Icon name="compose" place="action" />
    <span className="tl-label" aria-hidden="true">
      {strings[lang].compose}
    </span>
  </Button>
);

// The glyph of each kind of notification; a reaction shows its emoji instead.
const notificationGlyph: Record<Exclude<NotificationKind, "reaction">, IconName> = {
  favourite: "favourite",
  boost: "boost",
  follow: "profile",
  mention: "direct",
  quote: "quote",
};

/** A reaction's emoji, Unicode or custom, at the size of a caption glyph. */
function EmojiGlyph({ value }: { value: string }) {
  const src = emoji[value.slice(1, -1)];
  return src && value.startsWith(":") ? (
    <img src={src} alt={value} style={{ height: "1.1em", width: "1.1em" }} />
  ) : (
    <span>{value}</span>
  );
}

/**
 * A section's heading: the glyph of its place, the navigation's where it has one, and its name, as a link to that
 * place, as the navigation's item is.
 */
const Heading = ({ id, icon, href, children }: { id: string; icon: IconName; href: string; children: string }) => (
  <h2 id={id}>
    <a className="tl-heading-link" href={href}>
      <Icon name={icon} place="caption" />
      {children}
    </a>
  </h2>
);

/**
 * The right column, on wide screens only, all rough, for the decider to redesign later. From the top: the voice chat
 * room open now that the reader has not joined (when there is one), the unread announcements (when there are any),
 * the latest notifications, and the server's own section, each on the page and apart by space, under a heading that
 * links to its place: the room, the announcements, the notifications, and /about.
 */
function Aside({ lang }: { lang: Lang }) {
  const s = strings[lang];
  return (
    <aside className="tl-aside" aria-label={s.aside}>
      {openRoom && (
        <section aria-labelledby="tl-room">
          <Heading id="tl-room" icon="room" href="#room">
            {s.room}
          </Heading>
          <p className="tl-aside-line">
            <Avatar name={openRoom.host.name} src={openRoom.host.avatar} size="inline" decorative />
            <span>{s.roomStarted(openRoom.host.name)}</span>
          </p>
          <p className="tl-aside-title">{openRoom.title[lang]}</p>
          <div className="tl-aside-actions">
            <span className="tl-muted">{s.roomPeople(openRoom.people)}</span>
            <Button kind="secondary" size="timeline">
              {s.join}
            </Button>
          </div>
        </section>
      )}
      {announcements.length > 0 && (
        <section aria-labelledby="tl-announcements">
          <Heading id="tl-announcements" icon="announcements" href="#announcements">
            {s.announcements}
          </Heading>
          {announcements.map((a) => (
            <div key={a.title.en} className="tl-announcement">
              <p className="tl-aside-title">{a.title[lang]}</p>
              <p>{a.body[lang]}</p>
              <div className="tl-aside-actions">
                <span className="tl-muted">
                  <RelativeTime date={ago(a.ago)} />
                </span>
                <Button kind="quiet" size="timeline">
                  {s.markRead}
                </Button>
              </div>
            </div>
          ))}
        </section>
      )}
      <section aria-labelledby="tl-notifications">
        <Heading id="tl-notifications" icon="notifications" href="#notifications">
          {s.notifications}
        </Heading>
        <ul className="tl-notifications">
          {notifications.map((n, i) => (
            <li key={i}>
              <span className="tl-muted tl-notification-glyph">
                {n.kind === "reaction" ? (
                  <EmojiGlyph value={n.emoji ?? ""} />
                ) : (
                  <Icon name={notificationGlyph[n.kind]} place="caption" />
                )}
              </span>
              <Avatar name={n.who.name} src={n.who.avatar} size="inline" decorative />
              <span className="tl-notification-text">{s.notified[n.kind](n.who.name)}</span>
              <span className="tl-muted">
                <RelativeTime date={ago(n.ago)} />
              </span>
            </li>
          ))}
        </ul>
      </section>
      <section aria-labelledby="tl-server">
        <Heading id="tl-server" icon="about" href="/about">
          {s.about}
        </Heading>
        <p className="tl-aside-title">{server.name}</p>
        <p>{server.description[lang]}</p>
        <p className="tl-muted">{s.accounts(new Intl.NumberFormat(lang).format(server.accounts))}</p>
      </section>
    </aside>
  );
}

export function Timeline() {
  const lang: Lang = new Intl.Locale(useLocale().locale).language === "ja" ? "ja" : "en";
  const s = strings[lang];
  document.title = s.timeline;
  return (
    <div className="tl-screen">
      <nav className="tl-side" aria-label={s.nav}>
        <a className="tl-brand" href="#home" aria-label="mycroblog">
          <img className="tl-mark-light" src={wordmarkLight} alt="" />
          <img className="tl-mark-dark" src={wordmarkDark} alt="" />
        </a>
        <div className="tl-compose">
          <Compose lang={lang} />
        </div>
        <NavList lang={lang} />
        <div className="tl-account">
          <Avatar name={me.name} size="post" decorative />
          <span className="tl-account-name" style={{ fontSize: `${text.body.size}rem` }}>
            <span className="tl-label" style={{ fontWeight: 700 }}>
              {me.name}
            </span>
            <span
              className="tl-label"
              style={{ color: "var(--color-muted-text)", fontSize: `${text.caption.size}rem` }}
            >
              <bdi dir="ltr">{me.handle}</bdi>
            </span>
          </span>
        </div>
      </nav>
      <main className="tl-main">
        <h1 className="tl-visually-hidden">{s.home}</h1>
        <Column posts={posts} lang={lang} label={s.timeline} />
      </main>
      <Aside lang={lang} />
      <nav className="tl-tabbar" aria-label={s.nav}>
        <NavList lang={lang} />
      </nav>
      <div className="tl-fab">
        <Compose lang={lang} />
      </div>
    </div>
  );
}
