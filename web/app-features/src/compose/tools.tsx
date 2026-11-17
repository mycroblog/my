// SPDX-License-Identifier: AGPL-3.0-only

// The tools of a row that sends what is written, which the post composer and the article editor share: the form that
// sets the time it goes out within the server's limits (ADR-0156 §7), in the reader's zone, and the language it is
// written in, by the language's name in its own words.

import { languageName, localInput, readerZone, ScheduleForm } from "@my/app-components";
import { FormPopup, MenuGroup, MenuItem, MenuSurface, ToolText } from "@my/design-system";
import { useState, type ReactElement } from "react";
import type { stringsIn } from "../locale";

type T = ReturnType<typeof stringsIn>["t"];

/** How near and how far ahead a post may be scheduled. */
type ScheduleLimits = { readonly minLeadMinutes: number; readonly maxLeadDays: number };

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

type ScheduleProps = {
  /** When it goes out (ISO 8601), none while it goes out at once. */
  at: string | undefined;
  limits: ScheduleLimits;
  onSet: (at: string | undefined) => void;
  t: T;
  locale: string;
};

/**
 * The form that sets the time it goes out, from `at` (the next whole hour while none is set), within the server's
 * limits, in the reader's zone: Set gives `onSet` the time, and Clear, while one is set, none; either is `onDone`.
 */
export function ScheduleTimeForm({ at, limits, onSet, t, locale, onDone }: ScheduleProps & { onDone: () => void }) {
  const [value, setValue] = useState(() => localInput(at === undefined ? nextHour() : new Date(at)));
  const error = scheduleProblem(value, limits, t);
  return (
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
          onDone();
        },
      }}
      clear={
        at === undefined
          ? undefined
          : {
              label: t("compose.schedule.clear"),
              onPress: () => {
                onSet(undefined);
                onDone();
              },
            }
      }
    />
  );
}

/**
 * The schedule's form (`ScheduleTimeForm`), opened by its trigger (a pressable, which opens and closes it) on the one
 * menu's surface, and closed by Set or Clear.
 */
export function Schedule({
  open,
  setOpen,
  trigger,
  ...form
}: ScheduleProps & { open: boolean; setOpen: (o: boolean) => void; trigger: ReactElement }) {
  return (
    <FormPopup open={open} onOpenChange={setOpen} label={form.t("compose.schedule")} trigger={trigger}>
      <ScheduleTimeForm
        {...form}
        onDone={() => {
          setOpen(false);
        }}
      />
    </FormPopup>
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
    <MenuSurface
      open={open}
      onOpenChange={setOpen}
      label={t("compose.language")}
      placement="bottom end"
      trigger={
        <ToolText
          label={t("compose.language.tool", { language: ownName(shown) })}
          lang={shown}
          short={shown.split("-")[0]?.toUpperCase()}
        >
          {ownName(shown)}
        </ToolText>
      }
    >
      <MenuGroup chosen={shown} onChoose={onChoose}>
        {tags.map((tag) => (
          <MenuItem key={tag} id={tag} icon="translate">
            {ownName(tag)}
          </MenuItem>
        ))}
      </MenuGroup>
    </MenuSurface>
  );
}
