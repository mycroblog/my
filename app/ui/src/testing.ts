// SPDX-License-Identifier: AGPL-3.0-only
// Checks for the tests of the screens built with the kit (`@my/ui/testing`), run by app/web's test setup over every
// screen a feature test renders. They are not part of the kit's components and stay out of the app's bundle.

/** The submit buttons of a form: its own and those outside it that name it with `form=`. */
function hasSubmitButton(form: HTMLFormElement): boolean {
  return Array.from(form.elements).some(
    (element) =>
      (element instanceof HTMLButtonElement || element instanceof HTMLInputElement) && element.type === "submit",
  );
}

/** The error of a switch found inside a form with a submit button. */
function failure(found: Iterable<string>): Error {
  return new Error(
    `A switch takes effect at once and does not belong in a form saved by a submit button; use a Checkbox there: ${[...found].join(", ")}`,
  );
}

/** How a failure names a switch: its label's text, or its markup when it has no label. */
function describeSwitch(element: Element): string {
  const label = element.closest("label")?.textContent.trim();
  return label !== undefined && label !== "" ? `"${label}"` : element.outerHTML.slice(0, 120);
}

/**
 * The switches (`role="switch"`) under `root` that sit inside a form with a submit button, by their label. A switch
 * takes effect as soon as it changes (`Switch`, the APG switch pattern); in a form saved by a button it promises what
 * the form does not do, so such a form uses a `Checkbox` (web-client §21, ADR-0110 §6).
 */
export function switchesInSubmitForms(root: ParentNode = document): string[] {
  return Array.from(root.querySelectorAll('[role="switch"]'))
    .filter((element) => {
      const form = element instanceof HTMLInputElement ? element.form : element.closest("form");
      return form !== null && hasSubmitButton(form);
    })
    .map(describeSwitch);
}

/** Throws when `root` has a switch inside a form with a submit button (`switchesInSubmitForms`). */
export function expectNoSwitchInSubmitForm(root: ParentNode = document): void {
  const found = switchesInSubmitForms(root);
  if (found.length > 0) throw failure(found);
}

/**
 * Watches `root` while a test runs and remembers every switch that was ever inside a form with a submit button, so
 * that a dialog opened and closed during the test is checked too. `stop` ends the watch and throws as
 * `expectNoSwitchInSubmitForm` does when any was seen.
 */
export function watchSwitchesInSubmitForms(root: ParentNode & Node = document.body): { stop: () => void } {
  const seen = new Set<string>();
  const check = () => {
    for (const found of switchesInSubmitForms(root)) seen.add(found);
  };
  const observer = new MutationObserver(check);
  observer.observe(root, {
    childList: true,
    subtree: true,
    attributes: true,
    attributeFilter: ["role", "type", "form"],
  });
  return {
    stop: () => {
      observer.takeRecords();
      observer.disconnect();
      check();
      if (seen.size > 0) throw failure(seen);
    },
  };
}

/** The most pages of a group of a grouped menu (`NavMenu`'s `navMenuGroupLimit`, ADR-0110 §6). */
const menuGroupLimit = 7;

/** How a failure names a group of a menu: its heading, or its markup when it has none. */
function describeGroup(group: Element): string {
  const id = group.getAttribute("aria-labelledby");
  const heading = id === null ? undefined : group.ownerDocument.getElementById(id)?.textContent.trim();
  return heading !== undefined && heading !== "" ? `"${heading}"` : group.outerHTML.slice(0, 120);
}

/**
 * The problems of the grouped menus (`NavMenu`, `data-nav-menu-group`) under `root`: a group with more pages than
 * the limit, and a page without an icon (web-client §21, ADR-0110 §5, §6). Each page of a menu has an icon so that
 * the eye finds it, and a group stays short enough to scan; a longer group is split.
 */
export function navMenuProblems(root: ParentNode = document): string[] {
  const problems: string[] = [];
  for (const group of Array.from(root.querySelectorAll("[data-nav-menu-group]"))) {
    const links = Array.from(group.querySelectorAll("a"));
    if (links.length > menuGroupLimit)
      problems.push(`the group ${describeGroup(group)} has ${links.length} pages, more than ${menuGroupLimit}`);
    for (const link of links) {
      if (link.querySelector("svg") === null)
        problems.push(`the page "${link.textContent.trim()}" of the group ${describeGroup(group)} has no icon`);
    }
  }
  return problems;
}

/** The error of the problems of grouped menus. */
function menuFailure(found: Iterable<string>): Error {
  return new Error(
    `A grouped menu breaks its rules (an icon on every page, at most ${menuGroupLimit} pages a group): ${[...found].join("; ")}`,
  );
}

/** Throws when a grouped menu under `root` has a problem (`navMenuProblems`). */
export function expectValidNavMenus(root: ParentNode = document): void {
  const found = navMenuProblems(root);
  if (found.length > 0) throw menuFailure(found);
}

/**
 * Watches `root` while a test runs and remembers every problem of a grouped menu it ever showed, as
 * `watchSwitchesInSubmitForms` does for switches. `stop` ends the watch and throws when any was seen.
 */
export function watchNavMenus(root: ParentNode & Node = document.body): { stop: () => void } {
  const seen = new Set<string>();
  const check = () => {
    for (const found of navMenuProblems(root)) seen.add(found);
  };
  const observer = new MutationObserver(check);
  observer.observe(root, { childList: true, subtree: true });
  return {
    stop: () => {
      observer.takeRecords();
      observer.disconnect();
      check();
      if (seen.size > 0) throw menuFailure(seen);
    },
  };
}
