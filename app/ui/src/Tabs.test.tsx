// SPDX-License-Identifier: AGPL-3.0-only
import { render, screen } from "@testing-library/react";
import { userEvent } from "@testing-library/user-event";
import { I18nProvider } from "react-aria-components";
import { describe, expect, it, vi } from "vitest";
import { Tab, TabList, TabPanel, Tabs } from "./Tabs";

function Timelines({
  locale,
  selectedKey,
  onSelectionChange,
}: {
  locale: string;
  selectedKey?: string;
  onSelectionChange?: (key: unknown) => void;
}) {
  return (
    <I18nProvider locale={locale}>
      <Tabs
        {...(selectedKey === undefined ? {} : { selectedKey })}
        {...(onSelectionChange === undefined ? {} : { onSelectionChange })}
      >
        <TabList aria-label="Timelines">
          <Tab id="home">Home</Tab>
          <Tab id="local">Local</Tab>
        </TabList>
        <TabPanel id="home">Home posts</TabPanel>
        <TabPanel id="local">Local posts</TabPanel>
      </Tabs>
    </I18nProvider>
  );
}

describe("Tabs", () => {
  it.each([
    ["en", "{ArrowRight}"],
    ["ar", "{ArrowLeft}"],
  ])("moves to the next tab in the reading direction (%s)", async (locale, next) => {
    const user = userEvent.setup();
    render(<Timelines locale={locale} />);
    await user.tab();
    await user.keyboard(next);
    expect(screen.getByRole("tab", { name: "Local" })).toHaveAttribute("aria-selected", "true");
    expect(screen.getByRole("tabpanel")).toHaveTextContent("Local posts");
  });

  it("shows the controlled selection and reports changes", async () => {
    const user = userEvent.setup();
    const onSelectionChange = vi.fn();
    render(<Timelines locale="en" selectedKey="local" onSelectionChange={onSelectionChange} />);
    expect(screen.getByRole("tab", { name: "Local" })).toHaveAttribute("aria-selected", "true");
    await user.click(screen.getByRole("tab", { name: "Home" }));
    expect(onSelectionChange).toHaveBeenCalledWith("home");
    // Still controlled: the selection changes only when selectedKey does.
    expect(screen.getByRole("tab", { name: "Local" })).toHaveAttribute("aria-selected", "true");
  });
});
