// SPDX-License-Identifier: AGPL-3.0-only
import { render, screen } from "@testing-library/react";
import { userEvent } from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { Radio, RadioGroup } from "./RadioGroup";

function Visibility(props: { onChange?: (value: string) => void; isInvalid?: boolean }) {
  return (
    <RadioGroup
      label="Visibility"
      description="Who can see this post."
      errorMessage="Choose who can see it."
      defaultValue="public"
      {...props}
    >
      <Radio value="public" description="Anyone, on any server.">
        Public
      </Radio>
      <Radio value="followers">Followers</Radio>
      <Radio value="direct" isDisabled>
        Direct
      </Radio>
    </RadioGroup>
  );
}

describe("RadioGroup", () => {
  it("is a radiogroup named by its label and described by its description", () => {
    render(<Visibility />);
    const group = screen.getByRole("radiogroup", { name: "Visibility" });
    expect(group).toHaveAccessibleDescription("Who can see this post.");
    expect(screen.getByRole("radio", { name: "Public" })).toBeChecked();
    expect(screen.getByRole("radio", { name: "Public" })).toHaveAccessibleDescription(
      "Anyone, on any server. Who can see this post.",
    );
  });

  it("moves and selects with the arrow keys, skipping disabled options", async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    render(<Visibility onChange={onChange} />);
    await user.tab();
    expect(screen.getByRole("radio", { name: "Public" })).toHaveFocus();
    await user.keyboard("{ArrowDown}");
    const followers = screen.getByRole("radio", { name: "Followers" });
    expect(followers).toHaveFocus();
    expect(followers).toBeChecked();
    await user.keyboard("{ArrowDown}");
    expect(screen.getByRole("radio", { name: "Public" })).toHaveFocus();
    expect(onChange.mock.calls).toEqual([["followers"], ["public"]]);
  });

  it("is a single Tab stop", async () => {
    const user = userEvent.setup();
    render(<Visibility />);
    await user.tab();
    await user.tab();
    expect(document.body).toHaveFocus();
  });

  it("links the error message while invalid", () => {
    render(<Visibility isInvalid />);
    const group = screen.getByRole("radiogroup", { name: "Visibility" });
    expect(group).toHaveAccessibleDescription(expect.stringContaining("Choose who can see it."));
  });
});
