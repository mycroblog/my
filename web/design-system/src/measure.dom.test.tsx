// SPDX-License-Identifier: AGPL-3.0-only

// What the design system times by itself (ADR-0167 §1, §2), so that no feature does: a press of a control or of a key
// named by the ID of its action (`data-action`, what `useAction` gives), from the input to the next paint, and a
// change of screen by its kind. A disabled control, a control of no action, and a key whose action did nothing are
// not timed, and nothing is timed for a destination that stopped hearing.

import { afterEach, beforeEach, expect, test } from "bun:test";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { Button } from "./button";
import { measureWith, timeTransition, type InteractionSample } from "./measure";
import { transitionKinds } from "./motion";
import { ShortcutProvider, useAction } from "./shortcuts";

const keymap = {
  "shortcuts.help": { keys: ["?"] },
  "post.reply": { keys: ["r"] },
  "timeline.next": { keys: ["j"] },
};

/** Keeps the thread busy for `ms`, as an action that is slow to respond does. */
function busyFor(ms: number): void {
  const until = performance.now() + ms;
  let turns = 0;
  while (performance.now() < until) turns += 1;
  if (turns < 0) throw new Error("unreachable");
}

let samples: InteractionSample[] = [];
let stopMeasuring: (() => void) | null = null;

beforeEach(() => {
  samples = [];
  stopMeasuring = measureWith((sample) => {
    samples.push(sample);
  });
});

afterEach(() => {
  stopMeasuring?.();
  stopMeasuring = null;
});

/** Waits for the next frame to be painted, and the task after it. */
const painted = () =>
  new Promise<void>((resolve) => {
    requestAnimationFrame(() => setTimeout(resolve, 0));
  });

const presses = () => samples.filter((s) => s.metric === "press");

/** A reply button that declares its action, whose press takes `slow` ms, and an action of the screen that does nothing. */
function Screen({ slow = 0, disabled = false }: { slow?: number; disabled?: boolean }) {
  const reply = useAction(
    "post.reply",
    () => {
      busyFor(slow);
    },
    { scope: "screen", label: "Reply" },
  );
  useAction("timeline.next", () => false, { scope: "screen", label: "Go to next post" });
  return (
    <>
      <Button
        {...reply}
        isDisabled={disabled}
        onPress={() => {
          busyFor(slow);
        }}
      >
        Reply
      </Button>
      <Button data-action="post.favourite" onPress={() => undefined}>
        Favourite
      </Button>
      <Button onPress={() => undefined}>Plain</Button>
    </>
  );
}

const shown = (props: { slow?: number; disabled?: boolean } = {}) =>
  render(
    <ShortcutProvider keymap={keymap} mac={false}>
      <Screen {...props} />
    </ShortcutProvider>,
  );

test("a press of a control is named by its action and timed from the input to the next paint", async () => {
  const user = userEvent.setup();
  shown({ slow: 60 });
  expect(screen.getByRole("button", { name: "Reply" }).getAttribute("data-action")).toBe("post.reply");

  await user.click(screen.getByRole("button", { name: "Reply" }));
  await waitFor(() => {
    expect(presses().map((s) => s.name)).toEqual(["post.reply"]);
  });
  expect(presses()[0]?.value).toBeGreaterThanOrEqual(60);
});

test("a control named by data-action alone is timed by that name", async () => {
  const user = userEvent.setup();
  shown();
  await user.click(screen.getByRole("button", { name: "Favourite" }));
  await waitFor(() => {
    expect(presses().map((s) => s.name)).toEqual(["post.favourite"]);
  });
  expect(presses()[0]?.value).toBeGreaterThanOrEqual(0);
});

test("a key's press is timed by the action it ran", async () => {
  const user = userEvent.setup();
  shown({ slow: 30 });
  await user.keyboard("r");
  await waitFor(() => {
    expect(presses().map((s) => s.name)).toEqual(["post.reply"]);
  });
  expect(presses()[0]?.value).toBeGreaterThanOrEqual(30);
});

test("a key whose action did nothing and a control of no action are not timed", async () => {
  const user = userEvent.setup();
  shown();
  await user.keyboard("j");
  await user.click(screen.getByRole("button", { name: "Plain" }));
  // A press that is timed, after them: once its sample came, theirs would have come too.
  await user.click(screen.getByRole("button", { name: "Favourite" }));
  await waitFor(() => {
    expect(presses().map((s) => s.name)).toContain("post.favourite");
  });
  expect(presses().map((s) => s.name)).toEqual(["post.favourite"]);
});

test("a disabled control is not timed", async () => {
  const user = userEvent.setup();
  shown({ disabled: true });
  await user.click(screen.getByRole("button", { name: "Reply" }));
  await user.click(screen.getByRole("button", { name: "Favourite" }));
  await waitFor(() => {
    expect(presses().map((s) => s.name)).toContain("post.favourite");
  });
  expect(presses().map((s) => s.name)).toEqual(["post.favourite"]);
});

test.each(transitionKinds.map((kind) => [kind]))("a change of screen of kind %s is named by its kind", async (kind) => {
  const end = timeTransition(kind);
  busyFor(20);
  end();
  await waitFor(() => {
    expect(samples).toHaveLength(1);
  });
  expect(samples[0]?.metric).toBe("transition");
  expect(samples[0]?.name).toBe(kind);
  expect(samples[0]?.value).toBeGreaterThanOrEqual(20);
});

test("nothing is timed for a destination that stopped hearing, and a new one hears what follows", async () => {
  const user = userEvent.setup();
  stopMeasuring?.();
  stopMeasuring = null;
  shown();
  timeTransition("push")();
  await user.click(screen.getByRole("button", { name: "Favourite" }));
  await painted();
  await painted();

  const later: InteractionSample[] = [];
  stopMeasuring = measureWith((sample) => {
    later.push(sample);
  });
  await user.click(screen.getByRole("button", { name: "Reply" }));
  await waitFor(() => {
    expect(later.map((s) => s.name)).toContain("post.reply");
  });
  expect(later.map((s) => s.name)).toEqual(["post.reply"]);
  expect(samples).toEqual([]);
});
