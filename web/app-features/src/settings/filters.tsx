// SPDX-License-Identifier: AGPL-3.0-only

// Keyword filters, the entry of the settings at /settings/filters (FR-209, ADR-0157 §6, §9, §10): the reader's filters,
// oldest first, as the server lists them, each a row that opens its editor (filter-edit.tsx, a screen under the entry)
// and says, under its name, what it does to a post it matches (folds it behind a warning, or hides it) and how many
// words it has, then where it applies and when it ends. One that has ended is still listed, marked Expired, and its
// last line says when it ended. Add a filter, the screen's primary action, leads to the editor of a new one,
// from the section's head, or from what stands in the list's place while there is none; the reader who has the most
// filters the server keeps is told so in its place.

import {
  filterContexts,
  filterLimits,
  keywordFiltersQuery,
  useKeywordFilters,
  type AppData,
  type FilterContext,
  type KeywordFilter,
} from "@my/app-data";
import {
  dateFormat,
  LinkButton,
  SettingsEmpty,
  SettingsList,
  SettingsListRow,
  SettingsScreen,
  SettingsSection,
  useLocale,
  useTimeZone,
} from "@my/design-system";
import { stringsIn } from "../locale";
import { ReadingNotice } from "./controls";
import type { SettingsEntryProps } from "./entries";

type T = ReturnType<typeof stringsIn>["t"];

/** The address of the entry's screen. */
export const filtersPath = "/settings/filters";

/** The address of the editor of a new filter. */
const newFilterPath = `${filtersPath}/new`;

/** The address of the editor of the filter of `id`. */
const filterPath = (id: string): string => `${filtersPath}/${id}`;

/** What the route's loader waits for: the reader's filters, from the cache when it has them. */
export async function loadFilterSettings(data: AppData): Promise<void> {
  await data.queryClient.query({ ...keywordFiltersQuery(data), staleTime: Infinity }).catch(() => undefined);
}

/** The places `contexts` names, in the order the server names them, as one phrase of the reader's language. */
function placesOf(contexts: readonly FilterContext[], lang: string, t: T): string {
  const named = filterContexts
    .filter((c) => contexts.includes(c))
    .map((context) => t("settings.filters.place", { context }));
  return new Intl.ListFormat(lang, { type: "conjunction" }).format(named);
}

/** When a filter ends or ended, as the reader's locale and zone write it: its day, and with `time` its time too. */
export function useFilterDate(time = true): (at: string) => string {
  const { locale } = useLocale();
  const zone = useTimeZone();
  const format = dateFormat(locale, zone, time ? { dateStyle: "medium", timeStyle: "short" } : { dateStyle: "medium" });
  return (at) => format.format(new Date(at));
}

/**
 * What is known of `filter`, in two lines: what it does and how many words it has; where it applies and when it ends
 * or ended.
 */
function factsOf(filter: KeywordFilter, lang: string, date: (at: string) => string, t: T): string[] {
  return [
    t("settings.filters.facts.does", { action: filter.action, count: filter.keywords.length }),
    t("settings.filters.facts.where", {
      places: placesOf(filter.contexts, lang, t),
      state: filter.expires_at === undefined ? "never" : filter.expired ? "ended" : "ends",
      date: filter.expires_at === undefined ? "" : date(filter.expires_at),
    }),
  ];
}

/** The entry's screen: the list of filters with Add a filter, once it is read. */
export function FilterSettings({ back }: SettingsEntryProps) {
  const { t, lang } = stringsIn(useLocale().locale);
  const { filters, status, retry } = useKeywordFilters();
  const date = useFilterDate(false);
  const title = t("settings.entry.filters");
  if (status !== "success")
    return (
      <SettingsScreen
        icon="filter"
        title={title}
        back={back}
        notice={<ReadingNotice failed={status === "error"} onRetry={retry} />}
      />
    );
  const full = filters.length >= filterLimits.filters;
  const add = (
    <LinkButton href={newFilterPath} kind="primary" size="timeline">
      {t("settings.filters.add")}
    </LinkButton>
  );
  const heading = t("settings.filters.list");
  return (
    <SettingsScreen icon="filter" title={title} back={back}>
      <SettingsSection
        heading={heading}
        count={filters.length === 0 ? undefined : new Intl.NumberFormat(lang).format(filters.length)}
        // An empty list says what a filter does in its place, once.
        about={
          full
            ? t("settings.filters.full", { max: filterLimits.filters })
            : filters.length === 0
              ? undefined
              : t("settings.filters.about")
        }
        action={filters.length > 0 && !full ? add : undefined}
      >
        {filters.length === 0 ? (
          <SettingsEmpty action={add}>{t("settings.filters.empty")}</SettingsEmpty>
        ) : (
          <SettingsList label={heading}>
            {filters.map((filter) => (
              <SettingsListRow
                key={filter.id}
                title={filter.title}
                facts={factsOf(filter, lang, date, t)}
                state={filter.expired ? t("settings.filters.expired") : undefined}
                href={filterPath(filter.id)}
              />
            ))}
          </SettingsList>
        )}
      </SettingsSection>
    </SettingsScreen>
  );
}
