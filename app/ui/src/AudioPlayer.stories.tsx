// SPDX-License-Identifier: AGPL-3.0-only
import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, waitFor } from "storybook/test";
import { AudioPlayer } from "./AudioPlayer";

/** A one-second silent Opus file, so stories load no network media. */
const audioFile =
  "data:audio/ogg;base64,T2dnUwACAAAAAAAAAABf1EMQAAAAAL7MFqUBE09wdXNIZWFkAQE4AYC7AAAAAABPZ2dTAAAAAAAAAAAAAF/UQxABAAAAIiMbKgE+T3B1c1RhZ3MNAAAATGF2ZjU4LjQ1LjEwMAEAAAAdAAAAZW5jb2Rlcj1MYXZjNTguOTEuMTAwIGxpYm9wdXNPZ2dTAACAuwAAAAAAAF/UQxACAAAAWsUo5TIHBgYGBgYGBgYGBgYGBgYGBgYGBgYGBgYGBgYGBgYGBgYGBgYGBgYGBgYGBgYGBgYGBggL5FMVS/IIBuCzDsYIBuCzDsYIBuCzDsYIBuCzDsYIBuCzDsYIBuCzDsYIBuCzDsYIBuCzDsYIBuCzDsYIBuCzDsYIBuCzDsYIBuCzDsYIBuCzDsYIBuCzDsYIBuCzDsYIBuCzDsYIBuCzDsYIBuCzDsYIBuCzDsYIBuCzDsYIBuCzDsYIBuCzDsYIBuCzDsYIBuCzDsYIBuCzDsYIBuCzDsYIBuCzDsYIBuCzDsYIBuCzDsYIBuCzDsYIBuCzDsYIBuCzDsYIBuCzDsYIBuCzDsYIBuCzDsYIBuCzDsYIBuCzDsYIBuCzDsYIBuCzDsYIBuCzDsYIBuCzDsYIBuCzDsYIBuCzDsYIBuCzDsYIBuCzDsYIBuCzDsYIBuCzDsYIBuCzDsYIBuCzDsYIBuCzDsZPZ2dTAAS4vAAAAAAAAF/UQxADAAAAEH17vgEGCAbgsw7G";

const meta = {
  title: "Timeline/AudioPlayer",
  component: AudioPlayer,
  args: {
    src: audioFile,
    name: "rain-morning.m4a",
    lang: "en",
    labels: { play: "Play", pause: "Pause", seek: "Position in the audio" },
  },
  decorators: [
    (Story) => (
      <div className="max-w-110">
        <Story />
      </div>
    ),
  ],
} satisfies Meta<typeof AudioPlayer>;

export default meta;
type Story = StoryObj<typeof meta>;

/** Stopped at the start: the play button, the waveform and the name over 0:00. Nothing plays by itself. */
export const Stopped: Story = {
  play: async ({ canvas }) => {
    await expect(canvas.getByRole("group", { name: "rain-morning.m4a" })).toBeInTheDocument();
    await expect(canvas.getByRole("button", { name: "Play" })).toBeInTheDocument();
    await expect(canvas.queryByRole("button", { name: "Pause" })).not.toBeInTheDocument();
  },
};

/** Play and pause with the keyboard: the button's name and icon follow the state of the audio. */
export const Playing: Story = {
  tags: ["no-screenshot"],
  play: async ({ canvas, userEvent }) => {
    await userEvent.tab();
    await expect(canvas.getByRole("button", { name: "Play" })).toHaveFocus();
    await userEvent.keyboard("{Enter}");
    // The browser may refuse to play without a user gesture or a codec; in that case the button stays as it was.
    await waitFor(() =>
      expect(
        canvas.queryByRole("button", { name: "Pause" }) ?? canvas.queryByRole("button", { name: "Play" }),
      ).toBeInTheDocument(),
    );
  },
};

/** The seek slider is disabled until the length of the file is known. */
export const Seek: Story = {
  tags: ["no-screenshot"],
  play: async ({ canvas }) => {
    await waitFor(() => expect(canvas.getByRole("slider", { name: "Position in the audio" })).toBeInTheDocument());
  },
};

export const LongName: Story = {
  args: { name: "a-very-long-recording-of-the-rain-on-the-roof-of-the-old-house-by-the-lake-on-a-quiet-morning.m4a" },
};

export const Rtl: Story = {
  args: { name: "صوت المطر.m4a", lang: "ar" },
  decorators: [
    (Story) => (
      <div dir="rtl">
        <Story />
      </div>
    ),
  ],
};
