// SPDX-License-Identifier: AGPL-3.0-only

// Display and posting, the settings' first entry (#3199): each control shows what the server holds and saves a change
// at once with a PATCH /preferences of its own, and says on its row how the save went: saving, saved for about two
// seconds, or not saved, with the control back at the server's value and the way to try the same change again, or
// with the server's own words when it refuses the value. Before the preferences are read the screen says that it
// loads them, and when they cannot be read, that too. What a change does to the app (its language, its times, its
// timelines) is the app's test and each feature's; what shows while a change is on its way, preferences.dom.test.tsx.

import { describe, expect, onTestFinished, test } from "bun:test";
import { instanceQuery, type PreferencesChange } from "@my/app-data";
import type { FakeOptions } from "@my/e2e/fake";
import { act, screen, waitFor, within } from "@testing-library/react";
import type { UserEvent } from "@testing-library/user-event";
import { fakeData, renderFeature, type FakeData } from "../test/feature";
import { SettingsEntryScreen } from "./screen";

/** The languages the app has words in, which Display language offers. */
const languages = ["en", "ja"];

/**
 * Draws Display and posting on the server `on`, in the main landmark as the shell places it, once the shell read the
 * instance, and returns the reader's hands. The server's answers reach the screen when they come, not inside an act()
 * of the test, so React is told so until the test ends, and a test waits for what it asserts.
 */
async function drawn({ data }: FakeData): Promise<UserEvent> {
  const environment = globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean };
  const wrapped = environment.IS_REACT_ACT_ENVIRONMENT;
  environment.IS_REACT_ACT_ENVIRONMENT = false;
  onTestFinished(() => {
    if (wrapped === undefined) delete environment.IS_REACT_ACT_ENVIRONMENT;
    else environment.IS_REACT_ACT_ENVIRONMENT = wrapped;
  });
  await data.queryClient.query(instanceQuery(data));
  return renderFeature(
    data,
    <main>
      <SettingsEntryScreen entry="display" onBack={() => undefined} languages={languages} />
    </main>,
  );
}

/** Display and posting on a server of `options`, with its controls drawn: the preferences are read. */
async function read(options: FakeOptions = {}) {
  const on = fakeData(options);
  const user = await drawn(on);
  await screen.findByRole("combobox", { name: "Display language" });
  return { ...on, user };
}

/** The choice named `name`. */
const choice = (name: string) => screen.getByRole<HTMLSelectElement>("combobox", { name });
/** What the choice named `name` reads: the words of the option it is at. */
const chosen = (name: string) => choice(name).selectedOptions[0]?.textContent;
/** The options the choice named `name` offers, by their words. */
const offered = (name: string) =>
  within(choice(name))
    .getAllByRole("option")
    .map((option) => option.textContent);
/** The switch named `name`. */
const toggle = (name: string) => screen.getByRole<HTMLInputElement>("switch", { name });
/** The section of the screen titled `name`. */
const section = (name: string) => within(screen.getByRole("region", { name }));
/** What the rows of the section titled `name` say of a save. */
const saves = (name: string) =>
  section(name)
    .queryAllByRole("status")
    .map((status) => status.textContent);
/** The languages the timelines show, as the screen lists them. */
const shownLanguages = () =>
  screen
    .queryAllByRole("list", { name: "Languages to show" })
    .flatMap((list) => within(list).getAllByRole("button"))
    .map((button) => button.getAttribute("aria-label") ?? button.textContent);

const describing = "Ask me for a description before posting media without one";
const arriving = "Show new posts as they arrive";

/** An account's own choices, none of them a new account's. */
const own = {
  locale: "ja",
  locale_follows_browser: false,
  time_zone: "Asia/Tokyo",
  time_zone_chosen: true,
  time_zone_follows_browser: false,
  link_card_size: "small",
  default_visibility: "private",
  default_interaction_policy: {
    reply: { automatic: "everyone" },
    announce: { automatic: "everyone" },
    quote: { automatic: "followers" },
  },
  default_post_language: "ja",
  require_media_description: true,
  timeline_languages: ["en", "ja"],
  timeline_auto_insert: false,
} as const satisfies FakeOptions["preferences"];

