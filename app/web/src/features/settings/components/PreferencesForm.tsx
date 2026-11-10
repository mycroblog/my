// SPDX-License-Identifier: AGPL-3.0-only
import {
  AutosaveControl,
  Button,
  Callout,
  Checkbox,
  Grid,
  Hint,
  InlineError,
  Radio,
  RadioGroup,
  Select,
  SelectItem,
  Section,
  Stack,
  Switch,
  Text,
  type SaveState,
} from "@my/ui";
import { type ReactNode, useId, useRef } from "react";
import {
  InteractionPolicyFields,
  isVisibility,
  languageOptions,
  useOfferedVisibilities,
  visibilityDescription,
  visibilityLabel,
} from "../../compose";
import { useT } from "../../../lib/i18n";
import { useAppLocale, useDisplaySettings } from "../../../lib/locale";
import { useSaveLabels } from "../hooks/saveState";
import { displayLanguageOf, displayLanguageOptions, timelineLanguageOptions } from "../hooks/languages";
import { deviceTimeZone, sameTimeZone, useTimeZoneOptions } from "../hooks/timeZones";
import { followBrowser, type PreferenceControl, usePreferences } from "../hooks/usePreferences";
import { AutoplaySwitch } from "./AutoplaySwitch";
import { NavigationSettings } from "./NavigationSettings";
import { ThemeRadioGroup } from "./ThemeRadioGroup";

/**
 * The preferences in the sections of their screen requirements (web-client/screens/settings-preferences.md): the
 * display, the navigation, the defaults of new posts, and the timelines. Every control saves its value as soon as it
 * changes; the theme, the autoplay of animations and the arrangement of the navigation are kept on this browser, the
 * rest on the account, and each section's description says which.
 */
export function PreferencesForm() {
  const t = useT();
  const {
    locale,
    timeZone,
    defaultVisibility,
    defaultInteractionPolicy,
    defaultPostLanguage,
    requireMediaDescription,
    timelineLanguages,
    timelineAutoInsert,
    linkCardSize,
  } = usePreferences();
  const policyLabelId = useId();
  const policyDescriptionId = useId();
  // A one-person server takes no default of `local` (ADR-0152 §5) and reads a stored one as `private`.
  const offeredVisibilities = useOfferedVisibilities();
  return (
    <Stack gap="8">
      <Hint size="inherit">{t("client:settings.preferences.autosave")}</Hint>
      <Section
        title={t("client:settings.preferences.display.title")}
        description={t("client:settings.preferences.display.description")}
      >
        <Stack gap="6">
          <DisplayLanguageSelect control={locale} />
          <TimeZoneSelect control={timeZone} />
          <ThemeRadioGroup />
          <AutoplaySwitch />
          <Autosaved state={linkCardSize.saveState}>
            <RadioGroup
              label={t("client:settings.link_card_size.label")}
              value={linkCardSize.value}
              onChange={(value) => {
                if (value === "large" || value === "small") linkCardSize.onChange(value);
              }}
              isInvalid={linkCardSize.error !== undefined}
              errorMessage={linkCardSize.error}
            >
              <Radio value="large" description={t("client:settings.link_card_size.large.description")}>
                {t("client:settings.link_card_size.large")}
              </Radio>
              <Radio value="small" description={t("client:settings.link_card_size.small.description")}>
                {t("client:settings.link_card_size.small")}
              </Radio>
            </RadioGroup>
          </Autosaved>
        </Stack>
      </Section>
      <NavigationSettings />
      <Section
        title={t("client:settings.preferences.posting.title")}
        description={t("client:settings.preferences.posting.description")}
      >
        <Stack gap="6">
          <Autosaved state={defaultVisibility.saveState}>
            <RadioGroup
              label={t("client:settings.default_visibility.label")}
              description={t("client:settings.default_visibility.description")}
              value={defaultVisibility.value}
              onChange={(value) => {
                if (isVisibility(value)) defaultVisibility.onChange(value);
              }}
              isInvalid={defaultVisibility.error !== undefined}
              errorMessage={defaultVisibility.error}
            >
              {offeredVisibilities.map((visibility) => (
                <Radio key={visibility} value={visibility} description={t(visibilityDescription[visibility])}>
                  {t(visibilityLabel[visibility])}
                </Radio>
              ))}
            </RadioGroup>
          </Autosaved>
          <Autosaved state={defaultInteractionPolicy.saveState}>
            <Stack gap="2" role="group" aria-labelledby={policyLabelId} aria-describedby={policyDescriptionId}>
              <Text as="p" id={policyLabelId} weight="medium">
                {t("client:settings.default_interaction_policy.label")}
              </Text>
              <Hint id={policyDescriptionId}>{t("client:settings.default_interaction_policy.description")}</Hint>
              <InteractionPolicyFields
                value={defaultInteractionPolicy.value}
                onChange={defaultInteractionPolicy.onChange}
                errors={defaultInteractionPolicy.errors}
              />
            </Stack>
          </Autosaved>
          <DefaultPostLanguageSelect control={defaultPostLanguage} />
          <Autosaved state={requireMediaDescription.saveState}>
            <Switch
              isSelected={requireMediaDescription.value}
              onChange={requireMediaDescription.onChange}
              description={t("client:settings.require_media_description.description")}
              isInvalid={requireMediaDescription.error !== undefined}
              errorMessage={requireMediaDescription.error}
            >
              {t("client:settings.require_media_description.label")}
            </Switch>
          </Autosaved>
        </Stack>
      </Section>
      <Section
        title={t("client:settings.preferences.timeline.title")}
        description={t("client:settings.preferences.timeline.description")}
      >
        <Stack gap="6">
          <TimelineLanguages control={timelineLanguages} />
          <Autosaved state={timelineAutoInsert.saveState}>
            <Switch
              isSelected={timelineAutoInsert.value}
              onChange={timelineAutoInsert.onChange}
              description={t("client:settings.timeline_auto_insert.description")}
              isInvalid={timelineAutoInsert.error !== undefined}
              errorMessage={timelineAutoInsert.error}
            >
              {t("client:settings.timeline_auto_insert.label")}
            </Switch>
          </Autosaved>
        </Stack>
      </Section>
    </Stack>
  );
}

