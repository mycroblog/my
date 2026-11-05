// SPDX-License-Identifier: AGPL-3.0-only
import { ExternalLink, PostCardNote } from "@my/ui";
import { useIntl } from "react-intl";
import { useT } from "../../../lib/i18n";
import type { TranslatedBy } from "../api/translation";

export type StatusTranslationNoteProps = { by: TranslatedBy };

/**
 * The line over a translation that says whose words it is (ADR-0148 §6): the provider that machine translated it, with
 * the link and the logo the server gives for it, the browser that did (§12), or that the author wrote the text after all.
 */
export function StatusTranslationNote({ by }: StatusTranslationNoteProps) {
  const t = useT();
  const intl = useIntl();
  switch (by.kind) {
    case "provider":
      return (
        <PostCardNote {...(by.logo !== null && { logo: by.logo })}>
          {by.href === null
            ? t("client:status.translation.by", { provider: by.name })
            : intl.formatMessage(
                { id: "client:status.translation.by" },
                {
                  provider: (
                    <ExternalLink key="provider" href={by.href}>
                      {by.name}
                    </ExternalLink>
                  ),
                },
              )}
        </PostCardNote>
      );
    case "browser":
      return (
        <PostCardNote>
          {by.name === null
            ? t("client:status.translation.by_browser_unnamed")
            : t("client:status.translation.by_browser", { browser: by.name })}
        </PostCardNote>
      );
    case "author":
      return <PostCardNote>{t("client:status.translation.author")}</PostCardNote>;
  }
}
