// SPDX-License-Identifier: AGPL-3.0-only

// The parts of a voice chat room (voice.tsx) with reduced motion, as the catalog's tests run: a speaking
// participant's ring stands still; reactions fade in place and leave once done; and a press over a reaction reaches
// what is under it. Neither simulation runs CSS, so this is read in a browser; the motion itself is the stories'.

import type { Locator } from "@playwright/test";
import { expect, test } from "../test";

const stage = "design-system-voice-chat--stage-tiles";
const moving = "design-system-voice-chat--reactions-moving";
const still = "design-system-voice-chat--reactions";

/** A height that holds the whole stage and the reactions over it in the viewport. */
const height = 1200;

/** How many animations run on the element and inside it. */
const running = (part: Locator): Promise<number> =>
  part.evaluate(
    (element) => element.getAnimations({ subtree: true }).filter((motion) => motion.playState === "running").length,
  );

/** A sample of where a part stands and how much of it shows: its top, and the opacity it is painted with. */
type Sample = { top: number; opacity: number };

/** Samples of `part`, one a frame for `frames` frames, read in the page so that no round trip spaces them. */
const samples = (part: Locator, frames: number): Promise<Sample[]> =>
  part.evaluate(
    (element, count) =>
      new Promise<Sample[]>((resolve) => {
        const read = (): Sample => {
          let opacity = 1;
          for (let at: Element | null = element; at !== null; at = at.parentElement)
            opacity *= Number(getComputedStyle(at).opacity);
          return { top: element.getBoundingClientRect().top, opacity };
        };
        const taken: Sample[] = [];
        const next = () => {
          taken.push(read());
          if (taken.length >= count) resolve(taken);
          else requestAnimationFrame(next);
        };
        requestAnimationFrame(next);
      }),
    frames,
  );

/** How far the part moved up or down over the samples, in CSS px. */
const travel = (taken: Sample[]): number => {
  const tops = taken.map((sample) => sample.top);
  return Math.max(...tops) - Math.min(...tops);
};

/** How far the part's opacity changed over the samples. */
const fading = (taken: Sample[]): number => {
  const opacities = taken.map((sample) => sample.opacity);
  return Math.max(...opacities) - Math.min(...opacities);
};

test("a speaking participant's ring stands still with reduced motion", async ({ page, open }) => {
  const speaking = page.getByRole("button", { name: "Mika, Host, Speaking", exact: true });
  const speakingWithoutImage = page.getByRole("button", { name: "Aoi Tanaka, Speaking", exact: true });

  await open(stage, { height });
  await expect(speaking).toBeVisible();
  await expect.poll(() => running(speaking)).toBe(0);
  await expect.poll(() => running(speakingWithoutImage)).toBe(0);
});

test("with reduced motion a reaction fades in place, and leaves once done", async ({ page, open }) => {
  await open(moving, { height });
  // The second reaction of the story, there from its first draw; the same emoji comes again seconds later.
  const reaction = page.getByText("😂", { exact: true });
  await expect(reaction).toBeAttached();
  const first = await reaction.elementHandle();

  const taken = await samples(reaction, 30);
  expect(travel(taken)).toBeLessThan(0.5);
  expect(fading(taken)).toBeGreaterThan(0.05);
  // The story drops a reaction once the flow says it is done.
  await expect.poll(() => first.evaluate((element) => element.isConnected)).toBe(false);
});

test("a press over a reaction reaches what is under it", async ({ page, open }) => {
  await open(still, { height });
  const reaction = page.getByText("👏", { exact: true });
  await expect(reaction).toBeVisible();

  // What a press at the reaction's centre hits is outside the reactions' layer.
  await expect
    .poll(() =>
      reaction.evaluate((element) => {
        const layer = element.closest('[aria-hidden="true"]');
        const box = element.getBoundingClientRect();
        const top = document.elementFromPoint(box.left + box.width / 2, box.top + box.height / 2);
        return { layer: layer !== null, hitInLayer: top !== null && layer?.contains(top) === true };
      }),
    )
    .toEqual({ layer: true, hitInLayer: false });
});
