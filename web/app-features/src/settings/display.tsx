// SPDX-License-Identifier: AGPL-3.0-only

// Display and posting, the entry of the settings at /settings/display (FR-007, FR-110, FR-114, FR-208): the language
// and the time zone the app shows, each the account's choice or the browser's; how large a link's preview is; what a
// new post starts with (who can see it, who can quote it, its language, whether media needs a description); and what
// the timelines show (the languages, none chosen meaning all, and new posts as they arrive). Every control saves at
// once and says so in the line under its name (saving.ts): the change shows at once, as the account's preferences show
// it everywhere (app-data), and goes back to what the server has when the server does not take it. The language
// changes the whole app without a reload (the app's reader).

import {
  choiceOf,
  instanceQuery,
  preferencesQuery,
  useInstance,
  usePreferences,
  usePreferencesRead,
  useUpdatePreferences,
  type AppData,
  type Audience,
  type LinkCardSize,
  type Preferences,
  type PreferencesChange,
  type Visibility,
} from "@my/app-data";
import {
  Button,
  FeedNotice,
  Select,
  SettingRow,
  SettingsList,
  SettingsListRow,
  SettingsScreen,
  SettingsSection,
  Switch,
  TextButton,
  useLocale,
  useSaveState,
  type SelectGroup,
  type SelectOption,
} from "@my/design-system";
import { useId } from "react";
import { stringsIn } from "../locale";
import type { MessageId } from "../messages.gen";
import type { SettingsEntryProps } from "./entries";
import { languageOptions, ownName, withLanguage } from "./languages";
import { useSaving } from "./saving";
import { timeZoneOptions, withTimeZone } from "./time-zones";

type T = ReturnType<typeof stringsIn>["t"];

/** What the route's loader waits for: the reader's preferences and the instance, from the cache when it has them. */
export async function loadDisplaySettings(data: AppData): Promise<void> {
  await Promise.all([
    data.queryClient.query({ ...preferencesQuery(data), staleTime: Infinity }),
    data.queryClient.query({ ...instanceQuery(data), staleTime: Infinity }),
  ]).catch(() => undefined);
}

/** The value of the option that follows the browser, and of the post language that follows the display language. */
const browser = "";
const undetermined = "und";

/** A value that is none of a choice's own options, kept as it is until the reader chooses one of them. */
const kept = "kept";

/** A choice from a list that saves at once: its name over the line about it, the picker at the row's end. */
function Choice({
  label,
  about,
  options,
  value,
  onChoose,
}: {
  label: string;
  about?: string | undefined;
  options: readonly (SelectOption | SelectGroup)[];
  value: string;
  /** Saves the value chosen. */
  onChoose: (value: string) => Promise<unknown>;
}) {
  const id = useId();
  const { saving, save } = useSaving();
  const state = useSaveState(saving);
  return (
    <SettingRow id={id} label={label} about={about} save={state}>
      <Select
        labelledBy={id}
        options={options}
        value={value}
        onChange={(chosen) => {
          save(() => onChoose(chosen));
        }}
      />
    </SettingRow>
  );
}

/** A setting on or off that saves at once. */
function Toggle({
  label,
  about,
  on,
  onChange,
}: {
  label: string;
  about?: string | undefined;
  on: boolean;
  /** Saves the state chosen. */
  onChange: (on: boolean) => Promise<unknown>;
}) {
  const { saving, save } = useSaving();
  const state = useSaveState(saving);
  return (
    <Switch
      label={label}
      description={about}
      isSelected={on}
      onChange={(next) => {
        save(() => onChange(next));
      }}
      save={state}
    />
  );
}

/** The app's languages as options, each by its own name. */
const displayLanguages = (languages: readonly string[]): readonly SelectOption[] =>
  languages.map((value) => ({ value, label: ownName(value) }));

/**
 * The option of the display language the account chose: the language itself, else the app's language it is shown in
 * (ja for ja-JP), else itself, which the app has no words for and shows as the browser's.
 */
function chosenLanguage(locale: string, languages: readonly string[]): string {
  if (languages.includes(locale)) return locale;
  let language: string | undefined;
  try {
    ({ language } = new Intl.Locale(locale));
  } catch {
    return locale;
  }
  return languages.includes(language) ? language : locale;
}

