// SPDX-License-Identifier: AGPL-3.0-only
// A profile (FR-204): the header with the account's picture, name, handle, role badges, profile text, metadata fields,
// follow counts and the relationship buttons, then its posts in tabs (WAI-ARIA APG, tabs pattern) whose
// selection the route keeps in the URL (web-client/README.md S3), the posts tab starting with its pinned posts.
import {
  Box,
  EmojiText,
  ExternalLink,
  Hint,
  Link,
  PostContent,
  ProfileField,
  ProfileFields,
  ProfileHeader,
  RoleBadgeList,
  Stack,
  Tab,
  TabList,
  TabPanel,
  Tabs,
  VisuallyHidden,
} from "@my/ui";
import { type ComponentProps, type ReactNode, useId } from "react";
import { type ClientMessageKey, useT } from "../../../lib/i18n";
import { type AccountTimelineFilter, accountTimelineFilters, TimelineFeed, useRemoteFetch } from "../../timeline";
import { usePinnedStatuses } from "../hooks/usePinnedStatuses";
import { isolate, type ProfileView as ProfileViewData, useProfile } from "../hooks/useProfile";
import { useRelationship } from "../hooks/useRelationship";
import { MovedNote } from "./MovedNote";
import { RelationshipActions } from "./RelationshipActions";
import { imageSources } from "../../../lib/images";

/** The label of each tab. */
const tabLabels: Readonly<Record<AccountTimelineFilter, ClientMessageKey>> = {
  posts: "client:profile.tab.posts",
  replies: "client:profile.tab.replies",
  no_boosts: "client:profile.tab.no_boosts",
};

export type ProfileViewProps = {
  /** The handle in the address: `username`, or `username@domain`. */
  acct: string;
  /** The selected tab. */
  tab: AccountTimelineFilter;
  onTabChange: (tab: AccountTimelineFilter) => void;
  /** Renders the page heading (the route's level-one heading) around the account's name. */
  renderHeading: (name: ReactNode) => ReactNode;
};

/** The profile of the account whose handle is `acct`; the route loader has fetched the account and the selected tab. */
export function ProfileView({ acct, tab, onTabChange, renderHeading }: ProfileViewProps) {
  const t = useT();
  const profile = useProfile(acct);
  const relationship = useRelationship(profile.account);
  const { account } = profile;
  const relation = relationship.kind === "other" ? relationship.relationship : null;
  const tags = [
    relation?.followed_by === true && t("client:profile.tag.follows_you"),
    relation?.requested_by === true && t("client:profile.tag.requested_you"),
    relation?.muting === true && t("client:profile.tag.muted"),
    account.locked && t("client:profile.tag.locked"),
    account.bot && t("client:profile.tag.bot"),
  ].filter((tag) => tag !== false);

  return (
    <Stack gap="4">
      <ProfileHeader
        image={imageSources(account.header)}
        avatar={imageSources(account.avatar)}
        avatarSeed={account.id}
        name={profile.name}
        actions={<RelationshipActions account={account} name={profile.name} state={relationship} />}
        heading={renderHeading(
          <span dir="auto">
            <EmojiText text={profile.name} emojis={account.emojis} />
          </span>,
        )}
        handle={profile.handle}
        badges={<RoleBadgeList label={t("client:profile.badges")} badges={profile.badges} />}
        tags={tags}
        note={profile.note !== null && <PostContent html={profile.note} emojis={account.emojis} onLinkClick={profile.onLinkClick} />}
        fields={
          profile.fields.length > 0 && (
            <ProfileFields>
              {profile.fields.map((field) => (
                <ProfileField key={field.key} term={<EmojiText text={field.name} emojis={account.emojis} />}>
                  <PostContent html={field.value} emojis={account.emojis} onLinkClick={profile.onLinkClick} />
                </ProfileField>
              ))}
            </ProfileFields>
          )
        }
        joined={t("client:profile.joined", { date: profile.joined.date })}
        counts={[
          ...(profile.following === null
            ? []
            : [
                {
                  key: "following",
                  content: (
                    <Link href={profile.following.href} variant="standalone">
                      {t("client:profile.following_count", { count: profile.following.count })}
                    </Link>
                  ),
                },
              ]),
          ...(profile.followers === null
            ? []
            : [
                {
                  key: "followers",
                  content: (
                    <Link href={profile.followers.href} variant="standalone">
                      {t("client:profile.followers_count", { count: profile.followers.count })}
                    </Link>
                  ),
                },
              ]),
        ]}
      />
      {account.moved !== undefined && (
        <Box paddingX="4">
          <MovedNote name={profile.name} target={account.moved} isSelf={relationship.kind === "self"} />
        </Box>
      )}
      <ProfileTabs accountId={account.id} name={profile.name} original={profile.original} tab={tab} onTabChange={onTabChange} />
    </Stack>
  );
}

