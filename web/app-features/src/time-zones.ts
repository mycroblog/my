// SPDX-License-Identifier: AGPL-3.0-only

// The time zones a choice offers (the settings' zone of the app, FR-007, ADR-0009 §3; an event's zone, ADR-0161 §5):
// the platform's, by their names in the time zone database (Asia/Tokyo), under the part of the world each is in, each
// as its place and how far its clocks are from UTC now; UTC itself first, apart. The database names its places in
// English alone, so a place reads the same in every language.

import { dateFormat, type SelectGroup, type SelectOption } from "@my/design-system";
import type { stringsIn } from "./locale";

type T = ReturnType<typeof stringsIn>["t"];

const utc = "UTC";

/** The parts of the world the database's names start with, which the catalog names (settings.display.time_zone.region). */
const regions: ReadonlySet<string> = new Set([
  "Africa",
  "America",
  "Antarctica",
  "Arctic",
  "Asia",
  "Atlantic",
  "Australia",
  "Europe",
  "Indian",
  "Pacific",
]);

/** How far the clocks of `zone` are from UTC now, as `locale` writes it (GMT+9); none for a zone it cannot read. */
function offsetOf(zone: string, locale: string, now: Date): string | undefined {
  try {
    return dateFormat(locale, zone, { timeZoneName: "shortOffset" })
      .formatToParts(now)
      .find((part) => part.type === "timeZoneName")?.value;
  } catch {
    return undefined;
  }
}

/** A zone as an option: its place (Buenos Aires of America/Argentina/Buenos_Aires, with what is between) and its offset. */
function optionOf(zone: string, locale: string, t: T, now: Date): SelectOption {
  const place = zone
    .slice(zone.indexOf("/") + 1)
    .replaceAll("_", " ")
    .replaceAll("/", " / ");
  const offset = offsetOf(zone, locale, now);
  return {
    value: zone,
    label: offset === undefined ? place : t("time_zones.option", { place, offset }),
  };
}

/** The options of each reader's locale, made once: reading four hundred zones' clocks is not free. */
const made = new Map<string, readonly (SelectOption | SelectGroup)[]>();

/**
 * The time zones as the options of a choice in `locale`, whose strings `t` reads: UTC, then the platform's zones in
 * groups by their part of the world, as the database orders them. The offsets are those of the moment they were first
 * asked for on this page.
 */
export function timeZoneOptions(locale: string, t: T): readonly (SelectOption | SelectGroup)[] {
  let options = made.get(locale);
  if (options === undefined) {
    const now = new Date();
    const groups = new Map<string, SelectOption[]>();
    for (const zone of Intl.supportedValuesOf("timeZone")) {
      const region = zone.slice(0, Math.max(0, zone.indexOf("/")));
      if (!regions.has(region)) continue;
      const group = groups.get(region) ?? [];
      group.push(optionOf(zone, locale, t, now));
      groups.set(region, group);
    }
    options = [
      { value: utc, label: utc },
      ...[...groups].map(([region, zones]) => ({
        label: t("time_zones.region", { region }),
        options: zones,
      })),
    ];
    made.set(locale, options);
  }
  return options;
}

/** `options` with one for `zone` at their end, by its name, where they have none: a zone chosen elsewhere. */
export function withTimeZone(
  options: readonly (SelectOption | SelectGroup)[],
  zone: string,
): readonly (SelectOption | SelectGroup)[] {
  const has = options.some((o) => ("options" in o ? o.options.some((z) => z.value === zone) : o.value === zone));
  return has ? options : [...options, { value: zone, label: zone }];
}
