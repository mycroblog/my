// SPDX-License-Identifier: AGPL-3.0-only
import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { userEvent } from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { AudioPlayer } from "./AudioPlayer";

const labels = {
  play: "Play",
  pause: "Pause",
  seek: "Position",
  loading: "Loading media",
  failed: "Couldn't load this media.",
  retry: "Try again",
};

// jsdom implements no media playback, so the element's methods and properties are replaced.
function mockMedia() {
  const play = vi.spyOn(HTMLMediaElement.prototype, "play").mockImplementation(function (this: HTMLMediaElement) {
    Object.defineProperty(this, "paused", { configurable: true, value: false });
    this.dispatchEvent(new Event("play"));
    return Promise.resolve();
  });
  const pause = vi.spyOn(HTMLMediaElement.prototype, "pause").mockImplementation(function (this: HTMLMediaElement) {
    Object.defineProperty(this, "paused", { configurable: true, value: true });
    this.dispatchEvent(new Event("pause"));
  });
  return { play, pause };
}

afterEach(() => {
  vi.restoreAllMocks();
});

describe("AudioPlayer", () => {
  it("is a named group with a play button and does not play by itself", () => {
    const { play } = mockMedia();
    const { container } = render(<AudioPlayer src="/rain.m4a" name="rain.m4a" labels={labels} />);
    expect(screen.getByRole("group", { name: "rain.m4a" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Play" })).toBeInTheDocument();
    // eslint-disable-next-line testing-library/no-container, testing-library/no-node-access -- <audio> has no ARIA role.
    const audio = container.querySelector("audio");
    expect(audio).toHaveAttribute("src", "/rain.m4a");
    expect(audio).not.toHaveAttribute("controls");
    expect(audio).not.toHaveAttribute("autoplay");
    expect(play).not.toHaveBeenCalled();
    expect(screen.getByText("0:00 / 0:00")).toBeInTheDocument();
  });

  it("toggles between play and pause from the keyboard, renaming the button", async () => {
    const { play, pause } = mockMedia();
    render(<AudioPlayer src="/rain.m4a" name="rain.m4a" labels={labels} />);
    await userEvent.tab();
    await userEvent.keyboard("{Enter}");
    expect(play).toHaveBeenCalledTimes(1);
    expect(await screen.findByRole("button", { name: "Pause" })).toHaveFocus();
    await userEvent.keyboard(" ");
    expect(pause).toHaveBeenCalledTimes(1);
    expect(await screen.findByRole("button", { name: "Play" })).toBeInTheDocument();
  });

  it("shows the length once known and seeks with the arrow keys", async () => {
    mockMedia();
    const { container } = render(<AudioPlayer src="/rain.m4a" name="rain.m4a" labels={labels} />);
    // eslint-disable-next-line testing-library/no-container, testing-library/no-node-access -- <audio> has no ARIA role.
    const audio = container.querySelector("audio");
    if (audio === null) throw new Error("no audio element");
    Object.defineProperty(audio, "duration", { configurable: true, value: 38 });
    fireEvent(audio, new Event("durationchange"));
    expect(await screen.findByText("0:00 / 0:38")).toBeInTheDocument();
    const slider = screen.getByRole("slider", { name: "Position" });
    slider.focus();
    await userEvent.keyboard("{ArrowRight}");
    expect(audio.currentTime).toBe(1);
  });

  it("shows the wait after Play, and a file that could not load with Try again", async () => {
    let reject: (error: unknown) => void = () => undefined;
    const play = vi
      .spyOn(HTMLMediaElement.prototype, "play")
      .mockImplementation(() => new Promise<void>((_, onRejected) => (reject = onRejected)));
    const load = vi.spyOn(HTMLMediaElement.prototype, "load").mockImplementation(() => undefined);
    render(<AudioPlayer src="/rain.m4a" name="rain.m4a" labels={labels} />);
    await userEvent.click(screen.getByRole("button", { name: "Play" }));
    expect(screen.getByRole("status")).toHaveTextContent("Loading media");
    expect(screen.getByRole("button", { name: "Play" })).toHaveAttribute("data-waiting");
    reject(new DOMException("unsupported", "NotSupportedError"));
    const failure = await screen.findByRole("group", { name: "Couldn't load this media." });
    expect(screen.getByRole("status")).toHaveTextContent("");
    await userEvent.click(within(failure).getByRole("button", { name: "Try again" }));
    expect(load).toHaveBeenCalledOnce();
    expect(play).toHaveBeenCalledTimes(2);
    expect(screen.queryByRole("group", { name: "Couldn't load this media." })).not.toBeInTheDocument();
  });

  it("leaves the button as it was when the browser refuses to play without a gesture", async () => {
    vi.spyOn(HTMLMediaElement.prototype, "play").mockRejectedValue(new DOMException("no", "NotAllowedError"));
    render(<AudioPlayer src="/rain.m4a" name="rain.m4a" labels={labels} />);
    await userEvent.click(screen.getByRole("button", { name: "Play" }));
    await waitFor(() => {
      expect(screen.getByRole("button", { name: "Play" })).not.toHaveAttribute("data-waiting");
    });
    expect(screen.queryByRole("group", { name: "Couldn't load this media." })).not.toBeInTheDocument();
  });
});
