// SPDX-License-Identifier: AGPL-3.0-only
// Translating a post where it is (FR-120, ADR-0148, web-client/screens/status.md), as plain values for the card
// (web-client/README.md A4): one button that translates and goes back to the original, the translation to show, and
// the failure to say in place. The original is what a card shows until its reader asks, and again after an edit, which
// makes another version; a translation always comes with the line that says whose words it is.
import { useNotice } from "@my/ui";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { useIntl } from "react-intl";
import { ApiError, errorMessage } from "../../../lib/errors";
import { type Translate, useT } from "../../../lib/i18n";
import type { Status } from "../api/status";
import {
  cacheStatusTranslation,
  isInLanguage,
  statusTranslationQueryOptions,
  type TranslatedBy,
  type TranslatedPost,
  type Translator,
  translationFailedType,
  translationLimitType,
  translationUnavailableType,
  translationUnsupportedType,
} from "../api/translation";
import type { StatusView } from "./useStatusView";
import { useTranslator } from "./useTranslator";

export type StatusTranslationView = {
  /** The button that translates the post or goes back to the original; null when it has no use here. */
  action: { label: string; isPending: boolean; onPress: () => void } | null;
  /** The translation to show in place of the original; null shows the original. */
  shown: TranslatedPost | null;
  /** Why the last try gave no translation. The original stays. */
  failure: string | null;
};

/** The sentence that says whose words a translation is; a screen reader hears it as the result. */
function translatedByText(by: TranslatedBy, t: Translate): string {
  switch (by.kind) {
    case "provider":
      return t("client:status.translation.by", { provider: by.name });
    case "author":
      return t("client:status.translation.author");
  }
}

/** The sentence for a request that gave no translation: the problems of the API by their type (E2). */
function failureText(error: unknown, t: Translate): string {
  if (error instanceof ApiError) {
    switch (error.type) {
      case translationLimitType:
        return t("client:status.translation.limit");
      case translationUnavailableType:
        return t("client:status.translation.unavailable");
      case translationUnsupportedType:
        return t("client:status.translation.unsupported");
      case translationFailedType:
        return t("client:status.translation.failed");
    }
  }
  return t("client:status.translation.failed_reason", { reason: errorMessage(error) });
}

/**
 * The translation of `status` (the boosted post for a boost) into the language of the UI. `enabled` is false where the
 * card offers the viewer no actions (a moderator's page). The choice to show a translation stays with the card, like an
 * opened content warning; the translation itself stays in the cache, so that showing it again asks nothing of the server.
 */
export function useStatusTranslation(status: Status, enabled: boolean): StatusTranslationView {
  const post = status.boost ?? status;
  const t = useT();
  const target = useIntl().locale;
  const notice = useNotice();
  const queryClient = useQueryClient();
  const translator = useTranslator(post);
  const options = statusTranslationQueryOptions(post, target);
  const cached = useQuery(options).data;
  // The one version whose translation the card shows, so that an edit goes back to the original by itself.
  const version = JSON.stringify(options.queryKey);
  const [shownVersion, setShownVersion] = useState<string | null>(null);
  // A mutation and not a query: it runs only because the reader pressed the button (R7), so nothing sends it again on
  // its own, and a refused one is this button's failure, not a page's (a 429 of the limit is no rate limit to wait out).
  const translate = useMutation({
    mutationFn: ({ translator: run }: { translator: Translator; version: string }) => run(target),
    onSuccess: (answer, variables) => {
      cacheStatusTranslation(queryClient, post, target, answer);
      setShownVersion(variables.version);
      notice.inPlace(translatedByText(answer.by, t));
    },
  });
  const shown = enabled && shownVersion === version ? (cached ?? null) : null;
  const isPending = translate.isPending && translate.variables.version === version;
  const failed = translate.isError && translate.variables.version === version;

  const onPress = () => {
    if (shown !== null) {
      setShownVersion(null);
      notice.inPlace(t("client:status.translation.original_shown"));
    } else if (cached !== undefined) {
      setShownVersion(version);
      notice.inPlace(translatedByText(cached.by, t));
    } else if (translator !== null) {
      translate.mutate({ translator, version });
    }
  };
  const offered = enabled && translator !== null && !isInLanguage(post.language, target);
  let label = t("client:status.translation.action");
  if (shown !== null) label = t("client:status.translation.show_original");
  else if (isPending) label = t("client:status.translation.pending");
  return {
    // A translation on show keeps its way back, even if the server stopped offering it since.
    action: offered || shown !== null ? { label, isPending, onPress } : null,
    shown,
    failure: failed && shown === null ? failureText(translate.error, t) : null,
  };
}

/** What the card shows of the text of a post with `shown` in place of the original; without it, `view` as it is. */
export function translatedView(view: StatusView, shown: TranslatedPost | null) {
  if (shown === null) {
    const { html, lang, contentWarning, media } = view;
    return { html, lang, contentWarning, contentWarningLang: lang, media };
  }
  // What a translation lacks stays the author's, in the author's language: above all, a warning never goes away.
  const translatedWarning = shown.contentWarning !== "";
  return {
    html: shown.body.html,
    lang: shown.language,
    contentWarning: translatedWarning ? shown.contentWarning : view.contentWarning,
    contentWarningLang: translatedWarning ? shown.language : view.lang,
    media: view.media.map((item) => {
      const alt = shown.altTexts[item.id];
      return alt === undefined ? item : { ...item, alt, lang: shown.language };
    }),
  };
}
