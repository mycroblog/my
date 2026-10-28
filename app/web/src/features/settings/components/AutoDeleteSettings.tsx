// SPDX-License-Identifier: AGPL-3.0-only
// The automatic deletion of the user's own old posts (FR-118): turning it on, the age after which posts go, and the
// exceptions that keep them. The form saves the whole policy at once; the server deletes the posts in its hourly job.
import {
  Button,
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
} from "@my/ui";
import { useId } from "react";
import { visibilities, visibilityLabel } from "../../compose";
import { CompactStatus } from "../../status";
import { type ClientMessageKey, useT } from "../../../lib/i18n";
import type { AutoDeletePolicy } from "../api/autoDelete";
import { type AutoDeleteFlag, useAutoDeleteForm } from "../hooks/useAutoDeleteForm";
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

/**
 * The automatic deletion in the sections of its screen requirements (web-client/screens/settings-auto-delete.md): what
 * it deletes and when, the kinds of posts kept, the visibilities kept, the posts kept for their reactions, and what
 * those conditions delete now. The
 * save button and the policy it saved stay at the bottom edge of the screen (`FormActionBar` `sticky`).
 */
export function AutoDeleteSettings() {
  const t = useT();
  const form = useAutoDeleteForm();
  const thresholdLabel = (count: number) =>
    count === 0
      ? t("client:settings.auto_delete.threshold.off")
      : t("client:settings.auto_delete.threshold.count", { count });
  return (
    <>
      <Stack as="form" onSubmit={form.submit} gap="8">
        <Hint size="inherit">{t("client:settings.auto_delete.description")}</Hint>
        <Section title={t("client:settings.auto_delete.policy")}>
          <Stack gap="6">
            <Checkbox
              isSelected={form.policy.enabled}
              onChange={form.setEnabled}
              description={t("client:settings.auto_delete.enabled.description")}
            >
              {t("client:settings.auto_delete.enabled")}
            </Checkbox>
            <Select
              label={t("client:settings.auto_delete.age")}
              description={t("client:settings.auto_delete.age.description")}
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
          }))}
        />
        <Section title={t("client:settings.auto_delete.reactions")}>
          <Stack gap="6">
            <Select
              label={t("client:settings.auto_delete.min_boosts")}
              value={String(form.policy.keep_min_boosts)}
              onChange={(key) => {
                if (typeof key === "string") form.setMinBoosts(Number(key));
              }}
              items={form.boostChoices.map((count) => ({ id: String(count), name: thresholdLabel(count) }))}
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
              items={form.favouriteChoices.map((count) => ({ id: String(count), name: thresholdLabel(count) }))}
            >
              {(choice) => (
                <SelectItem id={choice.id} textValue={choice.name}>
                  {choice.name}
                </SelectItem>
              )}
            </Select>
          </Stack>
        </Section>
        <AutoDeletePreviewSection policy={form.policy} />
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
function AutoDeletePreviewSection({ policy }: { policy: AutoDeletePolicy }) {
  const t = useT();
  const preview = useAutoDeletePreview(policy);
  const examplesId = useId();
  let value = t("client:settings.auto_delete.preview.counting");
  if (preview.status === "done")
    value = t("client:settings.auto_delete.preview.value", { count: preview.count, more: String(preview.more) });
  return (
    <Section
      title={t("client:settings.auto_delete.preview")}
      description={t("client:settings.auto_delete.preview.description")}
      busy={preview.status === "loading" || (preview.status === "done" && preview.pending)}
    >
      <Stack gap="4">
        {preview.status === "error" ? (
          <InlineError alert size="inherit">
            {t("client:settings.auto_delete.preview.failed", { reason: preview.message })}
          </InlineError>
        ) : (
          <div role="status" aria-atomic="true">
            <StatTile label={t("client:settings.auto_delete.preview.count")} value={value} />
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

type CheckboxItem = { id: string; label: string; selected: boolean; onChange: (selected: boolean) => void };

/**
 * A section of checkboxes, each keeping a kind of post: a group (the APG checkbox pattern) named by the section's
 * heading and described by its description.
 */
function CheckboxSection({
  title,
  description,
  items,
}: {
  title: string;
  description: string;
  items: readonly CheckboxItem[];
}) {
  const headingId = useId();
  const descriptionId = useId();
  return (
    <Section title={title} headingId={headingId}>
      <Stack role="group" aria-labelledby={headingId} aria-describedby={descriptionId} container shrink gap="2">
        <Hint id={descriptionId}>{description}</Hint>
        <Grid as="ul" gap="6" rowGap="1">
          {items.map((item) => (
            <li key={item.id}>
              <Checkbox isSelected={item.selected} onChange={item.onChange}>
                {item.label}
              </Checkbox>
            </li>
          ))}
        </Grid>
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
