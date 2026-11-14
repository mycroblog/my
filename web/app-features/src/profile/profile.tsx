// SPDX-License-Identifier: AGPL-3.0-only

// A profile (FR-006, the rough of #3047): the account of a handle, from useAccount() of @my/app-data, in its state
// for the reader: their own, another local account, a remote one, a locked one, one they block or mute. Its head: the
// banner, the avatar with the reader's actions at the other end of its row (follow in its state, more), the name with a
// locked account's lock and a bot's badge, the handle with "Follows you" and the role badges, the bio, the fields, when
// it joined and its counts, the lines of its state (blocked, muted, locked, remote), and the featured hashtags. Under
// it, the tabs of its posts (posts, posts and replies, media), the pinned posts first under the first two. A blocked
// account shows its head and no posts. A post's actions are answered as on every screen (post-actions.tsx).

import { AccountName, handleOf, ListColumn, nameOf, Post, PostContent } from "@my/app-components";
import {
  accountQuery,
  useAccount,
  useAccountStatuses,
  useChangeRelationship,
  useFeaturedHashtags,
  useRelationship,
  useViewer,
  viewerQuery,
  type Account,
  type AccountPosts,
  type AppData,
  type Relationship,
  type RelationshipChange,
  type Status,
} from "@my/app-data";
import {
  Avatar,
  Badge,
  Button,
  CaptionLine,
  FeedNotice,
  FieldList,
  Icon,
  MenuButton,
  MenuGroup,
  MenuItem,
  MetaCount,
  MetaLine,
  ProfileHead,
  ScreenStack,
  TabRow,
  TagList,
  TextButton,
  useLocale,
} from "@my/design-system";
import { useId, useState } from "react";
import { stringsIn } from "../locale";
import { usePostActions } from "../post-actions";

type T = ReturnType<typeof stringsIn>["t"];

type Tab = Exclude<AccountPosts, "pinned">;

/**
 * What the route's loader waits for: the account of the handle and the viewer's own, from the cache unless a write
 * marked them stale. A failure is left to the screen.
 */
export async function loadProfile(data: AppData, acct: string): Promise<void> {
  const { queryClient } = data;
  await Promise.all([
    queryClient.query({ ...accountQuery(data, acct), staleTime: Infinity }),
    queryClient.query({ ...viewerQuery(data), staleTime: Infinity }),
  ]).catch(() => undefined);
}

/** What the loader of the navigation's profile waits for: the viewer's own account, then its profile. */
export async function loadOwnProfile(data: AppData): Promise<void> {
  const viewer = await data.queryClient.query({ ...viewerQuery(data), staleTime: Infinity }).catch(() => undefined);
  if (viewer !== undefined) await loadProfile(data, viewer.acct);
}

/**
 * The title of the profile of a handle (the viewer's own without one), once its account is known: its name and its
 * handle.
 */
export function useProfileTitle(acct: string | undefined): string | undefined {
  const { t } = stringsIn(useLocale().locale);
  const viewer = useViewer();
  const { account } = useAccount(acct ?? viewer?.acct);
  return account === undefined ? undefined : t("profile.title", { name: nameOf(account), handle: handleOf(account) });
}

/** The viewer's own profile, which the navigation's profile opens. */
export function OwnProfile() {
  const { t } = stringsIn(useLocale().locale);
  const viewer = useViewer();
  return viewer === undefined ? <FeedNotice>{t("profile.loading")}</FeedNotice> : <Profile acct={viewer.acct} />;
}

/** A press that waits for the server, busy until it answered. */
function usePress(): [boolean, (run: () => Promise<void>) => void] {
  const [busy, setBusy] = useState(false);
  return [
    busy,
    (run) => {
      setBusy(true);
      run()
        .catch(() => undefined)
        .finally(() => {
          setBusy(false);
        });
    },
  ];
}

/** Opens the account's page on its own server, in a new tab. */
const openOriginal = (account: Account) => () => {
  window.open(account.url, "_blank", "noopener");
};