/** The visibilities a new post may start with, from the widest; a one-person server has no local posts (ADR-0152). */
function visibilityOptions(current: Visibility, single: boolean, t: T): readonly SelectOption[] {
  const offered: Visibility[] = single ? ["public", "unlisted", "private"] : ["public", "local", "unlisted", "private"];
  // A default set elsewhere (direct) stays shown until the reader chooses another.
  return [...offered, ...(offered.includes(current) ? [] : [current])].map((value) => ({
    value,
    label: t(`compose.visibility.${value}`),
  }));
}

/** Who may quote a new post, as the settings offer it: anyone, the followers, or the author alone. */
const quoteAudiences: readonly Audience[] = ["everyone", "followers", "nobody"];

/**
 * The choice of who may quote: its options and the one the account's default is. A default the three audiences do not
 * say (the people mentioned, or others with the author's approval, set from another app) is an option of its own, in
 * the composer's words, kept until the reader chooses one of the three.
 */
function quoteChoice(preferences: Preferences, t: T): { options: readonly SelectOption[]; value: string } {
  const { audience, approval } = choiceOf(preferences.default_interaction_policy.quote);
  const asks = approval && audience !== "everyone";
  const options = quoteAudiences.map((value) => ({ value: value as string, label: t(`compose.audience.${value}`) }));
  if (quoteAudiences.includes(audience) && !asks) return { options, value: audience };
  const label = t("compose.permission.now", {
    audience: t(`compose.audience.${audience}`),
    approval: asks ? "yes" : "no",
  });
  return { options: [...options, { value: kept, label }], value: kept };
}

/** The languages the timelines show: the choice that adds one, and under it the ones chosen, each with Remove. */
function TimelineLanguages({
  chosen,
  onChange,
  lang,
  t,
}: {
  chosen: readonly string[];
  /** Saves the languages to show. */
  onChange: (languages: string[]) => Promise<unknown>;
  lang: string;
  t: T;
}) {
  const id = useId();
  const { saving, save } = useSaving();
  const state = useSaveState(saving);
  const name = t("settings.display.timeline_languages");
  // The choice holds no language: what it says is that all show, or that one more can be added.
  const first = t(
    chosen.length === 0 ? "settings.display.timeline_languages.all" : "settings.display.timeline_languages.add",
  );
  return (
    <>
      <SettingRow id={id} label={name} about={t("settings.display.timeline_languages.about")} save={state}>
        <Select
          labelledBy={id}
          options={[{ value: "", label: first }, ...languageOptions(lang).filter((o) => !chosen.includes(o.value))]}
          value=""
          onChange={(tag) => {
            if (tag !== "") save(() => onChange([...chosen, tag]));
          }}
        />
      </SettingRow>
      {chosen.length > 0 && (
        <SettingsList label={name}>
          {chosen.map((tag) => (
            <SettingsListRow
              key={tag}
              title={ownName(tag)}
              end={
                <Button
                  size="timeline"
                  aria-label={t("settings.display.timeline_languages.remove.name", { language: ownName(tag) })}
                  onPress={() => {
                    save(() => onChange(chosen.filter((other) => other !== tag)));
                  }}
                >
                  {t("settings.display.timeline_languages.remove")}
                </Button>
              }
            />
          ))}
        </SettingsList>
      )}
    </>
  );
}