function DisplayLanguageSelect({ control }: { control: PreferenceControl<string> }) {
  const t = useT();
  const { i18n } = useDisplaySettings();
  const appLocale = useAppLocale();
  const value = control.value === followBrowser ? followBrowser : displayLanguageOf(control.value);
  const languages = displayLanguageOptions(i18n.locale);
  const browserLocale = appLocale.browserLocale(navigator.languages);
  const browserLanguage = languages.find((language) => language.id === browserLocale)?.name ?? browserLocale;
  return (
    <Autosaved state={control.saveState}>
      <Select
        label={t("client:settings.locale.label")}
        description={t("client:settings.locale.description")}
        value={value}
        onChange={(key) => {
          if (typeof key === "string" && key !== value) control.onChange(key);
        }}
        isInvalid={control.error !== undefined}
        errorMessage={control.error}
        items={[
          { id: followBrowser, name: t("client:settings.locale.follow_browser", { language: browserLanguage }) },
          ...languages,
        ]}
      >
        {(item) =>
          item.id === followBrowser ? (
            <SelectItem
              id={item.id}
              textValue={item.name}
              description={t("client:settings.locale.follow_browser_description")}
            >
              {item.name}
            </SelectItem>
          ) : (
            // The name is in its own language, so a screen reader reads it in that language.
            <SelectItem
              id={item.id}
              textValue={item.name}
              {...("machineTranslated" in item && item.machineTranslated
                ? { description: t("client:settings.locale.machine_translated") }
                : {})}
            >
              <span lang={item.id}>{item.name}</span>
            </SelectItem>
          )
        }
      </Select>
    </Autosaved>
  );
}

/**
 * The language new posts start in (FR-007): the choices of the post form's language select, "undetermined"
 * (`und`) first, so that what is chosen here is what the form starts with.
 */
function DefaultPostLanguageSelect({ control }: { control: PreferenceControl<string> }) {
  const t = useT();
  const { i18n } = useDisplaySettings();
  return (
    <Autosaved state={control.saveState}>
      <Select
        label={t("client:settings.default_post_language.label")}
        description={t("client:settings.default_post_language.description")}
        value={control.value}
        onChange={(key) => {
          if (typeof key === "string" && key !== control.value) control.onChange(key);
        }}
        isInvalid={control.error !== undefined}
        errorMessage={control.error}
        items={languageOptions(i18n.locale, [control.value], t("client:compose.language.und"))}
      >
        {(item) => (
          <SelectItem id={item.id} textValue={item.name}>
            {item.name}
          </SelectItem>
        )}
      </Select>
    </Autosaved>
  );
}

/**
 * The time zone select, and under it, while the chosen zone is not this device's, a note that names the
 * device's zone with a button that chooses it.
 */
