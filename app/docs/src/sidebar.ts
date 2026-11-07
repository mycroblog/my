// SPDX-License-Identifier: AGPL-3.0-only
// The sidebar of the site, built from the sections and order of docs/README.md so that the documents have one table
// of contents (ADR-0150 §4): a document added to docs/README.md appears here with no other change, and the
// `### Headings` of a section become its nested groups. In a locale, a link is labelled with the title of its
// translation and a group with its label of app/docs/i18n/<locale>/sidebar.json (src/translations.ts); either keeps
// its English label when the locale has none.
import { type IndexSection, readIndex, slugOf } from "./published";
import { sidebarLabels, translatedTitle, translationLocales } from "./translations";

interface SidebarLink {
  label: string;
  slug: string;
  translations: Record<string, string>;
}

interface SidebarGroup {
  label: string;
  translations: Record<string, string>;
  items: (SidebarLink | SidebarGroup)[];
}

const groupLabels = Object.fromEntries(translationLocales.map((locale) => [locale, sidebarLabels(locale)]));

/** The labels in each locale of the group with the English label `label`. */
function groupTranslations(label: string): Record<string, string> {
  return Object.fromEntries(
    translationLocales.flatMap((locale) => {
      const translated = groupLabels[locale]?.[label];
      return translated === undefined ? [] : [[locale, translated]];
    }),
  );
}

/** The labels in each locale of the link to the document at `path`: the titles of its translations. */
function linkTranslations(path: string): Record<string, string> {
  return Object.fromEntries(
    translationLocales.flatMap((locale) => {
      const title = translatedTitle(locale, path);
      return title === undefined ? [] : [[locale, shortLabel(title)]];
    }),
  );
}

/** The text of a link in docs/README.md without its trailing explanation in parentheses, as a sidebar label. */
function shortLabel(label: string): string {
  return label.replace(/\s*\([^)]*\)\s*$/, "").replaceAll("`", "") || label;
}

/**
 * The items of a section: its documents in order, each run of documents under one `### Heading` of docs/README.md
 * gathered into a nested group with that label.
 */
function sectionItems(items: IndexSection["items"]): (SidebarLink | SidebarGroup)[] {
  const out: (SidebarLink | SidebarGroup)[] = [];
  let open: { name: string; group: SidebarGroup } | undefined;
  for (const item of items) {
    const link = { label: shortLabel(item.label), slug: slugOf(item.path), translations: linkTranslations(item.path) };
    if (item.group === undefined) {
      open = undefined;
      out.push(link);
    } else if (open?.name === item.group) {
      open.group.items.push(link);
    } else {
      open = {
        name: item.group,
        group: { label: item.group, translations: groupTranslations(item.group), items: [link] },
      };
      out.push(open.group);
    }
  }
  return out;
}

/**
 * The groups of the sidebar. `after` places groups that do not come from docs/README.md (such as the API reference
 * that a plugin fills) right after the section with that heading, or at the end when docs/README.md has no such section.
 */
export function docsSidebar<T>(after: Record<string, T[]> = {}): (SidebarGroup | T)[] {
  const placed = new Set<string>();
  const groups = readIndex().flatMap((section): (SidebarGroup | T)[] => {
    const group = {
      label: section.heading,
      translations: groupTranslations(section.heading),
      items: sectionItems(section.items),
    };
    placed.add(section.heading);
    return [group, ...(after[section.heading] ?? [])];
  });
  const rest = Object.entries(after).flatMap(([heading, extra]) => (placed.has(heading) ? [] : extra));
  return [...groups, ...rest];
}
