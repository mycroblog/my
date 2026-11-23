// SPDX-License-Identifier: AGPL-3.0-only

import { expect, test } from "bun:test";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { I18nProvider } from "react-aria-components";
import type { MediaItem } from "./media";
import { MediaViewer } from "./viewer";

/** A picture with a description, one without, and a video. */
const items: MediaItem[] = [
  { id: "a", src: "https://m.example/a-small.webp", href: "https://m.example/a.webp", alt: "A ridge at dawn" },
  { id: "b", src: "https://m.example/b-small.webp", href: "https://m.example/b.webp", alt: "" },
  {
    id: "v",
    kind: "video",
    src: "https://m.example/v-poster.webp",
    href: "https://m.example/v.webm",
    alt: "Waves at dusk",
    duration: 92,
  },
];

type Calls = { moves: number[]; closes: number[] };

/**
 * A post's button that opens the viewer at `at` among `media`, as a post holds it open, and what the viewer asked
 * for: the moves, which the post follows, and the closes, which it obeys.
 */
function Post({
  media,
  at,
  locale,
  calls,
}: {
  media: MediaItem[];
  at: number;
  locale?: string | undefined;
  calls: Calls;
}) {
  const [index, setIndex] = useState(at);
  const [open, setOpen] = useState(false);
  return (
    <>
      <button
        type="button"
        onClick={() => {
          setOpen(true);
        }}
      >
        Open media
      </button>
      {open && (
        <MediaViewer
          items={media}
          index={index}
          onIndexChange={(next) => {
            calls.moves.push(next);
            setIndex(next);
          }}
          onClose={() => {
            calls.closes.push(1);
            setOpen(false);
          }}
          locale={locale}
        />
      )}
    </>
  );
}

/** Opens the viewer from the post's button, and gives the dialog and what it asked for. */
async function opened(
  user: ReturnType<typeof userEvent.setup>,
  {
    at = 0,
    media = items,
    locale,
    name = "Media",
  }: { at?: number; media?: MediaItem[]; locale?: string; name?: string } = {},
) {
  const calls: Calls = { moves: [], closes: [] };
  render(<Post media={media} at={at} locale={locale} calls={calls} />);
  await user.click(screen.getByRole("button", { name: "Open media" }));
  const dialog = await screen.findByRole("dialog", { name });
  return { dialog, calls };
}

/** What the viewer says to assistive technology as the reader moves. */
const announced = (dialog: HTMLElement) => dialog.querySelector('[aria-live="polite"]')?.textContent;

test("the viewer is a dialog named Media that has the focus and says where it is among the post's media", async () => {
  const user = userEvent.setup();
  const { dialog } = await opened(user, { at: 1 });

  expect(dialog.contains(document.activeElement)).toBe(true);
  expect(within(dialog).getByText("2 of 3")).not.toBeNull();
});

test("the viewer shows a picture from its largest file, its description read once, as its caption", async () => {
  const user = userEvent.setup();
  const { dialog } = await opened(user);

  // The picture itself says nothing: the caption under it is its description.
  const picture = within(dialog).getByRole("presentation");
  expect(picture.getAttribute("src")).toBe("https://m.example/a.webp");
  expect(within(dialog).getAllByText("A ridge at dawn")).toHaveLength(1);
  expect(within(dialog).queryByRole("img")).toBeNull();
});

test("a picture zooms in and back out by one button; a video plays with the browser's controls and has no zoom", async () => {
  const user = userEvent.setup();
  const { dialog } = await opened(user);

  await user.click(within(dialog).getByRole("button", { name: "Zoom in" }));
  expect(within(dialog).queryByRole("button", { name: "Zoom in" })).toBeNull();
  await user.click(within(dialog).getByRole("button", { name: "Zoom out" }));
  expect(within(dialog).getByRole("button", { name: "Zoom in" })).not.toBeNull();

  await user.keyboard("{ArrowRight}{ArrowRight}");
  expect(within(dialog).queryByRole("button", { name: /^Zoom/ })).toBeNull();
  // A video has no role: the element is what plays it.
  const video = dialog.querySelector("video");
  expect(video?.getAttribute("src")).toBe("https://m.example/v.webm");
  expect(video?.hasAttribute("controls")).toBe(true);
  expect(within(dialog).getByText("Waves at dusk")).not.toBeNull();
});

test("the viewer has Close always, and Previous and Next where there is one, which ask for the move", async () => {
  const user = userEvent.setup();
  const { dialog, calls } = await opened(user);
  const button = (name: string) => within(dialog).queryByRole("button", { name });

  expect(button("Close")).not.toBeNull();
  expect(button("Previous")).toBeNull();
  await user.click(within(dialog).getByRole("button", { name: "Next" }));
  expect(within(dialog).getByText("2 of 3")).not.toBeNull();
  expect(button("Previous")).not.toBeNull();
  await user.click(within(dialog).getByRole("button", { name: "Next" }));
  expect(button("Next")).toBeNull();
  await user.click(within(dialog).getByRole("button", { name: "Previous" }));

  expect(calls.moves).toEqual([1, 2, 1]);
  expect(button("Close")).not.toBeNull();
});

test("the arrows move to the next and the previous, and never past the ends", async () => {
  const user = userEvent.setup();
  const { calls } = await opened(user);

  await user.keyboard("{ArrowLeft}");
  expect(calls.moves).toEqual([]);
  await user.keyboard("{ArrowRight}{ArrowRight}{ArrowRight}");
  expect(calls.moves).toEqual([1, 2]);
  await user.keyboard("{ArrowLeft}");
  expect(calls.moves).toEqual([1, 2, 1]);
});