/** The main action of another account's profile: the follow in its state, or unblock. */
function Follow({ account, relationship, t }: { account: Account; relationship: Relationship; t: T }) {
  const change = useChangeRelationship();
  const [busy, press] = usePress();
  const on = (c: RelationshipChange) => () => {
    press(() => change(account.id, c));
  };
  if (relationship.blocking)
    return (
      <Button size="timeline" busy={busy} onPress={on("unblock")}>
        {t("profile.unblock")}
      </Button>
    );
  if (relationship.following || relationship.requested)
    return (
      <Button size="timeline" busy={busy} onPress={on("unfollow")}>
        {t(relationship.following ? "profile.following" : "profile.requested")}
      </Button>
    );
  return (
    <Button size="timeline" kind="primary" busy={busy} onPress={on("follow")}>
      {t(
        account.locked ? "profile.request_follow" : relationship.followed_by ? "profile.follow_back" : "profile.follow",
      )}
    </Button>
  );
}

/** The menu of what else the reader may do: open the original page of a remote account, mute and block. */
function More({ account, relationship, t }: { account: Account; relationship: Relationship; t: T }) {
  const change = useChangeRelationship();
  const muting = relationship.muting ?? false;
  const blocking = relationship.blocking ?? false;
  const remote = account.acct.includes("@");
  return (
    <MenuButton icon="more" label={t("profile.more")}>
      {remote && (
        <MenuGroup>
          <MenuItem id="open" icon="open" onAction={openOriginal(account)}>
            {t("profile.open_original")}
          </MenuItem>
        </MenuGroup>
      )}
      <MenuGroup title={`@${account.username || account.domain}`}>
        <MenuItem
          id="mute"
          icon="mute"
          on={muting}
          onAction={() => {
            void change(account.id, muting ? "unmute" : "mute").catch(() => undefined);
          }}
        >
          {t(muting ? "profile.unmute" : "profile.mute")}
        </MenuItem>
        <MenuItem
          id="block"
          icon="block"
          on={blocking}
          onAction={() => {
            void change(account.id, blocking ? "unblock" : "block").catch(() => undefined);
          }}
        >
          {t(blocking ? "profile.unblock" : "profile.block")}
        </MenuItem>
      </MenuGroup>
    </MenuButton>
  );
}

function Head({
  account,
  relationship,
  own,
  t,
  lang,
}: {
  account: Account;
  relationship: Relationship | undefined;
  own: boolean;
  t: T;
  lang: string;
}) {
  const nameId = useId();
  const change = useChangeRelationship();
  const featured = useFeaturedHashtags(account.id);
  const n = (v: number) => new Intl.NumberFormat(lang).format(v);
  const joined = new Intl.DateTimeFormat(lang, { year: "numeric", month: "long" }).format(new Date(account.created_at));
  const blocked = relationship?.blocking ?? false;
  const remote = account.acct.includes("@");
  const avatar = account.avatar?.medium.url ?? account.avatar?.small.url;
  return (
    <ProfileHead
      nameId={nameId}
      banner={account.header?.medium.url}
      avatar={<Avatar name={nameOf(account)} src={avatar} size="preview" decorative />}
      actions={
        !own &&
        relationship !== undefined && (
          <>
            <More account={account} relationship={relationship} t={t} />
            <Follow account={account} relationship={relationship} t={t} />
          </>
        )
      }
      name={<AccountName account={account} />}
      marks={
        <>
          {account.locked && (
            <span role="img" aria-label={t("profile.locked")}>
              <Icon name="followers" place="caption" />
            </span>
          )}
          {account.bot && <Badge>{t("profile.bot")}</Badge>}
        </>
      }
      handle={handleOf(account)}
      badges={
        <>
          {relationship?.followed_by && <Badge>{t("profile.follows_you")}</Badge>}
          {account.badges.map((b) => (
            <Badge key={b.role_id}>{b.label}</Badge>
          ))}
        </>
      }
    >
      {blocked ? (
        <CaptionLine icon="block">{t("profile.blocked")}</CaptionLine>
      ) : (
        <>
          {relationship?.muting && (
            <CaptionLine
              icon="mute"
              action={
                <TextButton
                  onPress={() => {
                    void change(account.id, "unmute").catch(() => undefined);
                  }}
                >
                  {t("profile.unmute")}
                </TextButton>
              }
            >
              {t("profile.muted")}
            </CaptionLine>
          )}
          {account.note !== "" && <PostContent html={account.note} emojis={account.emojis} lang={undefined} />}
          {account.fields.length > 0 && (
            <FieldList
              items={account.fields.map((f) => ({
                name: f.name,
                value: <PostContent html={f.value} emojis={account.emojis} lang={undefined} />,
              }))}
            />
          )}
        </>
      )}
      <MetaLine>
        <span>{t("profile.joined", { date: joined })}</span>
        {account.following_count !== undefined && (
          <MetaCount label={t("profile.following_count")} value={n(account.following_count)} />
        )}
        {account.followers_count !== undefined && (
          <MetaCount label={t("profile.followers_count")} value={n(account.followers_count)} />
        )}
      </MetaLine>
      {account.locked && <CaptionLine icon="followers">{t("profile.locked")}</CaptionLine>}
      {remote && (
        <CaptionLine
          icon="open"
          action={<TextButton onPress={openOriginal(account)}>{t("profile.open_original")}</TextButton>}
        >
          {t("profile.remote", { domain: account.domain })}
        </CaptionLine>
      )}
      {!blocked && featured.length > 0 && (
        <TagList
          heading={t("profile.featured")}
          items={featured.map((tag) => ({
            name: tag.name,
            href: `/tags/${encodeURIComponent(tag.name)}`,
            count: t("profile.tag_posts", { count: tag.statuses_count }),
          }))}
        />
      )}
    </ProfileHead>
  );
}

