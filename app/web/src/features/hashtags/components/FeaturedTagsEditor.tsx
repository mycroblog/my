// SPDX-License-Identifier: AGPL-3.0-only
// The featured hashtags in the profile form (ADR-0006 §17; web-client/screens/settings-profile.md): a field per hashtag
// in the order the profile shows them, with two buttons that move it one place up or down and one that removes it, and
// the button that adds a row. The rows follow the WAI-ARIA APG's rearrangeable list as the navigation settings do (Up
// and Down buttons, the move announced, the focus staying with the moved row), with native tab stops. Nothing here
// saves: the form's save button saves the hashtags with the profile.
import {
  Box,
  Button,
  Cluster,
  MoveDownIcon,
  MoveUpIcon,
  Section,
  Stack,
  TextField,
  Tooltip,
  TooltipTrigger,
} from "@my/ui";
import { useEffect, useId, useRef } from "react";
import { useT } from "../../../lib/i18n";
import { hashtagLimit, maxFeaturedTags } from "../api/tags";
import type { FeaturedTagDraft, FeaturedTagsDraft } from "../hooks/useFeaturedTagsDraft";

export type FeaturedTagsEditorProps = {
  /** The rows and their changes, each change counted as an edit of the form. */
  draft: Omit<FeaturedTagsDraft, "save" | "reset" | "isSaving" | "isSaved" | "error" | "hasChanges">;
};

export function FeaturedTagsEditor({ draft }: FeaturedTagsEditorProps) {
  const t = useT();
  const baseId = useId();
  /** The button to focus once the rows have rendered a move. */
  const focusAfterMoveRef = useRef<string | null>(null);
  // Runs after every render: focus is moved in the DOM, outside React, once the moved row is in its new place.
  useEffect(() => {
    const id = focusAfterMoveRef.current;
    if (id === null) return;
    focusAfterMoveRef.current = null;
    document.getElementById(id)?.focus();
  });
  const buttonId = (key: number, step: -1 | 1) => `${baseId}-${String(key)}-${step === -1 ? "up" : "down"}`;
  const onMove = (row: FeaturedTagDraft, index: number, step: -1 | 1) => {
    // The focus stays on the moved row: on the same button, or on the other one where this one is now disabled.
    const to = index + step;
    const atEnd = step === -1 ? to === 0 : to === draft.rows.length - 1;
    focusAfterMoveRef.current = buttonId(row.key, atEnd ? (step === -1 ? 1 : -1) : step);
    draft.move(row.key, step);
  };
  return (
    <Section
      title={t("client:tag.featured.title")}
      description={t("client:tag.featured.description", { max: String(maxFeaturedTags) })}
    >
      {draft.rows.length > 0 && (
        <Stack as="ol" gap="3">
          {draft.rows.map((row, index) => {
            const number = index + 1;
            const upLabel = t("client:tag.featured.move_up", { number: String(number) });
            const downLabel = t("client:tag.featured.move_down", { number: String(number) });
            return (
              <Stack as="li" key={row.key} gap="2" rowAt="md">
                <Box shrink grow>
                  <TextField
                    label={t("client:tag.featured.field", { number: String(number) })}
                    value={row.name}
                    onChange={(name) => {
                      draft.update(row.key, name);
                    }}
                    // One more for the `#` the user may type.
                    maxLength={hashtagLimit + 1}
                    validate={draft.validate}
                    autoComplete="off"
                    autoCorrect="off"
                    spellCheck="false"
                  />
                </Box>
                <Cluster gap="2" align="center">
                  {/* Icon-only buttons are named by aria-label and show the same words as a tooltip (G12). */}
                  <TooltipTrigger>
                    <Button
                      id={buttonId(row.key, -1)}
                      variant="ghost"
                      size="icon"
                      aria-label={upLabel}
                      isDisabled={index === 0}
                      onPress={() => {
                        onMove(row, index, -1);
                      }}
                    >
                      <MoveUpIcon />
                    </Button>
                    <Tooltip>{upLabel}</Tooltip>
                  </TooltipTrigger>
                  <TooltipTrigger>
                    <Button
                      id={buttonId(row.key, 1)}
                      variant="ghost"
                      size="icon"
                      aria-label={downLabel}
                      isDisabled={index === draft.rows.length - 1}
                      onPress={() => {
                        onMove(row, index, 1);
                      }}
                    >
                      <MoveDownIcon />
                    </Button>
                    <Tooltip>{downLabel}</Tooltip>
                  </TooltipTrigger>
                  <Button
                    size="sm"
                    variant="ghost"
                    onPress={() => {
                      draft.remove(row.key);
                    }}
                  >
                    {t("client:tag.featured.remove", { number: String(number) })}
                  </Button>
                </Cluster>
              </Stack>
            );
          })}
        </Stack>
      )}
      <Box>
        <Button size="sm" onPress={draft.add} isDisabled={!draft.canAdd}>
          {t("client:tag.featured.add")}
        </Button>
      </Box>
    </Section>
  );
}
