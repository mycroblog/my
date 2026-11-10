// SPDX-License-Identifier: AGPL-3.0-only
import { Box, Button, Popover, PopoverTrigger, RepliesIcon, Text, Tooltip, TooltipTrigger } from "@my/ui";
import { useState } from "react";
import { useT } from "../../../lib/i18n";
import type { Visibility } from "../api/compose";
import {
  type InteractionKind,
  type InteractionPolicy,
  type InteractionRule,
  policySummary,
} from "../hooks/interactionPolicy";
import { InteractionPolicyFields } from "./InteractionPolicyFields";

export type InteractionPolicyPickerProps = {
  /** The policy the post gets: the chosen rules over the defaults of the preferences. */
  value: InteractionPolicy;
  onChange: (kind: InteractionKind, rule: InteractionRule) => void;
  visibility: Visibility;
};

/**
 * Who may reply to, boost, and quote the post (ADR-0155 §10): a `ghost` icon button of the form's tools, named by what
 * it chooses and showing the effective policy (the rules with what the visibility decides) as its tooltip, that opens
 * a popover with the choices of each kind (InteractionPolicyFields; WAI-ARIA APG dialog pattern). Choosing does not
 * close the popover; "Done" does.
 */
export function InteractionPolicyPicker({ value, onChange, visibility }: InteractionPolicyPickerProps) {
  const t = useT();
  const [isOpen, setIsOpen] = useState(false);
  const title = t("client:compose.interaction.title");
  const summary = policySummary(t, value, visibility);
  return (
    <PopoverTrigger isOpen={isOpen} onOpenChange={setIsOpen}>
      {/* The tooltip says the control's name and value like the other tools' (G12) and stays closed while the popover is open. */}
      <TooltipTrigger isDisabled={isOpen}>
        <Button variant="ghost" size="toolbar" aria-label={t("client:compose.interaction.button", { summary })}>
          <RepliesIcon />
        </Button>
        <Tooltip>{summary}</Tooltip>
      </TooltipTrigger>
      <Popover aria-label={title} placement="bottom start">
        {({ close }) => (
          <>
            <Text as="p" size="sm" weight="medium">
              {title}
            </Text>
            <InteractionPolicyFields value={value} onChange={onChange} visibility={visibility} />
            <Box self="end">
              <Button size="sm" variant="primary" onPress={close}>
                {t("client:compose.interaction.done")}
              </Button>
            </Box>
          </>
        )}
      </Popover>
    </PopoverTrigger>
  );
}
