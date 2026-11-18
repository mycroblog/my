// SPDX-License-Identifier: AGPL-3.0-only

// Profile, the entry of the settings at /settings/profile (FR-006, FR-213, FR-307, FR-311, FR-412, FR-913): the
// reader's profile as others see it, small (the profile's own head as a summary: the header image as a strip, the
// avatar, the name with its marks, the handle with the badges shown, the first lines of the bio), with the door to
// the form that edits it (profile-edit.tsx, a screen under this entry, saved as a whole), and what saves at once, each
// control showing the reader's choice from the press and saying where the save is in the line under its name
// (controls.tsx, saving.tsx): the hashtags they feature, saved as the whole list at each one added or removed; whether
// the account is a bot; the badges they show, a list of one switch each, a section that is absent with no badge
// (ADR-0070 §12); whether follows need their approval, and who sees the lists of whom they follow and who follows
// them; and how the account is found: suggestions and the search by name, search engines and this server's search of
// posts, AI, and the Atom feed, which follows the reader's choice for search until they set it. The search choice
// stays the reader's whatever the server tells search engines: other servers receive it with the profile and may use
// it for their search (ADR-0057 §6), and its line says what it still decides. Where the server refuses AI training
// and use for every account, the AI choice decides nothing anywhere: it is disabled, shows what the server enforces,
// and says so (ADR-0057 §1).

import { AccountName, handleOf, nameOf, PostContent, ProfileHead } from "@my/app-components";
import {
  badgeChoicesQuery,
  instanceQuery,
  ownFeaturedHashtagsQuery,
  profileLimits,
  useBadgeChoices,
  useChooseBadge,
  useCredentials,
  useInstance,
  useOwnFeaturedHashtags,
  useProfileRead,
  useSetFeaturedHashtags,
  useUpdateProfile,
  viewerQuery,
  type AppData,
  type CredentialAccount,
  type FollowsVisibility,
  type ProfileChange,
} from "@my/app-data";
import {
  Avatar,
  Badge,
  Excerpt,
  Icon,
  LinkButton,
  SettingRow,
  SettingsList,
  SettingsScreen,
  SettingsSection,
  Stack,
  TagInput,
  useLocale,
} from "@my/design-system";
import { useId } from "react";
import { stringsIn } from "../locale";
import { profileEditPath } from "../profile-identity";
import { Choice, Toggle, useChoice } from "./controls";
import type { SettingsEntryProps } from "./entries";
import { ReadingNotice } from "./parts";

type T = ReturnType<typeof stringsIn>["t"];

/**
 * What the route's loader waits for: the reader's own account, their badges and featured hashtags, and the instance,
 * from the cache when it has them.
 */
export async function loadProfileSettings(data: AppData): Promise<void> {
  await Promise.all([
    data.queryClient.query({ ...viewerQuery(data), staleTime: Infinity }),
    data.queryClient.query({ ...badgeChoicesQuery(data), staleTime: Infinity }),
    data.queryClient.query({ ...ownFeaturedHashtagsQuery(data), staleTime: Infinity }),
    data.queryClient.query({ ...instanceQuery(data), staleTime: Infinity }),
  ]).catch(() => undefined);
}

/** The address of the entry's page, which a change that could not be said in place leads back to. */
const page = "/settings/profile";

const followsAudiences = ["public", "followers", "private"] as const satisfies readonly FollowsVisibility[];

/**
 * The reader's profile as others see it, small: the profile's head as a summary, with what the account shows there
 * now (a change made on this screen shows in it as the server answers), and the door to the form that edits it.
 */