describe("each control is at what the server holds", () => {
  test("a new account's: the language and the time zone follow the browser, and the timelines show every language", async () => {
    await read();

    expect(chosen("Display language")).toBe("Same as the browser");
    expect(chosen("Time zone")).toBe("Same as the browser");
    expect(chosen("Link previews in posts")).toBe("Large");
    expect(chosen("Who can see it")).toBe("Public");
    expect(chosen("Who can quote")).toBe("Anyone");
    expect(chosen("Post language")).toBe("Same as the display language");
    expect(toggle(describing).checked).toBe(false);
    expect(chosen("Languages to show")).toBe("All languages");
    expect(shownLanguages()).toEqual([]);
    expect(toggle(arriving).checked).toBe(true);
  });

  test("an account's own choices, the timelines' languages each with its Remove", async () => {
    await read({ preferences: own });

    expect(chosen("Display language")).toBe("日本語");
    expect(chosen("Time zone")).toBe("Tokyo (GMT+9)");
    expect(chosen("Link previews in posts")).toBe("Small");
    expect(chosen("Who can see it")).toBe("Followers only");
    expect(chosen("Who can quote")).toBe("Followers");
    expect(chosen("Post language")).toBe("日本語");
    expect(toggle(describing).checked).toBe(true);
    expect(shownLanguages()).toEqual(["Remove English", "Remove 日本語"]);
    expect(toggle(arriving).checked).toBe(false);
  });

  test("the display language offers the languages the app has words in, each in its own name", async () => {
    await read();

    expect(offered("Display language")).toEqual(["Same as the browser", "English", "日本語"]);
  });
});

/** A change a reader makes on the screen: where the account starts, what they do, and what the server is sent. */
type Change = {
  what: string;
  from?: FakeOptions["preferences"];
  act: (user: UserEvent) => Promise<void>;
  sends: PreferencesChange;
};

const pick = (name: string, option: string) => (user: UserEvent) => user.selectOptions(choice(name), option);
const press = (name: string) => (user: UserEvent) => user.click(screen.getByRole("button", { name }));
const flip = (name: string) => (user: UserEvent) => user.click(toggle(name));

const changes: readonly Change[] = [
  { what: "a display language", act: pick("Display language", "日本語"), sends: { locale: "ja" } },
  {
    what: "the display language back to the browser's",
    from: own,
    act: pick("Display language", "Same as the browser"),
    sends: { locale_follows_browser: true },
  },
  { what: "a time zone", act: pick("Time zone", "Tokyo (GMT+9)"), sends: { time_zone: "Asia/Tokyo" } },
  {
    what: "the time zone back to the browser's",
    from: own,
    act: pick("Time zone", "Same as the browser"),
    sends: { time_zone_follows_browser: true },
  },
  { what: "small link previews", act: pick("Link previews in posts", "Small"), sends: { link_card_size: "small" } },
  {
    what: "who can see a new post",
    act: pick("Who can see it", "Followers only"),
    sends: { default_visibility: "private" },
  },
  {
    what: "who can quote a new post",
    act: pick("Who can quote", "Only you"),
    sends: { default_interaction_policy: { quote: { automatic: "nobody" } } },
  },
  { what: "a post language", act: pick("Post language", "日本語"), sends: { default_post_language: "ja" } },
  {
    what: "the post language back to the display language",
    from: own,
    act: pick("Post language", "Same as the display language"),
    sends: { default_post_language: "und" },
  },
  { what: "asking for media descriptions", act: flip(describing), sends: { require_media_description: true } },
  {
    what: "a first language of the timelines",
    act: pick("Languages to show", "日本語"),
    sends: { timeline_languages: ["ja"] },
  },
  {
    what: "one more language of the timelines",
    from: { timeline_languages: ["en", "ja"] },
    act: pick("Languages to show", "Deutsch"),
    sends: { timeline_languages: ["en", "ja", "de"] },
  },
  {
    what: "a language of the timelines removed",
    from: { timeline_languages: ["en", "ja"] },
    act: press("Remove English"),
    sends: { timeline_languages: ["ja"] },
  },
  {
    what: "the last language of the timelines removed",
    from: { timeline_languages: ["ja"] },
    act: press("Remove 日本語"),
    sends: { timeline_languages: [] },
  },
  { what: "holding new posts back", act: flip(arriving), sends: { timeline_auto_insert: false } },
];

