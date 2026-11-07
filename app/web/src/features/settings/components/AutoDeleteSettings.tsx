// SPDX-License-Identifier: AGPL-3.0-only
// The automatic deletion of the user's own old posts (FR-118): turning it on, the age after which posts go, and the
// exceptions that keep them. The form saves the whole policy at once; the server deletes the posts in its hourly job.
// While the server maximum applies to the user (ADR-0151), a section before the form's says what it is and which
// exceptions it allows, and, when it binds the user, from when and how it deletes their posts and how many go then.
import {
  Button,
  Callout,
  Checkbox,
  ConfirmDialog,
  FormActionBar,
  Grid,
  Heading,
  Hint,
  InlineError,
  Section,
  Select,
  SelectItem,
  Stack,
  StatTile,
  Text,
} from "@my/ui";
import { useId } from "react";
import { useIntl } from "react-intl";
import { visibilities, visibilityLabel } from "../../compose";
import { CompactStatus } from "../../status";
import { type ClientMessageKey, type Translate, useT } from "../../../lib/i18n";
import { type AutoDeletePolicy, type AutoDeleteServerLimit, policyOf } from "../api/autoDelete";
import { type AutoDeleteFlag, flagAllowed, useAutoDeleteForm } from "../hooks/useAutoDeleteForm";
import { useAutoDeletePreview } from "../hooks/useAutoDeletePreview";
import { useUnsavedChangesGuard } from "../hooks/useUnsavedChangesGuard";

/** The exceptions that keep a post the author marked or that has something in it, in the order of the form. */
const flags: readonly { id: AutoDeleteFlag; label: ClientMessageKey }[] = [
  { id: "keep_pinned", label: "client:settings.auto_delete.keep.pinned" },
  { id: "keep_bookmarked", label: "client:settings.auto_delete.keep.bookmarked" },
  { id: "keep_favourited", label: "client:settings.auto_delete.keep.favourited" },
  { id: "keep_media", label: "client:settings.auto_delete.keep.media" },
  { id: "keep_polls", label: "client:settings.auto_delete.keep.polls" },
];

/** The words of a threshold of boosts or favourites: a number of times, or none. */
function thresholdLabel(t: Translate, count: number): string {
  return count === 0
    ? t("client:settings.auto_delete.threshold.off")
    : t("client:settings.auto_delete.threshold.count", { count });
}

/** The props that mark a field whose value cannot be saved, with why under it; none when it can. */
function invalid(error: string | undefined) {
  return error === undefined ? {} : { isInvalid: true, errorMessage: error };
}

/**
 * The automatic deletion in the sections of its screen requirements (web-client/screens/settings-auto-delete.md): the
 * server maximum when it applies, what it deletes and when, the kinds of posts kept, the visibilities kept, the posts
 * kept for their reactions, and what those conditions delete now. The save button and the policy it saved stay at the
 * bottom edge of the screen (`FormActionBar` `sticky`).
 */
