// SPDX-License-Identifier: AGPL-3.0-only
import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { userEvent } from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { ariaSnapshot } from "../test/aria-snapshot";
import { MediaGrid, type MediaItem } from "./MediaGrid";
import { AutoplayContext } from "./Picture";

const audioLabels = { play: "Play", pause: "Pause", seek: "Position" };

const items: readonly MediaItem[] = [
  { id: "a", src: "/a.png", alt: "A cat on a sofa", lang: "en" },
  { id: "b", src: "/b.png", alt: null },
];

describe("MediaGrid", () => {
  it("names each picture by its description, or says it has none", () => {
    const { container } = render(
      <MediaGrid
        items={items}
        noDescriptionLabel="No description"
        revealLabel="Show sensitive media"
        audioLabels={audioLabels}
      />,
    );
    expect(ariaSnapshot(container)).toBe(
      [`- img "A cat on a sofa" [lang=en] [dir=auto]`, `- img "No description"`].join("\n"),
    );
    // Nothing is drawn for the picture without a description; its name already says it to screen readers.
    expect(screen.queryByText("No description")).not.toBeInTheDocument();
    expect(screen.queryByRole("button")).not.toBeInTheDocument();
  });

  it("draws a decorative note badge on a picture that has a description, and on no other", () => {
    const props = { noDescriptionLabel: "No description", revealLabel: "Show sensitive media", audioLabels };
    const { container, rerender } = render(<MediaGrid items={items.slice(0, 1)} {...props} />);
    // eslint-disable-next-line testing-library/no-container, testing-library/no-node-access -- a decorative icon has no role.
    expect(container.querySelectorAll('span[aria-hidden="true"] > svg')).toHaveLength(1);
    rerender(<MediaGrid items={items.slice(1)} {...props} />);
    // eslint-disable-next-line testing-library/no-container, testing-library/no-node-access -- a decorative icon has no role.
    expect(container.querySelector("svg")).toBeNull();
  });

  it("shows at most four pictures", () => {
    const many = ["1", "2", "3", "4", "5"].map((id) => ({ id, src: `/${id}.png`, alt: `Picture ${id}` }));
    render(
      <MediaGrid
        items={many}
        noDescriptionLabel="No description"
        revealLabel="Show sensitive media"
        audioLabels={audioLabels}
      />,
    );
    expect(screen.getAllByRole("img")).toHaveLength(4);
  });

  it("marks the attachments it leaves out with a link to the post that has them", () => {
    const props = { items, noDescriptionLabel: "No description", revealLabel: "Show sensitive media", audioLabels };
    const { rerender } = render(<MediaGrid {...props} />);
    expect(screen.queryByRole("link")).not.toBeInTheDocument();
    rerender(
      <MediaGrid
        {...props}
        more={{ text: "+2", label: "+2 more — see the original post", href: "https://remote.example/@carol/1" }}
      />,
    );
    const link = screen.getByRole("link", { name: "+2 more — see the original post" });
    expect(link).toHaveTextContent("+2");
    expect(link).toHaveAttribute("href", "https://remote.example/@carol/1");
    expect(link).toHaveAttribute("target", "_blank");
    expect(link).toHaveAttribute("rel", "noopener noreferrer");
  });

  it("conceals sensitive media until the toggle is pressed, keeping focus on it", async () => {
    const user = userEvent.setup();
    const onRevealedChange = vi.fn();
    render(
      <MediaGrid
        items={items}
        sensitive
        noDescriptionLabel="No description"
        revealLabel="Show sensitive media"
        audioLabels={audioLabels}
        onRevealedChange={onRevealedChange}
      />,
    );
    expect(screen.queryByRole("img")).not.toBeInTheDocument();
    const toggle = screen.getByRole("button", { name: "Show sensitive media" });
    expect(toggle).toHaveAttribute("aria-pressed", "false");

    await user.tab();
    await user.keyboard(" ");
    expect(toggle).toHaveAttribute("aria-pressed", "true");
    expect(toggle).toHaveFocus();
    expect(screen.getAllByRole("img")).toHaveLength(2);

    await user.keyboard(" ");
    expect(screen.queryByRole("img")).not.toBeInTheDocument();
    expect(onRevealedChange.mock.calls).toEqual([[true], [false]]);
  });

  it("covers sensitive media with their blur until the picture has loaded", async () => {
    const user = userEvent.setup();
    const blurred: readonly MediaItem[] = [
      { id: "a", src: "/a.png", alt: "A cat on a sofa", blurhash: "LEHV6nWB2yk8pyo0adR*.7kCMdnj" },
      { id: "b", src: "/b.png", alt: "A dog", blurhash: "not a blurhash" },
      { id: "c", src: "/c.png", alt: "A bird" },
    ];
    const { container } = render(
      <MediaGrid
        items={blurred}
        sensitive
        noDescriptionLabel="No description"
        revealLabel="Show sensitive media"
        audioLabels={audioLabels}
      />,
    );
    // Only a valid blurhash draws a blur; the other cells keep their plain surface. The blur is decorative.
    // eslint-disable-next-line testing-library/no-container, testing-library/no-node-access -- a decorative canvas has no role.
    const blurs = () => container.querySelectorAll("canvas");
    expect(blurs()).toHaveLength(1);
    expect(blurs()[0]).toHaveAttribute("aria-hidden", "true");

    await user.click(screen.getByRole("button", { name: "Show sensitive media" }));
    expect(blurs()).toHaveLength(1);
    fireEvent.load(screen.getByRole("img", { name: "A cat on a sofa" }));
    expect(blurs()).toHaveLength(0);
  });

  describe("viewer", () => {
    const viewerLabels = {
      title: (position: number, count: number) => `Image ${String(position)} of ${String(count)}`,
      close: "Close",
      position: (position: number, count: number) => `${String(position)} / ${String(count)}`,
      altText: "Description",
      previous: "Previous image",
      next: "Next image",
    };
    const viewable: readonly MediaItem[] = [
      {
        id: "a",
        src: "/a-small.webp",
        alt: "A cat on a sofa",
        lang: "en",
        original: { sources: { still: [{ url: "/a-large.png" }] }, width: 1600, height: 900 },
      },
      { id: "b", src: "/b-small.webp", alt: null, original: { sources: { still: [{ url: "/b-large.png" }] } } },
    ];

    it("opens a picture at its original size, steps with the buttons and the arrow keys, and closes on Esc", async () => {
      const user = userEvent.setup();
      render(
        <MediaGrid
          items={viewable}
          noDescriptionLabel="No description"
          revealLabel="Show sensitive media"
          audioLabels={audioLabels}
          viewerLabels={viewerLabels}
        />,
      );
      const first = screen.getByRole("button", { name: "A cat on a sofa" });
      expect(first).toHaveAttribute("aria-haspopup", "dialog");

      await user.click(first);
      const dialog = await screen.findByRole("dialog", { name: "Image 1 of 2" });
      const image = within(dialog).getByRole("img", { name: "A cat on a sofa" });
      expect(image).toHaveAttribute("src", "/a-large.png");
      expect(image).toHaveAttribute("width", "1600");

      await user.click(within(dialog).getByRole("button", { name: "Next image" }));
      expect(screen.getByRole("dialog", { name: "Image 2 of 2" })).toBeInTheDocument();
      expect(within(dialog).getByRole("img", { name: "No description" })).toHaveAttribute("src", "/b-large.png");
      // The last picture goes round to the first, so the button keeps its focus.
      await user.click(within(dialog).getByRole("button", { name: "Next image" }));
      expect(screen.getByRole("dialog", { name: "Image 1 of 2" })).toBeInTheDocument();
      expect(within(dialog).getByRole("button", { name: "Next image" })).toHaveFocus();

      await user.keyboard("{ArrowLeft}");
      expect(screen.getByRole("dialog", { name: "Image 2 of 2" })).toBeInTheDocument();
      await user.keyboard("{ArrowRight}");
      expect(screen.getByRole("dialog", { name: "Image 1 of 2" })).toBeInTheDocument();

      await user.keyboard("{Escape}");
      await waitFor(() => {
        expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
      });
      await waitFor(() => {
        expect(first).toHaveFocus();
      });
    });

    it("shows the description over the picture on demand, and only for a picture that has one", async () => {
      const user = userEvent.setup();
      render(
        <MediaGrid
          items={viewable}
          noDescriptionLabel="No description"
          revealLabel="Show sensitive media"
          viewerLabels={viewerLabels}
          audioLabels={audioLabels}
        />,
      );
      await user.click(screen.getByRole("button", { name: "A cat on a sofa" }));
      const dialog = await screen.findByRole("dialog", { name: "Image 1 of 2" });
      const toggle = within(dialog).getByRole("button", { name: "Description" });
      expect(toggle).toHaveAttribute("aria-expanded", "false");
      // Hidden from screen readers, which have the picture's name already; the text is there for everyone else.
      const note = within(dialog).getByText("A cat on a sofa", { selector: "p" });
      expect(toggle).toHaveAttribute("aria-controls", note.id);
      expect(note).not.toBeVisible();

      await user.click(toggle);
      expect(toggle).toHaveAttribute("aria-expanded", "true");
      expect(note).toBeVisible();
      await user.click(toggle);
      expect(note).not.toBeVisible();

      // The next picture has no description, so it has no button.
      await user.click(within(dialog).getByRole("button", { name: "Next image" }));
      expect(within(dialog).queryByRole("button", { name: "Description" })).not.toBeInTheDocument();
    });

    it("does not open pictures without viewer labels, an original, or while they are concealed", async () => {
      const user = userEvent.setup();
      const { rerender } = render(
        <MediaGrid
          items={viewable}
          noDescriptionLabel="No description"
          revealLabel="Show sensitive media"
          audioLabels={audioLabels}
        />,
      );
      expect(screen.queryByRole("button")).not.toBeInTheDocument();

      rerender(
        <MediaGrid
          items={items}
          noDescriptionLabel="No description"
          revealLabel="Show sensitive media"
          audioLabels={audioLabels}
          viewerLabels={viewerLabels}
        />,
      );
      expect(screen.queryByRole("button")).not.toBeInTheDocument();

      rerender(
        <MediaGrid
          items={viewable}
          sensitive
          noDescriptionLabel="No description"
          revealLabel="Show sensitive media"
          audioLabels={audioLabels}
          viewerLabels={viewerLabels}
        />,
      );
      expect(screen.getAllByRole("button")).toHaveLength(1);
      await user.click(screen.getByRole("button", { name: "Show sensitive media" }));
      expect(screen.getByRole("button", { name: "A cat on a sofa" })).toBeInTheDocument();
    });
  });

  it("plays videos and audio files with controls, never by themselves, named by their descriptions", () => {
    const playables: readonly MediaItem[] = [
      { id: "v", src: "/poster.png", playback: { kind: "video", url: "/clip.mp4" }, alt: "A cat jumps" },
      { id: "a", src: "/cover.png", playback: { kind: "audio", url: "/song.mp3" }, alt: "A song" },
      { id: "n", src: undefined, playback: { kind: "audio", url: "/talk.ogg" }, alt: null },
    ];
    const { container } = render(
      <MediaGrid
        items={playables}
        noDescriptionLabel="No description"
        revealLabel="Show sensitive media"
        audioLabels={audioLabels}
      />,
    );
    // eslint-disable-next-line testing-library/no-container, testing-library/no-node-access -- <video> and <audio> have no ARIA role.
    const [video, ...audios] = Array.from(container.querySelectorAll("video, audio"));
    expect(video).toHaveAttribute("src", "/clip.mp4");
    expect(video).toHaveAttribute("poster", "/poster.png");
    expect(video).toHaveAttribute("controls");
    expect(video).not.toHaveAttribute("autoplay");
    expect(video).toHaveAccessibleName("A cat jumps");
    // An audio file is played by an AudioPlayer: the browser's controls are off, and the player is named by the description.
    expect(
      audios.map((audio) => [
        audio.getAttribute("src"),
        audio.hasAttribute("controls"),
        audio.hasAttribute("autoplay"),
      ]),
    ).toEqual([
      ["/song.mp3", false, false],
      ["/talk.ogg", false, false],
    ]);
    expect(screen.getByRole("group", { name: "A song" })).toBeInTheDocument();
    expect(screen.getByRole("group", { name: "No description" })).toBeInTheDocument();
    expect(screen.getAllByRole("button", { name: "Play" })).toHaveLength(2);
    // The poster of an audio file is decorative: its player carries the description.
    // eslint-disable-next-line testing-library/no-container, testing-library/no-node-access -- a decorative image has no role.
    expect(container.querySelector('img[src="/cover.png"]')).toHaveAttribute("alt", "");
  });

  it("loops a gifv by itself only while animations play and motion is not reduced", () => {
    const gifv: readonly MediaItem[] = [
      { id: "g", src: "/still.png", playback: { kind: "gifv", url: "/loop.mp4" }, alt: "A dancing cat" },
    ];
    const props = { items: gifv, noDescriptionLabel: "No description", revealLabel: "Show sensitive media" };
    const { container, rerender } = render(<MediaGrid {...props} audioLabels={audioLabels} />);
    // eslint-disable-next-line testing-library/no-container, testing-library/no-node-access -- a <video> has no ARIA role.
    const video = () => container.querySelector("video");
    expect(video()).toHaveAttribute("autoplay");
    expect(video()).toHaveAttribute("loop");
    expect(video()).not.toHaveAttribute("controls");

    rerender(
      <AutoplayContext value={false}>
        <MediaGrid {...props} audioLabels={audioLabels} />
      </AutoplayContext>,
    );
    expect(video()).not.toHaveAttribute("autoplay");
    expect(video()).toHaveAttribute("controls");

    const matchMedia = vi.fn((query: string) => ({
      matches: query === "(prefers-reduced-motion: reduce)",
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
    }));
    vi.stubGlobal("matchMedia", matchMedia);
    try {
      rerender(<MediaGrid key="reduced" {...props} audioLabels={audioLabels} />);
      expect(video()).not.toHaveAttribute("autoplay");
      expect(video()).toHaveAttribute("controls");
    } finally {
      vi.unstubAllGlobals();
    }
  });
});