describe("a change is saved at once, with a PATCH of that preference alone", () => {
  for (const { what, from, act: change, sends } of changes)
    test(what, async () => {
      const { fake, user } = await read(from === undefined ? {} : { preferences: from });
      const sent = fake.script("PATCH /preferences");

      await change(user);

      expect((await sent.served()).body).toEqual(sends);
      expect(sent.seen).toHaveLength(1);
    });
});

describe("the timelines' languages", () => {
  test("a language added is listed with its Remove, and the picker offers another", async () => {
    const { fake, user } = await read();
    const sent = fake.script("PATCH /preferences");

    await user.selectOptions(choice("Languages to show"), "日本語");
    await sent.served();

    await waitFor(() => {
      expect(shownLanguages()).toEqual(["Remove 日本語"]);
    });
    expect(offered("Languages to show")).not.toContain("日本語");
    expect(offered("Languages to show")).toContain("English");
  });

  test("the last language removed, the timelines show all languages again", async () => {
    const { fake, user } = await read({ preferences: { timeline_languages: ["ja"] } });
    const sent = fake.script("PATCH /preferences");

    await user.click(screen.getByRole("button", { name: "Remove 日本語" }));
    await sent.served();

    await waitFor(() => {
      expect(shownLanguages()).toEqual([]);
    });
    expect(chosen("Languages to show")).toBe("All languages");
  });
});

/**
 * Keeps what the screen waits about two seconds for (from 1.5 s to 3 s) until the test lets it come, from now until
 * the test ends: `asked()` is how many such waits began, and `pass()` lets them come at once. What Testing Library
 * waits for is shorter, what the data's cache waits for longer, and both keep their time.
 */
function twoSecondWaits() {
  const wait = globalThis.setTimeout;
  const held: (() => void)[] = [];
  const holding = ((handler: TimerHandler, timeout?: number, ...rest: unknown[]) => {
    if (typeof handler !== "function" || timeout === undefined || timeout < 1500 || timeout > 3000)
      return wait(handler, timeout, ...rest);
    held.push(() => {
      (handler as (...given: unknown[]) => void)(...rest);
    });
    return wait(() => undefined, 0);
  }) as typeof setTimeout;
  globalThis.setTimeout = Object.assign(holding, wait);
  onTestFinished(() => {
    globalThis.setTimeout = wait;
  });
  return {
    asked: () => held.length,
    pass: () => {
      act(() => {
        for (const come of held.splice(0)) come();
      });
    },
  };
}

