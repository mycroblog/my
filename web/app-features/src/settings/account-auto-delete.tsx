// SPDX-License-Identifier: AGPL-3.0-only

// Delete old posts automatically, the screen under the settings' entry Account and data at
// /settings/account/auto-delete (FR-118, FR-121, ADR-0151 §7, §8), where the `auto_delete_limit` notification leads: a
// form saved as a whole by its submit bar.
//
// - Whether old posts are deleted, and past which age in days.
// - The posts that are kept however old: pinned, bookmarked or favourited by the reader, with media, with a poll,
//   articles; those made for an audience; and those boosted or favourited at least so many times (0 keeps none).
// - What the form as it stands would delete now, counted by the server before anything is saved and again whenever
//   the form changes: how many posts, and the oldest of them, each a link to its page. Off, it deletes nothing.
//
// Under a maximum of the server that applies to the reader, the form holds what the server uses (its `effective`
// policy): deletion is on and cannot be turned off, which its row says, the age goes up to the maximum, and what the
// server does not let be kept is unchecked and cannot be checked, its section saying so. A reader the maximum binds
// (it deletes posts their saved settings keep) reads so in the summary that heads the screen, with the day it starts
// to apply: until then their saved settings stay in effect, the count below is of what the maximum then deletes, and
// saving the form puts their settings within the maximum at once.
//
// What is wrong stands under the input it is about, which takes the focus: no age, and what the server refuses of a
// member it names, in the words of the rule where the form knows it; the submit bar then says only that nothing was
// saved. Once saved, the submit bar says so and the form stays. Leaving with changes not saved is the app's to notice
// (`onUnsaved`): it holds the reader back and gives `leaving`, and the screen asks whether to leave.

import { plainText, statusPath } from "@my/app-components";
import {
  autoDeleteLimits,
  autoDeleteQuery,
  refusalDetail,
  refusedField,
  useAutoDeleteEstimate,
  useAutoDeleteRead,
  useAutoDeleteWrites,
  useInstance,
  type AppData,
  type AutoDeletePolicy,
  type AutoDeleteServerLimit,
  type AutoDeleteSettings,
  type Visibility,
} from "@my/app-data";
import {
  BackButton,
  Box,
  Button,
  CheckField,
  ConfirmDialog,
  FieldError,
  focusRefusedField,
  FormAlert,
  ModalPresence,
  NumberField,
  SaveState,
  Select,
  SettingRow,
  SettingsEmpty,
  SettingsList,
  SettingsListRow,
  SettingsScreen,
  SettingsSection,
  Stack,
  StatusSummary,
  SubmitBar,
  useLocale,
} from "@my/design-system";
import { useEffect, useId, useRef, useState } from "react";
import { stringsIn } from "../locale";
import { useActionDate } from "./account";
import { ReadingNotice } from "./parts";

/** What the route's loader waits for: the reader's policy, from the cache when it has it. */
export async function loadAccountAutoDelete(data: AppData): Promise<void> {
  await data.queryClient.query({ ...autoDeleteQuery(data), staleTime: Infinity }).catch(() => undefined);
}

/** The title of the screen. */
export function useAccountAutoDeleteTitle(): string {
  return stringsIn(useLocale().locale).t("settings.account.auto_delete");
}

export type AccountAutoDeleteProps = {
  /** Goes back to the entry. */
  onBack: () => void;
  /** Hears whether the form holds changes that are not saved, as that comes and goes; false before it leaves. */
  onUnsaved?: ((unsaved: boolean) => void) | undefined;
  /**
   * The reader is leaving with changes not saved, and is held back: the screen asks whether to leave (`onLeave`, the
   * changes lost) or stay (`onStay`).
   */
  leaving?: { onStay: () => void; onLeave: () => void } | undefined;
};

/** The kinds of post a policy keeps by a property of its own, in the order shown, with the limit's word for each. */
const kept = [
  ["keep_pinned", "allow_pinned", "pinned"],
  ["keep_bookmarked", "allow_bookmarked", "bookmarked"],
  ["keep_favourited", "allow_favourited", "favourited"],
  ["keep_media", "allow_media", "media"],
  ["keep_polls", "allow_polls", "polls"],
  ["keep_articles", "allow_articles", "articles"],
] as const;
type Keep = (typeof kept)[number][0];

/** The audiences a policy may keep posts of, from the widest. */
const audiences: readonly Visibility[] = ["public", "local", "unlisted", "private", "direct"];

