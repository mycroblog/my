// SPDX-License-Identifier: AGPL-3.0-only

// The pages of the catalog, a page per layer, and the sections of each, in order: the index at / lists them, the
// catalog renders a page's sections (Catalog.tsx), and `web:shot --section <id>` finds the page of a section here
// (shot.ts). A section is the <section> its heading of that id labels. A screen renders itself, with no sections.

export interface Section {
  id: string;
  title: string;
}

export interface Page {
  path: string;
  title: string;
  /** What the page shows, on the index. */
  about: string;
  sections: readonly Section[];
}

export const pages = [
  {
    path: "/tokens",
    title: "Tokens",
    about: "The type scale and the colors of @my/design-tokens.",
    sections: [
      { id: "type", title: "Type" },
      { id: "color", title: "Color" },
    ],
  },
  {
    path: "/design-system",
    title: "Design system",
    about:
      "The parts of @my/design-system, the anatomy of a post built from them, and the motion of a change of screen.",
    sections: [
      { id: "button", title: "Button" },
      { id: "post", title: "Post" },
      { id: "motion", title: "Motion" },
    ],
  },
  {
    path: "/app-components",
    title: "App components",
    about: "The content renderer and the post of @my/app-components.",
    sections: [
      { id: "content", title: "Content" },
      { id: "post-states", title: "Post states" },
    ],
  },
  { path: "/app/timeline", title: "Timeline", about: "The app's home screen, on sample data.", sections: [] },
  { path: "/app/auth", title: "Auth", about: "The rough of the sign-in and sign-up screens.", sections: [] },
  {
    path: "/app/post",
    title: "Post",
    about: "The app's post with its thread, on sample data (?state=).",
    sections: [],
  },
  {
    path: "/app/notifications",
    title: "Notifications",
    about: "The app's notifications, on sample data, with a control that receives one.",
    sections: [],
  },
  {
    path: "/app/profile",
    title: "Profile",
    about: "The app's profile, on sample data, in each of its states (?state=).",
    sections: [],
  },
  { path: "/app/compose", title: "Compose", about: "The rough of the post composer, in each state.", sections: [] },
] as const satisfies readonly Page[];

export type SectionId = (typeof pages)[number]["sections"][number]["id"];

/** The page of a path, without its trailing slash; undefined for the index and for a path that is no page. */
export const pageAt = (path: string): Page | undefined => pages.find((p) => p.path === path.replace(/\/+$/, ""));

/** The page that holds a section, or undefined when no page lists it (an element id inside a section). */
export const pageOf = (id: string): Page | undefined => pages.find((p) => p.sections.some((s: Section) => s.id === id));