export function AutoDeleteSettings() {
  const t = useT();
  const form = useAutoDeleteForm();
  const { limit, locked } = form;
  const notAllowed = t("client:settings.auto_delete.limit.not_allowed");
  return (
    <>
      <Stack as="form" onSubmit={form.submit} gap="8">
        <Hint size="inherit">{t("client:settings.auto_delete.description")}</Hint>
        {limit !== undefined && <ServerLimitSection limit={limit} />}
        <Section title={t("client:settings.auto_delete.policy")}>
          <Stack gap="6">
            <Checkbox
              isSelected={form.policy.enabled}
              onChange={form.setEnabled}
              isDisabled={locked.enabled}
              description={
                limit === undefined
                  ? t("client:settings.auto_delete.enabled.description")
                  : t("client:settings.auto_delete.limit.enabled_locked")
              }
              {...invalid(form.fieldError("enabled"))}
            >
              {t("client:settings.auto_delete.enabled")}
            </Checkbox>
            <Select
              label={t("client:settings.auto_delete.age")}
              description={
                limit === undefined
                  ? t("client:settings.auto_delete.age.description")
                  : t("client:settings.auto_delete.limit.age", { days: limit.max_age_days })
              }
              {...invalid(form.fieldError("min_age_days"))}
              value={String(form.policy.min_age_days)}
              onChange={(key) => {
                if (typeof key === "string") form.setMinAgeDays(Number(key));
              }}
              items={form.ageChoices.map((days) => ({
                id: String(days),
                name: t("client:settings.auto_delete.age.days", { count: days }),
              }))}
            >
              {(choice) => (
                <SelectItem id={choice.id} textValue={choice.name}>
                  {choice.name}
                </SelectItem>
              )}
            </Select>
          </Stack>
        </Section>
        <CheckboxSection
          title={t("client:settings.auto_delete.keep")}
          description={t("client:settings.auto_delete.keep.description")}
          items={flags.map((flag) => ({
            id: flag.id,
            label: t(flag.label),
            selected: form.policy[flag.id],
            onChange: (selected: boolean) => {
              form.setFlag(flag.id, selected);
            },
            locked: locked.flag(flag.id),
            description:
              pinnedNote(t, limit, flag.id) === undefined
                ? locked.flag(flag.id)
                  ? notAllowed
                  : undefined
                : t("client:settings.auto_delete.limit.pins.note", { count: limit?.kept_pins ?? maxPins }),
            error: form.fieldError(flag.id),
          }))}
        />
        <CheckboxSection
          title={t("client:settings.auto_delete.visibilities")}
          description={t("client:settings.auto_delete.visibilities.description")}
          items={visibilities.map((visibility) => ({
            id: visibility,
            label: t(visibilityLabel[visibility]),
            selected: form.policy.keep_visibilities.includes(visibility),
            onChange: (selected: boolean) => {
              form.setKeepVisibility(visibility, selected);
            },
            locked: locked.visibility(visibility),
            description: undefined,
            error: undefined,
          }))}
          note={limit !== undefined && !limit.allow_visibilities ? notAllowed : undefined}
          error={form.fieldError("keep_visibilities")}
        />
        <Section title={t("client:settings.auto_delete.reactions")}>
          <Stack gap="6">
            <Select
              label={t("client:settings.auto_delete.min_boosts")}
              value={String(form.policy.keep_min_boosts)}
              onChange={(key) => {
                if (typeof key === "string") form.setMinBoosts(Number(key));
              }}
              isDisabled={locked.minBoosts}
              {...(limit !== undefined && !limit.allow_thresholds ? { description: notAllowed } : {})}
              {...invalid(form.fieldError("keep_min_boosts"))}
              items={form.boostChoices.map((count) => ({ id: String(count), name: thresholdLabel(t, count) }))}
            >
              {(choice) => (
                <SelectItem id={choice.id} textValue={choice.name}>
                  {choice.name}
                </SelectItem>
              )}
            </Select>
            <Select
              label={t("client:settings.auto_delete.min_favourites")}
              value={String(form.policy.keep_min_favourites)}
              onChange={(key) => {
                if (typeof key === "string") form.setMinFavourites(Number(key));
              }}
              isDisabled={locked.minFavourites}
              {...(limit !== undefined && !limit.allow_thresholds ? { description: notAllowed } : {})}
              {...invalid(form.fieldError("keep_min_favourites"))}
              items={form.favouriteChoices.map((count) => ({ id: String(count), name: thresholdLabel(t, count) }))}
            >
              {(choice) => (
                <SelectItem id={choice.id} textValue={choice.name}>
                  {choice.name}
                </SelectItem>
              )}
            </Select>
          </Stack>
        </Section>
        <AutoDeletePreviewSection policy={form.policy} limited={limit !== undefined} />
        <FormActionBar sticky result={form.result}>
          <Button type="submit" variant="primary" isPending={form.isSaving}>
            {t("client:settings.auto_delete.save")}
          </Button>
        </FormActionBar>
      </Stack>
      {/* Outside the form: React bubbles the dialog's own submit event through its portal to an enclosing form. */}
      <UnsavedChangesDialog hasChanges={form.hasChanges} />
    </>
  );
}

/**
 * What the conditions above delete now, before they are saved (ADR-0110 P3): the count as a figure, then the oldest of
 * those posts. The figure is a live region, so a change of a condition is answered where it is read; while the next
 * count is on its way the previous one stays, and the section is busy.
 */
function AutoDeletePreviewSection({ policy, limited }: { policy: AutoDeletePolicy; limited: boolean }) {
  const t = useT();
  const preview = useAutoDeletePreview(policy);
  const examplesId = useId();
  return (
    <Section
      title={t("client:settings.auto_delete.preview")}
      description={
        limited
          ? t("client:settings.auto_delete.preview.description.limited")
          : t("client:settings.auto_delete.preview.description")
      }
      busy={preview.status === "loading" || (preview.status === "done" && preview.pending)}
    >
      <Stack gap="4">
        {preview.status === "error" ? (
          <InlineError alert size="inherit">
            {t("client:settings.auto_delete.preview.failed", { reason: preview.message })}
          </InlineError>
        ) : (
          <div role="status" aria-atomic="true">
            <StatTile label={t("client:settings.auto_delete.preview.count")} value={previewValue(t, preview)} />
          </div>
        )}
        {preview.status === "done" && preview.examples.length > 0 && (
          <Stack gap="2">
            <Heading level={3} id={examplesId}>
              {t("client:settings.auto_delete.preview.examples")}
            </Heading>
            <Stack as="ul" aria-labelledby={examplesId} gap="2">
              {preview.examples.map((status) => (
                <li key={status.id}>
                  <CompactStatus status={status} label={t("client:settings.auto_delete.preview.example")} />
                </li>
              ))}
            </Stack>
          </Stack>
        )}
      </Stack>
    </Section>
  );
}

