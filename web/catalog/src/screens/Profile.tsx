// SPDX-License-Identifier: AGPL-3.0-only

// The rough of a profile (#3047), at /app/profile, in the state `?state=` names: the reader's own (the navigation's
// place), another local account, a remote one, a locked one, one the reader blocks, one they mute. Its head: the
// banner, the avatar over its lower edge with the reader's actions at the other end (follow in its state, notify,
// more), the name with the lock of a locked account and the bot's mark, the handle with "Follows you", the role badges,
// the bio, the fields (FR-006), when it joined and its counts (the API has no count of posts), the featured hashtags,
// and a line of the account's state (remote, locked, muted, blocked). Under it, the tabs of its posts (posts, posts and
// replies, media), the pinned posts first under posts. A blocked account shows its head and no posts.

import { PostContent } from "@my/app-components";
import {
  Avatar,
  Button,
  Feed,
  FeedItem,
  FeedNotice,
  Icon,
  IconButton,
  MenuButton,
  MenuGroup,
  MenuItem,
  TextButton,
  CaptionLine,
} from "@my/design-system";
import { useState } from "react";
import { serverHtml } from "../serverHtml";
import { emojis, type TimelinePost } from "../timeline/fixtures";
import { SamplePost } from "../timeline/SamplePost";
import { profiles, type ProfileSample, type ProfileState } from "./samples";
import { Moves, ScreenPreview, stateOf, useLang } from "./ScreenPreview";
import { Tabs } from "./parts";
import type { Strings } from "./strings";

const states: readonly { id: ProfileState; name: string }[] = [
  { id: "own", name: "Own" },
  { id: "local", name: "Local, follows you" },
  { id: "remote", name: "Remote, following" },
  { id: "locked", name: "Locked" },
  { id: "blocked", name: "Blocked" },
  { id: "muted", name: "Muted" },
];

type Tab = "posts" | "replies" | "media";

const domainOf = (handle: string) => handle.split("@")[2] ?? "";

/** What the reader may do on the profile, at the end of the avatar's row. */
function Actions({ p, state, s }: { p: ProfileSample; state: ProfileState; s: Strings }) {
  const user = `@${p.who.handle.split("@")[1] ?? ""}`;
  const main =
    state === "own" ? (
      <Button size="timeline">{s.editProfile}</Button>
    ) : p.rel.blocking ? (
      <Button size="timeline">{s.unblock}</Button>
    ) : p.rel.following ? (
      <Button size="timeline">{s.following}</Button>
    ) : p.rel.requested ? (
      <Button size="timeline">{s.requested}</Button>
    ) : (
      <Button size="timeline" kind="primary">
        {p.locked ? s.requestFollow : p.rel.followedBy ? s.followBack : s.follow}
      </Button>
    );
  return (
    <div className="sc-head-actions">
      {p.rel.following && <IconButton icon="notifications" label={s.notify} />}
      <MenuButton icon="more" label={s.more}>
        <MenuGroup>
          {state !== "own" && (
            <MenuItem id="mention" icon="direct">
              {s.mention}
            </MenuItem>
          )}
          <MenuItem id="copy" icon="link">
            {s.copyLink}
          </MenuItem>
          {domainOf(p.who.handle) !== "my.example" && (
            <MenuItem id="open" icon="open">
              {s.openOriginal}
            </MenuItem>
          )}
        </MenuGroup>
        {state !== "own" && (
          <>
            <MenuGroup>
              <MenuItem id="note" icon="edit">
                {s.note}
              </MenuItem>
            </MenuGroup>
            <MenuGroup title={user}>
              <MenuItem id="mute" icon="mute" on={p.rel.muting ?? false}>
                {p.rel.muting ? s.unmute : s.mute}
              </MenuItem>
              <MenuItem id="block" icon="block" on={p.rel.blocking ?? false}>
                {p.rel.blocking ? s.unblock : s.block}
              </MenuItem>
              <MenuItem id="report" icon="report">
                {s.report}
              </MenuItem>
            </MenuGroup>
          </>
        )}
      </MenuButton>
      {main}
    </div>
  );
}