/** What the form holds: the policy, with no number where a field is empty. */
type Form = Omit<AutoDeletePolicy, "min_age_days" | "keep_min_boosts" | "keep_min_favourites" | "keep_articles"> & {
  readonly min_age_days: number | null;
  readonly keep_min_boosts: number | null;
  readonly keep_min_favourites: number | null;
  readonly keep_articles: boolean;
};

/** The form of `policy`. */
const formOf = (policy: AutoDeletePolicy): Form => ({
  enabled: policy.enabled,
  min_age_days: policy.min_age_days,
  keep_pinned: policy.keep_pinned,
  keep_bookmarked: policy.keep_bookmarked,
  keep_favourited: policy.keep_favourited,
  keep_media: policy.keep_media,
  keep_polls: policy.keep_polls,
  keep_articles: policy.keep_articles ?? true,
  keep_visibilities: policy.keep_visibilities,
  keep_min_boosts: policy.keep_min_boosts,
  keep_min_favourites: policy.keep_min_favourites,
});

/** The policy the form saves and asks about, once it has an age; an empty threshold keeps none. */
const policyOf = (form: Form, days: number): AutoDeletePolicy => ({
  ...form,
  min_age_days: days,
  keep_visibilities: audiences.filter((v) => form.keep_visibilities.includes(v)),
  keep_min_boosts: form.keep_min_boosts ?? 0,
  keep_min_favourites: form.keep_min_favourites ?? 0,
});

/** What of the form is saved, to tell whether it changed. */
const savedOf = (form: Form) => JSON.stringify(policyOf(form, form.min_age_days ?? 0));

/** The inputs of the form, which what is wrong is placed on. */
type Input = "enabled" | "days" | "kept" | "audiences" | "boosts" | "favourites";

/** Which input answers to which member of the request (`PUT /preferences/auto_delete`). */
const inputs: Readonly<Record<string, Input>> = {
  enabled: "enabled",
  min_age_days: "days",
  keep_pinned: "kept",
  keep_bookmarked: "kept",
  keep_favourited: "kept",
  keep_media: "kept",
  keep_polls: "kept",
  keep_articles: "kept",
  keep_visibilities: "audiences",
  keep_min_boosts: "boosts",
  keep_min_favourites: "favourites",
};

/** Where the form's last save is; one refused, with what is wrong and where, or why in the submit bar's words. */
type Sent =
  | { readonly kind: "idle" | "sending" | "saved" }
  | {
      readonly kind: "refused";
      readonly field?: { readonly input: Input; readonly text: string } | undefined;
      readonly reason?: string | undefined;
    };

/** How much of a post's text names it among the oldest posts a policy would delete. */
const excerptLength = 100;

/** The start of the text of a post, as plain text on one line; undefined for a post without text. */
function excerptOf(content: string): string | undefined {
  const text = plainText(content).replace(/\s+/g, " ");
  if (text === "") return undefined;
  const letters = Array.from(text);
  return letters.length > excerptLength ? `${letters.slice(0, excerptLength).join("")}…` : text;
}

/** Where the server's maximum stands for a reader it binds: it will apply at a day, applies now, or will apply. */
function boundStateOf(limit: AutoDeleteServerLimit): "from" | "now" | "soon" {
  if (limit.enforce_from === undefined) return "soon";
  return Date.parse(limit.enforce_from) <= Date.now() ? "now" : "from";
}

