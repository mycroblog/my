// SPDX-License-Identifier: AGPL-3.0-only
import { Button, Cluster, InlineError, LanguageIcon, Stack, Text } from "@my/ui";
import { useReportCommentTranslation } from "../hooks/useReportCommentTranslation";
import { StatusTranslationNote } from "./StatusTranslationNote";

export type ReportCommentTextProps = {
  /** The report's ID. */
  report: string;
  /** The reporter's comment, in plain text; not empty. */
  comment: string;
};

/**
 * The comment of a report as a moderator reads it (ADR-0148 §13): the reporter's text in any language, and the button
 * that translates it and goes back to the original, as on a post's card. A translation comes with the line that says
 * whose words it is.
 */
export function ReportCommentText({ report, comment }: ReportCommentTextProps) {
  const translation = useReportCommentTranslation(report, comment);
  const { shown, action } = translation;
  return (
    <Stack gap="2">
      {shown !== null && <StatusTranslationNote by={shown.by} />}
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
      {action !== null && (
        // One button in one place, which only changes its words, so that the focus stays on it.
        <Cluster gap="2" align="center">
          <Button variant="subtle" size="toolbar" isPending={action.isPending} onPress={action.onPress}>
            <LanguageIcon />
            {action.label}
          </Button>
          {translation.failure !== null && (
            <InlineError as="span" alert>
              {translation.failure}
            </InlineError>
          )}
        </Cluster>
      )}
    </Stack>
  );
}