function Head({ p, state, s, lang }: { p: ProfileSample; state: ProfileState; s: Strings; lang: "en" | "ja" }) {
  const n = (v: number) => new Intl.NumberFormat(lang).format(v);
  const joined = new Intl.DateTimeFormat(lang, { year: "numeric", month: "long" }).format(p.joined);
  const domain = domainOf(p.who.handle);
  const remote = domain !== "my.example";
  const blocked = p.rel.blocking ?? false;
  return (
    <section className="sc-profile" aria-labelledby="profile-name">
      <div className="sc-banner">{p.banner !== undefined && <img src={p.banner} alt="" />}</div>
      <div className="sc-head-row">
        <span className="sc-profile-avatar">
          <Avatar name={p.who.name} src={p.who.avatar} size="preview" decorative />
        </span>
        <Actions p={p} state={state} s={s} />
      </div>
      <div className="sc-names">
        <h2 id="profile-name" className="sc-profile-name">
          <bdi>{p.who.name}</bdi>
          {p.locked && (
            <span role="img" aria-label={s.locked} className="sc-inline-glyph">
              <Icon name="followers" place="caption" />
            </span>
          )}
          {p.bot && <span className="sc-badge">{s.bot}</span>}
        </h2>
        <p className="sc-handle">
          <bdi dir="ltr">{p.who.handle}</bdi>
          {p.rel.followedBy && <span className="sc-badge">{s.followsYou}</span>}
          {p.badges?.map((b) => (
            <span key={b} className="sc-badge">
              {b}
            </span>
          ))}
        </p>
      </div>
      {blocked ? (
        <CaptionLine icon="block">{s.blocked}</CaptionLine>
      ) : (
        <>
          {p.rel.muting && (
            <CaptionLine icon="mute" action={<TextButton>{s.unmute}</TextButton>}>
              {s.muted}
            </CaptionLine>
          )}
          <PostContent html={serverHtml(p.bio, domain)} emojis={emojis} lang={undefined} />
          {p.fields.length > 0 && (
            <dl className="sc-fields">
              {p.fields.map(([name, value]) => (
                <div key={name}>
                  <dt>{name}</dt>
                  <dd>
                    <PostContent html={serverHtml(value, domain)} emojis={emojis} lang={undefined} />
                  </dd>
                </div>
              ))}
            </dl>
          )}
        </>
      )}
      <p className="sc-meta">
        <span>{s.joined(joined)}</span>
        <a href="#following">
          {s.followingCount} <strong>{n(p.following)}</strong>
        </a>
        <a href="#followers">
          {s.followersCount} <strong>{n(p.followers)}</strong>
        </a>
      </p>
      {p.locked && <CaptionLine icon="followers">{s.locked}</CaptionLine>}
      {remote && (
        <CaptionLine icon="open" action={<TextButton>{s.openOriginal}</TextButton>}>
          {s.remote(domain)}
        </CaptionLine>
      )}
      {!blocked && p.featured.length > 0 && (
        <div className="sc-featured">
          <h3 className="sc-group-heading">{s.featured}</h3>
          <ul>
            {p.featured.map(([tag, count]) => (
              <li key={tag}>
                <a href={`#tag-${tag}`}>
                  #{tag} <span>{s.tagPosts(count)}</span>
                </a>
              </li>
            ))}
          </ul>
        </div>
      )}
    </section>
  );
}

function Posts({ p, tab, s, lang }: { p: ProfileSample; tab: Tab; s: Strings; lang: "en" | "ja" }) {
  const shown = (posts: TimelinePost[]) =>
    tab === "media"
      ? posts.filter((x) => (x.media ?? []).length > 0)
      : tab === "posts"
        ? posts.filter((x) => x.replyTo === undefined)
        : posts;
  const pinned = tab === "media" ? [] : p.pinned;
  const posts = shown(p.posts);
  let position = 0;
  const item = (post: TimelinePost, pin: boolean) => {
    position += 1;
    return (
      <FeedItem key={post.id} id={post.id} position={position}>
        <SamplePost
          post={post}
          lang={lang}
          context={pin ? <CaptionLine icon="pin">{s.pinned}</CaptionLine> : undefined}
        />
      </FeedItem>
    );
  };
  return (
    <Feed
      label={s.tabs}
      count={pinned.length + posts.length}
      more
      busy={false}
      end={p.locked && !p.rel.following ? <FeedNotice>{s.lockedPosts}</FeedNotice> : undefined}
    >
      <Moves />
      {pinned.map((x) => item(x, true))}
      {posts.map((x) => item(x, false))}
    </Feed>
  );
}

export function Profile() {
  const { lang, s } = useLang();
  const state = stateOf(states.map((x) => x.id));
  const p = profiles[state];
  const [tab, setTab] = useState<Tab>("posts");
  return (
    <ScreenPreview
      place={state === "own" ? "/profile" : "/accounts"}
      title={`${p.who.name} (${p.who.handle})`}
      states={states}
      current={state}
    >
      <div className="sc-screen">
        <Head p={p} state={state} s={s} lang={lang} />
        {p.rel.blocking ? (
          <FeedNotice>{s.blockedPosts}</FeedNotice>
        ) : (
          <>
            <Tabs
              label={s.tabs}
              tabs={[
                { id: "posts", name: s.posts },
                { id: "replies", name: s.postsAndReplies },
                { id: "media", name: s.media },
              ]}
              current={tab}
              onChange={setTab}
            />
            <Posts p={p} tab={tab} s={s} lang={lang} />
          </>
        )}
      </div>
    </ScreenPreview>
  );
}
