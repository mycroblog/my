// SPDX-License-Identifier: AGPL-3.0-only
import {
  Box,
  Button,
  Popover,
  PopoverTrigger,
  Radio,
  RadioGroup,
  Tooltip,
  TooltipTrigger,
  VisibilityIcon,
  VisuallyHidden,
} from "@my/ui";
import { useId, useState } from "react";
import { useT } from "../../../lib/i18n";
import type { PostingLimits, Visibility } from "../api/compose";
import { isVisibility, visibilities, visibilityDescription, visibilityLabel } from "../hooks/visibilityMessages";

export type VisibilityPickerProps = {
  value: Visibility;
  onChange: (value: Visibility) => void;
  /** The account's posting limits: the visibilities outside them are unavailable. Null keeps every one. */
  limits: PostingLimits | null;
  isDisabled?: boolean;
};

/**
 * The visibility of the post: a button of the form's tools that shows the current choice with its icon (named "Visibility:
 * <choice>" and described by who can see it) and opens a popover with the five
 * visibilities as a radio group, each with who can see it (ADR-0017; WAI-ARIA APG radio group and dialog
 * patterns). Choosing does not close the popover, because the arrow keys choose as they move.
 * While a restriction narrows what the account can post, the wider visibilities are disabled and the
 * group says why (data-model/05-states-and-policies.md §5.4).
 */
export function VisibilityPicker({ value, onChange, limits, isDisabled = false }: VisibilityPickerProps) {
  const t = useT();
  const descriptionId = useId();
  const [isOpen, setIsOpen] = useState(false);
  const title = t("client:compose.visibility.title");
  const ceilingHint =
    limits === null
      ? undefined
      : t("client:compose.visibility.ceiling", { visibility: t(visibilityLabel[limits.visibility_ceiling]) });
  const buttonLabel = t("client:compose.visibility.button", { visibility: t(visibilityLabel[value]) });
  return (
    <PopoverTrigger isOpen={isOpen} onOpenChange={setIsOpen}>
      {/* The tooltip says the control's name and value like the icon buttons' (G12) and stays closed while the popover is open. */}
      <TooltipTrigger isDisabled={isOpen}>
        <Button
          variant="ghost"
          size="toolbar"
          aria-label={buttonLabel}
          aria-describedby={descriptionId}
          isDisabled={isDisabled}
        >
          <VisibilityIcon visibility={value} />
          {t(visibilityLabel[value])}
        </Button>
        <Tooltip>{buttonLabel}</Tooltip>
      </TooltipTrigger>
      <VisuallyHidden id={descriptionId}>{t(visibilityDescription[value])}</VisuallyHidden>
      <Popover aria-label={title} placement="bottom start">
        {({ close }) => (
          <>
            <RadioGroup
              label={title}
              description={ceilingHint}
              value={value}
              onChange={(next) => {
                if (isVisibility(next)) onChange(next);
              }}
            >
              {visibilities.map((visibility) => (
                <Radio
                  key={visibility}
                  value={visibility}
                  description={t(visibilityDescription[visibility])}
                  isDisabled={limits !== null && !limits.visibilities.includes(visibility)}
                >
                  {t(visibilityLabel[visibility])}
                </Radio>
              ))}
            </RadioGroup>
            <Box self="end">
              <Button size="sm" variant="primary" onPress={close}>
                {t("client:compose.visibility.done")}
              </Button>
            </Box>
          </>
        )}
      </Popover>
    </PopoverTrigger>
  );
}
