// SPDX-License-Identifier: AGPL-3.0-only
// The documentation site (ADR-0150): Astro Starlight, built to static files in dist/ by `scripts/task.sh docs:site-build`.
// The site is a build tool, not where the documents live: the pages under src/content/docs/ are copied there from the
// Markdown of the repository before the build; only the landing page (src/content/docs/index.mdx) and the notice of the
// site's analytics (src/content/docs/analytics.mdx) are written here.
// Search is Starlight's built-in Pagefind. The one external service is Cloudflare Web Analytics on the production site
// (ADR-0150 §7, src/analytics.ts): its beacon is in the head of every page only when DOCS_SITE_CF_BEACON_TOKEN is set,
// which docs-site-deploy.yml alone does, so local builds, the merge queue and the previews of pull requests have none.
import { satteri } from "@astrojs/markdown-satteri";
import starlight from "@astrojs/starlight";
import { defineConfig } from "astro/config";
import starlightLinksValidator from "starlight-links-validator";
import { base, site, sitePageLinks } from "./src/published";
import { docsSidebar } from "./src/sidebar";
import { apiReferencePlugin, apiReferenceSidebar } from "./src/api-reference";
import { llmsTxtPlugin } from "./src/llms-txt";
import { analyticsHead } from "./src/analytics";

export default defineConfig({
  site: "https://mycroblog.dev",
  // The path prefix of the version being built (DOCS_SITE_BASE: /latest/, /next/, ...; ADR-0150 §6), "/" by default.
  base,
  // The toolbar of `astro dev` is a development aid the site does not need.
  devToolbar: { enabled: false },
  // Heading attributes (`## 公開範囲 {#visibility}`) give each heading of a translated page the anchor of its English
  // heading (scripts/import-docs.ts), so that the links and #anchors of the sources lead to it in every locale.
  markdown: { processor: satteri({ features: { headingAttributes: true } }) },
  integrations: [
    starlight({
      title: "mycroblog",
      description:
        "Documentation of mycroblog, a microblog for the fediverse that starts small and grows with your community, and of my, its server software.",
      logo: {
        light: "./src/assets/wordmark-light.svg",
        dark: "./src/assets/wordmark-dark.svg",
        replacesTitle: true,
      },
      favicon: "/favicon.svg",
      // The link preview of every page: the wordmark on white (public/og-image.png, rendered by
      // scripts/docs-og-image.sh), by its absolute URL in the version being built, which the copy of the landing page
      // at the root of the domain names too. Starlight already sets twitter:card to summary_large_image.
      head: [
        { tag: "meta", attrs: { property: "og:image", content: `${site}${base}og-image.png` } },
        { tag: "meta", attrs: { property: "og:image:width", content: "1200" } },
        { tag: "meta", attrs: { property: "og:image:height", content: "630" } },
        { tag: "meta", attrs: { property: "og:image:alt", content: "The my wordmark" } },
        ...analyticsHead(process.env["DOCS_SITE_CF_BEACON_TOKEN"]),
      ],
      social: [{ icon: "github", label: "GitHub", href: "https://github.com/mycroblog/my" }],
      // The version switcher before the social links, and the end-of-support banner of an ended version (src/versions.ts).
      components: {
        SocialIcons: "./src/components/SocialIcons.astro",
        Banner: "./src/components/Banner.astro",
        Footer: "./src/components/Footer.astro",
      },
      customCss: ["./src/styles/custom.css"],
      // English at the root of the URL space, and the translations of app/docs/i18n/ under /<locale>/ (ADR-0150 §9,
      // src/translations.ts): a page a locale has not translated is the English page with Starlight's notice.
      defaultLocale: "root",
      locales: {
        root: { label: "English", lang: "en" },
        ja: { label: "日本語", lang: "ja" },
        "zh-cn": { label: "简体中文", lang: "zh-CN" },
        "zh-tw": { label: "繁體中文", lang: "zh-TW" },
        es: { label: "Español", lang: "es" },
        "pt-br": { label: "Português (Brasil)", lang: "pt-BR" },
        de: { label: "Deutsch", lang: "de" },
        fr: { label: "Français", lang: "fr" },
      },
      // Starlight's own lastUpdated reads the git history of the copied files, which says nothing about the documents.
      lastUpdated: false,
      // The groups and order of docs/README.md (src/sidebar.ts), with the client API reference after the guides.
      sidebar: docsSidebar({ Developers: [apiReferenceSidebar] }),
      // The Markdown form of each page in its head (src/route-middleware.ts), the landing page's hero buttons, which
      // name documents by their repository path (src/routeData.ts), and what the head tells search engines: noindex,
      // canonical and hreflang links (src/indexing.ts).
      routeMiddleware: ["./src/route-middleware.ts", "./src/routeData.ts", "./src/indexing.ts"],
      // A broken link or #anchor between pages fails the build; the validator cannot see the generated reference. A
      // translated page links the pages of its locale, which may be the English page in its place (a fallback page).
      plugins: [
        starlightLinksValidator({ exclude: sitePageLinks, errorOnFallbackPages: false }),
        apiReferencePlugin,
        llmsTxtPlugin,
      ],
    }),
  ],
});