describe("the row of a control says how its save goes", () => {
  test("Saving… until the server answers, then Saved, which goes after about two seconds", async () => {
    const { fake, user } = await read();
    const waits = twoSecondWaits();
    const sent = fake.script("PATCH /preferences", { hold: "response" });
    expect(saves("Display")).toEqual([]);

    await user.selectOptions(choice("Link previews in posts"), "Small");
    await sent.arrived();

    expect(saves("Display")).toEqual(["Saving…"]);
    // The other rows say nothing: the save is this control's.
    expect(screen.getAllByRole("status")).toHaveLength(1);

    sent.release();
    await waitFor(() => {
      expect(saves("Display")).toEqual(["Saved"]);
    });
    expect(chosen("Link previews in posts")).toBe("Small");
    // It stays for as long as the screen waits, about two seconds, and no longer.
    expect(waits.asked()).toBe(1);

    waits.pass();
    await waitFor(() => {
      expect(saves("Display")).toEqual([]);
    });
  });

  test("a save the server fails says so with Try again, and the control is back at the server's value", async () => {
    const { user } = await read({ preferencesFail: "save" });

    await user.selectOptions(choice("Link previews in posts"), "Small");

    await waitFor(() => {
      expect(saves("Display").join()).toContain("This could not be saved.");
    });
    expect(section("Display").getAllByRole("button", { name: "Try again" })).toHaveLength(1);
    expect(chosen("Link previews in posts")).toBe("Large");
  });

  test("Try again sends the same change, and once the server keeps it the row says Saved", async () => {
    const { fake, user } = await read();
    const sent = fake.script("PATCH /preferences");
    const failing = fake.script("PATCH /preferences", { times: 1, fail: 500 });
    await user.click(toggle(arriving));
    await section("Timelines").findByRole("button", { name: "Try again" });
    expect(toggle(arriving).checked).toBe(true);
    failing.end();

    await user.click(section("Timelines").getByRole("button", { name: "Try again" }));

    await waitFor(() => {
      expect(saves("Timelines")).toEqual(["Saved"]);
    });
    expect(sent.seen.map((request) => request.body)).toEqual([
      { timeline_auto_insert: false },
      { timeline_auto_insert: false },
    ]);
    expect(toggle(arriving).checked).toBe(false);
    expect(section("Timelines").queryAllByRole("button", { name: "Try again" })).toHaveLength(0);
  });

  test("a value the server refuses shows the server's words about it, with nothing to try again", async () => {
    const { fake, user } = await read();
    fake.script("PATCH /preferences", {
      times: 1,
      fail: {
        type: "about:blank",
        title: "Unprocessable Content",
        status: 422,
        field: "time_zone",
        detail: "The server does not know this time zone.",
      },
    });

    await user.selectOptions(choice("Time zone"), "Tokyo (GMT+9)");

    await waitFor(() => {
      expect(saves("Language and region")).toEqual(["The server does not know this time zone."]);
    });
    expect(section("Language and region").queryAllByRole("button", { name: "Try again" })).toHaveLength(0);
    expect(chosen("Time zone")).toBe("Same as the browser");
  });
});

describe("who can see a new post", () => {
  test("offers Local between Public and Quiet public", async () => {
    await read();

    expect(offered("Who can see it")).toEqual(["Public", "Local", "Quiet public", "Followers only"]);
  });

  test("offers no Local on a one-person server", async () => {
    await read({ singleUser: true });

    expect(offered("Who can see it")).toEqual(["Public", "Quiet public", "Followers only"]);
  });
});

describe("before the preferences are read", () => {
  test("the screen says that it loads them, with no control, until they come", async () => {
    const on = fakeData();
    const reading = on.fake.script("GET /preferences", { hold: "request" });
    await drawn(on);
    await reading.arrived();

    expect(screen.getAllByRole("status").map((status) => status.textContent)).toEqual(["Loading your settings…"]);
    expect(screen.queryAllByRole("combobox")).toHaveLength(0);

    reading.release();
    await screen.findByRole("combobox", { name: "Display language" });
    expect(screen.queryAllByRole("status")).toHaveLength(0);
  });

  test("a read that fails says so, and Try again reads them", async () => {
    const on = fakeData();
    const reading = on.fake.script("GET /preferences", { times: 1, fail: 500 });
    const user = await drawn(on);

    const failed = await screen.findByRole("alert");
    expect(failed.textContent).toContain("Your settings could not be loaded.");
    expect(screen.queryAllByRole("combobox")).toHaveLength(0);

    await user.click(within(failed).getByRole("button", { name: "Try again" }));

    await screen.findByRole("combobox", { name: "Display language" });
    expect(reading.seen).toHaveLength(2);
    expect(screen.queryAllByRole("alert")).toHaveLength(0);
  });
});
