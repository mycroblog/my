// SPDX-License-Identifier: AGPL-3.0-only
import { render, screen } from "@testing-library/react";
import { userEvent } from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { Button } from "./Button";

describe("Button", () => {
  it("is a button named by its children", () => {
    render(<Button>Post</Button>);
    expect(screen.getByRole("button", { name: "Post" })).toBeInTheDocument();
  });

  it("is pressed with a pointer, Enter and Space", async () => {
    const user = userEvent.setup();
    const onPress = vi.fn();
    render(<Button onPress={onPress}>Post</Button>);
    const button = screen.getByRole("button", { name: "Post" });

    await user.click(button);
    await user.tab();
    await user.tab({ shift: true });
    expect(button).toHaveFocus();
    await user.keyboard("{Enter}");
    await user.keyboard(" ");
    expect(onPress).toHaveBeenCalledTimes(3);
  });

  it("ignores presses while disabled and is skipped by Tab", async () => {
    const user = userEvent.setup();
    const onPress = vi.fn();
    render(
      <Button isDisabled onPress={onPress}>
        Post
      </Button>,
    );
    const button = screen.getByRole("button", { name: "Post" });
    await user.click(button);
    await user.tab();
    expect(onPress).not.toHaveBeenCalled();
    expect(button).toBeDisabled();
    expect(button).not.toHaveFocus();
  });

  it("ignores presses while pending but stays focusable", async () => {
    const user = userEvent.setup();
    const onPress = vi.fn();
    render(
      <Button isPending onPress={onPress}>
        Post
      </Button>,
    );
    await user.tab();
    const button = screen.getByRole("button", { name: "Post" });
    expect(button).toHaveFocus();
    await user.keyboard("{Enter}");
    expect(onPress).not.toHaveBeenCalled();
  });

  it("as a submit button, takes the focus when a pointer press ends, not when it starts (G23)", async () => {
    const user = userEvent.setup();
    const onSubmit = vi.fn();
    const onPress = vi.fn();
    render(
      <form
        onSubmit={(event) => {
          event.preventDefault();
          onSubmit();
        }}
      >
        <input aria-label="Reason" />
        <Button type="submit" onPress={onPress}>
          Send
        </Button>
      </form>,
    );
    const field = screen.getByRole("textbox", { name: "Reason" });
    const button = screen.getByRole("button", { name: "Send" });
    await user.click(field);

    // A field that kept the focus through the press does not commit its validation under the pointer.
    await user.pointer({ keys: "[MouseLeft>]", target: button });
    expect(field).toHaveFocus();
    await user.pointer({ keys: "[/MouseLeft]", target: button });
    expect(button).toHaveFocus();
    expect(onPress).toHaveBeenCalledTimes(1);
    expect(onSubmit).toHaveBeenCalledTimes(1);

    // Enter and Space already have the focus on it.
    await user.keyboard("{Enter}");
    expect(onSubmit).toHaveBeenCalledTimes(2);

    // Enter in the field submits the form and leaves the focus in the field.
    await user.click(field);
    await user.keyboard("{Enter}");
    expect(onSubmit).toHaveBeenCalledTimes(3);
    expect(field).toHaveFocus();
  });

  it("leaves the focus of other buttons to the browser", async () => {
    const user = userEvent.setup();
    render(
      <>
        <input aria-label="Reason" />
        <Button>Cancel</Button>
      </>,
    );
    await user.click(screen.getByRole("textbox", { name: "Reason" }));
    const button = screen.getByRole("button", { name: "Cancel" });
    await user.pointer({ keys: "[MouseLeft>]", target: button });
    expect(button).toHaveFocus();
  });

  it("merges className into the recipe, the later class winning", () => {
    render(
      <Button variant="primary" className="px-8">
        Post
      </Button>,
    );
    const button = screen.getByRole("button", { name: "Post" });
    expect(button).toHaveClass("px-8", "bg-accent-fill");
    expect(button).not.toHaveClass("px-4");
  });
});