function TimeZoneSelect({ control }: { control: PreferenceControl<string> }) {
  const t = useT();
  const device = deviceTimeZone();
  const following = control.value === followBrowser;
  const options = useTimeZoneOptions(following ? device : control.value);
  const groupRef = useRef<HTMLDivElement>(null);
  const differs = !following && !sameTimeZone(control.value, device);
  const deviceLabel = options.find((option) => sameTimeZone(option.id, device))?.label ?? device;
  return (
    <Stack ref={groupRef} gap="3">
      <TimeZoneField control={control} options={options} deviceLabel={deviceLabel} />
      {differs && (
        <Callout>
          <Text as="p" size="sm" tone="fg" wrap="anywhere">
            {t("client:settings.time_zone.device_differs", {
              zone: deviceLabel,
            })}
          </Text>
          <Button
            size="sm"
            onPress={() => {
              // The button goes away with the difference: focus moves to the select, which shows the new zone.
              groupRef.current?.querySelector<HTMLElement>("button")?.focus();
              control.onChange(device);
            }}
          >
            {t("client:settings.time_zone.use_device")}
          </Button>
        </Callout>
      )}
    </Stack>
  );
}

/**
 * The time zone: each option is the zone's name in the display language with its difference from UTC, and is
 * described by its IANA name, which tells apart the zones of one name (America/New_York, America/Detroit).
 */
function TimeZoneField({
  control,
  options,
  deviceLabel,
}: {
  control: PreferenceControl<string>;
  options: ReturnType<typeof useTimeZoneOptions>;
  deviceLabel: string;
}) {
  const t = useT();
  const followLabel = t("client:settings.time_zone.follow_browser", { zone: deviceLabel });
  return (
    <Autosaved state={control.saveState}>
      <Select
        label={t("client:settings.time_zone.label")}
        description={t("client:settings.time_zone.description")}
        value={control.value}
        onChange={(key) => {
          if (typeof key === "string") control.onChange(key);
        }}
        isInvalid={control.error !== undefined}
        errorMessage={control.error}
        items={[
          {
            id: followBrowser,
            label: followLabel,
            typeahead: followLabel,
            description: t("client:settings.time_zone.follow_browser_description"),
          },
          ...options.map((option) => ({ ...option, description: option.id })),
        ]}
      >
        {(item) => (
          <SelectItem id={item.id} textValue={item.typeahead} description={item.description}>
            {item.label}
          </SelectItem>
        )}
      </Select>
    </Autosaved>
  );
}

/** A group of checkboxes (a fieldset, as the APG checkbox pattern groups them); none checked shows every language. */
function TimelineLanguages({ control }: { control: PreferenceControl<string[]> }) {
  const t = useT();
  const { i18n } = useDisplaySettings();
  const descriptionId = useId();
  const errorId = useId();
  const chosen = new Set(control.value);
  const toggle = (tag: string, selected: boolean) => {
    const next = selected ? [...chosen, tag] : [...chosen].filter((c) => c !== tag);
    control.onChange(next.toSorted());
  };
  return (
    <Autosaved state={control.saveState}>
      <Stack
        as="fieldset"
        aria-describedby={control.error === undefined ? descriptionId : `${descriptionId} ${errorId}`}
        container
        shrink
        gap="2"
      >
        <Text as="legend" size="sm" weight="medium" tone="fg">
          {t("client:settings.timeline_languages.label")}
        </Text>
        <Hint id={descriptionId}>{t("client:settings.timeline_languages.description")}</Hint>
        <Grid as="ul" gap="6" rowGap="1">
          {timelineLanguageOptions(control.value, i18n.locale).map((option) => (
            <li key={option.id}>
              <Checkbox
                isSelected={chosen.has(option.id)}
                onChange={(selected) => {
                  toggle(option.id, selected);
                }}
              >
                {option.name}
              </Checkbox>
            </li>
          ))}
        </Grid>
        {control.error !== undefined && (
          <InlineError id={errorId} alert>
            {control.error}
          </InlineError>
        )}
      </Stack>
    </Autosaved>
  );
}

/**
 * A preference's control with its save mark beside it (web-client/16-look-states-and-wording.md G24): saving, saved,
 * or not saved, where the person changed it.
 */
function Autosaved({ state, children }: { state: SaveState; children: ReactNode }) {
  const labels = useSaveLabels();
  return (
    <AutosaveControl state={state} labels={labels}>
      {children}
    </AutosaveControl>
  );
}