test("in a right-to-left locale the arrows move the other way", async () => {
  const user = userEvent.setup();
  const calls: Calls = { moves: [], closes: [] };
  render(
    <I18nProvider locale="ar-XB">
      <Post media={items} at={0} calls={calls} />
    </I18nProvider>,
  );
  await user.click(screen.getByRole("button", { name: "Open media" }));
  await screen.findByRole("dialog");

  await user.keyboard("{ArrowRight}");
  expect(calls.moves).toEqual([]);
  await user.keyboard("{ArrowLeft}");
  expect(calls.moves).toEqual([1]);
  await user.keyboard("{ArrowRight}");
  expect(calls.moves).toEqual([1, 0]);
});

test("the viewer announces nothing as it opens, and each move with what it shows now", async () => {
  const user = userEvent.setup();
  const { dialog } = await opened(user);
  expect(announced(dialog)).toBe("");

  await user.keyboard("{ArrowRight}");
  expect(announced(dialog)).toBe("2 of 3, image");
  await user.keyboard("{ArrowRight}");
  expect(announced(dialog)).toBe("3 of 3, Waves at dusk");
  await user.keyboard("{ArrowLeft}{ArrowLeft}");
  expect(announced(dialog)).toBe("1 of 3, A ridge at dawn");
});

for (const [how, close] of [
  ["Close", (user: ReturnType<typeof userEvent.setup>) => user.click(screen.getByRole("button", { name: "Close" }))],
  ["Escape", (user: ReturnType<typeof userEvent.setup>) => user.keyboard("{Escape}")],
] as const)
  test(`${how} closes the viewer, and the focus is back on what opened it`, async () => {
    const user = userEvent.setup();
    const { calls } = await opened(user);

    await close(user);

    expect(calls.closes).toHaveLength(1);
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(document.activeElement).toBe(screen.getByRole("button", { name: "Open media" }));
  });

/** The stage around the shown picture: the backing a press on closes the viewer. */
const stageOf = (dialog: HTMLElement): HTMLElement => {
  const stage = dialog.querySelector<HTMLElement>("[data-viewer-stage]");
  if (stage === null) throw new Error("the viewer shows no stage");
  return stage;
};

/**
 * A mouse's press on `target` at `from`, let go at `to`, with the focus and the click it makes, once the viewer has
 * taken the focus as it opens.
 */
async function press(
  user: ReturnType<typeof userEvent.setup>,
  dialog: HTMLElement,
  target: Element,
  from: { x: number; y: number },
  to = from,
) {
  await waitFor(() => {
    expect(dialog.contains(document.activeElement)).toBe(true);
  });
  await user.pointer([
    { keys: "[MouseLeft>]", target, coords: { clientX: from.x, clientY: from.y } },
    { pointerName: "mouse", target, coords: { clientX: to.x, clientY: to.y } },
    { keys: "[/MouseLeft]", target, coords: { clientX: to.x, clientY: to.y } },
  ]);
}

test("a press on the backing around the picture closes the viewer, and the focus is back on what opened it", async () => {
  const user = userEvent.setup();
  const { dialog, calls } = await opened(user);

  await press(user, dialog, stageOf(dialog), { x: 10, y: 10 });

  expect(calls.closes).toHaveLength(1);
  expect(screen.queryByRole("dialog")).toBeNull();
  // The modal gives the focus back once it has unmounted, on the next frame.
  await waitFor(() => {
    expect(document.activeElement).toBe(screen.getByRole("button", { name: "Open media" }));
  });
});

test("a drag from the backing, or a press on the picture, keeps the viewer open", async () => {
  const user = userEvent.setup();
  const { dialog, calls } = await opened(user);
  const stage = stageOf(dialog);

  await press(user, dialog, stage, { x: 10, y: 10 }, { x: 10, y: 60 });
  await press(user, dialog, within(stage).getByRole("presentation"), { x: 10, y: 10 });

  expect(calls.closes).toEqual([]);
  expect(screen.getByRole("dialog", { name: "Media" })).not.toBeNull();
});

test("Tab never takes the focus out of the viewer", async () => {
  const user = userEvent.setup();
  const { dialog } = await opened(user, { at: 1 });

  // More presses than it has buttons, each way: the focus wraps inside it.
  for (const shift of [false, false, false, false, false, true, true, true, true, true, true]) {
    await user.tab({ shift });
    expect(dialog.contains(document.activeElement)).toBe(true);
  }
});

test("a single picture says no position and has nowhere to move", async () => {
  const user = userEvent.setup();
  const { dialog, calls } = await opened(user, { media: items.slice(0, 1) });

  expect(within(dialog).queryByText(/ of /)).toBeNull();
  expect(within(dialog).queryByRole("button", { name: /^(Previous|Next)$/ })).toBeNull();
  await user.keyboard("{ArrowRight}{ArrowLeft}");
  expect(calls.moves).toEqual([]);
});

test("the viewer reads in the reader's locale", async () => {
  const user = userEvent.setup();
  const { dialog } = await opened(user, { at: 1, locale: "ja", name: "メディア" });

  expect(within(dialog).getByText("3 件中 2 件目")).not.toBeNull();
  for (const name of ["閉じる", "前へ", "次へ"]) expect(within(dialog).getByRole("button", { name })).not.toBeNull();
});

test("the viewer in the flow of a page is a dialog named Media too", () => {
  render(
    <MediaViewer
      items={items}
      index={1}
      onIndexChange={() => undefined}
      onClose={() => undefined}
      layer={false}
      locale="en"
    />,
  );

  const dialog = screen.getByRole("dialog", { name: "Media" });
  expect(within(dialog).getByText("2 of 3")).not.toBeNull();
  expect(within(dialog).getByRole("button", { name: "Close" })).not.toBeNull();
});