/** A post of the list: the pinned ones first, keyed apart from the same post further down. */
type Item = { readonly status: Status; readonly pinned: boolean };

function Posts({ account, tab, closed, t }: { account: Account; tab: Tab; closed: boolean; t: T }) {
  const posts = useAccountStatuses(account.id, tab);
  const pinned = useAccountStatuses(account.id, "pinned", tab !== "media");
  const onAction = usePostActions();
  const items: Item[] = [
    ...(tab === "media" ? [] : pinned.items.map((status) => ({ status, pinned: true }))),
    ...posts.items.map((status) => ({ status, pinned: false })),
  ];
  return (
    <ListColumn
      {...posts}
      label={t("profile.tabs")}
      items={items}
      keyOf={(item) => `${item.pinned ? "pinned:" : ""}${item.status.id}`}
      empty={closed ? <FeedNotice>{t("profile.locked_posts")}</FeedNotice> : undefined}
    >
      {(item) => (
        <Post
          status={item.status}
          context={item.pinned ? <CaptionLine icon="pin">{t("profile.pinned")}</CaptionLine> : undefined}
          onAction={onAction}
        />
      )}
    </ListColumn>
  );
}

/** The profile of the account of a handle (`user`, or `user@domain` for a remote one). */
export function Profile({ acct }: { acct: string }) {
  const { t, lang } = stringsIn(useLocale().locale);
  const { account, error } = useAccount(acct);
  const viewer = useViewer();
  const own = account !== undefined && viewer?.id === account.id;
  const relationship = useRelationship(account === undefined || own ? undefined : account.id);
  const [tab, setTab] = useState<Tab>("posts");
  if (account === undefined)
    return error === null ? (
      <FeedNotice>{t("profile.loading")}</FeedNotice>
    ) : (
      <FeedNotice alert>{t("profile.failed")}</FeedNotice>
    );
  const blocked = relationship?.blocking ?? false;
  const closed = account.locked && !own && !(relationship?.following ?? false);
  return (
    <ScreenStack>
      <Head account={account} relationship={relationship} own={own} t={t} lang={lang} />
      {blocked ? (
        <FeedNotice>{t("profile.blocked_posts")}</FeedNotice>
      ) : (
        <TabRow
          label={t("profile.tabs")}
          tabs={[
            { id: "posts", name: t("profile.posts") },
            { id: "replies", name: t("profile.posts_and_replies") },
            { id: "media", name: t("profile.media") },
          ]}
          current={tab}
          onChange={setTab}
        >
          <Posts account={account} tab={tab} closed={closed} t={t} />
        </TabRow>
      )}
    </ScreenStack>
  );
}
