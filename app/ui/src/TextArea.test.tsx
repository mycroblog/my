// SPDX-License-Identifier: AGPL-3.0-only
import { act, render, screen } from "@testing-library/react";
import { userEvent } from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { countAnnounceDelay, TextArea } from "./TextArea";

const formatRemaining = (remaining: number) => (remaining < 0 ? `${-remaining} too many` : `${remaining} left`);

describe("TextArea", () => {
  it("links the label, description and error message to the text area", () => {
    render(<TextArea label="Bio" description="Markdown is not supported." isInvalid errorMessage="Too long." />);
    const textArea = screen.getByRole("textbox", { name: "Bio" });
    expect(textArea.tagName).toBe("TEXTAREA");
    expect(textArea).toBeInvalid();
    expect(textArea).toHaveAccessibleDescription(expect.stringContaining("Markdown is not supported."));
    expect(textArea).toHaveAccessibleDescription(expect.stringContaining("Too long."));
  });

  it("runs validate and stops its form while the text is invalid", async () => {
    const user = userEvent.setup();
    const onSubmit = vi.fn((event: SubmitEvent | { preventDefault: () => void }) => {
      event.preventDefault();
    });
    render(
      <form onSubmit={onSubmit}>
        <TextArea
          label="Reason"
          validate={(value) => (value.trim() === "" ? "Enter a reason." : null)}
          errorMessage="Enter a reason."
          characterLimit={10}
          formatRemaining={formatRemaining}
        />
        <button type="submit">Send</button>
      </form>,
    );
    const textArea = screen.getByRole("textbox", { name: "Reason" });
    await user.type(textArea, "   ");
    await user.click(screen.getByRole("button", { name: "Send" }));
    expect(onSubmit).not.toHaveBeenCalled();
    expect(textArea).toBeInvalid();
    expect(screen.getByText("Enter a reason.")).toBeInTheDocument();

    await user.type(textArea, "spam");
    await user.click(screen.getByRole("button", { name: "Send" }));
    expect(onSubmit).toHaveBeenCalledTimes(1);
  });

  it("has no count or status region without a limit", () => {
    render(<TextArea label="Bio" />);
    expect(screen.queryByRole("status")).not.toBeInTheDocument();
  });

  it("is controlled with value and onChange", async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    render(
      <TextArea label="Bio" value="fixed" onChange={onChange} characterLimit={10} formatRemaining={formatRemaining} />,
    );
    const textArea = screen.getByRole("textbox", { name: "Bio" });
    await user.type(textArea, "!");
    expect(onChange).toHaveBeenCalledWith("fixed!");
    expect(textArea).toHaveValue("fixed");
    expect(textArea).toHaveAccessibleDescription("5 left");
  });

  describe("character count", () => {
    beforeEach(() => {
      // Testing Library waits on real timeouts inside act(), so the fake clock also follows real time.
      vi.useFakeTimers({ toFake: ["setTimeout", "clearTimeout"], shouldAdvanceTime: true });
    });
    afterEach(() => {
      vi.useRealTimers();
    });

    // Without a delay between keystrokes, typing never waits on the fake clock.
    const setup = () => userEvent.setup({ delay: null });

    it("describes the text area with the current count", async () => {
      const user = setup();
      render(<TextArea label="Post" characterLimit={100} formatRemaining={formatRemaining} />);
      const textArea = screen.getByRole("textbox", { name: "Post" });
      expect(textArea).toHaveAccessibleDescription("100 left");
      await user.type(textArea, "hello");
      expect(textArea).toHaveAccessibleDescription("95 left");
    });

    it("counts code points, as the server does", async () => {
      const user = setup();
      render(<TextArea label="Post" characterLimit={10} formatRemaining={formatRemaining} />);
      const textArea = screen.getByRole("textbox", { name: "Post" });
      await user.type(textArea, "😀𠮷");
      expect(textArea).toHaveAccessibleDescription("8 left");
    });

    it("announces the count only once typing pauses", async () => {
      const user = setup();
      render(<TextArea label="Post" characterLimit={100} formatRemaining={formatRemaining} />);
      const status = screen.getByRole("status");
      await user.type(screen.getByRole("textbox", { name: "Post" }), "abc");
      expect(status).toBeEmptyDOMElement();
      // Real time also advances the fake clock a little (shouldAdvanceTime), so check well before the delay.
      act(() => {
        vi.advanceTimersByTime(countAnnounceDelay / 2);
      });
      expect(status).toBeEmptyDOMElement();
      act(() => {
        vi.advanceTimersByTime(countAnnounceDelay / 2);
      });
      expect(status).toHaveTextContent("97 left");
    });

    it("announces at once when the remaining count reaches 50, 20 and 0", async () => {
      const user = setup();
      render(<TextArea label="Post" characterLimit={52} formatRemaining={formatRemaining} />);
      const textArea = screen.getByRole("textbox", { name: "Post" });
      const status = screen.getByRole("status");

      await user.type(textArea, "a");
      expect(status).toBeEmptyDOMElement();
      await user.type(textArea, "b");
      expect(status).toHaveTextContent("50 left");

      await user.type(textArea, "c".repeat(30));
      expect(status).toHaveTextContent("20 left");

      await user.type(textArea, "d".repeat(20));
      expect(status).toHaveTextContent("0 left");
    });

    it("marks the field invalid over the limit without cutting the text", async () => {
      const user = setup();
      render(<TextArea label="Post" characterLimit={3} formatRemaining={formatRemaining} />);
      const textArea = screen.getByRole("textbox", { name: "Post" });
      await user.type(textArea, "abcde");
      expect(textArea).toHaveValue("abcde");
      expect(textArea).toBeInvalid();
      expect(textArea).toHaveAccessibleDescription("2 too many");
      act(() => {
        vi.advanceTimersByTime(countAnnounceDelay);
      });
      expect(screen.getByRole("status")).toHaveTextContent("2 too many");
    });

    it("keeps a description passed with aria-describedby", () => {
      render(
        <>
          <p id="hint">Be kind.</p>
          <TextArea label="Post" aria-describedby="hint" characterLimit={5} formatRemaining={formatRemaining} />
        </>,
      );
      expect(screen.getByRole("textbox", { name: "Post" })).toHaveAccessibleDescription("Be kind. 5 left");
    });
  });

  it("puts the end action after the text area in the tab order, without changing its name or description", async () => {
    const user = userEvent.setup();
    render(
      <TextArea
        label="Post"
        characterLimit={5}
        formatRemaining={formatRemaining}
        endAction={<button type="button">Add emoji</button>}
      />,
    );
    await user.tab();
    const textArea = screen.getByRole("textbox", { name: "Post" });
    expect(textArea).toHaveFocus();
    expect(textArea).toHaveAccessibleDescription("5 left");
    await user.tab();
    expect(screen.getByRole("button", { name: "Add emoji" })).toHaveFocus();
  });
});
