// SPDX-License-Identifier: AGPL-3.0-only
import { render, screen } from "@testing-library/react";
import { userEvent } from "@testing-library/user-event";
import { useState } from "react";
import { describe, expect, it, vi } from "vitest";
import { TextField } from "./TextField";

describe("TextField", () => {
  it("links the label and the description to the input", () => {
    render(<TextField label="Display name" description="Shown on your profile." />);
    const input = screen.getByRole("textbox", { name: "Display name" });
    expect(input).toHaveAccessibleDescription("Shown on your profile.");
  });

  it("links the error message while invalid", () => {
    render(
      <TextField label="Username" description="Letters and digits." isInvalid errorMessage="This username is taken." />,
    );
    const input = screen.getByRole("textbox", { name: "Username" });
    expect(input).toBeInvalid();
    expect(input).toHaveAccessibleDescription(expect.stringContaining("This username is taken."));
    expect(input).toHaveAccessibleDescription(expect.stringContaining("Letters and digits."));
  });

  it("shows no error message while valid", () => {
    render(<TextField label="Username" errorMessage="This username is taken." />);
    expect(screen.queryByText("This username is taken.")).not.toBeInTheDocument();
    expect(screen.getByRole("textbox", { name: "Username" })).toBeValid();
  });

  it("is uncontrolled with defaultValue", async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    render(<TextField label="Name" defaultValue="Al" onChange={onChange} />);
    const input = screen.getByRole("textbox", { name: "Name" });
    await user.type(input, "ice");
    expect(input).toHaveValue("Alice");
    expect(onChange).toHaveBeenLastCalledWith("Alice");
  });

  it("is controlled with value and onChange", async () => {
    const user = userEvent.setup();
    function Upper() {
      const [value, setValue] = useState("");
      return (
        <TextField
          label="Code"
          value={value}
          onChange={(next) => {
            setValue(next.toUpperCase());
          }}
        />
      );
    }
    render(<Upper />);
    const input = screen.getByRole("textbox", { name: "Code" });
    await user.type(input, "ab");
    expect(input).toHaveValue("AB");
  });

  it("is reached with Tab and accepts a placeholder", async () => {
    const user = userEvent.setup();
    render(<TextField aria-label="Search" placeholder="Search posts" />);
    await user.tab();
    const input = screen.getByRole("textbox", { name: "Search" });
    expect(input).toHaveFocus();
    expect(input).toHaveAttribute("placeholder", "Search posts");
  });

  it("puts the end action after the input in the tab order, without changing the input's name", async () => {
    const user = userEvent.setup();
    render(<TextField label="Display name" endAction={<button type="button">Add emoji</button>} />);
    await user.tab();
    expect(screen.getByRole("textbox", { name: "Display name" })).toHaveFocus();
    await user.tab();
    expect(screen.getByRole("button", { name: "Add emoji" })).toHaveFocus();
  });
});
