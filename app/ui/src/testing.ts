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
