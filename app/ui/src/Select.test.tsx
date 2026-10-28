// SPDX-License-Identifier: AGPL-3.0-only
import { render, screen, waitFor } from "@testing-library/react";
import { userEvent } from "@testing-library/user-event";
import { useState } from "react";
import type { Key } from "react-aria-components";
import { describe, expect, it, vi } from "vitest";
import { Select, SelectItem } from "./Select";

const languages = [
  { id: "en", name: "English" },
  { id: "ja", name: "日本語" },
  { id: "ar", name: "العربية" },
];

describe("Select", () => {
  it("names the trigger by the label and the value, and links the description", () => {
    render(
      <Select label="Language" description="Used for new posts." placeholder="Choose a language">
        <SelectItem id="en">English</SelectItem>
      </Select>,
    );
    const trigger = screen.getByRole("button", { name: /Language/ });
    expect(trigger).toHaveTextContent("Choose a language");
    expect(trigger).toHaveAccessibleDescription("Used for new posts.");
  });

  it("opens with the keyboard, moves with the arrow keys and selects with Enter", async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    render(
      <Select label="Language" items={languages} onChange={onChange}>
        {(language) => <SelectItem id={language.id}>{language.name}</SelectItem>}
      </Select>,
    );
    await user.tab();
    await user.keyboard("{ArrowDown}");
    const listbox = await screen.findByRole("listbox", { name: "Language" });
    expect(listbox).toBeInTheDocument();
    expect(screen.getByRole("option", { name: "English" })).toHaveFocus();
    await user.keyboard("{ArrowDown}{Enter}");
    expect(onChange).toHaveBeenCalledWith("ja");
    expect(screen.queryByRole("listbox")).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: /Language/ })).toHaveTextContent("日本語");
  });

  it("is controlled with value and onChange", async () => {
    const user = userEvent.setup();
    function Controlled() {
      const [value, setValue] = useState<Key | null>("ar");
      return (
        <>
          <Select label="Language" value={value} onChange={setValue}>
            <SelectItem id="en">English</SelectItem>
            <SelectItem id="ar">العربية</SelectItem>
          </Select>
          <output>{String(value)}</output>
        </>
      );
    }
    render(<Controlled />);
    await user.click(screen.getByRole("button", { name: /Language/ }));
    await user.click(await screen.findByRole("option", { name: "English" }));
    expect(screen.getByRole("status")).toHaveTextContent("en");
  });

  it("closes with Escape and returns focus to the trigger", async () => {
    const user = userEvent.setup();
    render(
      <Select label="Language">
        <SelectItem id="en">English</SelectItem>
      </Select>,
    );
    const trigger = screen.getByRole("button", { name: /Language/ });
    await user.click(trigger);
    expect(await screen.findByRole("listbox")).toBeInTheDocument();
    await user.keyboard("{Escape}");
    expect(screen.queryByRole("listbox")).not.toBeInTheDocument();
    // React Aria restores focus in the next animation frame.
    await waitFor(() => {
      expect(trigger).toHaveFocus();
    });
  });

  it("links the error message while invalid", () => {
    render(
      <Select label="Language" isInvalid errorMessage="Choose a language.">
        <SelectItem id="en">English</SelectItem>
      </Select>,
    );
    expect(screen.getByRole("button", { name: /Language/ })).toHaveAccessibleDescription("Choose a language.");
  });

  it("names an option by its label and describes it by its description", async () => {
    const user = userEvent.setup();
    render(
      <Select label="Time zone">
        <SelectItem id="America/New_York" textValue="Eastern Time" description="America/New_York">
          Eastern Time
        </SelectItem>
        <SelectItem id="America/Detroit" textValue="Eastern Time" description="America/Detroit">
          Eastern Time
        </SelectItem>
      </Select>,
    );
    await user.click(screen.getByRole("button", { name: /Time zone/ }));
    const options = await screen.findAllByRole("option", { name: "Eastern Time" });
    expect(options[0]).toHaveAccessibleDescription("America/New_York");
    expect(options[1]).toHaveAccessibleDescription("America/Detroit");
  });

  it("in a toolbar, shows the chosen option after its icon and names the button by the option and the aria-label", () => {
    render(
      <Select
        variant="toolbar"
        aria-label="Language"
        icon={<span aria-hidden="true">★</span>}
        items={languages}
        defaultValue="ja"
      >
        {(language) => <SelectItem id={language.id}>{language.name}</SelectItem>}
      </Select>,
    );
    const trigger = screen.getByRole("button", { name: "日本語 Language" });
    // The icon comes first and stays out of the name.
    expect(trigger).toHaveTextContent(/^★日本語/);
  });

  it("with a tooltip, shows it on keyboard focus, hides it on Esc, and keeps it closed while the list is open", async () => {
    const user = userEvent.setup();
    render(
      <Select variant="toolbar" aria-label="Language" tooltip="Language: English" items={languages} defaultValue="en">
        {(language) => <SelectItem id={language.id}>{language.name}</SelectItem>}
      </Select>,
    );
    const trigger = screen.getByRole("button", { name: "English Language" });
    await user.tab();
    expect(trigger).toHaveFocus();
    expect(await screen.findByRole("tooltip")).toHaveTextContent("Language: English");
    // The tooltip describes the button; the name stays the option and the aria-label.
    expect(trigger).toHaveAccessibleDescription("Language: English");
    await user.keyboard("{Escape}");
    await waitFor(() => {
      expect(screen.queryByRole("tooltip")).not.toBeInTheDocument();
    });
    // The select's own keys still open the list, and no tooltip comes with it.
    await user.keyboard("{Enter}");
    expect(await screen.findByRole("listbox")).toBeInTheDocument();
    expect(screen.queryByRole("tooltip")).not.toBeInTheDocument();
    await user.keyboard("{ArrowDown}{Enter}");
    await waitFor(() => {
      expect(screen.queryByRole("listbox")).not.toBeInTheDocument();
    });
    expect(trigger).toHaveTextContent("日本語");
  });
});
