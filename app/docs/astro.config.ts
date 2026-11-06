// SPDX-License-Identifier: AGPL-3.0-only
// The documentation site (ADR-0150): Astro Starlight, built to static files in dist/ by `scripts/task.sh docs:site-build`.
// The site is a build tool, not where the documents live: the pages under src/content/docs/ are copied there from the
// Markdown of the repository before the build, and only the landing page (src/content/docs/index.md) is written here.
// Search is Starlight's built-in Pagefind; the site loads no analytics and no external service (ADR-0150 §7).
import starlight from "@astrojs/starlight";
import { defineConfig } from "astro/config";

export default defineConfig({
  site: "https://mycroblog.dev",
  // The toolbar of `astro dev` is a development aid the site does not need.
  devToolbar: { enabled: false },
  integrations: [
    starlight({
      title: "my",
      description: "Documentation of my, an ActivityPub-compatible microblog server.",
      logo: {
        light: "./src/assets/wordmark-light.svg",
        dark: "./src/assets/wordmark-dark.svg",
        replacesTitle: true,
      },
      favicon: "/favicon.svg",
      social: [{ icon: "github", label: "GitHub", href: "https://github.com/mycroblog/my" }],
      // English only during v0, at the root of the URL space, so that translations added later keep every URL (ADR-0150 §9).
      defaultLocale: "root",
      locales: {
        root: { label: "English", lang: "en" },
      },
      // Starlight's own lastUpdated reads the git history of the copied files, which says nothing about the documents.
      lastUpdated: false,
    }),
  ],
});
