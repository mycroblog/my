// SPDX-License-Identifier: AGPL-3.0-only
// How much a rendered screen shows (web-client §21 P3, ADR-0110 §6): its controls, helper texts and words, and the
// controls of each region under an `h2`. The story test measures every screen story with it (storyBudget.ts) and
// compares the counts with the budget of the screen's L1 file (l4/budget.ts).
//
// What is counted is the page's `main` and the dialogs open outside it; the shell around it is the same on every
// screen. A list, a table or a feed counts as its largest row (`li`, `tr`, `article`, a row or an option), so the
// counts follow what a row offers and not how many rows a fixture has. The words of a `time` are left out: "5
// minutes ago" becomes "1 hour ago" between two runs.

/** The counts of one rendered screen. */
export type ScreenMeasure = {
  /** The controls a person can operate; a radio group is one. */
  controls: number;
  /** The helper texts: `Hint`, a field's or a region's `description`, `Callout`. */
  helpers: number;
  /** The words a person can read. */
  words: number;
  /** The regions under an `h2`, in the order of the page, with the controls of each. */
  regions: { title: string; controls: number }[];
};

const controlSelector = [
  "a[href]",
  "button",
  'input:not([type="hidden"])',
  "select",
  "textarea",
  "summary",
  '[contenteditable="true"]',
  ...[
    "button",
    "link",
    "checkbox",
    "radiogroup",
    "switch",
    "slider",
    "combobox",
    "textbox",
    "searchbox",
    "spinbutton",
    "tab",
    "menuitem",
    "menuitemcheckbox",
    "menuitemradio",
  ].map((role) => `[role="${role}"]`),
].join(", ");

/** `Hint` and `Callout` of @my/ui mark themselves; a field's description is React Aria's slot. */
const helperSelector = '[data-helper], [slot="description"]';

const rowSelector = 'li, tr, article, [role="row"], [role="listitem"], [role="option"], [role="article"]';

function visible(element: Element): boolean {
  if (element.closest('[aria-hidden="true"], [inert]') !== null) return false;
  // jsdom has no layout and no checkVisibility: there everything in the tree counts.
  return typeof element.checkVisibility !== "function" || element.checkVisibility({ visibilityProperty: true });
}

/** The elements of `scope` matching `selector` that are visible and not inside another match. */
function outermost(scope: Element, selector: string): Element[] {
  return Array.from(scope.querySelectorAll(selector)).filter((element) => {
    if (!visible(element)) return false;
    const outer = element.parentElement?.closest(selector);
    return outer === null || outer === undefined || !scope.contains(outer) || outer === scope;
  });
}

function textWords(element: Element): number {
  const text = element instanceof HTMLElement && typeof element.innerText === "string" ? element.innerText : null;
  return (text ?? element.textContent).split(/\s+/u).filter((word) => /[\p{L}\p{N}]/u.test(word)).length;
}

function wordsOf(element: Element): number {
  const times = element.matches("time") ? [element] : outermost(element, "time");
  return textWords(element) - times.reduce((sum, time) => sum + textWords(time), 0);
}

/**
 * `count` over `scope`, with every run of sibling rows counted as its largest row: the total, less what the other
 * rows of each run add.
 */
function countWithRows(scope: Element, total: number, count: (row: Element) => number): number {
  const runs = new Map<Element | null, number[]>();
  for (const row of outermost(scope, rowSelector)) {
    runs.set(row.parentElement, [...(runs.get(row.parentElement) ?? []), count(row)]);
  }
  let extra = 0;
  for (const counts of runs.values()) extra += counts.reduce((sum, n) => sum + n, 0) - Math.max(...counts);
  return total - extra;
}

function controlCount(scope: Element): number {
  // A control inside another (the radios of a radio group, a button in a link) is the outer one's.
  const controls = outermost(scope, controlSelector);
  return countWithRows(scope, controls.length, (row) => controls.filter((control) => row.contains(control)).length);
}

function helperCount(scope: Element): number {
  const helpers = outermost(scope, helperSelector);
  return countWithRows(scope, helpers.length, (row) => helpers.filter((helper) => row.contains(helper)).length);
}

function wordCount(scope: Element): number {
  return countWithRows(scope, wordsOf(scope), wordsOf);
}

/** The region an `h2` names: its `section` (@my/ui's `Section`), or its parent when it has none. */
function regionOf(heading: Element): Element {
  return heading.closest("section") ?? heading.parentElement ?? heading;
}

/** Measures the screen under `root`: its `main` and the dialogs open outside it. */
export function measureScreen(root: ParentNode = document): ScreenMeasure {
  const mains = Array.from(root.querySelectorAll("main")).filter((main) => main.parentElement?.closest("main") == null);
  const dialogs = Array.from(root.querySelectorAll('[role="dialog"], [role="alertdialog"]')).filter(
    (dialog) => dialog.closest("main") === null && dialog.parentElement?.closest('[role="dialog"]') == null,
  );
  const measure: ScreenMeasure = { controls: 0, helpers: 0, words: 0, regions: [] };
  for (const scope of [...mains, ...dialogs].filter(visible)) {
    measure.controls += controlCount(scope);
    measure.helpers += helperCount(scope);
    measure.words += wordCount(scope);
    for (const heading of Array.from(scope.querySelectorAll("h2")).filter(visible)) {
      measure.regions.push({
        title: heading.textContent.replace(/\s+/gu, " ").trim(),
        controls: controlCount(regionOf(heading)),
      });
    }
  }
  return measure;
}