/** The value of a count of the preview: the number, with "+" past the server's cap, or that it is being counted. */
function previewValue(t: Translate, preview: ReturnType<typeof useAutoDeletePreview>): string {
  if (preview.status !== "done") return t("client:settings.auto_delete.preview.counting");
  return t("client:settings.auto_delete.preview.value", { count: preview.count, more: String(preview.more) });
}

/** The note of the pins under a maximum that keeps only the newest of them; undefined otherwise. */
function pinnedNote(t: Translate, limit: AutoDeleteServerLimit | undefined, flag: AutoDeleteFlag): string | undefined {
  if (flag !== "keep_pinned" || limit === undefined || !limit.allow_pinned || limit.kept_pins >= maxPins)
    return undefined;
  return t("client:settings.auto_delete.limit.pins", { count: limit.kept_pins });
}

/** The most pins of an account: a maximum that keeps this many pins keeps every pinned post (ADR-0151 §3). */
const maxPins = 5;

/**
 * The server maximum as it applies to the user (ADR-0151 §3, §5): the most days a post is kept and the exceptions the
 * operator allows, as a list. While it binds the user, a callout says from when (or that it is soon, until the hourly
 * job has told them) it deletes their posts, how (the effective policy) and how many posts go then, counted by the
 * preview of the effective policy; saving settings within the maximum ends it.
 */
function ServerLimitSection({ limit }: { limit: AutoDeleteServerLimit }) {
  const t = useT();
  const allowedId = useId();
  const allowed = [
    ...flags
      .filter((flag) => flagAllowed(limit, flag.id))
      .map((flag) => ({ id: flag.id, label: pinnedNote(t, limit, flag.id) ?? t(flag.label) })),
    ...(limit.allow_visibilities
      ? [{ id: "visibilities", label: t("client:settings.auto_delete.limit.allowed.visibilities") }]
      : []),
    ...(limit.allow_thresholds
      ? [{ id: "thresholds", label: t("client:settings.auto_delete.limit.allowed.thresholds") }]
      : []),
  ];
  return (
    <Section title={t("client:settings.auto_delete.limit.title")}>
      <Stack gap="4">
        <p>{t("client:settings.auto_delete.limit.description", { days: limit.max_age_days })}</p>
        <Stack gap="2">
          <Heading level={3} id={allowedId}>
            {t("client:settings.auto_delete.limit.allowed")}
          </Heading>
          {allowed.length === 0 ? (
            <Hint as="p">{t("client:settings.auto_delete.limit.allowed.none")}</Hint>
          ) : (
            <Stack as="ul" marker="disc" gap="1" aria-labelledby={allowedId}>
              {allowed.map((item) => (
                <li key={item.id}>{item.label}</li>
              ))}
            </Stack>
          )}
        </Stack>
        {limit.bound && <BoundNotice limit={limit} />}
      </Stack>
    </Section>
  );
}

/** What the effective policy keeps, as the lines of a list: its exceptions, with the cap of the pins. */
function effectiveKeeps(t: Translate, limit: AutoDeleteServerLimit, formatList: (items: string[]) => string): string[] {
  const { effective } = limit;
  const keeps = flags
    .filter((flag) => effective[flag.id])
    .map((flag) => pinnedNote(t, limit, flag.id) ?? t(flag.label));
  if (effective.keep_visibilities.length > 0)
    keeps.push(
      t("client:settings.auto_delete.limit.keeps.visibilities", {
        visibilities: formatList(effective.keep_visibilities.map((visibility) => t(visibilityLabel[visibility]))),
      }),
    );
  if (effective.keep_min_boosts > 0)
    keeps.push(t("client:settings.auto_delete.limit.keeps.min_boosts", { count: effective.keep_min_boosts }));
  if (effective.keep_min_favourites > 0)
    keeps.push(t("client:settings.auto_delete.limit.keeps.min_favourites", { count: effective.keep_min_favourites }));
  return keeps;
}

/**
 * That the maximum binds the user: from when it deletes their posts, by which policy, how many posts go then, and how to
 * keep deciding for themselves.
 */