function PolicyForm({
  settings,
  onBack,
  onUnsaved,
  leaving,
}: AccountAutoDeleteProps & { settings: AutoDeleteSettings }) {
  const { t } = stringsIn(useLocale().locale);
  const instance = useInstance();
  const date = useActionDate();
  const day = useActionDate(false);
  const id = useId();
  const writes = useAutoDeleteWrites();
  const limit = settings.server_limit;
  // Under the server's maximum the form holds what the server uses, which is what a save may send.
  const [base, setBase] = useState<Form>(() => formOf(limit?.effective ?? settings));
  const [form, setForm] = useState<Form>(base);
  const [sent, setSent] = useState<Sent>({ kind: "idle" });
  // What the form as it stands would delete now: nothing while it is off, and no count without an age.
  const counted = useAutoDeleteEstimate(
    form.enabled && form.min_age_days !== null ? policyOf(form, form.min_age_days) : undefined,
  );
  const focusNext = useRef<string | undefined>(undefined);
  useEffect(() => {
    if (focusNext.current === undefined) return;
    focusRefusedField(focusNext.current);
    focusNext.current = undefined;
  });

  const busy = sent.kind === "sending";
  const unsaved = savedOf(form) !== savedOf(base);
  useEffect(() => {
    onUnsaved?.(unsaved);
  }, [onUnsaved, unsaved]);
  useEffect(
    () => () => {
      onUnsaved?.(false);
    },
    [onUnsaved],
  );

  /** A change of the form: what the last save said was about the form as it was. */
  const change = (part: Partial<Form>) => {
    setForm((now) => ({ ...now, ...part }));
    setSent((was) => (was.kind === "saved" || was.kind === "refused" ? { kind: "idle" } : was));
  };

  const maxDays = limit?.max_age_days ?? autoDeleteLimits.maxDays;
  const enabledId = `${id}-enabled`;
  const daysId = `${id}-days`;
  const boostsId = `${id}-boosts`;
  const favouritesId = `${id}-favourites`;
  const keepId = (keep: Keep) => `${id}-${keep}`;
  const audienceId = (v: Visibility) => `${id}-audience-${v}`;
  // What is wrong with a group of checkboxes is under the group, and comes into view with the first of them, which
  // takes the focus (`focusRefusedField` looks for `<id>-error`).
  const keptErrorId = `${keepId("keep_pinned")}-error`;
  const audiencesErrorId = `${audienceId("public")}-error`;
  /** The control of a setting's row, and under it the place of what is wrong with it. */
  const controlOf = (row: string) => `${row}-control`;
  const errorOf = (row: string) => `${controlOf(row)}-error`;

  /** The element of an input that was refused, which takes the focus. */
  const elementOf = (input: Input): string =>
    input === "enabled"
      ? controlOf(enabledId)
      : input === "days"
        ? controlOf(daysId)
        : input === "boosts"
          ? controlOf(boostsId)
          : input === "favourites"
            ? controlOf(favouritesId)
            : input === "kept"
              ? keepId("keep_pinned")
              : audienceId("public");
  /** The rule an input's value breaks, in the form's words: the server says only that the value is not valid. */
  const ruleOf = (input: Input): string =>
    input === "enabled"
      ? t("settings.account.auto_delete.enabled.fixed")
      : input === "days"
        ? t("settings.account.auto_delete.days.rule", { max: maxDays })
        : input === "boosts" || input === "favourites"
          ? t("settings.account.auto_delete.threshold.rule", { max: autoDeleteLimits.maxCount })
          : t("settings.account.auto_delete.kept.rule");
  const refuse = (input: Input, text: string) => {
    setSent({ kind: "refused", field: { input, text } });
    focusNext.current = elementOf(input);
  };

  const save = () => {
    if (busy) return;
    if (form.min_age_days === null) {
      refuse("days", t("settings.account.auto_delete.days.rule", { max: maxDays }));
      return;
    }
    const saving = form;
    setSent({ kind: "sending" });
    writes.save(policyOf(saving, form.min_age_days)).then(
      () => {
        setBase(saving);
        setSent({ kind: "saved" });
      },
      (error: unknown) => {
        const named = refusedField(error);
        const input = named === undefined ? undefined : inputs[named.member];
        if (input !== undefined) refuse(input, ruleOf(input));
        else setSent({ kind: "refused", reason: refusalDetail(error) });
      },
    );
  };

  const refused = sent.kind === "refused" ? sent.field : undefined;
  const errorAt = (input: Input) => (refused?.input === input ? refused.text : undefined);
  const daysError = errorAt("days");
  const enabledError = errorAt("enabled");
  const keptError = errorAt("kept");
  const audiencesError = errorAt("audiences");
  const thresholds = limit === undefined || limit.allow_thresholds;
  const byAudience = limit === undefined || limit.allow_visibilities;
  // A one-person server has no local posts (ADR-0152): the audience is offered only where it is kept already.
  const single = instance?.single_user_account !== undefined;
  const offered = audiences.filter((v) => v !== "local" || !single || form.keep_visibilities.includes("local"));
  const bound = limit?.bound === true ? boundStateOf(limit) : undefined;
  const whenHeading = t("settings.account.auto_delete.section.when");
  const keptHeading = t("settings.account.auto_delete.section.kept");
  const oldest = t("settings.account.auto_delete.count.oldest");

  const threshold = (row: string, input: "boosts" | "favourites", value: number | null) => {
    const error = errorAt(input);
    return (
      <>
        <SettingRow
          id={row}
          label={t("settings.account.auto_delete.threshold", { of: input })}
          about={t("settings.account.auto_delete.threshold.about")}
          isDisabled={!thresholds}
          form
        >
          <NumberField
            name={input}
            value={value}
            min={0}
            max={autoDeleteLimits.maxCount}
            unit={t("settings.account.auto_delete.threshold.unit")}
            describedBy={error === undefined ? undefined : errorOf(row)}
            onChange={(n) => {
              change(input === "boosts" ? { keep_min_boosts: n } : { keep_min_favourites: n });
            }}
            isDisabled={busy || !thresholds}
          />
        </SettingRow>
        {error !== undefined && <FieldError id={errorOf(row)}>{error}</FieldError>}
      </>
    );
  };

  return (
    <>
      <SettingsScreen
        sub
        title={t("settings.account.auto_delete")}
        back={<BackButton onPress={onBack}>{t("settings.entry.account")}</BackButton>}
        foot={
          <SubmitBar
            result={
              sent.kind === "saved" ? (
                <SaveState state="saved" />
              ) : sent.kind === "refused" ? (
                <FormAlert>
                  {sent.reason ??
                    t(
                      sent.field === undefined
                        ? "settings.account.auto_delete.save.failed"
                        : "settings.account.auto_delete.save.refused",
                    )}
                </FormAlert>
              ) : undefined
            }
          >
            <Button kind="primary" busy={busy} onPress={save}>
              {t("settings.account.auto_delete.save")}
            </Button>
          </SubmitBar>
        }
      >
        {limit !== undefined && bound !== undefined && (
          <StatusSummary
            icon="caution"
            title={t("settings.account.auto_delete.bound", {
              state: bound,
              date: limit.enforce_from === undefined ? "" : day(limit.enforce_from),
            })}
            about={t("settings.account.auto_delete.bound.about", { state: bound, days: limit.max_age_days })}
          />
        )}
        <SettingsSection
          heading={whenHeading}
          about={
            limit === undefined
              ? t("settings.account.auto_delete.when.about")
              : t("settings.account.auto_delete.when.about_limit", { days: limit.max_age_days })
          }
        >
          <SettingRow
            id={enabledId}
            label={t("settings.account.auto_delete.enabled")}
            isDisabled={limit !== undefined}
            reason={t("settings.account.auto_delete.enabled.fixed")}
            form
          >
            <Select
              name="enabled"
              options={(["off", "on"] as const).map((value) => ({
                value,
                label: t("settings.account.auto_delete.enabled.choice", { enabled: value }),
              }))}
              value={form.enabled ? "on" : "off"}
              describedBy={enabledError === undefined ? undefined : errorOf(enabledId)}
              onChange={(value) => {
                change({ enabled: value === "on" });
              }}
              isDisabled={busy || limit !== undefined}
            />
          </SettingRow>
          {enabledError !== undefined && <FieldError id={errorOf(enabledId)}>{enabledError}</FieldError>}
          <SettingRow
            id={daysId}
            label={t("settings.account.auto_delete.days")}
            about={t("settings.account.auto_delete.days.about", {
              limited: limit === undefined ? "no" : "yes",
              max: maxDays,
            })}
            form
          >
            <NumberField
              name="min_age_days"
              value={form.min_age_days}
              min={autoDeleteLimits.minDays}
              max={maxDays}
              unit={t("settings.account.auto_delete.days.unit")}
              describedBy={daysError === undefined ? undefined : errorOf(daysId)}
              onChange={(n) => {
                change({ min_age_days: n });
              }}
              isDisabled={busy}
            />
          </SettingRow>
          {daysError !== undefined && <FieldError id={errorOf(daysId)}>{daysError}</FieldError>}
        </SettingsSection>
        <SettingsSection
          heading={keptHeading}
          about={t("settings.account.auto_delete.kept.about", { limited: limit === undefined ? "no" : "yes" })}
        >
          <Stack>
            {kept.map(([keep, allow, word]) => {
              const allowed = limit === undefined || limit[allow] !== false;
              return (
                <CheckField
                  key={keep}
                  id={keepId(keep)}
                  name={keep}
                  value="1"
                  label={t("settings.account.auto_delete.keep", {
                    kind: word,
                    // Fewer pins than an account has keep only the most recently pinned.
                    pins: keep === "keep_pinned" && limit !== undefined && limit.kept_pins < 5 ? limit.kept_pins : 0,
                  })}
                  checked={allowed && form[keep]}
                  onChange={(on) => {
                    change({ [keep]: on });
                  }}
                  describedBy={keptError === undefined ? undefined : keptErrorId}
                  touch
                  disabled={busy || !allowed}
                />
              );
            })}
          </Stack>
          {keptError !== undefined && <FieldError id={keptErrorId}>{keptError}</FieldError>}
        </SettingsSection>
        <SettingsSection
          heading={t("settings.account.auto_delete.section.audiences")}
          about={t("settings.account.auto_delete.audiences.about", { allowed: byAudience ? "yes" : "no" })}
        >
          <Stack>
            {offered.map((v) => (
              <CheckField
                key={v}
                id={audienceId(v)}
                name="keep_visibilities"
                value={v}
                label={t(`compose.visibility.${v}`)}
                checked={byAudience && form.keep_visibilities.includes(v)}
                onChange={(on) => {
                  change({
                    keep_visibilities: on
                      ? [...form.keep_visibilities, v]
                      : form.keep_visibilities.filter((kept_) => kept_ !== v),
                  });
                }}
                describedBy={audiencesError === undefined ? undefined : audiencesErrorId}
                touch
                disabled={busy || !byAudience}
              />
            ))}
          </Stack>
          {audiencesError !== undefined && <FieldError id={audiencesErrorId}>{audiencesError}</FieldError>}
        </SettingsSection>
        <SettingsSection
          heading={t("settings.account.auto_delete.section.thresholds")}
          about={thresholds ? undefined : t("settings.account.auto_delete.thresholds.fixed")}
        >
          {threshold(boostsId, "boosts", form.keep_min_boosts)}
          {threshold(favouritesId, "favourites", form.keep_min_favourites)}
        </SettingsSection>
        <SettingsSection
          heading={t("settings.account.auto_delete.section.count")}
          about={t("settings.account.auto_delete.count.about")}
        >
          {/* Its place is a polite live region from the start: the count is read when it comes. */}
          <Box role="status">
            {counted.status !== "error" && (
              <Box as="p" breaks="phrase" tone={counted.preview === undefined ? "muted" : "text"}>
                {!form.enabled
                  ? t("settings.account.auto_delete.count.off")
                  : form.min_age_days === null
                    ? t("settings.account.auto_delete.count.no_age")
                    : counted.preview === undefined
                      ? t("settings.account.auto_delete.count.loading")
                      : t("settings.account.auto_delete.count", {
                          count: counted.preview.count,
                          more: counted.preview.more ? "yes" : "no",
                        })}
              </Box>
            )}
          </Box>
          {counted.status === "error" && (
            <SettingsEmpty
              action={
                <Button size="timeline" touch onPress={counted.retry}>
                  {t("settings.retry")}
                </Button>
              }
            >
              {t("settings.account.auto_delete.count.failed")}
            </SettingsEmpty>
          )}
          {counted.preview !== undefined && counted.preview.examples.length > 0 && (
            <Stack gap="hair">
              <Box as="p" text="caption" tone="muted">
                {oldest}
              </Box>
              <SettingsList label={oldest}>
                {counted.preview.examples.map((status) => (
                  <SettingsListRow
                    key={status.id}
                    title={t("settings.account.auto_delete.count.post", { date: date(status.created_at) })}
                    caption={excerptOf(status.content)}
                    href={statusPath(status)}
                  />
                ))}
              </SettingsList>
            </Stack>
          )}
        </SettingsSection>
      </SettingsScreen>
      <ModalPresence>
        {leaving !== undefined && (
          <ConfirmDialog
            key="leave"
            title={t("settings.account.auto_delete.leave.title")}
            cancel={t("settings.account.auto_delete.leave.cancel")}
            confirm={t("settings.account.auto_delete.leave.confirm")}
            onCancel={leaving.onStay}
            onConfirm={leaving.onLeave}
          >
            {t("settings.account.auto_delete.leave.text")}
          </ConfirmDialog>
        )}
      </ModalPresence>
    </>
  );
}

/** The screen: the form once the reader's policy is read, and until then that it loads. */
export function AccountAutoDelete({ onBack, onUnsaved, leaving }: AccountAutoDeleteProps) {
  const { t } = stringsIn(useLocale().locale);
  const { settings, status, retry } = useAutoDeleteRead();
  if (settings === undefined)
    return (
      <SettingsScreen
        sub
        title={t("settings.account.auto_delete")}
        back={<BackButton onPress={onBack}>{t("settings.entry.account")}</BackButton>}
        notice={
          <ReadingNotice
            words={{
              loading: t("settings.account.auto_delete.loading"),
              failed: t("settings.account.auto_delete.failed"),
            }}
            status={status === "error" ? "failed" : "loading"}
            retry={retry}
          />
        }
      />
    );
  return <PolicyForm settings={settings} onBack={onBack} onUnsaved={onUnsaved} leaving={leaving} />;
}