function OwnProfile({ account, t }: { account: CredentialAccount; t: T }) {
  const nameId = useId();
  return (
    <ProfileHead
      summary
      nameId={nameId}
      banner={account.header?.medium.url}
      avatar={
        <Avatar
          name={nameOf(account)}
          src={account.avatar?.medium.url ?? account.avatar?.small.url}
          size="preview"
          decorative
        />
      }
      actions={
        <LinkButton href={profileEditPath} size="timeline">
          {t("settings.profile.edit")}
        </LinkButton>
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
      badges={account.badges.map((badge) => (
        <Badge key={badge.role_id}>{badge.label}</Badge>
      ))}
    >
      {account.note !== "" && (
        <Excerpt lines={2} tone="text" body>
          <PostContent html={account.note} emojis={account.emojis} lang={undefined} />
        </Excerpt>
      )}
    </ProfileHead>
  );
}

/**
 * The hashtags the profile features: each with its remove, and the field that adds one, saved as the whole list. The
 * list shows the reader's change the moment they make it (one added is listed, one removed is gone) while it is on
 * its way, as the switches of the screen do, and the next change starts from it; a save the server did not take puts
 * back the server's list, and trying again sends the same list.
 */
function FeaturedHashtags({ t }: { t: T }) {
  const id = useId();
  const held = useOwnFeaturedHashtags().map((tag) => tag.name);
  const set = useSetFeaturedHashtags();
  const label = t("settings.profile.hashtags");
  const { shown: names, state, choose } = useChoice<readonly string[]>(label, page, held, set);
  const full = names.length >= profileLimits.featuredHashtags;
  const has = (tag: string) => names.some((name) => name.toLowerCase() === tag.toLowerCase());
  return (
    <SettingRow
      id={id}
      label={label}
      about={t("settings.profile.hashtags.about", { max: profileLimits.featuredHashtags })}
      wide
      save={state}
    >
      <TagInput
        label={label}
        items={names}
        removeLabel={(tag) => t("settings.profile.hashtags.remove", { tag })}
        placeholder={t(full ? "settings.profile.hashtags.full" : "settings.profile.hashtags.placeholder")}
        full={full}
        onAdd={(tag) => {
          if (!has(tag)) choose([...names, tag]);
        }}
        onRemove={(tag) => {
          choose(names.filter((name) => name !== tag));
        }}
      />
    </SettingRow>
  );
}

/** The sections of the screen, once what they show is read. */
function Sections({ account }: { account: CredentialAccount }) {
  const { t } = stringsIn(useLocale().locale);
  const instance = useInstance();
  const update = useUpdateProfile();
  const change = (c: ProfileChange) => update(c);
  const badges = useBadgeChoices();
  const chooseBadge = useChooseBadge();
  // What the server states for every account, which an account cannot widen (ADR-0057 §1). Where it tells search
  // engines not to list any account, the account's search choice still decides this server's own search of posts,
  // where it has one, and what other servers are sent of it (§6): it is never fixed, and its line says which. The AI
  // choice narrows this server's own statement alone, and is sent nowhere: where the server refuses both for every
  // account, it changes nothing, is fixed, and its switch shows what the server enforces.
  const unlisted = instance?.search_indexing === "deny";
  const postSearch = instance?.full_text_search.available === true;
  const ai = instance?.ai_usage;
  const aiFixed = ai === "deny";
  return (
    <>
      <SettingsSection heading={t("settings.profile.section.you")}>
        <OwnProfile account={account} t={t} />
        <FeaturedHashtags t={t} />
        <Toggle
          page={page}
          label={t("settings.profile.bot")}
          about={t("settings.profile.bot.about")}
          on={account.bot}
          onChange={(bot) => change({ bot })}
        />
      </SettingsSection>
      {badges.length > 0 && (
        <SettingsSection
          heading={t("settings.profile.section.badges")}
          about={t("settings.profile.section.badges.about")}
        >
          {/* One list, however many badges the account has: a section holds seven things at most (ADR-0110 §6). */}
          <SettingsList label={t("settings.profile.section.badges")}>
            {badges.map((badge) => (
              <Stack as="li" key={badge.role_id}>
                <Toggle
                  page={page}
                  label={t("settings.profile.badge", { label: badge.label })}
                  on={badge.displayed}
                  onChange={(displayed) => chooseBadge(badge.role_id, displayed)}
                />
              </Stack>
            ))}
          </SettingsList>
        </SettingsSection>
      )}
      <SettingsSection heading={t("settings.profile.section.follows")}>
        <Toggle
          page={page}
          label={t("settings.profile.locked")}
          about={t("settings.profile.locked.about")}
          on={account.locked}
          onChange={(locked) => change({ locked })}
        />
        <Choice
          page={page}
          label={t("settings.profile.follows_visibility")}
          about={t("settings.profile.follows_visibility.about")}
          options={followsAudiences.map((who) => ({
            value: who,
            label: t("settings.profile.follows_visibility.option", { who }),
          }))}
          value={account.follows_visibility}
          onChoose={(chosen) => {
            const follows_visibility = followsAudiences.find((who) => who === chosen);
            return follows_visibility === undefined ? Promise.resolve() : change({ follows_visibility });
          }}
        />
      </SettingsSection>
      <SettingsSection heading={t("settings.profile.section.discovery")}>
        <Toggle
          page={page}
          label={t("settings.profile.discoverable")}
          about={t("settings.profile.discoverable.about")}
          on={account.discoverable}
          onChange={(discoverable) => change({ discoverable })}
        />
        <Toggle
          page={page}
          label={t("settings.profile.indexable")}
          about={t("settings.profile.indexable.about", {
            where: unlisted ? (postSearch ? "server" : "elsewhere") : postSearch ? "both" : "engines",
          })}
          on={account.indexable}
          onChange={(indexable) => change({ indexable })}
        />
        <Toggle
          page={page}
          label={t("settings.profile.ai_opt_out")}
          about={t("settings.profile.ai_opt_out.about", { server: ai ?? "allow" })}
          on={aiFixed ? true : account.ai_opt_out}
          fixed={aiFixed ? t("settings.profile.ai_opt_out.overridden") : undefined}
          onChange={(ai_opt_out) => change({ ai_opt_out })}
        />
        <Toggle
          page={page}
          label={t("settings.profile.feed")}
          about={t("settings.profile.feed.about", { follows: account.feed_follows_indexable ? "yes" : "no" })}
          on={account.feed_enabled}
          onChange={(feed_enabled) => change({ feed_enabled })}
        />
      </SettingsSection>
    </>
  );
}

/** The screen of the entry: its sections once what they show is read, and until then that it loads or failed. */
export function ProfileSettings({ back }: SettingsEntryProps) {
  const { t } = stringsIn(useLocale().locale);
  const account = useCredentials();
  const { status, retry } = useProfileRead({ lists: true });
  const read = account !== undefined && status === "read";
  return (
    <SettingsScreen
      icon="profile"
      title={t("settings.entry.profile")}
      back={back}
      notice={read ? undefined : <ReadingNotice status={status === "failed" ? "failed" : "loading"} retry={retry} />}
    >
      {read && <Sections account={account} />}
    </SettingsScreen>
  );
}
