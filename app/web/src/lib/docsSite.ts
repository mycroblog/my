// SPDX-License-Identifier: AGPL-3.0-only
// The pages of the documentation site (app/docs) that the screens link to, at the site's version of the running server
// and in the UI's locale when the site has it.

/** The documentation site, which serves each version of `my` under a path of its own. */
const docsOrigin = "https://mycroblog.dev";

/** The path prefix of each locale of the documentation site that is not its root locale `en` (app/docs/astro.config.ts). */
const docsLocales: Readonly<Record<string, string>> = {
  ja: "ja",
  "zh-CN": "zh-cn",
  "zh-TW": "zh-tw",
  es: "es",
  "pt-BR": "pt-br",
  de: "de",
  fr: "fr",
};

/**
 * The address of `page` (a path under a version and a locale, such as `guide/keyword-filters/#how-words-match`) at
 * the documentation site's version of the server (`/vX.Y/` for a stable release vX.Y.Z, `/next/` for any other build,
 * as internal/platform/docsite picks it) and in the UI's locale when the site has it (English otherwise).
 */
export function docsSiteUrl(serverVersion: string | undefined, locale: string, page: string): string {
  const release = /^v?(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)$/.exec(serverVersion ?? "");
  const version = release === null ? "next" : `v${release[1] ?? ""}.${release[2] ?? ""}`;
  const prefix = docsLocales[locale];
  return `${docsOrigin}/${version}/${prefix === undefined ? "" : `${prefix}/`}${page}`;
}
