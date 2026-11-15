// SPDX-License-Identifier: AGPL-3.0-only

// The tools of a row that sends what is written, which the post composer and the article editor share: the form that
// sets the time it goes out within the server's limits (ADR-0156 §7), in the reader's zone, and the language it is
// written in, by the language's name in its own words.

import { languageName } from "@my/app-components";
import { ChoiceGroup, ChoicePopup, localInput, readerZone, ScheduleForm, ToolText } from "@my/design-system";
import { useState, type ReactNode } from "react";
import type { stringsIn } from "../locale";

type T = ReturnType<typeof stringsIn>["t"];

/** How near and how far ahead a post may be scheduled. */
export type ScheduleLimits = { readonly minLeadMinutes: number; readonly maxLeadDays: number };

/** A time `minutes` from now, at the next whole hour after it: where the schedule's field starts. */
function nextHour(minutes = 60): Date {
  const d = new Date(Date.now() + minutes * 60_000);
  d.setMinutes(0, 0, 0);
  d.setHours(d.getHours() + 1);
  return d;
}

/** What is wrong with a time to post at, by the server's limits; undefined when it is within them. */
function scheduleProblem(value: string, limits: ScheduleLimits, t: T): string | undefined {
  const at = new Date(value);
  if (value === "" || Number.isNaN(at.getTime())) return t("compose.schedule.invalid");
  if (at.getTime() < Date.now() + limits.minLeadMinutes * 60_000)
    return t("compose.schedule.too_soon", { minutes: limits.minLeadMinutes });
  if (at.getTime() > Date.now() + limits.maxLeadDays * 86_400_000)
    return t("compose.schedule.too_far", { days: limits.maxLeadDays });
  return undefined;
}

/**
 * The form that sets the time it goes out (`at`, ISO 8601, none while it goes out at once), in a popover under (or, on
 * a phone, over) its trigger: Set gives `onSet` the time, and Clear, while one is set, none.
 */
export function Schedule({
  at,
  limits,
  onSet,
  t,
  locale,
  open,
  setOpen,
  trigger,
}: {
  at: string | undefined;
  limits: ScheduleLimits;
  onSet: (at: string | undefined) => void;
  t: T;
  locale: string;
  open: boolean;
  setOpen: (o: boolean) => void;
  trigger: ReactNode;
}) {
  const [value, setValue] = useState(() => localInput(at === undefined ? nextHour() : new Date(at)));
  const error = scheduleProblem(value, limits, t);
  return (
    <ChoicePopup
      open={open}
      onClose={() => {
        setOpen(false);
      }}
      label={t("compose.schedule")}
      trigger={trigger}
    >
      <ScheduleForm
        title={t("compose.schedule")}
        label={t("compose.schedule.field")}
        value={value}
        onChange={setValue}
        min={new Date(Date.now() + limits.minLeadMinutes * 60_000)}
        max={new Date(Date.now() + limits.maxLeadDays * 86_400_000)}
        zone={readerZone(locale)}
        hint={t("compose.schedule.limits", { minutes: limits.minLeadMinutes, days: limits.maxLeadDays })}
        error={error}
        confirm={{
          label: t("compose.schedule.set"),
          onPress: () => {
            if (error !== undefined) return;
            onSet(new Date(value).toISOString());
            setOpen(false);
          },
        }}
        clear={
          at === undefined
            ? undefined
            : {
                label: t("compose.schedule.clear"),
                onPress: () => {
                  onSet(undefined);
                  setOpen(false);
                },
              }
        }
      />
    </ChoicePopup>
  );
}

/** The languages offered: the post's, the reader's default and the app's, each once. */
const appLanguages = ["en", "ja", "de", "es", "fr", "pt-BR", "zh-CN", "zh-TW"];

/** A language by its name in its own words (English, 日本語), its first letter capitalized as a picker lists it. */
function ownName(tag: string): string {
  const name = languageName(tag, tag) ?? tag;
  return name.charAt(0).toLocaleUpperCase(tag) + name.slice(1);
}

/**
 * The language tool, by the language's name, and its menu: the language chosen (`current`, empty for none), the
 * reader's default (`preferred`) and the app's (`lang`), then the app's languages.
 */
export function Language({
  current,
  preferred,
  lang,
  onChoose,
  t,
  open,
  setOpen,
}: {
  current: string;
  preferred: string;
  lang: string;
  onChoose: (language: string) => void;
  t: T;
  open: boolean;
  setOpen: (o: boolean) => void;
}) {
  const tags = [current, preferred, lang, ...appLanguages].filter(
    (tag, i, all) => tag !== "" && tag !== "und" && all.indexOf(tag) === i,
  );
  const shown = current === "" ? lang : current;
  return (
    <ChoicePopup
      open={open}
      onClose={() => {
        setOpen(false);
      }}
      label={t("compose.language")}
      end
      trigger={
        <ToolText
          label={t("compose.language.tool", { language: ownName(shown) })}
          lang={shown}
          short={shown.split("-")[0]?.toUpperCase()}
          expanded={open}
          onPress={() => {
            setOpen(!open);
          }}
        >
          {ownName(shown)}
        </ToolText>
      }
    >
      <ChoiceGroup
        title={t("compose.language")}
        items={tags.map((tag) => ({ id: tag, icon: "translate" as const, label: ownName(tag) }))}
        chosen={shown}
        onChoose={(language) => {
          onChoose(language);
          setOpen(false);
        }}
      />
    </ChoicePopup>
  );
}
