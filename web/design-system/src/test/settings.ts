// SPDX-License-Identifier: AGPL-3.0-only

// What a rendered settings screen breaks of the rules of ADR-0110 §6, read from the tree itself: the lint
// (web/devtools/src/restrictions.js: restrict/switch-in-form, settings-sections, settings-count) sees what one file
// writes out, and cannot count the entries a list is mapped to, nor see a form and its switch in two files. A test of
// a screen, or of a story of one, expects `settingsFindings` of what it rendered to be empty. The parts mark what
// they are (`data-settings-group`, `data-settings-section`, `data-settings-action`, `data-settings-screen`,
// `data-frame-head`, `data-frame-foot`).

/** The most entries of a group of the settings' menu, and the most a section holds for the reader to do. */
export const SETTINGS_MOST = 7;

const CONTROLS = "button, a[href], input:not([type=hidden]), select, textarea";
const SUBMITS = "[type=submit], button:not([type]), [data-frame-foot] button";

/** The words of `element`, on one line, cut short. */
const wordsOf = (element: Element | null | undefined): string =>
  (element?.textContent ?? "").replace(/\s+/g, " ").trim().slice(0, 60);

/** The words of the heading that names `region`, for a finding to say which one. */
function nameOf(region: Element): string {
  const ids = (region.getAttribute("aria-labelledby") ?? "").split(/\s+/).filter((id) => id !== "");
  return ids
    .map((id) => wordsOf(region.ownerDocument.getElementById(id)))
    .join(" ")
    .trim();
}

/** The entries of a group of the menu: the items of its list. */
function entries(group: Element): number {
  const list = [...group.children].find((child) => child.localName === "ul");
  return list === undefined ? 0 : list.children.length;
}

/** How many things the section holds for the reader to do: what stands under its head, and its own action. */
function held(section: Element): number {
  const [head, ...rows] = [...section.children];
  return rows.length + (head?.querySelector("[data-settings-action]") ? 1 : 0);
}

/**
 * What the tree under `root` breaks of the rules of the settings, each as a sentence; none when it keeps them:
 * - a group of the menu has seven entries at most, and a section holds seven things to do at most, its own action
 *   among them (a list is one, however many its rows);
 * - a section and a control outside any section do not stand under one parent: a settings screen holds its head,
 *   sections, and what ends it;
 * - no switch, which saves as it is pressed, stands in a form, or a settings screen, that has a submit button;
 * - a section adds to its list from one place: its head, or what stands in the empty list's place.
 */
export function settingsFindings(root: ParentNode): string[] {
  const found: string[] = [];
  for (const group of root.querySelectorAll("[data-settings-group]")) {
    const count = entries(group);
    if (count > SETTINGS_MOST)
      found.push(
        `the group "${nameOf(group)}" of the menu has ${String(count)} entries, over ${String(SETTINGS_MOST)}`,
      );
  }
  const parents = new Set<Element>();
  for (const section of root.querySelectorAll("[data-settings-section]")) {
    const count = held(section);
    if (count > SETTINGS_MOST)
      found.push(`the section "${nameOf(section)}" holds ${String(count)} things to do, over ${String(SETTINGS_MOST)}`);
    if (section.querySelectorAll("[data-settings-action]").length > 1)
      found.push(`the section "${nameOf(section)}" adds from its head and from its empty list: one place adds`);
    if (section.parentElement !== null) parents.add(section.parentElement);
  }
  for (const parent of parents)
    for (const child of parent.children) {
      if (child.matches("[data-settings-section], [data-frame-head], [data-frame-foot]")) continue;
      if (child.matches(CONTROLS) || child.querySelector(CONTROLS) !== null)
        found.push(`a control stands beside the sections, in none of them: "${wordsOf(child)}"`);
    }
  for (const toggle of root.querySelectorAll("[role=switch]")) {
    const form = toggle.closest("form, [data-settings-screen]");
    if (form !== null && form.querySelector(SUBMITS) !== null)
      found.push(
        `the switch "${wordsOf(toggle.closest("label"))}" stands in a form that has a submit button: a switch saves as it is pressed`,
      );
  }
  return found;
}
