// SPDX-License-Identifier: AGPL-3.0-only
import { Button, ExternalLink, InlineError, PostCardNote, PostCardNoteSeparator } from "@my/ui";
import type { ReactNode } from "react";
import { useIntl } from "react-intl";
import { type Translate, useT } from "../../../lib/i18n";
import type { TranslatedBy } from "../api/translation";

export type StatusTranslationNoteProps = {
  /** The translation on show: who translated it, and from which language (`und` when not known); null while none shows. */
  shown: { by: TranslatedBy; sourceLanguage: string } | null;
  /** What happens while a translation is on its way ("Translating…", the progress of the browser's model). */
  progress: string | null;
  /** Why the last try gave no translation. */
  failure: string | null;
  /** Goes back to the original: the line's "Show original". */
  onShowOriginal: () => void;
  /** The `id` of the line's "Show original", which takes the focus when the line replaces the button that translated. */
  showOriginalId?: string | undefined;
};

/** `tag`'s name in the UI's language, or the tag itself when the browser does not know it. */
function languageName(tag: string, locale: string): string {
  try {
    return new Intl.DisplayNames([locale], { type: "language", fallback: "code" }).of(tag) ?? tag;
  } catch {
    // Not a valid tag for Intl; the tag is its own name.
    return tag;
  }
}

/** What happened: from which language, or only that a machine translated it when the language is not known. */
function whatHappened(sourceLanguage: string, locale: string, t: Translate): string {
  return sourceLanguage === "und" || sourceLanguage === ""
    ? t("client:status.translation.machine")
    : t("client:status.translation.from", { language: languageName(sourceLanguage, locale) });
}

/** Who translated it: the provider, with its link when it has one, or the reader's browser. */
function translator(by: Exclude<TranslatedBy, { kind: "author" }>, t: Translate): ReactNode {
  if (by.kind === "browser") {
    return by.name === null
      ? t("client:status.translation.browser_unnamed")
      : t("client:status.translation.browser", { browser: by.name });
  }
  return by.href === null ? by.name : <ExternalLink href={by.href}>{by.name}</ExternalLink>;
}

/**
 * The line right after the text of a translated post or report comment (ADR-0148 §6): what happened ("Translated from
 * English"), who translated it (the provider with the link and logo the server gives for it, or the browser), and the
 * way back to the original, in one place; or that the author wrote the text after all. While a translation is on its
 * way it says so, and when one fails it says why.
 */
export function StatusTranslationNote({
  shown,
  progress,
  failure,
  onShowOriginal,
  showOriginalId,
}: StatusTranslationNoteProps) {
  const t = useT();
  const { locale } = useIntl();
  if (shown !== null) {
    const { by } = shown;
    return (
      <PostCardNote {...(by.kind === "provider" && by.logo !== null && { logo: by.logo })}>
        {by.kind === "author" ? (
          t("client:status.translation.author")
        ) : (
          <>
            {whatHappened(shown.sourceLanguage, locale, t)}
            <PostCardNoteSeparator />
            {translator(by, t)}
          </>
        )}
        <PostCardNoteSeparator />
        <Button
          {...(showOriginalId !== undefined && { id: showOriginalId })}
          variant="subtle"
          size="toolbar"
          onPress={onShowOriginal}
        >
          {t("client:status.translation.show_original")}
        </Button>
      </PostCardNote>
    );
  }
  if (progress !== null) return <PostCardNote>{progress}</PostCardNote>;
  if (failure !== null) {
    return (
      <InlineError as="p" alert>
        {failure}
      </InlineError>
    );
  }
  return null;
}