function BoundNotice({ limit }: { limit: AutoDeleteServerLimit }) {
  const t = useT();
  const intl = useIntl();
  const preview = useAutoDeletePreview(policyOf(limit.effective));
  const keepsId = useId();
  const keeps = effectiveKeeps(t, limit, (items) => intl.formatList(items, { type: "disjunction" }));
  const from = limit.enforce_from;
  return (
    <Callout>
      <Text as="p" weight="semibold">
        {from === undefined
          ? t("client:settings.auto_delete.limit.bound.soon")
          : intl.formatMessage(
              { id: "client:settings.auto_delete.limit.bound.dated" },
              {
                date: (
                  <time key="from" dateTime={from}>
                    {intl.formatDate(from, { dateStyle: "long" })}
                  </time>
                ),
              },
            )}
      </Text>
      <p id={keepsId}>
        {t("client:settings.auto_delete.limit.bound.effective", { days: limit.effective.min_age_days })}
      </p>
      {keeps.length === 0 ? (
        <p>{t("client:settings.auto_delete.limit.bound.keeps_none")}</p>
      ) : (
        <Stack as="ul" marker="disc" gap="1" aria-labelledby={keepsId}>
          {keeps.map((keep) => (
            <li key={keep}>{keep}</li>
          ))}
        </Stack>
      )}
      {preview.status === "error" ? (
        <InlineError size="inherit">
          {t("client:settings.auto_delete.preview.failed", { reason: preview.message })}
        </InlineError>
      ) : (
        <div role="status" aria-atomic="true">
          <StatTile label={t("client:settings.auto_delete.limit.bound.count")} value={previewValue(t, preview)} />
        </div>
      )}
      <p>{t("client:settings.auto_delete.limit.bound.fix")}</p>
    </Callout>
  );
}

type CheckboxItem = {
  id: string;
  label: string;
  selected: boolean;
  onChange: (selected: boolean) => void;
  /** Whether the server maximum keeps the checkbox where it is. */
  locked: boolean;
  /** A note under the checkbox: what the maximum allows of it. */
  description: string | undefined;
  /** Why its value cannot be saved; undefined when it can. */
  error: string | undefined;
};

/**
 * A section of checkboxes, each keeping a kind of post: a group (the APG checkbox pattern) named by the section's
 * heading and described by its description, the note of the server maximum on the whole group and why the group's
 * values cannot be saved.
 */
function CheckboxSection({
  title,
  description,
  items,
  note,
  error,
}: {
  title: string;
  description: string;
  items: readonly CheckboxItem[];
  note?: string | undefined;
  error?: string | undefined;
}) {
  const headingId = useId();
  const descriptionId = useId();
  const noteId = useId();
  const errorId = useId();
  const describedBy = [descriptionId, note === undefined ? null : noteId, error === undefined ? null : errorId]
    .filter((id) => id !== null)
    .join(" ");
  return (
    <Section title={title} headingId={headingId}>
      <Stack role="group" aria-labelledby={headingId} aria-describedby={describedBy} container shrink gap="2">
        <Hint id={descriptionId}>{description}</Hint>
        {note !== undefined && <Hint id={noteId}>{note}</Hint>}
        <Grid as="ul" gap="6" rowGap="1">
          {items.map((item) => (
            <li key={item.id}>
              <Checkbox
                isSelected={item.selected}
                onChange={item.onChange}
                isDisabled={item.locked}
                {...(item.description === undefined ? {} : { description: item.description })}
                {...invalid(item.error)}
                {...(error !== undefined && item.selected ? { isInvalid: true } : {})}
              >
                {item.label}
              </Checkbox>
            </li>
          ))}
        </Grid>
        {error !== undefined && (
          <InlineError id={errorId} size="inherit">
            {error}
          </InlineError>
        )}
      </Stack>
    </Section>
  );
}

/**
 * Asks before another page replaces the form with unsaved edits (the APG alert dialog pattern): focus starts on
 * "Keep editing", which stays on the form like Esc does.
 */
function UnsavedChangesDialog({ hasChanges }: { hasChanges: boolean }) {
  const t = useT();
  const guard = useUnsavedChangesGuard(hasChanges);
  return (
    <ConfirmDialog
      isOpen={guard.isAsking}
      onOpenChange={(isOpen) => {
        if (!isOpen) guard.stay();
      }}
      title={t("client:settings.auto_delete.unsaved.title")}
      description={t("client:settings.auto_delete.unsaved.description")}
      confirmLabel={t("client:settings.auto_delete.unsaved.leave")}
      cancelLabel={t("client:settings.auto_delete.unsaved.stay")}
      onConfirm={guard.leave}
    />
  );
}
