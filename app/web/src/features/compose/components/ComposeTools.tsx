// SPDX-License-Identifier: AGPL-3.0-only
import {
  Button,
  Cluster,
  FileTrigger,
  IconToggleButton,
  ImageIcon,
  LanguageIcon,
  PollIcon,
  QuoteIcon,
  Select,
  SelectItem,
  Tooltip,
  TooltipTrigger,
  VisuallyHidden,
  WarningIcon,
} from "@my/ui";
import { useId } from "react";
import { useT } from "../../../lib/i18n";
import { acceptedMediaTypes, maxAttachments } from "../hooks/policy";
import { quotePolicies, quotePolicyLabel } from "../hooks/quotePolicyMessages";
import type { ReadyComposeForm } from "../hooks/useComposeForm";
import { VisibilityPicker } from "./VisibilityPicker";

export type ComposeToolsProps = { form: ReadyComposeForm };

/**
 * The tools of the form, in one row under the text, by weight (web-client/screens/compose.md): first the visibility,
 * which decides most about the post, as a `secondary` button with the chosen level's icon and name; then, after a
 * rule, images, poll, content warning, who may quote the post (FR-114) and language as `ghost` buttons
 * (web-client/12-compose.md §12); the emoji picker is in the corner of the text (ADR-0091 §4).
 * Every control is a button of the same height, so the row stays level at every width. The icon-only buttons are named by aria-label and show the same words as a tooltip
 * (G12); each control also says its current value: the images attached (description), the poll and the warning
 * (aria-pressed), the visibility (its name and who can see it), and the quote policy and the language (the selects
 * show them).
 */
export function ComposeTools({ form }: ComposeToolsProps) {
  const t = useT();
  const countId = useId();
  const hintId = useId();
  const { draft, attachments } = form;
  const count = attachments.items.length;
  const addLabel = t("client:compose.media.add");
  const warningLabel = t("client:compose.warning.toggle");
  const pollLabel = t("client:compose.poll.toggle");
  const quotePolicyName = t(quotePolicyLabel[form.quotePolicy]);
  const languageName = form.languages.find((option) => option.id === form.language)?.name ?? form.language;
  return (
    <Cluster align="center" gap="2" shrink>
      <VisibilityPicker
        value={form.visibility}
        limits={form.limits}
        onChange={(visibility) => {
          form.edit({ visibility });
        }}
      />
      <Cluster align="center" gap="1" border="start" shrink>
        <TooltipTrigger>
          <FileTrigger
            acceptedFileTypes={acceptedMediaTypes}
            allowsMultiple
            onSelect={(files) => {
              attachments.add(files === null ? [] : Array.from(files));
            }}
          >
            <Button
              variant="ghost"
              size="toolbar"
              aria-label={addLabel}
              aria-describedby={count > 0 ? `${countId} ${hintId}` : hintId}
              isDisabled={!form.canAttach}
            >
              <ImageIcon />
            </Button>
          </FileTrigger>
          <Tooltip>{addLabel}</Tooltip>
        </TooltipTrigger>
        {count > 0 && <VisuallyHidden id={countId}>{t("client:compose.media.count", { count })}</VisuallyHidden>}
        <VisuallyHidden id={hintId}>{t("client:compose.media.hint", { max: maxAttachments })}</VisuallyHidden>
        <IconToggleButton
          label={pollLabel}
          isSelected={form.poll.draft !== null}
          isDisabled={!form.poll.canAdd}
          onChange={form.poll.toggle}
        >
          <PollIcon filled={form.poll.draft !== null} />
        </IconToggleButton>
        <IconToggleButton
          label={warningLabel}
          isSelected={draft.hasWarning}
          onChange={(hasWarning) => {
            form.edit({ hasWarning });
          }}
        >
          <WarningIcon filled={draft.hasWarning} />
        </IconToggleButton>
        <Select
          variant="toolbar"
          aria-label={t("client:compose.quote_policy.label")}
          tooltip={t("client:compose.quote_policy.tooltip", { value: quotePolicyName })}
          icon={<QuoteIcon />}
          items={quotePolicies.map((id) => ({ id, name: t(quotePolicyLabel[id]) }))}
          value={form.quotePolicy}
          onChange={(quotePolicy) => {
            if (quotePolicy === "public" || quotePolicy === "followers" || quotePolicy === "nobody")
              form.edit({ quotePolicy });
          }}
        >
          {(option) => (
            <SelectItem id={option.id} textValue={option.name}>
              {option.name}
            </SelectItem>
          )}
        </Select>
        <Select
          variant="toolbar"
          aria-label={t("client:compose.language.label")}
          tooltip={t("client:compose.language.tooltip", { value: languageName })}
          icon={<LanguageIcon />}
          items={form.languages}
          value={form.language}
          onChange={(language) => {
            if (typeof language === "string") form.edit({ language });
          }}
        >
          {(option) => (
            <SelectItem id={option.id} textValue={option.name}>
              {option.name}
            </SelectItem>
          )}
        </Select>
      </Cluster>
    </Cluster>
  );
}
