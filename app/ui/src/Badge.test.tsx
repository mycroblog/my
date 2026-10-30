// SPDX-License-Identifier: AGPL-3.0-only
import { render, screen } from "@testing-library/react";
import { I18nProvider } from "react-aria-components";
import { describe, expect, it } from "vitest";
import { Badge } from "./Badge";

describe("Badge", () => {
  it("is read as its label, not as the drawn digits", () => {
    render(
      <a href="#n">
        Notifications <Badge count={3} label="3 unread" />
      </a>,
    );
    expect(screen.getByRole("link", { name: "Notifications 3 unread" })).toBeInTheDocument();
  });

  it("caps the drawn number at max", () => {
    render(<Badge count={150} label="150 unread" />);
    expect(screen.getByText("99+")).toHaveAttribute("aria-hidden", "true");
    expect(screen.getByText("150 unread")).toBeInTheDocument();
  });

  it("takes a custom max and formats for the locale", () => {
    render(
      <I18nProvider locale="en-US">
        <Badge count={12345} max={9999} label="many" />
        <Badge count={1234} max={9999} label="some" />
      </I18nProvider>,
    );
    expect(screen.getByText("9,999+")).toBeInTheDocument();
    expect(screen.getByText("1,234")).toBeInTheDocument();
  });

  it("draws the count over its limit, uncapped", () => {
    render(
      <I18nProvider locale="en-US">
        <Badge count={120} limit={1000} label="120 of 1,000 members" />
      </I18nProvider>,
    );
    expect(screen.getByText("120 / 1,000")).toHaveAttribute("aria-hidden", "true");
    expect(screen.getByText("120 of 1,000 members")).toBeInTheDocument();
  });
});
