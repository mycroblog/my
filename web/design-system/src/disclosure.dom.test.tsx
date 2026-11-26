// SPDX-License-Identifier: AGPL-3.0-only

import { expect, test } from "bun:test";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { I18nProvider } from "react-aria-components";
import { Disclosure, type DisclosureProps } from "./disclosure";

/** A content warning that folds a link, between nothing else. */
const warning = (props: Partial<DisclosureProps> = {}) =>
  render(
    <Disclosure summary="Spoilers for the finale" icon="hidden" locale="en" {...props}>
      <a href="/posts/1">The ending</a>
    </Disclosure>,
  );

/** The link the disclosure folds, as assistive technology finds it: none while it is folded. */
const folded = () => screen.queryByRole("link", { name: "The ending" });

/** The language the element's words are marked with, for a screen reader's voice. */
const languageOf = (element: Element) => element.closest("[lang]")?.getAttribute("lang");

test("folded, its button is Show, collapsed, described by the summary, and what it folds is hidden", () => {
  warning();

  const button = screen.getByRole("button", { name: "Show", expanded: false, description: "Spoilers for the finale" });
  expect(folded()).toBeNull();
  // It controls what it folds, which is there to be found, hidden.
  const controlled = document.getElementById(button.getAttribute("aria-controls") ?? "");
  expect(controlled?.contains(screen.getByText("The ending"))).toBe(true);
});

test("the summary stays in reach while it is folded", () => {
  warning();

  expect(screen.getByText("Spoilers for the finale").closest('[aria-hidden="true"], [hidden]')).toBeNull();
});

test("expanded at first, its button is Hide, expanded, and what it folds is in reach", () => {
  warning({ defaultExpanded: true });

  expect(
    screen.getByRole("button", { name: "Hide", expanded: true, description: "Spoilers for the finale" }),
  ).not.toBeNull();
  expect(folded()).not.toBeNull();
});

test("a press, Enter and Space each toggle it, and its button keeps the focus", async () => {
  const user = userEvent.setup();
  warning();

  await user.click(screen.getByRole("button", { name: "Show", expanded: false }));
  const button = screen.getByRole("button", { name: "Hide", expanded: true });
  expect(folded()).not.toBeNull();
  expect(document.activeElement).toBe(button);

  await user.keyboard("{Enter}");
  expect(screen.getByRole("button", { name: "Show", expanded: false })).toBe(button);
  expect(folded()).toBeNull();

  await user.keyboard(" ");
  expect(screen.getByRole("button", { name: "Hide", expanded: true })).toBe(button);
  expect(folded()).not.toBeNull();
  expect(document.activeElement).toBe(button);
});

test("Tab reaches its button, then what it shows once expanded", async () => {
  const user = userEvent.setup();
  warning({ defaultExpanded: true });

  await user.tab();
  expect(document.activeElement).toBe(screen.getByRole("button", { name: "Hide" }));
  await user.tab();
  expect(document.activeElement).toBe(screen.getByRole("link", { name: "The ending" }));
});

test("the button's label is in the reader's locale, marked with its language; another language reads en", () => {
  for (const [locale, name, lang] of [
    ["ja", "表示", "ja"],
    ["ja-JP", "表示", "ja"],
    ["it", "Show", "en"],
  ] as const) {
    const { unmount } = warning({ locale });
    expect(languageOf(screen.getByText(name))).toBe(lang);
    expect(screen.getByRole("button", { name, expanded: false })).not.toBeNull();
    unmount();
  }
});

test("the label reads the pseudo-locales made from en, so a string left out of the catalog shows", () => {
  for (const locale of ["en-XA", "ar-XB"]) {
    const { unmount } = warning({ locale });
    const button = screen.getByRole("button", { expanded: false });
    expect(button.textContent).not.toBe("");
    expect(button.textContent).not.toBe("Show");
    expect(languageOf(screen.getByText(button.textContent))).toBe(locale);
    unmount();
  }
});

/** The direction the element's words run in, as the nearest element that sets one says. */
const directionOf = (element: Element) => element.closest("[dir]")?.getAttribute("dir");

test("without a locale of its own it reads the locale and the direction of the I18nProvider around it", () => {
  for (const [locale, lang, direction] of [
    ["ja", "ja", "ltr"],
    ["ar-XB", "ar-XB", "rtl"],
  ] as const) {
    const { unmount } = render(
      <I18nProvider locale={locale}>
        <Disclosure summary="Spoilers" icon="hidden">
          <a href="/posts/1">The ending</a>
        </Disclosure>
      </I18nProvider>,
    );
    const button = screen.getByRole("button", { expanded: false });
    expect(languageOf(screen.getByText(button.textContent))).toBe(lang);
    expect(directionOf(button)).toBe(direction);
    expect(directionOf(screen.getByText("Spoilers"))).toBe(direction);
    unmount();
  }
  render(
    <I18nProvider locale="ja">
      <Disclosure summary="Spoilers">
        <a href="/posts/1">The ending</a>
      </Disclosure>
    </I18nProvider>,
  );
  expect(screen.getByRole("button", { name: "表示", expanded: false })).not.toBeNull();
});
