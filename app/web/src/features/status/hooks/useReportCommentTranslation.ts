// SPDX-License-Identifier: AGPL-3.0-only
// Translating the comment of a report where it is, for a moderator (ADR-0148 §13, web-client/screens/moderation-report.md),
// as plain values for the page (web-client/README.md A4): the same button, sentences, and states as a post's card
// (useStatusTranslation). The translation is kept only in this hook's state, in memory: the server keeps none of a
// comment, and it goes with the page.
import { useNotice } from "@my/ui";
import { useMutation } from "@tanstack/react-query";
import { useState } from "react";
import { useIntl } from "react-intl";
import { useT } from "../../../lib/i18n";
import type { CommentTranslator, TranslatedComment } from "../api/translation";
import { failureText, translatedByText } from "./useStatusTranslation";
import { useReportCommentTranslator } from "./useTranslator";

export type ReportCommentTranslationView = {
  /** The button that translates the comment or goes back to the original; null when nothing can translate it. */
  action: { label: string; isPending: boolean; onPress: () => void } | null;
  /** The translation to show in place of the comment; null shows the comment. */
  shown: TranslatedComment | null;
  /** Why the last try gave no translation. The comment stays. */
  failure: string | null;
};

/** The translation of `comment`, the comment of report `report`, into the language of the UI. */
export function useReportCommentTranslation(report: string, comment: string): ReportCommentTranslationView {
  const t = useT();
  const target = useIntl().locale;
  const notice = useNotice();
  const translator = useReportCommentTranslator(report, comment, target);
  // The last translation, of this comment into this language, and whether it is on show.
  const [kept, setKept] = useState<{ key: string; translation: TranslatedComment } | null>(null);
  const [isShown, setIsShown] = useState(false);
  const [downloaded, setDownloaded] = useState<number | null>(null);
  const key = JSON.stringify([report, comment, target]);
  const cached = kept?.key === key ? kept.translation : null;
  // A mutation: it runs only because the moderator pressed the button (R7), and its failure is this button's.
  const translate = useMutation({
    mutationFn: ({ translator: run }: { translator: CommentTranslator; key: string }) =>
      run(target, (loaded) => {
        setDownloaded(loaded < 1 ? loaded : null);
      }),
    onSettled: () => {
      setDownloaded(null);
    },
    onSuccess: (answer, variables) => {
      setKept({ key: variables.key, translation: answer });
      setIsShown(true);
      notice.inPlace(translatedByText(answer.by, t));
    },
  });
  const shown = isShown && cached !== null ? cached : null;
  const isPending = translate.isPending && translate.variables.key === key;
  const failed = translate.isError && translate.variables.key === key;

  const onPress = () => {
    if (shown !== null) {
      setIsShown(false);
      notice.inPlace(t("client:status.translation.original_shown"));
    } else if (cached !== null) {
      setIsShown(true);
      notice.inPlace(translatedByText(cached.by, t));
    } else if (translator !== null) {
      translate.mutate({ translator, key });
    }
  };
  let label = t("client:status.translation.action");
  if (shown !== null) label = t("client:status.translation.show_original");
  else if (isPending && downloaded !== null) {
    label = t("client:status.translation.downloading", { progress: downloaded });
  } else if (isPending) label = t("client:status.translation.pending");
  return {
    action: translator !== null || shown !== null ? { label, isPending, onPress } : null,
    shown,
    failure: failed && shown === null ? failureText(translate.error, t) : null,
  };
}
