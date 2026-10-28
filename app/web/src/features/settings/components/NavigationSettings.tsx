// SPDX-License-Identifier: AGPL-3.0-only
import {
  Box,
  Button,
  Checkbox,
  Cluster,
  IconLabel,
  MoveDownIcon,
  MoveUpIcon,
  Section,
  Stack,
  Tooltip,
  TooltipTrigger,
} from "@my/ui";
import { useEffect, useId, useRef } from "react";
import { useT } from "../../../lib/i18n";
import { useTimelineAvailability } from "../../timeline";
import {
  alwaysShownEntry,
  type NavigationEntryId,
  navigationEntryLabels,
  tabBarLimit,
  useNavigationSettings,
} from "../hooks/useNavigationSettings";
import { navigationEntryIcons } from "./navigationEntryIcons";

/** The timeline each timeline item opens, which the server may not offer (FR-202). */
const timelineOf: Partial<Record<NavigationEntryId, "local" | "federated">> = { local: "local", public: "federated" };

/**
 * The items of the navigation: which ones show and in which order (web-client/09-app-shell.md §9). Each item is a row with a
 * checkbox that shows or hides it and two buttons that move it one place up or down. This is the model of the
 * WAI-ARIA APG's rearrangeable list (Up and Down buttons, a confirmation in a live region, the focus staying with
 * the moved item) with native tab stops instead of a listbox, so that every control is a plain button or checkbox.
 * A change applies at once and is kept on this browser, like the theme. Each row says whether the item is in the
 * phones' tab bar or in their menu.
 */
export function NavigationSettings() {
  const t = useT();
  const baseId = useId();
  const timelines = useTimelineAvailability();
  const { entries, move, setShown, reset } = useNavigationSettings();
  /** The button to focus once the list has rendered a move. */
  const focusAfterMoveRef = useRef<string | null>(null);
  // Runs after every render: focus is moved in the DOM, outside React, once the moved row is in its new place
  // (moving a focused element in the DOM takes the focus off it).
  useEffect(() => {
    const id = focusAfterMoveRef.current;
    if (id === null) return;
    focusAfterMoveRef.current = null;
    document.getElementById(id)?.focus();
  });
  const buttonId = (id: NavigationEntryId, step: -1 | 1) => `${baseId}-${id}-${step === -1 ? "up" : "down"}`;
  const onMove = (id: NavigationEntryId, index: number, step: -1 | 1) => {
    // The focus stays on the moved item: on the same button, or on the other one where this one is now disabled.
    const to = index + step;
    const atEnd = step === -1 ? to === 0 : to === entries.length - 1;
    focusAfterMoveRef.current = buttonId(id, atEnd ? (step === -1 ? 1 : -1) : step);
    move(id, step);
  };
  // The place on phones of each shown item: the first ones in the tab bar, the rest in the menu's sheet.
  const shownIds = entries.filter((entry) => entry.shown).map((entry) => entry.id);
  const placeOf = (id: NavigationEntryId) => {
    const timeline = timelineOf[id];
    if (timeline !== undefined && !timelines[timeline]) return t("client:settings.navigation.unavailable");
    const position = shownIds.indexOf(id);
    if (position < 0) return t("client:settings.navigation.hidden");
    return position < tabBarLimit
      ? t("client:settings.navigation.in_tab_bar")
      : t("client:settings.navigation.in_menu");
  };
  return (
    <Section
      title={t("client:settings.navigation.title")}
      description={t("client:settings.navigation.description", { count: tabBarLimit })}
    >
      <Stack as="ol" divided border="lg">
        {entries.map((entry, index) => {
          const name = t(navigationEntryLabels[entry.id]);
          const upLabel = t("client:settings.navigation.move_up", { name });
          const downLabel = t("client:settings.navigation.move_down", { name });
          return (
            <Cluster as="li" key={entry.id} wrap={false} align="start" gap="2" paddingX="3" paddingY="2">
              <Box shrink grow>
                <Checkbox
                  isSelected={entry.shown}
                  isDisabled={entry.id === alwaysShownEntry}
                  onChange={(shown) => {
                    setShown(entry.id, shown);
                  }}
                  description={placeOf(entry.id)}
                >
                  <IconLabel icon={navigationEntryIcons[entry.id]}>{name}</IconLabel>
                </Checkbox>
              </Box>
              {/* Icon-only buttons are named by aria-label and show the same words as a tooltip (web-client/16-look-states-and-wording.md G12). */}
              <TooltipTrigger>
                <Button
                  id={buttonId(entry.id, -1)}
                  variant="ghost"
                  size="icon"
                  aria-label={upLabel}
                  isDisabled={index === 0}
                  onPress={() => {
                    onMove(entry.id, index, -1);
                  }}
                >
                  <MoveUpIcon />
                </Button>
                <Tooltip>{upLabel}</Tooltip>
              </TooltipTrigger>
              <TooltipTrigger>
                <Button
                  id={buttonId(entry.id, 1)}
                  variant="ghost"
                  size="icon"
                  aria-label={downLabel}
                  isDisabled={index === entries.length - 1}
                  onPress={() => {
                    onMove(entry.id, index, 1);
                  }}
                >
                  <MoveDownIcon />
                </Button>
                <Tooltip>{downLabel}</Tooltip>
              </TooltipTrigger>
            </Cluster>
          );
        })}
      </Stack>
      <div>
        <Button variant="secondary" size="sm" onPress={reset}>
          {t("client:settings.navigation.reset")}
        </Button>
      </div>
    </Section>
  );
}
