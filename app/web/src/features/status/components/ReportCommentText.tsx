// SPDX-License-Identifier: AGPL-3.0-only
import { Button, LanguageIcon, Stack, Text } from "@my/ui";
import { useEffect, useId, useRef } from "react";
import { useReportCommentTranslation } from "../hooks/useReportCommentTranslation";
import { StatusTranslationNote } from "./StatusTranslationNote";

export type ReportCommentTextProps = {
  /** The report's ID. */
  report: string;
  /** The reporter's comment, in plain text; not empty. */
  comment: string;
};

/**
 * The comment of a report as a moderator reads it (ADR-0148 §13): the reporter's text in any language, and under it the
 * button that translates it, as on a post's card without the actions row (web-client §13). Once a translation shows,
 * the line that says what happened and goes back to the original takes the button's place, and the focus moves
 * between the two.
 */
export function ReportCommentText({ report, comment }: ReportCommentTextProps) {
  const translation = useReportCommentTranslation(report, comment);
  const { shown, action } = translation;
  const baseId = useId();
  const showOriginal = `${baseId}-original`;
  const translateButton = `${baseId}-translate`;
  // Set by the press, read once the translation has come or gone: the press itself cannot focus what it shows.
  const focusNextRef = useRef<"line" | "button" | null>(null);
  useEffect(() => {
    const next = focusNextRef.current;
    if (next === null || (next === "line") !== (shown !== null)) return;
    focusNextRef.current = null;
    document.getElementById(next === "line" ? showOriginal : translateButton)?.focus();
  }, [shown, showOriginal, translateButton]);
  return (
    <Stack gap="2">
      {shown === null ? (
        // Written by the reporter, in any language (Y10).
        <Text dir="auto" block align="start" wrap="anywhere" whitespace="preWrap">
          {comment}
        </Text>
      ) : (
        <Text lang={shown.language} dir="auto" block align="start" wrap="anywhere" whitespace="preWrap">
          {shown.text}
        </Text>
      )}
      {action !== null && shown === null && (
        <div>
          <Button
            id={translateButton}
            variant="subtle"
            size="toolbar"
            isPending={action.isPending}
            onPress={() => {
              action.onPress();
              focusNextRef.current = "line";
            }}
          >
            <LanguageIcon />
            {action.label}
          </Button>
        </div>
      )}
      {action !== null && (
        <StatusTranslationNote
          shown={shown}
          // The button says it itself.
          progress={null}
          failure={translation.failure}
          onShowOriginal={() => {
            action.onPress();
            focusNextRef.current = "button";
          }}
          showOriginalId={showOriginal}
        />
      )}
    </Stack>
  );
}
