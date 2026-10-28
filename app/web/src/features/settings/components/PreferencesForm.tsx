// SPDX-License-Identifier: AGPL-3.0-only
import {
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
  Stack,
  Switch,
  Text,
} from "@my/ui";
import { useId, useRef } from "react";
import { languageOptions, quotePolicies, quotePolicyDescription, quotePolicyLabel } from "../../compose";
import { type ClientMessageKey, useT } from "../../../lib/i18n";
import { useDisplaySettings } from "../../../lib/locale";
import type { Visibility } from "../api/me";
import { displayLanguageOf, displayLanguageOptions, timelineLanguageOptions } from "../hooks/languages";
import { deviceTimeZone, sameTimeZone, useTimeZoneOptions } from "../hooks/timeZones";
import { type PreferenceControl, usePreferences } from "../hooks/usePreferences";
import { AutoplaySwitch } from "./AutoplaySwitch";
import { NavigationSettings } from "./NavigationSettings";
import { ThemeRadioGroup } from "./ThemeRadioGroup";

/** The visibilities in the order of ADR-0017, widest first, with their label and description. */
const visibilities: readonly { id: Visibility; label: ClientMessageKey; description: ClientMessageKey }[] = [
  {
    id: "public",
    label: "client:settings.visibility.public",
    description: "client:settings.visibility.public.description",
  },
  {
    id: "local",
    label: "client:settings.visibility.local",
    description: "client:settings.visibility.local.description",
  },
  {
    id: "unlisted",
    label: "client:settings.visibility.unlisted",
    description: "client:settings.visibility.unlisted.description",
  },
  {
    id: "private",
    label: "client:settings.visibility.private",
    description: "client:settings.visibility.private.description",
  },
  {
    id: "direct",
    label: "client:settings.visibility.direct",
    description: "client:settings.visibility.direct.description",
  },
];

/**
 * The preferences: every control saves its value as soon as it changes; the theme, the autoplay of animations and the
 * arrangement of the navigation are kept on this browser.
 */
export function PreferencesForm() {
  const t = useT();
  const {
    locale,
    timeZone,
    defaultVisibility,
    defaultQuotePolicy,
    defaultPostLanguage,
    requireMediaDescription,
    timelineLanguages,
    timelineAutoInsert,
  } = usePreferences();
  return (
    <Stack gap="8">
      <Hint size="inherit">{t("client:settings.preferences.autosave")}</Hint>
      <DisplayLanguageSelect control={locale} />
      <TimeZoneSelect control={timeZone} />
      <ThemeRadioGroup />
      <AutoplaySwitch />
      <NavigationSettings />
      <RadioGroup
        label={t("client:settings.default_visibility.label")}
        description={t("client:settings.default_visibility.description")}
        value={defaultVisibility.value}
        onChange={(value) => {
          const visibility = visibilities.find((v) => v.id === value);
          if (visibility !== undefined) defaultVisibility.onChange(visibility.id);
        }}
        isInvalid={defaultVisibility.error !== undefined}
        errorMessage={defaultVisibility.error}
      >
        {visibilities.map((visibility) => (
          <Radio key={visibility.id} value={visibility.id} description={t(visibility.description)}>
            {t(visibility.label)}
          </Radio>
        ))}
      </RadioGroup>
      <RadioGroup
        label={t("client:settings.default_quote_policy.label")}
        description={t("client:settings.default_quote_policy.description")}
        value={defaultQuotePolicy.value}
        onChange={(value) => {
          const policy = quotePolicies.find((p) => p === value);
          if (policy !== undefined) defaultQuotePolicy.onChange(policy);
        }}
        isInvalid={defaultQuotePolicy.error !== undefined}
        errorMessage={defaultQuotePolicy.error}
      >
        {quotePolicies.map((policy) => (
          <Radio key={policy} value={policy} description={t(quotePolicyDescription[policy])}>
            {t(quotePolicyLabel[policy])}
          </Radio>
        ))}
      </RadioGroup>
      <DefaultPostLanguageSelect control={defaultPostLanguage} />
      <Switch
        isSelected={requireMediaDescription.value}
        onChange={requireMediaDescription.onChange}
        description={t("client:settings.require_media_description.description")}
        isInvalid={requireMediaDescription.error !== undefined}
        errorMessage={requireMediaDescription.error}
      >
        {t("client:settings.require_media_description.label")}
      </Switch>
      <TimelineLanguages control={timelineLanguages} />
      <Switch
        isSelected={timelineAutoInsert.value}
        onChange={timelineAutoInsert.onChange}
        description={t("client:settings.timeline_auto_insert.description")}
        isInvalid={timelineAutoInsert.error !== undefined}
        errorMessage={timelineAutoInsert.error}
      >
        {t("client:settings.timeline_auto_insert.label")}
      </Switch>
    </Stack>
  );
}

function DisplayLanguageSelect({ control }: { control: PreferenceControl<string> }) {
  const t = useT();
  const { i18n } = useDisplaySettings();
  const value = displayLanguageOf(control.value);
  return (
    <Select
      label={t("client:settings.locale.label")}
      description={t("client:settings.locale.description")}
      value={value}
      onChange={(key) => {
        if (typeof key === "string" && key !== value) control.onChange(key);
      }}
      isInvalid={control.error !== undefined}
      errorMessage={control.error}
      items={displayLanguageOptions(i18n.locale)}
    >
      {(item) => (
        // The name is in its own language, so a screen reader reads it in that language.
        <SelectItem id={item.id} textValue={item.name}>
          <span lang={item.id}>{item.name}</span>
        </SelectItem>
      )}
    </Select>
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
  );
}

/**
 * The time zone select, and under it, while the chosen zone is not this device's, a note that names the
 * device's zone with a button that chooses it.
 */
function TimeZoneSelect({ control }: { control: PreferenceControl<string> }) {
  const t = useT();
  const options = useTimeZoneOptions(control.value);
  const groupRef = useRef<HTMLDivElement>(null);
  const device = deviceTimeZone();
  const differs = !sameTimeZone(control.value, device);
  const deviceLabel = options.find((option) => sameTimeZone(option.id, device))?.label ?? device;
  return (
    <Stack ref={groupRef} gap="3">
      <TimeZoneField control={control} options={options} />
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
}: {
  control: PreferenceControl<string>;
  options: ReturnType<typeof useTimeZoneOptions>;
}) {
  const t = useT();
  return (
    <Select
      label={t("client:settings.time_zone.label")}
      description={t("client:settings.time_zone.description")}
      value={control.value}
      onChange={(key) => {
        if (typeof key === "string") control.onChange(key);
      }}
      isInvalid={control.error !== undefined}
      errorMessage={control.error}
      items={options}
    >
      {(item) => (
        <SelectItem id={item.id} textValue={item.typeahead} description={item.id}>
          {item.label}
        </SelectItem>
      )}
    </Select>
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
  );
}
