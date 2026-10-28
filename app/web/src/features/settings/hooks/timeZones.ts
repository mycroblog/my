// SPDX-License-Identifier: AGPL-3.0-only
// The choices of the time zone setting, named with the browser's CLDR data so that no zone name is written in
// the catalogs. Intl.DisplayNames has no time zone type; Intl.DateTimeFormat's `longGeneric` time zone name
// is CLDR's name of the zone in a locale ("Eastern Time", "米国東部時間"), and its `longOffset` is the zone's
// difference from UTC at a moment.
import { useState } from "react";
import { useT } from "../../../lib/i18n";
import { useDisplaySettings } from "../../../lib/locale";

/** The IANA time zone of this device, which is the browser's. */
export function deviceTimeZone(): string {
  return new Intl.DateTimeFormat().resolvedOptions().timeZone;
}

/** Whether two IANA names are one zone, such as Asia/Calcutta and Asia/Kolkata; a name Intl does not know is only itself. */
export function sameTimeZone(a: string, b: string): boolean {
  return a === b || canonicalTimeZone(a) === canonicalTimeZone(b);
}

function canonicalTimeZone(zone: string): string {
  try {
    return new Intl.DateTimeFormat("en", { timeZone: zone }).resolvedOptions().timeZone;
  } catch {
    return zone;
  }
}

/** A time zone, named in the display locale. */
type TimeZone = {
  /** The IANA name, which the server stores. */
  id: string;
  /** The zone's name; undefined when CLDR names it only by its offset (UTC, Etc/GMT+5). */
  name: string | undefined;
  /** The difference from UTC as `+09:00` / `−05:00` (U+2212, which is read as "minus"); undefined for a zone Intl does not know. */
  offset: string | undefined;
  /** The difference from UTC in minutes, which orders the zones. */
  minutes: number;
};

/** The name of `zone` at `at` in `locale`, in the given style. */
function zoneName(locale: string, zone: string, style: "longGeneric" | "long" | "longOffset", at: Date): string {
  const parts = new Intl.DateTimeFormat(locale, { timeZone: zone, timeZoneName: style }).formatToParts(at);
  return parts.find((part) => part.type === "timeZoneName")?.value ?? "";
}

/** `GMT+09:00`, `GMT-03:30` or `GMT` (in `en`, whose digits are ASCII) as minutes east of UTC. */
function offsetMinutes(gmt: string): number {
  const match = /^GMT([+-])(\d{2}):(\d{2})$/.exec(gmt);
  if (match === null) return 0;
  const minutes = Number(match[2]) * 60 + Number(match[3]);
  return match[1] === "-" ? -minutes : minutes;
}

function formatOffset(minutes: number): string {
  const abs = Math.abs(minutes);
  const hours = String(Math.floor(abs / 60)).padStart(2, "0");
  return `${minutes < 0 ? "−" : "+"}${hours}:${String(abs % 60).padStart(2, "0")}`;
}

/** One zone; a zone Intl does not know keeps only its IANA name. */
function timeZone(zone: string, locale: string, at: Date): TimeZone {
  let minutes: number;
  try {
    minutes = offsetMinutes(zoneName("en", zone, "longOffset", at));
  } catch {
    return { id: zone, name: undefined, offset: undefined, minutes: 0 };
  }
  // CLDR names a zone it has no name for by its offset in every style; the option shows the offset once.
  const offsetName = zoneName(locale, zone, "longOffset", at);
  const name = [zoneName(locale, zone, "longGeneric", at), zoneName(locale, zone, "long", at)].find(
    (n) => n !== "" && n !== offsetName,
  );
  return { id: zone, name, offset: formatOffset(minutes), minutes };
}

/**
 * The IANA time zones the browser knows, with the stored one when it is missing (UTC often is), named in
 * `locale` and ordered by their difference from UTC at `at`, then by name.
 */
function timeZones(current: string, locale: string, at: Date): TimeZone[] {
  const zones = Intl.supportedValuesOf("timeZone");
  const collator = new Intl.Collator(locale);
  return (zones.includes(current) ? zones : [current, ...zones])
    .map((zone) => timeZone(zone, locale, at))
    .toSorted(
      (a, b) => a.minutes - b.minutes || collator.compare(a.name ?? "", b.name ?? "") || collator.compare(a.id, b.id),
    );
}

/** An option of the time zone select: the IANA name as `id`, its label, and the text type-ahead matches. */
type TimeZoneOption = { id: string; label: string; typeahead: string };

/**
 * The time zone options in the display language: "(UTC+09:00) Japan Standard Time", the offset alone for a
 * zone CLDR has no name for, or the IANA name for a zone Intl does not know. Type-ahead matches the zone's
 * name, which the offset at the start of the label would otherwise hide.
 */
export function useTimeZoneOptions(current: string): TimeZoneOption[] {
  const t = useT();
  const { i18n } = useDisplaySettings();
  // The offsets at the moment the page opened, so that the order does not move while the user chooses.
  const [openedAt] = useState(() => new Date());
  return timeZones(current, i18n.locale, openedAt).map((zone) => {
    const label =
      zone.offset === undefined
        ? zone.id
        : zone.name === undefined
          ? t("client:settings.time_zone.offset", { offset: zone.offset })
          : t("client:settings.time_zone.option", { offset: zone.offset, name: zone.name });
    return { id: zone.id, label, typeahead: zone.name ?? label };
  });
}
