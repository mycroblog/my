// SPDX-License-Identifier: AGPL-3.0-only
// The documentation site (ADR-0150): Astro Starlight, built to static files in dist/ by `scripts/task.sh docs:site-build`.
// The site is a build tool, not where the documents live: the pages under src/content/docs/ are copied there from the
// Markdown of the repository before the build, and only the landing page (src/content/docs/index.mdx) is written here.
// Search is Starlight's built-in Pagefind; the site loads no analytics and no external service (ADR-0150 §7).
import { satteri } from "@astrojs/markdown-satteri";
import starlight from "@astrojs/starlight";
import { defineConfig } from "astro/config";
import starlightLinksValidator from "starlight-links-validator";
import { base, sitePageLinks } from "./src/published";
import { docsSidebar } from "./src/sidebar";
import { apiReferencePlugin, apiReferenceSidebar } from "./src/api-reference";
import { llmsTxtPlugin } from "./src/llms-txt";

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
      },
      // Starlight's own lastUpdated reads the git history of the copied files, which says nothing about the documents.
      lastUpdated: false,
      // The groups and order of docs/README.md (src/sidebar.ts), with the client API reference after the guides.
      sidebar: docsSidebar({ Developers: [apiReferenceSidebar] }),
      // The Markdown form of each page in its head (src/route-middleware.ts), and the landing page's hero buttons,
      // which name documents by their repository path (src/routeData.ts).
      routeMiddleware: ["./src/route-middleware.ts", "./src/routeData.ts"],
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