/** The sections of the screen, once the preferences are read. */
function Sections({ preferences, languages }: { preferences: Preferences; languages: readonly string[] }) {
  const { locale } = useLocale();
  const { t, lang } = stringsIn(locale);
  const instance = useInstance();
  const update = useUpdatePreferences();
  const change = (c: PreferencesChange) => update(c);
  const follows = t("settings.display.browser");
  const quote = quoteChoice(preferences, t);
  const visibility = preferences.default_visibility;
  const postLanguage = preferences.default_post_language;
  return (
    <>
      <SettingsSection heading={t("settings.display.section.region")}>
        <Choice
          label={t("settings.display.language")}
          about={t("settings.display.language.about")}
          options={[
            { value: browser, label: follows },
            ...(preferences.locale_follows_browser
              ? displayLanguages(languages)
              : withLanguage(displayLanguages(languages), chosenLanguage(preferences.locale, languages))),
          ]}
          value={preferences.locale_follows_browser ? browser : chosenLanguage(preferences.locale, languages)}
          onChoose={(value) => change(value === browser ? { locale_follows_browser: true } : { locale: value })}
        />
        <Choice
          label={t("settings.display.time_zone")}
          options={[
            { value: browser, label: follows },
            ...(preferences.time_zone_follows_browser
              ? timeZoneOptions(locale, t)
              : withTimeZone(timeZoneOptions(locale, t), preferences.time_zone)),
          ]}
          value={preferences.time_zone_follows_browser ? browser : preferences.time_zone}
          onChoose={(value) => change(value === browser ? { time_zone_follows_browser: true } : { time_zone: value })}
        />
      </SettingsSection>
      <SettingsSection heading={t("settings.display.section.display")}>
        <Choice
          label={t("settings.display.link_cards")}
          options={[
            { value: "large" satisfies LinkCardSize, label: t("settings.display.link_cards.large") },
            { value: "small" satisfies LinkCardSize, label: t("settings.display.link_cards.small") },
          ]}
          value={preferences.link_card_size}
          onChoose={(value) => change({ link_card_size: value === "small" ? "small" : "large" })}
        />
      </SettingsSection>
      <SettingsSection heading={t("settings.display.section.posts")} about={t("settings.display.section.posts.about")}>
        <Choice
          label={t("settings.display.audience")}
          about={t(`compose.visibility.${visibility}.about` as MessageId, { server: instance?.domain ?? "" })}
          options={visibilityOptions(visibility, instance?.single_user_account !== undefined, t)}
          value={visibility}
          onChoose={(value) => change({ default_visibility: value as Visibility })}
        />
        <Choice
          label={t("settings.display.quote")}
          options={quote.options}
          value={quote.value}
          onChoose={(value) => {
            const automatic = quoteAudiences.find((audience) => audience === value);
            return automatic === undefined
              ? Promise.resolve()
              : change({ default_interaction_policy: { quote: { automatic } } });
          }}
        />
        <Choice
          label={t("settings.display.post_language")}
          options={[
            { value: undetermined, label: t("settings.display.post_language.display") },
            ...(postLanguage === undetermined
              ? languageOptions(lang)
              : withLanguage(languageOptions(lang), postLanguage)),
          ]}
          value={postLanguage}
          onChoose={(value) => change({ default_post_language: value })}
        />
        <Toggle
          label={t("settings.display.media_description")}
          on={preferences.require_media_description}
          onChange={(on) => change({ require_media_description: on })}
        />
      </SettingsSection>
      <SettingsSection heading={t("settings.display.section.timelines")}>
        <TimelineLanguages
          chosen={preferences.timeline_languages}
          onChange={(timeline_languages) => change({ timeline_languages })}
          lang={lang}
          t={t}
        />
        <Toggle
          label={t("settings.display.auto_insert")}
          about={t("settings.display.auto_insert.about")}
          on={preferences.timeline_auto_insert}
          onChange={(on) => change({ timeline_auto_insert: on })}
        />
      </SettingsSection>
    </>
  );
}

/** The screen of the entry: its sections once the preferences are read, and until then that it loads or failed. */
export function DisplaySettings({ back, languages }: SettingsEntryProps) {
  const { t } = stringsIn(useLocale().locale);
  const preferences = usePreferences();
  const { status, retry } = usePreferencesRead();
  return (
    <SettingsScreen icon="display" title={t("settings.entry.display")} back={back}>
      {preferences !== undefined ? (
        <Sections preferences={preferences} languages={languages} />
      ) : status === "failed" ? (
        <FeedNotice alert action={<TextButton onPress={retry}>{t("settings.retry")}</TextButton>}>
          {t("settings.failed")}
        </FeedNotice>
      ) : (
        <FeedNotice>{t("settings.loading")}</FeedNotice>
      )}
    </SettingsScreen>
  );
}
