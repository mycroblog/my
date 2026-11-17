// SPDX-License-Identifier: AGPL-3.0-only

import { expect, test } from "bun:test";
import { render, screen } from "@testing-library/react";
import { I18nProvider } from "react-aria-components";
import { RelativeTime } from "./time";

test("a time carries its moment for machines and the full date for a reader who asks, in its language", () => {
  render(<RelativeTime date="2025-12-20T09:41:00Z" locale="en" />);

  const time = screen.getByRole("time");
  expect(time.getAttribute("datetime")).toBe("2025-12-20T09:41:00.000Z");
  // The full date, at the reader's own hour.
  expect(time.title).toMatch(/^December 20, 2025 at \d{1,2}:\d{2}/);
  expect(time.textContent).toBe("Dec 20, 2025");
  expect(time.lang).toBe("en");
});

test("a time takes a Date as it takes a string", () => {
  render(<RelativeTime date={new Date("2025-12-20T09:41:00Z")} locale="en" />);

  expect(screen.getByRole("time").getAttribute("datetime")).toBe("2025-12-20T09:41:00.000Z");
});

test("a time reads in the I18nProvider's locale, and in its own over it", () => {
  render(
    <I18nProvider locale="ja">
      <p>
        <RelativeTime date="2025-12-20T09:41:00Z" />
      </p>
      <p>
        <RelativeTime date="2025-12-20T09:41:00Z" locale="en" />
      </p>
    </I18nProvider>,
  );

  const [provided, own] = screen.getAllByRole("time");
  expect([provided?.textContent, provided?.lang]).toEqual(["2025年12月20日", "ja"]);
  expect(provided?.title).toMatch(/^2025年12月20日/);
  expect([own?.textContent, own?.lang]).toEqual(["Dec 20, 2025", "en"]);
});