type ProfileTabsProps = {
  accountId: string;
  name: string;
  original: ProfileViewData["original"];
  tab: AccountTimelineFilter;
  onTabChange: (tab: AccountTimelineFilter) => void;
};

function ProfileTabs({ accountId, name, original, tab, onTabChange }: ProfileTabsProps) {
  const t = useT();
  const headingId = useId();
  return (
    <Tabs
      selectedKey={tab}
      onSelectionChange={(key) => {
        const next = accountTimelineFilters.find((filter) => filter === key);
        if (next !== undefined) onTabChange(next);
      }}
    >
      <TabList aria-label={t("client:profile.tabs", { name: isolate(name) })}>
        {accountTimelineFilters.map((filter) => (
          <Tab key={filter} id={filter}>
            {t(tabLabels[filter])}
          </Tab>
        ))}
      </TabList>
      {/* Only the selected panel is mounted (React Aria Tabs), so only its timeline loads. */}
      {accountTimelineFilters.map((filter) => (
        <TabPanel key={filter} id={filter}>
          <VisuallyHidden>
            <h2 id={headingId}>{t(tabLabels[filter])}</h2>
          </VisuallyHidden>
          {/* Another tab is another timeline: nothing of the previous one carries over (web-client/README.md R10). */}
          <AccountPosts key={filter} accountId={accountId} filter={filter} original={original} labelledBy={headingId} />
        </TabPanel>
      ))}
    </Tabs>
  );
}

type AccountPostsProps = { accountId: string; filter: AccountTimelineFilter; original: ProfileViewData["original"]; labelledBy: string };

/**
 * A tab's posts. For an account of another server they are only those this instance has received, which
 * the feed says above them, with a link to the profile on the account's own server; while the server fetches
 * the account's recent posts from there, a line says so until they are read again (ADR-0044).
 */
function AccountPosts({ accountId, filter, original, labelledBy }: AccountPostsProps) {
  const t = useT();
  const fetching = useRemoteFetch(accountId, filter);
  const origin =
    original === null
      ? undefined
      : {
          notice: t("client:profile.remote.notice"),
          link: (
            // The profile on the account's own server leaves the app.
            <ExternalLink href={original.href} variant="standalone">
              {t("client:profile.remote.original", { host: isolate(original.host) })}
            </ExternalLink>
          ),
        };
  return (
    <Stack>
      {/* Visible only: useRemoteFetch announces it in the app's polite live region, which was in the page before it. */}
      {fetching && (
        <Box paddingX="4" paddingY="3">
          <Hint>{t("client:profile.remote.fetching")}</Hint>
        </Box>
      )}
      {filter === "posts" ? (
        <PostsWithPinned accountId={accountId} labelledBy={labelledBy} origin={origin} />
      ) : (
        <TimelineFeed timeline={{ kind: "account", id: accountId, filter }} aria-labelledby={labelledBy} {...(origin !== undefined && { origin })} />
      )}
    </Stack>
  );
}

type PostsWithPinnedProps = { accountId: string; labelledBy: string; origin: ComponentProps<typeof TimelineFeed>["origin"] };

/** The posts tab: the account's pinned posts at the top of its posts (FR-403), as Mastodon shows them. */
function PostsWithPinned({ accountId, labelledBy, origin }: PostsWithPinnedProps) {
  const pinned = usePinnedStatuses(accountId);
  return (
    <TimelineFeed
      timeline={{ kind: "account", id: accountId, filter: "posts" }}
      aria-labelledby={labelledBy}
      pinned={pinned}
      {...(origin !== undefined && { origin })}
    />
  );
}
