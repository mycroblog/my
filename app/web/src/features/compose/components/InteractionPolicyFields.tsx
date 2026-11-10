// SPDX-License-Identifier: AGPL-3.0-only
import { Checkbox, Select, SelectItem, Stack } from "@my/ui";
import { type ClientMessageKey, useT } from "../../../lib/i18n";
import {
  audienceLabel,
  type InteractionKind,
  type InteractionPolicy,
  type InteractionRule,
  interactionAudiences,
  interactionKinds,
  isInteractionAudience,
  kindLabel,
  visibilityLimit,
  withApproval,
  withAutomatic,
} from "../hooks/interactionPolicy";
import type { Visibility } from "../api/compose";

export type InteractionPolicyFieldsProps = {
  value: InteractionPolicy;
  /** Called with the new rule of the kind the person changed. */
  onChange: (kind: InteractionKind, rule: InteractionRule) => void;
  /**
   * The visibility of the post, whose limits disable the kinds they decide (a followers-only post is boosted and
   * quoted by its author only); left out for the defaults of new posts.
   */
  visibility?: Visibility;
  /** The error of each kind's last change, for the settings; none by default. */
  errors?: Partial<Record<InteractionKind, string>>;
};

const approvalLabel: Readonly<Record<InteractionKind, ClientMessageKey>> = {
  reply: "client:compose.interaction.reply.approval",
  announce: "client:compose.interaction.announce.approval",
  quote: "client:compose.interaction.quote.approval",
};

/**
 * Who may reply to, boost, and quote a post (ADR-0155 §2): for each kind, a select of the audience approved at once
 * and a checkbox that lets everyone else ask for the author's approval (the manual audience). The form, the menu of
 * the viewer's own post and the settings share it. A kind that the post's visibility decides is disabled and says why.
 */
export function InteractionPolicyFields({ value, onChange, visibility, errors }: InteractionPolicyFieldsProps) {
  const t = useT();
  return (
    <Stack gap="4">
      {interactionKinds.map((kind) => {
        const rule = value[kind];
        const limit = visibility === undefined ? null : visibilityLimit(kind, visibility);
        const error = errors?.[kind];
        const description =
          limit === "author"
            ? t("client:compose.interaction.limit.author")
            : limit === "none"
              ? t("client:compose.interaction.limit.none")
              : kind === "reply"
                ? t("client:compose.interaction.reply.description")
                : undefined;
        return (
          <Stack key={kind} gap="2">
            <Select
              label={t(kindLabel[kind])}
              {...(description !== undefined && { description })}
              items={interactionAudiences.map((id) => ({ id, name: t(audienceLabel[id]) }))}
              value={rule.automatic}
              isDisabled={limit !== null}
              isInvalid={error !== undefined}
              errorMessage={error}
              onChange={(audience) => {
                if (isInteractionAudience(audience)) onChange(kind, withAutomatic(rule, audience));
              }}
            >
              {(option) => (
                <SelectItem id={option.id} textValue={option.name}>
                  {option.name}
                </SelectItem>
              )}
            </Select>
            <Checkbox
              isSelected={rule.manual !== undefined}
              isDisabled={limit !== null || rule.automatic === "everyone"}
              onChange={(asks) => {
                onChange(kind, withApproval(rule, asks));
              }}
            >
              {t(approvalLabel[kind])}
            </Checkbox>
          </Stack>
        );
      })}
    </Stack>
  );
}
